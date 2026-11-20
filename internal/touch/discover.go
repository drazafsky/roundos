package touch

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// DeviceInfo describes one physical input event node with touch capabilities.
// The event number is intentionally not persisted: Linux can renumber events
// after each boot or whenever devices are added.
type DeviceInfo struct {
	Path       string
	Name       string
	Multitouch bool
	Score      int
}

// DiscoverDevice locates the touchscreen by its input capabilities and udev
// classification, rather than assuming /dev/input/event0.
// It deliberately refuses an ambiguous tie instead of selecting an arbitrary
// touchscreen (which could make the shell unresponsive to touch).
func DiscoverDevice() (DeviceInfo, error) {
	return discoverDevice("/sys/class/input", "/dev/input", "/run/udev/data")
}

func discoverDevice(sysDir, devDir, udevDir string) (DeviceInfo, error) {
	paths, err := filepath.Glob(filepath.Join(sysDir, "event*"))
	if err != nil {
		return DeviceInfo{}, err
	}
	var matches []DeviceInfo
	for _, p := range paths {
		event := filepath.Base(p)
		if !isEventName(event) {
			continue
		}
		devPath := filepath.Join(devDir, event)
		if _, err := os.Stat(devPath); err != nil {
			continue
		}
		name, err := os.ReadFile(filepath.Join(p, "device", "name"))
		if err != nil {
			continue
		}
		abs, err := readCapabilities(filepath.Join(p, "device", "capabilities", "abs"))
		if err != nil {
			continue
		}
		mt := abs.Bit(absMTPositionX) == 1 && abs.Bit(absMTPositionY) == 1 &&
			abs.Bit(absMTSlot) == 1 && abs.Bit(absMTTrackingID) == 1
		key, _ := readCapabilities(filepath.Join(p, "device", "capabilities", "key"))
		single := abs.Bit(absX) == 1 && abs.Bit(absY) == 1 &&
			key != nil && key.Bit(btnTouch) == 1
		if !mt && !single {
			continue
		}

		devno, _ := os.ReadFile(filepath.Join(p, "dev"))
		props := readUdevProperties(filepath.Join(udevDir, "c"+strings.TrimSpace(string(devno))))
		// A touchpad can expose the same multitouch axes; never mistake it
		// for the display's touchscreen.
		lower := strings.ToLower(strings.TrimSpace(string(name)))
		if props["ID_INPUT_TOUCHPAD"] == "1" || strings.Contains(lower, "touchpad") || props["ID_INPUT_TABLET"] == "1" {
			continue
		}
		score := 0
		if mt {
			score += 10
		}
		if props["ID_INPUT_TOUCHSCREEN"] == "1" {
			score += 100
		}
		for _, fragment := range []string{"edt-ft5", "ft5x06", "hyperpixel", "cst830", "goodix", "touchscreen", "focaltech"} {
			if strings.Contains(lower, fragment) {
				score += 50
				break
			}
		}
		matches = append(matches, DeviceInfo{Path: devPath, Name: strings.TrimSpace(string(name)), Multitouch: mt, Score: score})
	}
	if len(matches) == 0 {
		return DeviceInfo{}, errors.New("no touchscreen found: expected a device with multitouch position/slot/tracking axes or ABS_X/ABS_Y + BTN_TOUCH")
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Path < matches[j].Path
	})
	if len(matches) > 1 && matches[0].Score == matches[1].Score {
		return DeviceInfo{}, fmt.Errorf("multiple equally likely touchscreens (%s and %s); specify -touch explicitly", matches[0].Path, matches[1].Path)
	}
	return matches[0], nil
}

func isEventName(s string) bool {
	if !strings.HasPrefix(s, "event") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimPrefix(s, "event"))
	return err == nil
}

// Input capability bitmaps may be printed in native-word-sized hex groups.
// Each group is interpreted as one kernel unsigned long; both Go and Linux
// use the architecture's word size on the 32-bit Raspberry Pi OS target.
func readCapabilities(path string) (*big.Int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parts := strings.FieldsFunc(strings.TrimSpace(string(data)), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(parts) == 0 {
		return nil, errors.New("empty capabilities")
	}
	bitmap := new(big.Int)
	for _, part := range parts {
		word, ok := new(big.Int).SetString(part, 16)
		if !ok {
			return nil, fmt.Errorf("invalid capability bitmap %q", string(data))
		}
		bitmap.Lsh(bitmap, uint(strconv.IntSize))
		bitmap.Or(bitmap, word)
	}
	return bitmap, nil
}

func readUdevProperties(path string) map[string]string {
	props := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return props
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "E:") {
			if k, v, ok := strings.Cut(line[2:], "="); ok {
				props[k] = v
			}
		}
	}
	return props
}

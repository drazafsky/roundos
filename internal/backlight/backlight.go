// Package backlight controls a kernel-managed backlight, if one exists. RoundOS
// never claims the GPIO directly: the HyperPixel shared-bus driver owns it.
package backlight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Controller struct {
	path  string
	saved int
	max   int
	off   bool
}

func readInt(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return value, nil
}

// Open recognizes "auto" or an explicit /sys/class/backlight/<name> path.
// It never alters hardware during probing. auto returns an error if no
// unambiguous, working device is found: a black framebuffer is not a low-power
// backlight implementation.
func Open(name string) (*Controller, error) {
	var dirs []string
	if name == "auto" {
		matches, err := filepath.Glob("/sys/class/backlight/*")
		if err != nil {
			return nil, err
		}
		dirs = matches
	} else {
		dirs = []string{name}
	}
	valid := []*Controller{}
	for _, dir := range dirs {
		if name != "auto" && !strings.HasPrefix(filepath.Clean(dir), "/sys/class/backlight/") {
			return nil, errors.New("backlight must be a /sys/class/backlight device")
		}
		max, err := readInt(filepath.Join(dir, "max_brightness"))
		if err != nil || max <= 0 {
			continue
		}
		brightness := filepath.Join(dir, "brightness")
		current, err := readInt(brightness)
		if err != nil {
			continue
		}
		if current < 0 || current > max {
			continue
		}
		valid = append(valid, &Controller{path: brightness, saved: current, max: max})
	}
	if len(valid) == 0 {
		return nil, errors.New("no usable kernel backlight device found")
	}
	if len(valid) != 1 {
		return nil, fmt.Errorf("multiple backlights (%d); specify a device explicitly", len(valid))
	}
	if valid[0].saved == 0 {
		valid[0].saved = valid[0].max
	}
	return valid[0], nil
}
func (c *Controller) Name() string { return filepath.Base(filepath.Dir(c.path)) }
func (c *Controller) Off() error {
	if c.off {
		return nil
	}
	if err := os.WriteFile(c.path, []byte("0\n"), 0644); err != nil {
		return err
	}
	c.off = true
	return nil
}
func (c *Controller) On() error {
	if !c.off {
		return nil
	}
	if err := os.WriteFile(c.path, []byte(strconv.Itoa(c.saved)+"\n"), 0644); err != nil {
		return err
	}
	c.off = false
	return nil
}
func (c *Controller) Restore() error { return c.On() }

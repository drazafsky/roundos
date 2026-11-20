package touch

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func bitmap(bits ...int) string {
	x := new(big.Int)
	for _, b := range bits {
		x.SetBit(x, b, 1)
	}
	return x.Text(16) + "\n"
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func fakeEvent(t *testing.T, root, event, name string, mt, single bool, property string) {
	t.Helper()
	base := filepath.Join(root, "sys", event, "device")
	writeTestFile(t, filepath.Join(base, "name"), name+"\n")
	abs := []int{}
	keys := []int{}
	if mt {
		abs = append(abs, absMTSlot, absMTTrackingID, absMTPositionX, absMTPositionY)
	}
	if single {
		abs = append(abs, absX, absY)
		keys = append(keys, btnTouch)
	}
	writeTestFile(t, filepath.Join(base, "capabilities", "abs"), bitmap(abs...))
	writeTestFile(t, filepath.Join(base, "capabilities", "key"), bitmap(keys...))
	writeTestFile(t, filepath.Join(root, "dev", event), "")
	minor, err := strconv.Atoi(strings.TrimPrefix(event, "event"))
	if err != nil {
		t.Fatal(err)
	}
	devno := fmt.Sprintf("13:%d", 64+minor)
	writeTestFile(t, filepath.Join(root, "sys", event, "dev"), devno+"\n")
	if property != "" {
		writeTestFile(t, filepath.Join(root, "udev", "c"+devno), "E:"+property+"\n")
	}
}

func discoverTest(t *testing.T, root string) (DeviceInfo, error) {
	t.Helper()
	return discoverDevice(filepath.Join(root, "sys"), filepath.Join(root, "dev"), filepath.Join(root, "udev"))
}

func TestDiscoverRenumberedHyperPixel(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event0", "keyboard", false, false, "")
	fakeEvent(t, root, "event1", "edt-ft5x06", true, false, "")
	got, err := discoverTest(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != filepath.Join(root, "dev", "event1") || !got.Multitouch {
		t.Fatalf("expected HyperPixel event1, got %+v", got)
	}
	// Reboot or device hotplug may assign another event number.
	if err := os.Rename(filepath.Join(root, "sys", "event1"), filepath.Join(root, "sys", "event4")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "dev", "event1"), filepath.Join(root, "dev", "event4")); err != nil {
		t.Fatal(err)
	}
	got, err = discoverTest(t, root)
	if err != nil || got.Path != filepath.Join(root, "dev", "event4") {
		t.Fatalf("expected event4 after renumbering, got %+v, %v", got, err)
	}
}

func TestDiscoverRejectsTouchpad(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event0", "SynPS/2 TouchPad", true, false, "ID_INPUT_TOUCHPAD=1")
	fakeEvent(t, root, "event1", "edt-ft5x06", true, false, "")
	got, err := discoverTest(t, root)
	if err != nil || !strings.HasSuffix(got.Path, "event1") {
		t.Fatalf("touchpad should be ignored: %+v, %v", got, err)
	}
}

func TestDiscoverPrefersUdevTouchscreen(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event0", "generic multitouch input", true, false, "")
	fakeEvent(t, root, "event1", "generic panel", true, false, "ID_INPUT_TOUCHSCREEN=1")
	got, err := discoverTest(t, root)
	if err != nil || !strings.HasSuffix(got.Path, "event1") {
		t.Fatalf("udev touchscreen should win: %+v, %v", got, err)
	}
}

func TestDiscoverRejectsAmbiguity(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event1", "generic input A", true, false, "")
	fakeEvent(t, root, "event2", "generic input B", true, false, "")
	_, err := discoverTest(t, root)
	if err == nil || !strings.Contains(err.Error(), "multiple equally likely") {
		t.Fatalf("expected safe ambiguity error, got %v", err)
	}
}

func TestDiscoverSingleTouchFallback(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event7", "panel touchscreen", false, true, "")
	got, err := discoverTest(t, root)
	if err != nil || got.Multitouch || !strings.HasSuffix(got.Path, "event7") {
		t.Fatalf("expected supported single-touch event7: %+v, %v", got, err)
	}
}

func TestDiscoverRejectsNonTouchAbsDevice(t *testing.T) {
	root := t.TempDir()
	fakeEvent(t, root, "event1", "joystick", false, false, "")
	_, err := discoverTest(t, root)
	if err == nil || !strings.Contains(err.Error(), "no touchscreen found") {
		t.Fatalf("expected no touchscreen, got %v", err)
	}
}

func TestReadCapabilitiesGroupedBitmap(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "abs")
	// Bit (word-size + 21) is in the upper word; bit 1 in the lower.
	writeTestFile(t, p, "200000 00000002\n")
	got, err := readCapabilities(p)
	highBit := strconv.IntSize + 21
	if err != nil || got.Bit(highBit) != 1 || got.Bit(1) != 1 || got.Bit(highBit+1) != 0 {
		t.Fatalf("grouped bitmap incorrectly parsed: %v, %v", got, err)
	}
}

package systemgestures

import (
	"roundos/internal/gesture"
	"roundos/internal/shell"
	"roundos/internal/touch"
	"testing"
	"time"
)

// This is an empirical device-compatibility fixture for the tested HyperPixel
// evdev path. Touch hit locations were correct WITHOUT Y inversion, but its
// two-finger physical upward movement arrived as direction "down". Fix only
// the optional shell shortcut mapping, never the raw positions/edge swipes.
func TestHyperPixelTwoFingerVerticalMappingKeepsTapAndEdges(t *testing.T) {
	cfg := Default()
	cfg.TwoLauncher = true
	cfg.TwoSwitch = true
	if !cfg.TwoVerticalReverse {
		t.Fatal("HyperPixel two-finger vertical compatibility profile must be enabled")
	}

	// Physical up in the hardware report was classified as raw 'down'.
	r := New(cfg)
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "down", DY: 90, Fingers: 2}); got != Launcher {
		t.Fatalf("physical up => Home: got %q", got)
	}
	// Physical down must not open Home.
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "up", DY: -90, Fingers: 2}); got != None {
		t.Fatalf("physical down should do nothing: got %q", got)
	}
	// Horizontal switching unchanged.
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "left", Fingers: 2}); got != NextApp {
		t.Fatal(got)
	}
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "right", Fingers: 2}); got != PreviousApp {
		t.Fatal(got)
	}

	// One-finger notification edge must still start at actual top (Y~20).
	r = New(cfg)
	d := gesture.New()
	when := time.Unix(1000, 0)
	var got Action
	for _, frame := range []touch.Frame{
		{At: when, Contacts: []touch.Contact{{ID: 1, X: 240, Y: 20}}},
		{At: when.Add(100 * time.Millisecond), Contacts: []touch.Contact{{ID: 1, X: 240, Y: 110}}},
		{At: when.Add(140 * time.Millisecond)},
	} {
		for _, ev := range d.Process(frame) {
			if action := r.Handle(ev); action != None {
				got = action
			}
		}
	}
	if got != Notifications {
		t.Fatalf("top edge downward swipe must open notifications, got %q", got)
	}

	// Shell's touch target in the lower part of Clock must still open Home.
	m := shell.New()
	m.Select(shell.Clock)
	if changed, _ := m.Tap(240, 95); changed && m.Page == shell.Home {
		t.Fatal("top of Clock must not activate the bottom Home control")
	}
	if changed, _ := m.Tap(240, 390); !changed || m.Page != shell.Home {
		t.Fatalf("bottom of Clock must activate Home, changed=%t page=%d", changed, m.Page)
	}
}

func TestTwoFingerVerticalShortcutMappingCanBeDisabled(t *testing.T) {
	cfg := Default()
	cfg.TwoLauncher = true
	cfg.TwoVerticalReverse = false
	r := New(cfg)
	if a := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "up", Fingers: 2}); a != Launcher {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "down", Fingers: 2}); a != None {
		t.Fatal(a)
	}
}

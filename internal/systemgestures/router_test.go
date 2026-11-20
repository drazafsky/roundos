package systemgestures

import (
	"roundos/internal/gesture"
	"testing"
)

func TestDefaultsAndEdgeDirection(t *testing.T) {
	r := New(Default())
	if a := r.Handle(gesture.Event{Type: gesture.Down, X: 22, Y: 240, Fingers: 1}); a != None {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.Drag, X: 80, Y: 241, DX: 58, DY: 1, Fingers: 1}); a != Back {
		t.Fatal("Back", a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.Swipe, X: 120, Y: 241, DX: 98, DY: 1, Fingers: 1}); a != None {
		t.Fatal("duplicate", a)
	}
	r.Reset()
	r.Handle(gesture.Event{Type: gesture.Down, X: 240, Y: 15, Fingers: 1})
	if a := r.Handle(gesture.Event{Type: gesture.Drag, DX: 2, DY: 62, Fingers: 1}); a != Notifications {
		t.Fatal(a)
	}
}
func TestCornerArbitrationAndDisabled(t *testing.T) {
	r := New(Default())
	r.Handle(gesture.Event{Type: gesture.Down, X: 78, Y: 78, Fingers: 1})
	if a := r.Handle(gesture.Event{Type: gesture.Drag, DX: 65, DY: 15, Fingers: 1}); a != Back {
		t.Fatal("horizontal", a)
	}
	r.Reset()
	r.Handle(gesture.Event{Type: gesture.Down, X: 78, Y: 78, Fingers: 1})
	if a := r.Handle(gesture.Event{Type: gesture.Drag, DX: 12, DY: 66, Fingers: 1}); a != Notifications {
		t.Fatal("vertical", a)
	}
	r.Reset()
	r.Handle(gesture.Event{Type: gesture.Down, X: 78, Y: 78, Fingers: 1})
	if a := r.Handle(gesture.Event{Type: gesture.Drag, DX: 62, DY: 59, Fingers: 1}); a != None {
		t.Fatal("diagonal", a)
	}
	c := Default()
	c.TopNotifications = false
	r = New(c)
	r.Handle(gesture.Event{Type: gesture.Down, X: 240, Y: 9, Fingers: 1})
	if a := r.Handle(gesture.Event{Type: gesture.Swipe, DX: 1, DY: 81, Fingers: 1}); a != None {
		t.Fatal("disabled", a)
	}
}
func TestAdvancedOffByDefault(t *testing.T) {
	r := New(Default())
	for _, ev := range []gesture.Event{{Type: gesture.ThreeSwipe, Direction: "left", Fingers: 3}, {Type: gesture.FourTap, Fingers: 4}} {
		if a := r.Handle(ev); a != None {
			t.Fatal(a)
		}
	}
	c := Default()
	c.MaxContacts = 5
	c.ThreeSwitch = true
	c.ThreeLauncher = true
	c.FourSwitcher = true
	r = New(c)
	if a := r.Handle(gesture.Event{Type: gesture.ThreeSwipe, Direction: "left", Fingers: 3}); a != NextApp {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.ThreeSwipe, Direction: "right", Fingers: 3}); a != PreviousApp {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.ThreeSwipe, Direction: "up", Fingers: 3}); a != Launcher {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.FourTap, Fingers: 4}); a != Switcher {
		t.Fatal(a)
	}
}

func TestHyperPixelTwoContactShortcutsAndFutureHardware(t *testing.T) {
	c := Default()
	if c.MaxContacts != 2 {
		t.Fatalf("HyperPixel max_contacts: %d", c.MaxContacts)
	}
	c.TwoSwitch = true
	c.TwoLauncher = true
	c.ThreeSwitch = true
	c.ThreeLauncher = true
	c.FourSwitcher = true
	r := New(c)
	cases := []struct {
		kind      gesture.Type
		direction string
		fingers   int
		want      Action
	}{
		{gesture.TwoSwipe, "left", 2, NextApp},
		{gesture.TwoSwipe, "right", 2, PreviousApp},
		{gesture.TwoSwipe, "up", 2, None},
		{gesture.TwoSwipe, "down", 2, Launcher},
		{gesture.ThreeSwipe, "left", 3, None},
		{gesture.FourTap, "", 4, None},
	}
	for _, tc := range cases {
		got := r.Handle(gesture.Event{Type: tc.kind, Direction: tc.direction, Fingers: tc.fingers})
		if got != tc.want {
			t.Fatalf("%s/%s/%d got=%q want=%q", tc.kind, tc.direction, tc.fingers, got, tc.want)
		}
	}
	c.MaxContacts = 5
	r = New(c)
	if a := r.Handle(gesture.Event{Type: gesture.ThreeSwipe, Direction: "left", Fingers: 3}); a != NextApp {
		t.Fatal(a)
	}
	if a := r.Handle(gesture.Event{Type: gesture.FourTap, Fingers: 4}); a != Switcher {
		t.Fatal(a)
	}
}
func TestDisabledTwoFingerSystemSwipeLeavesAppInput(t *testing.T) {
	c := Default()
	r := New(c)
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "left", Fingers: 2}); got != None {
		t.Fatal(got)
	}
	c.TwoSwitch = true
	c.MaxContacts = 1
	r = New(c)
	if got := r.Handle(gesture.Event{Type: gesture.TwoSwipe, Direction: "left", Fingers: 2}); got != None {
		t.Fatal(got)
	}
}
func TestConfigCapValidated(t *testing.T) {
	for _, max := range []int{0, -1, 17} {
		c := Default()
		c.MaxContacts = max
		if c.Valid() {
			t.Fatalf("accepted max_contacts %d", max)
		}
	}
	for _, max := range []int{1, 2, 5, 16} {
		c := Default()
		c.MaxContacts = max
		if !c.Valid() {
			t.Fatalf("rejected max_contacts %d", max)
		}
	}
}

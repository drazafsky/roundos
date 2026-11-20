package shell

import (
	"roundos/internal/wire"
	"testing"
	"time"
)

func TestNavigation(t *testing.T) {
	m := New()
	if m.Page != Home {
		t.Fatal("initial page")
	}
	m.SetView(wire.ClockID, wire.View{Title: "CLOCK", Primary: "12:30"})
	m.SetView(wire.TimerID, wire.View{Title: "TIMER", Primary: "05:00", ButtonLabel: "START", ButtonAction: "toggle", RotaryAction: "adjust"})
	if changed, action := m.Tap(150, 250); !changed || action != "" || m.Page != Clock {
		t.Fatal("clock touch")
	}
	if changed, _ := m.Tap(240, 390); !changed || m.Page != Home {
		t.Fatal("home touch")
	}
	if changed, _, _ := m.Rotate(1); !changed || m.Selected != 1 {
		t.Fatal("carousel rotary focus")
	}
	if changed, _ := m.Tap(240, 258); !changed || m.Page != Timer {
		t.Fatal("focused carousel timer activation")
	}
	if changed, action := m.Tap(240, 280); !changed || action != "toggle" {
		t.Fatalf("timer action %q", action)
	}
	if changed, action, delta := m.Rotate(3); changed || action != "adjust" || delta != 3 {
		t.Fatal("rotary app action")
	}
	if b := Draw(m, time.Now()).Bounds(); b.Dx() != 480 || b.Dy() != 480 {
		t.Fatal(b)
	}
}

func TestHyperPixelProfileInTouchDiagnostics(t *testing.T) {
	m := New()
	if m.SupportedContacts != 2 {
		t.Fatalf("default supported contacts = %d", m.SupportedContacts)
	}
	m.SupportedContacts = 5 // alternative hardware profile should remain possible
	m.Select(Diagnostics)
	if img := Draw(m, time.Now()); img.Bounds().Dx() != 480 {
		t.Fatal("invalid diagnostic display")
	}
}

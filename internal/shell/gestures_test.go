package shell

import (
	"roundos/internal/touch"
	"roundos/internal/wire"
	"testing"
	"time"
)

func TestTwoFingerSystemNavigationAndDiagnostic(t *testing.T) {
	m := New()
	if !m.TwoFingerSwipe("left") || m.Page != Timer {
		t.Fatal("two finger left should open Timer")
	}
	if !m.TwoFingerSwipe("right") || m.Page != Clock {
		t.Fatal("two finger right should open Clock")
	}
	if !m.TwoFingerSwipe("up") || m.Page != Home {
		t.Fatal("two finger up should open Home")
	}
	if changed, _ := m.Tap(240, 393); !changed || m.Page != Diagnostics {
		t.Fatal("home touch diagnostic button")
	}
	if m.TwoFingerSwipe("left") {
		t.Fatal("diagnostics should not navigate with gestures")
	}
	if !m.UpdateContacts([]touch.Contact{{ID: 42, X: 200, Y: 200}, {ID: 43, X: 270, Y: 270}}) {
		t.Fatal("no diagnostic repaint")
	}
	if !m.SetGesture("two_swipe") {
		t.Fatal("no gesture label")
	}
	if m.LastGesture != "TWO SWIPE" {
		t.Fatal(m.LastGesture)
	}
	if im := Draw(m, time.Now()); im.Bounds().Dx() != 480 {
		t.Fatal("invalid drawing")
	}
	if changed, _ := m.Tap(240, 398); !changed || m.Page != Home {
		t.Fatal("return from diagnostic")
	}
}

func TestRotaryButtonActivatesAppControl(t *testing.T) {
	m := New()
	m.SetView("org.roundos.timer", wire.View{ButtonAction: "toggle"})
	m.Select(Timer)
	changed, action := m.Activate()
	if changed || action != "toggle" || m.Page != Timer {
		t.Fatalf("button activation: changed=%v action=%s page=%d", changed, action, m.Page)
	}
}

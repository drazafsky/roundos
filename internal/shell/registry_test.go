package shell

import (
	"roundos/internal/appmgr"
	"roundos/internal/wire"
	"roundos/sdk/ui"
	"testing"
	"time"
)

func TestInstalledAppCarouselAndIPC(t *testing.T) {
	m := New()
	m.SetInstalledApps([]appmgr.AppSpec{{ID: "org.roundos.demo", Name: "Demo", Icon: "DME", Exec: "/trusted/demo"}})
	if m.CarouselCount() != 4 {
		t.Fatalf("cards: %d", m.CarouselCount())
	}
	if changed, _, _ := m.Rotate(3); !changed || m.Selected != 3 {
		t.Fatal("demo not selectable")
	}
	if ch, _ := m.Tap(240, 252); !ch || m.AppID(m.Page) != "org.roundos.demo" {
		t.Fatal("demo not activated")
	}
	view := wire.View{Screens: []wire.Screen{ui.Screen("demo.root", "DEMO", "HELLO", "", ui.Button("button", "GO", "go"))}}
	if !m.SetView("org.roundos.demo", view) {
		t.Fatal("external app view not routed")
	}
	if !m.hasWidgets(m.Page) {
		t.Fatal("widgets missing")
	}
	if ch, action := m.Activate(); !ch || action != "go" {
		t.Fatalf("action: %v %q", ch, action)
	}
	if ch := m.OpenSwitcher(); !ch {
		t.Fatal("switcher inaccessible")
	}
	if m.NextRunning(1) {
		t.Fatal("NextRunning must not activate from switcher")
	}
	m.SwitcherFocus = 7
	if !m.SelectSwitcher() || m.AppID(m.Page) != "org.roundos.demo" {
		t.Fatal("switcher app selection failed")
	}
	if !m.Back() || m.Page != Home {
		t.Fatal("app Back to Home")
	}
	if Draw(m, time.Now()).Bounds().Dx() != 480 {
		t.Fatal("draw")
	}
}
func TestRotaryPickerCancelAndCommit(t *testing.T) {
	m := New()
	v := wire.View{Screens: []wire.Screen{ui.Screen("root", "TEST", "", "", ui.NumberPicker("n", "COUNT", "save", 5, 0, 12, 1))}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	m.Activate()
	m.Rotate(2)
	if !m.CancelPicker() || m.PickerID != "" || m.WidgetDelta != 0 {
		t.Fatal("held rotary cancellation committed value")
	}
	m.Activate()
	m.Rotate(2)
	if ch, act := m.Tap(184, 397); !ch || act != "" || m.PickerID != "" {
		t.Fatal("touch Cancel")
	}
	m.Activate()
	m.Rotate(1)
	if ch, act := m.Tap(296, 397); !ch || act != "save" || m.WidgetDelta != 6 {
		t.Fatalf("tap confirm: %v %q %d", ch, act, m.WidgetDelta)
	}
}

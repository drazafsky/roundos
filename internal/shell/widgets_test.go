package shell

import (
	"image"
	"math"
	"testing"
	"time"

	"roundos/internal/circleui"
	"roundos/internal/wire"
	"roundos/sdk/ui"
)

func viewForWidgets() wire.View {
	return wire.View{Screens: []wire.Screen{
		ui.Screen("root", "SAMPLE", "12:45", "demo", ui.Link("open", "OPEN SETTINGS", "settings"), ui.Button("do", "RUN", "run")),
		ui.Screen("settings", "SETTINGS", "", "", ui.Toggle("enabled", "ENABLED", "toggle", false), ui.Link("child", "MORE", "child"), ui.Slider("brightness", "BRIGHTNESS", "set_brightness", 50, 0, 100, 5), ui.Button("end", "CLOSE", "close")),
		ui.Screen("child", "DETAILS", "", "", ui.Button("close", "CLOSE", "close")),
	}}
}
func TestCarouselFocusLaunchAndWrap(t *testing.T) {
	m := New()
	if changed, _, _ := m.Rotate(-1); !changed || m.Selected != 2 {
		t.Fatalf("wrap left: %d", m.Selected)
	}
	m.AdvanceCarousel(time.Now().Add(carouselDuration))
	if changed, _ := m.Tap(240, 250); !changed || m.Page != Settings {
		t.Fatalf("center opens Settings: %d", m.Page)
	}
	if !m.Back() || m.Page != Home {
		t.Fatal("Settings Back must return launcher")
	}
	if changed, _ := m.Tap(50, 250); !changed || m.Page != Home || m.Selected != 1 {
		t.Fatalf("neighbor tap focuses Timer: page=%d selected=%d", m.Page, m.Selected)
	}
	m.AdvanceCarousel(time.Now().Add(carouselDuration))
	if changed, _ := m.Tap(240, 250); !changed || m.Page != Timer {
		t.Fatal("center opens Timer")
	}
	if !m.Back() || m.Page != Home {
		t.Fatal("app Back to launcher")
	}
	m.Selected = 0
	if changed, _ := m.Tap(240, 250); !changed || m.Page != Clock {
		t.Fatal("center opens Clock")
	}
}
func TestWidgetNavigationFocusRestore(t *testing.T) {
	m := New()
	view := viewForWidgets()
	m.SetView(wire.ClockID, view)
	m.Select(Clock)
	if m.CurrentFocus(Clock) != 0 || m.NavDepth(Clock) != 1 {
		t.Fatal("root state")
	}
	if changed, action := m.Activate(); !changed || action != "" || m.NavDepth(Clock) != 2 {
		t.Fatal("enter settings")
	}
	if changed, _, _ := m.Rotate(2); !changed || m.CurrentFocus(Clock) != 2 {
		t.Fatal("slider focus")
	}
	if changed, action := m.Activate(); !changed || action != "" || m.EditingWidget != "brightness" {
		t.Fatalf("edit mode: %q", m.EditingWidget)
	}
	if changed, _, _ := m.Rotate(2); !changed || m.WidgetEditValue != 60 {
		t.Fatalf("slider adjust = %d", m.WidgetEditValue)
	}
	if !m.Back() || m.EditingWidget != "" || m.NavDepth(Clock) != 2 {
		t.Fatal("first Back cancels edit without popping")
	}
	if changed, action := m.Activate(); !changed || action != "" {
		t.Fatal("re-enter slider")
	}
	m.Rotate(-2)
	if changed, action := m.Activate(); !changed || action != "set_brightness" || m.WidgetDelta != 40 {
		t.Fatalf("commit action=%s delta=%d", action, m.WidgetDelta)
	}
	if !m.OpenSwitcher() || !m.Back() || m.Page != Clock || m.NavDepth(Clock) != 2 || m.CurrentFocus(Clock) != 2 {
		t.Fatal("switcher focus restoration")
	}
	// A live theme update must not modify widget navigation state.
	theme := m.Appearance
	theme.NextTheme()
	m.SetAppearance(theme)
	if m.CurrentFocus(Clock) != 2 {
		t.Fatal("theme reset focus")
	}
	// An app update may reorder widgets while IDs are stable.
	updated := viewForWidgets()
	updated.Screens[1].Widgets[0], updated.Screens[1].Widgets[2] = updated.Screens[1].Widgets[2], updated.Screens[1].Widgets[0]
	m.SetView(wire.ClockID, updated)
	if m.CurrentFocus(Clock) != 0 {
		t.Fatalf("did not restore focus by ID: %d", m.CurrentFocus(Clock))
	}
	if !m.Back() || m.NavDepth(Clock) != 1 || m.CurrentFocus(Clock) != 0 {
		t.Fatal("parent focus not restored")
	}
}
func TestWidgetBackRotaryAndHitGeometry(t *testing.T) {
	m := New()
	m.SetView(wire.ClockID, viewForWidgets())
	m.Select(Clock)
	m.Activate() // nested settings
	if _, _, _ = m.Rotate(-1); m.CurrentFocus(Clock) != -2 {
		t.Fatal("Back not reachable by rotary")
	}
	if changed, action := m.Activate(); !changed || action != "" || m.NavDepth(Clock) != 1 {
		t.Fatal("Back button must pop")
	}
	m.Activate()
	back := circleui.BackRect()
	if changed, action := m.Tap((back.Left+back.Right)/2, (back.Top+back.Bottom)/2); !changed || action != "" || m.NavDepth(Clock) != 1 {
		t.Fatal("tap Back should pop")
	}
	// Layout and hit test must use the same rectangles inside the safe circle.
	s := viewForWidgets().Screens[1]
	for _, r := range circleui.VisibleAt(s.Widgets, 1, 0, false, false) {
		for _, pt := range []image.Point{{r.Rect.Left, r.Rect.Top}, {r.Rect.Right - 1, r.Rect.Bottom - 1}} {
			if circleui.Hit([]circleui.Row{r}, (r.Rect.Left+r.Rect.Right)/2, (r.Rect.Top+r.Rect.Bottom)/2) != r.Index {
				t.Fatal("hit mismatch")
			}
			if math.Hypot(float64(pt.X-240), float64(pt.Y-240)) > float64(circleui.SafeRadius)+1 {
				t.Fatalf("widget outside safe circle: %+v", r)
			}
		}
	}
}
func TestWidgetToggleAndTouchSlider(t *testing.T) {
	m := New()
	m.SetView(wire.ClockID, viewForWidgets())
	m.Select(Clock)
	m.Activate()
	if changed, action := m.Activate(); !changed || action != "toggle" {
		t.Fatal("toggle action")
	}
	s, _ := m.CurrentScreen(Clock)
	if !s.Widgets[0].Checked {
		t.Fatal("toggle did not update optimistically")
	}
	m.Rotate(2)
	rows := m.visibleWidgets()
	var sr circleui.Row
	for _, r := range rows {
		if r.Index == 2 {
			sr = r
		}
	}
	if sr.Rect.Right == 0 {
		t.Fatal("slider not visible")
	}
	if changed, action := m.Tap(sr.Rect.Right-24, (sr.Rect.Top+sr.Rect.Bottom)/2); !changed || action != "set_brightness" || m.WidgetDelta < 85 {
		t.Fatalf("slider tap action=%q value=%d", action, m.WidgetDelta)
	}
	// A slide/drag commit dispatches one action, not an action per input frame.
	m.WidgetDragStart(sr.Rect.Left+20, (sr.Rect.Top+sr.Rect.Bottom)/2)
	m.WidgetDragMove(sr.Rect.Right-18, (sr.Rect.Top+sr.Rect.Bottom)/2, 0)
	if changed, action := m.WidgetDragEnd(); !changed || action != "set_brightness" || m.WidgetDelta < 95 {
		t.Fatalf("drag commit action=%s value=%d", action, m.WidgetDelta)
	}
}
func TestWidgetSnapshotAndAppearance(t *testing.T) {
	m := New()
	v := viewForWidgets()
	m.SetView(wire.ClockID, v)
	v.Screens[0].Widgets[0].Label = "CORRUPTED"
	snapshot := m.Snapshot()
	snapshot.Views[Clock].Screens[0].Widgets[0].Label = "MODIFIED"
	if s, _ := m.CurrentScreen(Clock); s.Widgets[0].Label != "OPEN SETTINGS" {
		t.Fatal("snapshot alias or caller mutation")
	}
	m.Select(Clock)
	for _, theme := range []string{"dark", "light", "contrast"} {
		m.Appearance.Theme = theme
		if b := Draw(m, time.Now()).Bounds(); b.Dx() != 480 || b.Dy() != 480 {
			t.Fatal("wrong canvas size")
		}
	}
}

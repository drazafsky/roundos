package shell

import (
	"math"
	"roundos/internal/circleui"
	"roundos/internal/wire"
	"roundos/sdk/ui"
	"testing"
	"time"
)

func rc1View() wire.View {
	list := make([]wire.Widget, 0, 12)
	for i := 0; i < 12; i++ {
		list = append(list, ui.ListItem(string(rune('a'+i)), "LIST ITEM", "DETAIL", "item_selected"))
	}
	return wire.View{Screens: []wire.Screen{
		ui.Screen("root", "DEMO", "", "", ui.Link("open", "OPEN LIST", "list")),
		ui.Screen("list", "LONG LIST", "", "", list...),
	}}
}
func TestRC1ContinuousScrollInertiaAndBounds(t *testing.T) {
	m := New()
	m.SetView(wire.ClockID, rc1View())
	m.Select(Clock)
	m.Activate()
	base := time.Date(2026, 11, 11, 1, 0, 0, 0, time.UTC)
	m.WidgetDragStart(240, 230)
	if !m.WidgetDragMoveAt(240, 175, -55, base) {
		t.Fatal("finger drag did not move list")
	}
	f, _, _ := m.navFrame(Clock)
	if f.ScrollPx != 55 {
		t.Fatalf("expected 55 pixel continuous movement, got %.2f", f.ScrollPx)
	}
	if !m.WidgetDragMoveAt(240, 100, -130, base.Add(50*time.Millisecond)) {
		t.Fatal("second drag missing")
	}
	m.WidgetDragEndAt(base.Add(55 * time.Millisecond))
	if m.NextKineticWake(base.Add(55*time.Millisecond)) == 0 {
		t.Fatal("quick swipe should continue briefly")
	}
	before := f.ScrollPx
	if !m.AdvanceKinetic(base.Add(72*time.Millisecond)) || f.ScrollPx <= before {
		t.Fatalf("inertial movement missing: before=%g after=%g", before, f.ScrollPx)
	}
	for k := 1; k < 40; k++ {
		m.AdvanceKinetic(base.Add(72 * time.Millisecond).Add(time.Duration(k*16) * time.Millisecond))
	}
	if m.KineticVelocity != 0 {
		t.Fatal("inertia did not stop")
	}
	maxPx := circleui.MaxScrollPx(12, false, false)
	if f.ScrollPx < 0 || f.ScrollPx > maxPx {
		t.Fatal("outside list bounds")
	}
	for _, r := range m.visibleWidgets() {
		for _, xy := range [][2]int{{r.Rect.Left, r.Rect.Top}, {r.Rect.Right - 1, r.Rect.Bottom - 1}} {
			if math.Hypot(float64(xy[0]-240), float64(xy[1]-240)) > float64(circleui.SafeRadius)+1 {
				t.Errorf("row escaped circular safe region: %+v", r)
			}
		}
	}
	m.WidgetDragStart(240, 230)
	if m.KineticVelocity != 0 {
		t.Fatal("new contact must immediately halt inertia")
	}
}
func TestRC1ReduceMotionDisablesInertia(t *testing.T) {
	m := New()
	m.Appearance.ReduceMotion = true
	m.SetView(wire.ClockID, rc1View())
	m.Select(Clock)
	m.Activate()
	now := time.Now()
	m.WidgetDragStart(240, 230)
	m.WidgetDragMoveAt(240, 180, -50, now)
	m.WidgetDragMoveAt(240, 95, -135, now.Add(50*time.Millisecond))
	m.WidgetDragEndAt(now.Add(55 * time.Millisecond))
	if m.KineticVelocity != 0 || m.NextKineticWake(now) != 0 {
		t.Fatal("reduce motion must disable inertial continuation")
	}
}
func TestRC1DialogFocusCancelConfirmAndRestore(t *testing.T) {
	v := rc1View()
	v.Dialog = ui.ConfirmDialog("reset", "RESET?", "DELETE CURRENT TIME", "reset_confirm", "reset_cancel", true)
	m := New()
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	if !m.hasVisibleDialog() || m.DialogFocus != 0 {
		t.Fatal("modal must start with safe cancel focus")
	}
	if ch, act := m.Activate(); !ch || act != "" {
		t.Fatal("dialog activation must not activate underlying app")
	}
	if app, act := m.TakeDialogAction(); app != wire.ClockID || act != "reset_cancel" {
		t.Fatalf("cancel dispatch %q %q", app, act)
	}
	if m.hasVisibleDialog() {
		t.Fatal("modal should immediately hide after an action")
	}
	v.Dialog = nil
	m.SetView(wire.ClockID, v)
	v.Dialog = ui.ConfirmDialog("reset", "RESET?", "DELETE CURRENT TIME", "reset_confirm", "reset_cancel", true)
	m.SetView(wire.ClockID, v)
	m.Rotate(1)
	if m.DialogFocus != 1 {
		t.Fatal("confirm not focusable")
	}
	m.Activate()
	if _, act := m.TakeDialogAction(); act != "reset_confirm" {
		t.Fatalf("confirmation missing %q", act)
	}
	if m.NavDepth(Clock) != 1 || m.CurrentFocus(Clock) != 0 {
		t.Fatal("modal changed app focus")
	}
}
func TestRC1PickersCommitAndCancel(t *testing.T) {
	m := New()
	v := wire.View{Screens: []wire.Screen{ui.Screen("root", "ROOT", "", "", ui.Link("settings", "SETTINGS", "settings")), ui.Screen("settings", "SETTINGS", "", "", ui.TimePicker("time", "TIME", "save_time", 1435, 5), ui.NumberPicker("count", "COUNT", "save_count", 5, 0, 20, 1))}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	m.Activate()
	if changed, act := m.Activate(); !changed || act != "" || m.PickerID != "time" {
		t.Fatalf("picker not opened %q", m.PickerID)
	}
	m.Rotate(1)
	if m.PickerValue != 0 {
		t.Fatalf("wrap from 23:55 should be 00:00, got %d", m.PickerValue)
	}
	if changed, act := m.Activate(); !changed || act != "save_time" || m.WidgetDelta != 0 {
		t.Fatalf("picker commit %q,%d", act, m.WidgetDelta)
	}
	m.Rotate(1)
	m.Activate()
	if m.PickerID != "count" {
		t.Fatal("number picker not opened")
	}
	m.Rotate(4)
	m.Back()
	if m.PickerID != "" || m.WidgetDelta != 0 {
		t.Fatal("Back must cancel without publishing")
	}
}

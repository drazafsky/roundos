package shell

import (
	"roundos/internal/circleui"
	"roundos/internal/gesture"
	"roundos/internal/touch"
	"roundos/internal/wire"
	"roundos/sdk/ui"
	"testing"
	"time"
)

func TestReloadFocusUsesNearestOldNeighborAndPreservesDraft(t *testing.T) {
	m := New()
	v := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Button("a", "A", "a"), ui.Button("b", "B", "b"), ui.Button("c", "C", "c"), ui.NumberPicker("picker", "Pick", "pick", 5, 0, 10, 1))}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	m.AppStacks[Clock][0].FocusID = "b"
	m.PickerID = "picker"
	m.PickerValue = 7
	v.Screens[0].Widgets[1].Disabled = true
	m.SetView(wire.ClockID, v)
	if m.AppStacks[Clock][0].FocusID != "a" {
		t.Fatal("nearest focus did not prefer preceding neighbor")
	}
	if m.PickerID != "picker" || m.PickerValue != 7 {
		t.Fatal("compatible picker draft lost")
	}
	v.Screens[0].Widgets[3].Max = 6
	m.SetView(wire.ClockID, v)
	if m.PickerID != "" {
		t.Fatal("incompatible picker transaction retained")
	}
	for i := range v.Screens[0].Widgets {
		v.Screens[0].Widgets[i].Disabled = true
	}
	m.SetView(wire.ClockID, v)
	if m.AppStacks[Clock][0].FocusID != "__back" {
		t.Fatal("empty focus set did not fall back to Back")
	}
}

func TestReloadCancelsContactBeforeItCanActivateNewTarget(t *testing.T) {
	for _, change := range []string{"reorder", "action", "disabled", "hero", "removed-screen"} {
		t.Run(change, func(t *testing.T) {
			m := New()
			v := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Button("a", "A", "a"), ui.Button("b", "B", "b"))}}
			m.SetView(wire.ClockID, v)
			m.Select(Clock)
			d := gesture.New()
			at := time.Unix(100, 0)
			d.Process(touch.Frame{At: at, Contacts: []touch.Contact{{ID: 1, X: 240, Y: 190}}})
			revision := m.InputRevision
			switch change {
			case "reorder":
				v.Screens[0].Widgets[0], v.Screens[0].Widgets[1] = v.Screens[0].Widgets[1], v.Screens[0].Widgets[0]
			case "action":
				v.Screens[0].Widgets[0].Action = "other"
			case "disabled":
				v.Screens[0].Widgets[0].Disabled = true
			case "hero":
				v.Screens[0].Hero = "123"
			case "removed-screen":
				v.Screens[0].ID = "new-root"
			}
			m.SetView(wire.ClockID, v)
			if m.InputRevision == revision {
				t.Fatal("changed touch targets did not request input cancellation")
			}
			if !d.CancelUntilRelease() {
				t.Fatal("active contact was not suppressed")
			}
			for _, ev := range d.Process(touch.Frame{At: at.Add(80 * time.Millisecond)}) {
				if ev.Type == gesture.Tap || ev.Type == gesture.DragEnd {
					t.Fatalf("canceled contact can still activate a widget: %+v", ev)
				}
			}
			// The next intentional tap works on the replacement tree.
			d.Process(touch.Frame{At: at.Add(time.Second), Contacts: []touch.Contact{{ID: 2, X: 240, Y: 190}}})
			found := false
			for _, ev := range d.Process(touch.Frame{At: at.Add(1080 * time.Millisecond)}) {
				found = found || ev.Type == gesture.Tap
			}
			if !found {
				t.Fatal("fresh contact did not recover after cancellation")
			}
		})
	}
}

func TestReloadDetachesDragAndPreservesCompatibleDraft(t *testing.T) {
	m := New()
	v := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Slider("s", "Slider", "set", 3, 0, 10, 1), ui.Button("b", "B", "b"))}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	m.EditingWidget, m.TouchSlider, m.WidgetEditValue = "s", "s", 7
	m.DragVelocity, m.KineticVelocity = 900, 900
	m.DragLastAt = time.Now()
	revision := m.InputRevision
	v.Screens[0].Widgets[0].Value = 4
	v.Screens[0].Widgets[0].Label = "New label"
	m.SetView(wire.ClockID, v)
	if m.InputRevision != revision || m.TouchSlider != "s" {
		t.Fatal("value-only refresh canceled a compatible contact")
	}
	v.Screens[0].Widgets[0], v.Screens[0].Widgets[1] = v.Screens[0].Widgets[1], v.Screens[0].Widgets[0]
	m.SetView(wire.ClockID, v)
	if m.EditingWidget != "s" || m.WidgetEditValue != 7 || m.TouchSlider != "" || m.DragVelocity != 0 || m.KineticVelocity != 0 || !m.DragLastAt.IsZero() {
		t.Fatalf("draft or drag state after reload: %+v", m)
	}
	if _, action := m.WidgetDragEnd(); action != "" {
		t.Fatal("canceled finger committed the retained draft")
	}
}

func TestReloadRemovingParentCancelsUnchangedChildInput(t *testing.T) {
	m := New()
	v := ui.View{Screens: []ui.ScreenSpec{
		ui.Screen("root", "Root", "", "", ui.Button("a", "A", "a")),
		ui.Screen("child", "Child", "", "", ui.NumberPicker("pick", "Pick", "set", 3, 0, 10, 1)),
	}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	if !m.enterScreen("child") {
		t.Fatal("cannot enter fixture child")
	}
	m.PickerID, m.PickerValue = "pick", 7
	m.EditingWidget, m.TouchSlider = "pick", "pick"
	revision := m.InputRevision
	// The active child remains unchanged in the view, but its parent is
	// removed. Navigation falls back to the new root instead of the child.
	v.Screens[0].ID = "new-root"
	m.SetView(wire.ClockID, v)
	screen, _ := m.CurrentScreen(Clock)
	if screen.ID != "new-root" || m.InputRevision == revision || m.PickerID != "" || m.EditingWidget != "" || m.TouchSlider != "" {
		t.Fatalf("orphaned child input survived navigation fallback: screen=%s picker=%s edit=%s", screen.ID, m.PickerID, m.EditingWidget)
	}
}

func TestReloadRevealsRetainedFocusAfterReordering(t *testing.T) {
	m := New()
	v := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Button("a", "A", "a"), ui.Button("b", "B", "b"), ui.Button("c", "C", "c"), ui.Button("d", "D", "d"), ui.Button("e", "E", "e"))}}
	m.SetView(wire.ClockID, v)
	m.Select(Clock)
	v.Screens[0].Widgets[0], v.Screens[0].Widgets[4] = v.Screens[0].Widgets[4], v.Screens[0].Widgets[0]
	m.SetView(wire.ClockID, v)
	visible := false
	for _, row := range m.visibleWidgets() {
		if row.Index == 4 {
			visible = true
		}
	}
	if !visible || m.AppStacks[Clock][0].FocusID != "a" {
		t.Fatal("retained focus became an off-screen activation target")
	}
	if _, action := m.ActivateWidget(); action != "a" {
		t.Fatal("stable focused action lost", action)
	}
}

func TestReloadRootBackFallbackIsVisibleAndWorksWithTouchAndEncoder(t *testing.T) {
	for _, touch := range []bool{false, true} {
		m := New()
		view := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Button("a", "Action", "a"))}}
		m.SetView(wire.ClockID, view)
		m.Select(Clock)
		before := Draw(m, time.Unix(0, 0))
		view.Screens[0].Widgets[0].Disabled = true
		m.SetView(wire.ClockID, view)
		after := Draw(m, time.Unix(0, 0))
		back := circleui.BackRect()
		if before.RGBAAt(back.Left+2, back.Top+2) == after.RGBAAt(back.Left+2, back.Top+2) {
			t.Fatal("root Back fallback not rendered")
		}
		var changed bool
		var action string
		if touch {
			changed, action = m.WidgetTap((back.Left+back.Right)/2, (back.Top+back.Bottom)/2)
		} else {
			changed, action = m.ActivateWidget()
		}
		if !changed || action != "" || m.Page != Home {
			t.Fatal("root Back input did not return Home", touch, changed, action, m.Page)
		}
	}
}

package shell

import (
	"roundos/internal/wire"
	"roundos/sdk/ui"
	"testing"
)

func TestTypedOriginsUseActualWidgetAndCommittedValue(t *testing.T) {
	m := New()
	v := ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Toggle("toggle", "Toggle", "enable", true), ui.NumberPicker("picker", "Picker", "set", 3, 0, 10, 1))}}
	m.SetViewRevision(wire.ClockID, v, 7)
	m.Select(Clock)
	m.AppStacks[Clock][0].FocusID = "toggle"
	_, action := m.ActivateWidget()
	event := m.TakeWidgetInput(action)
	if event == nil || event.Screen != "root" || event.Widget != "toggle" || event.Kind != "bool" || event.Bool == nil || *event.Bool || event.LayoutRevision != 7 {
		t.Fatalf("wrong toggle origin: %+v", event)
	}
	if m.TakeWidgetInput(action) != nil {
		t.Fatal("origin reused")
	}
	m.AppStacks[Clock][0].FocusID = "picker"
	m.ActivateWidget()
	m.pickerRotate(2)
	_, action = m.pickerCommit()
	event = m.TakeWidgetInput(action)
	if event == nil || event.Widget != "picker" || event.Int == nil || *event.Int != 5 || event.LayoutRevision != 7 {
		t.Fatalf("wrong picker origin: %+v", event)
	}
}

func TestSaturatedInputQueueRestoresOptimisticToggle(t *testing.T) {
	m := New()
	m.SetViewRevision(wire.ClockID, ui.View{Screens: []ui.ScreenSpec{ui.Screen("root", "Root", "", "", ui.Toggle("toggle", "Toggle", "enable", true))}}, 7)
	m.Select(Clock)
	queue := make(chan wire.UIEvent, 1)
	queue <- wire.UIEvent{}
	_, action := m.ActivateWidget()
	if m.Views[Clock].Screens[0].Widgets[0].Checked {
		t.Fatal("toggle did not apply optimism")
	}
	handled, delivered := m.SendWidgetInput(action, func(event wire.UIEvent) bool {
		select {
		case queue <- event:
			return true
		default:
			return false
		}
	})
	if !handled || delivered || !m.Views[Clock].Screens[0].Widgets[0].Checked {
		t.Fatal("dropped event left optimistic toggle changed")
	}
	<-queue
	_, action = m.ActivateWidget()
	_, delivered = m.SendWidgetInput(action, func(event wire.UIEvent) bool { queue <- event; return true })
	if !delivered || m.Views[Clock].Screens[0].Widgets[0].Checked || *(<-queue).Bool {
		t.Fatal("successful input was rolled back")
	}
}

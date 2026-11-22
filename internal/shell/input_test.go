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

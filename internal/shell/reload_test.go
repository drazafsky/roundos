package shell

import (
	"roundos/internal/wire"
	"roundos/sdk/ui"
	"testing"
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

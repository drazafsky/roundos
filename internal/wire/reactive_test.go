package wire

import (
	"reflect"
	"strings"
	"testing"
)

func reactiveView() View {
	return View{Screens: []Screen{{ID: "root", Title: "Timer", Hero: "00:10", Widgets: []Widget{{ID: "count", Kind: "slider", Label: "Count", Action: "set", Value: 5, Min: 0, Max: 10}, {ID: "enabled", Kind: "toggle", Label: "Enable", Action: "enable"}}}}}
}
func TestPropertyRoundTripAndStructuralFallback(t *testing.T) {
	before := reactiveView()
	after := CopyView(before)
	after.Screens[0].Hero = "00:09"
	after.Screens[0].Widgets[0].Value = 6
	after.Screens[0].Widgets[1].Checked = true
	updates, ok := DiffProperties(before, after)
	if !ok || len(updates) != 3 {
		t.Fatal(updates, ok)
	}
	got, err := ApplyProperties(before, updates)
	if err != nil || !reflect.DeepEqual(got, after) {
		t.Fatal(got, err)
	}
	if before.Screens[0].Widgets[0].Value != 5 {
		t.Fatal("original snapshot mutated")
	}
	after.Screens[0].Widgets[0].Action = "new-action"
	if _, ok := DiffProperties(before, after); ok {
		t.Fatal("changed action encoded as property patch")
	}
}
func TestInvalidPropertyBatchesAreAtomic(t *testing.T) {
	text := "Changed"
	badText := strings.Repeat("x", 161)
	value := 99
	checked := true
	valid := PropertyUpdate{Screen: "root", Widget: "count", Property: "label", String: &text}
	for _, bad := range []PropertyUpdate{
		{Screen: "missing", Widget: "count", Property: "label", String: &text},
		{Screen: "root", Widget: "missing", Property: "label", String: &text},
		{Screen: "root", Widget: "count", Property: "action", String: &text},
		{Screen: "root", Widget: "count", Property: "label", String: &badText},
		{Screen: "root", Widget: "count", Property: "value", Int: &value},
		{Screen: "root", Widget: "count", Property: "checked", Bool: &checked},
		{Screen: "root", Widget: "count", Property: "label", String: &text, Bool: &checked},
		valid,
	} {
		before := reactiveView()
		saved := CopyView(before)
		if _, err := ApplyProperties(before, []PropertyUpdate{valid, bad}); err == nil {
			t.Fatalf("bad batch accepted: %+v", bad)
		}
		if !reflect.DeepEqual(before, saved) {
			t.Fatal("partial patch applied")
		}
	}
}
func TestCapabilitySelectionIsExplicit(t *testing.T) {
	if len(SelectCapabilities(nil)) != 0 {
		t.Fatal("legacy peer upgraded implicitly")
	}
	got := SelectCapabilities([]string{UIEventV1, "unknown", UIEventV1})
	if !reflect.DeepEqual(got, []string{UIEventV1}) {
		t.Fatal(got)
	}
}

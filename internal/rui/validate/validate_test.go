package validate

import (
	"fmt"
	"strings"
	"testing"

	"roundos/sdk/rui/spec"
)

func str(v string) spec.Value[string] {
	return spec.Value[string]{Literal: v, Present: true, Pos: spec.Pos{Line: 1, Column: 1}}
}
func minDoc() *spec.Document {
	return &spec.Document{Schema: spec.Version, Application: spec.Application{ID: "example.app"}, Screens: []spec.Screen{{ID: "app.root", Title: str("App"), Layout: "detail", Groups: []spec.Group{{ID: "content", Widgets: []spec.Widget{{ID: "app.label", Type: "label", Label: ptrStr("Hello")}}}}}}}
}
func ptrStr(s string) *spec.Value[string] { v := str(s); return &v }
func TestSemanticWidgetBudget(t *testing.T) {
	d := minDoc()
	d.Screens[0].Groups[0].Title = ptrStr("Heading") // costs one slot
	for i := 1; i < 63; i++ {
		d.Screens[0].Groups[0].Widgets = append(d.Screens[0].Groups[0].Widgets, spec.Widget{ID: fmt.Sprintf("app.label%d", i), Type: "label", Label: ptrStr("Hi")})
	}
	if _, e := Validate(d); e != nil {
		t.Fatalf("64 effective widget boundary rejected: %v", e)
	}
	d.Screens[0].Groups[0].Widgets = append(d.Screens[0].Groups[0].Widgets, spec.Widget{ID: "app.over", Type: "label", Label: ptrStr("over")})
	if _, e := Validate(d); e == nil || !strings.Contains(e.Error(), "exceeds 64 effective widgets") {
		t.Fatalf("budget overrun not rejected: %v", e)
	}
}
func TestSixteenScreenLimit(t *testing.T) {
	d := minDoc()
	for i := 1; i < 16; i++ {
		s := d.Screens[0]
		s.ID = fmt.Sprintf("app.screen%d", i)
		d.Screens = append(d.Screens, s)
	}
	if _, e := Validate(d); e != nil {
		t.Fatalf("valid 16 screens: %v", e)
	}
	d.Screens = append(d.Screens, d.Screens[0])
	if _, e := Validate(d); e == nil {
		t.Fatal("17 screens allowed")
	}
}
func TestBadIdentifiersAreRejected(t *testing.T) {
	for _, id := range []string{"", "__rui_generated", "rui.heading", "0bad", strings.Repeat("a", 65), "file/path"} {
		d := minDoc()
		d.Screens[0].ID = id
		if _, e := Validate(d); e == nil {
			t.Fatalf("identifier %q was accepted", id)
		}
	}
}
func TestBoundValueRangeDeferredToRuntime(t *testing.T) {
	d := minDoc()
	bound := spec.Value[int]{Present: true, Bound: true, Binding: "state.minutes"}
	min, max := 1, 100
	d.Screens[0].Groups[0].Widgets = []spec.Widget{{ID: "app.minutes", Type: "numberPicker", Label: ptrStr("Minutes"), Value: &bound, Min: &min, Max: &max, OnCommit: "state.setMinutes"}}
	manifest, e := Validate(d)
	if e != nil {
		t.Fatal(e)
	}
	if manifest.Bindings["state.minutes"] != spec.IntBinding {
		t.Fatal("bound integer type missing")
	}
}

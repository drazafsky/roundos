package rui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"roundos/internal/rui/compile"
	"roundos/sdk/app"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

const boundRangeLayout = `schema: roundos.rui/v1
application:
  id: example.ranges
  icon: RUI
screens:
  - id: root
    title: { bind: title }
    layout: list
    groups:
      - id: controls
        widgets:
          - id: date
            type: datePicker
            label: Date
            value: { bind: value }
            min: { bind: low }
            max: { bind: high }
            onCommit: set
`

func TestBoundRangesRetainConsistencyAndIndependentUpdates(t *testing.T) {
	data, _, err := compile.Compile([]byte(boundRangeLayout))
	if err != nil {
		t.Fatal(err)
	}
	pub := &capture{}
	r, err := NewForApp(pub, data, "example.ranges")
	if err != nil {
		t.Fatal(err)
	}
	low, high, value, title := 10, 20, 15, "Initial"
	for key, get := range map[string]func() int{"low": func() int { return low }, "high": func() int { return high }, "value": func() int { return value }} {
		if err := r.BindInt(key, get); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.BindString("title", func() string { return title }); err != nil {
		t.Fatal(err)
	}
	if err := r.OnInt("set", func(_ context.Context, v int) error { value = v; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateBindings(); err != nil {
		t.Fatal(err)
	}
	if err := r.PublishInitial(); err != nil {
		t.Fatal(err)
	}
	if err := r.PublishInitial(); err != nil || pub.count() != 1 {
		t.Fatal("readiness published twice", err)
	}
	high = 12
	title = "Valid update"
	r.Invalidate()
	if err := r.Flush(); err != nil || pub.count() != 1 {
		t.Fatal("hidden publication", err)
	}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err == nil {
		t.Fatal("inconsistent range accepted")
	}
	view := pub.last()
	if view.Title != title || view.Screens[0].Widgets[0].Max != 20 || view.Screens[0].Widgets[0].Value != 15 {
		t.Fatal("invalid range blocked independent updates or lost last good state", view)
	}
	value = 11
	if err := r.Flush(); err != nil {
		t.Fatal("failed range not retried", err)
	}
	if pub.last().Screens[0].Widgets[0].Max != 12 || pub.last().Screens[0].Widgets[0].Value != 11 {
		t.Fatal("range recovery failed")
	}
	if _, err := r.Dispatch(app.Event{Type: "input", Action: "set", Delta: 17}); err == nil || value != 11 {
		t.Fatal("legacy input bypassed published range", err)
	}
}

func BenchmarkBoundRangeValidation(b *testing.B) {
	file := &bundle.File{}
	values := map[string]boundValue{}
	for screen := 0; screen < 2; screen++ {
		group := spec.Group{}
		for widget := 0; widget < 64; widget++ {
			key := fmt.Sprintf("s%d.w%d", screen, widget)
			group.Widgets = append(group.Widgets, spec.Widget{Type: "numberPicker", Value: &spec.Value[int]{Bound: true, Binding: key + ".value"}, MinBinding: key + ".min", MaxBinding: key + ".max"})
			values[key+".value"] = boundValue{typ: spec.IntBinding, integer: 5}
			values[key+".min"] = boundValue{typ: spec.IntBinding, integer: 0}
			values[key+".max"] = boundValue{typ: spec.IntBinding, integer: 10}
			values[key+".label"] = boundValue{typ: spec.StringBinding, text: "Label"}
		}
		file.Document.Screens = append(file.Document.Screens, spec.Screen{Groups: []spec.Group{group}})
	}
	for _, textOnly := range []bool{false, true} {
		b.Run(fmt.Sprint("text_only=", textOnly), func(b *testing.B) {
			var changed map[string]bool
			if !textOnly {
				changed = map[string]bool{"s0.w0.value": true}
			}
			failed := map[string]bool{}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := validateResolvedRanges(file, values, values, failed, changed); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestPartlyBoundRangesCannotShareAmbiguousAction(t *testing.T) {
	source := strings.Replace(boundRangeLayout, "min: { bind: low }", "min: 0", 1)
	start := strings.Index(source, "          - id: date")
	second := strings.Replace(source[start:], "id: date", "id: second", 1)
	second = strings.Replace(second, "min: 0", "min: 10", 1)
	data, _, err := compile.Compile([]byte(source + second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewForApp(&capture{}, data, "example.ranges"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal("different literal bounds were ignored", err)
	}
}

func TestReloadRejectsNonportableBoundLimits(t *testing.T) {
	if strconv.IntSize == 32 {
		t.Skip("32-bit ints cannot represent this invalid getter result")
	}
	data, _, err := compile.Compile([]byte(boundRangeLayout))
	if err != nil {
		t.Fatal(err)
	}
	pub := &capture{}
	r, err := NewForApp(pub, data, "example.ranges")
	if err != nil {
		t.Fatal(err)
	}
	low, high, value := 10, 20, 15
	r.BindInt("low", func() int { return low })
	r.BindInt("high", func() int { return high })
	r.BindInt("value", func() int { return value })
	r.BindString("title", func() string { return "Ranges" })
	r.OnInt("set", func(context.Context, int) error { return nil })
	if err := r.ValidateBindings(); err != nil {
		t.Fatal(err)
	}
	if err := r.PublishInitial(); err != nil {
		t.Fatal(err)
	}
	large := int64(1) << 40
	high = int(large)
	if err := r.Replace(data); err == nil || !strings.Contains(err.Error(), "portable") {
		t.Fatal("reload accepted nonportable limit", err)
	}
	if pub.count() != 1 || pub.last().Screens[0].Widgets[0].Max != 20 {
		t.Fatal("invalid reload changed active view")
	}
}

func TestSharedRangeRollbackRevalidatesEarlierControl(t *testing.T) {
	start := strings.Index(boundRangeLayout, "          - id: date")
	earlier := strings.Replace(boundRangeLayout[start:], "id: date", "id: earlier", 1)
	earlier = strings.Replace(earlier, "{ bind: value }", "{ bind: other }", 1)
	_, file, err := compile.Compile([]byte(boundRangeLayout[:start] + earlier + boundRangeLayout[start:]))
	if err != nil {
		t.Fatal(err)
	}
	previous := map[string]boundValue{
		"low": {typ: spec.IntBinding, integer: 5}, "high": {typ: spec.IntBinding, integer: 10},
		"value": {typ: spec.IntBinding, integer: 6}, "other": {typ: spec.IntBinding, integer: 7},
	}
	values := map[string]boundValue{}
	for key, value := range previous {
		values[key] = value
	}
	values["low"] = boundValue{typ: spec.IntBinding, integer: 0}
	values["value"] = boundValue{typ: spec.IntBinding, integer: 11}
	values["other"] = boundValue{typ: spec.IntBinding, integer: 3}
	failed := map[string]bool{}
	if err := validateResolvedRanges(file, previous, values, failed, map[string]bool{"low": true, "value": true, "other": true}); err == nil {
		t.Fatal("invalid shared range accepted")
	}
	for _, key := range []string{"low", "value", "other"} {
		if values[key] != previous[key] {
			t.Fatal("shared rollback left a control outside its restored range", key, values)
		}
	}
}

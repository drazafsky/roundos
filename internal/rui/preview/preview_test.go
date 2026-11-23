package preview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"roundos/internal/rui/compile"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "testdata", "valid", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestAllWidgetsMapToValidV7AndRender(t *testing.T) {
	_, f, e := compile.Compile(fixture(t, "all-widgets.rui.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	samples := []byte(`{"settings.notifications":true,"settings.count":14,"settings.progress":39}`)
	// Partial user state is allowed; missing fields use visible placeholder values.
	state, e := NewState(f.Manifest, samples)
	if e != nil {
		t.Fatal(e)
	}
	view, e := View(f, state)
	if e != nil {
		t.Fatal(e)
	}
	found := map[string]bool{}
	for _, screen := range view.Screens {
		for _, w := range screen.Widgets {
			found[w.Kind] = true
		}
	}
	for _, kind := range []string{"button", "toggle", "slider", "list_item", "label", "progress", "picker", "time_picker", "date_picker"} {
		if !found[kind] {
			t.Fatalf("missing widget kind %q", kind)
		}
	}
	for _, theme := range []string{"dark", "light", "contrast"} {
		picture, e := Render(f, view, view.Screens[0].ID, theme)
		if e != nil {
			t.Fatal(e)
		}
		if picture.Bounds().Dx() != 480 || picture.Bounds().Dy() != 480 {
			t.Fatal("wrong preview size")
		}
		buf := &bytes.Buffer{}
		if e = png.Encode(buf, picture); e != nil {
			t.Fatal(e)
		}
		decoded, e := png.Decode(bytes.NewReader(buf.Bytes()))
		if e != nil || decoded.Bounds().Dx() != 480 {
			t.Fatalf("invalid PNG: %v", e)
		}
	}
	if _, e = Render(f, view, "missing.screen", "dark"); e == nil {
		t.Fatal("unknown screen accepted")
	}
}
func TestSampleBindingsTypedAndRangeChecked(t *testing.T) {
	_, f, e := compile.Compile(fixture(t, "timer.rui.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{`{"timer.minutes":"5"}`, `{"timer.minutes":1.5}`, `{"timer.minutes":null}`, `{"timer.unknown":1}`, `[]`, `{"timer.minutes":9999999999999}`} {
		if _, e := NewState(f.Manifest, []byte(s)); e == nil {
			t.Fatalf("accepted invalid samples: %s", s)
		}
	}
	state, e := NewState(f.Manifest, []byte(`{"timer.minutes":5000}`))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = View(f, state); e == nil || !strings.Contains(e.Error(), "outside") {
		t.Fatalf("out-of-range value accepted: %v", e)
	}
}

func TestOmittedRangeSamplesUseKnownEndpointAndValue(t *testing.T) {
	for _, test := range []struct {
		min, max, value, samples string
		low, high, selected      int
	}{
		{"{ bind: low }", "0", "0", "", -1, 0, 0},
		{"0", "{ bind: high }", "5", "", 0, 5, 5},
		{"{ bind: low }", "-10", "-15", "", -15, -10, -15},
		{"{ bind: low }", "{ bind: high }", "2147483647", "", 2147483646, 2147483647, 2147483647},
		{"{ bind: low }", "{ bind: high }", "-2147483648", "", -2147483648, -2147483647, -2147483648},
		{"{ bind: low }", "{ bind: high }", "{ bind: chosen }", `{"low":-20,"high":-10,"chosen":-15}`, -20, -10, -15},
	} {
		source := fmt.Sprintf(`schema: roundos.rui/v1
application:
  id: example.range
screens:
  - id: root
    title: Range
    layout: list
    groups:
      - id: controls
        widgets:
          - id: picker
            type: numberPicker
            label: Pick
            value: %s
            min: %s
            max: %s
            onCommit: set
`, test.value, test.min, test.max)
		_, file, err := compile.Compile([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		state, err := NewState(file.Manifest, []byte(test.samples))
		if err != nil {
			t.Fatal(err)
		}
		view, err := View(file, state)
		if err != nil {
			t.Fatal(source, err)
		}
		widget := view.Screens[0].Widgets[0]
		if widget.Min != test.low || widget.Max != test.high || widget.Value != test.selected {
			t.Fatalf("wrong inferred preview range: %+v", widget)
		}
	}
}
func TestEffectiveGroupsAndVisibility(t *testing.T) {
	_, f, e := compile.Compile(fixture(t, "timer.rui.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	state, e := NewState(f.Manifest, []byte(`{"timer.stopped":false}`))
	if e != nil {
		t.Fatal(e)
	}
	v, e := View(f, state)
	if e != nil {
		t.Fatal(e)
	}
	if v.Screens[1].Widgets[0].Kind != "label" || !strings.HasPrefix(v.Screens[1].Widgets[0].ID, "rui.heading.") {
		t.Fatalf("missing synthetic heading: %+v", v.Screens[1].Widgets)
	}
	if !v.Screens[1].Widgets[1].Disabled {
		t.Fatal("enabledWhen false not respected")
	}
	if len(state.Missing()) == 0 {
		t.Fatal("expected unprovided sample bindings")
	}
	n, e := FrameBytes(v)
	if e != nil || n <= 0 {
		t.Fatal("invalid frame", e)
	}
	_, _ = json.Marshal(spec.Version)
}

func TestRejectOversizedV7Frame(t *testing.T) {
	f := &bundle.File{Document: spec.Document{Schema: spec.Version, Application: spec.Application{ID: "org.roundos.timer"}}}
	for screen := 0; screen < 16; screen++ {
		s := spec.Screen{ID: fmt.Sprintf("s%d", screen), Title: spec.Value[string]{Present: true, Literal: "Screen"}, Layout: "list"}
		g := spec.Group{ID: "group"}
		for widget := 0; widget < 64; widget++ {
			label := spec.Value[string]{Present: true, Literal: strings.Repeat("L", 150)}
			g.Widgets = append(g.Widgets, spec.Widget{ID: fmt.Sprintf("w%d", widget), Type: "label", Label: &label})
		}
		s.Groups = []spec.Group{g}
		f.Document.Screens = append(f.Document.Screens, s)
	}
	_, err := View(f, &State{values: map[string]json.RawMessage{}})
	if err == nil || !strings.Contains(err.Error(), "IPC frame exceeds") {
		t.Fatalf("accepted oversized IPC frame: %v", err)
	}
}

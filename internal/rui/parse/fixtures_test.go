package parse_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"roundos/internal/rui/parse"
	"roundos/internal/rui/validate"
	"roundos/sdk/rui/spec"
)

func load(t *testing.T, p string) []byte {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestValidYAMLFixtures(t *testing.T) {
	files, e := filepath.Glob("../testdata/valid/*.rui.yaml")
	if e != nil || len(files) < 3 {
		t.Fatalf("valid fixtures: %v %v", files, e)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			d, e := parse.Parse(load(t, f))
			if e != nil {
				t.Fatal(e)
			}
			manifest, e := validate.Validate(d)
			if e != nil {
				t.Fatal(e)
			}
			if d.Schema != spec.Version || d.Application.ID == "" || len(d.Screens) == 0 {
				t.Fatal("broken AST")
			}
			// Re-parsing must give exact same AST/manifest, including source positions.
			other, e := parse.Parse(load(t, f))
			if e != nil || !reflect.DeepEqual(d, other) {
				t.Fatalf("nondeterministic parse: %v", e)
			}
			second, e := validate.Validate(other)
			if e != nil || !reflect.DeepEqual(manifest, second) {
				t.Fatalf("nondeterministic manifest: %v", e)
			}
			if _, e = json.Marshal(d); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestAllNineWidgetsAndTypedManifest(t *testing.T) {
	d, e := parse.Parse(load(t, "../testdata/valid/all-widgets.rui.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	m, e := validate.Validate(d)
	if e != nil {
		t.Fatal(e)
	}
	kinds := map[string]bool{}
	for _, s := range d.Screens {
		for _, g := range s.Groups {
			for _, w := range g.Widgets {
				kinds[w.Type] = true
			}
		}
	}
	for _, kind := range []string{"button", "toggle", "slider", "listItem", "label", "progress", "numberPicker", "timePicker", "datePicker"} {
		if !kinds[kind] {
			t.Fatalf("missing widget %s", kind)
		}
	}
	if m.Bindings["settings.notifications"] != spec.BoolBinding || m.Bindings["settings.count"] != spec.IntBinding {
		t.Fatalf("wrong typed manifest: %+v", m)
	}
	if !sort.SliceIsSorted(m.Actions, func(i, j int) bool { return m.Actions[i].Name < m.Actions[j].Name }) {
		t.Fatalf("unsorted actions")
	}
}
func TestInvalidGoldenFixtures(t *testing.T) {
	files, e := filepath.Glob("../testdata/invalid/*.rui.yaml")
	if e != nil || len(files) < 12 {
		t.Fatalf("fixtures: %d %v", len(files), e)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			d, e := parse.Parse(load(t, f))
			if e == nil {
				_, e = validate.Validate(d)
			}
			if e == nil {
				t.Fatal("invalid YAML accepted")
			}
			want := strings.TrimSpace(string(load(t, strings.TrimSuffix(f, ".rui.yaml")+".want")))
			if e.Error() != want {
				t.Fatalf("diagnostic mismatch\ngot:  %s\nwant: %s", e, want)
			}
		})
	}
}
func TestInputHardening(t *testing.T) {
	cases := []string{
		"schema: one\nschema: two\n",                           // duplicate key
		"schema: one\napplication: *alias\n",                   // alias
		"schema: one\napplication: !go/exec\n",                 // YAML tag
		"schema: one\napplication: { bind: foo, x: bar }\n",    // unsafe flow map
		"schema: one\rapplication: hi",                         // bare CR
		"schema: one\n  strange: indentation\n",                // unexpected nesting
		"schema: one\nscreens:\n  - id: foo\n    title: \"bad", // quote
		"schema: one\nflags: true\n---\nflags: false\n",        // multi-doc
	}
	for i, s := range cases {
		if _, e := parse.Parse([]byte(s)); e == nil {
			t.Fatalf("accepted unsafe YAML case %d", i)
		}
	}
	if _, e := parse.Parse([]byte{0xff, 0xfe}); e == nil {
		t.Fatal("invalid UTF8 accepted")
	}
	if _, e := parse.Parse([]byte(strings.Repeat("#", parse.MaxSourceBytes+1))); e == nil {
		t.Fatal("oversize source accepted")
	}
}
func TestSourcePositions(t *testing.T) {
	d, e := parse.Parse(load(t, "../testdata/valid/timer.rui.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if d.Screens[0].Pos.Line != 5 || d.Screens[0].Groups[0].Widgets[0].Pos.Line != 13 {
		t.Fatalf("unexpected source maps: screen %+v widget %+v", d.Screens[0].Pos, d.Screens[0].Groups[0].Widgets[0].Pos)
	}
}

func FuzzParseUntrustedInput(f *testing.F) {
	f.Add([]byte("schema: roundos.rui/v1\napplication:\n  id: example\n"))
	f.Add([]byte("screens:\n  - id: test\n    title: { bind: x }\n"))
	f.Add([]byte("schema: &bomb [1,2,3]"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parse.Parse(b) })
}
func TestBlockBindingSyntax(t *testing.T) {
	raw := []byte("schema: roundos.rui/v1\napplication:\n  id: example\nscreens:\n  - id: root\n    title:\n      bind: app.title\n    layout: detail\n    groups:\n      - id: info\n        widgets:\n          - id: info.label\n            type: label\n            label: \"Hello: world\"\n")
	d, e := parse.Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = validate.Validate(d); e != nil {
		t.Fatal(e)
	}
	if !d.Screens[0].Title.Bound || d.Screens[0].Title.Binding != "app.title" {
		t.Fatal("block binding not recognized")
	}
}

func TestPublishedExampleLayouts(t *testing.T) {
	paths, e := filepath.Glob("../../../layouts/*.rui.yaml")
	if e != nil || len(paths) < 3 {
		t.Fatalf("example layouts: %d %v", len(paths), e)
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			d, e := parse.Parse(load(t, p))
			if e != nil {
				t.Fatal(e)
			}
			if _, e = validate.Validate(d); e != nil {
				t.Fatal(e)
			}
		})
	}
}

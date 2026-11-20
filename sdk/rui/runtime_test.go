package rui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"roundos/internal/rui/compile"
	"roundos/sdk/app"
	"roundos/sdk/ui"
)

type capture struct {
	mu     sync.Mutex
	views  []ui.View
	fail   bool
	before func()
}

func (c *capture) Publish(v ui.View) error {
	if c.before != nil {
		c.before()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("simulated IPC failure")
	}
	c.views = append(c.views, v)
	return nil
}
func (c *capture) last() ui.View { c.mu.Lock(); defer c.mu.Unlock(); return c.views[len(c.views)-1] }
func (c *capture) count() int    { c.mu.Lock(); defer c.mu.Unlock(); return len(c.views) }
func loadDemo(t *testing.T) ([]byte, *capture, *Runtime) {
	t.Helper()
	data, e := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	if e != nil {
		t.Fatal(e)
	}
	encoded, _, e := compile.Compile(data)
	if e != nil {
		t.Fatal(e)
	}
	pub := &capture{}
	r, e := NewForApp(pub, encoded, "org.example.rui.demo")
	if e != nil {
		t.Fatal(e)
	}
	return encoded, pub, r
}
func registerDemo(t *testing.T, r *Runtime, number *int, enabled *bool, title *string) {
	t.Helper()
	regs := []func() error{
		func() error {
			return r.BindString("demo.display", func() string { return strings.Repeat("0", 2) + string(rune('0'+*number%10)) })
		},
		func() error { return r.BindString("demo.subtitle", func() string { return *title }) },
		func() error { return r.BindInt("demo.count", func() int { return *number }) },
		func() error { return r.BindBool("demo.enabled", func() bool { return *enabled }) },
		func() error {
			return r.OnAction("demo.increment", func(context.Context, ActionEvent) error { (*number)++; return nil })
		},
		func() error {
			return r.OnAction("demo.editTitle", func(context.Context, ActionEvent) error { return nil })
		},
		func() error {
			return r.OnAction("demo.requestReset", func(context.Context, ActionEvent) error { return r.ShowDialog("demo.confirmReset") })
		},
		func() error {
			return r.OnAction("demo.confirmReset", func(context.Context, ActionEvent) error { *number = 0; return nil })
		},
		func() error {
			return r.OnAction("demo.cancelReset", func(context.Context, ActionEvent) error { return nil })
		},
		func() error {
			return r.OnInt("demo.setCount", func(_ context.Context, n int) error { *number = n; return nil })
		},
		func() error {
			return r.OnBool("demo.setEnabled", func(_ context.Context, b bool) error { *enabled = b; return nil })
		},
	}
	for _, f := range regs {
		if e := f(); e != nil {
			t.Fatal(e)
		}
	}
	if err := r.ValidateBindings(); err != nil {
		t.Fatal(err)
	}
}
func TestEndToEndVisibilityActionsAndDialogs(t *testing.T) {
	_, pub, r := loadDemo(t)
	count := 3
	enabled := true
	title := "Demo"
	registerDemo(t, r, &count, &enabled, &title)
	if err := r.Flush(); err != nil || pub.count() != 0 {
		t.Fatalf("hidden flush %v count %d", err, pub.count())
	}
	if handled, e := r.Dispatch(app.Event{Type: "visibility", Visible: true}); !handled || e != nil {
		t.Fatal(handled, e)
	}
	if e := r.Flush(); e != nil {
		t.Fatal(e)
	}
	v := pub.last()
	if len(v.Screens) != 2 || v.Screens[0].ID != "demo.root" || v.Screens[0].Hero != "003" || v.Screens[1].Widgets[0].Checked != true {
		t.Fatalf("invalid initial view: %+v", v)
	}
	if e := r.Flush(); e != nil || pub.count() != 1 {
		t.Fatalf("unexpected repeat send: %v %d", e, pub.count())
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); !handled || e != nil || count != 4 {
		t.Fatal(handled, e, count)
	}
	if e := r.Flush(); e != nil || pub.last().Screens[0].Hero != "004" {
		t.Fatal(e, pub.last().Screens[0].Hero)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.setEnabled"}); !handled || e != nil || enabled {
		t.Fatalf("v7 toggle: %t %v %t", handled, e, enabled)
	}
	if e := r.Flush(); e != nil || pub.last().Screens[1].Widgets[0].Checked {
		t.Fatal(e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.setCount", Delta: 23}); !handled || e != nil || count != 23 {
		t.Fatal(handled, e, count)
	}
	if e := r.Flush(); e != nil || pub.last().Screens[1].Widgets[1].Value != 23 {
		t.Fatal(e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.setCount", Delta: 1000}); !handled || e == nil || count != 23 {
		t.Fatalf("accepted out-of-bounds value: %t %v", handled, e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.requestReset"}); !handled || e != nil {
		t.Fatal(handled, e)
	}
	if e := r.Flush(); e != nil || pub.last().Dialog == nil || pub.last().Dialog.ID != "demo.confirmReset" {
		t.Fatal(e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "demo.cancelReset"}); !handled || e != nil {
		t.Fatal(handled, e)
	}
	if e := r.Flush(); e != nil || pub.last().Dialog != nil || count != 23 {
		t.Fatal(e)
	}
	if _, e := r.Dispatch(app.Event{Type: "input", Action: "demo.requestReset"}); e != nil {
		t.Fatal(e)
	}
	if e := r.Flush(); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Dispatch(app.Event{Type: "input", Action: "demo.confirmReset"}); e != nil {
		t.Fatal(e)
	}
	if e := r.Flush(); e != nil || pub.last().Dialog != nil || count != 0 {
		t.Fatal(e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "text_result"}); handled || e != nil {
		t.Fatal(handled, e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "input", Action: "legacy-only"}); handled || e != nil {
		t.Fatal(handled, e)
	}
	if handled, e := r.Dispatch(app.Event{Type: "visibility", Visible: false}); !handled || e != nil {
		t.Fatal(handled, e)
	}
	title = "Modified while hidden"
	if e := r.Invalidate("demo.subtitle"); e != nil {
		t.Fatal(e)
	}
	prior := pub.count()
	if e := r.Flush(); e != nil || pub.count() != prior {
		t.Fatal(e)
	}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if e := r.Flush(); e != nil || pub.last().Screens[0].Subtitle != title {
		t.Fatal(e)
	}
}
func TestMandatoryRegistrationAndTypeSafety(t *testing.T) {
	_, _, r := loadDemo(t)
	if err := r.ValidateBindings(); err == nil {
		t.Fatal("accepted incomplete manifest")
	}
	if err := r.BindBool("demo.display", func() bool { return true }); err == nil {
		t.Fatal("accepted wrong binding type")
	}
	if err := r.BindString("unknown.key", func() string { return "" }); err == nil {
		t.Fatal("accepted unknown binding")
	}
	if err := r.OnInt("demo.increment", func(context.Context, int) error { return nil }); err == nil {
		t.Fatal("accepted wrong action type")
	}
	if err := r.OnAction("not.present", func(context.Context, ActionEvent) error { return nil }); err == nil {
		t.Fatal("accepted unknown action")
	}
	if err := r.Flush(); err == nil {
		t.Fatal("flushed without complete registration")
	}
	if err := r.Invalidate("invalid.key"); err == nil {
		t.Fatal("invalid invalidation key")
	}
	if err := r.ShowDialog("not.a.dialog"); err == nil {
		t.Fatal("invalid dialog")
	}
	n := 0
	b := true
	txt := "ok"
	registerDemo(t, r, &n, &b, &txt)
	if err := r.BindInt("demo.count", func() int { return 4 }); err == nil {
		t.Fatal("accepted late binding")
	}
	if err := r.OnAction("demo.increment", func(context.Context, ActionEvent) error { return nil }); err == nil {
		t.Fatal("accepted late handler")
	}
}
func TestRejectMismatchedAndCorruptedBundle(t *testing.T) {
	data, _, _ := loadDemo(t)
	if _, err := NewForApp(&capture{}, data, "org.example.other"); err == nil {
		t.Fatal("accepted wrong identity")
	}
	if _, err := NewForApp(&capture{}, data, ""); err == nil {
		t.Fatal("accepted empty identity")
	}
	corrupted := bytes.Clone(data)
	corrupted[12] ^= 0xff
	if _, err := NewForApp(&capture{}, corrupted, "org.example.rui.demo"); err == nil {
		t.Fatal("accepted damaged bundle")
	}
	if _, err := NewForApp(nil, data, "org.example.rui.demo"); err == nil {
		t.Fatal("accepted nil publisher")
	}
}
func TestErrorRecoveryBoundsAndConcurrentInvalidation(t *testing.T) {
	_, pub, r := loadDemo(t)
	n := 5
	enabled := true
	txt := "ok"
	registerDemo(t, r, &n, &enabled, &txt)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	pub.fail = true
	if err := r.Flush(); err == nil {
		t.Fatal("expected transport failure")
	}
	pub.fail = false
	if err := r.Flush(); err != nil || pub.count() != 1 {
		t.Fatalf("flush retry %v count %d", err, pub.count())
	}
	txt = strings.Repeat("x", 241)
	if err := r.Invalidate("demo.subtitle"); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(); err == nil {
		t.Fatal("accepted overlong bound text")
	}
	txt = "ok"
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	n = 200
	if err := r.Invalidate("demo.count"); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(); err == nil {
		t.Fatal("accepted out-of-range widget state")
	}
	n = 7
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	// The publisher triggers invalidation while Flush is still active; a second
	// publication must occur, rather than dropping the concurrent state change.
	var once sync.Once
	pub.before = func() { once.Do(func() { _ = r.Invalidate("demo.display") }) }
	if err := r.Invalidate("demo.display"); err != nil {
		t.Fatal(err)
	}
	before := pub.count()
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if after := pub.count(); after != before+2 {
		t.Fatalf("lost invalidation: before=%d after=%d", before, after)
	}
}
func TestCompiledTreeMatchesPreviewGeometry(t *testing.T) {
	_, pub, r := loadDemo(t)
	n := 9
	b := true
	txt := "hello"
	registerDemo(t, r, &n, &b, &txt)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	v := pub.last()
	if v.Screens[0].Widgets[0].ID != "demo.increment" || v.Screens[0].Widgets[2].Target != "demo.settings" {
		t.Fatalf("unexpected semantic tree %+v", v.Screens[0].Widgets)
	}
	if !reflect.DeepEqual(v.Screens[0].Widgets[0].Variant, "primary") {
		t.Fatal("semantic variant missing")
	}
}

func TestRejectLiteralToggleInV7(t *testing.T) {
	src, e := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	if e != nil {
		t.Fatal(e)
	}
	src = bytes.Replace(src, []byte("checked: { bind: demo.enabled }"), []byte("checked: true"), 1)
	encoded, _, e := compile.Compile(src)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewForApp(&capture{}, encoded, "org.example.rui.demo"); e == nil {
		t.Fatal("accepted live toggle with immutable literal checked value")
	}
}
func TestRejectAmbiguousV7IntActions(t *testing.T) {
	src, e := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	if e != nil {
		t.Fatal(e)
	}
	marker := []byte("          - id: demo.reset")
	extra := []byte("          - id: demo.extraPicker\n            type: numberPicker\n            label: Extra count\n            value: 10\n            min: 10\n            max: 40\n            onCommit: demo.setCount\n")
	src = bytes.Replace(src, marker, append(extra, marker...), 1)
	encoded, _, e := compile.Compile(src)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewForApp(&capture{}, encoded, "org.example.rui.demo"); e == nil {
		t.Fatal("accepted ambiguous v7 int action with disjoint ranges")
	}
}

func TestRuntimeRejectsOversizeV7View(t *testing.T) {
	// Every individual widget respects M1 limits, but aggregate bound strings
	// must still respect the wire's 64 KiB frame. Build a valid bounded YAML.
	var src strings.Builder
	src.WriteString("schema: roundos.rui/v1\napplication:\n  id: org.example.large\nscreens:\n")
	for i := 0; i < 16; i++ {
		src.WriteString(fmt.Sprintf("  - id: large.screen%d\n    title: Large\n    layout: list\n    groups:\n      - id: one\n        widgets:\n", i))
		for j := 0; j < 30; j++ {
			src.WriteString(fmt.Sprintf("          - id: w%d\n            type: label\n            label: %s\n", j, strings.Repeat("X", 145)))
		}
	}
	if src.Len() > 128<<10 {
		t.Fatalf("test source too large: %d", src.Len())
	}
	data, _, e := compile.Compile([]byte(src.String()))
	if e != nil {
		t.Fatal(e)
	}
	pub := &capture{}
	r, e := NewForApp(pub, data, "org.example.large")
	if e != nil {
		t.Fatal(e)
	}
	if e := r.ValidateBindings(); e != nil {
		t.Fatal(e)
	}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if e := r.Flush(); e == nil || !strings.Contains(e.Error(), "IPC frame") {
		t.Fatalf("expected IPC frame rejection; got %v", e)
	}
	if pub.count() != 0 {
		t.Fatal("oversized view was published")
	}
}

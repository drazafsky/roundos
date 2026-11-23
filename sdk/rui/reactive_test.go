package rui

import (
	"bytes"
	"strings"
	"testing"

	"roundos/sdk/app"
)

func TestDirtyBindingsEvaluateOnceAndSkipUnchangedViews(t *testing.T) {
	_, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "Demo"
	registerDemo(t, r, &count, &enabled, &title)
	calls := map[string]int{}
	for name, b := range r.bindings {
		name, b := name, b
		wrapped := b
		if b.text != nil {
			wrapped.text = func() string { calls[name]++; return b.text() }
		}
		if b.integer != nil {
			wrapped.integer = func() int { calls[name]++; return b.integer() }
		}
		if b.boolean != nil {
			wrapped.boolean = func() bool { calls[name]++; return b.boolean() }
		}
		r.bindings[name] = wrapped
	}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	for name := range r.bindings {
		if calls[name] != 1 {
			t.Fatalf("initial getter %s called %d times", name, calls[name])
		}
	}
	count = 6
	for range 20 {
		if err := r.Invalidate("demo.display", "demo.count"); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if calls["demo.display"] != 2 || calls["demo.count"] != 2 || calls["demo.subtitle"] != 1 || calls["demo.enabled"] != 1 {
		t.Fatal(calls)
	}
	if pub.count() != 2 || pub.last().Screens[0].Hero != "006" || pub.last().Screens[1].Widgets[1].Value != 6 {
		t.Fatal("dirty properties not published")
	}
	r.Invalidate("demo.display", "demo.count")
	if err := r.Flush(); err != nil || pub.count() != 2 {
		t.Fatal("unchanged semantic values were published", err, pub.count())
	}
}

func TestReloadSeedsLastGoodCacheBeforeTransientGetterFailure(t *testing.T) {
	_, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "Retained"
	registerDemo(t, r, &count, &enabled, &title)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	data := replacement(t, func(source []byte) []byte {
		return bytes.Replace(source, []byte("title: RUI Demo"), []byte("title: Reloaded"), 1)
	})
	if err := r.Replace(data); err != nil {
		t.Fatal(err)
	}
	broken := r.bindings["demo.subtitle"]
	broken.text = func() string { panic("private") }
	r.bindings["demo.subtitle"] = broken
	count = 6
	r.Invalidate("demo.subtitle", "demo.count")
	if err := r.Flush(); err == nil {
		t.Fatal("missing transient getter error")
	}
	if pub.last().Screens[0].Subtitle != "Retained" || pub.last().Screens[1].Widgets[1].Value != 6 {
		t.Fatal("reload discarded last-good values and blocked independent update")
	}
}

func TestFailedGetterKeepsLastPropertyWhileOtherBindingsUpdate(t *testing.T) {
	_, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "Last good"
	registerDemo(t, r, &count, &enabled, &title)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	old := r.bindings["demo.subtitle"]
	broken := old
	broken.text = func() string { panic("private getter payload") }
	r.bindings["demo.subtitle"] = broken
	count = 6
	r.Invalidate("demo.subtitle", "demo.count")
	err := r.Flush()
	if err == nil || !strings.Contains(err.Error(), "demo.subtitle") || strings.Contains(err.Error(), "private getter payload") {
		t.Fatal("getter error missing or unsanitized", err)
	}
	if pub.last().Screens[0].Subtitle != "Last good" || pub.last().Screens[1].Widgets[1].Value != 6 {
		t.Fatal("failed property corrupted the tree or blocked a valid update")
	}
	r.bindings["demo.subtitle"] = old
	title = "Recovered"
	if err := r.Flush(); err != nil || pub.last().Screens[0].Subtitle != "Recovered" {
		t.Fatal("failed binding was not retried", err)
	}
}

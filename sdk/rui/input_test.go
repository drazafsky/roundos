package rui

import (
	"bytes"
	"context"
	"testing"

	"roundos/sdk/app"
	"roundos/sdk/ui"
)

type typedCapture struct {
	capture
	revision      uint64
	authoritative int
}

func (c *typedCapture) LayoutRevision() uint64 { return c.revision }
func (c *typedCapture) PublishAuthoritative(view ui.View) error {
	c.authoritative++
	return c.Publish(view)
}

func TestTypedToggleUsesExactValueAndDeduplicatesEvents(t *testing.T) {
	data, _, _ := loadDemo(t)
	pub := &typedCapture{revision: 4}
	r, err := NewForApp(pub, data, "org.example.rui.demo")
	if err != nil {
		t.Fatal(err)
	}
	count, enabled, title := 5, true, "Demo"
	registerDemo(t, r, &count, &enabled, &title)
	calls := 0
	r.handlers["demo.setEnabled"] = handler{kind: "bool", boolean: func(_ context.Context, value bool) error { calls++; enabled = value; return nil }}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	target := true
	event := app.Event{Type: "input", Action: "demo.setEnabled", UI: &app.UIEvent{Screen: "demo.settings", Widget: "demo.enabled", Action: "demo.setEnabled", Kind: "bool", Bool: &target, LayoutRevision: 4, ID: 1}}
	for range 2 {
		if _, err := r.Dispatch(event); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || !enabled {
		t.Fatal("exact-state event toggled or dispatched twice", calls, enabled)
	}
	target = false
	event.UI.ID = 2
	if _, err := r.Dispatch(event); err != nil || enabled || calls != 2 {
		t.Fatal("explicit false target not applied", err)
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	event.UI.ID = 3
	event.UI.LayoutRevision = 3
	target = true
	if _, err := r.Dispatch(event); err == nil || enabled || calls != 2 {
		t.Fatal("stale event changed state", err)
	}
	if err := r.Flush(); err != nil || pub.authoritative != 1 || pub.last().Screens[1].Widgets[0].Checked {
		t.Fatal("rejected optimism was not corrected", err)
	}
}

func TestHiddenAndDisabledWidgetsCannotDispatchLegacyActions(t *testing.T) {
	for _, condition := range []string{"visibleWhen", "enabledWhen"} {
		t.Run(condition, func(t *testing.T) {
			data := replacement(t, func(source []byte) []byte {
				return bytes.Replace(source, []byte("label: Add One"), []byte("label: Add One\n            "+condition+": { bind: demo.enabled }"), 1)
			})
			pub := &capture{}
			r, err := NewForApp(pub, data, "org.example.rui.demo")
			if err != nil {
				t.Fatal(err)
			}
			count, enabled, title := 5, false, "Demo"
			registerDemo(t, r, &count, &enabled, &title)
			r.Dispatch(app.Event{Type: "visibility", Visible: true})
			if err := r.Flush(); err != nil {
				t.Fatal(err)
			}
			if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); !handled || err != nil || count != 5 {
				t.Fatal("inactive widget invoked handler", err, count)
			}
			enabled = true
			if _, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); err != nil || count != 6 {
				t.Fatal("eligible widget did not recover", err)
			}
			r.Dispatch(app.Event{Type: "visibility", Visible: false})
			if _, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); err != nil || count != 6 {
				t.Fatal("background widget invoked handler", err)
			}
		})
	}
}

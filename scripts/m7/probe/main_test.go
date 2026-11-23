package main

import (
	"encoding/json"
	"strings"
	"testing"

	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/ui"
)

type peer struct {
	views   []ui.View
	secure  bool
	initial string
}

func (p *peer) Publish(v ui.View) error { p.views = append(p.views, v); return nil }
func (p *peer) RequestText(_, _, value string, secure bool) error {
	p.initial, p.secure = value, secure
	return nil
}

func TestSecureFixtureNeverPublishesOrRetainsSubmittedValue(t *testing.T) {
	p := &peer{}
	state := &probeState{status: "Initial", enabled: true, shown: true}
	r, err := newProbe(p, p, state)
	if err != nil {
		t.Fatal(err)
	}
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Dispatch(app.Event{Type: "input", Action: "editSecret"}); err != nil {
		t.Fatal(err)
	}
	if !p.secure || p.initial != "" {
		t.Fatal("secure editor received prefilled state")
	}
	result := &wire.TextResult{ID: "secret", Value: "disposable-unit-test-value"}
	state.textResult(app.Event{Type: "text_result", TextResult: result})
	r.Invalidate("probe.status")
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(p.views)
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != "" || strings.Contains(string(encoded), "disposable-unit-test-value") || state.status != "SECURE EDIT COMPLETED" {
		t.Fatal("secure value escaped the acceptance fixture")
	}
	// A subsequent editor request also starts empty.
	r.Dispatch(app.Event{Type: "input", Action: "editSecret"})
	if p.initial != "" {
		t.Fatal("secure value retained for the next edit")
	}
}

package main

import (
	"testing"
	"time"

	"roundos/internal/notify"
	"roundos/internal/touch"
	"roundos/internal/update"
)

func TestInstallationOutcomeAfterRestartProducesNonModalNotice(t *testing.T) {
	for _, phase := range []string{"succeeded", "rolled_back", "failed", "recovery_required"} {
		request, ok := installationOutcomeNotice(update.InstallStatus{ID: "persisted", Version: "0.6.0", Phase: phase})
		if !ok || request.Title == "" || request.Body == "" || request.Priority != notify.Actionable {
			t.Fatalf("missing %s outcome notice", phase)
		}
	}
	if _, ok := installationOutcomeNotice(update.InstallStatus{Phase: "preparing"}); ok {
		t.Fatal("unfinished installation reported as complete")
	}
}

func TestInputCapturedBeforePromptCannotChooseAnUpdateAction(t *testing.T) {
	shown := time.Now()
	before := shown.Add(-time.Millisecond)
	for _, kind := range []string{"press", "rotate", "button_down", "button_up", "touch_frame"} {
		a := action{kind: kind, at: before, frame: touch.Frame{At: before}}
		if !staleUpdateInput(a, shown) {
			t.Fatalf("queued %s crossed the prompt boundary", kind)
		}
		a.at = shown.Add(time.Millisecond)
		a.frame.At = a.at
		if staleUpdateInput(a, shown) {
			t.Fatalf("fresh %s was suppressed", kind)
		}
	}
	if staleUpdateInput(action{kind: "wifi_result", at: before}, shown) {
		t.Fatal("prompt suppressed an unrelated result")
	}
}

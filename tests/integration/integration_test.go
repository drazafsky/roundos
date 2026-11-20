package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/wire"
)

func build(t *testing.T, pkg, name, root string) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", dest, pkg)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s failed: %v\n%s", name, err, out)
	}
	return dest
}

type eventInbox struct {
	events  <-chan appmgr.Event
	pending []appmgr.Event
}

func newEventInbox(events <-chan appmgr.Event) *eventInbox { return &eventInbox{events: events} }

func get(t *testing.T, inbox *eventInbox, app, kind string, timeout time.Duration) appmgr.Event {
	t.Helper()
	for i, event := range inbox.pending {
		if event.App == app && event.Kind == kind {
			inbox.pending = append(inbox.pending[:i], inbox.pending[i+1:]...)
			return event
		}
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev := <-inbox.events:
			if ev.App == app && ev.Kind == kind {
				return ev
			}
			// Background initialization can precede another app's connection.
			// Retain those events just as the real shell retains app views.
			inbox.pending = append(inbox.pending, ev)
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s %s", app, kind)
		}
	}
}
func TestSeparateProcessesBackgroundAndRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two OS processes")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	clockExe := build(t, "./cmd/round-clock", "round-clock", root)
	timerExe := build(t, "./cmd/round-timer", "round-timer", root)
	stateDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := appmgr.New(ctx, clockExe, timerExe, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	events := newEventInbox(mgr.Events())
	get(t, events, wire.ClockID, "connected", 5*time.Second)
	get(t, events, wire.TimerID, "connected", 5*time.Second)
	if err := mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	initial := get(t, events, wire.TimerID, "view", 5*time.Second)
	if initial.View.Primary != "05:00" {
		t.Fatalf("initial timer: %+v", initial.View)
	}
	if !mgr.Input(wire.TimerID, "toggle", 0) {
		t.Fatal("timer input not delivered")
	}
	started := get(t, events, wire.TimerID, "view", 5*time.Second)
	if started.View.ButtonLabel != "PAUSE" {
		t.Fatalf("timer did not start: %+v", started.View)
	}
	if err := mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	clock := get(t, events, wire.ClockID, "view", 5*time.Second)
	if clock.View.Primary == "" {
		t.Fatal("clock missing UI")
	}
	time.Sleep(2200 * time.Millisecond)
	if err := mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	resumed := get(t, events, wire.TimerID, "view", 5*time.Second)
	if resumed.View.Primary == "05:00" {
		t.Fatalf("timer did not count down while backgrounded: %+v", resumed.View)
	}
	// Crash only Timer. Its saved deadline must survive supervisor restart.
	if !mgr.Input(wire.TimerID, "crash", 0) {
		t.Fatal("unable to send crash test")
	}
	get(t, events, wire.TimerID, "error", 5*time.Second)
	get(t, events, wire.TimerID, "connected", 5*time.Second)
	recovered := get(t, events, wire.TimerID, "view", 5*time.Second)
	if recovered.View.ButtonLabel != "PAUSE" {
		t.Fatalf("timer did not recover countdown: %+v", recovered.View)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "timer.json")); err != nil {
		t.Fatalf("timer state not saved: %v", err)
	}
}

// The Clock app requests shell-owned text editing, then accepts the edited
// value through the authenticated IPC channel. This catches protocol/SDK
// regressions that isolated model tests would miss.
func TestClockTextRequestAndResult(t *testing.T) {
	if testing.Short() {
		t.Skip("starts independent application processes")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	clockExe := build(t, "./cmd/round-clock", "round-clock", root)
	timerExe := build(t, "./cmd/round-timer", "round-timer", root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := appmgr.New(ctx, clockExe, timerExe, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	events := newEventInbox(mgr.Events())
	if err := mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	initial := get(t, events, wire.ClockID, "view", 5*time.Second)
	if initial.View.ButtonAction != "edit_label" {
		t.Fatalf("clock missing editing action: %+v", initial.View)
	}
	if !mgr.Input(wire.ClockID, "edit_label", 0) {
		t.Fatal("failed to send app action")
	}
	requested := get(t, events, wire.ClockID, "text_request", 5*time.Second)
	if requested.TextRequest == nil || requested.TextRequest.ID != "clock-label" || requested.TextRequest.Secure {
		t.Fatalf("request %+v", requested.TextRequest)
	}
	if !mgr.TextResult(wire.ClockID, wire.TextResult{ID: "clock-label", Value: "HELLO"}) {
		t.Fatal("text result not delivered")
	}
	updated := get(t, events, wire.ClockID, "view", 5*time.Second)
	if updated.View.Hint != "HELLO" {
		t.Fatalf("clock did not receive input: %+v", updated.View)
	}
}

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/timerstate"
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
			switch ev.Kind {
			case "connected", "disconnected", "error", "stopped":
				kept := inbox.pending[:0]
				for _, pending := range inbox.pending {
					if pending.App != ev.App || pending.Kind != "view" {
						kept = append(kept, pending)
					}
				}
				inbox.pending = kept
			}
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

func TestInboxDiscardsViewsFromBeforeRestart(t *testing.T) {
	channel := make(chan appmgr.Event, 4)
	inbox := newEventInbox(channel)
	inbox.pending = []appmgr.Event{
		{App: wire.TimerID, Kind: "view", Detail: "stale"},
		{App: wire.ClockID, Kind: "view", Detail: "keep"},
	}
	channel <- appmgr.Event{App: wire.TimerID, Kind: "view", Detail: "in flight before crash"}
	channel <- appmgr.Event{App: wire.TimerID, Kind: "error"}
	channel <- appmgr.Event{App: wire.TimerID, Kind: "connected"}
	channel <- appmgr.Event{App: wire.TimerID, Kind: "view", Detail: "recovered"}
	get(t, inbox, wire.TimerID, "error", time.Second)
	get(t, inbox, wire.TimerID, "connected", time.Second)
	if event := get(t, inbox, wire.TimerID, "view", time.Second); event.Detail != "recovered" {
		t.Fatalf("old app generation satisfied recovery: %+v", event)
	}
	if event := get(t, inbox, wire.ClockID, "view", time.Second); event.Detail != "keep" {
		t.Fatal("unrelated startup view was lost")
	}
}

func timerViewAtDeadline(t *testing.T, inbox *eventInbox, deadline time.Time) appmgr.Event {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for {
		event := get(t, inbox, wire.TimerID, "view", time.Until(until))
		var minutes, seconds int
		if n, err := fmt.Sscanf(event.View.Primary, "%d:%d", &minutes, &seconds); err != nil || n != 2 {
			t.Fatalf("invalid Timer countdown: %q", event.View.Primary)
		}
		// Labels round up to seconds. Permit delivery jitter, but do not let
		// cached pre-background views stand in for the current saved deadline.
		difference := time.Duration(minutes*60+seconds)*time.Second - time.Until(deadline)
		if difference >= 0 && difference <= time.Second+100*time.Millisecond {
			return event
		}
	}
}

func TestTimerDeadlineAssertionSkipsStaleBufferedView(t *testing.T) {
	channel := make(chan appmgr.Event, 1)
	inbox := newEventInbox(channel)
	inbox.pending = []appmgr.Event{{App: wire.TimerID, Kind: "view", View: wire.View{Primary: "04:59"}, Detail: "stale"}}
	channel <- appmgr.Event{App: wire.TimerID, Kind: "view", View: wire.View{Primary: "04:57"}, Detail: "current"}
	if event := timerViewAtDeadline(t, inbox, time.Now().Add(297*time.Second)); event.Detail != "current" {
		t.Fatal("stale buffered countdown satisfied the deadline assertion")
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
	persisted := timerstate.Load(filepath.Join(stateDir, "timer.json"), time.Now())
	if !persisted.Running || persisted.Deadline.IsZero() {
		t.Fatal("running Timer deadline was not persisted")
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
	timerViewAtDeadline(t, events, persisted.Deadline)
	// Crash only Timer. Its saved deadline must survive supervisor restart.
	if !mgr.Input(wire.TimerID, "crash", 0) {
		t.Fatal("unable to send crash test")
	}
	get(t, events, wire.TimerID, "error", 5*time.Second)
	get(t, events, wire.TimerID, "connected", 5*time.Second)
	recovered := timerViewAtDeadline(t, events, persisted.Deadline)
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
	for _, label := range []string{"HELLO", strings.Repeat("x", 256), strings.Repeat("é", 256), strings.Repeat("🌍", 256)} {
		if !mgr.TextResult(wire.ClockID, wire.TextResult{ID: "clock-label", Value: label}) {
			t.Fatal("text result not delivered")
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			updated := get(t, events, wire.ClockID, "view", time.Until(deadline))
			if updated.View.Hint == label && updated.View.Screens[0].Subtitle == label {
				break
			}
		}
	}
}

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
func get(t *testing.T, ch <-chan appmgr.Event, app, kind string, timeout time.Duration) appmgr.Event {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case ev := <-ch:
			if ev.App == app && ev.Kind == kind {
				return ev
			}
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
	connected := map[string]bool{}
	startupTimeout := time.NewTimer(5 * time.Second)
	defer startupTimeout.Stop()
	for len(connected) < 2 {
		select {
		case ev := <-mgr.Events():
			if ev.Kind == "connected" {
				connected[ev.App] = true
			}
			if ev.Kind == "error" {
				t.Fatalf("startup error from %s: %s", ev.App, ev.Detail)
			}
		case <-startupTimeout.C:
			t.Fatalf("startup timeout: %+v", connected)
		}
	}
	if err := mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	initial := get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
	if initial.View.Primary != "05:00" {
		t.Fatalf("initial timer: %+v", initial.View)
	}
	if !mgr.Input(wire.TimerID, "toggle", 0) {
		t.Fatal("timer input not delivered")
	}
	started := get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
	if started.View.ButtonLabel != "PAUSE" {
		t.Fatalf("timer did not start: %+v", started.View)
	}
	if err := mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	clock := get(t, mgr.Events(), wire.ClockID, "view", 5*time.Second)
	if clock.View.Primary == "" {
		t.Fatal("clock missing UI")
	}
	time.Sleep(2200 * time.Millisecond)
	if err := mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	resumed := get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
	if resumed.View.Primary == "05:00" {
		t.Fatalf("timer did not count down while backgrounded: %+v", resumed.View)
	}
	// Crash only Timer. Its saved deadline must survive supervisor restart.
	if !mgr.Input(wire.TimerID, "crash", 0) {
		t.Fatal("unable to send crash test")
	}
	get(t, mgr.Events(), wire.TimerID, "error", 5*time.Second)
	get(t, mgr.Events(), wire.TimerID, "connected", 5*time.Second)
	recovered := get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
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
	if err := mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	initial := get(t, mgr.Events(), wire.ClockID, "view", 5*time.Second)
	if initial.View.ButtonAction != "edit_label" {
		t.Fatalf("clock missing editing action: %+v", initial.View)
	}
	if !mgr.Input(wire.ClockID, "edit_label", 0) {
		t.Fatal("failed to send app action")
	}
	requested := get(t, mgr.Events(), wire.ClockID, "text_request", 5*time.Second)
	if requested.TextRequest == nil || requested.TextRequest.ID != "clock-label" || requested.TextRequest.Secure {
		t.Fatalf("request %+v", requested.TextRequest)
	}
	if !mgr.TextResult(wire.ClockID, wire.TextResult{ID: "clock-label", Value: "HELLO"}) {
		t.Fatal("text result not delivered")
	}
	updated := get(t, mgr.Events(), wire.ClockID, "view", 5*time.Second)
	if updated.View.Hint != "HELLO" {
		t.Fatalf("clock did not receive input: %+v", updated.View)
	}
}

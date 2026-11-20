package integration

import (
	"context"
	"path/filepath"
	"roundos/internal/appmgr"
	"roundos/internal/wire"
	"runtime"
	"testing"
	"time"
)

// RC1 integration verifies actual independent application processes, not PNG
// preview fixtures. IPC version 7 must carry complete semantic widget trees,
// modal dialogs, picker values, and changes back from the shell.
func TestRC1AppWidgetsDialogAndPickerIPC(t *testing.T) {
	if testing.Short() {
		t.Skip("launches separate application processes")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := appmgr.New(ctx, build(t, "./cmd/round-clock", "round-clock", root), build(t, "./cmd/round-timer", "round-timer", root), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	events := newEventInbox(mgr.Events())
	if err = mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	clock := get(t, events, wire.ClockID, "view", 5*time.Second)
	if len(clock.View.Screens) < 3 || clock.View.Screens[0].Widgets[0].Kind != "button" {
		t.Fatalf("Clock is not using a declarative widget tree: %+v", clock.View)
	}
	if !mgr.Input(wire.ClockID, "set_snooze", 12) {
		t.Fatal("picker input lost")
	}
	changed := get(t, events, wire.ClockID, "view", 5*time.Second)
	found := false
	for _, s := range changed.View.Screens {
		for _, w := range s.Widgets {
			if w.ID == "clock.snooze" && w.Value == 12 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("Clock did not publish selected picker value")
	}
	if err = mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	timer := get(t, events, wire.TimerID, "view", 5*time.Second)
	if len(timer.View.Screens) < 2 {
		t.Fatal("Timer missing widget tree")
	}
	if !mgr.Input(wire.TimerID, "request_reset", 0) {
		t.Fatal("reset action rejected")
	}
	asked := get(t, events, wire.TimerID, "view", 5*time.Second)
	if asked.View.Dialog == nil || !asked.View.Dialog.Destructive || asked.View.Dialog.CancelAction != "cancel_reset" {
		t.Fatalf("modal not published: %+v", asked.View.Dialog)
	}
	if !mgr.Input(wire.TimerID, "cancel_reset", 0) {
		t.Fatal("cancel action lost")
	}
	cleared := get(t, events, wire.TimerID, "view", 5*time.Second)
	if cleared.View.Dialog != nil {
		t.Fatal("dialog did not close after cancel")
	}
	if !mgr.Input(wire.TimerID, "set_minutes", 7) {
		t.Fatal("set minutes input lost")
	}
	adjusted := get(t, events, wire.TimerID, "view", 5*time.Second)
	if adjusted.View.Screens[1].Widgets[0].Value != 7 {
		t.Fatalf("Timer did not receive absolute minutes: %+v", adjusted.View.Screens[1].Widgets[0])
	}
}

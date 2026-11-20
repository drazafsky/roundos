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

// An independently built example process authenticates with the existing
// supervisor, publishes SDK widgets, and responds to app actions; no shell
// source modifications or IPC special-cases are required.
func TestOptionalAppSupervisorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("launches three separate app processes")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	demo := build(t, "./cmd/round-demo", "round-demo", root)
	clock := build(t, "./cmd/round-clock", "round-clock", root)
	timer := build(t, "./cmd/round-timer", "round-timer", root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	extra := []appmgr.AppSpec{{ID: "org.roundos.demo", Name: "Demo", Icon: "DME", Exec: demo}}
	mgr, err := appmgr.NewWithApps(ctx, clock, timer, t.TempDir(), extra)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	events := newEventInbox(mgr.Events())
	if err = mgr.SetForeground("org.roundos.demo"); err != nil {
		t.Fatal(err)
	}
	v := get(t, events, "org.roundos.demo", "view", 5*time.Second)
	if len(v.View.Screens) != 2 || v.View.Screens[0].Title != "DEMO" {
		t.Fatalf("demo widget tree: %+v", v.View)
	}
	if !mgr.Input("org.roundos.demo", "increment", 0) {
		t.Fatal("app input not dispatched")
	}
	updated := get(t, events, "org.roundos.demo", "view", 5*time.Second)
	if updated.View.Screens[0].Subtitle != "COUNTED" {
		t.Fatalf("demo action not executed: %+v", updated.View)
	}
	if err = mgr.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	_ = get(t, events, wire.ClockID, "view", 5*time.Second)
}

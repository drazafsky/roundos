package integration

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/wire"
)

// Compare independently compiled, unmodified stable applications with the
// migrated processes, including their legacy scalar fields and every control.
func TestMigratedAppsRetainStablePresentation(t *testing.T) {
	if testing.Short() {
		t.Skip("builds stable and migrated processes")
	}
	legacy := legacySource(t)
	current, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	start := func(root, prefix string) (*appmgr.Manager, *eventInbox) {
		clock := build(t, "./cmd/round-clock", prefix+"-clock", root)
		timer := build(t, "./cmd/round-timer", prefix+"-timer", root)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		manager, err := appmgr.New(ctx, clock, timer, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { manager.Close() })
		return manager, newEventInbox(manager.Events())
	}
	old, oldEvents := start(legacy, "stable")
	next, nextEvents := start(current, "rui")
	compare := func(id string, match func(wire.View) bool) wire.View {
		t.Helper()
		read := func(events *eventInbox) wire.View {
			until := time.Now().Add(5 * time.Second)
			for {
				view := get(t, events, id, "view", time.Until(until)).View
				if match == nil || match(view) {
					return view
				}
			}
		}
		a := read(oldEvents)
		b := wire.WithoutTemplates(read(nextEvents))
		// Wall-clock formatting may cross a minute between process startups.
		if id == wire.ClockID {
			a.Primary = b.Primary
			a.Screens[0].Hero = b.Screens[0].Hero
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s presentation changed:\nstable: %+v\nRUI: %+v", id, a, b)
		}
		return b
	}
	// Original 0.5.0 publishes on foreground selection. The new processes
	// must already have readiness snapshots while they are still hidden.
	if err := old.SetForeground(wire.ClockID); err != nil {
		t.Fatal(err)
	}
	compare(wire.ClockID, nil)
	if err := old.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	compare(wire.TimerID, nil)
	for _, manager := range []*appmgr.Manager{old, next} {
		if err := manager.SetForeground(wire.ClockID); err != nil {
			t.Fatal(err)
		}
	}
	// Initial visibility may be delivered before or after the readiness view;
	// consume it only if it changes semantics by issuing explicit actions.
	for _, action := range []struct {
		name  string
		value int
	}{{"set_snooze", 12}, {"set_alarm_time", 615}, {"clock_help", 0}, {"close_help", 0}, {"toggle_24hour", 0}} {
		if !old.Input(wire.ClockID, action.name, action.value) || !next.Input(wire.ClockID, action.name, action.value) {
			t.Fatal("input not queued")
		}
		match := func(v wire.View) bool {
			switch action.name {
			case "set_snooze":
				return v.Screens[2].Widgets[1].Value == 12
			case "set_alarm_time":
				return v.Screens[2].Widgets[0].Value == 615
			case "clock_help":
				return v.Dialog != nil
			case "close_help":
				return v.Dialog == nil
			case "toggle_24hour":
				return !v.Screens[1].Widgets[0].Checked
			}
			return false
		}
		compare(wire.ClockID, match)
	}
	for _, letter := range "abcdefghijkl" {
		action := "demo_" + string(letter)
		if !old.Input(wire.ClockID, action, 0) || !next.Input(wire.ClockID, action, 0) {
			t.Fatal("gallery input not queued")
		}
		a := get(t, oldEvents, wire.ClockID, "notify", 5*time.Second).Notification
		b := get(t, nextEvents, wire.ClockID, "notify", 5*time.Second).Notification
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("gallery notification changed: %+v %+v", a, b)
		}
	}
}

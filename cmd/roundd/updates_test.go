package main

import (
	"testing"
	"time"

	"roundos/internal/shell"
)

func TestUpdatesRefreshDeadlineSurvivesFrequentEvents(t *testing.T) {
	m := shell.New()
	m.OpenUpdates()
	base := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	deadline := base.Add(time.Second)
	for _, elapsed := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 900 * time.Millisecond} {
		now := base.Add(elapsed)
		if got := now.Add(nextWake(m, now, deadline)); !got.Equal(deadline) {
			t.Fatalf("unrelated event postponed Updates refresh until %v", got)
		}
	}
	now := base.Add(1001 * time.Millisecond)
	if !updatesRefreshDue(m.Page, now, deadline) {
		t.Fatal("boundary-crossing handler lost the overdue refresh")
	}
	if nextWake(m, now, deadline) > time.Millisecond {
		t.Fatal("overdue refresh was postponed")
	}
	deadline = now.Add(time.Second) // advanced only after submitting a render
	if updatesRefreshDue(m.Page, now, deadline) {
		t.Fatal("render did not service the refresh")
	}
	m.Back()
	if next := nextWake(m, base, deadline); next != 0 {
		t.Fatalf("Updates refresh remained active after leaving the page: %v", next)
	}
}

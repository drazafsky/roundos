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
	for _, elapsed := range []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 900 * time.Millisecond} {
		now := base.Add(elapsed)
		if deadline := now.Add(nextWake(m, now)); !deadline.Equal(base.Add(time.Second)) {
			t.Fatalf("unrelated event postponed Updates refresh until %v", deadline)
		}
	}
	m.Back()
	if next := nextWake(m, base); next != 0 {
		t.Fatalf("Updates refresh remained active after leaving the page: %v", next)
	}
}

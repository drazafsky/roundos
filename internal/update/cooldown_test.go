package update

import (
	"testing"
	"time"
)

func TestCooldownBoundariesAndRestart(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		last time.Time
		wait time.Duration
	}{
		{"never attempted", time.Time{}, 0},
		{"just attempted", now, CheckCooldown},
		{"restart during cooldown", now.Add(-5 * time.Minute), 10 * time.Minute},
		{"exact expiry", now.Add(-CheckCooldown), 0},
		{"old attempt", now.Add(-24 * time.Hour), 0},
		{"clock moved back across boots", now.Add(24 * time.Hour), CheckCooldown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewCooldown(tc.last, now)
			if got := c.Remaining(now); got != tc.wait {
				t.Fatalf("wait %v, want %v", got, tc.wait)
			}
			if got := c.Remaining(now.Add(tc.wait)); got != 0 {
				t.Fatalf("cooldown did not expire: %v", got)
			}
		})
	}
}

func TestCooldownSurvivesBackwardsClockWithoutReplenishing(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	c := NewCooldown(now, now)
	if got := c.Remaining(now.Add(5 * time.Minute)); got != 10*time.Minute {
		t.Fatalf("initial wait: %v", got)
	}
	back := now.Add(-24 * time.Hour)
	if got := c.Remaining(back); got != 10*time.Minute {
		t.Fatalf("backwards clock replenished or bypassed wait: %v", got)
	}
	if got := c.Remaining(back.Add(10 * time.Minute)); got != 0 {
		t.Fatalf("backwards clock blocked checking indefinitely: %v", got)
	}
	if got := c.Remaining(back.Add(-time.Hour)); got != 0 {
		t.Fatalf("expired cooldown came back: %v", got)
	}
	c.Start(back)
	if got := c.Remaining(back); got != CheckCooldown {
		t.Fatalf("new attempt did not start a full wait: %v", got)
	}
}

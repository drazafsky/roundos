package update

import "time"

// Cooldown tracks a bounded remaining wait across clock corrections. During a
// running process, time.Time's monotonic component handles wall-clock changes.
// Explicit backwards clock changes rebase observation without replenishing the
// wait, so subsequent forward time can still expire it. Use from one event loop.
type Cooldown struct {
	remaining time.Duration
	observed  time.Time
}

func NewCooldown(lastAttempt, now time.Time) Cooldown {
	var remaining time.Duration
	if !lastAttempt.IsZero() {
		remaining = max(0, min(CheckCooldown, lastAttempt.Add(CheckCooldown).Sub(now)))
	}
	return Cooldown{remaining: remaining, observed: now}
}

func (c *Cooldown) Remaining(now time.Time) time.Duration {
	if elapsed := now.Sub(c.observed); elapsed > 0 {
		c.remaining = max(0, c.remaining-elapsed)
	}
	c.observed = now
	return c.remaining
}

func (c *Cooldown) Start(now time.Time) {
	c.remaining = CheckCooldown
	c.observed = now
}

package update

import (
	"context"
	"errors"
	"time"
)

type ReleaseClient interface {
	Check(context.Context, string) (CheckResult, error)
}

type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

type CommandKind int

const (
	ManualCheck CommandKind = iota
	ChangePreferences
	IgnoreRelease
)

type Command struct {
	ID          uint64
	Kind        CommandKind
	Preferences Preferences
	Version     string
}

type Status struct {
	LastCommandID  uint64 // identifies the command acknowledged by this snapshot
	CommandErr     error  // retained outcome of LastCommandID, independent of later checks
	Preferences    Preferences
	IgnoredVersion string
	Busy           bool
	LastAttempt    time.Time
	NextManual     time.Time
	NextAutomatic  time.Time
	Sequence       uint64 // increases once per completed check
	Manual         bool
	Result         CheckResult
	Err            error // outcome of the most recently completed check
	OperationErr   error // a command or state-persistence failure
}

var (
	ErrBusy     = errors.New("a release check is already running")
	ErrCooldown = errors.New("release checking is cooling down")
)

// Service owns scheduling, persistence, and network operations on its own event
// loop. UI callers submit commands and consume immutable status snapshots.
type Service struct {
	commands chan Command
	events   chan Status
	done     chan struct{}
}

func Start(ctx context.Context, store *Store, client ReleaseClient, installed string, clock Clock) *Service {
	if clock == nil {
		clock = systemClock{}
	}
	s := &Service{commands: make(chan Command, 16), events: make(chan Status, 1), done: make(chan struct{})}
	go s.run(ctx, store, client, installed, clock)
	return s
}

func (s *Service) Events() <-chan Status { return s.events }
func (s *Service) Done() <-chan struct{} { return s.done }

// Submit never blocks the shell input/render loop. A full queue or stopped
// service returns false so the caller can display an operation error.
func (s *Service) Submit(command Command) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.commands <- command:
		return true
	default:
		return false
	}
}

func (s *Service) publish(status Status) {
	if status.Result.Release != nil {
		r := *status.Result.Release
		r.Assets = append([]Asset(nil), r.Assets...)
		status.Result.Release = &r
	}
	// Only the newest snapshot is needed; preserving Result and Sequence across
	// preference changes prevents a slow UI from missing a completed check.
	select {
	case <-s.events:
	default:
	}
	s.events <- status
}

func (s *Service) run(ctx context.Context, store *Store, client ReleaseClient, installed string, clock Clock) {
	defer close(s.done)
	defer close(s.events)
	state := store.Snapshot()
	now := clock.Now()
	cooldown := NewCooldown(state.LastAttempt, now)
	retryAfter := state.RetryAfter
	status := Status{Preferences: state.Preferences, IgnoredVersion: state.IgnoredVersion, LastAttempt: state.LastAttempt}
	automaticDue := now // each enabled startup checks as soon as eligible
	pendingAutomatic := state.Preferences.Automatic
	type completion struct {
		result CheckResult
		err    error
		manual bool
	}
	completed := make(chan completion, 1)
	startCheck := func(manual bool, now time.Time) {
		if status.Busy {
			status.OperationErr = ErrBusy
			return
		}
		if cooldown.Remaining(now) > 0 || now.Before(retryAfter) {
			status.OperationErr = ErrCooldown
			return
		}
		if err := store.RecordAttempt(now); err != nil {
			status.OperationErr = err
			status.LastAttempt = store.Snapshot().LastAttempt
			// A failed durable write must not send a request or cause a busy
			// retry loop. Manual retry is allowed after the same bounded wait.
			cooldown.Start(now)
			automaticDue = now.Add(CheckCooldown)
			return
		}
		cooldown.Start(now)
		pendingAutomatic = false
		status.LastAttempt = now
		status.Busy = true
		status.OperationErr = nil
		automaticDue = now.Add(time.Duration(status.Preferences.IntervalMinutes) * time.Minute)
		go func() {
			r, err := client.Check(ctx, installed)
			select {
			case completed <- completion{r, err, manual}:
			case <-ctx.Done():
			}
		}()
	}
	for {
		now = clock.Now()
		eligible := now.Add(cooldown.Remaining(now))
		if retryAfter.After(eligible) {
			eligible = retryAfter
		}
		status.NextManual = eligible
		status.NextAutomatic = time.Time{}
		var wake <-chan time.Time
		if status.Preferences.Automatic && !status.Busy {
			next := automaticDue
			if pendingAutomatic {
				next = now
			}
			if eligible.After(next) {
				next = eligible
			}
			status.NextAutomatic = next
			wake = clock.After(max(0, next.Sub(now)))
		}
		s.publish(status)
		select {
		case <-ctx.Done():
			return
		case <-wake:
			startCheck(false, clock.Now())
		case command := <-s.commands:
			if command.ID != 0 {
				status.LastCommandID = command.ID
			}
			now := clock.Now()
			switch command.Kind {
			case ManualCheck:
				startCheck(true, now)
			case ChangePreferences:
				previous := status.Preferences
				status.OperationErr = store.SetPreferences(command.Preferences)
				status.Preferences = store.Snapshot().Preferences
				if !previous.Automatic && status.Preferences.Automatic {
					pendingAutomatic = true
				} else if !status.Preferences.Automatic {
					pendingAutomatic = false
				} else if previous.IntervalMinutes != status.Preferences.IntervalMinutes {
					automaticDue = status.LastAttempt.Add(time.Duration(status.Preferences.IntervalMinutes) * time.Minute)
					if status.LastAttempt.IsZero() {
						automaticDue = now
					}
				}
			case IgnoreRelease:
				status.OperationErr = store.IgnoreVersion(command.Version)
				status.IgnoredVersion = store.Snapshot().IgnoredVersion
			}
			if command.ID != 0 {
				status.CommandErr = status.OperationErr
			}
		case reply := <-completed:
			status.Busy = false
			status.Sequence++
			status.Manual = reply.manual
			status.Result = reply.result
			status.Err = reply.err
			status.OperationErr = nil
			var limited *RateLimitError
			var observed time.Time
			if errors.As(reply.err, &limited) {
				reference := limited.ObservedAt
				if reference.IsZero() {
					reference = clock.Now()
				}
				// Store the deadline in the completion's clock domain. Time's
				// monotonic component preserves elapsed time during NTP changes;
				// a backwards synthetic clock retains a bounded remaining wait.
				observed = clock.Now()
				wait := max(time.Duration(0), limited.RetryAt.Sub(reference))
				elapsed := max(time.Duration(0), observed.Sub(reference))
				retryAfter = observed.Add(wait - min(wait, elapsed))
			} else {
				retryAfter = time.Time{}
			}
			if err := store.SetRetryAfter(retryAfter, observed); err != nil {
				status.OperationErr = err
			}
		}
	}
}

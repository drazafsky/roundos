package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
	} else {
		c.timers = append(c.timers, fakeTimer{c.now.Add(d), ch})
	}
	return ch
}
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	var remaining []fakeTimer
	for _, timer := range c.timers {
		if !c.now.Before(timer.at) {
			timer.ch <- c.now
		} else {
			remaining = append(remaining, timer)
		}
	}
	c.timers = remaining
}

type fakeReply struct {
	result CheckResult
	err    error
}
type fakeCall struct {
	ctx       context.Context
	installed string
	reply     chan fakeReply
}
type fakeClient struct{ calls chan fakeCall }

func (f *fakeClient) Check(ctx context.Context, installed string) (CheckResult, error) {
	call := fakeCall{ctx, installed, make(chan fakeReply, 1)}
	select {
	case f.calls <- call:
	case <-ctx.Done():
		return CheckResult{}, ctx.Err()
	}
	select {
	case reply := <-call.reply:
		return reply.result, reply.err
	case <-ctx.Done():
		return CheckResult{}, ctx.Err()
	}
}

func newServiceFixture(t *testing.T, automatic bool) (*Store, *fakeClock, *fakeClient, *Service, context.CancelFunc) {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	store, err := Open(filepath.Join(t.TempDir(), "updates.json"), clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	p := DefaultPreferences()
	p.Automatic = automatic
	if err := store.SetPreferences(p); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{calls: make(chan fakeCall, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	s := Start(ctx, store, client, "0.5.1-dev", clock)
	t.Cleanup(func() { cancel(); <-s.Done() })
	return store, clock, client, s, cancel
}

func waitStatus(t *testing.T, s *Service, ready func(Status) bool) Status {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case status, ok := <-s.Events():
			if !ok {
				t.Fatal("service stopped before expected status")
			}
			if ready(status) {
				return status
			}
		case <-timer.C:
			t.Fatal("timed out waiting for service status")
		}
	}
}

func waitCall(t *testing.T, f *fakeClient) fakeCall {
	t.Helper()
	select {
	case call := <-f.calls:
		return call
	case <-time.After(3 * time.Second):
		t.Fatal("expected release request")
		return fakeCall{}
	}
}

func noCall(t *testing.T, f *fakeClient) {
	t.Helper()
	select {
	case <-f.calls:
		t.Fatal("unexpected release request")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestServiceStartupFailureManualCooldownAndPeriod(t *testing.T) {
	store, clock, client, s, _ := newServiceFixture(t, true)
	first := waitCall(t, client)
	if first.installed != "0.5.1-dev" || !store.Snapshot().LastAttempt.Equal(clock.Now()) {
		t.Fatal("request started before recording its cooldown")
	}
	first.reply <- fakeReply{err: errors.New("offline")}
	waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy && st.Err != nil })
	s.Submit(Command{Kind: ManualCheck})
	waitStatus(t, s, func(st Status) bool { return errors.Is(st.OperationErr, ErrCooldown) })
	clock.Advance(CheckCooldown - time.Second)
	noCall(t, client)
	clock.Advance(time.Second)
	s.Submit(Command{Kind: ManualCheck})
	manual := waitCall(t, client)
	manual.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
	st := waitStatus(t, s, func(st Status) bool { return st.Sequence == 2 && !st.Busy })
	if !st.Manual || !st.NextAutomatic.Equal(clock.Now().Add(24*time.Hour)) {
		t.Fatalf("manual completion did not update periodic schedule: %+v", st)
	}
	clock.Advance(24 * time.Hour)
	periodic := waitCall(t, client)
	periodic.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
	st = waitStatus(t, s, func(st Status) bool { return st.Sequence == 3 && !st.Busy })
	if st.Manual {
		t.Fatal("periodic check marked manual")
	}
}

func TestServiceToggleIntervalAndBusyCoalescing(t *testing.T) {
	_, clock, client, s, _ := newServiceFixture(t, false)
	waitStatus(t, s, func(st Status) bool { return !st.Preferences.Automatic })
	noCall(t, client)
	s.Submit(Command{Kind: ManualCheck})
	first := waitCall(t, client)
	s.Submit(Command{Kind: ManualCheck})
	waitStatus(t, s, func(st Status) bool { return st.Busy && errors.Is(st.OperationErr, ErrBusy) })
	noCall(t, client)
	first.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
	waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy })
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{Automatic: true, IntervalMinutes: 15}})
	waitStatus(t, s, func(st Status) bool { return st.Preferences.Automatic && st.Preferences.IntervalMinutes == 15 })
	noCall(t, client)
	clock.Advance(CheckCooldown)
	auto := waitCall(t, client)
	auto.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
	waitStatus(t, s, func(st Status) bool { return st.Sequence == 2 && !st.Busy })
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{Automatic: true, IntervalMinutes: 30}})
	st := waitStatus(t, s, func(st Status) bool { return st.Preferences.IntervalMinutes == 30 })
	if !st.NextAutomatic.Equal(clock.Now().Add(30 * time.Minute)) {
		t.Fatal("interval change did not recalculate next check")
	}
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{IntervalMinutes: 30}})
	waitStatus(t, s, func(st Status) bool { return !st.Preferences.Automatic && st.NextAutomatic.IsZero() })
	clock.Advance(2 * time.Hour)
	noCall(t, client)
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{Automatic: true, IntervalMinutes: 30}})
	waitCall(t, client)
}

func TestServicePersistedRateLimitDefersStartup(t *testing.T) {
	store, clock, client, s, cancel := newServiceFixture(t, true)
	first := waitCall(t, client)
	deadline := clock.Now().Add(time.Hour)
	first.reply <- fakeReply{err: &RateLimitError{RetryAt: deadline}}
	st := waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy })
	if !st.NextManual.Equal(deadline) {
		t.Fatal("server rate limit not reflected in manual cooldown")
	}
	cancel()
	<-s.Done()
	clock.Advance(10 * time.Minute)
	reopened, err := Open(store.path, clock.Now())
	if err != nil || !reopened.Snapshot().RetryAfter.Equal(deadline) {
		t.Fatalf("rate limit not persisted: %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	s2 := Start(ctx, reopened, client, "0.5.1-dev", clock)
	defer func() { stop(); <-s2.Done() }()
	waitStatus(t, s2, func(st Status) bool { return st.NextAutomatic.Equal(deadline) })
	clock.Advance(49 * time.Minute)
	noCall(t, client)
	clock.Advance(time.Minute)
	waitCall(t, client)
}

func TestServicePersistenceFailureSendsNoRequest(t *testing.T) {
	now := time.Now()
	clock := &fakeClock{now: now}
	path := filepath.Join(t.TempDir(), "updates.json")
	store, _ := Open(path, now)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{calls: make(chan fakeCall, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	s := Start(ctx, store, client, "0.5.1", clock)
	defer func() { cancel(); <-s.Done() }()
	st := waitStatus(t, s, func(st Status) bool { return st.OperationErr != nil })
	if !st.NextManual.Equal(now.Add(CheckCooldown)) {
		t.Fatal("persistence failure did not back off")
	}
	noCall(t, client)
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{IntervalMinutes: 30}})
	st = waitStatus(t, s, func(st Status) bool { return st.OperationErr != nil })
	if st.Preferences != DefaultPreferences() {
		t.Fatal("failed preference save claimed success")
	}
}

func TestServiceShutdownCancelsInflightRequest(t *testing.T) {
	_, clock, client, s, cancel := newServiceFixture(t, true)
	call := waitCall(t, client)
	cancel()
	<-s.Done()
	select {
	case <-call.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("in-flight request was not canceled")
	}
	clock.Advance(48 * time.Hour)
	noCall(t, client)
	if s.Submit(Command{Kind: ManualCheck}) {
		t.Fatal("stopped service accepted a command")
	}
}

func TestServiceRetainsCheckFailureAcrossCommands(t *testing.T) {
	_, _, client, s, _ := newServiceFixture(t, true)
	first := waitCall(t, client)
	networkErr := errors.New("offline")
	first.reply <- fakeReply{err: networkErr}
	waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy })
	s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{IntervalMinutes: 30}})
	st := waitStatus(t, s, func(st Status) bool { return !st.Preferences.Automatic })
	if st.Sequence != 1 || !errors.Is(st.Err, networkErr) || st.OperationErr != nil {
		t.Fatalf("preference command discarded check failure: %+v", st)
	}
	s.Submit(Command{Kind: IgnoreRelease, Version: "0.6.0"})
	st = waitStatus(t, s, func(st Status) bool { return st.IgnoredVersion == "0.6.0" })
	if st.Sequence != 1 || !errors.Is(st.Err, networkErr) {
		t.Fatalf("ignore command discarded check failure: %+v", st)
	}
}

func TestServiceCrashAfterExpiredBackoffAndClockCorrection(t *testing.T) {
	oldNow := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "updates.json")
	store, _ := Open(path, oldNow)
	if err := store.RecordAttempt(oldNow); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRetryAfter(oldNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordAttempt(oldNow.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if !store.Snapshot().RetryAfter.IsZero() {
		t.Fatal("new attempt retained an expired backoff")
	}
	corrected := oldNow.Add(-24 * time.Hour)
	reopened, err := Open(path, corrected)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: corrected}
	client := &fakeClient{calls: make(chan fakeCall, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	s := Start(ctx, reopened, client, "0.5.1-dev", clock)
	defer func() { cancel(); <-s.Done() }()
	st := waitStatus(t, s, func(st Status) bool { return !st.NextAutomatic.IsZero() })
	if !st.NextManual.Equal(corrected.Add(CheckCooldown)) {
		t.Fatalf("expired backoff delayed recovery: %+v", st)
	}
	clock.Advance(CheckCooldown)
	waitCall(t, client)
}

func TestServiceActiveBackoffClockRecoveryPersistsAcrossRestart(t *testing.T) {
	oldNow := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "updates.json")
	store, _ := Open(path, oldNow)
	if err := store.RecordAttempt(oldNow); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRetryAfter(oldNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	corrected := oldNow.Add(-24 * time.Hour)
	deadline := corrected.Add(time.Hour)
	rebased, err := Open(path, corrected)
	if err != nil || !rebased.Snapshot().RetryAfter.Equal(deadline) {
		t.Fatalf("active backoff was not rebased: %+v, %v", rebased.Snapshot(), err)
	}
	// Reopen again after ten minutes to prove recovery was durably recorded,
	// rather than granting another full hour on every daemon restart.
	clock := &fakeClock{now: corrected.Add(10 * time.Minute)}
	reopened, err := Open(path, clock.Now())
	if err != nil || !reopened.Snapshot().RetryAfter.Equal(deadline) {
		t.Fatalf("rebased backoff was not persisted: %+v, %v", reopened.Snapshot(), err)
	}
	client := &fakeClient{calls: make(chan fakeCall, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	s := Start(ctx, reopened, client, "0.5.1-dev", clock)
	defer func() { cancel(); <-s.Done() }()
	waitStatus(t, s, func(st Status) bool { return st.NextAutomatic.Equal(deadline) && st.NextManual.Equal(deadline) })
	clock.Advance(49 * time.Minute)
	s.Submit(Command{Kind: ManualCheck})
	waitStatus(t, s, func(st Status) bool { return errors.Is(st.OperationErr, ErrCooldown) })
	noCall(t, client)
	clock.Advance(time.Minute)
	call := waitCall(t, client)
	call.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
	waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy })
}

func TestServiceIntervalChangePreservesPendingStartupAndEnableCheck(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		name := "enable"
		if automatic {
			name = "startup"
		}
		t.Run(name, func(t *testing.T) {
			attempt := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
			clock := &fakeClock{now: attempt.Add(time.Minute)}
			store, _ := Open(filepath.Join(t.TempDir(), "updates.json"), attempt)
			if err := store.SetPreferences(Preferences{Automatic: automatic, IntervalMinutes: 15}); err != nil {
				t.Fatal(err)
			}
			if err := store.RecordAttempt(attempt); err != nil {
				t.Fatal(err)
			}
			client := &fakeClient{calls: make(chan fakeCall, 8)}
			ctx, cancel := context.WithCancel(context.Background())
			s := Start(ctx, store, client, "0.5.1-dev", clock)
			defer func() { cancel(); <-s.Done() }()
			waitStatus(t, s, func(st Status) bool { return st.Preferences.Automatic == automatic })
			if !automatic {
				s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{Automatic: true, IntervalMinutes: 15}})
				waitStatus(t, s, func(st Status) bool { return st.Preferences.Automatic })
			}
			s.Submit(Command{Kind: ChangePreferences, Preferences: Preferences{Automatic: true, IntervalMinutes: 1440}})
			st := waitStatus(t, s, func(st Status) bool { return st.Preferences.IntervalMinutes == 1440 })
			if !st.NextAutomatic.Equal(attempt.Add(CheckCooldown)) {
				t.Fatalf("interval change postponed pending %s check: %+v", name, st)
			}
			clock.Advance(14 * time.Minute)
			call := waitCall(t, client)
			call.reply <- fakeReply{result: CheckResult{Status: UpToDate}}
			st = waitStatus(t, s, func(st Status) bool { return st.Sequence == 1 && !st.Busy })
			if !st.NextAutomatic.Equal(clock.Now().Add(24 * time.Hour)) {
				t.Fatal("new interval did not apply after pending check")
			}
		})
	}
}

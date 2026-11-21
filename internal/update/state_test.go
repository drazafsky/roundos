package update

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStateDefaultsAndRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "shell", "updates.json")
	s, err := Open(path, now)
	if err != nil || s.Snapshot() != (State{Preferences: DefaultPreferences()}) {
		t.Fatalf("defaults: %+v, %v", s.Snapshot(), err)
	}
	if !reflect.DeepEqual(IntervalPresets(), []int{15, 30, 60, 180, 360, 720, 1440}) {
		t.Fatal("unexpected interval choices")
	}
	for _, minutes := range IntervalPresets() {
		if err := s.SetPreferences(Preferences{Automatic: false, IntervalMinutes: minutes}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordAttempt(now); err != nil {
		t.Fatal(err)
	}
	if err := s.IgnoreVersion("0.6.0"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, now.Add(5*time.Minute))
	if err != nil || reopened.Snapshot() != s.Snapshot() {
		t.Fatalf("round trip: %+v, %v", reopened.Snapshot(), err)
	}
	cooldown := NewCooldown(reopened.Snapshot().LastAttempt, now.Add(5*time.Minute))
	if got := cooldown.Remaining(now.Add(5 * time.Minute)); got != 10*time.Minute {
		t.Fatalf("restart lost cooldown: %v", got)
	}
	if err := reopened.IgnoreVersion(""); err != nil || reopened.Snapshot().IgnoredVersion != "" {
		t.Fatalf("clear ignored version: %v", err)
	}
	for _, target := range []struct {
		path string
		mode os.FileMode
	}{{path, 0600}, {filepath.Dir(path), 0700}} {
		info, err := os.Stat(target.path)
		if err != nil || info.Mode().Perm() != target.mode {
			t.Fatalf("private permissions for %s: %v, %v", target.path, info, err)
		}
	}
	assertNoTemporaryFiles(t, filepath.Dir(path))
}

func TestInvalidPreferencesPreserveOtherState(t *testing.T) {
	for _, preferences := range []string{
		`{"automatic":false,"interval_minutes":14}`,
		`{"automatic":false,"interval_minutes":"invalid"}`,
		`{"automatic":"invalid","interval_minutes":30}`,
		`false`,
	} {
		t.Run(preferences, func(t *testing.T) {
			testInvalidPreferencesPreserveOtherState(t, preferences)
		})
	}
}

func testInvalidPreferencesPreserveOtherState(t *testing.T, preferences string) {
	t.Helper()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "updates.json")
	data := `{"preferences":` + preferences + `,"last_attempt":"2026-11-20T11:55:00Z","ignored_version":"0.6.0"}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, now)
	if err == nil {
		t.Fatal("expected preference recovery diagnostic")
	}
	got := s.Snapshot()
	if got.Preferences != DefaultPreferences() || got.IgnoredVersion != "0.6.0" || !got.LastAttempt.Equal(now.Add(-5*time.Minute)) {
		t.Fatalf("recovery discarded independent state: %+v", got)
	}
	if _, err := Open(path, now); err != nil {
		t.Fatalf("repair was not persisted: %v", err)
	}
}

func TestCorruptAndOversizedStateRecoverOnce(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for name, data := range map[string]string{
		"truncated": `{"preferences":`,
		"null":      `null`,
		"timestamp": `{"last_attempt":"not-a-time"}`,
		"oversized": strings.Repeat(" ", maxStateBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "updates.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path, now)
			if err == nil || s.Snapshot().Preferences != DefaultPreferences() || !s.Snapshot().LastAttempt.Equal(now) {
				t.Fatalf("unsafe recovery: %+v, %v", s.Snapshot(), err)
			}
			reopened, err := Open(path, now.Add(CheckCooldown))
			if err != nil {
				t.Fatal(err)
			}
			cooldown := NewCooldown(reopened.Snapshot().LastAttempt, now.Add(CheckCooldown))
			if cooldown.Remaining(now.Add(CheckCooldown)) != 0 {
				t.Fatal("recovery blocked checks indefinitely")
			}
		})
	}
}

func TestFutureAttemptIsRebasedAndPersisted(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "updates.json")
	s, err := Open(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAttempt(now.AddDate(5, 0, 0)); err != nil {
		t.Fatal(err)
	}
	rebased, err := Open(path, now)
	if err != nil || !rebased.Snapshot().LastAttempt.Equal(now) {
		t.Fatalf("future attempt: %+v, %v", rebased.Snapshot(), err)
	}
	reopened, err := Open(path, now.Add(CheckCooldown))
	if err != nil || !reopened.Snapshot().LastAttempt.Equal(now) {
		t.Fatalf("rebase was not persisted: %+v, %v", reopened.Snapshot(), err)
	}
}

func TestFailedWritesDoNotPublishState(t *testing.T) {
	now := time.Now()
	path := filepath.Join(t.TempDir(), "updates.json")
	s, err := Open(path, now)
	if err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	// A directory at the destination deterministically fails rename, including
	// when tests run as root. The temporary file must still be removed.
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func() error{
		func() error { return s.SetPreferences(Preferences{IntervalMinutes: 30}) },
		func() error { return s.RecordAttempt(now) },
		func() error { return s.IgnoreVersion("0.6.0") },
	} {
		if err := change(); err == nil {
			t.Fatal("failed write reported success")
		}
		if s.Snapshot() != before {
			t.Fatal("failed write was published in memory")
		}
		assertNoTemporaryFiles(t, filepath.Dir(path))
	}
	if _, err := Open(path, now); err == nil {
		t.Fatal("unreadable state was not reported")
	}
}

func TestInvalidChangesPreserveSavedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "updates.json")
	s, _ := Open(path, time.Now())
	if err := s.IgnoreVersion("0.6.0"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, minutes := range []int{-1, 0, 14, 16, 1441} {
		if err := s.SetPreferences(Preferences{IntervalMinutes: minutes}); err == nil {
			t.Fatalf("accepted interval %d", minutes)
		}
	}
	if err := s.RecordAttempt(time.Time{}); err == nil {
		t.Fatal("accepted missing check time")
	}
	if err := s.IgnoreVersion(strings.Repeat("v", maxStateBytes)); err == nil {
		t.Fatal("accepted state that cannot be reopened")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || s.Snapshot().IgnoredVersion != "0.6.0" {
		t.Fatal("invalid changes modified state")
	}
}

func TestConcurrentChangesPreserveIndependentFields(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "updates.json")
	s, _ := Open(path, now)
	var wg sync.WaitGroup
	for _, change := range []func() error{
		func() error { return s.SetPreferences(Preferences{IntervalMinutes: 30}) },
		func() error { return s.RecordAttempt(now) },
		func() error { return s.IgnoreVersion("0.6.0") },
	} {
		wg.Add(1)
		go func(change func() error) {
			defer wg.Done()
			if err := change(); err != nil {
				t.Error(err)
			}
		}(change)
	}
	wg.Wait()
	reopened, err := Open(path, now)
	want := State{Preferences: Preferences{IntervalMinutes: 30}, LastAttempt: now, IgnoredVersion: "0.6.0"}
	if err != nil || reopened.Snapshot() != want {
		t.Fatalf("concurrent changes lost state: %+v, %v", reopened.Snapshot(), err)
	}
}

func assertNoTemporaryFiles(t *testing.T, dir string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, ".updates-*.tmp"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary files remain: %v, %v", files, err)
	}
}

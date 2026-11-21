// Package update manages shell-owned release update preferences and state.
package update

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	CheckCooldown = 15 * time.Minute
	maxStateBytes = 64 * 1024
)

// IntervalPresets returns a fresh copy of the supported check intervals.
func IntervalPresets() []int { return []int{15, 30, 60, 180, 360, 720, 1440} }

type Preferences struct {
	Automatic       bool `json:"automatic"`
	IntervalMinutes int  `json:"interval_minutes"`
}

func DefaultPreferences() Preferences {
	return Preferences{Automatic: true, IntervalMinutes: 1440}
}

func (p Preferences) Validate() error {
	for _, minutes := range IntervalPresets() {
		if p.IntervalMinutes == minutes {
			return nil
		}
	}
	return errors.New("unsupported automatic update interval")
}

type State struct {
	Preferences    Preferences `json:"preferences"`
	LastAttempt    time.Time   `json:"last_attempt"`
	IgnoredVersion string      `json:"ignored_version,omitempty"`
}

// Store serializes state changes and only publishes them in memory after the
// atomic file replacement succeeds. The path belongs in the private shell state
// directory, never in an application's writable state directory.
type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

// Open always returns a usable store. A non-nil error reports a read, recovery,
// or write failure and must be surfaced by the caller. Invalid preferences are
// reset independently, preserving a valid last attempt and ignored version.
// Unreadable state conservatively starts one cooldown rather than permitting an
// immediate request on every restart.
func Open(path string, now time.Time) (*Store, error) {
	s := &Store{path: path, state: State{Preferences: DefaultPreferences()}}
	var preferenceErr error
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err == nil {
		defer f.Close()
		var data []byte
		data, err = io.ReadAll(io.LimitReader(f, maxStateBytes+1))
		if err == nil && len(data) > maxStateBytes {
			err = errors.New("update state exceeds size limit")
		}
		if trimmed := bytes.TrimSpace(data); err == nil && (len(trimmed) == 0 || trimmed[0] != '{') {
			err = errors.New("update state must be a JSON object")
		}
		if err == nil {
			// Decode preferences separately: a malformed preference must not
			// discard an otherwise valid cooldown or ignored release.
			var saved struct {
				Preferences    json.RawMessage `json:"preferences"`
				LastAttempt    time.Time       `json:"last_attempt"`
				IgnoredVersion string          `json:"ignored_version"`
			}
			err = json.Unmarshal(data, &saved)
			if err == nil {
				s.state.LastAttempt = saved.LastAttempt
				s.state.IgnoredVersion = saved.IgnoredVersion
				if len(saved.Preferences) != 0 {
					preferenceErr = json.Unmarshal(saved.Preferences, &s.state.Preferences)
					if preferenceErr == nil {
						preferenceErr = s.state.Preferences.Validate()
					}
				}
			}
		}
	}
	if err != nil {
		s.state = State{Preferences: DefaultPreferences(), LastAttempt: now}
	} else if preferenceErr != nil {
		s.state.Preferences = DefaultPreferences()
		err = preferenceErr
	}
	// Persist a rebased future attempt once. A corrected clock must not make a
	// timestamp from an earlier boot postpone checks for days or years.
	repair := err != nil || s.state.LastAttempt.After(now)
	if s.state.LastAttempt.After(now) {
		s.state.LastAttempt = now
	}
	if repair {
		err = errors.Join(err, writeState(path, s.state))
	}
	return s, err
}

func (s *Store) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Store) SetPreferences(p Preferences) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return s.change(func(next *State) { next.Preferences = p })
}

// RecordAttempt must succeed before starting a release request, including a
// manual request. Failed network attempts consume the same persisted cooldown.
func (s *Store) RecordAttempt(at time.Time) error {
	if at.IsZero() {
		return errors.New("update check attempt requires a timestamp")
	}
	return s.change(func(next *State) { next.LastAttempt = at })
}

// IgnoreVersion records a dismissed release; an empty version clears it.
func (s *Store) IgnoreVersion(version string) error {
	return s.change(func(next *State) { next.IgnoredVersion = version })
}

func (s *Store) change(change func(*State)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.state
	change(&next)
	if err := writeState(s.path, next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func writeState(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxStateBytes {
		return errors.New("update state exceeds size limit")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create update state directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".updates-*.tmp")
	if err != nil {
		return fmt.Errorf("create update state file: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replace update state: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Package timerstate implements the Timer application's clock and durable state.
package timerstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Timer struct {
	Remaining       time.Duration `json:"-"`
	Deadline        time.Time     `json:"-"`
	Running         bool          `json:"-"`
	CompletedOnLoad bool          `json:"-"`
	Path            string        `json:"-"`
}
type stored struct {
	RemainingMS    int64 `json:"remaining_ms"`
	DeadlineUnixMS int64 `json:"deadline_unix_ms"`
	Running        bool  `json:"running"`
}

func Load(path string, now time.Time) *Timer {
	t := &Timer{Remaining: 5 * time.Minute, Path: path}
	b, err := os.ReadFile(path)
	if err == nil {
		var s stored
		if json.Unmarshal(b, &s) == nil && s.RemainingMS >= 0 && s.RemainingMS <= 999*60*1000 {
			t.Remaining = time.Duration(s.RemainingMS) * time.Millisecond
			t.Running = s.Running
			if s.Running {
				t.Deadline = time.UnixMilli(s.DeadlineUnixMS)
			}
		}
	}
	t.CompletedOnLoad = t.Resolve(now)
	return t
}
func (t *Timer) Resolve(now time.Time) bool {
	if t.Running && !now.Before(t.Deadline) {
		t.Running = false
		t.Remaining = 0
		return true
	}
	return false
}
func (t *Timer) Left(now time.Time) time.Duration {
	if t.Running {
		left := t.Deadline.Sub(now)
		if left < 0 {
			return 0
		}
		return left
	}
	if t.Remaining < 0 {
		return 0
	}
	return t.Remaining
}
func (t *Timer) Rotate(steps int, now time.Time) bool {
	if t.Running || steps == 0 {
		return false
	}
	minutes := int(t.Remaining/time.Minute) + steps
	if minutes < 0 {
		minutes = 0
	}
	if minutes > 999 {
		minutes = 999
	}
	next := time.Duration(minutes) * time.Minute
	if next == t.Remaining {
		return false
	}
	t.Remaining = next
	return true
}
func (t *Timer) Toggle(now time.Time) {
	if t.Running {
		t.Remaining = t.Left(now)
		t.Running = false
	} else {
		if t.Remaining <= 0 {
			t.Remaining = 5 * time.Minute
		}
		t.Deadline = now.Add(t.Remaining)
		t.Running = true
	}
}
func (t *Timer) Label(now time.Time) string {
	left := t.Left(now)
	secs := int((left + time.Second - 1) / time.Second)
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}
func (t *Timer) Save() error {
	if t.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0700); err != nil {
		return err
	}
	s := stored{RemainingMS: t.Remaining.Milliseconds(), Running: t.Running}
	if t.Running {
		s.DeadlineUnixMS = t.Deadline.UnixMilli()
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(t.Path), ".timer-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, t.Path)
}

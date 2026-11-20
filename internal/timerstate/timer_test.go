package timerstate

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTimerContinuesInBackground(t *testing.T) {
	start := time.Now()
	timer := &Timer{Remaining: 5 * time.Minute}
	timer.Toggle(start)
	if !timer.Running {
		t.Fatal("not running")
	}
	after := start.Add(42 * time.Second)
	if got := timer.Left(after); got != 258*time.Second {
		t.Fatalf("left %s", got)
	}
	timer.Toggle(after)
	if timer.Running || timer.Remaining != 258*time.Second {
		t.Fatalf("pause state: %+v", timer)
	}
}
func TestDurabilityAndExpiry(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	path := filepath.Join(t.TempDir(), "timer.json")
	timer := Load(path, now)
	timer.Toggle(now)
	if err := timer.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := Load(path, now.Add(30*time.Second))
	if !loaded.Running || loaded.Left(now.Add(30*time.Second)) != 270*time.Second {
		t.Fatalf("load: %+v", loaded)
	}
	expired := Load(path, now.Add(6*time.Minute))
	if expired.Running || expired.Left(now.Add(6*time.Minute)) != 0 {
		t.Fatal("timer failed to expire")
	}
}
func TestRotation(t *testing.T) {
	x := &Timer{Remaining: 5 * time.Minute}
	if !x.Rotate(-10, time.Now()) || x.Remaining != 0 {
		t.Fatal("rotary lower bound")
	}
	if !x.Rotate(1200, time.Now()) || x.Remaining != 999*time.Minute {
		t.Fatal("rotary upper bound")
	}
}

func TestCompletionRecoveredAfterRestart(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	original := &Timer{Path: filepath.Join(dir, "expired.json"), Remaining: time.Minute, Running: true, Deadline: now.Add(-2 * time.Second)}
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	recovered := Load(original.Path, now)
	if recovered.Running || recovered.Remaining != 0 || !recovered.CompletedOnLoad {
		t.Fatalf("missed completion: %+v", recovered)
	}
	if err := recovered.Save(); err != nil {
		t.Fatal(err)
	}
	second := Load(original.Path, now)
	if second.CompletedOnLoad {
		t.Fatal("duplicate completion notification on second restart")
	}
}

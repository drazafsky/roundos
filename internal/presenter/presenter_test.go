package presenter

import (
	"image"
	"sync"
	"testing"
	"time"

	"roundos/internal/perf"
	"roundos/internal/shell"
)

type blockedDisplay struct {
	gate    chan struct{}
	entered chan struct{}
	once    sync.Once
	mu      sync.Mutex
	frames  int
}

func (d *blockedDisplay) Present(*image.RGBA) error {
	d.once.Do(func() { close(d.entered) })
	<-d.gate
	d.mu.Lock()
	d.frames++
	d.mu.Unlock()
	return nil
}
func (d *blockedDisplay) Count() int { d.mu.Lock(); defer d.mu.Unlock(); return d.frames }

func TestInputCannotBlockOnBusyDRM(t *testing.T) {
	d := &blockedDisplay{gate: make(chan struct{}), entered: make(chan struct{})}
	p := New(d, perf.New())
	m := shell.New()
	p.Submit(m, time.Now())
	select {
	case <-d.entered:
	case <-time.After(time.Second):
		close(d.gate)
		p.Close()
		t.Fatal("render never started")
	}
	// The first frame is deliberately stuck in DRM. A new touch/input event
	// must never wait for it, and the mailbox must not accumulate old images.
	began := time.Now()
	for i := 0; i < 200; i++ {
		m.Selected = i % 2
		p.Submit(m, time.Now())
	}
	elapsed := time.Since(began)
	if elapsed > 150*time.Millisecond {
		t.Errorf("input blocked behind page flip: %v", elapsed)
	}
	close(d.gate)
	// Both the initial frame and at most one pending latest frame are shown.
	deadline := time.Now().Add(time.Second)
	for d.Count() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p.Close()
	if got := d.Count(); got != 2 {
		t.Fatalf("expected coalesced frames, got %d", got)
	}
}

func TestModelSnapshotDoesNotShareMutableState(t *testing.T) {
	m := shell.New()
	snapshot := m.Snapshot()
	m.Selected = 1
	m.Status[shell.Timer] = "changed"
	if snapshot.Selected != 0 || snapshot.Status[shell.Timer] == "changed" {
		t.Fatal("snapshot changed after source mutated")
	}
}

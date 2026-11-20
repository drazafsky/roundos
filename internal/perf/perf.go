// Package perf records measurements only when a frame is actually presented.
// There is no periodic monitoring goroutine or heartbeat.
package perf

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sampleCap = 256

type Stats struct {
	mu             sync.Mutex
	started        time.Time
	Frames         uint64
	TotalFrameTime time.Duration
	MaxFrameTime   time.Duration
	frameSamples   [sampleCap]time.Duration
	frameN         int
	inputSamples   [sampleCap]time.Duration
	inputN         int
}

func New() *Stats { return &Stats{started: time.Now()} }

func (s *Stats) Frame(renderTime time.Duration, inputToPresent time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if renderTime < 0 {
		renderTime = 0
	}
	s.frameSamples[s.Frames%sampleCap] = renderTime
	if s.frameN < sampleCap {
		s.frameN++
	}
	s.Frames++
	s.TotalFrameTime += renderTime
	if renderTime > s.MaxFrameTime {
		s.MaxFrameTime = renderTime
	}
	if inputToPresent > 0 {
		s.inputSamples[s.inputN%sampleCap] = inputToPresent
		s.inputN++
	}
}

func percentile(values []time.Duration, n int, pct int) time.Duration {
	if n == 0 {
		return 0
	}
	if n > sampleCap {
		n = sampleCap
	}
	b := append([]time.Duration(nil), values[:n]...)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	index := (n*pct+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(b) {
		index = len(b) - 1
	}
	return b[index]
}
func (s *Stats) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.started)
	var avg time.Duration
	if s.Frames > 0 {
		avg = s.TotalFrameTime / time.Duration(s.Frames)
	}
	return fmt.Sprintf("frames=%d uptime=%s avg-frame=%s frame-p95=%s frame-max=%s input-to-present-p95=%s (n=%d; measured from app event handling, not kernel IRQ)",
		s.Frames, elapsed.Round(time.Second), avg.Round(time.Microsecond),
		percentile(s.frameSamples[:], s.frameN, 95).Round(time.Microsecond),
		s.MaxFrameTime.Round(time.Microsecond),
		percentile(s.inputSamples[:], s.inputN, 95).Round(time.Microsecond), s.inputN)
}

// RSS reads procfs only when diagnostics are requested; it does not poll.
func RSS(pid int) (uint64, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid statm for pid %d", pid)
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * uint64(os.Getpagesize()), nil
}

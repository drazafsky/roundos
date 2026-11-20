package perf

import (
	"strings"
	"testing"
	"time"
)

func TestStatsOnDemand(t *testing.T) {
	s := New()
	for i := 1; i <= 300; i++ {
		s.Frame(time.Duration(i)*time.Microsecond, 2*time.Millisecond)
	}
	summary := s.Summary()
	for _, key := range []string{"frames=300", "frame-p95=", "input-to-present-p95="} {
		if !strings.Contains(summary, key) {
			t.Errorf("missing %q from %q", key, summary)
		}
	}
}

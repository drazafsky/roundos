package gesture

import "testing"

// Hardware often emits a frame for the first updated finger before the other.
// That transient change in spacing must not permanently claim a pinch.
func TestTwoFingerStaggeredMotionRecognizesSwipe(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 160, 180)))
	d.Process(f(15, c(1, 160, 180), c(2, 270, 180)))
	// Simulate two separate sensor updates per movement increment.
	found := false
	for step := 1; step <= 6; step++ {
		dx := step * 15
		ev := d.Process(f(step*40, c(1, 160+dx, 180), c(2, 270+(step-1)*15, 180)))
		if contains(ev, Pinch) || contains(ev, Rotate) {
			t.Fatalf("phantom transform at first contact update step %d: %+v", step, ev)
		}
		if sw, ok := find(ev, TwoSwipe); ok {
			if sw.Direction != "right" {
				t.Fatalf("wrong direction: %+v", sw)
			}
			found = true
		}
		ev = d.Process(f(step*40+12, c(1, 160+dx, 180), c(2, 270+dx, 180)))
		if contains(ev, Pinch) || contains(ev, Rotate) {
			t.Fatalf("phantom transform at pair update step %d: %+v", step, ev)
		}
		if sw, ok := find(ev, TwoSwipe); ok {
			if sw.Direction != "right" {
				t.Fatalf("wrong direction: %+v", sw)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected a coherent two-finger swipe despite staggered contact updates")
	}
}

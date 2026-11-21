package gesture

import (
	"math"
	"roundos/internal/touch"
	"testing"
	"time"
)

var start = time.Unix(1000, 0)

func c(id, x, y int) touch.Contact { return touch.Contact{ID: id, Slot: id, X: x, Y: y} }
func f(ms int, cc ...touch.Contact) touch.Frame {
	return touch.Frame{At: start.Add(time.Duration(ms) * time.Millisecond), Contacts: cc}
}
func contains(events []Event, kind Type) bool {
	for _, e := range events {
		if e.Type == kind {
			return true
		}
	}
	return false
}
func find(events []Event, kind Type) (Event, bool) {
	for _, e := range events {
		if e.Type == kind {
			return e, true
		}
	}
	return Event{}, false
}

func TestCancelUntilReleaseSuppressesDragAndRemainingContacts(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 240, 190)))
	d.Process(f(40, c(1, 240, 240)))
	if !d.CancelUntilRelease() || !d.CancelUntilRelease() {
		t.Fatal("active contact was lost during repeated cancellation")
	}
	for _, frame := range []touch.Frame{f(80, c(1, 240, 240), c(2, 200, 240)), f(120, c(2, 200, 240)), f(160)} {
		if events := d.Process(frame); len(events) != 0 {
			t.Fatalf("canceled contact emitted events: %+v", events)
		}
	}
	if !d.Deadline().IsZero() || len(d.Tick(start.Add(time.Second))) != 0 {
		t.Fatal("canceled contact retained a long-press deadline")
	}
	if d.CancelUntilRelease() {
		t.Fatal("idle cancellation suppressed the next fresh contact")
	}
	d.Process(f(1100, c(3, 240, 190)))
	if !contains(d.Process(f(1180)), Tap) {
		t.Fatal("fresh tap was suppressed")
	}
}
func TestTapDoubleTap(t *testing.T) {
	d := New()
	if !contains(d.Process(f(0, c(1, 150, 150))), Down) {
		t.Fatal("down missing")
	}
	if !contains(d.Process(f(80)), Tap) {
		t.Fatal("tap missing")
	}
	d.Process(f(170, c(2, 154, 152)))
	ev := d.Process(f(230))
	if !contains(ev, DoubleTap) || !contains(ev, Tap) {
		t.Fatal(ev)
	}
}
func TestLongPressHasDeadlineNoTap(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 210, 220)))
	if d.Deadline().IsZero() {
		t.Fatal("no deadline")
	}
	if len(d.Tick(start.Add(400*time.Millisecond))) != 0 {
		t.Fatal("long press too soon")
	}
	if !contains(d.Tick(start.Add(700*time.Millisecond)), LongPress) {
		t.Fatal("long press missing")
	}
	if !d.Deadline().IsZero() {
		t.Fatal("long press deadline not cleared")
	}
	if contains(d.Process(f(800)), Tap) {
		t.Fatal("long press should not tap")
	}
}
func TestSingleFingerSwipeAndDrag(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 80, 180)))
	ev := d.Process(f(100, c(1, 210, 182)))
	if !contains(ev, DragStart) || !contains(ev, Drag) {
		t.Fatal(ev)
	}
	ev = d.Process(f(200))
	swipe, ok := find(ev, Swipe)
	if !ok || swipe.Direction != "right" || !contains(ev, DragEnd) || contains(ev, Tap) {
		t.Fatal(ev)
	}
}
func TestTwoFingerSwipeDoesNotTap(t *testing.T) {
	d := New()
	d.Process(f(0, c(3, 100, 160), c(4, 210, 160)))
	ev := d.Process(f(120, c(3, 180, 162), c(4, 290, 162)))
	swipe, ok := find(ev, TwoSwipe)
	if !ok || swipe.Direction != "right" || contains(ev, Swipe) {
		t.Fatal("two finger swipe should recognize before release", ev)
	}
	ev = d.Process(f(180, c(4, 290, 162)))
	if contains(ev, TwoSwipe) {
		t.Fatal("duplicate swipe on release", ev)
	}
	if contains(d.Process(f(230)), Tap) {
		t.Fatal("remaining finger generated tap")
	}
}
func TestPinchRotationAndConflictResolution(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 180, 230), c(2, 280, 230)))
	pinch, ok := find(d.Process(f(80, c(1, 150, 230), c(2, 310, 230))), Pinch)
	if !ok || math.Abs(pinch.Scale-1.6) > .01 {
		t.Fatalf("pinch: %+v %v", pinch, ok)
	}
	if contains(d.Process(f(140)), TwoSwipe) {
		t.Fatal("pinch wrongly recognized as swipe")
	}
	d = New()
	d.Process(f(0, c(1, 160, 240), c(2, 280, 240)))
	rot, ok := find(d.Process(f(80, c(1, 220, 180), c(2, 220, 300))), Rotate)
	if !ok || math.Abs(rot.Angle-90) > 1 {
		t.Fatalf("rotation: %+v %v", rot, ok)
	}
	if contains(d.Process(f(150)), TwoSwipe) {
		t.Fatal("rotation wrongly recognized as swipe")
	}
}
func TestCancelStopsGesture(t *testing.T) {
	d := New()
	d.Process(f(0, c(2, 150, 150)))
	if !contains(d.Process(touch.Frame{At: start.Add(100 * time.Millisecond), Canceled: true}), Cancel) {
		t.Fatal("no cancel")
	}
	if !d.Deadline().IsZero() {
		t.Fatal("deadline after cancel")
	}
	if contains(d.Process(f(150)), Tap) {
		t.Fatal("ghost tap")
	}
}

func TestSmallJitterDoesNotTurnTapIntoDrag(t *testing.T) {
	d := New()
	d.Process(f(0, c(10, 200, 210)))
	for i := 1; i <= 4; i++ {
		ev := d.Process(f(30*i, c(10, 200+i*3, 210-i*2)))
		if contains(ev, DragStart) || contains(ev, Drag) {
			t.Fatalf("jitter mistaken for drag: %v", ev)
		}
	}
	if !contains(d.Process(f(180)), Tap) {
		t.Fatal("no tap after small jitter")
	}
}

func TestResidualContactAfterMultitouchNeverTaps(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 150, 160)))
	d.Process(f(25, c(1, 150, 160), c(2, 260, 160)))
	d.Process(f(80, c(1, 160, 165)))  // second finger released
	d.Process(f(130, c(1, 180, 168))) // remaining finger moves
	if contains(d.Process(f(170)), Tap) {
		t.Fatal("ghost tap from residual finger")
	}
}

func TestTwoFingerJitterProducesNoTransforms(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 160, 240), c(2, 280, 240)))
	for i := 1; i <= 12; i++ {
		x := i%5 - 2
		ev := d.Process(f(i*20, c(1, 160+x, 240-x), c(2, 280-x, 240+x)))
		if contains(ev, Pinch) || contains(ev, Rotate) || contains(ev, TwoSwipe) {
			t.Fatalf("noisy stationary fingers: %v", ev)
		}
	}
}

func TestSmallTwoFingerSwipeFallsBackOnRelease(t *testing.T) {
	d := New()
	d.Process(f(0, c(1, 100, 150), c(2, 210, 150)))
	ev := d.Process(f(80, c(1, 166, 151), c(2, 276, 151)))
	if contains(ev, TwoSwipe) {
		t.Fatal("early threshold should be higher")
	}
	ev = d.Process(f(100))
	if !contains(ev, TwoSwipe) {
		t.Fatal("release swipe fallback missing")
	}
}

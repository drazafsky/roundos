package gesture

import (
	"math"
	"sort"
	"time"

	"roundos/internal/touch"
)

// Multiple-contact gestures are deliberately strict on this small panel.
// A swipe must be coherent across all three tracking IDs; a pinch, rotation,
// or contact-count transition must never become a system navigation event.
const (
	threeSwipeDistance = 76.0
	fourTapSlop        = 18.0
	fourTapDuration    = 430 * time.Millisecond
)

func (d *Detector) startMany(current map[int]touch.Contact, at time.Time) {
	d.manyCount = len(current)
	d.manyAt = at
	d.manyInitial = make(map[int]point, len(current))
	d.manyLast = make(map[int]point, len(current))
	for id, c := range current {
		d.manyInitial[id] = pos(c)
		d.manyLast[id] = pos(c)
	}
	d.manyFired = false
	d.manyMoved = false
	d.pendingFourTap = false
	d.twoClaimed = true // never fall back into a two-finger system swipe
}

func (d *Detector) processMany(current map[int]touch.Contact, at time.Time) []Event {
	if d.manyCount != len(current) || !sameKeys(d.manyInitial, current) {
		d.startMany(current, at)
		return nil
	}
	var dx, dy float64
	for id, c := range current {
		v := pos(c)
		prev := d.manyInitial[id]
		if distance(v, prev) > fourTapSlop {
			d.manyMoved = true
		}
		d.manyLast[id] = v
		dx += v.x - prev.x
		dy += v.y - prev.y
	}
	dx /= float64(d.manyCount)
	dy /= float64(d.manyCount)
	if d.manyCount != 3 || d.manyFired || math.Hypot(dx, dy) < threeSwipeDistance {
		return nil
	}
	if math.Max(math.Abs(dx), math.Abs(dy)) < 1.45*math.Min(math.Abs(dx), math.Abs(dy)) {
		return nil
	}
	// Coherence, not centroid motion alone: three fingertips must translate
	// in roughly the same direction and distance. Reject pinch/rotation.
	magnitude := math.Hypot(dx, dy)
	for id, v := range d.manyLast {
		p := d.manyInitial[id]
		x, y := v.x-p.x, v.y-p.y
		n := math.Hypot(x, y)
		if n < threeSwipeDistance*.62 || n > magnitude*1.55 || (x*dx+y*dy)/(n*magnitude) < .86 {
			return nil
		}
	}
	d.manyFired = true
	return []Event{{Type: ThreeSwipe, At: at, X: 240, Y: 240, DX: dx, DY: dy, Fingers: 3, Direction: direction(dx, dy)}}
}

func sameKeys(a map[int]point, b map[int]touch.Contact) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

// End-of-sequence decisions must be evaluated after all fingers lift, so
// finger-release ordering cannot generate a phantom four-finger tap.
func (d *Detector) endMany(at time.Time) {
	if d.manyCount == 4 && !d.manyMoved && !d.manyFired && at.Sub(d.manyAt) <= fourTapDuration {
		d.pendingFourTap = true
	}
	d.manyCount = 0
	d.manyInitial = nil
	d.manyLast = nil
}

// Stable ID ordering is useful to consumers of raw multi-contact snapshots.
func SortedIDs(m map[int]touch.Contact) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

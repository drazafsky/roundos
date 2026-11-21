// Package gesture recognizes touch interactions from synchronized contact
// snapshots. No polling loop is required: only a pending long press needs a
// scheduled deadline. The detector must be called from one goroutine.
package gesture

import (
	"math"
	"roundos/internal/touch"
	"time"
)

type Type string

const (
	Down         Type = "down"
	Up           Type = "up"
	Tap          Type = "tap"
	DoubleTap    Type = "double_tap"
	LongPress    Type = "long_press"
	DragStart    Type = "drag_start"
	Drag         Type = "drag"
	DragEnd      Type = "drag_end"
	Swipe        Type = "swipe"
	TwoSwipe     Type = "two_swipe"
	ThreeSwipe   Type = "three_swipe"
	FourTap      Type = "four_tap"
	FiveContacts Type = "five_contacts"
	Pinch        Type = "pinch"
	Rotate       Type = "finger_rotate"
	Cancel       Type = "cancel"
)

type Event struct {
	Type      Type
	At        time.Time
	X, Y      float64
	DX, DY    float64 // displacement from gesture start
	Scale     float64 // two-finger distance / initial distance
	Angle     float64 // rotation in degrees from initial orientation, -180..180
	Fingers   int
	Direction string // left/right/up/down, for swipes
}

const (
	slop                  = 20.0 // small contact jitter must not become a drag
	swipeDistance         = 50.0
	twoSwipeDistance      = 58.0 // release-time fallback
	earlyTwoSwipeDistance = 75.0 // trigger while fingers are still down
	longPressDelay        = 550 * time.Millisecond
	tapMaxDuration        = 500 * time.Millisecond
	doubleTapWindow       = 320 * time.Millisecond
)

type point struct{ x, y float64 }

func pos(c touch.Contact) point   { return point{float64(c.X), float64(c.Y)} }
func distance(a, b point) float64 { return math.Hypot(a.x-b.x, a.y-b.y) }
func center(a, b point) point     { return point{(a.x + b.x) / 2, (a.y + b.y) / 2} }
func direction(dx, dy float64) string {
	if math.Abs(dx) > math.Abs(dy) {
		if dx > 0 {
			return "right"
		}
		return "left"
	}
	if dy > 0 {
		return "down"
	}
	return "up"
}
func angle(a, b point) float64 { return math.Atan2(b.y-a.y, b.x-a.x) * 180 / math.Pi }
func normalizedAngle(a float64) float64 {
	for a > 180 {
		a -= 360
	}
	for a < -180 {
		a += 360
	}
	return a
}

type Detector struct {
	last                                              map[int]touch.Contact
	start                                             point
	startAt                                           time.Time
	primaryID                                         int
	tracking                                          bool
	dragged, held, multi                              bool
	twoIDs                                            [2]int
	twoStart, twoLast                                 point
	twoAStart, twoBStart                              point
	transformCandidateAt                              time.Time
	initialDistance, initialAngle, twoScale, twoAngle float64
	lastTapAt                                         time.Time
	lastTap                                           point
	twoClaimed, twoSwipeFired, waitingForRelease      bool
	lastPinchEmitted, lastRotationEmitted             float64
	manyCount                                         int
	manyAt                                            time.Time
	manyInitial, manyLast                             map[int]point
	manyMoved, manyFired, pendingFourTap, manyBlocked bool
}

func New() *Detector       { return &Detector{} }
func (d *Detector) Reset() { *d = Detector{} }

// CancelUntilRelease discards pending gestures and ignores the rest of an
// active contact sequence. It returns whether contacts still need releasing.
func (d *Detector) CancelUntilRelease() bool {
	active := len(d.last) > 0 || d.waitingForRelease
	d.Reset()
	d.waitingForRelease = active
	return active
}

// Deadline returns the next long-press deadline, or zero when none is pending.
func (d *Detector) Deadline() time.Time {
	if d.tracking && !d.dragged && !d.held && !d.multi {
		return d.startAt.Add(longPressDelay)
	}
	return time.Time{}
}
func (d *Detector) Tick(now time.Time) []Event {
	deadline := d.Deadline()
	if deadline.IsZero() || now.Before(deadline) {
		return nil
	}
	d.held = true
	return []Event{{Type: LongPress, At: now, X: d.start.x, Y: d.start.y, Fingers: 1}}
}
func single(e Type, at time.Time, p point) Event {
	return Event{Type: e, At: at, X: p.x, Y: p.y, Fingers: 1}
}
func orderedPair(m map[int]touch.Contact) (int, int, bool) {
	if len(m) != 2 {
		return 0, 0, false
	}
	ids := [2]int{}
	i := 0
	for id := range m {
		ids[i] = id
		i++
	}
	if ids[0] > ids[1] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	return ids[0], ids[1], true
}

// coherentTwo rejects centroid movement caused by only one fingertip moving.
// Type-B controllers may update the two contacts in separate SYN_REPORT frames.
// Two-finger navigation requires both fingers to travel in the same direction;
// pinches/rotations must not accidentally be routed as system shortcuts.
func (d *Detector) coherentTwo(a, b point) bool {
	ax, ay := a.x-d.twoAStart.x, a.y-d.twoAStart.y
	bx, by := b.x-d.twoBStart.x, b.y-d.twoBStart.y
	am, bm := math.Hypot(ax, ay), math.Hypot(bx, by)
	if am < 18 || bm < 18 || math.Min(am, bm) < math.Max(am, bm)*0.5 {
		return false
	}
	return (ax*bx+ay*by)/(am*bm) >= 0.86
}
func contactsMap(contacts []touch.Contact) map[int]touch.Contact {
	m := make(map[int]touch.Contact, len(contacts))
	for _, c := range contacts {
		m[c.ID] = c
	}
	return m
}
func (d *Detector) Process(f touch.Frame) []Event {
	if f.Canceled {
		d.Reset()
		return []Event{{Type: Cancel, At: f.At}}
	}
	current := contactsMap(f.Contacts)
	events := make([]Event, 0, 4)
	// If one finger lifts after a multi-touch gesture, the remaining finger
	// belongs to the same gesture. Do not turn it into a new tap or press.
	if d.waitingForRelease {
		if len(current) == 0 {
			d.waitingForRelease = false
			if d.pendingFourTap && f.At.Sub(d.manyAt) <= fourTapDuration {
				events = append(events, Event{Type: FourTap, At: f.At, Fingers: 4, X: 240, Y: 240})
			}
			d.pendingFourTap = false
		}
		d.last = current
		return events
	}
	if d.manyBlocked {
		if len(current) == 0 {
			d.manyBlocked = false
			d.multi = false
			d.waitingForRelease = false
		} else if len(current) < 3 {
			d.manyBlocked = false
			d.multi = false
			d.waitingForRelease = true
		}
		d.last = current
		return events
	}
	// A multi-contact sequence terminates as a unit. Any fingers still down
	// are suppressed until their release, rather than being reclassified as
	// two- or one-finger gestures.
	if d.manyCount > 0 && (len(current) < d.manyCount || (len(current) >= 3 && !sameKeys(d.manyInitial, current))) {
		d.endMany(f.At)
		if len(current) < d.manyCount || d.manyCount == 0 {
			d.multi = false
			d.waitingForRelease = len(current) > 0
			d.last = current
			if len(current) == 0 && d.pendingFourTap {
				events = append(events, Event{Type: FourTap, At: f.At, Fingers: 4, X: 240, Y: 240})
				d.pendingFourTap = false
			}
			return events
		}
	}
	if len(current) >= 2 {
		if !d.multi {
			// The second finger cancels an in-progress single-finger
			// interaction; it must not leave an app with a stuck pointer.
			if d.tracking {
				events = append(events, Event{Type: Cancel, At: f.At})
			}
			d.tracking = false
			d.dragged = false
			d.held = false
			d.lastTapAt = time.Time{}
		}
		d.multi = true
		if len(current) >= 3 {
			if d.manyCount == 0 {
				if d.twoClaimed || d.twoSwipeFired {
					d.manyBlocked = true
					d.last = current
					return events
				}
				if d.initialDistance > 0 {
					events = append(events, Event{Type: Cancel, At: f.At})
				}
				d.startMany(current, f.At)
				if len(current) >= 5 {
					events = append(events, Event{Type: FiveContacts, At: f.At, Fingers: len(current), X: 240, Y: 240})
				}
			} else {
				events = append(events, d.processMany(current, f.At)...)
			}
			d.last = current
			return events
		}
		if id1, id2, ok := orderedPair(current); ok {
			a, b := pos(current[id1]), pos(current[id2])
			mid := center(a, b)
			if d.twoIDs != [2]int{id1, id2} || d.initialDistance == 0 {
				d.twoIDs = [2]int{id1, id2}
				d.twoStart = mid
				d.twoLast = mid
				d.twoAStart, d.twoBStart = a, b
				d.transformCandidateAt = time.Time{}
				d.initialDistance = distance(a, b)
				d.initialAngle = angle(a, b)
				d.twoScale = 1
				d.twoAngle = 0
				d.twoClaimed = false
				d.twoSwipeFired = false
				d.lastPinchEmitted = 1
				d.lastRotationEmitted = 0
			} else {
				scale := 1.0
				if d.initialDistance > 1 {
					scale = distance(a, b) / d.initialDistance
				}
				rot := normalizedAngle(angle(a, b) - d.initialAngle)
				d.twoScale = scale
				d.twoAngle = rot
				dx, dy := mid.x-d.twoStart.x, mid.y-d.twoStart.y
				// One tracking slot may be updated earlier than the other on a
				// type-B touchscreen. An isolated 13% separation change is
				// therefore NOT sufficient to claim a pinch forever. Confirm
				// genuine relative motion (both fingertips moving differently),
				// or a sustained one-finger transform before locking it in.
				transform := math.Abs(scale-1) >= 0.13 || math.Abs(rot) >= 15
				if transform && !d.twoClaimed && !d.twoSwipeFired {
					if d.transformCandidateAt.IsZero() {
						d.transformCandidateAt = f.At
					}
					ax, ay := a.x-d.twoAStart.x, a.y-d.twoAStart.y
					bx, by := b.x-d.twoBStart.x, b.y-d.twoBStart.y
					am, bm := math.Hypot(ax, ay), math.Hypot(bx, by)
					// Coherent translation (including slightly staggered input)
					// is not evidence of a pinch or a rotation.
					opposed := am >= 14 && bm >= 14 && (ax*bx+ay*by)/(am*bm) < 0.72
					if opposed || f.At.Sub(d.transformCandidateAt) >= 110*time.Millisecond {
						d.twoClaimed = true
					}
				} else if !transform {
					d.transformCandidateAt = time.Time{}
				}
				if d.twoClaimed && !d.twoSwipeFired {
					if math.Abs(scale-1) >= 0.13 && math.Abs(scale-d.lastPinchEmitted) >= 0.07 {
						events = append(events, Event{Type: Pinch, At: f.At, X: mid.x, Y: mid.y, DX: dx, DY: dy, Scale: scale, Angle: rot, Fingers: 2})
						d.lastPinchEmitted = scale
					}
					if math.Abs(rot) >= 15 && math.Abs(normalizedAngle(rot-d.lastRotationEmitted)) >= 8 {
						events = append(events, Event{Type: Rotate, At: f.At, X: mid.x, Y: mid.y, DX: dx, DY: dy, Scale: scale, Angle: rot, Fingers: 2})
						d.lastRotationEmitted = rot
					}
				}
				d.twoLast = mid
				// Require BOTH tracked fingertips to translate coherently.
				// This also protects against false navigation when one finger
				// is stationary and the other performs a pinch.
				if !d.twoSwipeFired && !d.twoClaimed && math.Hypot(dx, dy) >= earlyTwoSwipeDistance && d.coherentTwo(a, b) &&
					(math.Abs(dx) >= 1.4*math.Abs(dy) || math.Abs(dy) >= 1.4*math.Abs(dx)) {
					events = append(events, Event{Type: TwoSwipe, At: f.At, X: mid.x, Y: mid.y, DX: dx, DY: dy, Fingers: 2, Direction: direction(dx, dy)})
					d.twoSwipeFired = true
				}
			}
		}
		d.last = current
		return events
	}
	if d.multi {
		if d.initialDistance > 0 {
			dx, dy := d.twoLast.x-d.twoStart.x, d.twoLast.y-d.twoStart.y
			// Only call a two-finger movement a swipe if scale and rotation
			// stayed near their initial values. This avoids pinch conflicts.
			id1, id2, ok := orderedPair(d.last)
			coherent := ok && d.coherentTwo(pos(d.last[id1]), pos(d.last[id2]))
			if !d.twoSwipeFired && !d.twoClaimed && coherent && distance(d.twoLast, d.twoStart) >= twoSwipeDistance {
				// The last two-contact coordinates are not present in the
				// release frame, so preserve scale/rotation while processing.
				if math.Abs(d.lastScale()-1) < 0.25 && math.Abs(d.lastAngle()) < 25 {
					events = append(events, Event{Type: TwoSwipe, At: f.At, X: d.twoLast.x, Y: d.twoLast.y, DX: dx, DY: dy, Fingers: 2, Direction: direction(dx, dy)})
				}
			}
		}
		d.tracking = false
		d.dragged = false
		d.held = false
		d.multi = false
		d.twoIDs = [2]int{}
		d.initialDistance = 0
		d.initialAngle = 0
		d.twoScale = 0
		d.twoAngle = 0
		d.twoClaimed = false
		d.twoSwipeFired = false
		d.transformCandidateAt = time.Time{}
		d.waitingForRelease = len(current) != 0
		// A leftover contact remains suppressed until it lifts.
		d.last = current
		return events
	}
	if len(current) == 1 {
		var id int
		var c touch.Contact
		for id, c = range current {
			break
		}
		p := pos(c)
		if !d.tracking || id != d.primaryID {
			d.tracking = true
			d.primaryID = id
			d.start = p
			d.startAt = f.At
			d.dragged = false
			d.held = false
			events = append(events, single(Down, f.At, p))
		} else {
			dx, dy := p.x-d.start.x, p.y-d.start.y
			if !d.held && distance(p, d.start) > slop {
				if !d.dragged {
					d.dragged = true
					events = append(events, Event{Type: DragStart, At: f.At, X: p.x, Y: p.y, DX: dx, DY: dy, Fingers: 1})
				}
				events = append(events, Event{Type: Drag, At: f.At, X: p.x, Y: p.y, DX: dx, DY: dy, Fingers: 1})
			}
		}
	} else if d.tracking {
		p := d.start
		if c, ok := d.last[d.primaryID]; ok {
			p = pos(c)
		}
		dx, dy := p.x-d.start.x, p.y-d.start.y
		if d.dragged {
			events = append(events, Event{Type: DragEnd, At: f.At, X: p.x, Y: p.y, DX: dx, DY: dy, Fingers: 1})
			if distance(p, d.start) >= swipeDistance && f.At.Sub(d.startAt) < 1100*time.Millisecond {
				events = append(events, Event{Type: Swipe, At: f.At, X: p.x, Y: p.y, DX: dx, DY: dy, Fingers: 1, Direction: direction(dx, dy)})
			}
		} else if !d.held && f.At.Sub(d.startAt) <= tapMaxDuration {
			if !d.lastTapAt.IsZero() && f.At.Sub(d.lastTapAt) <= doubleTapWindow && distance(d.start, d.lastTap) < 35 {
				// Preserve immediate tap semantics, including rapid repeated buttons.
				// DoubleTap is an additional signal, not a replacement for Tap.
				events = append(events, single(Tap, f.At, p), single(DoubleTap, f.At, p))
				d.lastTapAt = time.Time{}
			} else {
				events = append(events, single(Tap, f.At, p))
				d.lastTapAt = f.At
				d.lastTap = d.start
			}
		}
		events = append(events, single(Up, f.At, p))
		d.tracking = false
		d.dragged = false
		d.held = false
	}
	d.last = current
	return events
}

// Separate end-of-gesture fields remember the last relative transform.
// They are updated by the wrapper below via the two-contact branch.
func (d *Detector) lastScale() float64 {
	if d.twoScale == 0 {
		return 1
	}
	return d.twoScale
}
func (d *Detector) lastAngle() float64 { return d.twoAngle }

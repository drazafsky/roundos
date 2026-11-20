// Package touch reads Linux evdev type-B multitouch frames from the HyperPixel.
// It keeps all active contacts instead of reducing them to a single pointer.
package touch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"syscall"
	"time"
	"unsafe"
)

const (
	evSyn           = 0
	evKey           = 1
	evAbs           = 3
	synReport       = 0
	synDropped      = 3
	btnTouch        = 330
	absX            = 0
	absY            = 1
	absMTSlot       = 0x2f
	absMTPositionX  = 0x35
	absMTPositionY  = 0x36
	absMTTrackingID = 0x39
	maxSlots        = 16
)

// Contact.ID is the tracking ID assigned by the kernel, not the mutable slot.
type Contact struct {
	ID   int
	Slot int
	X    int
	Y    int
}
type Frame struct {
	At       time.Time
	Contacts []Contact
	Canceled bool
}
type axisInfo struct{ Value, Min, Max, Fuzz, Flat, Resolution int32 }
type slotInfo struct {
	id           int
	haveX, haveY bool
	x, y         int
}

type Decoder struct {
	xRange, yRange axisInfo
	hasMT          bool
	invertY        bool
	slots          [maxSlots]slotInfo
	curSlot        int
	x, y           int
	buttonDown     bool
	dropped        bool
	last           []Contact
}

func newDecoder(x, y axisInfo, mt bool) (*Decoder, error) {
	if x.Max <= x.Min || y.Max <= y.Min {
		return nil, errors.New("invalid touch axis ranges")
	}
	d := &Decoder{xRange: x, yRange: y, hasMT: mt}
	d.reset()
	return d, nil
}
func (d *Decoder) reset() {
	for i := range d.slots {
		d.slots[i] = slotInfo{id: -1}
	}
	d.curSlot = 0
	d.last = nil
	d.buttonDown = false
}
func scale(v int, a axisInfo) int {
	n := int(math.Round(float64(v-int(a.Min)) * 479 / float64(a.Max-a.Min)))
	if n < 0 {
		return 0
	}
	if n > 479 {
		return 479
	}
	return n
}

// normalizedY optionally mirrors the physical touch Y coordinate. The tested
// HyperPixel Round installation has correct tap positions with inversion off.
// This must not be used to fix only a two-finger shortcut direction, because
// it also mirrors hit testing and one-finger edge gestures.
func (d *Decoder) normalizedY(raw int) int {
	y := scale(raw, d.yRange)
	if d.invertY {
		return 479 - y
	}
	return y
}

func (d *Decoder) snapshot() []Contact {
	if !d.hasMT {
		if !d.buttonDown {
			return nil
		}
		return []Contact{{ID: 0, Slot: 0, X: scale(d.x, d.xRange), Y: d.normalizedY(d.y)}}
	}
	contacts := make([]Contact, 0, 5)
	for i, p := range d.slots {
		if p.id >= 0 && p.haveX && p.haveY {
			contacts = append(contacts, Contact{ID: p.id, Slot: i, X: scale(p.x, d.xRange), Y: d.normalizedY(p.y)})
		}
	}
	if len(contacts) == 0 {
		return nil
	}
	return contacts
}

// Feed consumes a raw evdev event and returns a complete contact snapshot only
// on SYN_REPORT. SYN_DROPPED produces an immediate cancel and discards stale
// state; subsequent contacts require fresh tracking IDs. Kernel slot recovery
// via EVIOCGMTSLOTS can be added if persistent lost-event recovery is needed.
func (d *Decoder) Feed(typ, code uint16, value int, at time.Time) (Frame, bool) {
	if typ == evSyn && code == synDropped {
		d.reset()
		d.dropped = true
		return Frame{At: at, Canceled: true}, true
	}
	if d.dropped {
		if typ == evSyn && code == synReport {
			d.dropped = false
		}
		return Frame{}, false
	}
	switch typ {
	case evAbs:
		if d.hasMT {
			switch code {
			case absMTSlot:
				if value >= 0 && value < maxSlots {
					d.curSlot = value
				} else {
					d.curSlot = -1
				}
			case absMTTrackingID:
				if d.curSlot >= 0 {
					p := &d.slots[d.curSlot]
					if value < 0 {
						*p = slotInfo{id: -1}
					} else {
						*p = slotInfo{id: value}
					}
				}
			case absMTPositionX:
				if d.curSlot >= 0 {
					p := &d.slots[d.curSlot]
					p.x = value
					p.haveX = true
				}
			case absMTPositionY:
				if d.curSlot >= 0 {
					p := &d.slots[d.curSlot]
					p.y = value
					p.haveY = true
				}
			}
		} else {
			if code == absX {
				d.x = value
			} else if code == absY {
				d.y = value
			}
		}
	case evKey:
		if !d.hasMT && code == btnTouch {
			d.buttonDown = value != 0
		}
	case evSyn:
		if code == synReport {
			now := d.snapshot()
			if reflect.DeepEqual(now, d.last) {
				return Frame{}, false
			}
			d.last = now
			return Frame{At: at, Contacts: now}, true
		}
	}
	return Frame{}, false
}

func absInfo(fd uintptr, axis int) (axisInfo, error) {
	var i axisInfo
	req := uintptr((uint32(2) << 30) | (uint32(unsafe.Sizeof(i)) << 16) | (uint32('E') << 8) | uint32(0x40+axis))
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(&i)))
	if errno != 0 {
		return i, errno
	}
	return i, nil
}

type Reader struct {
	f       *os.File
	decoder *Decoder
}

// Open preserves the historical axis convention for callers that already
// provide correctly oriented input coordinates.
func Open(path string) (*Reader, error) { return OpenWithOrientation(path, false) }

// OpenWithOrientation optionally mirrors Y for panels whose touch coordinates
// are opposite their display coordinates. The tested HyperPixel panel uses false.
func OpenWithOrientation(path string, invertY bool) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	x, xe := absInfo(f.Fd(), absMTPositionX)
	y, ye := absInfo(f.Fd(), absMTPositionY)
	mt := xe == nil && ye == nil
	if !mt {
		x, err = absInfo(f.Fd(), absX)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("touch device lacks X axis: %w", err)
		}
		y, err = absInfo(f.Fd(), absY)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("touch device lacks Y axis: %w", err)
		}
	}
	d, err := newDecoder(x, y, mt)
	if err != nil {
		f.Close()
		return nil, err
	}
	d.invertY = invertY
	return &Reader{f: f, decoder: d}, nil
}
func (r *Reader) Description() string {
	d := r.decoder
	return fmt.Sprintf("touch axes X=%d..%d Y=%d..%d, multitouch=%t, invert_y=%t", d.xRange.Min, d.xRange.Max, d.yRange.Min, d.yRange.Max, d.hasMT, d.invertY)
}
func (r *Reader) Close() error { return r.f.Close() }
func (r *Reader) Run(ctx context.Context, out chan<- Frame) error {
	// input_event: native timeval, uint16 type, uint16 code, int32 value.
	size := int(unsafe.Sizeof(syscall.Timeval{})) + 8
	if size != 16 && size != 24 {
		return fmt.Errorf("unsupported evdev event size %d", size)
	}
	b := make([]byte, size)
	for {
		if ctx.Err() != nil {
			return nil
		}
		_, err := io.ReadFull(r.f, b)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		o := size - 8
		typ := binary.LittleEndian.Uint16(b[o : o+2])
		code := binary.LittleEndian.Uint16(b[o+2 : o+4])
		value := int(int32(binary.LittleEndian.Uint32(b[o+4 : o+8])))
		frame, ok := r.decoder.Feed(typ, code, value, time.Now())
		if ok {
			select {
			case out <- frame:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

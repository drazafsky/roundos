// Package rotary maps Linux rotary-encoder and gpio-keys evdev events into
// RoundOS navigation. No GPIO ownership or kernel driver changes are required.
package rotary

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	evSyn     = 0
	evKey     = 1
	evRel     = 2
	synReport = 0
)
const (
	KeyEnter = 28
	KeyUp    = 103
	KeyDown  = 108
	RelDial  = 7
	RelWheel = 8
)

type Config struct {
	RelCode  int
	UpKey    int
	DownKey  int
	PressKey int
	Reverse  bool
}

func DefaultConfig() Config {
	return Config{RelCode: RelDial, UpKey: KeyUp, DownKey: KeyDown, PressKey: KeyEnter}
}

type Event struct {
	At    time.Time
	Kind  string
	Delta int
}
type Decoder struct {
	cfg     Config
	delta   int
	pressed bool
}

func NewDecoder(cfg Config) *Decoder { return &Decoder{cfg: cfg} }
func (d *Decoder) Feed(typ, code uint16, value int) []Event {
	switch typ {
	case evRel:
		if int(code) == d.cfg.RelCode {
			d.delta += value
		}
	case evKey:
		if value == 2 {
			return nil
		} // auto-repeat must not turn one click into multiple clicks
		if int(code) == d.cfg.PressKey {
			if value == 1 && !d.pressed {
				d.pressed = true
				return []Event{{Kind: "button_down"}}
			}
			if value == 0 && d.pressed {
				d.pressed = false
				return []Event{{Kind: "button_up"}}
			}
		}
		if value == 1 && int(code) == d.cfg.UpKey {
			d.delta--
		}
		if value == 1 && int(code) == d.cfg.DownKey {
			d.delta++
		}
	case evSyn:
		if code == synReport && d.delta != 0 {
			n := d.delta
			d.delta = 0
			if d.cfg.Reverse {
				n = -n
			}
			return []Event{{Kind: "rotate", Delta: n}}
		}
	}
	return nil
}

type Reader struct {
	f   *os.File
	dec *Decoder
}

func Open(path string, cfg Config) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &Reader{f: f, dec: NewDecoder(cfg)}, nil
}
func (r *Reader) Close() error { return r.f.Close() }
func (r *Reader) Run(ctx context.Context, out chan<- Event) error {
	size := int(unsafe.Sizeof(syscall.Timeval{})) + 8
	if size != 16 && size != 24 {
		return fmt.Errorf("unsupported input_event size %d", size)
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
		offset := size - 8
		typ := binary.LittleEndian.Uint16(b[offset : offset+2])
		code := binary.LittleEndian.Uint16(b[offset+2 : offset+4])
		value := int(int32(binary.LittleEndian.Uint32(b[offset+4 : offset+8])))
		for _, ev := range r.dec.Feed(typ, code, value) {
			ev.At = time.Now()
			select {
			case out <- ev:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

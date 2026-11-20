// Package wire defines RoundOS 0.4's bounded, length-prefixed IPC protocol.
// JSON is intentionally an interim prototype encoding; transport and versioning allow
// a subsequent transition to generated Protobuf without changing app lifecycle.
package wire

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const Version = 7
const MaxMessage = 64 << 10
const ClockID = "org.roundos.clock"
const TimerID = "org.roundos.timer"

type View struct {
	Title        string   `json:"title,omitempty"`
	Primary      string   `json:"primary,omitempty"`
	Secondary    string   `json:"secondary,omitempty"`
	Hint         string   `json:"hint,omitempty"`
	ButtonLabel  string   `json:"button_label,omitempty"`
	ButtonAction string   `json:"button_action,omitempty"`
	RotaryAction string   `json:"rotary_action,omitempty"`
	Screens      []Screen `json:"screens,omitempty"`
	Dialog       *Dialog  `json:"dialog,omitempty"`
}

// Gesture is a high-level touch event. Values are in physical framebuffer
// coordinates, scale is relative to the beginning of a two-finger gesture.
// Consumers must tolerate unknown event names for forward compatibility.
type Gesture struct {
	Kind      string  `json:"kind"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	DX        float64 `json:"dx,omitempty"`
	DY        float64 `json:"dy,omitempty"`
	Scale     float64 `json:"scale,omitempty"`
	Angle     float64 `json:"angle,omitempty"`
	Fingers   int     `json:"fingers,omitempty"`
	Direction string  `json:"direction,omitempty"`
}

// App notifications and optional raw contact events use the same authenticated
// bounded transport as view/gesture updates. Subscription is opt-in.
type Notification struct {
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
	Priority string `json:"priority,omitempty"`
	Group    string `json:"group,omitempty"`
	Action   string `json:"action,omitempty"`
}
type Contact struct {
	ID   int `json:"id"`
	Slot int `json:"slot"`
	X    int `json:"x"`
	Y    int `json:"y"`
}

// TextRequest asks the shell to open its accessible full-screen editor.
// The request ID is application-chosen and echoed with the result.
type TextRequest struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Initial string `json:"initial,omitempty"`
	Secure  bool   `json:"secure,omitempty"`
}

// A canceled response never includes the draft value.
type TextResult struct {
	ID       string `json:"id"`
	Value    string `json:"value,omitempty"`
	Canceled bool   `json:"canceled,omitempty"`
}

type Message struct {
	Version      int           `json:"version"`
	Type         string        `json:"type"` // hello, visibility, view, input, close
	App          string        `json:"app,omitempty"`
	Token        string        `json:"token,omitempty"`
	Visible      bool          `json:"visible,omitempty"`
	Action       string        `json:"action,omitempty"`
	Delta        int           `json:"delta,omitempty"`
	View         *View         `json:"view,omitempty"`
	Patch        *ViewPatch    `json:"patch,omitempty"`
	Gesture      *Gesture      `json:"gesture,omitempty"`
	Notification *Notification `json:"notification,omitempty"`
	TextRequest  *TextRequest  `json:"text_request,omitempty"`
	TextResult   *TextResult   `json:"text_result,omitempty"`
	RawContacts  bool          `json:"raw_contacts,omitempty"`
	Contacts     []Contact     `json:"contacts,omitempty"`
}

func Write(w io.Writer, msg Message) error {
	if msg.Version != Version {
		return fmt.Errorf("invalid protocol version %d", msg.Version)
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if len(b) == 0 || len(b) > MaxMessage {
		return fmt.Errorf("message exceeds %d bytes", MaxMessage)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
	if err := writeAll(w, lenBuf[:]); err != nil {
		return err
	}
	return writeAll(w, b)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func Read(r io.Reader) (Message, error) {
	var msg Message
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return msg, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 || n > MaxMessage {
		return msg, errors.New("invalid IPC frame size")
	}
	b := make([]byte, int(n))
	if _, err := io.ReadFull(r, b); err != nil {
		return msg, err
	}
	if err := json.Unmarshal(b, &msg); err != nil {
		return msg, err
	}
	if msg.Version != Version {
		return msg, fmt.Errorf("unsupported protocol version %d", msg.Version)
	}
	return msg, nil
}

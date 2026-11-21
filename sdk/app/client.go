// Package app is the minimal RoundOS SDK transport for independently executed apps.
package app

import (
	"errors"
	"net"
	"os"
	"reflect"
	"sync"
	"time"

	"roundos/internal/wire"
	sdkgesture "roundos/sdk/gesture"
)

// Event is the public application-side input/lifecycle event. Existing apps
// continue to inspect Type, Visible, Action and Delta as before.
type Event struct {
	Type       string
	Visible    bool
	Action     string
	Delta      int
	Gesture    *sdkgesture.Event
	Contacts   []Contact
	TextResult *wire.TextResult
}

// Contact contains the Linux multitouch tracking ID and 480x480 coordinates.
type Contact = wire.Contact

func fromWire(m wire.Message) Event {
	e := Event{Type: m.Type, Visible: m.Visible, Action: m.Action, Delta: m.Delta}
	if g := m.Gesture; g != nil {
		e.Gesture = &sdkgesture.Event{Type: g.Kind, X: g.X, Y: g.Y, DX: g.DX, DY: g.DY, Scale: g.Scale, Angle: g.Angle, Fingers: g.Fingers, Direction: g.Direction}
	}
	if len(m.Contacts) > 0 {
		e.Contacts = append([]Contact(nil), m.Contacts...)
	}
	if m.TextResult != nil {
		result := *m.TextResult
		e.TextResult = &result
	}
	return e
}

type Client struct {
	conn         net.Conn
	mu           sync.Mutex
	events       chan Event
	done         chan struct{}
	once         sync.Once
	last         *wire.View
	secureEditor string
}

func Connect() (*Client, error) {
	path, id, token := os.Getenv("ROUNDOS_SOCKET"), os.Getenv("ROUNDOS_APP_ID"), os.Getenv("ROUNDOS_TOKEN")
	if path == "" || id == "" || token == "" {
		return nil, errors.New("missing ROUNDOS_SOCKET, ROUNDOS_APP_ID, or ROUNDOS_TOKEN")
	}
	deadline := time.Now().Add(4 * time.Second)
	var conn net.Conn
	var err error
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("unix", path, 300*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	if err = wire.Write(conn, wire.Message{Version: wire.Version, Type: "hello", App: id, Token: token}); err != nil {
		conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, events: make(chan Event, 64), done: make(chan struct{})}
	go c.readLoop()
	return c, nil
}
func (c *Client) readLoop() {
	defer close(c.events)
	defer close(c.done)
	for {
		msg, err := wire.Read(c.conn)
		if err != nil {
			return
		}
		if msg.Type != "visibility" && msg.Type != "input" && msg.Type != "gesture" && msg.Type != "close" && msg.Type != "contacts" && msg.Type != "text_result" {
			continue
		}
		if msg.Type == "text_result" && msg.TextResult != nil {
			c.mu.Lock()
			if c.secureEditor == msg.TextResult.ID {
				c.secureEditor = ""
			}
			c.mu.Unlock()
		}
		select {
		case c.events <- fromWire(msg):
		case <-time.After(2 * time.Second):
			return
		}
	}
}
func (c *Client) Events() <-chan Event { return c.events }

// Publish transmits a full view initially and property patches thereafter.
// Publishing an identical view does not wake the shell or request a repaint.
func (c *Client) Publish(view wire.View) error {
	if err := wire.ValidateView(view); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := wire.Message{Version: wire.Version, Type: "view", View: &view}
	if c.last != nil {
		if !reflect.DeepEqual(c.last.Screens, view.Screens) || !reflect.DeepEqual(c.last.Dialog, view.Dialog) {
			// Screens are full immutable snapshots: never apply scalar patches to a tree.
			msg.Type = "view"
			msg.View = &view
		} else {
			patch, changed := wire.DiffView(*c.last, view)
			if changed == 0 {
				return nil
			}
			if changed <= 3 {
				msg.Type = "view_patch"
				msg.View = nil
				msg.Patch = &patch
			}
		}
	}
	if err := wire.Write(c.conn, msg); err != nil {
		return err
	}
	v := wire.CopyView(view)
	c.last = &v
	return nil
}

// Notify requests a shell-managed notification. Apps cannot create critical
// system alerts; the shell applies user preferences and priority limits.
func (c *Client) Notify(title, body, priority, group, action string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return wire.Write(c.conn, wire.Message{Version: wire.Version, Type: "notify", Notification: &wire.Notification{Title: title, Body: body, Priority: priority, Group: group, Action: action}})
}

// SubscribeContacts opts into raw multi-contact snapshots. When an app cannot
// keep up, snapshots can be coalesced or dropped; gestures remain independent.
func (c *Client) SubscribeContacts(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return wire.Write(c.conn, wire.Message{Version: wire.Version, Type: "subscribe_contacts", RawContacts: enabled})
}

// RequestText opens a shell-owned editor for this foreground application.
// The eventual Event has Type="text_result" and a TextResult containing
// either Value or Canceled=true. Never use this to persist secret drafts.
func (c *Client) RequestText(id, label, initial string, secure bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	err := wire.Write(c.conn, wire.Message{Version: wire.Version, Type: "text_request", TextRequest: &wire.TextRequest{ID: id, Label: label, Initial: initial, Secure: secure}})
	if err == nil && secure {
		c.secureEditor = id
	}
	return err
}

// SecureEditorActive lets development tooling defer layout replacement without
// reading or exposing any editor value. Only a matching result clears it.
func (c *Client) SecureEditorActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.secureEditor != ""
}
func (c *Client) Close() error { c.once.Do(func() { c.conn.Close() }); return nil }

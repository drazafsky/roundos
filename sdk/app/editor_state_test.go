package app

import (
	"net"
	"testing"
	"time"

	"roundos/internal/wire"
)

func TestSecureEditorStateClearsOnlyForItsOwnResult(t *testing.T) {
	clientConn, peer := net.Pipe()
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(3 * time.Second))
	c := &Client{conn: clientConn, events: make(chan Event, 4), done: make(chan struct{})}
	defer c.Close()
	go c.readLoop()
	requestSeen := make(chan error, 1)
	resume := make(chan struct{})
	matched := make(chan struct{})
	go func() {
		_, err := wire.Read(peer)
		requestSeen <- err
		<-resume
		wire.Write(peer, wire.Message{Version: wire.Version, Type: "text_result", TextResult: &wire.TextResult{ID: "other"}})
		<-matched
		wire.Write(peer, wire.Message{Version: wire.Version, Type: "text_result", TextResult: &wire.TextResult{ID: "secret", Canceled: true}})
	}()
	if err := c.RequestText("secret", "Password", "", true); err != nil {
		t.Fatal(err)
	}
	if err := <-requestSeen; err != nil {
		t.Fatal(err)
	}
	if !c.SecureEditorActive() {
		t.Fatal("secure request was not tracked")
	}
	close(resume)
	select {
	case <-c.Events():
	case <-time.After(3 * time.Second):
		t.Fatal("missing unmatched result")
	}
	if !c.SecureEditorActive() {
		t.Fatal("unmatched result cleared secure editing")
	}
	close(matched)
	for range 1 {
		select {
		case <-c.Events():
		case <-time.After(3 * time.Second):
			t.Fatal("missing editor result")
		}
	}
	if c.SecureEditorActive() {
		t.Fatal("matching canceled result did not clear editing state")
	}
}

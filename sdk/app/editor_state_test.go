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
	if err := c.RequestText("other", "Other", "", true); err == nil {
		t.Fatal("overlapping secure request accepted")
	}
	if err := c.RequestText("secret", "Other", "", false); err == nil {
		t.Fatal("overlapping nonsecure request with reused ID accepted")
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

func TestNonsecureRequestIDCannotBeReusedForSecureEditor(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(3 * time.Second))
	c := &Client{conn: conn, events: make(chan Event, 4), done: make(chan struct{})}
	defer c.Close()
	go c.readLoop()
	seen := make(chan error, 1)
	go func() { _, err := wire.Read(peer); seen <- err }()
	if err := c.RequestText("shared", "Title", "", false); err != nil {
		t.Fatal(err)
	}
	if err := <-seen; err != nil {
		t.Fatal(err)
	}
	if !c.TextEditorActive() {
		t.Fatal("nonsecure editor was not tracked for reload deferral")
	}
	if err := c.RequestText("shared", "Password", "", true); err == nil {
		t.Fatal("outstanding nonsecure ID reused for secure editor")
	}
	if c.SecureEditorActive() {
		t.Fatal("rejected secure request changed editor state")
	}
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "text_result", TextResult: &wire.TextResult{ID: "shared", Canceled: true}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Events():
	case <-time.After(3 * time.Second):
		t.Fatal("prior result missing")
	}
	if c.TextEditorActive() {
		t.Fatal("completed nonsecure request still blocks reload")
	}
	go func() { _, err := wire.Read(peer); seen <- err }()
	if err := c.RequestText("shared", "Password", "", true); err != nil {
		t.Fatal("ID not reusable after prior result", err)
	}
	if err := <-seen; err != nil {
		t.Fatal(err)
	}
	if !c.SecureEditorActive() {
		t.Fatal("secure request not tracked after safe reuse")
	}
}

func TestTrackedEditorResultBlocksReloadUntilApplicationDispatch(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(3 * time.Second))
	c := &Client{conn: conn, events: make(chan Event, 4), done: make(chan struct{})}
	defer c.Close()
	c.TrackEditorResults()
	go c.readLoop()
	seen := make(chan error, 1)
	go func() { _, err := wire.Read(peer); seen <- err }()
	if err := c.RequestText("title", "Title", "", false); err != nil {
		t.Fatal(err)
	}
	if err := <-seen; err != nil {
		t.Fatal(err)
	}
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "text_result", TextResult: &wire.TextResult{ID: "title", Value: "Changed"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Events():
	case <-time.After(time.Second):
		t.Fatal("result not delivered")
	}
	if !c.TextEditorActive() {
		t.Fatal("queued result no longer protects layout context")
	}
	if err := c.RequestText("title", "Title", "", false); err == nil {
		t.Fatal("unacknowledged editor ID reused")
	}
	c.AcknowledgeTextResult("title")
	if c.TextEditorActive() {
		t.Fatal("dispatched result still blocks reload")
	}
}

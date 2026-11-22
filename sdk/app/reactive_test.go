package app

import (
	"context"
	"net"
	"testing"
	"time"

	"roundos/internal/wire"
)

func reactiveClient(t *testing.T, caps []string) (*Client, net.Conn) {
	t.Helper()
	conn, peer := net.Pipe()
	c := &Client{conn: conn, events: make(chan Event, 16), done: make(chan struct{}), selected: true, capabilities: caps}
	go c.readLoop()
	t.Cleanup(func() { c.Close(); peer.Close(); <-c.done })
	return c, peer
}
func appView(value string) wire.View {
	return wire.View{Screens: []wire.Screen{{ID: "root", Title: "Timer", Hero: value, Widgets: []wire.Widget{{ID: "button", Kind: "button", Label: "Start", Action: "start"}}}}}
}
func readAppMessage(t *testing.T, peer net.Conn) wire.Message {
	t.Helper()
	peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := wire.Read(peer)
	peer.SetReadDeadline(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}
func publishAndRead(t *testing.T, c *Client, peer net.Conn, view wire.View) wire.Message {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- c.Publish(view) }()
	msg := readAppMessage(t, peer)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return msg
}
func ack(t *testing.T, peer net.Conn, msg wire.Message) {
	t.Helper()
	peer.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "view_ack", LayoutRevision: msg.LayoutRevision, NewRevision: msg.NewRevision}); err != nil {
		t.Fatal(err)
	}
	peer.SetWriteDeadline(time.Time{})
}
func noAppMessage(t *testing.T, peer net.Conn) {
	t.Helper()
	peer.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	_, err := wire.Read(peer)
	peer.SetReadDeadline(time.Time{})
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("unexpected message or disconnect: %v", err)
	}
}
func TestReactiveCoalescingAndResync(t *testing.T) {
	c, peer := reactiveClient(t, wire.Capabilities())
	first := publishAndRead(t, c, peer, appView("10"))
	if first.Type != "view" || first.LayoutRevision != 1 || first.NewRevision != 1 {
		t.Fatal(first)
	}
	if err := c.Publish(appView("09")); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(appView("08")); err != nil {
		t.Fatal(err)
	}
	noAppMessage(t, peer)
	ack(t, peer, first)
	patch := readAppMessage(t, peer)
	if patch.Type != "widget_patch" || patch.LayoutRevision != 1 || patch.BaseRevision != 1 || patch.NewRevision != 2 || len(patch.Updates) != 1 || *patch.Updates[0].String != "08" {
		t.Fatal(patch)
	}
	ack(t, peer, patch)
	if err := c.Publish(appView("08")); err != nil {
		t.Fatal(err)
	}
	noAppMessage(t, peer)
	changed := appView("07")
	changed.Screens[0].Widgets[0].Action = "pause"
	full := publishAndRead(t, c, peer, changed)
	if full.Type != "view" || full.LayoutRevision != 2 {
		t.Fatal(full)
	}
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "resync_required"}); err != nil {
		t.Fatal(err)
	}
	resync := readAppMessage(t, peer)
	if resync.Type != "view" || resync.LayoutRevision != 3 || resync.View.Screens[0].Widgets[0].Action != "pause" {
		t.Fatal(resync)
	}
	ack(t, peer, first) // late acknowledgement must not undo the resync
	ack(t, peer, resync)
	noAppMessage(t, peer)
}
func TestLegacyFallbackSendsNoNewMessageTypes(t *testing.T) {
	c, peer := reactiveClient(t, nil)
	first := publishAndRead(t, c, peer, appView("10"))
	second := publishAndRead(t, c, peer, appView("09"))
	if first.Type != "view" || second.Type != "view" || first.LayoutRevision != 0 || second.LayoutRevision != 0 {
		t.Fatal(first, second)
	}
	if err := c.Publish(appView("09")); err != nil {
		t.Fatal(err)
	}
	noAppMessage(t, peer)
}
func TestNegotiationResendsPreviouslyPublishedLegacyView(t *testing.T) {
	c, peer := reactiveClient(t, nil)
	c.mu.Lock()
	c.selected = false
	c.mu.Unlock()
	first := publishAndRead(t, c, peer, appView("10"))
	if first.LayoutRevision != 0 {
		t.Fatal(first)
	}
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "visibility", Capabilities: wire.Capabilities()}); err != nil {
		t.Fatal(err)
	}
	full := readAppMessage(t, peer)
	if full.Type != "view" || full.LayoutRevision != 1 || full.View.Screens[0].Hero != "10" {
		t.Fatal(full)
	}
	ack(t, peer, full)
}
func TestMissingAcknowledgementClosesBoundedConnection(t *testing.T) {
	c, peer := reactiveClient(t, wire.Capabilities())
	publishAndRead(t, c, peer, appView("10"))
	if err := c.Publish(appView("09")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
	case <-time.After(4 * time.Second):
		t.Fatal("unacknowledged update did not time out")
	}
}

func TestDialogClearReopenSurvivesCoalescingAndResync(t *testing.T) {
	c, peer := reactiveClient(t, wire.Capabilities())
	view := appView("10")
	view.Dialog = &wire.Dialog{ID: "confirm", Title: "Confirm", Cancel: "No", Confirm: "Yes", CancelAction: "cancel", ConfirmAction: "confirm"}
	first := publishAndRead(t, c, peer, view)
	cleared := wire.CopyView(view)
	cleared.Dialog = nil
	if err := c.Publish(cleared); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish(view); err != nil {
		t.Fatal(err)
	}
	ack(t, peer, first)
	clearMessage := readAppMessage(t, peer)
	if clearMessage.Type != "view" || clearMessage.View.Dialog != nil {
		t.Fatal("dialog clear was coalesced away", clearMessage)
	}
	// Even a rejected clear must be restored before the pending reopen.
	if err := wire.Write(peer, wire.Message{Version: wire.Version, Type: "resync_required"}); err != nil {
		t.Fatal(err)
	}
	clearMessage = readAppMessage(t, peer)
	if clearMessage.View == nil || clearMessage.View.Dialog != nil {
		t.Fatal("resync lost clear barrier")
	}
	ack(t, peer, clearMessage)
	reopen := readAppMessage(t, peer)
	if reopen.View == nil || reopen.View.Dialog == nil || reopen.View.Dialog.ID != "confirm" {
		t.Fatal("dialog did not reopen", reopen)
	}
	ack(t, peer, reopen)
}

func TestConfirmedPublicationWaitsForItsOwnAcknowledgement(t *testing.T) {
	c, peer := reactiveClient(t, wire.Capabilities())
	view := appView("10")
	first := publishAndRead(t, c, peer, view)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { done <- c.PublishConfirmed(ctx, view) }()
	// Synchronize with registration of the confirmation before the old ack.
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.Lock()
		registered := c.confirmed != nil
		c.mu.Unlock()
		if registered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("confirmation did not start")
		}
		time.Sleep(time.Millisecond)
	}
	ack(t, peer, first)
	confirmed := readAppMessage(t, peer)
	select {
	case err := <-done:
		t.Fatal("old ack completed new confirmation", err)
	default:
	}
	if confirmed.Type != "view" || confirmed.LayoutRevision <= first.LayoutRevision {
		t.Fatal(confirmed)
	}
	ack(t, peer, confirmed)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("matching ack did not complete publication")
	}
}

func TestTemplateMetadataRequiresPeerSelection(t *testing.T) {
	for _, caps := range [][]string{nil, {wire.WidgetPatchV1}, wire.Capabilities()} {
		c, peer := reactiveClient(t, caps)
		view := appView("10")
		view.Screens[0].Layout = "form"
		view.Screens[0].Widgets[0].Group = "actions"
		msg := publishAndRead(t, c, peer, view)
		want := wire.HasCapability(caps, wire.TemplatesV1)
		if wire.UsesTemplates(*msg.View) != want || msg.View.Screens[0].Widgets[0].Action != "start" {
			t.Fatal("template fallback changed semantics or skipped negotiation", msg)
		}
		if msg.LayoutRevision != 0 {
			ack(t, peer, msg)
		}
	}
}

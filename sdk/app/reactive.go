package app

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"roundos/internal/wire"
)

const (
	WidgetPatchV1 = wire.WidgetPatchV1
	UIEventV1     = wire.UIEventV1
	TemplatesV1   = wire.TemplatesV1
)

func (c *Client) Supports(capability string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return wire.HasCapability(c.capabilities, capability)
}

func (c *Client) LayoutRevision() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.layout
}

func (c *Client) reactiveLocked() bool {
	return wire.HasCapability(c.capabilities, wire.WidgetPatchV1) || wire.HasCapability(c.capabilities, wire.UIEventV1)
}

func (c *Client) writeLocked(msg wire.Message) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	defer c.conn.SetWriteDeadline(time.Time{})
	return wire.Write(c.conn, msg)
}

func (c *Client) validateDesiredLocked(view wire.View) error {
	view = c.transportViewLocked(view)
	msg := wire.Message{Version: wire.Version, Type: "view", View: &view}
	if c.reactiveLocked() {
		msg.LayoutRevision = c.layout + 1
		msg.NewRevision = 1
	}
	if !wire.MessageFits(msg) {
		return fmt.Errorf("snapshot exceeds IPC frame")
	}
	return nil
}

func (c *Client) transportViewLocked(view wire.View) wire.View {
	if !wire.HasCapability(c.capabilities, wire.TemplatesV1) {
		return wire.WithoutTemplates(view)
	}
	return view
}

func (c *Client) setDesiredLocked(view wire.View) {
	v := wire.CopyView(view)
	// A dialog-free transition is a semantic barrier: coalescing a quick
	// clear/reopen into the original dialog would leave it dismissed.
	if c.pending && c.sent != nil && c.sent.Dialog != nil && v.Dialog == nil {
		transition := wire.CopyView(v)
		c.transition = &transition
	}
	c.last = &v
}

// PublishConfirmed waits for a negotiated acknowledgement. Development layout
// replacement uses this barrier before activating its new action metadata.
// An ambiguous failure closes the connection, preventing input against a tree
// whose acknowledgement was lost. Legacy peers retain synchronous v7 writes.
func (c *Client) PublishConfirmed(ctx context.Context, view wire.View) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := wire.ValidateView(view); err != nil {
		return err
	}
	c.mu.Lock()
	if c.confirmed != nil {
		c.mu.Unlock()
		return fmt.Errorf("another confirmed publication is pending")
	}
	if err := c.validateDesiredLocked(view); err != nil {
		c.mu.Unlock()
		return err
	}
	c.setDesiredLocked(view)
	if !c.reactiveLocked() {
		err := c.publishLegacyLocked(view)
		c.mu.Unlock()
		return err
	}
	confirmed := wire.CopyView(view)
	c.confirmed = &confirmed
	c.confirmation = make(chan struct{})
	done := c.confirmation
	err := c.publishNextLocked(true)
	c.mu.Unlock()
	if err != nil {
		c.Close()
		return err
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		select {
		case <-done:
			return nil
		default:
		}
		c.Close()
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("connection closed before publication acknowledgement")
	}
}

// PublishAuthoritative also publishes unchanged state, undoing rejected shell
// optimism. If an update is in flight, only the latest requested view is kept.
func (c *Client) PublishAuthoritative(view wire.View) error {
	if err := wire.ValidateView(view); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.validateDesiredLocked(view); err != nil {
		return err
	}
	c.setDesiredLocked(view)
	if !c.reactiveLocked() {
		c.legacy = nil
		return c.publishLegacyLocked(view)
	}
	return c.publishNextLocked(true)
}

func (c *Client) publishNextLocked(force bool) error {
	c.forcePending = c.forcePending || force
	if c.last == nil {
		return nil
	}
	if c.pending {
		return nil
	}
	view := wire.CopyView(*c.last)
	transition := c.transition != nil
	if transition {
		view = wire.CopyView(*c.transition)
	} else if c.confirmed != nil {
		view = wire.CopyView(*c.confirmed)
	}
	view = c.transportViewLocked(view)
	if !c.forcePending && c.acked != nil && reflect.DeepEqual(*c.acked, view) {
		if transition {
			c.transition = nil
			return c.publishNextLocked(false)
		}
		return nil
	}
	if c.layout == ^uint64(0) || c.revision == ^uint64(0) {
		return fmt.Errorf("reactive revision exhausted")
	}
	msg := wire.Message{Version: wire.Version, Type: "view", View: &view, LayoutRevision: c.layout + 1, NewRevision: 1}
	if !c.forcePending && c.acked != nil && wire.HasCapability(c.capabilities, wire.WidgetPatchV1) {
		if updates, ok := wire.DiffProperties(*c.acked, view); ok && len(updates) > 0 && len(updates) <= wire.MaxPropertyUpdates {
			patch := wire.Message{Version: wire.Version, Type: "widget_patch", LayoutRevision: c.layout, BaseRevision: c.revision, NewRevision: c.revision + 1, Updates: updates}
			if wire.MessageFits(patch) {
				msg = patch
			}
		}
	}
	if !wire.MessageFits(msg) {
		return fmt.Errorf("reactive snapshot exceeds IPC frame")
	}
	if err := c.writeLocked(msg); err != nil {
		return err
	}
	c.layout, c.revision = msg.LayoutRevision, msg.NewRevision
	c.sent = &view
	c.sentTransition = transition
	c.sentConfirmed = !transition && c.confirmed != nil
	if transition {
		c.transition = nil
	}
	c.pending = true
	c.forcePending = false
	layout, revision := c.layout, c.revision
	if c.ackTimer != nil {
		c.ackTimer.Stop()
	}
	c.ackTimer = time.AfterFunc(3*time.Second, func() {
		c.mu.Lock()
		expired := c.pending && c.layout == layout && c.revision == revision
		c.mu.Unlock()
		if expired {
			c.Close()
		}
	})
	return nil
}

func (c *Client) handleControl(msg wire.Message) (bool, error) {
	if msg.Type != "visibility" && msg.Type != "view_ack" && msg.Type != "resync_required" {
		return false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.Type == "visibility" {
		if !c.selected {
			c.selected = true
			c.capabilities = wire.SelectCapabilities(msg.Capabilities)
			if c.reactiveLocked() {
				return false, c.publishNextLocked(true)
			}
		}
		return false, nil
	}
	if !c.reactiveLocked() {
		return true, nil
	}
	if msg.Type == "resync_required" {
		if c.pending && c.sentTransition {
			c.transition = c.sent
		}
		c.pending = false
		return true, c.publishNextLocked(true)
	}
	if msg.LayoutRevision < c.layout || msg.LayoutRevision == c.layout && msg.NewRevision < c.revision {
		return true, nil
	}
	if !c.pending || msg.LayoutRevision != c.layout || msg.NewRevision != c.revision {
		if c.pending && c.sentTransition {
			c.transition = c.sent
		}
		c.pending = false
		return true, c.publishNextLocked(true)
	}
	if c.ackTimer != nil {
		c.ackTimer.Stop()
	}
	c.acked = c.sent
	if c.sentConfirmed && c.confirmed != nil && reflect.DeepEqual(*c.acked, c.transportViewLocked(*c.confirmed)) {
		c.confirmed = nil
		close(c.confirmation)
		c.confirmation = nil
	}
	c.sent = nil
	c.sentTransition = false
	c.sentConfirmed = false
	c.pending = false
	return true, c.publishNextLocked(false)
}

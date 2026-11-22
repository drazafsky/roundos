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
	if c.closed.Load() {
		return 0
	}
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
	err := wire.Write(c.conn, msg)
	if err != nil {
		c.Close()
	}
	return err
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

// PublishSnapshot synchronously writes a full replacement. A new layout
// supersedes an older unacknowledged update, whose late ack is ignored. This
// avoids waiting for control messages while the application's event loop is
// applying a reload; ordinary Publish calls continue to coalesce behind ACKs.
func (c *Client) PublishSnapshot(ctx context.Context, view wire.View) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := wire.ValidateView(view); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.validateDesiredLocked(view); err != nil {
		return err
	}
	if !c.reactiveLocked() {
		c.legacy = nil
		err := c.publishLegacyLocked(view)
		if err == nil {
			raw := wire.CopyView(view)
			c.last = &raw
		}
		return err
	}
	if c.transition != nil && view.Dialog != nil {
		if err := c.writeSnapshotLocked(ctx, *c.transition); err != nil {
			return err
		}
	}
	return c.writeSnapshotLocked(ctx, view)
}

func (c *Client) writeSnapshotLocked(ctx context.Context, view wire.View) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.layout == ^uint64(0) {
		return fmt.Errorf("reactive revision exhausted")
	}
	raw := wire.CopyView(view)
	view = c.transportViewLocked(wire.CopyView(view))
	msg := wire.Message{Version: wire.Version, Type: "view", View: &view, LayoutRevision: c.layout + 1, NewRevision: 1}
	if !wire.MessageFits(msg) {
		return fmt.Errorf("replacement snapshot exceeds IPC frame")
	}
	if err := c.writeLocked(msg); err != nil {
		c.Close()
		return err
	}
	c.last = &raw
	c.transition = nil
	c.trackUpdateLocked(msg, view, false)
	return nil
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
			_, valid := wire.ApplyProperties(*c.acked, updates)
			if valid == nil && wire.MessageFits(patch) {
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
	c.trackUpdateLocked(msg, view, transition)
	return nil
}

func (c *Client) trackUpdateLocked(msg wire.Message, view wire.View, transition bool) {
	c.layout, c.revision = msg.LayoutRevision, msg.NewRevision
	c.sent = &view
	c.sentTransition = transition
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
	c.sent = nil
	c.sentTransition = false
	c.pending = false
	return true, c.publishNextLocked(false)
}

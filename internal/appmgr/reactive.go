package appmgr

import "roundos/internal/wire"

// retainedView is owned by one connection reader. A rejected message never
// advances its baseline; reconnections start with an empty receiver.
type retainedView struct {
	view             *wire.View
	layout, revision uint64
}

func (s *retainedView) accept(msg wire.Message) (wire.View, bool) {
	var next wire.View
	if !wire.MessageFits(msg) {
		return next, false
	}
	switch msg.Type {
	case "view":
		if msg.View == nil || msg.LayoutRevision == 0 || msg.LayoutRevision <= s.layout || msg.NewRevision != 1 || msg.BaseRevision != 0 {
			return next, false
		}
		next = wire.CopyView(*msg.View)
		if wire.ValidateView(next) != nil || !wire.MessageFits(msg) {
			return wire.View{}, false
		}
	case "widget_patch":
		if s.view == nil || msg.LayoutRevision != s.layout || msg.BaseRevision != s.revision || msg.NewRevision <= msg.BaseRevision || msg.NewRevision != msg.BaseRevision+1 {
			return next, false
		}
		var err error
		next, err = wire.ApplyProperties(*s.view, msg.Updates)
		if err != nil {
			return wire.View{}, false
		}
	default:
		return next, false
	}
	s.view = &next
	s.layout, s.revision = msg.LayoutRevision, msg.NewRevision
	return next, true
}

// UIInput receives an origin recorded by shell hit-testing or rotary focus.
// Its layout revision must match the model that produced that input.
func (m *Manager) UIInput(id string, event wire.UIEvent) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.peers[id]
	if e == nil || id != m.active {
		return false
	}
	if !wire.HasCapability(e.capabilities, wire.UIEventV1) {
		delta := 0
		if event.Int != nil {
			delta = *event.Int
		}
		return e.enqueue(wire.Message{Version: wire.Version, Type: "input", Action: event.Action, Delta: delta})
	}
	if e.view == nil || event.LayoutRevision != e.layout || e.eventID == ^uint64(0) {
		return false
	}
	event.ID = e.eventID + 1
	if wire.ValidateUIEvent(*e.view, event) != nil {
		return false
	}
	if !e.enqueue(wire.Message{Version: wire.Version, Type: "ui_event", UIEvent: &event}) {
		return false
	}
	e.eventID = event.ID
	return true
}

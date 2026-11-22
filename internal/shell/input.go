package shell

import "roundos/internal/wire"

func (m *Model) recordWidgetInput(widget wire.Widget, kind string, value int, checked bool) {
	screen, ok := m.CurrentScreen(m.Page)
	if !ok {
		return
	}
	event := &wire.UIEvent{Screen: screen.ID, Widget: widget.ID, Action: widget.Action, Kind: kind, LayoutRevision: m.ViewLayouts[m.Page]}
	switch kind {
	case "int":
		event.Int = &value
	case "bool":
		event.Bool = &checked
	}
	m.widgetInput = event
}

// TakeWidgetInput consumes exactly the origin of the just-completed action.
// Dialogs, notifications and legacy controls keep their ordinary input route.
func (m *Model) TakeWidgetInput(action string) *wire.UIEvent {
	event := m.widgetInput
	m.widgetInput = nil
	if event != nil && event.Action == action {
		return event
	}
	return nil
}

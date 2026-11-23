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

// SendWidgetInput consumes a recorded origin and restores a rejected optimistic
// toggle locally. A dropped event never reaches the app's recovery path.
func (m *Model) SendWidgetInput(action string, send func(wire.UIEvent) bool) (handled, delivered bool) {
	event := m.TakeWidgetInput(action)
	if event == nil {
		return false, false
	}
	if send(*event) {
		return true, true
	}
	if event.Bool != nil && event.LayoutRevision == m.ViewLayouts[m.Page] {
		view := m.Views[m.Page]
		for si := range view.Screens {
			if view.Screens[si].ID != event.Screen {
				continue
			}
			for wi := range view.Screens[si].Widgets {
				widget := &view.Screens[si].Widgets[wi]
				if widget.ID == event.Widget && widget.Kind == "toggle" && widget.Action == event.Action && widget.Checked == *event.Bool {
					widget.Checked = !*event.Bool
				}
			}
		}
	}
	return true, false
}

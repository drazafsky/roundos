package shell

import (
	"math"

	"roundos/internal/circleui"
	"roundos/internal/wire"
)

func widgetByID(screen wire.Screen, id string) (wire.Widget, int, bool) {
	for i, w := range screen.Widgets {
		if w.ID == id {
			return w, i, true
		}
	}
	return wire.Widget{}, 0, false
}

// reconcileReplacement preserves stable identities while resolving focus using
// the old visual order. Labels/values alone never cancel an in-progress edit.
func (m *Model) reconcileReplacement(page int, old, next wire.View) {
	frames := m.AppStacks[page]
	for i := range frames {
		before, ok := findScreen(old, frames[i].ScreenID)
		if !ok {
			continue
		}
		after, ok := findScreen(next, frames[i].ScreenID)
		if !ok {
			continue
		}
		if w, _, ok := widgetByID(after, frames[i].FocusID); ok && circleui.Focusable(w) {
			continue
		}
		_, at, hadFocus := widgetByID(before, frames[i].FocusID)
		if !hadFocus {
			continue
		}
		best, distance := "", math.MaxInt
		for j, w := range before.Widgets {
			candidate, _, exists := widgetByID(after, w.ID)
			if !exists || !circleui.Focusable(candidate) {
				continue
			}
			d := j - at
			if d < 0 {
				d = -d
			}
			if d < distance {
				best, distance = w.ID, d
			}
		}
		if best == "" {
			if first := circleui.First(after.Widgets); first >= 0 {
				best = after.Widgets[first].ID
			} else {
				best = "__back"
			}
		}
		frames[i].FocusID = best
	}
	if page != m.Page || len(frames) == 0 {
		return
	}
	id := frames[len(frames)-1].ScreenID
	before, okBefore := findScreen(old, id)
	after, okAfter := findScreen(next, id)
	compatible := func(id string) bool {
		if id == "" {
			return true
		}
		a, _, okA := widgetByID(before, id)
		b, _, okB := widgetByID(after, id)
		return okBefore && okAfter && okA && okB && !b.Disabled && a.Kind == b.Kind && a.Action == b.Action && a.Target == b.Target && a.Min == b.Min && a.Max == b.Max && a.Step == b.Step && a.Wrap == b.Wrap
	}
	if !compatible(m.PickerID) {
		m.cancelPicker()
	}
	if !compatible(m.EditingWidget) || !compatible(m.TouchSlider) {
		m.cancelWidgetEdit()
	}
	changed := !okBefore || !okAfter || len(before.Widgets) != len(after.Widgets) || before.Hero != "" && after.Hero == "" || before.Hero == "" && after.Hero != ""
	if !changed {
		for i, w := range before.Widgets {
			if w.ID != after.Widgets[i].ID || w.Kind != after.Widgets[i].Kind {
				changed = true
				break
			}
		}
	}
	if changed {
		m.StopKinetic()
	}
}

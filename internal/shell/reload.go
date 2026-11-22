package shell

import (
	"math"
	"time"

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
	if page == m.Page && (old.Dialog == nil) != (next.Dialog == nil) {
		m.cancelReloadContact()
	} else if page == m.Page && old.Dialog != nil && next.Dialog != nil &&
		(old.Dialog.ID != next.Dialog.ID || old.Dialog.CancelAction != next.Dialog.CancelAction || old.Dialog.ConfirmAction != next.Dialog.ConfirmAction) {
		m.cancelReloadContact()
	}
	for i := range frames {
		before, ok := findScreen(old, frames[i].ScreenID)
		if !ok {
			continue
		}
		after, ok := findScreen(next, frames[i].ScreenID)
		if !ok {
			continue
		}
		if w, nextIndex, ok := widgetByID(after, frames[i].FocusID); ok && circleui.Focusable(w) {
			_, oldIndex, existed := widgetByID(before, frames[i].FocusID)
			if !existed || oldIndex != nextIndex || before.Layout != after.Layout || (before.Hero == "") != (after.Hero == "") {
				m.revealReloadFocus(&frames[i], after, nextIndex)
			}
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
		if _, index, ok := widgetByID(after, best); ok {
			m.revealReloadFocus(&frames[i], after, index)
		}
	}
	if page != m.Page {
		return
	}
	if len(frames) == 0 {
		if len(next.Screens) != 0 || old.ButtonAction != next.ButtonAction {
			m.cancelReloadContact()
		}
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
	changed := !okBefore || !okAfter || before.Layout != after.Layout || len(before.Widgets) != len(after.Widgets) || before.Hero != "" && after.Hero == "" || before.Hero == "" && after.Hero != ""
	if !changed {
		for i, w := range before.Widgets {
			if w.ID != after.Widgets[i].ID || !sameInputTarget(w, after.Widgets[i]) {
				changed = true
				break
			}
		}
	}
	if changed {
		m.cancelReloadContact()
	}
}

func (m *Model) revealReloadFocus(frame *NavFrame, screen wire.Screen, index int) {
	_, _, _, height := circleui.ScreenMetrics(screen, m.Appearance.LargeTargets)
	for _, row := range circleui.VisibleScreenPixelAt(screen, frame.ScrollPx, m.Appearance.LargeTargets) {
		if row.Index == index && row.Rect.Bottom-row.Rect.Top >= height {
			return
		}
	}
	frame.Scroll = circleui.ScreenScrollTo(screen, index, frame.Scroll, m.Appearance.LargeTargets)
	frame.ScrollPx = circleui.ScreenRowOffset(screen, frame.Scroll, m.Appearance.LargeTargets)
}

func sameInputTarget(a, b wire.Widget) bool {
	return a.Kind == b.Kind && a.Group == b.Group && a.Action == b.Action && a.Target == b.Target &&
		a.Disabled == b.Disabled && a.Min == b.Min && a.Max == b.Max && a.Step == b.Step && a.Wrap == b.Wrap
}

func (m *Model) cancelReloadContact() {
	m.InputRevision++
	// Keep compatible rotary/picker drafts, but detach the canceled finger so
	// its release cannot commit a slider or start an inertial fling.
	m.TouchSlider = ""
	m.DragOriginScroll, m.DragOriginY = 0, 0
	m.DragOriginPx, m.DragLastDY, m.DragVelocity = 0, 0, 0
	m.DragLastAt = time.Time{}
	m.StopKinetic()
}

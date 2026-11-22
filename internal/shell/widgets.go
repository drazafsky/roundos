package shell

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"roundos/internal/circleui"
	"roundos/internal/wire"
)

// NavFrame captures a screen's focused widget and its list viewport. Frames
// belong to the application; shell overlays never mutate the stack.
type NavFrame struct {
	ScreenID string
	FocusID  string
	Scroll   int
	ScrollPx float64
}

func findScreen(v wire.View, id string) (wire.Screen, bool) {
	for _, s := range v.Screens {
		if s.ID == id {
			return s, true
		}
	}
	return wire.Screen{}, false
}
func findFocus(s wire.Screen, id string) int {
	for i, w := range s.Widgets {
		if w.ID == id && circleui.Focusable(w) {
			return i
		}
	}
	return circleui.First(s.Widgets)
}
func (m *Model) navFrame(page int) (*NavFrame, wire.Screen, bool) {
	stack := m.AppStacks[page]
	if len(stack) == 0 {
		return nil, wire.Screen{}, false
	}
	f := &m.AppStacks[page][len(stack)-1]
	s, ok := findScreen(m.Views[page], f.ScreenID)
	return f, s, ok
}
func (m *Model) hasWidgets(page int) bool {
	return m.IsAppPage(page) && len(m.Views[page].Screens) > 0
}
func (m *Model) syncNavigation(page int) {
	v := m.Views[page]
	if len(v.Screens) == 0 {
		delete(m.AppStacks, page)
		return
	}
	if len(m.AppStacks[page]) == 0 {
		m.AppStacks[page] = []NavFrame{{ScreenID: v.Screens[0].ID}}
	}
	frames := m.AppStacks[page]
	valid := frames[:0]
	for _, f := range frames {
		if s, ok := findScreen(v, f.ScreenID); ok {
			i := findFocus(s, f.FocusID)
			if f.FocusID == "__back" && (len(valid) > 0 || circleui.First(s.Widgets) < 0) {
				// Preserve nested-screen Back focus after live app updates.
			} else if i >= 0 {
				f.FocusID = s.Widgets[i].ID
			} else {
				f.FocusID = "__back"
			}
			if f.Scroll < 0 {
				f.Scroll = 0
			}
			if f.Scroll >= len(s.Widgets) {
				f.Scroll = max(0, len(s.Widgets)-1)
			}
			f.ScrollPx = math.Max(0, math.Min(circleui.ScreenMaxScrollPx(s, m.Appearance.LargeTargets), f.ScrollPx))
			valid = append(valid, f)
		} else {
			break
		}
	}
	if len(valid) == 0 {
		valid = []NavFrame{{ScreenID: v.Screens[0].ID}}
		if i := circleui.First(v.Screens[0].Widgets); i >= 0 {
			valid[0].FocusID = v.Screens[0].Widgets[i].ID
		} else {
			valid[0].FocusID = "__back"
		}
	}
	m.AppStacks[page] = valid
}
func (m *Model) CurrentScreen(page int) (wire.Screen, bool) {
	_, s, ok := m.navFrame(page)
	return s, ok
}
func (m *Model) CurrentFocus(page int) int {
	f, s, ok := m.navFrame(page)
	if !ok {
		return -1
	}
	if f.FocusID == "__back" {
		return -2
	}
	return findFocus(s, f.FocusID)
}
func (m *Model) NavDepth(page int) int { return len(m.AppStacks[page]) }
func (m *Model) enterScreen(target string) bool {
	if target == "" || len(m.AppStacks[m.Page]) >= 32 {
		return false
	}
	v := m.Views[m.Page]
	s, ok := findScreen(v, target)
	if !ok {
		return false
	}
	f := NavFrame{ScreenID: target}
	if i := circleui.First(s.Widgets); i >= 0 {
		f.FocusID = s.Widgets[i].ID
	}
	m.cancelWidgetEdit()
	m.AppStacks[m.Page] = append(m.AppStacks[m.Page], f)
	return true
}
func (m *Model) backAppScreen() bool {
	if !m.hasWidgets(m.Page) || len(m.AppStacks[m.Page]) < 2 {
		return false
	}
	m.cancelWidgetEdit()
	m.AppStacks[m.Page] = m.AppStacks[m.Page][:len(m.AppStacks[m.Page])-1]
	return true
}
func (m *Model) cancelWidgetEdit() { m.EditingWidget = ""; m.TouchSlider = ""; m.WidgetDelta = 0 }
func (m *Model) visibleWidgets() []circleui.Row {
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return nil
	}
	idx := findFocus(s, f.FocusID)
	_ = idx
	return circleui.VisibleScreenPixelAt(s, f.ScrollPx, m.Appearance.LargeTargets)
}
func (m *Model) WidgetRotate(delta int) bool {
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return false
	}
	if f.FocusID == "__back" {
		if delta > 0 {
			if i := circleui.First(s.Widgets); i >= 0 {
				f.FocusID = s.Widgets[i].ID
				return true
			}
		}
		return false
	}
	i := findFocus(s, f.FocusID)
	if delta < 0 && i == circleui.First(s.Widgets) && len(m.AppStacks[m.Page]) > 1 {
		f.FocusID = "__back"
		return true
	}
	if m.EditingWidget != "" && i >= 0 && s.Widgets[i].ID == m.EditingWidget {
		w := s.Widgets[i]
		step := w.Step
		if step <= 0 {
			step = 1
		}
		low, high := w.Min, w.Max
		if high < low {
			low, high = high, low
		}
		value := max(low, min(high, m.WidgetEditValue+step*delta))
		if value == m.WidgetEditValue {
			return false
		}
		m.WidgetEditValue = value
		return true
	}
	if delta < 0 && i == circleui.First(s.Widgets) && len(m.AppStacks[m.Page]) > 1 {
		f.FocusID = "__back"
		return true
	}
	next := circleui.Next(s.Widgets, i, delta)
	if next == i {
		return false
	}
	f.FocusID = s.Widgets[next].ID
	f.Scroll = circleui.ScreenScrollTo(s, next, f.Scroll, m.Appearance.LargeTargets)
	_, _, step, _ := circleui.ScreenMetrics(s, m.Appearance.LargeTargets)
	f.ScrollPx = float64(f.Scroll * step)
	m.StopKinetic()
	return true
}
func (m *Model) ActivateWidget() (bool, string) {
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return false, ""
	}
	if f.FocusID == "__back" {
		return m.Back(), ""
	}
	i := findFocus(s, f.FocusID)
	if i < 0 {
		return false, ""
	}
	w := s.Widgets[i]
	if !circleui.Focusable(w) {
		return false, ""
	}
	if w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker" {
		return m.openPicker(w), ""
	}
	if w.Kind == "slider" {
		if m.EditingWidget == w.ID {
			m.EditingWidget = ""
			m.WidgetDelta = m.WidgetEditValue
			if m.WidgetEditValue == w.Value {
				return true, ""
			}
			m.recordWidgetInput(w, "int", m.WidgetEditValue, false)
			return true, w.Action
		}
		m.EditingWidget = w.ID
		m.WidgetEditValue = w.Value
		m.WidgetDelta = 0
		return true, ""
	}
	if w.Target != "" {
		return m.enterScreen(w.Target), ""
	}
	if w.Kind == "toggle" {
		for si := range m.Views[m.Page].Screens {
			if m.Views[m.Page].Screens[si].ID == s.ID {
				for wi := range m.Views[m.Page].Screens[si].Widgets {
					if m.Views[m.Page].Screens[si].Widgets[wi].ID == w.ID {
						m.Views[m.Page].Screens[si].Widgets[wi].Checked = !w.Checked
					}
				}
			}
		}
		m.WidgetDelta = 0
		m.recordWidgetInput(w, "bool", 0, !w.Checked)
		return true, w.Action
	}
	if w.Kind == "button" || w.Kind == "list_item" {
		m.WidgetDelta = 0
		m.recordWidgetInput(w, "press", 0, false)
		return true, w.Action
	}
	return false, ""
}
func (m *Model) WidgetTap(x, y int) (bool, string) {
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return false, ""
	}
	// The Back affordance uses exactly the same rectangle for draw and touch.
	if (len(m.AppStacks[m.Page]) > 1 || f.FocusID == "__back") && circleui.BackRect().Hit(x, y) {
		return m.Back(), ""
	}
	rows := m.visibleWidgets()
	i := circleui.Hit(rows, x, y)
	if i < 0 {
		return false, ""
	}
	w := s.Widgets[i]
	if w.Disabled {
		return false, ""
	}
	if !circleui.Focusable(w) {
		return false, ""
	}
	f.FocusID = w.ID
	if w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker" {
		return m.openPicker(w), ""
	}
	if w.Kind == "slider" {
		m.cancelWidgetEdit()
		m.WidgetDelta = sliderValue(w, x, rows, i)
		if m.WidgetDelta == w.Value {
			return true, ""
		}
		m.recordWidgetInput(w, "int", m.WidgetDelta, false)
		return true, w.Action
	}
	return m.ActivateWidget()
}
func sliderValue(w wire.Widget, x int, rows []circleui.Row, idx int) int {
	low, high := w.Min, w.Max
	if high < low {
		low, high = high, low
	}
	step := w.Step
	if step <= 0 {
		step = 1
	}
	for _, r := range rows {
		if r.Index == idx {
			left, right := r.Rect.Left+18, r.Rect.Right-18
			if right <= left {
				return low
			}
			ratio := float64(x-left) / float64(right-left)
			ratio = math.Max(0, math.Min(1, ratio))
			value := low + int(math.Round((float64(high-low)*ratio)/float64(step)))*step
			return max(low, min(high, value))
		}
	}
	return w.Value
}

// Touch scrolling uses continuous pixel offsets, with a bounded short inertial
// glide when the release velocity is high. Input events and animation never
// wait on DRM page-flips; the UI loop schedules only necessary animation ticks.
func (m *Model) StopKinetic() {
	m.KineticVelocity = 0
	m.KineticLast = time.Time{}
	m.KineticUntil = time.Time{}
}
func (m *Model) NextKineticWake(now time.Time) time.Duration {
	if m.KineticVelocity == 0 || m.Page != m.KineticPage {
		return 0
	}
	if m.KineticLast.IsZero() {
		return 16 * time.Millisecond
	}
	wait := m.KineticLast.Add(16 * time.Millisecond).Sub(now)
	if wait <= 0 {
		return time.Millisecond
	}
	return wait
}
func (m *Model) AdvanceKinetic(now time.Time) bool {
	if m.KineticVelocity == 0 || m.Page != m.KineticPage {
		m.StopKinetic()
		return false
	}
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		m.StopKinetic()
		return false
	}
	dt := now.Sub(m.KineticLast).Seconds()
	if dt <= 0 {
		return false
	}
	if dt > 0.05 {
		dt = 0.05
	}
	old := f.ScrollPx
	next := old + m.KineticVelocity*dt
	maxPx := circleui.ScreenMaxScrollPx(s, m.Appearance.LargeTargets)
	f.ScrollPx = math.Max(0, math.Min(maxPx, next))
	_, _, step, _ := circleui.ScreenMetrics(s, m.Appearance.LargeTargets)
	f.Scroll = int(f.ScrollPx / float64(step))
	speed := math.Abs(m.KineticVelocity) - 2200*dt
	if speed <= 0 || now.After(m.KineticUntil) || (f.ScrollPx == old && next != old) {
		m.StopKinetic()
	} else {
		m.KineticVelocity = math.Copysign(speed, m.KineticVelocity)
		m.KineticLast = now
	}
	return math.Abs(f.ScrollPx-old) > 0.05
}
func (m *Model) WidgetDragStart(x, y int) bool {
	m.StopKinetic()
	if !m.hasWidgets(m.Page) || m.hasVisibleDialog() || m.PickerID != "" {
		return false
	}
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return false
	}
	m.DragOriginPx = f.ScrollPx
	m.DragOriginY = y
	m.DragLastDY = 0
	m.DragLastAt = time.Time{}
	m.DragVelocity = 0
	m.TouchSlider = ""
	for _, r := range m.visibleWidgets() {
		if r.Rect.Hit(x, y) && s.Widgets[r.Index].Kind == "slider" && !s.Widgets[r.Index].Disabled {
			m.TouchSlider = s.Widgets[r.Index].ID
			break
		}
	}
	return false
}
func (m *Model) WidgetDragMove(x, y int, deltaY float64) bool {
	return m.WidgetDragMoveAt(x, y, deltaY, time.Now())
}
func (m *Model) WidgetDragMoveAt(x, y int, deltaY float64, at time.Time) bool {
	if !m.hasWidgets(m.Page) || m.hasVisibleDialog() || m.PickerID != "" {
		return false
	}
	f, s, ok := m.navFrame(m.Page)
	if !ok {
		return false
	}
	if m.TouchSlider != "" {
		for _, r := range m.visibleWidgets() {
			if s.Widgets[r.Index].ID == m.TouchSlider {
				value := sliderValue(s.Widgets[r.Index], x, []circleui.Row{r}, r.Index)
				if value != m.WidgetEditValue || m.EditingWidget != m.TouchSlider {
					m.EditingWidget = m.TouchSlider
					m.WidgetEditValue = value
					return true
				}
				return false
			}
		}
		return false
	}
	maxPx := circleui.ScreenMaxScrollPx(s, m.Appearance.LargeTargets)
	if maxPx == 0 {
		return false
	}
	old := f.ScrollPx
	f.ScrollPx = math.Max(0, math.Min(maxPx, m.DragOriginPx-deltaY))
	_, _, step, _ := circleui.ScreenMetrics(s, m.Appearance.LargeTargets)
	f.Scroll = int(f.ScrollPx / float64(step))
	if !m.DragLastAt.IsZero() {
		dt := at.Sub(m.DragLastAt).Seconds()
		if dt >= 0.004 && dt <= 0.20 {
			instant := -(deltaY - m.DragLastDY) / dt
			if instant > 1400 {
				instant = 1400
			}
			if instant < -1400 {
				instant = -1400
			}
			m.DragVelocity = 0.4*m.DragVelocity + 0.6*instant
		}
	}
	m.DragLastAt = at
	m.DragLastDY = deltaY
	// After a direct touch scroll, focus the first fully or substantially visible
	// selectable item, instead of pulling the viewport back to the old focus.
	rows := m.visibleWidgets()
	focusVisible := false
	for _, r := range rows {
		if r.Index == findFocus(s, f.FocusID) && r.Rect.Bottom-r.Rect.Top >= 38 {
			focusVisible = true
		}
	}
	if !focusVisible {
		for _, r := range rows {
			if circleui.Focusable(s.Widgets[r.Index]) && r.Rect.Bottom-r.Rect.Top >= 38 {
				f.FocusID = s.Widgets[r.Index].ID
				break
			}
		}
	}
	return math.Abs(f.ScrollPx-old) > 0.05
}
func (m *Model) WidgetDragEnd() (bool, string) { return m.WidgetDragEndAt(time.Now()) }
func (m *Model) WidgetDragEndAt(at time.Time) (bool, string) {
	if m.TouchSlider != "" && m.EditingWidget == m.TouchSlider {
		_, s, ok := m.navFrame(m.Page)
		if ok {
			for _, w := range s.Widgets {
				if w.ID == m.TouchSlider {
					m.WidgetDelta = m.WidgetEditValue
					changed := m.WidgetEditValue != w.Value
					m.cancelWidgetEditKeepDelta()
					if changed {
						m.recordWidgetInput(w, "int", m.WidgetEditValue, false)
						return true, w.Action
					}
					return true, ""
				}
			}
		}
	}
	m.TouchSlider = ""
	if !m.Appearance.ReduceMotion && math.Abs(m.DragVelocity) >= 230 && !m.DragLastAt.IsZero() && at.Sub(m.DragLastAt) < 140*time.Millisecond {
		m.KineticVelocity = m.DragVelocity
		m.KineticPage = m.Page
		m.KineticLast = at
		m.KineticUntil = at.Add(220 * time.Millisecond)
	}
	m.DragVelocity = 0
	return false, ""
}
func (m *Model) cancelWidgetEditKeepDelta() { m.EditingWidget = ""; m.TouchSlider = "" }

func drawWidgetScreen(im *image.RGBA, m *Model) {
	s, ok := m.CurrentScreen(m.Page)
	if !ok {
		return
	}
	p := m.Appearance.Palette()
	stack := m.AppStacks[m.Page]
	hasBack := len(stack) > 1 || m.CurrentFocus(m.Page) == -2
	if hasBack {
		back := circleui.BackRect()
		rect(im, back.Left, back.Top, back.Right, back.Bottom, p.Surface)
		if stack[len(stack)-1].FocusID == "__back" {
			outlineRect(im, back.Left, back.Top, back.Right, back.Bottom, p.Focus)
		}
		textTheme(im, (back.Left+back.Right)/2, (back.Top+back.Bottom)/2-10, 3, "<", p.PrimaryText, true, m.Appearance)
	}
	title := s.Title
	if title == "" {
		title = "APPLICATION"
	}
	if hasBack && len(title) > 10 {
		words := strings.Fields(strings.ToUpper(title))
		if len(words) > 1 {
			textTheme(im, 240, 86, 3, words[0], p.PrimaryText, true, m.Appearance)
			textTheme(im, 240, 114, 3, strings.Join(words[1:], " "), p.PrimaryText, true, m.Appearance)
		} else {
			textTheme(im, 240, 100, 2, strings.ToUpper(title), p.PrimaryText, true, m.Appearance)
		}
	} else {
		textTheme(im, 240, 100, 3, strings.ToUpper(title), p.PrimaryText, true, m.Appearance)
	}
	if s.Hero != "" {
		textTheme(im, 240, 191, 6, strings.ToUpper(s.Hero), p.PrimaryText, true, m.Appearance)
		if s.Subtitle != "" {
			textTheme(im, 240, 246, 2, strings.ToUpper(s.Subtitle), p.SecondaryText, true, m.Appearance)
		}
	} else if s.Subtitle != "" {
		textTheme(im, 240, 148, 1, strings.ToUpper(s.Subtitle), p.SecondaryText, true, m.Appearance)
	}
	rows := m.visibleWidgets()
	focus := m.CurrentFocus(m.Page)
	for _, r := range rows {
		w := s.Widgets[r.Index]
		focused := r.Index == focus
		fill := p.Surface
		if focused {
			fill = p.Selected
		}
		if w.Kind == "button" && w.Variant == "primary" {
			fill = p.Accent
		}
		rect(im, r.Rect.Left, r.Rect.Top, r.Rect.Right, r.Rect.Bottom, fill)
		border := p.Border
		if focused {
			border = p.Focus
		}
		if focused && w.Kind == "button" && w.Variant == "primary" {
			border = p.PrimaryText
		}
		outlineRect(im, r.Rect.Left, r.Rect.Top, r.Rect.Right, r.Rect.Bottom, border)
		if w.Group != "" && strings.HasPrefix(w.ID, "rui.heading.") {
			columns := max(8, (r.Rect.Right-r.Rect.Left-24)/12)
			lines := wrapDetail(strings.ToUpper(w.Label), columns, 2)
			for i, line := range lines {
				textTheme(im, 240, r.Rect.Top+12+i*20, 1, line, p.Accent, true, m.Appearance)
			}
			continue
		}
		if s.Layout == "detail" && w.Kind == "label" {
			content := w.Label
			if w.Detail != "" {
				content += " " + w.Detail
			}
			scale := 1
			if m.Appearance.TextScale >= 125 {
				scale = 2
			}
			columns := max(8, (r.Rect.Right-r.Rect.Left-24)/(6*scale))
			lines := wrapDetail(strings.ToUpper(content), columns, max(1, (r.Rect.Bottom-r.Rect.Top-16)/18))
			color := p.PrimaryText
			if w.Disabled {
				color = p.Disabled
			}
			for i, line := range lines {
				textTheme(im, 240, r.Rect.Top+8+i*18, 1, line, color, true, m.Appearance)
			}
			continue
		}
		label := strings.ToUpper(w.Label)
		if runes := []rune(label); len(runes) > 17 {
			label = string(runes[:17])
		}
		c := p.PrimaryText
		if w.Kind == "button" && w.Variant == "primary" {
			c = p.OnAccent
		}
		if w.Disabled {
			c = p.Disabled
		}
		top := r.Rect.Top
		middle := (r.Rect.Top + r.Rect.Bottom) / 2
		if w.Kind == "toggle" {
			state := "OFF"
			if w.Checked {
				state = "ON"
			}
			textTheme(im, 240, top+9, 2, label, c, true, m.Appearance)
			textTheme(im, 240, top+35, 2, state, p.Accent, true, m.Appearance)
		} else if w.Kind == "slider" {
			value := w.Value
			if focused && m.EditingWidget == w.ID {
				value = m.WidgetEditValue
			}
			textTheme(im, 240, top+7, 2, label, c, true, m.Appearance)
			pct := 0
			if w.Max > w.Min {
				pct = (value - w.Min) * 100 / (w.Max - w.Min)
			}
			trackL, trackR := r.Rect.Left+18, r.Rect.Right-18
			lineY := r.Rect.Bottom - 14
			rect(im, trackL, lineY, trackR, lineY+6, p.Border)
			rect(im, trackL, lineY, trackL+(trackR-trackL)*max(0, min(100, pct))/100, lineY+6, p.Accent)
			amount := fmt.Sprintf("%d", value)
			if w.Detail != "" {
				amount += " " + strings.ToUpper(w.Detail)
			}
			textTheme(im, 240, top+28, 2, amount, c, true, m.Appearance)
			if focused && m.EditingWidget == w.ID {
				outlineRect(im, r.Rect.Left-3, r.Rect.Top-3, r.Rect.Right+3, r.Rect.Bottom+3, p.Accent)
			}
		} else if w.Kind == "picker" || w.Kind == "time_picker" || w.Kind == "date_picker" {
			display := fmt.Sprintf("%d", w.Value)
			if w.Kind == "time_picker" {
				display = fmt.Sprintf("%02d:%02d", w.Value/60, w.Value%60)
			}
			if w.Kind == "date_picker" {
				display = time.Unix(int64(w.Value)*86400, 0).UTC().Format("2006-01-02")
			}
			textTheme(im, 240, top+7, 2, label, c, true, m.Appearance)
			textTheme(im, 240, top+34, 2, display+"  >", p.Accent, true, m.Appearance)
		} else if w.Kind == "progress" {
			textTheme(im, 240, top+6, 2, label, c, true, m.Appearance)
			textTheme(im, 240, top+30, 1, fmt.Sprintf("%d", w.Value), c, true, m.Appearance)
			pct := 0
			if w.Max > w.Min {
				pct = (w.Value - w.Min) * 100 / (w.Max - w.Min)
			}
			rect(im, r.Rect.Left+18, r.Rect.Bottom-14, r.Rect.Right-18, r.Rect.Bottom-8, p.Border)
			rect(im, r.Rect.Left+18, r.Rect.Bottom-14, r.Rect.Left+18+(r.Rect.Right-r.Rect.Left-36)*max(0, min(100, pct))/100, r.Rect.Bottom-8, p.Accent)
		} else {
			textTheme(im, 240, middle-10, 2, label, c, true, m.Appearance)
			if w.Detail != "" && r.Rect.Bottom-r.Rect.Top >= 60 {
				textTheme(im, 240, middle+12, 1, strings.ToUpper(w.Detail), p.SecondaryText, true, m.Appearance)
			}
		}
	}
	if len(s.Widgets) > len(rows) && !strings.HasPrefix(s.ID, "root") {
		textTheme(im, 240, 432, 1, "ROTATE OR SWIPE TO BROWSE", p.SecondaryText, true, m.Appearance)
	}
}

// CancelWidgetDrag prevents edge gestures or a second finger from committing
// a partially dragged slider or producing an inertial fling.
func (m *Model) CancelWidgetDrag() {
	if m.TouchSlider != "" {
		m.cancelWidgetEdit()
	}
	m.DragVelocity = 0
	m.StopKinetic()
}

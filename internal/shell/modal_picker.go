package shell

import (
	"fmt"
	"image"
	"roundos/internal/wire"
	"strings"
	"time"
)

func (m *Model) currentDialog() *wire.Dialog {
	if !m.hasWidgets(m.Page) {
		return nil
	}
	return m.Views[m.Page].Dialog
}
func (m *Model) hasVisibleDialog() bool { return m.currentDialog() != nil && !m.DialogHidden }
func (m *Model) syncDialog(page int) {
	d := m.Views[page].Dialog
	if d == nil {
		m.DialogID = ""
		m.DialogHidden = false
		m.DialogFocus = 0
		return
	}
	if d.ID != m.DialogID {
		m.DialogID = d.ID
		m.DialogFocus = 0
		m.DialogHidden = false
	}
}
func (m *Model) dismissDialog(confirm bool) bool {
	d := m.currentDialog()
	if d == nil || m.DialogHidden {
		return false
	}
	m.DialogHidden = true // prevent duplicate actions until the app clears the dialog
	m.PendingDialogApp = m.AppID(m.Page)
	m.PendingDialogAction = d.CancelAction
	if confirm {
		m.PendingDialogAction = d.ConfirmAction
	}
	return true
}
func (m *Model) TakeDialogAction() (string, string) {
	app, action := m.PendingDialogApp, m.PendingDialogAction
	m.PendingDialogApp, m.PendingDialogAction = "", ""
	return app, action
}
func (m *Model) dialogRotate(delta int) bool {
	if delta == 0 {
		return false
	}
	old := m.DialogFocus
	m.DialogFocus = max(0, min(1, m.DialogFocus+delta))
	return old != m.DialogFocus
}
func (m *Model) dialogActivate() (bool, string) { return m.dismissDialog(m.DialogFocus == 1), "" }
func (m *Model) dialogTap(x, y int) (bool, string) {
	if y < 274 || y > 352 {
		return false, ""
	}
	if x >= 99 && x < 235 {
		m.DialogFocus = 0
		return m.dialogActivate()
	}
	if x >= 245 && x <= 381 {
		m.DialogFocus = 1
		return m.dialogActivate()
	}
	return false, ""
}
func (m *Model) openPicker(w wire.Widget) bool {
	if w.Disabled || (w.Kind != "picker" && w.Kind != "time_picker" && w.Kind != "date_picker") {
		return false
	}
	m.PickerID = w.ID
	m.PickerValue = w.Value
	m.PickerAction = w.Action
	return true
}
func (m *Model) cancelPicker() { m.PickerID = ""; m.PickerAction = ""; m.PickerValue = 0 }

// CancelPicker is reachable using a 700ms encoder hold, with no touch required.
// It never sends an uncommitted value or an app action.
func (m *Model) CancelPicker() bool {
	if m.PickerID == "" {
		return false
	}
	m.cancelPicker()
	return true
}
func (m *Model) pickerWidget() (wire.Widget, bool) {
	s, ok := m.CurrentScreen(m.Page)
	if !ok {
		return wire.Widget{}, false
	}
	for _, w := range s.Widgets {
		if w.ID == m.PickerID {
			return w, true
		}
	}
	return wire.Widget{}, false
}
func (m *Model) pickerRotate(delta int) bool {
	w, ok := m.pickerWidget()
	if !ok {
		m.cancelPicker()
		return false
	}
	step := w.Step
	if step <= 0 {
		step = 1
	}
	next := m.PickerValue + delta*step
	if w.Wrap {
		if next > w.Max {
			next = w.Min
		}
		if next < w.Min {
			next = w.Max - (w.Max-w.Min)%step
		}
	}
	next = max(w.Min, min(w.Max, next))
	if next == m.PickerValue {
		return false
	}
	m.PickerValue = next
	return true
}
func (m *Model) pickerCommit() (bool, string) {
	w, ok := m.pickerWidget()
	if !ok {
		m.cancelPicker()
		return false, ""
	}
	value, action := m.PickerValue, m.PickerAction
	m.cancelPicker()
	if value == w.Value {
		return true, ""
	}
	m.WidgetDelta = value
	return true, action
}
func (m *Model) pickerTap(x, y int) (bool, string) {
	if y < 147 && x < 162 {
		m.cancelPicker()
		return true, ""
	}
	if y >= 155 && y < 227 {
		return m.pickerRotate(1), ""
	}
	if y >= 295 && y < 365 {
		return m.pickerRotate(-1), ""
	}
	if y >= 371 && y < 430 {
		if x >= 133 && x < 233 {
			return m.CancelPicker(), ""
		}
		if x >= 247 && x <= 347 {
			return m.pickerCommit()
		}
	}
	return false, ""
}
func pickerLabel(w wire.Widget, value int) string {
	if w.Kind == "time_picker" {
		return fmt.Sprintf("%02d:%02d", value/60, value%60)
	}
	if w.Kind == "date_picker" {
		return time.Unix(int64(value)*86400, 0).UTC().Format("2006-01-02")
	}
	return fmt.Sprintf("%d", value)
}
func drawWidgetOverlays(im *image.RGBA, m *Model) {
	p := m.Appearance.Palette()
	if m.PickerID != "" {
		w, ok := m.pickerWidget()
		if !ok {
			return
		}
		disc(im, 240, 240, 221, p.Background)
		textTheme(im, 240, 106, 3, "SET "+strings.ToUpper(w.Label), p.PrimaryText, true, m.Appearance)
		rect(im, 91, 85, 155, 145, p.Surface)
		textTheme(im, 123, 101, 3, "<", p.PrimaryText, true, m.Appearance)
		textTheme(im, 240, 178, 3, pickerLabel(w, max(w.Min, min(w.Max, m.PickerValue+max(1, w.Step)))), p.SecondaryText, true, m.Appearance)
		rect(im, 126, 224, 354, 293, p.Selected)
		outlineRect(im, 126, 224, 354, 293, p.Focus)
		textTheme(im, 240, 238, 5, pickerLabel(w, m.PickerValue), p.PrimaryText, true, m.Appearance)
		textTheme(im, 240, 317, 3, pickerLabel(w, max(w.Min, min(w.Max, m.PickerValue-max(1, w.Step)))), p.SecondaryText, true, m.Appearance)
		rect(im, 136, 373, 232, 423, p.Surface)
		outlineRect(im, 136, 373, 232, 423, p.Border)
		textTheme(im, 184, 389, 2, "CANCEL", p.PrimaryText, true, m.Appearance)
		rect(im, 248, 373, 344, 423, p.Accent)
		outlineRect(im, 246, 371, 346, 425, p.Focus)
		textTheme(im, 296, 389, 2, "OK", p.OnAccent, true, m.Appearance)
		textTheme(im, 240, 433, 1, "HOLD KNOB CANCEL", p.SecondaryText, true, m.Appearance)
		return
	}
	if !m.hasVisibleDialog() {
		return
	}
	d := m.currentDialog()
	if d == nil {
		return
	}
	disc(im, 240, 240, 224, p.Background)
	disc(im, 240, 240, 168, p.Surface)
	title := strings.ToUpper(d.Title)
	if len(title) > 22 {
		title = title[:22]
	}
	textTheme(im, 240, 160, 3, title, p.PrimaryText, true, m.Appearance)
	body := strings.ToUpper(d.Body)
	if len(body) > 31 {
		body = body[:31]
	}
	textTheme(im, 240, 213, 2, body, p.SecondaryText, true, m.Appearance)
	for i := 0; i < 2; i++ {
		left := 99 + i*146
		right := left + 136
		fill, fg := p.Surface, p.PrimaryText
		if m.DialogFocus == i {
			fill = p.Selected
		}
		rect(im, left, 275, right, 346, fill)
		outline := p.Border
		if m.DialogFocus == i {
			outline = p.Focus
		}
		outlineRect(im, left, 275, right, 346, outline)
		label := d.Cancel
		if i == 1 {
			label = d.Confirm
		}
		if len(label) > 9 {
			label = label[:9]
		}
		textTheme(im, (left+right)/2, 299, 2, strings.ToUpper(label), fg, true, m.Appearance)
	}
}

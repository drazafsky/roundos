package shell

import (
	"fmt"
	"image"
	"strings"

	"roundos/internal/appearance"
	"roundos/internal/textedit"
	"roundos/internal/wire"
)

const (
	TextEditor    = NotificationDetail + 1
	Accessibility = NotificationDetail + 2
	DiscardPrompt = NotificationDetail + 3
)

// OpenAccessibility keeps the previous system view and focus position intact.
func (m *Model) OpenAccessibility() bool {
	if m.Page == Accessibility {
		return false
	}
	m.AccessParent = m.Page
	return m.Select(Accessibility)
}

// Editor is a shell-owned sample field; later app widgets can invoke the same
// editor model without introducing a separate gesture or theme engine.
func (m *Model) OpenEditor() bool {
	if m.Page == TextEditor || m.Page == DiscardPrompt {
		return false
	}
	m.EditorParent = m.Page
	m.EditorAppID = ""
	m.EditorRequestID = ""
	m.EditorLabel = "Edit text"
	m.Editor = textedit.New(m.Editor.Committed, false)
	return m.Select(TextEditor)
}

// OpenAppEditor opens the same editor on behalf of a foreground application.
func (m *Model) OpenAppEditor(appID string, req wire.TextRequest) bool {
	if appID == "" || m.Page != m.PageForApp(appID) || len(req.ID) == 0 || len(req.ID) > 64 {
		return false
	}
	m.EditorParent = m.Page
	m.EditorAppID = appID
	m.EditorRequestID = req.ID
	m.EditorLabel = req.Label
	if m.EditorLabel == "" {
		m.EditorLabel = "Edit text"
	}
	m.Editor = textedit.New(req.Initial, req.Secure)
	return m.Select(TextEditor)
}
func (m *Model) finishEditor(saved bool) bool {
	m.WiFiEditorFinish(saved)
	if m.EditorAppID != "" {
		r := wire.TextResult{ID: m.EditorRequestID, Canceled: !saved}
		if saved {
			r.Value = m.Editor.Committed
		}
		m.PendingEditorResult = &r
		m.PendingEditorApp = m.EditorAppID
	}
	if m.Editor.Secure {
		m.Editor.ClearSensitive()
	}
	return m.Select(m.EditorParent)
}
func (m *Model) TakeTextResult() (string, *wire.TextResult) {
	app, result := m.PendingEditorApp, m.PendingEditorResult
	m.PendingEditorApp = ""
	m.PendingEditorResult = nil
	return app, result
}
func (m *Model) EditorBack() bool {
	switch m.Page {
	case TextEditor:
		if m.Editor.Visible {
			m.Editor.Hide()
			return true
		}
		if m.Editor.Dirty() {
			m.DiscardFocus = 0
			return m.Select(DiscardPrompt)
		}
		return m.finishEditor(false)
	case DiscardPrompt:
		return m.Select(TextEditor)
	}
	return false
}
func (m *Model) ResolveDiscard(discard bool) bool {
	if m.Page != DiscardPrompt {
		return false
	}
	if discard {
		m.Editor.Cancel()
		return m.finishEditor(false)
	}
	m.Editor.Show()
	return m.Select(TextEditor)
}
func (m *Model) SetAppearance(s appearance.Settings) bool {
	if m.Appearance == s {
		return false
	}
	m.Appearance = s
	m.AppearanceDirty = true
	return true
}
func (m *Model) ChangeAppearance(row int) bool {
	s := m.Appearance
	switch row {
	case 0:
		s.NextTheme()
	case 1:
		s.NextScale()
	case 2:
		s.ReduceMotion = !s.ReduceMotion
	case 3:
		s.LargeTargets = !s.LargeTargets
	default:
		return false
	}
	return m.SetAppearance(s)
}

// accessGeometry changes both the visible density and actual touch hit targets.
func accessGeometry(m *Model) (start, visible, top, step, height int) {
	if m.Appearance.LargeTargets {
		return max(0, m.AccessFocus-1), 2, 149, 94, 82
	}
	return max(0, m.AccessFocus-2), 3, 150, 76, 66
}
func (m *Model) TapSupplement(x, y int) (bool, string) {
	switch m.Page {
	case TextEditor:
		return m.editorTap(x, y), ""
	case Accessibility:
		start, rows, rowY, step, height := accessGeometry(m)
		for i := start; i < 4 && i < start+rows; i++ {
			top := rowY + (i-start)*step
			if y >= top && y < top+height && x >= 91 && x < 389 {
				m.AccessFocus = i
				return m.ChangeAppearance(i), ""
			}
		}
		if y < 135 && x < 155 {
			return m.Back(), ""
		}
		return false, ""
	case DiscardPrompt:
		if y > 249 && y < 345 {
			if x < 240 {
				m.DiscardFocus = 0
			} else {
				m.DiscardFocus = 1
			}
			return m.ResolveDiscard(m.DiscardFocus == 1), ""
		}
		return false, ""
	}
	return false, ""
}
func (m *Model) editorTap(x, y int) bool {
	e := &m.Editor
	if !e.Visible {
		if y >= 176 && y < 381 {
			idx := (y - 176) / 71
			if idx > 2 {
				idx = 2
			}
			m.EditorFocus = idx
			switch idx {
			case 0:
				e.Show()
			case 1:
				e.Save()
				return m.finishEditor(true)
			case 2:
				return m.EditorBack()
			}
			return true
		}
		if y < 137 && x < 158 {
			return m.EditorBack()
		}
		return false
	}
	if y < 112 && x < 150 {
		return m.EditorBack()
	}
	if y >= 112 && y < 157 {
		e.SetCursorFromTap(float64(x-110) / 260)
		return true
	}
	if y >= 162 && y < 196 {
		if x >= 93 && x < 393 {
			e.SelectGroup((x - 93) / 100)
			return true
		}
	}
	if y >= 198 && y < 246 {
		if x >= 46 && x < 436 {
			idx := (x - 46) / 39
			if idx < 10 {
				return m.editorLetter("QWERTYUIOP", idx)
			}
		}
	}
	if y >= 247 && y < 297 {
		if x >= 66 && x < 417 {
			idx := (x - 66) / 39
			if idx < 9 {
				return m.editorLetter("ASDFGHJKL", idx)
			}
		}
	}
	if y >= 298 && y < 347 {
		if x >= 105 && x < 378 {
			idx := (x - 105) / 39
			if idx < 7 {
				return m.editorLetter("ZXCVBNM", idx)
			}
		}
	}
	if y >= 348 && y < 394 {
		if x >= 95 && x < 158 {
			e.Upper = !e.Upper
			return true
		}
		if x >= 162 && x < 294 {
			return e.Insert(" ")
		}
		if x >= 296 && x < 389 {
			return e.Erase()
		}
	}
	if y >= 395 && y < 445 && x >= 137 && x < 344 {
		e.Save()
		return m.finishEditor(true)
	}
	return false
}
func (m *Model) editorLetter(keys string, idx int) bool {
	if idx < 0 || idx >= len(keys) {
		return false
	}
	k := string(keys[idx])
	if !m.Editor.Upper {
		k = strings.ToLower(k)
	}
	return m.Editor.Insert(k)
}
func (m *Model) RotateSupplement(delta int) (bool, string, int) {
	switch m.Page {
	case TextEditor:
		if m.Editor.Visible {
			m.Editor.Rotate(delta)
		} else {
			m.EditorFocus = max(0, min(2, m.EditorFocus+delta))
		}
		return true, "", 0
	case Accessibility:
		m.AccessFocus = max(0, min(3, m.AccessFocus+delta))
		return true, "", 0
	case DiscardPrompt:
		m.DiscardFocus = max(0, min(1, m.DiscardFocus+delta))
		return true, "", 0
	}
	return false, "", 0
}
func (m *Model) ActivateSupplement() (bool, string) {
	switch m.Page {
	case TextEditor:
		if m.Editor.Visible {
			done, changed := m.Editor.ActivateWheel()
			if done {
				return m.finishEditor(true), ""
			}
			return changed, ""
		}
		switch m.EditorFocus {
		case 0:
			m.Editor.Show()
			return true, ""
		case 1:
			m.Editor.Save()
			return m.finishEditor(true), ""
		case 2:
			return m.EditorBack(), ""
		}
	case Accessibility:
		return m.ChangeAppearance(m.AccessFocus), ""
	case DiscardPrompt:
		return m.ResolveDiscard(m.DiscardFocus == 1), ""
	}
	return false, ""
}
func (m *Model) DrawSupplement(im *image.RGBA) {
	p := m.Appearance.Palette()
	bg, accent, fg, muted, selected := p.Background, p.Accent, p.PrimaryText, p.SecondaryText, p.Selected
	switch m.Page {
	case TextEditor:
		title := strings.ToUpper(m.EditorLabel)
		if len(title) > 13 {
			title = title[:13]
		}
		text(im, 240, 76, 3, title, accent, true)
		text(im, 122, 87, 2, "<", fg, true)
		rect(im, 100, 111, 380, 158, p.Surface)
		outlineRect(im, 100, 111, 380, 158, p.Border)
		content := []rune(m.Editor.Masked())
		cursor := m.Editor.Cursor
		if cursor > len(content) {
			cursor = len(content)
		}
		start := 0
		if len(content) > 17 {
			start = max(0, cursor-12)
			start = min(start, len(content)-17)
			content = content[start : start+17]
		}
		cursor -= start
		labelRunes := append([]rune(nil), content...)
		labelRunes = append(labelRunes[:cursor], append([]rune{'|'}, labelRunes[cursor:]...)...)
		label := strings.ToUpper(string(labelRunes))
		text(im, 240, 124, 2, label, fg, true)
		if !m.Editor.Visible {
			text(im, 240, 166, 2, "DRAFT SAVED IN MEMORY", muted, true)
			for i, label := range []string{"CONTINUE", "SAVE", "BACK"} {
				y := 200 + i*71
				rect(im, 112, y-16, 368, y+40, p.Surface)
				if m.EditorFocus == i {
					outlineRect(im, 110, y-18, 370, y+42, p.Focus)
				}
				text(im, 240, y, 2, label, fg, true)
			}
			return
		}
		text(im, 240, 98, 1, fmt.Sprintf("WHEEL %s : ROTATE / PRESS", m.Editor.WheelLabel()), muted, true)
		for i, g := range []string{"ABC", "123", "SYM"} {
			x := 95 + i*100
			rect(im, x, 164, x+93, 195, p.Surface)
			if m.Editor.Group == i {
				outlineRect(im, x, 164, x+93, 195, p.Focus)
			}
			text(im, x+46, 171, 2, g, fg, true)
		}
		rows := []struct {
			keys string
			x, y int
		}{{"QWERTYUIOP", 46, 200}, {"ASDFGHJKL", 66, 250}, {"ZXCVBNM", 105, 299}}
		for _, row := range rows {
			for i, k := range row.keys {
				x := row.x + i*39
				selectedWheel := m.Editor.WheelLabel() == string(k)
				keyBg := p.Surface
				if selectedWheel {
					keyBg = selected
				}
				rect(im, x, row.y, x+36, row.y+41, keyBg)
				if selectedWheel {
					outlineRect(im, x, row.y, x+36, row.y+41, p.Focus)
				}
				text(im, x+18, row.y+11, 2, string(k), fg, true)
			}
		}
		shiftColor := p.Surface
		if m.Editor.Upper {
			shiftColor = p.Selected
		}
		rect(im, 95, 348, 158, 392, shiftColor)
		if m.Editor.Upper {
			outlineRect(im, 95, 348, 158, 392, p.Focus)
		}
		text(im, 126, 361, 2, "SHIFT", fg, true)
		rect(im, 162, 348, 294, 392, p.Surface)
		text(im, 228, 361, 2, "SPACE", fg, true)
		rect(im, 296, 348, 389, 392, p.Surface)
		text(im, 342, 361, 2, "BKSP", fg, true)
		rect(im, 138, 399, 342, 442, accent)
		text(im, 240, 410, 3, "DONE", p.OnAccent, true)

	case Accessibility:
		text(im, 240, 94, 3, "ACCESSIBILITY", accent, true)
		text(im, 118, 116, 2, "<", fg, true)
		values := []string{strings.ToUpper(m.Appearance.Theme), fmt.Sprintf("%d%%", m.Appearance.TextScale), boolWord(m.Appearance.ReduceMotion), boolWord(m.Appearance.LargeTargets)}
		labels := []string{"THEME", "TEXT SCALE", "REDUCE MOTION", "LARGE TARGETS"}
		start, visible, top, step, height := accessGeometry(m)
		for i := start; i < len(labels) && i < start+visible; i++ {
			y := top + (i-start)*step
			rect(im, 93, y, 387, y+height, p.Surface)
			if i == m.AccessFocus {
				outlineRect(im, 91, y-2, 389, y+height+2, p.Focus)
			}
			sz := 2
			if m.Appearance.TextScale == 150 {
				sz = 3
			}
			text(im, 240, y+9, sz, labels[i], fg, true)
			text(im, 240, y+height-27, 2, values[i], accent, true)
		}
		text(im, 240, 430, 1, "ROTATE / PRESS TO CHANGE", muted, true)
	case DiscardPrompt:
		disc(im, 240, 240, 166, bg)
		disc(im, 240, 240, 153, p.Surface)
		text(im, 240, 180, 3, "UNSAVED TEXT", fg, true)
		text(im, 240, 215, 2, "DISCARD CHANGES?", muted, true)
		for i, label := range []string{"KEEP", "DISCARD"} {
			x := 100 + i*146
			rect(im, x, 270, x+137, 328, p.Selected)
			if m.DiscardFocus == i {
				outlineRect(im, x, 270, x+137, 328, p.Focus)
			}
			text(im, x+68, 290, 2, label, fg, true)
		}
	}
}
func boolWord(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

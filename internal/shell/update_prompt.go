package shell

import (
	"fmt"
	"image"
	"strings"
	"unicode"

	"roundos/internal/update"
)

const (
	UpdatePrompt = UpdatesPage + 1
	ReleaseNotes = UpdatesPage + 2
)

// notesDocument is immutable after creation and may be shared with the renderer.
type notesDocument struct {
	columns int
	lines   []string
}

func copyUpdateRelease(r *update.Release) *update.Release {
	if r == nil {
		return nil
	}
	copy := *r
	copy.Assets = append([]update.Asset(nil), r.Assets...)
	return &copy
}

func (m *Model) observeUpdateResult(status update.Status) {
	// Busy describes the next check; Result/Manual/Sequence still identify
	// the last completed one, even when its idle snapshot was coalesced.
	if status.Sequence <= m.Updates.SeenSequence {
		return
	}
	m.Updates.SeenSequence = status.Sequence
	if status.Err == nil && (status.Result.Status == update.UpToDate || status.Result.Status == update.NoStableRelease) {
		m.Updates.PendingRelease = nil
	}
	if status.Err != nil || status.Result.Status != update.Available || status.Result.Release == nil {
		return
	}
	r := status.Result.Release
	// A successful discovery supersedes any notice queued by an older check.
	// The currently visible release remains pinned until the user closes it.
	m.Updates.PendingRelease = nil
	if !status.Manual && status.IgnoredVersion == r.Version {
		return
	}
	if m.updatePromptInNavigation() && m.Updates.Notice != nil && m.Updates.Notice.Version == r.Version {
		return // never replace the release currently being reviewed
	}
	m.Updates.PendingRelease = copyUpdateRelease(r)
	m.Updates.PendingManual = status.Manual
}

// MaybePromptUpdate is called from the UI event loop after navigation. Deferring
// editors, pickers, app dialogs and critical alerts preserves their interaction.
func (m *Model) MaybePromptUpdate() bool {
	if m.Updates.PendingRelease == nil || m.Page == TextEditor || m.Page == DiscardPrompt || m.Page == WiFiConfirm ||
		m.Page == Accessibility || m.Page == Switcher || m.Page == NotificationCenter || m.Page == NotificationDetail ||
		m.updatePromptInNavigation() || m.PickerID != "" || m.hasVisibleDialog() || m.HasCritical() || m.Banner != nil || m.EditingWidget != "" {
		return false
	}
	if !m.Updates.PendingManual && m.Updates.Status.IgnoredVersion == m.Updates.PendingRelease.Version {
		m.Updates.PendingRelease = nil
		return false
	}
	m.Updates.ReturnPage = m.Page
	m.Updates.Notice = m.Updates.PendingRelease
	m.Updates.PendingRelease = nil
	m.Updates.PromptFocus = 0 // Cancel is the safe rotary default.
	m.Updates.NotesOffset = 0
	m.Updates.Notes = nil
	m.Updates.Message = ""
	return m.Select(UpdatePrompt)
}

func (m *Model) updatePromptInNavigation() bool {
	page, depth := m.Page, len(m.ShellParents)
	seen := make(map[[2]int]bool)
	for {
		if page == UpdatePrompt || page == ReleaseNotes {
			return true
		}
		key := [2]int{page, depth}
		if seen[key] {
			return false
		}
		seen[key] = true
		switch page {
		case NotificationDetail:
			page = NotificationCenter
		case NotificationCenter, Switcher:
			if depth == 0 {
				return false
			}
			depth--
			page = m.ShellParents[depth]
		case Accessibility:
			page = m.AccessParent
		case TextEditor, DiscardPrompt:
			page = m.EditorParent
		default:
			return false
		}
	}
}

// Retire an acknowledged Ignore even when a system overlay is in front of it.
// Keep that overlay open, but remove the ignored prompt from its return path.
func (m *Model) retireUpdatePrompt() {
	for i, page := range m.ShellParents {
		if page == UpdatePrompt || page == ReleaseNotes {
			m.ShellParents[i] = m.Updates.ReturnPage
		}
	}
	if m.AccessParent == UpdatePrompt || m.AccessParent == ReleaseNotes {
		m.AccessParent = m.Updates.ReturnPage
	}
	if m.EditorParent == UpdatePrompt || m.EditorParent == ReleaseNotes {
		m.EditorParent = m.Updates.ReturnPage
	}
	m.DismissUpdate()
}

func (m *Model) DismissUpdate() bool {
	if m.Page != UpdatePrompt && m.Page != ReleaseNotes {
		return false
	}
	return m.Select(m.Updates.ReturnPage)
}

func (m *Model) UpdatePromptRotate(delta int) bool {
	next := max(0, min(3, m.Updates.PromptFocus+delta))
	if next == m.Updates.PromptFocus {
		return false
	}
	m.Updates.PromptFocus = next
	return true
}

func (m *Model) UpdatePromptActivate() bool {
	if m.Updates.Notice == nil {
		return m.DismissUpdate()
	}
	switch m.Updates.PromptFocus {
	case 0:
		return m.DismissUpdate()
	case 1:
		if !m.installationAllowed() {
			m.Updates.Message = m.installationDisabledReason()
			return true
		}
		m.Updates.NextID++
		m.Updates.PendingInstall = &update.InstallCommand{ID: m.Updates.NextID, Release: *copyUpdateRelease(m.Updates.Notice)}
		m.Updates.InstallInFlight = m.Updates.NextID
		m.Updates.InstallRequestVersion = m.Updates.Notice.Version
		m.Updates.InstallQueueError = false
		m.Updates.Message = ""
		return true
	case 2:
		m.ReflowReleaseNotes()
		return m.Select(ReleaseNotes)
	case 3:
		if m.Updates.InFlight != 0 {
			return false
		}
		m.Updates.NextID++
		command := update.Command{ID: m.Updates.NextID, Kind: update.IgnoreRelease, Version: m.Updates.Notice.Version}
		m.Updates.Pending = &command
		m.Updates.InFlight = command.ID
		m.Updates.IgnoreRequestID = command.ID
		m.Updates.IgnoreRequestVersion = command.Version
		return true
	}
	return false
}

func (m *Model) UpdatePromptTap(x, y int) bool {
	if x < 93 || x > 387 {
		return false
	}
	for i := 0; i < 4; i++ {
		top := 165 + i*57
		if y >= top && y < top+49 {
			m.Updates.PromptFocus = i
			return m.UpdatePromptActivate()
		}
	}
	return false
}

func (m *Model) notesMetrics() (scale, step, visible, columns int) {
	scale = 2
	if m.Appearance.TextScale >= 125 {
		scale = 3
	}
	step = scale*7 + 5
	return scale, step, 200 / step, 288 / (scale * 6)
}

func (m *Model) ReflowReleaseNotes() {
	if m.Updates.Notice == nil {
		return
	}
	_, _, visible, columns := m.notesMetrics()
	if m.Updates.Notes == nil || m.Updates.Notes.columns != columns {
		m.Updates.Notes = &notesDocument{columns: columns, lines: wrapReleaseNotes(m.Updates.Notice.Notes, columns)}
	}
	m.Updates.NotesOffset = max(0, min(m.Updates.NotesOffset, len(m.Updates.Notes.lines)-visible))
}

func wrapReleaseNotes(body string, columns int) []string {
	if strings.TrimSpace(body) == "" {
		body = "No release notes provided."
	}
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.Map(func(r rune) rune {
		switch r {
		case '\r':
			return '\n'
		case '•':
			return '*'
		case '–', '—':
			return '-'
		case '‘', '’':
			return '\''
		case '“', '”':
			return '"'
		}
		if r == '\n' || r == '\t' {
			return r
		}
		r = unicode.ToUpper(r)
		if _, ok := glyph[r]; !ok {
			return '?'
		}
		return r
	}, body)
	var lines []string
	for _, paragraph := range strings.Split(body, "\n") {
		var line []rune
		for _, word := range strings.Fields(paragraph) {
			runes := []rune(word)
			if len(line) != 0 && len(line)+1+len(runes) > columns {
				lines = append(lines, string(line))
				line = nil
			}
			for len(runes) > columns {
				lines = append(lines, string(runes[:columns]))
				runes = runes[columns:]
			}
			if len(line) != 0 {
				line = append(line, ' ')
			}
			line = append(line, runes...)
		}
		lines = append(lines, string(line))
	}
	return lines
}

func (m *Model) ReleaseNotesMove(delta int) bool {
	if m.Page != ReleaseNotes || m.Updates.Notes == nil {
		return false
	}
	_, _, visible, _ := m.notesMetrics()
	next := max(0, min(m.Updates.NotesOffset+delta, len(m.Updates.Notes.lines)-visible))
	if next == m.Updates.NotesOffset {
		return false
	}
	m.Updates.NotesOffset = next
	return true
}

func (m *Model) ReleaseNotesDragStart(x, y int) bool {
	m.Updates.NotesDragging = false
	if m.Page != ReleaseNotes || x < 93 || x > 387 || y < 143 || y > 354 {
		return false
	}
	m.Updates.NotesDragging = true
	m.Updates.NotesDragOrigin = m.Updates.NotesOffset
	return true
}

func (m *Model) ReleaseNotesDragMove(dy float64) bool {
	if !m.Updates.NotesDragging || m.Page != ReleaseNotes {
		return false
	}
	_, step, _, _ := m.notesMetrics()
	delta := m.Updates.NotesDragOrigin - int(dy/float64(step)) - m.Updates.NotesOffset
	return m.ReleaseNotesMove(delta)
}

func (m *Model) DrawUpdatePrompt(im *image.RGBA) {
	p := m.Appearance.Palette()
	textTheme(im, 240, 82, 3, "UPDATE AVAILABLE", p.Accent, true, m.Appearance)
	if m.Updates.Notice != nil {
		textTheme(im, 240, 116, 2, strings.ToUpper(m.Updates.Notice.Version), p.PrimaryText, true, m.Appearance)
	}
	message := m.updatePromptMessage()
	if message != "" {
		textTheme(im, 240, 143, 1, message, p.Warning, true, m.Appearance)
	}
	labels := []string{"CANCEL", "INSTALL", "RELEASE NOTES", "IGNORE RELEASE"}
	details := []string{"REMIND AT THE NEXT CHECK", "MANAGED DEVICE REQUIRED", "READ WHAT CHANGED", "SKIP THIS VERSION"}
	if !m.Updates.Installation.Ready {
		details[1] = "CHECKING SUPPORT"
	}
	if m.Updates.Installation.Status.Phase == "recovery_required" {
		details[1] = "SSH RECOVERY REQUIRED"
	}
	if m.Updates.Installation.Status.InProgress() || m.Updates.InstallInFlight != 0 {
		labels[0] = "CLOSE"
		details[0] = "UPDATE WILL CONTINUE"
		details[1] = "UPDATE IN PROGRESS"
	}
	if m.installationAllowed() {
		details[1] = "ROUNDOS WILL RESTART"
	}
	for i, label := range labels {
		y := 165 + i*57
		rect(im, 93, y, 387, y+49, p.Surface)
		if m.Updates.PromptFocus == i {
			outlineRect(im, 91, y-2, 389, y+51, p.Focus)
		}
		color := p.PrimaryText
		if (i == 1 && !m.installationAllowed()) || (i == 3 && m.Updates.InFlight != 0) {
			color = p.Disabled
		}
		textTheme(im, 240, y+6, 2, label, color, true, m.Appearance)
		textTheme(im, 240, y+30, 1, details[i], p.SecondaryText, true, m.Appearance)
	}
}

func (m *Model) updatePromptMessage() string {
	if m.Updates.Notice != nil && m.Updates.InstallRequestVersion == m.Updates.Notice.Version {
		if message := m.installationMessage(); message != "" {
			return message
		}
	}
	return m.Updates.Message
}

func (m *Model) DrawReleaseNotes(im *image.RGBA) {
	p := m.Appearance.Palette()
	textTheme(im, 240, 78, 2, "RELEASE NOTES", p.Accent, true, m.Appearance)
	if m.Updates.Notice != nil {
		textTheme(im, 240, 112, 2, strings.ToUpper(m.Updates.Notice.Version), p.PrimaryText, true, m.Appearance)
	}
	if m.Updates.Notes == nil {
		return
	}
	scale, step, visible, _ := m.notesMetrics()
	lines := m.Updates.Notes.lines
	start := max(0, min(m.Updates.NotesOffset, len(lines)-visible))
	end := min(len(lines), start+visible)
	for i := start; i < end; i++ {
		text(im, 96, 150+(i-start)*step, scale, lines[i], p.PrimaryText, false)
	}
	textTheme(im, 240, 365, 1, fmt.Sprintf("%d-%d OF %d  SCROLL TO READ", start+1, end, len(lines)), p.SecondaryText, true, m.Appearance)
	rect(im, 150, 389, 330, 433, p.Surface)
	outlineRect(im, 148, 387, 332, 435, p.Focus)
	textTheme(im, 240, 400, 2, "CLOSE", p.PrimaryText, true, m.Appearance)
}

func (m *Model) releaseNotesTap(x, y int) bool {
	if x >= 150 && x <= 330 && y >= 389 && y <= 433 {
		return m.Select(UpdatePrompt)
	}
	return false
}

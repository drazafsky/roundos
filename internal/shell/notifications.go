package shell

import (
	"roundos/internal/notify"
	"time"
)

const (
	NotificationCenter = Diagnostics + 1
	Switcher           = Diagnostics + 2
	NotificationDetail = Diagnostics + 3
)

// The single UI event loop owns the Store; immutable Model snapshots contain
// only copied Items and never read the pointer from the rendering goroutine.
func (m *Model) UpdateNotices(items []notify.Item, result notify.Result, now time.Time) bool {
	selectedID := uint64(0)
	if m.NotificationFocus >= 0 && m.NotificationFocus < len(m.Notices) {
		selectedID = m.Notices[m.NotificationFocus].ID
	}
	m.Notices = append([]notify.Item(nil), items...)
	if selectedID != 0 {
		for i, item := range m.Notices {
			if item.ID == selectedID {
				m.NotificationFocus = i
				break
			}
		}
	}
	if result.Presentation == notify.Banner {
		item := result.Item
		m.Banner = &item
		m.BannerUntil = now.Add(4 * time.Second)
	} else if result.Presentation == notify.Modal {
		item := result.Item
		m.Critical = &item
	}
	if m.NotificationFocus >= len(m.Notices) && m.NotificationFocus > 0 {
		m.NotificationFocus = len(m.Notices) - 1
	}
	return true
}
func (m *Model) BannerExpired(now time.Time) bool {
	if m.Banner == nil || now.Before(m.BannerUntil) {
		return false
	}
	m.Banner = nil
	return true
}
func (m *Model) OpenNotifications() bool {
	if m.Page == NotificationCenter {
		return false
	}
	m.ShellParents = append(m.ShellParents, m.Page)
	m.Select(NotificationCenter)
	m.NotificationFocus = 0
	m.NotificationDetailFocus = 0
	m.Banner = nil
	return true
}
func (m *Model) OpenSwitcher() bool {
	if m.Page == Switcher {
		return false
	}
	m.ShellParents = append(m.ShellParents, m.Page)
	m.Select(Switcher)
	m.SwitcherFocus = 0
	return true
}
func (m *Model) Back() bool {
	if m.Page == TextEditor || m.Page == DiscardPrompt {
		return m.EditorBack()
	}
	if m.Page == Accessibility {
		return m.Select(m.AccessParent)
	}
	if m.Page == WiFiConfirm {
		return m.Select(WiFiPage)
	}
	if m.Page == WiFiPage {
		return m.Select(Settings)
	}
	if m.Page == Settings {
		return m.Select(Home)
	}
	if m.Critical != nil {
		return false
	}
	if m.PickerID != "" {
		m.cancelPicker()
		return true
	}
	if m.hasVisibleDialog() {
		return m.dismissDialog(false)
	}
	if m.Page == NotificationDetail {
		return m.Select(NotificationCenter)
	}
	if m.Page == NotificationCenter || m.Page == Switcher {
		restore := Home
		if len(m.ShellParents) > 0 {
			restore = m.ShellParents[len(m.ShellParents)-1]
			m.ShellParents = m.ShellParents[:len(m.ShellParents)-1]
		}
		return m.Select(restore)
	}
	if m.EditingWidget != "" {
		m.cancelWidgetEdit()
		return true
	}
	if m.backAppScreen() {
		return true
	}
	return m.Select(Home)
}
func (m *Model) AcknowledgeCritical() bool {
	if m.Critical == nil {
		return false
	}
	m.Critical = nil
	return true
}
func (m *Model) NextRunning(delta int) bool {
	apps := m.SwitcherItems()
	if len(apps) < 2 {
		return false
	}
	current := -1
	for i, a := range apps {
		if a.Page == m.Page {
			current = i
			break
		}
	}
	if current < 0 {
		return false
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	n := (current + dir + len(apps)) % len(apps)
	return m.Select(apps[n].Page)
}
func (m *Model) OpenNotificationDetail() bool {
	if m.NotificationFocus < 0 || m.NotificationFocus >= len(m.Notices) {
		return false
	}
	m.NotificationDetailFocus = 0
	return m.Select(NotificationDetail)
}
func (m *Model) ActivateNotification() bool {
	item, ok := m.SelectedNotification()
	if !ok {
		return false
	}
	if m.Notifier != nil {
		m.Notifier.MarkRead(item.ID)
	}
	page := m.PageForApp(item.App)
	if page == Home {
		return false
	}
	return m.Select(page)
}
func (m *Model) DismissNotification() bool {
	item, ok := m.SelectedNotification()
	if !ok {
		return false
	}
	if m.Notifier != nil {
		m.Notifier.Dismiss(item.ID)
		m.Notices = m.Notifier.Items()
	} else {
		m.Notices = append(m.Notices[:m.NotificationFocus], m.Notices[m.NotificationFocus+1:]...)
	}
	if m.NotificationFocus >= len(m.Notices) && m.NotificationFocus > 0 {
		m.NotificationFocus--
	}
	if m.Banner != nil && m.Banner.ID == item.ID {
		m.Banner = nil
	}
	return m.Select(NotificationCenter)
}
func (m *Model) DetailActivate() bool {
	switch m.NotificationDetailFocus {
	case 0:
		return m.ActivateNotification()
	case 1:
		return m.DismissNotification()
	case 2:
		return m.Select(NotificationCenter)
	}
	return false
}
func (m *Model) SelectSwitcher() bool {
	if m.SwitcherFocus >= 7 {
		i := m.SwitcherFocus - 7 + 3
		if i < len(m.AppCards) {
			return m.Select(m.AppCards[i].Page)
		}
		return false
	}
	switch m.SwitcherFocus {
	case 0:
		return m.Select(Clock)
	case 1:
		return m.Select(Timer)
	case 2:
		return m.OpenNotifications()
	case 3:
		return m.OpenEditor()
	case 4:
		return m.OpenAccessibility()
	case 5:
		return m.Select(Home)
	case 6:
		return m.OpenSettings()
	}
	return false
}
func (m *Model) SelectedNotification() (notify.Item, bool) {
	if m.NotificationFocus < 0 || m.NotificationFocus >= len(m.Notices) {
		return notify.Item{}, false
	}
	return m.Notices[m.NotificationFocus], true
}
func (m *Model) HasCritical() bool { return m.Critical != nil }
func (m *Model) NotificationSourceApp() string {
	item, ok := m.SelectedNotification()
	if !ok {
		return ""
	}
	if m.PageForApp(item.App) != Home {
		return item.App
	}
	return ""
}

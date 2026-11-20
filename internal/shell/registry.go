package shell

import (
	"roundos/internal/appmgr"
	"roundos/internal/wire"
	"strings"
)

const extraPageBase = 1000

// AppCard is a trusted installed app entry in the focused launcher.
type AppCard struct {
	Page            int
	ID, Title, Icon string
}

func (m *Model) SetInstalledApps(specs []appmgr.AppSpec) {
	m.AppCards = []AppCard{{Clock, wire.ClockID, "CLOCK", "CLK"}, {Timer, wire.TimerID, "TIMER", "TMR"}, {Settings, "", "SETTINGS", "CFG"}}
	m.AppPages = map[string]int{wire.ClockID: Clock, wire.TimerID: Timer}
	m.PageApps = map[int]string{Clock: wire.ClockID, Timer: wire.TimerID}
	for i, app := range specs {
		page := extraPageBase + i
		m.AppCards = append(m.AppCards, AppCard{Page: page, ID: app.ID, Title: strings.ToUpper(app.Name), Icon: app.Icon})
		m.AppPages[app.ID] = page
		m.PageApps[page] = app.ID
		m.Status[page] = "WAITING FOR APP"
	}
	if m.Selected >= len(m.AppCards) {
		m.Selected = 0
	}
}
func (m *Model) AppID(page int) string {
	if id := m.PageApps[page]; id != "" {
		return id
	}
	return AppID(page)
}
func (m *Model) PageForApp(id string) int {
	if p, ok := m.AppPages[id]; ok {
		return p
	}
	return PageForApp(id)
}
func (m *Model) IsAppPage(page int) bool { return m.AppID(page) != "" }
func (m *Model) CarouselCount() int {
	if len(m.AppCards) < 3 {
		return 3
	}
	return len(m.AppCards)
}
func (m *Model) SelectCarousel(i int) bool {
	if i < 0 || i >= m.CarouselCount() {
		return false
	}
	if i == 2 {
		return m.OpenSettings()
	}
	return m.Select(m.AppCards[i].Page)
}
func (m *Model) SwitcherItems() []AppCard {
	out := []AppCard{{Clock, wire.ClockID, "CLOCK", "CLK"}, {Timer, wire.TimerID, "TIMER", "TMR"}}
	for _, a := range m.AppCards[3:] {
		out = append(out, a)
	}
	return out
}
func (m *Model) SwitcherEntries() []string {
	items := []string{"CLOCK", "TIMER", "NOTIFICATIONS", "TEXT EDIT", "ACCESSIBILITY", "HOME", "SETTINGS"}
	for _, a := range m.AppCards[3:] {
		items = append(items, a.Title)
	}
	return items
}

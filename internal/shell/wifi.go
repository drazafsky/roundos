package shell

import (
	"fmt"
	"image"
	"strings"

	"roundos/internal/textedit"
	"roundos/internal/wifi"
)

const (
	Settings    = DiscardPrompt + 1
	WiFiPage    = DiscardPrompt + 2
	WiFiConfirm = DiscardPrompt + 3
)

type WiFiRequest struct {
	Kind, Interface string
	Network         wifi.Network
	Password        string
}

type WiFiState struct {
	Interface     string
	Current       string
	Networks      []wifi.Network
	Busy          bool
	Message       string
	Focus         int
	ConfirmFocus  int
	SettingsFocus int
	Candidate     wifi.Network
	Pending       *WiFiRequest
	EditorPurpose string
}

func (m *Model) OpenSettings() bool {
	if m.Page == Settings {
		return false
	}
	m.WiFi.SettingsFocus = 0
	return m.Select(Settings)
}
func (m *Model) OpenWiFi() bool {
	if m.Page == WiFiPage {
		return false
	}
	if !m.Select(WiFiPage) {
		return false
	}
	m.WiFi.Focus = 0
	m.WiFi.Message = "SCANNING..."
	m.WiFi.Busy = true
	m.WiFi.Pending = &WiFiRequest{Kind: "scan"}
	return true
}
func (m *Model) WiFiRefresh() bool {
	if m.WiFi.Busy {
		return false
	}
	m.WiFi.Busy = true
	m.WiFi.Message = "SCANNING..."
	m.WiFi.Pending = &WiFiRequest{Kind: "scan"}
	return true
}
func (m *Model) TakeWiFiRequest() *WiFiRequest {
	req := m.WiFi.Pending
	m.WiFi.Pending = nil
	return req
}
func (m *Model) WiFiResult(status wifi.Status, kind string, err error) bool {
	m.WiFi.Busy = false
	if err != nil {
		// Never expose nmcli command output on screen: it may contain identifiers.
		m.WiFi.Message = "WI-FI FAILED - CHECK LOG"
		return true
	}
	if kind == "scan" {
		m.WiFi.Interface = status.Interface
		m.WiFi.Current = status.Current
		m.WiFi.Networks = append([]wifi.Network(nil), status.Networks...)
		m.WiFi.Focus = min(m.WiFi.Focus, len(m.WiFi.Networks)+1)
		m.WiFi.Message = fmt.Sprintf("%d NETWORKS FOUND", len(status.Networks))
	} else {
		m.WiFi.Message = "NETWORK UPDATED"
		m.WiFi.Pending = &WiFiRequest{Kind: "scan"}
		m.WiFi.Busy = true
	}
	return true
}
func (m *Model) WiFiFocusMove(delta int) bool {
	next := max(0, min(len(m.WiFi.Networks)+1, m.WiFi.Focus+delta))
	if next == m.WiFi.Focus {
		return false
	}
	m.WiFi.Focus = next
	return true
}
func (m *Model) WiFiSwipe(dir string) bool {
	if m.Page != WiFiPage {
		return false
	}
	if dir == "up" {
		return m.WiFiFocusMove(2)
	}
	if dir == "down" {
		return m.WiFiFocusMove(-2)
	}
	return false
}
func (m *Model) SettingsRotate(delta int) bool {
	if m.Page == WiFiConfirm {
		next := max(0, min(1, m.WiFi.ConfirmFocus+delta))
		if next == m.WiFi.ConfirmFocus {
			return false
		}
		m.WiFi.ConfirmFocus = next
		return true
	}
	if m.Page == WiFiPage {
		return m.WiFiFocusMove(delta)
	}
	next := max(0, min(3, m.WiFi.SettingsFocus+delta))
	if next == m.WiFi.SettingsFocus {
		return false
	}
	m.WiFi.SettingsFocus = next
	return true
}
func (m *Model) SettingsActivate() bool {
	switch m.Page {
	case Settings:
		switch m.WiFi.SettingsFocus {
		case 0:
			return m.OpenWiFi()
		case 1:
			return m.OpenAccessibility()
		case 2:
			return m.OpenUpdates()
		case 3:
			return m.Select(Home)
		}
	case WiFiPage:
		if m.WiFi.Focus == 0 {
			return m.WiFiRefresh()
		}
		if m.WiFi.Focus == len(m.WiFi.Networks)+1 {
			return m.Select(Settings)
		}
		i := m.WiFi.Focus - 1
		if i < 0 || i >= len(m.WiFi.Networks) || m.WiFi.Busy {
			return false
		}
		m.WiFi.Candidate = m.WiFi.Networks[i]
		m.WiFi.ConfirmFocus = 0 // CANCEL is the safe rotary default
		return m.Select(WiFiConfirm)
	case WiFiConfirm:
		if m.WiFi.ConfirmFocus == 0 {
			return m.Select(WiFiPage)
		}
		if m.WiFi.Candidate.Connected {
			m.WiFi.Pending = &WiFiRequest{Kind: "disconnect", Interface: m.WiFi.Interface}
			m.WiFi.Busy = true
			m.WiFi.Message = "DISCONNECTING..."
			return m.Select(WiFiPage)
		}
		if m.WiFi.Candidate.Security != "" && m.WiFi.Candidate.Security != "--" {
			if strings.Contains(strings.ToUpper(m.WiFi.Candidate.Security), "802.1X") || strings.Contains(strings.ToUpper(m.WiFi.Candidate.Security), "EAP") {
				m.WiFi.Message = "ENTERPRISE WI-FI UNSUPPORTED"
				return m.Select(WiFiPage)
			}
			m.EditorParent = WiFiPage
			m.EditorAppID = ""
			m.EditorRequestID = ""
			m.EditorLabel = "Wi-Fi password"
			m.Editor = textedit.New("", true)
			m.WiFi.EditorPurpose = "wifi-password"
			return m.Select(TextEditor)
		}
		m.WiFi.Pending = &WiFiRequest{Kind: "connect", Interface: m.WiFi.Interface, Network: m.WiFi.Candidate}
		m.WiFi.Busy = true
		m.WiFi.Message = "CONNECTING..."
		return m.Select(WiFiPage)
	}
	return false
}
func (m *Model) WiFiEditorFinish(saved bool) {
	if m.WiFi.EditorPurpose != "wifi-password" {
		return
	}
	if saved {
		m.WiFi.Pending = &WiFiRequest{Kind: "connect", Interface: m.WiFi.Interface, Network: m.WiFi.Candidate, Password: m.Editor.Committed}
		m.WiFi.Busy = true
		m.WiFi.Message = "CONNECTING..."
	}
	m.WiFi.EditorPurpose = ""
}
func (m *Model) SettingsTap(x, y int) bool {
	if y < 143 && x < 155 {
		return m.Back()
	}
	if m.Page == Settings {
		for i := 0; i < 4; i++ {
			top := 146 + i*61
			if y >= top && y < top+53 {
				m.WiFi.SettingsFocus = i
				return m.SettingsActivate()
			}
		}
	} else if m.Page == WiFiPage {
		start := max(0, m.WiFi.Focus-2)
		total := len(m.WiFi.Networks) + 2
		for i := start; i < total && i < start+3; i++ {
			top := 159 + (i-start)*76
			if y >= top && y < top+67 {
				m.WiFi.Focus = i
				return m.SettingsActivate()
			}
		}
		if y > 403 {
			return m.Select(Settings)
		}
	} else if m.Page == WiFiConfirm {
		if y >= 274 && y <= 352 {
			if x < 240 {
				m.WiFi.ConfirmFocus = 0
			} else {
				m.WiFi.ConfirmFocus = 1
			}
			return m.SettingsActivate()
		}
	}
	return false
}
func wifiLabel(s string, limit int) string {
	r := []rune(strings.ToUpper(s))
	if len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return string(r)
}
func (m *Model) DrawSettings(im *image.RGBA) {
	p := m.Appearance.Palette()
	switch m.Page {
	case Settings:
		textTheme(im, 240, 99, 3, "SETTINGS", p.Accent, true, m.Appearance)
		textTheme(im, 118, 123, 2, "<", p.PrimaryText, true, m.Appearance)
		labels := []string{"WI-FI NETWORKS", "APPEARANCE", "UPDATES", "HOME"}
		sub := []string{"SCAN AND CONNECT", "THEME AND ACCESS", "CHECK FOR NEW RELEASES", "BACK TO LAUNCHER"}
		for i, label := range labels {
			y := 146 + i*61
			rect(im, 93, y, 387, y+53, p.Surface)
			if m.WiFi.SettingsFocus == i {
				outlineRect(im, 91, y-2, 389, y+55, p.Focus)
			}
			textTheme(im, 240, y+7, 2, label, p.PrimaryText, true, m.Appearance)
			textTheme(im, 240, y+33, 1, sub[i], p.SecondaryText, true, m.Appearance)
		}
	case WiFiPage:
		textTheme(im, 240, 95, 3, "WI-FI", p.Accent, true, m.Appearance)
		textTheme(im, 112, 120, 2, "<", p.PrimaryText, true, m.Appearance)
		status := "NOT CONNECTED"
		if m.WiFi.Current != "" {
			status = "ON: " + wifiLabel(m.WiFi.Current, 17)
		}
		textTheme(im, 240, 130, 2, status, p.SecondaryText, true, m.Appearance)
		total := len(m.WiFi.Networks) + 2
		start := max(0, m.WiFi.Focus-2)
		for i := start; i < total && i < start+3; i++ {
			title := "SCAN AGAIN"
			detail := "REFRESH NETWORKS"
			if i > 0 && i <= len(m.WiFi.Networks) {
				n := m.WiFi.Networks[i-1]
				title = wifiLabel(n.SSID, 20)
				detail = fmt.Sprintf("%d%%", n.Signal)
				if n.Security != "" && n.Security != "--" {
					detail += "  LOCKED"
				} else {
					detail += "  OPEN"
				}
				if n.Connected {
					detail = "CONNECTED  " + detail
				}
			} else if i == total-1 {
				title = "BACK"
				detail = "SETTINGS"
			}
			y := 159 + (i-start)*76
			rect(im, 93, y, 387, y+67, p.Surface)
			if m.WiFi.Focus == i {
				outlineRect(im, 91, y-2, 389, y+69, p.Focus)
			}
			textTheme(im, 240, y+10, 2, title, p.PrimaryText, true, m.Appearance)
			textTheme(im, 240, y+40, 1, detail, p.SecondaryText, true, m.Appearance)
		}
		msg := m.WiFi.Message
		if m.WiFi.Busy {
			msg = "PLEASE WAIT - " + msg
		}
		textTheme(im, 240, 423, 1, wifiLabel(msg, 30), p.SecondaryText, true, m.Appearance)
	case WiFiConfirm:
		disc(im, 240, 240, 162, p.Surface)
		textTheme(im, 240, 161, 3, "WI-FI NETWORK", p.Accent, true, m.Appearance)
		textTheme(im, 240, 205, 2, wifiLabel(m.WiFi.Candidate.SSID, 21), p.PrimaryText, true, m.Appearance)
		textTheme(im, 240, 238, 1, "SSH CONNECTION MAY DROP", p.SecondaryText, true, m.Appearance)
		label := "CONNECT"
		if m.WiFi.Candidate.Connected {
			label = "DISCONNECT"
		}
		for i, v := range []string{"CANCEL", label} {
			x := 96 + i*146
			rect(im, x, 280, x+138, 338, p.Selected)
			if m.WiFi.ConfirmFocus == i {
				outlineRect(im, x-2, 278, x+140, 340, p.Focus)
			}
			textTheme(im, x+69, 301, 2, v, p.PrimaryText, true, m.Appearance)
		}
	}
}

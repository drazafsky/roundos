package shell

import (
	"roundos/internal/wifi"
	"testing"
)

func TestSettingsCarouselAndWiFiScan(t *testing.T) {
	m := New()
	m.Selected = 2
	if !m.Press() || m.Page != Settings {
		t.Fatal("settings must be launchable")
	}
	if !m.SettingsActivate() || m.Page != WiFiPage {
		t.Fatal("Wi-Fi page from settings")
	}
	req := m.TakeWiFiRequest()
	if req == nil || req.Kind != "scan" {
		t.Fatalf("scan request %+v", req)
	}
	st := wifi.Status{Interface: "wlan0", Current: "Home", Networks: []wifi.Network{{SSID: "Home", Connected: true, Security: "WPA2", Signal: 95}, {SSID: "Guest", Signal: 50, Security: "--"}}}
	m.WiFiResult(st, "scan", nil)
	if m.WiFi.Current != "Home" || len(m.WiFi.Networks) != 2 {
		t.Fatalf("scan state: %+v", m.WiFi)
	}
	if !m.WiFiFocusMove(1) || !m.SettingsActivate() || m.Page != WiFiConfirm {
		t.Fatal("select current network")
	}
	if m.WiFi.ConfirmFocus != 0 {
		t.Fatal("cancel must be default")
	}
	if !m.SettingsActivate() || m.Page != WiFiPage {
		t.Fatal("default cancel")
	}
	if !m.WiFiSwipe("up") || m.WiFi.Focus != 3 {
		t.Fatal("swipe Wi-Fi list")
	}
	if !m.Back() || m.Page != Settings {
		t.Fatal("Wi-Fi Back")
	}
	if !m.Back() || m.Page != Home {
		t.Fatal("Settings Back")
	}
}
func TestWiFiPasswordSecureEditor(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.OpenWiFi()
	m.TakeWiFiRequest()
	m.WiFiResult(wifi.Status{Interface: "wlan0", Networks: []wifi.Network{{SSID: "Private", Security: "WPA2", Signal: 80}}}, "scan", nil)
	m.WiFi.Focus = 1
	if !m.SettingsActivate() || m.Page != WiFiConfirm {
		t.Fatal("confirm view")
	}
	m.WiFi.ConfirmFocus = 1
	if !m.SettingsActivate() || m.Page != TextEditor || !m.Editor.Secure {
		t.Fatal("secure editor required")
	}
	m.Editor.Insert("password123")
	m.Editor.Save()
	if !m.finishEditor(true) || m.Page != WiFiPage {
		t.Fatal("return to Wi-Fi")
	}
	if m.Editor.Draft != "" || m.Editor.Committed != "" {
		t.Fatal("secret retained in text field")
	}
	req := m.TakeWiFiRequest()
	if req == nil || req.Kind != "connect" || req.Password != "password123" {
		t.Fatal("secret not handed to network worker")
	}
	if m.Snapshot().WiFi.Pending != nil {
		t.Fatal("snapshot leaked operation")
	}
}
func TestWiFiSecuredBackDiscards(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.OpenWiFi()
	m.TakeWiFiRequest()
	m.WiFiResult(wifi.Status{Interface: "wlan0", Networks: []wifi.Network{{SSID: "Private", Security: "WPA2"}}}, "scan", nil)
	m.WiFi.Focus = 1
	m.SettingsActivate()
	m.WiFi.ConfirmFocus = 1
	m.SettingsActivate()
	m.Editor.Insert("secretpass")
	if !m.Back() || m.Editor.Visible {
		t.Fatal("first Back hides keyboard")
	}
	if !m.Back() || m.Page != DiscardPrompt {
		t.Fatal("second Back asks")
	}
	if !m.ResolveDiscard(true) || m.Page != WiFiPage {
		t.Fatal("discard returns to Wi-Fi")
	}
	if m.WiFi.Pending != nil || m.Editor.Draft != "" {
		t.Fatal("discard leaked password")
	}
}
func TestWiFiPasswordShortcutClearsPurpose(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.OpenWiFi()
	m.TakeWiFiRequest()
	m.WiFiResult(wifi.Status{Interface: "wlan0", Networks: []wifi.Network{{SSID: "Private", Security: "WPA2"}}}, "scan", nil)
	m.WiFi.Focus = 1
	m.SettingsActivate()
	m.WiFi.ConfirmFocus = 1
	m.SettingsActivate()
	m.Editor.Insert("secretpassword")
	if !m.Select(Home) {
		t.Fatal("Home should be reachable")
	}
	if m.WiFi.EditorPurpose != "" || m.Editor.Draft != "" {
		t.Fatal("secure editor should be cleared by shortcut")
	}
	if m.WiFi.Pending != nil {
		t.Fatal("shortcut must not connect")
	}
}
func TestSettingsAppearanceReturnsToSettings(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.SettingsRotate(1)
	if !m.SettingsActivate() || m.Page != Accessibility {
		t.Fatal("appearance from Settings")
	}
	if !m.Back() || m.Page != Settings {
		t.Fatal("Back must return to Settings")
	}
}

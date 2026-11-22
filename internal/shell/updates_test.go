package shell

import (
	"errors"
	"testing"
	"time"

	"roundos/internal/appearance"
	"roundos/internal/update"
)

func TestSettingsUpdatesNavigationAndPreferences(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.SettingsRotate(2)
	if !m.SettingsActivate() || m.Page != UpdatesPage {
		t.Fatal("Updates is not reachable from Settings")
	}
	if m.UpdatesActivate(time.Now()) || m.TakeUpdateRequest() != nil {
		t.Fatal("controls accepted input before settings loaded")
	}
	status := update.Status{Preferences: update.DefaultPreferences()}
	m.SetUpdateStatus(status)
	if !m.UpdatesActivate(time.Now()) {
		t.Fatal("toggle did not submit a change")
	}
	r := m.TakeUpdateRequest()
	if r == nil || r.Kind != update.ChangePreferences || r.Preferences.Automatic || r.Preferences.IntervalMinutes != 1440 {
		t.Fatalf("toggle request: %+v", r)
	}
	if m.Updates.Status.Preferences != update.DefaultPreferences() {
		t.Fatal("UI claimed preference saved before worker confirmation")
	}
	status.Preferences = r.Preferences
	status.LastCommandID = r.ID
	m.SetUpdateStatus(status)
	m.Updates.Focus = 1
	for _, want := range update.IntervalPresets() {
		if !m.UpdatesActivate(time.Now()) {
			t.Fatal("interval did not submit a change")
		}
		r = m.TakeUpdateRequest()
		if r == nil || r.Preferences.IntervalMinutes != want || r.Preferences.Automatic {
			t.Fatalf("interval request: %+v, want %d", r, want)
		}
		status.Preferences = r.Preferences
		status.LastCommandID = r.ID
		m.SetUpdateStatus(status)
	}
	if !m.Back() || m.Page != Settings || !m.Back() || m.Page != Home {
		t.Fatal("Updates Back navigation")
	}
}

func TestUpdatesManualCheckCooldownAndBusy(t *testing.T) {
	m := New()
	now := time.Now()
	status := update.Status{Preferences: update.Preferences{IntervalMinutes: 15}, NextManual: now.Add(15 * time.Minute)}
	m.SetUpdateStatus(status)
	m.OpenUpdates()
	m.Updates.Focus = 2
	if m.UpdatesActivate(now) || m.TakeUpdateRequest() != nil {
		t.Fatal("manual check bypassed cooldown")
	}
	if !m.UpdatesActivate(status.NextManual) {
		t.Fatal("manual check remained disabled at exact cooldown expiry")
	}
	r := m.TakeUpdateRequest()
	if r == nil || r.Kind != update.ManualCheck {
		t.Fatalf("manual request with automatic checking off: %+v", r)
	}
	status.Busy = true
	status.LastCommandID = r.ID
	m.SetUpdateStatus(status)
	if m.UpdatesActivate(now.Add(time.Hour)) || m.TakeUpdateRequest() != nil {
		t.Fatal("manual check bypassed busy guard")
	}
}

func TestUpdatesTouchRotaryAndBackControls(t *testing.T) {
	m := New()
	m.OpenSettings()
	if !m.SettingsTap(240, 146+2*61+20) || m.Page != UpdatesPage {
		t.Fatal("touch did not open Updates")
	}
	m.SetUpdateStatus(update.Status{Preferences: update.DefaultPreferences()})
	if changed, action := m.Tap(240, 218); !changed || action != "" {
		t.Fatal("touch did not activate toggle")
	}
	r := m.TakeUpdateRequest()
	if r == nil {
		t.Fatal("touch toggle request missing")
	}
	m.SetUpdateStatus(update.Status{Preferences: r.Preferences, LastCommandID: r.ID})
	if changed, _, _ := m.Rotate(2); !changed || m.Updates.Focus != 2 {
		t.Fatal("rotary did not focus manual check")
	}
	if changed, _ := m.Activate(); !changed || m.TakeUpdateRequest() == nil {
		t.Fatal("rotary did not activate manual check")
	}
	m.Rotate(1)
	if changed, _ := m.Activate(); !changed || m.Page != Settings {
		t.Fatal("rotary Back control")
	}
	m.OpenUpdates()
	if !m.UpdatesTap(118, 120) || m.Page != Settings {
		t.Fatal("touch Back control")
	}
}

func TestUpdatesMessagesAndSnapshot(t *testing.T) {
	m := New()
	m.OpenUpdates()
	for _, tc := range []struct {
		status update.Status
		want   string
	}{
		{update.Status{Err: errors.New("offline")}, "CHECK FAILED - TRY AGAIN LATER"},
		{update.Status{Err: update.ErrIncompatibleRelease}, "RELEASE HAS NO COMPATIBLE PACKAGE"},
		{update.Status{Err: &update.RateLimitError{}}, "GITHUB RATE LIMIT - PLEASE WAIT"},
		{update.Status{OperationErr: errors.New("disk full")}, "COULD NOT SAVE UPDATE SETTINGS"},
		{update.Status{Result: update.CheckResult{Status: update.UpToDate}}, "YOU ARE UP TO DATE"},
		{update.Status{Result: update.CheckResult{Status: update.NoStableRelease}}, "NO STABLE RELEASE PUBLISHED"},
	} {
		m.SetUpdateStatus(tc.status)
		if got := m.updateMessage(); got != tc.want {
			t.Fatalf("message %q, want %q", got, tc.want)
		}
	}
	m.UpdateRequestFailed()
	if m.updateMessage() != "UPDATE SERVICE BUSY - TRY AGAIN" {
		t.Fatal("queue failure not surfaced")
	}
	m.SetUpdateStatus(update.Status{Preferences: update.DefaultPreferences(), Result: update.CheckResult{Release: &update.Release{Assets: []update.Asset{{Name: "asset"}}}}})
	m.UpdatesActivate(time.Now())
	snapshot := m.Snapshot()
	if snapshot.Updates.Pending != nil {
		t.Fatal("renderer received pending update command")
	}
	snapshot.Updates.Status.Result.Release.Assets[0].Name = "changed"
	if m.Updates.Status.Result.Release.Assets[0].Name != "asset" {
		t.Fatal("snapshot aliases release assets")
	}
	for _, theme := range []string{appearance.Dark, appearance.Light, appearance.Contrast} {
		m.Appearance.Theme = theme
		m.Appearance.TextScale = 150
		m.Appearance.LargeTargets = true
		if im := Draw(m, time.Now()); im.Bounds().Dx() != 480 {
			t.Fatal("Updates did not render at device dimensions")
		}
	}
}

func TestUpdatesWaitsForMatchingCommandAcknowledgement(t *testing.T) {
	m := New()
	m.SetUpdateStatus(update.Status{Preferences: update.DefaultPreferences()})
	m.OpenUpdates()
	m.UpdatesActivate(time.Now())
	first := m.TakeUpdateRequest()
	m.Updates.Focus = 1
	if m.UpdatesActivate(time.Now()) || m.TakeUpdateRequest() != nil {
		t.Fatal("interval change overwrote an unconfirmed toggle")
	}
	// A concurrent check-status event cannot release the preference guard.
	m.SetUpdateStatus(update.Status{Preferences: update.DefaultPreferences(), Busy: true})
	if m.UpdatesActivate(time.Now()) {
		t.Fatal("unrelated status acknowledged the preference change")
	}
	m.SetUpdateStatus(update.Status{Preferences: first.Preferences, LastCommandID: first.ID})
	if !m.UpdatesActivate(time.Now()) {
		t.Fatal("acknowledged preference change did not release controls")
	}
	second := m.TakeUpdateRequest()
	if second == nil || second.ID == first.ID || second.Preferences.Automatic || second.Preferences.IntervalMinutes != 15 {
		t.Fatalf("confirmed toggle was lost: %+v", second)
	}
	m.UpdateRequestFailed()
	if !m.UpdatesActivate(time.Now()) {
		t.Fatal("queue rejection left controls blocked")
	}
}

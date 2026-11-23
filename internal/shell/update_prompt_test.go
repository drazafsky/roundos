package shell

import (
	"errors"
	"strings"
	"testing"
	"time"

	"roundos/internal/notify"
	"roundos/internal/update"
)

func availableStatus(sequence uint64, version, notes string, manual bool, ignored string) update.Status {
	return update.Status{Preferences: update.DefaultPreferences(), Sequence: sequence, Manual: manual, IgnoredVersion: ignored,
		Result: update.CheckResult{Status: update.Available, Release: &update.Release{ID: 123, Version: version, Notes: notes}}}
}

func TestUpdatePromptCancelAndRepeatOnNextCheck(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "Changes", false, ""))
	if !m.MaybePromptUpdate() || m.Page != UpdatePrompt || m.Updates.PromptFocus != 0 {
		t.Fatal("available release did not default to Cancel")
	}
	if !m.UpdatePromptActivate() || m.Page != Clock || m.MaybePromptUpdate() {
		t.Fatal("Cancel did not return to the previous screen")
	}
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "Changes", false, ""))
	if m.MaybePromptUpdate() {
		t.Fatal("same completion prompted twice")
	}
	m.SetUpdateStatus(availableStatus(2, "0.6.0", "Changes", false, ""))
	if !m.MaybePromptUpdate() {
		t.Fatal("Cancel suppressed the next successful check")
	}
	m.Updates.PromptFocus = 1
	m.UpdatePromptActivate()
	if m.Page != UpdatePrompt || m.Updates.Message != "CHECKING INSTALL SUPPORT" || m.TakeUpdateRequest() != nil {
		t.Fatal("unavailable Install action was not explained safely")
	}
}

func TestIgnoredReleaseManualRediscoveryAndNewerVersion(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, "0.6.0"))
	if m.MaybePromptUpdate() {
		t.Fatal("automatic check offered an ignored release")
	}
	m.SetUpdateStatus(availableStatus(2, "0.6.0", "", true, "0.6.0"))
	if !m.MaybePromptUpdate() {
		t.Fatal("manual check could not rediscover ignored release")
	}
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	request := m.TakeUpdateRequest()
	if request == nil || request.Kind != update.IgnoreRelease {
		t.Fatal("Ignore did not request a durable save")
	}
	status := availableStatus(2, "0.6.0", "", true, "0.6.0")
	status.LastCommandID = request.ID
	m.SetUpdateStatus(status)
	if m.Page != Home {
		t.Fatal("confirmed Ignore did not dismiss the prompt")
	}
	m.SetUpdateStatus(availableStatus(3, "0.7.0", "", false, "0.6.0"))
	if !m.MaybePromptUpdate() || m.Updates.Notice.Version != "0.7.0" {
		t.Fatal("ignored release suppressed a newer version")
	}
}

func TestIgnoreWaitsForPersistenceAndReportsFailure(t *testing.T) {
	m := New()
	m.OpenUpdates()
	status := availableStatus(1, "0.6.0", "Changes", false, "")
	m.SetUpdateStatus(status)
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	request := m.TakeUpdateRequest()
	if request == nil || request.Kind != update.IgnoreRelease || request.Version != "0.6.0" || m.Page != UpdatePrompt {
		t.Fatalf("Ignore request: %+v", request)
	}
	status.LastCommandID = request.ID
	status.OperationErr = errors.New("disk full")
	status.CommandErr = status.OperationErr
	status.IgnoredVersion = request.Version // rename committed, directory sync failed
	m.SetUpdateStatus(status)
	if m.Page != UpdatePrompt || !strings.Contains(m.Updates.Message, "COULD NOT IGNORE") {
		t.Fatal("failed Ignore was reported as saved")
	}
	status.Busy = true
	m.SetUpdateStatus(status)
	if !strings.Contains(m.Updates.Message, "COULD NOT IGNORE") {
		t.Fatal("concurrent check hid Ignore failure")
	}
	status.Busy = false
	status.Sequence++
	status.OperationErr = nil
	m.SetUpdateStatus(status)
	if !strings.Contains(m.Updates.Message, "COULD NOT IGNORE") {
		t.Fatal("check completion hid Ignore failure")
	}
	m.UpdatePromptActivate()
	request = m.TakeUpdateRequest()
	status.LastCommandID = request.ID
	status.OperationErr = nil
	status.CommandErr = nil
	status.IgnoredVersion = request.Version
	m.SetUpdateStatus(status)
	if m.Page != UpdatesPage || m.MaybePromptUpdate() {
		t.Fatal("confirmed Ignore did not return to Settings Updates")
	}
}

func TestManualResultAfterAbandoningPrompt(t *testing.T) {
	for _, version := range []string{"0.6.0", "0.7.0"} {
		m := New()
		m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
		m.MaybePromptUpdate()
		m.OpenSwitcher()
		m.OpenSettings()
		m.OpenUpdates()
		m.SetUpdateStatus(availableStatus(2, version, "", true, ""))
		if !m.MaybePromptUpdate() || m.Updates.Notice.Version != version {
			t.Fatal("abandoned overlay history blocked manual result")
		}
	}
}

func TestIgnoreAcknowledgementBeneathOverlay(t *testing.T) {
	m := New()
	m.Select(Clock)
	status := availableStatus(1, "0.6.0", "", false, "")
	m.SetUpdateStatus(status)
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	request := m.TakeUpdateRequest()
	m.OpenNotifications()
	status.LastCommandID, status.IgnoredVersion = request.ID, request.Version
	m.SetUpdateStatus(status)
	if m.Page != NotificationCenter {
		t.Fatal("Ignore closed an overlay")
	}
	m.Back()
	if m.Page != Clock {
		t.Fatal("Back reopened an ignored prompt")
	}
}

func TestIgnoreAcknowledgementRedrawsSwitcherParent(t *testing.T) {
	m := New()
	m.Select(Clock)
	status := availableStatus(1, "0.6.0", "", false, "")
	m.SetUpdateStatus(status)
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	request := m.TakeUpdateRequest()
	m.OpenSwitcher()
	status.LastCommandID, status.IgnoredVersion = request.ID, request.Version
	if !m.SetUpdateStatus(status) || m.Page != Switcher || m.ShellParents[len(m.ShellParents)-1] != Clock {
		t.Fatal("restored Switcher parent did not request a redraw")
	}
}

func TestDefinitiveNoUpdateClearsDeferredNotice(t *testing.T) {
	for _, result := range []update.CheckStatus{update.UpToDate, update.NoStableRelease} {
		m := New()
		m.OpenEditor()
		m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
		m.SetUpdateStatus(update.Status{Sequence: 2, Result: update.CheckResult{Status: result}})
		m.Select(Home)
		if m.MaybePromptUpdate() {
			t.Fatal("stale deferred release was offered")
		}
	}
}

func TestBusySnapshotRetainsUnseenCompletedReleaseResult(t *testing.T) {
	m := New()
	// A manual discovery may be retained in a newer busy snapshot after
	// the UI misses the completion snapshot. Manual can offer ignored releases.
	status := availableStatus(1, "0.6.0", "Manual notes", true, "0.6.0")
	status.Busy = true
	m.SetUpdateStatus(status)
	if !m.MaybePromptUpdate() || m.Updates.Notice.Notes != "Manual notes" {
		t.Fatal("coalesced busy snapshot lost completed manual discovery")
	}
	m.DismissUpdate()
	m.SetUpdateStatus(status)
	if m.MaybePromptUpdate() {
		t.Fatal("same completed sequence was presented twice")
	}
	m.OpenEditor()
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "", false, ""))
	m.SetUpdateStatus(update.Status{Sequence: 3, Busy: true, Result: update.CheckResult{Status: update.UpToDate}})
	m.Select(Home)
	if m.MaybePromptUpdate() || m.Updates.PendingRelease != nil {
		t.Fatal("coalesced up-to-date result retained a stale deferred notice")
	}
}

func TestOldIgnoreAcknowledgementDoesNotDismissNewRelease(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	request := m.TakeUpdateRequest()
	m.Back()
	newer := availableStatus(2, "0.7.0", "", false, "")
	m.SetUpdateStatus(newer)
	m.MaybePromptUpdate()
	newer.LastCommandID = request.ID
	newer.IgnoredVersion = "0.6.0"
	m.SetUpdateStatus(newer)
	if m.Page != UpdatePrompt || m.Updates.Notice.Version != "0.7.0" || m.Updates.Message != "" {
		t.Fatal("acknowledgement for an older release changed the new prompt")
	}
}

func TestReleaseNotesScrollCloseReopenAndPinnedIdentity(t *testing.T) {
	m := New()
	notes := strings.Repeat("A release note with enough text to wrap across several lines.\n", 40)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", notes, true, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 2
	m.UpdatePromptActivate()
	if m.Page != ReleaseNotes || len(m.Updates.Notes.lines) <= 10 {
		t.Fatal("long notes did not open as scrollable content")
	}
	if changed, _, _ := m.Rotate(4); !changed || m.Updates.NotesOffset != 4 {
		t.Fatal("rotary did not scroll notes")
	}
	if !m.ReleaseNotesDragStart(240, 200) || !m.ReleaseNotesDragMove(-76) || m.Updates.NotesOffset != 8 {
		t.Fatal("touch drag did not scroll notes")
	}
	m.ReleaseNotesMove(100000)
	_, _, visible, _ := m.notesMetrics()
	if m.Updates.NotesOffset != len(m.Updates.Notes.lines)-visible || m.ReleaseNotesMove(1) {
		t.Fatal("notes scrolled beyond the end")
	}
	m.ReleaseNotesMove(-100000)
	if m.Updates.NotesOffset != 0 || m.ReleaseNotesMove(-1) {
		t.Fatal("notes scrolled before the start")
	}
	m.ReleaseNotesMove(3)
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "New notes", false, ""))
	if m.MaybePromptUpdate() || m.Updates.Notice.Version != "0.6.0" {
		t.Fatal("new check replaced the release currently being reviewed")
	}
	if !m.Back() || m.Page != UpdatePrompt {
		t.Fatal("Back from notes did not return to choices")
	}
	m.UpdatePromptActivate()
	if m.Page != ReleaseNotes || m.Updates.NotesOffset != 3 || m.TakeUpdateRequest() != nil {
		t.Fatal("reopening notes lost position or started another check")
	}
	if changed, _ := m.Tap(240, 410); !changed || m.Page != UpdatePrompt {
		t.Fatal("Close did not return to update choices")
	}
	m.Back()
	if !m.MaybePromptUpdate() || m.Updates.Notice.Version != "0.7.0" {
		t.Fatal("queued newer release was lost")
	}
}

func TestUpdatePromptDefersEditorsAndCriticalAlerts(t *testing.T) {
	m := New()
	m.OpenEditor()
	m.Editor.Secure = true
	m.Editor.Insert("secret")
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	if m.MaybePromptUpdate() || m.Editor.Draft != "secret" || m.Page != TextEditor {
		t.Fatal("automatic prompt interrupted secure text entry")
	}
	m.Select(Home)
	m.Critical = &notify.Item{}
	if m.MaybePromptUpdate() {
		t.Fatal("automatic prompt interrupted a critical alert")
	}
	m.Critical = nil
	if !m.MaybePromptUpdate() {
		t.Fatal("deferred prompt did not open after interaction finished")
	}
}

func TestUpdatePromptWaitsForWiFiConfirmation(t *testing.T) {
	m := New()
	m.Select(WiFiConfirm)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	if m.MaybePromptUpdate() || m.Page != WiFiConfirm {
		t.Fatal("update interrupted Wi-Fi confirmation")
	}
	m.Back()
	if !m.MaybePromptUpdate() || m.Updates.ReturnPage != WiFiPage {
		t.Fatal("deferred notice was lost after confirmation closed")
	}
}

func TestUpdatePromptWaitsForBannerAndKeepsNotesReadable(t *testing.T) {
	m := New()
	now := time.Now()
	item := notify.Item{ID: 1, Request: notify.Request{Title: "Timer finished"}}
	m.UpdateNotices([]notify.Item{item}, notify.Result{Presentation: notify.Banner, Item: item}, now)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "Release notes", false, ""))
	if m.MaybePromptUpdate() {
		t.Fatal("prompt opened under a visible banner")
	}
	if !m.BannerExpired(now.Add(4*time.Second)) || !m.MaybePromptUpdate() {
		t.Fatal("queued prompt did not open after banner expiry")
	}
	for _, page := range []int{UpdatePrompt, ReleaseNotes} {
		m.Select(page)
		m.UpdateNotices([]notify.Item{item}, notify.Result{Presentation: notify.Banner, Item: item}, now)
		if m.Banner != nil || len(m.Notices) != 1 {
			t.Fatal("notification obscured the selected release or disappeared from history")
		}
	}
	m.UpdateNotices([]notify.Item{item}, notify.Result{Presentation: notify.Modal, Item: item}, now)
	if !m.HasCritical() {
		t.Fatal("release notes suppressed a critical alert")
	}
}

func TestMissingNotesWrappingAndAppearanceReflow(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", true, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 2
	m.UpdatePromptActivate()
	if !strings.Contains(strings.Join(m.Updates.Notes.lines, " "), "NO RELEASE NOTES PROVIDED.") {
		t.Fatal("missing notes were not explained")
	}
	for _, line := range wrapReleaseNotes("• Don't hide smart ‘quotes’.\n"+strings.Repeat("Z", 100), 16) {
		if len([]rune(line)) > 16 {
			t.Fatal("release note line exceeds safe text width")
		}
	}
	snapshot := m.Snapshot()
	oldDocument := snapshot.Updates.Notes
	prefs := m.Appearance
	prefs.TextScale = 150
	m.SetAppearance(prefs)
	if m.Updates.Notes == oldDocument || m.Updates.Notes.columns != 16 || oldDocument.columns != 24 {
		t.Fatal("appearance reflow changed the renderer's immutable document")
	}
	if Draw(m, time.Now()).Bounds().Dx() != 480 {
		t.Fatal("notes rendering failed")
	}
}

func TestQueuedUpdateWaitsForPromptOverlayToClose(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "", false, ""))
	m.OpenSwitcher()
	if m.MaybePromptUpdate() {
		t.Fatal("queued release replaced the prompt beneath an overlay")
	}
	m.Back()
	if m.Page != UpdatePrompt || m.Updates.ReturnPage != Clock {
		t.Fatal("overlay lost the prompt's original return page")
	}
	m.Back()
	if m.Page != Clock {
		t.Fatal("Cancel created a navigation cycle")
	}
	if !m.MaybePromptUpdate() || m.Updates.Notice.Version != "0.7.0" || m.Updates.ReturnPage != Clock {
		t.Fatal("queued release was lost after closing the original prompt")
	}
}

func TestUpdatePromptPreservesUncommittedWidgetEdit(t *testing.T) {
	m := New()
	m.Select(Timer)
	m.EditingWidget = "timer.duration"
	m.WidgetEditValue = 42
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	if m.MaybePromptUpdate() || m.EditingWidget != "timer.duration" || m.WidgetEditValue != 42 {
		t.Fatal("automatic prompt discarded an uncommitted rotary edit")
	}
	m.EditingWidget = ""
	if !m.MaybePromptUpdate() {
		t.Fatal("prompt did not appear after edit completion")
	}
}

func TestQueuedUpdateWaitsForNestedAppearanceOverlay(t *testing.T) {
	m := New()
	m.Select(Clock)
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "", false, ""))
	m.OpenAccessibility()
	m.OpenSwitcher()
	if m.MaybePromptUpdate() {
		t.Fatal("nested overlay lost the existing prompt ancestry")
	}
	m.Back()
	m.Back()
	if m.Page != UpdatePrompt || m.Updates.ReturnPage != Clock {
		t.Fatal("nested overlays lost the original return page")
	}
}

func TestAutomaticPromptWaitsForMutableAppearanceParent(t *testing.T) {
	m := New()
	m.OpenSettings()
	m.OpenAccessibility()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	if m.MaybePromptUpdate() {
		t.Fatal("prompt interrupted Appearance navigation")
	}
	m.Back()
	if m.Page != Settings || !m.MaybePromptUpdate() {
		t.Fatal("deferred prompt lost Settings parent")
	}
}

func TestLatestDiscoverySupersedesQueuedRelease(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "", false, ""))
	m.SetUpdateStatus(availableStatus(3, "0.6.0", "", false, ""))
	m.Back()
	if m.MaybePromptUpdate() {
		t.Fatal("withdrawn queued release was still offered")
	}
}

func TestOldIgnoreFailureDoesNotLabelNewerRelease(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	r := m.TakeUpdateRequest()
	m.Back()
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "", false, ""))
	m.MaybePromptUpdate()
	st := availableStatus(2, "0.7.0", "", false, "")
	st.LastCommandID, st.CommandErr = r.ID, errors.New("disk full")
	m.SetUpdateStatus(st)
	if m.Updates.Message != "" {
		t.Fatal("older Ignore failure labeled a newer release")
	}
}

func TestIgnoreAcknowledgementRetainsItsOwnOutcome(t *testing.T) {
	m := New()
	st := availableStatus(1, "0.6.0", "", false, "")
	m.SetUpdateStatus(st)
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 3
	m.UpdatePromptActivate()
	r := m.TakeUpdateRequest()
	st.LastCommandID, st.IgnoredVersion = r.ID, r.Version
	st.OperationErr = errors.New("later check persistence failure")
	m.SetUpdateStatus(st)
	if m.Page != Home {
		t.Fatal("later check error hid a successful Ignore acknowledgement")
	}
}

func TestRejectedNewDragClearsInterruptedNotesDrag(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", strings.Repeat("Line\n", 40), false, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 2
	m.UpdatePromptActivate()
	m.ReleaseNotesDragStart(240, 200)
	if m.ReleaseNotesDragStart(20, 20) || m.ReleaseNotesDragMove(-80) {
		t.Fatal("rejected drag reused interrupted scroll state")
	}
}

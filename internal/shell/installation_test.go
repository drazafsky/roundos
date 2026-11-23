package shell

import (
	"errors"
	"strings"
	"testing"

	"roundos/internal/update"
)

func TestInstallRequiresExplicitActionAndPinsReviewedRelease(t *testing.T) {
	m := New()
	status := availableStatus(1, "0.6.0", "Selected notes", false, "")
	status.Result.Release.Tag = "v0.6.0"
	status.Result.Release.Assets = []update.Asset{{Name: "selected", Digest: "sha256:pinned"}}
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true})
	m.SetUpdateStatus(status)
	m.MaybePromptUpdate()
	if m.TakeInstallRequest() != nil {
		t.Fatal("discovery started installation")
	}
	m.Updates.PromptFocus = 2
	m.UpdatePromptActivate()
	m.Back()
	if m.TakeInstallRequest() != nil {
		t.Fatal("reading notes started installation")
	}
	m.SetUpdateStatus(availableStatus(2, "0.7.0", "New notes", false, ""))
	m.Updates.PromptFocus = 1
	if !m.UpdatePromptActivate() {
		t.Fatal("explicit Install was refused")
	}
	if m.Snapshot().Updates.PendingInstall != nil {
		t.Fatal("render snapshot received an install command")
	}
	r := m.TakeInstallRequest()
	if r == nil || r.Release.Version != "0.6.0" || r.Release.ID != 123 || r.Release.Assets[0].Digest != "sha256:pinned" {
		t.Fatalf("Install did not pin the reviewed release: %+v", r)
	}
	m.Updates.Notice.Assets[0].Name = "changed"
	if r.Release.Assets[0].Name != "selected" {
		t.Fatal("request aliases mutable prompt assets")
	}
	m.UpdatePromptActivate()
	if m.TakeInstallRequest() != nil {
		t.Fatal("duplicate Install was queued")
	}
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true})
	m.UpdatePromptActivate()
	if m.TakeInstallRequest() != nil {
		t.Fatal("unrelated status acknowledged an in-flight Install")
	}
	m.SetInstallationStatus(update.InstallationState{Ready: true, LastCommandID: r.ID, Status: update.InstallStatus{ID: "transaction", Version: "0.6.0", Phase: "preparing"}})
	if m.Updates.InstallInFlight != 0 || m.installationAllowed() {
		t.Fatal("preparing installation allowed a duplicate")
	}
}

func TestUnsupportedInstallKeepsReleaseNotesAndChecksAvailable(t *testing.T) {
	m := New()
	m.SetInstallationStatus(update.InstallationState{Ready: true, UnsupportedReason: "unmanaged session"})
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "Notes", true, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 1
	m.UpdatePromptActivate()
	if m.TakeInstallRequest() != nil || m.Updates.Message != "INSTALL NEEDS A MANAGED DEVICE" {
		t.Fatal("unsupported Install was not explained")
	}
	m.Updates.PromptFocus = 2
	m.UpdatePromptActivate()
	if m.Page != ReleaseNotes {
		t.Fatal("unsupported installation blocked notes")
	}
	m.Back()
	m.Back()
	m.OpenUpdates()
	m.Updates.Focus = 2
	if !m.UpdatesActivate(m.Updates.Status.NextManual) {
		t.Fatal("unsupported installation blocked manual checking")
	}
}

func TestInstallationErrorsPermitRetryAndPersistedOutcomesRemainVisible(t *testing.T) {
	m := New()
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true})
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.Updates.PromptFocus = 1
	m.UpdatePromptActivate()
	r := m.TakeInstallRequest()
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true, LastCommandID: r.ID, CommandErr: errors.New("helper launch failed")})
	if !m.installationAllowed() || !strings.Contains(m.installationMessage(), "TRY AGAIN") {
		t.Fatal("request failure blocked retry or hid error")
	}
	m.UpdatePromptActivate()
	retry := m.TakeInstallRequest()
	if retry == nil || retry.ID == r.ID {
		t.Fatal("retry did not use a new command")
	}
	for _, phase := range []string{"succeeded", "rolled_back", "failed", "recovery_required"} {
		restarted := New()
		restarted.SetInstallationStatus(update.InstallationState{Ready: true, Status: update.InstallStatus{ID: "transaction", Version: "0.6.0", Phase: phase}})
		if restarted.installationMessage() == "" {
			t.Fatalf("persisted %s outcome is invisible", phase)
		}
	}
}

func TestRecoveryRequiredKeepsInstallDisabledAndRecoveryGuidanceVisible(t *testing.T) {
	m := New()
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true, CommandErr: update.ErrInstallBusy, Status: update.InstallStatus{ID: "failed", Version: "0.6.0", Phase: "recovery_required"}})
	m.Updates.PromptFocus = 1
	m.UpdatePromptActivate()
	if m.installationAllowed() || m.TakeInstallRequest() != nil || !strings.Contains(m.installationMessage(), "SSH") {
		t.Fatal("recovery requirement became a retry loop")
	}
}

func TestAcceptedTransactionProgressAndOutcomeOverrideLaunchError(t *testing.T) {
	m := New()
	st := update.InstallationState{Ready: true, CommandTransactionID: "accepted", CommandErr: errors.New("service manager returned an error"), Status: update.InstallStatus{ID: "accepted", Version: "0.6.0", Phase: "preparing"}}
	m.SetInstallationStatus(st)
	if strings.Contains(m.installationMessage(), "TRY AGAIN") || strings.Contains(m.installationSummary(), "FAILED") {
		t.Fatal("active transaction looked like a failed launch")
	}
	st.Status.Phase = "succeeded"
	m.SetInstallationStatus(st)
	if m.installationMessage() != "UPDATE INSTALLED" {
		t.Fatal("accepted successful transaction retained launch warning")
	}
	st.CommandTransactionID = ""
	m.SetInstallationStatus(st)
	if !strings.Contains(m.installationMessage(), "TRY AGAIN") {
		t.Fatal("unrelated earlier success hid a failed new request")
	}
}

func TestHistoricalInstallOutcomeDoesNotLabelANewPromptAsInstalled(t *testing.T) {
	m := New()
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true, Status: update.InstallStatus{ID: "previous", Version: "0.6.0", Phase: "succeeded"}})
	m.SetUpdateStatus(availableStatus(1, "0.6.0", "", false, ""))
	m.MaybePromptUpdate()
	if m.updatePromptMessage() != "" {
		t.Fatal("historic install labelled a newly offered release as installed")
	}
	m.Updates.PromptFocus = 1
	m.UpdatePromptActivate()
	if m.updatePromptMessage() != "PREPARING UPDATE" {
		t.Fatal("new explicit installation did not show its own progress")
	}
}

func TestInstallationReadErrorDoesNotPretendCachedProgressIsCurrent(t *testing.T) {
	m := New()
	m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true, Err: errors.New("state read failed"), Status: update.InstallStatus{ID: "transaction", Version: "0.6.0", Phase: "starting"}})
	if !strings.Contains(m.installationMessage(), "UNAVAILABLE") || m.installationSummary() != "UPDATE STATUS ERROR" {
		t.Fatal("unreadable status was presented as current progress")
	}
}

func TestIgnoreFailureRemainsVisibleAfterInstallationAttempt(t *testing.T) {
	for _, phase := range []string{"failed", "preparing"} {
		t.Run(phase, func(t *testing.T) {
			m := New()
			m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true})
			status := availableStatus(1, "0.6.0", "", false, "")
			m.SetUpdateStatus(status)
			m.MaybePromptUpdate()
			m.Updates.PromptFocus = 1
			m.UpdatePromptActivate()
			install := m.TakeInstallRequest()
			m.SetInstallationStatus(update.InstallationState{Ready: true, Supported: true, LastCommandID: install.ID,
				Status: update.InstallStatus{ID: "transaction", Version: "0.6.0", Phase: phase}})
			m.Updates.PromptFocus = 3
			m.UpdatePromptActivate()
			ignore := m.TakeUpdateRequest()
			status.LastCommandID = ignore.ID
			status.CommandErr = errors.New("disk write failed")
			m.SetUpdateStatus(status)
			if !strings.Contains(m.updatePromptMessage(), "COULD NOT IGNORE") {
				t.Fatal("installation status hid the current Ignore error")
			}
		})
	}
}

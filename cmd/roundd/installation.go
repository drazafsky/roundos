package main

import (
	"roundos/internal/notify"
	"roundos/internal/update"
	"time"
)

// Input already queued for the page under a new prompt must not choose Install
// or activate a control on a screen restored by an asynchronous Ignore result.
func staleUpdateInput(a action, shownAt time.Time) bool {
	if shownAt.IsZero() {
		return false
	}
	at := a.at
	if a.kind == "touch_frame" {
		at = a.frame.At
	}
	if at.IsZero() || !at.Before(shownAt) {
		return false
	}
	switch a.kind {
	case "touch_frame", "button_down", "button_up", "rotate", "press":
		return true
	}
	return false
}

func installationOutcomeNotice(status update.InstallStatus) (notify.Request, bool) {
	request := notify.Request{Priority: notify.Actionable, Group: "roundos-updates"}
	switch status.Phase {
	case "succeeded":
		request.Title = "RoundOS update installed"
		request.Body = "Version " + status.Version + " is ready. See Settings > Updates."
	case "rolled_back":
		request.Title = "RoundOS update rolled back"
		request.Body = "The previous version has been restored. See Settings > Updates."
	case "failed":
		request.Title = "RoundOS update could not start"
		request.Body = "The running version is unchanged. Retry from Settings > Updates."
	case "recovery_required":
		request.Title = "RoundOS update needs recovery"
		request.Body = "Connect over SSH and inspect the update recovery log."
	default:
		return notify.Request{}, false
	}
	return request, true
}

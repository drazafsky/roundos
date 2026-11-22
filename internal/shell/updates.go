package shell

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"roundos/internal/release"
	"roundos/internal/update"
)

const UpdatesPage = WiFiConfirm + 1

type UpdateState struct {
	Status   update.Status
	Ready    bool
	Focus    int
	Pending  *update.Command
	Message  string
	InFlight uint64
	NextID   uint64
}

func (m *Model) OpenUpdates() bool {
	m.Updates.Focus = 0
	return m.Select(UpdatesPage)
}

func (m *Model) SetUpdateStatus(status update.Status) bool {
	m.Updates.Status = status
	if status.LastCommandID == m.Updates.InFlight {
		m.Updates.InFlight = 0
	}
	m.Updates.Ready = true
	m.Updates.Message = ""
	return m.Page == UpdatesPage
}

func (m *Model) TakeUpdateRequest() *update.Command {
	request := m.Updates.Pending
	m.Updates.Pending = nil
	return request
}

func (m *Model) UpdateRequestFailed() {
	m.Updates.InFlight = 0
	m.Updates.Message = "UPDATE SERVICE BUSY - TRY AGAIN"
}

func (m *Model) UpdatesRotate(delta int) bool {
	next := max(0, min(3, m.Updates.Focus+delta))
	if next == m.Updates.Focus {
		return false
	}
	m.Updates.Focus = next
	return true
}

func (m *Model) UpdatesActivate(now time.Time) bool {
	if m.Updates.Focus == 3 {
		return m.Back()
	}
	if !m.Updates.Ready || m.Updates.Pending != nil || m.Updates.InFlight != 0 {
		return false
	}
	status := m.Updates.Status
	command := update.Command{Kind: update.ChangePreferences, Preferences: status.Preferences}
	switch m.Updates.Focus {
	case 0:
		command.Preferences.Automatic = !status.Preferences.Automatic
	case 1:
		choices := update.IntervalPresets()
		for i, minutes := range choices {
			if minutes == status.Preferences.IntervalMinutes {
				command.Preferences.IntervalMinutes = choices[(i+1)%len(choices)]
				break
			}
		}
	case 2:
		if status.Busy || now.Before(status.NextManual) {
			return false
		}
		command.Kind = update.ManualCheck
	}
	m.Updates.NextID++
	command.ID = m.Updates.NextID
	m.Updates.InFlight = command.ID
	m.Updates.Pending = &command
	return true
}

func (m *Model) UpdatesTap(x, y int) bool {
	if y < 145 && x < 160 {
		return m.Back()
	}
	if x < 93 || x > 387 {
		return false
	}
	for i := 0; i < 4; i++ {
		top, height := m.updateControlGeometry(i)
		if y >= top && y < top+height {
			m.Updates.Focus = i
			return m.UpdatesActivate(time.Now())
		}
	}
	return false
}

func (m *Model) updateControlGeometry(i int) (top, height int) {
	if m.Appearance.LargeTargets {
		return 194 + i*58, 54
	}
	return 198 + i*54, 48
}

func intervalLabel(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d MINUTES", minutes)
	}
	if minutes == 60 {
		return "1 HOUR"
	}
	return fmt.Sprintf("%d HOURS", minutes/60)
}

func waitLabel(wait time.Duration) string {
	seconds := max(0, int(wait.Round(time.Second)/time.Second))
	if seconds >= 3600 {
		return fmt.Sprintf("%dH %02dM", seconds/3600, seconds%3600/60)
	}
	return fmt.Sprintf("%dM %02dS", seconds/60, seconds%60)
}

func (m *Model) updateMessage() string {
	status := m.Updates.Status
	if m.Updates.Message != "" {
		return m.Updates.Message
	}
	if !m.Updates.Ready {
		return "LOADING UPDATE SETTINGS"
	}
	if m.Updates.InFlight != 0 {
		return "PLEASE WAIT"
	}
	operationErr := status.CommandErr
	if operationErr == nil || errors.Is(operationErr, update.ErrBusy) || errors.Is(operationErr, update.ErrCooldown) {
		operationErr = status.OperationErr
	}
	if errors.Is(operationErr, update.ErrCooldown) {
		return "PLEASE WAIT BEFORE CHECKING"
	}
	if operationErr != nil && !errors.Is(operationErr, update.ErrBusy) {
		return "COULD NOT SAVE UPDATE SETTINGS"
	}
	if status.Busy || errors.Is(operationErr, update.ErrBusy) {
		return "CHECKING FOR A RELEASE"
	}
	var limited *update.RateLimitError
	if errors.As(status.Err, &limited) {
		return "GITHUB RATE LIMIT - PLEASE WAIT"
	}
	if errors.Is(status.Err, update.ErrIncompatibleRelease) {
		return "RELEASE HAS NO COMPATIBLE PACKAGE"
	}
	if status.Err != nil {
		return "CHECK FAILED - TRY AGAIN LATER"
	}
	switch status.Result.Status {
	case update.Available:
		return "NEW RELEASE AVAILABLE"
	case update.UpToDate:
		return "YOU ARE UP TO DATE"
	case update.NoStableRelease:
		return "NO STABLE RELEASE PUBLISHED"
	}
	return "CHECK FOR A STABLE RELEASE"
}

func (m *Model) DrawUpdates(im *image.RGBA, now time.Time) {
	p := m.Appearance.Palette()
	status := m.Updates.Status
	textTheme(im, 240, 86, 3, "UPDATES", p.Accent, true, m.Appearance)
	textTheme(im, 118, 116, 2, "<", p.PrimaryText, true, m.Appearance)
	textTheme(im, 240, 119, 1, "INSTALLED "+strings.ToUpper(release.Version), p.SecondaryText, true, m.Appearance)
	last := "LAST CHECK: NEVER"
	if !status.LastAttempt.IsZero() {
		last = "LAST CHECK: " + status.LastAttempt.Local().Format("Jan 02 15:04")
	}
	textTheme(im, 240, 142, 1, strings.ToUpper(last), p.SecondaryText, true, m.Appearance)
	textTheme(im, 240, 163, 1, m.updateMessage(), p.PrimaryText, true, m.Appearance)
	next := "AUTOMATIC CHECKS OFF"
	if status.Preferences.Automatic {
		next = "NEXT CHECK: AFTER CURRENT CHECK"
		if !status.NextAutomatic.IsZero() {
			next = "NEXT CHECK IN " + waitLabel(status.NextAutomatic.Sub(now))
		}
	}
	textTheme(im, 240, 179, 1, next, p.SecondaryText, true, m.Appearance)
	on := "OFF"
	if status.Preferences.Automatic {
		on = "ON"
	}
	labels := []string{"AUTOMATIC: " + on, "EVERY " + intervalLabel(status.Preferences.IntervalMinutes), "CHECK NOW", "BACK"}
	details := []string{"PRESS TO TOGGLE", "PRESS TO CHANGE INTERVAL", "CHECK FOR A STABLE RELEASE", "SETTINGS"}
	disabled := status.Busy || now.Before(status.NextManual)
	if status.Busy {
		details[2] = "CHECK IN PROGRESS"
	} else if now.Before(status.NextManual) {
		details[2] = "AVAILABLE IN " + waitLabel(status.NextManual.Sub(now))
	}
	for i, label := range labels {
		y, height := m.updateControlGeometry(i)
		rect(im, 93, y, 387, y+height, p.Surface)
		if m.Updates.Focus == i {
			outlineRect(im, 91, y-2, 389, y+height+2, p.Focus)
		}
		color := p.PrimaryText
		if (i == 2 && disabled) || ((!m.Updates.Ready || m.Updates.InFlight != 0) && i != 3) {
			color = p.Disabled
		}
		textTheme(im, 240, y+6, 2, label, color, true, m.Appearance)
		textTheme(im, 240, y+29, 1, details[i], p.SecondaryText, true, m.Appearance)
	}
}

package update

import (
	"context"
	"errors"
	"time"
)

type InstallationBackend interface {
	Supported(context.Context) error
	Request(context.Context, Release) (InstallStatus, error)
	ReadStatus() (InstallStatus, error)
}

type InstallCommand struct {
	ID      uint64
	Release Release
}

type InstallationState struct {
	Ready                bool
	Supported            bool
	UnsupportedReason    string
	Status               InstallStatus
	LastCommandID        uint64
	CommandTransactionID string
	CommandErr           error
	Err                  error
}

// InstallationService keeps service-manager calls and disk reads off the UI
// loop. Only an explicit command can request an installation; polling is read-only.
type InstallationService struct {
	commands chan InstallCommand
	events   chan InstallationState
	done     chan struct{}
}

func StartInstallation(ctx context.Context, backend InstallationBackend) *InstallationService {
	s := newInstallationService()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		s.run(ctx, backend, ticker.C)
	}()
	return s
}

func newInstallationService() *InstallationService {
	return &InstallationService{commands: make(chan InstallCommand, 1), events: make(chan InstallationState, 1), done: make(chan struct{})}
}

func (s *InstallationService) Events() <-chan InstallationState { return s.events }
func (s *InstallationService) Done() <-chan struct{}            { return s.done }

func (s *InstallationService) Submit(command InstallCommand) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	command.Release.Assets = append([]Asset(nil), command.Release.Assets...)
	select {
	case s.commands <- command:
		return true
	default:
		return false
	}
}

func installationErrorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func sameInstallationState(a, b InstallationState) bool {
	return a.Ready == b.Ready && a.Supported == b.Supported && a.UnsupportedReason == b.UnsupportedReason &&
		a.Status == b.Status && a.LastCommandID == b.LastCommandID && a.CommandTransactionID == b.CommandTransactionID && installationErrorText(a.CommandErr) == installationErrorText(b.CommandErr) && installationErrorText(a.Err) == installationErrorText(b.Err)
}

func (s *InstallationService) run(ctx context.Context, backend InstallationBackend, polls <-chan time.Time) {
	defer close(s.done)
	defer close(s.events)
	var state, last InstallationState
	published := false
	refresh := func(checkSupport bool) {
		previous := state.Status
		status, err := backend.ReadStatus()
		state.Err = err
		if err == nil {
			state.Status = status
		}
		if checkSupport || previous.Phase != state.Status.Phase {
			if state.Status.InProgress() || state.Status.Phase == "recovery_required" {
				state.Supported = false
				state.UnsupportedReason = "Installation is in progress"
				if state.Status.Phase == "recovery_required" {
					state.UnsupportedReason = "SSH recovery is required"
				}
			} else {
				probe, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := backend.Supported(probe)
				cancel()
				state.Supported = err == nil
				state.UnsupportedReason = installationErrorText(err)
			}
		}
		state.Ready = true
	}
	emit := func() {
		if published && sameInstallationState(last, state) {
			return
		}
		last, published = state, true
		select {
		case s.events <- state:
			return
		default:
		}
		select {
		case <-s.events:
		default:
		}
		select {
		case s.events <- state:
		default:
		}
	}
	refresh(true)
	emit()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-polls:
			ticks++
			refresh(ticks%30 == 0)
		case command := <-s.commands:
			state.LastCommandID = command.ID
			state.CommandErr = nil
			state.CommandTransactionID = ""
			if state.Status.InProgress() || state.Status.Phase == "recovery_required" {
				state.CommandErr = ErrInstallBusy
			} else if state.Err != nil {
				state.CommandErr = state.Err
			} else if !state.Supported {
				state.CommandErr = errors.New("installation requires a supported managed device")
			} else {
				request, cancel := context.WithTimeout(ctx, 30*time.Second)
				previousID := state.Status.ID
				status, err := backend.Request(request, command.Release)
				cancel()
				state.CommandErr = err
				if err == nil || status.ID != previousID {
					state.CommandTransactionID = status.ID
				}
				if status.ID != "" {
					state.Status = status
				}
				refresh(true)
			}
		}
		emit()
	}
}

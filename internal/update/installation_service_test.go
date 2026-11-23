package update

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type installationReply struct {
	status InstallStatus
	err    error
}
type fakeInstallationBackend struct {
	mu                  sync.Mutex
	status              InstallStatus
	readErr, supportErr error
	requests            chan Release
	replies             chan installationReply
}

func (f *fakeInstallationBackend) Supported(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supportErr
}
func (f *fakeInstallationBackend) ReadStatus() (InstallStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, f.readErr
}
func (f *fakeInstallationBackend) Request(ctx context.Context, r Release) (InstallStatus, error) {
	select {
	case f.requests <- r:
	case <-ctx.Done():
		return InstallStatus{}, ctx.Err()
	}
	select {
	case reply := <-f.replies:
		f.mu.Lock()
		f.status = reply.status
		f.mu.Unlock()
		return reply.status, reply.err
	case <-ctx.Done():
		return InstallStatus{}, ctx.Err()
	}
}
func installationFixture(t *testing.T) (*InstallationService, *fakeInstallationBackend, chan time.Time) {
	t.Helper()
	f := &fakeInstallationBackend{requests: make(chan Release, 1), replies: make(chan installationReply, 1)}
	s := newInstallationService()
	polls := make(chan time.Time, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go s.run(ctx, f, polls)
	t.Cleanup(func() { cancel(); <-s.Done() })
	return s, f, polls
}
func waitInstallation(t *testing.T, s *InstallationService, ready func(InstallationState) bool) InstallationState {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case state, ok := <-s.Events():
			if !ok {
				t.Fatal("installation service stopped")
			}
			if ready(state) {
				return state
			}
		case <-timeout.C:
			t.Fatal("installation status timeout")
		}
	}
}
func TestInstallationWorkerRequiresExplicitSelectionAndRetainsCommandOutcome(t *testing.T) {
	s, f, polls := installationFixture(t)
	waitInstallation(t, s, func(st InstallationState) bool { return st.Ready && st.Supported })
	polls <- time.Now()
	if len(f.requests) != 0 {
		t.Fatal("polling started an installation")
	}
	r := Release{ID: 123, Version: "0.6.0", Assets: []Asset{{Name: "selected", Digest: "pinned"}}}
	if !s.Submit(InstallCommand{ID: 7, Release: r}) {
		t.Fatal("command refused")
	}
	r.Assets[0].Name = "changed"
	select {
	case selected := <-f.requests:
		if selected.ID != 123 || selected.Assets[0].Name != "selected" || selected.Assets[0].Digest != "pinned" {
			t.Fatal("selected asset identity changed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("explicit request not delivered")
	}
	f.replies <- installationReply{status: InstallStatus{ID: "transaction", Version: "0.6.0", Phase: "queued"}}
	waitInstallation(t, s, func(st InstallationState) bool { return st.LastCommandID == 7 && st.Status.Phase == "queued" })
	f.mu.Lock()
	f.status.Phase = "succeeded"
	f.readErr = errors.New("temporary read failure")
	f.mu.Unlock()
	polls <- time.Now()
	st := waitInstallation(t, s, func(st InstallationState) bool { return st.Err != nil })
	if st.LastCommandID != 7 || st.CommandErr != nil {
		t.Fatal("later read error overwrote request acknowledgement")
	}
	f.mu.Lock()
	f.readErr = nil
	f.mu.Unlock()
	polls <- time.Now()
	waitInstallation(t, s, func(st InstallationState) bool { return st.Status.Phase == "succeeded" && st.Supported })
}

func TestInstallationWorkerUnsupportedSessionDoesNotCallInstaller(t *testing.T) {
	s, f, polls := installationFixture(t)
	waitInstallation(t, s, func(st InstallationState) bool { return st.Ready })
	f.mu.Lock()
	f.supportErr = errors.New("unmanaged session")
	f.mu.Unlock()
	for range 30 {
		polls <- time.Now()
	}
	waitInstallation(t, s, func(st InstallationState) bool { return !st.Supported })
	s.Submit(InstallCommand{ID: 9, Release: Release{Version: "0.6.0"}})
	waitInstallation(t, s, func(st InstallationState) bool { return st.LastCommandID == 9 && st.CommandErr != nil })
	if len(f.requests) != 0 {
		t.Fatal("unmanaged session reached installer")
	}
}

func TestInstallationWorkerReportsPersistedRollbackAfterRestart(t *testing.T) {
	f := &fakeInstallationBackend{status: InstallStatus{ID: "completed", Version: "0.6.0", Phase: "rolled_back"}}
	s := newInstallationService()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); <-s.Done() }()
	go s.run(ctx, f, nil)
	st := waitInstallation(t, s, func(st InstallationState) bool { return st.Ready })
	if st.Status.Phase != "rolled_back" || st.Status.Version != "0.6.0" {
		t.Fatal("restart lost persisted outcome")
	}
}

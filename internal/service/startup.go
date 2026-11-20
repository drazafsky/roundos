// Package service implements the daemon's systemd startup readiness contract.
package service

import (
	"net"
	"os"
	"time"

	"roundos/internal/wire"
)

const StartupProtocol = "sd-notify-v1"

// NotifyReady is a no-op for manual sessions. A managed startup must deliver
// READY=1 successfully; otherwise systemd must not report startup success.
func NotifyReady() error {
	path := os.Getenv("NOTIFY_SOCKET")
	if path == "" {
		return nil
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	_, err = conn.Write([]byte("READY=1\nSTATUS=Display presented; Clock and Timer initialized"))
	return err
}

// Startup is owned by the shell event loop. Optional apps and ongoing device
// acceptance do not gate startup, but both built-in apps must publish a view.
type Startup struct {
	presented bool
	views     map[string]bool
	notified  bool
	notify    func() error
}

func NewStartup(notify func() error) *Startup {
	return &Startup{views: make(map[string]bool), notify: notify}
}

func (s *Startup) FramePresented() error {
	s.presented = true
	return s.ready()
}

func (s *Startup) AppEvent(app, kind string) error {
	if app == wire.ClockID || app == wire.TimerID {
		switch kind {
		case "view":
			s.views[app] = true
		case "connected", "disconnected", "error", "stopped":
			s.views[app] = false
		}
	}
	return s.ready()
}

func (s *Startup) ready() error {
	if s.notified || !s.presented || !s.views[wire.ClockID] || !s.views[wire.TimerID] {
		return nil
	}
	if err := s.notify(); err != nil {
		return err
	}
	s.notified = true
	return nil
}

package service

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roundos/internal/wire"
)

func TestReadinessRequiresDisplayAndBothInitializedApps(t *testing.T) {
	for _, frameFirst := range []bool{true, false} {
		count := 0
		s := NewStartup(func() error { count++; return nil })
		if frameFirst {
			_ = s.FramePresented()
		}
		_ = s.AppEvent(wire.ClockID, "connected")
		_ = s.AppEvent(wire.TimerID, "connected")
		_ = s.AppEvent(wire.ClockID, "view")
		_ = s.AppEvent("org.roundos.optional", "view")
		if count != 0 {
			t.Fatal("reported ready before both built-in views")
		}
		_ = s.AppEvent(wire.TimerID, "view")
		if !frameFirst {
			if count != 0 {
				t.Fatal("reported ready before a successful presentation")
			}
			_ = s.FramePresented()
		}
		_ = s.AppEvent(wire.TimerID, "view")
		_ = s.FramePresented()
		if count != 1 {
			t.Fatalf("expected exactly one readiness notification, got %d", count)
		}
	}
}

func TestStartupDisconnectInvalidatesInitialView(t *testing.T) {
	count := 0
	s := NewStartup(func() error { count++; return nil })
	_ = s.AppEvent(wire.ClockID, "view")
	_ = s.AppEvent(wire.ClockID, "disconnected")
	_ = s.AppEvent(wire.TimerID, "view")
	_ = s.FramePresented()
	if count != 0 {
		t.Fatal("disconnected app allowed startup")
	}
	_ = s.AppEvent(wire.ClockID, "view")
	if count != 1 {
		t.Fatal("reinitialized app did not complete startup")
	}
}

func TestNotificationErrorPropagates(t *testing.T) {
	want := errors.New("notification failed")
	s := NewStartup(func() error { return want })
	_ = s.FramePresented()
	_ = s.AppEvent(wire.ClockID, "view")
	if err := s.AppEvent(wire.TimerID, "view"); !errors.Is(err, want) {
		t.Fatalf("notification error was lost: %v", err)
	}
}

func TestNotifyReadySocketAndManualSession(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	if err := NotifyReady(); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "ros-notify-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "notify.sock")
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("NOTIFY_SOCKET", path)
	if err := NotifyReady(); err != nil {
		t.Fatal(err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 256)
	n, _, err := listener.ReadFromUnix(buf)
	if err != nil || !strings.HasPrefix(string(buf[:n]), "READY=1\n") {
		t.Fatalf("missing readiness datagram: %q, %v", buf[:n], err)
	}
	t.Setenv("NOTIFY_SOCKET", filepath.Join(dir, "missing.sock"))
	if err := NotifyReady(); err == nil {
		t.Fatal("missing notification socket accepted")
	}
}

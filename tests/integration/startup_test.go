package integration

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/service"
)

func TestStartupProtocolProbeDoesNotOpenDevices(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	daemon := build(t, "./cmd/roundd", "roundd", root)
	out, err := exec.Command(daemon, "-startup-protocol", "-backend", "drm", "-card", "/does-not-exist", "-apps-dir", "/does-not-exist").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != service.StartupProtocol {
		t.Fatalf("hardware-free startup probe failed: %s, %v", out, err)
	}
}

func TestBackgroundBuiltInsPublishStartupViews(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	clock := build(t, "./cmd/round-clock", "round-clock", root)
	timer := build(t, "./cmd/round-timer", "round-timer", root)
	mgr, err := appmgr.New(context.Background(), clock, timer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	notified := false
	startup := service.NewStartup(func() error { notified = true; return nil })
	_ = startup.FramePresented()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for !notified {
		select {
		case event := <-mgr.Events():
			if err := startup.AppEvent(event.App, event.Kind); err != nil {
				t.Fatal(err)
			}
		case <-deadline.C:
			t.Fatal("built-in initialization never completed while Home is foreground")
		}
	}
}

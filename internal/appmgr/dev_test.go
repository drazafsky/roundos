package appmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"roundos/internal/wire"
)

func TestManagedSessionsRejectDevelopmentBeforeLaunchingApps(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "/private/systemd-notify")
	_, err := NewWithAppsRuntimeDev(context.Background(), "missing-clock", "missing-timer", "", nil, nil, &DevConfig{AppID: wire.ClockID, Directory: t.TempDir()})
	if err == nil {
		t.Fatal("managed service accepted development configuration")
	}
}

func TestDevelopmentEnvironmentOnlyReachesSelectedApp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("this test uses the original non-root manual app flow")
	}
	t.Setenv("ROUNDOS_RUI_DEV_DIR", "inherited-must-not-leak")
	t.Setenv("NOTIFY_SOCKET", "")
	t.Setenv("INVOCATION_ID", "")
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "production", true: "manual-development"}[enabled], func(t *testing.T) {
			state := t.TempDir()
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(t.TempDir(), "app")
			script := "#!/bin/sh\nprintf '%s' \"${ROUNDOS_RUI_DEV_DIR:-}\" > \"$ROUNDOS_STATE_DIR/$ROUNDOS_APP_ID\"\nexec sleep 30\n"
			if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			var dev *DevConfig
			if enabled {
				dev = &DevConfig{AppID: "org.example.dev", Directory: dir}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m, err := NewWithAppsRuntimeDev(ctx, helper, helper, state, []AppSpec{{ID: "org.example.dev", Exec: helper}}, nil, dev)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			deadline := time.Now().Add(3 * time.Second)
			for _, id := range []string{wire.ClockID, wire.TimerID, "org.example.dev"} {
				for {
					value, err := os.ReadFile(filepath.Join(state, id))
					if err == nil {
						want := ""
						if enabled && id == "org.example.dev" {
							want = dir
						}
						if string(value) != want {
							t.Fatalf("%s received unexpected development env %q", id, value)
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("app did not start", err)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}
}

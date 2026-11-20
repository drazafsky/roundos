package release

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"roundos/internal/wire"
)

func TestFinalVersionAndIPC(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	base, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if Version != strings.TrimSpace(string(base))+"-dev" {
		t.Fatalf("unexpected release: %q", Version)
	}
	if wire.Version != 7 {
		t.Fatalf("protocol change must be documented as breaking: %d", wire.Version)
	}
}
func TestManagedUnitSafetyContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	content, err := os.ReadFile(filepath.Join(root, "systemd", "roundos.service"))
	if err != nil {
		t.Fatal(err)
	}
	unit := string(content)
	required := []string{
		"User=root", "Conflicts=lightdm.service", "WantedBy=multi-user.target",
		"Restart=on-failure", "StartLimitBurst=5", "NoNewPrivileges=yes", "ProtectHome=yes",
		"Type=notify", "NotifyAccess=main", "TimeoutStartSec=45s",
		"-backend drm", "-touch auto", "-app-user roundos-app", "-state-dir /var/lib/roundos/shell-data",
		"-app-state-dir /var/lib/roundos/app-data", "-apps-dir /etc/roundos/apps", "-metrics-every 60s",
	}
	for _, item := range required {
		if !strings.Contains(unit, item) {
			t.Errorf("unit missing %s", item)
		}
	}
	settings := map[string]string{}
	for _, line := range strings.Split(unit, "\n") {
		if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
			settings[parts[0]] = parts[1]
		}
	}
	burst, err := strconv.Atoi(settings["StartLimitBurst"])
	if err != nil || burst < 1 {
		t.Fatal("invalid restart limit")
	}
	duration := func(key string) time.Duration {
		t.Helper()
		value := settings[key]
		if _, err := strconv.Atoi(value); err == nil {
			value += "s"
		}
		d, err := time.ParseDuration(value)
		if err != nil {
			t.Fatalf("invalid %s: %v", key, err)
		}
		return d
	}
	cycle := duration("TimeoutStartSec") + duration("TimeoutStopSec") + duration("RestartSec")
	if duration("StartLimitIntervalSec") <= time.Duration(burst)*cycle {
		t.Fatal("restart window permits endless slow startup/shutdown failures")
	}
	if strings.Contains(unit, "-touch-invert-y") {
		t.Fatal("must preserve hardware-validated unmirrored Y mapping")
	}
	if strings.Contains(unit, "-gesture-log-moves") || strings.Contains(unit, "-touch-trace") {
		t.Fatal("high-frequency touch logging must be opt-in")
	}
	installer, err := os.ReadFile(filepath.Join(root, "scripts", "install-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(installer), "systemctl enable ") || strings.Contains(string(installer), "systemctl stop ") {
		t.Fatal("installer must not switch display/boot services automatically")
	}
}

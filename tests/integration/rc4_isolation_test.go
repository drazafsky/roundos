package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/wire"
)

func TestRC4AppPrivilegeSeparation(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs actual reduced-privilege child processes")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root to exercise setuid/setgid")
	}
	u, err := user.Lookup("nobody")
	if err != nil {
		t.Skip("test-only nobody account is not available")
	}
	uid64, _ := strconv.ParseUint(u.Uid, 10, 32)
	gid64, _ := strconv.ParseUint(u.Gid, 10, 32)
	if uid64 == 0 || gid64 == 0 {
		t.Skip("no safe unprivileged test identity")
	}
	base, err := os.MkdirTemp("/var/lib", "roundos-rc4-test-")
	if err != nil {
		t.Skipf("cannot allocate root-owned state test root: %v", err)
	}
	defer os.RemoveAll(base)
	if err = os.Chmod(base, 0755); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(base, "app-state")
	if err = os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(state, int(uid64), int(gid64)); err != nil {
		t.Fatal(err)
	}
	if err = appmgr.CheckRuntime(&appmgr.Runtime{User: u.Username, StateDir: state}); err != nil {
		t.Fatal(err)
	}
	// Disallow exposure of untrusted host environment variables to child apps.
	const marker = "ROOT_SECRET_MUST_NOT_REACH_CHILD"
	if err = os.Setenv("ROUNDOS_HOST_PRIVATE_TEST", marker); err != nil {
		t.Fatal(err)
	}
	defer os.Unsetenv("ROUNDOS_HOST_PRIVATE_TEST")
	_, file, _, _ := runtime.Caller(0)
	srcRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	bins := map[string]string{}
	for _, which := range []string{"round-clock", "round-timer"} {
		dest := filepath.Join(base, which)
		cmd := exec.Command("go", "build", "-o", dest, "./cmd/"+which)
		cmd.Dir = srcRoot
		out, buildErr := cmd.CombinedOutput()
		if buildErr != nil {
			t.Fatalf("go build %s: %v\n%s", which, buildErr, out)
		}
		if err = os.Chmod(dest, 0755); err != nil {
			t.Fatal(err)
		}
		bins[which] = dest
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr, err := appmgr.NewWithAppsRuntime(ctx, bins["round-clock"], bins["round-timer"], filepath.Join(base, "root-state"), nil, &appmgr.Runtime{User: u.Username, StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	parentSocketDir := filepath.Dir(mgr.Socket())
	dirInfo, err := os.Stat(parentSocketDir)
	if err != nil {
		t.Fatal(err)
	}
	socketInfo, err := os.Stat(mgr.Socket())
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0710 || socketInfo.Mode().Perm() != 0660 {
		t.Fatalf("isolated socket permissions unsafe: directory=%#o socket=%#o", dirInfo.Mode().Perm(), socketInfo.Mode().Perm())
	}
	connected := map[string]bool{}
	timeout := time.NewTimer(8 * time.Second)
	defer timeout.Stop()
	for len(connected) < 2 {
		select {
		case ev := <-mgr.Events():
			if ev.Kind == "error" {
				t.Fatalf("unprivileged child failed to start: %s: %s", ev.App, ev.Detail)
			}
			if ev.Kind == "connected" {
				connected[ev.App] = true
			}
		case <-timeout.C:
			t.Fatalf("unprivileged apps failed IPC handshake: %v", connected)
		}
	}
	for id, pid := range mgr.PIDs() {
		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			t.Fatal(err)
		}
		got := strings.SplitAfter(string(status), "\n")
		foundUID, foundGID := false, false
		for _, line := range got {
			if strings.HasPrefix(line, "Uid:") {
				foundUID = strings.Fields(line)[1] == u.Uid
			}
			if strings.HasPrefix(line, "Gid:") {
				foundGID = strings.Fields(line)[1] == u.Gid
			}
		}
		if !foundUID || !foundGID {
			t.Fatalf("%s pid=%d did not drop privileges: uid=%v gid=%v", id, pid, foundUID, foundGID)
		}
		childEnv, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", pid))
		// Linux dumpability/ptrace settings can deny even root access to
		// environments after a credential change. The exact restricted
		// environment is covered by the unit test in appmgr.
		if err == nil && strings.Contains(string(childEnv), marker) {
			t.Fatal("inherited root environment leaked to application")
		}
	}
	if err = mgr.SetForeground(wire.TimerID); err != nil {
		t.Fatal(err)
	}
	get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
	if !mgr.Input(wire.TimerID, "toggle", 0) {
		t.Fatal("cannot send timer toggle")
	}
	get(t, mgr.Events(), wire.TimerID, "view", 5*time.Second)
	timerState := filepath.Join(state, "timer.json")
	var st os.FileInfo
	for i := 0; i < 20; i++ {
		st, err = os.Stat(timerState)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("timer state not stored in app-owned directory: %v", err)
	}
	f, err := os.Open(timerState)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if len(data) == 0 || st.Mode().Perm() != 0600 {
		t.Fatalf("timer state missing/private mode: %o", st.Mode().Perm())
	}
}

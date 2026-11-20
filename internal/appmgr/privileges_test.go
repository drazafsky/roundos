package appmgr

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRC4PrivilegePolicy(t *testing.T) {
	if _, err := validateRuntime(nil); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		t.Skip("runtime checks only root privilege drops")
	}
	if _, err := validateRuntime(&Runtime{User: "root", StateDir: "/"}); err == nil {
		t.Fatal("root app account accepted")
	}
	if _, err := validateRuntime(&Runtime{User: "nobody", StateDir: "relative/path"}); err == nil {
		t.Fatal("relative app data path accepted")
	}
	u, err := user.Lookup("nobody")
	if err != nil {
		t.Skip("no test account")
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	parent, err := os.MkdirTemp("/var/lib", "roundos-policy-test-")
	if err != nil {
		t.Skipf("/var/lib not writable: %v", err)
	}
	defer os.RemoveAll(parent)
	if err = os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "state")
	if err = os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(child, uid, gid); err != nil {
		t.Fatal(err)
	}
	r := &Runtime{User: u.Username, StateDir: child}
	if _, err = validateRuntime(r); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(child, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err = validateRuntime(r); err == nil {
		t.Fatal("unsafe state permissions accepted")
	}
	os.Chmod(child, 0700)
	if err = os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err = validateRuntime(r); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("unsafe parent accepted: %v", err)
	}
}
func TestRC4RootEnvironmentNotInherited(t *testing.T) {
	c := &appCredentials{uid: 2000, gid: 2000, username: "roundos-app", stateDir: "/var/lib/roundos/app-data"}
	env := strings.Join(c.restrictedEnv("/run/example.sock", "org.roundos.clock", "token123"), "\n")
	if !strings.Contains(env, "ROUNDOS_TOKEN=token123") || strings.Contains(env, "HOME=/root") || strings.Contains(env, "SUDO_") {
		t.Fatal("unsafe child environment")
	}
}

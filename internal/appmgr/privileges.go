package appmgr

// Privilege separation for the DRM shell. The shell retains its device access
// while child applications run as a dedicated, non-root system user. This
// separates apps from NetworkManager credentials and display input devices,
// but does not isolate apps from each other when they share the same UID.

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// Runtime specifies the dedicated non-root account and pre-provisioned state
// directory. A nil Runtime retains the original unprivileged developer flow.
// Production root launches must explicitly select Runtime and never silently
// fall back to launching third-party processes as root.
type Runtime struct {
	User     string
	StateDir string
}

type appCredentials struct {
	uid      uint32
	gid      uint32
	username string
	stateDir string
}

func validateRuntime(runtime *Runtime) (*appCredentials, error) {
	if runtime == nil {
		return nil, nil
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("isolated child account requires a root-owned RoundOS shell")
	}
	if runtime.User == "" || runtime.StateDir == "" {
		return nil, fmt.Errorf("both app user and app state directory are required")
	}
	u, err := user.Lookup(runtime.User)
	if err != nil {
		return nil, fmt.Errorf("app account %q not installed: %w", runtime.User, err)
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, err
	}
	if uid == 0 || gid == 0 {
		return nil, fmt.Errorf("app account must not be root or use root group")
	}
	path := filepath.Clean(runtime.StateDir)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("app state directory must be absolute")
	}
	// Never follow links: root must not pass a mutable redirection into a
	// privileged or system directory. The provisioning script creates dirs.
	st, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("app state directory not provisioned: %w", err)
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("app state must be an actual directory")
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	if !ok || uint64(stat.Uid) != uid || uint64(stat.Gid) != gid || st.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("app state directory must be owned by app user/group and mode 0700")
	}
	// All ancestors must be administrator-managed; this prevents redirecting
	// state through a user-writable parent even when final directory is safe.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			return nil, err
		}
		sys, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || sys.Uid != 0 || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("app state parent must be a trusted root-owned directory: %s", parent)
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return &appCredentials{uid: uint32(uid), gid: uint32(gid), username: runtime.User, stateDir: path}, nil
}

// restrictedEnv intentionally excludes the root shell's inherited environment
// (which can hold credentials, display settings, cloud tokens, etc.).
func (c *appCredentials) restrictedEnv(socket, id, token string) []string {
	return []string{
		"ROUNDOS_SOCKET=" + socket,
		"ROUNDOS_APP_ID=" + id,
		"ROUNDOS_TOKEN=" + token,
		"ROUNDOS_STATE_DIR=" + c.stateDir,
		"HOME=" + filepath.Dir(c.stateDir),
		"USER=" + c.username,
		"LOGNAME=" + c.username,
		"PATH=/usr/bin:/bin",
		"LANG=C.UTF-8",
	}
}

// CheckRuntime validates a preprovisioned app account/state directory before
// touching DRM devices or stopping the host desktop.
func CheckRuntime(runtime *Runtime) error { _, err := validateRuntime(runtime); return err }

// peerUserID obtains the kernel-authenticated Unix socket UID; unlike an IPC
// hello token, this cannot be forged by application-supplied JSON.
func peerUserID(conn net.Conn) (uint32, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("non-Unix IPC peer")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var socketErr error
	if err = raw.Control(func(fd uintptr) {
		cred, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if e != nil {
			socketErr = e
			return
		}
		uid = cred.Uid
	}); err != nil {
		return 0, err
	}
	return uid, socketErr
}

package appmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DevConfig is deliberately absent from installed manifests and production
// Runtime. Only an explicit manual shell launch can enable a selected app.
type DevConfig struct{ AppID, Directory string }

func validateDev(config *DevConfig, credentials *appCredentials, apps map[string]string) error {
	if config == nil {
		return nil
	}
	if os.Getenv("NOTIFY_SOCKET") != "" || os.Getenv("INVOCATION_ID") != "" {
		return fmt.Errorf("RUI development is unavailable in managed service sessions")
	}
	if _, ok := apps[config.AppID]; !ok {
		return fmt.Errorf("unknown RUI development app")
	}
	if !filepath.IsAbs(config.Directory) {
		return fmt.Errorf("RUI development directory must be absolute")
	}
	info, err := os.Lstat(config.Directory)
	if err != nil {
		return err
	}
	uid := uint32(os.Geteuid())
	if credentials != nil {
		uid = credentials.uid
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if uid == 0 || !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || st.Uid != uid || info.Mode().Perm() != 0700 {
		return fmt.Errorf("RUI development directory must be owned by the non-root app user with mode 0700")
	}
	return nil
}

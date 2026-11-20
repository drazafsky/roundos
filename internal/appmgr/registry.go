// Package appmgr discovery allows trusted local Go applications to be installed
// without recompiling roundd. Manifests are read at startup from a explicitly
// configured directory; this is not a sandbox or untrusted app store.
package appmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

type AppSpec struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Exec string `json:"exec"`
}

var validAppID = regexp.MustCompile(`^org\.[a-z0-9][a-z0-9.-]{2,63}$`)
var validIcon = regexp.MustCompile(`^[A-Z0-9]{1,3}$`)

// Discover rejects duplicates, symlink escapes, oversized manifests, and
// executable entries that aren't regular files. Manifest paths are never
// interpreted by a shell or treated as an argument list.
func Discover(dir string) ([]AppSpec, error) {
	if dir == "" {
		return nil, nil
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	st, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("app directory is not a directory")
	}
	// Root-launched RoundOS must only execute administrator-controlled apps.
	if err := checkTrustedPath(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := make([]AppSpec, 0)
	seen := map[string]bool{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if len(out) >= 12 {
			return nil, fmt.Errorf("maximum of 12 optional apps")
		}
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("manifest must be regular file: %s", entry.Name())
		}
		file := filepath.Join(root, entry.Name())
		fi, e := os.Lstat(file)
		if e != nil {
			return nil, e
		}
		if fi.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("manifest writable by group or other: %s", entry.Name())
		}
		if os.Geteuid() == 0 && !rootOwned(fi) {
			return nil, fmt.Errorf("manifest must be root-owned: %s", entry.Name())
		}
		if fi.Size() > 4096 {
			return nil, fmt.Errorf("manifest too large")
		}
		b, e := os.ReadFile(file)
		if e != nil {
			return nil, e
		}
		var a AppSpec
		if e = json.Unmarshal(b, &a); e != nil {
			return nil, fmt.Errorf("manifest %s: %w", entry.Name(), e)
		}
		if !validAppID.MatchString(a.ID) || a.ID == "org.roundos.clock" || a.ID == "org.roundos.timer" || seen[a.ID] {
			return nil, fmt.Errorf("invalid or duplicate app id in %s", entry.Name())
		}
		if len(a.Name) == 0 || len([]rune(a.Name)) > 15 || strings.ContainsAny(a.Name, "\n\r\t") || !validIcon.MatchString(a.Icon) {
			return nil, fmt.Errorf("invalid app title/icon in %s", entry.Name())
		}
		if a.Exec == "" || filepath.IsAbs(a.Exec) {
			return nil, fmt.Errorf("exec must be relative to manifest directory")
		}
		resolved, e := filepath.EvalSymlinks(filepath.Join(root, a.Exec))
		if e != nil {
			return nil, e
		}
		rel, e := filepath.Rel(root, resolved)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("executable escapes trusted app directory")
		}
		info, e := os.Stat(resolved)
		if e != nil {
			return nil, e
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 {
			return nil, fmt.Errorf("invalid executable permissions for %s", entry.Name())
		}
		if os.Geteuid() == 0 && !rootOwned(info) {
			return nil, fmt.Errorf("executable must be root-owned: %s", entry.Name())
		}
		a.Exec = resolved
		out = append(out, a)
		seen[a.ID] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func rootOwned(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Uid == 0
}
func checkTrustedPath(dir string) error {
	for path := dir; ; path = filepath.Dir(path) {
		st, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !st.IsDir() {
			return fmt.Errorf("unsafe application directory or ancestor: %s", path)
		}
		if st.Mode().Perm()&0022 != 0 && !(st.Mode()&os.ModeSticky != 0 && rootOwned(st)) {
			return fmt.Errorf("unsafe writable application directory or ancestor: %s", path)
		}
		if os.Geteuid() == 0 && !rootOwned(st) {
			return fmt.Errorf("application directory or ancestor is not root owned: %s", path)
		}
		if path == filepath.Dir(path) {
			break
		}
	}
	return nil
}

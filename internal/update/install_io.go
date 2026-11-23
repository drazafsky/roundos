package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var ErrInstallBusy = errors.New("an update installation is already in progress")

func writeInstallJSON(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 256*1024 {
		return errors.New("installation state exceeds size limit")
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".update-state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), file); err != nil {
		return err
	}
	return syncStateDirectory(filepath.Dir(file))
}

func readInstallJSON(file string, value any) error {
	if err := privateFile(file); err != nil {
		return err
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 256*1024 || len(data) == 0 {
		return errors.New("invalid installation state size")
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("installation state must be an object")
	}
	return json.Unmarshal(data, value)
}

func privateFile(file string) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0077 != 0 {
		return errors.New("installation state must be a private owned regular file")
	}
	return nil
}

func (i *Installer) lock() (*os.File, error) {
	if err := prepareCache(i.stateDir); err != nil {
		return nil, err
	}
	file := filepath.Join(i.stateDir, "update-install.lock")
	// flock works on a read-only descriptor. Existing recovery records must
	// remain usable when /var becomes read-only after an interrupted update.
	f, err := os.OpenFile(file, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		f, err = os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
		if errors.Is(err, os.ErrExist) {
			f, err = os.OpenFile(file, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := privateFile(file); err != nil {
		f.Close()
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrInstallBusy
		}
		return nil, err
	}
	return f, nil
}

func atomicInstallLink(root, name, target string) error {
	f, err := os.CreateTemp(root, ".update-link-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	f.Close()
	defer os.Remove(temporary)
	if err := os.Remove(temporary); err != nil {
		return err
	}
	if err := os.Symlink(target, temporary); err != nil {
		return err
	}
	if err := os.Rename(temporary, filepath.Join(root, name)); err != nil {
		return err
	}
	return syncStateDirectory(root)
}

func (i *Installer) releasePath(name string) bool {
	base := filepath.Base(name)
	if filepath.Dir(name) != filepath.Join(i.root, "releases") || base == "" || base[0] < '0' || base[0] > '9' {
		return false
	}
	return strings.Trim(base, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz.+-") == ""
}

func (i *Installer) releaseLink(name string, optional bool) (string, error) {
	file := filepath.Join(i.root, name)
	info, err := os.Lstat(file)
	if errors.Is(err, os.ErrNotExist) && optional {
		return "", nil
	}
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("invalid managed %s link", name)
	}
	target, err := filepath.EvalSymlinks(file)
	if err != nil || !i.releasePath(target) {
		return "", fmt.Errorf("untrusted managed %s release", name)
	}
	if err := trustedInstalledPath(target, true); err != nil {
		return "", err
	}
	return target, nil
}

func trustedInstalledPath(file string, directory bool) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 ||
		(directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return errors.New("untrusted installed release file or directory")
	}
	return nil
}

func unitDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func replaceUnit(file string, data []byte) error {
	if err := trustedInstalledPath(filepath.Dir(file), true); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".roundos-unit-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0644); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), file); err != nil {
		return err
	}
	return syncStateDirectory(filepath.Dir(file))
}

func syncInstalledRelease(ctx context.Context, root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := trustedInstalledPath(path, entry.IsDir()); err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		return errors.Join(file.Sync(), file.Close())
	})
	if err != nil {
		return err
	}
	for n := len(dirs) - 1; n >= 0; n-- {
		if err := syncStateDirectory(dirs[n]); err != nil {
			return err
		}
	}
	return syncStateDirectory(filepath.Dir(root))
}

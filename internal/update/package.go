package update

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	maxArchiveBytes = 128 << 20
	maxExtractBytes = 512 << 20
	maxArchiveFiles = 4096
	maxManifestSize = 1 << 20
)

var checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Package struct {
	Root    string
	Version string
	Commit  string
	stage   string
}

func (p *Package) Cleanup() error { return os.RemoveAll(p.stage) }

// Stager downloads only the selected repository release and validates it in a
// private cache before any installed release or service is changed.
type Stager struct {
	http   *http.Client
	probe  func(context.Context, *Package) error
	limits extractionLimits
}

type extractionLimits struct {
	bytes   int64
	objects int
}

func NewStager() *Stager {
	return &Stager{
		http: &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			host := req.URL.Host
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.User != nil ||
				(host != "github.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com") {
				return errors.New("unexpected release asset redirect")
			}
			return nil
		}},
		probe:  verifyPackageRuntime,
		limits: extractionLimits{bytes: maxExtractBytes, objects: maxArchiveFiles},
	}
}

func (s *Stager) Stage(ctx context.Context, release Release, cache string) (_ *Package, err error) {
	v, err := parseVersion(release.Version)
	if err != nil || v.pre != "" || release.ID <= 0 || strings.HasPrefix(release.Version, "v") || strings.TrimPrefix(release.Tag, "v") != release.Version {
		return nil, errors.New("invalid stable package selection")
	}
	name := "roundos-" + release.Version + "-linux-armv7"
	limits := map[string]int64{name + ".tar.gz": maxArchiveBytes, "release.json": maxManifestSize, "SHA256SUMS": maxManifestSize}
	assets := make(map[string]Asset)
	for _, asset := range release.Assets {
		limit, ok := limits[asset.Name]
		if !ok || asset.Size <= 0 || asset.Size > limit || !validAssetURL(asset.URL, release.Tag, asset.Name) || !validAssetDigest(asset.Digest) {
			return nil, ErrIncompatibleRelease
		}
		if _, duplicate := assets[asset.Name]; duplicate {
			return nil, errors.New("duplicate selected release asset")
		}
		assets[asset.Name] = asset
	}
	if len(assets) != len(limits) {
		return nil, ErrIncompatibleRelease
	}
	cache, err = filepath.Abs(cache)
	if err != nil {
		return nil, err
	}
	if err := prepareCache(cache); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(cache, ".download-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(stage)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	for _, assetName := range []string{"SHA256SUMS", "release.json", name + ".tar.gz"} {
		if err := s.download(ctx, assets[assetName], filepath.Join(stage, assetName)); err != nil {
			return nil, err
		}
		if err := verifyFile(ctx, filepath.Join(stage, assetName), strings.TrimPrefix(assets[assetName].Digest, "sha256:")); err != nil {
			return nil, err
		}
	}
	outer, err := readChecksums(filepath.Join(stage, "SHA256SUMS"))
	if err != nil || len(outer) != 2 {
		return nil, errors.New("invalid release asset checksums")
	}
	for _, assetName := range []string{"release.json", name + ".tar.gz"} {
		if err := verifyFile(ctx, filepath.Join(stage, assetName), outer[assetName]); err != nil {
			return nil, err
		}
	}
	var metadata struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		Target  string `json:"target"`
	}
	data, err := os.ReadFile(filepath.Join(stage, "release.json"))
	if err != nil || json.Unmarshal(data, &metadata) != nil || metadata.Version != release.Version || metadata.Target != "linux-armv7" || !commitPattern.MatchString(metadata.Commit) {
		return nil, errors.New("release package metadata mismatch")
	}
	root := filepath.Join(stage, name)
	if err := extractPackage(ctx, filepath.Join(stage, name+".tar.gz"), stage, name, s.limits); err != nil {
		return nil, err
	}
	if err := verifyContents(ctx, root, data, release.Version); err != nil {
		return nil, err
	}
	p := &Package{Root: root, Version: metadata.Version, Commit: metadata.Commit, stage: stage}
	if err := s.probe(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func prepareCache(cache string) error {
	var missing []string
	for dir := cache; ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !info.IsDir() || !ok || (stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) ||
				(info.Mode().Perm()&0022 != 0 && !(info.Mode()&os.ModeSticky != 0 && stat.Uid == 0)) {
				return errors.New("untrusted release cache directory or ancestor")
			}
		} else {
			missing = append(missing, dir)
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	// Create exclusively beneath the already verified ancestors. Never accept
	// a component installed by another process between validation and creation.
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(cache)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("release cache must be a private directory")
	}
	return nil
}

func (s *Stager) download(ctx context.Context, asset Asset, destination string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "RoundOS-Updater")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned HTTP %d", asset.Name, resp.StatusCode)
	}
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(resp.Body, asset.Size+1))
	if err != nil {
		return err
	}
	if n != asset.Size {
		return fmt.Errorf("release asset %s has unexpected size", asset.Name)
	}
	return f.Close()
}

func readChecksums(file string) (map[string]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxManifestSize+1))
	if err != nil || len(data) > maxManifestSize {
		return nil, errors.New("package checksum manifest exceeds limit")
	}
	checksums := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		sum, name, ok := strings.Cut(line, "  ")
		if !ok || !checksumPattern.MatchString(sum) || !safeMember(name) || checksums[name] != "" {
			return nil, errors.New("invalid package checksum manifest")
		}
		checksums[name] = sum
	}
	return checksums, nil
}

func verifyFile(ctx context.Context, file, expected string) error {
	if !checksumPattern.MatchString(expected) {
		return errors.New("missing package checksum")
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, contextReader{ctx, f}); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("package checksum mismatch: %s", filepath.Base(file))
	}
	return nil
}

func safeMember(name string) bool {
	return name != "" && name != "." && !path.IsAbs(name) && path.Clean(name) == name &&
		name != ".." && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\\r\n\x00")
}

func extractPackage(ctx context.Context, archive, stage, root string, limits extractionLimits) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	compressed := bufio.NewReader(f)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	// Count every decompressed byte, including metadata consumed internally by
	// archive/tar, not only the regular-file payloads returned by Next.
	budget := &io.LimitedReader{R: contextReader{ctx, gz}, N: limits.bytes + 1}
	reader := tar.NewReader(budget)
	seen := make(map[string]bool)
	objects := make(map[string]bool)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if budget.N == 0 {
			return errors.New("package decompression exceeds size limit")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !safeMember(name) || (name != root && !strings.HasPrefix(name, root+"/")) || seen[name] || len(seen) >= maxArchiveFiles {
			return errors.New("unsafe or duplicate package archive path")
		}
		seen[name] = true
		// MkdirAll may create parents that have no explicit archive header.
		// Include those directories in the filesystem-object budget.
		for object := name; ; object = path.Dir(object) {
			if !objects[object] {
				if len(objects) >= limits.objects {
					return errors.New("package filesystem object limit exceeded")
				}
				objects[object] = true
			}
			if object == root {
				break
			}
		}
		destination := filepath.Join(stage, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 {
				return errors.New("invalid package directory entry")
			}
			if err := os.MkdirAll(destination, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if name == root || header.Size < 0 || header.Size > maxExtractBytes-total ||
				((name == root+"/SHA256SUMS" || name == root+"/release.json") && header.Size > maxManifestSize) ||
				(name == root+"/RELEASE_VERSION" && header.Size > 256) {
				return errors.New("package extraction exceeds size limit")
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600|os.FileMode(header.Mode)&0111)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(file, contextReader{ctx, reader}, header.Size)
			closeErr := file.Close()
			if budget.N == 0 {
				return errors.New("package decompression exceeds size limit")
			}
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
		default:
			return errors.New("package contains links or special files")
		}
	}
	// Drain bounded zero padding to validate gzip's footer and reject appended
	// streams or arbitrary data after the tar end marker.
	padding, err := io.ReadAll(io.LimitReader(budget, maxManifestSize+1))
	if budget.N == 0 {
		return errors.New("package decompression exceeds size limit")
	}
	if err != nil || len(padding) > maxManifestSize || len(bytes.Trim(padding, "\x00")) != 0 {
		return errors.New("invalid package archive trailer")
	}
	if _, err := compressed.Peek(1); err != io.EOF {
		return errors.New("unexpected data after compressed package")
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func verifyContents(ctx context.Context, root string, metadata []byte, version string) error {
	checksums, err := readChecksums(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return err
	}
	count := 0
	err = filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "SHA256SUMS" {
			return nil
		}
		count++
		return verifyFile(ctx, file, checksums[name])
	})
	if err != nil || count != len(checksums) {
		return errors.Join(errors.New("package file manifest mismatch"), err)
	}
	for _, required := range []string{"README.md", "systemd/roundos.service", "scripts/install-release.sh", "scripts/setup-app-user.sh", "scripts/roundosctl.sh", "config/gestures.example.json"} {
		if checksums[required] == "" {
			return fmt.Errorf("package is missing required installer file: %s", required)
		}
	}
	for _, required := range []string{"roundd", "round-clock", "round-timer", "round-demo", "roundui", "scripts/install-release.sh", "scripts/setup-app-user.sh", "scripts/roundosctl.sh"} {
		info, err := os.Stat(filepath.Join(root, required))
		if err != nil || info.Mode().Perm()&0100 == 0 {
			return fmt.Errorf("package input is not executable: %s", required)
		}
	}
	inside, err := os.ReadFile(filepath.Join(root, "release.json"))
	if err != nil || !bytes.Equal(inside, metadata) {
		return errors.New("archive release metadata mismatch")
	}
	identity, err := os.ReadFile(filepath.Join(root, "RELEASE_VERSION"))
	if err != nil || string(identity) != version+"\n" {
		return errors.New("archive release version mismatch")
	}
	for _, binary := range []string{"roundd", "round-clock", "round-timer", "round-demo", "roundui"} {
		if err := verifyARMExecutable(filepath.Join(root, binary)); err != nil {
			return err
		}
	}
	return nil
}

func verifyARMExecutable(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if f.Class != elf.ELFCLASS32 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_ARM ||
		(f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN) || f.Version != elf.EV_CURRENT || f.Entry == 0 {
		return errors.New("package binary is not an ARM loadable executable")
	}
	entry := false
	for _, p := range f.Progs {
		if p.Type != elf.PT_LOAD {
			continue
		}
		if p.Filesz > p.Memsz || p.Off > uint64(info.Size()) || p.Filesz > uint64(info.Size())-p.Off {
			return errors.New("truncated package executable segment")
		}
		if p.Flags&elf.PF_X != 0 && f.Entry >= p.Vaddr && f.Entry-p.Vaddr < p.Memsz {
			entry = true
		}
	}
	if !entry {
		return errors.New("package executable has no loadable entry point")
	}
	return nil
}

type probeOutput struct{ buffer bytes.Buffer }

func (b *probeOutput) Write(p []byte) (int, error) {
	if len(p) > 1024-b.buffer.Len() {
		return 0, errors.New("package probe output exceeds limit")
	}
	return b.buffer.Write(p)
}

func packageProbe(ctx context.Context, binary, flag string) (string, error) {
	cmd := exec.CommandContext(ctx, binary, flag)
	cmd.WaitDelay = time.Second
	var out probeOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := runUpdateCommand(cmd)
	return out.buffer.String(), err
}

func verifyPackageRuntime(ctx context.Context, p *Package) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "arm" {
		return errors.New("installation requires a supported 32-bit ARM Linux device")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	binary := filepath.Join(p.Root, "roundd")
	version, err := packageProbe(ctx, binary, "-version")
	if err != nil || version != "RoundOS "+p.Version+" ("+p.Commit+")\n" {
		return errors.New("staged executable release identity mismatch")
	}
	protocol, err := packageProbe(ctx, binary, "-startup-protocol")
	if err != nil || protocol != "sd-notify-v1\n" {
		return errors.New("staged executable lacks managed readiness support")
	}
	return nil
}

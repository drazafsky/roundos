package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const fixtureRoot = "roundos-0.6.0-linux-armv7"

// Structurally loadable ELF32 ARM fixture; never executed by the host tests.
func fixtureELF() []byte {
	b := make([]byte, 84)
	copy(b, []byte{0x7f, 'E', 'L', 'F', 1, 1, 1})
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(b[off:], v) }
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }
	put16(16, 2)
	put16(18, 40)
	put32(20, 1)
	put32(24, 0x10000)
	put32(28, 52)
	put16(40, 52)
	put16(42, 32)
	put16(44, 1)
	put32(52, 1)
	put32(60, 0x10000)
	put32(68, uint32(len(b)))
	put32(72, uint32(len(b)))
	put32(76, 5)
	put32(80, 0x10000)
	return b
}

func TestStagePinsDiscoveryDigests(t *testing.T) {
	f := makePackageFixture(t, nil)
	selected := f.release
	// Replace a complete, otherwise self-consistent release under the same tag.
	f.assets["release.json"][0] ^= 1
	f.refresh()
	s := fixtureStager(t, f)
	_, err := s.Stage(context.Background(), selected, filepath.Join(t.TempDir(), "cache"))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("selected identity was not pinned: %v", err)
	}
}

func TestStageSmallValidArchiveExceedsInjectedBudget(t *testing.T) {
	f := makePackageFixture(t, func(f *packageFixture) { f.files["extra"] = bytes.Repeat([]byte{'x'}, 32*1024) })
	s := fixtureStager(t, f)
	s.limits.bytes = 16 * 1024
	_, err := s.Stage(context.Background(), f.release, filepath.Join(t.TempDir(), "cache"))
	if err == nil || !strings.Contains(err.Error(), "decompression") {
		t.Fatalf("expected byte budget rejection: %v", err)
	}
}

func TestChecksumCancellationAndProbeOutputLimit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("fixture"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyFile(ctx, file, strings.Repeat("a", 64)); !errors.Is(err, context.Canceled) {
		t.Fatalf("checksum cancellation: %v", err)
	}
	var out probeOutput
	if _, err := out.Write(make([]byte, 1025)); err == nil || out.buffer.Len() > 1024 {
		t.Fatal("probe output is unbounded")
	}
	script := filepath.Join(t.TempDir(), "probe.sh")
	if err := os.WriteFile(script, []byte("printf '%2048s' x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := packageProbe(context.Background(), "/bin/sh", script)
	if err == nil || len(output) > 1024 {
		t.Fatalf("subprocess output bypassed limit: size=%d, error=%v", len(output), err)
	}
}

type packageFixture struct {
	files       map[string][]byte
	extra       []tar.Header
	corruptFile string
	inner       *string
	assets      map[string][]byte
	modes       map[string]int64
	release     Release
}

func checksumsFor(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		fmt.Fprintf(&out, "%x  %s\n", sum, name)
	}
	return []byte(out.String())
}

func makePackageFixture(t *testing.T, change func(*packageFixture)) *packageFixture {
	t.Helper()
	metadata, _ := json.Marshal(map[string]any{"version": "0.6.0", "commit": strings.Repeat("a", 40), "target": "linux-armv7", "hardware_validated": false})
	f := &packageFixture{files: map[string][]byte{
		"release.json": metadata, "RELEASE_VERSION": []byte("0.6.0\n"), "README.md": []byte("RoundOS fixture\n"),
		"systemd/roundos.service": []byte("[Service]\nType=notify\n"), "config/gestures.example.json": []byte("{}"),
		"scripts/install-release.sh": []byte("#!/bin/sh\n"), "scripts/setup-app-user.sh": []byte("#!/bin/sh\n"), "scripts/roundosctl.sh": []byte("#!/bin/sh\n"),
	}}
	f.modes = make(map[string]int64)
	for _, name := range []string{"roundd", "round-clock", "round-timer", "round-demo", "roundui"} {
		f.files[name] = fixtureELF()
	}
	if change != nil {
		change(f)
	}
	manifest := checksumsFor(f.files)
	if f.inner != nil {
		manifest = []byte(*f.inner)
	}
	f.files["SHA256SUMS"] = manifest
	if f.corruptFile != "" {
		f.files[f.corruptFile] = append(f.files[f.corruptFile], '!')
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: fixtureRoot + "/", Typeflag: tar.TypeDir, Mode: 0755})
	names := make([]string, 0, len(f.files))
	for name := range f.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := f.files[name]
		mode := int64(0755)
		if value, ok := f.modes[name]; ok {
			mode = value
		}
		if err := tw.WriteHeader(&tar.Header{Name: fixtureRoot + "/" + name, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, header := range f.extra {
		if err := tw.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.assets = map[string][]byte{fixtureRoot + ".tar.gz": archive.Bytes(), "release.json": metadata}
	f.refresh()
	return f
}

func (f *packageFixture) refresh() {
	files := map[string][]byte{fixtureRoot + ".tar.gz": f.assets[fixtureRoot+".tar.gz"], "release.json": f.assets["release.json"]}
	f.assets["SHA256SUMS"] = checksumsFor(files)
	f.release = Release{ID: 123, Version: "0.6.0", Tag: "v0.6.0"}
	for _, name := range []string{fixtureRoot + ".tar.gz", "release.json", "SHA256SUMS"} {
		sum := sha256.Sum256(f.assets[name])
		f.release.Assets = append(f.release.Assets, Asset{Name: name, Digest: fmt.Sprintf("sha256:%x", sum), Size: int64(len(f.assets[name])), URL: "https://github.com/drazafsky/roundos/releases/download/v0.6.0/" + name})
	}
}

type rewriteTransport struct{ target *url.URL }

func (r rewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	address := *request.URL
	address.Scheme, address.Host = r.target.Scheme, r.target.Host
	copy.URL = &address
	return http.DefaultTransport.RoundTrip(copy)
}

func fixtureStager(t *testing.T, f *packageFixture) *Stager {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := f.assets[path.Base(r.URL.Path)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	s := NewStager()
	s.http.Transport = rewriteTransport{target}
	s.probe = func(ctx context.Context, p *Package) error {
		if p.Version != "0.6.0" || p.Commit != strings.Repeat("a", 40) {
			t.Fatalf("probe received wrong package identity: %+v", p)
		}
		return nil
	}
	return s
}

func TestStageVerifiedPackageAndCleanup(t *testing.T) {
	f := makePackageFixture(t, nil)
	s := fixtureStager(t, f)
	cache := filepath.Join(t.TempDir(), "cache")
	p, err := s.Stage(context.Background(), f.release, cache)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(p.Root, "RELEASE_VERSION")); err != nil || string(data) != "0.6.0\n" {
		t.Fatalf("staged release: %s, %v", data, err)
	}
	if info, err := os.Stat(p.stage); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("unsafe stage permissions: %v, %v", info, err)
	}
	if err := p.Cleanup(); err != nil {
		t.Fatal(err)
	}
	assertEmptyCache(t, cache)
}

func TestStageRejectsUnsafeArchives(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*packageFixture)
	}{
		{"traversal", func(f *packageFixture) {
			f.extra = []tar.Header{{Name: fixtureRoot + "/../escaped", Typeflag: tar.TypeReg}}
		}},
		{"absolute path", func(f *packageFixture) { f.extra = []tar.Header{{Name: "/outside", Typeflag: tar.TypeReg}} }},
		{"other root", func(f *packageFixture) { f.extra = []tar.Header{{Name: "other/file", Typeflag: tar.TypeReg}} }},
		{"symlink", func(f *packageFixture) {
			f.extra = []tar.Header{{Name: fixtureRoot + "/link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}}
		}},
		{"hardlink", func(f *packageFixture) {
			f.extra = []tar.Header{{Name: fixtureRoot + "/link", Typeflag: tar.TypeLink, Linkname: fixtureRoot + "/roundd"}}
		}},
		{"special file", func(f *packageFixture) { f.extra = []tar.Header{{Name: fixtureRoot + "/fifo", Typeflag: tar.TypeFifo}} }},
		{"duplicate", func(f *packageFixture) {
			f.extra = []tar.Header{{Name: fixtureRoot + "/roundd", Typeflag: tar.TypeReg}}
		}},
		{"version too large", func(f *packageFixture) { f.files["RELEASE_VERSION"] = bytes.Repeat([]byte{'v'}, 257) }},
		{"wrong architecture", func(f *packageFixture) { f.files["roundd"][4] = 2 }},
		{"relocatable app", func(f *packageFixture) { f.files["round-clock"][16] = 1 }},
		{"truncated app", func(f *packageFixture) { f.files["round-timer"] = f.files["round-timer"][:64] }},
		{"no loadable entry", func(f *packageFixture) { f.files["round-timer"][24] = 255 }},
		{"nonexecutable helper", func(f *packageFixture) { f.modes["scripts/setup-app-user.sh"] = 0644 }},
		{"nonexecutable app", func(f *packageFixture) { f.modes["round-clock"] = 0644 }},
		{"missing installer", func(f *packageFixture) { delete(f.files, "scripts/install-release.sh") }},
		{"inner checksum mismatch", func(f *packageFixture) { f.corruptFile = "README.md" }},
		{"wrong version", func(f *packageFixture) { f.files["RELEASE_VERSION"] = []byte("0.7.0\n") }},
		{"wrong metadata", func(f *packageFixture) { f.files["release.json"] = []byte("{}") }},
		{"unsafe checksum path", func(f *packageFixture) { value := strings.Repeat("a", 64) + "  ../outside\n"; f.inner = &value }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := makePackageFixture(t, tc.change)
			s := fixtureStager(t, f)
			cache := filepath.Join(t.TempDir(), "cache")
			if _, err := s.Stage(context.Background(), f.release, cache); err == nil {
				t.Fatal("unsafe archive was staged")
			}
			assertEmptyCache(t, cache)
		})
	}
}

func TestStageRejectsCorruptAssetsAndTrailers(t *testing.T) {
	for _, kind := range []string{"checksum", "size", "gzip footer", "appended stream", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			f := makePackageFixture(t, nil)
			archive := f.assets[fixtureRoot+".tar.gz"]
			switch kind {
			case "checksum":
				archive[20] ^= 1
			case "size":
				f.release.Assets[0].Size++
			case "gzip footer":
				archive[len(archive)-8] ^= 1
				f.refresh()
			case "appended stream":
				f.assets[fixtureRoot+".tar.gz"] = append(archive, []byte("extra")...)
				f.refresh()
			case "metadata":
				f.assets["release.json"] = []byte(`{"version":"0.6.0","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target":"linux-arm64"}`)
				f.refresh()
			}
			s := fixtureStager(t, f)
			cache := filepath.Join(t.TempDir(), "cache")
			if _, err := s.Stage(context.Background(), f.release, cache); err == nil {
				t.Fatal("corrupt assets were staged")
			}
			assertEmptyCache(t, cache)
		})
	}
}

func TestStageRejectsUntrustedSelectionAndCache(t *testing.T) {
	f := makePackageFixture(t, nil)
	s := fixtureStager(t, f)
	for _, mutate := range []func(*Release){
		func(r *Release) { r.Version = "0.6.0-rc.1" },
		func(r *Release) { r.Assets = r.Assets[:2] },
		func(r *Release) { r.Assets[0].Size = maxArchiveBytes + 1 },
		func(r *Release) { r.Assets[0].URL = "https://example.com/package" },
	} {
		r := f.release
		r.Assets = append([]Asset(nil), r.Assets...)
		mutate(&r)
		if _, err := s.Stage(context.Background(), r, filepath.Join(t.TempDir(), "cache")); err == nil {
			t.Fatal("accepted untrusted package selection")
		}
	}
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	if err := os.Symlink(dir, cache); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(context.Background(), f.release, cache); err == nil {
		t.Fatal("accepted symlink cache")
	}
}

func TestStageCancellationAndProbeFailureCleanUp(t *testing.T) {
	f := makePackageFixture(t, nil)
	s := fixtureStager(t, f)
	cache := filepath.Join(t.TempDir(), "cache")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Stage(ctx, f.release, cache); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not honored: %v", err)
	}
	assertEmptyCache(t, cache)
	probeFailure := errors.New("startup protocol mismatch")
	s.probe = func(context.Context, *Package) error { return probeFailure }
	if _, err := s.Stage(context.Background(), f.release, cache); !errors.Is(err, probeFailure) {
		t.Fatalf("probe failure not returned: %v", err)
	}
	assertEmptyCache(t, cache)
}

func TestStageBudgetsIncludeHiddenMetadataAndImplicitDirectories(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits extractionLimits
		header tar.Header
		want   string
	}{
		{"metadata", extractionLimits{bytes: 16 * 1024, objects: maxArchiveFiles}, tar.Header{Name: fixtureRoot + "/extra", Typeflag: tar.TypeReg, PAXRecords: map[string]string{"comment": strings.Repeat("x", 32*1024)}}, "decompression"},
		{"implicit directories", extractionLimits{bytes: maxExtractBytes, objects: 24}, tar.Header{Name: fixtureRoot + "/a/b/c/d/e/f/g/h/leaf", Typeflag: tar.TypeReg}, "filesystem object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := makePackageFixture(t, func(f *packageFixture) { f.extra = []tar.Header{tc.header} })
			s := fixtureStager(t, f)
			s.limits = tc.limits
			cache := filepath.Join(t.TempDir(), "cache")
			if _, err := s.Stage(context.Background(), f.release, cache); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("archive budget was not enforced: %v", err)
			}
			assertEmptyCache(t, cache)
		})
	}
}

func TestReleaseAssetRedirectPolicy(t *testing.T) {
	s := NewStager()
	for _, address := range []string{"http://github.com/file", "https://example.com/file", "https://user@github.com/file"} {
		req, _ := http.NewRequest("GET", address, nil)
		if err := s.http.CheckRedirect(req, nil); err == nil {
			t.Fatalf("accepted redirect %s", address)
		}
	}
	for _, address := range []string{"https://github.com/file", "https://release-assets.githubusercontent.com/file?token=example", "https://objects.githubusercontent.com/file"} {
		req, _ := http.NewRequest("GET", address, nil)
		if err := s.http.CheckRedirect(req, nil); err != nil {
			t.Fatalf("rejected GitHub asset redirect: %v", err)
		}
	}
}

func assertEmptyCache(t *testing.T, cache string) {
	t.Helper()
	entries, err := os.ReadDir(cache)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("incomplete package was retained: %v", entries)
	}
}

package compile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"roundos/sdk/rui/bundle"
)

func input(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "testdata", "valid", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestCompileFixtureDeterministically(t *testing.T) {
	for _, name := range []string{"minimal.rui.yaml", "timer.rui.yaml", "all-widgets.rui.yaml"} {
		t.Run(name, func(t *testing.T) {
			src := input(t, name)
			a, f, e := Compile(src)
			if e != nil {
				t.Fatal(e)
			}
			b, g, e := Compile(src)
			if e != nil || !bytes.Equal(a, b) {
				t.Fatal("nonreproducible bundle", e)
			}
			if !bytes.Equal(a[:4], []byte("RUI2")) {
				t.Fatal("bad magic")
			}
			verified, e := Verify(a)
			if e != nil {
				t.Fatal(e)
			}
			if verified.SourceSHA256 != g.SourceSHA256 || f.SourceMap["application"].Line == 0 {
				t.Fatal("missing manifest or sourcemap")
			}
			var temp bundle.File
			if e = json.Unmarshal([]byte(`{}`), &temp); e != nil {
				t.Fatal(e)
			}
			goCode, e := GoConstants(f, "layouts")
			if e != nil || !bytes.Contains(goCode, []byte("package layouts")) {
				t.Fatalf("codegen: %v", e)
			}
			goCode2, e := GoConstants(f, "layouts")
			if e != nil || !bytes.Equal(goCode, goCode2) {
				t.Fatal("nonreproducible Go constants")
			}
		})
	}
}
func TestVerifyManifestAndMapTampering(t *testing.T) {
	src := input(t, "timer.rui.yaml")
	_, f, e := Compile(src)
	if e != nil {
		t.Fatal(e)
	}
	delete(f.Manifest.Bindings, "timer.minutes")
	b, e := bundle.Encode(f)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Verify(b); e == nil || !strings.Contains(e.Error(), "manifest") {
		t.Fatalf("accepted tampered manifest: %v", e)
	}
	_, f, _ = Compile(src)
	delete(f.SourceMap, "application")
	b, _ = bundle.Encode(f)
	if _, e = Verify(b); e == nil || !strings.Contains(e.Error(), "source map") {
		t.Fatalf("accepted tampered source map: %v", e)
	}
}

func TestDependenciesAreStoredVerifiedAndDerivedForLegacyBundles(t *testing.T) {
	_, file, err := Compile(input(t, "timer.rui.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Dependencies["timer.minutes"]) == 0 {
		t.Fatal("compiled dependency indices missing")
	}
	file.Format = bundle.LegacyFormat
	file.Dependencies = nil
	legacy, err := bundle.Encode(file)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(legacy)
	if err != nil || len(verified.Dependencies["timer.minutes"]) == 0 {
		t.Fatal("legacy dependency derivation failed", err)
	}
	_, file, err = Compile(input(t, "timer.rui.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file.Dependencies["timer.minutes"][0].Screen = 99
	tampered, err := bundle.Encode(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(tampered); err == nil || !strings.Contains(err.Error(), "dependencies") {
		t.Fatal("checksum-valid dependency tampering accepted", err)
	}
}
func TestValidationSurvivesCompiler(t *testing.T) {
	matches, _ := filepath.Glob(filepath.Join("..", "testdata", "invalid", "*.rui.yaml"))
	for _, p := range matches {
		src, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e := Compile(src); e == nil {
			t.Fatal("accepted invalid source", p)
		}
	}
}

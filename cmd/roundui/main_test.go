package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEndCLI(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join("..", "..", "layouts", "timer.rui.yaml")
	artifact := filepath.Join(tmp, "timer.rui.bin")
	constants := filepath.Join(tmp, "ids.go")
	png := filepath.Join(tmp, "timer.png")
	var out bytes.Buffer
	for _, args := range [][]string{
		{"validate", source},
		{"compile", "-o", artifact, "-go", constants, "-package", "main", source},
		{"inspect", artifact},
		{"inspect", "-json", artifact},
		{"preview", "-o", png, artifact},
	} {
		out.Reset()
		if e := run(args, &out); e != nil {
			t.Fatalf("%v: %v", args, e)
		}
	}
	if !strings.Contains(out.String(), "480x480") {
		t.Fatal("preview output missing dimensions")
	}
	b, e := os.ReadFile(png)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(b[:8], []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("not a PNG")
	}
	code, e := os.ReadFile(constants)
	if e != nil || !bytes.Contains(code, []byte("RUIBinding")) {
		t.Fatalf("generated Go code: %v", e)
	}
	if e := run([]string{"preview", "-screen", "missing.screen", "-o", filepath.Join(tmp, "bad.png"), artifact}, &out); e == nil {
		t.Fatal("missing screen accepted")
	}
}
func TestInvalidYAMLFailsAndDoesNotWrite(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join("..", "..", "internal", "rui", "testdata", "invalid", "unknown-target.rui.yaml")
	dst := filepath.Join(tmp, "bad.bin")
	err := run([]string{"compile", "-o", dst, source}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), ".rui.yaml:") {
		t.Fatalf("error has no source location: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("compile wrote output on invalid input: %v", err)
	}
}
func TestCorruptBundleCannotPreview(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join("..", "..", "layouts", "clock.rui.yaml")
	artifact := filepath.Join(tmp, "clock.rui.bin")
	if err := run([]string{"compile", "-o", artifact, source}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(artifact)
	b[12] ^= 0xFF
	if err := os.WriteFile(artifact, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"preview", artifact}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("accepted corrupt bundle: %v", err)
	}
}

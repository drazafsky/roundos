package rui_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This intentionally uses a distinct module, rather than relying on the SDK
// example compiling only as a package within the RoundOS module.
func TestIndependentModuleBuildsPublicSDKExample(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	module := fmt.Sprintf("module example.org/roundos-demo\n\ngo 1.23\n\nrequire roundos v0.0.0\nreplace roundos => %q\n", root)
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"main.go", "layout.rui.bin"} {
		data, err := os.ReadFile(filepath.Join(root, "examples/rui-demo", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "build", "-o", filepath.Join(directory, "demo"), ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("independent SDK build: %v\n%s", err, output)
	}
}

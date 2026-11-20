package ui_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// A user-supplied Go app must compile from a distinct module using only SDK
// imports. Avoid direct imports of roundos/internal/wire in third-party code.
func TestExternalModuleCanBuildAgainstPublicSDK(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a temporary external Go module")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	dir := t.TempDir()
	mod := "module example.org/roundos-app\n\ngo 1.23\n\nrequire roundos v0.0.0\nreplace roundos => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	src := `package main
import ("roundos/sdk/app"; "roundos/sdk/ui")
func main(){ v:=ui.View{Title:"DEMO",Screens:[]ui.ScreenSpec{ui.Screen("root","DEMO","","",ui.Button("b","OK","go"))}}; _=v; _=app.Connect }
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "app"), ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("standalone SDK build: %v: %s", err, output)
	}
}

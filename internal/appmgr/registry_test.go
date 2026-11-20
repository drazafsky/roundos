package appmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createDemo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo-bin"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), []byte(`{"id":"org.roundos.demo","name":"Demo","icon":"DME","exec":"demo-bin"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestDiscoverTrustedManifest(t *testing.T) {
	dir := createDemo(t)
	apps, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "org.roundos.demo" || apps[0].Exec != filepath.Join(dir, "demo-bin") {
		t.Fatalf("unexpected apps: %+v", apps)
	}
}
func TestDiscoverRejectsUnsafeManifest(t *testing.T) {
	dir := createDemo(t)
	if err := os.Chmod(filepath.Join(dir, "demo.json"), 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("world-writable manifest accepted")
	}
	if err := os.Chmod(filepath.Join(dir, "demo.json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.json"), []byte(`{"id":"org.roundos.clock","name":"Wrong","icon":"W","exec":"demo-bin"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("built-in ID override accepted")
	}
}
func TestDiscoverRejectsExecutableEscape(t *testing.T) {
	dir := createDemo(t)
	path := filepath.Join(dir, "demo.json")
	if err := os.WriteFile(path, []byte(`{"id":"org.roundos.demo","name":"Demo","icon":"DME","exec":"../../bin/sh"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("directory escape accepted")
	}
	if err := os.WriteFile(path, []byte(`{"id":"org.roundos.demo","name":"Demo","icon":"DME","exec":"demo-bin"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("writable directory accepted: %v", err)
	}
}
func TestDiscoverMissingDirectoryIsEmpty(t *testing.T) {
	a, err := Discover(filepath.Join(t.TempDir(), "notinstalled"))
	if err != nil || len(a) != 0 {
		t.Fatalf("%v %v", a, err)
	}
}

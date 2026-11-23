package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/wire"
)

func legacySource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/v050-src.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "63a3cd7886b44069c610e04a5debf7bda158477a593690a0eda0b3926e5b2659" {
		t.Fatal("0.5.0 fixture digest mismatch")
	}
	manifest, err := os.ReadFile("../../scripts/m0/BASELINE-SOURCE.SHA256")
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		sha, name, ok := strings.Cut(line, "  ")
		if !ok {
			t.Fatal("invalid baseline source manifest")
		}
		if name == "go.mod" || strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			expected[name] = sha
		}
	}
	root := t.TempDir()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsLocal(header.Name) || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > 512<<10 {
			t.Fatal("invalid compatibility fixture entry")
		}
		body, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if want, ok := expected[header.Name]; !ok || fmt.Sprintf("%x", sha256.Sum256(body)) != want {
			t.Fatalf("modified or unexpected 0.5.0 source: %s", header.Name)
		}
		delete(expected, header.Name)
		path := filepath.Join(root, header.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(expected) != 0 {
		t.Fatal("0.5.0 fixture omitted original Go sources", expected)
	}
	return root
}

func TestUnmodifiedV050AppsAndManagerRemainCompatible(t *testing.T) {
	if testing.Short() {
		t.Skip("builds independently compiled 0.5.0 processes")
	}
	legacy := legacySource(t)
	current, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	clock := build(t, "./cmd/round-clock", "v050-clock", legacy)
	timer := build(t, "./cmd/round-timer", "v050-timer", legacy)
	demo := build(t, "./cmd/round-demo", "v050-demo", legacy)
	t.Run("old-app-with-new-manager", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		const id = "org.example.legacy.demo"
		manager, err := appmgr.NewWithApps(ctx, clock, timer, t.TempDir(), []appmgr.AppSpec{{ID: id, Exec: demo}})
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()
		if err := manager.SetForeground(id); err != nil {
			t.Fatal(err)
		}
		inbox := newEventInbox(manager.Events())
		first := get(t, inbox, id, "view", 5*time.Second)
		if first.LayoutRevision != 0 || wire.UsesTemplates(first.View) || first.View.Screens[0].Subtitle != "OPTIONAL APP" {
			t.Fatal("old binary was upgraded implicitly", first)
		}
		if !manager.UIInput(id, wire.UIEvent{Screen: "demo.root", Widget: "count", Action: "increment", Kind: "press"}) {
			t.Fatal("legacy input adapter rejected old binary")
		}
		next := get(t, inbox, id, "view", 5*time.Second)
		if next.LayoutRevision != 0 || next.View.Screens[0].Subtitle != "COUNTED" {
			t.Fatal("old binary did not receive original input semantics", next)
		}
	})
	t.Run("new-rui-app-with-old-manager", func(t *testing.T) {
		ruiDemo := build(t, "./examples/rui-demo", "new-rui-demo", current)
		dir := filepath.Join(legacy, "cmd/compat-harness")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(legacyManagerHarness), 0600); err != nil {
			t.Fatal(err)
		}
		harness := build(t, "./cmd/compat-harness", "v050-manager-harness", legacy)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, harness, clock, timer, ruiDemo, t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "PASS old-manager: RUI full-view fallback and increment") {
			t.Fatalf("new SDK failed against unmodified old manager: %v\n%s", err, out)
		}
	})
}

// This harness imports only original 0.5.0 packages in the extracted module.
// It adds an observer and sends legacy input; it does not edit those packages.
const legacyManagerHarness = `package main
import (
 "context"
 "fmt"
 "log"
 "os"
 "time"
 "roundos/internal/appmgr"
)
func main() {
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second);defer cancel()
 const id="org.example.rui.demo"
 manager,err:=appmgr.NewWithApps(ctx,os.Args[1],os.Args[2],os.Args[4],[]appmgr.AppSpec{{ID:id,Exec:os.Args[3]}})
 if err!=nil { log.Fatal(err) };defer manager.Close()
 if err:=manager.SetForeground(id);err!=nil { log.Fatal(err) }
 incremented:=false
 for {
  select {
  case event:=<-manager.Events():
   if event.App!=id || event.Kind!="view" { continue }
   if len(event.View.Screens)!=2 { log.Fatal("missing fallback screens") }
   hero:=event.View.Screens[0].Hero
   if !incremented && hero=="00" {
    incremented=true
    if !manager.Input(id,"demo.increment",0) { log.Fatal("input not delivered") }
   } else if incremented && hero=="01" {
    fmt.Println("PASS old-manager: RUI full-view fallback and increment")
    return
   }
  case <-ctx.Done():log.Fatal("legacy manager never received updated full view: ",ctx.Err())
  }
 }
}
`

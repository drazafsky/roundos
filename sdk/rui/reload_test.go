package rui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"roundos/internal/rui/compile"
	"roundos/internal/rui/devproto"
	"roundos/sdk/app"
)

func replacement(t *testing.T, change func([]byte) []byte) []byte {
	t.Helper()
	source, err := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := compile.Compile(change(source))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReplacementIsStagedWithoutActionsOrStateLoss(t *testing.T) {
	_, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "Application state"
	registerDemo(t, r, &count, &enabled, &title)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, old, next string }{
		{"identity", "org.example.rui.demo", "org.example.other"},
		{"binding", "demo.subtitle", "demo.unregistered"},
		{"action", "onPress: demo.increment", "onPress: demo.unregistered"},
		{"dynamic bounds", "max: 99", "max: 4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := replacement(t, func(src []byte) []byte { return bytes.ReplaceAll(src, []byte(test.old), []byte(test.next)) })
			before := r.file
			if err := r.Replace(data); err == nil {
				t.Fatal("invalid replacement accepted")
			}
			if r.file != before || count != 5 || title != "Application state" || pub.count() != 1 {
				t.Fatal("failed staging changed runtime/app/published state")
			}
		})
	}
	if err := r.Replace([]byte("corrupt")); err == nil {
		t.Fatal("corrupt bundle accepted")
	}
	data := replacement(t, func(src []byte) []byte {
		return bytes.Replace(src, []byte("hero: { bind: demo.display }"), []byte("hero: Reloaded"), 1)
	})
	if err := r.Replace(data); err != nil {
		t.Fatal(err)
	}
	if err := r.Invalidate("demo.display"); err != nil {
		t.Fatal("registered binding removed from layout must remain a valid invalidation key", err)
	}
	if count != 5 || title != "Application state" || pub.count() != 2 {
		t.Fatal("replace did not atomically publish while preserving app state")
	}
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	if pub.last().Screens[0].Hero != "Reloaded" {
		t.Fatal("new layout not published")
	}
	if _, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); err != nil || count != 6 {
		t.Fatal("registered handler was lost", err)
	}
}

func TestReplacementPublicationFailureKeepsActiveActions(t *testing.T) {
	_, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "Old layout"
	registerDemo(t, r, &count, &enabled, &title)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	old := r.file
	data := replacement(t, func(source []byte) []byte {
		return bytes.Replace(source, []byte("onPress: demo.increment"), []byte("navigate: demo.settings"), 1)
	})
	pub.fail = true
	if err := r.Replace(data); err == nil {
		t.Fatal("expected publication failure")
	}
	if r.file != old || pub.count() != 1 {
		t.Fatal("publication failure committed new runtime")
	}
	if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); !handled || err != nil || count != 6 {
		t.Fatal("displayed old action stopped working", handled, err, count)
	}
	pub.fail = false
	if err := r.Replace(data); err != nil {
		t.Fatal(err)
	}
	if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); handled || err != nil || count != 6 {
		t.Fatal("removed action remained active after successful publication")
	}
}

func TestReplacementValidatesHiddenWidgetsAndInactiveDialogs(t *testing.T) {
	for _, kind := range []string{"hidden-number", "hidden-label", "inactive-dialog"} {
		t.Run(kind, func(t *testing.T) {
			_, pub, r := loadDemo(t)
			count, enabled, title := 5, false, strings.Repeat("x", 200)
			registerDemo(t, r, &count, &enabled, &title)
			r.Dispatch(app.Event{Type: "visibility", Visible: true})
			if err := r.Flush(); err != nil {
				t.Fatal(err)
			}
			old := r.file
			data := replacement(t, func(source []byte) []byte {
				switch kind {
				case "hidden-number":
					source = bytes.Replace(source, []byte("max: 99"), []byte("max: 4\n            visibleWhen: { bind: demo.enabled }"), 1)
				case "hidden-label":
					source = bytes.Replace(source, []byte("label: Count"), []byte("label: { bind: demo.subtitle }\n            visibleWhen: { bind: demo.enabled }"), 1)
				case "inactive-dialog":
					source = bytes.Replace(source, []byte("title: Reset count?"), []byte("title: { bind: demo.subtitle }"), 1)
				}
				return source
			})
			if err := r.Replace(data); err == nil {
				t.Fatal("invalid unrendered property was accepted")
			}
			if r.file != old || pub.count() != 1 {
				t.Fatal("failed hidden validation changed last-good state")
			}
		})
	}
}

func TestReplacementChecksFutureSnapshotFrameBudgets(t *testing.T) {
	for _, kind := range []string{"hidden-widgets", "inactive-dialog"} {
		t.Run(kind, func(t *testing.T) {
			_, pub, r := loadDemo(t)
			count, enabled, title := 5, false, strings.Repeat("x", 160)
			registerDemo(t, r, &count, &enabled, &title)
			r.Dispatch(app.Event{Type: "visibility", Visible: true})
			if err := r.Flush(); err != nil {
				t.Fatal(err)
			}
			var source strings.Builder
			source.WriteString("schema: roundos.rui/v1\napplication:\n  id: org.example.rui.demo\nscreens:\n")
			for screen := 0; screen < 8; screen++ {
				fmt.Fprintf(&source, "  - id: screen%d\n    title: Test\n    layout: list\n    groups:\n      - id: items\n        widgets:\n", screen)
				for widget := 0; widget < 64; widget++ {
					fmt.Fprintf(&source, "          - id: item%d\n            type: label\n            label: { bind: demo.subtitle }\n", widget)
					if kind == "hidden-widgets" {
						source.WriteString("            visibleWhen: { bind: demo.enabled }\n")
					}
				}
			}
			source.WriteString("dialogs:\n  - id: test.confirm\n    title: Test\n    body: { bind: demo.subtitle }\n    confirmLabel: OK\n    cancelLabel: CANCEL\n    onConfirm: demo.confirmReset\n    onCancel: demo.cancelReset\n")
			data, file, err := compile.Compile([]byte(source.String()))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "inactive-dialog" {
				// Fit the ordinary snapshot just below the frame boundary, where
				// opening the otherwise valid dialog would exceed that boundary.
				for {
					if _, err := buildView(file, stateSnapshot(r.bindings), ""); err == nil {
						break
					}
					screens := file.Document.Screens
					widgets := screens[len(screens)-1].Groups[0].Widgets
					if len(widgets) == 1 {
						file.Document.Screens = screens[:len(screens)-1]
					} else {
						screens[len(screens)-1].Groups[0].Widgets = widgets[:len(widgets)-1]
					}
				}
				if _, err := buildView(file, stateSnapshot(r.bindings), "test.confirm"); err == nil {
					t.Fatal("fixture dialog did not exceed frame budget")
				}
				data, _, err = compile.FromDocument(&file.Document, []byte(source.String()))
				if err != nil {
					t.Fatal(err)
				}
			}
			old := r.file
			if err := r.Replace(data); err == nil || !strings.Contains(err.Error(), "IPC frame") {
				t.Fatalf("future oversized snapshot accepted or unexpected error: %v", err)
			}
			if r.file != old || pub.count() != 1 {
				t.Fatal("frame-budget failure replaced the last-good layout")
			}
		})
	}
}

func TestRemovedActionsStayRegisteredButCannotDispatch(t *testing.T) {
	original, _, r := loadDemo(t)
	count, enabled, title := 5, true, "retained"
	registerDemo(t, r, &count, &enabled, &title)
	data := replacement(t, func(src []byte) []byte {
		return bytes.Replace(src, []byte("onPress: demo.increment"), []byte("navigate: demo.settings"), 1)
	})
	if err := r.Replace(data); err != nil {
		t.Fatal(err)
	}
	if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); handled || err != nil || count != 5 {
		t.Fatal("removed action still dispatched", handled, err, count)
	}
	if err := r.Replace(original); err != nil {
		t.Fatal(err)
	}
	if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); !handled || err != nil || count != 6 {
		t.Fatal("restored registration was lost", handled, err, count)
	}
}

type secureCapture struct {
	capture
	secure bool
}

func (p *secureCapture) SecureEditorActive() bool { return p.secure }
func TestSecureEditorAndPanickingGettersKeepLastLayout(t *testing.T) {
	data, _, _ := loadDemo(t)
	pub := &secureCapture{}
	r, err := NewForApp(pub, data, "org.example.rui.demo")
	if err != nil {
		t.Fatal(err)
	}
	count, enabled, title := 1, true, "valid"
	registerDemo(t, r, &count, &enabled, &title)
	pub.secure = true
	if err := r.Replace(data); !errors.Is(err, ErrReloadDeferred) {
		t.Fatal("secure editor was not deferred", err)
	}
	pub.secure = false
	old := r.file
	r.bindings["demo.subtitle"] = binding{text: func() string { panic("sensitive draft") }, typ: r.bindings["demo.subtitle"].typ}
	if err := r.Replace(data); err == nil || strings.Contains(err.Error(), "sensitive draft") {
		t.Fatal("getter panic not contained/sanitized", err)
	}
	if r.file != old {
		t.Fatal("panic changed layout")
	}
}

func TestPrivateDevelopmentSessionDeliversOnAppLoop(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root development sessions are deliberately refused")
	}
	dir, err := os.MkdirTemp("", "rui-dev-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	t.Setenv("ROUNDOS_RUI_DEV_DIR", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := ListenDev(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	metadata, err := os.ReadFile(session.Path)
	if err != nil {
		t.Fatal(err)
	}
	var address devAddress
	if err := json.Unmarshal(metadata, &address); err != nil {
		t.Fatal(err)
	}
	unauthorized, err := net.Dial("unix", address.Socket)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.SetDeadline(time.Now().Add(time.Second))
	if err := devproto.Write(unauthorized, devproto.Request{Token: "wrong-token", Bundle: []byte("anything")}); err != nil {
		t.Fatal(err)
	}
	if err := devproto.Read(unauthorized, &devproto.Response{}); err == nil {
		t.Fatal("unauthorized control request accepted")
	}
	unauthorized.Close()
	select {
	case <-session.Updates():
		t.Fatal("unauthenticated data reached app loop")
	default:
	}
	data, pub, r := loadDemo(t)
	count, enabled, title := 1, true, "retained"
	registerDemo(t, r, &count, &enabled, &title)
	done := make(chan error, 1)
	go func() { _, err := SendDevBundle(ctx, session.Path, data); done <- err }()
	select {
	case update := <-session.Updates():
		if pub.count() != 0 {
			t.Fatal("transport evaluated or published app state")
		}
		if err := update.Apply(r); err != nil {
			t.Fatal(err)
		}
		if err := update.Apply(r); err == nil {
			t.Fatal("update applied twice")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("update never reached app loop")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(session.Path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SendDevBundle(ctx, session.Path, data); err == nil {
		t.Fatal("nonprivate session file accepted")
	}
}

var _ Publisher = (*secureCapture)(nil)

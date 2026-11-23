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
	"roundos/internal/wire"
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
		{"dialog actions", "onConfirm: demo.confirmReset", "onConfirm: demo.cancelReset"},
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

func TestReplacementRejectsRemappingRegisteredToggleSource(t *testing.T) {
	addGuard := func(source []byte) []byte {
		return bytes.Replace(source, []byte("label: Add One"), []byte("label: Add One\n            visibleWhen: { bind: demo.otherEnabled }"), 1)
	}
	data := replacement(t, addGuard)
	pub := &capture{}
	r, err := NewForApp(pub, data, "org.example.rui.demo")
	if err != nil {
		t.Fatal(err)
	}
	other := true
	if err := r.BindBool("demo.otherEnabled", func() bool { return other }); err != nil {
		t.Fatal(err)
	}
	count, enabled, title := 5, true, "Demo"
	registerDemo(t, r, &count, &enabled, &title)
	r.Dispatch(app.Event{Type: "visibility", Visible: true})
	if err := r.Flush(); err != nil {
		t.Fatal(err)
	}
	changed := replacement(t, func(source []byte) []byte {
		return bytes.Replace(addGuard(source), []byte("checked: { bind: demo.enabled }"), []byte("checked: { bind: demo.otherEnabled }"), 1)
	})
	if err := r.Replace(changed); err == nil || !strings.Contains(err.Error(), "remaps boolean") {
		t.Fatal("registered source remap accepted", err)
	}
	// Remove the entire toggle while retaining the rest of the layout.
	source, _ := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	start := bytes.Index(source, []byte("          - id: demo.enabled"))
	if start < 0 {
		t.Fatal("missing toggle fixture")
	}
	end := bytes.Index(source[start:], []byte("          - id: demo.count")) + start
	if start < 0 || end <= start {
		t.Fatal("missing toggle fixture")
	}
	removed, _, err := compile.Compile(append(addGuard(append([]byte(nil), source[:start]...)), source[end:]...))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Replace(removed); err != nil {
		t.Fatal(err)
	}
	if err := r.Replace(changed); err == nil || !strings.Contains(err.Error(), "remaps boolean") {
		t.Fatal("removed toggle lost session contract", err)
	}
	if err := r.Replace(data); err != nil {
		t.Fatal("original toggle did not restore", err)
	}
	if _, err := r.Dispatch(app.Event{Type: "input", Action: "demo.setEnabled"}); err != nil || enabled || !other {
		t.Fatal("queued legacy toggle lost its original meaning", err)
	}
}

func TestReplacementRetainsRemovedDialogContract(t *testing.T) {
	original, _, r := loadDemo(t)
	count, enabled, title := 5, true, "Demo"
	registerDemo(t, r, &count, &enabled, &title)
	removed := replacement(t, func(source []byte) []byte { return bytes.Split(source, []byte("dialogs:"))[0] })
	if err := r.Replace(removed); err != nil {
		t.Fatal(err)
	}
	changed := replacement(t, func(source []byte) []byte {
		return bytes.Replace(source, []byte("onConfirm: demo.confirmReset"), []byte("onConfirm: demo.cancelReset"), 1)
	})
	if err := r.Replace(changed); err == nil || !strings.Contains(err.Error(), "remaps dialog") {
		t.Fatal("removed dialog lost session contract", err)
	}
	if err := r.Replace(original); err != nil {
		t.Fatal("original dialog did not restore", err)
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
	if handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"}); !handled || err != nil || count != 6 {
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
	legacyCalls := 0
	handled, err := r.Dispatch(app.Event{Type: "input", Action: "demo.increment"})
	if !handled {
		legacyCalls++
	}
	if !handled || err != nil || count != 5 || legacyCalls != 0 {
		t.Fatal("removed action dispatched or escaped into legacy fallback", handled, err, count, legacyCalls)
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

func TestReplacementCanceledDuringStagingKeepsLastLayout(t *testing.T) {
	data, pub, r := loadDemo(t)
	count, enabled, title := 5, true, "retained"
	registerDemo(t, r, &count, &enabled, &title)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	getter := r.bindings["demo.subtitle"]
	getter.text = func() string { cancel(); return title }
	r.bindings["demo.subtitle"] = getter
	old := r.file
	if err := r.ReplaceContext(ctx, data); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled staging was accepted", err)
	}
	if r.file != old || pub.count() != 0 {
		t.Fatal("canceled staging changed active tree or published a snapshot")
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

type textCapture struct {
	capture
	active  bool
	tracked int
}

func (p *textCapture) TextEditorActive() bool       { return p.active }
func (p *textCapture) TrackEditorResults()          { p.tracked++ }
func (p *textCapture) AcknowledgeTextResult(string) { p.active = false }

func TestNonsecureEditorDefersIncompatibleReloadUntilResult(t *testing.T) {
	data, _, _ := loadDemo(t)
	pub := &textCapture{active: true}
	r, err := NewForApp(pub, data, "org.example.rui.demo")
	if err != nil {
		t.Fatal(err)
	}
	count, enabled, title := 5, true, "draft owner"
	registerDemo(t, r, &count, &enabled, &title)
	replacement := replacement(t, func(source []byte) []byte {
		return bytes.Replace(source, []byte("onPress: demo.editTitle"), []byte("navigate: demo.settings"), 1)
	})
	old := r.file
	if err := r.Replace(replacement); !errors.Is(err, ErrReloadDeferred) || r.file != old || pub.count() != 0 {
		t.Fatal("incompatible reload replaced an outstanding editor context", err)
	}
	if handled, err := r.Dispatch(app.Event{Type: "text_result", TextResult: &wire.TextResult{ID: "title", Value: title}}); handled || err != nil {
		t.Fatal("text result did not remain available to the app handler", handled, err)
	}
	if err := r.Replace(replacement); err != nil || pub.count() != 1 {
		t.Fatal("reload did not resume after editor result", err)
	}
}

func TestEditorResultTrackingRequiresDevelopmentOptIn(t *testing.T) {
	data, _, _ := loadDemo(t)
	for _, directory := range []string{"", t.TempDir()} {
		t.Setenv("ROUNDOS_RUI_DEV_DIR", directory)
		pub := &textCapture{}
		if _, err := NewForApp(pub, data, "org.example.rui.demo"); err != nil {
			t.Fatal(err)
		}
		want := 0
		if directory != "" {
			want = 1
		}
		if pub.tracked != want {
			t.Fatal("editor tracking changed outside development opt-in", directory, pub.tracked)
		}
	}
}

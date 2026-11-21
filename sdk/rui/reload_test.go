package rui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	if count != 5 || title != "Application state" || pub.count() != 1 {
		t.Fatal("replace executed effects or published outside Flush")
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

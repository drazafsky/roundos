package rui_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"roundos/internal/appmgr"
	"roundos/internal/rui/compile"
	"roundos/internal/shell"
	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/rui"
)

// A genuine SDK Client and wire protocol v7 connection, with an in-process
// authenticated shell peer. No systemd, DRM device, or Pi input is required.
func TestLiveV7SDKClientIntegration(t *testing.T) {
	src, e := os.ReadFile("../../examples/rui-demo/layout.rui.yaml")
	if e != nil {
		t.Fatal(e)
	}
	binary, _, e := compile.Compile(src)
	if e != nil {
		t.Fatal(e)
	}
	socket := filepath.Join(t.TempDir(), "test.sock")
	listener, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	t.Setenv("ROUNDOS_SOCKET", socket)
	t.Setenv("ROUNDOS_APP_ID", "org.example.rui.demo")
	t.Setenv("ROUNDOS_TOKEN", "test-token")
	results := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			results <- e
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		msg, e := wire.Read(conn)
		if e != nil {
			results <- e
			return
		}
		if msg.Type != "hello" || msg.App != "org.example.rui.demo" || msg.Token != "test-token" {
			results <- errProtocol("unexpected hello handshake")
			return
		}
		if e = wire.Write(conn, wire.Message{Version: 7, Type: "visibility", Visible: true}); e != nil {
			results <- e
			return
		}
		msg, e = wire.Read(conn)
		if e != nil {
			results <- e
			return
		}
		if msg.Type != "view" || msg.View == nil || len(msg.View.Screens) != 2 || msg.View.Screens[0].Hero != "0" {
			results <- errProtocol("incorrect initial RUI view over v7")
			return
		}
		if e = wire.Write(conn, wire.Message{Version: 7, Type: "input", Action: "demo.increment"}); e != nil {
			results <- e
			return
		}
		msg, e = wire.Read(conn)
		if e != nil {
			results <- e
			return
		}
		if msg.Type != "view" || msg.View == nil || msg.View.Screens[0].Hero != "1" {
			results <- errProtocol("incorrect state update over v7")
			return
		}
		model := shell.New()
		model.SetInstalledApps([]appmgr.AppSpec{{ID: "org.example.rui.demo", Name: "RUI Demo", Icon: "RUI"}})
		model.SetView("org.example.rui.demo", *msg.View)
		model.Select(model.PageForApp("org.example.rui.demo"))
		readView := func() (*wire.View, error) {
			message, err := wire.Read(conn)
			if err != nil {
				return nil, err
			}
			if message.Type != "view" || message.View == nil {
				return nil, errProtocol("dialog transition requires a full view")
			}
			model.SetView("org.example.rui.demo", *message.View)
			return message.View, nil
		}
		if e = wire.Write(conn, wire.Message{Version: 7, Type: "input", Action: "demo.requestReset"}); e != nil {
			results <- e
			return
		}
		view, e := readView()
		if e != nil {
			results <- e
			return
		}
		if view.Dialog == nil || model.DialogHidden {
			results <- errProtocol("initial dialog is not visible")
			return
		}
		// Both cancel and confirm handlers reopen the same declaration. The
		// third dismissal closes it. Drive real shell input and inspect the
		// genuine SDK wire frames, including its unchanged-view suppression.
		for i, action := range []string{"demo.cancelReset", "demo.confirmReset", "demo.cancelReset"} {
			if action == "demo.confirmReset" {
				model.Rotate(1)
			} else {
				model.Rotate(-1)
			}
			model.Activate()
			id, pending := model.TakeDialogAction()
			if id != "org.example.rui.demo" || pending != action || !model.DialogHidden {
				results <- errProtocol("reopened dialog could not be dismissed in the shell")
				return
			}
			if e = wire.Write(conn, wire.Message{Version: 7, Type: "input", Action: pending}); e != nil {
				results <- e
				return
			}
			view, e = readView()
			if e != nil {
				results <- e
				return
			}
			if view.Dialog != nil {
				results <- errProtocol("dialog-free transition was not published")
				return
			}
			if i < 2 {
				view, e = readView()
				if e != nil {
					results <- e
					return
				}
				if view.Dialog == nil || view.Dialog.ID != "demo.confirmReset" || model.DialogHidden {
					results <- errProtocol("same-ID dialog stayed hidden after reopening")
					return
				}
			}
		}
		results <- nil
	}()
	client, e := app.Connect()
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	r, e := rui.New(client, binary)
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	enabled := true
	title := "ok"
	cancels := 0
	bind := []func() error{
		func() error { return r.BindString("demo.display", func() string { return string(rune('0' + count)) }) },
		func() error { return r.BindString("demo.subtitle", func() string { return title }) },
		func() error { return r.BindInt("demo.count", func() int { return count }) },
		func() error { return r.BindBool("demo.enabled", func() bool { return enabled }) },
		func() error {
			return r.OnAction("demo.increment", func(context.Context, rui.ActionEvent) error { count++; return nil })
		},
		func() error {
			return r.OnAction("demo.editTitle", func(context.Context, rui.ActionEvent) error { return nil })
		},
		func() error {
			return r.OnAction("demo.requestReset", func(context.Context, rui.ActionEvent) error { return r.ShowDialog("demo.confirmReset") })
		},
		func() error {
			return r.OnAction("demo.confirmReset", func(context.Context, rui.ActionEvent) error { return r.ShowDialog("demo.confirmReset") })
		},
		func() error {
			return r.OnAction("demo.cancelReset", func(context.Context, rui.ActionEvent) error {
				cancels++
				if cancels == 1 {
					return r.ShowDialog("demo.confirmReset")
				}
				return nil
			})
		},
		func() error { return r.OnInt("demo.setCount", func(context.Context, int) error { return nil }) },
		func() error { return r.OnBool("demo.setEnabled", func(context.Context, bool) error { return nil }) },
	}
	for _, fn := range bind {
		if e := fn(); e != nil {
			t.Fatal(e)
		}
	}
	if e := r.ValidateBindings(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 6; i++ {
		select {
		case ev, ok := <-client.Events():
			if !ok {
				t.Fatal("events closed early")
			}
			if handled, e := r.Dispatch(ev); !handled || e != nil {
				t.Fatal(handled, e)
			}
			if e := r.Flush(); e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for v7 input")
		}
	}
	select {
	case e := <-results:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for v7 server")
	}
}

type errProtocol string

func (e errProtocol) Error() string { return string(e) }

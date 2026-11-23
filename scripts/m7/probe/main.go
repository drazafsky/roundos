// A manual acceptance fixture. It is never installed by the release package.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"unicode/utf8"

	"roundos/sdk/app"
	"roundos/sdk/rui"
	"roundos/sdk/ui"
)

//go:generate go run ../../../cmd/roundui compile -o layout.rui.bin layout.rui.yaml
//go:embed layout.rui.bin
var layout []byte

type publisher interface{ Publish(ui.View) error }
type editor interface {
	RequestText(string, string, string, bool) error
}
type probeState struct {
	count          int
	status         string
	enabled, shown bool
}

func newProbe(p publisher, e editor, state *probeState) (*rui.Runtime, error) {
	r, err := rui.NewForApp(p, layout, "org.example.rui.acceptance")
	if err != nil {
		return nil, err
	}
	for _, register := range []func() error{
		func() error { return r.BindString("probe.count", func() string { return fmt.Sprint(state.count) }) },
		func() error { return r.BindString("probe.status", func() string { return state.status }) },
		func() error { return r.BindBool("probe.enabled", func() bool { return state.enabled }) },
		func() error { return r.BindBool("probe.shown", func() bool { return state.shown }) },
		func() error {
			return r.OnAction("increment", func(context.Context, rui.ActionEvent) error { state.count++; return nil })
		},
		func() error {
			return r.OnAction("editPublic", func(context.Context, rui.ActionEvent) error {
				return e.RequestText("public", "Public label", state.status, false)
			})
		},
		func() error {
			return r.OnAction("editSecret", func(context.Context, rui.ActionEvent) error {
				return e.RequestText("secret", "Disposable test text", "", true)
			})
		},
		func() error {
			return r.OnBool("setEnabled", func(_ context.Context, value bool) error { state.enabled = value; return nil })
		},
		func() error {
			return r.OnBool("setShown", func(_ context.Context, value bool) error { state.shown = value; return nil })
		},
	} {
		if err := register(); err != nil {
			return nil, err
		}
	}
	return r, r.ValidateBindings()
}

func (s *probeState) textResult(event app.Event) {
	if event.TextResult == nil {
		return
	}
	result := event.TextResult
	defer func() { result.Value = "" }()
	if result.Canceled {
		return
	}
	switch result.ID {
	case "secret":
		// Never retain, publish, format, or log the secure value. Only completion
		// status is observable; the SDK/shell still own masking and clearing.
		s.status = "SECURE EDIT COMPLETED"
	case "public":
		if utf8.ValidString(result.Value) && utf8.RuneCountInString(result.Value) <= 256 {
			s.status = result.Value
		}
	}
}

func main() {
	client, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	state := &probeState{status: "STATE SURVIVES RELOAD", enabled: true, shown: true}
	runtime, err := newProbe(client, client, state)
	if err != nil {
		log.Fatal(err)
	}
	if err := runtime.PublishInitial(); err != nil {
		log.Fatal(err)
	}
	var updates <-chan *rui.DevUpdate
	if directory := os.Getenv("ROUNDOS_RUI_DEV_DIR"); directory != "" {
		session, err := rui.ListenDev(context.Background(), directory)
		if err != nil {
			log.Fatal(err)
		}
		defer session.Close()
		updates = session.Updates()
		log.Printf("RUI development session: %s", session.Path)
	}
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			if err := update.Apply(runtime); err != nil {
				log.Printf("RUI reload: %v", err)
			}
		case event, ok := <-client.Events():
			if !ok || event.Type == "close" {
				return
			}
			if _, err := runtime.Dispatch(event); err != nil {
				log.Printf("RUI dispatch: %v", err)
			}
			if event.Type == "text_result" {
				state.textResult(event)
				_ = runtime.Invalidate("probe.status")
			}
		}
		if err := runtime.Flush(); err != nil {
			log.Printf("RUI flush: %v", err)
		}
	}
}

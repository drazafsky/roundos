// An independent application using only the public RoundOS SDK. Compile
// layout.rui.yaml with roundui before building: go:embed requires a binary.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"roundos/sdk/app"
	"roundos/sdk/rui"
	"unicode/utf8"
)

//go:embed layout.rui.bin
var layout []byte

func setTitle(title *string, value string) error {
	if len(value) > 240 || !utf8.ValidString(value) {
		return fmt.Errorf("title must be valid UTF-8 and at most 240 bytes")
	}
	*title = value
	return nil
}

func main() {
	client, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	runtime, err := rui.New(client, layout)
	if err != nil {
		log.Fatal(err)
	}
	count := 0
	enabled := true
	title := "RUI v1"
	for _, reg := range []func() error{
		func() error {
			return runtime.BindString("demo.display", func() string { return fmt.Sprintf("%02d", count) })
		},
		func() error { return runtime.BindString("demo.subtitle", func() string { return title }) },
		func() error { return runtime.BindInt("demo.count", func() int { return count }) },
		func() error { return runtime.BindBool("demo.enabled", func() bool { return enabled }) },
		func() error {
			return runtime.OnAction("demo.increment", func(_ context.Context, _ rui.ActionEvent) error {
				if enabled && count < 99 {
					count++
				}
				return nil
			})
		},
		func() error {
			return runtime.OnAction("demo.editTitle", func(_ context.Context, _ rui.ActionEvent) error {
				return client.RequestText("demo-title", "Demo title", title, false)
			})
		},
		func() error {
			return runtime.OnAction("demo.requestReset", func(_ context.Context, _ rui.ActionEvent) error { return runtime.ShowDialog("demo.confirmReset") })
		},
		func() error {
			return runtime.OnAction("demo.confirmReset", func(_ context.Context, _ rui.ActionEvent) error { count = 0; return nil })
		},
		func() error {
			return runtime.OnAction("demo.cancelReset", func(_ context.Context, _ rui.ActionEvent) error { return nil })
		},
		func() error {
			return runtime.OnBool("demo.setEnabled", func(_ context.Context, v bool) error { enabled = v; return nil })
		},
		func() error {
			return runtime.OnInt("demo.setCount", func(_ context.Context, v int) error { count = v; return nil })
		},
	} {
		if err := reg(); err != nil {
			log.Fatal(err)
		}
	}
	if err := runtime.ValidateBindings(); err != nil {
		log.Fatal(err)
	}
	for ev := range client.Events() {
		if ev.Type == "close" {
			return
		}
		handled, err := runtime.Dispatch(ev)
		if err != nil {
			log.Printf("RUI dispatch: %v", err)
		}
		if !handled && ev.Type == "text_result" && ev.TextResult != nil && ev.TextResult.ID == "demo-title" && !ev.TextResult.Canceled {
			if err := setTitle(&title, ev.TextResult.Value); err != nil {
				log.Printf("Title edit rejected: %v", err)
			} else {
				_ = runtime.Invalidate("demo.subtitle")
			}
		}
		if err := runtime.Flush(); err != nil {
			log.Printf("RUI flush: %v", err)
		}
	}
}

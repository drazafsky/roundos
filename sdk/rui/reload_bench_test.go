package rui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"roundos/internal/rui/compile"
	"roundos/sdk/ui"
)

type discardReloadView struct{}

func (discardReloadView) Publish(ui.View) error { return nil }

// Exercise a sizeable valid screen tree plus many dialogs. Staging should
// scale with their combined size, rather than rebuilding the tree per dialog.
func BenchmarkReplacementManyDialogs(b *testing.B) {
	var source strings.Builder
	source.WriteString("schema: roundos.rui/v1\napplication:\n  id: org.example.load\nscreens:\n")
	for screen := 0; screen < 4; screen++ {
		fmt.Fprintf(&source, "  - id: screen%d\n    title: Load\n    layout: list\n    groups:\n      - id: items\n        widgets:\n", screen)
		for widget := 0; widget < 64; widget++ {
			fmt.Fprintf(&source, "          - id: item%d\n            type: label\n            label: { bind: caption }\n", widget)
		}
	}
	source.WriteString("dialogs:\n")
	for dialog := 0; dialog < 128; dialog++ {
		fmt.Fprintf(&source, "  - id: dialog%d\n    title: Confirm\n    body: Preserve application state\n    confirmLabel: OK\n    cancelLabel: CANCEL\n    onConfirm: confirm\n    onCancel: cancel\n", dialog)
	}
	data, _, err := compile.Compile([]byte(source.String()))
	if err != nil {
		b.Fatal(err)
	}
	r, err := NewForApp(discardReloadView{}, data, "org.example.load")
	if err != nil {
		b.Fatal(err)
	}
	if err := r.BindString("caption", func() string { return "Ready" }); err != nil {
		b.Fatal(err)
	}
	for _, action := range []string{"confirm", "cancel"} {
		if err := r.OnAction(action, func(context.Context, ActionEvent) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
	if err := r.ValidateBindings(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Replace(data); err != nil {
			b.Fatal(err)
		}
	}
}

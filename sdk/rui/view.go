package rui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"roundos/internal/wire"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
	"roundos/sdk/ui"
)

// stateSnapshot is an immutable set of binding getter functions captured at Flush.
// Getters are invoked only by the calling application's Flush goroutine.
type stateSnapshot map[string]binding

func (s stateSnapshot) text(v spec.Value[string], max int) (string, error) {
	if !v.Bound {
		return v.Literal, nil
	}
	b, ok := s[v.Binding]
	if !ok || b.text == nil {
		return "", fmt.Errorf("missing string binding %q", v.Binding)
	}
	x := b.text()
	if len(x) > max || !utf8.ValidString(x) {
		return "", fmt.Errorf("binding %q must be valid UTF-8 and at most %d bytes", v.Binding, max)
	}
	return x, nil
}
func (s stateSnapshot) integer(v spec.Value[int]) (int, error) {
	if !v.Bound {
		return v.Literal, nil
	}
	b, ok := s[v.Binding]
	if !ok || b.integer == nil {
		return 0, fmt.Errorf("missing int binding %q", v.Binding)
	}
	return b.integer(), nil
}
func (s stateSnapshot) boolean(v spec.Value[bool]) (bool, error) {
	if !v.Bound {
		return v.Literal, nil
	}
	b, ok := s[v.Binding]
	if !ok || b.boolean == nil {
		return false, fmt.Errorf("missing bool binding %q", v.Binding)
	}
	return b.boolean(), nil
}
func (s stateSnapshot) optText(p *spec.Value[string], max int) (string, error) {
	if p == nil {
		return "", nil
	}
	return s.text(*p, max)
}
func headingID(screen, group string) string {
	h := sha256.Sum256([]byte(screen + "\x00" + group))
	return fmt.Sprintf("rui.heading.%x", h[:10])
}

// buildView lowers fully resolved RUI into the existing v7 shell view model.
// All geometry, colors, focus, navigation and hit testing stay in the shell.
func buildView(f *bundle.File, states stateSnapshot, dialogID string) (ui.View, error) {
	v := ui.View{Title: strings.ToUpper(f.Document.Application.ID)}
	for _, screen := range f.Document.Screens {
		title, err := states.text(screen.Title, 80)
		if err != nil {
			return v, fmt.Errorf("screen %s title: %w", screen.ID, err)
		}
		sub, err := states.optText(screen.Subtitle, 240)
		if err != nil {
			return v, fmt.Errorf("screen %s subtitle: %w", screen.ID, err)
		}
		sc := ui.ScreenSpec{ID: screen.ID, Title: title, Subtitle: sub}
		if screen.Hero != nil {
			sc.Hero, err = states.text(*screen.Hero, 80)
			if err != nil {
				return v, fmt.Errorf("screen %s hero: %w", screen.ID, err)
			}
		}
		for _, group := range screen.Groups {
			if group.Title != nil {
				t, e := states.text(*group.Title, 80)
				if e != nil {
					return v, e
				}
				sc.Widgets = append(sc.Widgets, ui.Widget{ID: headingID(screen.ID, group.ID), Kind: "label", Label: t})
			}
			for _, w := range group.Widgets {
				if w.VisibleWhen != nil {
					show, e := states.boolean(*w.VisibleWhen)
					if e != nil {
						return v, e
					}
					if !show {
						continue
					}
				}
				item := ui.Widget{ID: w.ID, Variant: w.Variant, Target: w.Navigate}
				item.Label, err = states.optText(w.Label, 160)
				if err != nil {
					return v, fmt.Errorf("widget %s label: %w", w.ID, err)
				}
				item.Detail, err = states.optText(w.Detail, 240)
				if err != nil {
					return v, fmt.Errorf("widget %s detail: %w", w.ID, err)
				}
				switch w.Type {
				case "button":
					item.Kind = "button"
					item.Action = w.OnPress
				case "listItem":
					item.Kind = "list_item"
					item.Action = w.OnPress
				case "toggle":
					item.Kind = "toggle"
					item.Action = w.OnChange
				case "slider":
					item.Kind = "slider"
					item.Action = w.OnCommit
				case "numberPicker":
					item.Kind = "picker"
					item.Action = w.OnCommit
				case "timePicker":
					item.Kind = "time_picker"
					item.Action = w.OnCommit
				case "datePicker":
					item.Kind = "date_picker"
					item.Action = w.OnCommit
				case "label":
					item.Kind = "label"
				case "progress":
					item.Kind = "progress"
				default:
					return v, fmt.Errorf("unsupported RUI widget %q", w.Type)
				}
				if w.Min != nil {
					item.Min = *w.Min
				}
				if w.Max != nil {
					item.Max = *w.Max
				}
				if w.Type == "timePicker" {
					item.Min, item.Max = 0, 1439
				}
				if w.Step != nil {
					item.Step = *w.Step
				}
				if w.Wrap != nil {
					item.Wrap = *w.Wrap
				}
				if w.Value != nil {
					item.Value, err = states.integer(*w.Value)
					if err != nil {
						return v, fmt.Errorf("widget %s value: %w", w.ID, err)
					}
					if item.Value < item.Min || item.Value > item.Max {
						return v, fmt.Errorf("widget %s value %d outside %d..%d", w.ID, item.Value, item.Min, item.Max)
					}
				}
				if w.Checked != nil {
					item.Checked, err = states.boolean(*w.Checked)
					if err != nil {
						return v, err
					}
				}
				if w.EnabledWhen != nil {
					enabled, e := states.boolean(*w.EnabledWhen)
					if e != nil {
						return v, e
					}
					item.Disabled = !enabled
				}
				sc.Widgets = append(sc.Widgets, item)
			}
		}
		if len(sc.Widgets) > 64 {
			return v, fmt.Errorf("screen %q expands to over 64 widgets", sc.ID)
		}
		v.Screens = append(v.Screens, sc)
	}
	if dialogID != "" {
		found := false
		for _, d := range f.Document.Dialogs {
			if d.ID != dialogID {
				continue
			}
			found = true
			title, err := states.text(d.Title, 80)
			if err != nil {
				return v, err
			}
			body, err := states.optText(d.Body, 240)
			if err != nil {
				return v, err
			}
			v.Dialog = &ui.Dialog{ID: d.ID, Title: title, Body: body, Confirm: d.ConfirmLabel, Cancel: d.CancelLabel, ConfirmAction: d.OnConfirm, CancelAction: d.OnCancel, Destructive: d.Destructive}
			break
		}
		if !found {
			return v, fmt.Errorf("unknown dialog %q", dialogID)
		}
	}
	if err := wire.ValidateView(v); err != nil {
		return v, err
	}
	b, err := json.Marshal(wire.Message{Version: wire.Version, Type: "view", View: &v})
	if err != nil {
		return v, err
	}
	if len(b) > wire.MaxMessage {
		return v, fmt.Errorf("RUI view exceeds %d-byte IPC frame (%d bytes)", wire.MaxMessage, len(b))
	}
	return v, nil
}

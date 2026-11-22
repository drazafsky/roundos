package wire

import (
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"
)

const (
	WidgetPatchV1      = "widget_patch_v1"
	UIEventV1          = "ui_event_v1"
	TemplatesV1        = "rui_templates_v1"
	MaxPropertyUpdates = 2048
)

// Capabilities returns a fresh, stable offer for additive v7 features.
func Capabilities() []string { return []string{WidgetPatchV1, UIEventV1, TemplatesV1} }
func HasCapability(caps []string, name string) bool {
	for _, cap := range caps {
		if cap == name {
			return true
		}
	}
	return false
}
func SelectCapabilities(offer []string) []string {
	var selected []string
	for _, name := range Capabilities() {
		if HasCapability(offer, name) {
			selected = append(selected, name)
		}
	}
	return selected
}

// PropertyUpdate is deliberately typed and excludes actions, identities and
// layout. Those changes require a new authoritative snapshot.
type PropertyUpdate struct {
	Screen   string  `json:"screen,omitempty"`
	Widget   string  `json:"widget,omitempty"`
	Property string  `json:"property"`
	String   *string `json:"string,omitempty"`
	Int      *int    `json:"int,omitempty"`
	Bool     *bool   `json:"bool,omitempty"`
}

type UIEvent struct {
	Screen         string `json:"screen"`
	Widget         string `json:"widget"`
	Action         string `json:"action"`
	Kind           string `json:"kind"` // press, bool, int
	Bool           *bool  `json:"bool,omitempty"`
	Int            *int   `json:"int,omitempty"`
	LayoutRevision uint64 `json:"layout_revision"`
	ID             uint64 `json:"id"`
}

// ValidateUIEvent checks identity, action, enabled state and exact value kind
// against an authoritative snapshot. Hidden widgets have no snapshot node.
func ValidateUIEvent(view View, event UIEvent) error {
	if event.ID == 0 || event.LayoutRevision == 0 {
		return fmt.Errorf("missing UI event identity")
	}
	for _, screen := range view.Screens {
		if screen.ID != event.Screen {
			continue
		}
		for _, widget := range screen.Widgets {
			if widget.ID != event.Widget {
				continue
			}
			if widget.Disabled || widget.Action == "" || widget.Action != event.Action || widget.Target != "" {
				return fmt.Errorf("inactive UI event target")
			}
			switch event.Kind {
			case "press":
				if (widget.Kind == "button" || widget.Kind == "list_item") && event.Bool == nil && event.Int == nil {
					return nil
				}
			case "bool":
				if widget.Kind == "toggle" && event.Bool != nil && event.Int == nil {
					return nil
				}
			case "int":
				if event.Int != nil && event.Bool == nil && (widget.Kind == "slider" || widget.Kind == "picker" || widget.Kind == "time_picker" || widget.Kind == "date_picker") && *event.Int >= widget.Min && *event.Int <= widget.Max {
					return nil
				}
			}
			return fmt.Errorf("invalid UI event value")
		}
	}
	return fmt.Errorf("unknown UI event target")
}

func MessageFits(m Message) bool {
	b, err := json.Marshal(m)
	return err == nil && len(b) <= MaxMessage
}

func UsesTemplates(view View) bool {
	for _, screen := range view.Screens {
		if screen.Layout != "" {
			return true
		}
		for _, widget := range screen.Widgets {
			if widget.Group != "" {
				return true
			}
		}
	}
	return false
}

func WithoutTemplates(view View) View {
	if !UsesTemplates(view) {
		return view
	}
	view = CopyView(view)
	for si := range view.Screens {
		view.Screens[si].Layout = ""
		for wi := range view.Screens[si].Widgets {
			view.Screens[si].Widgets[wi].Group = ""
		}
	}
	return view
}

// DiffProperties returns false for structural changes. Returning an empty
// slice with true means the two semantic views are identical.
func DiffProperties(before, after View) ([]PropertyUpdate, bool) {
	var updates []PropertyUpdate
	text := func(screen, widget, property, a, b string) {
		if a != b {
			value := b
			updates = append(updates, PropertyUpdate{Screen: screen, Widget: widget, Property: property, String: &value})
		}
	}
	if before.ButtonAction != after.ButtonAction || before.RotaryAction != after.RotaryAction || !reflect.DeepEqual(before.Dialog, after.Dialog) || len(before.Screens) != len(after.Screens) {
		return nil, false
	}
	text("", "", "title", before.Title, after.Title)
	text("", "", "primary", before.Primary, after.Primary)
	text("", "", "secondary", before.Secondary, after.Secondary)
	text("", "", "hint", before.Hint, after.Hint)
	text("", "", "button_label", before.ButtonLabel, after.ButtonLabel)
	for i, a := range before.Screens {
		b := after.Screens[i]
		if a.ID != b.ID || a.Layout != b.Layout || len(a.Widgets) != len(b.Widgets) {
			return nil, false
		}
		text(b.ID, "", "title", a.Title, b.Title)
		text(b.ID, "", "hero", a.Hero, b.Hero)
		text(b.ID, "", "subtitle", a.Subtitle, b.Subtitle)
		for j, aw := range a.Widgets {
			bw := b.Widgets[j]
			text(b.ID, bw.ID, "label", aw.Label, bw.Label)
			text(b.ID, bw.ID, "detail", aw.Detail, bw.Detail)
			if aw.Value != bw.Value {
				value := bw.Value
				updates = append(updates, PropertyUpdate{Screen: b.ID, Widget: bw.ID, Property: "value", Int: &value})
			}
			if aw.Checked != bw.Checked {
				value := bw.Checked
				updates = append(updates, PropertyUpdate{Screen: b.ID, Widget: bw.ID, Property: "checked", Bool: &value})
			}
			if aw.Disabled != bw.Disabled {
				value := bw.Disabled
				updates = append(updates, PropertyUpdate{Screen: b.ID, Widget: bw.ID, Property: "disabled", Bool: &value})
			}
			aw.Label, aw.Detail, aw.Value, aw.Checked, aw.Disabled = bw.Label, bw.Detail, bw.Value, bw.Checked, bw.Disabled
			if aw != bw {
				return nil, false
			}
		}
	}
	return updates, true
}

// ApplyProperties validates an entire batch on a private copy. It returns no
// modified tree on failure; the receiver can request an authoritative resync.
func ApplyProperties(current View, updates []PropertyUpdate) (View, error) {
	if len(updates) == 0 || len(updates) > MaxPropertyUpdates {
		return View{}, fmt.Errorf("invalid property batch length")
	}
	next := CopyView(current)
	seen := map[[3]string]bool{}
	for _, u := range updates {
		key := [3]string{u.Screen, u.Widget, u.Property}
		if seen[key] {
			return View{}, fmt.Errorf("duplicate property target")
		}
		seen[key] = true
		count := 0
		if u.String != nil {
			count++
		}
		if u.Int != nil {
			count++
		}
		if u.Bool != nil {
			count++
		}
		if count != 1 {
			return View{}, fmt.Errorf("property requires exactly one typed value")
		}
		var str *string
		limit := 240
		if u.Screen == "" {
			if u.Widget != "" {
				return View{}, fmt.Errorf("widget requires a screen")
			}
			switch u.Property {
			case "title":
				str = &next.Title
				limit = 80
			case "primary":
				str = &next.Primary
			case "secondary":
				str = &next.Secondary
			case "hint":
				str = &next.Hint
			case "button_label":
				str = &next.ButtonLabel
			}
		} else {
			var screen *Screen
			for i := range next.Screens {
				if next.Screens[i].ID == u.Screen {
					screen = &next.Screens[i]
					break
				}
			}
			if screen == nil {
				return View{}, fmt.Errorf("unknown screen")
			}
			if u.Widget == "" {
				switch u.Property {
				case "title":
					str = &screen.Title
					limit = 80
				case "hero":
					str = &screen.Hero
					limit = 80
				case "subtitle":
					str = &screen.Subtitle
				}
			} else {
				var widget *Widget
				for i := range screen.Widgets {
					if screen.Widgets[i].ID == u.Widget {
						widget = &screen.Widgets[i]
						break
					}
				}
				if widget == nil {
					return View{}, fmt.Errorf("unknown widget")
				}
				switch u.Property {
				case "label":
					str = &widget.Label
					limit = 160
				case "detail":
					str = &widget.Detail
				case "value":
					if u.Int == nil || !(widget.Kind == "slider" || widget.Kind == "progress" || widget.Kind == "picker" || widget.Kind == "time_picker" || widget.Kind == "date_picker") {
						return View{}, fmt.Errorf("invalid integer property")
					}
					widget.Value = *u.Int
					continue
				case "checked":
					if u.Bool == nil || widget.Kind != "toggle" {
						return View{}, fmt.Errorf("invalid checked property")
					}
					widget.Checked = *u.Bool
					continue
				case "disabled":
					if u.Bool == nil {
						return View{}, fmt.Errorf("invalid disabled property")
					}
					widget.Disabled = *u.Bool
					continue
				}
			}
		}
		if str == nil || u.String == nil || len(*u.String) > limit || !utf8.ValidString(*u.String) {
			return View{}, fmt.Errorf("forbidden property or invalid text")
		}
		*str = *u.String
	}
	if err := ValidateView(next); err != nil {
		return View{}, err
	}
	if !MessageFits(Message{Version: Version, Type: "view", View: &next, LayoutRevision: 1, NewRevision: 1}) {
		return View{}, fmt.Errorf("updated snapshot exceeds IPC frame")
	}
	return next, nil
}

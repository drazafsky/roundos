// Package validate performs semantic RUI v1 verification without touching
// application state, networking, files, DRM or the existing IPC transport.
package validate

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"roundos/sdk/rui/spec"
)

var idRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)

const MaxScreens = 16
const MaxEffectiveWidgets = 64

type Diagnostic struct {
	Pos     spec.Pos
	Message string
}

func (e Diagnostic) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Pos.Line, e.Pos.Column, e.Message)
}
func bad(pos spec.Pos, format string, args ...any) error {
	return Diagnostic{pos, fmt.Sprintf(format, args...)}
}
func identifier(s string, p spec.Pos, label string) error {
	if len(s) < 1 || len(s) > 64 || !idRE.MatchString(s) {
		return bad(p, "invalid %s %q (1-64 ASCII letters/digits/dots/dashes/underscores; begins with a letter)", label, s)
	}
	if strings.HasPrefix(s, "rui.") || strings.HasPrefix(s, "__rui_") {
		return bad(p, "%s %q uses a reserved prefix", label, s)
	}
	return nil
}
func literalText(v spec.Value[string], max int, label string) error {
	if !v.Present {
		return bad(v.Pos, "missing %s", label)
	}
	if v.Bound {
		return identifier(v.Binding, v.Pos, "binding")
	}
	if len(v.Literal) > max {
		return bad(v.Pos, "%s exceeds %d bytes", label, max)
	}
	return nil
}
func boolWhen(v *spec.Value[bool], name string) error {
	if v != nil && !v.Bound {
		return bad(v.Pos, "%s must reference a bool binding", name)
	}
	return nil
}
func intRange(w spec.Widget) (int, int, error) {
	if w.Type == "timePicker" {
		if w.Min != nil || w.Max != nil || w.MinBinding != "" || w.MaxBinding != "" {
			return 0, 0, bad(w.Pos, "timePicker uses the fixed 0..1439 minute-of-day range")
		}
		return 0, 1439, nil
	}
	if (w.Min == nil) == (w.MinBinding == "") || (w.Max == nil) == (w.MaxBinding == "") {
		return 0, 0, bad(w.Pos, "%s requires min and max", w.Type)
	}
	if w.Min != nil && w.Max != nil && *w.Max <= *w.Min {
		return 0, 0, bad(w.Pos, "max must exceed min")
	}
	low, high := math.MinInt32, math.MaxInt32
	if w.Min != nil {
		low = *w.Min
	}
	if w.Max != nil {
		high = *w.Max
	}
	return low, high, nil
}
func requireAction(name string, p spec.Pos) error { return identifier(name, p, "action") }
func oneOfVariant(s string, p spec.Pos) error {
	switch s {
	case "", "primary", "secondary", "tertiary":
		return nil
	}
	return bad(p, "unknown semantic variant %q", s)
}

// Validate checks both binding types and navigation references. The manifest
// is a static contract for later SDK binding/handler registration (M3).
func Validate(d *spec.Document) (*spec.Manifest, error) {
	if d == nil {
		return nil, bad(spec.Pos{Line: 1, Column: 1}, "nil RUI document")
	}
	if d.Schema != spec.Version {
		return nil, bad(d.Pos, "unsupported schema %q (expected %q)", d.Schema, spec.Version)
	}
	if err := identifier(d.Application.ID, d.Application.Pos, "application ID"); err != nil {
		return nil, err
	}
	if len(d.Screens) == 0 || len(d.Screens) > MaxScreens {
		return nil, bad(d.Pos, "screens must contain 1..%d entries", MaxScreens)
	}
	result := &spec.Manifest{Bindings: map[string]spec.BindingType{}}
	actionKinds := map[string]string{}
	bindAny := func(name string, t spec.BindingType, p spec.Pos) error {
		if err := identifier(name, p, "binding"); err != nil {
			return err
		}
		if old, exists := result.Bindings[name]; exists && old != t {
			return bad(p, "binding %q used as both %s and %s", name, old, t)
		}
		result.Bindings[name] = t
		return nil
	}
	addText := func(v *spec.Value[string], label string, max int) error {
		if v == nil {
			return nil
		}
		if err := literalText(*v, max, label); err != nil {
			return err
		}
		if v.Bound {
			return bindAny(v.Binding, spec.StringBinding, v.Pos)
		}
		return nil
	}
	addInt := func(v *spec.Value[int]) error {
		if v != nil && v.Bound {
			return bindAny(v.Binding, spec.IntBinding, v.Pos)
		}
		return nil
	}
	addBool := func(v *spec.Value[bool]) error {
		if v != nil && v.Bound {
			return bindAny(v.Binding, spec.BoolBinding, v.Pos)
		}
		return nil
	}
	addAction := func(name, kind string, p spec.Pos) error {
		if name == "" {
			return nil
		}
		if err := requireAction(name, p); err != nil {
			return err
		}
		if old, ok := actionKinds[name]; ok && old != kind {
			return bad(p, "action %q used with incompatible event types %s and %s", name, old, kind)
		}
		actionKinds[name] = kind
		return nil
	}
	screenIDs := map[string]bool{}
	for _, s := range d.Screens {
		if err := identifier(s.ID, s.Pos, "screen ID"); err != nil {
			return nil, err
		}
		if screenIDs[s.ID] {
			return nil, bad(s.Pos, "duplicate screen ID %q", s.ID)
		}
		screenIDs[s.ID] = true
	}
	for _, s := range d.Screens {
		if err := addText(&s.Title, "screen title", 80); err != nil {
			return nil, err
		}
		if err := addText(s.Hero, "hero", 80); err != nil {
			return nil, err
		}
		if err := addText(s.Subtitle, "subtitle", 240); err != nil {
			return nil, err
		}
		switch s.Layout {
		case "hero_list", "list", "form", "detail":
		default:
			return nil, bad(s.Pos, "unknown layout template %q", s.Layout)
		}
		if s.Layout != "hero_list" && s.Hero != nil {
			return nil, bad(s.Hero.Pos, "hero is supported only by hero_list")
		}
		if s.Layout == "hero_list" && s.Hero == nil {
			return nil, bad(s.Pos, "hero_list requires hero")
		}
		if len(s.Groups) == 0 {
			return nil, bad(s.Pos, "screen %q requires one or more groups", s.ID)
		}
		groupIDs := map[string]bool{}
		widgetIDs := map[string]bool{}
		effective := 0
		for _, g := range s.Groups {
			if err := identifier(g.ID, g.Pos, "group ID"); err != nil {
				return nil, err
			}
			if groupIDs[g.ID] {
				return nil, bad(g.Pos, "duplicate group ID %q", g.ID)
			}
			groupIDs[g.ID] = true
			if err := addText(g.Title, "group title", 80); err != nil {
				return nil, err
			}
			if g.Title != nil {
				effective++
			} // synthetic nonfocusable heading
			if len(g.Widgets) == 0 {
				return nil, bad(g.Pos, "group %q must contain at least one widget", g.ID)
			}
			for _, w := range g.Widgets {
				effective++
				if effective > MaxEffectiveWidgets {
					return nil, bad(w.Pos, "screen %q exceeds %d effective widgets (including group headings)", s.ID, MaxEffectiveWidgets)
				}
				if err := identifier(w.ID, w.Pos, "widget ID"); err != nil {
					return nil, err
				}
				if widgetIDs[w.ID] {
					return nil, bad(w.Pos, "duplicate widget ID %q in screen %q", w.ID, s.ID)
				}
				widgetIDs[w.ID] = true
				if err := oneOfVariant(w.Variant, w.Pos); err != nil {
					return nil, err
				}
				if w.Label == nil {
					return nil, bad(w.Pos, "widget %q requires label", w.ID)
				}
				if err := addText(w.Label, "widget label", 160); err != nil {
					return nil, err
				}
				if err := addText(w.Detail, "widget detail", 240); err != nil {
					return nil, err
				}
				if err := boolWhen(w.EnabledWhen, "enabledWhen"); err != nil {
					return nil, err
				}
				if err := boolWhen(w.VisibleWhen, "visibleWhen"); err != nil {
					return nil, err
				}
				if err := addBool(w.EnabledWhen); err != nil {
					return nil, err
				}
				if err := addBool(w.VisibleWhen); err != nil {
					return nil, err
				}
				if w.Navigate != "" {
					if !screenIDs[w.Navigate] {
						return nil, bad(w.Pos, "navigate target %q does not exist", w.Navigate)
					}
					if w.OnPress != "" || w.OnChange != "" || w.OnCommit != "" {
						return nil, bad(w.Pos, "navigate cannot be combined with action handlers")
					}
				}
				if w.OnPress != "" {
					if err := addAction(w.OnPress, "action", w.Pos); err != nil {
						return nil, err
					}
				}
				if w.OnChange != "" {
					if err := addAction(w.OnChange, "bool", w.Pos); err != nil {
						return nil, err
					}
				}
				if w.OnCommit != "" {
					if err := addAction(w.OnCommit, "int", w.Pos); err != nil {
						return nil, err
					}
				}
				if err := addInt(w.Value); err != nil {
					return nil, err
				}
				for _, key := range []string{w.MinBinding, w.MaxBinding} {
					if key != "" {
						if err := addInt(&spec.Value[int]{Bound: true, Binding: key, Present: true, Pos: w.Pos}); err != nil {
							return nil, err
						}
					}
				}
				if err := addBool(w.Checked); err != nil {
					return nil, err
				}
				switch w.Type {
				case "button", "listItem":
					if w.Value != nil || w.Checked != nil || w.Min != nil || w.Max != nil || w.MinBinding != "" || w.MaxBinding != "" || w.Step != nil || w.Wrap != nil || w.OnChange != "" || w.OnCommit != "" {
						return nil, bad(w.Pos, "%s contains unsupported value/handler properties", w.Type)
					}
					if w.Navigate == "" && w.OnPress == "" {
						return nil, bad(w.Pos, "%s requires exactly one of onPress or navigate", w.Type)
					}
				case "toggle":
					if w.Checked == nil || w.OnChange == "" {
						return nil, bad(w.Pos, "toggle requires checked and onChange")
					}
					if w.Value != nil || w.Min != nil || w.Max != nil || w.MinBinding != "" || w.MaxBinding != "" || w.Step != nil || w.Wrap != nil || w.OnPress != "" || w.OnCommit != "" || w.Navigate != "" {
						return nil, bad(w.Pos, "toggle has incompatible properties")
					}
				case "slider", "numberPicker", "timePicker", "datePicker", "progress":
					if w.Value == nil {
						return nil, bad(w.Pos, "%s requires integer value", w.Type)
					}
					if w.Checked != nil || w.OnPress != "" || w.OnChange != "" || w.Navigate != "" {
						return nil, bad(w.Pos, "%s has incompatible properties", w.Type)
					}
					low, high, e := intRange(w)
					if e != nil {
						return nil, e
					}
					if !w.Value.Bound && (w.Value.Literal < low || w.Value.Literal > high) {
						return nil, bad(w.Value.Pos, "%s value %d outside %d..%d", w.Type, w.Value.Literal, low, high)
					}
					if w.Type == "progress" {
						if w.OnCommit != "" || w.Step != nil || w.Wrap != nil {
							return nil, bad(w.Pos, "progress is read-only")
						}
					} else {
						if w.OnCommit == "" {
							return nil, bad(w.Pos, "%s requires onCommit", w.Type)
						}
						if w.Step != nil && *w.Step <= 0 {
							return nil, bad(w.Pos, "step must be positive")
						}
						if w.Type == "timePicker" && w.Step != nil && 1440%*w.Step != 0 {
							return nil, bad(w.Pos, "timePicker step must divide 1440")
						}
					}
				case "label":
					if w.Value != nil || w.Checked != nil || w.Min != nil || w.Max != nil || w.MinBinding != "" || w.MaxBinding != "" || w.Step != nil || w.Wrap != nil || w.OnPress != "" || w.OnChange != "" || w.OnCommit != "" || w.Navigate != "" {
						return nil, bad(w.Pos, "label is noninteractive text")
					}
				default:
					return nil, bad(w.Pos, "unsupported widget type %q", w.Type)
				}
			}
		}
		if effective > MaxEffectiveWidgets {
			return nil, bad(s.Pos, "screen %q exceeds %d effective widgets", s.ID, MaxEffectiveWidgets)
		}
	}
	seenDialogs := map[string]bool{}
	for _, dlog := range d.Dialogs {
		if err := identifier(dlog.ID, dlog.Pos, "dialog ID"); err != nil {
			return nil, err
		}
		if seenDialogs[dlog.ID] {
			return nil, bad(dlog.Pos, "duplicate dialog ID %q", dlog.ID)
		}
		seenDialogs[dlog.ID] = true
		if err := addText(&dlog.Title, "dialog title", 80); err != nil {
			return nil, err
		}
		if err := addText(dlog.Body, "dialog body", 240); err != nil {
			return nil, err
		}
		if dlog.ConfirmLabel == "" || dlog.CancelLabel == "" || len(dlog.ConfirmLabel) > 80 || len(dlog.CancelLabel) > 80 {
			return nil, bad(dlog.Pos, "dialog requires bounded confirmLabel/cancelLabel")
		}
		if dlog.OnConfirm == "" || dlog.OnCancel == "" {
			return nil, bad(dlog.Pos, "dialog requires onConfirm and onCancel")
		}
		if err := addAction(dlog.OnConfirm, "action", dlog.Pos); err != nil {
			return nil, err
		}
		if err := addAction(dlog.OnCancel, "action", dlog.Pos); err != nil {
			return nil, err
		}
	}
	for name, kind := range actionKinds {
		result.Actions = append(result.Actions, spec.Action{Name: name, ValueType: kind})
	}
	sort.Slice(result.Actions, func(i, j int) bool { return result.Actions[i].Name < result.Actions[j].Name })
	return result, nil
}

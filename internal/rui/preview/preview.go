// Package preview maps a compiled RUI tree and typed sample state into the
// existing v7 wire widget model, then draws it with the real RoundOS shell.
// This is an offline design preview; no application code or IPC is executed.
package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"strings"
	"time"

	"roundos/internal/appearance"
	"roundos/internal/appmgr"
	"roundos/internal/shell"
	"roundos/internal/wire"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

const MaxValuesJSON = 32 << 10

// State holds explicit sample values and defaults for an offline preview.
// Missing string bindings display [binding.name]. Boolean conditions default
// true so designers see their controls; other booleans default false; integer
// values use their minimum bound. In real M3 application mode ALL bindings
// must be explicitly registered and values checked at every publication.
type State struct {
	values   map[string]json.RawMessage
	manifest spec.Manifest
}

func NewState(manifest spec.Manifest, raw []byte) (*State, error) {
	s := &State{values: map[string]json.RawMessage{}, manifest: manifest}
	if len(raw) == 0 {
		return s, nil
	}
	if len(raw) > MaxValuesJSON {
		return nil, fmt.Errorf("sample binding JSON exceeds %d bytes", MaxValuesJSON)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s.values); err != nil {
		return nil, fmt.Errorf("sample bindings: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("sample bindings: trailing JSON")
	}
	if s.values == nil {
		return nil, fmt.Errorf("sample bindings must be an object")
	}
	for k, v := range s.values {
		typ, ok := manifest.Bindings[k]
		if !ok {
			return nil, fmt.Errorf("sample binding %q is not declared", k)
		}
		switch typ {
		case spec.StringBinding:
			var x string
			if err := json.Unmarshal(v, &x); err != nil || bytes.Equal(v, []byte("null")) {
				return nil, fmt.Errorf("sample binding %q must be string", k)
			}
		case spec.IntBinding:
			var x int32
			if err := json.Unmarshal(v, &x); err != nil || bytes.Equal(v, []byte("null")) {
				return nil, fmt.Errorf("sample binding %q must be 32-bit integer", k)
			}
		case spec.BoolBinding:
			var x bool
			if err := json.Unmarshal(v, &x); err != nil || bytes.Equal(v, []byte("null")) {
				return nil, fmt.Errorf("sample binding %q must be boolean", k)
			}
		default:
			return nil, fmt.Errorf("unknown sample binding type %q", typ)
		}
	}
	return s, nil
}

func (s *State) Missing() []string {
	var result []string
	for k := range s.manifest.Bindings {
		if _, ok := s.values[k]; !ok {
			result = append(result, k)
		}
	}
	// Ensure CLI warnings deterministic.
	sortStrings(result)
	return result
}

func sortStrings(a []string) { // insertion sort, small and bounded by schema
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
func (s *State) text(v spec.Value[string]) string {
	if !v.Bound {
		return v.Literal
	}
	if raw, ok := s.values[v.Binding]; ok {
		var x string
		_ = json.Unmarshal(raw, &x)
		return x
	}
	return "[" + v.Binding + "]"
}
func (s *State) integer(v spec.Value[int], fallback int) int {
	if !v.Bound {
		return v.Literal
	}
	if raw, ok := s.values[v.Binding]; ok {
		var x int
		_ = json.Unmarshal(raw, &x)
		return x
	}
	return fallback
}
func (s *State) boolean(v spec.Value[bool], fallback bool) bool {
	if !v.Bound {
		return v.Literal
	}
	if raw, ok := s.values[v.Binding]; ok {
		var x bool
		_ = json.Unmarshal(raw, &x)
		return x
	}
	return fallback
}
func resolveString(s *State, p *spec.Value[string]) string {
	if p == nil {
		return ""
	}
	return s.text(*p)
}
func genHeading(screenID, groupID string) string {
	digest := sha256.Sum256([]byte(screenID + "\x00" + groupID))
	return fmt.Sprintf("rui.heading.%x", digest[:10])
}

// View converts to the existing v7 wire tree, expanding nonfocusable headings
// with stable reserved IDs, preserving all navigation/actions and rechecking
// the exact JSON IPC envelope limit before any later runtime publication.
// Template geometry is shared with production; old peers receive a flattened
// row-list view through the SDK's explicitly documented compatibility adapter.
func View(f *bundle.File, s *State) (wire.View, error) {
	if f == nil || s == nil {
		return wire.View{}, fmt.Errorf("missing bundle or preview state")
	}
	v := wire.View{Title: strings.ToUpper(f.Document.Application.ID)}
	for _, screen := range f.Document.Screens {
		ws := wire.Screen{ID: screen.ID, Title: s.text(screen.Title), Subtitle: resolveString(s, screen.Subtitle)}
		ws.Layout = screen.Layout
		if screen.Hero != nil {
			ws.Hero = s.text(*screen.Hero)
		}
		for _, group := range screen.Groups {
			if group.Title != nil {
				ws.Widgets = append(ws.Widgets, wire.Widget{ID: genHeading(screen.ID, group.ID), Group: group.ID, Kind: "label", Label: s.text(*group.Title)})
			}
			for _, w := range group.Widgets {
				if w.VisibleWhen != nil && !s.boolean(*w.VisibleWhen, true) {
					continue
				}
				item := wire.Widget{ID: w.ID, Group: group.ID, Variant: w.Variant, Label: resolveString(s, w.Label), Detail: resolveString(s, w.Detail), Target: w.Navigate}
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
					return wire.View{}, fmt.Errorf("unmapped widget kind %s", w.Type)
				}
				if w.Min != nil {
					item.Min = *w.Min
				}
				if w.Max != nil {
					item.Max = *w.Max
				}
				if w.MinBinding != "" {
					item.Min = s.integer(spec.Value[int]{Bound: true, Binding: w.MinBinding}, 0)
				}
				if w.MaxBinding != "" {
					item.Max = s.integer(spec.Value[int]{Bound: true, Binding: w.MaxBinding}, item.Min+1)
				}
				if w.Type == "timePicker" {
					item.Min = 0
					item.Max = 1439
				}
				if w.Step != nil {
					item.Step = *w.Step
				}
				if w.Wrap != nil {
					item.Wrap = *w.Wrap
				}
				if w.Value != nil {
					item.Value = s.integer(*w.Value, item.Min)
					if item.Value < item.Min || item.Value > item.Max {
						return wire.View{}, fmt.Errorf("screen %s widget %s: value %d outside %d..%d", screen.ID, w.ID, item.Value, item.Min, item.Max)
					}
				}
				if w.Checked != nil {
					item.Checked = s.boolean(*w.Checked, false)
				}
				if w.EnabledWhen != nil {
					item.Disabled = !s.boolean(*w.EnabledWhen, true)
				}
				ws.Widgets = append(ws.Widgets, item)
			}
		}
		if len(ws.Widgets) > 64 {
			return wire.View{}, fmt.Errorf("screen %q has %d effective widgets after conversion", screen.ID, len(ws.Widgets))
		}
		v.Screens = append(v.Screens, ws)
	}
	v = wire.RootHeader(v)
	if err := wire.ValidateView(v); err != nil {
		return wire.View{}, err
	}
	if n, err := FrameBytes(v); err != nil {
		return wire.View{}, err
	} else if n > wire.MaxMessage {
		return wire.View{}, fmt.Errorf("v7 view IPC frame exceeds %d bytes (%d)", wire.MaxMessage, n)
	}
	return v, nil
}
func FrameBytes(v wire.View) (int, error) {
	b, e := json.Marshal(wire.Message{Version: wire.Version, Type: "view", View: &v})
	if e != nil {
		return 0, e
	}
	return len(b), nil
}

// Render uses the identical 480x480 RoundOS draw path as the PNG backend in
// cmd/roundd. It does not access /dev/dri or launch an app.
func Render(f *bundle.File, v wire.View, screenID, theme string) (*image.RGBA, error) {
	var id string
	if f == nil {
		return nil, fmt.Errorf("nil bundle")
	}
	id = f.Document.Application.ID
	model := shell.New()
	if id != wire.ClockID && id != wire.TimerID {
		model.SetInstalledApps([]appmgr.AppSpec{{ID: id, Name: "RUI PREVIEW", Icon: "RUI"}})
	}
	if theme == "" {
		theme = appearance.Dark
	}
	model.Appearance.Theme = theme
	if err := model.Appearance.Valid(); err != nil {
		return nil, err
	}
	model.SetView(id, v)
	page := model.PageForApp(id)
	if page == shell.Home {
		return nil, fmt.Errorf("cannot preview application %q", id)
	}
	model.Select(page)
	if screenID != "" && screenID != v.Screens[0].ID {
		found := false
		for _, sc := range v.Screens {
			if sc.ID == screenID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown preview screen %q", screenID)
		}
		model.AppStacks[page] = []shell.NavFrame{{ScreenID: v.Screens[0].ID}, {ScreenID: screenID}}
	}
	// Fixed time makes previews reproducible even if the shell draws the
	// current date/clock somewhere in the contextual chrome.
	return shell.Draw(model, time.Unix(1_700_000_000, 0).UTC()), nil
}

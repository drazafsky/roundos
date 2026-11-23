package compile

import (
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

// Dependencies is deterministic and is verified independently when loading a
// bundle. Runtime invalidation follows indices instead of parsing paths.
func Dependencies(doc *spec.Document) map[string][]bundle.Dependency {
	deps := map[string][]bundle.Dependency{}
	add := func(key, property string, at bundle.Dependency) {
		at.Property = property
		deps[key] = append(deps[key], at)
	}
	text := func(v *spec.Value[string], property string, at bundle.Dependency) {
		if v != nil && v.Bound {
			add(v.Binding, property, at)
		}
	}
	integer := func(v *spec.Value[int], property string, at bundle.Dependency) {
		if v != nil && v.Bound {
			add(v.Binding, property, at)
		}
	}
	boolean := func(v *spec.Value[bool], property string, at bundle.Dependency) {
		if v != nil && v.Bound {
			add(v.Binding, property, at)
		}
	}
	for si, s := range doc.Screens {
		at := bundle.Dependency{Screen: si, Group: -1, Widget: -1, Dialog: -1}
		text(&s.Title, "title", at)
		text(s.Hero, "hero", at)
		text(s.Subtitle, "subtitle", at)
		for gi, g := range s.Groups {
			at.Group = gi
			text(g.Title, "title", at)
			for wi, w := range g.Widgets {
				at.Widget = wi
				text(w.Label, "label", at)
				text(w.Detail, "detail", at)
				integer(w.Value, "value", at)
				if w.MinBinding != "" {
					add(w.MinBinding, "min", at)
				}
				if w.MaxBinding != "" {
					add(w.MaxBinding, "max", at)
				}
				boolean(w.Checked, "checked", at)
				boolean(w.EnabledWhen, "enabled", at)
				boolean(w.VisibleWhen, "visible", at)
			}
			at.Widget = -1
		}
	}
	for di, d := range doc.Dialogs {
		at := bundle.Dependency{Screen: -1, Group: -1, Widget: -1, Dialog: di}
		text(&d.Title, "title", at)
		text(d.Body, "body", at)
	}
	return deps
}

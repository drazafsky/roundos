package parse

import (
	"fmt"
	"strconv"
	"strings"

	"roundos/sdk/rui/spec"
)

func only(n *node, allowed ...string) error {
	if n == nil || n.kind != "map" {
		p := spec.Pos{Line: 1, Column: 1}
		if n != nil {
			p = n.pos
		}
		return fail(p, "expected a mapping")
	}
	legal := map[string]bool{}
	for _, v := range allowed {
		legal[v] = true
	}
	for _, k := range n.order {
		if !legal[k] {
			return fail(n.get(k).pos, "unknown field %q", k)
		}
	}
	return nil
}
func required(n *node, k string) (*node, error) {
	v := n.get(k)
	if v == nil {
		return nil, fail(n.pos, "missing required field %q", k)
	}
	return v, nil
}
func sequence(n *node, name string) ([]*node, error) {
	if n == nil {
		return nil, fail(spec.Pos{Line: 1, Column: 1}, "missing sequence %q", name)
	}
	if n.kind != "seq" {
		return nil, fail(n.pos, "%s must be a sequence", name)
	}
	return n.sequence, nil
}
func scalar(n *node) (string, error) {
	if n == nil || n.kind != "scalar" {
		p := spec.Pos{Line: 1, Column: 1}
		if n != nil {
			p = n.pos
		}
		return "", fail(p, "expected a scalar")
	}
	return n.raw, nil
}
func stringLiteral(n *node) (string, error) {
	s, e := scalar(n)
	if e != nil {
		return "", e
	}
	if !n.quoted {
		switch strings.ToLower(s) {
		case "true", "false":
			return "", fail(n.pos, "boolean cannot be used as a string; quote it")
		}
		if _, err := strconv.ParseInt(s, 10, 64); err == nil {
			return "", fail(n.pos, "number cannot be used as a string; quote it")
		}
	}
	return s, nil
}
func intLiteral(n *node) (int, error) {
	s, e := scalar(n)
	if e != nil {
		return 0, e
	}
	if n.quoted {
		return 0, fail(n.pos, "integer must not be quoted")
	}
	if s == "" {
		return 0, fail(n.pos, "integer is empty")
	}
	start := 0
	if s[0] == '-' {
		start = 1
	}
	if start == len(s) || (len(s) > start+1 && s[start] == '0') {
		return 0, fail(n.pos, "integer must be canonical base 10")
	}
	for _, c := range s[start:] {
		if c < '0' || c > '9' {
			return 0, fail(n.pos, "expected base-10 integer")
		}
	}
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fail(n.pos, "integer outside 32-bit range")
	}
	return int(v), nil
}
func boolLiteral(n *node) (bool, error) {
	s, e := scalar(n)
	if e != nil {
		return false, e
	}
	if n.quoted {
		return false, fail(n.pos, "boolean must not be quoted")
	}
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fail(n.pos, "expected boolean true or false")
}
func bind(n *node) (string, bool, error) {
	if n == nil || n.kind != "map" {
		return "", false, nil
	}
	if e := only(n, "bind"); e != nil {
		return "", true, e
	}
	v, e := required(n, "bind")
	if e != nil {
		return "", true, e
	}
	s, e := stringLiteral(v)
	return s, true, e
}
func valueString(n *node) (spec.Value[string], error) {
	v := spec.Value[string]{Present: true, Pos: n.pos}
	name, b, e := bind(n)
	if e != nil {
		return v, e
	}
	if b {
		v.Bound = true
		v.Binding = name
		return v, nil
	}
	v.Literal, e = stringLiteral(n)
	return v, e
}
func valueInt(n *node) (spec.Value[int], error) {
	v := spec.Value[int]{Present: true, Pos: n.pos}
	name, b, e := bind(n)
	if e != nil {
		return v, e
	}
	if b {
		v.Bound = true
		v.Binding = name
		return v, nil
	}
	v.Literal, e = intLiteral(n)
	return v, e
}
func valueBool(n *node) (spec.Value[bool], error) {
	v := spec.Value[bool]{Present: true, Pos: n.pos}
	name, b, e := bind(n)
	if e != nil {
		return v, e
	}
	if b {
		v.Bound = true
		v.Binding = name
		return v, nil
	}
	v.Literal, e = boolLiteral(n)
	return v, e
}
func optString(n *node) (*spec.Value[string], error) {
	if n == nil {
		return nil, nil
	}
	v, e := valueString(n)
	return &v, e
}
func optInt(n *node) (*spec.Value[int], error) {
	if n == nil {
		return nil, nil
	}
	v, e := valueInt(n)
	return &v, e
}
func optBool(n *node) (*spec.Value[bool], error) {
	if n == nil {
		return nil, nil
	}
	v, e := valueBool(n)
	return &v, e
}
func optIntLit(n *node) (*int, error) {
	if n == nil {
		return nil, nil
	}
	i, e := intLiteral(n)
	return &i, e
}
func optBoolLit(n *node) (*bool, error) {
	if n == nil {
		return nil, nil
	}
	b, e := boolLiteral(n)
	return &b, e
}
func optWord(n *node) (string, error) {
	if n == nil {
		return "", nil
	}
	return stringLiteral(n)
}
func decodeDocument(root *node) (*spec.Document, error) {
	if e := only(root, "schema", "application", "screens", "dialogs"); e != nil {
		return nil, e
	}
	v, e := required(root, "schema")
	if e != nil {
		return nil, e
	}
	version, e := stringLiteral(v)
	if e != nil {
		return nil, e
	}
	app, e := required(root, "application")
	if e != nil {
		return nil, e
	}
	if e = only(app, "id"); e != nil {
		return nil, e
	}
	idnode, e := required(app, "id")
	if e != nil {
		return nil, e
	}
	id, e := stringLiteral(idnode)
	if e != nil {
		return nil, e
	}
	screensNode, e := required(root, "screens")
	if e != nil {
		return nil, e
	}
	screenItems, e := sequence(screensNode, "screens")
	if e != nil {
		return nil, e
	}
	doc := &spec.Document{Schema: version, Application: spec.Application{ID: id, Pos: app.pos}, Pos: root.pos}
	for _, n := range screenItems {
		s, e := decodeScreen(n)
		if e != nil {
			return nil, e
		}
		doc.Screens = append(doc.Screens, s)
	}
	if d := root.get("dialogs"); d != nil {
		items, e := sequence(d, "dialogs")
		if e != nil {
			return nil, e
		}
		for _, n := range items {
			item, e := decodeDialog(n)
			if e != nil {
				return nil, e
			}
			doc.Dialogs = append(doc.Dialogs, item)
		}
	}
	return doc, nil
}
func decodeScreen(n *node) (spec.Screen, error) {
	s := spec.Screen{Pos: n.pos}
	if e := only(n, "id", "title", "layout", "hero", "subtitle", "groups"); e != nil {
		return s, e
	}
	id, e := required(n, "id")
	if e != nil {
		return s, e
	}
	s.ID, e = stringLiteral(id)
	if e != nil {
		return s, e
	}
	t, e := required(n, "title")
	if e != nil {
		return s, e
	}
	s.Title, e = valueString(t)
	if e != nil {
		return s, e
	}
	layout, e := required(n, "layout")
	if e != nil {
		return s, e
	}
	s.Layout, e = stringLiteral(layout)
	if e != nil {
		return s, e
	}
	s.Hero, e = optString(n.get("hero"))
	if e != nil {
		return s, e
	}
	s.Subtitle, e = optString(n.get("subtitle"))
	if e != nil {
		return s, e
	}
	groupNode, e := required(n, "groups")
	if e != nil {
		return s, e
	}
	groups, e := sequence(groupNode, "groups")
	if e != nil {
		return s, e
	}
	for _, g := range groups {
		group, e := decodeGroup(g)
		if e != nil {
			return s, e
		}
		s.Groups = append(s.Groups, group)
	}
	return s, nil
}
func decodeGroup(n *node) (spec.Group, error) {
	g := spec.Group{Pos: n.pos}
	if e := only(n, "id", "title", "widgets"); e != nil {
		return g, e
	}
	id, e := required(n, "id")
	if e != nil {
		return g, e
	}
	g.ID, e = stringLiteral(id)
	if e != nil {
		return g, e
	}
	g.Title, e = optString(n.get("title"))
	if e != nil {
		return g, e
	}
	w, e := required(n, "widgets")
	if e != nil {
		return g, e
	}
	items, e := sequence(w, "widgets")
	if e != nil {
		return g, e
	}
	for _, item := range items {
		widget, e := decodeWidget(item)
		if e != nil {
			return g, e
		}
		g.Widgets = append(g.Widgets, widget)
	}
	return g, nil
}
func decodeWidget(n *node) (spec.Widget, error) {
	w := spec.Widget{Pos: n.pos}
	if e := only(n, "id", "type", "label", "detail", "variant", "value", "checked", "min", "max", "step", "wrap", "enabledWhen", "visibleWhen", "onPress", "onChange", "onCommit", "navigate"); e != nil {
		return w, e
	}
	id, e := required(n, "id")
	if e != nil {
		return w, e
	}
	w.ID, e = stringLiteral(id)
	if e != nil {
		return w, e
	}
	kind, e := required(n, "type")
	if e != nil {
		return w, e
	}
	w.Type, e = stringLiteral(kind)
	if e != nil {
		return w, e
	}
	w.Label, e = optString(n.get("label"))
	if e != nil {
		return w, e
	}
	w.Detail, e = optString(n.get("detail"))
	if e != nil {
		return w, e
	}
	w.Variant, e = optWord(n.get("variant"))
	if e != nil {
		return w, e
	}
	w.Value, e = optInt(n.get("value"))
	if e != nil {
		return w, e
	}
	w.Checked, e = optBool(n.get("checked"))
	if e != nil {
		return w, e
	}
	w.Min, e = optIntLit(n.get("min"))
	if e != nil {
		return w, e
	}
	w.Max, e = optIntLit(n.get("max"))
	if e != nil {
		return w, e
	}
	w.Step, e = optIntLit(n.get("step"))
	if e != nil {
		return w, e
	}
	w.Wrap, e = optBoolLit(n.get("wrap"))
	if e != nil {
		return w, e
	}
	w.EnabledWhen, e = optBool(n.get("enabledWhen"))
	if e != nil {
		return w, e
	}
	w.VisibleWhen, e = optBool(n.get("visibleWhen"))
	if e != nil {
		return w, e
	}
	w.OnPress, e = optWord(n.get("onPress"))
	if e != nil {
		return w, e
	}
	w.OnChange, e = optWord(n.get("onChange"))
	if e != nil {
		return w, e
	}
	w.OnCommit, e = optWord(n.get("onCommit"))
	if e != nil {
		return w, e
	}
	w.Navigate, e = optWord(n.get("navigate"))
	if e != nil {
		return w, e
	}
	return w, nil
}
func decodeDialog(n *node) (spec.Dialog, error) {
	d := spec.Dialog{Pos: n.pos}
	if e := only(n, "id", "title", "body", "confirmLabel", "cancelLabel", "onConfirm", "onCancel", "destructive"); e != nil {
		return d, e
	}
	for _, pair := range []struct {
		key string
		dst *string
	}{{"id", &d.ID}, {"confirmLabel", &d.ConfirmLabel}, {"cancelLabel", &d.CancelLabel}, {"onConfirm", &d.OnConfirm}, {"onCancel", &d.OnCancel}} {
		v, e := required(n, pair.key)
		if e != nil {
			return d, e
		}
		*pair.dst, e = stringLiteral(v)
		if e != nil {
			return d, e
		}
	}
	v, e := required(n, "title")
	if e != nil {
		return d, e
	}
	d.Title, e = valueString(v)
	if e != nil {
		return d, e
	}
	d.Body, e = optString(n.get("body"))
	if e != nil {
		return d, e
	}
	if b := n.get("destructive"); b != nil {
		d.Destructive, e = boolLiteral(b)
		if e != nil {
			return d, e
		}
	}
	return d, nil
}

// ParseFile is intentionally absent from this parser. M2 tooling will decide
// where files may be read; this package only consumes caller-provided bytes.
var _ = fmt.Sprintf

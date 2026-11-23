package rui

import (
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
)

type boundValue struct {
	typ     spec.BindingType
	text    string
	integer int
	boolean bool
}

// BindingError identifies a rejected getter without retaining its value or
// panic payload. Applications may log it without disclosing sensitive state.
type BindingError struct{ Key, Reason string }

func (e *BindingError) Error() string { return fmt.Sprintf("RUI binding %q: %s", e.Key, e.Reason) }

func evaluateBinding(name string, b binding) (value boundValue, err error) {
	defer func() {
		if recover() != nil {
			err = &BindingError{Key: name, Reason: "getter failed"}
		}
	}()
	value.typ = b.typ
	switch b.typ {
	case spec.StringBinding:
		value.text = b.text()
		if !utf8.ValidString(value.text) {
			return value, &BindingError{Key: name, Reason: "invalid UTF-8"}
		}
	case spec.IntBinding:
		value.integer = b.integer()
	case spec.BoolBinding:
		value.boolean = b.boolean()
	default:
		return value, &BindingError{Key: name, Reason: "invalid binding type"}
	}
	return value, nil
}

func validateBindingValue(f *bundle.File, key string, value boundValue) error {
	for _, dep := range f.Dependencies[key] {
		if value.typ == spec.StringBinding {
			limit := 80
			if dep.Property == "subtitle" || dep.Property == "detail" || dep.Property == "body" {
				limit = 240
			}
			if dep.Property == "label" {
				limit = 160
			}
			if len(value.text) > limit {
				return &BindingError{Key: key, Reason: "text exceeds property limit"}
			}
		}
		if value.typ == spec.IntBinding {
			w := f.Document.Screens[dep.Screen].Groups[dep.Group].Widgets[dep.Widget]
			low, high := 0, 1439
			if w.Type != "timePicker" {
				low, high = *w.Min, *w.Max
			}
			if value.integer < low || value.integer > high {
				return &BindingError{Key: key, Reason: "value outside property range"}
			}
		}
	}
	return nil
}

func evaluateDirty(f *bundle.File, bindings map[string]binding, previous map[string]boundValue, dirty map[string]bool, all bool) (map[string]boundValue, map[int]bool, map[string]bool, error) {
	values := make(map[string]boundValue, len(f.Manifest.Bindings))
	for key, value := range previous {
		if _, ok := f.Manifest.Bindings[key]; ok {
			values[key] = value
		}
	}
	affected := map[int]bool{}
	failed := map[string]bool{}
	var errs []error
	var names []string
	for key := range f.Manifest.Bindings {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		old, cached := values[key]
		if !all && !dirty[key] && cached {
			continue
		}
		value, err := evaluateBinding(key, bindings[key])
		if err == nil {
			err = validateBindingValue(f, key, value)
		}
		if err != nil {
			failed[key] = true
			errs = append(errs, err)
			continue
		}
		values[key] = value
		if !cached || value != old {
			for _, dep := range f.Dependencies[key] {
				if dep.Screen >= 0 {
					affected[dep.Screen] = true
				}
			}
		}
	}
	return values, affected, failed, errors.Join(errs...)
}

func frozenSnapshot(values map[string]boundValue) stateSnapshot {
	states := make(stateSnapshot, len(values))
	for key, value := range values {
		value := value
		states[key] = binding{typ: value.typ, text: func() string { return value.text }, integer: func() int { return value.integer }, boolean: func() bool { return value.boolean }}
	}
	return states
}

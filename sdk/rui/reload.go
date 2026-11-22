package rui

import (
	"context"
	"errors"
	"fmt"

	"roundos/sdk/rui/spec"
	"roundos/sdk/ui"
)

// ErrReloadDeferred means the caller should retry after the editor closes.
var ErrReloadDeferred = errors.New("layout reload deferred while a text editor is active")

// Replace validates a compiled layout against existing registrations and state
// before changing anything. Call it on the Dispatch/Flush event loop. It never
// invokes action handlers, parses YAML, or changes application-owned state.
func (r *Runtime) Replace(data []byte) error {
	return r.ReplaceContext(context.Background(), data)
}

// ReplaceContext checks cancellation before publication, including after
// staging getters. Once publication begins, successful delivery commits the
// matching tree/actions even if cancellation arrives during that delivery.
func (r *Runtime) ReplaceContext(ctx context.Context, data []byte) (err error) {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if editor, ok := r.publisher.(interface{ TextEditorActive() bool }); ok && editor.TextEditorActive() {
		return ErrReloadDeferred
	}
	if editor, ok := r.publisher.(interface{ SecureEditorActive() bool }); ok && editor.SecureEditorActive() {
		return ErrReloadDeferred
	}
	r.mu.Lock()
	if !r.sealed {
		r.mu.Unlock()
		return fmt.Errorf("validate registrations before replacing a layout")
	}
	id := r.file.Document.Application.ID
	bindings := make(map[string]binding, len(r.bindings))
	for k, v := range r.bindings {
		bindings[k] = v
	}
	handlers := make(map[string]handler, len(r.handlers))
	for k, v := range r.handlers {
		handlers[k] = v
	}
	dialog := r.dialogID
	needsClear := r.dialogNeedsClear
	lastView := r.lastView
	wasDirty := r.dirty
	r.dirty = false
	r.mu.Unlock()
	committed := false
	defer func() {
		if !committed {
			r.mu.Lock()
			r.dirty = r.dirty || wasDirty
			r.mu.Unlock()
		}
	}()
	candidate, err := NewForApp(r.publisher, data, id)
	if err != nil {
		return err
	}
	for name, typ := range candidate.file.Manifest.Bindings {
		b, ok := bindings[name]
		if !ok || b.typ != typ {
			return fmt.Errorf("replacement needs unavailable binding %q (%s)", name, typ)
		}
	}
	for _, act := range candidate.file.Manifest.Actions {
		h, ok := handlers[act.Name]
		if !ok || h.kind != act.ValueType {
			return fmt.Errorf("replacement needs unavailable action %q (%s)", act.Name, act.ValueType)
		}
	}
	if _, ok := candidate.dialogActions[dialog]; !ok {
		dialog = ""
	}
	view, err := stageReload(candidate, bindings, dialog)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Any required dialog reset uses the old tree. If candidate publication
	// fails, displayed controls still agree with the active runtime/actions.
	if needsClear && view.Dialog != nil && lastView != nil && lastView.Dialog != nil {
		cleared := *lastView
		cleared.Dialog = nil
		if err := r.publish(cleared); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.publish(view); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.file = candidate.file
	r.intActions, r.boolActions, r.dialogActions = candidate.intActions, candidate.boolActions, candidate.dialogActions
	r.dialogID = dialog
	r.dialogNeedsClear = false
	committed = true
	return nil
}

func stageReload(r *Runtime, bindings map[string]binding, dialog string) (view ui.View, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("replacement binding evaluation failed")
		}
	}()
	// Resolve required getters once, including fields currently hidden or in
	// inactive dialogs. Validation must not depend on today's visibility.
	states := make(stateSnapshot, len(r.file.Manifest.Bindings))
	for name, typ := range r.file.Manifest.Bindings {
		b := bindings[name]
		switch typ {
		case spec.StringBinding:
			value := b.text()
			states[name] = binding{typ: typ, text: func() string { return value }}
		case spec.IntBinding:
			value := b.integer()
			states[name] = binding{typ: typ, integer: func() int { return value }}
		case spec.BoolBinding:
			value := b.boolean()
			states[name] = binding{typ: typ, boolean: func() bool { return value }}
		}
	}
	text := func(value *spec.Value[string], limit int) error { _, err := states.optText(value, limit); return err }
	for _, screen := range r.file.Document.Screens {
		if err := text(&screen.Title, 80); err != nil {
			return view, err
		}
		if err := text(screen.Hero, 80); err != nil {
			return view, err
		}
		if err := text(screen.Subtitle, 240); err != nil {
			return view, err
		}
		for _, group := range screen.Groups {
			if err := text(group.Title, 80); err != nil {
				return view, err
			}
			for _, widget := range group.Widgets {
				if err := text(widget.Label, 160); err != nil {
					return view, err
				}
				if err := text(widget.Detail, 240); err != nil {
					return view, err
				}
				if widget.Value != nil {
					value, err := states.integer(*widget.Value)
					if err != nil {
						return view, err
					}
					low, high := 0, 1439
					if widget.Type != "timePicker" {
						low, high = *widget.Min, *widget.Max
					}
					if value < low || value > high {
						return view, fmt.Errorf("replacement widget %q value outside declared range", widget.ID)
					}
				}
			}
		}
	}
	for _, item := range r.file.Document.Dialogs {
		if err := text(&item.Title, 80); err != nil {
			return view, err
		}
		if err := text(item.Body, 240); err != nil {
			return view, err
		}
	}
	// A hidden control or inactive dialog can become visible without another
	// layout replacement. Verify that every such snapshot fits the IPC frame
	// before discarding the last-good layout, using the same frozen values.
	if _, err := buildViewWithVisibility(r.file, states, "", true); err != nil {
		return view, fmt.Errorf("replacement all-visible snapshot: %w", err)
	}
	for _, item := range r.file.Document.Dialogs {
		if _, err := buildViewWithVisibility(r.file, states, item.ID, true); err != nil {
			return view, fmt.Errorf("replacement dialog %q snapshot: %w", item.ID, err)
		}
	}
	return buildView(r.file, states, dialog)
}

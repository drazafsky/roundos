package rui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"roundos/internal/wire"
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
	// Legacy input has no layout revision. Keep the meaning of a retained
	// toggle action and dialog ID stable so queued input cannot acquire a new
	// source, or dismiss an old dialog while leaving its replacement hidden.
	for action, source := range candidate.boolActions {
		if previous, ok := r.boolContracts[action]; ok && previous != source {
			return fmt.Errorf("replacement remaps boolean action %q", action)
		}
	}
	dialogCount := len(r.dialogContracts)
	for id, actions := range candidate.dialogActions {
		if previous, ok := r.dialogContracts[id]; ok && previous != actions {
			return fmt.Errorf("replacement remaps dialog actions %q", id)
		}
		if _, known := r.dialogContracts[id]; !known {
			dialogCount++
		}
	}
	if dialogCount > maxDialogContracts {
		return fmt.Errorf("dialog identity history exhausted; restart development session")
	}
	if _, ok := candidate.dialogActions[dialog]; !ok {
		dialog = ""
	}
	view, values, err := stageReload(ctx, candidate, bindings, dialog)
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
		if err := r.publishReplacement(ctx, cleared); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.publishReplacement(ctx, view); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.file = candidate.file
	for action, source := range candidate.boolActions {
		r.boolContracts[action] = source
	}
	for id, actions := range candidate.dialogActions {
		r.dialogContracts[id] = actions
	}
	r.cache = values
	r.intActions, r.boolActions, r.dialogActions = candidate.intActions, candidate.boolActions, candidate.dialogActions
	r.dialogID = dialog
	r.dialogNeedsClear = false
	committed = true
	return nil
}

func stageReload(ctx context.Context, r *Runtime, bindings map[string]binding, dialog string) (view ui.View, values map[string]boundValue, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("replacement binding evaluation failed")
		}
	}()
	// Resolve required getters once, including fields currently hidden or in
	// inactive dialogs. Validation must not depend on today's visibility.
	values = make(map[string]boundValue, len(r.file.Manifest.Bindings))
	for name := range r.file.Manifest.Bindings {
		if err := ctx.Err(); err != nil {
			return view, nil, err
		}
		value, err := evaluateBinding(name, bindings[name])
		if err != nil {
			return view, nil, err
		}
		values[name] = value
	}
	states := frozenSnapshot(values)
	text := func(value *spec.Value[string], limit int) error { _, err := states.optText(value, limit); return err }
	for _, screen := range r.file.Document.Screens {
		if err := text(&screen.Title, 80); err != nil {
			return view, nil, err
		}
		if err := text(screen.Hero, 80); err != nil {
			return view, nil, err
		}
		if err := text(screen.Subtitle, 240); err != nil {
			return view, nil, err
		}
		for _, group := range screen.Groups {
			if err := text(group.Title, 80); err != nil {
				return view, nil, err
			}
			for _, widget := range group.Widgets {
				if err := ctx.Err(); err != nil {
					return view, nil, err
				}
				if err := text(widget.Label, 160); err != nil {
					return view, nil, err
				}
				if err := text(widget.Detail, 240); err != nil {
					return view, nil, err
				}
				if widget.Value != nil {
					value, err := states.integer(*widget.Value)
					if err != nil {
						return view, nil, err
					}
					low, high := 0, 1439
					if widget.Type != "timePicker" {
						low, high = *widget.Min, *widget.Max
					}
					if value < low || value > high {
						return view, nil, fmt.Errorf("replacement widget %q value outside declared range", widget.ID)
					}
				}
			}
		}
	}
	for _, item := range r.file.Document.Dialogs {
		if err := ctx.Err(); err != nil {
			return view, nil, err
		}
		if err := text(&item.Title, 80); err != nil {
			return view, nil, err
		}
		if err := text(item.Body, 240); err != nil {
			return view, nil, err
		}
	}
	// A hidden control or inactive dialog can become visible without another
	// layout replacement. Verify that every such snapshot fits the IPC frame
	// before discarding the last-good layout, using the same frozen values.
	allVisible, err := buildViewWithVisibility(r.file, states, "", true)
	if err != nil {
		return view, nil, fmt.Errorf("replacement all-visible snapshot: %w", err)
	}
	frame, err := json.Marshal(wire.Message{Version: wire.Version, Type: "view", View: &allVisible})
	if err != nil {
		return view, nil, err
	}
	for _, item := range r.file.Document.Dialogs {
		if err := ctx.Err(); err != nil {
			return view, nil, err
		}
		resolved, err := resolveDialog(item, states)
		if err != nil {
			return view, nil, err
		}
		data, err := json.Marshal(resolved)
		if err != nil {
			return view, nil, err
		}
		// The nonempty view has no dialog. Adding its JSON member costs
		// exactly this key/comma plus the encoded dialog, independent of
		// screen count. Never rebuild or serialize every screen per dialog.
		size := len(frame) + len(",\"dialog\":") + len(data)
		if size > wire.MaxMessage {
			return view, nil, fmt.Errorf("replacement dialog %q snapshot exceeds %d-byte IPC frame (%d bytes)", item.ID, wire.MaxMessage, size)
		}
	}
	view, err = buildView(r.file, states, dialog)
	return view, values, err
}

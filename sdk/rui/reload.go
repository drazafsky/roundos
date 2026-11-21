package rui

import (
	"errors"
	"fmt"

	"roundos/sdk/ui"
)

// ErrReloadDeferred means the caller should retry after the secure editor closes.
var ErrReloadDeferred = errors.New("layout reload deferred while secure editor is active")

// Replace validates a compiled layout against existing registrations and state
// before changing anything. Call it on the Dispatch/Flush event loop. It never
// invokes action handlers, parses YAML, or changes application-owned state.
func (r *Runtime) Replace(data []byte) (err error) {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
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
	r.mu.Unlock()
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
	if _, err := stageReload(candidate, bindings, dialog); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.file = candidate.file
	r.intActions, r.boolActions, r.dialogActions = candidate.intActions, candidate.boolActions, candidate.dialogActions
	if dialog != r.dialogID {
		r.clearDialogLocked()
	}
	r.dialogID = dialog
	r.dirty = true
	return nil
}

func stageReload(r *Runtime, bindings map[string]binding, dialog string) (view ui.View, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("replacement binding evaluation failed")
		}
	}()
	return buildView(r.file, stateSnapshot(bindings), dialog)
}

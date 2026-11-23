// Package rui runs compiled RoundOS declarative UI layouts through the existing
// Go SDK and v7 IPC client. It does not parse YAML or execute embedded scripts.
// The application owns state and event handling; the shell owns presentation.
package rui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"

	"roundos/internal/rui/compile"
	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
	"roundos/sdk/ui"
)

// Publisher is satisfied by *app.Client; tests may supply an in-memory publisher.
type Publisher interface{ Publish(ui.View) error }

type binding struct {
	typ     spec.BindingType
	text    func() string
	integer func() int
	boolean func() bool
}
type handler struct {
	kind    string
	action  func(context.Context, ActionEvent) error
	integer func(context.Context, int) error
	boolean func(context.Context, bool) error
}

// ActionEvent identifies an untyped press, including a legacy delta if present.
type ActionEvent struct {
	Name  string
	Delta int
}
type intRange struct{ min, max int }

const maxDialogContracts = 1024

type boolSource struct {
	binding string
	literal bool
	bound   bool
}

// Runtime is a single-application, serialized publisher and event dispatcher.
// Bindings and handlers are registered before ValidateBindings. Invalidate is
// safe from background goroutines. Dispatch and Flush belong on one app event
// loop; neither creates a competing reader for Client.Events().
type Runtime struct {
	mu                     sync.Mutex
	flushMu                sync.Mutex
	file                   *bundle.File
	publisher              Publisher
	bindings               map[string]binding
	handlers               map[string]handler
	intActions             map[string]intRange
	boolActions            map[string]boolSource
	dialogActions          map[string][2]string
	boolContracts          map[string]boolSource
	dialogContracts        map[string][2]string
	dialogID               string
	dialogNeedsClear       bool
	lastView               *ui.View
	cache                  map[string]boundValue
	dirtyKeys              map[string]bool
	dirtyAll               bool
	lastEventID            uint64
	authoritative          bool
	visible, dirty, sealed bool
}

// New loads an embedded compiled layout, checking its app ID against the
// authenticated identity supplied by the RoundOS supervisor.
func New(p Publisher, data []byte) (*Runtime, error) {
	return NewForApp(p, data, os.Getenv("ROUNDOS_APP_ID"))
}

// NewForApp is useful for offline tests and trusted external SDK integration.
// A mismatched app ID is always rejected. The shell still authenticates IPC.
func NewForApp(p Publisher, data []byte, expectedAppID string) (*Runtime, error) {
	if p == nil {
		return nil, fmt.Errorf("nil RUI publisher")
	}
	if expectedAppID == "" {
		return nil, fmt.Errorf("missing authenticated ROUNDOS_APP_ID")
	}
	f, err := compile.Verify(data)
	if err != nil {
		return nil, fmt.Errorf("RUI bundle verification: %w", err)
	}
	if f.Document.Application.ID != expectedAppID {
		return nil, fmt.Errorf("RUI application ID %q differs from authenticated ID %q", f.Document.Application.ID, expectedAppID)
	}
	r := &Runtime{file: f, publisher: p, bindings: map[string]binding{}, handlers: map[string]handler{}, intActions: map[string]intRange{}, boolActions: map[string]boolSource{}, dialogActions: map[string][2]string{}, dirty: true}
	for _, s := range f.Document.Screens {
		for _, g := range s.Groups {
			for _, w := range g.Widgets {
				if w.OnChange != "" {
					// v7 carries the action name but not the new checked value.
					// A live mutable toggle must therefore expose its authoritative
					// state through a bool binding (unlike an offline preview).
					if w.Checked == nil || !w.Checked.Bound {
						return nil, fmt.Errorf("live RUI toggle %q requires a checked bool binding", w.ID)
					}
					src := boolSource{}
					if w.Checked != nil {
						src.bound = w.Checked.Bound
						src.binding = w.Checked.Binding
						src.literal = w.Checked.Literal
					}
					if prev, ok := r.boolActions[w.OnChange]; ok && prev != src {
						return nil, fmt.Errorf("ambiguous v7 boolean action %q: distinct checked sources", w.OnChange)
					}
					r.boolActions[w.OnChange] = src
				}
				if w.OnCommit != "" {
					rg := intRange{}
					if w.Type == "timePicker" {
						rg = intRange{0, 1439}
					} else if w.Min != nil && w.Max != nil {
						rg = intRange{*w.Min, *w.Max}
					}
					if prev, ok := r.intActions[w.OnCommit]; ok && prev != rg {
						return nil, fmt.Errorf("ambiguous v7 integer action %q: distinct value ranges", w.OnCommit)
					}
					r.intActions[w.OnCommit] = rg
				}
			}
		}
	}
	for _, d := range f.Document.Dialogs {
		r.dialogActions[d.ID] = [2]string{d.OnConfirm, d.OnCancel}
	}
	if os.Getenv("ROUNDOS_RUI_DEV_DIR") != "" {
		if tracker, ok := p.(interface{ TrackEditorResults() }); ok {
			tracker.TrackEditorResults()
		}
	}
	r.boolContracts = make(map[string]boolSource, len(r.boolActions))
	for action, source := range r.boolActions {
		r.boolContracts[action] = source
	}
	r.dialogContracts = make(map[string][2]string, len(r.dialogActions))
	if len(r.dialogActions) > maxDialogContracts {
		return nil, fmt.Errorf("too many dialog identities")
	}
	for id, actions := range r.dialogActions {
		r.dialogContracts[id] = actions
	}
	return r, nil
}
func (r *Runtime) registerBinding(name string, b binding) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("cannot register binding after validation")
	}
	expected, ok := r.file.Manifest.Bindings[name]
	if !ok {
		return fmt.Errorf("undeclared RUI binding %q", name)
	}
	if expected != b.typ {
		return fmt.Errorf("binding %q requires %s, got %s", name, expected, b.typ)
	}
	if _, exists := r.bindings[name]; exists {
		return fmt.Errorf("duplicate RUI binding %q", name)
	}
	r.bindings[name] = b
	return nil
}
func (r *Runtime) BindString(name string, get func() string) error {
	if get == nil {
		return fmt.Errorf("nil string getter")
	}
	return r.registerBinding(name, binding{typ: spec.StringBinding, text: get})
}
func (r *Runtime) BindInt(name string, get func() int) error {
	if get == nil {
		return fmt.Errorf("nil integer getter")
	}
	return r.registerBinding(name, binding{typ: spec.IntBinding, integer: get})
}
func (r *Runtime) BindBool(name string, get func() bool) error {
	if get == nil {
		return fmt.Errorf("nil boolean getter")
	}
	return r.registerBinding(name, binding{typ: spec.BoolBinding, boolean: get})
}
func (r *Runtime) registerAction(name string, h handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return fmt.Errorf("cannot register action after validation")
	}
	declared := ""
	for _, a := range r.file.Manifest.Actions {
		if a.Name == name {
			declared = a.ValueType
			break
		}
	}
	if declared == "" {
		return fmt.Errorf("undeclared RUI action %q", name)
	}
	if declared != h.kind {
		return fmt.Errorf("action %q requires %s handler, got %s", name, declared, h.kind)
	}
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("duplicate RUI action %q", name)
	}
	r.handlers[name] = h
	return nil
}
func (r *Runtime) OnAction(name string, fn func(context.Context, ActionEvent) error) error {
	if fn == nil {
		return fmt.Errorf("nil action handler")
	}
	return r.registerAction(name, handler{kind: "action", action: fn})
}
func (r *Runtime) OnInt(name string, fn func(context.Context, int) error) error {
	if fn == nil {
		return fmt.Errorf("nil integer handler")
	}
	return r.registerAction(name, handler{kind: "int", integer: fn})
}
func (r *Runtime) OnBool(name string, fn func(context.Context, bool) error) error {
	if fn == nil {
		return fmt.Errorf("nil boolean handler")
	}
	return r.registerAction(name, handler{kind: "bool", boolean: fn})
}

// ValidateBindings seals registration and checks the complete compiled manifest.
// It must pass before the first event or view is processed.
func (r *Runtime) ValidateBindings() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var missing []string
	for name := range r.file.Manifest.Bindings {
		if _, ok := r.bindings[name]; !ok {
			missing = append(missing, "binding "+name)
		}
	}
	for _, act := range r.file.Manifest.Actions {
		if _, ok := r.handlers[act.Name]; !ok {
			missing = append(missing, "action "+act.Name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing RUI registrations: %v", missing)
	}
	r.sealed = true
	return nil
}

// Invalidate coalesces changed keys. No keys means reevaluate all bindings.
func (r *Runtime) Invalidate(keys ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range keys {
		_, declared := r.file.Manifest.Bindings[k]
		_, registered := r.bindings[k]
		if !declared && !registered {
			return fmt.Errorf("undeclared invalidation key %q", k)
		}
	}
	if len(keys) == 0 {
		r.dirtyAll = true
	} else {
		if r.dirtyKeys == nil {
			r.dirtyKeys = map[string]bool{}
		}
		for _, key := range keys {
			r.dirtyKeys[key] = true
		}
	}
	r.dirty = true
	return nil
}
func (r *Runtime) ShowDialog(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.dialogActions[id]; !ok {
		return fmt.Errorf("undeclared RUI dialog %q", id)
	}
	r.dialogID = id
	r.dirty = true
	return nil
}
func (r *Runtime) clearDialogLocked() {
	if r.dialogID != "" {
		r.dialogNeedsClear = true
	}
	r.dialogID = ""
	r.dirty = true
}

func (r *Runtime) HideDialog() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clearDialogLocked()
}

// Dispatch consumes existing SDK input and visibility events without creating
// another event reader. For v7 toggles, the wire protocol omits checked state:
// the SDK computes !currentBoundValue. OnBool handlers must update that state.
// For picker/slider commits, Delta is the absolute integer value.
func (r *Runtime) Dispatch(ev app.Event) (bool, error) {
	return r.DispatchContext(context.Background(), ev)
}
func (r *Runtime) DispatchContext(ctx context.Context, ev app.Event) (bool, error) {
	if ev.Type == "text_result" && ev.TextResult != nil {
		if tracker, ok := r.publisher.(interface{ AcknowledgeTextResult(string) }); ok {
			tracker.AcknowledgeTextResult(ev.TextResult.ID)
		}
		return false, nil
	}
	if ev.Type == "visibility" {
		r.mu.Lock()
		r.visible = ev.Visible
		if ev.Visible {
			r.dirty = true
			r.dirtyAll = true
		}
		r.mu.Unlock()
		return true, nil
	}
	if ev.Type != "input" {
		return false, nil
	}
	r.mu.Lock()
	if !r.sealed {
		r.mu.Unlock()
		return false, fmt.Errorf("RUI registrations must be validated before dispatch")
	}
	h, ok := r.handlers[ev.Action]
	declared := false
	for _, action := range r.file.Manifest.Actions {
		if action.Name == ev.Action {
			declared = true
			break
		}
	}
	if !ok {
		r.mu.Unlock()
		return false, nil
	}
	if !declared {
		// Retained registrations remain owned by RUI after layout removal.
		// Consume delayed input so a caller's legacy fallback cannot run it.
		r.mu.Unlock()
		return true, nil
	}
	if !r.visible {
		r.mu.Unlock()
		return true, nil
	}
	rg := r.intActions[ev.Action]
	bs := r.boolActions[ev.Action]
	bindings := r.bindings
	file, dialog, lastView := r.file, r.dialogID, r.lastView
	if ev.UI != nil && ev.UI.ID <= r.lastEventID {
		r.mu.Unlock()
		return true, nil
	}
	r.mu.Unlock()
	allowed, guardErr := actionAllowed(file, bindings, dialog, ev.Action, ev.UI)
	if guardErr != nil || !allowed {
		return true, r.rejectInput(guardErr)
	}
	if ev.UI != nil {
		revisions, ok := r.publisher.(interface{ LayoutRevision() uint64 })
		if !ok || lastView == nil || ev.UI.LayoutRevision != revisions.LayoutRevision() || ev.UI.Action != ev.Action {
			return true, r.rejectInput(fmt.Errorf("stale RUI UI event"))
		}
		if err := wire.ValidateUIEvent(*lastView, *ev.UI); err != nil {
			return true, r.rejectInput(err)
		}
		r.mu.Lock()
		r.lastEventID = ev.UI.ID
		r.mu.Unlock()
	}
	r.mu.Lock()
	// Clear any active dialog when its confirm/cancel action arrives.
	if acts, exists := r.dialogActions[r.dialogID]; exists && (ev.Action == acts[0] || ev.Action == acts[1]) {
		r.clearDialogLocked()
	}
	r.mu.Unlock()
	var err error
	switch h.kind {
	case "action":
		err = h.action(ctx, ActionEvent{Name: ev.Action, Delta: ev.Delta})
	case "int":
		if ev.UI != nil && ev.UI.Int != nil {
			ev.Delta = *ev.UI.Int
		}
		if ev.Delta < rg.min || ev.Delta > rg.max {
			return true, r.rejectInput(fmt.Errorf("RUI action %q value outside declared range", ev.Action))
		}
		err = h.integer(ctx, ev.Delta)
	case "bool":
		if ev.UI != nil && ev.UI.Bool != nil {
			err = h.boolean(ctx, *ev.UI.Bool)
			break
		}
		current := bs.literal
		if bs.bound {
			b, exists := bindings[bs.binding]
			if !exists || b.boolean == nil {
				return true, fmt.Errorf("missing bool source %q for action %q", bs.binding, ev.Action)
			}
			value, evaluateErr := evaluateBinding(bs.binding, b)
			if evaluateErr != nil {
				return true, r.rejectInput(evaluateErr)
			}
			current = value.boolean
		}
		err = h.boolean(ctx, !current)
	default:
		return true, fmt.Errorf("unsupported RUI handler kind %q", h.kind)
	}
	if err != nil {
		return true, r.rejectInput(err)
	}
	return true, r.Invalidate()
}

// Flush evaluates bindings on the application's calling goroutine. Failures
// retain dirty state for retry; reopening a dismissed dialog can publish a
// dialog-free transition first. Concurrent invalidations survive this Flush.
func (r *Runtime) Flush() error {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	r.mu.Lock()
	if !r.sealed {
		r.mu.Unlock()
		return fmt.Errorf("RUI registrations must be validated before Flush")
	}
	if !r.visible || !r.dirty {
		r.mu.Unlock()
		return nil
	}
	r.dirty = false
	bindings := make(map[string]binding, len(r.bindings))
	for k, v := range r.bindings {
		bindings[k] = v
	}
	dialog := r.dialogID
	needsClear := r.dialogNeedsClear
	keys, all, cache := r.dirtyKeys, r.dirtyAll || r.cache == nil, r.cache
	r.dirtyKeys = nil
	r.dirtyAll = false
	base := r.lastView
	authoritative := r.authoritative
	r.mu.Unlock()
	values, affected, failed, bindingErr := evaluateDirty(r.file, bindings, cache, keys, all)
	retry := func(full bool) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.dirty = true
		r.dirtyAll = r.dirtyAll || full
		if r.dirtyKeys == nil {
			r.dirtyKeys = map[string]bool{}
		}
		for key := range keys {
			r.dirtyKeys[key] = true
		}
		for key := range failed {
			r.dirtyKeys[key] = true
		}
	}
	if len(values) != len(r.file.Manifest.Bindings) {
		retry(true)
		return bindingErr
	}
	v, err := buildViewIncremental(r.file, frozenSnapshot(values), dialog, base, affected)
	if err == nil && dialog != "" && needsClear {
		// The v7 shell keeps a dismissed dialog hidden until it sees no dialog
		// or a different ID. Publish the clear even if a handler immediately
		// reopens the same declaration, before the client can deduplicate it.
		cleared := v
		cleared.Dialog = nil
		err = r.publish(cleared)
	}
	if err == nil {
		if authoritative {
			err = r.publishAuthoritative(v)
		} else {
			err = r.publish(v)
		}
	}
	if err != nil {
		retry(all)
		return err
	}
	r.mu.Lock()
	if needsClear {
		r.dialogNeedsClear = false
	}
	r.cache = values
	r.authoritative = false
	if len(failed) > 0 {
		r.dirty = true
		if r.dirtyKeys == nil {
			r.dirtyKeys = map[string]bool{}
		}
		for key := range failed {
			r.dirtyKeys[key] = true
		}
	}
	r.mu.Unlock()
	return bindingErr
}

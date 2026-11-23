package rui

import (
	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/rui/bundle"
	"roundos/sdk/rui/spec"
	"roundos/sdk/ui"
)

func actionAllowed(file *bundle.File, bindings map[string]binding, dialog, action string, event *app.UIEvent) (bool, error) {
	if dialog != "" {
		for _, d := range file.Document.Dialogs {
			if d.ID == dialog {
				return event == nil && (action == d.OnConfirm || action == d.OnCancel), nil
			}
		}
		return false, nil
	}
	condition := func(v *spec.Value[bool]) (bool, error) {
		if v == nil {
			return true, nil
		}
		value, err := evaluateBinding(v.Binding, bindings[v.Binding])
		return value.boolean, err
	}
	for _, screen := range file.Document.Screens {
		if event != nil && event.Screen != screen.ID {
			continue
		}
		for _, group := range screen.Groups {
			for _, widget := range group.Widgets {
				if event != nil && event.Widget != widget.ID {
					continue
				}
				if action != widget.OnPress && action != widget.OnChange && action != widget.OnCommit {
					continue
				}
				show, err := condition(widget.VisibleWhen)
				if err != nil {
					return false, err
				}
				if !show {
					continue
				}
				enabled, err := condition(widget.EnabledWhen)
				if err != nil {
					return false, err
				}
				if !enabled {
					continue
				}
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *Runtime) rejectInput(err error) error {
	r.mu.Lock()
	r.authoritative = true
	r.mu.Unlock()
	_ = r.Invalidate()
	return err
}

func (r *Runtime) publishAuthoritative(view ui.View) error {
	if publisher, ok := r.publisher.(interface{ PublishAuthoritative(ui.View) error }); ok {
		if err := publisher.PublishAuthoritative(view); err != nil {
			return err
		}
	} else if err := r.publisher.Publish(view); err != nil {
		return err
	}
	copy := wire.CopyView(view)
	r.mu.Lock()
	r.lastView = &copy
	r.mu.Unlock()
	return nil
}

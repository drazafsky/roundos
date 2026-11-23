package main

import (
	"context"
	"time"

	"roundos/internal/timerstate"
	"roundos/sdk/app"
	"roundos/sdk/rui"
	"roundos/sdk/ui"
)

// Only the legacy scalar rotary route needs an adapter. Every visible field
// and control is lowered from the embedded YAML by RUI.
type timerPublisher struct{ *app.Client }

func timerLegacy(view ui.View) ui.View           { view.RotaryAction = "adjust"; return view }
func (p timerPublisher) Publish(v ui.View) error { return p.Client.Publish(timerLegacy(v)) }
func (p timerPublisher) PublishAuthoritative(v ui.View) error {
	return p.Client.PublishAuthoritative(timerLegacy(v))
}
func (p timerPublisher) PublishSnapshot(ctx context.Context, v ui.View) error {
	return p.Client.PublishSnapshot(ctx, timerLegacy(v))
}

type timerModel struct {
	timer *timerstate.Timer
	now   time.Time
}

func (s *timerModel) minutes() int { return int(s.timer.Left(s.now) / time.Minute) }
func (s *timerModel) status() string {
	if s.timer.Running {
		return "RUNNING IN BACKGROUND"
	}
	return "ROTATE TO ADJUST"
}
func (s *timerModel) toggleLabel() string {
	if s.timer.Running {
		return "PAUSE"
	}
	return "START"
}

func registerTimer(r *rui.Runtime, s *timerModel) error {
	regs := []func() error{
		func() error { return r.BindString("timer.display", func() string { return s.timer.Label(s.now) }) },
		func() error { return r.BindString("timer.status", s.status) },
		func() error { return r.BindString("timer.toggleLabel", s.toggleLabel) },
		func() error { return r.BindInt("timer.minutes", s.minutes) },
		func() error { return r.BindBool("timer.stopped", func() bool { return !s.timer.Running }) },
		func() error {
			return r.OnAction("toggle", func(context.Context, rui.ActionEvent) error { s.timer.Toggle(s.now); return s.timer.Save() })
		},
		func() error {
			return r.OnInt("set_minutes", func(_ context.Context, value int) error {
				if !s.timer.Running {
					s.timer.Rotate(value-int(s.timer.Remaining/time.Minute), s.now)
					return s.timer.Save()
				}
				return nil
			})
		},
		func() error {
			return r.OnAction("request_reset", func(context.Context, rui.ActionEvent) error { return r.ShowDialog("timer-reset") })
		},
		func() error {
			return r.OnAction("cancel_reset", func(context.Context, rui.ActionEvent) error { return nil })
		},
		func() error {
			return r.OnAction("confirm_reset", func(context.Context, rui.ActionEvent) error {
				if s.timer.Running {
					s.timer.Toggle(s.now)
				}
				s.timer.Rotate(5-int(s.timer.Remaining/time.Minute), s.now)
				return s.timer.Save()
			})
		},
	}
	for _, reg := range regs {
		if err := reg(); err != nil {
			return err
		}
	}
	return r.ValidateBindings()
}

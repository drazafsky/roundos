package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"roundos/sdk/rui"
)

type clockState struct {
	now                                               time.Time
	use24                                             bool
	alarmMinute, snoozeMinutes, baseDate, exampleDate int
	label                                             string
}

func newClockState(now time.Time) *clockState {
	day := int(now.UTC().Unix() / 86400)
	return &clockState{now: now, use24: true, alarmMinute: 8 * 60, snoozeMinutes: 5, baseDate: day, exampleDate: day}
}

func (s *clockState) display() string {
	format := "15:04"
	if !s.use24 {
		format = "03:04 PM"
	}
	return s.now.Format(format)
}
func (s *clockState) caption() string {
	if s.label != "" {
		return s.label
	}
	return s.now.Format("MON 02 JAN")
}
func (s *clockState) setLabel(value string) error {
	if len(value) > 240 || !utf8.ValidString(value) {
		return fmt.Errorf("clock label must be valid UTF-8 and at most 240 bytes")
	}
	s.label = value
	return nil
}

type clockServices interface {
	RequestText(string, string, string, bool) error
	Notify(string, string, string, string, string) error
}

func registerClock(r *rui.Runtime, services clockServices, s *clockState) error {
	regs := []func() error{
		func() error { return r.BindString("clock.display", s.display) },
		func() error { return r.BindString("clock.caption", s.caption) },
		func() error { return r.BindBool("clock.use24Hour", func() bool { return s.use24 }) },
		func() error { return r.BindInt("clock.alarmMinute", func() int { return s.alarmMinute }) },
		func() error { return r.BindInt("clock.snoozeMinutes", func() int { return s.snoozeMinutes }) },
		func() error { return r.BindInt("clock.exampleDate", func() int { return s.exampleDate }) },
		func() error { return r.BindInt("clock.minDate", func() int { return s.baseDate - 365 }) },
		func() error { return r.BindInt("clock.maxDate", func() int { return s.baseDate + 365 }) },
		func() error {
			return r.OnBool("toggle_24hour", func(_ context.Context, v bool) error { s.use24 = v; return nil })
		},
		func() error {
			return r.OnInt("set_alarm_time", func(_ context.Context, v int) error { s.alarmMinute = v; return nil })
		},
		func() error {
			return r.OnInt("set_snooze", func(_ context.Context, v int) error { s.snoozeMinutes = v; return nil })
		},
		func() error {
			return r.OnInt("set_date", func(_ context.Context, v int) error { s.exampleDate = v; return nil })
		},
		func() error {
			return r.OnAction("edit_label", func(context.Context, rui.ActionEvent) error {
				return services.RequestText("clock-label", "Clock label", s.label, false)
			})
		},
		func() error {
			return r.OnAction("clock_help", func(context.Context, rui.ActionEvent) error { return r.ShowDialog("help") })
		},
		func() error {
			return r.OnAction("close_help", func(context.Context, rui.ActionEvent) error { return nil })
		},
	}
	for _, reg := range regs {
		if err := reg(); err != nil {
			return err
		}
	}
	for _, letter := range "abcdefghijkl" {
		action := "demo_" + string(letter)
		if err := r.OnAction(action, func(context.Context, rui.ActionEvent) error {
			return services.Notify("Gallery selection", strings.ToUpper(action), "actionable", "gallery", "open")
		}); err != nil {
			return err
		}
	}
	return r.ValidateBindings()
}

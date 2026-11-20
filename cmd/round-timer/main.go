package main

import (
	"log"
	"os"
	"path/filepath"
	"roundos/internal/timerstate"
	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/ui"
	"time"
)

func main() {
	client, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	stateDir := os.Getenv("ROUNDOS_STATE_DIR")
	if stateDir == "" {
		stateDir = os.TempDir()
	}
	timer := timerstate.Load(filepath.Join(stateDir, "timer.json"), time.Now())
	notifyFinished := func() {
		if err := client.Notify("Timer finished", "Countdown completed", "actionable", "timer-finished", "open"); err != nil {
			log.Printf("timer notify: %v", err)
		}
	}
	if timer.CompletedOnLoad {
		if err := timer.Save(); err != nil {
			log.Printf("timer state save: %v", err)
		}
		notifyFinished()
	}
	visible := false
	confirmReset := false
	var tick *time.Timer
	var tickCh <-chan time.Time
	var deadline *time.Timer
	var deadlineCh <-chan time.Time
	stopTimer := func(t **time.Timer) {
		if *t != nil {
			(*t).Stop()
			*t = nil
		}
	}
	publish := func() {
		now := time.Now()
		if timer.Resolve(now) {
			if err := timer.Save(); err != nil {
				log.Printf("timer save: %v", err)
			}
			notifyFinished()
		}
		action := "START"
		hint := "ROTATE TO ADJUST"
		if timer.Running {
			action = "PAUSE"
			hint = "RUNNING IN BACKGROUND"
		}
		minutes := int(timer.Left(now) / time.Minute)
		duration := ui.Slider("timer.minutes", "MINUTES", "set_minutes", minutes, 0, 999, 1)
		if timer.Running {
			duration.Disabled = true
		}
		picker := ui.NumberPicker("timer.picker", "SET MINUTES", "set_minutes", minutes, 0, 999, 1)
		if timer.Running {
			picker.Disabled = true
		}
		view := wire.View{Title: "TIMER", Primary: timer.Label(now), Hint: hint, ButtonLabel: action, ButtonAction: "toggle", RotaryAction: "adjust", Screens: []wire.Screen{
			ui.Screen("timer.root", "TIMER", timer.Label(now), hint,
				ui.PrimaryButton("timer.toggle", action, "toggle"),
				ui.Link("timer.duration.link", "DURATION", "timer.duration")),
			ui.Screen("timer.duration", "DURATION", "", "STOP TIMER TO EDIT",
				duration,
				picker,
				ui.Button("timer.reset", "RESET TIMER", "request_reset"),
				ui.Progress("timer.progress", "TIME REMAINING", minutes, 0, 999)),
		}}
		if confirmReset {
			view.Dialog = ui.ConfirmDialog("timer-reset", "RESET TIMER?", "THIS WILL DISCARD TIME", "confirm_reset", "cancel_reset", true)
		}
		if err := client.Publish(view); err != nil {
			log.Printf("timer publish: %v", err)
		}
	}
	schedule := func() {
		stopTimer(&tick)
		stopTimer(&deadline)
		tickCh = nil
		deadlineCh = nil
		if timer.Running {
			d := time.Until(timer.Deadline)
			if d <= 0 {
				d = time.Millisecond
			}
			deadline = time.NewTimer(d)
			deadlineCh = deadline.C
			if visible {
				left := timer.Left(time.Now())
				step := left % time.Second
				if step <= 0 {
					step = time.Second
				}
				// Wake just after a displayed-second boundary.
				tick = time.NewTimer(step + 3*time.Millisecond)
				tickCh = tick.C
			}
		}
	}
	schedule()
	// The first view acknowledges loaded Timer state even while backgrounded.
	publish()
	defer func() { stopTimer(&tick); stopTimer(&deadline) }()
	for {
		select {
		case m, ok := <-client.Events():
			if !ok {
				return
			}
			// This app has no gesture-specific controls. Do not cancel and
			// recreate timer deadlines on every incoming drag/pinch event.
			if m.Type == "gesture" {
				continue
			}
			switch m.Type {
			case "visibility":
				visible = m.Visible
				if visible {
					publish()
				}
			case "input":
				now := time.Now()
				changed := false
				switch m.Action {
				case "toggle":
					timer.Toggle(now)
					changed = true
				case "adjust":
					changed = timer.Rotate(m.Delta, now)
				case "set_minutes":
					if !timer.Running {
						changed = timer.Rotate(m.Delta-int(timer.Remaining/time.Minute), now)
					}
				case "request_reset":
					confirmReset = true
					changed = true
				case "cancel_reset":
					confirmReset = false
					changed = true
				case "confirm_reset":
					confirmReset = false
					if timer.Running {
						timer.Toggle(now)
					}
					changed = timer.Rotate(5-int(timer.Remaining/time.Minute), now) || true
				case "crash":
					os.Exit(42) // explicit diagnostics-only crash test
				}
				if changed {
					if err := timer.Save(); err != nil {
						log.Printf("timer save: %v", err)
					}
					if visible {
						publish()
					}
				}
			case "close":
				return
			}
			schedule()
		case <-tickCh:
			tickCh = nil
			if visible {
				publish()
			}
			schedule()
		case <-deadlineCh:
			if timer.Resolve(time.Now()) {
				if err := timer.Save(); err != nil {
					log.Printf("timer save: %v", err)
				}
				if visible {
					publish()
				}
				log.Print("timer finished")
				notifyFinished()
			}
			schedule()
		}
	}
}

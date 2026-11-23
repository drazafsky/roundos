package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"roundos/internal/timerstate"
	"roundos/layouts"
	"roundos/sdk/app"
	"roundos/sdk/rui"
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
	state := &timerModel{timer: timerstate.Load(filepath.Join(stateDir, "timer.json"), time.Now()), now: time.Now()}
	runtime, err := rui.New(timerPublisher{client}, layouts.Timer)
	if err != nil {
		log.Fatal(err)
	}
	if err := registerTimer(runtime, state); err != nil {
		log.Fatal(err)
	}
	notifyFinished := func() {
		if err := client.Notify("Timer finished", "Countdown completed", "actionable", "timer-finished", "open"); err != nil {
			log.Printf("timer notify: %v", err)
		}
	}
	save := func() {
		if err := state.timer.Save(); err != nil {
			log.Printf("timer save: %v", err)
		}
	}
	if state.timer.CompletedOnLoad {
		save()
		notifyFinished()
	}
	if err := runtime.PublishInitial(); err != nil {
		log.Fatal(err)
	}
	var updates <-chan *rui.DevUpdate
	if directory := os.Getenv("ROUNDOS_RUI_DEV_DIR"); directory != "" {
		session, err := rui.ListenDev(context.Background(), directory)
		if err != nil {
			log.Fatal(err)
		}
		defer session.Close()
		updates = session.Updates()
		log.Printf("RUI development session: %s", session.Path)
	}
	visible := false
	var tick, deadline *time.Timer
	var tickCh, deadlineCh <-chan time.Time
	stop := func(timer **time.Timer) {
		if *timer != nil {
			(*timer).Stop()
			*timer = nil
		}
	}
	defer func() { stop(&tick); stop(&deadline) }()
	resolve := func() {
		state.now = time.Now()
		if state.timer.Resolve(state.now) {
			save()
			notifyFinished()
			log.Print("timer finished")
		}
	}
	publish := func() {
		resolve()
		_ = runtime.Invalidate()
		if err := runtime.Flush(); err != nil {
			log.Printf("timer publish: %v", err)
		}
	}
	schedule := func() {
		stop(&tick)
		stop(&deadline)
		tickCh = nil
		deadlineCh = nil
		if state.timer.Running {
			delay := time.Until(state.timer.Deadline)
			if delay <= 0 {
				delay = time.Millisecond
			}
			deadline = time.NewTimer(delay)
			deadlineCh = deadline.C
			if visible {
				step := state.timer.Left(time.Now()) % time.Second
				if step <= 0 {
					step = time.Second
				}
				tick = time.NewTimer(step + 3*time.Millisecond)
				tickCh = tick.C
			}
		}
	}
	schedule()
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			resolve()
			if err := update.Apply(runtime); err != nil {
				log.Printf("RUI reload: %v", err)
			}
			publish()
		case event, ok := <-client.Events():
			if !ok || event.Type == "close" {
				return
			}
			if event.Type == "gesture" {
				continue
			}
			resolve()
			if event.Type == "visibility" {
				visible = event.Visible
			}
			handled, err := runtime.Dispatch(event)
			if err != nil {
				log.Printf("timer input: %v", err)
			}
			if !handled && event.Type == "input" {
				switch event.Action {
				case "adjust":
					if state.timer.Rotate(event.Delta, state.now) {
						save()
					}
				case "crash":
					os.Exit(42)
				}
			}
			publish()
		case <-tickCh:
			publish()
		case <-deadlineCh:
			publish()
		}
		schedule()
	}
}

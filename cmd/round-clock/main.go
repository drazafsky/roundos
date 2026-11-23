package main

import (
	"context"
	"log"
	"os"
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
	runtime, err := rui.New(client, layouts.Clock)
	if err != nil {
		log.Fatal(err)
	}
	state := newClockState(time.Now())
	if err := registerClock(runtime, client, state); err != nil {
		log.Fatal(err)
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
	var alarm *time.Timer
	var alarmCh <-chan time.Time
	stop := func() {
		if alarm != nil {
			alarm.Stop()
			alarm = nil
		}
		alarmCh = nil
	}
	defer stop()
	publish := func() {
		state.now = time.Now()
		_ = runtime.Invalidate()
		if err := runtime.Flush(); err != nil {
			log.Printf("clock publish: %v", err)
		}
	}
	schedule := func() {
		stop()
		if visible {
			delay := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute))
			if delay <= 0 {
				delay = time.Millisecond
			}
			alarm = time.NewTimer(delay)
			alarmCh = alarm.C
		}
	}
	for {
		select {
		case update, ok := <-updates:
			if !ok {
				updates = nil
				continue
			}
			state.now = time.Now()
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
			if event.Type == "visibility" {
				visible = event.Visible
			}
			if _, err := runtime.Dispatch(event); err != nil {
				log.Printf("clock input: %v", err)
			}
			if event.Type == "text_result" && event.TextResult != nil && event.TextResult.ID == "clock-label" && !event.TextResult.Canceled {
				if err := state.setLabel(event.TextResult.Value); err != nil {
					log.Printf("clock label: %v", err)
				}
			}
			publish()
		case <-alarmCh:
			publish()
		}
		schedule()
	}
}

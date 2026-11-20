package main

import (
	"log"
	"roundos/internal/wire"
	"roundos/sdk/app"
	"roundos/sdk/ui"
	"strings"
	"time"
)

func main() {
	client, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	var visible bool
	use24 := true
	alarmMinute := 8 * 60
	snoozeMinutes := 5
	baseDate := int(time.Now().UTC().Unix() / 86400)
	exampleDate := baseDate
	helpDialog := false
	label := ""
	var alarm *time.Timer
	var alarmCh <-chan time.Time
	stop := func() {
		if alarm != nil {
			alarm.Stop()
			alarm = nil
		}
		alarmCh = nil
	}
	publish := func() {
		now := time.Now()
		hint := now.Format("MON 02 JAN")
		if label != "" {
			hint = label
		}
		clockFormat := "15:04"
		if !use24 {
			clockFormat = "03:04 PM"
		}
		gallery := make([]wire.Widget, 0, 12)
		for i := 0; i < 12; i++ {
			gallery = append(gallery, ui.ListItem("gallery."+string(rune('a'+i)), "GALLERY ITEM "+string(rune('A'+i)), "SELECT TO NOTIFY", "demo_"+string(rune('a'+i))))
		}
		view := wire.View{Title: "CLOCK", Primary: now.Format(clockFormat), Hint: hint, ButtonLabel: "EDIT LABEL", ButtonAction: "edit_label", Screens: []wire.Screen{
			ui.Screen("clock.root", "CLOCK", now.Format(clockFormat), hint,
				ui.PrimaryButton("clock.label", "EDIT LABEL", "edit_label"),
				ui.Link("clock.options", "OPTIONS", "clock.options")),
			ui.Screen("clock.options", "CLOCK OPTIONS", "", "",
				ui.Toggle("clock.24hour", "24-HOUR CLOCK", "toggle_24hour", use24),
				ui.Link("clock.details", "PICKER EXAMPLES", "clock.details"),
				ui.Link("clock.gallery", "WIDGET GALLERY", "clock.gallery"),
				ui.ListItem("clock.help", "HELP", "HOW TO USE THE CLOCK", "clock_help"),
				ui.Link("clock.about", "ABOUT", "clock.about")),
			ui.Screen("clock.details", "DETAILS", "", "",
				ui.TimePicker("clock.alarm", "TIME EXAMPLE", "set_alarm_time", alarmMinute, 5),
				ui.NumberPicker("clock.snooze", "NUMBER EXAMPLE", "set_snooze", snoozeMinutes, 1, 30, 1),
				ui.DatePickerDays("clock.date", "DATE EXAMPLE", "set_date", exampleDate, baseDate-365, baseDate+365)),
			ui.Screen("clock.gallery", "WIDGET GALLERY", "", "SCROLL TO EXPLORE", gallery...),
			ui.Screen("clock.about", "ABOUT", "", "LOCAL CLOCK", ui.Label("clock.local", "LOCAL")),
		}}
		if helpDialog {
			view.Dialog = ui.ConfirmDialog("help", "CLOCK HELP", "TOUCH OR ROTATE", "close_help", "close_help", false)
		}
		err := client.Publish(view)
		if err != nil {
			log.Printf("clock publish: %v", err)
		}
	}
	defer stop()
	for {
		select {
		case m, ok := <-client.Events():
			if !ok {
				return
			}
			if m.Type == "gesture" {
				continue
			}
			if m.Type == "input" && m.Action == "set_alarm_time" {
				if m.Delta >= 0 && m.Delta <= 1439 {
					alarmMinute = m.Delta
					if visible {
						publish()
					}
				}
			}
			if m.Type == "input" && m.Action == "set_date" {
				if m.Delta >= baseDate-365 && m.Delta <= baseDate+365 {
					exampleDate = m.Delta
					if visible {
						publish()
					}
				}
			}
			if m.Type == "input" && m.Action == "clock_help" {
				helpDialog = true
				if visible {
					publish()
				}
			}
			if m.Type == "input" && m.Action == "close_help" {
				helpDialog = false
				if visible {
					publish()
				}
			}
			if m.Type == "input" && strings.HasPrefix(m.Action, "demo_") {
				if err := client.Notify("Gallery selection", strings.ToUpper(m.Action), "actionable", "gallery", "open"); err != nil {
					log.Printf("gallery notify: %v", err)
				}
			}
			if m.Type == "input" && m.Action == "set_snooze" {
				if m.Delta >= 1 && m.Delta <= 30 {
					snoozeMinutes = m.Delta
					if visible {
						publish()
					}
				}
			}
			if m.Type == "input" && m.Action == "toggle_24hour" {
				use24 = !use24
				if visible {
					publish()
				}
			}
			if m.Type == "input" && m.Action == "edit_label" {
				if err := client.RequestText("clock-label", "Clock label", label, false); err != nil {
					log.Printf("clock request text: %v", err)
				}
			}
			if m.Type == "text_result" && m.TextResult != nil && m.TextResult.ID == "clock-label" && !m.TextResult.Canceled {
				label = m.TextResult.Value
				if visible {
					publish()
				}
			}
			if m.Type == "visibility" {
				visible = m.Visible
				if visible {
					publish()
				} else {
					stop()
				}
			}
			if m.Type == "close" {
				return
			}
		case <-alarmCh:
			if visible {
				publish()
			}
		}
		if visible && alarm == nil {
			d := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute))
			if d <= 0 {
				d = time.Millisecond
			}
			alarm = time.NewTimer(d)
			alarmCh = alarm.C
		} else if visible && alarm != nil {
			// Reset on every wake: no permanent 1 Hz timer.
			alarm.Stop()
			d := time.Until(time.Now().Truncate(time.Minute).Add(time.Minute))
			if d <= 0 {
				d = time.Millisecond
			}
			alarm.Reset(d)
			alarmCh = alarm.C
		}
	}
}

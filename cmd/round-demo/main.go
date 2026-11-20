// RoundOS demo: locally reviewed optional app running as the roundos-app user.
// All third-party apps still share this UID and are not separate sandboxes.
package main

import (
	"log"
	"roundos/sdk/app"
	"roundos/sdk/ui"
)

func main() {
	client, err := app.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	count := 0
	visible := false
	publish := func() {
		v := ui.View{Title: "DEMO", Screens: []ui.ScreenSpec{
			ui.Screen("demo.root", "DEMO", "ROUNDOS", "OPTIONAL APP", ui.PrimaryButton("count", "COUNT UP", "increment"), ui.Link("about", "ABOUT", "demo.about")),
			ui.Screen("demo.about", "ABOUT DEMO", "", "SDK V7", ui.Label("info", "INSTALLED APP")),
		}}
		if count > 0 {
			v.Screens[0].Subtitle = "COUNTED"
		}
		if err := client.Publish(v); err != nil {
			log.Print("demo publish: ", err)
		}
	}
	for ev := range client.Events() {
		switch ev.Type {
		case "visibility":
			visible = ev.Visible
			if visible {
				publish()
			}
		case "input":
			if ev.Action == "increment" {
				count++
				if visible {
					publish()
				}
			}
		case "close":
			return
		}
	}
}

# RoundOS 0.5.0 Go SDK — IPC v7

The shell owns display composition, circular-safe layout, keyboard, dialog rendering, focus and navigation stacks. Applications are independent supervised processes that publish semantic screens and receive strongly typed events. Use `roundos/sdk/app` and `roundos/sdk/ui`; the `internal/wire` implementation is not a supported external import.

## Minimal external app

```go
package main

import (
    "log"
    "roundos/sdk/app"
    "roundos/sdk/ui"
)

func main() {
    client, err := app.Connect()
    if err != nil { log.Fatal(err) }
    defer client.Close()
    n := 0
    publish := func() {
        view := ui.View{Title: "EXAMPLE", Screens: []ui.ScreenSpec{
            ui.Screen("root", "EXAMPLE", "READY", "ROUNDOS SDK",
                ui.PrimaryButton("increment", "ADD", "add")),
        }}
        if err := client.Publish(view); err != nil { log.Print(err) }
    }
    for event := range client.Events() {
        switch event.Type {
        case "visibility": if event.Visible { publish() }
        case "input": if event.Action == "add" { n++; publish() }
        case "close": return
        }
    }
}
```

RoundOS supplies the IPC socket, app ID and token through a restricted process environment. Apps must not hardcode a socket or embed the token in source. Never log environment variables.

## Build outside the source tree

An independent Go module can use the SDK public aliases, provided it has a trusted local checkout of the same `roundos` module version. Example `go.mod`:

```go
module example.org/pocketwatch-demo

go 1.23

require roundos v0.0.0
replace roundos => /home/pi/roundos-0.5.0
```

Build on the Pi with `GOOS=linux GOARCH=arm GOARM=7`, or simply `go build` on the native 32-bit Pi OS. API/IPC version is 7; app binaries should use the matching SDK. External module builds were checked at source level, but no public package registry, semantic-versioned module release or independent app sandbox is offered.

## Declarative layout

- `ui.Screen(id, title, hero, subtitle, widgets...)` defines a full-screen view. Root screen comes first.
- `ui.Button`, `ui.PrimaryButton`, `ui.Link`, `ui.Toggle`, `ui.Slider`, `ui.ListItem`, `ui.Label`, `ui.Progress`, `ui.NumberPicker`, `ui.TimePicker`, `ui.DatePickerDays`, `ui.ConfirmDialog` expose shell-managed controls.
- Stable per-screen widget IDs preserve focus through updates; `ui.Link` targets other screen IDs owned by the same view.
- Time picker value uses minutes since midnight; date picker value uses UTC days since Unix epoch; sliders and numeric pickers report committed absolute values through `event.Delta`. Ordinary relative rotary actions may use relative deltas.
- The shell applies system theme/semantic colors, touch geometry and Reduce Motion; applications do not select touch pixel rectangles or custom raw colors.
- `Client.Publish` validates widget trees and transmits full views when screen structure changes, or scalar patches when only small scalar fields change. Do not mutate a view concurrently with publishing.

## Events and services

`client.Events()` delivers `visibility`, `input`, `gesture`, `contacts`, `text_result` and `close`. `Client.RequestText(id,label,initial,secure)` opens the shell editor for a foreground app; on a secure edit, avoid retaining raw passwords or printing drafts. `Client.Notify(title,body,priority,group,action)` requests a notification. `Client.SubscribeContacts(true)` opts into evdev contact snapshots; the HyperPixel 2.1 Round physically supports at most two simultaneous contacts under the verified profile.

## Installation/permissions

Install only reviewed source and binaries. Add a root-owned manifest under `/etc/roundos/apps`, with a root-owned executable inside that directory, both free of group/world write permissions. Example manifest:

```json
{"id":"org.example.demo","name":"Demo","icon":"DME","exec":"my-demo"}
```

App IDs must begin `org.`; icon has 1–3 uppercase alphanumeric characters. Maximum 12 optional apps. Restart the service to rescan. Apps are launched as `roundos-app` and **share one UID**: this release is not an untrusted-app platform. The demo implementation is under `cmd/round-demo`; installing it remains explicitly opt-in with `scripts/install-demo.sh`.

App-specific persistent state should use the supplied `ROUNDOS_STATE_DIR`, not root's home or the read-only release tree, and must not alter other apps' data. Long CPU/network tasks belong in app goroutines rather than the shell rendering path.

## Lifecycle

The supervisor restarts crashed apps with bounded retries; it sends `visibility` changes rather than killing background apps when the carousel changes. Persist state independently when required (see Timer implementation). Apps should close resources on cancellation or receipt of `close`. Changes to trusted manifests require a service restart; the shell's built-in Clock and Timer names are reserved.

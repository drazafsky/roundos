# 0.5-dev5.1.2: two-finger navigation regression fix

## Observed defect

On a type-B multitouch input device, the two active contacts can be updated on
separate `SYN_REPORT` frames. The prior recognizer permanently claimed a
pinch/rotation if a **single intermediate frame** changed spacing by 13% or
orientation by 15 degrees. Even a coherent two-finger swipe could therefore be
excluded from the `two_swipe` event path before the second contact caught up.

The reproduction was added to `internal/gesture/staggered_test.go` and confirmed
failing against 0.5-dev5.1.1. The updated recognizer now requires both tracked
fingers to translate coherently for a system swipe, and requires coherent
relative movement or sustained evidence to claim a pinch/rotation. It retains
release-time swipe fallback and cancel-on-remaining-finger behavior.

The shell routing regression in
`internal/systemgestures/hardware_swipes_test.go` verifies actual staged touch
frames: left selects Timer from Clock, right selects Clock from Timer, and up
returns Home. Pinch and rotation unit tests remain in place.

## Build on Raspberry Pi

From SSH, preserve the previous release and run:

```sh
unzip roundos-0.5-dev5.1.2.zip
cd roundos-0.5-dev5.1.2
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json -gesture-log
```

The launcher temporarily stops LightDM and restores it when you exit with `q`.
No boot, driver, kernel-overlay, or Plymouth modifications are needed.

1. Press `c` to display Clock. Place two fingers near the center and move them
   together to the left about 80-100 display pixels. Expect Timer.
2. Repeat to the right. Expect Clock.
3. Swipe upward with two fingers. Expect Home.
4. Press `g` for contact diagnostics. **System two-finger navigation is suppressed
   on this screen by design**; it is only for observing contacts/events.
5. Confirm pinch/rotation do not navigate, and single-finger edge gestures work.
6. Try a rapid sequence of swipes and compare response to the 0.4.1 baseline.

### If the swipe is still not recognized

Restart from SSH with diagnostic flags:

```sh
./scripts/test-drm.sh \
  -gesture-config config/gestures.example.json \
  -gesture-log -gesture-log-moves -touch-trace
```

The diagnostic-only `-touch-trace` option prints active IDs and coordinates at
most about every 75 ms (plus changes in contact count). Look for:

- `touch trace: contacts=2 positions=...` while both fingers move;
- `gesture two_swipe ... direction=left|right|up`;
- `system gesture routed: next_app|previous_app|launcher`.

If raw contacts show no synchronized movement, troubleshoot the touch input
hardware/driver. If `two_swipe` prints but no routed action appears, verify
`two_finger_switch` and `two_finger_launcher` are enabled in the passed JSON.
If the routed action prints but the screen does not change, inspect shell state
and foreground selection. Disable tracing when measuring input latency.

## Verification performed in development environment

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build ./...` (no DRM CGO)

ARMv7 **DRM-enabled build and physical gesture verification remain pending**.

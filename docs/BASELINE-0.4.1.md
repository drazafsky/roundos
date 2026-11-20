# RoundOS 0.4.1 — Touch latency and gesture accuracy hotfix

This is a **drop-in source replacement for 0.4**, based on the hardware-verified 0.3 rendering/IPC/timer baseline. It addresses a confirmed software latency path in 0.4. **The improvement has been tested in Go simulation and unit/integration tests, but has NOT been measured on the real Pi yet.**

## What's fixed in 0.4.1

- **DRM no longer stalls touch handling.** The shell no longer waits for the synchronous DRM page-flip event before processing the next touch frame. A dedicated rendering goroutine owns the framebuffer and DRM backend; input handling queues immutable model snapshots in a latest-wins, one-slot mailbox. Only outdated *images* are coalesced; all touch events still enter the gesture detector in order.
- **Swipe recognized before release.** When both fingers move together at least 75 pixels in a clearly dominant direction, and no pinch/rotation has claimed the gesture, the system navigation event fires while the fingers are still down. A 58-pixel end-of-gesture fallback is also supported. Navigation fires at most once per gesture.
- **Less accidental gesture recognition.** Single-finger jitter below 20 pixels remains a tap candidate. Pinch requires a substantial relative scale change (13%) and rotation requires at least 15 degrees; gesture updates are rate-limited by displacement thresholds. Two-finger pinches and rotations are not misclassified as system swipes once claimed.
- **No ghost taps after multi-touch.** A remaining finger is suppressed until lifted; entering multi-touch cancels an existing one-finger gesture.
- **Reduced SSH logging overhead.** `-gesture-log` reports completed/meaningful events only. New `-gesture-log-moves` opts into high-volume raw gesture-movement diagnostics; leave it off for speed testing.
- **Built-in Clock/Timer skip unused gesture messages** rather than rescheduling their timers on every gesture packet.
- **Performance diagnostics:** `s` now also reports `coalesced-visual-frames`. Coalescing visual frames during fast movement is expected; touch input events are not discarded.

## Install / hardware test on Raspberry Pi Zero 2 W

**Keep RoundOS 0.3 and 0.4 installed as fallbacks.** Use a fresh directory and test over SSH. Do not disable LightDM at boot.

```sh
# Copy roundos-0.4.1.zip to your Pi first, then:
sudo apt update
sudo apt install -y build-essential pkg-config libdrm-dev unzip evtest
unzip roundos-0.4.1.zip
cd roundos-0.4.1

go test ./...
./scripts/build.sh
./scripts/test-drm.sh
```

The test script stops LightDM temporarily, opens `/dev/dri/card0` / `DPI-1`, reads `/dev/input/event0`, and attempts to restore the desktop when `roundd` exits. **If it does not restore the desktop, run `sudo systemctl start lightdm.service` over SSH.**

### Performance/recognition tests

1. Enter `g` in the RoundOS SSH session to open the Touch Test view. Move a single finger smoothly across the display. The marker should follow without a long backlog. Slight stationary jitter should not be logged as DRAG or SWIPE.
2. Tap an app button on Home. It should respond as reliably as in 0.3; confirm Timer still starts and stops.
3. On the diagnostic view, press one finger, move it over 50 pixels and release: expect `SWIPE`. Hold for 550 ms: expect `LONG PRESS` (do not hold a finger still when measuring tap responsiveness).
4. Check whether the diagnostic screen reports `FINGERS 2` for two independent contacts. **If it only reports 1, the physical driver/panel is not providing two contacts to RoundOS; two-finger gestures cannot work until the hardware input path does.**
5. With `FINGERS 2` confirmed, move both fingers together horizontally across Clock/Timer. A decisive two-finger swipe should navigate **before** the fingers lift. For pinch/rotation, use the diagnostic view; thresholds deliberately reject tiny motions.
6. Enter `s` to print frame statistics and the `coalesced-visual-frames` count. Compare p95 input-to-present and frame time to your 0.4 test under similar movement. Note: the latency statistic begins when Go reads the evdev frame, not at the physical touch IRQ.
7. Repeat the background Timer and `x` crash-recovery test to verify regression safety. Enter `q` to quit and restore the desktop.

For verbose gesture logging use `./scripts/test-drm.sh -gesture-log`; avoid `-gesture-log-moves` during performance comparisons. High-volume SSH/journal logging can itself increase latency.

### If touch is still delayed

With RoundOS stopped, examine **raw kernel touch delivery**:

```sh
sudo evtest /dev/input/event0
```

Move one and then two fingers. Look for `ABS_MT_TRACKING_ID`, `ABS_MT_POSITION_X/Y`, `SYN_REPORT`, and `SYN_DROPPED`. If the raw events themselves appear late, irregular, or only one active contact is reported, this points to the FT5x06/shared-GPIO bus path rather than RoundOS gesture recognition. Exit `evtest` with Ctrl+C. Record a short output excerpt and the output of `s` from the running shell for further diagnosis.

### Rollback

If 0.4.1 has a problem, press `q` to exit; return to the working 0.3 directory and use its existing `./scripts/test-drm.sh`. The patch does not change the kernel, display overlay, default graphical session, installed apps, or persistent timer data format.

---

# Original RoundOS 0.4 capabilities (included in 0.4.1)

RoundOS 0.4 extends the **hardware-verified RoundOS 0.3** shell for the Raspberry Pi Zero 2 W, Raspberry Pi OS Trixie (32-bit ARMv7), the HyperPixel 2.1 Round 480×480 DRM display, and its FT5x06 touchscreen. It preserves the existing Clock, Timer, independent Go application processes, background countdown, crash restart, incremental IPC updates, DRM output, and opt-in backlight management.

## New in 0.4

- **Full type-B multitouch contact tracking.** Tracks up to 16 evdev slots, preserving each contact's Linux tracking ID, coordinates, and slot through `SYN_REPORT` frames. Only changed frames wake the shell. A `SYN_DROPPED` cancels the current gesture rather than generating a ghost tap. Hardware support for two or more simultaneous contacts remains to be verified.
- **Event-driven gesture detection**: tap, double tap (additional event), long touch, drag start/move/end, directional swipe, two-finger swipe, pinch-scale, two-finger rotation, and input cancellation. A timer is armed only while a long press is pending. All recognition is inside `roundd`.
- **System two-finger swipes**: horizontal left → Timer; horizontal right → Clock; downward → Home. The gesture must be clearly translational, not primarily a pinch or rotation. The system captures these navigation gestures. Single-finger swipes are sent to apps and do not automatically switch apps.
- **Touch diagnostic screen**: tap `TOUCH TEST` on Home, or enter `g` + Enter via SSH. See the current number of contacts, tracking IDs, crosshairs, and last recognized gesture. Exit via `HOME` or `h` + Enter.
- **Public app SDK gesture events**: `sdk/app.Client.Events()` now exposes a public `app.Event` with an optional `*sdk/gesture.Event`. The IPC version increments to **v4**; build all three executables from this ZIP together. Wire transport is still bounded, length-prefixed JSON (not yet Protobuf).
- **Optional physical rotary / push button**: `-encoder /dev/input/eventN` and optional `-button /dev/input/eventM`. Supports `EV_REL REL_DIAL` (7), `REL_WHEEL` (8), `EV_KEY KEY_UP/KEY_DOWN`, and a configurable push-button key code (default `KEY_ENTER` = 28). Short press activates the selected application button; long press (700ms) returns Home. Simulated SSH `l` / `r` / `p` remain available.
- **Preserved power-oriented behavior**: no gesture polling loop, no periodic rendering in unchanged views, and existing optional backlight idle-off settings remain disabled by default.

**Not yet included:** generalized circular widget SDK, touch-pressure or palm rejection, Type-A multitouch devices, complete lost-event re-synchronization using `EVIOCGMTSLOTS`, hardware encoder GPIO wiring/overlay, application sandboxes, persistent app manifests, and a verified pinout. Touch gesture recognition is present, but Clock and Timer do not implement zooming or rotating their content. Full feature availability depends on the FT5x06 device reporting multiple simultaneous contacts.

## Install — retain the working desktop and previous 0.3 source

From a separate SSH session on the Pi:

```sh
sudo apt update
sudo apt install -y build-essential pkg-config libdrm-dev unzip evtest
unzip roundos-0.4.zip
cd roundos-0.4.1

go version
go test ./...
./scripts/build.sh
```

Build output: `roundd`, `round-clock`, `round-timer` (built together using IPC v4). Run the new binaries from the 0.4 directory, not from the 0.3 directory.

Safe preview (does **not** stop the desktop or open DRM):

```sh
./roundd -backend png -output /tmp/roundos-home.png
./roundd -backend png -preview-page touch -output /tmp/roundos-touch.png
```

Start on the actual HyperPixel from SSH:

```sh
./scripts/test-drm.sh -gesture-log
```

The script temporarily stops LightDM and automatically attempts to restore it when RoundOS exits normally. It uses DRM card `/dev/dri/card0`, `DPI-1`, and touch device `/dev/input/event0` by default, matching the hardware configuration already verified in RoundOS 0.3. It does **not** change the boot configuration. The `-gesture-log` option logs detected gestures and is useful while validating the panel; omit it for normal low-overhead use.

### Interactive SSH controls

| Command | Action |
|---|---|
| `h` | Home |
| `c` | Clock |
| `t` | Timer |
| `g` | Touch diagnostic screen |
| `l` / `r` | Simulated rotary left / right |
| `p` | Simulated short press: Home launches selected app; Timer activates Start/Pause |
| `x` | Crash Timer intentionally to test auto-recovery |
| `s` | Print frame timing, input-to-present and process RSS |
| `q` | Exit and restore LightDM |

### Physical touchscreen test sequence

1. Open the `TOUCH TEST` view by tapping it on Home or entering `g` on SSH.
2. Touch with **one finger**. Verify `FINGERS 1` and a marker at the actual contact. Release; the marker should disappear and `TAP` should appear.
3. Drag one finger, then release. Expect `DRAG`, followed by `DRAG END` and a `SWIPE` if the movement is long and fast enough.
4. Press and hold one finger for at least **650 ms**. Expect `LONG PRESS` without a tap on release.
5. Try two fingers simultaneously, well within the visible circle. **If the driver and panel support it**, expect `FINGERS 2` and two markers with distinct IDs. If it always says `FINGERS 1`, do not assume pinch/two-finger functionality works on this panel.
6. With two contacts detected, move them apart/together to produce `PINCH`; turn the line joining them to produce `FINGER ROTATE`; move both in parallel to produce `TWO SWIPE` when released.
7. Exit diagnostic via `HOME`. On Clock, swipe **two fingers to the left** to switch to Timer, and two fingers to the right from Timer to switch to Clock. A downward two-finger swipe goes Home.
8. Verify single-finger taps still start/pause the Timer. Start the countdown, switch to Clock, return and ensure the Timer continued. Enter `x` to repeat the 0.2 crash recovery test.

**Gesture design note:** A first tap is delivered immediately for low latency. A second nearby tap within 320ms generates both an ordinary tap and an additional double-tap event. The double-tap event is therefore informational in this prototype; a future widget API can opt into delaying single-tap delivery when exclusive double-tap actions are required.

### Optional physical rotary/button devices

No rotary input device was present in the original `/proc/bus/input/devices` diagnostics. **Do not guess GPIO assignments**, because the HyperPixel occupies many of the Raspberry Pi's pins. Once the encoder/button has a valid kernel evdev device, identify it with:

```sh
cat /proc/bus/input/devices
sudo evtest
```

An encoder configured to report `REL_DIAL`, with its button in the same event device:

```sh
./scripts/test-drm.sh -encoder /dev/input/eventN -gesture-log
```

An encoder reporting `REL_WHEEL` and a separate `gpio-keys` push-button device:

```sh
./scripts/test-drm.sh \
  -encoder /dev/input/eventN -encoder-rel 8 \
  -button /dev/input/eventM -encoder-press-key 28
```

Replace `eventN` and `eventM` with **verified** device paths. The key code depends on the GPIO key's actual device-tree configuration. Use `-encoder-reverse` if direction is opposite to your preference. `-encoder` and `-button` both default to `off`, so they do not open the existing HDMI input devices or require hardware additions.

### Example SDK gesture handler (for future apps)

```go
for e := range client.Events() {
    switch e.Type {
    case "visibility":
        visible = e.Visible
    case "gesture":
        if e.Gesture != nil {
            switch e.Gesture.Type {
            case "swipe":
                log.Printf("swiped %s", e.Gesture.Direction)
            case "pinch":
                log.Printf("pinch scale %.2f", e.Gesture.Scale)
            case "finger_rotate":
                log.Printf("rotation %.1f degrees", e.Gesture.Angle)
            }
        }
    }
}
```

System-owned two-finger swipe navigation is intercepted by RoundOS and does not activate a background application.

## Validation and recovery

On a development PC, test and cross-compile the pure Go components:

```sh
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build ./...
```

Hardware-specific libdrm compilation requires the Pi's `libdrm-dev` and `CGO_ENABLED=1` via `./scripts/build.sh`; tests in a non-Pi environment **do not establish hardware gesture support**. Keep the previous working RoundOS 0.3 ZIP and source directory intact.

To exit and restore the desktop, enter `q` in the running SSH session. If LightDM does not restore:

```sh
sudo systemctl start lightdm.service
```

No systemd startup service is installed and LightDM remains the default desktop in 0.4.

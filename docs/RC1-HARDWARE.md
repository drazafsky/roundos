# RoundOS 0.5-rc1 hardware acceptance and known limits

## Baseline

This build extends the hardware-tested 0.5-dev7 milestone. Native Pi DRM build and physical touch validation are **pending**. Host checks passed: `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, Linux ARMv7 `CGO_ENABLED=0` builds. The latter does not exercise the `drm` build tag or libdrm device access.

## Start

From SSH, with the test ZIP extracted to a fresh directory:

```sh
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

`./scripts/build.sh` compiles all three v7 executables. If compilation lacks DRM headers, on Pi install `libdrm-dev` and retry; do not modify the working kernel overlay.

`q` exits and attempts to restore the desktop. If needed run `sudo systemctl start lightdm.service`.

## Input controls

`h` Home, `c` Clock, `t` Timer, `k` switcher, `n` Notifications, `g` touch diagnostic, `z` Accessibility, `e` editor, `b` Back; `l`/`r` rotate and `p` press; `s` current frame/RSS statistics; `x` deliberately crashes Timer; `q` exit. All commands are typed on the RoundOS SSH console then Enter.

## Test checklist

- [ ] **Actual app widget trees:** Clock and Timer show circular widget buttons rather than the old legacy form; Clock `OPTIONS` opens nested widgets; Timer `DURATION` opens slider and picker.
- [ ] **Launcher:** `h`, `l/r`, `p` navigate focused carousel. Switching apps does not restart Timer.
- [ ] **Clock toggle:** Clock `OPTIONS` -> `24-HOUR CLOCK` changes the displayed time format. Subscreen Back restores focus.
- [ ] **Clock gallery:** Clock `OPTIONS` -> `WIDGET GALLERY`; rotate through 12 list items without wraparound; focus stays visible. Touch-drag the list slowly: content follows the finger and stops on release.
- [ ] **Kinetic scrolling:** Flick that gallery rapidly: scroll continues briefly and stops within ~220ms; another finger down stops inertia immediately. No text or button should spill outside the circular safe area. `z` -> Reduce Motion ON: subsequent flings should stop on release without inertia.
- [ ] **Picker:** Clock `OPTIONS` -> `PICKER EXAMPLES` -> `TIME EXAMPLE`, `NUMBER EXAMPLE`, `DATE EXAMPLE`. Rotate to change selected value, press to commit, and verify the row reflects it; `b`/visible Back cancels an uncommitted picker. These do **not** program an actual alarm or change the system clock.
- [ ] **Timer slider:** Timer `DURATION` -> `MINUTES`. Press, rotate and press commits the selected duration. Back while editing restores previous value. Touch slider drag commits on finger release, not on every touch frame.
- [ ] **Timer dialog:** Timer `DURATION` -> `RESET TIMER` opens confirmation. Initial focus must be CANCEL. Rotate to CONFIRM; press clears it and resets to 5 minutes. Back or CANCEL leaves duration unchanged. Tapping outside does not confirm a destructive action.
- [ ] **Themes:** `z` -> Dark, Light, High Contrast; change at runtime and inspect every standard widget without restarting Clock/Timer. Large Targets and Reduced Motion must remain effective. Preferences persist across restart.
- [ ] **Text editor:** Clock `EDIT LABEL` -> QWERTY/rotary, Back once hides keyboard while keeping draft; second Back offers Keep/Discard. Validate secure field clearing if used by another app.
- [ ] **Gestures:** One-finger top-edge opens Notification Center; one-finger left-edge is Back; two-finger staggered left/right switches apps and staggered up opens Home. The HyperPixel's tested physical contact limit is **two**, despite Linux advertising five slots. Keep the unmirrored touch-Y setting; DO NOT invert global Y.
- [ ] **Background Timer:** Start Timer, switch to Clock, return after 10 seconds; count remains correct. When Timer expires it generates an actionable notification. On `x`, supervisor restarts Timer without losing countdown state.
- [ ] **Responsiveness:** With `-gesture-log` disabled, rapidly touch/rotate while the Timer runs, and observe no lag, stuck controls or queued animation frames.

## Measure on the Pi

With RoundOS running, type `s` to log: `frame-p95`, `input-to-present-p95`, coalesced visual frames, and RSS for roundd and each app. Repeat during a fast gallery flick and during 60 seconds of idle UI. Record the numbers from the Pi: host preview speed is not a substitute for actual input-to-present measurements. Existing `perf.Stats` measures event-to-present, not touch IRQ-to-light, so describe its limitations accurately. Optionally inspect `top -p $(pgrep -d, -f "roundd|round-clock|round-timer")` from a second SSH window to estimate process CPU usage.

## Rollback

Keep the known-good `~/roundos-0.5-dev7` directory; quit rc1 and run the dev7 script from SSH to restore the previous build. No boot, Plymouth, kernel module or LightDM service file is modified permanently.

## Known limits

- Native DRM/ARMv7 and power behavior need device testing.
- Pixel scroll is bounded and short-lived, but it needs physical validation for edge gestures and slow drags.
- Picker Back is touch/SSH accessible. A dedicated rotary-only picker Cancel focus is not finalized; a long hardware press still opens the switcher and leaves a noncommitted picker.
- Clock's picker examples and gallery are UI SDK demonstrations; they do not schedule alarms or calendar events.
- Installed apps are still fixed Clock and Timer, not an arbitrary app registry or sandbox.
- Full Qt/X11/Wayland third-party apps are not supported.

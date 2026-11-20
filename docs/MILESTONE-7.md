# RoundOS 0.5-dev7 — Integration checkpoint

**Status:** host-tested source candidate; Raspberry Pi Zero 2 W / HyperPixel 2.1 Round physical DRM validation pending.

This integrates the missing focused carousel, declarative circular widgets and application navigation stack into the hardware-tested 0.5-dev6 lineage. It is not yet the final 0.5 release.

## What's new

- Focused Home **carousel**: one centered Clock/Timer card, neighboring cards are partially visible. Rotate to choose, press or tap center to launch. Touch the side card to change selection. Touch Test remains accessible at the bottom.
- Declarative app view schema (`wire.View.Screens`) and SDK constructors (`sdk/ui`). Applications publish stable screen and widget IDs, labels, values, actions and navigation targets **without pixel positions or colors**. IPC protocol is now **version 6**: build all three executables from this package.
- Shell-drawn circular widgets: button (including themed primary), toggle, slider, label and progress. Renderer and hit testing share geometry; rows stay within the circular safe radius.
- Per-application full-screen navigation stacks, visible Back on nested screens, and rotary-focusable Back (rotate left while the first widget is selected). Navigate via short press or touch. Back returns to parent and restores focused widget by stable ID even after live app view updates. Root Back returns to Home.
- Standard widgets use the active **system semantic theme tokens**. Dark, Light and High Contrast restyle the live UI without rebuilding apps. App-supplied raw styles are rejected by the `wire.ValidateView` contract.
- Clock uses the new SDK on its root/Options/About screens, with 24-hour clock toggle and the existing text editor request. Timer uses the new SDK on root/Duration screens, with themed Start/Pause button and a duration slider. Background countdown, notification, crash recovery and text editing are unchanged.
- Touch and rotary slider selection: press to enter edit, rotate to adjust, press to commit. Back cancels an uncommitted rotary edit. Touch tap/drag updates the slider and dispatches an absolute value on completion. `wire.Message.Delta` for `set_minutes` is the absolute target; legacy `adjust` remains a relative step count.
- Compact running-app switcher shows foreground/background/offline status and notification count; existing shell menu entries remain.
- Existing device-auto-discovery (`-touch auto`), **unmirrored Y** touch mapping, optional two-contact gestures, notifications, editor, backlight controls and asynchronous DRM presenter are preserved.

## Quick start on the Pi (SSH only)

```sh
unzip roundos-0.5-dev7.zip
cd roundos-0.5-dev7
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

`./scripts/test-drm.sh` temporarily stops LightDM, owns the DRM display, and attempts to restore LightDM on exit. Never launch this from the Pi desktop session. Rollback: exit with `q`; if needed run `sudo systemctl start lightdm.service`. The package never changes the working HyperPixel driver, kernel overlay, Plymouth splash or boot configuration. Keep your working dev6 directory.

**SSH controls**: `h` Home, `c` Clock, `t` Timer, `k` switcher, `n` notifications, `g` touch diagnostics, `z` accessibility, `e` standalone editor, `b` Back, `l/r/p` rotary left/right/press, `s` performance stats, `q` quit. `nq/na/nc` simulate quiet/actionable/critical notifications. `x` deliberately crashes Timer for recovery testing.

## New Milestone 7 hardware checks

1. Home carousel: `h`, `r` selects Timer, `l` selects Clock. `p` opens currently focused app. Tap side card to focus without launching; tap centered card to launch.
2. Clock: on Clock root, select **OPTIONS** (rotate once from EDIT LABEL), then press. Rotate through 24-HOUR CLOCK / EDIT LABEL / ABOUT. Activate the toggle; Clock text changes to 12-hour format when disabled. Tap the visible Back control or `b` to return. Back restores the OPTIONS selection on Clock root. Test rotary-only Back: from 24-HOUR CLOCK, rotate left once to focus Back and press.
3. Clock editor regression: tap EDIT LABEL, enter text by QWERTY and rotary wheel. First Back hides keyboard without discarding draft, second Back prompts Keep/Discard; Save returns to Clock.
4. Timer: launch Timer, keep it stopped, choose DURATION via rotary and press. On the Duration screen, press on MINUTES slider to edit, rotate left/right and press to commit; timer minutes update. Back cancels uncommitted slider changes; RETURN keeps scroll/focus. Start Timer and confirm countdown continues in background while Clock is foregrounded.
5. Switcher: long rotary button hold or `k`, select Clock/Timer and return. Verify foreground/background indicators and previously selected nested screens survive app switches.
6. Appearance: `z`, switch Dark → Light → High Contrast, test Clock, Timer, shell, Back and focus outlines; navigate back and verify state is preserved.
7. Gesture regression: single taps match physical positions; top-edge 1-finger swipe opens notifications, left-edge swipe is Back, two-finger staggered left/right switches apps and staggered up opens Home (per previous verified HyperPixel finger arrangement). Touch diagnostics reports at most 2 simultaneous contacts on this hardware.
8. Timer completion produces an actionable notification without interrupting Clock; critical alerts require explicit acknowledgment. Re-run crash/restart and desktop restoration tests.
9. Run `s` for system metrics and repeat rapid rotary presses/gesture transitions to assess regressions. Report hardware-observed input response and framebuffer performance; host tests do not establish physical 50-ms p95 targets.

## Preview UI without stopping LightDM

```sh
./roundd -backend png -preview-page home -output /tmp/roundos-home.png
./roundd -backend png -preview-page clock -output /tmp/roundos-clock.png
./roundd -backend png -preview-page clock-options -output /tmp/roundos-options.png
./roundd -backend png -preview-page timer-duration -output /tmp/roundos-duration.png
```

## Compatibility and known limits

- `roundd`, `round-clock` and `round-timer` must use the same v6 IPC protocol; do not run an old v5 executable with dev7 `roundd`.
- Current SDK widgets are button, toggle, slider, label and progress. App-defined generic dialogs, date/time pickers, and the final touch keyboard as an ordinary widget are not yet unified with the new declarative SDK. The Milestone 6 full-screen editor and shell-owned modal dialogs remain operational.
- Long app lists support focus-based rotary navigation and touch row scrolling. The final continuous pixel-level short-inertial-glide animation is **not completed in this checkpoint**; this must be addressed before a full 0.5 release claim.
- The development build manages two applications (Clock and Timer), not arbitrary installed apps. A generic application registry and app-sandboxing remain separate implementation work.
- App UI colors use semantic tokens. App-defined custom drawing must migrate to the theme contract; this package covers the built-in Clock, Timer and shell-provided widgets.
- This host cannot build the actual Pi DRM/libdrm executable; it requires Raspberry Pi toolchain and `libdrm-dev`. ARMv7 non-DRM crossbuild and host tests are not substitutes for Pi DRM testing.

## Regression commands

```sh
go test ./...
go test -race ./...
go vet ./...
```

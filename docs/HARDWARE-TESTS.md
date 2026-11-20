# RoundOS 0.5-dev5 — Hardware acceptance checklist

Run from SSH with the original 0.4.1 directory preserved. Do not modify boot configuration. Use the same physical panel orientation and compare against the 0.4.1 results.

## Baseline

- [ ] `go test ./...` passes on Pi.
- [ ] `./scripts/build.sh` compiles matching IPC-v5 executables.
- [ ] PNG previews render, without stopping LightDM.
- [ ] `./scripts/test-drm.sh -gesture-config config/gestures.example.json` opens DRM display and touchscreen.
- [ ] `q` exits and restores LightDM.

## Gesture recognition on the HyperPixel two-contact profile

- [ ] Run `sudo evtest /dev/input/eventN (use the touchscreen event reported at startup)`; confirm that the current driver path reports two simultaneous tracking IDs, even with a third finger.
- [ ] Use `-gesture-config config/gestures.example.json` (this enables the opt-in two-finger shortcuts and declares `max_contacts: 2`).
- [ ] One finger from visible left edge moving inward returns Back.
- [ ] One finger from top edge moving downward opens Notification Center.
- [ ] Ordinary one-finger taps/drags away from edges never open shell views.
- [ ] Two fingers moving together left or right switch the running foreground application **once**.
- [ ] Two fingers moving together upward open Home/launcher **once**.
- [ ] Pinch and rotation do not trigger navigation, even when two-finger shortcuts are enabled.
- [ ] In a configuration with the two-finger shortcuts disabled, two-finger swipes are passed to the application.
- [ ] Touch diagnostics (`g`) shows `TOUCH 2 OF 2` with separate tracking IDs.
- [ ] A third finger does not cause false taps or phantom app switches.
- [ ] Three-/four-/five-finger shortcuts remain inactive on a profile reporting only two contacts.
- [ ] Existing touch marker and UI responsiveness match the 0.4.1 baseline.

For other hardware, `max_contacts` may be set higher **only after** raw Linux events verify the device reports additional independent contacts. The generic decoder still supports those contacts.

## Notifications

- [ ] `nq` creates history but no banner.
- [ ] `na` produces a nonmodal banner; it expires after about four seconds.
- [ ] `n` opens the history list; rotary selects; press opens details; Dismiss and Back work.
- [ ] `k` opens a switcher; selecting Notifications opens the Notification Center; Back unwinds both overlays.
- [ ] `nc` displays a persistent critical dialog; ordinary taps outside Acknowledge do not dismiss it.
- [ ] Press or touch the explicit acknowledgment control to close the critical dialog.
- [ ] Start Timer, switch to Clock, wait for actual expiry, verify Timer notification appears without leaving Clock.
- [ ] `x` crash recovery still restores Timer background countdown and can notify on completion.

## Performance and power

- [ ] `s` prints input-to-present/frame-time statistics; compare p95 to 0.4.1 under similar workloads.
- [ ] Touch marker follows one finger without a backlog.
- [ ] Rapid gestures do not queue DRM page-flip waits on input processing.
- [ ] No excessive idle redrawing; static Notification Center remains static.
- [ ] The optional backlight controller still honors explicit opt-in and restores backlight after exit.

## Recovery

If the shell fails while LightDM is stopped:

```sh
sudo systemctl start lightdm.service
```

If a raw touch issue is suspected, stop RoundOS and run `sudo evtest /dev/input/eventN (use the touchscreen event reported at startup)`; inspect `ABS_MT_SLOT`, `ABS_MT_TRACKING_ID`, positions, `SYN_REPORT` and `SYN_DROPPED`. Driver-advertised slots do not prove usable physical multitouch.

## 0.5-dev5.1.1 automatic touchscreen discovery regression

`test-drm.sh` no longer assumes `/dev/input/event0`. The `-touch` flag defaults to `auto` in both the launcher and `roundd`.

1. From SSH run `./scripts/test-drm.sh -gesture-config config/gestures.example.json` with no `-touch` flag.
2. Verify startup prints `touch auto-discovered: /dev/input/eventN (<controller name>)` followed by `touch axes ... multitouch=true`.
3. Open diagnostics (`g`) and touch the screen with one or two fingers. Both must appear.
4. Restart after reboot, confirm touch works even if the event number changed.
5. Troubleshooting override: `./scripts/test-drm.sh -touch /dev/input/event1 -gesture-config config/gestures.example.json`. This must still work.

If discovery finds multiple equally likely touch devices, it logs a clear ambiguity warning rather than choosing an arbitrary mouse/touchpad.

## 0.5-dev5.1.4 vertical touch regression

The attempted global Y inversion in 5.1.3 must remain **disabled** on this
HyperPixel: correctly aligned taps and top-edge Notification Center are the
baseline. For the currently observed two-finger vertical shell shortcut, the
scoped `two_finger_vertical_reverse` configuration is enabled by default.
See `REGRESSION-5.1.4.md` for the physical tests and fallback switch.

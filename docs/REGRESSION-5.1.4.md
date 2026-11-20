# RoundOS 0.5-dev5.1.4 — touch coordinates vs. shortcut direction

## Physical issue observed

The tested HyperPixel touch path reports only two simultaneous contacts.
Version 5.1.2 recognized two-finger left/right application switching, but a
physical two-finger downward swipe triggered Home instead of upward. Version
5.1.3 attempted to fix that by globally inverting touch Y. This broke
previously working single-finger tap hit testing and the top-edge notification
swipe. The user's field test found that pressing Clock's bottom Home control
made the touch marker appear near the top; bottom-origin swipes could trigger
notifications.

## Implemented correction

- Default `-touch-invert-y=false` keeps the pre-5.1.3 tap coordinate convention.
- New `two_finger_vertical_reverse` is scoped **only** to the shell's
  `two_swipe` launcher action; it defaults to `true` in the HyperPixel
  configuration and its runtime default profile, and is explicitly present
  in `config/gestures.example.json`.
- Horizontal two-finger app switching, one-finger edge Back/Notifications,
  touch hit-testing, raw contacts, and pinch/rotation are unchanged.
- Alternative hardware can disable the two-finger vertical remap. Touch-panel
  Y inversion remains opt-in for genuinely mirrored touch coordinate devices.

This is a compatibility mapping from a reproducible physical observation, not
proof of a controller-level coordinate-system root cause. `-gesture-log` and
`-touch-trace` can help verify raw contact paths if hardware behavior differs.

## Pi test

Run from SSH:

```sh
cd ~/roundos-0.5-dev5.1.4
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json -gesture-log
```

Check the startup log includes `invert_y=false` and
`vertical_two_swipe_reverse=true`. Then test:

- [ ] Clock bottom Home tap works in the physical bottom of the display.
- [ ] Clock top area does not accidentally trigger the Home control.
- [ ] Touch diagnostics contact markers follow real fingers vertically.
- [ ] Single-finger TOP-edge inward/down gesture opens Notification Center.
- [ ] A one-finger gesture beginning at bottom does not open notifications.
- [ ] Left-edge inward swipe performs Back where a Back action is available.
- [ ] Two-finger left/right swipes switch Clock and Timer.
- [ ] Physical two-finger UP opens Home exactly once.
- [ ] Physical two-finger DOWN does NOT open Home.
- [ ] Pinch and rotate do not invoke a system shortcut.
- [ ] `q` exits the test launcher and restores LightDM.

If the two-finger vertical directions remain wrong while taps are correct,
copy `config/gestures.example.json` to `config/gestures.json` and try
`"two_finger_vertical_reverse": false`; then restart with
`-gesture-config config/gestures.json`. Do not flip global Y to solve a
single gesture classification issue.

## Host verification

- `go test ./...` passed.
- `go test -race ./...` passed.
- `go vet ./...` passed.
- Linux ARMv7 non-DRM cross-build of all three Go binaries passed.

The native Pi DRM/libdrm build and physical touch tests remain unverified.
No kernel driver, overlay, boot configuration, LightDM service, or Plymouth
changes are made by this checkpoint.

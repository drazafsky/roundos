> **Superseded by 0.5-dev5.1.4.** The global Y inversion documented
> below caused mirrored taps and is no longer enabled by default. See
> `REGRESSION-5.1.4.md` for the corrected hardware-aware approach.

# RoundOS 0.5-dev5.1.3: HyperPixel vertical orientation correction

## Hardware report

- Real HyperPixel two-finger horizontal swipes work in 0.5-dev5.1.2.
- Two-finger physical **down** opens Home, while physical **up** does not.
- The active system gesture router only maps **up** to Home; therefore the
  incoming Y orientation appears reversed relative to display-space gestures.
- This is consistent with the evdev decoder scaling raw Y without inversion.

## Fix

`roundd` now defaults to `-touch-invert-y=true` for the known HyperPixel Round
installation. The inversion occurs on both MT and single-touch axes inside the
input decoder before any gesture recognition, application events, or hit tests.
Horizontal motion is unchanged. A non-HyperPixel or differently calibrated
panel can use `-touch-invert-y=false` explicitly. The chosen setting appears
in the startup `touch axes ... invert_y=...` log line.

The unused legacy shell two-finger swipe helper is also updated to agree that
**up** is Home, eliminating a contradictory test expectation.

## Raspberry Pi validation

```sh
unzip roundos-0.5-dev5.1.3.zip
cd roundos-0.5-dev5.1.3
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json -gesture-log
```

1. From Clock/Timer, move **two fingers physically up**; verify Home opens and
   `gesture two_swipe ... direction=up` and `system gesture routed: launcher`.
2. From Clock/Timer, move **two fingers physically down**; verify it does **not**
   open Home and logs `direction=down` (unless another app-defined action exists).
3. Verify left/right two-finger app switching remains unchanged.
4. From the physical **top edge**, swipe down with one finger; verify that
   Notification Center opens. From the physical **left edge**, swipe inward;
   verify Back in a context where Back is available.
5. In diagnostic view (`g`), verify contacts follow the fingers vertically and
   do not appear mirrored. This view deliberately suppresses system two-finger
   navigation.
6. Check ordinary touch targets and the Timer/Clock controls for alignment.

If the coordinates were already correct for a different installation, run the
same test command with `-touch-invert-y=false` to restore legacy behavior.

No driver, overlay, boot-splash, or LightDM changes are required. DRM-enabled
build and physical gesture acceptance remain Pi-side checks.

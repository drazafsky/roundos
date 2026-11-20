# Archived RC3 README (reference only)

# RoundOS 0.5-rc3-wifi — optional app registry + rotary picker Cancel

This release-hardening candidate is built from the exact `roundos-0.5-rc2-wifi` source. It preserves the Wi-Fi settings implementation. It adds trusted, startup-discovered Go apps in the circular launcher/switcher and explicit touch/encoder Cancel for all pickers. It is a **hardware-testing candidate**, not a final 0.5 release.

**Quick start:**

```sh
unzip roundos-0.5-rc3-wifi.zip
cd roundos-0.5-rc3-wifi
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

**Release notes and device tests:** [docs/RC3-RELEASE.md](docs/RC3-RELEASE.md). **App SDK/install guide:** [docs/SDK-APPS.md](docs/SDK-APPS.md). **Wi-Fi documentation:** [docs/WIFI-SETTINGS.md](docs/WIFI-SETTINGS.md).

Optional apps are **unsandboxed**; do not install untrusted code. The default `/etc/roundos/apps` registry is opt-in and is absent on systems that have not installed additional apps. Never replace the previously validated release until physical acceptance tests pass.

---

# RoundOS 0.5-rc2 Wi-Fi Settings development checkpoint

**New in this checkpoint:** Settings is the third app in the Home carousel. Settings → Wi-Fi Networks uses NetworkManager to scan/select/connect/disconnect networks, with touch and rotary controls, secure QWERTY password entry, nonblocking operations and an explicit confirmation warning before network changes. [Testing and limitations](docs/WIFI-SETTINGS.md).

**This builds on hardware-validated 0.5-rc1, which remains the rollback baseline.** Host tests passed; physical Wi-Fi testing on the Pi is pending.

# RoundOS 0.5-rc1 — release candidate for hardware testing

**Status:** Host-validated, **not yet physically validated on the Pi**. Built on the user-confirmed `roundos-0.5-dev7` baseline. All three processes use IPC **v7**; replace all three binaries together.

## Quick start (SSH)

```sh
unzip roundos-0.5-rc1.zip
cd roundos-0.5-rc1
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

The test script temporarily stops LightDM and attempts to restore it on exit. Exit via `q`; if necessary `sudo systemctl start lightdm.service`. Never launch directly from a Pi desktop session. The package changes **no boot splash, Linux driver, overlay, or installed RoundOS version**.

**New vs dev7:** Real Clock/Timer app processes now publish the semantic widget trees (rather than just fixture PNGs); a shell-rendered application dialog system; number, time and date picker controls; a scrollable Clock widget gallery; continuous, bounded, velocity-dependent inertial list scrolling; tests for app IPC, modal focus/cancel, picker value dispatch, scrolling geometry and Reduce Motion. The `roundd` event loop now connects actual touch drag events to the circular widget scrolling and slider handling. Previously working notifications, text editor, input auto-discovery and two-contact gesture mapping are retained.

**Important limitation:** Time/number/date controls on Clock's "Picker Examples" screen are **SDK demonstrations, not a real alarm or calendar scheduler**. The generic installed-app registry is still fixed to Clock and Timer. The input-to-visible p95 latency target needs hardware measurement; host tests cannot certify Pi DRM performance. Full rotary-only cancellation of an open picker remains an input-UX refinement (Back is available by touch/SSH; the system switcher remains available by hardware long-press).

Read [docs/RC1-HARDWARE.md](docs/RC1-HARDWARE.md) for the acceptance checklist and troubleshooting steps. The previous dev7 directory should be kept for rollback.

---

# RoundOS 0.5-dev7 — Milestone 7 integration candidate

**Test on your Raspberry Pi before replacing any working release.** This development source adds the approved focused-carousel Home, centrally themed circular widgets and app-owned full-screen navigation stacks to the hardware-tested 0.5-dev6 lineage. It preserves the verified two-contact HyperPixel gestures, runtime appearance, editor, background Timer and auto-discovered touchscreen path.

Quick start from SSH:

```sh
unzip roundos-0.5-dev7.zip
cd roundos-0.5-dev7
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

**Full Milestone 7 feature list, hardware test plan and known limits:** [docs/MILESTONE-7.md](docs/MILESTONE-7.md). **IPC version 6:** all three executables must come from the same package. Keep 0.5-dev6 intact for rollback.

---

# RoundOS 0.5-dev6 — Milestone 6: Text entry and accessibility

**Source checkpoint, not the full RoundOS 0.5 release.** Based on the user-tested 0.5-dev5.1.4 branch. The display/touch driver, auto-discovered evdev node, unmirrored Y coordinate mapping, two-contact gesture routing and nonblocking DRM renderer are unchanged.

## Quick start (SSH on Raspberry Pi)

```sh
unzip roundos-0.5-dev6.zip
cd roundos-0.5-dev6
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

The script stops LightDM while the DRM test runs, then attempts to restore it. Keep your working 0.4.1 build and driver untouched. **Do not launch this script from the desktop session.** `libdrm-dev` is required for the Pi `-tags drm` build.

### New features

- **Clock app text entry:** tap Clock's EDIT LABEL button; use the touch QWERTY or rotary wheel, Done to save. The shell delivers the value over the existing authenticated IPC connection.
- **Back:** first Back hides the keyboard and preserves the draft. A second Back prompts to keep editing or discard unsaved changes. The default dialog focus is KEEP.
- **Theme:** DARK/LIGHT/HIGH CONTRAST colors restyle all shell-rendered views immediately. Settings also contain Text Scale, Reduce Motion and Large Targets. Runtime preferences are saved to `<state-dir>/appearance.json`.
- **Navigation:** `k` opens the running-app switcher; rotate down to TEXT EDIT or ACCESSIBILITY. SSH `e` opens the text editor demo; `z` opens accessibility; `b` sends Back; `l/r/p` simulate rotary. `g`, `n`, `c`, `t`, `h`, `s`, `q` retain their earlier meanings.

**Detailed features, known limits and hardware test steps:** [docs/MILESTONE-6.md](docs/MILESTONE-6.md). The remainder of this README retains prior milestone regression and driver guidance.

---

# RoundOS 0.5-dev5.1.4 — restored tap locations and isolated two-finger Home direction

**This checkpoint corrects the regression introduced by 0.5-dev5.1.3.**
The physical HyperPixel test showed that 5.1.3's default global Y inversion
mirrored taps and the one-finger top-edge Notification Center shortcut. Clock's
lower Home control was hit near the upper part of the screen, and notifications
could open from the bottom.

The source now **defaults to `-touch-invert-y=false`**, restoring the correct
mapping verified in 5.1.2. A separately configurable system shortcut mapping
(`two_finger_vertical_reverse: true` in `config/gestures.example.json`) maps the
vertical two-finger gesture reported by the controller to Home **without
changing contact positions, touch hit testing, one-finger gestures, or app
contact snapshots**. The empirical direction setting can be switched off for
a differently oriented touchscreen. The three-to-five finger shortcuts remain
disabled for the verified two-contact HyperPixel profile.

To build and test from an **SSH** session:

```sh
unzip roundos-0.5-dev5.1.4.zip
cd roundos-0.5-dev5.1.4
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json -gesture-log
```

**Check on hardware in this order:**

1. On Clock, tap its lower Home button. It must activate from the actual lower
   screen, not from a mirrored upper position.
2. From the physical TOP edge, swipe DOWN with **one finger** to open
   Notification Center. Starting from the bottom must not open it.
3. On Clock/Timer, swipe LEFT and RIGHT with two fingers to switch applications.
4. Swipe physically UP with two fingers to open Home. Swipe physically DOWN
   with two fingers and confirm it does not open Home.
5. In diagnostics (`g`), verify touch markers follow fingers with the correct
   vertical orientation.

For this specific HyperPixel the two-finger event used by the launcher may log
`two_swipe direction=down` when the fingers physically moved up; this is the
raw label before the scoped shell compatibility mapping. The routed action
should log `system gesture routed: launcher`. A different controller can set
`"two_finger_vertical_reverse": false` in its gesture configuration. For
panels whose *tap coordinates themselves* are inverted, `-touch-invert-y=true`
remains an explicit override, but should **not** be used on this working setup.

See `docs/REGRESSION-5.1.4.md` for the full regression checklist.

---

# Previous checkpoint: RoundOS 0.5-dev5.1.2 — Two-Contact Gesture Regression Fix

**This is an independently testable milestone-5 development checkpoint, based on the source of RoundOS 0.4.1.** It does **not** include the as-yet-unavailable 0.5-dev1 through 0.5-dev4 code (runtime themes, complete circular widgets, or finalized launcher/navigation). Do not treat it as the full RoundOS 0.5 release.

The working 0.4.1 implementation was the only prior source available to extend. The existing DRM presenter, independent Clock and Timer processes, background countdown, restart supervision, touch decoder, and touch-render decoupling are retained. IPC is now **version 5**; build `roundd`, `round-clock` and `round-timer` together from this directory.

## Two-finger system gesture regression fix (5.1.2)

A two-finger swipe could be misclassified as a pinch when the touchscreen
reports one contact's updated coordinates before the other's. That temporarily
changes fingertip spacing, and the old recognizer permanently claimed a pinch,
preventing both application switching and the launcher shortcut. This build
fixes classification with coherent per-finger movement and delayed/sustained
transform confirmation. A failing-before/passing-after reproduction and shell
routing tests now cover separately updated contact positions.

Normal operation remains input-event driven. For diagnostic-only raw touch
coordinates, add `-touch-trace` (rate-limited logging). For emitted gestures
and shell actions, add `-gesture-log`. See
`docs/REGRESSION-5.1.2.md` for the physical Pi test procedure.

## Automatic touchscreen discovery (5.1.1 fix)

Linux may assign a different `/dev/input/eventN` after boot. On this hardware, `event1` works while the previous hard-coded `event0` does not. The shell now uses `-touch auto` by default: it examines `/sys/class/input/event*/device/capabilities` for proper touch axes, consults udev's touchscreen classification when available, and prefers known HyperPixel-compatible touch controller names. It rejects touchpads and refuses to silently select between equally ranked candidate touchscreens. This does not change the kernel or display overlay and requires no user interaction when the device is unambiguous.

To override for diagnostics only: `./scripts/test-drm.sh -touch /dev/input/event1`. To disable touch deliberately: `./scripts/test-drm.sh -touch off`. The chosen device name and path are logged on startup. Detection also applies when launching `roundd` directly. If detection fails, the shell keeps SSH controls available and logs the reason.

## Updated for the actual HyperPixel touch input path

**Hardware finding:** `evtest` showed a maximum of **two simultaneous `ABS_MT_TRACKING_ID` contacts** even with three fingers on the Pimoroni HyperPixel 2.1 Round screen. Linux's advertised five tracking slots are not evidence of five detectable fingers.

This checkpoint keeps the original 0.5-dev5 adaptive notifications and nonblocking render/input pipeline, but changes the gesture profile:

- `max_contacts: 2` is the default verified HyperPixel capability. This is a **profile for shell shortcuts and diagnostics**, not a hard cap on the generic touch decoder.
- `two_finger_switch: true` in `config/gestures.example.json` enables two-finger left/right swipes to switch between running applications.
- `two_finger_launcher: true` enables a two-finger upward swipe to open Home/launcher.
- One-finger left-edge Back and top-edge Notification Center gestures remain enabled.
- Three-/four-/five-finger shortcuts cannot activate unless a future hardware profile explicitly sets `max_contacts` to a sufficient number; the underlying recognizer and raw app contact stream remain generic.
- Two-finger pinch and rotation retain priority over two-finger system navigation. If `two_finger_switch` and `two_finger_launcher` are false, the shell forwards unclaimed two-finger swipes to foreground applications.
- Diagnostic screen now displays `TOUCH N OF 2` and renders the actual contacts; the PNG preview shows two touches.
- Existing Timer completion notifications, critical-alert handling, IPC, runtime state recovery, and switcher are unchanged.

**Configuration defaults:** the shell starts with two-finger shortcuts **off** if `-gesture-config` is omitted, to avoid surprising applications that use two-finger swipes. The bundled example JSON enables them for this hardware test. To opt out, set those two keys to `false`. No driver/overlay/kernel changes are required.

This is a hardware-validated input profile, **not proof of a controller silicon limit**. If a driver update later exposes additional contacts, set a higher `max_contacts` and validate it with `evtest` first.

### Why this is still a checkpoint build

Like 0.5-dev5, this is built from the stable **RoundOS 0.4.1** foundation. Earlier 0.5 runtime-theme, circular widget, and finalized launcher milestones are still not present in this package. Do not treat it as the complete 0.5 release.

## Install and build on the Raspberry Pi Zero 2 W

Preserve your working 0.4.1 directory and do **not** replace boot configuration, kernel modules, or the existing display overlay.

1. Copy `roundos-0.5-dev5.1.4.zip` to the Pi, then in an **SSH session** run:

```sh
sudo apt update
sudo apt install -y build-essential pkg-config libdrm-dev unzip evtest
unzip roundos-0.5-dev5.1.4.zip
cd roundos-0.5-dev5.1.4

go test ./...
./scripts/build.sh
```

2. Render previews without touching the desktop:

```sh
./roundd -backend png -preview-page notifications -output /tmp/roundos-notifications.png
./roundd -backend png -preview-page switcher -output /tmp/roundos-switcher.png
./roundd -backend png -preview-page touch -output /tmp/roundos-touch.png
```

3. Test on the actual round display from **SSH**:

```sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

The script temporarily stops LightDM, owns `/dev/dri/card0` / `DPI-1` and an auto-discovered `/dev/input/eventN`, and attempts to restore the graphical desktop on exit. **If restoration fails:** `sudo systemctl start lightdm.service` over SSH. Test from SSH, not from the Pi display itself.

### SSH controls

| Command | Action |
|---|---|
| `h`, `c`, `t` | Home, Clock, Timer |
| `g` | Touch diagnostics (tracks all contacts) |
| `k` | Compact running-app switcher (includes Notifications entry) |
| `n` | Notification Center |
| `l`, `r`, `p` | Simulate rotary left, right, short press |
| `nq` | Simulate a quiet weather update |
| `na` | Simulate an actionable timer notification/banner |
| `nc` | Simulate a critical **system** alert |
| `a` | Acknowledge critical alert |
| `x` | Crash Timer intentionally to test recovery |
| `s` | Print input/render/performance statistics |
| `q` | Quit and restore desktop |

To test two-finger shortcuts, use the provided example directly: `./scripts/test-drm.sh -gesture-config config/gestures.example.json`. Place two fingers down and move them together left/right to switch applications, or upward to open Home. For app-defined pinch/rotate, the recognizer will not turn a genuine transform into a system swipe. Disable the two optional shortcuts in a copied configuration when your application needs uninterrupted two-finger swipes. Keep `-gesture-log` off for latency measurements; `-gesture-log` helps recognition debugging but prints to SSH.

For smoke-test details see **`docs/HARDWARE-TESTS.md`**.

## Developer APIs

```go
// From a connected RoundOS application:
client.Notify("Timer finished", "Countdown completed", "actionable", "timer-finished", "open")
client.SubscribeContacts(true)
for event := range client.Events() {
    if event.Type == "contacts" {
        // event.Contacts includes each current tracking ID/slot/position.
    }
    if event.Type == "gesture" && event.Gesture != nil {
        // See 'three_swipe', 'four_tap', 'five_contacts', etc.
    }
}
```

Raw contact snapshots are intentionally coalescible when apps cannot read them quickly. Applications must never treat raw frames as an infallible high-frequency recording. System edge gestures claim the sequence and send an app cancel event.

## Tests

```sh
go test ./...
go vet ./...
go test -race ./...  # development PC with race-enabled toolchain
```

The Go unit/integration suite covers strict swipe recognition, four-finger tap suppression, five-contact lifecycle, edge conflict arbitration, priority downgrade, notification grouping and dismissal, IPC transport, timer-recovery notification state, async presenter, and existing shell integration. **Actual evdev, DRM, real finger count, and p95 responsiveness still require running the hardware tests on your Pi.**

## Incomplete / pending before full 0.5 release

- No dev1–dev4 widget library, runtime theme engine, final focused-carousel launcher, or full navigation stack is available in this source checkpoint. The new screens use the existing 0.4.1 renderer and palette as a provisional UI.
- Advanced gesture settings are file-based, not yet editable via the full-screen Settings UI.
- Notification history is in-memory for this checkpoint; persistence, app-specific priority UI, and system alert sources remain future integration work.
- Full physical five-contact reliability and p95 latency measurements are unverified.
- Physical rotary hardware is still optional; SSH simulation and optional evdev encoder are available.

**Rollback:** exit with `q`; run your working 0.4.1 source and binaries again. This checkpoint makes no boot-time configuration changes.

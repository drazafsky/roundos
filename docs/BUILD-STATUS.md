# Build and validation status — RoundOS 0.5-dev7

Source: Milestone 7 candidate extending the user's hardware-tested 0.5-dev6 baseline.

Host platform: Linux amd64; Go 1.23.2.

- `go test ./... -count=1` — **PASS**, including independently running Clock/Timer, background Timer countdown, crash recovery, IPC, text editor, adaptive notifications, gestures, and new circular widget/navigation tests.
- `go test -race ./... -count=1` — **PASS**.
- `go vet ./...` — **PASS**.
- `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build ...` — **PASS** for roundd, round-clock and round-timer. This builds the non-DRM preview variant, not the Pi's actual DRM bridge.
- PNG render previews generated and inspected: Home focused carousel, Clock, Clock Options, Timer and Timer Duration.

Pending verification:

- Native DRM-enabled Pi build (`./scripts/build.sh`) with `libdrm-dev`.
- User's physical HyperPixel 2.1 Round touch-target and rotary-gesture testing.
- Real-world latency and CPU measurements on Raspberry Pi Zero 2 W, and longer-run memory/power observations.
- Final-release features identified in `docs/MILESTONE-7.md` as not yet complete (e.g., continuous kinetic scrolling and generalized SDK dialogs/pickers).

Nothing in this package changes the Linux kernel module, display overlay, boot settings, splash logo, or installed 0.5-dev6 release. It is safe to keep the known-good build for rollback.

---

# Build and validation status — RoundOS 0.5-dev5.1

Source base: RoundOS 0.4.1. Source is a milestone-5 backport/development branch, not a full implementation of earlier 0.5 milestones.

Checked in container environment (Linux amd64, Go 1.23.2):

- `go test ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./...` — PASS
- `go build -o roundd ./cmd/roundd` (non-DRM preview backend) — PASS
- 480×480 PNG previews (touch, notifications, switcher) — PASS

Not validated here:

- Pi ARMv7 DRM/libdrm build. A host-side ARM cross-compilation attempt exceeded the available execution time; it is not considered a pass or a compiler failure. Use `./scripts/build.sh` on the Pi with libdrm-dev installed.
- The user observed only two simultaneous `ABS_MT_TRACKING_ID` inputs with a third finger down; 3+/4+/5-finger physical shortcuts are deliberately unavailable in this HyperPixel profile.
- DRM frame latency or p95 input-to-present on the physical device.
- Gesture recognizer behavior under actual FT5x06 touch noise.
- Final runtime theme integration (depends on earlier dev2 artifact not available here).
- Full widget library, focused-carousel redesign, or final Settings UI (earlier milestones not present in this source base).

The source package contains no boot, kernel, device-tree, or systemd service modifications. Preserve the working RoundOS 0.4.1 release for rollback.

New in dev5.1:

- Hardware-aware max-contact profile (2 by default), plus optional two-finger left/right app switching and two-finger upward launcher.
- Updated two-contact diagnostic display and preview.
- Routing tests for two-contact actions, unsupported shortcuts, and generic future five-contact profiles.
- No changes to the existing HyperPixel overlay or driver.

## 0.5-dev5.1.1 — touchscreen discovery fix

The previous test launcher hard-coded `/dev/input/event0`, which can move between boots. The new shell auto-selects the touchscreen from sysfs capabilities, udev touchscreen classification, and controller names, ignoring touchpads. `-touch` remains an optional explicit override; `off` remains supported. Added discovery regression tests, including changing event numbers and ambiguous devices. Running on the physical HyperPixel remains a separate test.


## 0.5-dev5.1.4 — scoped gesture compatibility correction

The physical HyperPixel confirmed that global Y inversion in 5.1.3 was
incorrect: it reversed tap locations and one-finger edge gestures. Default
`-touch-invert-y` is now false, and a separately configurable
`two_finger_vertical_reverse` remaps only the shell's two-finger launcher
action. New tests exercise bottom/Home tap vs top touch, top-edge notifications,
horizontal swipe preservation, both vertical mapping policies, and the default
decoder's unmirrored coordinates. Host `go test ./...`, `go test -race ./...`,
`go vet ./...`, and non-DRM ARMv7 cross-build all passed. DRM-enabled build
and physical HyperPixel verification must still run on the Pi.

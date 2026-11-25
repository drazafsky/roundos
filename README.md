# RoundOS 0.7.0 — Release candidate

Candidate source targeting the Raspberry Pi Zero 2 W and Pimoroni HyperPixel 2.1 Round (480×480) on Raspberry Pi OS Trixie. Stable 0.6.0 and the recorded update-feature implementation have completed physical acceptance. **This 0.7.0 candidate still requires its own Pi acceptance**; earlier results do not attest it. See [0.7.0 candidate and acceptance](docs/RELEASE-0.7.0.md).

## Included

- Focused carousel Home, shell-owned semantic widgets/navigation, Clock/Timer, background supervision, notifications, QWERTY/rotary editor, runtime themes and accessibility, two-contact gesture profile, Wi-Fi scanning/remembered networks with NetworkManager, trusted optional app manifests.
- Settings update checks, prompts, notes, verified installation and recovery; RUI layouts for Clock and Timer. Current main also includes the launcher carousel, wrapping preference, required RUI icon declarations, and local date/time header from the planned 0.8.0 launcher work.
- The shell retains DRM/Wi-Fi access; apps run as `roundos-app` and preserve their existing private state. IPC v7 remains unchanged.
- New: managed boot service, bounded crash restart, explicit activation/desktop restoration, immutable root-owned release layout with checksum verification and managed rollback, periodic input/render metrics, read-only JSONL host performance collection, source release identity (`roundd -version`), and updated public SDK aliases.

## Build and install (SSH only)

Use the exact verified ARMv7 candidate package linked from its acceptance record and follow [CI package installation](docs/CI-RELEASES.md#installing-on-a-32-bit-pi). For a native source build, run `go test ./...` and `sh scripts/build.sh`, then review `sudo ./scripts/install-release.sh` before installation. Preserve the package's complete version and commit identity in the acceptance record.

**Installation does not stop LightDM or enable the boot service.** Review [0.7.0 candidate notes](docs/RELEASE-0.7.0.md), the [managed installation background](docs/RELEASE-0.5.0.md) and [recovery guide](docs/SYSTEMD-ROLLBACK.md). To try the managed boot path, over an SSH session execute:

```sh
sudo roundosctl activate
```

This explicitly switches from LightDM to the RoundOS service and enables it on boot. Verify touchscreen/rotary/Wi-Fi first, then reboot to check automatic startup. Use `sudo roundosctl disable` to restore the prior desktop state. `sudo roundosctl rollback` restores a previous **managed** release, if one exists; otherwise it disables RoundOS and restores the desktop so the earlier standalone RC4 launcher can be run manually.

For manual SSH-console testing, follow the candidate/recovery guide to stop the managed display session before running the verified candidate's `./scripts/test-drm.sh`. A systemd service has **no interactive stdin**, so SSH `l/r/p/s/q` commands are available only in a manual test session, not to the auto-started service.

## Hardware and performance options

The managed service defaults to the physical touchscreen and the verified unmirrored-Y, two-contact gestures. Optional rotary device paths and backlight settings may be configured by a root-owned `/etc/roundos/device.env` (see release docs); no Wi-Fi passwords or other secrets belong in that file. Shell settings are under `/var/lib/roundos/shell-data`, app state under `/var/lib/roundos/app-data`, trusted manifests in `/etc/roundos/apps`, and remembered Wi-Fi credentials continue to be managed by NetworkManager.

To record host CPU, RSS and thermal data while RoundOS is running:

```sh
python3 scripts/collect-baseline.py --seconds 60 --output /tmp/roundos-idle.jsonl
```

Periodic app-RSS, frame p95 and input-to-present p95 summaries are written to the RoundOS journal every 60 seconds. See [docs/PERFORMANCE-0.5.0.md](docs/PERFORMANCE-0.5.0.md).

## Important boundaries

This release does **not** install a driver, change a kernel overlay, alter Plymouth, erase Timer state, modify existing NetworkManager profiles, or automatically enable a replacement desktop. No untrusted app sandbox exists: optional apps share one non-root account. Use a reviewed local build or a verified release package; never run a privileged installer from an untrusted archive.

See [docs/SDK-APPS.md](docs/SDK-APPS.md) for the IPC v7 Go SDK, [docs/SECURITY-0.5.0.md](docs/SECURITY-0.5.0.md) for the privilege boundary, and [docs/TESTING-0.5.0.md](docs/TESTING-0.5.0.md) for final-release acceptance criteria.

## Contributing

Follow the [repository rules](AGENTS.md) for issue tracking, Copilot review,
comment resolution, and squash merging with branch deletion.

Every PR runs automated tests and builds. Successful merges to `main` publish
installable ARMv7 development prereleases. See [CI and releases](docs/CI-RELEASES.md)
for checks, downloads, installation, and the separate physical Pi acceptance gate.

See [release updates](docs/UPDATES.md) for Settings controls, installation, and
SSH recovery, and the [update acceptance record](docs/UPDATE-ACCEPTANCE.md) for
the completed prior-build feature checks. New candidate acceptance is tracked separately.

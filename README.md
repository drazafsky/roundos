# RoundOS 0.5.0 — Managed release

Release source extending the **hardware-validated RoundOS 0.5-rc4-wifi** baseline. RoundOS targets the Raspberry Pi Zero 2 W and Pimoroni HyperPixel 2.1 Round (480×480) with working DRM display/touch integration on Raspberry Pi OS Trixie. **The new unattended systemd startup and desktop switch are not yet hardware-validated**; install them deliberately over SSH.

## Included

- Focused carousel Home, shell-owned semantic widgets/navigation, Clock/Timer, background supervision, notifications, QWERTY/rotary editor, runtime themes and accessibility, two-contact gesture profile, Wi-Fi scanning/remembered networks with NetworkManager, trusted optional app manifests.
- The shell retains DRM/Wi-Fi access; apps run as `roundos-app` and preserve their existing private state. IPC v7 remains unchanged.
- New: managed boot service, bounded crash restart, explicit activation/desktop restoration, immutable root-owned release layout with checksum verification and managed rollback, periodic input/render metrics, read-only JSONL host performance collection, source release identity (`roundd -version`), and updated public SDK aliases.

## Build and install (SSH only)

```sh
cd ~
unzip roundos-0.5.0.zip
cd roundos-0.5.0
go test ./...
./scripts/build.sh
sudo ./scripts/install-release.sh
sudo roundosctl verify
sudo roundosctl status
```

**Installation does not stop LightDM or enable the boot service.** Review [docs/RELEASE-0.5.0.md](docs/RELEASE-0.5.0.md) and [docs/SYSTEMD-ROLLBACK.md](docs/SYSTEMD-ROLLBACK.md). To try the managed boot path, over an SSH session execute:

```sh
sudo roundosctl activate
```

This explicitly switches from LightDM to the RoundOS service and enables it on boot. Verify touchscreen/rotary/Wi-Fi first, then reboot to check automatic startup. Use `sudo roundosctl disable` to restore the prior desktop state. `sudo roundosctl rollback` restores a previous **managed** release, if one exists; otherwise it disables RoundOS and restores the desktop so the earlier standalone RC4 launcher can be run manually.

For manual SSH-console testing without changing boot, keep using the previously validated RC4 directory and `./scripts/test-drm.sh`. A systemd service has **no interactive stdin**, so SSH `l/r/p/s/q` commands are available only in a manual test session, not to the auto-started service.

## Hardware and performance options

The managed service defaults to the physical touchscreen and the verified unmirrored-Y, two-contact gestures. Optional rotary device paths and backlight settings may be configured by a root-owned `/etc/roundos/device.env` (see release docs); no Wi-Fi passwords or other secrets belong in that file. Shell settings are under `/var/lib/roundos/shell-data`, app state under `/var/lib/roundos/app-data`, trusted manifests in `/etc/roundos/apps`, and remembered Wi-Fi credentials continue to be managed by NetworkManager.

To record host CPU, RSS and thermal data while RoundOS is running:

```sh
python3 scripts/collect-baseline.py --seconds 60 --output /tmp/roundos-idle.jsonl
```

Periodic app-RSS, frame p95 and input-to-present p95 summaries are written to the RoundOS journal every 60 seconds. See [docs/PERFORMANCE-0.5.0.md](docs/PERFORMANCE-0.5.0.md).

## Important boundaries

This release does **not** install a driver, change a kernel overlay, alter Plymouth, erase Timer state, modify existing NetworkManager profiles, or automatically enable a replacement desktop. No untrusted app sandbox exists: optional apps share one non-root account. The installer is intended for a reviewed local build on the Pi; never run a privileged installer from an untrusted archive.

See [docs/SDK-APPS.md](docs/SDK-APPS.md) for the IPC v7 Go SDK, [docs/SECURITY-0.5.0.md](docs/SECURITY-0.5.0.md) for the privilege boundary, and [docs/TESTING-0.5.0.md](docs/TESTING-0.5.0.md) for final-release acceptance criteria.

## Contributing

Follow the [repository rules](AGENTS.md) for issue tracking, Copilot review,
comment resolution, and squash merging with branch deletion.

Every PR runs automated tests and builds. Successful merges to `main` publish
installable ARMv7 development prereleases. See [CI and releases](docs/CI-RELEASES.md)
for checks, downloads, installation, and the separate physical Pi acceptance gate.

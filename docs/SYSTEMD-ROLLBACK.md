# Managed startup and rollback runbook

**Do not run from the Pi desktop.** Use SSH, ideally with a serial console or local input as backup. Switching the display manager can terminate desktop sessions, and changing Wi-Fi can disconnect SSH.

## Release lifecycle

1. Build on the Pi or extract and verify an ARMv7 prerelease as described in [CI and releases](CI-RELEASES.md). If RoundOS is running, use the **new package's** `sudo sh ./scripts/roundosctl.sh disable` over SSH first, and verify that the desktop was restored. Then run `sudo ./scripts/install-release.sh`; this only copies/checks root-owned files, migrates appearance once, installs the managed unit, and calls `daemon-reload`.
2. Run `sudo roundosctl verify`; this checks the current-release symlink target, binary checksums and `roundd -version`.
3. Run `sudo roundosctl activate` to record the previous LightDM enabled/active states, disable/stop LightDM, and enable/start RoundOS. The `Type=notify` unit waits up to 45 seconds for the daemon's readiness signal: one successfully presented frame plus initial Clock and Timer views. Startup failure or timeout disables RoundOS and attempts restoration of LightDM. The five-start limit covers a six-minute window, including maximum startup and shutdown timeouts.
4. Inspect `sudo roundosctl status` and `sudo journalctl -u roundos.service -n 100 --no-pager`. `active` now indicates software initialization completed; **physical display, touch, Wi-Fi and boot acceptance still require observation on the Pi**.
5. Test screen/touch/encoder, app privilege separation, Wi-Fi and Timer recovery, then reboot and repeat.

`roundosctl disable` stops/disables the service and restores LightDM's prior recorded enabled/active states. Runtime-only enablement and links remain runtime-only. A failed enable/start or mismatched restored enablement returns an error and retains `/var/lib/roundos/desktop-before-roundos`. Fix the reported systemd failure and retry `sudo roundosctl disable` over SSH. The record is removed only after successful restoration. Do not delete `/var/lib/roundos` or NetworkManager profiles when rolling back.

`roundosctl rollback` uses `/opt/roundos/previous` for prior managed releases. During an initial 0.5.0 migration, RC4 was launched manually and has **no** managed symlink. In that case rollback disables the new service and restores the desktop; from SSH, start your original `~/roundos-0.5-rc4-wifi/scripts/test-drm.sh` when needed.

Rollback verifies the target's checksums and `roundd -startup-protocol` before stopping the current service or switching links. Only `sd-notify-v1` releases can satisfy this managed startup contract. Earlier 0.5.0 and development packages lack that protocol and are rejected without changing the current release. Use `roundosctl disable` to restore the desktop instead; keep the older launcher for deliberate manual testing. Rollback between readiness-capable releases waits for the same readiness signal as activation. If the selected release fails initialization, RoundOS is disabled and the desktop restoration path is used; the selected links remain available for diagnosis.

## Failure triage

Installation proceeds only while RoundOS is inactive or failed. Rollback rejects
transitional states such as activating, deactivating, or reloading before changing
release links. Wait for startup/shutdown to finish, or use `roundosctl disable`
over SSH to stop the service and recover the desktop first.

```sh
sudo roundosctl status
sudo roundosctl verify
sudo systemctl status roundos.service --no-pager
sudo journalctl -u roundos.service -n 100 --no-pager
./scripts/verify-app-privileges.sh
```

Check missing libdrm/KMS device, incorrect `DPI-1` connector, unreachable encoder device, corrupted `/etc/roundos/gestures.json`, invalid untrusted optional manifests, or absent/misowned `/var/lib/roundos/app-data`. Display driver, overlay and touch configuration are not changed by this installer. **Never** fix an app start failure by running third-party apps as root.

A previous release exists only if `/opt/roundos/previous` resolves to a version directory inside `/opt/roundos/releases`. The release swap uses atomic symlink replacement and never deletes settings, app state, or saved Wi-Fi profiles.

## Provisioning safety

The root-owned installer refuses overwrites of an existing release and custom unit edits. It permits an upgrade of the exact original managed unit or a unit matching a verified template from the current or previous release. This keeps managed upgrades possible after rollback leaves the newer unit installed. New releases store and checksum their unit template alongside the binaries. It does not stop LightDM, modify boot overlays, enable startup, or add user application data to the immutable binary directory. The desktop switch is a **separate** explicit operation. Keep a copy of RC4 before testing initial managed startup.

`scripts/setup-app-user.sh` re-executes itself through sudo when invoked without root privileges, so both protected legacy Timer checks and provisioning run with the required access. Migration preserves the original file, uses mode 0600, and never overwrites existing app state or follows a legacy-state symlink.

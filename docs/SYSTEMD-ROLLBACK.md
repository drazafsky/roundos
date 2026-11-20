# Managed startup and rollback runbook

**Do not run from the Pi desktop.** Use SSH, ideally with a serial console or local input as backup. Switching the display manager can terminate desktop sessions, and changing Wi-Fi can disconnect SSH.

## Release lifecycle

1. Build on the Pi and run `sudo ./scripts/install-release.sh`; this only copies/checks root-owned files, migrates appearance once, installs a disabled unit, and calls `daemon-reload`.
2. Run `sudo roundosctl verify`; this checks the current-release symlink target, binary checksums and `roundd -version`.
3. Run `sudo roundosctl activate` to record the previous LightDM enabled/active states, disable/stop LightDM, and enable/start RoundOS. A failed service launch attempts restoration of LightDM.
4. Inspect `sudo roundosctl status` and `sudo journalctl -u roundos.service -n 100 --no-pager`. Do **not** treat service `active` alone as physical display success.
5. Test screen/touch/encoder, app privilege separation, Wi-Fi and Timer recovery, then reboot and repeat.

`roundosctl disable` stops/disables the service and restores LightDM's prior recorded enabled/active states. Do not delete `/var/lib/roundos` or NetworkManager profiles when rolling back.

`roundosctl rollback` uses `/opt/roundos/previous` for prior managed releases. During an initial 0.5.0 migration, RC4 was launched manually and has **no** managed symlink. In that case rollback disables the new service and restores the desktop; from SSH, start your original `~/roundos-0.5-rc4-wifi/scripts/test-drm.sh` when needed.

## Failure triage

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

The root-owned installer refuses overwrites of `0.5.0` and of a differing existing unit. It does not stop LightDM, modify boot overlays, create a service automatically in the boot target, or add user application data to the immutable binary directory. The desktop switch is a **separate** explicit operation. Keep a copy of RC4 before testing initial managed startup.

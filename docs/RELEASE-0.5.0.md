# RoundOS 0.5.0 final release milestone

## Baseline and scope

The user reported **all RoundOS 0.5-rc4-wifi hardware tests passed** on the Pi Zero 2 W / HyperPixel 2.1 Round. Version 0.5.0 is a separate managed-release source package on top of that baseline. No new GUI behavior is required. The remaining acceptance gate is to validate unattended systemd boot, desktop transitions, recovery, and performance logging on the physical Pi.

RoundOS v0.5.0 retains IPC **v7**. Built-in apps and optional apps must be built against the same protocol; RC4 optional app binaries are expected to remain compatible, but verify them under the managed service.

## Versioned layout

- `/opt/roundos/releases/0.5.0/bin/{roundd,round-clock,round-timer}` — installed root-owned executables
- `/opt/roundos/current` — symlink to current **managed** release, never user-writable
- `/opt/roundos/previous` — symlink to previous **managed** release, when one exists
- `/opt/roundos/current/SHA256SUMS` — installed binary integrity manifest
- `/etc/systemd/system/roundos.service` — service unit, installed but not activated automatically
- `/usr/local/sbin/roundosctl` — root-controlled activation/status/rollback tool
- `/etc/roundos/gestures.json` — preserved if it already exists; otherwise initialized from the hardware-profile example
- `/etc/roundos/device.env` — optional administrator-owned hardware flags
- `/var/lib/roundos/shell-data` — root-owned appearance state; existing `/root/.local/state/roundos/appearance.json` is copied once, if available
- `/var/lib/roundos/app-data` — unchanged app-owned Timer/application state (mode 0700)
- `/etc/roundos/apps` — existing reviewed third-party app manifests and executables
- NetworkManager profiles — unchanged and stored by NetworkManager, not by RoundOS

Only the operator triggers managed activation. The installer refuses to overwrite an existing release and refuses to overwrite a manually modified systemd unit. The previous standalone RC4 directory is **not** automatically imported as a managed release.

## Upgrade from RC4 via SSH

```sh
unzip roundos-0.5.0.zip
cd roundos-0.5.0
./scripts/build.sh
sudo ./scripts/install-release.sh
sudo roundosctl verify
sudo roundosctl status
```

After reviewing the configured hardware paths, activate explicitly:

```sh
sudo roundosctl activate
sudo roundosctl status
sudo journalctl -u roundos.service -n 70 --no-pager
```

If sudo removes SSH environment variables, run `sudo env SSH_TTY="$SSH_TTY" roundosctl activate` from a *real* SSH session. Never run the display transition from the Pi desktop. The app process account is provisioned by the installer using the previously verified RC4 setup logic.

Once validated, `sudo reboot` and verify that the focused Home launcher appears automatically, the desktop is not competing for the DRM device, and the known saved Wi-Fi network reconnects.

## Hardware-specific configuration

The managed service uses the same `-card /dev/dri/card0 -connector DPI-1 -touch auto` defaults as RC4. For a physical encoder or backlight configured via explicit evdev path, create a root-managed file:

```sh
sudo sh -c 'printf "%s\n" '\''ROUNDOS_DEVICE_ARGS="-encoder /dev/input/event2 -backlight auto -idle-off 45s"'\'' > /etc/roundos/device.env'
sudo chown root:root /etc/roundos/device.env
sudo chmod 0600 /etc/roundos/device.env
sudo systemctl restart roundos.service
```

The `event2` path above is **illustrative**, not the previously established device node. Check `ls -l /dev/input/by-id /dev/input/by-path` and the device's actual encoder input first. No need to set an encoder path if one is not installed. Do not put passwords, arbitrary shell commands or user-writable file paths in this root-controlled option file. systemd expands its argument string without invoking a shell.

## Service failure/recovery

- Shell process crash or presenter failure: exit nonzero, systemd `Restart=on-failure` with a 3-second delay, rate-limited to 5 launches in 120 seconds. A clean `systemctl stop` is not restarted.
- Application crash: existing RoundOS app supervisor restarts the individual process and preserves Timer state; exhausted app restarts are reported as offline without rebooting the display.
- Managed uninstall/disable: `sudo roundosctl disable` stops RoundOS, restores prior LightDM state, leaves app data and Wi-Fi intact.
- Release rollback: `sudo roundosctl rollback` swaps to the previous **managed** version. When there is no prior managed version, this disables the service and restores the desktop; launch RC4 manually from its preserved directory.
- Emergency desktop return: `sudo systemctl disable --now roundos.service; sudo systemctl enable --now lightdm.service`. The second command assumes LightDM was your intended previous desktop setup.

## Limits

0.5.0 is not yet a certified Pi-native install until its new systemd lifecycle has passed physical testing. No per-app sandbox exists; apps still share `roundos-app` and its 0700 private data directory. Service unit hardening uses `ProtectSystem=full`, `ProtectHome=yes`, `PrivateTmp=yes`, `NoNewPrivileges=yes`, but the shell must retain root hardware and NetworkManager privileges. Real-world p95 input latency and power/thermal metrics must be collected on the Pi rather than inferred from off-device tests.

# RoundOS 0.5-rc2-wifi — System Settings and NetworkManager

## Purpose

The Settings card is now the third application in the focused launcher carousel (Clock, Timer, Settings). It is shell-owned, uses the global semantic theme, and works through rotary/button input or the touchscreen. Settings → Wi-Fi Networks scans for nearby SSIDs and shows signal, lock/OPEN status and the current connection.

## Hardware test (SSH)

```sh
cd ~/roundos-0.5-rc2-wifi
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

From Home, rotate to **SETTINGS** and press. Alternatively type `w` into the test terminal. Press **WI-FI NETWORKS** to scan. Expect an asynchronous “SCANNING” message, followed by a list of SSIDs. Rotate through entries; press one to open a confirmation. Cancel is focused by default. Select “SCAN AGAIN” to rescan. The Wi-Fi page has a Back entry and the standard left-edge swipe Back gesture.

**Do not change/disconnect the active Wi-Fi network unless you have an alternate recovery path.** Doing so can terminate the SSH connection you are using to run RoundOS. The Linux NetworkManager service remains in control of connections; RoundOS does not modify boot-time Wi-Fi configurations directly.

## Connecting

For an open network, select an SSID, then confirm CONNECT. For a secured personal network (WPA2-PSK or WPA3-SAE where supported by the adapter/NetworkManager), confirm, enter its password in the existing full-screen editor using QWERTY or rotary input, and select DONE. The operation runs in a worker goroutine, so touch rendering stays responsive. Connection success triggers a fresh scan. Errors are shown in the UI and logged to SSH.

Selecting the currently connected SSID offers a **disconnect** confirmation. This may interrupt the current SSH session; a successful disconnection may leave the device offline until NetworkManager reconnects.

Passwords are never passed as command-line arguments or logged by RoundOS. They are handed to `nmcli connection up ... passwd-file` via a temporary mode-0600 file under `/dev/shm` where available, removed afterward; NetworkManager owns saved network credentials. No password is written to RoundOS state or appearance configuration.

## Notes and restrictions

- Requires NetworkManager's `nmcli` binary. Raspberry Pi OS Trixie uses NetworkManager by default. The test launcher runs RoundOS as root, which permits Wi-Fi changes. If launching as an unprivileged systemd service later, NetworkManager authorization must be configured separately.
- Supports visible SSIDs; hidden SSIDs, legacy WEP and enterprise 802.1X configuration are not implemented in this checkpoint.
- NetworkManager may create a new connection profile when joining a secured network. Repeated manual joins can leave duplicate profiles; profile cleanup is left to NetworkManager tooling.
- The password editor is intentionally **secure**, not auto-restored, and clears its contents when dismissed or interrupted. Back first hides the keyboard and subsequent Back invokes discard confirmation when modified.
- Wi-Fi scan and connection operations are asynchronous; status refresh occurs when they complete. Slow network operations do not block the input loop.
- `-backend png -preview-page settings` and `-backend png -preview-page wifi` show theme-aware UI with sample offline SSIDs, not live network data.

## Acceptance checklist

- [ ] Home carousel contains Settings after Clock and Timer; rotary wraps among all three.
- [ ] Settings → Wi-Fi scans and displays actual nearby SSIDs.
- [ ] Wi-Fi entries identify current connection, signal and security.
- [ ] Rotate, press, touch and Back work throughout the Wi-Fi pages.
- [ ] Connection confirmation defaults to Cancel, and warns about SSH loss.
- [ ] Secure network opens masked touch QWERTY + rotary password editor.
- [ ] Saving a valid passphrase initiates a connection asynchronously.
- [ ] Cancelling or leaving the password editor never initiates a connection.
- [ ] After a connection completes, RoundOS refreshes network status.
- [ ] Active-network Disconnect is explicitly confirmed before action.
- [ ] Wi-Fi operations do not regress Clock, Timer, gesture recognition, themes or notifications.

## Rollback

Exit with `q`. If LightDM does not restart, execute `sudo systemctl start lightdm.service`. The known-good `roundos-0.5-rc1` directory remains untouched.

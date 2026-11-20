# RoundOS 0.5 final acceptance checklist

## Source and release

- [ ] `go test ./...`, `go test -race ./...`, `go vet ./...` passed on the Pi.
- [ ] `./scripts/build.sh` produces native DRM `roundd` and Clock/Timer binaries.
- [ ] `roundd -version` prints `RoundOS 0.5.0 (...)` without opening hardware.
- [ ] `sudo ./scripts/install-release.sh` installs without interrupting running LightDM.
- [ ] `sudo roundosctl verify` passes all checksums and `status` shows service inactive until activation.
- [ ] Settings and Timer state migrate or remain preserved, including appearance preferences.

## Managed startup and recovery

- [ ] From SSH, `sudo roundosctl activate` stops LightDM and starts a visible 480×480 RoundOS launcher.
- [ ] Touch targets, two-finger staggered gestures, physical encoder (if configured), Wi-Fi scans, saved password reconnect and non-root app UIDs all work.
- [ ] `sudo systemctl restart roundos.service` restores the shell without resetting saved preferences, Wi-Fi profiles or Timer state.
- [ ] `sudo reboot` launches RoundOS automatically without LightDM racing for KMS, and without SSH/manual console commands.
- [ ] On shell process failure, systemd restarts it with bounded retries; on app crash the existing app supervisor recovers Timer state.
- [ ] `sudo roundosctl disable` restores prior desktop enabled/active state; running the RC4 manual test launcher remains possible.
- [ ] `sudo roundosctl rollback` either swaps to a prior managed version (if installed) or disables RoundOS and restores the desktop.

## Perf and security

- [ ] `roundos-app` owns Clock/Timer/optional app processes; root owns the shell. No optional app executes as UID 0.
- [ ] Repeat idle, gallery flick, Timer background and Wi-Fi scan runs with `scripts/collect-baseline.py`. Record RSS, interval CPU, thermal and frame/input-to-present p95 from journal. Do not invent threshold results.
- [ ] No password appears in process command lines, journal debug logs or performance samples.
- [ ] No regression in the masked editor, rotary-only picker cancellation, notifications, runtime themes, background Clock/Timer or touch orientation.

This release package is **code-complete**, but unattended boot and performance acceptance are unverified until these physical checks are completed. Prior RC4 hardware success does not automatically validate the new systemd lifecycle.

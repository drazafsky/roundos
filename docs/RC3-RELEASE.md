# RoundOS 0.5-rc3-wifi — release-hardening candidate

**Starting point:** exact `roundos-0.5-rc2-wifi.zip` source. Wi-Fi scanning, saved-network ordering, NetworkManager credential handling, password editor, accessibility and touch gestures are preserved. **RC2 Wi-Fi hardware acceptance was not reported in this session; keep previous working builds for rollback.**

## New capabilities

### Trusted optional Go application discovery

RoundOS now checks `/etc/roundos/apps` at startup for **trusted** JSON manifests. Manifest fields:

```json
{"id":"org.roundos.demo","name":"Demo","icon":"DME","exec":"round-demo"}
```

- `id`: globally unique `org.*` app identifier, no duplicate or overriding built-ins.
- `name`: up to 15 Unicode characters for the focused carousel.
- `icon`: one to three uppercase ASCII letters/digits for the circular card.
- `exec`: executable filename relative to the trusted manifest directory; no shell arguments, absolute paths or outside-directory symlinks.
- Maximum 12 optional apps per system. Discovery is at **RoundOS startup**, not on every file change. A restart is required after installing/removing apps.
- The manager launches and supervises each process with its own authenticated IPC token and Unix socket. Built-in Clock and Timer remain unchanged. Custom apps use SDK v7 (`roundos/sdk/app`, `roundos/sdk/ui`).
- App view, nested navigation, action routing, notifications, text editing and focus restoration now support the new app page IDs.
- The Home carousel and app switcher show optional apps alongside Clock/Timer and built-in Settings.
- **Important security boundary:** Optional apps are **not sandboxed**. The DRM test launcher runs RoundOS as root, so a discovered app also runs as root. Accordingly, the registry requires root ownership and safe permissions for all ancestor directories, manifests and binaries when root-launched. Never install unreviewed or downloaded code without isolating it. The example installer is explicitly opt-in and does not run automatically.

### Encoder-only picker cancellation

- While a number/time/date picker is open, **short encoder press = confirm**, **hold encoder approximately 700 ms = cancel without publishing any action**.
- Touch Cancel and OK are separate, centrally displayed buttons. The existing touch Back and SSH `b` paths still cancel.
- A canceled picker long press does not then open the switcher or Home, even if the button remains held for two seconds.

### Performance snapshots

Run `./scripts/collect-perf.sh` from **another SSH session** while RoundOS is active. This prints host load, memory, temperature, and per-process CPU/RSS without network names or passwords. Type `s` in the RoundOS console for frame-p95, input-to-present-p95, coalesced-visual-frame count and memory diagnostics. The software cannot certify physical touch-to-photon latency without measurement on the device.

## Build/test on Pi

```sh
cd ~
unzip roundos-0.5-rc3-wifi.zip
cd roundos-0.5-rc3-wifi
go test ./...
./scripts/build.sh
./scripts/test-drm.sh -gesture-config config/gestures.example.json
```

`./scripts/test-drm.sh` stops LightDM temporarily and attempts restoration at exit; use a separate SSH session (not the desktop). `q` exits. If necessary run `sudo systemctl start lightdm.service`. The build does not touch kernel overlays, splash screen, boot files or the known-good RC2 directory.

### Optional app demonstration

After reviewing `cmd/round-demo/main.go`, run **only if you want to test app installation**:

```sh
./scripts/install-demo.sh
```

This explicitly builds and installs the compiled example into `/etc/roundos/apps` using `sudo`, with admin-owned manifest and executable. Restart the RoundOS test launcher: the fourth Home card should be **DEMO**. Select it by rotary or touch; its button and nested screen should work. It should also appear in the switcher. To uninstall the example, exit RoundOS and then run:

```sh
sudo rm -f /etc/roundos/apps/org.roundos.demo.json /etc/roundos/apps/round-demo
```

Only remove these exact demo paths, not the whole registry folder. Restart RoundOS to refresh the registry.

## Hardware acceptance checklist

- [ ] RC2 Wi-Fi: Settings and saved networks still scan, rank, connect and reconnect; password input remains masked, and Remember uses protected NetworkManager storage.
- [ ] Home: Clock, Timer, Settings remain selectable; any installed Demo is a fourth card, and focus wraps correctly.
- [ ] Demo: app starts, displays SDK widgets, responds to actions, retains state on switching, and appears in the switcher.
- [ ] On a date/time/number picker: rotate the knob, **hold it for 700 ms**, verify it cancels without changing the original value. Repeat holding beyond two seconds; it must not unexpectedly jump Home.
- [ ] Repeat with a short press: selected value commits once. Touch Cancel/OK buttons work and never overlap.
- [ ] Two-finger staggered gestures, notifications, touch QWERTY, rotary wheel and Timer crash recovery have no regressions.
- [ ] During active gallery scrolling and idle UI, collect `s` output and `collect-perf.sh` snapshots on the actual Pi; report the measured values instead of treating host measurements as hardware evidence.
- [ ] Exit with `q` and confirm LightDM restoration.

## Validated off-device

See `docs/RC3-VALIDATION.md` for commands and results. Native Pi DRM compilation, actual touch/rotary behavior, Wi-Fi environment and performance numbers require hardware validation.

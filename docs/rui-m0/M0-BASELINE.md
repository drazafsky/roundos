# RUI Milestone M0 — reproducible RoundOS 0.5.0 baseline

**Current gate: OPEN (hardware not run).** This package is *only* M0 verification tooling and unmodified 0.5.0 application/runtime source. It does not implement YAML/RUI behavior and does not change the Pi installation until the operator explicitly runs the existing installer/activation commands.

## Source provenance

Original source archive `roundos-0.5.0.zip` SHA-256:

```
06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75
```

`source/` is not a fresh reimplementation: this M0 checkpoint contains the actual 0.5.0 source plus **only** additions in `scripts/m0/` and `docs/rui-m0/`. `scripts/m0/BASELINE-SOURCE.SHA256` pins every one of the 149 original release files. New tooling refuses to pass if any original source file is changed, deleted, or replaced, and refuses unexpected new files outside the M0 directories.

### Host verification

```sh
cd roundos-0.5.0-rui-m0
python3 scripts/m0/verify_source.py
python3 scripts/m0/verify_host.py --output ~/roundos-m0-host-verify
```

For strict provenance, if the **original unmodified** ZIP is still available on the test machine:

```sh
python3 scripts/m0/verify_host.py \
  --archive ~/roundos-0.5.0.zip \
  --output ~/roundos-m0-host-verify-sourcechecked
```

The host verifier executes all original Go tests, race detector, vet, Python sampler tests, new isolated release-control tests, shell syntax tests, `roundd -version`, static service policy checks and non-DRM ARMv7 crossbuilds for four application binaries. The report is private JSON plus individual logs. It **never** starts/stops systemd, manipulates `/opt/roundos`, changes Wi-Fi, or claims DRM/Pi acceptance. Run it in a separate output directory each time.

### Pi build and read-only verification

Use a separate SSH session **while keeping the known-good working release available**. The Pi native DRM build must be performed on the Pi because the host does not have libdrm headers. Before activating the system service, run:

```sh
cd ~/roundos-0.5.0-rui-m0
python3 scripts/m0/verify_source.py
./scripts/build.sh
./roundd -version
sudo ./scripts/install-release.sh      # only if managed 0.5.0 is not already installed
sudo roundosctl verify
sudo roundosctl status
```

**Important:** the root-owned installer refuses to overwrite an installed managed release. For an already active verified 0.5.0 installation, do *not* reinstall: preserve it. The `./scripts/build.sh` DRM artifacts should be checked without replacing any running binaries. If commands or service policy differ on the machine, investigate before activating.

## Operator-controlled systemd acceptance

All lifecycle steps require a real Pi SSH connection. Never initiate the transition from a Pi desktop terminal, and do not attempt to force-test boot or rollback by a script on a remote machine. Ensure SSH reachability before activation; keep an SSH session available during display-manager transitions.

Collect a **read-only** state snapshot before and after each action. This example uses `~/roundos-m0-evidence` as a private directory:

```sh
mkdir -m 700 -p ~/roundos-m0-evidence
python3 scripts/m0/acceptance.py --phase preinstall --output ~/roundos-m0-evidence/preinstall.json
# If installed and verified:
python3 scripts/m0/acceptance.py --phase installed --output ~/roundos-m0-evidence/installed.json \
  --observed 'roundosctl verify and status succeeded'

# Explicit, operator-controlled activation from SSH, if appropriate:
sudo roundosctl activate
# Observe the physical 480x480 launcher, normal touch, and correct GPU display ownership.
python3 scripts/m0/acceptance.py --phase active --output ~/roundos-m0-evidence/active.json \
  --observed 'Home launcher visible and responsive on the Pi'
```

Then, **only if safe and desired**, proceed with:

1. `sudo reboot` from SSH; reconnect and inspect `roundosctl status`, physical display, and NetworkManager reconnection. Record `after_reboot.json` with `--phase after_reboot --observed '...'` only after actual observation.
2. `sudo systemctl restart roundos.service` from SSH; verify Clock/Timer, preferences and Wi-Fi remain usable. Record `after_restart.json` after observation.
3. `sudo roundosctl disable` from SSH; confirm LightDM/desktop restoration; record `after_disable.json` after observation.
4. Reactivate **only if appropriate** and execute `sudo roundosctl rollback` from SSH. If there is a previous managed release, it swaps to it. **If no prior managed release exists, rollback deliberately disables RoundOS and restores the desktop**; it does not automatically install RC4. Record `after_rollback.json` based on the behavior you actually observed.

**Emergency recovery:** `sudo systemctl disable --now roundos.service; sudo systemctl enable --now lightdm.service` (when LightDM was the previous desktop). The original manual test launcher and kernel driver are untouched. Do not make a duplicate managed 0.5.0 release merely to satisfy the rollback test.

### Hardware acceptance checklist (operator-observed, not automated assertions)

- [ ] Pi boots straight into the correct RoundOS launcher without LightDM/DRM contention.
- [ ] Touch Y is not mirrored; visible Back, notifications and Clock/Timer controls work.
- [ ] Two-finger staggered left/right and upward Home shortcuts work; physical encoder if installed.
- [ ] Wi-Fi profile reconnects after reboot; credentials are not logged.
- [ ] Clock/Timer run as `roundos-app`, not UID 0; system shell retains required hardware access.
- [ ] Restart does not reset saved theme, Timer state, app manifests or Wi-Fi.
- [ ] Shell/systemd restart limitation and app crash recovery are exercised safely.
- [ ] Disable restores the prior desktop; rollback works for the actual installed release arrangement.
- [ ] Power/backlight behavior is only marked tested if that hardware controller is installed.

The `acceptance.py` snapshots can prove service states and symlink targets but **cannot prove physical screen contents or human touch behavior**; `--observed` explicitly means a human observed that action on the Pi. Never claim full acceptance from snapshots alone.

## Performance baseline protocol

The reference is the **Pi Zero 2 W / HyperPixel 2.1 Round at 480×480** with the managed 0.5.0 service running and `-metrics-every 60s` enabled. `collect_pi.py` wraps the existing safe `scripts/collect-baseline.py` and captures only allowlisted renderer durations from the journal. It will not record SSIDs, passwords, event coordinates or full journal lines.

Capture **three independent 60-second samples** per scenario (12 samples total):

| Scenario | On-device activity during each 60-second run |
|---|---|
| `idle` | Home/Clock idle; no gestures |
| `gallery_flick` | Repeated Clock gallery flicks and short pauses |
| `timer_background` | Run Timer in background and navigate Clock |
| `wifi_scan` | Repeat Settings Wi-Fi scans; keep credentials private |

For example:

```sh
python3 scripts/m0/collect_pi.py --scenario idle \
  --output ~/roundos-m0-evidence/idle
python3 scripts/m0/collect_pi.py --scenario gallery_flick \
  --output ~/roundos-m0-evidence/gallery
python3 scripts/m0/collect_pi.py --scenario timer_background \
  --output ~/roundos-m0-evidence/timer
python3 scripts/m0/collect_pi.py --scenario wifi_scan \
  --output ~/roundos-m0-evidence/wifi
```

The program prompts before each run and saves `run-NN.jsonl` and `session.json` with safe aggregate CPU/RSS/thermal and renderer summaries. Existing output directories are intentionally refused to prevent overwriting evidence. Session acceptance requires the reference Pi, active managed service, 3×60 samples and a performance-journal entry per run; otherwise the session is **collected but not M0-accepted**.

For Reduce Motion comparison, optionally run `--scenario reduce_motion`. Power consumption requires an external meter; the script does not invent battery/current data.

**Latency definition:** `input-to-present-p95` measures from application event handling to DRM presenter completion, NOT touch-controller IRQ to emitted photons. Do not call it electrical or end-to-end user input latency.

If journal summaries are missing because the operator's SSH user cannot read the system journal, adjust journal group access using the normal system administration process. Never paste the full journal to a bug report without checking it for sensitive data.

## Combine evidence into a final M0 gate report

```sh
python3 scripts/m0/report.py \
  --host ~/roundos-m0-host-verify/host-result.json \
  --sessions ~/roundos-m0-evidence/{idle,gallery,timer,wifi}/session.json \
  --acceptance ~/roundos-m0-evidence/{installed,active,after_reboot,after_restart,after_disable,after_rollback}.json \
  --output ~/roundos-m0-evidence/M0-report.json
```

The gate is `OPEN` until the genuine Pi sampling and observed managed-service lifecycle are completed. A complete set is `READY_FOR_HUMAN_REVIEW`, **not automatically certified**, because physical UI, gesture and network behavior still require human sign-off. Keep raw evidence and report for performance comparison during RUI M7. Do not substitute mocked tests or host estimates for Pi measurements.

## M0 exit criteria and status

- **Implemented in this checkpoint:** byte-verified source provenance, untouched 0.5.0 runtime, automated host checks, isolated managed-release simulation, safe Pi read-only samples, lifecycle evidence snapshots and gate report.
- **Passed off-device:** source hash, source-file integrity, and isolated activation/disable/rollback tests. The rest is detailed in `HOST-RESULTS.md` after the host verifier run.
- **Still pending:** on-Pi native DRM compile, unattended systemd boot, real Pi touch/rotary/Wi-Fi behavior, three 60-second runs of each condition, measured frame/input-to-present p95, and physical managed rollback/desktop restoration. No M1 work should depend on assuming these passed.

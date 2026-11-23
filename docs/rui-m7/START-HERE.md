# M7: managed Pi acceptance for RUI 0.6.0

**Hardware gate: PENDING.** VERSION selects the 0.6.0 development candidate
series. It does not certify hardware or publish a stable release. Issue #15
remains open until the evidence below is collected and reviewed.

Use the reference Pi Zero 2 W / HyperPixel 480×480 over SSH. Keep the existing
managed 0.5.0 release and its state. Collect baseline evidence before changing
the release. The operator controls reboot, installation and display transitions
from an SSH terminal; the collector restarts only with its explicit --restart-for-latency option.
Identity inspection, sample binding and comparison remain read-only.

## Prepare the exact candidate and recovery files

Use the ARMv7 package from a successful main CI run containing M4–M7 preparation.
Verify its outer SHA256SUMS, release.json version/full commit, and the extracted
package's SHA256SUMS before running anything from it. Record the workflow URL,
package SHA-256, full commit, version and toolchain in the acceptance notes.
Do not substitute a locally built shell for performance measurements: main CI
uses the pinned .go-version toolchain and ARMv7 DRM build.

Check out the **same commit's source** in a separate Pi workspace for the M7
collectors, integration test, and manual fixture. Release packages contain
binaries and documentation; they do not contain the source/test tools.
Set CANDIDATE_VERSION to the verified 0.6.0-dev version and CANDIDATE_COMMIT
to its full 40-character commit. The installed roundd build ID must match it.

The baseline must actually run 0.5.0 with its original control files. If the Pi
has already been upgraded, wait until its current testing finishes, retain that
working release, and restore the preserved managed 0.5.0 with control files from
the original SHA-256-verified archive. Newer control files cannot substitute for
the originals. The checks below stop before creating a backup if that baseline
has not been restored.

Before the first M7 candidate installation, preserve the verified originals:

```sh
/opt/roundos/current/bin/roundd -version | grep -Fx 'RoundOS 0.5.0 (0.5.0)' && \
printf '%s\n' '06453b95893557e389483a3e06cac2926717d245f7895ef362d3364531c0285c  /etc/systemd/system/roundos.service' '92ddf2e48ca905970f0b9cf45be299b2c64d20d07b3f753e58efedf4e94cdf06  /usr/local/sbin/roundosctl' | sha256sum -c - && \
sudo mkdir -m 0700 /var/lib/roundos/m7-recovery && \
sudo cp -p /etc/systemd/system/roundos.service /var/lib/roundos/m7-recovery/roundos.service && \
sudo cp -p /usr/local/sbin/roundosctl /var/lib/roundos/m7-recovery/roundosctl && \
sudo sh -c 'cd /opt/roundos/releases/0.5.0 && sha256sum -c SHA256SUMS' && \
sudo sh -c 'cd /var/lib/roundos/m7-recovery && sha256sum roundos.service roundosctl > SHA256SUMS'
```

An existing recovery directory requires inspection; never overwrite it with a
later candidate's files. On the reference device, the original unit hash is
06453b95893557e389483a3e06cac2926717d245f7895ef362d3364531c0285c
and the original control script hash is
92ddf2e48ca905970f0b9cf45be299b2c64d20d07b3f753e58efedf4e94cdf06.
Confirm the actual device matches before using this recovery procedure.

The original 0.5.0 uses Type=simple and has no readiness protocol. The newer
roundosctl correctly refuses that target. Recovery to this particular baseline
must also restore its original service/control files, as described below.
Newer readiness-capable release pairs use normal roundosctl rollback.

## Collect matched baseline and candidate runs

First observe a managed 0.5.0 boot, display/touch, gestures, installed encoder
(if present), and Wi-Fi reconnect. Record actual observations, not expectations.
Then collect the baseline scenarios. Each run starts a new managed shell:

```sh
mkdir -m 0700 -p ~/roundos-m7-evidence/baseline ~/roundos-m7-evidence/candidate
python3 scripts/m7/collect.py --expect-version 0.5.0 --restart-for-latency --scenario idle --output ~/roundos-m7-evidence/baseline/idle
python3 scripts/m7/collect.py --expect-version 0.5.0 --restart-for-latency --scenario gallery_flick --output ~/roundos-m7-evidence/baseline/gallery_flick
python3 scripts/m7/collect.py --expect-version 0.5.0 --restart-for-latency --scenario timer_background --output ~/roundos-m7-evidence/baseline/timer_background
python3 scripts/m7/collect.py --expect-version 0.5.0 --restart-for-latency --scenario wifi_scan --output ~/roundos-m7-evidence/baseline/wifi_scan
python3 scripts/m7/collect.py --expect-version 0.5.0 --restart-for-latency --scenario reduce_motion --output ~/roundos-m7-evidence/baseline/reduce_motion
```

Each command prompts for three independent 60-second runs and explicitly
checks that roundos.service is healthy, clears its start-rate counter and
restarts it after you prepare each workload. These deliberate restarts would
otherwise exhaust the five-start limit across scenarios; the installed unit
and its failure policy remain unchanged. The original renderer
keeps a rolling 256-input window, so a fresh process is required to exclude
prior runs. All navigation and controls after restart belong to that workload.
The journal reader accepts only that PID and boot, with a summary inside its
new process window. An interactive run with no measured inputs stays pending. Follow its displayed
workload, using the same theme, text scale, input method, network and device
configuration on both releases. Reduce Motion must be enabled for its scenario
and reset to the same setting for each following workload. The managed service
must log performance summaries every 60 seconds; retain only the collector's
allowlisted timing values. CPU and RSS use the original sampler's definitions.

The wrapper uses sudo for the explicitly requested service restarts and
read-only identity inspection. Enter any sudo
password in your own SSH terminal. Outputs start private (0600/0700), refuse
existing paths, and never record process arguments, SSIDs, editor values or
credentials. Before/after identity must match the actual root-owned running
binary, Timer hash, Pi identity, boot and PID. Raw process samples are checked
against that PID and hashed; CPU/RSS summaries are recalculated and require at least 45 finite,
nonnegative CPU readings and 45 positive RSS readings per run. Incomplete
runs, absent journal timings, a host smoke test or an unexpected restart during sampling cannot pass.

From the verified candidate package directory, after baseline collection:

```sh
sudo /usr/bin/systemctl stop roundos.service && \
sudo sh scripts/install-release.sh && \
sudo /usr/local/sbin/roundosctl verify && \
sudo /usr/local/sbin/roundosctl activate
```

Observe the launcher and bounded readiness, then reboot from SSH and confirm
boot/Wi-Fi/display. Collect the same five scenarios with
--expect-version "$CANDIDATE_VERSION" --restart-for-latency and output under
candidate/SCENARIO.
Finish all performance collection before the separate desktop/recovery checks.

Run the traffic test on the Pi from the matching source workspace against the
installed Timer, using the available native Go toolchain:

```sh
ROUNDOS_M7_TIMER_BINARY=/opt/roundos/current/bin/round-timer \
  go test ./tests/integration -run '^TestM7ActualTimerTrafficAgainstV050$' -v -count=1
```

Save the JSON following M7_TIMER_PROTOCOL as protocol.json. Its candidate
executable hash must equal the identity-bound installed Timer. Earlier native
source-build results demonstrate the test, but cannot certify a different
packaged executable.

## Observe functionality and reloads

Copy observations.template.json to a private evidence directory. Fill in the
exact candidate commit. For each check record observed=true or false with a
specific observation and evidence location. null or absent evidence stays
PENDING; an observed failure is FAIL. Keep sensitive values out of notes/video.

Check Clock label save/cancel and draft preservation, alarm/snooze/date controls,
all gallery buttons, Timer picker/slider commit/cancel/reset dialog, background
deadlines and notifications, persisted state, and app crash recovery. Exercise
Back/notifications, horizontal switching and staggered two-finger Home. Repeat
touch/rotary navigation with dark/light/high-contrast themes, text scales and
Reduce Motion. For themed_app_icons, confirm Clock’s analog clock, Timer’s
stopwatch and Settings’ gear in the launcher and switcher after changing themes
and text scales. Record absent optional hardware explicitly for human review.

For secure editing and reloads, scripts/m7/probe is a separate manual SDK app.
It records only secure-editor completion, never the submitted value. Build it
with go build -o round-rui-acceptance ./scripts/m7/probe. Use the isolated manual
session setup in ../rui-m4/START-HERE.md, substituting this binary, manifest ID
org.example.rui.acceptance and scripts/m7/probe/layout.rui.yaml. Use the exact
candidate shell/Clock/Timer/roundui binaries, a separate same-user state/apps
folder and a private control directory. Stop the managed shell for this brief
session and restore it when the manual shell exits; do not enable development
flags in systemd or change production manifests.

Increment several times, change the YAML title and observe the count retained.
Test invalid YAML and an unregistered action: the last-good display and handlers
must remain usable. Hide/disable/remove/reorder the focused increment control
and check nearby focus or Back. Exercise held touch/picker/rotary release while
changing layout; cancellation must not commit a stale action. During public and
secure editing, change the layout: reload must defer until the editor result is
processed. Use disposable test text, verify masking/clearing, and confirm that
the secure value never appears in output or the reloaded UI.

Measure at least three stable saves through roundui dev from desktop to Pi.
A 60-fps-or-faster recording must show the save gesture and resulting Pi frame;
record method="save-gesture-to-visible-frame", integer frames and actual fps in
reload_measurements. The report conservatively includes one extra frame of
uncertainty. This starts before stable-save completion and is stricter than
that budget. A control ACK or compilation duration alone is insufficient.

## Desktop restoration and actual 0.5.0 rollback

After candidate performance capture, test a managed restart and verify settings,
Timer and Wi-Fi. Then run sudo /usr/local/sbin/roundosctl disable from SSH and
observe the restored desktop. Before restoring 0.5.0, verify its release hashes
and the saved control hashes again. Stop the manual shell first if one exists.

```sh
sudo sh -c 'cd /opt/roundos/releases/0.5.0 && sha256sum -c SHA256SUMS' && \
sudo sh -c 'cd /var/lib/roundos/m7-recovery && sha256sum -c SHA256SUMS' && \
sudo install -m 0644 /var/lib/roundos/m7-recovery/roundos.service /etc/systemd/system/roundos.service && \
sudo install -m 0755 /var/lib/roundos/m7-recovery/roundosctl /usr/local/sbin/roundosctl && \
sudo ln -s /opt/roundos/releases/0.5.0 /opt/roundos/.m7-current && \
sudo mv -Tf /opt/roundos/.m7-current /opt/roundos/current && \
sudo ln -s "/opt/roundos/releases/$CANDIDATE_VERSION" /opt/roundos/.m7-previous && \
sudo mv -Tf /opt/roundos/.m7-previous /opt/roundos/previous && \
sudo systemctl daemon-reload && \
sudo /usr/local/sbin/roundosctl verify && \
sudo /usr/local/sbin/roundosctl activate
```

Observe the actual old launcher and verify preferences, Timer and Wi-Fi remain
usable with the unchanged data formats. Record a read-only identity with
python3 scripts/m7/identity.py --expect-version 0.5.0 (sudo for process identity).
Do not replace these observations with a simulated rollback test. To return to
the candidate after acceptance, disable 0.5.0, restore the verified candidate's
unit/control script and release link, daemon-reload, verify and activate; keep
0.5.0 as previous. Reusing the installer would overwrite an existing release
and is intentionally refused.

## Compare and review the evidence

Run from the matching source workspace; use private output paths:

```sh
python3 scripts/m7/compare.py \
  --baseline-identity ~/roundos-m7-evidence/baseline/idle/before.json \
  --candidate-identity ~/roundos-m7-evidence/candidate/idle/before.json \
  --baseline ~/roundos-m7-evidence/baseline/*/session-bound.json \
  --candidate ~/roundos-m7-evidence/candidate/*/session-bound.json \
  --observations ~/roundos-m7-evidence/observations.json \
  --protocol ~/roundos-m7-evidence/protocol.json \
  --output ~/roundos-m7-evidence/report.json
```

Every matched run must meet input-to-present p95 ≤110% of baseline (interactive
scenarios), idle CPU ≤110%, and shell RSS ≤baseline+8 MiB. For multiple journal
summaries in a run, compare each run's largest p95. Traffic must drop at least
50%, with zero post-initialization full trees; each measured reload's upper
bound must be ≤500 ms. Input-to-present is a software/DRM metric, not electrical
touch-to-photon latency. Optional power/thermal observations require instruments.

READY_FOR_HUMAN_REVIEW requires every numeric gate and operator observation.
It never publishes a release or closes an issue. Attach reviewed results to #15,
complete its final evidence PR under normal review/CI rules, and only then use
the parent #28 stable-release workflow for the exact accepted 0.6.0 commit.
A code change after acceptance requires assessing and rerunning affected gates;
an old commit's attestation cannot certify new binaries.

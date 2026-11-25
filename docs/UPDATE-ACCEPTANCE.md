# Release-update acceptance record

Tracking: [update parent #25](https://github.com/drazafsky/roundos/issues/25),
[device acceptance #24](https://github.com/drazafsky/roundos/issues/24).
The RUI hardware/release work in
[#15](https://github.com/drazafsky/roundos/issues/15) is complete, and stable
[0.6.0](https://github.com/drazafsky/roundos/releases/tag/v0.6.0) is available.
The update parent's planned release is 0.7.0. The observations below validate
the update implementation in source commit
`3c096d52824997daa6ca2e92a9d3b2dc54df932b`; they do not attest a future 0.7.0
candidate or authorize its publication.

**Update-feature device acceptance is complete for this tested source.** The
[completed evidence](https://github.com/drazafsky/roundos/issues/24#issuecomment-7167904633)
combines fresh physical observations with actual systemd transactions and
journals. Host tests and CI alone do not establish display, touch, or physical
installation/recovery behavior.

Reference device: Raspberry Pi Zero 2 W / HyperPixel 480×480, 32-bit ARM,
Raspbian 13.6 (Trixie), glibc 2.41. The operator has no rotary encoder installed;
encoder-only observations are not applicable. The initial 0.5.0 Type=simple
installation was bootstrapped to the managed readiness/update service in
[the earlier acceptance work](https://github.com/drazafsky/roundos/issues/24#issuecomment-7111472703).
The original release remains retained for recovery.

On 2026-11-24, the operator chose **Install** in the real update prompt and
observed progress, restart, and the success notification. The independent helper
installed stable 0.6.0 from `0.6.0-dev.115.g3c096d5`; both builds identify the
source commit above. The operator confirmed preserved appearance/text size,
Clock label, and Timer state. The durable transaction and systemd journal agree
with that observation. See [the installation evidence](https://github.com/drazafsky/roundos/issues/24#issuecomment-7138841756).

## Candidate and automated evidence

The tested commit, embedded build identities, package hashes, toolchain, device,
release links and CI runs are recorded here. Settings and application data stayed
outside the immutable release directories throughout testing.

| Check | Evidence/status |
| --- | --- |
| Preferences, durable cooldown, stable discovery, scheduling | Host regression tests passed in their milestone PRs |
| Settings controls and countdown/error handling | Host tests and CI passed in PR #43 |
| Selected-asset staging and validation | Host tests and CI passed in PR #47 |
| Prompt, Ignore, notes navigation and scrolling | Host tests passed; delivery tracked in [PR #48](https://github.com/drazafsky/roundos/pull/48) |
| Managed transaction, rollback and boot recovery | Host fault-injection and installer-script tests passed; delivery tracked in [PR #48](https://github.com/drazafsky/roundos/pull/48) |
| Explicit Install worker, status and stale input handling | Worker and UI race tests passed; delivery tracked in [PR #48](https://github.com/drazafsky/roundos/pull/48) |
| Reviewed implementation and ARMv7 delivery | [PR #48](https://github.com/drazafsky/roundos/pull/48), [CI run 45235763423](https://github.com/drazafsky/roundos/actions/runs/45235763423), [verified initial package](https://github.com/drazafsky/roundos/issues/24#issuecomment-7110642356) |
| Tested source and stable ARMv7 target | Source `3c096d52824997daa6ca2e92a9d3b2dc54df932b`; [candidate CI 45327880472](https://github.com/drazafsky/roundos/actions/runs/45327880472); [stable publication 45435460738](https://github.com/drazafsky/roundos/actions/runs/45435460738) |

Stable target identity: GitHub release ID `457038535`, tag `v0.6.0`, built with
Go `1.27.4`. Verified GitHub asset digests and package manifests:

| Asset | SHA-256 |
| --- | --- |
| `roundos-0.6.0-linux-armv7.tar.gz` | `8bedd96e4726ebbb935defef8ef30878f24a33b57411c86032a05507412fc8f4` |
| `release.json` | `70b89a85c0a6ab9c6a2bcbb6d7cf4713e6c9bfb2136d237007cd25da949b1e02` |
| `SHA256SUMS` | `3d210a5b922c602a116000e0b3c45bc2054b38950dbc1a418a1e54de9707c665` |

All three executable and all three managed unit checksums passed on the Pi for
both the installed stable build and retained development predecessor. Before
further tests, the operator created a fresh root-private backup of state,
configuration, service units, release links, and available NetworkManager
profiles. Private contents are not attached to the public issue.

The earlier default/preset observations are retained as unchanged-component
evidence. The update package, Settings update controls, prompt implementation,
installation wiring, managed units, installer and recovery CLI have no source
diff between reviewed delivery `5c800e4af914c7ace3cda78d1077d8beda98d252` and
the tested source above. Fresh timing, prompt, failure, and recovery results
identify their actual build and transaction separately below.

## Physical scenarios

Results identify actual observations and transaction evidence for the pinned
implementation. A host preview or readiness signal alone was not counted as a
physical pass.

| Scenario | Expected observation | Result |
| --- | --- | --- |
| Startup, automatic On | Check occurs at startup, or waits for the shared cooldown/server deadline | Passed: attempt 10:38:29 CST; updated shell started 10:41:32; next eligible check logged 10:53:30 on 2026-11-24 |
| Interval changes | Every supported preset persists; 15-minute scheduling works; 24-hour selection shows the expected next check | Passed: all seven choices and persistence observed; real periodic check at 11:08:29 CST on 2026-11-24; fresh 24-hour selection displayed `NEXT CHECK IN 23H 57M` while manual checking retained `12M 30S` cooldown; saved interval verified as 1440 minutes |
| Automatic Off | Startup/periodic checks stop; manual checking remains usable | Passed in [earlier observations](https://github.com/drazafsky/roundos/issues/24#issuecomment-7112495115); repeated with persisted Off and ignored 0.6.0, then an explicit successful manual check |
| Manual check/cooldown | Repeated attempts stay blocked for 15 minutes, including after failure or shell restart | Manual cooldown expiration/reapplication observed earlier; installed-service restart retained the real startup deadline above |
| GitHub retry response | Longer server backoff is displayed and honored | Passed on the unchanged Pi executable using a controlled HTTP 429 response with `Retry-After: 1200`: operator observed the longer disabled countdown and its persistence across service restart |
| Available release, Cancel | Prompt closes; a later successful check can offer the release again | Operator confirmed dismissal to the launcher; repeated eligible check offered the same release again |
| Ignore/restart/manual check | Automatic prompts stay suppressed for that version; a newer version or manual result can prompt | Passed: explicit Ignore persisted as `0.6.0`; eligible automatic check after restart suppressed the prompt; operator confirmed Check now while Off offered the ignored release, Cancel returned normally, and immediate checking remained blocked |
| Release notes | Long notes scroll with touch and encoder; Close/Back returns to choices; reopening retains position | Passed: operator confirmed touch scrolling and retained position after Close/Back and reopening; encoder not installed |
| Ongoing interaction | No prompt interrupts text entry, Wi-Fi confirmation, overlays, or active touch/button sequences | Passed: editor, overlay, held-touch, and fresh Wi-Fi confirmation observations held delayed results until the interaction ended; Wi-Fi Cancel retained connectivity and the saved profile |
| Explicit Install | Selected identity stays pinned; progress is shown; RoundOS restarts and reports the installed version | Successful real installation observed; transaction `2ee984e48697640135a7d642acf904a0`, helper 10:41:01–10:41:36 CST. Fresh same-prompt retry retained its selected digest as recorded below |
| Preparation failure | Validation/network failure leaves the running release usable and offers a retry | Passed: fresh selected-digest failures left the development build running; changing future discovery metadata did not change the open prompt's selected digest |
| Replacement startup failure | Prior release and units are restored; rollback outcome is visible | Passed: controlled new-candidate startup failure produced automatic rollback, transaction `5fd12694df971018217a2f2df9e4cbb5`; operator confirmed normal display, touch, saved appearance/Clock/Timer and usable Wi-Fi |
| Interrupted transaction | Independent recovery restores/reconciles the recorded transaction and reports the outcome | Passed: real switching transaction `7b2d34af501fd1697f8ab70b664c23e2` survived helper SIGKILL; its independent restarted helper restored the previous release and recorded rollback; operator confirmed the usable UI and saved state |
| Preservation | Settings, app/Timer state, Wi-Fi profiles, and activation policy remain intact | Passed after final real installation: all saved-profile, device-config, app-data, appearance and unit hashes matched the fresh baseline; operator confirmed saved state, touch and Wi-Fi; activation policy was retained |
| Unmanaged/custom service | Checking and notes work; installation is disabled with a useful explanation | Passed with a temporary runtime service override: operator confirmed checking/notes worked, Install was disabled with an explanation, and tapping it created no installation transaction; override removed afterward |

Use controlled acceptance fixtures for failures and interruption, with retained
SSH access and verified prior releases. Do not relabel development archives or
publish test fixtures as stable releases. Device reboot is outside the Install
integration milestone; test service restart and the recovery entry point first.

The startup and periodic times above were observed using real elapsed time.
Subsequent repeated prompt tests explicitly expire the saved check deadlines
while the shell is stopped, retaining preferences and the Ignore choice. These
accelerated fixtures do not count as additional wall-clock timing evidence.

The rate-limit and selected-digest cases used a temporary loopback HTTPS response
fixture restricted to the repository's latest-release endpoint. Its ordinary
response contained real stable release metadata; fault modes returned an HTTP
429 retry deadline or deliberately changed a selected asset digest. The
certificate applied only to the test shell process through its environment;
the system trust store was unchanged. These controlled responses do not claim
that GitHub actually rate-limited the device or changed the published assets.

On 2026-11-25, the selected-digest fixture first failed with transaction
`068f89351ec1094a8ec50b88e9b17ece` and detail `package checksum mismatch:
SHA256SUMS`. A later new check selected the corrected real digests and installed
successfully (`bd86669a67fb6c774fa42ec00d4cc19d`); that was a fresh selection,
not the same-prompt pinning test.

The guided pinning test then kept the same prompt open. Transactions
`ddfbee7e0e2f03502b4a7149af49925a` and
`641d97aa6edf0f8fbded1f83e8458df4` both failed on the deliberately selected
all-zero SHA256SUMS digest. Between those attempts, the fixture's future
discovery response was corrected. The persisted check timestamp remained
`2026-11-25T09:14:02.643050682-06:00`, proving no new check replaced the
selection. The operator observed `PREPARATION FAILED - TRY AGAIN` after each
Install, and the old running release remained usable.

The startup-failure test used the real installer and systemd service. At
15:24:35 UTC on 2026-11-25, the helper was paused after the validated new release
became current and its installer child finished, with durable phase `switching`.
Execute permission was removed from only the new candidate's daemon and the
helper resumed. Systemd recorded `203/EXEC` / `Permission denied` at 15:24:40 UTC.
The unmodified helper restored the previous release, controller and unit bytes,
recorded `rolled_back`, and retained the failed candidate in its quarantine.
The restored development build passed all three executable and all three unit
checksums. The operator confirmed the usable UI and preserved saved state.

The interruption test reached its separate switching checkpoint at
15:37:46 UTC, with the valid new candidate current and the installer child
finished. The helper was killed with SIGKILL. Systemd recorded `status=9/KILL`,
started its replacement at 15:37:49, and completed recovery at 15:37:56.
The restarted, unmodified helper read the durable transaction and restored the
development current/controller and backed-up units. The valid interrupted
candidate was quarantined with its executable mode intact (0755). All six
restored checksums passed, the independent watcher reported successful recovery,
and the operator confirmed the physical recovery and preserved saved state.

Rapid accelerated restarts initially reached the unchanged managed service's
five-start limit. The checklist stopped, retained its recorded observations,
and restored the original environment. Clearing the failed start
counter restored the shell. The corrected test controls reset counters before
deliberate accelerated restarts; the production service policy was not changed.

The detailed interaction results are recorded in
[the follow-up evidence](https://github.com/drazafsky/roundos/issues/24#issuecomment-7140752974).
Early confirmations without a corresponding new check/install request were
excluded. Fresh held-touch and validation-failure observations with matching
requests and outcomes replaced those unexecuted entries.

## Preserved data and corrected observations

At 18:37 UTC on 2026-11-24, the verified original stable installation was
restored to `/opt/roundos/releases/0.6.0`, with current and controller links
pointing there and the development build retained as previous. This cleanup was
a managed file/link restoration, not another in-app installation result.
Automatic checking was restored to On / 15 minutes and the test Ignore choice
removed. The original device environment was restored, the response fixture
stopped, and the temporary runtime override removed. RoundOS was active/enabled
with zero unexpected restarts; LightDM remained inactive/disabled and Wi-Fi
connected.

The executable/unit checksums and appearance, application-data, and device
configuration hashes matched the baseline. The saved Wi-Fi profile inventory
changed from zero files to one during the session, requiring the audit below.
The profile was retained; no network or app-state backup was restored to force
a match with the initial empty inventory.
The subsequent metadata/journal audit found a NetworkManager device disconnect
at 17:48:31 UTC on 2026-11-24, followed by successful connection-add and
connection-activate operations at 17:49:05 UTC. The saved profile's timestamps
match those operations; it is root-owned, mode 0600, and belongs to the active
connection. Its aggregate hash remained unchanged in subsequent snapshots.
No installer was active during that interval. No previously saved profile was
lost: the initial saved-profile inventory was empty.

The disconnect preceded the controlled release response at 17:48:38 UTC, so the
earlier Wi-Fi confirmation result was withdrawn as evidence of a held
confirmation followed by Cancel. The added profile was retained in the fresh
2026-11-25 baseline for the installation/recovery tests and the corrected
physical observation below.

The corrected Wi-Fi retest ran on 2026-11-25. Its new check began at
15:54:19 UTC and the controlled successful response arrived at 15:54:39, after
the 20-second delay. The operator held the confirmation open, then used its
left Cancel button and observed the queued update prompt. Cancelling that prompt
returned normally. The same shell/check identity persisted throughout; the
NetworkManager journal recorded no disconnect transition or mutating operation,
Wi-Fi remained connected, and the saved-profile aggregate hash matched the fresh
baseline before and after Cancel. This new observation supersedes the earlier
confirmation result.

The final real GitHub installation completed as transaction
`d27a994245fcd548e819493cd939a572`, phase `succeeded`. The response fixture was
removed before discovery and the operator confirmed progress, restart, success
notification, usable touch/Wi-Fi, and correct saved appearance/text size, Clock
label and Timer state. Current and controller point to verified stable 0.6.0;
the development predecessor remains retained. All six executable/unit checksums
passed. Saved-profile, device-configuration, application-data, appearance and
unit hashes matched the fresh 2026-11-25 baseline, including the retained added
profile. RoundOS remained enabled, LightDM inactive, and Wi-Fi connected. The
original device environment was restored and the fixture remained stopped.

## Final verified state

At 16:21:35 UTC on 2026-11-25, the final audit verified stable 0.6.0 as both
current and controller, with the tested development build retained as previous.
The original Automatic On / 15-minute preference and Ignore state were restored
after verifying the operator's 24-hour selection. The real up-to-date check at
16:18:18 UTC had stored attempt `2026-11-25T10:18:17.966138819-06:00`; that exact
timestamp remained unchanged through preference restoration and service restart,
preserving the shared cooldown.

All executable/unit checksums and saved-profile, device-configuration,
application-data, appearance and unit hashes matched the fresh baseline.
RoundOS was active/enabled, LightDM inactive, and Wi-Fi connected. The original
device environment was restored, the runtime override removed and the response
fixture stopped. Verified prior releases, failed-candidate quarantines, private
backups and timestamped evidence were retained for recovery and diagnosis.

## Completion

All listed applicable scenarios have physical observations and supporting
evidence in [#24](https://github.com/drazafsky/roundos/issues/24). Encoder-only
checks are not applicable to the installed hardware. This completes update
feature acceptance for the recorded source. Future release-candidate pinning and
publication remain governed by [CI and releases](CI-RELEASES.md).

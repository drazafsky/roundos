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

**Physical acceptance remains in progress.** Completed observations and remaining
cases are distinguished below. Host tests and CI do not establish display,
touch, or actual installation/recovery behavior on the Pi.

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

Record the candidate commit, embedded version/build ID, verified package hashes,
Go/toolchain version, device model/OS, managed current/previous/controller links,
and the CI run URLs before device testing. Keep settings and application data
outside the immutable release directory.

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

## Physical scenarios

Record observations and logs for each scenario against the same pinned candidate.
Do not infer a pass from a host preview or software readiness signal.

| Scenario | Expected observation | Result |
| --- | --- | --- |
| Startup, automatic On | Check occurs at startup, or waits for the shared cooldown/server deadline | Passed: attempt 10:38:29 CST; updated shell started 10:41:32; next eligible check logged 10:53:30 on 2026-11-24 |
| Interval changes | Every supported preset persists; 15-minute scheduling works; 24-hour selection shows the expected next check | All seven choices and restart persistence observed earlier; real next periodic check at 11:08:29 CST confirmed 15-minute scheduling; 24-hour next-check display pending |
| Automatic Off | Startup/periodic checks stop; manual checking remains usable | Passed in [earlier observations](https://github.com/drazafsky/roundos/issues/24#issuecomment-7112495115); repeated with persisted Off and ignored 0.6.0, then an explicit successful manual check |
| Manual check/cooldown | Repeated attempts stay blocked for 15 minutes, including after failure or shell restart | Manual cooldown expiration/reapplication observed earlier; installed-service restart retained the real startup deadline above |
| GitHub retry response | Longer server backoff is displayed and honored | Passed on the unchanged Pi executable using a controlled HTTP 429 response with `Retry-After: 1200`: operator observed the longer disabled countdown and its persistence across service restart |
| Available release, Cancel | Prompt closes; a later successful check can offer the release again | Operator confirmed dismissal to the launcher; repeated eligible check offered the same release again |
| Ignore/restart/manual check | Automatic prompts stay suppressed for that version; a newer version or manual result can prompt | Passed: explicit Ignore persisted as `0.6.0`; eligible automatic check after restart suppressed the prompt; operator confirmed Check now while Off offered the ignored release, Cancel returned normally, and immediate checking remained blocked |
| Release notes | Long notes scroll with touch and encoder; Close/Back returns to choices; reopening retains position | Passed: operator confirmed touch scrolling and retained position after Close/Back and reopening; encoder not installed |
| Ongoing interaction | No prompt interrupts text entry, Wi-Fi confirmation, overlays, or active touch/button sequences | Editor, Wi-Fi confirmation, and overlay passed with delayed responses and queued prompts appearing after dismissal; held-touch check pending |
| Explicit Install | Selected identity stays pinned; progress is shown; RoundOS restarts and reports the installed version | Successful real installation observed; transaction `2ee984e48697640135a7d642acf904a0`, helper 10:41:01–10:41:36 CST; selected-digest failure fixture pending |
| Preparation failure | Validation/network failure leaves the running release usable and offers a retry | Pending |
| Replacement startup failure | Prior release and units are restored; rollback outcome is visible | Pending |
| Interrupted transaction | Independent recovery restores/reconciles the recorded transaction and reports the outcome | Pending |
| Preservation | Settings, app/Timer state, Wi-Fi profiles, and activation policy remain intact | Pending |
| Unmanaged/custom service | Checking and notes work; installation is disabled with a useful explanation | Passed with a temporary runtime service override: operator confirmed checking/notes worked, Install was disabled with an explanation, and tapping it created no installation transaction; override removed afterward |

Use controlled acceptance fixtures for failures and interruption, with retained
SSH access and verified prior releases. Do not relabel development archives or
publish test fixtures as stable releases. Device reboot is outside the Install
integration milestone; test service restart and the recovery entry point first.

The startup and periodic times above were observed using real elapsed time.
Subsequent repeated prompt tests explicitly expire the saved check deadlines
while the shell is stopped, retaining preferences and the Ignore choice. These
accelerated fixtures do not count as additional wall-clock timing evidence.

The rate-limit case used a temporary loopback HTTPS response fixture restricted
to the repository's latest-release endpoint. A selected-digest failure mode is
prepared but its installation test remains pending. The fixture's ordinary
response contains the real stable release metadata; its fault modes return an
HTTP 429 retry deadline or deliberately alter a selected asset digest. The
certificate applies only to the test shell process through its environment;
the system trust store is unchanged. These controlled responses are not claims
that GitHub actually rate-limited the device or changed the published assets.

Rapid accelerated restarts initially reached the unchanged managed service's
five-start limit. The checklist stopped, retained its five completed physical
observations, and restored the original environment. Clearing the failed start
counter restored the shell. The corrected test controls reset counters before
deliberate accelerated restarts; the production service policy was not changed.

The detailed interaction results are recorded in
[the follow-up evidence](https://github.com/drazafsky/roundos/issues/24#issuecomment-7140752974).
Confirmations without a corresponding new check/install request were rejected;
they do not establish held-touch or validation-failure acceptance.

## State after the recorded tests

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
changed from zero files to one during the session. Its origin has not yet been
established, so identical network-profile hashes are not claimed. The profile
was retained; no network or app-state backup was restored to force a match.
Profile provenance and the remaining physical cases above still require evidence.

## Completion

Attach physical observations, relevant journal excerpts, package verification,
and any failures to #24. Keep #24 open until these scenarios have actual evidence.
Follow the stable-release acceptance format in CI-RELEASES.md only when the
matching candidate is ready and the maintainer has accepted its Pi results.
This record does not authorize stable publication.

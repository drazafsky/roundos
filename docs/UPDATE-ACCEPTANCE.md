# Release-update acceptance record

Tracking: [update parent #25](https://github.com/drazafsky/roundos/issues/25),
[device acceptance #24](https://github.com/drazafsky/roundos/issues/24).
Coordinate with [RUI acceptance #15](https://github.com/drazafsky/roundos/issues/15).
The planned update release is 0.7.0, after the 0.6.0 RUI release. Pin and test the
actual candidate commit before recording a maintainer acceptance attestation.

**Physical acceptance is pending.** Host tests and CI do not establish display,
touch, encoder, Wi-Fi, or actual installation/recovery behavior on the Pi.
The initial read-only SSH inspection found an active ARMv7 `pocketwatch` device
running RoundOS 0.5.0 on Raspbian 13.6 (Trixie) with a Type=simple service.
No update controller is installed. SSH works; privileged acceptance commands
require interactive sudo access. This is baseline information,
not acceptance of the update candidate. No update deployment is recorded here.

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
| Complete candidate verification and ARMv7 package | Pending final candidate |

Update pending entries with exact commit/run evidence when the final candidate
has completed the repository verification suite. Use the repository CI verifier
and ARMv7 package verifier described in [CI and releases](CI-RELEASES.md).

## Physical scenarios

Record observations and logs for each scenario against the same pinned candidate.
Do not infer a pass from a host preview or software readiness signal.

| Scenario | Expected observation | Result |
| --- | --- | --- |
| Startup, automatic On | Check occurs at startup, or waits for the shared cooldown/server deadline | Pending |
| Interval changes | Every supported preset persists; 15-minute scheduling works; 24-hour selection shows the expected next check | Pending |
| Automatic Off | Startup/periodic checks stop; manual checking remains usable | Pending |
| Manual check/cooldown | Repeated attempts stay blocked for 15 minutes, including after failure or shell restart | Pending |
| GitHub retry response | Longer server backoff is displayed and honored | Pending |
| Available release, Cancel | Prompt closes; a later successful check can offer the release again | Pending |
| Ignore/restart/manual check | Automatic prompts stay suppressed for that version; a newer version or manual result can prompt | Pending |
| Release notes | Long notes scroll with touch and encoder; Close/Back returns to choices; reopening retains position | Pending |
| Ongoing interaction | No prompt interrupts text entry, Wi-Fi confirmation, overlays, or active touch/button sequences | Pending |
| Explicit Install | Selected identity stays pinned; progress is shown; RoundOS restarts and reports the installed version | Pending |
| Preparation failure | Validation/network failure leaves the running release usable and offers a retry | Pending |
| Replacement startup failure | Prior release and units are restored; rollback outcome is visible | Pending |
| Interrupted transaction | Independent recovery restores/reconciles the recorded transaction and reports the outcome | Pending |
| Preservation | Settings, app/Timer state, Wi-Fi profiles, and activation policy remain intact | Pending |
| Unmanaged/custom service | Checking and notes work; installation is disabled with a useful explanation | Pending |

Use controlled acceptance fixtures for failures and interruption, with retained
SSH access and verified prior releases. Do not relabel development archives or
publish test fixtures as stable releases. Device reboot is outside the Install
integration milestone; test service restart and the recovery entry point first.

## Completion

Attach physical observations, relevant journal excerpts, package verification,
and any failures to #24. Keep #24 open until these scenarios have actual evidence.
Follow the stable-release acceptance format in CI-RELEASES.md only when the
matching candidate is ready and the maintainer has accepted its Pi results.
This record does not authorize stable publication.

# RoundOS 0.7.0 candidate

Release parent: [#25](https://github.com/drazafsky/roundos/issues/25).
Candidate preparation: [#66](https://github.com/drazafsky/roundos/issues/66).
Exact-candidate Pi acceptance: [#65](https://github.com/drazafsky/roundos/issues/65).

**Physical acceptance is pending for this candidate.** The completed update
feature record in [#24](https://github.com/drazafsky/roundos/issues/24) applies
to its recorded 0.6.0 source. Preserve that evidence; do not copy its passing
attestation or observations to this candidate.

## Contents and release order

The candidate includes Settings update preferences and scheduling, release
prompts and scrollable notes, verified packages, managed installation and
recovery. Clock and Timer use embedded RUI layouts. Current main also includes
the launcher work from [#38](https://github.com/drazafsky/roundos/issues/38):
app icons and names, sliding focus, configurable wrapping, required RUI icon
declarations, and the local time/date header. Exercise those changes during
this candidate's physical smoke tests as well.

The release plan retains 0.7.0 before 0.8.0. After 0.7.0 publication, prepare
the separate 0.8.0 candidate under
[#67](https://github.com/drazafsky/roundos/issues/67); its physical acceptance
remains [#44](https://github.com/drazafsky/roundos/issues/44). An earlier build's
acceptance does not certify that later candidate.

## Select and verify the candidate

After merging candidate preparation, record the full main commit whose
`VERSION` is `0.7.0`. Wait for that commit's **CI and prerelease** push run,
including Tests, Host DRM build and ARMv7 build and package, to pass.
Record the exact package version, build ID, CI run, and archive/metadata hashes
in #65. A development package carries its full `0.7.0-dev.RUN.gCOMMIT` identity;
record that installed identity separately from the target stable version.

Verify outer `SHA256SUMS`, release.json identity, extracted file checksums, and
`roundd -version` before installing. Use the
[package instructions](CI-RELEASES.md#installing-on-a-32-bit-pi) and
[SSH recovery guide](SYSTEMD-ROLLBACK.md). Retain the verified prior release
and a fresh private backup of configuration, application state and service
configuration. No physical result is established by those automated checks.

## Guided device acceptance

Record actual operator observations and corresponding identity/journal evidence
for the same installed candidate:

- Display and touch, local time/date, every installed app's icon/name, focus
  slides, wrapping on/off, reduced motion, and touch-only access to Appearance
  rows. Record rotary checks as not applicable if no encoder is installed.
- Clock/Timer operation, text editing, background Timer and notifications,
  preserved appearance/application state and connected Wi-Fi.
- Settings update defaults/preferences, manual checking, shared cooldown,
  automatic scheduling, notes, Cancel/Ignore and prompt deferral as applicable.
- Managed installation/restart and boot; preservation of activation policy,
  settings and application/network configuration. Record the actual transaction
  and verify the installed release/controller links and checksums.
- Applicable failure/interruption and rollback/recovery checks, with fresh
  transaction IDs, actual service outcomes and operator observations. Existing
  backups and verified recovery releases must remain available throughout.

Record unexecuted cases as pending. Never infer physical results from SSH,
automated tests, a previous build, or an earlier installation transaction.

After acceptance passes, record the maintainer attestation for target `0.7.0`
and its exact source commit using the
[release declaration and acceptance formats](CI-RELEASES.md#automatic-stable-releases-from-parent-issues).
The evidence must state the actual installed build identity. Close #65 as
completed only after the recorded evidence and attestation agree. The stable
workflow rechecks all gates and builds that exact commit for publication.

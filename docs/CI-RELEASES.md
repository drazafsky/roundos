# Automated checks and development releases

Every PR targeting `main` and every push to `main` runs the **CI and prerelease**
workflow. It checks Go tests, the race detector, DRM-tagged tests, vet, all three
Python suites, shell syntax, and RUI validation, compilation, previews and bundle
determinism. Separate jobs build the host DRM backend and the ARMv7 package.
RUI checks also recompile the M3 demo's embedded bundle byte-for-byte, preview
it, and build the optional demo for the host and ARMv7. The example stays in
the source tree and is not installed or included as a managed release app.
The ARM build runs in emulated 32-bit Debian Trixie with its native compiler and
libdrm headers; Go is pinned in `.go-version` for both architectures. When
updating Go, also update its official ARM archive checksum in the Dockerfile.

The original M0/M1/M2 provenance tools describe an immutable 0.5.0 archive.
Their checksum manifest remains unchanged. Current CI tests the verifier with
isolated fixtures and runs current source checks independently; it does not
claim that today's source is byte identical to the historical release.

## Releases after merging

Only a push to `main` can publish. All three validation/build jobs must pass
for that exact commit first. Every successful merge, including documentation
changes, gets a prerelease such as `v0.5.1-dev.42.gabcdef0`. `VERSION` holds the
next stable base version; the workflow run number and commit identify the build.
Reruns keep the same version, and separate merges are never cancelled together.

The release contains an installable `linux-armv7` tarball, `release.json`, and
`SHA256SUMS`. The tarball also has checksums for its files. A draft is created,
uploaded assets are downloaded and verified, and only then is the release
published. Existing published releases are verified without overwriting them;
a rerun with different bytes fails rather than replacing a published asset.
Only publishing receives a write token. No personal access token is needed.

The package's `RELEASE_VERSION`, `roundd -version`, and installer directory agree.
The package includes `roundd`, Clock, Timer, Demo, `roundui`, installer helpers,
configuration, layouts, schemas, service definition, and documentation.

## Installing on a 32-bit Pi

Download all three release assets into an empty directory, then:

```sh
sha256sum -c SHA256SUMS
tar -xzf roundos-*-linux-armv7.tar.gz
cd roundos-*-linux-armv7
sha256sum -c SHA256SUMS
./roundd -version
sudo ./scripts/install-release.sh
sudo roundosctl verify
```

The installer preserves its service, ownership, symlink, and overwrite checks.
It does not activate RoundOS or switch the desktop. Follow the managed release
and systemd rollback documentation for deliberate activation over SSH.

For a native source build, install Go, a C compiler, pkg-config and libdrm-dev,
then run `sh scripts/build.sh`. This produces a `0.5.1-dev` identity by default.
Automation sets `ROUNDOS_VERSION`, `ROUNDOS_BUILD_ID`, and `ROUNDOS_OUTPUT_DIR`.

## Merge protection and hardware acceptance

The `main` ruleset requires **Tests**, **Host DRM build**, and **ARMv7 build and
package**, an up-to-date branch, a PR, and resolved review conversations.
Copilot review completion, open-issue references, assignment, and review feedback
resolution follow `AGENTS.md`. GitHub excludes its internal Copilot check from
required status checks, so verify the submitted Copilot review before merging.
Squash merging and automatic branch deletion are enabled in repository settings.

CI never marks physical display, touch, Wi-Fi, managed boot, recovery or
performance acceptance as passed. These remain manual checks on the Pi.
Prereleases are not marked latest. Stable publication is a separate automated
workflow after recorded hardware validation, described below. Neither workflow
installs or activates anything on a device.

## Automatic stable releases from parent issues

The **Stable release** workflow publishes one stable version when a designated
release parent meets all release conditions. Ordinary feature parents, including
the Settings update parent, do not enable publication by themselves. Per-merge
development prereleases continue as before. The Settings updater discovers only
newer compatible stable releases and still requires the user to choose Install.

Choose a release commit that includes the implementation and whose `VERSION`
file contains the intended stable version. Complete its physical Pi testing.
The workflow builds this exact commit with the stable version embedded; it does
not relabel an existing development archive or automatically select a newer main.

To enable a release, a repository maintainer creates the parent, attaches its
milestones as native GitHub sub-issues, adds the `release:stable` label, and puts
one declaration in the parent description:

````markdown
```roundos-release
{
  "version": "0.6.0",
  "commit": "FULL_40_CHARACTER_LOWERCASE_COMMIT_SHA",
  "acceptance_issue": 123
}
```
````

Replace the example SHA and issue number with actual values. The acceptance
issue must be a descendant of that parent. Every descendant, including nested
grouping issues and the acceptance issue, must be closed as **completed**;
closing as **not planned** does not qualify. The release parent itself can
remain open while the last milestone finishes; closing it as not planned blocks
publication.

Every implementation leaf needs a GitHub closing PR merged into `main`, and
every merged closing PR must be included in the selected commit. Use closing keywords
in the PR description so GitHub records the relationship. For an issue devoted
only to manual verification, a maintainer can use `release:verification` instead
of a closing PR. Such issues must also be maintainer-authored. The designated
physical acceptance issue does not need a PR. Grouping issues can have children
instead of a closing PR.

A release parent can contain up to 45 descendants. The workflow also limits API
reads per reconciliation so large issue or PR histories cannot exhaust the
Actions token's quota. Discovery logs show when a limit prevents a release.

After testing, a maintainer with current repository write access records a
comment on the acceptance issue with this block:

````markdown
```roundos-acceptance
{
  "version": "0.6.0",
  "commit": "FULL_40_CHARACTER_LOWERCASE_COMMIT_SHA",
  "result": "passed",
  "evidence": "https://github.com/drazafsky/roundos/issues/123#issuecomment-456"
}
```
````

The evidence URL must point into this repository and identify the actual Pi
results: display/touch, Wi-Fi, boot, performance, installation and recovery as
applicable to that release. The comment is an operator attestation, not a result
inferred by CI. Version and commit must match the parent exactly. Reopen the
acceptance issue or remove `release:stable` to withdraw acceptance before
publication. To withdraw acceptance while keeping the issue closed and the
label in place, remove every passing attestation for that version and commit.

The selected commit must be on main, with a successful **CI and prerelease**
push run and successful Tests, Host DRM build, and ARMv7 build and package jobs.
The most recent run/attempt is checked; an earlier successful attempt cannot
override a pending or failed rerun. The target version must match the selected
source's `VERSION` and cannot be older than a published stable version.

This workflow uses the built-in `GITHUB_TOKEN` and requires no release secret.
GitHub restricts that token from creating or updating a release whose target's
workflow files differ from the default branch. Therefore the accepted commit's
`.github/workflows` contents must still match main. Later code or documentation
merges are allowed; a workflow change blocks the old candidate before building,
with a diagnostic asking for acceptance of a commit with current workflows.
The workflow never substitutes main for the accepted commit. See GitHub's
[release API permissions](https://docs.github.com/en/rest/releases/releases#create-a-release).

The workflow rechecks parents after issue changes, acceptance comment changes,
and CI completion. An hourly reconciliation covers events generated with
`GITHUB_TOKEN` and events coalesced by concurrency. **Run workflow** provides an
immediate reconciliation without bypassing any gates. Blocked parents and their
reasons appear in the discovery job log.

Eligible releases build in the existing emulated ARMv7 toolchain with a stable
version, full commit identity, startup-protocol check, and architecture smoke
checks. Stable packages carry `hardware_validated: true` only after the recorded
acceptance gate. The control scripts come from main; packaged source and docs
come from the accepted commit. The existing asset layout remains a tarball,
`release.json`, and `SHA256SUMS`.

Immediately before building, before upload, and after upload verification, the
workflow rechecks the parent, implementation merges, CI, and acceptance. It
creates a draft, uploads and downloads the assets to verify checksums, then
publishes with prerelease disabled and marks the stable release latest. Release
notes link the parent and acceptance record and include generated change notes.

Stable runs serialize, and a published version is never overwritten. Repeated
events verify the existing tag and expected asset names before skipping a
published version. A failed upload leaves a resumable
draft; rerun the workflow after correcting the failure. A conflicting tag,
commit, release type, or published asset causes a failure rather than replacement.
Changing the parent during a build invalidates the run. After publication,
reopening issues cannot retract installed software; publish a new version for a
correction.

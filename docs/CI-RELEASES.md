# Automated checks and development releases

Every PR targeting `main` and every push to `main` runs the **CI and prerelease**
workflow. It checks Go tests, the race detector, DRM-tagged tests, vet, all three
Python suites, shell syntax, and RUI validation, compilation, previews and bundle
determinism. Separate jobs build the host DRM backend and the ARMv7 package.
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
Prereleases are not marked latest. Stable release promotion is a separate manual
process after hardware validation; this workflow does not install or activate
anything on a device.

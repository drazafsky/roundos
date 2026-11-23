# RoundOS RUI Milestone M2 — offline developer toolchain

**Status:** implementation complete; host checks and Pi build verification must be performed separately. This package **does not change** the 0.5.0 shell, installed apps, IPC v7 or systemd release. The earlier M0 physical reboot/rollback and performance gate remains open.

## Quick start from the repository root

```sh
# On the Pi, offline; does not affect the running RoundOS 0.5.0 service.
./scripts/m2/verify.sh
# Or install only the CLI into a local user-owned directory:
mkdir -p "$HOME/bin"
go build -o "$HOME/bin/roundui" ./cmd/roundui

# Authoring checks: exit nonzero, print source file:line:column on errors
"$HOME/bin/roundui" validate layouts/timer.rui.yaml

# Deterministic and embeddable layout artifact + typed ID constants
"$HOME/bin/roundui" compile -o /tmp/timer.rui.bin \
  -go /tmp/timer.ids.go -package main layouts/timer.rui.yaml

# Inspect contents and type contracts; --json includes source map
"$HOME/bin/roundui" inspect --json /tmp/timer.rui.bin

# Draw through the actual RoundOS 0.5.0 circular renderer, no DRM access
"$HOME/bin/roundui" preview -o /tmp/timer.png \
  -values layouts/timer.sample.json /tmp/timer.rui.bin
"$HOME/bin/roundui" preview -o /tmp/clock-options.png \
  -screen clock.options -theme light \
  -values layouts/clock.sample.json layouts/clock.rui.yaml
```

## Artifact contract

`roundos.rui.bundle/v1` is a deterministic, unsigned file, suitable for embedding via `//go:embed` in Go applications. It is NOT executable and not a signature, installation permission, or trust proof. Byte layout:

```
4 bytes  ASCII magic "RUI1"
4 bytes  unsigned big-endian canonical JSON length (1..524288)
N bytes  JSON encoding of the bundle File payload
32 bytes SHA-256 digest of the preceding N JSON bytes
```

The payload includes the full typed AST, semantic manifest of required bindings and actions, source SHA-256, and stable source-position map. No timestamp or machine-specific path is embedded. Encoding uses Go `encoding/json` (deterministic map-key ordering). `bundle.Decode` rejects damaged headers, trailing data, size overflow, unsupported versions and corrupted hashes. The CLI `compile.Verify` also recomputes the semantic manifest and source map. An independent application may import `roundos/sdk/rui/bundle` and `roundos/sdk/rui/spec` without importing internal toolchain packages; M3 will add safe runtime loading and **application identity authorization**.

The decoder's checksum protects against accidental corruption, **not** a malicious party able to replace the artifact and its digest. Install artifacts only from trusted, immutable application releases, and do not treat layout YAML as privileged code.

## Preview semantics

`roundui preview` maps all nine M1 widgets to the matching v7 `wire.Widget` kinds. Semantic group headings become stable synthetic, nonfocusable `label` widgets; navigation/actions/step/range/checked/disabled properties retain their v7 meanings. It validates `wire.View` and measures a real JSON `wire.Message{version:7,type:"view"}` frame before rasterizing. Oversized (64 KiB) frames and sample integers outside widget ranges fail.

Sample JSON is a flat dictionary of manifest binding name to a **properly typed** value; undeclared keys are errors. Missing keys show `[binding]` for strings, lower-bound integers for numeric widgets, false for toggles, and true for `enabledWhen`/`visibleWhen` to expose controls. Missing sample keys are reported. No sample data is used in production or executes handlers.

**M2 limitation:** the existing 0.5.0 shell only distinguishes screens with/without a hero. `list`, `form` and `detail` all use the same shell-managed row geometry during offline preview. This is explicitly reported, rather than silently implying that RUI-specific template spacing has been implemented. M5 will implement distinct template semantics where necessary. M2 preview is a safe static snapshot: no interactive events, text editor or dialog activation, and no hot reload. M3 will connect the runtime and M4 will add development hot reload.

## Preservation and compatibility

- Original 149 RoundOS 0.5.0 files are byte-identical to the baseline; all runtime changes are deferred.
- The existing `sdk/ui`, IPC v7, root-owned managed releases, app supervision and DRM rendering remain untouched.
- RUI `application.id` must be checked against the authenticated/registered app at M3 publication; compilation alone is not authorization.
- Static validation checks up to 16 screens and 64 effective widgets including headings; a *future* runtime must also guard changing bound strings and the actual v7 64 KiB frame size.
- The built-in release verifier and safe preview do not access `/dev/dri`, install binaries, restart services or modify LightDM.

## M2 acceptance

1. Validate all M1 positive fixtures and reject all invalid golden fixtures with positions.
2. Compile the same YAML twice; output bundles match byte-for-byte.
3. Reject damaged, truncated, oversized, trailing and changed-format bundles.
4. `inspect` recovers all screens, action/binding types, source map, source SHA.
5. Render Clock, Timer and Gallery at 480 × 480 on the real shell draw path with all three themes.
6. Confirm generated Go constants compile, carry original names, and distinguish punctuation collisions.
7. M0 provenance, full Go regression, `go vet`, Python regression and ARMv7 CLI build pass on host.
8. Pi may separately run `./scripts/m2/verify.sh` (offline), and no service changes are required.

M2 does not close the outstanding M0 physical performance, reboot or rollback gates.

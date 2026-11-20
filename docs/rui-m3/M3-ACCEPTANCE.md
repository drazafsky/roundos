# M3 acceptance evidence

Source: `roundos-rui-m3-patch.zip`, targeting the M2 checkpoint. Integrated on current `main` after CI/release automation and startup recovery. M2 language, validator, bundle and preview packages and the current shell/service implementation are preserved. Verification scripts and documentation are adapted to current main; the original 0.5.0 checksum manifest remains immutable historical evidence.

Host verification: `./scripts/m3/verify.sh` (Go unit/integration tests, Python provenance fixtures and regressions, `go vet`, RUI previews, deterministic demo bundle comparison, ARMv7 cross-builds). Main CI runs the same demo bundle/build checks. An independent live IPC test establishes an authenticated v7 handshake and round-trip view/input/value publication with `sdk/app.Client` and no real shell device access.

Additional host: `go test -race ./... -count=1` on supported host. On physical 32-bit Pi, `-race` is unsupported and should be skipped rather than failed.

Pending on physical Pi: native optional demo end-to-end interaction and the previously outstanding M0 lifecycle/performance gates. Neither is implicitly accepted by the host suite.

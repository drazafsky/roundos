# M1 host validation — November 19, 2026

**Status: PASS for M1 language, schema and validation implementation.**

- RoundOS 0.5.0 provenance: **PASS**, all 149 original files unchanged.
- `scripts/m1/verify.sh`: **PASS** — provenance, valid/invalid fixture tests, JSON schema syntax, Go vet of new packages.
- `go test ./... -count=1`: **PASS** — existing RoundOS and new M1 tests.
- `go test -race ./... -count=1`: **PASS** — supported amd64 host (the Pi's 32-bit ARM race detector remains unsupported).
- `go vet ./...`: **PASS**.
- M0 Python unit tests: **PASS**, 12 tests, including the updated additive-source allowance.
- Linux ARMv7 non-DRM cross-build: **PASS** for roundd, round-clock, round-timer and round-demo.
- JSON Schema 2020-12 structural validation of the valid example YAML files: **PASS** on host using an optional developer-side schema validator. Does not require one on Pi.
- Native Pi DRM, touchscreen and managed-release service tests: **unchanged by M1 and not rerun here**. Existing M0 pending performance/reboot/rollback checks remain open.

No M1 runtime feature or live YAML interpretation is claimed. M2 will add the compiler and preview tooling. In M1, the parser and validator are tested as library functions only.

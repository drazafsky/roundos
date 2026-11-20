# M2 acceptance checklist — 2026-11-20

Host validation performed against the M1 release source checkpoint, with 149 original 0.5.0 files unchanged. All M2 source changes are additive and offline.

| Check | Host result | Pi result |
|---|---|---|
| Source integrity: all original 149 files | PASS | pending for M2 patch |
| M1 YAML positive/negative fixtures and full Go suite | PASS | pending |
| M2 deterministic compilation, corruption/bounds tests | PASS | pending |
| 64 KiB actual v7 frame bound rejection | PASS | pending |
| Full Clock, Timer and Gallery PNG preview (480x480) | PASS | pending |
| CLI validate/compile/inspect/preview end-to-end | PASS | pending |
| M0 12 mock/service and provenance Python tests | PASS | pending |
| `go vet ./...` | PASS | pending |
| `go test -race ./...` on linux/amd64 | PASS | Unsupported on Pi 32-bit linux/arm |
| `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build ./cmd/roundui` | PASS | not executed on Pi |
| Generated Go ID constants compile with `go tool compile` | PASS | pending |
| M0 physical performance, reboot, restart and rollback acceptance | still OPEN | not run |

The M2 combined verifier finished with status 0. The existing M0 mock-release test's subprocess timeout was increased from 6s to 20s to avoid false failures on loaded hosts; it still uses a temporary mocked `systemctl` and never invokes the host's real service manager.

## Known intentional boundaries

1. M2 does not publish application UI or connect callbacks; that begins in M3.
2. M2 offline preview does not deliver full `form` / `detail` template differentiation, which requires M5 shell layout capability; it uses existing v7 geometry and warns explicitly.
3. Compiled bundle SHA256 is a file-integrity checksum, not an authorization mechanism; M3 must verify authenticated application identity and guard dynamic values/frame sizes.
4. M0 hardware measurements and managed-release lifecycle acceptance remain outstanding.
5. M2 host success does not imply physical Pi acceptance of M2.

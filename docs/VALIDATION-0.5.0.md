# RoundOS 0.5.0 — host validation record

Host: Linux amd64, Go 1.23.2; generated from the RC4 source baseline after the user confirmed full RC4 hardware acceptance. All checks below were executed during preparation of this 0.5.0 source release.

| Check | Result |
|---|---|
| `go test ./... -count=1` | PASS |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| Linux ARMv7 `CGO_ENABLED=0` cross-compilation for roundd/Clock/Timer/Demo | PASS (non-DRM) |
| External Go module importing `sdk/app`, `sdk/ui` only | PASS |
| systemd service unit policy/startup guards | PASS (static test) |
| `sh -n scripts/*.sh` | PASS |
| Python performance-sampler unit tests | PASS |
| 1-second, five-record performance JSONL smoke test; output mode 0600 | PASS |
| `go run ./cmd/roundd -version` | `RoundOS 0.5.0 (source)` |
| `systemd-analyze verify systemd/roundos.service` | Unit syntax inspected; expected warning: `/opt/roundos/current/bin/roundd` absent on host |
| Native DRM build with libdrm headers on Pi | NOT RUN on host (libdrm headers unavailable) |
| Managed systemd boot/restart/desktop restore on Pi | PENDING hardware acceptance |
| Real Pi input-to-present p95 / idle CPU / thermal / battery results | PENDING physical measurements |

## Interpretation

RC4's touch, rotary, Wi-Fi, notifications, widgets, Timer crash recovery and privilege separation have previously passed physical tests per user report; the new 0.5 managed lifecycle and root-owned release layout are **not yet** physically accepted. No claim is made that host Go tests validate DRM/KMS or physical power consumption. This release never modifies the working overlay, screen kernel driver or OS boot splash.

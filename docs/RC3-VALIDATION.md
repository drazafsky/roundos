# RoundOS 0.5-rc3-wifi — off-device validation

Validation completed on Linux amd64 with Go 1.23.2, using the source imported from the exact `roundos-0.5-rc2-wifi.zip` archive.

| Check | Result |
|---|---|
| `go test ./... -count=1` | PASS |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| Linux `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0` cross-build | PASS for `roundd`, `round-clock`, `round-timer`, `round-demo` |
| Built-in Clock/Timer separate-process integration | PASS |
| Optional demo process IPC integration | PASS |
| Manifest validation tests (permissions, duplicate IDs, unsafe executable paths, missing directory) | PASS |
| Nested app widget navigation / installed app carousel | PASS |
| Touch picker Cancel/OK and no-action cancellation | PASS |
| Host PNG 480x480 launcher + picker previews | Rendered and manually inspected |
| NetworkManager / Wi-Fi unit tests carried forward | PASS |
| Native ARM DRM (`-tags drm`, libdrm) compilation on Raspberry Pi | NOT tested here |
| Physical touchscreen and rotary hold acceptance | NOT tested here |
| Real network scan/connect/saved credential tests | NOT tested here |
| On-device idle/input latency/CPU/RSS/temperature | NOT measured here |

RC3 is a **release-hardening candidate**, not a claim of final 0.5 acceptance. A test runner with libdrm and a physical HyperPixel must build the Pi version, exercise RC2 Wi-Fi, test the new encoder hold and install/demo workflow, and record performance numbers before declaring success.

The manifest registry intentionally refuses a user-writable root or group/other-writable binaries. Installed extra apps are still not isolated: any root-launched optional process may hold root privileges. Use only explicitly reviewed administrator-installed apps until process sandboxing is implemented.

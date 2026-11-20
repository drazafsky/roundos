# RC4 off-device validation — 2026-11-16

Exact source baseline: `roundos-0.5-rc3-wifi.zip`, copied into a new `roundos-0.5-rc4-wifi` working directory. Host: Linux amd64, Go 1.23.2, with root privileges for setuid tests.

| Check | Result |
|---|---|
| `go test ./... -count=1` | PASS |
| `go test -race ./... -count=1` | PASS |
| `go vet ./...` | PASS |
| `sh -n` on all new/modified shell scripts | PASS |
| `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build` for `roundd`, `round-clock`, `round-timer`, and `round-demo` | PASS |
| Root account validation, relative/unsafe writable state paths | PASS |
| Actual Clock/Timer child startup under `nobody` (test account) | PASS |
| Child UIDs/GIDs verified through `/proc/<pid>/status` | PASS |
| IPC directory 0710, socket 0660, authenticated app IPC | PASS |
| Non-root Timer state written with 0600 mode | PASS |
| Existing Clock/Timer/demo IPC and RoundOS widget regression suite | PASS |
| Native Pi DRM build (`-tags drm`) | NOT TESTED on host (no target libdrm) |
| Physical RoundOS controls, Wi-Fi and Timer migration on Pi | PENDING |
| Pi framebuffer latency, idle/active CPU, RSS, power measurements | PENDING |

**Security interpretation:** this milestone prevents the *normal* root-to-app privilege inheritance in the launch flow. It is not an app sandbox or a guarantee of protection against a malicious executable that can exploit a local privilege escalation, other apps sharing the same UID, or bugs in privileged shell code. Keep installation restricted to trusted programs.

**Noninvasive tests:** root-only integration tests create and remove temporary test state directories under `/var/lib/roundos-rc4-test-*` and `/var/lib/roundos-policy-test-*`. They do not install a permanent system user, install service files, modify NetworkManager, or change kernel/display configuration.

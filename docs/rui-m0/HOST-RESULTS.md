# RUI M0 — host validation record

**Run date:** November 19, 2026 (Linux amd64, Go 1.23.2)

**Original RoundOS 0.5.0 SHA-256:** `06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75`.

The original ZIP digest matched, and all 149 original source files matched the additive checkpoint manifest. The verification commands executed against the exact original 0.5.0 source plus M0-only files.

| Check | Host outcome |
|---|---|
| `provenance` | PASS |
| `go_test` | PASS |
| `go_vet` | PASS |
| `python_tests` | PASS |
| `m0_python_tests` | PASS |
| `shell_install_syntax` | PASS |
| `shell_ctl_syntax` | PASS |
| `shell_drm_syntax` | PASS |
| `version` | PASS |
| `go_race` | PASS |
| `armv7_roundd` | PASS |
| `armv7_round-clock` | PASS |
| `armv7_round-timer` | PASS |
| `armv7_round-demo` | PASS |
| `service_static` | PASS |

The isolated service-control tests exercised the real 0.5.0 `roundosctl.sh` script with mock `systemctl` and temporary managed-release directories; **they do not prove real systemd/boot behavior**.

The four ARMv7 builds are **non-DRM crossbuilds** only. This host lacks `libdrm` development headers, so a native Pi DRM build was not possible. The original 0.5.0 code and IPC v7 remain unchanged.

**M0 gate: OPEN.** Required external steps: Pi-native DRM build, operator-controlled managed install/activate/reboot/restart/disable/rollback, and 3 × 60-second reference Pi samples in each of the idle, gallery-flick, Timer-background, and Wi-Fi-scan conditions. Do not treat PASS_HOST_ONLY as full milestone acceptance.

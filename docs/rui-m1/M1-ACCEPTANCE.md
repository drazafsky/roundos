# RUI M1 acceptance record

| Criterion | Status / evidence |
|---|---|
| Original RoundOS 0.5.0 source files unchanged | Automated SHA-256 provenance, `scripts/m0/verify_source.py` (149 entries) |
| Schema version and YAML grammar documented | `RUI-V1-LANGUAGE.md` |
| Public typed syntax tree and source positions | `sdk/rui/spec/types.go` |
| Strict bounded YAML reader | `internal/rui/parse` |
| Semantic validator and typed binding/action manifest | `internal/rui/validate` |
| Nine SDK widget types and four templates | Go test fixtures; `all-widgets.rui.yaml` |
| Dialog, group, navigation, value constraints | Go test fixtures and schema |
| Duplicate keys, unknown fields, invalid types, aliases/tags rejected | Invalid golden YAML fixtures with exact diagnostic output |
| Transport budget awareness | Source limits, 16 screens, 64 effective widgets enforced. Runtime 64-KiB check deferred to M2/M3 |
| 0.5.0 shell/IPC and managed service unchanged | 149-file provenance check; no app runtime edits |
| Hardware M0 performance/boot/rollback | **PENDING**; independent M0 gate remains open |

M1 is **language-definition complete** when the same tests pass on a supported
host and on Pi. Host race-detector validation is required for the new Go packages
on a supported architecture; `-race` on the Pi's `linux/arm` is unsupported.

Run `scripts/m1/verify.sh` to check schema syntax, provenance, Go parser tests
and static vet. Run `go test ./...` to cover the full 0.5.0 regression suite.
No release deployment or systemd service change is needed for M1.

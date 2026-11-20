# RUI M1 — Start here

- `RUI-V1-LANGUAGE.md`: approved file format, typed bindings, widget/layout
  semantics, existing IPC mapping and future SDK/compiler handoff.
- `M1-ACCEPTANCE.md`: acceptance checklist and explicit pending M0 hardware gate.
- `../../schemas/roundos-rui-v1.schema.json`: machine-readable structural
  schema (not a replacement for semantic validation).
- `../../internal/rui/testdata/valid/*.rui.yaml`: samples including the nine
  supported widgets and a Timer layout.
- `../../internal/rui/testdata/invalid/*.rui.yaml` plus `.want`: golden
  rejection cases and line/column messages.

From SSH on the Pi (does not touch the running service):

```sh
./scripts/m1/verify.sh
go test ./...
```

Do not run the installer, activate, reboot or modify the kernel for M1.

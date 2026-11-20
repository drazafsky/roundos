#!/bin/sh
# No systemctl, DRM device, installer or network operation is performed here.
set -eu
cd "$(dirname "$0")/../.."
python3 scripts/m0/verify_source.py
python3 -m json.tool schemas/roundos-rui-v1.schema.json >/dev/null
go test ./internal/rui/... ./sdk/rui/... -count=1
go vet ./internal/rui/... ./sdk/rui/...
printf '\nM1 schema, source provenance, Go syntax and golden fixtures passed.\n'

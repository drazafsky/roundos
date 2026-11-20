#!/bin/sh
# Safe, deterministic M2 checks; never accesses DRM, systemd or installed apps.
set -eu
cd "$(dirname "$0")/../.."
python3 scripts/m0/verify_source.py
python3 -m unittest discover -s scripts/m0/tests -p 'test_*.py'
python3 -m json.tool schemas/roundos-rui-v1.schema.json >/dev/null
go test ./... -count=1
go vet ./...
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT HUP INT TERM
go build -o "$TMPDIR/roundui" ./cmd/roundui
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o "$TMPDIR/roundui-armv7" ./cmd/roundui
for f in layouts/*.rui.yaml; do
  "$TMPDIR/roundui" validate "$f"
  name=$(basename "$f" .rui.yaml)
  "$TMPDIR/roundui" compile -o "$TMPDIR/$name.rui.bin" -go "$TMPDIR/$name.ids.go" -package main "$f"
  "$TMPDIR/roundui" inspect "$TMPDIR/$name.rui.bin"
  "$TMPDIR/roundui" preview -o "$TMPDIR/$name.png" "$TMPDIR/$name.rui.bin"
  "$TMPDIR/roundui" compile -o "$TMPDIR/$name-again.rui.bin" "$f"
  cmp "$TMPDIR/$name.rui.bin" "$TMPDIR/$name-again.rui.bin"
done
printf '\nM2 language, bundle determinism, offline preview and host tools passed.\n'
printf 'ARMv7 roundui build succeeded (not run on Pi in this verification).\n'

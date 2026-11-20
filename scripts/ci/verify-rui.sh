#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
python3 -m json.tool schemas/roundos-rui-v1.schema.json >/dev/null
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
go build -o "$work/roundui" ./cmd/roundui
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o "$work/roundui-armv7" ./cmd/roundui
for layout in layouts/*.rui.yaml; do
  name=$(basename "$layout" .rui.yaml)
  "$work/roundui" validate "$layout"
  "$work/roundui" compile -o "$work/$name.rui.bin" -go "$work/$name.ids.go" -package main "$layout"
  "$work/roundui" inspect "$work/$name.rui.bin"
  "$work/roundui" preview -o "$work/$name.png" "$work/$name.rui.bin"
  "$work/roundui" compile -o "$work/$name-again.rui.bin" "$layout"
  cmp "$work/$name.rui.bin" "$work/$name-again.rui.bin"
done

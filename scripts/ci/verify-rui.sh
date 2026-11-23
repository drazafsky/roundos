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

# Built-in app presentation must match its checked-in embedded artifact.
cmp "$work/clock.rui.bin" layouts/clock.rui.bin
cmp "$work/timer.rui.bin" layouts/timer.rui.bin

# M3 is an opt-in SDK example. Verify its checked-in embedded bundle and both
# build targets without installing the demo or starting a shell/service.
"$work/roundui" validate examples/rui-demo/layout.rui.yaml
"$work/roundui" compile -o "$work/demo.rui.bin" examples/rui-demo/layout.rui.yaml
cmp "$work/demo.rui.bin" examples/rui-demo/layout.rui.bin
"$work/roundui" inspect "$work/demo.rui.bin"
"$work/roundui" preview -o "$work/demo.png" "$work/demo.rui.bin"
go build -o "$work/round-rui-demo" ./examples/rui-demo
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o "$work/round-rui-demo-armv7" ./examples/rui-demo

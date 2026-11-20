#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
GOFLAGS='' go test ./...
CGO_ENABLED=1 go build -trimpath -buildvcs=false -tags drm -ldflags "-X roundos/internal/release.BuildID=0.5.0" -o roundd ./cmd/roundd
CGO_ENABLED=0 go build -o round-clock ./cmd/round-clock
CGO_ENABLED=0 go build -o round-timer ./cmd/round-timer
printf 'Built roundd, round-clock, round-timer\n'

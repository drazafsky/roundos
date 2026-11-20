#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
version=${ROUNDOS_VERSION:-$(cat VERSION)-dev}
build_id=${ROUNDOS_BUILD_ID:-source}
output=${ROUNDOS_OUTPUT_DIR:-.}
# Values are passed to the Go linker and used in installation paths.
case "$version" in ''|*[!0-9A-Za-z.+-]*|[!0-9]*) echo 'Invalid release version' >&2; exit 1;; esac
case "$build_id" in ''|*[!0-9A-Za-z.+-]*) echo 'Invalid build ID' >&2; exit 1;; esac
mkdir -p "$output"
# Invalidate stale metadata before starting a new build.
rm -f "$output/RELEASE_VERSION"
GOFLAGS='' go test ./...
CGO_ENABLED=1 go build -trimpath -buildvcs=false -tags drm -ldflags "-X roundos/internal/release.Version=$version -X roundos/internal/release.BuildID=$build_id" -o "$output/roundd" ./cmd/roundd
for app in round-clock round-timer round-demo roundui; do
  CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$output/$app" "./cmd/$app"
done
printf '%s\n' "$version" > "$output/RELEASE_VERSION"
printf 'Built RoundOS %s in %s\n' "$version" "$output"

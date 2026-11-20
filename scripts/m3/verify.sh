#!/bin/sh
# Offline, nonprivileged M3 acceptance. No modification of installed RoundOS.
set -eu
cd "$(dirname "$0")/../.."
# Current main intentionally differs from the immutable 0.5.0 archive.
# Exercise historical provenance with isolated fixtures, as the main CI does.
python3 -m unittest discover -s scripts/m0/tests -p 'test_*.py'
python3 -m unittest discover -s scripts/tests -p 'test_*.py'
python3 -m unittest discover -s scripts/ci/tests -p 'test_*.py'
go test ./... -count=1
go vet ./...
sh scripts/ci/verify-rui.sh
printf '\nM3 RUI runtime, v7 IPC integration, demo embedding, and ARMv7 builds passed.\n'
printf 'This does not assert physical hardware acceptance or close outstanding M0 gates.\n'

#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
sh scripts/m3/verify.sh
printf '\nM4 host checks passed; physical Pi acceptance remains separate.\n'

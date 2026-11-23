#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
sh scripts/m5/verify.sh
printf '\nM6 app migration and independent SDK build checks passed.\n'
printf 'Physical Pi acceptance and release promotion remain M7 gates.\n'

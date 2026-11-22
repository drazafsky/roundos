#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
sh scripts/m4/verify.sh
printf '\nM5 host protocol, binding, template and compatibility checks passed.\n'
printf 'Physical acceptance and stable release promotion remain M7 gates.\n'

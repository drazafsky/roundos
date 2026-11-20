#!/bin/sh
# Install the exact built-in application binaries from this source checkpoint.
# The source checkout might be in a 0700 user home directory, which the
# unprivileged app account cannot traverse.
set -eu
cd "$(dirname "$0")/.."
for bin in round-clock round-timer; do
  if [ ! -x "./$bin" ]; then
    echo "Missing ./$bin; run ./scripts/build.sh first" >&2
    exit 1
  fi
done
sudo install -d -o root -g root -m 0755 /usr/local/libexec/roundos
for bin in round-clock round-timer; do
  sudo install -o root -g root -m 0755 "./$bin" "/usr/local/libexec/roundos/$bin"
done
printf 'Installed trusted app binaries to /usr/local/libexec/roundos.\n'

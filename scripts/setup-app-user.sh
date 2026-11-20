#!/bin/sh
# Explicit, one-time provisioning; never run automatically during build/start.
# Does not change kernel, Wi-Fi profiles, LightDM service or boot settings.
set -eu
if [ "$(id -u)" -eq 0 ]; then
  AS_ROOT=''
else
  AS_ROOT=sudo
fi
APPUSER=roundos-app
STATE=/var/lib/roundos/app-data
if ! getent passwd "$APPUSER" >/dev/null; then
  echo "Creating dedicated unprivileged user: $APPUSER"
  $AS_ROOT useradd --system --user-group --no-create-home --home-dir /var/lib/roundos --shell /usr/sbin/nologin "$APPUSER"
fi
if [ "$(id -u "$APPUSER")" -eq 0 ]; then
  echo 'Refusing: app user resolves to UID 0' >&2
  exit 1
fi
if [ "$(id -g "$APPUSER")" -eq 0 ]; then
  echo 'Refusing: app user resolves to GID 0' >&2
  exit 1
fi
$AS_ROOT install -d -o root -g root -m 0755 /var/lib/roundos
# Refuse a user-supplied symlink in place of the state directory.
if [ -L "$STATE" ]; then
  echo "Refusing symlinked state directory: $STATE" >&2
  exit 1
fi
$AS_ROOT install -d -o "$APPUSER" -g "$APPUSER" -m 0700 "$STATE"
# Preserve the historical root-launched Timer state without overwriting a
# previously migrated state. Install as 0600 so no other UID can read it.
OLD=/root/.local/state/roundos/timer.json
NEW=$STATE/timer.json
if [ -f "$OLD" ] && [ ! -e "$NEW" ] && [ ! -L "$NEW" ]; then
  $AS_ROOT install -o "$APPUSER" -g "$APPUSER" -m 0600 "$OLD" "$NEW"
  echo "Copied prior Timer state to $NEW (original unchanged)."
fi
printf 'Provisioned restricted app runtime: %s\n' "$STATE"
printf 'Run ./scripts/build.sh then ./scripts/test-drm.sh from SSH.\n'

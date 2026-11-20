#!/bin/sh
# Install a versioned, root-owned RoundOS release without taking over the display.
# Intentionally never enables/stops LightDM or RoundOS; use roundosctl activate.
set -eu
umask 077
if [ "$(id -u)" -ne 0 ]; then
  echo 'Run sudo ./scripts/install-release.sh after ./scripts/build.sh' >&2
  exit 1
fi
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
VERSION=0.5.0
RELEASES=/opt/roundos/releases
DEST=$RELEASES/$VERSION
for bin in roundd round-clock round-timer; do
  if [ ! -f "$ROOT/$bin" ] || [ ! -x "$ROOT/$bin" ] || [ -L "$ROOT/$bin" ]; then
    echo "Missing or untrusted $bin; run ./scripts/build.sh first" >&2
    exit 1
  fi
done
actual=$($ROOT/roundd -version)
case "$actual" in
  'RoundOS 0.5.0 '*) : ;;
  *) echo "Wrong roundd build: $actual" >&2; exit 1;;
esac
unit=/etc/systemd/system/roundos.service
if [ -f "$unit" ] && ! cmp -s "$ROOT/systemd/roundos.service" "$unit"; then
  echo "Existing $unit differs; refusing to replace a manually edited unit." >&2
  exit 1
fi
if [ -L "$unit" ]; then echo 'Refusing a symlinked service unit' >&2;exit 1;fi
if systemctl is-active --quiet roundos.service 2>/dev/null; then
  echo 'Refusing to change installed release while the RoundOS service is active.' >&2
  exit 1
fi
if [ -e "$DEST" ] || [ -L "$DEST" ]; then
  echo "Release already installed, refusing overwrite: $DEST" >&2
  exit 1
fi
# Never follow pre-existing symlinks in privileged installation roots.
for dir in /opt/roundos "$RELEASES" /etc/roundos /var/lib/roundos; do
  if [ -L "$dir" ]; then echo "Refusing symlinked trusted directory $dir" >&2; exit 1; fi
done
"$ROOT/scripts/setup-app-user.sh"
install -d -o root -g root -m 0755 /opt/roundos "$RELEASES" /etc/roundos
if [ -L /var/lib/roundos/shell-data ]; then
  echo 'Refusing symlinked shell state directory' >&2; exit 1
fi
install -d -o root -g root -m 0700 /var/lib/roundos/shell-data
# Carry forward RC4's root-launched shell appearance without overwriting new
# settings or changing the older, rollback-capable state file.
prior=/root/.local/state/roundos/appearance.json
current=/var/lib/roundos/shell-data/appearance.json
if [ -f "$prior" ] && [ ! -L "$prior" ] && [ ! -e "$current" ] && [ ! -L "$current" ]; then
  install -o root -g root -m 0600 "$prior" "$current"
  echo 'Copied existing RoundOS appearance settings (original unchanged).'
fi
if [ ! -e /etc/roundos/gestures.json ]; then
  install -o root -g root -m 0644 "$ROOT/config/gestures.example.json" /etc/roundos/gestures.json
fi
stage=$(mktemp -d "$RELEASES/.install.XXXXXXXX")
cleanup() { if [ -n "${stage:-}" ] && [ -d "$stage" ]; then rm -rf -- "$stage"; fi; }
trap cleanup EXIT HUP INT TERM
install -d -o root -g root -m 0755 "$stage/bin"
for bin in roundd round-clock round-timer; do
  install -o root -g root -m 0755 "$ROOT/$bin" "$stage/bin/$bin"
done
install -o root -g root -m 0644 "$ROOT/README.md" "$stage/README.md"
( cd "$stage" && sha256sum bin/roundd bin/round-clock bin/round-timer > SHA256SUMS && sha256sum -c SHA256SUMS )
chmod 0755 "$stage"
mv -- "$stage" "$DEST"
stage=''
# Keep existing managed release as rollback target if one exists.
if [ -L /opt/roundos/current ]; then
  old=$(readlink -f /opt/roundos/current)
  case "$old" in
    "$RELEASES"/*) if [ -d "$old" ] && [ "$old" != "$DEST" ]; then
      ln -s "$old" /opt/roundos/.previous.new
      mv -Tf /opt/roundos/.previous.new /opt/roundos/previous
    fi;;
    *) echo 'Refusing untrusted current release link' >&2; exit 1;;
  esac
elif [ -e /opt/roundos/current ]; then
  echo 'Refusing a non-link current release' >&2;exit 1
fi
ln -s "$DEST" /opt/roundos/.current.new
mv -Tf /opt/roundos/.current.new /opt/roundos/current
install -o root -g root -m 0644 "$ROOT/systemd/roundos.service" "$unit"
install -o root -g root -m 0755 "$ROOT/scripts/roundosctl.sh" /usr/local/sbin/roundosctl
systemctl daemon-reload
echo "Installed $DEST; service NOT enabled. Review docs and run: sudo roundosctl activate"

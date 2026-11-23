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
if [ ! -f "$ROOT/RELEASE_VERSION" ] || [ -L "$ROOT/RELEASE_VERSION" ]; then
  echo 'Missing or untrusted RELEASE_VERSION; run ./scripts/build.sh first' >&2
  exit 1
fi
VERSION=$(cat "$ROOT/RELEASE_VERSION")
case "$VERSION" in ''|*[!0-9A-Za-z.+-]*|[!0-9]*) echo 'Invalid release version' >&2; exit 1;; esac
RELEASES=/opt/roundos/releases
DEST=$RELEASES/$VERSION
for bin in roundd round-clock round-timer; do
  if [ ! -f "$ROOT/$bin" ] || [ ! -x "$ROOT/$bin" ] || [ -L "$ROOT/$bin" ]; then
    echo "Missing or untrusted $bin; run ./scripts/build.sh first" >&2
    exit 1
  fi
done
actual=$("$ROOT/roundd" -version)
case "$actual" in
  "RoundOS $VERSION ("*")") : ;;
  *) echo "Wrong roundd build: $actual" >&2; exit 1;;
esac
protocol=$("$ROOT/roundd" -update-protocol)
if [ "$protocol" != roundos-update-v1 ]; then
  echo 'Build lacks managed update/recovery support' >&2
  exit 1
fi
for update_unit in roundos.service roundos-update.service roundos-update-recover.service; do
  if [ ! -f "$ROOT/systemd/$update_unit" ] || [ -L "$ROOT/systemd/$update_unit" ]; then
    echo "Missing or untrusted service template: $update_unit" >&2; exit 1
  fi
done
unit=/etc/systemd/system/roundos.service
if [ -f "$unit" ] && ! cmp -s "$ROOT/systemd/roundos.service" "$unit"; then
  # Permit a managed template upgrade, including the original unmodified
  # Type=simple unit predating stored templates. Custom unit edits still fail.
  old_hash=$(sha256sum "$unit")
  old_hash=${old_hash%% *}
  managed_unit=0
  # Rollback swaps these links but leaves the installed template unchanged.
  # Recognize either verified snapshot so a later managed upgrade still works.
  for release_link in current previous; do
    prior_release=$(readlink -f "/opt/roundos/$release_link" 2>/dev/null || true)
    case "$prior_release" in
      /opt/roundos/releases/*)
        if [ -f "$prior_release/roundos.service" ] && [ ! -L "$prior_release/roundos.service" ] &&
           [ -f "$prior_release/SHA256SUMS" ] && [ ! -L "$prior_release/SHA256SUMS" ] &&
           (cd "$prior_release" && grep -E '^[0-9a-f]{64}  roundos[.]service$' SHA256SUMS | sha256sum -c --status -) &&
           cmp -s "$prior_release/roundos.service" "$unit"; then
          managed_unit=1
          break
        fi;;
    esac
  done
  if [ "$old_hash" != 06453b95893557e389483a3e06cac2926717d245f7895ef362d3364531c0285c ] && [ "$managed_unit" -ne 1 ]; then
    echo "Existing $unit differs; refusing to replace a manually edited unit." >&2
    exit 1
  fi
fi
if [ -L "$unit" ]; then echo 'Refusing a symlinked service unit' >&2;exit 1;fi
# The controller and its units remain unchanged during a live transaction. The
# independent helper upgrades them only after the replacement shell is ready.
check_update_unit() {
  update_unit=$1
  update_target=/etc/systemd/system/$update_unit
  if [ -L "$update_target" ]; then echo 'Refusing a symlinked update service unit' >&2; exit 1; fi
  if [ ! -e "$update_target" ] || cmp -s "$ROOT/systemd/$update_unit" "$update_target"; then return; fi
  for update_link in current previous update-controller; do
    update_prior=$(readlink -f "/opt/roundos/$update_link" 2>/dev/null || true)
    case "$update_prior" in
      /opt/roundos/releases/*)
        if [ -f "$update_prior/$update_unit" ] && [ ! -L "$update_prior/$update_unit" ] &&
           [ -f "$update_prior/SHA256SUMS" ] && [ ! -L "$update_prior/SHA256SUMS" ] &&
           (cd "$update_prior" && awk -v name="$update_unit" 'NF == 2 && $2 == name && length($1) == 64 && $1 ~ /^[0-9a-f]+$/ {printf "%s  %s\n", $1, $2}' SHA256SUMS | sha256sum -c --status -) &&
           cmp -s "$update_prior/$update_unit" "$update_target"; then return; fi;;
    esac
  done
  echo "Refusing to replace a manually edited update unit: $update_target" >&2; exit 1
}
if [ "${ROUNDOS_UPDATE_TRANSACTION:-0}" != 1 ]; then
  check_update_unit roundos-update.service
  check_update_unit roundos-update-recover.service
  if [ -L /opt/roundos/update-controller ]; then
    update_controller=$(readlink -f /opt/roundos/update-controller)
    case "$update_controller" in
      /opt/roundos/releases/*) [ -d "$update_controller" ] || { echo 'Missing update controller release' >&2; exit 1; };;
      *) echo 'Refusing an untrusted update controller link' >&2; exit 1;;
    esac
  elif [ -e /opt/roundos/update-controller ]; then
    echo 'Refusing a non-link update controller' >&2; exit 1
  fi
fi
service_state=$(systemctl is-active roundos.service 2>/dev/null || true)
case "$service_state" in
  inactive|failed) :;;
  *) echo "Refusing to change installed release while the RoundOS service is $service_state (must be inactive or failed)." >&2; exit 1;;
esac
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
install -o root -g root -m 0644 "$ROOT/systemd/roundos.service" "$stage/roundos.service"
install -o root -g root -m 0644 "$ROOT/systemd/roundos-update.service" "$stage/roundos-update.service"
install -o root -g root -m 0644 "$ROOT/systemd/roundos-update-recover.service" "$stage/roundos-update-recover.service"
( cd "$stage" && sha256sum bin/roundd bin/round-clock bin/round-timer roundos.service roundos-update.service roundos-update-recover.service > SHA256SUMS && sha256sum -c SHA256SUMS )
chmod 0755 "$stage"
mv -- "$stage" "$DEST"
stage=''
# Persist release contents and directory entries before making them reachable
# through current or the independent recovery-controller link (GNU syncfs).
sync -f "$DEST"
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
install -o root -g root -m 0755 "$ROOT/scripts/roundosctl.sh" /usr/local/sbin/roundosctl
if [ "${ROUNDOS_UPDATE_TRANSACTION:-0}" != 1 ]; then
  ln -s "$DEST" /opt/roundos/.update-controller.new
  mv -Tf /opt/roundos/.update-controller.new /opt/roundos/update-controller
  sync -f /opt/roundos
  # Install dependencies before the shell unit that requires them. Atomic
  # replacement preserves a readable boot unit across interruption.
  for update_unit in roundos-update.service roundos-update-recover.service roundos.service; do
    unit_temp=$(mktemp /etc/systemd/system/.roundos-unit.XXXXXXXX)
    install -o root -g root -m 0644 "$ROOT/systemd/$update_unit" "$unit_temp"
    sync -f "$unit_temp"
    unit_target="/etc/systemd/system/$update_unit"
    if [ "$update_unit" = roundos.service ]; then unit_target=$unit; fi
    mv -Tf "$unit_temp" "$unit_target"
    sync -f /etc/systemd/system
  done
  systemctl daemon-reload
fi
# The managed Go helper replaces the shell unit atomically after this returns.
echo "Installed $DEST; service NOT enabled. Review docs and run: sudo roundosctl activate"

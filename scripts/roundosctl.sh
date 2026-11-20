#!/bin/sh
# Root-only, reversible systemd activation for the RoundOS managed release.
# No passwords, Wi-Fi profiles, or app data are copied or deleted.
set -eu
umask 077
UNIT=roundos.service
DM=lightdm.service
ROOT=/opt/roundos
STAMP=/var/lib/roundos/desktop-before-roundos
usage() { echo 'Usage: roundosctl {status|verify|activate|disable|rollback|logs}' >&2; exit 2; }
status() {
  printf 'Release: '; readlink -f "$ROOT/current" 2>/dev/null || echo unavailable
  printf 'Service: '; systemctl is-active "$UNIT" 2>/dev/null || :
  printf 'Boot: '; systemctl is-enabled "$UNIT" 2>/dev/null || :
  printf 'Display manager: '; systemctl is-active "$DM" 2>/dev/null || :
}
verify() {
  dir=$(readlink -f "$ROOT/current" 2>/dev/null || true)
  case "$dir" in "$ROOT"/releases/*) : ;; *) echo 'Invalid release link' >&2; exit 1;; esac
  [ -d "$dir" ] && [ -f "$dir/SHA256SUMS" ] || { echo 'Release missing' >&2; exit 1; }
  (cd "$dir" && sha256sum -c SHA256SUMS)
  "$dir/bin/roundd" -version
}
need_root() { [ "$(id -u)" -eq 0 ] || { echo 'Run with sudo.' >&2; exit 1; }; }
need_ssh() {
  if [ -z "${SSH_CONNECTION:-}" ] && [ -z "${SSH_TTY:-}" ]; then
    echo 'Display switching requires an SSH session, not the Pi desktop.' >&2; exit 1
  fi
}
restore_dm() {
  was_enabled=unknown; was_active=inactive
  if [ -f "$STAMP" ]; then
    read -r was_enabled was_active < "$STAMP" || :
  fi
  case "$was_enabled" in
    enabled|enabled-runtime|linked|linked-runtime) systemctl enable "$DM" || :;;
    disabled|masked|static|indirect|generated|transient|unknown) :;;
    *) echo 'Ignoring unrecognized prior desktop state' >&2;;
  esac
  if [ "$was_active" = active ]; then systemctl start "$DM" || :; fi
}
activate() {
  need_root;need_ssh;verify
  if systemctl is-active --quiet "$UNIT";then echo 'RoundOS is already active';return;fi
  if [ ! -f "$STAMP" ];then
    before_enabled=$(systemctl is-enabled "$DM" 2>/dev/null || true)
    before_active=$(systemctl is-active "$DM" 2>/dev/null || true)
    printf '%s %s\n' "$before_enabled" "$before_active" > "$STAMP"
    chmod 0600 "$STAMP"
  fi
  systemctl enable "$UNIT"
  # Disable desktop autostart only after the previous boot state is saved.
  # A failed disable/stop must not leave the machine in an ambiguous boot state.
  failed=0
  systemctl disable "$DM" || failed=1
  if [ "$failed" -eq 0 ]; then systemctl stop "$DM" || failed=1; fi
  if [ "$failed" -eq 1 ] || ! systemctl start "$UNIT";then
    echo 'RoundOS failed to start; restoring previous desktop state.' >&2
    systemctl disable --now "$UNIT" || :
    restore_dm
    exit 1
  fi
  echo 'RoundOS started and enabled for future boots.'
}
disable() {
  need_root;need_ssh
  systemctl disable --now "$UNIT"
  restore_dm
  rm -f "$STAMP"
  echo 'RoundOS disabled; prior desktop state restored where available.'
}
rollback() {
  need_root; need_ssh
  old=$(readlink -f "$ROOT/previous" 2>/dev/null || true)
  case "$old" in
    "$ROOT"/releases/*) [ -d "$old" ] || { echo 'Rollback release directory missing' >&2;exit 1;};;
    *) echo 'No previous managed release; disabling RoundOS and restoring the desktop.'; disable; return;;
  esac
  current=$(readlink -f "$ROOT/current")
  if [ "$old" = "$current" ];then echo 'Previous release equals current release' >&2;exit 1;fi
  active=0
  if systemctl is-active --quiet "$UNIT";then active=1; systemctl stop "$UNIT";fi
  ln -s "$old" "$ROOT/.current.new"
  mv -Tf "$ROOT/.current.new" "$ROOT/current"
  ln -s "$current" "$ROOT/.previous.new"
  mv -Tf "$ROOT/.previous.new" "$ROOT/previous"
  if [ "$active" -eq 1 ];then
    if ! systemctl start "$UNIT";then
      echo 'Rollback release failed to start; restoring desktop.' >&2
      disable
      exit 1
    fi
  fi
  echo "Rolled back to $old. App state and Wi-Fi profiles unchanged."
}
case "${1:-}" in
  status) status;;
  verify) verify;;
  activate) activate;;
  disable) disable;;
  rollback) rollback;;
  logs) exec journalctl -u "$UNIT" -n 100 --no-pager;;
  *) usage;;
esac

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
load_desktop_state() {
  [ ! -L "$STAMP" ] && [ -f "$STAMP" ] || { echo 'Missing or unsafe desktop recovery record' >&2; return 1; }
  # Two-field records from previous versions remain valid for ordinary units.
  read -r was_enabled was_active was_fragment < "$STAMP" || return 1
  case "$was_enabled" in
    enabled|enabled-runtime|disabled|masked|masked-runtime|static|indirect|generated|transient) :;;
    linked|linked-runtime)
      case "$was_fragment" in /*) :;; *) echo 'Missing linked desktop unit path' >&2; return 1;; esac
      [ -f "$was_fragment" ] || return 1;;
    *) echo 'Unrecognized prior desktop enablement state' >&2; return 1;;
  esac
  case "$was_active" in active|inactive|failed) :;; *) echo 'Unrecognized prior desktop active state' >&2; return 1;; esac
}
capture_desktop_state() {
  if [ -e "$STAMP" ] || [ -L "$STAMP" ]; then load_desktop_state; return; fi
  was_enabled=$(systemctl is-enabled "$DM" 2>/dev/null || true)
  was_active=$(systemctl is-active "$DM" 2>/dev/null || true)
  was_fragment=''
  case "$was_enabled" in
    linked|linked-runtime) was_fragment=$(systemctl show --property=FragmentPath --value "$DM") || return 1;;
  esac
  record=$(mktemp "$STAMP.XXXXXX") || return 1
  if ! printf '%s %s %s\n' "$was_enabled" "$was_active" "$was_fragment" > "$record" || ! mv -f "$record" "$STAMP"; then
    rm -f "$record"
    return 1
  fi
  load_desktop_state
}
restore_dm() {
  if [ ! -e "$STAMP" ] && [ ! -L "$STAMP" ]; then
    echo 'No desktop recovery record; desktop configuration unchanged.'
    return 0
  fi
  load_desktop_state || return 1
  case "$was_enabled" in
    enabled) systemctl enable "$DM" || return 1;;
    enabled-runtime) systemctl enable --runtime "$DM" || return 1;;
    linked) systemctl link "$was_fragment" || return 1;;
    linked-runtime) systemctl link --runtime "$was_fragment" || return 1;;
  esac
  actual_enabled=$(systemctl is-enabled "$DM" 2>/dev/null || true)
  if [ "$actual_enabled" != "$was_enabled" ]; then
    echo "Desktop enablement restoration incomplete: expected $was_enabled, got $actual_enabled" >&2
    return 1
  fi
  if [ "$was_active" = active ]; then
    systemctl start "$DM" && systemctl is-active --quiet "$DM" || return 1
  fi
}
check_startup_protocol() {
  protocol=$("$1/bin/roundd" -startup-protocol 2>/dev/null) || protocol=''
  if [ "$protocol" != sd-notify-v1 ]; then
    echo 'Release lacks managed readiness support; no release switch performed. Use roundosctl disable to restore the desktop.' >&2
    return 1
  fi
  unit_type=$(systemctl show --property=Type --value "$UNIT") || return 1
  if [ "$unit_type" != notify ]; then
    echo 'Managed startup requires the installed Type=notify unit; reinstall the managed release before activation.' >&2
    return 1
  fi
}
start_ready() {
  # Type=notify makes start wait for READY=1 or TimeoutStartSec/failure.
  systemctl start "$UNIT" && systemctl is-active --quiet "$UNIT"
}
recover_desktop() {
  # Explicitly cancel pending automatic restarts before handing DRM to LightDM.
  systemctl disable --now "$UNIT" || return 1
  if ! restore_dm; then
    echo "Desktop restoration failed; recovery record retained at $STAMP. Retry roundosctl disable over SSH." >&2
    return 1
  fi
  rm -f "$STAMP"
}
activate() {
  need_root;need_ssh;verify
  check_startup_protocol "$(readlink -f "$ROOT/current")"
  # An active notify service has completed initialization, including on retry.
  if systemctl is-active --quiet "$UNIT";then echo 'RoundOS is already ready';return;fi
  capture_desktop_state
  # Disable desktop autostart only after the previous boot state is saved.
  # A failed disable/stop must not leave the machine in an ambiguous boot state.
  failed=0
  systemctl enable "$UNIT" || failed=1
  if [ "$failed" -eq 0 ]; then
    case "$was_enabled" in
      enabled) systemctl disable "$DM" || failed=1;;
      enabled-runtime) systemctl disable --runtime "$DM" || failed=1;;
    esac
  fi
  if [ "$failed" -eq 0 ] && [ "$was_active" = active ]; then systemctl stop "$DM" || failed=1; fi
  if [ "$failed" -eq 1 ] || ! start_ready;then
    echo 'RoundOS failed to start; restoring previous desktop state.' >&2
    recover_desktop || return 1
    exit 1
  fi
  echo 'RoundOS ready and enabled for future boots.'
}
disable() {
  need_root;need_ssh
  recover_desktop
  echo 'RoundOS disabled; prior recorded desktop state restored.'
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
  (cd "$old" && sha256sum -c SHA256SUMS)
  check_startup_protocol "$old"
  active=0
  if systemctl is-active --quiet "$UNIT";then active=1; systemctl stop "$UNIT";fi
  ln -s "$old" "$ROOT/.current.new"
  mv -Tf "$ROOT/.current.new" "$ROOT/current"
  ln -s "$current" "$ROOT/.previous.new"
  mv -Tf "$ROOT/.previous.new" "$ROOT/previous"
  if [ "$active" -eq 1 ];then
    if ! start_ready;then
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

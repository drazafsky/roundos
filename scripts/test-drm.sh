#!/bin/sh
# Launch from SSH only; temporarily stop LightDM and restore it on exit.
set -eu
if [ -z "${SSH_CONNECTION:-}" ] && [ -z "${SSH_TTY:-}" ]; then
  echo 'Start this script from a separate SSH session.' >&2
  exit 1
fi
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
for bin in roundd round-clock round-timer; do
  if [ ! -x "./$bin" ]; then
    echo "Missing $bin. Run ./scripts/build.sh first." >&2
    exit 1
  fi
done
# Never attempt a second DRM master while the boot-managed RoundOS service is
# running; an SSH-only manual preview must be mutually exclusive with systemd.
if systemctl is-active --quiet roundos.service 2>/dev/null; then
  echo 'Managed roundos.service is active. Stop it before manual DRM testing.' >&2
  exit 1
fi
if ! getent passwd roundos-app >/dev/null; then
  echo 'Missing unprivileged application account. Run ./scripts/setup-app-user.sh first.' >&2
  exit 1
fi
if [ ! -d /var/lib/roundos/app-data ]; then
  echo 'Missing private app state directory. Run ./scripts/setup-app-user.sh first.' >&2
  exit 1
fi
# Compile-free app installation to a root-managed path outside a potentially
# private /home/<user> directory. This is necessary for the unprivileged UID.
./scripts/install-app-binaries.sh
if ! command -v nmcli >/dev/null 2>&1; then
  echo 'Warning: nmcli is not installed; Settings -> Wi-Fi will be unavailable.' >&2
fi
PREVIOUS=$(systemctl is-active lightdm.service || true)
restore() {
  if [ "$PREVIOUS" = active ]; then
    echo 'Restoring LightDM desktop...'
    sudo systemctl start lightdm.service || true
  fi
}
trap restore EXIT
# Do not use an EXIT trap on INT/TERM as well: avoid duplicate restore attempts.
trap 'exit 130' INT
trap 'exit 143' TERM
if [ "$PREVIOUS" = active ]; then
  echo 'Stopping LightDM temporarily...'
  sudo systemctl stop lightdm.service
fi
echo 'RoundOS 0.5.0 manual test: l/r/p rotary, b Back, w Settings, e editor, z accessibility, h/c/t/g, k switcher, n notifications, nq/na/nc, a/x/s/q.'
sudo ./roundd -backend drm -card /dev/dri/card0 -connector DPI-1 -touch auto -app-user roundos-app -app-state-dir /var/lib/roundos/app-data -clock-exec /usr/local/libexec/roundos/round-clock -timer-exec /usr/local/libexec/roundos/round-timer "$@"

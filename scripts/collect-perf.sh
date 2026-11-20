#!/bin/sh
# Non-mutating, SSH-safe host diagnostics; never records SSIDs or passwords.
# Run in a second SSH session while RoundOS is active, especially while
# testing gallery flicks, and copy the 's' output from the RoundOS terminal.
set -eu
stamp=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
printf 'RoundOS performance snapshot UTC=%s\n' "$stamp"
printf 'Uptime/load: '; cat /proc/loadavg
printf 'Thermal (C): '
if [ -r /sys/class/thermal/thermal_zone0/temp ]; then awk '{printf "%.1f\n", $1/1000}' /sys/class/thermal/thermal_zone0/temp; else echo 'unavailable'; fi
printf 'Memory:\n'; free -m
printf 'Process RSS/CPU:\n'
# Read-only process inspection; avoid command arguments that could include
# manually supplied secrets. Display names and numeric PIDs only.
for name in roundd round-clock round-timer round-demo; do
 for pid in $(pgrep -x "$name" 2>/dev/null || true); do
  ps -p "$pid" -o pid=,comm=,%cpu=,rss=,etime= || true
 done
done
printf 'Use SSH console command s for render/input-to-present p95 metrics.\n'

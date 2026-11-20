#!/bin/sh
# Read-only RC4 physical verification: no environment, tokens, secrets or
# wireless connection details are printed.
set -eu
expected=roundos-app
if ! getent passwd "$expected" >/dev/null; then
  echo "Missing $expected; run scripts/setup-app-user.sh first." >&2
  exit 1
fi
echo 'Expected: roundd = root; round-clock/round-timer/round-demo = roundos-app.'
ps -eo pid,ppid,user,group,euid,egid,comm | awk '$7 == "roundd" || $7 == "round-clock" || $7 == "round-timer" || $7 == "round-demo" {print}'
for name in round-clock round-timer; do
  if pidof "$name" >/dev/null 2>&1; then
    for pid in $(pidof "$name"); do
      uid=$(awk '/^Uid:/ {print $2; exit}' "/proc/$pid/status" 2>/dev/null || true)
      expected_uid=$(id -u "$expected")
      if [ "$uid" != "$expected_uid" ]; then
        printf '%s PID %s: NOT ISOLATED (uid=%s)\n' "$name" "$pid" "$uid" >&2
        exit 1
      fi
    done
  else
    printf 'Warning: %s not running.\n' "$name"
  fi
done
echo 'Observed running built-in app processes have the expected UID.'

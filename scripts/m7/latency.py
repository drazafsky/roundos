#!/usr/bin/env python3
"""Allowlisted latency summaries for one newly started managed process."""
from datetime import datetime, timezone
import json
import re
import subprocess

DURATION = r'\d+(?:\.\d+)?(?:ns|µs|us|ms|s)'


def parse(records, identity, start, end):
    found = []
    boot = identity['boot_id'].replace('-', '')
    first = datetime.fromisoformat(start).timestamp()
    last = datetime.fromisoformat(end).timestamp()
    for record in records:
        if record.get('_PID') != str(identity['service_pid']) or record.get('_BOOT_ID') != boot:
            continue
        instant = int(record['__REALTIME_TIMESTAMP']) / 1000000
        if not first <= instant <= last:
            continue
        message = record.get('MESSAGE', '')
        if not isinstance(message, str) or 'RoundOS performance:' not in message:
            continue
        duration = re.search(r'(?<![\w-])input-to-present-p95=(' + DURATION + r')(?=\s)', message)
        count = re.search(r'\(n=(\d+);', message)
        if duration and count:
            found.append({'input-to-present-p95': duration[1], 'input_samples': int(count[1]),
                          'service_pid': identity['service_pid'], 'boot_id': identity['boot_id'],
                          'recorded_utc': datetime.fromtimestamp(instant, timezone.utc).isoformat(), 'run_scoped': True})
    return found


def collect(identity, start, end):
    result = subprocess.run(['journalctl', '-u', 'roundos.service', '_PID=' + str(identity['service_pid']),
                             '_BOOT_ID=' + identity['boot_id'].replace('-', ''), '--since', start,
                             '--until', end, '--no-pager', '--output=json'],
                            capture_output=True, text=True, timeout=15, check=True)
    return parse((json.loads(line) for line in result.stdout.splitlines() if line.strip()), identity, start, end)

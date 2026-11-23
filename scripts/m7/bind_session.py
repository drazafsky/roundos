#!/usr/bin/env python3
"""Bind an M0 sampler session to verified before/after managed identities.

Checks raw sample timestamps and shell PIDs, recalculates CPU/RSS summaries,
and writes a new evidence file. Originals remain unchanged.
"""
import argparse
from datetime import datetime
import hashlib
import importlib.util
import json
from pathlib import Path

from compare import IDENTITY_KEYS, load, number, valid_identity, write_new

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('m0_collector', ROOT / 'scripts/m0/collect_pi.py')
m0 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m0)


def instant(value):
    stamp = datetime.fromisoformat(value)
    if stamp.tzinfo is None:
        raise ValueError('Evidence timestamp lacks a timezone')
    return stamp


def bind(session_path, before, after):
    baseline = before.get('version') == '0.5.0'
    if not valid_identity(before, baseline) or not valid_identity(after, baseline):
        raise ValueError('Unverified managed identity')
    if any(before.get(key) != after.get(key) for key in IDENTITY_KEYS):
        raise ValueError('Managed identity changed during collection')
    session = load(session_path)
    first, last = instant(before['recorded_utc']), instant(after['recorded_utc'])
    if not first <= instant(session['start_utc']) <= instant(session['end_utc']) <= last:
        raise ValueError('Session is outside the verified identity window')
    hashes = {}
    for run in session['runs']:
        start, end = instant(run['start_utc']), instant(run['end_utc'])
        if not first <= start < end <= last or (end - start).total_seconds() < 59:
            raise ValueError('Invalid run window')
        path = Path(session_path).parent / f'run-{run["run"]:02d}.jsonl'
        data = path.read_bytes()
        if len(data) > 4 << 20:
            raise ValueError('Oversized raw sample')
        records = [json.loads(line) for line in data.splitlines() if line.strip()]
        if len(records) < 45 or records[-1]['elapsed_seconds'] < 59:
            raise ValueError('Incomplete raw sampling interval')
        elapsed = -1
        cpu_count = rss_count = 0
        for record in records:
            if not number(record.get('elapsed_seconds')) or not start <= instant(record['utc']) <= end or record['elapsed_seconds'] <= elapsed:
                raise ValueError('Raw timestamps do not match the session')
            elapsed = record['elapsed_seconds']
            shells = [process for process in record['processes'] if process['name'] == 'roundd']
            pids = [process['pid'] for process in shells]
            if pids != [before['service_pid']]:
                raise ValueError('Sampler did not observe exactly the verified shell')
            shell = shells[0]
            if shell.get('cpu_percent') is not None:
                if not number(shell['cpu_percent']): raise ValueError('Invalid raw CPU counter')
                cpu_count += 1
            if shell.get('rss_kib') is not None:
                if not number(shell['rss_kib']) or shell['rss_kib'] <= 0: raise ValueError('Invalid raw RSS counter')
                rss_count += 1
        if min(cpu_count, rss_count) < 45:
            raise ValueError('Too few valid CPU or RSS intervals')
        run['sample'] = m0.summarize_sample(path)
        run['sample'].update(shell_cpu_valid_records=cpu_count, shell_rss_valid_records=rss_count)
        hashes[path.name] = hashlib.sha256(data).hexdigest()
    session['m7_identity'] = {key: before.get(key) for key in IDENTITY_KEYS}
    session['m7_identity'].update({'verified': True, 'before_utc': before['recorded_utc'], 'after_utc': after['recorded_utc'], 'raw_sha256': hashes})
    return session


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('session', 'before', 'after', 'output'):
        parser.add_argument('--' + name, type=Path, required=True)
    args = parser.parse_args()
    try:
        session = bind(args.session, load(args.before), load(args.after))
        write_new(args.output, session)
    except (OSError, ValueError, KeyError, TypeError, IndexError) as error:
        parser.exit(1, f'Session binding failed: {type(error).__name__}\n')
    print('Wrote identity-bound session:', args.output)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

#!/usr/bin/env python3
"""Collect three M7 runs, explicitly restarting the managed shell for each.

--restart-for-latency authorizes each prompted systemctl restart. Other steps
are read-only. A fresh process prevents the original 0.5.0 rolling latency
window from retaining inputs from another run or workload.
"""
import argparse
import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from bind_session import bind
from compare import ROOT, RELEASE_KEYS, SCENARIOS, valid_identity, write_new
import latency


def identity(version):
    args = [sys.executable, str(ROOT / 'scripts/m7/identity.py'), '--expect-version', version]
    if os.geteuid() != 0:
        args = ['sudo', '--', *args]
    result = subprocess.run(args, stdout=subprocess.PIPE, text=True, check=True)
    value = json.loads(result.stdout)
    if not valid_identity(value, version == '0.5.0'):
        raise ValueError('Managed identity is not acceptable for M7')
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--expect-version', required=True)
    parser.add_argument('--scenario', choices=SCENARIOS, required=True)
    parser.add_argument('--output', type=Path, required=True, help='new private directory')
    parser.add_argument('--restart-for-latency', action='store_true', help='authorize one prompted service restart per run')
    args = parser.parse_args()
    if not args.restart_for_latency:
        parser.error('--restart-for-latency is required: legacy latency samples cannot otherwise be scoped to each run')
    try:
        args.output.mkdir(mode=0o700)
        runs = []
        first_identity = None
        expected = identity(args.expect_version)
        for number in (1, 2, 3):
            print(f'Run {number}/3: {args.scenario}. Prepare the workload/settings first.')
            input('Press Enter to restart roundos.service, then perform this workload until sampling finishes: ')
            armed = time.clock_gettime(time.CLOCK_BOOTTIME)
            subprocess.run(['sudo', '/usr/bin/systemctl', 'restart', 'roundos.service'], check=True)
            before = identity(args.expect_version)
            if any(before[key] != expected[key] for key in RELEASE_KEYS):
                raise ValueError('Managed release changed before restart')
            if before['service_start_boottime_seconds'] < armed:
                raise ValueError('Managed shell did not start inside this run window')
            if first_identity and any(before[key] != first_identity[key] for key in RELEASE_KEYS):
                raise ValueError('Managed release changed between runs')
            first_identity = first_identity or before
            directory = args.output / f'run-{number:02d}'
            directory.mkdir(mode=0o700)
            write_new(directory / 'before.json', before)
            # Reuse the original CPU/RSS sampler and one-run interactive flow.
            subprocess.run([sys.executable, str(ROOT / 'scripts/m0/collect_pi.py'), '--repeats', '1',
                            '--scenario', args.scenario, '--output', str(directory / 'samples')], check=True)
            after = identity(args.expect_version)
            write_new(directory / 'after.json', after)
            bound = bind(directory / 'samples/session.json', before, after)
            run = copy.deepcopy(bound['runs'][0])
            run['run'] = number
            run['m7_identity'] = bound['m7_identity']
            run['m7_identity']['window_start_boottime_seconds'] = armed
            run['renderer_metrics'] = latency.collect(before, run['start_utc'], run['end_utc'])
            if not run['renderer_metrics']:
                raise ValueError('No latency summary from the new managed process')
            write_new(directory / 'session-bound.json', bound)
            runs.append(run)
        session = {'scenario': args.scenario, 'accepted_for_m0': True, 'reference_pi_managed': True,
                   'status': 'COLLECTED', 'target_seconds': 60, 'target_repeats': 3,
                   'interval_seconds': 1, 'runs': runs}
        write_new(args.output / 'before.json', first_identity)
        write_new(args.output / 'session-bound.json', session)
    except (OSError, ValueError, KeyError, TypeError, EOFError, subprocess.SubprocessError) as error:
        parser.exit(1, f'M7 collection incomplete: {type(error).__name__}; existing evidence retained.\n')
    print('M7 evidence:', args.output / 'session-bound.json')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

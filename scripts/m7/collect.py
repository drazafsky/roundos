#!/usr/bin/env python3
"""Collect three M7 runs, explicitly restarting the managed shell for each.

--restart-for-latency authorizes each prompted start-limit reset and restart. Other steps
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
import tempfile
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


def checkpoint(path, value):
    """Replace only the report in this invocation's new private directory."""
    fd, name = tempfile.mkstemp(prefix='.scenario-', dir=path.parent)
    temporary = Path(name)
    try:
        with os.fdopen(fd, 'w', encoding='utf-8') as output:
            json.dump(value, output, indent=2, allow_nan=False)
            output.write('\n')
            output.flush()
            os.fsync(output.fileno())
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--expect-version', required=True)
    parser.add_argument('--scenario', choices=SCENARIOS, required=True)
    parser.add_argument('--output', type=Path, required=True, help='new private directory')
    parser.add_argument('--restart-for-latency', action='store_true', help='authorize one prompted start-limit reset and service restart per run')
    args = parser.parse_args()
    if not args.restart_for_latency:
        parser.error('--restart-for-latency is required: legacy latency samples cannot otherwise be scoped to each run')
    try:
        args.output.mkdir(mode=0o700)
        runs = []
        first_identity = None
        expected = identity(args.expect_version)
        session = {'scenario': args.scenario, 'accepted_for_m0': False, 'reference_pi_managed': True,
                   'status': 'INCOMPLETE', 'target_seconds': 60, 'target_repeats': 3,
                   'interval_seconds': 1, 'runs': runs}
        checkpoint(args.output / 'session-bound.json', session)
        for number in (1, 2, 3):
            print(f'Run {number}/3: {args.scenario}. Prepare the workload/settings first.')
            input('Press Enter to reset the start counter and restart roundos.service, then perform this workload: ')
            current = identity(args.expect_version)
            if any(current[key] != expected[key] for key in RELEASE_KEYS):
                raise ValueError('Managed release changed while preparing the run')
            armed = time.clock_gettime(time.CLOCK_BOOTTIME)
            # Deliberate benchmark restarts would otherwise exhaust the unit's
            # five-start limit across consecutive scenarios. Never reset an
            # unhealthy service: identity() above requires an active shell.
            subprocess.run(['sudo', '/usr/bin/systemctl', 'reset-failed', 'roundos.service'], check=True)
            subprocess.run(['sudo', '/usr/bin/systemctl', 'restart', 'roundos.service'], check=True)
            before = identity(args.expect_version)
            if any(before[key] != expected[key] for key in RELEASE_KEYS):
                raise ValueError('Managed release changed before restart')
            if before['service_start_boottime_seconds'] < armed:
                raise ValueError('Managed shell did not start inside this run window')
            if first_identity and any(before[key] != first_identity[key] for key in RELEASE_KEYS):
                raise ValueError('Managed release changed between runs')
            if first_identity is None:
                first_identity = before
                write_new(args.output / 'before.json', before)
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
            run['renderer_metrics'] = []
            write_new(directory / 'session-bound.json', bound)
            runs.append(run)
            checkpoint(args.output / 'session-bound.json', session)
            run['renderer_metrics'] = latency.collect(before, run['start_utc'], run['end_utc'])
            checkpoint(args.output / 'session-bound.json', session)
            if not run['renderer_metrics']:
                raise ValueError('No latency summary from the new managed process')
        session.update(accepted_for_m0=True, status='COLLECTED')
        checkpoint(args.output / 'session-bound.json', session)
    except (OSError, ValueError, KeyError, TypeError, EOFError, KeyboardInterrupt, subprocess.SubprocessError) as error:
        parser.exit(1, f'M7 collection incomplete: {type(error).__name__}; existing evidence retained.\n')
    print('M7 evidence:', args.output / 'session-bound.json')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

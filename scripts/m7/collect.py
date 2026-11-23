#!/usr/bin/env python3
"""Collect one interactive M7 scenario with verified managed identities.

Runs only read-only collectors. sudo is used to inspect the managed process;
all evidence files are created by the caller, with private permissions.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

from bind_session import bind
from compare import ROOT, SCENARIOS, valid_identity, write_new


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
    args = parser.parse_args()
    try:
        args.output.mkdir(mode=0o700)
        before = identity(args.expect_version)
        write_new(args.output / 'before.json', before)
        result = subprocess.run([sys.executable, str(ROOT / 'scripts/m0/collect_pi.py'),
                                 '--scenario', args.scenario, '--output', str(args.output / 'samples')], check=False)
        after = identity(args.expect_version)
        write_new(args.output / 'after.json', after)
        if result.returncode:
            raise ValueError('Sampler did not finish all runs')
        session = bind(args.output / 'samples/session.json', before, after)
        if session.get('accepted_for_m0') is not True:
            raise ValueError('Timing or sampling evidence is incomplete')
        write_new(args.output / 'session-bound.json', session)
    except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
        parser.exit(1, f'M7 collection incomplete: {type(error).__name__}; existing evidence retained.\n')
    print('M7 evidence:', args.output / 'session-bound.json')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())

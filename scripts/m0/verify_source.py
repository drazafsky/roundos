#!/usr/bin/env python3
"""Verify unchanged RoundOS 0.5.0 inputs for additive RUI milestone branches.

Checks every original file against the immutable 0.5.0 archive manifest.
Approved M0/M1/M2 additions are allowed, but a missing/modified baseline file fails.
"""
import argparse
import hashlib
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
MANIFEST = Path(__file__).with_name('BASELINE-SOURCE.SHA256')
ARCHIVE_SHA = '06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75'


def is_generated_binary(path):
    """Accept only root-level ELF outputs from the official 0.5.0 build."""
    if path.is_symlink() or not path.is_file():
        return False
    try:
        with path.open('rb') as binary:
            return binary.read(4) == b'\x7fELF'
    except OSError:
        return False


def verify(root=ROOT, manifest=MANIFEST, archive=None):
    errors = []
    entries = []
    for row in manifest.read_text(encoding='ascii').splitlines():
        expected, rel = row.split('  ', 1)
        entries.append(rel)
        path = root / rel
        if not path.is_file():
            errors.append(f'MISSING {rel}')
        elif hashlib.sha256(path.read_bytes()).hexdigest() != expected:
            errors.append(f'MODIFIED {rel}')
    if len(set(entries)) != len(entries):
        errors.append('manifest contains duplicate paths')
    if archive is not None:
        if not archive.is_file() or hashlib.sha256(archive.read_bytes()).hexdigest() != ARCHIVE_SHA:
            errors.append('original 0.5.0 ZIP SHA-256 mismatch')
    # Additional SOURCE files are limited to milestone-owned directories.
    # Official 0.5.0 build output is a narrow, verified ELF allowlist. Never
    # treat arbitrary new files or modified 0.5.0 source as acceptable.
    generated_binaries = {'roundd', 'round-clock', 'round-timer'}
    extras = []
    for p in root.rglob('*'):
        if not p.is_file():
            continue
        rel = p.relative_to(root).as_posix()
        if rel in entries or '__pycache__/' in rel or rel.endswith('.pyc') or rel.startswith('.git/'):
            continue
        if rel in generated_binaries and not p.is_symlink() and is_generated_binary(p):
            continue
        allowed = ('scripts/m0/', 'docs/rui-m0/',
                   'internal/rui/', 'sdk/rui/', 'schemas/',
                   'docs/rui-m1/', 'scripts/m1/', 'layouts/',
                   'cmd/roundui/', 'docs/rui-m2/', 'scripts/m2/')
        if not any(rel.startswith(prefix) for prefix in allowed):
            extras.append(rel)
    errors.extend('UNEXPECTED ' + rel for rel in extras)
    return len(entries), errors


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--archive', type=Path, help='optional original roundos-0.5.0.zip')
    a = ap.parse_args()
    count, errors = verify(archive=a.archive)
    for error in errors:
        print(error, file=sys.stderr)
    print(f'0.5.0 source provenance: {count} original files, {len(errors)} error(s)')
    return 1 if errors else 0


if __name__ == '__main__':
    raise SystemExit(main())

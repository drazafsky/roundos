#!/usr/bin/env python3
"""Print allowlisted, read-only identity for the managed Pi release.

Run with sudo when roundd is root-owned, redirecting stdout from the caller's
shell into a private evidence file. No process arguments or credentials are read.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import stat
import subprocess


def command(args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=8, check=False)
    return result.stdout.strip() if result.returncode == 0 else None


def digest(path):
    value = hashlib.sha256()
    with path.open('rb') as source:
        for data in iter(lambda: source.read(1 << 20), b''):
            value.update(data)
    return value.hexdigest()


def scan(expected_version):
    model = Path('/proc/device-tree/model').read_bytes().replace(b'\0', b'').decode('ascii', 'replace').strip()
    release = Path('/opt/roundos/current').resolve(strict=True)
    if release.parent != Path('/opt/roundos/releases'):
        raise ValueError('Managed release is outside the trusted releases directory')
    binary = release / 'bin/roundd'
    timer = release / 'bin/round-timer'
    installed = binary.lstat()
    timer_stat = timer.lstat()
    trusted_binary = stat.S_ISREG(installed.st_mode) and installed.st_uid == 0 and not installed.st_mode & 0o022
    trusted_timer = stat.S_ISREG(timer_stat.st_mode) and timer_stat.st_uid == 0 and not timer_stat.st_mode & 0o022
    # Check file and parent ownership before invoking a binary under sudo.
    for directory in (release, binary.parent, Path('/opt/roundos/releases'), Path('/opt/roundos'), Path('/opt')):
        info = directory.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('Untrusted managed release directory')
    if not trusted_binary or not trusted_timer:
        raise ValueError('Untrusted installed executable')
    version_line = command([str(binary), '-version'])
    match = re.fullmatch(r'RoundOS ([0-9A-Za-z.+-]+) \(([0-9A-Za-z.+-]+)\)', version_line or '')
    if not match:
        raise ValueError('Installed binary did not report valid version metadata')
    version, build = match.groups()
    metadata = {}
    if (release / 'release.json').exists():
        data = (release / 'release.json').read_bytes()
        if len(data) > 8192:
            raise ValueError('Oversized release metadata')
        metadata = json.loads(data)
    pid_text = command(['systemctl', 'show', '-p', 'MainPID', '--value', 'roundos.service']) or ''
    pid = int(pid_text) if pid_text.isdecimal() else 0
    running_matches = False
    try:
        running = Path(f'/proc/{pid}/exe').stat()
        running_matches = (running.st_dev, running.st_ino) == (installed.st_dev, installed.st_ino)
    except OSError:
        pass
    declared_version = metadata.get('version')
    declared_commit = metadata.get('commit')
    metadata_matches = (not metadata or declared_version == version and declared_commit == build)
    active = command(['systemctl', 'is-active', 'roundos.service']) == 'active'
    reference = 'Raspberry Pi Zero 2' in model
    result = {
        'recorded_utc': datetime.now(timezone.utc).isoformat(),
        'device_model': model,
        'device_id_sha256': digest(Path('/etc/machine-id')),
        'boot_id': Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
        'release': str(release), 'version': version, 'build': build,
        'commit': declared_commit if isinstance(declared_commit, str) else build if re.fullmatch(r'[0-9a-f]{40}', build) else None,
        'binary_sha256': digest(binary), 'service_pid': pid,
        'timer_binary_sha256': digest(timer), 'root_owned_regular_timer': trusted_timer,
        'reference_pi': reference, 'service_active': active,
        'root_owned_regular_binary': trusted_binary,
        'running_binary_matches': running_matches,
        'release_metadata_matches': metadata_matches,
        'expected_version_matches': version == expected_version,
    }
    result['identity_verified'] = all((reference, active, trusted_binary, trusted_timer, running_matches, metadata_matches, version == expected_version))
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--expect-version', required=True)
    args = parser.parse_args()
    try:
        result = scan(args.expect_version)
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        parser.exit(1, f'Identity collection failed: {type(error).__name__}\n')
    print(json.dumps(result, indent=2))
    return 0 if result['identity_verified'] else 1


if __name__ == '__main__':
    raise SystemExit(main())

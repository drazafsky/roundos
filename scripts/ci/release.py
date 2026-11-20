#!/usr/bin/env python3
"""Build reproducible ARMv7 packages and publish verified GitHub prereleases."""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
BINARIES = ('roundd', 'round-clock', 'round-timer', 'round-demo', 'roundui')


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate(version, sha):
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+-dev\.[1-9][0-9]*\.g[0-9a-f]{7}', version):
        raise ValueError('Expected a generated development prerelease version')
    if not re.fullmatch(r'[0-9a-f]{40}', sha) or not version.endswith('.g' + sha[:7]):
        raise ValueError('Release version and full commit SHA must agree')


def version():
    base = (ROOT / 'VERSION').read_text().strip()
    number = os.environ['GITHUB_RUN_NUMBER']
    sha = os.environ['GITHUB_SHA']
    result = f'{base}-dev.{number}.g{sha[:7]}'
    validate(result, sha)
    return result


def write_checksums(directory, paths):
    (directory / 'SHA256SUMS').write_text(''.join(
        f'{digest(directory / path)}  {path}\n' for path in sorted(paths)))


def package(build, output, release_version, sha):
    validate(release_version, sha)
    if (build / 'RELEASE_VERSION').read_text().strip() != release_version:
        raise ValueError('Build metadata does not match release version')
    for name in BINARIES:
        path = build / name
        header = path.read_bytes()[:20]
        if path.is_symlink() or header[:6] != b'\x7fELF\x01\x01' or header[18:20] != b'\x28\x00':
            raise ValueError(f'{name} is not a regular little-endian 32-bit ARM ELF')
    output.mkdir(parents=True, exist_ok=True)
    metadata = json.dumps({'version': release_version, 'commit': sha,
                           'target': 'linux-armv7', 'hardware_validated': False},
                          sort_keys=True, indent=2) + '\n'
    name = f'roundos-{release_version}-linux-armv7'
    with tempfile.TemporaryDirectory() as tmp:
        stage = Path(tmp) / name
        stage.mkdir()
        for binary in BINARIES:
            shutil.copyfile(build / binary, stage / binary)
            (stage / binary).chmod(0o755)
        for directory in ('config', 'docs', 'layouts', 'schemas', 'systemd'):
            shutil.copytree(ROOT / directory, stage / directory)
        (stage / 'scripts').mkdir()
        for script in ('install-release.sh', 'roundosctl.sh', 'setup-app-user.sh'):
            shutil.copyfile(ROOT / 'scripts' / script, stage / 'scripts' / script)
            (stage / 'scripts' / script).chmod(0o755)
        shutil.copyfile(ROOT / 'README.md', stage / 'README.md')
        (stage / 'RELEASE_VERSION').write_text(release_version + '\n')
        (stage / 'release.json').write_text(metadata)
        paths = [p.relative_to(stage).as_posix() for p in stage.rglob('*') if p.is_file()]
        write_checksums(stage, paths)
        # Fixed metadata and gzip timestamp make identical inputs byte identical.
        with (output / (name + '.tar.gz')).open('wb') as raw:
            with gzip.GzipFile(filename='', mode='wb', fileobj=raw, mtime=0) as zipped:
                with tarfile.open(fileobj=zipped, mode='w') as archive:
                    for path in [stage] + sorted(stage.rglob('*')):
                        info = archive.gettarinfo(str(path), path.relative_to(stage.parent).as_posix())
                        info.uid = info.gid = info.mtime = 0
                        info.uname = info.gname = ''
                        info.mode = 0o755 if path.is_dir() or path.name in BINARIES or path.suffix == '.sh' else 0o644
                        archive.addfile(info, io.BytesIO(path.read_bytes()) if info.isfile() else None)
    (output / 'release.json').write_text(metadata)
    write_checksums(output, [name + '.tar.gz', 'release.json'])


def verify_assets(output, release_version, sha):
    validate(release_version, sha)
    name = f'roundos-{release_version}-linux-armv7'
    expected = {name + '.tar.gz', 'release.json'}
    rows = (output / 'SHA256SUMS').read_text().splitlines()
    checksums = {}
    for row in rows:
        checksum, path = row.split('  ', 1)
        if path not in expected or not re.fullmatch('[0-9a-f]{64}', checksum) or path in checksums:
            raise ValueError('Invalid package checksums')
        if digest(output / path) != checksum:
            raise ValueError(f'Checksum mismatch: {path}')
        checksums[path] = checksum
    if checksums.keys() != expected:
        raise ValueError('Missing package checksum')
    metadata = json.loads((output / 'release.json').read_text())
    if metadata != {'version': release_version, 'commit': sha, 'target': 'linux-armv7', 'hardware_validated': False}:
        raise ValueError('Release metadata mismatch')
    # Inspect without extracting or executing untrusted archive members.
    with tarfile.open(output / (name + '.tar.gz')) as archive:
        members = {m.name: m for m in archive.getmembers()}
        def read(path):
            member = members[name + '/' + path]
            if not member.isfile():
                raise ValueError('Package contains a non-regular required file')
            return archive.extractfile(member).read()
        if read('RELEASE_VERSION') != (release_version + '\n').encode() or read('release.json') != (output / 'release.json').read_bytes():
            raise ValueError('Archive identity mismatch')
        for row in read('SHA256SUMS').decode().splitlines():
            checksum, path = row.split('  ', 1)
            if hashlib.sha256(read(path)).hexdigest() != checksum:
                raise ValueError(f'Archive checksum mismatch: {path}')
    return sorted(expected | {'SHA256SUMS'})


def gh(*args):
    return subprocess.run(['gh', *args], check=True, capture_output=True, text=True).stdout


def api(path, missing_ok=False):
    result = subprocess.run(['gh', 'api', path], capture_output=True, text=True)
    if result.returncode:
        if missing_ok and '(HTTP 404)' in result.stderr:
            return None
        raise RuntimeError(result.stderr)
    return json.loads(result.stdout)


def publish(output, release_version, sha):
    assets = verify_assets(output, release_version, sha)
    repo = os.environ['GH_REPO']
    tag = 'v' + release_version
    prefix = f'repos/{repo}'
    release = api(f'{prefix}/releases/tags/{tag}', missing_ok=True)
    commit = api(f'{prefix}/commits/{tag}', missing_ok=True)
    if commit and commit['sha'] != sha:
        raise ValueError('Existing tag points at a different commit')
    if release:
        if release['target_commitish'] != sha or not release['prerelease']:
            raise ValueError('Existing release has different identity or release type')
        if not release['draft']:
            if not commit or {a['name'] for a in release['assets']} != set(assets):
                raise ValueError('Published release is incomplete')
            with tempfile.TemporaryDirectory() as tmp:
                gh('release', 'download', tag, '--dir', tmp)
                # A full workflow rerun may rebuild; never replace published bytes.
                for asset in assets:
                    if digest(Path(tmp) / asset) != digest(output / asset):
                        raise ValueError(f'Published asset differs: {asset}')
            print(f'Already published and verified {tag}')
            return
    else:
        gh('release', 'create', tag, '--draft', '--prerelease', '--latest=false',
           '--target', sha, '--title', f'RoundOS {release_version}', '--generate-notes',
           '--notes', 'Development ARMv7 build. Physical Pi display, touch, Wi-Fi, boot, recovery and performance acceptance remain pending. Installation and activation are manual. See docs/CI-RELEASES.md in the package.')
    # Partial drafts may be resumed. Published releases are never overwritten.
    gh('release', 'upload', tag, *[str(output / a) for a in assets], '--clobber')
    with tempfile.TemporaryDirectory() as tmp:
        gh('release', 'download', tag, '--dir', tmp)
        for asset in assets:
            if digest(Path(tmp) / asset) != digest(output / asset):
                raise ValueError(f'Uploaded asset differs: {asset}')
    gh('release', 'edit', tag, '--draft=false', '--prerelease', '--latest=false')
    # Confirm GitHub created the tag at the exact tested merge commit.
    if api(f'{prefix}/commits/{tag}')['sha'] != sha:
        raise ValueError('Published tag does not match tested commit')
    print(f'Published {tag}; hardware acceptance remains pending')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    sub.add_parser('version')
    for command in ('package', 'publish', 'verify'):
        p = sub.add_parser(command)
        p.add_argument('--version', required=True)
        p.add_argument('--sha', required=True)
        p.add_argument('--output', type=Path, required=True)
        if command == 'package':
            p.add_argument('--build', type=Path, required=True)
    args = parser.parse_args()
    if args.command == 'version':
        print(version())
    elif args.command == 'package':
        package(args.build, args.output, args.version, args.sha)
        verify_assets(args.output, args.version, args.sha)
    elif args.command == 'verify':
        verify_assets(args.output, args.version, args.sha)
    else:
        publish(args.output, args.version, args.sha)


if __name__ == '__main__':
    main()

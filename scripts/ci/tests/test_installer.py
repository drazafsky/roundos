"""Exercise installer identity checks before any installation can occur."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
VERSION = '0.5.1-dev.42.gaaaaaaa'


class InstallerIdentityTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        (self.root / 'scripts').mkdir()
        shutil.copyfile(ROOT / 'scripts/install-release.sh', self.root / 'scripts/install-release.sh')
        self.mockbin = self.root / 'mockbin'
        self.mockbin.mkdir()
        self.command('id', 'echo 0')
        # Stop a matching build before touching service files or installation roots.
        self.command('systemctl', 'echo CALLED_SYSTEMCTL; exit 0')
        for name in ('roundd', 'round-clock', 'round-timer'):
            path = self.root / name
            path.write_text('#!/bin/sh\necho "RoundOS ' + VERSION + ' (source)"\n')
            path.chmod(0o755)
        self.metadata = self.root / 'RELEASE_VERSION'
        self.metadata.write_text(VERSION + '\n')

    def command(self, name, body):
        path = self.mockbin / name
        path.write_text('#!/bin/sh\n' + body + '\n')
        path.chmod(0o755)

    def run_installer(self):
        env = dict(os.environ, PATH=str(self.mockbin) + os.pathsep + os.environ['PATH'])
        return subprocess.run(['sh', str(self.root / 'scripts/install-release.sh')],
                              env=env, capture_output=True, text=True, check=False)

    def test_matching_version_reaches_existing_service_safety_gate(self):
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Refusing to change installed release', result.stderr)

    def test_wrong_binary_version_fails_before_service_access(self):
        self.metadata.write_text('0.5.0\n')
        result = self.run_installer()
        self.assertIn('Wrong roundd build', result.stderr)
        self.assertNotIn('CALLED_SYSTEMCTL', result.stdout)

    def test_missing_symlink_or_unsafe_metadata_rejected(self):
        self.metadata.unlink()
        result = self.run_installer()
        self.assertIn('Missing or untrusted RELEASE_VERSION', result.stderr)
        self.metadata.symlink_to(self.root / 'roundd')
        result = self.run_installer()
        self.assertIn('Missing or untrusted RELEASE_VERSION', result.stderr)
        self.metadata.unlink()
        self.metadata.write_text('../../escape\n')
        result = self.run_installer()
        self.assertIn('Invalid release version', result.stderr)


if __name__ == '__main__':
    unittest.main()

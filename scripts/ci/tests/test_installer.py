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
        script = (ROOT / 'scripts/install-release.sh').read_text()
        script = script.replace('/etc/systemd/system/roundos.service', str(self.root / 'installed.service'))
        script = script.replace('/opt/roundos', str(self.root / 'managed'))
        (self.root / 'scripts/install-release.sh').write_text(script)
        (self.root / 'systemd').mkdir()
        shutil.copyfile(ROOT / 'systemd/roundos.service', self.root / 'systemd/roundos.service')
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

    def test_original_managed_simple_unit_can_be_upgraded(self):
        legacy = (ROOT / 'systemd/roundos.service').read_text().replace(
            'Type=notify\nNotifyAccess=main\nTimeoutStartSec=45s', 'Type=simple')
        legacy = legacy.replace('StartLimitIntervalSec=300', 'StartLimitIntervalSec=120')
        (self.root / 'installed.service').write_text(legacy)
        result = self.run_installer()
        self.assertIn('Refusing to change installed release while', result.stderr)
        self.assertNotIn('manually edited unit', result.stderr)

    def test_custom_unit_is_not_overwritten(self):
        (self.root / 'installed.service').write_text('[Service]\nType=exec\n# local customization\n')
        result = self.run_installer()
        self.assertIn('refusing to replace a manually edited unit', result.stderr)

    def test_stored_managed_template_can_be_upgraded(self):
        installed = '[Service]\nType=notify\n# prior managed template\n'
        previous = self.root / 'managed/releases/prior'
        previous.mkdir(parents=True)
        (previous / 'roundos.service').write_text(installed)
        (self.root / 'managed/current').symlink_to(previous)
        (self.root / 'installed.service').write_text(installed)
        result = self.run_installer()
        self.assertIn('Refusing to change installed release while', result.stderr)

    def test_symlinked_stored_unit_does_not_authorize_overwrite(self):
        previous = self.root / 'managed/releases/prior'
        previous.mkdir(parents=True)
        installed = self.root / 'installed.service'
        installed.write_text('[Service]\n# custom\n')
        (previous / 'roundos.service').symlink_to(installed)
        (self.root / 'managed/current').symlink_to(previous)
        result = self.run_installer()
        self.assertIn('refusing to replace a manually edited unit', result.stderr)


if __name__ == '__main__':
    unittest.main()

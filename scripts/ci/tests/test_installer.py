"""Exercise installer identity checks before any installation can occur."""
import os
import hashlib
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
        self.systemctl_calls = self.root / 'systemctl.calls'
        self.command('id', 'echo 0')
        # Stop a matching build before touching service files or installation roots.
        self.command('systemctl', 'echo active; exit 0')
        for name in ('roundd', 'round-clock', 'round-timer'):
            path = self.root / name
            path.write_text('#!/bin/sh\necho "RoundOS ' + VERSION + ' (source)"\n')
            path.chmod(0o755)
        self.metadata = self.root / 'RELEASE_VERSION'
        self.metadata.write_text(VERSION + '\n')

    def command(self, name, body):
        path = self.mockbin / name
        if name == 'systemctl':
            body = 'printf "%s\\n" "$*" >> "$SYSTEMCTL_CALL_LOG"\n' + body
        path.write_text('#!/bin/sh\n' + body + '\n')
        path.chmod(0o755)

    def run_installer(self):
        env = dict(os.environ, PATH=str(self.mockbin) + os.pathsep + os.environ['PATH'],
                   SYSTEMCTL_CALL_LOG=str(self.systemctl_calls))
        return subprocess.run(['sh', str(self.root / 'scripts/install-release.sh')],
                              env=env, capture_output=True, text=True, check=False)

    def record_template(self, directory, content):
        directory.mkdir(parents=True, exist_ok=True)
        (directory / 'roundos.service').write_text(content)
        checksum = hashlib.sha256(content.encode()).hexdigest()
        (directory / 'SHA256SUMS').write_text(checksum + '  roundos.service\n')

    def test_matching_version_reaches_existing_service_safety_gate(self):
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Refusing to change installed release', result.stderr)
        self.assertEqual(self.systemctl_calls.read_text().splitlines(),
                         ['is-active roundos.service'])

    def test_wrong_binary_version_fails_before_service_access(self):
        self.metadata.write_text('0.5.0\n')
        result = self.run_installer()
        self.assertIn('Wrong roundd build', result.stderr)
        self.assertFalse(self.systemctl_calls.exists(), 'systemctl accessed before identity validation')

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
        legacy = legacy.replace('StartLimitIntervalSec=360', 'StartLimitIntervalSec=120')
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
        self.record_template(previous, installed)
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

    def test_previous_verified_template_recognized_after_rollback(self):
        older = self.root / 'managed/releases/older'
        newer = self.root / 'managed/releases/newer'
        self.record_template(older, '[Service]\n# older managed template\n')
        installed = '[Service]\n# newer managed template still installed after rollback\n'
        self.record_template(newer, installed)
        (self.root / 'managed/current').symlink_to(older)
        (self.root / 'managed/previous').symlink_to(newer)
        (self.root / 'installed.service').write_text(installed)
        result = self.run_installer()
        self.assertIn('Refusing to change installed release while', result.stderr)
        self.assertNotIn('manually edited unit', result.stderr)

    def test_modified_snapshot_does_not_authorize_unit_overwrite(self):
        previous = self.root / 'managed/releases/prior'
        self.record_template(previous, '[Service]\n# original\n')
        changed = '[Service]\n# modified after recording checksum\n'
        (previous / 'roundos.service').write_text(changed)
        (self.root / 'managed/previous').symlink_to(previous)
        (self.root / 'installed.service').write_text(changed)
        result = self.run_installer()
        self.assertIn('refusing to replace a manually edited unit', result.stderr)

    def test_transitional_or_unknown_services_refuse_installation(self):
        for state in ('activating', 'deactivating', 'reloading', 'maintenance', ''):
            with self.subTest(state=state):
                self.command('systemctl', 'printf "%s\\n" "' + state + '"; exit 3')
                result = self.run_installer()
                self.assertIn('must be inactive or failed', result.stderr)
                self.assertFalse((self.root / 'managed/releases' / VERSION).exists())

    def test_unchecksummed_unit_copy_does_not_authorize_overwrite(self):
        previous = self.root / 'managed/releases/prior'
        installed = '[Service]\n# unverified unit copy\n'
        self.record_template(previous, installed)
        (previous / 'other.txt').write_text('verified unrelated file')
        checksum = hashlib.sha256(b'verified unrelated file').hexdigest()
        (previous / 'SHA256SUMS').write_text(checksum + '  other.txt\n')
        (self.root / 'managed/previous').symlink_to(previous)
        (self.root / 'installed.service').write_text(installed)
        result = self.run_installer()
        self.assertIn('refusing to replace a manually edited unit', result.stderr)

    def test_valid_unit_snapshot_allows_repair_of_other_corrupt_files(self):
        previous = self.root / 'managed/releases/prior'
        installed = '[Service]\n# verified managed unit\n'
        self.record_template(previous, installed)
        (previous / 'broken.bin').write_text('corrupt binary')
        with (previous / 'SHA256SUMS').open('a') as manifest:
            manifest.write('0' * 64 + '  broken.bin\n')
        (self.root / 'managed/current').symlink_to(previous)
        (self.root / 'installed.service').write_text(installed)
        result = self.run_installer()
        self.assertIn('Refusing to change installed release while', result.stderr)
        self.assertNotIn('manually edited unit', result.stderr)

    def test_inactive_and_failed_services_pass_installation_gate(self):
        setup = self.root / 'scripts/setup-app-user.sh'
        setup.write_text('#!/bin/sh\necho REACHED_SETUP >&2\nexit 97\n')
        setup.chmod(0o755)
        for state in ('inactive', 'failed'):
            with self.subTest(state=state):
                self.command('systemctl', 'echo ' + state + '; exit 3')
                result = self.run_installer()
                self.assertEqual(result.returncode, 97)
                self.assertIn('REACHED_SETUP', result.stderr)


if __name__ == '__main__':
    unittest.main()

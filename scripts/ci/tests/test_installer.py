"""Exercise installer identity checks before any installation can occur."""
import os
import fcntl
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
        script = script.replace('/etc/systemd/system', str(self.root / 'installed-units'))
        script = script.replace('/opt/roundos', str(self.root / 'managed'))
        script = script.replace('/var/lib/roundos', str(self.root / 'state'))
        (self.root / 'scripts/install-release.sh').write_text(script)
        (self.root / 'systemd').mkdir()
        for name in ('roundos.service', 'roundos-update.service', 'roundos-update-recover.service'):
            shutil.copyfile(ROOT / 'systemd' / name, self.root / 'systemd' / name)
        self.mockbin = self.root / 'mockbin'
        self.mockbin.mkdir()
        self.systemctl_calls = self.root / 'systemctl.calls'
        self.command('id', 'echo 0')
        # Stop a matching build before touching service files or installation roots.
        self.command('systemctl', 'echo active; exit 0')
        for name in ('roundd', 'round-clock', 'round-timer'):
            path = self.root / name
            path.write_text('#!/bin/sh\ncase "$1" in -update-protocol) echo roundos-update-v1;; *) echo "RoundOS ' + VERSION + ' (source)";; esac\n')
            path.chmod(0o755)
        self.metadata = self.root / 'RELEASE_VERSION'
        self.metadata.write_text(VERSION + '\n')

    def command(self, name, body):
        path = self.mockbin / name
        if name == 'systemctl':
            body = 'printf "%s\\n" "$*" >> "$SYSTEMCTL_CALL_LOG"\n' + body
        path.write_text('#!/bin/sh\n' + body + '\n')
        path.chmod(0o755)

    def run_installer(self, **extra_env):
        env = dict(os.environ, PATH=str(self.mockbin) + os.pathsep + os.environ['PATH'],
                   SYSTEMCTL_CALL_LOG=str(self.systemctl_calls), **extra_env)
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
        legacy = legacy.replace('Requires=roundos-update-recover.service\n', '').replace(' roundos-update-recover.service', '')
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

    def complete_install_fixture(self):
        script = self.root / 'scripts/install-release.sh'
        text = script.read_text()
        for real, local in (('/etc/roundos', 'etc-roundos'), ('/var/lib/roundos', 'state'),
                            ('/usr/local/sbin', 'localbin'), ('/root/.local/state/roundos', 'old-state')):
            text = text.replace(real, str(self.root / local))
        script.write_text(text)
        (self.root / 'installed-units').mkdir()
        (self.root / 'localbin').mkdir()
        (self.root / 'config').mkdir()
        (self.root / 'config/gestures.example.json').write_text('{}')
        (self.root / 'README.md').write_text('fixture')
        setup = self.root / 'scripts/setup-app-user.sh'
        setup.write_text('#!/bin/sh\nexit 0\n')
        setup.chmod(0o755)
        (self.root / 'scripts/roundosctl.sh').write_text('#!/bin/sh\n')
        self.command('systemctl', 'if [ "$1" = is-active ]; then echo inactive; exit 3; fi')
        # Run the real installer in private temporary paths without root chown.
        wrapper = self.root / 'install-wrapper.py'
        wrapper.write_text('import os,sys\nargs=iter(sys.argv[1:]); kept=[]\n'
                           'for arg in args:\n'
                           ' if arg in ("-o","-g"): next(args)\n'
                           ' else: kept.append(arg)\n'
                           'os.execv("/usr/bin/install", ["install"]+kept)\n')
        self.command('install', 'exec python3 "' + str(wrapper) + '" "$@"')
        self.sync_log = self.root / 'sync.calls'
        self.command('sync', 'printf "%s\\n" "$*" >> "' + str(self.sync_log) + '"')

    def test_complete_manual_install_sets_controller_and_all_units(self):
        self.complete_install_fixture()
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = self.root / 'managed/releases' / VERSION
        self.assertEqual((self.root / 'managed/update-controller').resolve(), dest)
        self.assertEqual((self.root / 'installed.service').read_bytes(),
                         (self.root / 'systemd/roundos.service').read_bytes())
        for name in ('roundos-update.service', 'roundos-update-recover.service'):
            self.assertEqual((self.root / 'installed-units' / name).read_bytes(),
                             (self.root / 'systemd' / name).read_bytes())
        self.assertIn('-f ' + str(dest), self.sync_log.read_text().splitlines())
        self.assertEqual(self.systemctl_calls.read_text().splitlines(),
                         ['is-active roundos.service', 'daemon-reload'])

    def test_managed_transaction_defers_unit_and_controller_commit(self):
        self.complete_install_fixture()
        installed = self.root / 'installed.service'
        original = (self.root / 'systemd/roundos.service').read_bytes()
        installed.write_bytes(original)
        result = self.run_installer(ROUNDOS_UPDATE_TRANSACTION='1',
                                    ROUNDOS_UPDATE_STAGE=str(self.root / 'managed/releases' / ('.install-update-' + 'a' * 32)))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(installed.read_bytes(), original)
        self.assertFalse((self.root / 'managed/update-controller').exists())
        self.assertFalse((self.root / 'installed-units/roundos-update.service').exists())
        self.assertEqual(self.systemctl_calls.read_text().splitlines(), ['is-active roundos.service'])

    def test_manual_install_obeys_the_managed_transaction_lock(self):
        self.complete_install_fixture()
        state = self.root / 'state/shell-data'
        state.mkdir(parents=True)
        with (state / 'update-install.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Another update installer is running', result.stderr)
        self.assertFalse((self.root / 'managed/releases' / VERSION).exists())

    def test_previous_v1_controller_environment_still_installs(self):
        self.complete_install_fixture()
        result = self.run_installer(ROUNDOS_UPDATE_TRANSACTION='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.root / 'managed/current').resolve(), self.root / 'managed/releases' / VERSION)
        self.assertFalse((self.root / 'managed/update-controller').exists())


if __name__ == '__main__':
    unittest.main()

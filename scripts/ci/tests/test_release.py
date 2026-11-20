"""Offline package and publishing contract tests; no GitHub or hardware writes."""
import importlib.util
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location('release_ci', ROOT / 'scripts/ci/release.py')
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
SHA = 'a' * 40
VERSION = '0.5.1-dev.42.gaaaaaaa'


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.build = self.root / 'build'
        self.build.mkdir()
        # Minimal architecture fixtures, not executable substitutes for CI builds.
        elf = bytearray(64)
        elf[:6] = b'\x7fELF\x01\x01'
        elf[18:20] = b'\x28\x00'
        for name in release.BINARIES:
            (self.build / name).write_bytes(elf)
        (self.build / 'RELEASE_VERSION').write_text(VERSION + '\n')
        self.output = self.root / 'dist'

    def package(self):
        release.package(self.build, self.output, VERSION, SHA)
        return release.verify_assets(self.output, VERSION, SHA)

    def test_version_stable_on_rerun_and_unique_per_merge(self):
        with mock.patch.dict(os.environ, GITHUB_RUN_NUMBER='42', GITHUB_SHA=SHA):
            self.assertEqual(release.version(), VERSION)
            with mock.patch.dict(os.environ, GITHUB_RUN_ATTEMPT='2'):
                self.assertEqual(release.version(), VERSION)
            with mock.patch.dict(os.environ, GITHUB_RUN_NUMBER='43'):
                self.assertNotEqual(release.version(), VERSION)

    def test_rejects_wrong_commit_or_unsafe_version(self):
        for version, sha in [(VERSION, 'b' * 40), ('../../bad', SHA), (VERSION, 'a' * 7)]:
            with self.assertRaises(ValueError):
                release.validate(version, sha)

    def test_package_reproducible_and_installable(self):
        assets = self.package()
        before = {name: release.digest(self.output / name) for name in assets}
        os.utime(self.build / 'roundd', (1234, 5678))
        self.package()
        self.assertEqual(before, {name: release.digest(self.output / name) for name in assets})
        import tarfile
        archive = next(self.output.glob('*.tar.gz'))
        with tarfile.open(archive) as tar:
            prefix = archive.name.removesuffix('.tar.gz')
            for name in release.BINARIES + ('scripts/install-release.sh', 'scripts/setup-app-user.sh', 'scripts/roundosctl.sh'):
                self.assertEqual(tar.getmember(prefix + '/' + name).mode, 0o755)
            self.assertIn(prefix + '/config/gestures.example.json', tar.getnames())
            self.assertIn(prefix + '/systemd/roundos.service', tar.getnames())

    def test_rejects_non_arm_binary_and_stale_version(self):
        (self.build / 'roundd').write_bytes(b'not an ARM ELF')
        with self.assertRaises(ValueError):
            self.package()
        (self.build / 'RELEASE_VERSION').write_text('0.5.0')
        with self.assertRaises(ValueError):
            self.package()

    def test_rejects_corrupt_assets_and_missing_checksums(self):
        self.package()
        (self.output / 'release.json').write_text('{}')
        with self.assertRaises(ValueError):
            release.verify_assets(self.output, VERSION, SHA)
        self.package()
        (self.output / 'SHA256SUMS').write_text('')
        with self.assertRaises(ValueError):
            release.verify_assets(self.output, VERSION, SHA)

    def publish_fixture(self, existing=False, wrong_commit=False, corrupt=False):
        assets = self.package()
        published = {'target_commitish': SHA, 'prerelease': True, 'draft': False,
                     'assets': [{'name': name} for name in assets]}
        calls = []
        def api(path, missing_ok=False):
            if '/releases/tags/' in path:
                return published if existing else None
            if '/git/ref/tags/' in path:
                return {'ref': 'refs/tags/v' + VERSION} if existing or wrong_commit else None
            return {'sha': 'b' * 40 if wrong_commit else SHA}
        def gh(*args):
            calls.append(args)
            if args[:2] == ('release', 'download'):
                target = Path(args[args.index('--dir') + 1])
                for asset in assets:
                    shutil.copyfile(self.output / asset, target / asset)
                if corrupt:
                    (target / 'release.json').write_text('{}')
            return ''
        return calls, api, gh

    def test_publish_draft_upload_verify_then_publish(self):
        calls, api, gh = self.publish_fixture()
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, VERSION, SHA)
        self.assertEqual([call[:2] for call in calls], [('release', 'create'), ('release', 'upload'), ('release', 'download'), ('release', 'edit')])
        self.assertIn('--draft', calls[0])
        self.assertIn(SHA, calls[0])
        self.assertIn('--latest=false', calls[-1])

    def test_published_rerun_only_verifies_never_overwrites(self):
        calls, api, gh = self.publish_fixture(existing=True)
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, VERSION, SHA)
        self.assertEqual([call[:2] for call in calls], [('release', 'download')])

    def test_wrong_tag_or_corrupt_upload_never_publishes(self):
        for wrong_commit, corrupt in [(True, False), (False, True)]:
            calls, api, gh = self.publish_fixture(wrong_commit=wrong_commit, corrupt=corrupt)
            with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
                with self.assertRaises(ValueError):
                    release.publish(self.output, VERSION, SHA)
            self.assertNotIn(('release', 'edit'), [call[:2] for call in calls])


if __name__ == '__main__':
    unittest.main()

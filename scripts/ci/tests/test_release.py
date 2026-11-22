"""Offline package and publishing contract tests; no GitHub or hardware writes."""
import importlib.util
import json
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

    def publish_fixture(self, existing=False, wrong_commit=False, corrupt=False, draft=False, stable=False):
        version = '0.6.0' if stable else VERSION
        if stable:
            (self.build / 'RELEASE_VERSION').write_text(version + '\n')
            release.package(self.build, self.output, version, SHA, stable=True)
            assets = release.verify_assets(self.output, version, SHA, stable=True)
        else:
            assets = self.package()
        published = {'tag_name': 'v' + version, 'target_commitish': SHA, 'prerelease': not stable, 'draft': draft,
                     'assets': [{'name': name} for name in assets]}
        calls = []
        def api(path, missing_ok=False):
            if '/releases/tags/' in path:
                return published if existing and not draft else None
            if '/git/ref/tags/' in path:
                promoted = ('release', 'edit') in [call[:2] for call in calls if '--draft=false' in call]
                return {'ref': 'refs/tags/v' + version} if (existing and not draft) or wrong_commit or promoted else None
            return {'sha': 'b' * 40 if wrong_commit else SHA}
        def gh(*args):
            calls.append(args)
            if args[0] == 'api':
                # The matching draft may be on a later page, without a tag ref.
                return json.dumps([[], [published] if existing else []])
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
        self.assertEqual([call[:2] for call in calls if call[0] == 'release'], [('release', 'create'), ('release', 'upload'), ('release', 'download'), ('release', 'edit')])
        self.assertIn('--draft', calls[1])
        self.assertIn(SHA, calls[1])
        self.assertIn('--latest=false', calls[-1])

    def test_published_rerun_only_verifies_never_overwrites(self):
        calls, api, gh = self.publish_fixture(existing=True)
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, VERSION, SHA)
        self.assertEqual([call[:2] for call in calls], [('release', 'download')])

    def test_draft_on_later_page_resumed_without_creating_duplicate(self):
        calls, api, gh = self.publish_fixture(existing=True, draft=True)
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, VERSION, SHA)
        self.assertEqual(calls[0], ('api', 'repos/owner/repo/releases?per_page=100', '--paginate', '--slurp'))
        self.assertEqual([call[:2] for call in calls if call[0] == 'release'], [('release', 'upload'), ('release', 'download'), ('release', 'edit')])

    def test_wrong_tag_or_corrupt_upload_never_publishes(self):
        for wrong_commit, corrupt in [(True, False), (False, True)]:
            calls, api, gh = self.publish_fixture(wrong_commit=wrong_commit, corrupt=corrupt)
            with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), mock.patch.dict(os.environ, GH_REPO='owner/repo'):
                with self.assertRaises(ValueError):
                    release.publish(self.output, VERSION, SHA)
            self.assertNotIn(('release', 'edit'), [call[:2] for call in calls])

    def test_stable_package_requires_opt_in_and_keeps_stable_identity(self):
        calls, api, gh = self.publish_fixture(stable=True)
        metadata = json.loads((self.output / 'release.json').read_text())
        self.assertEqual(metadata, {'version': '0.6.0', 'commit': SHA,
                                    'target': 'linux-armv7', 'hardware_validated': True})
        with self.assertRaises(ValueError):
            release.publish(self.output, '0.6.0', SHA)
        with self.assertRaises(ValueError):
            release.verify_assets(self.output, '0.6.0', SHA)
        recheck = mock.Mock()
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), \
                mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, '0.6.0', SHA, stable=True, notes='Recorded Pi evidence', before_publish=recheck)
        recheck.assert_called_once()
        create = next(call for call in calls if call[:2] == ('release', 'create'))
        self.assertIn('--prerelease=false', create)
        self.assertIn('Recorded Pi evidence', create[create.index('--notes') + 1])
        self.assertIn('--latest', calls[-1])
        self.assertIn('--prerelease=false', calls[-1])

    def test_stable_package_uses_accepted_source_instead_of_control_checkout(self):
        source = self.root / 'source'
        source.mkdir()
        for directory in ('config', 'docs', 'layouts', 'schemas', 'systemd', 'scripts'):
            (source / directory).mkdir()
        for script in ('install-release.sh', 'roundosctl.sh', 'setup-app-user.sh'):
            (source / 'scripts' / script).write_text('# selected source\n')
        (source / 'README.md').write_text('Accepted commit documentation\n')
        (self.build / 'RELEASE_VERSION').write_text('0.6.0\n')
        release.package(self.build, self.output, '0.6.0', SHA, stable=True, source=source)
        release.verify_assets(self.output, '0.6.0', SHA, stable=True)
        import tarfile
        with tarfile.open(next(self.output.glob('*.tar.gz'))) as archive:
            self.assertEqual(archive.extractfile('roundos-0.6.0-linux-armv7/README.md').read(),
                             b'Accepted commit documentation\n')

    def test_stable_reruns_never_overwrite_and_conflicts_or_corruption_block(self):
        for existing, wrong_commit, corrupt in [(True, False, False), (False, True, False), (False, False, True)]:
            calls, api, gh = self.publish_fixture(stable=True, existing=existing, wrong_commit=wrong_commit, corrupt=corrupt)
            with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), \
                    mock.patch.dict(os.environ, GH_REPO='owner/repo'):
                if wrong_commit or corrupt:
                    with self.assertRaises(ValueError):
                        release.publish(self.output, '0.6.0', SHA, stable=True)
                    self.assertNotIn(('release', 'edit'), [call[:2] for call in calls])
                else:
                    release.publish(self.output, '0.6.0', SHA, stable=True)
                    self.assertEqual([call[:2] for call in calls], [('release', 'download')])

    def test_acceptance_revoked_during_upload_keeps_stable_release_a_draft(self):
        calls, api, gh = self.publish_fixture(stable=True)
        with mock.patch.object(release, 'api', side_effect=api), mock.patch.object(release, 'gh', side_effect=gh), \
                mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            with self.assertRaisesRegex(ValueError, 'Acceptance revoked'):
                release.publish(self.output, '0.6.0', SHA, stable=True,
                                before_publish=mock.Mock(side_effect=ValueError('Acceptance revoked')))
        self.assertNotIn(('release', 'edit'), [call[:2] for call in calls])

    def test_tag_created_during_upload_cannot_point_at_another_commit(self):
        calls, api, gh = self.publish_fixture(stable=True)
        def tag_appears(path, missing_ok=False):
            if ('release', 'upload') in [call[:2] for call in calls]:
                if '/git/ref/tags/' in path:
                    return {'ref': 'refs/tags/v0.6.0'}
                if '/commits/refs/tags/' in path:
                    return {'sha': 'b' * 40}
            return api(path, missing_ok=missing_ok)
        with mock.patch.object(release, 'api', side_effect=tag_appears), mock.patch.object(release, 'gh', side_effect=gh), \
                mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            with self.assertRaisesRegex(ValueError, 'Stable tag changed'):
                release.publish(self.output, '0.6.0', SHA, stable=True)
        self.assertFalse(any('--draft=false' in call for call in calls))

    def test_resumed_stable_draft_refreshes_acceptance_and_preserves_change_notes(self):
        calls, api, gh = self.publish_fixture(stable=True, existing=True, draft=True)
        original_api = api
        old = f'{release.PROVENANCE_START}\nold acceptance\n{release.PROVENANCE_END}\n\n## Changes\n- A feature'
        def api_with_body(path, missing_ok=False):
            if '/releases/tags/' in path:
                return {'tag_name': 'v0.6.0', 'target_commitish': SHA, 'prerelease': False,
                        'draft': True, 'assets': [], 'body': old}
            return original_api(path, missing_ok=missing_ok)
        with mock.patch.object(release, 'api', side_effect=api_with_body), mock.patch.object(release, 'gh', side_effect=gh), \
                mock.patch.dict(os.environ, GH_REPO='owner/repo'):
            release.publish(self.output, '0.6.0', SHA, stable=True, notes='replacement acceptance')
        edit = next(call for call in calls if '--notes' in call)
        updated = edit[edit.index('--notes') + 1]
        self.assertIn('replacement acceptance', updated)
        self.assertNotIn('old acceptance', updated)
        self.assertIn('## Changes\n- A feature', updated)


if __name__ == '__main__':
    unittest.main()

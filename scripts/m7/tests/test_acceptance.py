"""Synthetic unit fixtures exercise gate decisions; they are not Pi evidence."""
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import compare as gate
import bind_session


def identity(baseline=False):
    result = {key: True for key in ('identity_verified', 'reference_pi', 'service_active', 'running_binary_matches',
                                   'root_owned_regular_binary', 'root_owned_regular_timer', 'release_metadata_matches', 'expected_version_matches')}
    result.update(version='0.5.0' if baseline else '0.6.0-dev.1.g' + 'a' * 7,
                  commit=None if baseline else 'a' * 40, binary_sha256='b' * 64,
                  timer_binary_sha256='c' * 64, device_id_sha256='d' * 64, boot_id='test-boot', service_pid=123,
                  service_start_boottime_seconds=100)
    return result


def sessions(device):
    bound = {**{key: device[key] for key in gate.IDENTITY_KEYS}, 'verified': True,
             'raw_sha256': {'run-01.jsonl': 'e' * 64}, 'window_start_boottime_seconds': 99}
    return {name: {'scenario': name, 'accepted_for_m0': True, 'reference_pi_managed': True,
                   'status': 'COLLECTED', 'target_seconds': 60, 'target_repeats': 3, 'interval_seconds': 1,
                   'runs': [{'run': i, 'complete': True, 'm7_identity': dict(bound),
                             'sample': {'records': 60, 'shell_cpu_percent_mean': 10, 'shell_rss_kib_peak': 20000,
                                        'shell_cpu_valid_records': 60, 'shell_rss_valid_records': 60},
                             'renderer_metrics': [{'input-to-present-p95': '10ms', 'run_scoped': True,
                                                   'input_samples': 10, 'service_pid': device['service_pid'], 'boot_id': device['boot_id']}]} for i in (1, 2, 3)]}
            for name in gate.SCENARIOS}


class GateTests(unittest.TestCase):
    def setUp(self):
        self.old, self.new = identity(True), identity()
        self.before, self.after = sessions(self.old), sessions(self.new)
        self.observed = {'candidate_commit': self.new['commit'], 'checks': {
            key: {'observed': True, 'evidence': 'synthetic unit test only'} for key in gate.OBSERVATIONS},
            'reload_measurements': [{'method': 'save-gesture-to-visible-frame', 'fps': 60, 'frames': 20}] * 3}
        self.protocol = {'architecture': 'arm', 'baseline_source_archive_sha256':
                         '06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75',
                         'baseline': {'ticks': 5, 'bytes': 4755},
                         'candidate': {'ticks': 5, 'bytes': 1385, 'full_trees': 0, 'binary_sha256': self.new['timer_binary_sha256']}}

    def report(self):
        return gate.compare(self.old, self.new, self.before, self.after, self.observed, self.protocol)

    def test_complete_measurements_still_require_human_review(self):
        self.assertEqual(self.report()['gate'], 'READY_FOR_HUMAN_REVIEW')
        self.observed['checks'].pop('candidate_boot_wifi_reconnect')
        self.assertEqual(self.report()['gate'], 'PENDING')

    def test_missing_and_mismatched_evidence_never_pass(self):
        for change in ('device', 'commit', 'binding', 'sample', 'duration', 'metrics', 'hashes'):
            with self.subTest(change=change):
                self.setUp()
                if change == 'device': self.new['device_id_sha256'] = 'f' * 64
                if change == 'commit': self.observed['candidate_commit'] = 'f' * 40
                entry = self.after['gallery_flick']
                if change == 'binding': entry['runs'][0]['m7_identity']['service_pid'] += 1
                if change == 'sample': entry['runs'][0]['sample']['shell_cpu_percent_mean'] = float('nan')
                if change == 'duration': entry['target_seconds'] = 10
                if change == 'metrics': entry['runs'][0]['renderer_metrics'] = [{'input-to-present-p95': '0s'}]
                if change == 'hashes': entry['runs'][0]['m7_identity']['raw_sha256'] = {}
                self.assertEqual(self.report()['gate'], 'PENDING')

    def test_each_run_must_meet_latency_cpu_and_memory_limits(self):
        for scenario, field, value in [('gallery_flick', 'latency', '11.01ms'), ('idle', 'shell_cpu_percent_mean', 11.01),
                                       ('wifi_scan', 'shell_rss_kib_peak', 28193)]:
            with self.subTest(field=field):
                self.setUp()
                run = self.after[scenario]['runs'][1]
                if field == 'latency': run['renderer_metrics'][0]['input-to-present-p95'] = value
                else: run['sample'][field] = value
                self.assertEqual(self.report()['scenarios'][scenario]['status'], 'FAIL')
                self.assertEqual(self.report()['gate'], 'FAIL')

    def test_protocol_full_tree_or_traffic_regression_fails(self):
        for key, value in [('full_trees', 1), ('bytes', 2378)]:
            self.setUp()
            self.protocol['candidate'][key] = value
            self.assertEqual(self.report()['protocol'], 'FAIL')
        self.protocol['candidate']['binary_sha256'] = 'f' * 64
        self.assertEqual(self.report()['protocol'], 'PENDING')

    def test_reload_and_reported_functional_failure(self):
        self.observed['reload_measurements'][0]['frames'] = 30
        self.assertEqual(self.report()['reload']['status'], 'FAIL')
        self.setUp()
        self.observed['checks']['clock_label_pickers_gallery']['observed'] = False
        self.assertEqual(self.report()['gate'], 'FAIL')

    def test_known_rss_failure_survives_other_missing_metrics(self):
        for change in ('other latency', 'same latency', 'other run', 'CPU'):
            self.setUp()
            runs = self.after['gallery_flick']['runs']
            runs[0]['sample']['shell_rss_kib_peak'] = 30000
            if change == 'other latency': runs[2]['renderer_metrics'][0]['input_samples'] = 0
            if change == 'same latency': runs[0]['renderer_metrics'][0]['input_samples'] = 0
            if change == 'other run': runs.pop()
            if change == 'CPU': runs[0]['sample']['shell_cpu_valid_records'] = 1
            result = self.report()
            self.assertEqual(result['scenarios']['gallery_flick']['status'], 'FAIL', change)
            self.assertEqual(result['gate'], 'FAIL', change)

    def test_one_known_reload_failure_survives_incomplete_recordings(self):
        failed = {'method': 'save-gesture-to-visible-frame', 'fps': 60, 'frames': 30}
        for values in ([failed], [failed, {}], [failed, {}, {}]):
            self.observed['reload_measurements'] = values
            self.assertEqual(self.report()['reload']['status'], 'FAIL')
            self.assertEqual(self.report()['gate'], 'FAIL')

    def test_identity_requires_all_trust_checks_and_exact_candidate_commit(self):
        for key, value in identity().items():
            if value is True:
                changed = identity()
                changed[key] = False
                self.assertFalse(gate.valid_identity(changed, False), key)
        for value in (None, '', 'a' * 7):
            changed = identity()
            changed['commit'] = value
            self.assertFalse(gate.valid_identity(changed, False))
        self.assertTrue(gate.valid_identity(identity(True), True))

    def test_private_exclusive_output(self):
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp) / 'evidence.json'
            gate.write_new(output, {'gate': 'PENDING'})
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError): gate.write_new(output, {'gate': 'OTHER'})
            self.assertEqual(json.loads(output.read_text()), {'gate': 'PENDING'})

    def test_reused_latency_or_insufficient_counters_stay_pending(self):
        for change in ('old process', 'no scope', 'no inputs', 'few CPU', 'few RSS'):
            self.setUp()
            run = self.after['gallery_flick']['runs'][0]
            if change == 'old process': run['m7_identity']['window_start_boottime_seconds'] = 101
            if change == 'no scope': run['renderer_metrics'][0].pop('run_scoped')
            if change == 'no inputs': run['renderer_metrics'][0]['input_samples'] = 0
            if change == 'few CPU': run['sample']['shell_cpu_valid_records'] = 1
            if change == 'few RSS': run['sample']['shell_rss_valid_records'] = 1
            self.assertEqual(self.report()['gate'], 'PENDING', change)

    def test_service_restarts_require_explicit_option(self):
        import collect
        from unittest import mock
        with mock.patch.object(sys, 'argv', ['collect.py', '--expect-version', '0.5.0', '--scenario', 'idle', '--output', '/unused']), mock.patch.object(collect.subprocess, 'run') as run, mock.patch.object(collect.argparse.ArgumentParser, 'error', side_effect=ValueError):
            with self.assertRaises(ValueError): collect.main()
            run.assert_not_called()

    def test_collector_output_preserves_three_independent_run_identities(self):
        import collect
        from unittest import mock
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        identities = [identity(True)]
        for index in range(3):
            before, after = identity(True), identity(True)
            for value in (before, after):
                value['service_pid'] = 123 + index
                value['service_start_boottime_seconds'] = 100 + index * 100
            before['recorded_utc'] = (base + timedelta(seconds=index * 100)).isoformat()
            after['recorded_utc'] = (base + timedelta(seconds=index * 100 + 70)).isoformat()
            identities.extend((before, before, after))
        run_number = 0
        def execute(args, **kwargs):
            nonlocal run_number
            if args[0] == 'sudo': return None
            directory = Path(args[args.index('--output') + 1])
            directory.mkdir()
            start = base + timedelta(seconds=run_number * 100 + 1)
            raw = [{'utc': (start + timedelta(seconds=i)).isoformat(), 'elapsed_seconds': i,
                    'processes': [{'name': 'roundd', 'pid': 123 + run_number, 'cpu_percent': 2, 'rss_kib': 12345}]} for i in range(1, 61)]
            (directory / 'run-01.jsonl').write_text('\n'.join(json.dumps(value) for value in raw))
            session = {'start_utc': start.isoformat(), 'end_utc': (start + timedelta(seconds=60.5)).isoformat(),
                       'runs': [{'run': 1, 'start_utc': start.isoformat(), 'end_utc': (start + timedelta(seconds=60.5)).isoformat(), 'complete': True}]}
            (directory / 'session.json').write_text(json.dumps(session))
            run_number += 1
        def metrics(device, start, end):
            return [{'input-to-present-p95': '10ms', 'input_samples': 7, 'run_scoped': True,
                     'service_pid': device['service_pid'], 'boot_id': device['boot_id'], 'recorded_utc': end}]
        with tempfile.TemporaryDirectory() as temp:
            output = Path(temp) / 'new-session'
            args = ['collect.py', '--expect-version', '0.5.0', '--scenario', 'gallery_flick', '--output', str(output), '--restart-for-latency']
            with mock.patch.object(sys, 'argv', args), mock.patch.object(collect, 'identity', side_effect=identities), mock.patch.object(collect.time, 'clock_gettime', side_effect=[99, 199, 299]), mock.patch('builtins.input', return_value=''), mock.patch('builtins.print'), mock.patch.object(collect.subprocess, 'run', side_effect=execute) as commands, mock.patch.object(collect.latency, 'collect', side_effect=metrics):
                self.assertEqual(collect.main(), 0)
            result = json.loads((output / 'session-bound.json').read_text())
            self.assertIsNotNone(gate.session_runs(result, 'gallery_flick', identities[1]))
            self.assertEqual([value['m7_identity']['service_pid'] for value in result['runs']], [123, 124, 125])
            self.assertEqual(sum(call.args[0][0] == 'sudo' for call in commands.call_args_list), 6)


class BindingTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / 'session.json'
        self.start = datetime(2026, 1, 1, tzinfo=timezone.utc)
        self.before, self.after = identity(True), identity(True)
        self.before['recorded_utc'] = self.start.isoformat()
        self.after['recorded_utc'] = (self.start + timedelta(seconds=200)).isoformat()
        self.session = sessions(self.before)['idle']
        self.session.update(start_utc=self.before['recorded_utc'], end_utc=self.after['recorded_utc'])
        for run in self.session['runs']:
            offset = (run['run'] - 1) * 61
            run['start_utc'] = (self.start + timedelta(seconds=offset)).isoformat()
            run['end_utc'] = (self.start + timedelta(seconds=offset + 60.5)).isoformat()
            records = [{'utc': (self.start + timedelta(seconds=offset + i)).isoformat(), 'elapsed_seconds': i,
                        'processes': [{'name': 'roundd', 'pid': 123, 'cpu_percent': 2, 'rss_kib': 12345}]} for i in range(1, 61)]
            (self.path.parent / f'run-{run["run"]:02d}.jsonl').write_text('\n'.join(json.dumps(value) for value in records))
        self.path.write_text(json.dumps(self.session))

    def test_recalculates_summaries_and_preserves_original(self):
        original = self.path.read_bytes()
        bound = bind_session.bind(self.path, self.before, self.after)
        self.assertEqual(bound['runs'][0]['sample']['shell_cpu_percent_mean'], 2)
        self.assertEqual(bound['runs'][0]['sample']['shell_rss_kib_peak'], 12345)
        self.assertEqual(len(bound['m7_identity']['raw_sha256']), 3)
        self.assertEqual(self.path.read_bytes(), original)
        self.assertEqual(bound['runs'][0]['sample']['shell_cpu_valid_records'], 60)

    def test_rejects_restart_or_other_process_samples(self):
        self.after['service_pid'] = 124
        with self.assertRaises(ValueError): bind_session.bind(self.path, self.before, self.after)
        self.after['service_pid'] = 123
        path = self.path.parent / 'run-01.jsonl'
        path.write_text(path.read_text().replace('"pid": 123', '"pid": 124'))
        with self.assertRaises(ValueError): bind_session.bind(self.path, self.before, self.after)

    def test_rejects_truncated_and_out_of_window_samples(self):
        path = self.path.parent / 'run-01.jsonl'
        records = path.read_text().splitlines()
        path.write_text('\n'.join(records[:10]))
        with self.assertRaises(ValueError): bind_session.bind(self.path, self.before, self.after)
        path.write_text('\n'.join(records))
        self.before['recorded_utc'] = (self.start + timedelta(seconds=1)).isoformat()
        with self.assertRaises(ValueError): bind_session.bind(self.path, self.before, self.after)
    def test_rejects_mostly_missing_or_invalid_shell_counters(self):
        path = self.path.parent / 'run-01.jsonl'
        original = path.read_text()
        for replacement in ('null', '-1', 'NaN'):
            path.write_text(original.replace('"cpu_percent": 2', '"cpu_percent": ' + replacement))
            with self.assertRaises(ValueError): bind_session.bind(self.path, self.before, self.after)


class LatencyTests(unittest.TestCase):
    def test_filters_pid_boot_window_and_retains_only_safe_measurements(self):
        import latency
        def record(pid='123', boot='testboot', stamp='1000000'):
            return {'_PID': pid, '_BOOT_ID': boot, '__REALTIME_TIMESTAMP': stamp,
                    'MESSAGE': 'RoundOS performance: input-to-present-p95=10ms (n=7; unsafe-extra-text)'}
        rows = [record(), record('124'), record(boot='otherboot'), record(stamp='4000000')]
        result = latency.parse(rows, identity(), '1970-01-01T00:00:00+00:00', '1970-01-01T00:00:02+00:00')
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]['input_samples'], 7)
        self.assertEqual(result[0]['input-to-present-p95'], '10ms')
        self.assertNotIn('unsafe-extra-text', json.dumps(result))


class IdentityCollectorTests(unittest.TestCase):
    def test_installed_build_id_identifies_candidate_without_release_json(self):
        # The installer copies binaries and checksums, not release.json.
        import identity as collector
        from unittest import mock
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            release = root / 'opt/roundos/releases/test'
            (release / 'bin').mkdir(parents=True)
            for name in ('roundd', 'round-timer'):
                (release / 'bin' / name).write_bytes(b'test binary')
                (release / 'bin' / name).chmod(0o755)
            for directory in (release, release / 'bin', release.parent, release.parent.parent, root / 'opt'):
                directory.chmod(0o755)
            (root / 'opt/roundos/current').symlink_to(release)
            for name, value in [('proc/device-tree/model', 'Raspberry Pi Zero 2 W\0'),
                                ('proc/sys/kernel/random/boot_id', 'boot'), ('etc/machine-id', 'unit-test-machine')]:
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(value)
            (root / 'proc/123').mkdir(parents=True)
            fields = ['0'] * 22
            fields[19] = '10000'
            (root / 'proc/123/stat').write_text('123 (roundd) ' + ' '.join(fields))
            os.link(release / 'bin/roundd', root / 'proc/123/exe')
            def mapped(value):
                return root / str(value).lstrip('/')
            def command(args):
                if args[-1] == '-version': return 'RoundOS 0.6.0-dev.1.gaaaaaaa (' + 'a' * 40 + ')'
                if args[1] == 'show': return '123'
                return 'active'
            original = Path.lstat
            def owned(path, *args, **kwargs):
                values = list(original(path, *args, **kwargs))
                values[4] = 0
                return os.stat_result(values)
            with mock.patch.object(collector, 'Path', side_effect=mapped), mock.patch.object(collector, 'command', side_effect=command), mock.patch.object(Path, 'lstat', owned):
                result = collector.scan('0.6.0-dev.1.gaaaaaaa')
                self.assertEqual(result['commit'], 'a' * 40)
                self.assertTrue(result['identity_verified'])
                (release / 'release.json').write_text(json.dumps({'version': result['version'], 'commit': 'f' * 40}))
                self.assertFalse(collector.scan(result['version'])['identity_verified'])
                def untrusted(path, *args, **kwargs):
                    values = list(owned(path, *args, **kwargs))
                    if path == release / 'bin/roundd': values[4] = 1000
                    return os.stat_result(values)
                with mock.patch.object(Path, 'lstat', untrusted), mock.patch.object(collector, 'command') as execute:
                    with self.assertRaises(ValueError): collector.scan(result['version'])
                    execute.assert_not_called()


if __name__ == '__main__':
    unittest.main()

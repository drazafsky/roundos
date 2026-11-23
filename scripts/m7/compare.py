#!/usr/bin/env python3
"""Compare genuine Pi sessions against the approved RUI release budgets.

Missing measurements and observations remain PENDING. This tool does not
approve a release, manufacture operator observations, or modify GitHub.
"""
import argparse
from datetime import datetime, timezone
import importlib.util
import json
import math
from pathlib import Path
import re
import os

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('m0_report', ROOT / 'scripts/m0/report.py')
m0 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m0)

SCENARIOS = ('idle', 'gallery_flick', 'timer_background', 'wifi_scan', 'reduce_motion')
RELEASE_KEYS = ('version', 'commit', 'binary_sha256', 'timer_binary_sha256', 'device_id_sha256')
IDENTITY_KEYS = (*RELEASE_KEYS, 'boot_id', 'service_pid', 'service_start_boottime_seconds')
OBSERVATIONS = (
    'baseline_managed_boot', 'baseline_display_touch_encoder_wifi',
    'candidate_display_touch_encoder', 'clock_label_pickers_gallery',
    'timer_background_notification_persistence_crash_recovery',
    'secure_editor_reload_deferral', 'hidden_disabled_focus',
    'candidate_boot_wifi_reconnect', 'restart_preserves_settings_and_timer',
    'disable_restores_desktop', 'rollback_restores_managed_050',
    'hot_reload_retains_app_state', 'no_irreversible_state_migrations',
    'reduce_motion_and_accessibility', 'dialogs_and_picker_commit_cancel',
)


def number(value):
    return type(value) in (int, float) and math.isfinite(value) and value >= 0


def load(path):
    data = Path(path).read_bytes()
    if len(data) > 4 << 20:
        raise ValueError('Evidence file is too large')
    return json.loads(data)


def write_new(path, value):
    # Exclusive creation and mode 0600 apply before the first evidence byte.
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w', encoding='utf-8') as output:
        output.write(json.dumps(value, indent=2, allow_nan=False) + '\n')


def hex_id(value, length):
    return isinstance(value, str) and re.fullmatch('[0-9a-f]{' + str(length) + '}', value) is not None


def valid_identity(identity, baseline):
    return (identity.get('identity_verified') is True and identity.get('reference_pi') is True
            and identity.get('service_active') is True and identity.get('running_binary_matches') is True
            and identity.get('root_owned_regular_binary') is True and identity.get('release_metadata_matches') is True
            and identity.get('root_owned_regular_timer') is True
            and identity.get('expected_version_matches') is True
            and hex_id(identity.get('binary_sha256'), 64)
            and hex_id(identity.get('timer_binary_sha256'), 64)
            and hex_id(identity.get('device_id_sha256'), 64)
            and number(identity.get('service_start_boottime_seconds')) and identity['service_start_boottime_seconds'] > 0
            and type(identity.get('service_pid')) is int and identity['service_pid'] > 0
            and isinstance(identity.get('boot_id'), str) and bool(identity['boot_id'])
            and (identity.get('version') == '0.5.0' if baseline else
                 isinstance(identity.get('version'), str) and bool(re.fullmatch(r'0\.6\.0(?:-[0-9A-Za-z.+-]+)?', identity['version']))
                 and hex_id(identity.get('commit'), 40)))


def session_runs(session, scenario, identity):
    if (session.get('scenario') != scenario or session.get('accepted_for_m0') is not True
            or session.get('reference_pi_managed') is not True or session.get('status') != 'COLLECTED'
            or session.get('target_seconds') != 60 or session.get('target_repeats') != 3
            or not number(session.get('interval_seconds')) or not .2 <= session['interval_seconds'] <= 30):
        return None
    runs = session.get('runs', [])
    if len(runs) != 3 or [run.get('run') for run in runs] != [1, 2, 3]:
        return None
    for run in runs:
        binding = run.get('m7_identity', {})
        if (binding.get('verified') is not True or any(binding.get(key) != identity.get(key) for key in RELEASE_KEYS)
                or not number(binding.get('service_start_boottime_seconds'))
                or not number(binding.get('window_start_boottime_seconds'))
                or binding['service_start_boottime_seconds'] < binding['window_start_boottime_seconds']
                or type(binding.get('service_pid')) is not int or binding['service_pid'] <= 0):
            return None
        hashes = binding.get('raw_sha256', {})
        if set(hashes) != {'run-01.jsonl'} or not all(hex_id(value, 64) for value in hashes.values()):
            return None
        sample = run.get('sample', {})
        if (run.get('complete') is not True or sample.get('records', 0) < 45
                or not number(sample.get('shell_cpu_percent_mean')) or not number(sample.get('shell_rss_kib_peak'))
                or sample['shell_rss_kib_peak'] <= 0 or sample.get('shell_cpu_valid_records', 0) < 45
                or sample.get('shell_rss_valid_records', 0) < 45):
            return None
        timings = [m0.ms(item.get('input-to-present-p95')) for item in run.get('renderer_metrics', [])]
        if not timings or any(not number(value) or scenario != 'idle' and value <= 0 for value in timings):
            return None
        if any(item.get('run_scoped') is not True or item.get('service_pid') != binding['service_pid']
               or item.get('boot_id') != binding.get('boot_id')
               or type(item.get('input_samples')) is not int or item['input_samples'] < (0 if scenario == 'idle' else 1)
               for item in run.get('renderer_metrics', [])):
            return None
    return runs


def compare(baseline_identity, candidate_identity, baseline, candidate, observations, protocol):
    same_device = baseline_identity.get('device_id_sha256') == candidate_identity.get('device_id_sha256')
    identity_ok = valid_identity(baseline_identity, True) and valid_identity(candidate_identity, False) and same_device
    result = {'generated_utc': datetime.now(timezone.utc).isoformat(),
              'candidate_commit': candidate_identity.get('commit'),
              'identity': 'PASS' if identity_ok else 'PENDING', 'scenarios': {}, 'observations': {}}
    for scenario in SCENARIOS:
        old, new = session_runs(baseline.get(scenario, {}), scenario, baseline_identity), session_runs(candidate.get(scenario, {}), scenario, candidate_identity)
        if old is None or new is None or not identity_ok:
            result['scenarios'][scenario] = {'status': 'PENDING'}
            continue
        if baseline[scenario].get('interval_seconds') != candidate[scenario].get('interval_seconds'):
            result['scenarios'][scenario] = {'status': 'PENDING'}
            continue
        comparisons = []
        for before, after in zip(old, new):
            # Compare matched run summaries; retain absolute values for review.
            a = max(m0.ms(item['input-to-present-p95']) for item in before['renderer_metrics'])
            b = max(m0.ms(item['input-to-present-p95']) for item in after['renderer_metrics'])
            old_sample, new_sample = before['sample'], after['sample']
            latency = scenario == 'idle' or b <= a * 1.1
            cpu = scenario != 'idle' or new_sample['shell_cpu_percent_mean'] <= old_sample['shell_cpu_percent_mean'] * 1.1
            rss = new_sample['shell_rss_kib_peak'] <= old_sample['shell_rss_kib_peak'] + 8192
            comparisons.append({'run': before['run'], 'baseline_input_p95_ms': a, 'candidate_input_p95_ms': b,
                                'baseline_cpu_percent': old_sample['shell_cpu_percent_mean'],
                                'candidate_cpu_percent': new_sample['shell_cpu_percent_mean'],
                                'baseline_rss_kib': old_sample['shell_rss_kib_peak'], 'candidate_rss_kib': new_sample['shell_rss_kib_peak'],
                                'status': 'PASS' if latency and cpu and rss else 'FAIL'})
        result['scenarios'][scenario] = {'status': 'PASS' if all(item['status'] == 'PASS' for item in comparisons) else 'FAIL', 'runs': comparisons}
    matching_observations = observations.get('candidate_commit') == candidate_identity.get('commit') and identity_ok
    for name in OBSERVATIONS:
        entry = observations.get('checks', {}).get(name, {})
        observed = matching_observations and entry.get('observed') is True and isinstance(entry.get('evidence'), str) and bool(entry['evidence'].strip())
        result['observations'][name] = 'OBSERVED' if observed else 'FAIL' if matching_observations and entry.get('observed') is False and entry.get('evidence') else 'PENDING'
    reloads = observations.get('reload_measurements', [])
    valid_reload = matching_observations and len(reloads) >= 3 and all(
        item.get('method') == 'save-gesture-to-visible-frame' and number(item.get('frames')) and type(item['frames']) is int
        and number(item.get('fps')) and item['fps'] >= 60 for item in reloads)
    if valid_reload:
        upper_bounds = [1000 * (item['frames'] + 1) / item['fps'] for item in reloads]
        result['reload'] = {'status': 'PASS' if max(upper_bounds) <= 500 else 'FAIL', 'upper_bounds_ms': upper_bounds}
    else:
        result['reload'] = {'status': 'PENDING'}
    old_traffic, new_traffic = protocol.get('baseline', {}), protocol.get('candidate', {})
    valid_protocol = (identity_ok and protocol.get('architecture') == 'arm' and type(old_traffic.get('ticks')) is int and old_traffic['ticks'] >= 5
                      and protocol.get('baseline_source_archive_sha256') == '06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75'
                      and new_traffic.get('binary_sha256') == candidate_identity.get('timer_binary_sha256')
                      and old_traffic.get('ticks') == new_traffic.get('ticks')
                      and number(old_traffic.get('bytes')) and old_traffic['bytes'] > 0
                      and number(new_traffic.get('bytes')) and type(new_traffic.get('full_trees')) is int and new_traffic['full_trees'] >= 0)
    result['protocol'] = 'PENDING' if not valid_protocol else 'PASS' if new_traffic['full_trees'] == 0 and new_traffic['bytes'] * 2 <= old_traffic['bytes'] else 'FAIL'
    statuses = [result['identity'], result['protocol'], result['reload']['status'], *[item['status'] for item in result['scenarios'].values()]]
    all_observed = all(value == 'OBSERVED' for value in result['observations'].values())
    result['gate'] = 'FAIL' if 'FAIL' in statuses or 'FAIL' in result['observations'].values() else 'READY_FOR_HUMAN_REVIEW' if all(value == 'PASS' for value in statuses) and all_observed else 'PENDING'
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('baseline-identity', 'candidate-identity', 'observations', 'protocol', 'output'):
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--baseline', nargs='+', type=Path, required=True)
    parser.add_argument('--candidate', nargs='+', type=Path, required=True)
    args = parser.parse_args()
    def sessions(paths):
        found = {}
        for path in paths:
            session = load(path)
            name = session.get('scenario')
            if name in found:
                raise ValueError('Duplicate scenario evidence')
            found[name] = session
        return found
    try:
        report = compare(load(args.baseline_identity), load(args.candidate_identity), sessions(args.baseline), sessions(args.candidate), load(args.observations), load(args.protocol))
        write_new(args.output, report)
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.exit(1, f'Evidence comparison failed: {type(error).__name__}\n')
    print('M7 GATE:', report['gate'])
    return 0 if report['gate'] == 'READY_FOR_HUMAN_REVIEW' else 1


if __name__ == '__main__':
    raise SystemExit(main())

#!/usr/bin/env python3
"""Summarize M0 evidence without turning absent hardware tests into PASS."""
import argparse
import json
from pathlib import Path
import statistics
import re
from datetime import datetime,timezone

REQUIRED=('idle','gallery_flick','timer_background','wifi_scan')

def ms(duration):
    if not isinstance(duration,str):return None
    match=re.fullmatch(r'(\d+(?:\.\d+)?)(ns|µs|us|ms|s)',duration)
    if not match:return None
    value=float(match.group(1))
    return round(value*{'ns':.000001,'µs':.001,'us':.001,'ms':1,'s':1000}[match.group(2)],4)

MANUAL=('installed','active','after_reboot','after_restart','after_disable','after_rollback')


def build(host,session_files,acceptance_files):
    host_status='PENDING' if host is None else host.get('result','PENDING')
    sessions={}
    for f in session_files:
        try:
            s=json.loads(Path(f).read_text(encoding='utf-8'))
        except (OSError,ValueError):continue
        # Keep only accepted genuine Pi sessions; host smoke cannot pass.
        if s.get('accepted_for_m0'):
            sessions.setdefault(s['scenario'],[]).append(s)
    scenarios={}
    for name in REQUIRED:
        valid=sessions.get(name,[])
        runs=[r for s in valid for r in s.get('runs',[])]
        frames=[ms(m.get('frame-p95')) for r in runs for m in r.get('renderer_metrics',[])]
        input_latency=[ms(m.get('input-to-present-p95')) for r in runs for m in r.get('renderer_metrics',[])]
        frames=[x for x in frames if x is not None]
        input_latency=[x for x in input_latency if x is not None]
        scenarios[name]={'status':'PASS' if len(runs)>=3 else 'PENDING',
            'median_frame_p95_ms':round(statistics.median(frames),3) if frames else None,
            'median_input_to_present_p95_ms':round(statistics.median(input_latency),3) if input_latency else None,
            'frame_timing_points':len(frames),'input_timing_points':len(input_latency),
            'accepted_runs':len(runs),
            'mean_shell_cpu_pct':round(statistics.mean(x['sample']['shell_cpu_percent_mean'] for x in runs if x['sample']['shell_cpu_percent_mean'] is not None),3) if any(x['sample']['shell_cpu_percent_mean'] is not None for x in runs) else None,
            'peak_shell_rss_kib':max((x['sample']['shell_rss_kib_peak'] for x in runs if x['sample']['shell_rss_kib_peak'] is not None),default=None),
            'peak_temperature_c':max((x['sample']['temperature_c_max'] for x in runs if x['sample']['temperature_c_max'] is not None),default=None)}
    # Hardware acceptance is only passed after an operator observation in an
    # actual Pi evidence file; a host snapshot is not hardware attestation.
    snapshots={}
    for f in acceptance_files:
        try:s=json.loads(Path(f).read_text(encoding='utf-8'))
        except (OSError,ValueError):continue
        if s.get('phase') in MANUAL and s.get('operator_confirmed') and s.get('reference_pi'):
            snapshots[s['phase']]='OBSERVED'
    lifecycle={x:snapshots.get(x,'PENDING') for x in MANUAL}
    complete=(host_status=='PASS_HOST_ONLY' and all(v['status']=='PASS' for v in scenarios.values())
              and all(v=='OBSERVED' for v in lifecycle.values()))
    return {'generated_utc':datetime.now(timezone.utc).isoformat(),
        'host':host_status,'scenarios':scenarios,'lifecycle':lifecycle,
        'm0_gate':'READY_FOR_HUMAN_REVIEW' if complete else 'OPEN',
        'note':'READY_FOR_HUMAN_REVIEW still requires review of boot, display, behavior and rollback evidence.'}


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--host',type=Path,help='verify_host.py host-result.json')
    ap.add_argument('--sessions',type=Path,nargs='*',default=[],help='one or more collect_pi.py session.json files')
    ap.add_argument('--acceptance',type=Path,nargs='*',default=[],help='acceptance.py evidence files')
    ap.add_argument('--output',type=Path,required=True,help='report JSON path')
    args=ap.parse_args()
    host=json.loads(args.host.read_text(encoding='utf-8')) if args.host and args.host.exists() else None
    report=build(host,args.sessions,args.acceptance)
    args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    args.output.chmod(0o600)
    print('M0 GATE:',report['m0_gate'])
    for name,val in report['scenarios'].items():print(name,val['status'],val['accepted_runs'])
    return 0

if __name__=='__main__':raise SystemExit(main())

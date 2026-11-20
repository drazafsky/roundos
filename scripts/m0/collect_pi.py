#!/usr/bin/env python3
"""Collect three 60-second RoundOS 0.5.0 Pi baseline runs for ONE scenario.

Read-only: never starts/stops services, never changes the desktop, Wi-Fi or
application state. Interactions are performed manually on the touchscreen.
Raw journal lines, SSIDs, credentials and process argument lists are not saved.
"""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import statistics
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SCENARIOS = {
 'idle': 'Leave Home/Clock idle. Do not interact during each sample.',
 'gallery_flick': 'Repeatedly flick the Clock gallery; include short pauses between flicks.',
 'timer_background': 'Start Timer, switch to Clock, use Clock controls while Timer runs.',
 'wifi_scan': 'Use Settings > Wi-Fi, repeat scans; never copy SSIDs/passwords into notes.',
 'reduce_motion': 'Enable Reduce Motion and repeat the Clock gallery flick workload.',
}
DURATION_RE = re.compile(r'^\d+(?:\.\d+)?(?:ns|µs|us|ms|s)$')


def pi_model():
    try:
        return Path('/proc/device-tree/model').read_bytes().replace(b'\x00',b'').decode('ascii','replace').strip()
    except OSError:
        return 'unavailable'


def is_active():
    try:
        return subprocess.run(['systemctl','is-active','--quiet','roundos.service'],check=False).returncode == 0
    except OSError:
        return False


def journal_metrics(since, until):
    """Return allowlisted derived timing values, never raw journal text."""
    cmd=['journalctl','-u','roundos.service','--since',since,'--until',until,'--no-pager','--output=cat']
    try:
        p=subprocess.run(cmd,capture_output=True,text=True,timeout=15,check=False)
    except (OSError,subprocess.TimeoutExpired):
        return [],'journal unavailable'
    if p.returncode:
        return [],'journal inaccessible; check journal group or read via sudo separately'
    found=[]
    for line in p.stdout.splitlines():
        if 'RoundOS performance:' not in line:
            continue
        result={}
        for key in ['frames','frame-p95','input-to-present-p95','avg-frame','frame-max','coalesced-visual-frames']:
            match=re.search(r'(?<![\w-])'+re.escape(key)+r'=([^\s;]+)',line)
            if not match:continue
            val=match.group(1)
            if key in ('frames','coalesced-visual-frames'):
                if val.isdecimal():result[key]=int(val)
            elif DURATION_RE.fullmatch(val):
                result[key]=val
        if result:found.append(result)
    return found,None if found else 'no performance summary in journal window'


def summarize_sample(path):
    samples=[]
    for line in path.read_text(encoding='utf-8').splitlines():
        if line.strip(): samples.append(json.loads(line))
    shell_cpu=[]; shell_rss=[];temps=[]; available=[]
    for record in samples:
        if isinstance(record.get('temperature_c'),(int,float)):
            temps.append(record['temperature_c'])
        if isinstance(record.get('memory_available_kib'),(int,float)):
            available.append(record['memory_available_kib'])
        for process in record.get('processes',[]):
            if process.get('name')=='roundd':
                if isinstance(process.get('cpu_percent'),(int,float)):
                    shell_cpu.append(process['cpu_percent'])
                if isinstance(process.get('rss_kib'),(int,float)):
                    shell_rss.append(process['rss_kib'])
    return {'records':len(samples),
            'shell_cpu_percent_mean':round(statistics.mean(shell_cpu),3) if shell_cpu else None,
            'shell_cpu_percent_peak':max(shell_cpu,default=None),
            'shell_rss_kib_mean':round(statistics.mean(shell_rss),1) if shell_rss else None,
            'shell_rss_kib_peak':max(shell_rss,default=None),
            'temperature_c_mean':round(statistics.mean(temps),2) if temps else None,
            'temperature_c_max':max(temps,default=None),
            'memory_available_kib_min':min(available,default=None)}


def now():return datetime.now(timezone.utc).isoformat()


def save_json(path,value):
    path.write_text(json.dumps(value,indent=2)+'\n',encoding='utf-8')
    os.chmod(path,0o600)


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--scenario',choices=sorted(SCENARIOS),required=True)
    ap.add_argument('--output',type=Path,required=True,help='new PRIVATE directory')
    ap.add_argument('--repeats',type=int,default=3)
    ap.add_argument('--seconds',type=int,default=60)
    ap.add_argument('--interval',type=float,default=1)
    ap.add_argument('--unmanaged',action='store_true',help='diagnostic only, not accepted M0 evidence')
    ap.add_argument('--host-smoke',action='store_true',help='diagnostic only: not a real Pi result')
    args=ap.parse_args()
    if not 1<=args.repeats<=10 or not 1<=args.seconds<=3600 or not .2<=args.interval<=30:
        ap.error('repeats must be 1..10, seconds 1..3600, interval 0.2..30')
    model=pi_model()
    genuine=('Raspberry Pi Zero 2' in model and is_active() and not args.unmanaged and not args.host_smoke)
    if not args.host_smoke and not 'Raspberry Pi Zero 2' in model:
        ap.error('M0 baseline must be recorded on the reference Pi Zero 2 W; use --host-smoke for a dry run')
    if not (is_active() or args.unmanaged or args.host_smoke):
        ap.error('Managed roundos.service must be active; use --unmanaged only for diagnostic tests')
    out=args.output.expanduser().resolve()
    out.mkdir(mode=0o700,parents=False,exist_ok=False)
    os.chmod(out,0o700)
    session={'scenario':args.scenario,'instruction':SCENARIOS[args.scenario],
        'device_model':model,'managed_service_active':is_active(),
        'reference_pi_managed':genuine,'start_utc':now(),
        'interval_seconds':args.interval,'target_seconds':args.seconds,'target_repeats':args.repeats,
        'runs':[],'status':'INCOMPLETE'}
    save_json(out/'session.json',session)
    print(f'Scenario: {args.scenario}\n{SCENARIOS[args.scenario]}')
    print('Only process counters and sanitized renderer timings will be retained.')
    try:
        for i in range(1,args.repeats+1):
            input(f'Prepare for run {i}/{args.repeats} and press Enter when ready: ')
            start=now()
            path=out/f'run-{i:02d}.jsonl'
            print('MEASURING — perform the scenario now...')
            cmd=[sys.executable,str(ROOT/'scripts/collect-baseline.py'),'--seconds',str(args.seconds),
                 '--interval',str(args.interval),'--output',str(path)]
            proc=subprocess.run(cmd,check=False)
            end=now()
            if proc.returncode or not path.exists():
                print('Sampler failed; incomplete session retained for diagnosis.',file=sys.stderr)
                break
            metrics,warning=journal_metrics(start,end)
            report={'run':i,'start_utc':start,'end_utc':end,
                    'sample':summarize_sample(path),'renderer_metrics':metrics,
                    'renderer_warning':warning,'complete':True}
            session['runs'].append(report)
            save_json(out/'session.json',session)
            print(f"Run {i}: {report['sample']['records']} records; shell CPU mean {report['sample']['shell_cpu_percent_mean']}%, "
                  f"RSS peak {report['sample']['shell_rss_kib_peak']} KiB; renderer summaries {len(metrics)}")
    except (KeyboardInterrupt, EOFError):
        print('\nInterrupted; session remains INCOMPLETE.',file=sys.stderr)
    session['end_utc']=now()
    session['status']='COLLECTED' if len(session['runs'])==args.repeats else 'INCOMPLETE'
    session['accepted_for_m0']=(session['status']=='COLLECTED' and genuine and args.seconds==60 and args.repeats>=3
        and all(r['sample']['records']>=45 and r['renderer_metrics'] for r in session['runs']))
    if not session['accepted_for_m0']:
        session['acceptance_note']='Hardware/service/sample count or renderer timing evidence incomplete; not a signed-off baseline.'
    save_json(out/'session.json',session)
    print('SESSION:',session['status'],'M0_ACCEPTED:',session['accepted_for_m0'])
    print('DATA:',out)
    return 0 if session['status']=='COLLECTED' else 1

if __name__=='__main__':
    raise SystemExit(main())

#!/usr/bin/env python3
"""Run reproducible, non-invasive M0 host checks; never activates services.

Writes machine-readable evidence to a newly created private directory.
The Pi's native DRM build and physical boot acceptance are deliberately NOT
marked as passed by this host verifier.
"""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]


def execute(cmd, cwd, log, env=None, timeout=300):
    started=time.monotonic()
    try:
        with log.open('w',encoding='utf-8') as out:
            os.chmod(log,0o600)
            result=subprocess.run(cmd,cwd=cwd,env=env,stdout=out,stderr=subprocess.STDOUT,
                                  timeout=timeout,check=False)
        rc=result.returncode
    except (subprocess.TimeoutExpired, OSError) as exc:
        with log.open('a',encoding='utf-8') as out:
            out.write(f'\nM0 runner failure: {type(exc).__name__}: {exc}\n')
        rc=124
    return {'command':cmd,'exit_code':rc,'seconds':round(time.monotonic()-started,2),
            'log':log.name,'status':'PASS' if rc==0 else 'FAIL'}


def race_unavailable_reason(env=None):
    """Only skip known unsupported Go race configurations.

    Go's race detector is not available on 32-bit linux/arm. On other
    configurations without cgo, -race cannot run either. Any unrecognized
    configuration is tested normally instead of being silently skipped.
    """
    try:
        result=subprocess.run(['go','env','GOOS','GOARCH','CGO_ENABLED'],
                              stdout=subprocess.PIPE,stderr=subprocess.PIPE,
                              text=True,env=env,check=False,timeout=15)
    except (OSError,subprocess.TimeoutExpired):
        return None  # Leave actual invocation to report the failure.
    if result.returncode != 0:
        return None
    fields=result.stdout.strip().splitlines()
    if len(fields)!=3:
        return None
    goos,goarch,cgo=fields
    if goos=='linux' and goarch=='arm':
        return 'Go race detector is not supported on linux/arm (32-bit ARM). Run it on a supported host instead.'
    if cgo=='0':
        return 'Go race detector requires CGO_ENABLED=1 on this target. Run it on a supported host instead.'
    return None


def skip_check(log, reason, status):
    log.write_text(reason+'\n',encoding='utf-8')
    os.chmod(log,0o600)
    return {'command':['go','test','-race','./...','-count=1'],
            'exit_code':None,'seconds':0,'log':log.name,
            'status':status,'reason':reason}


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--output',type=Path,required=True,help='new directory for private validation results')
    ap.add_argument('--archive',type=Path,help='original 0.5.0 ZIP for byte verification')
    ap.add_argument('--skip-race',action='store_true',help='diagnostic only; produces PARTIAL status')
    args=ap.parse_args()
    out=args.output.resolve()
    out.mkdir(mode=0o700,parents=False,exist_ok=False)
    os.chmod(out,0o700)
    work=Path(tempfile.mkdtemp(prefix='roundos-m0-bins-'))
    env=os.environ.copy()
    # Do not store process environment or arguments containing local secrets.
    entries={}
    try:
        cmds={
            'provenance':[sys.executable,'scripts/m0/verify_source.py']+(['--archive',str(args.archive.resolve())] if args.archive else []),
            'go_test':['go','test','./...','-count=1'],
            'go_vet':['go','vet','./...'],
            'python_tests':[sys.executable,'-m','unittest','discover','-s','scripts/tests','-p','test_*.py'],
            'm0_python_tests':[sys.executable,'-m','unittest','discover','-s','scripts/m0/tests','-p','test_*.py'],
            'shell_install_syntax':['sh','-n','scripts/install-release.sh'],
            'shell_ctl_syntax':['sh','-n','scripts/roundosctl.sh'],
            'shell_drm_syntax':['sh','-n','scripts/test-drm.sh'],
            'version':['go','run','./cmd/roundd','-version'],
        }
        race_reason = race_unavailable_reason(env)
        if not args.skip_race and race_reason is None:
            cmds['go_race']=['go','test','-race','./...','-count=1']
        for name,cmd in cmds.items():
            entries[name]=execute(cmd,ROOT,out/f'{name}.log',timeout=420 if name=='go_race' else 300)
        if args.skip_race:
            entries['go_race']=skip_check(out/'go_race.log','Race detector skipped by --skip-race (diagnostic run only).','SKIP_REQUESTED')
        elif race_reason:
            entries['go_race']=skip_check(out/'go_race.log',race_reason,'SKIP_UNSUPPORTED')
        arm_env=env.copy()
        arm_env.update(GOOS='linux',GOARCH='arm',GOARM='7',CGO_ENABLED='0')
        for name in ('roundd','round-clock','round-timer','round-demo'):
            entries[f'armv7_{name}']=execute(['go','build','-o',str(work/name),'./cmd/'+name],ROOT,
                                            out/f'armv7_{name}.log',env=arm_env)
        unit=ROOT/'systemd/roundos.service'
        # This is a deliberately narrow static guard, not a simulated boot.
        unit_text=unit.read_text(encoding='utf-8')
        checks=['Restart=on-failure','StartLimitBurst=5','ExecStart=/opt/roundos/current/bin/roundd',
                'User=root','Conflicts=lightdm.service','-metrics-every 60s']
        missing=[x for x in checks if x not in unit_text]
        static=out/'systemd_static.log'
        static.write_text('Missing required service policy strings: '+repr(missing)+'\n',encoding='utf-8')
        os.chmod(static,0o600)
        entries['service_static']={'status':'PASS' if not missing else 'FAIL',
            'exit_code':0 if not missing else 1,'log':static.name,'command':['static service policy check']}
        # Native DRM requires Pi's libdrm development libraries; mark pending.
        host={'timestamp_utc':datetime.now(timezone.utc).isoformat(),
              'source_archive_sha256':'06a1241970b365a4d91fa5bee65561322d155cd79303c08d097bd7035e100c75',
              'checks':entries,'native_drm_on_pi':'PENDING','managed_boot_on_pi':'PENDING',
              'managed_rollback_on_pi':'PENDING','pi_perf_samples':'PENDING',
              'result':'FAIL' if any(x['status']=='FAIL' for x in entries.values()) else ('PARTIAL' if entries['go_race']['status'].startswith('SKIP') else 'PASS_HOST_ONLY')}
        path=out/'host-result.json'
        path.write_text(json.dumps(host,indent=2)+'\n',encoding='utf-8')
        os.chmod(path,0o600)
        for name,item in entries.items():
            print(f"{name:21s} {item['status']}")
        for name,item in entries.items():
            if item['status']=='FAIL':
                print(f"CHECK FAILED: {name}; inspect {out / item['log']}")
            elif item['status'].startswith('SKIP'):
                print(f"CHECK SKIPPED: {name}: {item.get('reason','unsupported')}")
        print('RESULT:',host['result'],'(hardware checks always pending)')
        print('EVIDENCE:',out)
        return 1 if host['result']=='FAIL' else 0
    finally:
        shutil.rmtree(work,ignore_errors=True)

if __name__=='__main__':
    raise SystemExit(main())

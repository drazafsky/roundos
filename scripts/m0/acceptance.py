#!/usr/bin/env python3
"""Record read-only managed-release lifecycle evidence and manual observations.

No systemd, links, releases, Wi-Fi settings or application data are changed.
Use after operator-controlled activate/reboot/disable/rollback operations.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
from datetime import datetime,timezone

PHASES=('preinstall','installed','active','after_reboot','after_restart','after_disable','after_rollback')


def command(args):
    try:
        r=subprocess.run(args,capture_output=True,text=True,timeout=8,check=False)
        return r.stdout.strip().splitlines()[0][:100] if r.stdout.strip() else f'exit {r.returncode}'
    except (OSError,subprocess.TimeoutExpired):return 'UNAVAILABLE'


def read_target(link):
    try:
        p=Path(link)
        return str(p.resolve(strict=True)) if p.is_symlink() else 'ABSENT'
    except (OSError,RuntimeError):return 'UNAVAILABLE'


def scan(phase,observed):
    return {'phase':phase,'recorded_utc':datetime.now(timezone.utc).isoformat(),
      'reference_pi': 'Raspberry Pi Zero 2' in (Path('/proc/device-tree/model').read_bytes().replace(b'\x00',b'').decode('ascii','replace') if Path('/proc/device-tree/model').exists() else ''),
      'service_active':command(['systemctl','is-active','roundos.service']),
      'service_enabled':command(['systemctl','is-enabled','roundos.service']),
      'desktop_active':command(['systemctl','is-active','lightdm.service']),
      'desktop_enabled':command(['systemctl','is-enabled','lightdm.service']),
      'release_current':read_target('/opt/roundos/current'),
      'release_previous':read_target('/opt/roundos/previous'),
      'reported_version':command(['/opt/roundos/current/bin/roundd','-version']) if Path('/opt/roundos/current/bin/roundd').exists() else 'ABSENT',
      'operator_observation':observed if observed else 'NOT VERIFIED',
      'operator_confirmed':bool(observed),
      'note':'Recorded states do not by themselves prove successful DRM display, touch or real boot behavior.'}


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--phase',choices=PHASES,required=True)
    ap.add_argument('--output',type=Path,required=True,help='new JSON evidence file (0600)')
    ap.add_argument('--observed',help='brief manual observation; omit if not verified')
    args=ap.parse_args()
    out=args.output.expanduser().resolve()
    fd=os.open(out,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    try:
        with os.fdopen(fd,'w',encoding='utf-8') as f:
            json.dump(scan(args.phase,args.observed),f,indent=2)
            f.write('\n')
    except BaseException:
        out.unlink(missing_ok=True)
        raise
    print('Wrote read-only state evidence:',out)
    return 0

if __name__=='__main__':raise SystemExit(main())

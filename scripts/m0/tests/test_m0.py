"""Unit and sandbox-only service lifecycle validation. Never touches host systemd."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT=Path(__file__).resolve().parents[3]

def module(name,rel):
    spec=importlib.util.spec_from_file_location(name,ROOT/rel)
    obj=importlib.util.module_from_spec(spec)
    spec.loader.exec_module(obj)
    return obj

source=module('m0_source','scripts/m0/verify_source.py')
collect=module('m0_collect','scripts/m0/collect_pi.py')
report=module('m0_report','scripts/m0/report.py')
host_verify=module('m0_host_verify','scripts/m0/verify_host.py')

class SourceTests(unittest.TestCase):
    def test_manifest_covers_original_release(self):
        # The immutable archive describes the historical release, not HEAD.
        rows=source.MANIFEST.read_text().splitlines()
        self.assertEqual(len(rows),149)
        self.assertEqual(len({row.split('  ',1)[1] for row in rows}),149)
        self.assertTrue(all(len(row.split('  ',1)[0])==64 for row in rows))

    def test_accepts_unchanged_archive_fixture(self):
        import hashlib
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            content=b'original release source\n'
            (root/'go.mod').write_bytes(content)
            manifest=root/'scripts/m0/original.sha'
            manifest.parent.mkdir(parents=True)
            manifest.write_text(hashlib.sha256(content).hexdigest()+'  go.mod\n')
            self.assertEqual(source.verify(root,manifest),(1,[]))

    def test_detects_modification(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            (root/'go.mod').write_text('altered')
            (root/'scripts'/'m0').mkdir(parents=True)
            manifest=root/'scripts'/'m0'/'original.sha'
            manifest.write_text('0'*64+'  go.mod\n')
            n,issues=source.verify(root,manifest)
            self.assertEqual(n,1)
            self.assertTrue(any('MODIFIED go.mod' in x for x in issues))

    def test_build_binaries_are_not_treated_as_source_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            content=b'original source\n'
            (root/'go.mod').write_bytes(content)
            (root/'scripts'/'m0').mkdir(parents=True)
            manifest=root/'scripts'/'m0'/'original.sha'
            import hashlib
            manifest.write_text(hashlib.sha256(content).hexdigest()+'  go.mod\n')
            for name in ('roundd','round-clock','round-timer'):
                (root/name).write_bytes(b'\x7fELFcompiled-test-fixture')
            n,issues=source.verify(root,manifest)
            self.assertEqual(n,1)
            self.assertEqual(issues,[])
            # Do not make a broad ignore rule: arbitrary extra code must fail.
            (root/'unapproved.py').write_text('pass\n')
            _,issues=source.verify(root,manifest)
            self.assertIn('UNEXPECTED unapproved.py',issues)
            (root/'unapproved.py').unlink()
            # A lookalike text file or symlink is not a genuine build output.
            (root/'roundd').write_text('untrusted')
            _,issues=source.verify(root,manifest)
            self.assertIn('UNEXPECTED roundd',issues)
            (root/'roundd').unlink()
            # RUI M1 source is additive and approved; another root source is not.
            (root/'sdk'/'rui'/'spec').mkdir(parents=True)
            (root/'sdk'/'rui'/'spec'/'types.go').write_text('package spec\n')
            _,issues=source.verify(root,manifest)
            self.assertEqual(issues,[])
            (root/'sdk'/'unknown').mkdir()
            (root/'sdk'/'unknown'/'code.go').write_text('package unknown\n')
            _,issues=source.verify(root,manifest)
            self.assertIn('UNEXPECTED sdk/unknown/code.go',issues)

class RacePlatformTests(unittest.TestCase):
    def test_go_linux_arm_skips_race(self):
        r=type('Result',(),{'returncode':0,'stdout':'linux\narm\n1\n'})()
        with mock.patch.object(host_verify.subprocess,'run',return_value=r):
            self.assertIn('linux/arm',host_verify.race_unavailable_reason())
    def test_supported_platform_runs_race(self):
        r=type('Result',(),{'returncode':0,'stdout':'linux\namd64\n1\n'})()
        with mock.patch.object(host_verify.subprocess,'run',return_value=r):
            self.assertIsNone(host_verify.race_unavailable_reason())
    def test_cgo_disabled_skips_race(self):
        r=type('Result',(),{'returncode':0,'stdout':'linux\namd64\n0\n'})()
        with mock.patch.object(host_verify.subprocess,'run',return_value=r):
            self.assertIn('CGO_ENABLED',host_verify.race_unavailable_reason())

class MetricsTests(unittest.TestCase):
    def test_journal_metrics_are_allowlisted(self):
        raw='''ssid=PRIVATE-SSID wifi password=secret\n2026/11/19 14:12:10 RoundOS performance: frames=120 uptime=1m avg-frame=2.3ms frame-p95=5.1ms frame-max=8.2ms input-to-present-p95=10.4ms (n=44; measured from app event handling, not kernel IRQ); coalesced-visual-frames=12\n'''
        fake=type('Result',(),{'returncode':0,'stdout':raw})()
        with mock.patch.object(collect.subprocess,'run',return_value=fake):
            values,warning=collect.journal_metrics('yesterday','today')
        self.assertEqual(warning,None)
        self.assertEqual(values[0]['input-to-present-p95'],'10.4ms')
        self.assertEqual(values[0]['coalesced-visual-frames'],12)
        self.assertNotIn('ssid',json.dumps(values).lower())
        self.assertNotIn('password',json.dumps(values).lower())

    def test_ms_duration_conversion(self):
        self.assertEqual(report.ms('250us'),0.25)
        self.assertEqual(report.ms('12.5ms'),12.5)
        self.assertEqual(report.ms('1s'),1000)
        self.assertIsNone(report.ms('top secret'))

    def test_nonpi_never_accepted(self):
        payload=report.build({'result':'PASS_HOST_ONLY'},[],[])
        self.assertEqual(payload['m0_gate'],'OPEN')
        self.assertEqual(payload['scenarios']['idle']['status'],'PENDING')

class ReleaseControlSimulation(unittest.TestCase):
    """Executes a rewritten copy of 0.5.0's *actual* roundosctl.sh. Fake
    systemctl and temporary paths guarantee no host service mutations.
    """
    def setUp(self):
        self.t=tempfile.TemporaryDirectory();self.addCleanup(self.t.cleanup)
        self.base=Path(self.t.name)
        self.root=self.base/'opt'/'roundos';self.root.mkdir(parents=True)
        (self.root/'releases').mkdir()
        self.bin=self.base/'mockbin';self.bin.mkdir()
        self.state=self.base/'systemctl_state.json'
        self.state.write_text(json.dumps({'roundos.service':{'active':False,'enabled':False},
                                         'lightdm.service':{'active':True,'enabled':True}}))
        systemctl=self.bin/'systemctl'
        systemctl.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
p=Path(os.environ['M0_FAKE_SYSTEMCTL_STATE']);s=json.loads(p.read_text())
a=sys.argv[1:];cmd=a[0];unit=a[-1];u=s[unit];rc=0
if cmd=='is-active':
 print('active' if u['active'] else 'inactive');rc=0 if u['active'] else 3
elif cmd=='is-enabled':
 print('enabled' if u['enabled'] else 'disabled');rc=0 if u['enabled'] else 1
elif cmd=='enable':u['enabled']=True
elif cmd=='disable':u['enabled']=False;u['active']=False if '--now' in a else u['active']
elif cmd=='stop':u['active']=False
elif cmd=='start':u['active']=True
else:rc=99
p.write_text(json.dumps(s));sys.exit(rc)
''')
        systemctl.chmod(0o755)
        original=(ROOT/'scripts/roundosctl.sh').read_text()
        original=original.replace('ROOT=/opt/roundos',f'ROOT={self.root}')
        original=original.replace('STAMP=/var/lib/roundos/desktop-before-roundos',f'STAMP={self.base/"desktop-stamp"}')
        original=original.replace('need_root() { [ "$(id -u)" -eq 0 ] || { echo \'Run with sudo.\' >&2; exit 1; }; }',
                                  'need_root() { :; }')
        self.assertIn('need_root() { :; }',original)
        self.script=self.base/'test-roundosctl.sh';self.script.write_text(original)
        self.env=os.environ.copy()
        self.env.update(M0_FAKE_SYSTEMCTL_STATE=str(self.state),SSH_CONNECTION='sandbox test',
                        PATH=str(self.bin)+os.pathsep+self.env.get('PATH',''))
        for version in ('0.5.0','0.4.9'):
            d=self.root/'releases'/version/'bin';d.mkdir(parents=True)
            program=d/'roundd';program.write_text('#!/bin/sh\necho "RoundOS '+version+'"\n');program.chmod(0o755)
            result=subprocess.run(['sha256sum','bin/roundd'],cwd=d.parent,capture_output=True,text=True,check=True)
            (d.parent/'SHA256SUMS').write_text(result.stdout)
        (self.root/'current').symlink_to(self.root/'releases'/'0.5.0')

    def call(self,action):
        # Mock systemctl launches several Python subprocesses; allow loaded CI hosts
        # enough startup time while still detecting a hung release script.
        r=subprocess.run(['sh',str(self.script),action],env=self.env,capture_output=True,text=True,timeout=20)
        self.assertEqual(r.returncode,0,action+' '+r.stdout+' '+r.stderr)
        return r

    def test_activate_verify_disable_restores_desktop(self):
        self.call('verify')
        self.call('activate')
        states=json.loads(self.state.read_text())
        self.assertTrue(states['roundos.service']['active'])
        self.assertFalse(states['lightdm.service']['active'])
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.call('disable')
        states=json.loads(self.state.read_text())
        self.assertFalse(states['roundos.service']['enabled'])
        self.assertTrue(states['lightdm.service']['active'])
        self.assertTrue(states['lightdm.service']['enabled'])
        self.assertFalse((self.base/'desktop-stamp').exists())

    def test_rollback_switches_managed_symlinks(self):
        (self.root/'previous').symlink_to(self.root/'releases'/'0.4.9')
        self.call('activate')
        self.call('rollback')
        self.assertEqual(self.root.joinpath('current').resolve().name,'0.4.9')
        self.assertEqual(self.root.joinpath('previous').resolve().name,'0.5.0')
        self.assertTrue(json.loads(self.state.read_text())['roundos.service']['active'])
        self.call('disable')

    def test_no_prior_managed_release_restores_desktop(self):
        self.call('activate')
        self.call('rollback')
        state=json.loads(self.state.read_text())
        self.assertFalse(state['roundos.service']['active'])
        self.assertTrue(state['lightdm.service']['active'])

if __name__=='__main__':unittest.main()

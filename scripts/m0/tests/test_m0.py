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
    """Executes a rewritten copy of the actual managed roundosctl.sh. Fake
    systemctl and temporary paths guarantee no host service mutations.
    """
    def setUp(self):
        self.t=tempfile.TemporaryDirectory();self.addCleanup(self.t.cleanup)
        self.base=Path(self.t.name)
        self.root=self.base/'opt'/'roundos';self.root.mkdir(parents=True)
        (self.root/'releases').mkdir()
        self.bin=self.base/'mockbin';self.bin.mkdir()
        self.state=self.base/'systemctl_state.json'
        self.fragment=self.base/'lightdm.service';self.fragment.write_text('[Service]\n')
        self.state.write_text(json.dumps({'roundos.service':{'active':False,'enabled':False},
                                         'lightdm.service':{'active':True,'enabled':True,'fragment':str(self.fragment)}}))
        systemctl=self.bin/'systemctl'
        systemctl.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
p=Path(os.environ['M0_FAKE_SYSTEMCTL_STATE']);s=json.loads(p.read_text())
a=sys.argv[1:];cmd=a[0];by_path=a[-1].startswith('/')
unit='lightdm.service' if cmd=='link' or (cmd=='enable' and by_path) else a[-1];u=s[unit];rc=0
s.setdefault('_calls',[]).append(a)
failure=cmd+':'+unit
if s.get('_fail',{}).get(failure,0)>0:
 s['_fail'][failure]-=1;p.write_text(json.dumps(s));sys.exit(1)
if cmd=='is-active':
 if unit=='roundos.service' and s.pop('_die_on_check',False):
  u['active']=False;u.pop('active_state',None)
 state=u.get('active_state','active' if u['active'] else 'inactive')
 print(state);rc=0 if state in ('active','reloading') else 3
elif cmd=='is-enabled':
 mode='linked' if u.get('linked') else ('enabled' if u['enabled'] else u.get('mode','disabled'))
 if u.get('runtime') and mode in ('linked','enabled'):mode+='-runtime'
 if u.get('external') and not u.get('unit_linked'):mode='not-found'
 print(mode);rc=0 if u['enabled'] else 1
elif cmd=='show':
 print(s.get('_unit_type','notify') if '--property=Type' in a else u['fragment'])
elif cmd=='enable':
 if u.get('external') and not u.get('unit_linked') and not by_path:
  p.write_text(json.dumps(s));sys.exit(1)
 u.update(enabled=True,runtime='--runtime' in a)
 if by_path:u['unit_linked']=True
elif cmd=='link':u.update(linked=True,runtime='--runtime' in a,fragment=a[-1])
elif cmd=='disable':
 if '--runtime' not in a or u.get('runtime'):u.update(enabled=False,runtime=False)
 if u.get('external'):u['unit_linked']=False
 if '--now' in a:u['active']=False;u.pop('active_state',None)
elif cmd=='stop':u['active']=False;u.pop('active_state',None)
elif cmd=='start':
 if u.get('external') and not u.get('unit_linked'):
  p.write_text(json.dumps(s));sys.exit(1)
 u['active']=True
 u.pop('active_state',None)
 if unit=='roundos.service' and s.get('_delayed_exit'):
  s['_die_on_check']=True
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
            program=d/'roundd';program.write_text('#!/bin/sh\nif [ "$1" = -startup-protocol ]; then echo sd-notify-v1; else echo "RoundOS '+version+'"; fi\n');program.chmod(0o755)
            result=subprocess.run(['sha256sum','bin/roundd'],cwd=d.parent,capture_output=True,text=True,check=True)
            (d.parent/'SHA256SUMS').write_text(result.stdout)
        (self.root/'current').symlink_to(self.root/'releases'/'0.5.0')

    def call(self,action,success=True):
        # Mock systemctl launches several Python subprocesses; allow loaded CI hosts
        # enough startup time while still detecting a hung release script.
        r=subprocess.run(['sh',str(self.script),action],env=self.env,capture_output=True,text=True,timeout=20)
        if success:
            self.assertEqual(r.returncode,0,action+' '+r.stdout+' '+r.stderr)
        else:
            self.assertNotEqual(r.returncode,0,action+' '+r.stdout+' '+r.stderr)
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

    def test_lifecycle_commands_share_the_update_transaction_lock(self):
        import fcntl
        state=self.base/'shell-data';state.mkdir()
        before=self.state.read_bytes()
        with (state/'update-install.lock').open('w') as lock:
            fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
            for action in ('activate','disable','rollback'):
                result=self.call(action,success=False)
                self.assertIn('Another update installer is running',result.stderr)
                self.assertEqual(self.state.read_bytes(),before)

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

    def change_state(self,**values):
        state=json.loads(self.state.read_text());state.update(values)
        self.state.write_text(json.dumps(state))

    def test_runtime_only_enablement_is_not_made_persistent(self):
        state=json.loads(self.state.read_text())
        state['lightdm.service']['runtime']=True
        self.state.write_text(json.dumps(state))
        self.call('activate');self.call('disable')
        state=json.loads(self.state.read_text())
        self.assertTrue(state['lightdm.service']['enabled'])
        self.assertTrue(state['lightdm.service']['runtime'])
        self.assertIn(['disable','--runtime','lightdm.service'],state['_calls'])
        self.assertIn(['enable','--runtime',str(self.fragment)],state['_calls'])

    def test_linked_runtime_unit_is_restored_without_enabling(self):
        fragment=self.base/'lightdm.service';fragment.write_text('[Service]\n')
        state=json.loads(self.state.read_text())
        state['lightdm.service'].update(enabled=False,linked=True,runtime=True,fragment=str(fragment))
        self.state.write_text(json.dumps(state))
        self.call('activate');self.call('disable')
        state=json.loads(self.state.read_text())
        self.assertFalse(state['lightdm.service']['enabled'])
        self.assertTrue(state['lightdm.service']['runtime'])
        self.assertIn(['link','--runtime',str(fragment)],state['_calls'])

    def test_disabled_desktop_is_started_without_enabling(self):
        state=json.loads(self.state.read_text());state['lightdm.service']['enabled']=False
        self.state.write_text(json.dumps(state))
        self.call('activate');self.call('disable')
        state=json.loads(self.state.read_text())
        self.assertTrue(state['lightdm.service']['active'])
        self.assertFalse(state['lightdm.service']['enabled'])

    def test_restore_start_failure_keeps_stamp_and_retry_succeeds(self):
        self.call('activate')
        self.change_state(_fail={'start:lightdm.service':1})
        result=self.call('disable',success=False)
        self.assertIn('recovery record retained',result.stderr)
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.assertNotIn('prior recorded desktop state restored',result.stdout)
        self.call('disable')
        self.assertFalse((self.base/'desktop-stamp').exists())
        self.assertTrue(json.loads(self.state.read_text())['lightdm.service']['active'])

    def test_restore_enable_failure_keeps_stamp_and_retry_succeeds(self):
        self.call('activate')
        self.change_state(_fail={'enable:lightdm.service':1})
        self.call('disable',success=False)
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.call('disable')
        self.assertFalse((self.base/'desktop-stamp').exists())

    def test_activation_readiness_failure_restores_desktop_and_disables_restarts(self):
        self.change_state(_fail={'start:roundos.service':1})
        self.call('activate',success=False)
        state=json.loads(self.state.read_text())
        self.assertFalse(state['roundos.service']['active'])
        self.assertFalse(state['roundos.service']['enabled'])
        self.assertTrue(state['lightdm.service']['active'])
        self.assertTrue(state['lightdm.service']['enabled'])

    def test_exit_immediately_after_start_is_not_reported_as_ready(self):
        self.change_state(_delayed_exit=True)
        result=self.call('activate',success=False)
        self.assertNotIn('RoundOS ready',result.stdout)
        self.assertTrue(json.loads(self.state.read_text())['lightdm.service']['active'])

    def test_rollback_readiness_failure_restores_desktop(self):
        (self.root/'previous').symlink_to(self.root/'releases'/'0.4.9')
        self.call('activate')
        self.change_state(_fail={'start:roundos.service':1})
        self.call('rollback',success=False)
        state=json.loads(self.state.read_text())
        self.assertFalse(state['roundos.service']['active'])
        self.assertFalse(state['roundos.service']['enabled'])
        self.assertTrue(state['lightdm.service']['active'])

    def test_rollback_failure_retains_stamp_when_desktop_restore_fails(self):
        (self.root/'previous').symlink_to(self.root/'releases'/'0.4.9')
        self.call('activate')
        self.change_state(_fail={'start:roundos.service':1,'start:lightdm.service':1})
        self.call('rollback',success=False)
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.call('disable')
        self.assertFalse((self.base/'desktop-stamp').exists())

    def test_legacy_rollback_is_rejected_before_stopping_current_release(self):
        old=self.root/'releases'/'0.4.9'
        (old/'bin/roundd').write_text('#!/bin/sh\necho old-version\n')
        result=subprocess.run(['sha256sum','bin/roundd'],cwd=old,capture_output=True,text=True,check=True)
        (old/'SHA256SUMS').write_text(result.stdout)
        (self.root/'previous').symlink_to(old)
        self.call('activate')
        result=self.call('rollback',success=False)
        self.assertIn('lacks managed readiness support',result.stderr)
        self.assertEqual((self.root/'current').resolve().name,'0.5.0')
        self.assertTrue(json.loads(self.state.read_text())['roundos.service']['active'])

    def test_old_simple_unit_rejected_before_desktop_switch(self):
        self.change_state(_unit_type='simple')
        self.call('activate',success=False)
        state=json.loads(self.state.read_text())
        self.assertTrue(state['lightdm.service']['active'])
        self.assertFalse(state['roundos.service']['enabled'])

    def test_unknown_snapshot_does_not_poison_record_or_prevent_retry(self):
        state=json.loads(self.state.read_text())
        state['lightdm.service'].update(enabled=False,mode='unknown')
        self.state.write_text(json.dumps(state))
        self.call('activate',success=False)
        self.assertFalse((self.base/'desktop-stamp').exists())
        state=json.loads(self.state.read_text())
        self.assertFalse(state['roundos.service']['enabled'])
        self.assertTrue(state['lightdm.service']['active'])
        state['lightdm.service']['mode']='disabled'
        self.state.write_text(json.dumps(state))
        self.call('activate');self.call('disable')

    def test_roundos_stop_failure_does_not_start_desktop_or_delete_record(self):
        self.call('activate')
        self.change_state(_fail={'disable:roundos.service':1})
        self.call('disable',success=False)
        state=json.loads(self.state.read_text())
        self.assertTrue(state['roundos.service']['active'])
        self.assertFalse(state['lightdm.service']['active'])
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.call('disable')

    def test_legacy_two_field_recovery_record_is_supported(self):
        self.call('activate')
        (self.base/'desktop-stamp').write_text('enabled active\n')
        self.call('disable')
        self.assertFalse((self.base/'desktop-stamp').exists())
        self.assertTrue(json.loads(self.state.read_text())['lightdm.service']['active'])

    def test_invalid_existing_record_is_preserved_for_diagnosis(self):
        record=self.base/'desktop-stamp';record.write_text('invalid recovery record\n')
        self.call('activate',success=False)
        self.assertEqual(record.read_text(),'invalid recovery record\n')
        self.assertTrue(json.loads(self.state.read_text())['lightdm.service']['active'])

    def external_unit(self,runtime):
        self.fragment=self.base/'custom'/'lightdm.service'
        self.fragment.parent.mkdir();self.fragment.write_text('[Service]\n')
        state=json.loads(self.state.read_text())
        state['lightdm.service'].update(external=True,unit_linked=True,runtime=runtime,fragment=str(self.fragment))
        self.state.write_text(json.dumps(state))

    def test_runtime_enabled_external_unit_link_is_recreated_on_retry(self):
        self.external_unit(runtime=True)
        self.call('activate')
        self.assertFalse(json.loads(self.state.read_text())['lightdm.service']['unit_linked'])
        self.change_state(_fail={'enable:lightdm.service':1})
        self.call('disable',success=False)
        self.assertTrue((self.base/'desktop-stamp').exists())
        self.call('disable')
        state=json.loads(self.state.read_text())
        self.assertTrue(state['lightdm.service']['unit_linked'])
        self.assertTrue(state['lightdm.service']['runtime'])
        self.assertTrue(state['lightdm.service']['active'])
        self.assertIn(['enable','--runtime',str(self.fragment)],state['_calls'])

    def test_persistently_enabled_external_unit_link_is_recreated(self):
        self.external_unit(runtime=False)
        self.call('activate');self.call('disable')
        state=json.loads(self.state.read_text())
        self.assertTrue(state['lightdm.service']['unit_linked'])
        self.assertTrue(state['lightdm.service']['enabled'])
        self.assertFalse(state['lightdm.service']['runtime'])
        self.assertIn(['enable',str(self.fragment)],state['_calls'])

    def test_rollback_refuses_transitional_state_without_changing_links(self):
        old=self.root/'releases'/'0.4.9'
        (self.root/'previous').symlink_to(old)
        for mode in ('activating','deactivating','reloading'):
            with self.subTest(mode=mode):
                state=json.loads(self.state.read_text())
                state['roundos.service']['active_state']=mode
                count=len(state.get('_calls',[]))
                self.state.write_text(json.dumps(state))
                result=self.call('rollback',success=False)
                self.assertIn('Cannot roll back while service state is',result.stderr)
                self.assertEqual((self.root/'current').resolve().name,'0.5.0')
                self.assertEqual((self.root/'previous').resolve().name,'0.4.9')
                calls=json.loads(self.state.read_text())['_calls'][count:]
                self.assertFalse(any(call[0] in ('start','stop','enable','disable') for call in calls))

if __name__=='__main__':unittest.main()

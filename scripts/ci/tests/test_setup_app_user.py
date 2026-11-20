"""Provisioning tests use rewritten paths and fake privilege/system commands."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class SetupAppUserTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.log = self.root / 'commands.jsonl'
        self.state = self.root / 'var/lib/roundos/app-data'
        self.old = self.root / 'legacy/timer.json'
        self.old.parent.mkdir(mode=0o700)
        self.old.write_text('{"deadline":12345}\n')
        source = (ROOT / 'scripts/setup-app-user.sh').read_text()
        source = source.replace('/var/lib/roundos', str(self.root / 'var/lib/roundos'))
        source = source.replace('/root/.local/state/roundos/timer.json', str(self.old))
        self.script = self.root / 'setup-app-user.sh'
        self.script.write_text(source)
        self.command('id', '''import os,sys
if len(sys.argv)>2:print(1001)
else:print(0 if os.environ.get('SETUP_TEST_ELEVATED')=='1' else 1000)
''')
        self.command('sudo', '''import os,sys
args=sys.argv[1:]
if args[0]=='--':args=args[1:]
if os.environ.get('SETUP_TEST_DENY_SUDO'):sys.exit(1)
os.execvpe(args[0],args,dict(os.environ,SETUP_TEST_ELEVATED='1'))
''')
        self.command('getent', '''import os,sys
sys.exit(0 if os.environ.get('SETUP_TEST_ELEVATED')=='1' else 99)
''')
        self.command('install', '''import os,sys
if os.environ.get('SETUP_TEST_ELEVATED')!='1':sys.exit(99)
args=[];i=1
while i<len(sys.argv):
 if sys.argv[i] in ('-o','-g'):i+=2;continue
 args.append(sys.argv[i]);i+=1
os.execv('/usr/bin/install',['install',*args])
''')
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        SETUP_TEST_LOG=str(self.log))

    def command(self, name, body):
        path = self.bin / name
        path.write_text('''#!/usr/bin/env python3
import json,os,sys
with open(os.environ['SETUP_TEST_LOG'],'a') as log:
 log.write(json.dumps([os.path.basename(sys.argv[0]),*sys.argv[1:]])+'\\n')
''' + body)
        path.chmod(0o755)

    def run_setup(self, success=True):
        result = subprocess.run(['sh', str(self.script)], env=self.env,
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def test_unprivileged_entry_elevates_before_migration_and_provisioning(self):
        original = self.old.read_bytes()
        self.run_setup()
        self.assertEqual((self.state / 'timer.json').read_bytes(), original)
        self.assertEqual(self.old.read_bytes(), original)
        self.assertEqual((self.state / 'timer.json').stat().st_mode & 0o777, 0o600)
        calls = [json.loads(row) for row in self.log.read_text().splitlines()]
        self.assertEqual(calls[1], ['sudo', '--', 'sh', str(self.script)])
        migration = [args for args in calls if args[0] == 'install' and str(self.old) in args]
        self.assertEqual(migration, [['install', '-o', 'roundos-app', '-g', 'roundos-app', '-m', '0600', str(self.old), str(self.state / 'timer.json')]])

    def test_existing_state_preserved(self):
        self.run_setup()
        (self.state / 'timer.json').write_text('newer state')
        self.run_setup()
        self.assertEqual((self.state / 'timer.json').read_text(), 'newer state')

    def test_denied_elevation_leaves_protected_state_untouched(self):
        self.env['SETUP_TEST_DENY_SUDO'] = '1'
        self.run_setup(success=False)
        self.assertFalse(self.state.exists())
        self.assertTrue(self.old.exists())

    def test_legacy_symlink_is_not_copied(self):
        actual = self.old.with_name('other.json')
        self.old.rename(actual)
        self.old.symlink_to(actual)
        self.run_setup()
        self.assertFalse((self.state / 'timer.json').exists())
        self.assertEqual(actual.read_text(), '{"deadline":12345}\n')


if __name__ == '__main__':
    unittest.main()

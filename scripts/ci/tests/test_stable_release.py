"""Offline release-parent gates; never publish or claim physical acceptance."""
import base64
import copy
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import stable_release as stable

SHA = 'a' * 40
MERGE = 'b' * 40
VERSION = '0.6.0'
REPO = 'owner/repo'


def block_record(kind, value):
    return f'```{kind}\n{json.dumps(value)}\n```'


def issue(number):
    return {'number': number, 'state': 'closed', 'state_reason': 'completed',
            'repository_url': f'https://api.github.com/repos/{REPO}',
            'labels': [], 'user': {'login': 'maintainer'}, 'body': ''}


class Fixture:
    repo = REPO
    prefix = f'repos/{REPO}'

    def __init__(self):
        self.issues = {1: issue(1), 2: issue(2), 3: issue(3)}
        self.issues[1]['state'] = 'open'
        self.issues[1]['labels'] = [{'name': stable.LABEL}]
        self.issues[1]['body'] = block_record('roundos-release', {
            'version': VERSION, 'commit': SHA, 'acceptance_issue': 3})
        self.children = {1: [2, 3], 2: [], 3: []}
        self.prs = {2: [{'number': 10, 'merged': True, 'baseRefName': 'main',
                         'mergeCommit': {'oid': MERGE}, 'repository': {'nameWithOwner': REPO}}], 3: []}
        self.comments = [{'body': block_record('roundos-acceptance', {
            'version': VERSION, 'commit': SHA, 'result': 'passed',
            'evidence': f'https://github.com/{REPO}/issues/3#issuecomment-123'}),
            'user': {'login': 'maintainer'}, 'html_url': f'https://github.com/{REPO}/issues/3#issuecomment-456'}]
        self.releases = []
        self.excluded = set()
        self.ci = True
        self.source_version = VERSION
        self.source_workflows = [{'name': 'ci.yml', 'type': 'file', 'sha': 'c' * 40}]
        self.main_workflows = copy.deepcopy(self.source_workflows)

    def trusted(self, login):
        return login == 'maintainer'

    def included(self, ancestor, descendant):
        return (ancestor, descendant) not in self.excluded

    def get(self, path, *, missing_ok=False):
        if path.startswith('contents/.github/workflows?'):
            return self.main_workflows if path.endswith('=main') else self.source_workflows
        if path.startswith('contents/VERSION?'):
            return {'content': base64.b64encode(self.source_version.encode()).decode()}
        return copy.deepcopy(self.issues[int(path.split('/')[1])])

    def pages(self, path):
        if path == 'releases':
            return self.releases
        if path.startswith('issues?'):
            return [self.issues[1]]
        number = int(path.split('/')[1])
        if path.endswith('/sub_issues'):
            return [copy.deepcopy(self.issues[child]) for child in self.children[number]]
        return self.comments

    def closing_prs(self, number):
        return self.prs.get(number, [])

    def ci_passed(self, sha):
        stable.block(self.ci, 'CI pending')


class EligibilityTests(unittest.TestCase):
    def setUp(self):
        self.github = Fixture()

    def ready(self):
        return stable.eligibility(self.github, 1)

    def test_complete_tree_with_accepted_commit_is_ready(self):
        self.assertEqual(self.ready(), {'parent': 1, 'version': VERSION, 'sha': SHA,
                                      'acceptance': self.github.comments[0]['html_url']})

    def test_requires_explicit_maintainer_opt_in(self):
        for field, value in [('labels', []), ('user', {'login': 'outsider'}),
                             ('body', ''), ('body', block_record('roundos-release', {'version': '../../bad'}))]:
            with self.subTest(field=field, value=value):
                self.github = Fixture()
                self.github.issues[1][field] = value
                with self.assertRaises(stable.Blocked):
                    self.ready()

    def test_rejects_ambiguous_or_malformed_records(self):
        body = self.github.issues[1]['body']
        for text in [body + '\n' + body, '```roundos-release\n[]\n```', '```roundos-release\ninvalid\n```']:
            with self.subTest(text=text), self.assertRaises(stable.Blocked):
                stable.record(text, 'roundos-release')

    def test_requires_version_and_commit_on_main(self):
        self.github.excluded.add((SHA, 'main'))
        with self.assertRaisesRegex(stable.Blocked, 'not on main'):
            self.ready()
        self.github.excluded.clear()
        self.github.source_version = '0.5.1'
        with self.assertRaisesRegex(stable.Blocked, 'VERSION'):
            self.ready()

    def test_all_descendants_must_be_completed(self):
        self.github.issues[4] = issue(4)
        self.github.children[2] = [4]
        self.github.children[4] = []
        self.github.prs[4] = self.github.prs[2]
        self.ready()
        for state, reason in [('open', None), ('closed', 'not_planned')]:
            self.github.issues[4].update(state=state, state_reason=reason)
            with self.subTest(state=state), self.assertRaisesRegex(stable.Blocked, '#4'):
                self.ready()

    def test_historical_workflow_changes_block_before_build_with_default_token(self):
        self.github.main_workflows.append({'name': 'other.yml', 'type': 'file', 'sha': 'd' * 40})
        with self.assertRaisesRegex(stable.Blocked, 'GITHUB_TOKEN cannot publish'):
            self.ready()

    def test_no_children_or_wrong_acceptance_relationship_blocks(self):
        self.github.children[1] = []
        with self.assertRaisesRegex(stable.Blocked, 'no native sub-issues'):
            self.ready()
        self.github.children[1] = [2]
        with self.assertRaisesRegex(stable.Blocked, 'Acceptance issue'):
            self.ready()

    def test_rejects_cross_repository_cycles_and_abandoned_parent(self):
        self.github.issues[2]['repository_url'] = 'https://api.github.com/repos/other/repo'
        with self.assertRaisesRegex(stable.Blocked, 'Cross-repository'):
            self.ready()
        self.github = Fixture()
        self.github.children[2] = [1]
        with self.assertRaisesRegex(stable.Blocked, 'Cyclic'):
            self.ready()
        self.github = Fixture()
        self.github.issues[1].update(state='closed', state_reason='not_planned')
        with self.assertRaisesRegex(stable.Blocked, 'abandoned'):
            self.ready()

    def test_implementation_requires_merged_main_pr_in_release_commit(self):
        for field, value in [('merged', False), ('baseRefName', 'feature'), ('mergeCommit', None),
                             ('repository', {'nameWithOwner': 'other/repo'})]:
            with self.subTest(field=field):
                self.github = Fixture()
                self.github.prs[2][0][field] = value
                with self.assertRaisesRegex(stable.Blocked, 'PR #10'):
                    self.ready()
        self.github = Fixture()
        self.github.excluded.add((MERGE, SHA))
        with self.assertRaisesRegex(stable.Blocked, 'PR #10'):
            self.ready()
        self.github.prs[2] = []
        with self.assertRaisesRegex(stable.Blocked, 'no closing PR'):
            self.ready()

    def test_manual_verification_exception_requires_maintainer_issue(self):
        self.github.prs[2] = []
        self.github.issues[2]['labels'] = [{'name': stable.MANUAL_LABEL}]
        self.ready()
        self.github.issues[2]['user']['login'] = 'outsider'
        with self.assertRaisesRegex(stable.Blocked, 'Manual verification'):
            self.ready()

    def test_requires_current_maintainer_acceptance_and_evidence(self):
        original = copy.deepcopy(self.github.comments[0])
        values = [dict(version=VERSION, commit='c' * 40, result='passed', evidence=f'https://github.com/{REPO}/issues/3'),
                  dict(version='0.5.0', commit=SHA, result='passed', evidence=f'https://github.com/{REPO}/issues/3'),
                  dict(version=VERSION, commit=SHA, result='failed', evidence=f'https://github.com/{REPO}/issues/3'),
                  dict(version=VERSION, commit=SHA, result='passed', evidence='https://example.com/results')]
        for value in values:
            self.github.comments[0]['body'] = block_record('roundos-acceptance', value)
            with self.subTest(value=value), self.assertRaisesRegex(stable.Blocked, 'No maintainer Pi acceptance'):
                self.ready()
        self.github.comments[0] = original
        self.github.comments[0]['user']['login'] = 'outsider'
        with self.assertRaisesRegex(stable.Blocked, 'No maintainer Pi acceptance'):
            self.ready()

    def test_pending_ci_and_newer_stable_block_publication(self):
        self.github.ci = False
        with self.assertRaisesRegex(stable.Blocked, 'CI pending'):
            self.ready()
        self.github.ci = True
        self.github.releases = [{'tag_name': 'v0.7.0', 'draft': False, 'prerelease': False}]
        with self.assertRaisesRegex(stable.Blocked, 'newer stable'):
            self.ready()

    def test_discovery_skips_blocked_and_already_published_parents(self):
        with mock.patch.object(stable.release, 'api', return_value=None):
            self.assertEqual(len(stable.candidates(self.github)), 1)
            self.github.ci = False
            self.assertEqual(stable.candidates(self.github), [])
        self.github.ci = True
        with mock.patch.object(stable.release, 'api', return_value={
                'draft': False, 'prerelease': False, 'target_commitish': SHA}):
            self.assertEqual(stable.candidates(self.github), [])

    def test_publish_rechecks_declaration_and_passes_acceptance_notes(self):
        with tempfile.TemporaryDirectory() as temp:
            args = ['stable_release.py', 'publish', '--parent', '1', '--version', VERSION,
                    '--sha', SHA, '--output', temp]
            with mock.patch.object(sys, 'argv', args), mock.patch.dict(os.environ, GH_REPO=REPO), \
                    mock.patch.object(stable, 'GitHub', return_value=self.github), \
                    mock.patch.object(stable.release, 'publish') as publish:
                stable.main()
                self.assertTrue(publish.call_args.kwargs['stable'])
                self.assertIn(self.github.comments[0]['html_url'], publish.call_args.kwargs['notes'])
                publish.reset_mock()
                self.github.ci = False
                with self.assertRaises(stable.Blocked):
                    stable.main()
                publish.assert_not_called()
                self.github.ci = True
                args[args.index(VERSION)] = '0.6.1'
                with self.assertRaisesRegex(stable.Blocked, 'declaration changed'):
                    stable.main()
                publish.assert_not_called()


class GitHubTests(unittest.TestCase):
    def test_noncollaborator_is_untrusted_rather_than_failing_discovery(self):
        with mock.patch.object(stable.release, 'api', return_value=None) as api:
            self.assertFalse(stable.GitHub(REPO).trusted('outsider'))
            self.assertTrue(api.call_args.kwargs['missing_ok'])

    def test_query_includes_merged_closing_prs(self):
        connection = {'nodes': [{'number': 10, 'merged': True}], 'pageInfo': {'hasNextPage': False}}
        response = {'data': {'repository': {'issue': {'closedByPullRequestsReferences': connection}}}}
        with mock.patch.object(stable.release, 'gh', return_value=json.dumps(response)) as gh:
            self.assertEqual(stable.GitHub(REPO).closing_prs(2), connection['nodes'])
            query = next(arg for arg in gh.call_args.args if arg.startswith('query='))
            self.assertIn('includeClosedPrs:true', query)

    def test_missing_commit_blocks_parent_but_api_outage_is_not_hidden(self):
        github = stable.GitHub(REPO)
        with mock.patch.object(stable.release, 'api', return_value=None) as api:
            with self.assertRaisesRegex(stable.Blocked, 'unknown or missing commit'):
                github.included(SHA, 'main')
            self.assertTrue(api.call_args.kwargs['missing_ok'])
        with mock.patch.object(stable.release, 'api', side_effect=RuntimeError('HTTP 503')):
            with self.assertRaisesRegex(RuntimeError, '503'):
                github.included(SHA, 'main')

    def test_pagination_is_flattened_and_query_is_preserved(self):
        with mock.patch.object(stable.release, 'gh', return_value='[[{"number":1}],[{"number":2}]]') as gh:
            self.assertEqual(len(stable.GitHub(REPO).pages('issues?state=all')), 2)
            self.assertIn('state=all&per_page=100', gh.call_args.args[1])

    def test_ci_uses_latest_main_push_attempt_and_requires_jobs(self):
        github = stable.GitHub(REPO)
        run = dict(id=10, run_number=42, run_attempt=2, head_sha=SHA, event='push',
                   head_branch='main', status='completed', conclusion='success')
        jobs = [{'name': name, 'conclusion': 'success'} for name in
                ('Tests', 'Host DRM build', 'ARMv7 build and package')]
        with mock.patch.object(github, 'get', return_value={'workflow_runs': [run]}) as get, \
                mock.patch.object(stable.release, 'gh', return_value=json.dumps([{'jobs': jobs}])):
            github.ci_passed(SHA)
            self.assertIn('event=push&branch=main&head_sha=' + SHA, get.call_args.args[0])
            jobs.pop()
        with mock.patch.object(github, 'get', return_value={'workflow_runs': [run]}), \
                mock.patch.object(stable.release, 'gh', return_value=json.dumps([{'jobs': jobs}])):
            with self.assertRaisesRegex(stable.Blocked, 'Required CI jobs'):
                github.ci_passed(SHA)
        failed = dict(run, run_attempt=3, conclusion='failure')
        with mock.patch.object(github, 'get', return_value={'workflow_runs': [run, failed]}):
            with self.assertRaisesRegex(stable.Blocked, 'pending or failed'):
                github.ci_passed(SHA)


if __name__ == '__main__':
    unittest.main()

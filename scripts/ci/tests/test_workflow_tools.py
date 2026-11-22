"""Offline tests for the CLI's GitHub decisions and mutation boundaries."""

from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path
import sys
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import github_tools as tools
import release_parent as parent

REPO = 'drazafsky/roundos'
REPO_URL = f'https://api.github.com/repos/{REPO}'
SHA = 'a' * 40


def issue(number, *, body='', state='open', assignee='drazafsky', labels=()):
    return dict(number=number, id=100 + number, node_id=f'issue-{number}', body=body,
                state=state, assignees=[{'login': assignee}], user={'login': 'drazafsky'},
                labels=[{'name': name} for name in labels], updated_at='2026-11-22T00:00:00Z', repository_url=REPO_URL)


class GitHubToolTests(unittest.TestCase):
    def test_issue_lookup_excludes_cross_repo_links_code_and_prs(self):
        github = mock.Mock(repo=REPO)
        pr = dict(body='Refs #20 and https://github.com/drazafsky/roundos/issues/18. '
                       'Please see other/repo#1, `#5`, and #42.\n```\n#6\n```\n<!-- #7 -->',
                  closingIssuesReferences=[{'url': f'https://github.com/{REPO}/issues/19', 'number': 19}],
                  assignees=[{'login': 'owner'}])
        objects = {18: issue(18, assignee='owner', state='closed'), 19: issue(19), 20: issue(20),
                   42: {'pull_request': {}}}
        github.api.side_effect = lambda path, **_: objects[int(path.split('/')[-1])]
        linked = tools.tracked_issues(github, pr)
        self.assertEqual([item['number'] for item in linked], [18, 19, 20])
        self.assertEqual(tools.linked_assignees(pr, linked), [])
        pr['assignees'] = [{'login': 'drazafsky'}]
        self.assertEqual(tools.linked_assignees(pr, linked), ['PR assignees must match #18: owner'])

    def test_exact_commit_ci_checks_and_copilot(self):
        github = mock.Mock(repo=REPO)
        sha = SHA
        pr = dict(state='OPEN', isDraft=False, body='Closes #45', headRefOid=sha, mergeStateStatus='CLEAN',
                  assignees=[{'login': 'drazafsky'}], closingIssuesReferences=[], statusCheckRollup=[
                      {'__typename': 'CheckRun', 'status': 'COMPLETED', 'conclusion': 'SUCCESS', 'name': name}
                      for name in tools.CI_JOBS])
        run = dict(head_sha=sha, workflow_id=1, event='pull_request', run_number=2, run_attempt=1,
                   path='.github/workflows/ci.yml', name='RoundOS CI', html_url='run-url', status='completed', conclusion='success')
        review = dict(user={'login': 'copilot-pull-request-reviewer[bot]'}, commit_id='old', submitted_at='now',
                      state='COMMENTED', body='Check edge cases', html_url='review-url')
        comments = [{'body': 'Maintainer feedback', 'html_url': 'comment-url'}]
        github.api.return_value = issue(45)
        github.repo_graphql.return_value = {'pullRequest': {'reviewThreads': {'nodes': [], 'pageInfo': {'hasNextPage': False}}}}
        github.pages.side_effect = lambda path, *_: [run] if path.startswith('actions/') else [review] if 'reviews' in path else comments
        with mock.patch.object(tools, 'pr_data', return_value=pr):
            ok, lines, _ = tools.pr_report(github, 7)
            self.assertFalse(ok)
            self.assertIn('Copilot has not submitted a review for this commit', lines)
            review['commit_id'] = sha
            ok, lines, details = tools.pr_report(github, 7, feedback=True)
            self.assertTrue(ok)
            self.assertIn('GitHub checks passed', lines)
            self.assertEqual(details, ['review-url\nCheck edge cases', 'comment-url\nMaintainer feedback'])
            run['conclusion'] = 'failure'
            self.assertFalse(tools.pr_report(github, 7)[0])
        with mock.patch.object(tools, 'pr_data', side_effect=[pr, dict(pr, headRefOid='new')]):
            self.assertIn('PR commit changed during the check; run again', tools.pr_report(github, 7)[1])

    def test_review_threads_page_to_completion(self):
        github = mock.Mock()
        github.repo_graphql.side_effect = [
            {'pullRequest': {'reviewThreads': {'nodes': [], 'pageInfo': {'hasNextPage': True, 'endCursor': 'next'}}}},
            {'pullRequest': {'reviewThreads': {'nodes': [{'isResolved': False}], 'pageInfo': {'hasNextPage': False}}}},
        ]
        self.assertEqual(list(tools.review_threads(github, 10)), [{'isResolved': False}])
        self.assertEqual(github.repo_graphql.call_args.kwargs['cursor'], 'next')

    def test_missing_project_scope_prevents_even_the_add_mutation(self):
        github = mock.Mock()
        with mock.patch.object(tools, 'gh', return_value='HTTP/2 200\nX-Oauth-Scopes: repo, read:project\n\n{}'):
            with self.assertRaisesRegex(tools.ToolError, 'Projects write access'):
                tools.project_status(github, [issue(45)], 'In Progress', project_name='RoundOS', apply=True)
        github.graphql.assert_not_called()

    def test_project_status_preview_and_unchanged_status_do_not_write(self):
        github = mock.Mock()
        project = {'id': 'project', 'title': 'RoundOS', 'field': {'id': 'status', 'options': [{'id': 'option', 'name': 'PR Review'}]}}
        state = {'name': 'In Progress'}
        items = {'nodes': [{'id': 'item', 'project': {'id': 'project'}, 'fieldValueByName': state}],
                 'pageInfo': {'hasNextPage': False}}
        project_data = {'projectsV2': {'nodes': [project], 'pageInfo': {'hasNextPage': False}}}
        github.repo_graphql.side_effect = [project_data, {'issue': {'projectItems': items}}] * 3
        github.graphql.return_value = {'updateProjectV2ItemFieldValue': {
            'projectV2Item': {'fieldValueByName': {'name': 'PR Review'}}}}
        with mock.patch.object(tools, 'gh', return_value='HTTP/2 200\nX-Oauth-Scopes: repo, project\n\n{}'), redirect_stdout(StringIO()):
            tools.project_status(github, [issue(45)], 'PR Review', project_name='RoundOS', apply=False)
            github.graphql.assert_not_called()
            tools.project_status(github, [issue(45)], 'PR Review', project_name='RoundOS', apply=True)
            github.graphql.assert_called_once()
            self.assertEqual(github.graphql.call_args.kwargs, dict(project='project', item='item', field='status', option='option'))
            state['name'] = 'PR Review'
            tools.project_status(github, [issue(45)], 'PR Review', project_name='RoundOS', apply=True)
            self.assertEqual(github.graphql.call_count, 1)


class ParentPlanTests(unittest.TestCase):
    def test_planned_section_preserves_original_and_repeated_runs_are_idempotent(self):
        original = 'Original requirements\n\n- Keep this\n'
        prepared = parent.replace_plan(original, '0.6.0', 15, None)
        self.assertTrue(prepared.startswith(original))
        self.assertNotIn('```roundos-release', prepared)
        opted = parent.replace_plan(prepared, '0.6.0', 15, SHA)
        self.assertEqual(parent.automation_record(opted), ('0.6.0', SHA, 15))
        self.assertEqual(parent.replace_plan(opted, '0.6.0', 15, SHA), opted)
        with self.assertRaisesRegex(tools.ToolError, 'declaration differs'):
            parent.replace_plan(opted, '0.6.1', 15, SHA)

    def test_attach_failure_reuses_existing_issue_on_retry(self):
        github = mock.Mock(repo=REPO)
        publisher = mock.Mock()
        publisher.trusted.return_value = True
        items = {28: issue(28, body='Original'), 44: issue(44)}
        attached = False
        failed = False

        def api(path, *args, **kwargs):
            nonlocal attached, failed
            if path == 'issues':
                body = next(arg[5:] for arg in args if arg.startswith('body='))
                items[44]['body'] = body
                return items[44]
            if path == 'issues/44/parent':
                return items[28] if attached else None
            if path == 'issues/28/sub_issues':
                self.assertIn('sub_issue_id=144', args)
                if not failed:
                    failed = True
                    raise tools.ToolError('Temporary attach failure')
                attached = True
                return items[28]
            number = int(path.split('/')[1])
            if 'PATCH' in args:
                items[number]['body'] = next(arg[5:] for arg in args if arg.startswith('body='))
            return items[number].copy()

        github.api.side_effect = api
        github.pages.side_effect = lambda _: list(items.values())
        with redirect_stdout(StringIO()):
            kwargs = dict(acceptance=None, create_acceptance=True, commit=None, apply=True)
            with self.assertRaisesRegex(tools.ToolError, 'Temporary attach failure'):
                parent.prepare(github, publisher, 28, '0.6.0', **kwargs)
            parent.prepare(github, publisher, 28, '0.6.0', **kwargs)
        creates = [call for call in github.api.call_args_list if call.args[0] == 'issues']
        self.assertEqual(len(creates), 1)
        self.assertIn('Pi acceptance: #44', items[28]['body'])
        self.assertFalse(items[28]['labels'])

    def test_preview_does_not_write_and_opt_in_updates_label_and_declaration_together(self):
        github = mock.Mock(repo=REPO)
        publisher = mock.Mock()
        original = issue(28, body='Manual description', labels=['keep'])

        def api(path, *args, **kwargs):
            if path == 'issues/15/parent':
                return original
            if path == 'issues/15':
                return issue(15)
            if not args:
                return original
            body = next(arg[5:] for arg in args if arg.startswith('body='))
            labels = [arg[9:] for arg in args if arg.startswith('labels[]=')]
            return issue(28, body=body, labels=labels)

        github.api.side_effect = api
        with redirect_stdout(StringIO()), mock.patch.object(parent.stable_release, 'candidate_source'):
            parent.prepare(github, publisher, 28, '0.6.0', acceptance=15, create_acceptance=False, commit=SHA, apply=False)
            self.assertTrue(all(len(call.args) == 1 for call in github.api.call_args_list))
            parent.prepare(github, publisher, 28, '0.6.0', acceptance=15, create_acceptance=False, commit=SHA, apply=True)
        writes = [call.args for call in github.api.call_args_list if 'PATCH' in call.args]
        self.assertEqual(len(writes), 1)
        self.assertIn('labels[]=keep', writes[0])
        self.assertIn('labels[]=release:stable', writes[0])
        self.assertEqual(parent.automation_record(next(arg[5:] for arg in writes[0] if arg.startswith('body='))), ('0.6.0', SHA, 15))

"""RoundOS review evidence, reviewer failures, and closeout replay."""
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path
import sys
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import review_feedback as feedback


def submission(body):
    return dict(user={'login': 'copilot-pull-request-reviewer[bot]'}, state='COMMENTED',
                body=body, submitted_at='2026-11-25', html_url='review-url', commit_id='abc')


class CloseoutTests(unittest.TestCase):
    def test_paginated_links_choose_unique_local_merged_prs(self):
        github = mock.Mock(repo='example/project')
        pr = dict(number=9, merged=True, repository={'nameWithOwner': 'Example/Project'})
        def issue_page(prs, more):
            return {'issue': {'state': 'CLOSED', 'closedByPullRequestsReferences': {
                'pageInfo': {'hasNextPage': more, 'endCursor': 'next'}, 'nodes': prs}}}
        pages = [
            issue_page([pr, dict(pr, merged=False, number=11)], True),
            issue_page([pr, dict(pr, number=12, repository={'nameWithOwner': 'other/repo'})], False)]
        references = [dict(pr, number=3, body='Refs #1'), dict(pr, number=4, body='Example: `#1`')]
        github.repo_graphql.side_effect = pages + [{'issue': {'state': 'CLOSED', 'timelineItems': {
            'nodes': [{'source': ref} for ref in references], 'pageInfo': {'hasNextPage': False}}}}]
        self.assertEqual(feedback.closing_prs(github, 1), [3, 9])
        self.assertEqual(github.repo_graphql.call_args_list[1].kwargs['cursor'], 'next')
        github.repo_graphql.side_effect = None
        github.repo_graphql.return_value = {'issue': {'state': 'OPEN'}}
        self.assertEqual(feedback.closing_prs(github, 1), [])

    def test_evidence_includes_summary_only_findings_inline_replies_and_resolutions(self):
        github = mock.Mock()
        github.api.return_value = dict(merged_at='now', labels=[], html_url='pr-url', head={'sha': 'abc'}, body='Local checks')
        summary = submission('No open issues. A retained measurement can mask a regression.')
        quota = submission('Copilot was unable to review this pull request because the user has insufficient remaining quota.')
        failure = submission('One or more reviewer sessions failed, leaving the review evidence incomplete.')
        inline = [{'body': 'Failure reported', 'id': 3}, {'in_reply_to_id': 3, 'body': 'Fixed'}]
        bot = dict(user={'login': 'github-actions[bot]'}, body=feedback.MARKER + '\nrecord', id=19)
        mimic = dict(bot, user={'login': 'someone'})
        pages = {'issues/12/comments': [bot, mimic], 'pulls/12/reviews': [quota, failure, summary],
                 'pulls/12/comments': inline}
        github.pages.side_effect = pages.__getitem__
        github.repo_graphql.return_value = {'pullRequest': {'reviewThreads': {
            'nodes': [{'isResolved': True}], 'pageInfo': {'hasNextPage': False}}}}
        evidence, existing = feedback.collect(github, 12)
        self.assertEqual(existing, bot)
        self.assertEqual(evidence['conversation'], [mimic])
        self.assertEqual(evidence['inline'], inline)
        self.assertEqual(evidence['description'], 'Local checks')
        self.assertEqual([r['copilot_outcome'] for r in evidence['reviews']], ['quota', 'incomplete', 'substantive'])
        self.assertEqual(evidence['reviews'][-1]['body'], summary['body'])
        body = feedback.record_body(evidence)
        self.assertIn('1 substantive, 1 incomplete, 1 quota failures', body)
        self.assertIn('1 of 1 review threads resolved', body)

    def test_repeat_collection_keeps_one_record_and_preserves_assessed_queue_state(self):
        github = mock.Mock()
        evidence = dict(pr=1, url='pr', head='abc', reviews=[], threads=[], conversation=[], labels=[])
        body = feedback.record_body(evidence)
        github.api.side_effect = [{}, None, {}, {}]
        feedback.save_record(github, evidence, None)
        self.assertEqual([c.args[0] for c in github.api.call_args_list],
                         ['issues/1/comments', 'labels/review:closeout', 'labels', 'issues/1/labels'])
        github.api.reset_mock(side_effect=True)
        # Retry after comment success and label failure completes the missing write.
        feedback.save_record(github, evidence, dict(body=body, id=9))
        self.assertEqual(github.api.call_args.args[0], 'issues/1/labels')
        github.api.reset_mock()
        evidence['conversation'] = [dict(body='## Closeout assessment\nExisting test covers the finding.',
                                         author_association='OWNER')]
        feedback.save_record(github, evidence, dict(body=body, id=9))
        github.api.assert_not_called()
        feedback.save_record(github, evidence, dict(body='older record', id=9))
        self.assertEqual(github.api.call_args.args[:3], ('issues/comments/9', '--method', 'PATCH'))

    def test_local_credential_cannot_publish_a_bot_record(self):
        with mock.patch.object(sys, 'argv', ['feedback', 'evidence', '9', '--record']), mock.patch.object(feedback, 'GitHub') as client:
            client.return_value.graphql.return_value = {'viewer': {'login': 'maintainer'}}
            with self.assertRaisesRegex(feedback.ToolError, 'GitHub Actions'):
                feedback.main()
            client.return_value.api.assert_not_called()

    def test_issue_start_selects_review_questions_and_surfaces_external_closeouts(self):
        github = mock.Mock(repo='example/repo')
        issue = dict(title='Launcher carousel', body=None, labels=[])
        output = StringIO()
        with mock.patch.object(feedback, 'gh', return_value='[{"url":"queued-pr-url"}]'), redirect_stdout(output):
            feedback.start_handoff(github, issue)
        self.assertIn('Controls and input', output.getvalue())
        self.assertNotIn('Persistence and recovery', output.getvalue())
        self.assertIn('queued-pr-url', output.getvalue())

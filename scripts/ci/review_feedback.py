#!/usr/bin/env python3
"""Collect and retrieve RoundOS review evidence using the repository's gh CLI."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

from workflow_github import GitHub, REPO, ToolError, gh, positive, referenced_numbers

COPILOT = {'copilot-pull-request-reviewer', 'copilot-pull-request-reviewer[bot]'}
MARKER = '<!-- roundos-review-closeout-v1 -->'
PENDING = 'review:closeout'
PLAYBOOK = Path(__file__).resolve().parents[2] / 'docs' / 'PRE-PR-REVIEW.md'
CLOSING = '''query($owner:String!,$name:String!,$number:Int!,$cursor:String) {
  repository(owner:$owner,name:$name) { issue(number:$number) {
    state closedByPullRequestsReferences(first:100,after:$cursor,includeClosedPrs:true) {
      nodes { number merged repository { nameWithOwner } } pageInfo { hasNextPage endCursor }
    }
  } }
}'''
REFERENCES = '''query($owner:String!,$name:String!,$number:Int!,$cursor:String) {
  repository(owner:$owner,name:$name) { issue(number:$number) {
    state timelineItems(first:100,after:$cursor,itemTypes:[CROSS_REFERENCED_EVENT]) {
      nodes { ... on CrossReferencedEvent { source { ... on PullRequest {
        number body merged repository { nameWithOwner }
      } } } } pageInfo { hasNextPage endCursor }
    }
  } }
}'''
THREADS = '''query($owner:String!,$name:String!,$number:Int!,$cursor:String) {
  repository(owner:$owner,name:$name) { pullRequest(number:$number) {
    reviewThreads(first:100,after:$cursor) { nodes { isResolved comments(first:1) { nodes { url } } }
      pageInfo { hasNextPage endCursor } }
  } }
}'''


def review_kind(review):
    """Classify submitted Copilot reviews; other authors and pending reviews are irrelevant."""
    if ((review.get('user') or {}).get('login') not in COPILOT or not review.get('submitted_at')
            or review['state'] not in {'APPROVED', 'COMMENTED', 'CHANGES_REQUESTED', 'DISMISSED'}):
        return None
    # Look only at the overview before its file summaries, which can quote arbitrary code.
    summary = (review.get('body') or '').partition('<details')[0].lower()
    unable = re.search(r"^copilot (?:encountered an error and )?(?:was unable to|wasn't able to|was not able to|could not|couldn't) review\b",
                       summary.replace('’', "'"), re.M)
    if unable:
        return 'quota' if 'quota' in summary else 'incomplete'
    if re.search(r'^(?:one or more reviewer sessions failed|(?:the |a )?review session failed)\b', summary, re.M):
        return 'incomplete'
    return 'substantive'


def connection(github, number, query, resource, field):
    args = {'number': number}
    while True:
        item = github.repo_graphql(query, **args)[resource]
        if item is None:
            raise ToolError(f'Cannot read {resource} #{number}')
        if resource == 'issue' and item['state'] != 'CLOSED':
            return
        page = item[field]
        yield from page['nodes']
        if not page['pageInfo']['hasNextPage']:
            return
        args['cursor'] = page['pageInfo']['endCursor']


def closing_prs(github, issue):
    prs = connection(github, issue, CLOSING, 'issue', 'closedByPullRequestsReferences')
    def local_merged(pr):
        return pr.get('merged') and pr['repository']['nameWithOwner'].lower() == github.repo.lower()

    numbers = {pr['number'] for pr in prs if local_merged(pr)}
    for event in connection(github, issue, REFERENCES, 'issue', 'timelineItems'):
        pr = event.get('source') or {}
        if local_merged(pr) and issue in referenced_numbers(github.repo, pr['body']):
            numbers.add(pr['number'])
    return sorted(numbers)


def review_threads(github, number):
    return connection(github, number, THREADS, 'pullRequest', 'reviewThreads')


def is_record(comment):
    return ((comment.get('user') or {}).get('login') == 'github-actions[bot]'
            and (comment.get('body') or '').startswith(MARKER + '\n'))


def collect(github, number):
    pr = github.api(f'pulls/{number}')
    if not pr['merged_at']:
        raise ToolError(f'PR #{number} has not merged')
    comments = list(github.pages(f'issues/{number}/comments'))
    reviews = list(github.pages(f'pulls/{number}/reviews'))
    for review in reviews:
        review['copilot_outcome'] = review_kind(review)
    evidence = dict(pr=number, url=pr['html_url'], description=pr['body'], head=pr['head']['sha'],
                    merged_at=pr['merged_at'], labels=[label['name'] for label in pr['labels']],
                    reviews=reviews, threads=list(review_threads(github, number)),
                    inline=list(github.pages(f'pulls/{number}/comments')),
                    conversation=[c for c in comments if not is_record(c)])
    return evidence, next((c for c in comments if is_record(c)), None)


def record_body(evidence):
    url = evidence['url']
    reviews = [r for r in evidence['reviews'] if r['copilot_outcome']]
    counts = {kind: sum(r['copilot_outcome'] == kind for r in reviews)
              for kind in ('substantive', 'incomplete', 'quota')}
    lines = [MARKER, '## Review closeout', '',
             f"Merged PR head `{evidence['head']}`.",
             f"Copilot submissions: {counts['substantive']} substantive, {counts['incomplete']} incomplete, "
             f"{counts['quota']} quota failures.", '']
    for review in reviews[:100]:
        sha = (review.get('commit_id') or 'unknown')[:12]
        lines.append(f"- [{review['submitted_at']}]({review['html_url']}) · `{sha}` · {review['copilot_outcome']}")
    threads = evidence['threads']
    lines += ['', f"{sum(t['isResolved'] for t in threads)} of {len(threads)} review threads resolved.",
              f"[Pre-review note and conversation]({url}) · [Inline findings and replies]({url}/files)", '',
              f"Read the full evidence with `python3 scripts/ci/review_feedback.py evidence {evidence['pr']}`.",
              'Record confirmed misses and their prevention checks using docs/PRE-PR-REVIEW.md.']
    return '\n'.join(lines)


def save_record(github, evidence, existing):
    number = evidence['pr']
    body = record_body(evidence)
    if not existing:
        github.api(f'issues/{number}/comments', '--method', 'POST', '-f', f'body={body}')
    elif existing['body'] != body:
        github.api(f"issues/comments/{existing['id']}", '--method', 'PATCH', '-f', f'body={body}')
    # The assessment survives label removal and makes replay safe after a partial write.
    assessed = any(c.get('author_association') in {'OWNER', 'MEMBER', 'COLLABORATOR'}
                   and (c.get('body') or '').splitlines()[:1] == ['## Closeout assessment']
                   for c in evidence['conversation'])
    if PENDING in evidence['labels'] or assessed:
        return
    if not github.api(f'labels/{PENDING}', missing_ok=True):
        try:
            github.api('labels', '--method', 'POST', '-f', f'name={PENDING}', '-f', 'color=537895')
        except ToolError:
            if not github.api(f'labels/{PENDING}', missing_ok=True):
                raise
    github.api(f'issues/{number}/labels', '--method', 'POST', '-f', f'labels[]={PENDING}')


def matching_lessons(context):
    sections = re.split(r'^### ', PLAYBOOK.read_text(), flags=re.M)[1:]
    for section in sections:
        tags = re.search(r'^Applies to: (.+)$', section, re.M)
        if tags and any(re.search(r'(?<![a-z0-9])' + re.escape(tag), context, re.I)
                        for tag in tags[1].split(', ')):
            yield section.splitlines()[0] + ' — ' + section.split('\n\n')[1].strip()


def start_handoff(github, issue):
    context = ' '.join([issue['title'], issue.get('body') or '', *(x['name'] for x in issue['labels'])])
    print('Local review: docs/PRE-PR-REVIEW.md (recheck against the final diff).')
    for lesson in matching_lessons(context):
        print(lesson)
    queued = json.loads(gh('pr', 'list', '--repo', github.repo, '--state', 'merged', '--label', PENDING,
                           '--search', 'sort:created-asc', '--limit', '5', '--json', 'url'))
    if queued:
        print('Pending closeouts (up to five). Assess the first entry before implementation:')
        for pr in queued:
            print(pr['url'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default=os.environ.get('GITHUB_REPOSITORY', REPO))
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('closing-prs').add_argument('issue', type=positive)
    evidence = commands.add_parser('evidence')
    evidence.add_argument('pr', type=positive)
    evidence.add_argument('--record', action='store_true', help='Write the queue entry using the Actions bot token')
    args = parser.parse_args()
    github = GitHub(args.repo)
    if args.command == 'closing-prs':
        print(json.dumps(closing_prs(github, args.issue)))
        return
    if args.record and github.graphql('query { viewer { login } }')['viewer']['login'] != 'github-actions[bot]':
        raise ToolError('Closeout records are written by GitHub Actions. Dispatch review-closeout.yml to reconcile.')
    data, existing = collect(github, args.pr)
    if args.record:
        save_record(github, data, existing)
    print(json.dumps(data, indent=2))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.TimeoutExpired) as error:
        sys.exit(str(error))

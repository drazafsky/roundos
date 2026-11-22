#!/usr/bin/env python3
"""Reconcile opted-in release parents; publish only accepted, tested commits."""
import argparse
import base64
import json
import os
from pathlib import Path
import re
import sys

import release

LABEL = 'release:stable'
MANUAL_LABEL = 'release:verification'
MAX_ISSUES = 46  # One parent plus at most 45 descendants.
MAX_API_READS = 240  # Budget per eligibility pass, including discovery across candidates.


class Blocked(ValueError):
    """A release is not ready; reconsider it on the next reconciliation."""


def block(condition, message):
    if not condition:
        raise Blocked(message)


def record(body, kind):
    matches = re.findall(r'^```' + re.escape(kind) + r'\s*\n(.*?)\n```\s*$', body or '', re.M | re.S)
    block(len(matches) == 1, f'Expected one {kind} JSON block')
    try:
        value = json.loads(matches[0])
    except json.JSONDecodeError as error:
        raise Blocked(f'Invalid {kind} JSON') from error
    block(isinstance(value, dict), f'{kind} must be an object')
    return value


class GitHub:
    def __init__(self, repo):
        block(bool(re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo)), 'Invalid repository')
        self.repo = repo
        self.prefix = f'repos/{repo}'
        self.permissions = {}
        self.reads_left = MAX_API_READS

    def reserve_reads(self, count=1):
        block(self.reads_left >= count, 'Release reconciliation exceeded its API request budget')
        self.reads_left -= count

    def get(self, path, *, missing_ok=False):
        self.reserve_reads()
        return release.api(f'{self.prefix}/{path}', missing_ok=missing_ok)

    def pages(self, path, *, key=None):
        separator = '&' if '?' in path else '?'
        items = []
        page = 1
        while True:
            response = self.get(f'{path}{separator}per_page=100&page={page}')
            batch = response[key] if key else response
            items.extend(batch)
            if len(batch) < 100:
                return items
            page += 1

    def trusted(self, login):
        if login not in self.permissions:
            result = self.get(f'collaborators/{login}/permission', missing_ok=True)
            self.permissions[login] = bool(result and result['permission'] in ('admin', 'maintain', 'write'))
        return self.permissions[login]

    def included(self, ancestor, descendant):
        comparison = self.get(f'compare/{ancestor}...{descendant}', missing_ok=True)
        block(comparison is not None, 'Commit comparison is unavailable; unknown or missing commit')
        return comparison['status'] in ('ahead', 'identical')

    def closing_prs(self, number):
        self.reserve_reads()
        owner, name = self.repo.split('/')
        query = '''query($owner:String!,$name:String!,$number:Int!) {
          repository(owner:$owner,name:$name) { issue(number:$number) {
            closedByPullRequestsReferences(first:100,includeClosedPrs:true) {
              nodes { number state merged baseRefName mergeCommit { oid } repository { nameWithOwner } }
              pageInfo { hasNextPage }
            }
          } }
        }'''
        result = json.loads(release.gh('api', 'graphql', '-f', f'query={query}', '-f', f'owner={owner}',
                                      '-f', f'name={name}', '-F', f'number={number}'))
        block(not result.get('errors'), 'Unable to read closing PRs')
        connection = result['data']['repository']['issue']['closedByPullRequestsReferences']
        block(not connection['pageInfo']['hasNextPage'], 'Too many closing PRs')
        return connection['nodes']

    def ci_passed(self, sha):
        # Use the newest push run (including its latest attempt), never an old
        # success or a PR's synthetic merge result.
        result = self.get(f'actions/workflows/ci.yml/runs?event=push&branch=main&head_sha={sha}&per_page=100')
        runs = result['workflow_runs']
        block(bool(runs), 'No main CI run for the release commit')
        run = max(runs, key=lambda item: (item['run_number'], item['run_attempt']))
        block(run['head_sha'] == sha and run['event'] == 'push' and run['head_branch'] == 'main'
              and run['status'] == 'completed' and run['conclusion'] == 'success', 'Main CI is pending or failed')
        jobs = {job['name']: job['conclusion'] for job in self.pages(f'actions/runs/{run["id"]}/jobs?filter=latest', key='jobs')}
        block(all(jobs.get(name) == 'success' for name in
                  ('Tests', 'Host DRM build', 'ARMv7 build and package')), 'Required CI jobs did not pass')


def declaration(github, number):
    parent = github.get(f'issues/{number}')
    labels = {label['name'] for label in parent['labels']}
    block(LABEL in labels and 'pull_request' not in parent, 'Parent is not opted in')
    block(parent['state'] == 'open' or parent.get('state_reason') == 'completed', 'Parent was abandoned')
    block(github.trusted(parent['user']['login']), 'Release parent must be authored by a maintainer')
    config = record(parent.get('body'), 'roundos-release')
    block(set(config) == {'version', 'commit', 'acceptance_issue'}, 'Unexpected release declaration fields')
    version, sha, acceptance = config['version'], config['commit'], config['acceptance_issue']
    block(isinstance(version, str) and len(version) <= 64 and
          bool(re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version)), 'Invalid stable version')
    block(isinstance(sha, str) and bool(re.fullmatch(r'[0-9a-f]{40}', sha)), 'Expected a full commit SHA')
    block(type(acceptance) is int and acceptance > 0, 'Expected an acceptance issue number')
    block(all(int(part) <= 2**64 - 1 for part in version.split('.')), 'Version component is too large')
    return version, sha, acceptance


def eligibility(github, number, config=None):
    version, sha, acceptance = config if config else declaration(github, number)
    target = tuple(map(int, version.split('.')))
    for existing in github.pages('releases'):
        tag = existing['tag_name'].removeprefix('v')
        if existing['draft'] or existing['prerelease'] or not re.fullmatch(r'\d+\.\d+\.\d+', tag):
            continue
        block(tuple(map(int, tag.split('.'))) <= target, 'A newer stable release is already published')
    block(github.included(sha, 'main'), 'Release commit is not on main')
    source_version = github.get(f'contents/VERSION?ref={sha}', missing_ok=True)
    block(source_version is not None, 'Release commit has no VERSION file')
    block(base64.b64decode(source_version['content']).decode().strip() == version,
          'VERSION at the release commit differs from the target version')
    # GITHUB_TOKEN cannot create or update a release targeting historical
    # workflow content that differs from the default branch. Detect this before
    # an expensive build; never silently replace the accepted commit with main.
    def workflows(ref):
        entries = github.get(f'contents/.github/workflows?ref={ref}', missing_ok=True)
        block(isinstance(entries, list), 'Release commit has no workflow directory')
        return sorted((entry['name'], entry['type'], entry['sha']) for entry in entries)
    block(workflows(sha) == workflows('main'),
          'Workflow files differ from main; GITHUB_TOKEN cannot publish this revision. Accept a commit with current workflows')

    seen = {number}
    descendants = {}

    def visit(issue_number):
        children = github.pages(f'issues/{issue_number}/sub_issues')
        for child in children:
            child_number = child['number']
            block(child['repository_url'] == f'https://api.github.com/repos/{github.repo}', 'Cross-repository sub-issue')
            block(child_number not in seen and len(seen) < MAX_ISSUES, 'Cyclic or oversized issue tree')
            seen.add(child_number)
            # Refetch: list responses may be stale relative to a closure event.
            child = github.get(f'issues/{child_number}')
            block(child['state'] == 'closed' and child.get('state_reason') == 'completed',
                  f'Issue #{child_number} is not completed')
            descendants[child_number] = child
            has_children = visit(child_number)
            closing_prs = github.closing_prs(child_number)
            for pr in closing_prs:
                block(pr['state'] != 'OPEN', f'Closing PR #{pr["number"]} is still open')
            prs = [pr for pr in closing_prs if pr['merged']]
            manual = MANUAL_LABEL in {label['name'] for label in child['labels']}
            if manual:
                block(github.trusted(child['user']['login']), 'Manual verification issues must be authored by a maintainer')
            block(has_children or child_number == acceptance or manual or bool(prs),
                  f'Implementation issue #{child_number} has no closing PR')
            for pr in prs:
                block(pr['baseRefName'] == 'main' and pr['repository']['nameWithOwner'] == github.repo
                      and pr['mergeCommit'] and github.included(pr['mergeCommit']['oid'], sha),
                      f'PR #{pr["number"]} is not merged into the selected release commit')
        return bool(children)

    block(visit(number), 'Release parent has no native sub-issues')
    block(acceptance in descendants, 'Acceptance issue must be a completed descendant')
    evidence = None
    for comment in github.pages(f'issues/{acceptance}/comments'):
        if '```roundos-acceptance' not in (comment.get('body') or ''):
            continue
        if not github.trusted(comment['user']['login']):
            continue
        try:
            accepted = record(comment['body'], 'roundos-acceptance')
        except Blocked:
            continue
        # This is an operator attestation with evidence, not a claim that CI
        # can establish physical-device behavior.
        if (accepted.get('version') == version and accepted.get('commit') == sha
                and accepted.get('result') == 'passed' and isinstance(accepted.get('evidence'), str)
                and accepted['evidence'].startswith(f'https://github.com/{github.repo}/')
                and len(accepted['evidence']) > len(f'https://github.com/{github.repo}/')):
            evidence = comment['html_url']
    block(evidence is not None, 'No maintainer Pi acceptance for this version and commit')
    github.ci_passed(sha)
    return {'parent': number, 'version': version, 'sha': sha, 'acceptance': evidence}


def candidates(github):
    ready = []
    for issue in github.pages('issues?state=all&labels=release%3Astable'):
        if 'pull_request' in issue:
            continue
        try:
            config = declaration(github, issue['number'])
            version, sha, _ = config
            tag = 'v' + version
            existing = github.get(f'releases/tags/{tag}', missing_ok=True)
            if existing and not existing['draft']:
                block(not existing['prerelease'] and existing['target_commitish'] == sha, 'Existing release identity differs')
                block(github.get(f'git/ref/tags/{tag}', missing_ok=True) is not None
                      and github.get(f'commits/refs/tags/{tag}')['sha'] == sha, 'Published tag points at a different commit')
                block({asset['name'] for asset in existing['assets']} == release.asset_names(version),
                      'Published stable release has missing or unexpected assets')
                print(f'#{issue["number"]}: already published', file=sys.stderr)
                continue
            ready.append(eligibility(github, issue['number'], config))
        except Blocked as error:
            print(f'#{issue["number"]}: {error}', file=sys.stderr)
    return sorted(ready, key=lambda item: tuple(map(int, item['version'].split('.'))))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    discover = sub.add_parser('discover')
    discover.add_argument('--output', type=Path, required=True)
    for command in ('check', 'publish'):
        child = sub.add_parser(command)
        child.add_argument('--parent', type=int, required=True)
        child.add_argument('--version', required=True)
        child.add_argument('--sha', required=True)
        child.add_argument('--acceptance', required=True)
        if command == 'publish':
            child.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    repo = os.environ['GH_REPO']
    if args.command == 'discover':
        plans = candidates(GitHub(repo))
        block(len(plans) <= 256, 'Too many release parents for an Actions matrix')
        args.output.write_text(json.dumps({'include': plans}))
    else:
        def check():
            plan = eligibility(GitHub(repo), args.parent)
            block(plan['version'] == args.version and plan['sha'] == args.sha, 'Release declaration changed during the run')
            block(plan['acceptance'] == args.acceptance, 'Acceptance changed during the run')
            return plan
        plan = check()
        if args.command == 'publish':
            release.publish(args.output, args.version, args.sha, stable=True,
                            notes=f'Stable ARMv7 release. Release parent: https://github.com/{repo}/issues/{args.parent}\n'
                                  f'Physical Pi acceptance: {plan["acceptance"]}', before_publish=check)


if __name__ == '__main__':
    main()

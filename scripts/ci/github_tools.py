#!/usr/bin/env python3
"""Check RoundOS PR readiness and update issue project status."""

import argparse
import json
import re
import subprocess
import sys
import time

from workflow_github import GitHub, REPO, ToolError, gh, positive

PR_FIELDS = 'state,isDraft,body,headRefOid,mergeStateStatus,assignees,statusCheckRollup'
COPILOT = {'copilot-pull-request-reviewer', 'copilot-pull-request-reviewer[bot]'}
CI_JOBS = {'Tests', 'Host DRM build', 'ARMv7 build and package'}
THREAD_QUERY = '''query($owner:String!,$name:String!,$number:Int!,$cursor:String) {
  repository(owner:$owner,name:$name) { pullRequest(number:$number) {
    reviewThreads(first:100,after:$cursor) { nodes { isResolved comments(first:1) { nodes { url } } }
      pageInfo { hasNextPage endCursor } }
  } }
}'''


def pr_data(github, number):
    return json.loads(gh('pr', 'view', str(number), '--repo', github.repo, '--json', PR_FIELDS))


def description_prose(body):
    fence = None
    lines = []
    for line in re.sub(r'<!--.*?-->', '', body or '', flags=re.S).splitlines():
        if fence:
            if re.fullmatch(r' {0,3}' + re.escape(fence[0]) + '{' + str(len(fence)) + r',}[ \t]*', line):
                fence = None
            continue
        match = re.match(r'^ {0,3}(`{3,}(?!`)[^`]*$|~{3,})', line)
        if match:
            fence = re.match(r'`+|~+', match[1])[0]
        else:
            lines.append(line)
    return re.sub(r'(?<!`)(`+)(?!`)[\s\S]*?(?<!`)\1(?!`)', '', '\n'.join(lines))


def tracked_issues(github, pr):
    # The rules require a reference in the description, even if the issue
    # was also linked in the sidebar. Accept local mentions and issue URLs.
    repo = re.escape(github.repo)
    body = description_prose(pr['body'])
    pattern = rf'(?:https://github\.com/{repo}/issues/|(?<![\w/#-])(?:{repo})?#)([1-9][0-9]*)\b'
    numbers = {int(value) for value in re.findall(pattern, body, re.I)}
    issues = []
    for number in sorted(numbers):
        issue = github.api(f'issues/{number}', missing_ok=True)
        if issue and 'pull_request' not in issue:
            issues.append(issue)
    return issues


def linked_assignees(pr, issues):
    if not issues:
        return ['No repository issue referenced in the PR description']
    problems = []
    if not any(issue['state'] == 'open' for issue in issues):
        problems.append('No referenced issue is open')
    first = issues[0]
    expected = {assignee['login'].lower() for assignee in first['assignees']}
    actual = {assignee['login'].lower() for assignee in pr['assignees']}
    if not expected:
        problems.append(f'Issue #{first["number"]} has no assignee')
    elif expected != actual:
        problems.append(f'PR assignees must match #{first["number"]}: {", ".join(sorted(expected))}')
    return problems


def review_threads(github, number):
    args = {'number': number}
    for _ in range(30):
        threads = github.repo_graphql(THREAD_QUERY, **args)['pullRequest']['reviewThreads']
        yield from threads['nodes']
        if not threads['pageInfo']['hasNextPage']:
            return
        args['cursor'] = threads['pageInfo']['endCursor']
    raise ToolError('Too many review threads to verify completely')


def run_blockers(github, sha, checks):
    latest = {}
    for run in github.pages(f'actions/runs?head_sha={sha}', 'workflow_runs'):
        if run['head_sha'] != sha:
            continue
        key = (run['workflow_id'], run['event'])
        order = (run['run_number'], run['run_attempt'])
        if key not in latest or order > latest[key][0]:
            latest[key] = (order, run)
    runs = [item[1] for item in latest.values()]
    blockers = [f'{run["name"]}: {run["conclusion"] or run["status"]} ({run["html_url"]})'
                for run in runs if run['status'] != 'completed' or run['conclusion'] != 'success']
    if not any(run['path'].split('@')[0] == '.github/workflows/ci.yml' and run['event'] == 'pull_request' for run in runs):
        blockers.append('No RoundOS PR CI run for this commit')
    passed = set()
    for check in checks:
        if check['__typename'] == 'CheckRun':
            name = check['name']
            state = check.get('conclusion') if check['status'] == 'COMPLETED' else check['status']
            if state == 'SUCCESS':
                passed.add(name)
            elif state not in {'SKIPPED', 'NEUTRAL'}:
                blockers.append(f'{name}: {state or "pending"}')
        elif check['__typename'] == 'StatusContext' and check['state'] != 'SUCCESS':
            blockers.append(f'{check["context"]}: {check["state"]}')
    missing = CI_JOBS - passed
    if missing:
        blockers.append('Required checks not passing: ' + ', '.join(sorted(missing)))
    return blockers


def pr_report(github, number, *, feedback=False):
    pr = pr_data(github, number)
    sha = pr['headRefOid']
    headline = f'PR #{number} at {sha}'
    if pr['state'] != 'OPEN' or pr['isDraft']:
        return False, [headline, 'PR is a draft or is no longer open'], []
    blockers = linked_assignees(pr, tracked_issues(github, pr))
    if pr['mergeStateStatus'] != 'CLEAN':
        blockers.append(f'GitHub merge state: {pr["mergeStateStatus"]}')
    reviews = list(github.pages(f'pulls/{number}/reviews'))
    if not any((review.get('user') or {}).get('login') in COPILOT and review.get('commit_id') == sha
               and review.get('submitted_at') and review['state'] in {'APPROVED', 'COMMENTED', 'CHANGES_REQUESTED'}
               for review in reviews):
        blockers.append('Copilot has not submitted a review for this commit')
    threads = [node for node in review_threads(github, number) if not node['isResolved']]
    blockers.extend('Unresolved thread: ' + (thread['comments']['nodes'] or [{'url': '(link unavailable)'}])[0]['url']
                    for thread in threads)
    blockers.extend(run_blockers(github, sha, pr['statusCheckRollup']))
    summaries = [item for item in reviews if item.get('body', '').strip()]
    comments = list(github.pages(f'issues/{number}/comments'))
    details = []
    if feedback:
        for item in [*summaries, *comments]:
            details.append(f'{item["html_url"]}\n{item["body"]}')
    if pr_data(github, number)['headRefOid'] != sha:
        blockers.append('PR commit changed during the check; run again')
    lines = [headline, *dict.fromkeys(blockers)] if blockers else [headline, 'GitHub checks passed']
    if summaries or comments:
        lines.append(f'Read and address {len(summaries)} review summaries and {len(comments)} conversation comments before merging (use --feedback).')
    return not blockers, lines, details


PROJECT_QUERY = '''query($owner:String!,$name:String!) {
  repository(owner:$owner,name:$name) { projectsV2(first:100) { nodes {
    id title field(name:"Status") { ... on ProjectV2SingleSelectField { id options { id name } } }
  } pageInfo { hasNextPage } } }
}'''
ITEM_QUERY = '''query($owner:String!,$name:String!,$number:Int!) {
  repository(owner:$owner,name:$name) { issue(number:$number) { projectItems(first:100) {
    nodes { id project { id } fieldValueByName(name:"Status") { ... on ProjectV2ItemFieldSingleSelectValue { name } } }
    pageInfo { hasNextPage }
  } } }
}'''


def project_status(github, issues, target, *, project_name, apply):
    if not issues:
        raise ToolError('No issues to update')
    for issue in issues:
        if issue['state'] != 'open':
            raise ToolError(f'Issue #{issue["number"]} must be open')
    if apply:
        # Classic OAuth tokens expose scopes here. Fine-grained tokens may not
        # advertise them, so let the Projects API check those credentials.
        headers = gh('api', '--include', f'repos/{github.repo}').split('\n\n', 1)[0]
        scopes = re.search(r'^x-oauth-scopes:\s*(.*)$', headers, re.I | re.M)
        if scopes and scopes[1].strip() and 'project' not in {scope.strip() for scope in scopes[1].split(',')}:
            raise ToolError('GitHub CLI needs Projects write access (gh auth refresh --scopes project)')
    projects = github.repo_graphql(PROJECT_QUERY)['projectsV2']
    matches = [project for project in projects['nodes'] if project['title'] == project_name]
    if projects['pageInfo']['hasNextPage'] or len(matches) != 1 or not matches[0]['field']:
        raise ToolError(f'Cannot uniquely find project {project_name!r} with a Status field')
    project = matches[0]
    field = project['field']
    option = next((item['id'] for item in field['options'] if item['name'] == target), None)
    if not option:
        raise ToolError(f'Project {project_name!r} has no {target!r} status')
    planned = []
    for issue in issues:
        items = github.repo_graphql(ITEM_QUERY, number=issue['number'])['issue']['projectItems']
        if items['pageInfo']['hasNextPage']:
            raise ToolError(f'Issue #{issue["number"]} has too many project items to verify')
        item = next((value for value in items['nodes'] if value['project']['id'] == project['id']), None)
        planned.append((issue, item))
    for issue, item in planned:
        current = (item['fieldValueByName'] or {}).get('name') if item else None
        number = issue['number']
        if current == target:
            print(f'#{number}: already {target}')
            continue
        if not apply:
            print(f'#{number}: {current or "no status"} → {target} (preview)')
            continue
        if item:
            item_id = item['id']
        else:
            data = github.graphql('''mutation($project:ID!,$issue:ID!) {
              addProjectV2ItemById(input:{projectId:$project,contentId:$issue}) { item { id } }
            }''', project=project['id'], issue=issue['node_id'])
            item_id = data['addProjectV2ItemById']['item']['id']
        data = github.graphql('''mutation($project:ID!,$item:ID!,$field:ID!,$option:String!) {
          updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$field,
            value:{singleSelectOptionId:$option}}) { projectV2Item {
              fieldValueByName(name:"Status") { ... on ProjectV2ItemFieldSingleSelectValue { name } }
            } }
        }''', project=project['id'], item=item_id, field=field['id'], option=option)
        actual = data['updateProjectV2ItemFieldValue']['projectV2Item']['fieldValueByName']['name']
        if actual != target:
            raise ToolError(f'Issue #{number}: GitHub did not confirm {target}')
        print(f'#{number}: set to {target}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default=REPO)
    sub = parser.add_subparsers(dest='command', required=True)
    ready = sub.add_parser('pr-ready', help='Check CI, Copilot, issue links and open review threads')
    ready.add_argument('pr', type=positive)
    ready.add_argument('--watch', action='store_true')
    ready.add_argument('--timeout', type=positive, default=900, help='Watch timeout in seconds')
    ready.add_argument('--interval', type=positive, default=30, help='Seconds between checks')
    ready.add_argument('--feedback', action='store_true', help='Show review summaries and conversation comments')
    for name, target in [('issue-start', 'In Progress'), ('issue-review', 'PR Review')]:
        command = sub.add_parser(name, help=f'Preview or set project status {target}')
        command.add_argument('number', type=positive, metavar='ISSUE' if name == 'issue-start' else 'PR')
        command.add_argument('--project', default='RoundOS')
        command.add_argument('--apply', action='store_true')
        command.set_defaults(target=target)
    args = parser.parse_args()
    try:
        github = GitHub(args.repo)
        if args.command == 'pr-ready':
            deadline = time.monotonic() + args.timeout
            previous_lines = previous_details = None
            while True:
                passed, lines, details = pr_report(github, args.pr, feedback=args.feedback)
                if lines != previous_lines:
                    print('\n'.join(lines), flush=True)
                    previous_lines = lines
                if details and details != previous_details:
                    print('\n'.join(details), flush=True)
                previous_details = details
                if passed or not args.watch or time.monotonic() >= deadline:
                    return 0 if passed else 1
                time.sleep(min(args.interval, max(0, deadline - time.monotonic())))
        if args.command == 'issue-start':
            issue = github.api(f'issues/{args.number}')
            if 'pull_request' in issue:
                raise ToolError(f'#{args.number} is a pull request, expected an issue')
            issues = [issue]
        else:
            pr = pr_data(github, args.number)
            if pr['state'] != 'OPEN':
                raise ToolError(f'PR #{args.number} is not open')
            issues = tracked_issues(github, pr)
            blockers = linked_assignees(pr, issues)
            if blockers:
                raise ToolError('; '.join(blockers))
            issues = [issue for issue in issues if issue['state'] == 'open']
        project_status(github, issues, args.target, project_name=args.project, apply=args.apply)
        return 0
    except (ValueError, OSError, subprocess.TimeoutExpired) as error:
        print(error, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

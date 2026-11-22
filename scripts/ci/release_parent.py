#!/usr/bin/env python3
"""Plan a stable release parent or check it against the publication workflow."""

import argparse
import json
import re
import subprocess
import sys

import stable_release
from workflow_github import GitHub, REPO, ToolError, positive

START = '<!-- roundos-release-plan:start -->'
END = '<!-- roundos-release-plan:end -->'
ASSIGNEE = 'drazafsky'
PLAN = re.compile(rf'(?m)^{re.escape(START)}\n.*?^{re.escape(END)}$', re.S)
GUIDE = 'https://github.com/drazafsky/roundos/blob/main/docs/CI-RELEASES.md#automatic-stable-releases-from-parent-issues'


def issue(github, number):
    result = github.api(f'issues/{number}')
    if 'pull_request' in result:
        raise ToolError(f'#{number} is a pull request, expected an issue')
    return result


def automation_record(body):
    # Reserve the fence prefix even when malformed, so editing does not leave
    # the existing declaration and append a competing one.
    if re.search(r'(?m)^\s*```roundos-release\b', body):
        return stable_release.validate_declaration(stable_release.record(body, 'roundos-release'))
    return None


def replace_plan(body, version, acceptance, commit):
    has_markers = START in body or END in body
    if has_markers and (body.count(START) != 1 or body.count(END) != 1 or not PLAN.search(body)):
        raise ToolError('Issue has an incomplete or duplicate managed release plan; repair it first')
    existing = automation_record(body)
    if existing and (existing[0] != version or existing[2] != acceptance or (commit and existing[1] != commit)):
        raise ToolError('Existing release declaration differs; review or remove it before changing the plan')
    lines = [START, '## Release automation plan', '', f'Target stable version: `{version}`',
             f'Pi acceptance: #{acceptance}' if acceptance else 'Pi acceptance: create a native sub-issue',
             '', f'[Record Pi acceptance and opt in]({GUIDE}).']
    # A human-written declaration lives outside the managed section. Preserve
    # that block verbatim instead of copying it into the managed section.
    outside = PLAN.sub('', body) if has_markers else body
    if not automation_record(outside) and (commit or existing):
        sha = commit or existing[1]
        config = {'version': version, 'commit': sha, 'acceptance_issue': acceptance}
        lines.extend(['', '```roundos-release', json.dumps(config, indent=2), '```'])
    lines.append(END)
    section = '\n'.join(lines)
    if has_markers:
        return PLAN.sub(lambda _: section, body)
    spacing = '' if body.endswith('\n\n') else '\n' if body.endswith('\n') else '\n\n'
    return f'{body}{spacing}{section}\n' if body else section + '\n'


def ensure_descendant(github, parent, child):
    seen = {child}
    repository_url = f'https://api.github.com/repos/{github.repo}'.lower()
    for _ in range(stable_release.MAX_ISSUES):
        ancestor = github.api(f'issues/{child}/parent', missing_ok=True)
        if not ancestor or (ancestor.get('repository_url') or '').lower() != repository_url:
            break
        child = ancestor['number']
        if child == parent:
            return
        if child in seen:
            break
        seen.add(child)
    raise ToolError(f'Pi acceptance must be a native sub-issue below #{parent}')


def find_acceptance(github, publisher, parent):
    marker = f'<!-- roundos-release-acceptance:parent={parent} -->'
    matches = [entry for entry in github.pages('issues?state=all') if 'pull_request' not in entry
               and marker in (entry.get('body') or '').splitlines()
               and any(person['login'].lower() == ASSIGNEE for person in entry['assignees'])
               and publisher.trusted(entry['user']['login'])]
    if len(matches) > 1:
        raise ToolError(f'Multiple tracked acceptance issues for #{parent}; select one using --acceptance')
    return (matches[0] if matches else None), marker


def prepare(github, publisher, parent, version, *, acceptance, create_acceptance, commit, apply):
    # Validate typed input before any GitHub writes. No commit means planning
    # only: do not invent a commit or set a publishing label.
    stable_release.validate_declaration({'version': version, 'commit': commit or '0' * 40,
                                         'acceptance_issue': acceptance or 1})
    target = issue(github, parent)
    body, labels = target.get('body') or '', [label['name'] for label in target['labels']]
    if target['state'] != 'open' and target.get('state_reason') != 'completed':
        raise ToolError('Parent was abandoned')
    if not publisher.trusted(target['user']['login']):
        raise ToolError('Release parent must be authored by a maintainer')
    existing = automation_record(body)
    if existing and (version != existing[0] or (commit and commit != existing[1])):
        raise ToolError('Existing release declaration differs; review or remove it before changing the plan')
    if not commit and stable_release.LABEL in labels and not existing:
        raise ToolError('Parent has the release label but no declaration; opt in with an explicit commit or remove the label')
    if commit:
        stable_release.candidate_source(publisher, version, commit)
    if create_acceptance and existing:
        acceptance, create_acceptance = existing[2], False
    elif create_acceptance and (previous := PLAN.search(body)):
        if match := re.search(r'^Pi acceptance: #([1-9][0-9]*)$', previous[0], re.M):
            acceptance, create_acceptance = int(match[1]), False
    if acceptance:
        issue(github, acceptance)
        ensure_descendant(github, parent, acceptance)
    else:
        found, marker = find_acceptance(github, publisher, parent)
        if found:
            acceptance = found['number']
            current = github.api(f'issues/{acceptance}/parent', missing_ok=True)
            if current:
                ensure_descendant(github, parent, acceptance)
                create_acceptance = False
    proposed = replace_plan(body, version, acceptance, commit if acceptance else None)
    if commit and stable_release.LABEL not in labels:
        labels.append(stable_release.LABEL)
    print(f'#{parent}: target {version}, Pi acceptance ' + (f'#{acceptance}' if acceptance else 'new issue'))
    print(f'Candidate: {commit}; add or retain {stable_release.LABEL}' if commit else 'Version planning only')
    if not apply:
        if create_acceptance:
            print('Would ' + (f'attach existing issue #{acceptance}' if acceptance else 'create and attach a Pi acceptance issue'))
        print('Proposed issue section:\n' + PLAN.search(proposed)[0])
        return
    if create_acceptance:
        if not acceptance:
            result = github.api('issues', '--method', 'POST', '-f',
                                f'title=Validate Pi acceptance for RoundOS {version} (#{parent})', '-f',
                                f'body={marker}\n\nRecord physical Pi results for the release candidate in #{parent}. '
                                f'Include version, commit SHA, device and results. Post the matching maintainer '
                                f'attestation and close this issue as completed after acceptance passes. [Format]({GUIDE}).',
                                '-f', f'assignees[]={ASSIGNEE}')
            acceptance = result['number']
            print(f'Created acceptance issue #{acceptance}')
        result = issue(github, acceptance)
        github.api(f'issues/{parent}/sub_issues', '--method', 'POST', '-F', f'sub_issue_id={result["id"]}')
        ensure_descendant(github, parent, acceptance)
        print(f'Attached acceptance issue #{acceptance} below #{parent}')
        proposed = replace_plan(body, version, acceptance, commit)
    if proposed == body and labels == [label['name'] for label in target['labels']]:
        print('No changes needed')
        return
    latest = issue(github, parent)
    if ((latest.get('body') or '') != body or latest['labels'] != target['labels'] or latest['state'] != target['state']):
        raise ToolError('Parent changed during planning; run again to preserve the latest edits')
    result = github.api(f'issues/{parent}', '--method', 'PATCH', '-f', f'body={proposed}',
                        *(arg for label in labels for arg in ('-f', f'labels[]={label}')))
    if result['body'] != proposed or set(labels) != {label['name'] for label in result['labels']}:
        raise ToolError('GitHub did not confirm the proposed parent description and labels')
    print(f'Updated parent #{parent}')


def status(github, parent):
    config = stable_release.declaration(github, parent)
    if stable_release.already_published(github, *config[:2]):
        print(f'#{parent}: {config[0]} already published')
    else:
        plan = stable_release.eligibility(github, parent, config)
        print(f'#{parent}: {plan["version"]} eligible at {plan["sha"]}; Pi acceptance {plan["acceptance"]}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', default=REPO)
    sub = parser.add_subparsers(dest='command', required=True)
    check = sub.add_parser('status', help='Show publication eligibility or the first blocking condition')
    check.add_argument('parent', type=positive)
    plan = sub.add_parser('plan', help='Preview or apply a release plan to an existing parent')
    plan.add_argument('parent', type=positive)
    plan.add_argument('--version', required=True)
    child = plan.add_mutually_exclusive_group(required=True)
    child.add_argument('--acceptance', type=positive, metavar='ISSUE')
    child.add_argument('--create-acceptance', action='store_true', help='Create or reuse a Pi acceptance sub-issue')
    plan.add_argument('--commit', metavar='SHA', help='Opt in to publishing with this exact main commit')
    plan.add_argument('--apply', action='store_true', help='Write the previewed GitHub updates')
    args = parser.parse_args()
    try:
        publisher = stable_release.GitHub(args.repo)
        if args.command == 'status':
            status(publisher, args.parent)
        else:
            prepare(GitHub(args.repo), publisher, args.parent, args.version, acceptance=args.acceptance,
                    create_acceptance=args.create_acceptance, commit=args.commit, apply=args.apply)
        return 0
    except stable_release.Blocked as error:
        print(f'#{args.parent}: {error}', file=sys.stderr)
        return 1
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        detail = getattr(error, 'stderr', None) or str(error)
        print(detail.strip().splitlines()[0], file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

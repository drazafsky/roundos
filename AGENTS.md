# Repository rules

These rules apply to all work in this repository, including code, documentation,
configuration, and maintenance.

1. **Every task requires a GitHub issue.** Before making changes, identify the
   issue that tracks the work or create one. Always assign newly created issues
   to **@drazafsky**. Reference the issue in the PR description, using a closing
   keyword such as `Closes #123` when the PR completes the issue.

2. **Every PR must reference an open GitHub issue.** Reference at least one open
   issue in the PR description. Verify that at least one referenced issue is
   still open when creating the PR and immediately before merging it.

3. **Assign every PR to the assignee of its referenced issue.** If the PR
   references multiple issues, use the assignee of the lowest numbered
   referenced issue, including when the issues have different assignees. Check
   issue assignments when creating the PR and before merging, and update the
   PR assignee to match if necessary.

4. **Review code changes before creating a GitHub PR.** Use the `/review` skill
   on the complete code diff intended for the PR. Address its actionable findings
   and complete the review before creating the PR in GitHub. This review is in
   addition to the required GitHub Copilot review. At issue start and again
   against the final diff, use the applicable questions in the
   [pre-PR review playbook](docs/PRE-PR-REVIEW.md). Record the failure scenario,
   verification and locally caught defects in the initial PR description.
   Batch related fixes and check sibling paths before requesting another review.

5. **Every PR requires a completed GitHub Copilot review.** Request Copilot as a
   reviewer and wait for its submitted review before merging. A pending review
   request does not satisfy this requirement. Ensure the review covers the
   changes being merged; request another review when subsequent changes need it.

6. **Resolve every PR comment before merging.** Read and address comments from
   all reviewers, including Copilot, in review threads, review summaries, and
   the PR conversation. Make the necessary changes or explain the resolution.
   Explicitly resolve each review thread after its feedback has been addressed;
   marking a thread resolved without addressing its feedback is insufficient.
   No unresolved comments or review threads may remain at merge time.

7. **Finish the PR once all merge conditions are met.** Verify the open issue
   reference and matching PR assignee, that the Copilot review is complete,
   every comment and review thread is resolved, and all CI workflows and checks
   have passed for the latest PR commit. Once these conditions are met, squash
   merge the PR and delete its branch. Do not merge while an issue reference or
   PR assignment requirement is unmet, or while a review, comment resolution,
   or CI result is still pending or failing.

8. **Group implementation plans with more than two milestones.** Any
   implementation plan with three or more milestones must have a parent GitHub
   issue, with each milestone represented by its own native GitHub sub-issue
   attached to that parent. Create or identify the parent issue and all milestone
   sub-issues before beginning implementation.

9. **Move issues to In Progress when work begins.** When starting work on a
   GitHub issue, set its GitHub project status to **In Progress** before making
   repository changes.

10. **Move issues to PR Review when their PR is created.** After creating a
    pull request, set the GitHub project status of each issue tracked by the PR
    to **PR Review**.

11. **Only one In Progress sub-issue per parent.** In the selected project,
    at most one direct native GitHub sub-issue of a parent may have status
    **In Progress**. Run `issue-start` before editing and stop if it reports a
    sibling already in progress or cannot verify capacity. Finish that work
    or move it to **PR Review** before starting another sibling. Do not bypass
    the guard by changing project status directly. Different parents have
    independent slots; issues without a parent have no shared slot. A parent's
    own status does not occupy a child slot, and nested parents each have
    their own direct-child limit. See [issue work limits](docs/CI-RELEASES.md#issue-work-limits)
    for locking, permissions, and recovery.

12. **Learn from closed issues.** After merge, assess the PR's review closeout
    using the [playbook](docs/PRE-PR-REVIEW.md). On issue start, process a queued
    closeout if one is shown. Link confirmed misses to the earliest affected
    commit and a concrete prevention check. Incorporate reusable improvements
    through the normal issue and PR workflow.

Use the repository scripts below for routine GitHub work. Run them from the
repository root with Python 3 and an authenticated `gh` CLI.

- Before editing, run `python3 scripts/ci/github_tools.py issue-start ISSUE --apply`.
  After creating the PR, run `python3 scripts/ci/github_tools.py issue-review PR --apply`.
  Omit `--apply` to preview. Project writes require GitHub Projects permission.
- While waiting for review and CI, run
  `python3 scripts/ci/github_tools.py pr-ready PR --watch`. Use `--feedback` to
  read review summaries and PR conversation comments, and follow the thread
  links for inline feedback. Confirm feedback is addressed before merging.
- To plan a stable release, run
  `python3 scripts/ci/release_parent.py plan PARENT --version VERSION --acceptance ISSUE`.
  Use `--create-acceptance` if the parent has no Pi acceptance issue. Add
  `--apply` to save the plan. Add `--commit SHA` with `--apply` only when opting
  in the exact candidate. Check eligibility with
  `python3 scripts/ci/release_parent.py status PARENT`. See [CI and releases](docs/CI-RELEASES.md).

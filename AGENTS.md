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
   addition to the required GitHub Copilot review.

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

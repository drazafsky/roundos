# Local review and issue closeout

After the issue-start capacity check, use the questions below that apply to the
issue. Revisit the selection using the actual paths and behavior in the finished
diff. For a prose-only change, verify the affected instructions or links.
Trace the changed behavior through its callers, consumers and tests. For a
confirmed defect, inspect sibling paths for the same assumption. Use the
existing host suite (`sh scripts/ci/verify.sh`) and focused regressions where
they exercise the failure. Record physical Pi acceptance separately.

Complete the required full-diff `/review` before opening the PR. Include a
**Local review evidence** paragraph in the initial PR description with the
failure scenario, check or command and result, defects found locally (or none),
and any verification still pending. Preserve this paragraph when updating the
description. For example, “Appended a missing sample to an over-budget run.
The verdict changed to PENDING before the fix and remains FAIL in the mixed
sample regression.” After Copilot responds, read summaries, inline replies
and the conversation. Batch related fixes and rerun the affected checks before
requesting review on the new head.

## Review questions

### Stateful UI and protocol

Can a late result after cancellation, reload or ID reuse reach the wrong
consumer? Follow one request through publication, interruption and completion.
Exercise a failed publication or a full queue where relevant. [PR 30](https://github.com/drazafsky/roundos/pull/30)

Applies to: protocol, sdk, dialog, editor, reload, queue, reactive, request id, wire

### Persistence and recovery

What will the next invocation observe if execution stops directly after a write,
rename, service transition or lock release? Restart through the normal entry
point and verify files, processes and the visible recovery state. [PR 50](https://github.com/drazafsky/roundos/pull/50)

Applies to: persist, recovery, recover, install, rollback, systemd, storage, updater, release

### Tests and acceptance gates

Which broken implementation or bad input could still pass? Feed the gate a
known-bad case. For aggregated evidence, confirm that adding incomplete or
duplicate samples preserves an established failure. [PR 56](https://github.com/drazafsky/roundos/pull/56)

Applies to: acceptance, fixture, test, benchmark, measurement, evidence, gate, sample, scripts/m7

### Controls and input

Can a user reach and activate the visible control starting at the default
screen? Exercise the affected input methods, large targets and transitions.
Check the actual action and the rendered result. [PR 63](https://github.com/drazafsky/roundos/pull/63)

Applies to: touch, keyboard, carousel, focus, glyph, launcher, control, gesture, render, accessibility

## After closure

The **Collect review closeout** Action records each PR merged into main.
When an issue closes it also finds same-repository merged PRs that close the issue or
cross-reference it from their description. Each PR gets one bot comment indexing review outcomes
and one pending label (`review:closeout`). Each run archives the PR description,
review summaries and reviewed SHAs, inline comments and replies, thread
resolution states, and PR conversation in its evidence artifact. Multiple
closed issues can point to the same record. A closed issue with no linked merged
PR has no review to assess. For a missed event or failed run, dispatch on the default branch with
`gh workflow run review-closeout.yml -f pr=PR` for a merged PR, or
`gh workflow run review-closeout.yml -f issue=ISSUE` for a closed issue.

1. After merge, inspect the bot's closeout. If collection is still running,
   wait for it; a later issue-start also surfaces queued work. Retrieve all
   evidence with `python3 scripts/ci/review_feedback.py evidence PR`. Inspect
   summary-only findings even when every inline thread was resolved. Check
   the PR commits and resolution against the original local review note.
2. Post one maintainer comment beginning with the line `## Closeout assessment` on the merged PR. For each confirmed miss,
   link the finding, first review that reported it, earliest affected commit
   (or “unknown” with the reason), fix, and the cheapest check that would
   have exposed it. Identify whether the first reviewed head already contained
   it or a subsequent change introduced it. Record verified local catches,
   later-round misses, and pending hardware checks separately. A review-only
   caution or a failed reviewer session needs its own disposition. When there
   were no confirmed misses, say so in one sentence.
3. Link an existing regression when it already covers the defect. For an
   uncovered repeatable failure, track a regression; for a recurring judgment
   call, propose one concrete review question and relevant “Applies to” terms
   above. Group related proposals in an assigned issue, with links to the
   evidence, then use the normal PR process. After the assessment and follow-up
   links are recorded, remove the pending label with
   `gh pr edit PR --remove-label 'review:closeout'`. On retry, reuse the
   existing assessment and follow-up issue. Issue-start reads the same
   playbook, so a merged question is available to future Codex sessions.

## Evaluate the first ten comparable PRs

Count each merged PR once. The historical reference was 105 submissions across
28 PRs with review content (3.75 per PR), including incomplete sessions and
unmerged PRs. Match the cohort and outcome definitions for direct comparisons.
Report substantive, incomplete and quota outcomes separately and retain the
total for a like-for-like comparison. Two rounds per PR on average is a pilot
target. Use the assessments to count confirmed misses first reported after
round one, split by whether they existed before the first review or arose
during fixes. Track local catches and linked post-merge regressions. Compare
actual GitHub AI usage for the same periods using the account usage report;
mark unavailable or shared usage explicitly. Review counts alone do not measure
billed usage. Keep questions that catch defects and consolidate overlapping ones.

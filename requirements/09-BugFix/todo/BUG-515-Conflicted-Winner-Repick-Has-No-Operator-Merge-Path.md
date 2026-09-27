# BUG-515 — Re-picking a conflicted tournament winner re-applies the same stored patch; no operator-resolved-diff path

## Status
FIXED — 2026-09-26 (CA-1021).

## Found during
Deep review B-5 (`requirements/07-Coding-Plan/note/
CP-Deep-Review-12CP-2026-09-26.md`). Round-1 disposition wrongly treated
the claim as fully disproven because the merge card already labels the
conflicted winner (`picked winner — patch conflicts`). The second review
corrected it: the label exists, but the *behavior loop* does not — there
was no way to submit a manually resolved merge.

## Observed

On a merge-stage conflict, `tournamentMergeDecisionCard` re-offers every
snapshotted candidate including the conflicted winner. Picking it routes
through `resumeTournamentChoice` → `runTournamentMergeNode`, which passes
`rs.tournamentPatches[winner]` — the SAME stored patch — back into
`behaviorTournamentMerge`. `git apply --check` fails identically, the
card re-parks with "human merge required", and the loop can only end via
discard. Live tell: run-3688 (re-pick → no-progress → discard). The
decision's `feedback` channel was carried into `runTournamentMergeNode`
but never read.

## Fix (CA-1021)

`runTournamentMergeNode` now parses the decision feedback with
`extractOperatorPatch` — a `diff --git` header anywhere in the text
(operator prose before a pasted diff is tolerated), or a body starting
with a `--- `/`+++ ` unified header pair; a trailing markdown fence is
stripped, and a missing trailing newline is restored (git apply rejects
a hunk lacking it). A parsed diff OVERRIDES the recorded snapshot for
that apply; clean apply → winners sweep + `finishTournamentRun`, a
conflict re-parks the card honestly with the operator's patch in the
payload. Plain-prose feedback keeps the stored-patch path — prior
behavior unchanged. The merge card's candidate-option consequence now
documents the escape hatch.

## Tests

`bug515_merge_operator_diff_test.go`:
- `TestBug515_OperatorResolvedDiffOverridesStoredPatch` — winner's stored
  patch conflicts on drifted workspace → re-park; re-pick with a resolved
  `git diff` in feedback → operator content lands in the workspace, loop
  settles done.
- `TestBug515_ProseFeedbackKeepsStoredPatch` — prose-only feedback keeps
  the recorded snapshot path and merges cleanly.

## Residual

No dedicated "operator merged in workspace, settle done without applying
anything" option — the operator-diff path covers the documented need.
Provider-agnostic (merge machinery is provider-independent).

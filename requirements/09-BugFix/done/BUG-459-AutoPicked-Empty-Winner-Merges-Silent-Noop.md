# BUG-459 — Auto-picked empty winner patch merges as a silent no-op

- Status: **FIXED** (CA-956)
- Found: live `run-20041` (CP-65 full live test, post-BUG-458 build)
- Severity: high — a tournament flow reported `done` + "tournament winner
  merged" while applying literally nothing to the main workspace.

## Symptom

`run-20041` (tournament-harness, hub=devin, candidates grok+codex):

- `candidate-a` (grok-4.5) wrote a real green change: `Mul` + table-driven
  `TestMul` + `change-audit/CA-004.md`; suite green in its worktree.
- `candidate-b` (codex gpt-5.4-mini) completed in <1s having written nothing
  — its worktree stayed a clean copy of base.
- Arbiter auto-picked **`candidate-b`**: untouched baseline scores 1.0
  (tests pass, LSP clean, 0 dependents) while candidate-a paid the 20%
  blast-radius penalty for touching depended-on code.
- `merge_and_audit` returned `done` — "tournament winner candidate-b
  merged" — but `func Mul` never appeared in the main workspace and no diff
  was applied. Flow completed as a silent false success.

## Root cause

`runTournamentArbiterNode` (tournament_dispatch.go) stashed
`rs.tournamentPatches` **only in the escalate/ask branch**. The auto-pick
`"done"` branch stashed only `rs.tournamentWinner`.

Downstream, `runTournamentMergeNode` reads
`rs.tournamentPatches[winner]` — an absent key means `patchRecorded=false`,
which deliberately keeps the legacy `MergeWinner` live-worktree path
(BUG-453 design: absent → live worktree; present+empty → explicit
escalate). `worktree.Manager.Apply*` returns `nil` for an empty diff
("nothing to merge — still a successful (empty) win"), so the merge node
reported `done` on a no-op.

The escalate-on-empty contract added by BUG-453 was therefore unreachable
for auto-picked winners — reachable only through the human decision card.

## Fix (CA-956)

`tournament_dispatch.go`: hoist the `rs.tournamentPatches` stash above the
status switch so **every** arbiter verdict (done / retry / escalate)
records the per-candidate snapshots. An auto-picked winner whose recorded
patch is empty now hits `patchRecorded && empty → escalate` in
`behaviorTournamentMerge` — same explicit card the human-pick path already
had, reason `empty_patch`. The winner's worktree survives the escalate, so
the human can still pick a different candidate whose stored snapshot
applies.

## Regression coverage

- `TestBug459AutoPickedEmptyWinnerPatchEscalates`
  (bug446_453_tournament_test.go): service-level drive of
  `runTournamentArbiterNode` on a real git repo — candidate-a writes a real
  green change but pays a stubbed blast penalty, candidate-b is untouched.
  Asserts the done branch stashes `tournamentPatches` and the merge does
  NOT settle the flow `done` (escalates instead). RED before the fix.

## Open design observation (not changed here)

The deterministic scoring itself lets an unchanged worktree win: a clean
baseline scores 1.0 on tests/LSP/blast while any real diff risks a blast
penalty. With this fix that outcome now surfaces as an explicit
`empty_patch` escalate (human picks the real candidate's stored patch)
rather than a false merge. Whether empty-diff candidates should be
eligible to *win* outright is a CP-65 scoring-semantics question —
flagged for review, not unilaterally changed.

# CA-408: reused flow-node card count reads the step-transition log, not turn-log prompt counting

## Summary

Operator ran the same Review Loop flow shape on Codex, Claude, and Grok. Codex
and Grok restored correctly after a server restart; the Claude run (which had
genuinely looped to round 2 live — coder committed a fix, reviewer requested
changes, coder re-committed) restored with only round 1's cards. Root cause:
the Claude flow used exactly one reviewer (no cohort), and the restored
activation count for a reused ("reinvoke"-lifecycle) node came from a
turn-log prompt-count heuristic capped by a "peer start-time wave" signal
inferred from *other* children in the hub — with no other child, there is no
wave to infer a second round from, so the cap silently collapsed every reused
node to one activation regardless of how many genuinely happened. Full
analysis in
[BUG-314](../requirements/09-BugFix/done/BUG-314-Single-Reviewer-Reinvoke-Round2-Agent-Cards-Lost-On-Restart.md).

## Change

- `resumedParentAgentAnnotations` (interactive_resume.go) now derives a reused
  node's restored activation count and per-activation timing from the durable
  step-transition sidecar (Task-239, `<runID>-step-transitions.ndjson`) when
  exactly one child run claims that node's label (`labelCounts`) — each
  `RUNNING`→terminal pair in the log is a real activation, ground truth that
  isn't inflated by a mid-round gate reprompt landing as an extra turn-log
  prompt line.
- New helpers `stepNodeActivationsFromLog` / `stepNodeActivationTimes` /
  `stepNodeActivation`.
- Falls through unchanged to the pre-existing `resumeChildActivationCount` +
  `peerStartWaveTimes`-capped `resumeActivationTimestamps` path whenever the
  sidecar is absent/empty for that label, OR the label is legitimately shared
  by several distinct child runs (a spawn-lifecycle node getting a fresh run
  id each round, e.g. Codex's `reviewer_correctness`/`reviewer_security` —
  already correct, must not be triple-counted from the shared-label log).

Provider parity: no `providerKey` branch anywhere in the change; verified by
an identical cross-provider round-trip test for Codex, Claude, and Grok, plus
a live re-check of all three real hubs from the operator's own test session.

## additive-tests-only compliance

New test file (`bug314_reinvoke_activation_from_step_log_test.go`, 3 test
functions + 1 small helper) only. No existing test touched.

## Verification

- Red-first TDD: the repro test and its 3 cross-provider subtests fail on
  pre-fix HEAD with the exact defect (`coder round-2 card lost on restart:
  spawns = 1, want 2`), proven via `git stash` isolating only the
  `interactive_resume.go` fix (new test file kept in place), then pass after
  popping the stash back.
- A 4th test locks in the `labelCounts` guard: a Codex-shaped hub with 2
  reviewer labels reused across 3 rounds (fresh run id each round, so the
  step-transition log has 3 `RUNNING`/`DONE` pairs per shared label) must
  still restore exactly 1 card per distinct child run — passes both before
  and after the fix, since it guards this fix's own new code path rather than
  reproducing a pre-existing bug.
- `go build ./...`, `go vet ./internal/runner/...`: clean. Targeted sweep
  (`TestBug314*` + every existing resume-order test: `TestRun5695*`,
  `TestRun9034*`, `TestRun1264*`, `TestRun20332*`, `TestRun24377*`,
  `TestBug294*`, `TestRun12613*`): 37/37 pass, all pre-existing tests
  unchanged. Full-package sweep on the fix: 2486 passed / 16 failed (the
  catalogued pre-existing machine/CLI-dependent set). Same sweep on a
  `git stash`-isolated baseline: 2479 passed / **17** failed — the same 16
  plus one extra pre-existing flake (`TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`,
  a Windows `TempDir` cleanup race in an unrelated file) — confirming zero
  new failures from this change.
- Live end-to-end on the running dev runner (project Gate-sandbox), rebuilt
  and restarted: resumed the operator's own 3 real hubs. Claude `run-53157`
  now shows both round-2 cards (`my-coder`/`run-53162` and
  `my-reviewer-claude`/`run-53261`, each spawned+resulted twice, second coder
  result carrying the real "Committed as `2c70dfa`…" text). Codex `run-46797`
  (2 reviewers × 3 rounds) and Grok `run-45881` (single round) unaffected —
  identical card counts before and after.

## Known limits (documented, out of scope)

- The Drive-sync manifest does not carry the step-transition sidecar, so a
  chat restored on a different machine still uses the pre-existing
  turn-log-heuristic path for activation timing (same as before this fix) —
  mirroring the turn-log gap BUG-313 closed would be a natural follow-up, not
  required here since the fallback is already correct, just less precise.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-314
change_type: bugfix
summary: resumedParentAgentAnnotations reads the durable step-transition log for a reinvoke-lifecycle node's restored activation count when it uniquely owns its label, fixing lost round-2+ agent cards for flows with no peer cohort to infer rounds from.
# --->8---

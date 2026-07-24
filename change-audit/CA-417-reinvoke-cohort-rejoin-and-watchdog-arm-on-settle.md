# CA-417: reinvoke-lifecycle cohort member re-joins each round's cohort, and the hub-stall watchdog arms on every flow child settle

## Summary

A Claude Review-Loop chat with a single reinvoke-lifecycle reviewer
(`my-reviewer-claude`) hung after round 2: both coder and reviewer completed but
the hub never synthesized and no `hub_stalled` card appeared (live repro
`run-54862`, `run-54473`). Two coupled defects:

- P1: `tryAdvanceFlowFromNode`'s reuse-child branch reinvoked the reviewer without
  refreshing its `flowCohortId`. Round 0 spawned it into cohort
  `flow-auto-coder-round-0` (expected=1); the join drained that key (`drainCohort`
  deletes the buffer AND `cohortExpected`). Round 1 reinvoked the same child, still
  tagged the drained `...round-0` id; its completion appended to a dead cohort, so
  `cohortComplete` returned false (`cohortExpected==0`) and
  `maybeAutoReinvokeHubWithNote` never fired. Because the completion still entered
  the `if rs.flowCohortId != ""` branch, the `else if advanceOrNotifyHub` fallback
  was skipped too — nothing scheduled the hub.
- P2: every `maybeScheduleHubStallCheck` call site is coupled to a hub
  reinvoke/notify actually being scheduled, so when P1 dropped the reinvoke the
  F-0 hub-stall watchdog was never armed for that window — the safety net shared
  the exact blind spot it exists to catch.

Full analysis in
[BUG-318](../requirements/09-BugFix/done/BUG-318-Reinvoke-Lifecycle-Cohort-Reviewer-Loses-Membership-Round2-Hub-Hangs.md).

## Change

- `reinvokeExistingFlowChild(parentRunID, nodeID, prompt, cohortID string, cohortSize int)`:
  when `cohortID != ""`, `preRegisterCohort`s this round's cohort and re-tags the
  matched child's `flowCohortId` (set inside `reinvokeMatchingFlowChild`'s match
  predicate, under its lock), mirroring the spawn path's `FlowCohortID` +
  `preRegisterCohort`. `tryAdvanceFlowFromNode` passes the round's `cohortID` +
  `len(targetNodes)`.
- The other caller (`flow_validate_audit_dispatch.go`, validate/audit retry)
  passes `cohortID=""` — that path's spawn fallback carries no cohort, so behavior
  is unchanged (BUG-279 reuse intact).
- `settleFlowChildTurnCompletedLocked` arms
  `go s.maybeScheduleHubStallCheck(rs.parentRunID)` on every flow child settle,
  independent of whether the completion schedules a reinvoke.

Design notes: `drainCohort` is intentionally left deleting `cohortExpected` (its
create-if-absent re-use contract); the fix re-registers a FRESH per-round cohort
instead. `flowCohortId` is NOT cleared to route through `advanceOrNotifyHub` —
that would bypass the joined-result note the synthesizer requires
(BUG-synthesis-hang). The watchdog arm is event-driven (on settle), not a periodic
sweep, and `checkAndBlockStalledHub` already re-arms while a child is active or the
hub is busy, so it never false-trips a live flow.

## Provider parity

Provider-agnostic: the reuse/cohort/watchdog machinery is shared across
Claude/Codex/Grok. Only the reinvoke-lifecycle-reviewer SHAPE (custom `flow-claude`)
triggers it — the built-in `review-loop.yaml` uses `lifecycle: spawn` reviewers
(fresh child + fresh cohort each round) and is unaffected. The regression test runs
all three providers.

## additive-tests-only compliance

New test file (`bug318_reinvoke_cohort_rejoin_test.go`, 4 test functions) only. No
pre-existing test edited. Two production callers of `reinvokeExistingFlowChild`
updated for the new signature; no test called it directly (verified by grep).

R3 matrix coverage: reported repro (single reinvoke reviewer, round 0->1,
cross-provider); multi-round (rounds 0->1->2 each re-tag the fresh cohort);
multi-member reuse cohort (two reinvoke reviewers both re-join the shared round
cohort, exercising `preRegisterCohort` with size 2 and an expected==2 barrier);
and the P2 watchdog-on-settle guard. Spawn-lifecycle reviewers stay covered by the
pre-existing `TestTryAdvanceFlowFromNodeLifecycleSpawnCreatesFreshTarget` /
`TestCoderCompletionAutoSpawnsReviewerCohort` (untouched path).

## Verification

- Red-first (P1), cross-provider Claude/Codex/Grok:
  `TestReinvokeLifecycleCohortReviewerRejoinsFreshCohortEachRound` fails pre-fix
  (`flowCohortId = "flow-auto-coder-round-0"`, want `...round-1`) then passes.
- Red-first (P2): `TestFlowChildSettleArmsHubStallWatchdogWithoutReinvoke` times
  out (hub never blocks) with the P2 change stashed, and blocks with `hub_stalled`
  after a short stall timeout once restored.
- Targeted sweep (TryAdvance/Cohort/Reinvoke/Reuse/Stall/Settle/ReviewLoop/
  FlowControl/AutoAdvance/CoderCompletion/Continue/BUG-234/307/314/318): 258
  passed. `go build ./...`, `go vet ./internal/runner/...`: clean.
- Full-package sweep vs `git stash` baseline at the same HEAD: the failures that
  differ are entirely pre-existing order-dependent flakes (fix-only extras
  `TestFinalizerHookSurfacesArtifacts` + `TestGeminiAdapterPromptPrepAndEnv` both
  pass in isolation; baseline-only extras are the known Windows-teardown /
  concurrency flakes). No changed-area test regressed.
- Real `provider-accounts.json` verified unchanged after the sweeps.

## Not fixed by recent commits

Commits 7bfc6ed (card order after restart), 246a689 (Continue/Stop after blocked
restart), 8f1ec62 (Codex transcript isolation after restart) all postdate the hung
binary and touch restart/resume/Codex-isolation only. The live cohort-join /
reinvoke / hub-stall code is byte-identical to where it hung — none of them address
this bug.

## Known limits (documented, out of scope)

- None remaining. Live end-to-end re-verification is done: user rebuilt/restarted
  via `just dev` and ran a live `flow-claude` review-loop chat to completion (hub
  synthesized, no hang).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-318
change_type: bugfix
summary: a reinvoke-lifecycle cohort reviewer re-joins each round's cohort (fresh FlowCohortID + preRegisterCohort) when reinvoked instead of keeping its drained round-0 cohort id, so the hub synthesizes every round instead of hanging after round 2; and the hub-stall watchdog is now armed on every flow child settle (not only when a reinvoke is scheduled), so any future "nothing scheduled" hub hang still surfaces an actionable hub_stalled card.
# --->8---

# BUG-318: Reinvoke-Lifecycle Cohort Reviewer Loses Membership On Round 2+ — Hub Never Synthesizes, Flow Hangs Silently

## Metadata

- Document ID: `BUG-318`
- Title: `A review-loop whose reviewer node is reinvoke-lifecycle (one reused child across rounds) hangs after round 2's reviewer completes: the reused child keeps its drained round-0 cohort id, cohortComplete stays false, the hub synthesis reinvoke never fires, and the hub-stall watchdog — armed only when a reinvoke is scheduled — never trips`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-24`
- Last Updated: `2026-07-24`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Child Documents: `none`
- Related Documents: [BUG-314: Single-Reviewer Reinvoke Round2 Agent Cards Lost On Restart](../done/BUG-314-Single-Reviewer-Reinvoke-Round2-Agent-Cards-Lost-On-Restart.md) (same reinvoke-lifecycle-single-reviewer surface — BUG-314 fixed the restored-card COUNT; this fixes the LIVE round-2 synthesis-reinvoke hang), [CA-417](../../change-audit/CA-417-reinvoke-cohort-rejoin-and-watchdog-arm-on-settle.md)
- Replaces: `none`
- Tags: `agent-flow-engine, cohort, reinvoke-lifecycle, review-loop, hub-stall, watchdog, cross-provider, severity-high`

## AI Quick View

### Summary

A Claude Review-Loop chat (custom `flow-claude`: one `my-coder` + one
reinvoke-lifecycle `my-reviewer-claude`) hangs after round 2: coder and reviewer
both complete, but the hub never runs its synthesis turn and no watchdog trips.
Round 0 works (reviewer spawned into cohort `flow-auto-coder-round-0`, join drains
it, hub synthesizes). Round 1's reviewer is REINVOKED (same child reused) via
`tryAdvanceFlowFromNode`'s reuse-child branch, which never refreshed the child's
`flowCohortId`; the reused reviewer keeps the drained round-0 cohort id, its
completion appends to a dead cohort (`cohortExpected==0`), `cohortComplete` stays
false, and `maybeAutoReinvokeHubWithNote` is never scheduled. The hub-stall
watchdog is armed only as a side effect of scheduling a reinvoke, so when the
reinvoke is dropped the safety net is never armed either.

### Current Ask

(P1) Re-register the round's cohort and re-tag the reused child when reinvoking a
reinvoke-lifecycle cohort member, so every round joins a live barrier. (P2) Arm
the hub-stall watchdog on every flow child settle, independent of whether a
reinvoke was scheduled.

### Key Decisions

- Fix the cohort membership in the reuse branch (mirror the spawn path's
  `FlowCohortID` + `preRegisterCohort`), NOT by making `drainCohort` retain the
  expected count (which would break the create-if-absent re-use contract) and NOT
  by clearing `flowCohortId` to route through `advanceOrNotifyHub` (which would
  bypass the joined-result note the synthesizer needs — BUG-synthesis-hang).
- Watchdog hardening arms on child settle rather than a periodic sweep: minimal,
  event-driven, and `checkAndBlockStalledHub` already re-arms while a child is
  active / the hub is busy, so it never false-trips a live flow.
- Provider-agnostic by construction: the reuse/cohort/watchdog code is shared;
  only the reinvoke-lifecycle-reviewer SHAPE (not the provider) triggers it.

### Constraints

- Additive tests only; no pre-existing test edited.
- No real machine paths in tests.

### Open Questions

- `none`.

### Source Refs

- `apps/local-runner/internal/runner/flow_executor.go` —
  `tryAdvanceFlowFromNode` reuse branch, `reinvokeExistingFlowChild`.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` — the other
  `reinvokeExistingFlowChild` caller (validate/audit retry — no cohort).
- `apps/local-runner/internal/runner/interactive_service.go` —
  `settleFlowChildTurnCompletedLocked` (P2 watchdog arm).
- `apps/local-runner/internal/runner/agent_orchestrator.go` — `preRegisterCohort`,
  `drainCohort`, `cohortComplete`.
- `apps/local-runner/internal/runner/hub_stall.go` — `maybeScheduleHubStallCheck`,
  `checkAndBlockStalledHub`.

## 1. Issue Summary

Live repro `run-54862` (also `run-54473`): a Claude flow-claude chat. Round 1
synthesized fine; after round 2's coder (`run-54867`) and reviewer (`run-54927`)
both completed (07:02:48 / 07:03:03), the hub went silent — no synthesis, no
`hub_stalled` card — for minutes. Reproduces for any provider whose flow uses a
reinvoke-lifecycle reviewer in a cohort; it is not Claude-specific.

## 2. Parent Links

CP-51 (durable turn dispatch / flow-engine). Same reinvoke-lifecycle-single-
reviewer surface as BUG-314 (which fixed the restored-card count); this fixes the
live round-2 synthesis-reinvoke hang and hardens the F-0 hub-stall watchdog
(BUG-289).

## 3. Environment and Reproduction

Runner started with `just dev`; target project `gate-sandbox`. Dispatch V2 on,
Claude V2-enabled. Custom flow `flow-claude` with a single `my-reviewer-claude`
node whose lifecycle is `reinvoke` (reuses one child run across rounds).

1. Run a review-loop that reaches "changes requested" so it loops past round 1.
2. Round 1: coder re-runs, reviewer re-runs (reinvoked, same child), both complete.
3. The hub never synthesizes round 1's result; the flow hangs; the 2-minute
   hub-stall watchdog never trips either.

Confirmed on the latest binary (booted 06:59 2026-07-24, past commits 7bfc6ed /
246a689 / 8f1ec62). Those commits fix restart/resume/Codex-isolation only; the
live cohort-join / reinvoke / watchdog code is byte-identical to where it hung, so
none of them address this.

## 4. Expected vs Actual

- Expected: after round N's reviewer completes, the hub synthesizes round N and
  either loops again or finishes; if anything stalls, the watchdog blocks with an
  actionable card within the timeout.
- Actual: round 2+ reviewer completion joins a dead cohort, no synthesis fires,
  and no watchdog arms — the hub hangs silently and indefinitely.

## 5. Root Cause

Two coupled defects:

- P1 (primary): `tryAdvanceFlowFromNode` builds a fresh per-round cohort id
  (`flow-auto-<src>-round-<N>`) and, for SPAWN-lifecycle targets, spawns them into
  it with `FlowCohortID` + `preRegisterCohort`. For REUSE-lifecycle targets it
  called `reinvokeExistingFlowChild` -> `reinvokeMatchingFlowChild`, which
  reactivates the child but never touches `flowCohortId`. Round 0 spawned the
  reviewer into `...round-0` (expected=1); its join drained that key
  (`drainCohort` deletes both the buffer and `cohortExpected`). Round 1 reinvoked
  the same child, still tagged `...round-0`. On completion,
  `settleFlowChildTurnCompletedLocked` took the `if rs.flowCohortId != ""` branch,
  appended to the drained `...round-0` cohort, and `cohortComplete` returned false
  (`cohortExpected==0`). The `if cohortComplete { drain + maybeAutoReinvokeHubWithNote }`
  block never ran, and because the outer `if` was taken the `else if
  advanceOrNotifyHub` fallback was skipped too — nothing scheduled the hub.
- P2 (safety net): every `maybeScheduleHubStallCheck` call site is coupled to a
  hub reinvoke/notify actually being scheduled (inside
  `maybeAutoReinvokeHubWithNote`/`WithPrompt`, or a reinvoke-fail path). So when P1
  dropped the reinvoke, the F-0 watchdog was never armed for that window — the
  safety net shared the exact blind spot it exists to catch.

The coder never hangs because it is not a cohort member (entry / back-edge
continue), so its completion always routes through `advanceOrNotifyHub`. The
built-in `review-loop.yaml` never hangs because its reviewers are
`lifecycle: spawn` (fresh child + fresh cohort each round); only a reinvoke-
lifecycle reviewer in a cohort trips it.

## 6. Fix Strategy

- P1: `reinvokeExistingFlowChild(parentRunID, nodeID, prompt, cohortID, cohortSize)`
  now, when `cohortID != ""`, `preRegisterCohort`s the fresh round's cohort and
  re-tags the matched child's `flowCohortId` (under the reinvoke lock), mirroring
  the spawn path. `tryAdvanceFlowFromNode` passes this round's `cohortID` +
  `len(targetNodes)`. The validate/audit retry caller passes `cohortID=""` (that
  path's spawn fallback carries no cohort — unchanged BUG-279 behavior).
- P2: `settleFlowChildTurnCompletedLocked` arms
  `go s.maybeScheduleHubStallCheck(rs.parentRunID)` on every flow child settle, so
  any "children done, hub idle, nothing scheduled" state is caught within the
  stall timeout regardless of cause.

## 7. Validation

- Red-first, cross-provider (Claude/Codex/Grok):
  `TestReinvokeLifecycleCohortReviewerRejoinsFreshCohortEachRound` fails pre-fix —
  the reinvoked reviewer's `flowCohortId` stays `flow-auto-coder-round-0` instead
  of `...round-1`, and the round-1 cohort never registers — then passes after P1.
- Multi-round + multi-member coverage (R3):
  `TestReinvokeLifecycleCohortReviewerRejoinsEachRoundThroughRound2` (rounds
  0->1->2 each re-tag the fresh cohort) and
  `TestReinvokeLifecycleMultiMemberReuseCohortBothMembersRejoin` (two reinvoke
  reviewers both re-join the shared round cohort; barrier expected==2, so one
  member completing does NOT complete it).
- Red-first for P2: `TestFlowChildSettleArmsHubStallWatchdogWithoutReinvoke`
  settles a lone member into an incomplete cohort (schedules no reinvoke) and
  asserts the hub blocks with `hub_stalled` after a short stall timeout. With the
  P2 change stashed (`git stash` interactive_service.go) it times out (never
  blocks); with it, it blocks. 
- Targeted sweep (every TryAdvance / Cohort / Reinvoke / Reuse / Stall / Settle /
  ReviewLoop / FlowControl / AutoAdvance / CoderCompletion / Continue / BUG-234 /
  307 / 314 / 318 test): 258 passed.
- `go build ./...`, `go vet ./internal/runner/...`: clean.
- Full-package sweep (`go test ./internal/runner/`): fix = 2307 passed / 19
  failed; `git stash` baseline (fix removed) at the same HEAD failed a symmetric
  set. The failures that DIFFER between the two runs are entirely pre-existing
  order-dependent flakes — fix-only extras `TestFinalizerHookSurfacesArtifacts`
  and `TestGeminiAdapterPromptPrepAndEnv` both pass in isolation on the fixed
  tree; baseline-only extras `TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`
  and `TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows` are the
  known Windows-teardown / concurrency flakes. No changed-area test regressed.
- Real `provider-accounts.json` verified unchanged (8 accounts, correct grok
  flags) after the sweeps.
- Live end-to-end: PENDING — the fix loads at runner build/boot (flow-engine
  reinvoke path); the running binary predates it. To verify: rebuild + restart
  (`just dev`), run a flow-claude review-loop to "changes requested" so it loops
  past round 1, and confirm the hub synthesizes round 2 (no hang).

## 8. Regression Guard

- `TestReinvokeLifecycleCohortReviewerRejoinsFreshCohortEachRound` locks the
  per-round cohort re-join for all three providers.
- `TestReinvokeLifecycleCohortReviewerRejoinsEachRoundThroughRound2` and
  `TestReinvokeLifecycleMultiMemberReuseCohortBothMembersRejoin` lock the
  multi-round and multi-member (size-2 barrier) shapes.
- `TestFlowChildSettleArmsHubStallWatchdogWithoutReinvoke` locks the watchdog
  arming on settle (defense-in-depth for any future "nothing scheduled" hang).
- Existing spawn-lifecycle cohort tests (`TestTryAdvanceFlowFromNodeLifecycleSpawnCreatesFreshTarget`,
  `TestCoderCompletionAutoSpawnsReviewerCohort`) stay green — the spawn path is
  untouched.

## 9. Follow-Up Document Updates

- CA-417 records the change.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-318
change_type: bugfix
summary: a reinvoke-lifecycle cohort reviewer now re-joins each round's cohort (fresh FlowCohortID + preRegisterCohort) when reinvoked instead of keeping its drained round-0 cohort id, so the hub synthesizes every round instead of hanging; and the hub-stall watchdog is armed on every flow child settle (not only when a reinvoke is scheduled) so any future "nothing scheduled" hang still surfaces an actionable card.
# --->8---

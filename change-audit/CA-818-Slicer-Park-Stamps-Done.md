# CA-818 — slicer park stamps step DONE before parking

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-364
change_type: bugfix
summary: Slicer fail-closed park stamps the completed node DONE first so the step cannot strand RUNNING behind its own park
# --->8---

## Why

Live run-640953: the `task_slicer` child completed (zero Task files) and
the CA-817 park fired correctly — but the flow step stayed RUNNING 7+ min
with no card and no watchdog. `tryAdvanceFlowFromNode` calls
`onVibeCpNodeDone` (`flow_executor.go:1160`) BEFORE its `loopIsAdvancing`
gate (`:1166`) and DONE writes (`:1203-1206`): the park tripped its own
downstream gate (self-poisoning), so the `:1203` terminal write never ran.
Settle dispatch was correct (`flow_advance_skipped_loop_blocked` is only
reachable inside `tryAdvance`); hub liveness was not required (earlier
advances succeeded after main completed).

## Change

- BUG-363 park branch (`vibe_cp.go` slicer case): `setFlowStepStatus(ctx,
  parentRunID, completedNodeID, StepStatusDone)` BEFORE
  `parkVibeRequirement` — mirrors the cohort self-settle
  (`interactive_service.go:5009-5013`) and the `:1203` terminal write it
  replaces on this path. Unlocked variant (no `s.mu` held, CA-811).
- Explicitly NOT changed: `tryAdvance` head order (`:1160` stays — hoisting
  DONE there would touch every node type), the `:1166` gate (BUG-234
  protection), CA-817 park decision/reason text.

## Tests

- `bug364_slicer_park_stamps_done_test.go` (new, additive-only):
  `TestBUG364_SettleSlicerCompletionStampsDoneAndParks` drives a linear (non-cohort)
  slicer child through the real `settleFlowChildTurnCompletedLocked` →
  advance ordering on the overlaid vibe graph; asserts step DONE + loop
  `blocked/requirement` with the Task reason. Red-before (FAIL on CA-817
  code, 3s poll timeout) / green-after. BUG-363 direct-call tests could not
  see this ordering by construction.
- Contract subset green: BUG-363/364, CA-78x/79x/80x/81x, `TestOnVibe*`,
  `TestCollect*`, `TestAdvanceHubDone*`, Task-321/326, CA-793,
  `TestVibeSession_*` — 103 PASS, 0 FAIL.

## Providers

Agnostic Case 1: step-status write + disk glob only; zero `providerKey`
in `vibe_cp.go`. Test uses `ProviderKeyCodex` via `newTestServer`, same
precedent as CA-796/797/798 and BUG-363.

## Will not undo

CA-817 park decision + reason (extended, not replaced). BUG-234 `:1166`
gate. Cohort self-settle. CA-783 SS fallback. CA-786/791 overlay/join.
Collector semantics (`collectVibeTaskPlan`/`collectVibeSprintPlan`
untouched). Residuals carry over from CA-817 (stale-CP presence,
`inprogress/`-Task parks by design); TUI card surfacing for requirement
parks remains a separate follow-up.

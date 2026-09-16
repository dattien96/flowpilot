# CA-873 — BUG-288 V10: flow-engine root defers Completed until gate

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: emitLocked TurnCompleted defers Completed for flow-engine-driven roots even without code events, closing the early-release race the children branch already closes
# --->8---

## Why

`TestRootFlowEngineDefersCompletedUntilGate` (`v10_residual_test.go:16`, V10
residual P0) was red: a flow-engine-driven ROOT published `Completed`
immediately on a codeless turn. Root cause in `emitLocked`'s TurnCompleted
branch (`interactive_service.go:5432-5490`): the flow-engine deferral checked
only `parent.flowEngineDriven` (children, line 5449), while roots fell into
the run-208282 branch that defers solely on FileChanged code events. A
codeless flow-engine root therefore completed before its post-turn gate ran,
releasing flow dependents early — the same race F-15 closed for children.
`resumePendingFlowGate` explicitly supports roots (V10 P0, line 4667-4670),
so the armed state is resumable; only the arming condition was missing.

## Change

- **`internal/runner/interactive_service.go`** (1-line production change):
  root-branch seed `hasCode := false` → `hasCode := rs.flowEngineDriven`,
  so a flow-engine-driven root arms `markPendingFlowGateSettleLocked` exactly
  like a code-touching root. Plain chat roots unchanged (still complete
  immediately without code — run-208282 contract); the BUG-305
  `turnStartedAfterLoopDone` else-branch is untouched and keeps precedence.
- **`internal/runner/v10_flow_engine_root_deferral_test.go`** (new,
  test-only): R3 matrix — flow root with code defers, plain codeless root
  completes, plain root with code defers, flow root after loop-done completes
  (BUG-305 precedence pin). No pre-existing test modified.

## Tests

- Previously red `TestRootFlowEngineDefersCompletedUntilGate` now green;
  4 new matrix tests green; neighboring V10 tests
  (`TestChildGateSnapshotPersistsTurnBase`,
  `TestResumePendingFlowGateMaterializesTurnCompleted`,
  `TestRootPendingGateNotCancelledByNormalize`,
  `TestSessionStateOfGateAndStepFields`) green.
- Regression sweep: full `runner` package run triaged failure-by-failure —
  all 29 failures reproduce identically on clean HEAD `c0f21c7` (proven via
  isolated worktree runs) or pass in isolation with this change
  (`TestBug360FreezeProceedsFromStashWithoutScoutChild`,
  `TestRun201295InTurnSubmitFlowControlDispatchesFreeze`,
  `TestRun207435NoReparkAfterFreezeDone` — full-run flakes under load).
  Zero regressions attributable to this change.
- Incidental pre-existing findings for owners (untouched, R1 STOP):
  `TestBUG327_EmitLockedDoesNotUnparkWaitingChild` self-deadlocks
  (holds `s.mu` at `:562` while `agentGraphSnapshot` locks it at
  `interactive_service.go:985` — deterministic, also hangs on clean HEAD);
  `TestRun144900_*` blocked on the run151954/run144900 spec conflict;
  assorted machine-specific failures (Supabase demo env, provider count,
  Windows path separators, `.sh` fixtures).

## Providers

Provider-agnostic (Case-1). Evidence: the TurnCompleted root decision
branches only on `parentRunID`, `flowEngineDriven`, `turnStartedAfterLoopDone`
and the run's own event list — `ProviderKey` appears in `emitLocked` solely
stamped onto events/telemetry, never branched on. One representative
provider (`ProviderKeyCodex`, matching the V10 suite convention) suffices.

## Prior CA claims kept intact

- BUG-288 F-15/V10 P0-P1 (defer + resume incl. roots), run-208282 (codeless
  chat completes immediately), BUG-305 (post-loop-done follow-up completes),
  R17-P1 (checkpoint durability), CA-645/run151954 (scaffold exclusion),
  CP-64 `r-reproduce` (untouched files; CP-64 E2E suite unaffected — separate
  gate path, re-verified green during CP-64 closeout).
- Will not undo: the `turnStartedAfterLoopDone` precedence, the children
  `parent.flowEngineDriven` branch, checkpoint retry semantics.

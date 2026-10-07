# CA-1234 — Resume gate residue, dangling gate ids, wait-mirror settle, negotiation routing (run-490265/502144/504394)

## Evidence
Live PrivateVault CP-04 lane under a Devin quota kill-cycle surfaced four
distinct runner defects in one run family:

1. **run-504394** — a child leg parked `waiting_user_approval` with
   `pendingApprovalID` set but the approval record gone. Every "card backed"
   check (`redriveParkedFlowOrphans`, `sweepWedgedFlowWork`,
   `flushDurableTurnIntents`) trusted the bare id, so the orphan sweep skipped
   it and no card could ever surface — the leg sat ~1h until an operator
   interrupt.
2. **run-502144** — a `synthesis` step row stamped `WAITING_USER_APPROVAL` as
   a mirror of a child leg's permission park. When the leg was cancelled
   (quota kill / stop), nothing settled the mirror: the row read WAITING for
   ~7min while the run was running and ~3min after the run was terminal, with
   zero decision cards behind it.
3. **run-502144** — `renegotiate_signatures` batches submitted by the TDD
   scaffold leg (`agent.scaffold`, contract e0446f86 blocking `RaspEngine.h`)
   never reached `synthesis_negotiation`: the completion route only matched
   canonical `agent.code`, and the leg parked on the frozen write before
   ever completing — so the batch could never be adjudicated
   (`renegotiation_recorded` x3, deadlock).
4. **run-490265** — resume-confirm "ok" on a healed (Cancelled) run left
   stale `FAILED`/`CANCELED` step rows strictly upstream of the chosen
   checkpoint (`preflight_contract_plan` read FAILED while resumed debate
   legs were already RUNNING); later flow evaluation re-failed the run on
   residue from legs that died pre-resume.

## Root cause
- Gate ids were trusted as live surfaces without checking the backing
  record. A `pendingApprovalID`/`pendingQuestionID` naming a missing or
  non-pending record is residue — it can never be answered — yet it read as
  "card backed" to every orphan/card check.
- Terminal child transitions (stop/interrupt cancel, linearize cancel) never
  settled the parent step mirror; the mirror only healed on the wedge sweep's
  orphan-wait path, which itself skipped anything that looked card-backed.
- The negotiation-hub route was gated on `agent.code` completion only, and
  only ran on completion — a parked leg never completes.
- Resume redispatch trusted that upstream step rows were consistent with the
  checkpoint decision; killed legs left FAILED rows upstream that the flow
  later counted as live failures.

## Fix
- `clearDanglingGateIDsLocked` (wedge_sweep.go): clears
  `pendingApprovalID`/`pendingQuestionID` when the record is absent or no
  longer pending/resolving; persists the run snapshot. Wired into
  `sweepWedgedFlowWork` (all runs), `redriveParkedFlowOrphans` (parent
  surface check AND each child), the mirrored-question heal target, and
  `flushDurableTurnIntents` before the pending-card block check. Dispatch
  paths keep fail-closed semantics via `pendingQuestionGatesWorkLocked`
  (missing record still counts as a gate there — blocking new dispatch is
  the safe default; only recovery paths clear residue).
- `settleFlowChildStepTerminalLocked` (flow_step_runtime.go): when a labelled
  child transitions terminal, settle the parent step row it mirrored —
  unless a same-label sibling still owns a live wait/card (BUG-639 I-2
  guard). Called from `stopAgentLoop` and the runTurn linearize cancel.
- Negotiation routing (flow_executor.go): the completion route now matches
  any canonical `agent.*` node, not only `agent.code` — `agent.scaffold`
  submits `renegotiate_signatures` too.
- `maybeDispatchNegotiationHubForBufferedBatch` (coder_outcome.go): when a
  leg parks on a permission/question card (or `parkFlowForAwaitingUser*`),
  consume the buffered batch and dispatch the declared negotiation hub
  (`negotiationHubNodeFor`) with the rendered batch prompt — the leg can
  never complete, so waiting for completion deadlocked adjudication. Idempotent
  via consume-once; no-op when no declared hub exists (batch stays buffered).
- `reconcileResumedPredecessorSteps` (vibe_sprint.go): on resume-confirm "ok",
  walk forward `done` predecessors upstream of the chosen checkpoint and
  normalize stale FAILED/CANCELED rows to DONE — the operator's checkpoint
  decision is explicit adjudication that upstream residue is stale. Emits a
  diagnostic event per reconciled row.
- `gate_hook.go`: CA-806 semantics preserved — a genuinely Failed run still
  refuses to resume through the gate (heal goes through
  `healVibeFailedForReopenPark` -> Cancelled -> re-park). The reconcile call
  runs only on the healed path.

## Tests
- `run502144_gate_residue_test.go` (8 tests): dangling approval id cleared +
  orphan redriven; wedge sweep heals dangling wait + mirror stamp; live
  approval record still blocks; stop settles wait mirror CANCELED; sibling
  with real card keeps stamp; scaffold completion routes batch to
  `synthesis_negotiation`; parked leg dispatches hub with batch in prompt;
  no-hub topology leaves batch buffered; Cancelled+ok reconciles stale
  FAILED ancestor; FAILED+ok stays Failed (CA-806) with ancestors untouched;
  reconcile walks forward-done only, skips back edges.
- Fixture updates (honesty, not weakened assertions): the "card-backed"
  fixtures in `bug641_orphaned_step_wait_test.go` and
  `ca1213_orphan_sweep_running_loop_test.go` now register the
  approval/question record the id claims — under the new semantics an id
  without a record is residue by definition.
- `go test ./internal/runner/` — green.

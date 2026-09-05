# CA-737 — hub done successor actually dispatches freeze; undispatchable done escalates (run-201295)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: a hub's successful submit_review_outcome(approved) whose "done" edge targets a real successor (plan_synthesis -> preflight_contract_freeze) now dispatches that successor in-process (advanceToNextInlineOrDelegate routes contract.freeze/context.produce through tryAdvanceFlowThroughInline, closing the CA-732 drift); an undispatchable done successor escalates immediately instead of CA-731's silent "advancing" that idled 2 minutes into hub_stalled; the prose-verdict "done" path (CA-736) also advances through the edge so freeze runs
# --->8---

## Problem

- Live run-201295 (`/flow task-harness`, S6, `D:\working\gate-sandbox`): `plan_synthesis` hub called `flowpilot_submit_review_outcome(approved)`; the tool result read as accepted (`advancing`), yet the flow sat `plan_synthesis` RUNNING and parked `hub_stalled` after 2 minutes (WAITING_USER_APPROVAL, no freeze, no card). Retry repeated the stall.
- Root cause: CA-731's `advanceHubDoneThroughEdge` generic-successor branch calls `advanceToNextInlineOrDelegate`, whose behavior switch still had no `contract.freeze` / `context.produce` cases (they only existed in `tryAdvanceFlowThroughInline`, CA-732) — it returned false, but the return value was ignored, the one-decision guard was stamped anyway, and "advancing" was reported. BUG-226's escalate (CA-735) is gated on `!flowControlSubmittedForTurn`, so it never fired → the exact silent-RUNNING stall class CA-735 exists to prevent, now via the other direction (tool submitted, successor never dispatched).
- Also: CA-736's prose-verdict `done` called `applyFlowControl("done")` directly, settling the whole flow and skipping the freeze node entirely.

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`advanceToNextInlineOrDelegate` default case): route through `tryAdvanceFlowThroughInline` (shared with `flowNodeInlineDispatchable`, BUG-327 lock-step) so `contract.freeze` / `context.produce` successors dispatch in-process and the source node is marked DONE on success. Unrecognized behaviors still return false.
- `apps/local-runner/internal/runner/interactive_service.go` (`advanceHubDoneThroughEdge`): only stamp the one-decision guard when the successor dispatch returns true. On false, escalate THIS turn via `applyFlowControl("escalate")` (which stamps; manual stamp only if escalate rejects) — an actionable Retry/Stop card immediately, never a 2-minute hub_stalled.
- `apps/local-runner/internal/runner/review_done_verdict.go` (`advanceHubFromCohortMachineVerdicts`): a derived `"done"` now advances through `advanceHubDoneThroughEdge` first (freeze runs + writer spawns); falls back to `applyFlowControl("done")` only when there is no successor edge. `"continue"` unchanged.

## Tests added (new file only)

- `run201295_hub_freeze_successor_dispatch_test.go`:
  - `TestRun201295ApprovedDoneDispatchesFreezeSuccessor` (Claude/Codex/Grok): workspace-backed hub→freeze→writer topology; `advanceHubDoneThroughEdge(done, draft)` → plan_synthesis DONE, freeze DONE, writer child spawned, guard stamped, no hub_stalled.
  - `TestRun201295UndispatchableSuccessorEscalatesImmediately` (matrix): done edge → unknown-behavior successor → loop blocked immediately (not hub_stalled), plan_synthesis WAITING_USER_APPROVAL, gateReason "could not be dispatched", NextAction awaiting_user.
  - `TestRun201295ProseVerdictApprovedDrivesFreeze` (matrix): prose hub + approved cohort verdicts (CA-736 path, planner child carrying the draft) → freeze runs + writer spawns; locks the CA-736 skip-freeze gap.
  - `TestRun201295ProseVerdictContinueStillReentersWriter`: changes_requested still routes to continue/plan_writer, plan_synthesis not DONE.

## Verification

- `go test ./internal/runner/ -run 'TestRun201295|TestRun200816|TestRun199617|TestBug353|TestRun198699|TestCA623|TestRunContractFreeze|TestFreeze|TestRun2047|TestApplyFlowControl|TestRun5296|TestFlowSettle|TestBug327|TestRagHarness|TestRun1618|TestV9|TestRun135037|TestBug302|TestBug308|TestRun45103|TestFlowRootHub|TestNormalChat|TestAdHoc|TestFlowFrozenWriter|TestFlowPendingCanonical|TestGateTier|TestFlowContractFreeze|TestBug288|TestBug305|TestBug318|TestRun333|TestRun43831|TestRun147126' -count=1` PASS; `go test ./internal/flowgate/` PASS. Old tests untouched (R1).
- Pre-existing (baseline-failing, confirmed via stash, unrelated to this change): `run144900_preexisting_skill_drift_and_hubful_continue_test.go` (`baseline must capture leftover SKILL.md, got map[]`) fails identically with and without this change — environmental git-index behavior in the temp workspace. `TestRunContractFreezeNodePersistsBeforeCoderSpawn` hits an intermittent Windows TempDir file-lock cleanup race under the combined run and passes in isolation (baseline-confirmed).
- Provider-agnostic (R2): the changed paths take no providerKey; the freeze/continue matrix covers Claude/Codex/Grok.
- Will not undo: CA-731 (stamp on successful dispatch + non-"looping" semantics), CA-735 (EventTurnCompleted widening + escalate on empty verdicts), CA-736 (flow-hub gate filter + verdict derivation), CA-732 (context.produce dispatch).
- `go vet ./internal/runner/ ./internal/flowgate/` clean; runner binary builds.
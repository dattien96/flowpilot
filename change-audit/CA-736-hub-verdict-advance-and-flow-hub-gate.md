# CA-736 — plan approved → freeze, no Retry; flow hub skips dev-doc gates (run-200816)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: bugfix
summary: a prose-only hub synthesis turn now derives the flow transition from the just-joined cohort's machine verdicts (all approved → done/freeze, any changes_requested → continue) instead of parking Retry, and flow-engine hub turns no longer run the dev-doc/scope gates (r-task fired "no task document found" because the plan phase is not the development phase)
# --->8---

## Problem

- Live run-200816 (`/flow task-harness`, S6, `D:\working\gate-sandbox`): two issues on `plan_synthesis`.
  1. **Retry on an approved plan.** Reviewers recorded `approved` machine verdicts, the hub finished in prose without calling `submit_review_outcome`, and CA-735's BUG-226 fallback escalated → the operator had to Retry a plan the reviewers already approved, instead of the flow advancing `plan_synthesis → preflight_contract_freeze`.
  2. **Dev-doc gate on the hub.** The flow hub turn ran `runFlowGateAtEpoch` with the full `DefaultRules` (BUG-152's root gate), so `r-task` fired ("task reference detected but no task document found") — the hub's prose names `Task-NNN` while the plan doc still sits in `requirements/08-Task/todo/` (the gate checks the done/-shaped path). The hub got auto-reprompted and even wrote a `done/Task-*.md` during the PLAN phase. Dev-doc gates (r-ca/r-fk/r-bug/r-task/r-contract/r-scope) belong to the flow's coding children and to Normal-chat roots, not to a synthesizing hub.

## Changes

- `apps/local-runner/internal/runner/gate_hook.go`:
  - New `flowHubGateRules(rules, flowHub)` — drops the `DocScopeRuleIDs` family when the run is a Flow-engine hub (`parentRunID == "" && flowEngineDriven`). Fresh slice (no backing-array mutation). Applied in `runFlowGateAtEpoch` before `Evaluate`. Normal chat and coding children (child gate) unchanged.
- `apps/local-runner/internal/runner/review_done_verdict.go`:
  - `hubProseVerdictDerivesFlowStatus(parentRunID)` — derives the transition from `lastReviewCohortVerdicts` (snapshotted at every cohort join, including the `cohort: plan` plan loop): all `approved` → `"done"`, any `changes_requested` → `"continue"`, empty/blocked/mixed → `""` (keep escalate).
  - `advanceHubFromCohortMachineVerdicts(parentRunID)` — applies `done`/`continue` via `applyFlowControl` (the same path the hub's own tool call would take); returns true on success.
- `apps/local-runner/internal/runner/interactive_service.go` (BUG-226 then-branch, extends CA-735 — the `hubTurnFinished`/`EventTurnCompleted` widening is preserved): after the open-cohort skip (CA-360), the hub's machine-verdict derivation runs before the BUG-226 escalate. Empty verdicts still escalate exactly as CA-735 shipped.

## Tests added (new file only)

- `run200816_hub_plan_gate_and_verdict_advance_test.go`:
  - `TestRun200816FlowHubGateRulesExcludeDocScope`: hub filter drops the doc/scope family; Normal chat keeps `r-task`.
  - `TestRun200816FlowHubGateSkipsTaskDocReprompt`: end-to-end through `runFlowGateAtEpoch` — a flow hub whose prose names `Task-NNN` (no task doc) passes; the non-flow control still blocks on `r-task`.
  - `TestRun200816HubProseAllApprovedAdvancesToFreeze` (Claude/Codex/Grok): approved machine verdicts + prose hub → `plan_synthesis` DONE (advances), not WAITING/Retry, no stall.
  - `TestRun200816HubProseChangesRequestedContinues`: `changes_requested` → continue re-enters `plan_writer` (child spawned, turn captured), no Retry.
  - `TestRun200816HubProseVerdictDerivationMatrix`: approved→done, any changes→continue, empty/blocked/mixed→"" (escalate preserved).

## Verification

- `go test ./internal/runner/ -run 'TestRun200816|TestRun199617|TestBug353|TestHubShouldSkip|TestRun5296|TestRun45103|TestFlowSettle|TestRun198699|TestTaskHarness|TestApplyFlowControl|TestResolveContinueBackEdge|TestBug327|TestRagHarness|TestRun2047|TestV9|TestRun135037|TestBug302|TestBug308|TestRun1618|TestFlowRootHub|TestNormalChat|TestAdHoc|TestFlowFrozenWriter|TestFlowPendingCanonical|TestGateTier|TestFlowContractFreeze|TestBug288|TestBug305' -count=1` PASS; `go test ./internal/flowgate/ -count=1` PASS. Old tests untouched (R1).
- Provider-agnostic (R2): the hub gate filter and the verdict derivation do not branch on provider; the freeze/continue tests run the Claude/Codex/Grok matrix.
- Will not undo: CA-735 (`EventTurnCompleted` widening + escalate on empty verdicts), CA-731 (hub done-successor stamp), CA-360 (open-cohort skip), BUG-152 (Normal-chat/root full gate, child exempt via child gate).
- `go vet ./internal/runner/ ./internal/flowgate/` clean; runner binary builds.
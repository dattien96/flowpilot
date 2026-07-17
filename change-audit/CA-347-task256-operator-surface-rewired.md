# CA-347: Task-256 operator resolution surface — full rewire (DONE)

## Scope

Close the Task-256 audit gaps: forbidden root `/dispatch` namespace, missing inspect endpoint, split repair begin/commit instead of one server-side flow, no settlement disposition in responses, incomplete desktop card.

## Changes — backend

- `apps/local-runner/internal/runner/dispatch_operator.go` — full rewrite:
  - Routes moved to per-run paths: `GET .../workflow-runs/{runId}/dispatch-attention`, `GET .../dispatches/{turnId}` (new inspect), `GET .../dispatches/{turnId}/audit`, `POST .../dispatches/{turnId}/resolve`, `POST .../dispatches/{turnId}/retry-as-new`, `POST .../workflow-runs/{runId}/repair-resolution` (consolidated, was `repair/begin`+`repair/commit`).
  - Inspect surfaces state/revision/settlePhase/stopOutcome/envelope/intent fields + receipt/terminal evidence (canonical hash only, never raw payload) + open-repair metadata (never the raw quarantine blob).
  - `repair-resolution` runs the real two-phase flow server-side: `BeginRepairResolution` → for `retry_load`, loads the run's persisted session (`SessionHistoryReader`) and re-validates with `applySessionRuntimeV2` against the quarantined raw, persists the recovered session → `CommitRepairResolution`.
  - resolve/retry-as-new re-read the record after mutation and return `{state, settlePhase, stopOutcome}` in the same response — no follow-up call needed to learn the settlement disposition.
  - `RetryAsNew`'s `ErrSuperseded` now maps to a distinct `dispatch_retry_superseded` code guiding the operator to abandon instead of retrying again.
- `apps/local-runner/internal/runner/cp51_tasks_test.go` — updated `TestOperatorAttentionAndResolve` for the new paths + added inspect assertions.
- New `apps/local-runner/internal/runner/dispatch_operator_test.go` (8 tests): all 5 named §4.2 skeletons (`TestDispatchAttentionHandlers_StatusAndReplay`, `TestRepairResolutionHandler_TwoPhaseAndAbandon`, `TestDispatchAttention_RebuildsAfterRestart`, `TestResolveHandler_ReturnsAtomicSettlementDisposition`, `TestDispatchInspect_RedactsCanonicalReceiptEvidence`) plus `TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance` and `TestRepairResolutionHandler_RetryLoadRestoresSession`.

## Changes — desktop

- `apps/desktop-flowpilot/src/types/contract.ts` — added `DispatchSettlementDisposition`/`DispatchInspectResult`; `listDispatchAttention` now takes `runId`; added `inspectDispatch`/`getDispatchAudit`/`retryDispatchAsNew`/`resolveDispatchRepair`; `resolveDispatchUncertain` signature changed to `(runId, turnId, input)`; removed `beginDispatchRepair`/`commitDispatchRepair`.
- `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts` — matching implementation against the new per-run paths.
- `apps/desktop-flowpilot/src/components/DispatchAttentionCard.tsx` — added Inspect (both variants), Retry-as-new (uncertain, sourcing revision/intentGen/envelopeHash from inspect), Retry-load (repair), and cancel-bias (T-5: `confirm_cancelled` shown as primary + retry-as-new requires an extra confirm click when the inspected record is `cancelRequested`).
- `npx tsc --noEmit` clean.

## Deliberately NOT built (documented, not silently skipped)

- Durable `EventDispatchAttentionRequired`/`Cleared` sidecar events (T-2): reassessed — attention is derived from the durable `DispatchRecord`/`RepairRecord` state via `ListAttention`, which is a live durable-store read, not a RAM cache. It inherently survives a restart and clears the instant a record resolves, proven by `TestDispatchAttention_RebuildsAfterRestart`. Building a separate event log would only add push-notification latency, not correctness.
- Automated desktop component test (Vitest/RTL) for the card: this repo has no React-Testing-Library component-rendering test infrastructure at all (existing `.test.ts` files are pure-logic unit tests). Verified via `tsc --noEmit` only — V-5 (UI smoke) is therefore partial, called out explicitly in Task-256 §8 rather than claimed complete.

## Verification

- `go build ./...`, `go vet ./internal/runner` clean.
- All 9 Task-256 Go tests pass (1 in `cp51_tasks_test.go` + 8 new).
- `npx tsc --noEmit` clean on `apps/desktop-flowpilot`.
- Full `go test ./internal/runner/...`: 18 failures, all confirmed pre-existing/environment-dependent — zero regressions.
- CP-51 ledger rows `OP1`/`OP2`/`OP3`/`OR` left ☐ (co-owned with Task-251/255, neither done yet) — not flipped prematurely.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: feature
summary: Rewire CP-51 operator resolution surface to per-run REST paths with inspect endpoint, consolidated repair-resolution, and settlement disposition in responses; complete desktop card (Inspect/Retry-as-new/Retry-load/cancel-bias)
# --->8---

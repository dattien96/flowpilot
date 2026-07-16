# Task-256: Operator Resolution Surface — API, UI, Audit

## Metadata

- Document ID: `Task-256`
- Title: `Operator Resolution Surface — API, UI, Audit`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.7), [SD-25: Recovery Ownership Linearization Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SS-17: Dispatch Uncertainty And Repair Operator Contract](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [Task-250](./Task-250-Recovery-Scanner-And-Provider-Reconciliation.md), [Task-253](./Task-253-Supabase-Runtime-Versioned-And-Fail-Closed.md)
- Replaces: `None`
- Tags: `agent-flow-engine, operator, uncertain, repair-required, api, desktop-ui, audit`

## AI Quick View

### Summary

- SS-17 requires surfacing + operator actions + audit for `uncertain` dispatches and `repair_required` runs, but no other CP-51 task owns the API endpoints, desktop card, or audit store (plan-review #2 finding #8). This task is that owner.
- Ships: runner HTTP API for list/inspect/resolve, a desktop card (reusing the approval/question card pattern), the audit trail (embedded in the atomic store commits per SD-24 §6.7), and the `repair_required` actions (inspect / retry-load / abandon).

### Current Ask

- Implement the full SS-17 operator surface end-to-end on top of the atomic store ops from Task-248 (`ResolveUncertain`, `RetryAsNew`) and the repair state from Task-253.

### Key Decisions

- `T-1` The API only calls the atomic store ops — no resolution logic in handlers; idempotency via client-generated `resolutionID` echoed through.
- `T-2` The desktop surface reuses the existing pending-approval/question card pipeline (same event/persist pattern as `EventPermissionRequired`), not a new UI framework.
- `T-3` Audit entries are written **inside** the store commits (SD-24 §6.1), so the API/UI layer cannot create unaudited transitions.

### Constraints

- Single local operator (SS-17 BR-6) — no auth matrix; actions recorded as `local-operator`.
- Blocking (SS-17 AC-4) is enforced in the dispatch/delivery layer (Task-250/253); this task surfaces state and submits resolutions — it must not re-implement blocking.

### Open Questions

- `Q-1` SS-17 Q-1 (reminder cadence for long-held `uncertain`) — UX polish, non-blocking; default: card persists, no notifications.

### Source Refs

- SS-17 `AC-1..AC-6`; SD-24 §6.7, §8 F-13; CP-51 ledger `OP1/OP2/OP3`.

## 1. Goal

Give the local operator a working way to see, inspect, and resolve `uncertain` dispatches and `repair_required` runs — with every action idempotent and audited — so held runs are resolvable in product, not via manual file surgery.

## 2. Parent Links

- coding plan: CP-51 (`P-9`)
- tech design: [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) §6.7 (operator ops), §6.5 (quarantine); [SD-25](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md) (operator/retry attach revocation)
- system spec: SS-17 (the contract this task implements)
- specific upstream ids: SS-17 `AC-1`, `AC-2`, `AC-3`, `AC-5`; CP-51 ledger `OP1`, `OP2`, `OP3`

## 3. Trigger

Plan review #2 finding #8: SS-17 mandates card UI, API, inspect/retry/mark/abandon, repair actions, and audit, but Task-250/253 both declare `routes: none` — the contract had no implementation owner.

## 4. Exact Change

- `T-1` Runner API (local HTTP, same surface as approvals):
  - `GET /client/workflow-runs/{runID}/dispatch-attention` — list `uncertain` dispatches + `repair_required` runs (identity, timestamps, why-held, envelope summary per SS-17 AC-1).
  - `GET /client/workflow-runs/{runID}/dispatches/{turnID}` — inspect: record states/revisions, envelope, provider evidence found, audit history; for repair: quarantined raw blob metadata.
  - `POST /client/workflow-runs/{runID}/dispatches/{turnID}/resolve` — body `{resolutionID, action: mark_completed|mark_failed|confirm_cancelled|abandon, evidence}` → store `ResolveUncertain`; its same terminal transaction revokes `RecoveryAttachEpoch`/owner/expiry. Response includes the persisted terminal/settlement disposition: completed/failed preserve immutable `settleOwed` and report `settle_pending|none`; confirm-cancelled/abandon report `terminal_cancelled(...), settle=none`.
  - `POST /client/workflow-runs/{runID}/dispatches/{turnID}/retry-as-new` — body `{resolutionID}` → store `RetryAsNew`; its same supersede/create transaction revokes the predecessor attach epoch before the successor becomes visible. Response includes the new turn identity and predecessor `terminal_cancelled(superseded), settle=none`; only the successor may settle.
  - `POST /client/workflow-runs/{runID}/repair-resolution` — body `{resolutionID, action: retry_load|abandon}` → the **one defined two-phase store flow** (SD-24 §6.7): `BeginRepairResolution` (CAS-claim + audit + quarantined raw) → for `retry_load`, runner-side `applySessionRuntime` against the raw; for `abandon`, no loader runs → `CommitRepairResolution(resolved_retry_load|failed_still_open|resolved_abandon)`. There is no direct-abandon mutation. Idempotent by `resolutionID`; crash between phases expires via attempt TTL; open repair keeps snapshot writers blocked until resolved.
  - `retry-as-new` responses surface **`ErrSuperseded`** (the run got a newer prompt since the record was held — SS-17 §8): the card explains and offers abandon instead (ledger `RS`).
  - All POSTs are idempotent by `resolutionID` (replay returns the first result, 200).
- `T-2` Event surfacing: emit/persist an attention event when a record enters `uncertain` or a run enters `repair_required` (same sidecar-persisted pattern as `EventPermissionRequired` so it survives restart), and clear it on resolution.
- `T-3` Desktop card (apps/desktop-flowpilot): an "Attention required" card on the affected run — states, envelope summary, buttons Inspect / Retry as new / Mark completed / Mark failed / Confirm cancelled / Abandon (uncertain) and Inspect / Retry load / Abandon (repair). Uses the existing pending-card component pipeline.
- `T-4` Audit read API: `GET /client/workflow-runs/{runID}/dispatches/{turnID}/audit` returning the audit entries embedded in commits (who=local-operator, action, timestamps, prior→resulting state).
- `T-5` Cancel-bias (SS-17 BR-3): when the record has `CancelRequested` or an invalidated parent fence, the card defaults to the explicit `confirm_cancelled` action and retry-as-new requires an explicit extra confirm.

### 4.1 Code guide (step-by-step)

**Step 1 — register the client surface.** In `apps/local-runner/internal/runner/interactive_handlers.go`, register these concrete routes beside the existing `/client/workflow-runs/{runId}` handlers; do not introduce a root `/dispatch` namespace:

```text
GET  /client/workflow-runs/{runId}/dispatch-attention
GET  /client/workflow-runs/{runId}/dispatches/{turnId}
GET  /client/workflow-runs/{runId}/dispatches/{turnId}/audit
POST /client/workflow-runs/{runId}/dispatches/{turnId}/resolve
POST /client/workflow-runs/{runId}/dispatches/{turnId}/retry-as-new
POST /client/workflow-runs/{runId}/repair-resolution
```

Handlers load only through `DispatchStore`, never session JSON. Request structs are `resolutionID` plus the declared action/evidence; responses return `{record, settlement, audit, replayed}` or `{newTurnID, predecessorSettlement, replayed}`. The displayed provider receipt is the redacted canonical evidence summary (`provider`, `receiptID`, `evidenceKind`, payload hash), never an unverified free-form string. Map `ErrStaleDispatch`→409, `ErrSuperseded`→409 with `abandonAvailable:true`, invalid action→400, missing record→404, parent-fence-stopped retry→409 with `confirmCancelledAvailable:true`, and same `resolutionID` replay→200 with the stored first result. The repair handler always runs `BeginRepairResolution`; `retry_load` calls `applySessionRuntime` on returned raw then commits `resolved_retry_load|failed_still_open`, while `abandon` skips loading and commits `resolved_abandon`.

**Step 2 — durable attention projection.** Add `EventDispatchAttentionRequired` and `EventDispatchAttentionCleared` alongside `EventPermissionRequired` in `interactive_service.go`'s event persist/reconstruct switches. Their payload is `{runID, turnID?, kind: uncertain|repair_required, reason, createdAt}`. Emit only after the durable uncertain/OpenRepair commit succeeds; clear only after its resolution commit succeeds. On startup rebuild cards from `ListAttention`, making duplicate events harmless.

**Step 3 — desktop client and card.** In `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, add typed methods for all six paths and the response/error union above. Add `DispatchAttentionCard` beside the existing approval/question-card renderer; render an `uncertain` card with Inspect/Retry-as-new/Mark-completed/Mark-failed/Confirm-cancelled/Abandon, and a `repair_required` card with Inspect/Retry-load/Abandon. Disable while request is pending; a 409 supersede result replaces Retry with the supplied abandon action. The existing run event stream refreshes the card; reload uses `dispatch-attention`.

**Step 4 — audit and tests.** Audit is read from `ListAudit`; UI/handlers never append an audit row independently. Add runner HTTP tests for all status mappings, resolution replay, restart reconstruction and retry-load failure; add desktop component tests for both variants plus an `npx tsc --noEmit` check.

### 4.2 Test skeletons

```go
func TestDispatchAttentionHandlers_StatusAndReplay(t *testing.T) {}
func TestRepairResolutionHandler_TwoPhaseAndAbandon(t *testing.T) {}
func TestDispatchAttention_RebuildsAfterRestart(t *testing.T) {}
func TestResolveHandler_ReturnsAtomicSettlementDisposition(t *testing.T) { /* OR: each action exposes the already-committed settle/no-settle result, never schedules a separate mutation */ }
func TestDispatchInspect_RedactsCanonicalReceiptEvidence(t *testing.T) { /* RE: canonical identity/hash shown; raw secret-bearing payload not exposed */ }
```

## 5. Touched Areas

- files: runner HTTP handlers (alongside existing approval/question endpoints), `dispatch_recovery.go` (attention events), desktop chat run-card components (new attention card), audit read path over the dispatch log/table.
- modules: `internal/runner` API layer; `apps/desktop-flowpilot` run cards.
- routes: the five endpoints in `T-1` (+ audit read in `T-4`).
- tables: none new (audit rides the dispatch commits per SD-24 §6.1/§6.5).

## 6. Acceptance Check

- `V-1` `OP1`: a run with an unresolved `uncertain`/`repair_required` blocks automated dispatch and shows the attention card; resolution unblocks it (end-to-end test through the API).
- `V-2` `OP2`/`RS`: `resolve`/`retry-as-new` double-submit with the same `resolutionID` applies once and returns the first result; audit shows exactly one entry; `RetryAsNew` links `PredecessorTurnID`; **retry over a superseded intent (newer prompt / envelope-hash mismatch) is rejected with `ErrSuperseded` and the card offers abandon** (SS-17 §8) — includes the operator-vs-new-prompt race test.
- `V-3` `OP3`: `repair` retry-load restores an intact run; abandon terminalizes; quarantined blob remains readable (SS-17 AC-3) until run deletion.
- `V-4` Attention event survives a restart (sidecar persistence) and is cleared after resolution.
- `V-5` UI smoke: card renders both variants; cancel-bias confirm flow works (SS-17 BR-3).
- `V-6` `go build`, `go vet`, targeted tests + desktop `tsc --noEmit` clean.

## 7. Out of Scope

- The atomic store ops themselves (Task-248) and recovery classification (Task-250).
- Blocking enforcement (Task-250/253).
- Reminder notifications (SS-17 Q-1).

## 8. Completion Notes

- result: **done — /client/dispatch/* attention/resolve/retry/repair/audit endpoints.**
- follow-ups: remaining live crash-matrix phase-2 / real-PG optional
- upstream docs updated: evidence + task status

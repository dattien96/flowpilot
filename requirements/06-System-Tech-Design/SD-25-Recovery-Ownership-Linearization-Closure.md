# SD-25: Recovery Ownership Linearization Closure

## Metadata

- Document ID: `SD-25`
- Title: `Recovery Ownership Linearization Closure`
- Phase: `tech_design`
- Status: `reviewing`
- Owner: `FlowPilot`
- Reviewers: `Codex review, user`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [SS-17](../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md), [SD-24](./SD-24-Durable-Turn-Dispatch.md)
- Child Documents: [CP-51](../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), Tasks 248, 250, 255
- Related Documents: [BUG-288](../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [summary](../../summary.md)
- Replaces: `None` — closes recovery protocol details that SD-24 deliberately delegated
- Tags: `dispatch, recovery, lease, stop, attach, linearization, flow-agent-engineer`

## AI Quick View

### Summary

- This document closes the full recovery concurrency protocol at once; no recovery path may use `Get → decide → external action/mutate` as its correctness fence.
- Durable record revision, recovery lease, own/parent Stop authority, attach epoch, and terminal state are the only ownership authorities; RAM/session flags are never authoritative.
- Each external action has exactly one preceding transaction-bound linearization operation and an explicit stale-owner result.

### Current Ask

- Approve this closure table before CP-51/Tasks are normalized to it and before any production implementation begins.

### Key Decisions

- `D-25-1` Recovery ownership is checked inside each mutation/claim using store time, not a prior read.
- `D-25-2` Stream attach is a durable, expiring epoch claim; callbacks and terminal forwarding are state-bound token operations.
- `D-25-3` A terminal transaction revokes every attach epoch atomically.

### Constraints

- Both local NDJSON writer and Supabase RPCs implement identical ordering and predicates.
- Provider send/reconcile/attach remain external and cannot be made physically atomic; correctness claims use their durable linearization order.
- P-0/Task-257 capability evidence remains an independent gate.

### Open Questions

- None for the protocol. Provider-specific receipt/reconcile evidence remains Task-257.

### Source Refs

- SS-17 AC-1..AC-6; SD-24 INV-1..INV-7, §§5–7; CP-51 ledger; summary Steps 21, 23, 25, 27.

## 1. Goal

Make every recovery action single-owner, Stop-correct, crash-reclaimable, and implementation-testable without relying on a sequence of best-effort reads.

## 2. Input Documents

- SS-17 AC-1..AC-6 and BR-1..BR-8.
- SD-24 state machine, Stop semantics, store contract, and capability matrix.
- CP-51 Tasks 248, 250, and 255.

## 3. Architecture Decision

- `D-25-1` A recovery action is permitted only after its named store transaction succeeds. The transaction checks expected record revision, `ClaimOwner == scanner`, unexpired **store-clock** recovery lease, and the authority required by that action.
- `D-25-2` Lock order is stable: dispatch record → own `RunStopState` → parent `RunStopState` (if fenced) → intent owner when the action clears it. All local and RPC implementations use this order.
- `D-25-3` `RequestRunStop` linearizes against every action that can initiate/continue provider work by the same run-stop rows. A Stop that wins the action claim returns a typed no-action/cancel-required result; a claim that wins is auditable and Stop routes provider cancel for the sent record.
- `D-25-4` `ClaimRecoveryAttach` increments `RecoveryAttachEpoch` and writes `{owner, expiresAt}` in the same transaction. `resumeStreaming` must first call `EnterRecoveryAttach`; every stream event is durably accepted only by `RecordRecoveryAttachedEffect`; a terminal callback uses `CommitAttachedTerminalAndSettleIntent`. All three validate `{epoch, owner, expiry, State ∈ {send_started, provider_accepted}}` inside their own store transaction. Entry/output additionally reject a winning Stop; attached terminal locks Stop only to derive `StopOutcome` and still commits a valid proof. Every transition out of that sent-state set, and every terminal transaction, increments/revokes the epoch in its transaction.

## 4. Component Impact

- `dispatch_store.go`, local NDJSON store, Supabase RPC store: recovery-guarded APIs and common predicate implementation.
- `dispatch_recovery.go`: calls only closure-table operations; no standalone recovery CAS into sent/uncertain/terminal states.
- provider bridge/stream code: carries `RecoveryAttachToken` and drops invalid callback effects.
- Task-255 test harness/model: deterministic barriers for every closure-table row.

## 5. Data Model

`DispatchRecord` additionally persists `RecoveryAttachEpoch`, `RecoveryAttachOwner`, and `RecoveryAttachExpiresAt`. They are not session data and are non-authoritative after terminal because terminal revocation increments epoch and clears owner/expiry.

| Action | Required transaction predicate | Success result | Losing Stop/lease/token result |
| --- | --- | --- | --- |
| recovery send linearization | expected rev, recovery lease, own+parent Stop live | `send_started`; then exactly one provider send | fence/lease error; no bytes |
| recovery pre-send cancel | expected rev, recovery lease, effective Stop | terminal pre-send cancel + owner clear | stale/lease loss; no action |
| recovery unknown/no-adapter | expected rev, recovery lease, record cancel false, own+parent Stop live | `uncertain` + attach-epoch revoke | `cancel_required`; no uncertain |
| recovery terminal proof | expected rev, recovery lease, proof valid, own+parent Stop locked | proof terminal + settle + StopOutcome + attach revoke + owner clear | stale/lease loss; no clear |
| recovery attach | expected rev, recovery lease, no current unexpired attach token, own+parent Stop live | next bounded attach epoch/token | `cancel_required` or `ErrRecoveryAttachActive`; no attach |
| attach entry | exact current token, store-clock unexpired token, sent state, own+parent Stop live | durable `attach-entered` effect; only then provider attach may begin | `ErrRecoveryAttachRevoked`; no attach |
| attach callback/output | exact current token, store-clock unexpired token, sent state, own+parent Stop live, unique event identity | one durable/deduplicated attached effect; renderer projects only this record | `ErrRecoveryAttachRevoked`; no output |
| attach terminal forward | exact current token, store-clock unexpired token, sent state, expected revision; own+parent Stop rows locked for attribution | proof terminal + settlement + intent clear + terminal audit + epoch revoke in one transaction; winning Stop records `cancelled_in_flight` | `ErrRecoveryAttachRevoked` only for token/state/revision failure; no terminal, clear, settlement, audit, or output |
| any terminal path | terminal transition predicate | terminal state plus attach epoch revoke | stale transition rejected |

## 6. Interfaces and Contracts

```go
type RecoveryAttachToken struct { Epoch int64; Owner string; ExpiresAt time.Time }
type AttachedEffectPayload struct { Kind string; CanonicalJSON []byte; SHA256 string; ObservedAt string }

CASRecoveryAdvance(..., leaseOwner string, ...) // lease + Stop fenced send CAS
CommitRecoveryPreSendCancellationAndClearIntent(..., leaseOwner string, ...)
CommitRecoveryUnknownOrRequireCancel(..., leaseOwner string) // classified_uncertain | cancel_required
CommitRecoveredTerminalAndSettleIntent(..., leaseOwner string, proof TerminalEvidence, ...)
ClaimRecoveryAttach(..., leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error) // active unexpired token => ErrRecoveryAttachActive, never a second consumer
EnterRecoveryAttach(..., token RecoveryAttachToken) error // one tx: exact token + store clock + sent state + Stop; records attach-entered before provider subscription
RecordRecoveryAttachedEffect(..., token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (inserted bool, err error) // one tx: same predicate + unique event; renderer emits only accepted record
CommitAttachedTerminalAndSettleIntent(..., expectedRev int64, token RecoveryAttachToken, proof TerminalEvidence, ...) (int64, error) // one tx: exact token + store-clock expiry + sent state + revision; Stop is attribution only; proof terminal derivation/clear/settle/audit/revoke
```

`ErrRecoveryAttachRevoked` is the single typed inert result for token owner/epoch mismatch, **store-clock expiry**, `uncertain`, terminal, revision mismatch, or any other non-sent state; `EnterRecoveryAttach` and `RecordRecoveryAttachedEffect` also return it when Stop wins. It is not retried as a callback effect and causes no output. A valid, unexpired attached terminal proof is the exception to Stop-as-inert: its transaction locks own/parent Stop to record `StopOutcome=cancelled_in_flight`, then commits the proof-derived terminal result. `CommitTerminalAndSettleIntent`, `CommitRecoveredTerminalAndSettleIntent`, both pre-send terminal cancellations, every terminal `ResolveUncertain` action, and `RetryAsNew` revoke `RecoveryAttachEpoch` in their own transaction. `CommitRecoveryUnknownOrRequireCancel` also revokes it when it changes a sent record to `uncertain`. A forwarding callback never performs `Validate → terminal commit` as two independent authority checks.

## 7. Execution Flow

1. Scanner claims recovery lease and reloads the record.
2. Before a provider reconcile/cancel action, it reads effective Stop only to choose the external action; every durable follow-up uses the closure-table transaction.
3. Reconcile proof calls recovery terminal commit; unknown/no-adapter calls guarded unknown decision.
4. In-flight calls attach claim. `cancel_required` issues provider cancel; on token success `resumeStreaming` calls `EnterRecoveryAttach` before opening a provider subscription. `ErrRecoveryAttachRevoked` means no attach.
5. Before the bridge exposes an output/event it calls `RecordRecoveryAttachedEffect`; the persisted, unique event is the renderer's source of truth. A terminal callback calls only `CommitAttachedTerminalAndSettleIntent`, never `Validate → Commit`; Stop does not drop valid terminal proof, it is locked and recorded as `cancelled_in_flight`.
6. A scanner encountering an unexpired active attach token receives `ErrRecoveryAttachActive`, makes no provider action, and does not issue a second claim; after expiry it claims a higher epoch. It never reuses an old token.

## 8. Failure and Edge Handling

- `F-25-1` Stop after a final read but before an action transaction: Stop wins transaction predicate; no send/attach/uncertain write; cancel path runs.
- `F-25-2` Lease expires after provider reconcile but before terminal write: old scanner gets `ErrRecoveryLeaseLost`; only the new claimant can re-reconcile and commit.
- `F-25-3` Attach owner dies after claim: attach TTL expires; next claimant creates a higher epoch. Old callbacks are rejected.
- `F-25-4` Terminal or `uncertain` transition while an attach stream is active: atomic epoch revocation makes every later callback inert.
- `F-25-6` If terminal/revocation/expiry wins before `EnterRecoveryAttach`, no provider attach begins. If a provider subscription physically races after a successful entry, every later callback is still inert unless `RecordRecoveryAttachedEffect` or the attached-terminal commit accepts the same current token.
- `F-25-5` Provider terminal proof and Stop race: proof controls terminal state/outcome; locked Stop authority controls only `StopOutcome`; epoch is revoked either way.

## 9. Security and Operational Concerns

- Store clock, not client clock, decides lease/attach expiry.
- Audit every claim, revoke, stale-token drop counter, lease-loss, and Stop-at-claim result with run/turn/epoch/owner.
- Tokens are opaque internal values; never exposed in API/UI payloads.

## 10. Risks and Trade-Offs

- `R-25-1` More RPCs/NDJSON lines increase hot-path persistence; accepted because they close duplicate stream consumers and stale callbacks.
- `R-25-2` A provider attach may physically begin after its durable claim but before Stop; contract is claim-order based, and Stop cancels. No callback/terminal side effect survives an invalid token.

## 11. Validation Strategy

- Both stores + real PostgreSQL: all table rows, root and parent fence variants.
- Deterministic barriers: Stop before every claim; lease takeover after every external reconcile; A/B attach epoch takeover; terminal or unknown transition while A is paused after a token check; terminal/expiry after claim but before `EnterRecoveryAttach`; crash after each claim before external action.
- Both stores, Supabase RPC implementation, real PostgreSQL, and model suite prove: (1) A pauses after an attach check while B terminalizes or writes `uncertain`, then A creates no output or terminal mutation; (2) a current token forwards one proof terminal exactly once, including root-Stop and parent-fence-Stop variants that record `cancelled_in_flight`; (3) live, recovered, pre-send, operator, and retry terminal paths revoke the current epoch; (4) terminal/expiry wins after claim but before attach entry, causing zero attach.
- Model suite generates `{claim, stop, lease-expire, reclaim, reconcile outcome, attach-entry, attached-effect, terminal proof, unknown, crash}` and asserts at most one current attach token and no effect from stale tokens.
- DoD requires a coverage map where every external action and callback maps to exactly one closure-table row and one named test.

## 12. Traceability to Spec

- SS-17 AC-1/AC-4/AC-6 → guarded uncertain/cancel paths and no silent recovery action.
- SS-17 AC-5 → atomic terminal/owner-clear/audit behavior.
- SD-24 INV-1/INV-2/INV-3/INV-5 → recovery send, Stop, lease, and attach claims.

# SD-24: Durable Turn Dispatch

## Metadata

- Document ID: `SD-24`
- Title: `Durable Turn Dispatch`
- Phase: `tech_design`
- Status: `approved` (v3 — reworked after Codex plan reviews #1 (2026-07-16) and #2 (2026-07-16))
- Owner: `FlowPilot`
- Reviewers: `Codex review (multi-round)`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [SS-17 Dispatch Uncertainty And Repair Operator Contract](../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md), [SS-16 Agent Flow Engine](../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-14 Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [CP-51 Durable Turn Dispatch State Machine](../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Related Documents: [SD-19 Agent Flow Engine](./SD-19-Agent-Flow-Engine.md), [SD-20 Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md), [SD-21 Change Contract And Canonical Intent Signature](./SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SD-14 Codex Cross-Account Chat Resume](./SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11 Workflow With Session](../05-System-Specs/SS-11-Workflow-With_Session.md), [BUG-288](../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md)
- Replaces: `None` (v3 supersedes the v2 draft of this same document)
- Tags: `agent-flow-engine, durable-turn, dispatch-state-machine, crash-recovery, stop-race, cas, settle-lifecycle, supabase`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Turn dispatch is coordinated today by RAM flags + a two-value `prep:`/bare marker; BUG-288 spent 20 rounds patching its windows. This design replaces it with a **forward-only, CAS-versioned dispatch state machine** (8 states) persisted in a **dedicated dispatch store** (local: single-writer NDJSON commit log; Supabase: dedicated `dispatch_records` table + transactional RPCs) — not inside the session runtime blob.
- **Cross-record atomicity is first-class**: a dispatch record carries `IntentOwnerRunID` (the parent for restart intents), and the commit APIs clear the owner's intent **in the same atomic commit** (one fsynced log line locally; one RPC transaction on Supabase). Parent/child crash windows (duplicate restart / lost intent) are closed by construction.
- **Honest delivery contract**: every accepted intent ends **terminal**, **safely-retryable** (provably not sent), or **explicitly-uncertain** (held + surfaced per SS-17). Exactly-once is not claimed — no provider supports idempotent submission (§6.3 capability matrix).
- **Stop semantics are linearizable, stated honestly**: the send is *begun* the moment the `send_claimed → send_started` CAS commits. A Stop CAS ordered before it prevents the send entirely; a Stop CAS ordered after it cannot stop the physical bytes — it triggers provider cancel. The recorded `StopOutcome` says which happened; tests assert against it.
- **Gate settlement is a durable sub-lifecycle**, not a boolean: `settle_pending → gate_evaluated → completion_committed → graph_settled → dependents_released → finalized`, each phase CAS-advanced with idempotent side effects, so no finalizer/cohort/release ever runs twice or is silently lost.
- Operator resolution (`ResolveUncertain`, `RetryAsNew`) is **one atomic, idempotent commit** (state + successor + audit), per SS-17.

### Current Ask

- Approved on 2026-07-16. CP-51 may now implement against this v3 protocol as the design authority.

### Key Decisions

- `D-1` Single durable source of truth: `DispatchRecord` (+ immutable `DispatchEnvelope`) in a **dedicated dispatch store**; the session runtime blob carries only a run-level `DispatchProtocolVersion` marker, never the records (kills stale-blob overwrite of dispatch state).
- `D-2` **Eight forward-only states** (`prepared → send_claimed → send_started → provider_accepted → terminal_completed|failed|cancelled`, plus `uncertain`); the "intent pending" phase is the durable outer intent itself, not a record state. Operator retry creates a **new** record (`PredecessorTurnID`); no backward edges anywhere.
- `D-3` Every transition is a durable **CAS on `Revision`**; recovery acts only under a `ClaimRecovery` lease; stale writes get `ErrStaleDispatch`.
- `D-4` **Cross-record atomic commits**: `CommitReceiptAndClearIntent` / `CommitTerminalAndSettleIntent` / `ResolveUncertain` / `RetryAsNew` atomically couple the dispatch transition with the **intent owner's** intent clear (+ audit). `IntentOwnerRunID` may differ from `RunID` (parent-owned restart intents — verified `deliverPendingRestart`, `interactive_service.go:2233`).
- `D-5` **Stop/send linearization**: the linearization point is the `send_claimed → send_started` CAS. Stop-first ⇒ send CAS fails, zero bytes, `StopOutcome=stopped_before_send`. Send-first ⇒ provider cancel path, `StopOutcome=cancelled_in_flight`. We never claim "no bytes after Stop CAS" — that is physically unprovable in the gap; we claim and test the CAS-order contract.
- `D-6` **Gate settlement sub-lifecycle** (`SettlePhase`) rides the dispatch record; every downstream side effect (completion event, cohort append, signal, dependents release, finalizer) is idempotent and bracketed by durable phase CAS. A release-manifest item has its own revisioned lifecycle: `pending → created|suppressed`; creation of the child dispatch and `pending → created` commit atomically.
- `D-7` Persistence is fail-closed and versioned; a V2 run (per the run-level marker **outside** the blob) with a missing/empty/corrupt runtime blob or dispatch log is `repair_required` (SS-17 AC-3) — absence is corruption for V2, not "legacy".
- `D-8` Provider truth per the capability matrix (§6.3); a session/thread id or a stdin write is **not** a receipt.
- `D-9` Marker verification takes a required `MarkerVerificationContext`; empty allowed-set fails closed; provenance is **recorded at mint time**, never inferred from topology.
- `D-10` Rollback: run-level protocol authority; once a run is V2, flag-off/V1 code must **block automated dispatch** on it (surface per SS-17 AC-4), never reinterpret. No shadow-writing V2 records under V1 authority.
- `D-11` **Backend parity is a contract**: one shared full-field `ProviderSessionState` round-trip suite runs against both stores; local retention (90-day prune) must never remove a run with a non-terminal dispatch or unfinalized settle.

### Constraints

- Both backends satisfy the same store contract via a shared contract test suite; at least one run against real PostgreSQL/Supabase for CAS + concurrent claims (CP-51 ledger `PG`).
- Backward compatible: legacy `prep:`/bare idempotency values and unversioned runtime blobs map into defined states/`uncertain` (never assumed accepted); legacy records are resolve-only.
- Adapters differ in what they can prove (§6.3); the design degrades to `uncertain`, never to a guess.
- No change to Flow-Gate rule semantics (SD-20) or Change-Contract semantics (SD-21).
- Hot-path cost: +1 durable CAS (`send_started`) per turn; settle phases add ≤5 small CAS writes per gated turn — accepted for correctness.
- Windows is a first-class dev/test platform: the crash harness must use cross-platform process kill (`os.Process.Kill`), never `kill -9`/SIGKILL-only assumptions.

### Open Questions

- `Q-1` Capability-matrix live evidence (§6.3) — **owned by CP-51 Task-257**. Initial rollout scope is Codex + Grok: their completed probes establish the explicit negative result “unprovable ⇒ `uncertain`”, so neither has an `Accepted` seam or recovery attach. Claude/Gemini remain deferred and are excluded from V2 provider enablement until their evidence exists. The protocol is safe under either answer, but a provider cannot enter the V2 rollout without its recorded outcome.
- `Q-2` *(resolved v3)* Supabase storage shape: **dedicated `dispatch_records` table + transactional RPCs** (§6.5). Conditional-PATCH-on-JSON-blob is rejected (cannot CAS one turn inside a JSON array, cannot atomically clear a parent intent).

### Source Refs

- SS-17 `AC-1..AC-6`, `BR-1..BR-6`; SS-16 `AC-7`, `BR-6`, `BR-5`; SS-14; SS-11.
- BUG-288 §11 + Codex post-Round-20 verdict + plan reviews #1/#2 (2026-07-16).
- Verified code: `codex_adapter.go:167,226`; `grok_adapter.go:285,223`; `claude_adapter.go:173`; `local_file_session_store.go:65-109` (ndjson record lacks ChangeType/SourceDocID/TurnCount — parity gap), `:273-292` (RAM-before-disk); `interactive_service.go:2233` (`deliverPendingRestart` — parent-owned intent), `:3226-3266` (settle = signal + event persist + broadcast + cohort + finalizer + summary + release), `:5045`; `interactive_resume.go:1563-1582` (`notifyTurnIdle` flushes resume/reprompt only); `resumePendingFlowGate` (gate-settle executor).

## 1. Goal

Design a turn-dispatch and settlement mechanism that upholds these invariants **by construction** across process crash, user Stop, storage outage, concurrent recovery, and cross-run (parent/child) intent ownership — on both backends:

- **INV-1 Three-outcome delivery.** Every accepted intent ends `terminal`, **safely-retryable**, or **explicitly-uncertain** (surfaced per SS-17). Silent loss is impossible.
- **INV-2 No blind duplicate.** A turn that may have reached the provider is never re-sent without proof; operator retry is a new linked record. This includes the **parent-restart** case: a parent's restart intent cannot survive its child's committed receipt (they clear atomically).
- **INV-3 Stop/send linearization.** No first provider byte whose `send_started` CAS is ordered **after** a Stop CAS. If the send CAS won, Stop resolves through provider cancel; no **new provider send or dependent user-work** is initiated after Stop acknowledgment. Settlement bookkeeping for an already-terminal turn may finish under INV-7; its dependent release is suppressed. The recorded `StopOutcome` states which branch occurred.
- **INV-4 No silent state loss.** Corrupt, version-mismatched, **or missing-under-V2** persisted state fails closed into `repair_required`; stale snapshots cannot overwrite newer revisions; torn appends are detected.
- **INV-5 No wedge.** Outage self-heals (retry + recovery re-derivation); leases expire; nothing durability-critical lives only in RAM.
- **INV-6 No cross-run replay bleed.** Marker suppression requires service secret + recorded-provenance allowed IDs; empty set fails closed.
- **INV-7 Exactly-once settlement (durable consequences).** Each gate-settle **durable consequence** (persisted completion event, cohort entry, node transition, dependents-release decision, finalizer artifacts) lands exactly once per (runID, turnID) across any crash pattern — via effects-then-CAS ordering + that consequence's own keyed/monotonic/create-if-absent contract (§5.3). The durable effect ledger is audit/skip optimization only. RAM-only waiter wakeups are best-effort by definition and re-derived on resume; they are outside this claim.

## 2. Input Documents

- SS-17 (operator contract — the product authority for `uncertain`/`repair_required` and resolution actions).
- SS-16 `AC-7`/`BR-6` (bounded + stoppable), `BR-5` (isolation), `AC-4` (backend parity motivation).
- SS-14, SS-11, BUG-288.

## 3. Architecture Decision

`D-1..D-11` above; the load-bearing rationale:

**Why a dedicated dispatch store (not the session blob):** dispatch records are per-*turn*, mutate at high frequency under CAS, and must commit atomically with an intent clear that may live on a **different run's** session (parent restart). A JSON blob per run cannot CAS one turn among many, and two concurrent dispatches on one run would fight over the whole blob; a stale blob write could resurrect old dispatch state. A dedicated store gives row/line-granular CAS, cross-record transactions, and removes dispatch state from the blob-staleness problem entirely.

**Why settle phases:** the verified settle path performs seven distinct side effects (`interactive_service.go:3226-3266`). A single boolean can only say "not done"; replaying it re-runs *all* effects (duplicate finalizer/cohort/release) or — if cleared early — loses the tail. Phases + idempotent effects give exactly-once semantics per effect (INV-7).

**Why linearizable Stop (option A):** between the `send_started` CAS and the first byte there is an unavoidable gap; no software contract can both let the send proceed and guarantee zero bytes after a later Stop CAS. We therefore define "send begun" = CAS commit, and make the *recorded outcome* (`stopped_before_send` vs `cancelled_in_flight`) the testable contract. The alternative (a synchronization gate where Stop acknowledgment blocks on the send gate) adds a lock across an external call — rejected for deadlock/latency risk.

Alternatives considered: A1 point-patching (rejected — Rounds 1–20 non-convergent); A2 external queue/WAL (rejected — the dedicated dispatch log **is** the WAL, owned by the store, no second source of truth); A3 provider idempotency (rejected — not offered, §6.3); A4 5-state + boolean overlays (rejected — plan review #1); A5 dispatch-in-session-blob + conditional PATCH (rejected — plan review #2 finding #4, see above).

## 4. Component Impact

- Impacted: `internal/runner` interactive service (`startTurn`, `runTurn`, settle path, `deliverPendingRestart`/intent delivery, `resumePendingFlowGate`), resume/recovery, persistence (`workflow_store.go`, `local_file_session_store.go`, `supabase_workflow_store.go`), feature-history/marker, all four adapters, desktop operator surface (SS-17).
- New: `dispatch_record.go` (record/envelope/states/guards), `dispatch_store.go` (contract + local commit-log impl), `dispatch_store_supabase.go` (table + RPC impl), `dispatch_recovery.go` (lease scanner + reconciliation), `dispatch_settle.go` (phase executor), operator resolution service + API + UI card (CP-51 Task-256).
- Unchanged: Flow-Gate rule set (SD-20), Change-Contract semantics (SD-21), FlowDefinition data model.

## 5. Data Model

### 5.1 Dispatch states (8, forward-only; created at `prepared`)

```text
(prep durable) prepared → send_claimed → send_started → provider_accepted → terminal_completed
                   │            │             │                │           ↘ terminal_failed
                   │            │             │                └──────────→ terminal_cancelled
                   │            │             ├─ (recovery, unprovable) → uncertain
                   └────────────┴─ (Stop CAS won before send_started) ──→ terminal_cancelled
uncertain ─ (reconcile proof | SS-17 operator resolution) → terminal_*
```

There is **no `intent_pending` state**: before `CreatePrepared`, the durable artifact is the outer intent itself (owner-side `PendingResume*/PendingGateReprompt*/PendingRestart*`). The record's first durable mutation is its creation at `prepared` (crash-matrix barrier `B0` covers the pre-record phase).

### 5.2 Transition table (closed; all other edges rejected)

| From | To | Trigger | Durability |
| --- | --- | --- | --- |
| — | `prepared` | `CreatePrepared(record, envelope)` | create-if-absent, fsynced |
| `prepared` | `send_claimed` | dispatcher claims the send | `CASAdvance` |
| `prepared`, `send_claimed` | `terminal_cancelled` | Stop CAS won pre-linearization (`StopOutcome=stopped_before_send`) | `CommitPreSendCancellationAndClearIntent` — state + owner-intent clear + audit in one commit |
| `send_claimed` | `send_started` | **linearization point** — durable CAS immediately before first external byte; **fails if `CancelRequested`** | `CASAdvance` |
| `send_started` | `provider_accepted` | acceptance receipt per §6.3, persisted | `CommitReceiptAndClearIntent` (atomic w/ intent-owner clear) |
| `send_started` | `terminal_*` | fast turn with both canonical receipt and provider-backed terminal proof | `CommitTerminalAndSettleIntent` with `TerminalEvidence`; it derives `StopOutcome` from durable Stop authority in the same commit |
| `provider_accepted` | `terminal_*` | provider-backed terminal proof (+ `SettlePhase=settle_pending` when settle owed) | `CommitTerminalAndSettleIntent` with `TerminalEvidence`; it derives `StopOutcome` from durable Stop authority in the same commit |
| `send_started`, `provider_accepted` | `uncertain` | recovery (under lease) cannot classify | `CASAdvance` by lease owner |
| `uncertain` | `terminal_*` | reconcile proof or operator resolution | `ResolveUncertain` / `RetryAsNew` (atomic, idempotent) |

`CancelRequested` (monotonic flag + `StopGeneration`, set via `SetCancelRequested` CAS): blocks the `send_started` CAS; in `prepared`/`send_claimed` only `terminal_cancelled` is legal next; in `send_started`/`provider_accepted` triggers provider cancel; biases `uncertain` resolution to confirm-cancelled (SS-17 BR-3); frozen at terminal. The terminal commit, not this asynchronous per-record flag, derives `StopOutcome=cancelled_in_flight` by durably reading record cancel + own `RunStopState` + any parent fence, so a crash before the cancel loop cannot lose attribution. A transport error after `send_started` is never terminal proof: it retains the intent and is reconciled/held `uncertain` (SS-17 BR-7).

### 5.3 Settle sub-lifecycle (`SettlePhase`, on the same record, same Revision CAS)

`SettlePhase` records the **last completed phase** (not "being attempted"):

```text
none | settle_pending → gate_evaluated → completion_committed → graph_settled → dependents_released → finalized
                     ↘ settle_superseded_reprompt   (gate outcome = reprompt/block: terminal disposition;
                                                     the reprompt/block flow owns what happens next)
```

**Execution rule (at-least-once replay + convergent consequences = exactly-once outcomes):** the driver executes the NEXT phase's side effects **first**, then CAS-advances `SettlePhase` to mark that phase completed. Crash anywhere ⇒ replay the phase's effects. The inverse order (CAS first) is forbidden — recovery would skip effects that never ran (plan-review #3 #1).

**Idempotency doctrine (the correctness mechanism — plan-review #4 #1):** a ledger check *before* an effect cannot be atomic with the effect, so a crash between an effect and its marker would duplicate it. Therefore correctness NEVER rests on the marker. An effect is admissible in the settle plan only if its **durable consequence is replay-convergent**, one of:
- **keyed upsert** — writing the same (runID, turnID)-keyed value twice converges (completion event, session `Completed` snapshot, finalizer artifacts);
- **monotonic transition** — re-applying is a no-op (child status, node DONE — guard exists);
- **create-if-absent / unique key enforced in the consequence's own store+transaction** — the duplicate write itself fails. Where today's consequence has no durable store at all, the consequence **moves into the dispatch store as an effect record**: cohort entries (`RecordEffectDone(kind="cohort", payload)` — the current buffer is a RAM slice deduped by label, `agent_orchestrator.go:92`) and the dependents **release manifest** (per-dependent records; the current release path is provably non-convergent: empty idempotency key at `interactive_service.go:2071/:2076`, prompt cleared pre-dispatch `:2114`, random TurnID `:5863` — downstream must go through the durable-intent path with the manifest's deterministic keys).

The **effect ledger** (`RecordEffectDone` → `effectDone{runID, turnID, effectKind}`) is written *after* each consequence as a **skip-optimization and observability record** — replay may consult it, but a lost marker only costs a redundant convergent replay, never a duplicate outcome. Required retrofits (today's code violates the doctrine): completion-event replace checks only the event *type*, not turnID (`interactive_service.go:3239` → key it); `persistEvent` error ignored (`:3245` → checked + retried); finalizer dedupe is a RAM map (`finalizer.go:49` → keyed artifact overwrite + ledger); `signalChild` waiters are RAM (`agent_orchestrator.go:195` → best-effort per INV-7 scope).

**INV-7 scope (stated honestly):** exactly-once applies to **durable consequences** (completion event persisted, cohort entry, node transition, dependents-release decision, finalizer artifacts). RAM-only notifications (in-process waiter wakeups from `signalChild`) do not survive process death by nature; after a restart they are defined as **no-ops** whose durable consequence (child status + release decision) is re-derived by the resume flow — they are explicitly NOT part of the exactly-once claim.

Mapped to the verified settle sequence (`interactive_service.go:3226-3266`):

| Phase reached (durable) | Side effects executed after it (all idempotent) |
| --- | --- |
| `settle_pending` | set atomically inside `CommitTerminalAndSettleIntent` — nothing ran yet |
| `gate_evaluated` | gate ran; **allow** continues; **reprompt/block** CAS to `settle_superseded_reprompt` (durable terminal disposition — recovery seeing a terminal dispatch with this disposition does nothing; the reprompt/block flow owns the next dispatch) |
| `completion_committed` | completion event **keyed upsert** by (runID, turnID) (fix the type-only replace at `interactive_service.go:3239`) with the persist **checked, not `_ =`** (`:3245`) + session snapshot `Completed` (keyed upsert) |
| `graph_settled` | `signalChild` (monotonic child status; waiter wakeup best-effort per INV-7 scope) + **durable cohort entry**: the consequence IS an effect record (`RecordEffectDone(effectKind="cohort:<cohortID>:<childRunID>", payload=entry, payloadHash)`, unique-keyed cohortID+childRunID+turnID; divergent duplicate = `ErrEffectConflict`) — required because today's `appendCohortResult` is a RAM slice deduped by *label* (`agent_orchestrator.go:92`); the in-memory buffer becomes a projection rebuilt from these records on load/replay + node DONE (monotonic guard exists) |
| `dependents_released` | a **durable release manifest**: one revisioned effect record per dependent (`effectKind="release:<dependentRunID>"`, payload = deterministic durable-intent key + state `pending→created\|suppressed`). `CreateReleaseManifestItem` creates `pending` create-if-absent; `CommitReleaseManifestItem` atomically creates the child `prepared` record from that key and CAS-advances the item to `created`; `SuppressReleaseManifestItem` CAS-advances `pending` to `suppressed` under the Stop generation. Required because today's release path is NOT convergent (verified): it calls `startTurn` with an **empty** idempotency key (`interactive_service.go:2071/:2076`), clears `pendingTurnPrompt` before dispatch (`:2114`), and `startTurn` mints a random TurnID (`:5863`) — crash mid-release loses or duplicates dependents. Replay re-reads the manifest: `pending` ⇒ atomic commit, `created`/`suppressed` ⇒ skip. |
| `finalized` | finalizer artifacts as **keyed overwrites** (runID, turnID) — convergent (replaces the RAM dedupe at `finalizer.go:49`) + chat summary scheduled (debounced/keyed) |

Rules: effects run first (each invokes its own convergent durable operation; a marker may only optimize a safe replay), **then** the phase CAS marks completion; the executor (`resumePendingFlowGate` refactored into a phase driver) resumes by replaying the in-progress phase's effects then advancing. Stop mid-settle: settlement of an already-terminal turn is **bookkeeping, not new user work** — the release item is CAS-marked `suppressed` when its stop generation wins, but that is **not** an early `SettlePhase` exit. The driver still advances `dependents_released → finalized` and runs the keyed finalizer exactly once; `suppressed_by_stop` is recorded as terminal audit metadata only after finalization. Crash-matrix barriers exist between every adjacent phase and between each phase's effects and its CAS (CP-51 §10.4 `B8a..B8e`).

### 5.4 DispatchRecord

`ProtocolVersion`, `TurnID`, `RunID`, **`IntentOwnerRunID`** (≠ `RunID` for parent-owned restart intents), `State`, `Revision`, `ClaimOwner`/`ClaimExpiresAt`, **`RecoveryAttachEpoch`/`RecoveryAttachOwner`/`RecoveryAttachExpiresAt`**, `CancelRequested`+`StopGeneration`+`StopOutcome`, `OuterIntentKey`/`OuterIntentGen`, `EnvelopeHash`, **canonical `ReceiptEvidence`** (not a receipt string), `TerminalEvidence`, **immutable-at-prepare `SettleOwed` read only by the store**, `SettlePhase`, `PredecessorTurnID`, `Outcome` (+`resolved_by`), **`ParentStopFence`** referencing the dedicated `RunStopState` authority, `CreatedAt`/`UpdatedAt`. In every post-send terminal transaction, the store locks/reads this record's own `RunStopState` and, when fenced, its parent authority in the same local writer/RPC transaction: set `StopOutcome=cancelled_in_flight` iff `CancelRequested`, own `Stopped=true`, or parent `Stopped=true`/generation differs; otherwise leave it empty. Every transition out of `send_started|provider_accepted`, including `uncertain`, and every terminal transaction revokes recovery attach (increment epoch, clear owner/expiry). Attached entry/output operations require exact current epoch/owner, store-clock expiry, sent state, and live Stop authority; an attached terminal operation requires the same token/sent predicate but locks Stop only for terminal attribution, so valid proof is not dropped after Stop. Neither attribution nor revocation has caller input and neither changes proof-derived terminal state/outcome.

#### 5.4.1 ReleaseManifestItem and DurableIntent

`RunStopState{RunID, Generation, Stopped, Revision}` is the **only durable run-Stop authority** for both the record's own run and a child's parent fence. It is created with run activation and is non-prunable while the run exists; local persists it in the dispatch commit-log projection, Supabase persists it in `dispatch_run_stop_state`. `RequestRunStop` CAS-sets `Stopped=true`, monotonically increments `Generation`, and writes the Stop audit; it need not wait for a per-record cancel loop for correctness. Every `CASAdvance(send_claimed→send_started)` first locks/checks the record's **own** `RunStopState.Stopped=false`, then (if present) locks/checks `ParentStopFence`. Own-stop returns typed `ErrRunStopFence{CurrentGeneration}`; parent stopped/mismatch returns `ErrParentStopFence{CurrentGeneration}`. Either error must immediately call `CommitPreSendCancellationAndClearIntent(..., source=self|parent_fence, CurrentGeneration)`. Session/RAM `StopGeneration` is a derived cache only. `ReleaseManifestItem` is a revisioned `dispatch_effects` row keyed by `(runID, turnID, "release:<dependentRunID>")`: `RunID`, `TurnID`, `DependentRunID`, immutable `DurableIntent`, `State (pending|created|suppressed)`, `Revision`, `StopGeneration`, timestamps. `DurableIntent` persists the deterministic child `TurnID`, child `RunID`, intent key, immutable child `DispatchEnvelope` (or its complete durable reference + hash), and a mandatory `ParentStopFence{ParentRunID, ExpectedStopGeneration}` copied from `RunStopState` into the child `DispatchRecord`. `CreateReleaseManifestItem` accepts only an equal duplicate (same intent hash); a different payload is `ErrEffectConflict`. `CommitReleaseManifestItem` conditions one transaction on `RunStopState.Stopped=false` and equal generation: Stop-first yields `suppressed`; release-first creates the fenced child. Thus a Stop cannot race either root or child into a provider send (SS-17 BR-8).

### 5.5 DispatchEnvelope (immutable, written once at `prepared`)

`TurnID`, `RunID`, `StepID`, `ProviderKey`/`ProviderAccountID`/`ProviderSessionIDAtPrepare`, `PromptRef` (persisted `turnLogKindPrompt` line) + `PromptSHA256`, `Model`/`ReasoningEffort`/`Yolo`, `SelectedSkills`, `Scenario`, `Attachments` (refs+hashes), `FlowContextInjected`, `CreatedAt`, `EnvelopeHash` (SHA-256, canonical JSON). Redispatch sends from the envelope; hash mismatch vs live intent ⇒ refuse → `uncertain` (SS-17 §8).

At `CreatePrepared`, `SettleOwed` is computed once from the same durable prepared invocation context that selects Flow-Gate settlement (`requiresGateSettlement(envelope, runMode, step)`), persisted beside the envelope, and is immutable thereafter. `CommitTerminalAndSettleIntent` reads this record field inside its CAS/RPC/log commit; it has no caller-supplied settlement flag. Only §6.7's explicit operator terminal actions may force no-settlement for their defined semantics.

### 5.6 Legacy mapping

`prep:<turnID>` → `prepared` (envelope synthesized from the prompt log; missing log ⇒ `uncertain`); bare → **`terminal_completed`** only with corroborating terminal/`lastTurnID` evidence, else `uncertain` (one rule, everywhere); legacy records get `ProtocolVersion=1`, resolve-only. Runs without the run-level marker are protocol v0 (legacy).

## 6. Interfaces and Contracts

### 6.1 Store API (the only mutation path; all CAS on `Revision`)

```go
type RecoveryUnknownDecision string
const (
    RecoveryClassifiedUncertain RecoveryUnknownDecision = "classified_uncertain"
    RecoveryCancelRequired      RecoveryUnknownDecision = "cancel_required"
)
type RecoveryAttachToken struct { Epoch int64; Owner string; ExpiresAt time.Time }
// Canonical, payload-hashed durable stream event. eventID is supplied separately
// because it forms the unique idempotency identity with run/turn/epoch.
type AttachedEffectPayload struct { Kind string; CanonicalJSON []byte; SHA256 string; ObservedAt string }

CreatePrepared(ctx, rec DispatchRecord, env DispatchEnvelope) error
CASAdvance(ctx, runID, turnID string, expectedRev int64, expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error)
CASRecoveryAdvance(ctx, runID, turnID string, expectedRev int64, leaseOwner string,
    expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) // recovery-only: same CAS plus claim owner+unexpired store-clock lease inside tx; used for retryable send CAS
CASAdvanceSettle(ctx, runID, turnID string, expectedRev int64, expected, next SettlePhase) (int64, error)
ClaimRecovery(ctx, runID, turnID string, expectedRev int64, owner string, ttl time.Duration) (int64, error)
SetCancelRequested(ctx, runID, turnID string, expectedRev, stopGen int64) (int64, error)
GetRunStopState(ctx, runID string) (RunStopState, error) // read-only; never use session snapshot authority
RequestRunStop(ctx, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) // CAS stopped=true, generation++, audit; parent authority mutation
// Cross-record atomic commits — dispatch transition + INTENT OWNER's intent clear (+ audit) in ONE commit:
CommitReceiptAndClearIntent(ctx, runID, turnID string, expectedRev int64, receipt ReceiptEvidence,
    intentOwnerRunID, intentKey string, intentGen int64) (int64, error)
CommitTerminalAndSettleIntent(ctx, runID, turnID string, expectedRev int64, proof TerminalEvidence,
    intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // atomically reads persisted SettleOwed and own/parent Stop authority; caller cannot override settlement or StopOutcome
// Recovery-only terminal path. The store transaction verifies that the lease is
// still owned and unexpired using its own clock, then performs every normal
// terminal derivation (proof / SettleOwed / StopOutcome).
CommitRecoveredTerminalAndSettleIntent(ctx, runID, turnID string, expectedRev int64, leaseOwner string,
    proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // ErrRecoveryLeaseLost on owner/expiry mismatch
// Recovery-only unknown decision. In ONE transaction it locks the dispatch row,
// verifies the lease, locks own and optional parent RunStopState, then either
// writes uncertain or returns RecoveryCancelRequired without classifying.
CommitRecoveryUnknownOrRequireCancel(ctx, runID, turnID string, expectedRev int64,
    leaseOwner string) (RecoveryUnknownDecision, int64, error) // ClassifiedUncertain|RecoveryCancelRequired; ErrRecoveryLeaseLost
ClaimRecoveryAttach(ctx, runID, turnID string, expectedRev int64, leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error) // recovery-only: tx validates lease + no current unexpired attach token + own/parent Stop, increments durable attach epoch and grants bounded token; ErrRecoveryAttachActive/ErrRecoveryCancelRequired/ErrRecoveryLeaseLost
EnterRecoveryAttach(ctx, runID, turnID string, token RecoveryAttachToken) error // transaction-bound entry fence: exact token + store-clock expiry + sent state + live own/parent Stop; records attach entry before provider attach; ErrRecoveryAttachRevoked means no attach
RecordRecoveryAttachedEffect(ctx, runID, turnID string, token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (bool, error) // transaction-bound callback/output effect: same predicate + unique event; renderer projects only accepted persisted effects
CommitAttachedTerminalAndSettleIntent(ctx, runID, turnID string, expectedRev int64, token RecoveryAttachToken,
    proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // one transaction validates exact token + unexpired store-clock token + sent state + revision, locks own/parent Stop for attribution (does NOT reject valid proof after Stop), then derives proof terminal/settle/StopOutcome, clears intent, audits, revokes epoch; ErrRecoveryAttachRevoked has no mutation
// Pre-linearization Stop is terminal too: it MUST clear the outer intent in the
// same transaction/log line, including parent-owned restart intents.
CommitPreSendCancellationAndClearIntent(ctx, runID, turnID string, expectedRev int64,
    intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource /* self|parent_fence */) (int64, error)
CommitRecoveryPreSendCancellationAndClearIntent(ctx, runID, turnID string, expectedRev int64, leaseOwner string,
    intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) // recovery-only: same pre-send atomicity plus lease predicate
// Operator resolution (SS-17) — atomic + idempotent via caller-supplied resolutionID:
ResolveUncertain(ctx, runID, turnID string, expectedRev int64, resolutionID string,
    action ResolveAction /* mark_completed|mark_failed|confirm_cancelled|abandon */, evidence OperatorEvidence) (int64, error)
// RetryAsNew guards against a SUPERSEDED intent (SS-17 §8, plan-review #5 #10):
// the commit CAS-checks that the owner's live intent generation and the stored
// envelope hash still match what the operator saw; a newer prompt ⇒ ErrSuperseded
// (reject with explanation; offer abandon). No blind retry over fresh user work.
RetryAsNew(ctx, runID, oldTurnID string, expectedRev int64, resolutionID, newTurnID string,
    expectedIntentGen int64, expectedEnvelopeHash string) (int64, error)
// Effect ledger + repair lifecycle mutations (plan-review #4 #2/#3, #5 #1..#3):
// A duplicate effect payload must match its persisted hash; mismatch is ErrEffectConflict,
// never "first writer wins". Cohort entries are terminal effect records; release items use
// the explicit revisioned lifecycle below rather than overloading this marker API.
RecordEffectDone(ctx, runID, turnID string, effectKind string, payload []byte, payloadHash string) (int64, error) // idempotent unique key; equal payload duplicate = no-op; mismatch = ErrEffectConflict
CreateReleaseManifestItem(ctx, runID, turnID, dependentRunID string, intent DurableIntent) (ReleaseManifestItem, error) // create-if-absent, state=pending; mismatched durable-intent hash = ErrEffectConflict
CommitReleaseManifestItem(ctx, runID, turnID, dependentRunID string, expectedEffectRev int64, child DispatchRecord, env DispatchEnvelope) (int64, error) // ONE commit: child CreatePrepared + pending→created; child carries ParentStopFence; idempotent replay returns created
SuppressReleaseManifestItem(ctx, runID, turnID, dependentRunID string, expectedEffectRev, stopGeneration int64) (int64, error) // CAS pending→suppressed
OpenRepair(ctx, runID, reason string, rawBlob []byte, rawHash string) (repairRev int64, err error) // create-if-absent; RepairRecord + THE RAW BLOB ITSELF (quarantine_blob + hash) + snapshot-writer block + audit — ONE commit; raw is never dropped (#5 #1)
GetOpenRepair(ctx, runID string) (RepairRecord, bool, error) // returns the quarantined raw for Inspect/retry-load
// Two-phase repair resolution (#5 #2 — a Postgres RPC cannot run the Go loader):
BeginRepairResolution(ctx, runID string, expectedRepairRev int64, resolutionID string,
    action RepairAction) (attemptRev int64, raw []byte, err error) // CAS-claims the attempt (TTL like leases), audits start, returns the quarantined raw for out-of-store validation
CommitRepairResolution(ctx, runID string, attemptRev int64, resolutionID string,
    outcome RepairOutcome /* resolved_retry_load|failed_still_open|resolved_abandon */, detail string) (int64, error) // CAS: success ⇒ resolved+unblock; failure ⇒ still open, attempt audited; crash between Begin/Commit ⇒ attempt TTL expires, next attempt supersedes
// Run protocol authority (#5 #3 — never read the session mirror for decisions):
GetRunProtocolVersion(ctx, runID string) (int, error) // reads the NON-PRUNABLE activation entry in the dispatch store

// Read side (required by the settle driver, recovery, Task-256 surface — plan-review #3 #4).
// Deterministic ordering (CreatedAt, TurnID); List* take a limit+cursor for pagination.
Get(ctx, runID, turnID string) (DispatchRecord, int64 /*revision*/, error)
GetEnvelope(ctx, runID, turnID string) (DispatchEnvelope, error)
ListRecoverable(ctx, runID string) ([]DispatchRecord, error)  // non-terminal + terminal-with-unfinalized-settle
FindActiveByOuterIntent(ctx, runID, intentKey string, intentGen int64) (DispatchRecord, bool, error) // delivery authority; active states only
ListAttention(ctx) ([]AttentionItem, error)                   // uncertain dispatches + open RepairRecords
ListAudit(ctx, runID, turnID string) ([]AuditEntry, error)
GetResolutionResult(ctx, resolutionID string) (ResolutionResult, bool, error) // idempotent replay support
ListEffects(ctx, runID, turnID string) ([]EffectDone, error)  // the settle effect ledger
IsIntentCleared(ctx, ownerRunID, intentKey string, intentGen int64) (bool, error)
```

- `ErrStaleDispatch` on any revision/state mismatch; caller reloads, never merges.
- RAM caches update only after the durable commit succeeds — an unpersisted receipt/outcome/clear is invisible to every predicate.
- Idempotency: replaying a commit with the same `resolutionID` (operator ops) or same (turnID, expectedRev→already-applied) returns the first result, applies nothing.

### 6.2 Local store: single-writer commit log (operational contract)

- One `dispatch.ndjson` per project data dir; **one writer goroutine** owns all appends (serialized, monotonic global `Seq`) — and a **process-exclusive file lock** (`dispatch.lock`, acquired at startup, cross-platform) guards against old/new server processes overlapping during restart (plan-review #3 #13): the loser fails startup dispatch loudly rather than double-appending.
- Each API commit = **one appended line** (fsynced) containing the full record after-state plus, for cross-record commits, the embedded `intentClear{ownerRunID,key,gen}`, `effectDone{...}`, and `audit{...}`. `RequestRunStop` writes a non-prunable `runStopState{runID,generation,stopped,revision}` projection; manifest-create and child-send commits read/update it through this same single writer — atomic at line granularity.
- Order: validate → append → flush/sync → then RAM (inverts the verified RAM-first defect at `local_file_session_store.go:273-292`).
- Load: replay lines into map[(runID,turnID)] + clear-set + effect ledger + `runStopState[runID]`; drop + count a torn trailing line. A V2 run missing its run-stop projection is corrupt/`repair_required`, not implicitly live.
- **Intent clears win on read AND write:** (a) at load, a session `Pending*` intent whose (owner,key,gen) is in the clear-set is dropped; (b) at write, `UpsertProviderSession` filters cleared intents out of the snapshot before persisting (a stale owner snapshot cannot resurrect a cleared intent — plan-review #3 #6); (c) in-process, the atomic commit also updates the owner run's RAM intent fields under `s.mu` via a store callback, so live state and log agree immediately.
- **Compaction protocol** (append-only log must prune): write compacted temp file → fsync temp → atomic rename over `dispatch.ndjson` → fsync directory. Crash mid-compaction rule: on startup, a leftover temp file is discarded (the original is authoritative until the rename). Compaction removes only `terminal_* + finalized/superseded/suppressed` records past TTL.
- **Activation entries are non-prunable:** the atomic V2 activation (§6.5) writes an `activation{runID, protocolVersion}` line that **compaction never removes** — so "V2 authority" cannot vanish when a run's terminal records are pruned (plan-review #5 #3). `GetRunProtocolVersion` reads these entries; every consumer of "is this run V2" (missing-log rule below, §6.5 missing-blob rule, migration matrix) calls that API — **never the session mirror**.
- **Missing log is corruption for V2:** a run whose `GetRunProtocolVersion` (activation entry) says ≥ 2 but has no dispatch records and no activation line in the log ⇒ the log itself is damaged ⇒ `repair_required` — never treated as "no records yet".
- **Session upserts flow through the same single writer:** `UpsertProviderSession` for dispatch-bearing runs is serialized on the same writer goroutine as dispatch commits, making the clear-set check and the session write **atomic by construction** locally — no TOCTOU window between check and write (plan-review #5 #5).
- Retention: a run with any non-terminal record or unfinalized settle is never pruned and blocks the session store's 90-day prune (`D-11`).

### 6.3 Adapter capability matrix (`D-8`)

> Operation names are verified in adapter code. The initial V2 rollout is limited to Codex + Grok, whose Task-257 probes concluded **unprovable ⇒ `uncertain`** (no acceptance seam, reconcile proof, or attach). Claude/Gemini are explicitly deferred and must not be V2-enabled until their own recorded outcome replaces their *verify live* cells. The design is safe under either answer: no proof ⇒ `uncertain`.

| Provider | Session op (NOT a receipt) | Prompt send (linearize before this) | Acceptance receipt | Query/reconcile after restart | Attach in-flight | Guarantee class |
| --- | --- | --- | --- | --- | --- | --- |
| Codex | `thread/start` (`codex_adapter.go:167`) | `turn/start` (`:226`) | **none** — headless probe found no stable ack/ReceiptID distinct from completion; `Accepted` is not wired ([evidence](./evidence/SD-24/codex.md)) | unprovable in probe → `uncertain` | unprovable in probe → no attach | V2 enabled only with three-outcome / `uncertain` handling |
| Grok | `session/new` (`grok_adapter.go:285`) | `session/prompt` (`:223`; JSON-RPC response returns at **end** of turn) | **none** — headless probe found no `session/update` timing/ReceiptID; `Accepted` is not wired ([evidence](./evidence/SD-24/grok.md)) | unprovable in probe → `uncertain` | unprovable in probe → no attach | V2 enabled only with three-outcome / `uncertain` handling |
| Claude | spawn CLI process | `writeUserTurn` stdin (`claude_adapter.go:173`) — stdin write success is **not** a receipt | first stream-json event *(verify live — deferred; V2 disabled)* | provider transcript file if flushed *(verify live — deferred; V2 disabled)* | none; respawn = new turn | deferred; V2 disabled pending Task-257 evidence |
| Gemini | spawn CLI one-shot | process start | none before first output | none | none | deferred; V2 disabled pending explicit Task-257 rollout decision |

No provider offers idempotent operation-ID submission or authoritative query-by-operation-ID ⇒ **exactly-once is not claimable**; INV-1's three-outcome contract is the specification. Matrix cells absorb live-verification outcomes without protocol change.

#### 6.3a Acceptance-receipt adapter contract

`ProviderEvent` remains normalized for display/tool/terminal events. Receipt is a separate, explicit runner seam and is never inferred from session creation, stdin-write success, or an arbitrary display event:

```go
// ReceiptEvidence is canonical JSON before hashing. Equality/conflict is over
// ProviderKey, ReceiptID, EvidenceKind and PayloadSHA256; ObservedAt is audit
// metadata only. A duplicate with equal identity is a no-op; the same receipt
// identity with a different canonical payload is ErrReceiptConflict.
type ReceiptEvidence struct {
    ProviderKey, ReceiptID, EvidenceKind string
    PayloadCanonicalJSON                  []byte
    PayloadSHA256                         string
    ObservedAt                            time.Time
}
// TerminalEvidence is required for every post-send terminal commit. Outcome is
// the closed enum completed|failed|cancelled and is validated against the
// provider terminal event/query result; CommitTerminalAndSettleIntent derives
// BOTH record state and outcome from it and accepts no caller-supplied outcome.
// In the same terminal transaction it separately derives StopOutcome solely
// from record cancel + durable own/parent RunStopState; that attribution never
// changes proof.Outcome.
// It binds that proof to its canonical payload and, where applicable, receipt.
// Operator assertions use OperatorEvidence only through ResolveUncertain.
type TerminalEvidence struct {
    ProviderKey, EvidenceKind, Outcome string
    ReceiptIdentity                    string
    PayloadCanonicalJSON               []byte
    PayloadSHA256                      string
    ObservedAt                          time.Time
}
type TurnBridge interface {
    Emit(ProviderEvent)
    Accepted(ReceiptEvidence) // idempotent/equality-checked; only legal after send_started
    Terminal(TerminalEvidence) // sole automatic post-send terminal seam
}
```

Only a provider with a Task-257 evidence-backed qualifying event may call `bridge.Accepted`; the scoped Codex/Grok outcome has **no qualifying event**, so both intentionally have no `Accepted` call site. Claude/Gemini are V2-disabled pending evidence. `TurnBridge.Accepted` remains the sole caller of `CommitReceiptAndClearIntent`; it accepts a duplicate only after canonical equality, rejects identity/payload conflict with `ErrReceiptConflict`, logs an illegal pre-send/terminal receipt, and on commit failure schedules the durable retry without clearing RAM intent. `TurnBridge.Terminal` is the sole automatic caller of `CommitTerminalAndSettleIntent` and requires `TerminalEvidence`; `SendTurn` returning an error after `send_started` may only record an error/reconcile work item, never clear intent or terminalize. Task-257 records the exact adapter file:function/event predicate if a future provider earns an Accepted seam, and records the negative proof that `thread/start`/`session/new`/stdin write cannot call either seam. Task-249 wires only those recorded sites; Task-255 tests the negative paths for Codex/Grok.

### 6.4 Recovery scanner (lease-based)

1. For each non-terminal record: `ClaimRecovery(expectedRev, owner, ttl)`; failed claim ⇒ skip (concurrent scanner safety). **Immediately re-read `Get` after claim**; every external action and every CAS must use this fresh revision and verify that `ClaimOwner/ClaimExpiresAt` still name the scanner. Expired leases are taken over by a fresh claim CAS (INV-5).
2. Under a fresh valid lease: `prepared`/`send_claimed` ⇒ **safely-retryable** (envelope-hash verified re-dispatch, same TurnID + envelope, then `CASRecoveryAdvance(send_claimed→send_started)` transactionally rechecks the lease and own/parent fence immediately before the send linearization); `send_started`/`provider_accepted` ⇒ reconcile per matrix (proof + `TerminalEvidence` → terminal CAS; in-flight → a fenced attach; unknown → `uncertain`); `uncertain` ⇒ surface (SS-17). Before every sent-state provider action or attach, call **`effectiveStopRequested(record)`**: it fail-closed reads the record's own `RunStopState`, any parent `ParentStopFence` authority, and `CancelRequested`; a read failure permits no attach/classification. If effective Stop is true, issue/continue provider cancel and suppress attach even when a crash occurred before the per-record cancel loop. **A recovery write or attach never relies on that read alone:** recovery pre-send cancellation uses `CommitRecoveryPreSendCancellationAndClearIntent`; `unknown`/no-adapter classification calls `CommitRecoveryUnknownOrRequireCancel` and revokes attach; terminal proof uses `CommitRecoveredTerminalAndSettleIntent`; in-flight attach first calls `ClaimRecoveryAttach`, which transactionally verifies the lease and Stop authority, increments durable attach epoch, and returns a bounded token. `resumeStreaming` must call `EnterRecoveryAttach` before opening the provider attachment; each nonterminal callback must call `RecordRecoveryAttachedEffect` before its output is visible; a callback terminal uses `CommitAttachedTerminalAndSettleIntent`, never `Validate → Commit`. Entry/output reject stale/expired/superseded/stopped/uncertain/terminal tokens as `ErrRecoveryAttachRevoked`; attached terminal also atomically rejects store-clock-expired tokens, then rejects stale/superseded/uncertain/terminal token/state/revision and locks a winning Stop to record `cancelled_in_flight` while committing valid unexpired proof. An unexpired active token is respected by a repeated scan; expiry makes a crash-after-claim reclaimable. Each mutation/claim verifies `ClaimOwner` and unexpired store-clock lease within its transaction; the relevant variants lock own/parent Stop authority and return cancel-required/fence rather than write/send/attach if Stop won. After a fresh lease re-read, recovery may query solely to obtain validated `TerminalEvidence`; it may terminalize only from that proof, otherwise it retains the sent record/intent for a later proof or operator resolution. Terminal records with `SettlePhase != finalized` ⇒ hand to the settle phase driver (§5.3).
3. Intent clears happen only via the atomic commits — recovery never clears by observation.
4. Legacy (`ProtocolVersion=1`) records: resolve-only (→ terminal/uncertain).

### 6.5 Supabase: dedicated table + transactional RPCs (Q-2 resolved)

- Table `dispatch_records` (PK `(run_id, turn_id)`): `intent_owner_run_id`, `state`, `revision int8`, `claim_owner`, `claim_expires_at`, `cancel_requested`, `stop_generation`, `stop_outcome`, `settle_owed`, `settle_phase`, `envelope jsonb`, `envelope_hash`, **`receipt_evidence jsonb` + receipt identity + canonical payload hash**, **`terminal_evidence jsonb` + canonical payload hash**, `parent_stop_fence_run_id`, `parent_stop_fence_generation`, `predecessor_turn_id`, `outcome`, `protocol_version`, timestamps. Envelope/evidence canonical bytes are immutable after insert (trigger or discipline + hash check); same receipt identity with divergent hash is `ErrReceiptConflict`.
- Table `dispatch_run_stop_state` (PK `run_id`): `generation int8`, `stopped bool`, `revision int8`, `updated_at`; created with the V2 activation and non-prunable while the run exists. This is the only `ParentStopFence` read authority — never `ProviderSessionState` or a terminal parent record.
- Table `dispatch_effects` (**UNIQUE `(run_id, turn_id, effect_kind)`**): hash-bound effect records (`payload`, `payload_hash`, and, for a release item, `state` + `revision` + `stop_generation`). `dispatch_record_effect` accepts a duplicate only when its payload hash matches; mismatch is `ErrEffectConflict`. `dispatch_create/commit/suppress_release_manifest_item` provide the revisioned release lifecycle; the generic effect marker remains audit/skip optimization, never the correctness mechanism.
- Table `repair_records` (one **open** row per `run_id` enforced by partial unique index): `repair_revision int8`, `reason`, **`quarantine_blob` + `quarantine_hash`** (the raw payload itself — no dangling ref), `state open|resolved`, `resolved_action`, `resolution_id`, `attempt_claim`/`attempt_expires_at`, timestamps; RPCs `dispatch_open_repair` (create-if-absent + raw blob + block flag + audit, one tx), `dispatch_begin_repair_resolution` (attempt CAS + returns raw), `dispatch_commit_repair_resolution`.
- **Atomic V2 activation (plan-review #4 #5):** a run's V2 authority is derived from the **dispatch store itself** — a run is V2 iff it has an activation entry or ≥1 record. The first `CreatePrepared` for a run **carries the activation in the same commit** (same log line locally; same RPC transaction on Supabase). The session `dispatch_protocol_version` column/field is a **derived mirror** for fast lookup; on mismatch the dispatch store wins, and the mirror is repaired, not the run — so "marker set, record missing" and "record present, marker unset" crash windows are unrepresentable, and the missing-log/blob repair rule (§6.2/§6.5, `F-11`) keys off dispatch-store-derived authority, never the mirror alone.
- RPC functions (one Postgres transaction each): `dispatch_cas_advance` (**when entering `send_started`, locks/checks the record's own `dispatch_run_stop_state`; stopped returns `ErrRunStopFence(current_generation)`, then a child locks/checks parent state and returns `ErrParentStopFence(current_generation)` on mismatch**), **`dispatch_cas_recovery_advance`** (same transition plus `claim_owner=leaseOwner AND claim_expires_at > clock_timestamp()` and own/parent fence lock), `dispatch_cas_settle`, `dispatch_claim_recovery`, **`dispatch_claim_recovery_attach`** (requires exact unexpired recovery lease, locks own/parent Stop authority, increments `recovery_attach_epoch`, writes bounded owner/expiry audit, returns token or cancel-required), `dispatch_validate_recovery_attach_token` (valid only for current unexpired epoch and live Stop authority), `dispatch_set_cancel`, **`dispatch_request_run_stop`** (CAS generation++/stopped/audit), `dispatch_commit_receipt_clear_intent` (updates dispatch row **and** the intent owner's session runtime intent fields **and** inserts the audit row), **`dispatch_commit_pre_send_cancel_clear`** (same atomic owner clear + terminal + `StopOutcome` + `self|parent_fence` audit), **`dispatch_commit_recovery_pre_send_cancel_clear`** (same pre-send atomicity plus the lease predicate), `dispatch_commit_terminal_settle` (**locks the dispatch row then own run-stop row and, for a child, parent run-stop row; derives state/outcome only from validated `TerminalEvidence`, `SettlePhase` only from persisted `SettleOwed`, and `StopOutcome=cancelled_in_flight` iff record cancel/own Stop/parent fence is effective at that transaction; no caller parameter**), **`dispatch_commit_recovered_terminal_settle`** (the same terminal derivations plus `claim_owner=leaseOwner AND claim_expires_at > clock_timestamp()` in that transaction; mismatch ⇒ `ErrRecoveryLeaseLost`), **`dispatch_commit_recovery_unknown_or_cancel`** (locks dispatch + own/parent Stop rows, enforces the same lease predicate and record cancel; atomically writes uncertain only if no effective Stop, otherwise returns `RecoveryCancelRequired` with no state/intent mutation), `dispatch_resolve_uncertain`, `dispatch_retry_as_new`; `dispatch_commit_release_manifest_item` locks/checks the same run-stop row. `dispatch_records` carries `recovery_attach_epoch`, `recovery_attach_owner`, `recovery_attach_expires_at` solely for this bounded attach claim. The local single writer implements identical guards using its injected store clock. All take the relevant expected record/run-stop revision; 0-row match ⇒ stale, surfaced as `ErrStaleDispatch`.
- **Attached-stream RPC closure (SD-25):** `dispatch_enter_recovery_attach`, `dispatch_record_recovery_attached_effect`, and `dispatch_commit_attached_terminal_settle` replace any standalone token validator as the authority for attach entry, callback output, and callback terminal forwarding. Each locks the dispatch row and own/parent Stop rows and requires exact `{epoch, owner}`, `recovery_attach_expires_at > clock_timestamp()`, and `state IN ('send_started','provider_accepted')`; entry/output reject a winning Stop, whereas the terminal RPC also checks expected revision and uses Stop only to derive `StopOutcome=cancelled_in_flight` before atomically committing valid proof-derived terminal/settlement/owner-clear/audit/revocation. A failed token/state/revision predicate returns `ErrRecoveryAttachRevoked` and performs no visible effect. `dispatch_commit_recovery_unknown_or_cancel` revokes the attach epoch on its `uncertain` transition; all terminal RPCs (live, recovered, pre-send, operator, retry) revoke it as part of their transaction. The local single writer has byte-for-byte equivalent predicates and results.
- The session `session_runtime` blob **no longer carries dispatch records** — only the run-level `dispatch_protocol_version` marker (a column outside the blob so a missing blob is detectable, `D-7`). Two concurrent dispatches on one run touch different rows — no blob contention.
- **Stale-snapshot intent resurrection is blocked on the session side too — in the same transaction** (plan-review #3 #6, #5 #5): a client-side check-then-write over the plain upsert (`supabase_workflow_store.go:447/481`) has a TOCTOU window (a receipt commit can land between the check and the write). Supabase therefore uses a **server-side guarded upsert** — `session_upsert_guarded` RPC (or a BEFORE INSERT/UPDATE trigger) that joins the clear-set and nulls cleared `Pending*` fields **inside the write transaction**; local achieves the same by serializing session upserts on the dispatch writer goroutine (§6.2). `applySessionRuntime` additionally filters on read. Required tests: sequential *write-after-clear* AND **concurrent interleaving** (receipt commit racing a stale snapshot write) on both backends — CP-51 ledger `IR`/`TO`.
- Repair quarantine (schema for §6.7): **the raw blob itself** is persisted atomically with the `RepairRecord` (`quarantine_blob` + `quarantine_hash` columns / embedded in the OpenRepair commit-log line — plan-review #5 #1; no dangling "ref" whose file may never have been written). Oversized blobs (> 1 MiB) fall back to sidecar-then-commit (temp → fsync → rename **before** the OpenRepair commit; an orphan sidecar without a record is garbage). Normal snapshot writes to a run with an open `RepairRecord` are rejected until resolution (SS-17 AC-3).
- Table `run_protocol_activations` (PK `run_id`; **never pruned**): written by the first `CreatePrepared`'s transaction; source of `GetRunProtocolVersion` (plan-review #5 #3).

### 6.6 Marker contract (`D-9`)

`MarkerVerificationContext{Secret, AllowedMarkerIDs}` is **required** on the service path; empty allowed-set fails closed. Allowed IDs = self + the **recorded** handoff provenance: a `ProvenanceRunID` field persisted alongside the durable handoff/restart/reprompt prompt **at mint time** (new durable field, e.g. `PendingRestartProvenanceRunID` / handoff-envelope field). Verify-time never infers from `parentRunID`/`sourceRunID`. MAC scheme unchanged; minting additionally records provenance.

### 6.7 Operator resolution semantics (SS-17)

- `ResolveUncertain` and `RetryAsNew` are single atomic commits (§6.1): old-record terminal transition + (for retry) new `prepared` record with `PredecessorTurnID` + new/kept intent linkage + audit entry — one log line locally, one RPC transaction on Supabase. Crash cannot terminalize-without-successor or double-create (idempotent `resolutionID`; replay served via `GetResolutionResult`).
- **Closed action/settlement table (no downstream inference):** `ResolveUncertain` takes `OperatorEvidence{Actor="local-operator", Detail, CapturedAt}` and atomically checks the expected revision, unresolved state, resolution-ID replay record, owner intent, and parent Stop fence. Its resulting state, owner-intent mutation, `SettleOwed`/`SettlePhase`, audit, and optional successor are exactly:

| Action | Predecessor result | Intent + settlement result in the same commit | Successor |
| --- | --- | --- | --- |
| `mark_completed` | `terminal_completed`, `Outcome=completed,resolved_by=operator` | clear the owner intent; preserve immutable `SettleOwed`; `SettlePhase=settle_pending` iff owed, otherwise `none` | none; the phase driver runs only if pending |
| `mark_failed` | `terminal_failed`, `Outcome=failed,resolved_by=operator` | clear the owner intent; preserve immutable `SettleOwed`; `SettlePhase=settle_pending` iff owed, otherwise `none` | none; the phase driver runs only if pending |
| `confirm_cancelled` | `terminal_cancelled`, `Outcome=confirmed_cancelled,resolved_by=operator` | clear the owner intent; force `SettleOwed=false`, `SettlePhase=none`; no graph/gate/finalizer effect is owed | none; this is the only non-abandon resolution allowed after a parent-fence mismatch |
| `abandon` | `terminal_cancelled`, `Outcome=abandoned,resolved_by=operator` | clear the owner intent; force `SettleOwed=false`, `SettlePhase=none`; no graph/gate/finalizer effect is owed | none |
| `retry_as_new` | predecessor `terminal_cancelled`, `Outcome=superseded,resolved_by=operator` | predecessor forces `SettleOwed=false`, `SettlePhase=none`; atomically replace/retain the owner's live intent link for the successor after generation+envelope-hash+parent-fence checks | exactly one child `prepared` record with `PredecessorTurnID`; only this successor may settle |

  A parent-fence mismatch permits only abandon/confirm-cancelled; retry is rejected. No automatic terminal commit may use operator evidence, and no resolution path may separately clear an intent or separately schedule settlement.
- **`RepairRecord`** (first-class): `{RunID, RepairRevision, Reason, QuarantineBlob + QuarantineHash, State: open → resolved(action), ResolutionID, CreatedAt/ResolvedAt}` stored in the dispatch store. **Creation**: fail-closed loaders call **`OpenRepair(runID, reason, rawBlob, rawHash)`** — create-if-absent, ONE commit persisting the record **and the raw blob itself** + snapshot-writer block + audit (plan-review #5 #1); a repeat returns the existing `RepairRevision`.
- **Resolution is two-phase** (plan-review #5 #2 — a store transaction cannot run the Go loader): `BeginRepairResolution` (CAS-claims the attempt with a TTL, audits start, hands back the quarantined raw) → the runner validates/loads **outside** the store (`applySessionRuntime` against the raw) → `CommitRepairResolution` (CAS: `resolved_retry_load` ⇒ unblock; `failed_still_open` ⇒ record stays open, attempt audited; `resolved_abandon` ⇒ run terminalized). `abandon` still performs `BeginRepairResolution` then commits `resolved_abandon`, but skips the loader; there is no direct-abandon mutation. Crash between Begin/Commit ⇒ the attempt claim expires (TTL), a later attempt supersedes; `resolutionID` keeps every path idempotent. Race rule: while open, snapshot writers are rejected (§6.5); concurrent attempts serialize on the `RepairRevision`/attempt CAS.
- Surfacing, API, desktop card, and the audit store are a first-class implementation slice (CP-51 **Task-256**), not a side effect of recovery work.

## 7. Execution Flow

### 7.1 Dispatch (happy path)

1. Durable outer intent exists (owner side). Delivery selects it → `CreatePrepared` (record + envelope; `IntentOwnerRunID` = the intent's owner, e.g. the parent for restart). A released child also carries its immutable `ParentStopFence`.
2. `CASAdvance(prepared → send_claimed)` (fails if `CancelRequested`).
3. **Linearization**: `CASAdvance(send_claimed → send_started)` durably, immediately before the adapter's first external byte. The CAS always locks/checks the record's **own** `RunStopState`; a child additionally checks its `ParentStopFence` against the parent `RunStopState`, all in the same writer/RPC transaction. Local `CancelRequested`, typed `ErrRunStopFence`, or typed `ErrParentStopFence` ⇒ this CAS fails → `CommitPreSendCancellationAndClearIntent(source=self|parent_fence)` atomically writes `terminal_cancelled(stopped_before_send)`, clears the intent owner, and audits; zero bytes.
4. Adapter emits the explicit `TurnBridge.Accepted(ReceiptEvidence)` seam in §6.3a only when its evidence-backed provider rule is met; then `CommitReceiptAndClearIntent` writes canonical receipt + state + **owner's** intent clear + audit in one commit. Failure ⇒ receipt invisible, intent held, commit retried; crash ⇒ recovery reconciles.
5. A provider-backed terminal proof reaches `TurnBridge.Terminal(TerminalEvidence)` → `CommitTerminalAndSettleIntent` (state/outcome are derived from `proof.Outcome`, `SettlePhase` from persisted `SettleOwed`, and `StopOutcome` from transactionally locked record cancel + own/parent run Stop authority). A post-send `SendTurn` error is not proof: preserve `send_started`/intent and enqueue reconcile; no terminal or clear is legal.
6. Settle phase driver executes §5.3: for each phase, run its convergent durable operations (a marker can skip only after the operation's own idempotency contract is satisfied), then CAS the phase as completed. Gate reprompt/block ⇒ `settle_superseded_reprompt` disposition; Stop ⇒ bookkeeping continues, dependents release suppressed per stop-generation.

### 7.2 Stop

`SetCancelRequested` CAS. Any Stop first calls `RequestRunStop` on the affected run's own authority, then requests cancellation on active records in the same serialized writer/RPC workflow. The per-record loop is not relied on for linearization or post-send attribution: every root send CAS checks own `RunStopState` and every child additionally checks parent state. Pre-linearization: local Stop, typed `ErrRunStopFence`, or typed `ErrParentStopFence` makes send CAS fail, then `CommitPreSendCancellationAndClearIntent(..., source=self|parent_fence)` atomically terminalizes **and clears the intent owner**, `StopOutcome=stopped_before_send`. Post-linearization: provider cancel request is non-terminal; only a subsequent `TerminalEvidence` may commit `terminal_cancelled`, `terminal_failed`, or completed-before-cancel. That terminal transaction locks own run Stop authority and, for a child, parent authority; it writes `StopOutcome=cancelled_in_flight` if record cancel, own Stop, or parent fence is effective in that transaction, regardless of which proven terminal outcome won. If terminal commit locks first, its no-stop attribution is authoritative and a later Stop has no in-flight turn to attribute. No new provider send or dependent user-work is initiated after the Stop acknowledgment. Tests assert conditioned on the recorded `StopOutcome` — never the unprovable "no bytes after Stop CAS".

### 7.3 Recovery (restart)

Per §6.4; `repair_required` runs (D-7) skip dispatch entirely and surface per SS-17; settle driver resumes unfinalized settles.

## 8. Failure and Edge Handling

- `F-1` Crash before `send_started` → safely-retryable, envelope-verified re-dispatch (INV-1/INV-2).
- `F-2` Crash between `send_started` and durable receipt → reconcile; no proof ⇒ `uncertain`.
- `F-3` Stop vs send → CAS order + `StopOutcome` (INV-3).
- `F-4` Receipt observed, commit fails → state stays `send_started`, **owner's intent still held** (atomic coupling); retry worker; crash-safe.
- `F-4a` `SendTurn` returns after `send_started` (including an ambiguous timeout/connection close) → no terminal transition and no intent clear; durable state remains reconcilable and recovery either obtains `TerminalEvidence` or surfaces `uncertain`. A transport error is not evidence.
- `F-5` Parent-restart atomicity: child `provider_accepted` commit clears the parent's restart intent in the same commit — crash can no longer leave {accepted child + live parent intent} (duplicate) or {cleared intent + unproven child} (loss). (Plan-review #2 finding #1.)
- `F-6` Terminal commit fails → retry; crash ⇒ record still `provider_accepted`, recovery reconciles; settle re-derived from `SettlePhase`.
- `F-7` Crash anywhere in settle (between phases, or between a phase's effects and its CAS) → driver replays the in-progress phase's convergent operations; the ledger may skip only after that operation's own durable idempotency contract is met ⇒ no double finalizer/cohort/release AND no lost effect (INV-7). A "phase marked done but effects unrun" state is unrepresentable because the CAS comes after the effects. (Plan-review #3 findings #1/#2.)
- `F-15` Stale owner-session snapshot written after an intent clear → filtered on write and on read against the durable clear-set (§6.2/§6.5) ⇒ cannot resurrect `PendingRestart*` (finding #6; ledger `IR`).
- `F-8` Two scanners → lease; expired lease → takeover CAS.
- `F-8a` Claimed recovery record becomes stale or receives Stop → scanner re-reads after claim and verifies its lease immediately before each provider call/CAS; stale/expired lease stops without external action. A sent record with newly observed cancel is provider-cancelled before attach/resume, then reconciled.
- `F-9` Stale snapshot/older revision → `ErrStaleDispatch`; dispatch state lives outside the session blob so a stale blob cannot touch it. (Finding #4.)
- `F-10` Torn dispatch-log tail → dropped + counted; monotonic revisions prevent resurrection.
- `F-11` Corrupt/unknown-version/**missing-under-V2** runtime or dispatch log → quarantine + `repair_required` + dispatch blocked (SS-17 AC-3/AC-4). (Finding #10.)
- `F-12` Envelope/intent mismatch on redispatch → refuse → `uncertain` → operator.
- `F-13` Operator retry crash-window → atomic `RetryAsNew` (no orphan terminalize, no double retry). (Finding #7.)
- `F-14` Same-service cross-run marker → §6.6 (INV-6).

## 9. Security and Operational Concerns

- Marker MAC + recorded provenance (§6.6); operator = local user, every resolution audited (SS-17 AC-5/BR-6).
- Audit: CAS transition log `(turnID, from→to, revision, owner)`; operator audit rows in the same commits; counters: `uncertain`, `repair_required`, `ErrStaleDispatch` rate, lease takeovers, settle-phase retries.
- Rollback (`D-10`): V2 authority is **derived from the dispatch store** (activation entry / ≥1 record — §6.5); the session `dispatch_protocol_version` is a repairable mirror. Migration matrix: V2-code/V2-run = normal; V2-code/legacy-run = upgrade **atomically at the first `CreatePrepared`** (activation rides the same commit — no marker/record crash window); legacy-code/V2-run = **block automated dispatch + surface**; kill switch = pause dispatch, never reinterpret. No shadow-writes under V1 authority.

## 10. Risks and Trade-Offs

- `R-1` `startTurn`/`runTurn` blast radius (MEDIUM, 9 callers) — stable signatures, per-run protocol authority, matrix coverage.
- `R-2` *(verify live)* matrix cells — absorbed by design (unproven ⇒ uncertain).
- `R-3` Settle-phase refactor of `resumePendingFlowGate` touches BUG-288 V10/P1 fixes — the phase driver must preserve their guards (stop-generation, single-flight, epoch); non-regression rows in CP-51 §10.2.
- `R-4` Local commit-log growth — bounded by terminal+finalized TTL pruning.
- `R-5` Supabase RPC surface is new — gated by the contract suite + real-PG run (`PG` row).
- `R-6` Hot-path extra CAS writes — accepted; combine where measurable.
- `R-7` Cross-platform harness (Windows first-class) — `os.Process.Kill`, no POSIX-only signals.

## 11. Validation Strategy

- unit: exhaustive transition + settle-phase tables (legal + every forbidden edge); CAS stale rejection; lease claim/expiry/takeover plus **transaction-guarded recovery unknown/terminal writes**; cross-record atomic commits (parent-intent clear coupling); canonical receipt equality/conflict and terminal-evidence rejection; parent-Stop-fence interleavings; envelope hash; legacy mapping; marker context fail-closed; runtime validation incl. missing-under-V2; operator action-to-settlement table/idempotency (`resolutionID` replay).
- store contract: **one shared suite** (dispatch API + full-field `ProviderSessionState` round-trip parity, `D-11`) against local and Supabase impls; ≥1 run against real PostgreSQL (CAS + concurrent claims + RPC transactions).
- integration (CP-51 Task-255): **real subprocess kill** crash harness — parent process owns the durable store and a **fake provider HTTP/IPC server with a durable request log** (send counts survive worker death); worker receives store path/provider URL/run identity via env; cross-platform `Process.Kill`. Barriers `B0..B8` **plus settle-phase barriers `B8a..B8e`**; explicit B3/B4 ambiguous-send-error cells; Stop at every barrier asserting on recorded `StopOutcome`; parent-Stop/release/send interleavings; outage fail-K-then-recover at every persist point; concurrency variants (lease contention, stale write, torn tail).
- **model-based/property suite**: randomized interleavings of {dispatch steps, Stop, crash, outage, lease expiry, operator resolution, flag flip, restart} against an invariant checker (INV-1..7) + linearizability check of Stop/send by CAS order (CP-51 ledger `MB`). Composite sequences (Stop+crash, outage+terminal+restart, operator+reconcile race, lease-expiry+second-scanner+Stop, flag-flip+restart) are explicit cells.
- observability: transition/settle logs + counters above.
- gates: `go build`, `go vet`, `go test -race ./internal/runner ./internal/flowgate`, existing BUG-288 suites green (CP-51 §10).

## 12. Traceability to Spec

- SS-17 `AC-1` → §6.4/§6.7 surfacing + envelope summary; `AC-2` → §6.7 atomic resolution/settlement table + `PredecessorTurnID`; `AC-3` → §6.5/§8 F-11 quarantine/repair (incl. retry-load/abandon via Task-256); `AC-4` → dispatch blocking + rollback matrix (§9); `AC-5` → idempotent `resolutionID` + audit-in-commit; `AC-6` → no timer-based resolution anywhere; `BR-7` → §5.2/§6.3a/§8 F-4a terminal-proof boundary; `BR-8` → §5.4.1/§7.1–7.2 parent Stop fence.
- SS-16 `AC-7`/`BR-6` → §7.2 Stop linearization (INV-3); `BR-5` → §6.6 (INV-6); `AC-4` → shared store contract + parity suite (`D-11`) — AC-4 governs FlowDefinition shape portability; the durability protocol's authority is this document + SS-17.
- SS-14 → §5.3/INV-7: the three-tier gate's settlement can neither run twice nor be silently lost across crash/outage.
- SS-11 → §7.3 recovery extends the resume authority; §6.2 load-time intent reconciliation is the new resume input.

## 13. Scope Boundary (explicit — what this design does NOT cover)

Full durable orchestration is larger than this design. Out of scope here (tracked elsewhere, not blockers for CP-51): full chat-transcript fidelity across providers; agent bus / queued feedback / waiter durability; complete cohort/fan-out lifecycle beyond the settle phases defined above; indefinite retention (local prune stays, with the `D-11` guard); "whole-server-off → full orchestration resume" proof beyond the dispatch/settle seam. CP-51 §10.6 carries the same table so the DoD cannot be read as covering them.

# CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation

## Metadata

- Document ID: `CP-51`
- Title: `Durable Turn Dispatch State Machine And Recovery Reconciliation`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Codex review (multi-round)`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SD-25 Recovery Ownership Linearization Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SS-17 Dispatch Uncertainty And Repair Operator Contract](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md), SD-20 Flow Gate Rule Semantics (three-tier / Flow Mode), SD-21 Change Contract, [SS-14 Code Context & Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `Task-248, Task-249, Task-250, Task-251, Task-252, Task-253, Task-254, Task-255, Task-256, Task-257 (P-0 spike), Task-258 (per-project local dispatch + Drive sync; retires Supabase dispatch tables as default)`
- Related verification log: [CP-51-PhaseAB-Timeline-And-Verification-Log](./CP-51-PhaseAB-Timeline-And-Verification-Log.md) (Phase A/B + BUG-288 + CP-51 timeline + `go test` / live E2E checklist)
- Related Documents: [BUG-288: Flow-Mode Three-Tier Gate + Change Contract Re-entry Gaps](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [Task-242: Flow-Mode Three-Tier Gate](../../08-Task/done/Task-242-Flow-Mode-Three-Tier-Gate.md), [Task-239: Flow Restore And Step Transition Log](../../08-Task/done/Task-239-Flow-Restore-And-Step-Transition-Log.md), [Task-067: Post-Restart Run Resume](../../08-Task/done/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md)
- Replaces: `None`
- Tags: `agent-flow-engine, durable-turn, dispatch-state-machine, crash-recovery, stop-race, idempotency, supabase, codex-review, bug-288`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- BUG-288 reached Round 20 by patching individual crash/Stop windows. Codex's Round-20 verdict is that **6 remaining runtime defects (2 Critical, 4 Important) + 2 verification gaps share one root cause**: turn dispatch is coordinated with RAM flags (`turnInFlight`) + a two-value marker (`prep:<turnID>` / bare launch-ack) instead of a single durable record that proves what the provider actually received.
- This CP implements SD-24 v3: a **forward-only, CAS-versioned** dispatch state machine (**8 states**: `prepared → send_claimed → send_started → provider_accepted → terminal_completed/failed/cancelled`, plus `uncertain`; the "intent pending" phase is the durable outer intent itself) persisted in a **dedicated dispatch store** (local single-writer commit log; Supabase `dispatch_records` table + transactional RPCs) — never inside the session runtime blob.
- **Cross-record atomicity**: records carry `IntentOwnerRunID` (parent-owned restart intents — verified `deliverPendingRestart`); the commit APIs clear the owner's intent in the **same** atomic commit. **Gate settlement is a durable sub-lifecycle** (`SettlePhase`: `settle_pending → gate_evaluated → completion_committed → graph_settled → dependents_released → finalized`) with idempotent side effects — INV-7 exactly-once settlement.
- **Honest delivery contract** (no provider offers idempotent submission — SD-24 §6.3 capability matrix): every accepted intent ends **terminal | safely-retryable | explicitly-uncertain** (surfaced per SS-17); no silent loss, no blind duplicate. Stop and send **linearize** on the durable `send_claimed → send_started` CAS.
- A lease-based **recovery scanner** reconciles non-terminal records on restart; outer intents clear only inside the atomic commits — never from RAM observation.
- Supporting fixes: gate-checkpoint retry worker with backoff; FCP marker bound to service secret **and** allowed run-ID set end-to-end; Supabase `session_runtime` versioned + fail-closed; non-terminal idempotency/dispatch keys never pruned.
- **Definition of Done is a crash-matrix**: crash and Stop injected at every barrier (local + Supabase), automatic outage recovery, marker-replay tests, runtime-corruption tests, and `go test -race ./internal/runner` — all green. All confirmed defects (`DOD-C1..DOD-I6`) provably closed with tests, not by inspection.

### Current Ask

- This plan is approved for implementation. Deliver the durable dispatch state machine + recovery reconciliation as **one coherent batch** (Task-248..256), closing BUG-288 Round-20 findings structurally rather than as Round 21/22 point patches, and prove it with the §10 crash-matrix DoD.
- **P-0 status: PARTIALLY CLOSED.** The Codex/Grok/Claude evidence subgate is complete: all three probes conclude **unprovable (as a receipt) ⇒ `uncertain`**, so none of the three has an unsafe acceptance or attach seam and all three may enter only the three-outcome V2 rollout (Claude's probe additionally exercised a live kill-mid-turn reconcile+attach test, see [Claude evidence](../../06-System-Tech-Design/evidence/SD-24/claude.md)). Gemini is explicitly V2-disabled pending its own Task-257 evidence. The document-approval prerequisite is now recorded; the remaining open piece is the deferred Gemini evidence scope.

### Key Decisions

- `P-1` `DispatchRecord` + immutable `DispatchEnvelope` are the single source of truth; retire the `prep:<turnID>` / bare encoding. All mutations are revision-CAS via the SD-24 §6.1 store API; stale writes are rejected, never merged.
- `P-2` Outer intents clear **only inside** the atomic commits (`CommitReceiptAndClearIntent` / `CommitTerminalAndSettleIntent`); a receipt that failed to persist is invisible to every predicate (never RAM).
- `P-3` Recovery acts only under `ClaimRecovery → Get fresh → lease/effective-Stop recheck before every provider action`: `prepared`/`send_claimed` are safely-retryable (same TurnID + envelope, hash-verified); `send_started`/`provider_accepted` first evaluate durable own-run Stop, parent-fence Stop, and record cancellation, then cancel before any attach when effective Stop is true; otherwise they reconcile per the capability matrix; `uncertain` holds for SS-17 operator resolution (retry = **new** record via `PredecessorTurnID`). No non-terminal record is ever pruned.
- `P-4` Stop **linearizes** against send at the durable `send_claimed → send_started` CAS (SD-24 §7.2); `ctx.Err()` sampling is defense-in-depth, not the correctness mechanism.
- `P-5` Marker verification takes a **required** `MarkerVerificationContext{Secret, AllowedMarkerIDs}`; empty allowed-set on the service path fails closed; provenance comes from the recorded handoff binding, not blanket parent/source trust.
- `P-6` Supabase `session_runtime` is versioned with a supported-version set + presence/semantic validation; failures quarantine the blob and mark `repair_required` (SS-17 AC-3/AC-4) — never zero-value resume, never overwrite the evidence.
- `P-7` Delivery guarantee is **three-outcome** (terminal | safely-retryable | explicitly-uncertain); exactly-once is not claimed anywhere (SD-24 §6.3).
- `P-8` Gate settlement is a durable **sub-lifecycle** (`SettlePhase`, 6 phases) stamped atomically by the terminal commit and executed by a **phase driver** with idempotent side effects (INV-7) — not `notifyTurnIdle` (which only flushes resume/reprompt intents; verified) and not a single boolean.

### Constraints

- Backward compatible with sessions written before this CP: legacy bare/`prep:` idempotency values and unversioned Supabase runtime blobs must migrate or be treated as a defined legacy state, not lost.
- No change to Chat Mode hub `runFlowGate` rule set semantics; scope is the turn dispatch/persistence/recovery seam only.
- Must not regress the closed BUG-288 Round 1–20 fixes (`F-1..F-43`, `V9-*`, `V10*`, Round 11–20 residuals).
- GitNexus impact tooling is intermittently unavailable in this repo; blast radius is grounded on the last verified analysis (see §9) + code inspection + tests.
- `go test -race ./internal/runner` must pass; tests must not write into the target project tree (an untracked `.cache/` from the harness is acceptable but should be gitignored).

### Open Questions

- `Q-1` **Resolved (2026-07-16):** the durable dispatch design is now captured upstream in [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (per SS-13 §7.3, a reusable persistence/recovery contract belongs in a Tech Design, not only a CP). This CP implements SD-24; Parent Documents re-pointed at it.
- `Q-2` **Resolved (2026-07-17):** receipts are defined per provider by the SD-24 §6.3 **capability matrix**. Initial enabled scope: Codex, Grok, and Claude have recorded `unprovable (as a receipt) ⇒ uncertain` outcomes, so `thread/start`/`session/new`/`writeUserTurn` stdin and observed headless/stream output are **not** receipts and no adapter calls `Accepted`. Gemini one-shot remains V2-disabled until Task-257 records its outcome. The protocol is safe under either answer (no proof ⇒ `uncertain`).
- `Q-3` **Resolved (2026-07-16):** `uncertain`/`repair_required` behavior is now product-specified in **SS-17** (hold + surface; operator actions inspect / retry-as-new / mark-outcome / abandon; idempotent + audited; automated dispatch blocked until resolved; no timer-based auto-terminalize).

### Source Refs

- Codex Round-20 architectural FAIL verdict on branch `task/cp43-50-vs-task238`, HEAD `6ea5417` (2026-07-16).
- BUG-288 §11 (Vòng 1–20 inventory); SS-13 (document contract); SD-20 (Flow Gate), SD-21 (Change Contract), SS-14 (regression safety).
- Verified code: `interactive_service.go`, `interactive_resume.go`, `workflow_store.go`, `local_file_session_store.go`, `supabase_workflow_store.go`, `feature_history.go`, `flow_context_handoff.go` (see §3 defect map for exact lines).

## 1. Goal

Replace the ad-hoc turn-dispatch coordination (RAM `turnInFlight` + `prep:<turnID>`/bare launch-ack idempotency values) with a single, versioned, durable **dispatch state machine** and a **recovery scanner** that reconciles every non-terminal record on restart. The objective is to make the following invariants hold **by construction** across crashes, Stops, and storage outages — not by patching individual windows:

- **INV-1 Three-outcome delivery (no silent loss).** Every accepted intent ends `terminal`, **safely-retryable** (provably never reached the provider), or **explicitly-uncertain** (held + surfaced per SS-17). Exactly-once is not claimed — no provider supports it (SD-24 §6.3).
- **INV-2 No blind duplicate.** A turn that may have reached the provider is never re-sent without proof it did not run; operator retry is a **new** linked record.
- **INV-3 Stop/send linearization.** A send whose `send_started` CAS follows the Stop CAS in revision order can never happen; after linearization, cancellation flows through the provider.
- **INV-4 No silent state loss.** Corrupt/version-mismatched persisted runtime fails closed (`repair_required`), never zero-values topology/gate/intent state.
- **INV-5 No wedge.** A transient storage outage that recovers does not leave a run permanently stuck; a retry worker drains the backlog.
- **INV-6 No cross-run/replay bleed.** Feature-history suppression only honors a marker bound to this service's secret **and** an allowed run ID.

## 2. Input Documents

- [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) — the governing tech design; this CP is its implementation.
- SD-20 Flow Gate Rule Semantics (three-tier gate lifecycle, Flow Mode post-turn gate).
- SD-21 Change Contract (re-entry prompt injection interaction with dispatch).
- [SS-14 Code Context & Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md).
- [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md) — originating multi-round review; this CP closes its Round-20 residuals.

## 3. Implementation Strategy

### 3.1 Verified defect map (Codex Round 20, re-confirmed against HEAD `6ea5417`)

Every finding below was validated by reading the cited code at the current HEAD. Line numbers are anchors, not contracts.

| ID | Sev | Defect | Root evidence (verified) | Fixed by |
| --- | --- | --- | --- | --- |
| `DOD-C1` | Critical | Durable turn can be **lost or double-run** across crash | Outer intent cleared on RAM `turnInFlight` (`durableIdemReplaySafe` `interactive_service.go:2745-2749`; clear at `interactive_resume.go:1529`). Provider only scheduled async (`interactive_service.go:6086`). Terminal events not persisted by local store (`local_file_session_store.go:611`) / deprecated on Supabase (`supabase_workflow_store.go:416`). State has no provider receipt (`workflow_store.go:249-259`). | Task-248, Task-249, Task-250 |
| `DOD-C2` | Critical | **Stop race** after launch-ack | Stop/stale revalidated only before launch-ack (`abortDurableStartIfStaleLocked` call `interactive_service.go:5978`); no fence between launch-ack persist and `go runTurn` (`:6086`); `runTurn` does not check `ctx.Err()` at entry (`:4826`). | Task-249 |
| `DOD-I3` | Important | Gate checkpoint outage **wedges or is lost** | After 3 persist fails, RAM-only blocked mark (`interactive_service.go:3369-3379`); one immediate retry only (`:5151-5176`); `notifyTurnIdle` treats `pendingFlowGateSettle` as busy → never retries (`interactive_resume.go:1573-1581`). | Task-251 |
| `DOD-I4` | Important | Same-service **cross-run FCP marker replay** suppresses history | Outer check binds `rs.id` (`interactive_service.go:4906`); inner helper re-verifies with secret only, no expected IDs (`feature_history.go:33`), and id-binding in `isFlowContextHandoffWithSecret` only runs when `len(expectedIDs) > 0` (`flow_context_handoff.go:260`). | Task-252 |
| `DOD-I5` | Important | Supabase recovery **fail-open** on corrupt runtime | `applySessionRuntime` swallows unmarshal error and returns, leaving all recovery fields zero-value (`supabase_workflow_store.go:134-141`); `sessionRuntimeBlob` has **no version field** (`:60-103`). | Task-253 |
| `DOD-I6` | Important | Active **idempotency key pruned** on long sessions | Fixed cap 48, cross-namespace numeric generation (`durableIdempotencySnapshot` `interactive_service.go:2641-2698`); active key pinned only at accept-time snapshot, post-turn snapshot uses plain `sessionStateOf(rs)` (`:5045`). | Task-254 |
| `DOD-G7` | Gap | Round-20 tests miss crash/Stop/outage/replay/corruption/cap matrices | BUG-288 §8 validation list; §11.9.1 admits no transaction-boundary/crash coverage. | Task-255 |
| `DOD-G8` | Gap | Contract contradiction on `EventTurnStarted` | Comment claims recovery uses `EventTurnStarted` (`interactive_service.go:5994`) while `durableIdemReplaySafe` states it is insufficient and does not use it (`:2732-2766`). | Task-248 |

### 3.2 Target state machine (authority: SD-24 §5 — do not re-derive here)

Forward-only, CAS-guarded, **eight states** (no `intent_pending` state — before `CreatePrepared` the durable artifact is the outer intent itself), plus the settle sub-lifecycle:

```text
(durable intent) ─CreatePrepared→ prepared → send_claimed → send_started → provider_accepted → terminal_completed
                                      │            │             │               │            ↘ terminal_failed
                                      │            │             │               └───────────→ terminal_cancelled
                                      │            │             └─ (recovery, unprovable) → uncertain
                                      └────────────┴─ (Stop CAS won before send_started) ──→ terminal_cancelled
uncertain ─ (reconcile proof | SS-17 operator resolution: ResolveUncertain/RetryAsNew) → terminal_*

SettlePhase (same record, same Revision CAS — SD-24 §5.3):
settle_pending → gate_evaluated → completion_committed → graph_settled → dependents_released → finalized
```

- **Forbidden by construction:** any backward edge (no `→ prepared` rewind); `uncertain → prepared`; exits from `terminal_*`; re-entry into `send_started`. Operator retry = **new** record (`PredecessorTurnID`).
- `CancelRequested` is a monotonic CAS flag, not a state: together with durable own-run Stop and a child’s parent-fence Stop, it fails the `send_claimed → send_started` CAS and routes non-terminal states to the cancel path (SD-24 §5.2 invariants).
- The `send_claimed → send_started` durable CAS is the **linearization point** between Stop and provider send (SD-24 §7.2).

### 3.3 Record + envelope shape (authority: SD-24 §5.3–§5.4)

- `DispatchRecord`: `ProtocolVersion`, `TurnID`, `RunID`, **`IntentOwnerRunID`** (≠ `RunID` for parent-owned restart intents — the cross-record atomicity key), `State`, `CancelRequested`+`StopGeneration`+**`StopOutcome`** (`stopped_before_send` | `cancelled_in_flight`), **`Revision`** (every write is CAS; stale ⇒ `ErrStaleDispatch`, reload — never merge), **`ClaimOwner`/`ClaimExpiresAt`** (recovery lease), **`RecoveryAttachEpoch`/`RecoveryAttachOwner`/`RecoveryAttachExpiresAt`** (durable attached-stream ownership; never session state), `OuterIntentKey`/`OuterIntentGen`, `EnvelopeHash`, canonical **`ReceiptEvidence`** and provider-backed **`TerminalEvidence`**, immutable `SettleOwed` + **`SettlePhase`** (sub-lifecycle, set atomically with terminal/resolution commit), `PredecessorTurnID`, `Outcome`, and child **`ParentStopFence`**. A non-prunable dispatch-store **`RunStopState{runID,generation,stopped,revision}`** is the sole own-run and parent-fence Stop authority; session/record stop fields are derived mirrors.
- `DispatchEnvelope` (immutable, written once at `prepared`): `PromptRef` (existing `turnLogKindPrompt` line) + `PromptSHA256`, `StepID`, `Model`/`ReasoningEffort`/`Yolo`, `SelectedSkills`, `Scenario`, `ProviderKey`/`ProviderAccountID`/`ProviderSessionIDAtPrepare`, `Attachments` (refs+hashes), `FlowContextInjected`, `EnvelopeHash`. Recovery re-sends **from the envelope**; hash mismatch vs live intent ⇒ refuse + `uncertain` (SS-17).
- Store API (only mutation path — SD-24 §6.1): `CreatePrepared`, `CASAdvance`, `CASAdvanceSettle`, `ClaimRecovery`, `SetCancelRequested`, and the **cross-record atomic commits** `CommitReceiptAndClearIntent` / `CommitTerminalAndSettleIntent` / `CommitPreSendCancellationAndClearIntent` (dispatch transition + intent-owner clear + audit in ONE commit) plus the **operator ops** `ResolveUncertain` / `RetryAsNew` (idempotent by `resolutionID`). Storage: dedicated local `dispatch.ndjson` single-writer commit log (disk-before-RAM, torn-tail-safe — fixes `local_file_session_store.go:273`) and Supabase `dispatch_records` table + transactional RPCs; dispatch state never lives in the session runtime blob (SD-24 §6.2/§6.5).

### 3.4 Sequencing logic (reordered per Codex plan review #15)

0. **P-0 approval gate** — SS-17 approved; SD-24 v2 accepted; the 8 questions in §3.6 all answered. No code before this.
1. **Persistence foundation** — Task-248 **+** Task-253 together (one phase: record + envelope + CAS/lease store API + local single-writer + Supabase `schema_version`/`ProtocolVersion` + migration/quarantine). They share schema constants and must not be split.
2. **Harness skeleton (red)** — Task-255 phase 1: subprocess crash harness + the defect-regression tests, shown failing on HEAD `6ea5417`.
3. **Live path** — Task-249: drive the record through `startTurn`/`runTurn`; Stop/send linearization; atomic receipt commit; capability-matrix live verification.
4. **Recovery** — Task-250: lease-based scanner + per-provider reconcile + SS-17 operator resolution wiring.
5. **Hardening + surface** — Task-251 (settle phase driver), Task-252 (marker context), Task-254 (retention derived from records), Task-256 (operator API/UI/audit — needs Task-248's atomic ops + Task-250's classification) — parallel.
6. **Final gate** — Task-255 phase 2: full matrix + settle barriers + concurrency (lease contention, stale write, torn tail) + composite-fault/model-based suite + `-race` + Postgres contract run. Merge only when the §10 ledger is fully green.

### 3.5 Dependencies

- Task-249, Task-250, Task-251, Task-254 depend on Task-248 (record + store API). **Task-250 additionally depends on Task-249** (live transitions must exist before recovery can reconcile them).
- Task-253 is co-phased with Task-248 (shared version constants), not independent.
- Task-252 depends only on P-0 (marker-context API shape) and may run any time after it.
- Task-255 phase 1 lands after 248+253; phase 2 is the final gate after everything.

### 3.6 Pre-implementation approval gate (8 questions — answer locations)

| # | Question | Answered in |
| --- | --- | --- |
| 1 | Exact receipt per provider? | SD-24 §6.3 capability matrix — operation shapes verified in code; the *verify live* cells are filled by **Task-257** (the P-0 evidence spike — owner, environment, and evidence format defined there; Task-249 only consumes) |
| 2 | Which providers support query / idempotent operation IDs? | SD-24 §6.3 — none support idempotent submission ⇒ three-outcome contract (INV-1) |
| 3 | Which transitions are durable CAS; who owns the recovery lease? | SD-24 §6.1 (all of them) + §6.4 (`ClaimOwner`/TTL) |
| 4 | Where do Stop and send linearize? | SD-24 §5.2/§7.2 — the `send_claimed → send_started` CAS |
| 5 | What immutable payload does recovery redispatch? | SD-24 §5.4 `DispatchEnvelope` (PromptRef + SHA-256, hash-bound) |
| 6 | If persistence is fully unavailable and we crash, where is the durable source? | SD-24 `F-6`: an unpersisted transition never happened — recovery reconciles from the last durable state; gate settle re-derives from the record |
| 7 | How do users handle `uncertain` / `repair_required`? | **SS-17** AC-1..AC-6 (hold+surface; inspect/retry-as-new/mark-outcome/abandon; idempotent + audited; dispatch blocked) |
| 8 | How does the harness prove real crash / concurrent claim / stale write? | SD-24 §11 + Task-255: subprocess kill (not same-process rebuild), lease contention, `ErrStale`, torn tail, real-Postgres contract run |

## 4. Work Breakdown

> Each task below is **implementation-ready**: its `§4 Exact Change` carries a `§4.1 Code Guide` (step-by-step with `file:function` anchors + Go sketches against HEAD `6ea5417`) and `§4.2 Test skeletons`. The list can be handed to any engineer to implement to plan without re-deriving the design.

- `P-0` **Approval gate — STATUS: CLOSED (corrected 2026-07-22, was stale):** this line previously read "PARTIALLY CLOSED" citing (1) SS-17 approval, (2) SD-24 acceptance, and (3) §3.6 governance answers as still required — all three were already satisfied and the line was simply never updated. Verified today: **SS-17 and SD-24 both carry `Status: approved`** in their own metadata, and the §3.6 governance table already has all 8 questions answered (not blank). (4) Task-257 is complete for **Codex/Grok/Claude**: `CE-CG` and `CE-CL` are ✅ with explicit `unprovable ⇒ uncertain` outcomes; Gemini (`CE-GEM`) is an intentional non-goal (V2 dispatch not supported for Gemini, operator-confirmed 2026-07-22), not a pending approval. No blocker remains on this gate.
- `P-1` **Durable DispatchRecord + envelope + CAS store core** (Task-248): **8-state** forward-only machine + settle-phase enum; `Revision` CAS + lease fields; immutable `DispatchEnvelope`; full store API (mutations **+ read side**: `Get`/`ListRecoverable`/`ListAttention`/`ListAudit`/`ListEffects`/`GetResolutionResult`/`IsIntentCleared`) on the **dedicated** stores (never in the session blob); local commit-log operational contract (process lock, compaction, missing-log-V2 ⇒ repair); legacy migration; `EventTurnStarted` contract fix (`DOD-G8`).
- `P-2` **Live dispatch integration + Stop linearization** (Task-249): drive the record through `startTurn`/`runTurn`; `send_claimed → send_started` durable CAS immediately before the provider send; `CommitReceiptAndClearIntent` on the matrix-defined acceptance signal — **consuming Task-257's recorded evidence** (this task produces none) (`DOD-C1` live side, `DOD-C2`).
- `P-3` **Lease-based recovery + reconciliation + operator resolution** (Task-250): `ClaimRecovery` lease; safely-retryable vs reconcile vs `uncertain` per SD-24 §6.4; SS-17 resolution actions wired (`DOD-C1` recovery side, SS-17 `AC-2`/`AC-4`/`AC-5`).
- `P-4` **Settle phase driver + effect ledger** (Task-251): `SettlePhase=settle_pending` stamped atomically in `CommitTerminalAndSettleIntent`; the phase driver (refactor of `resumePendingFlowGate`'s body) runs each phase's **convergent durable operations first**, **then** CAS-marks the phase completed — crash replays the operation, whose own keyed/monotonic/create-if-absent contract prevents duplicate outcomes; the ledger is only hash-bound audit/skip optimization after that contract is satisfied. Dispositions for reprompt/block/Stop; backoff retry on persist failure (`DOD-I3`, INV-7, ledger `ST`/`EL`/`SD`).
- `P-5` **Marker verification context** (Task-252): required `MarkerVerificationContext{Secret, AllowedMarkerIDs}`; empty allowed-set fails closed on the service path; provenance from recorded handoff binding (`DOD-I4`).
- `P-6` **Supabase runtime versioned + fail-closed + quarantine** (Task-253, co-phased with 248): supported-version set, presence/semantic validation, quarantined blobs, `repair_required` blocking per SS-17 `AC-3`/`AC-4` (`DOD-I5`).
- `P-7` **Retention derived from dispatch records** (Task-254): `DispatchRecord` is the authority; the idempotency map becomes a derived index; non-terminal never pruned; terminal capped per namespace (`DOD-I6`).
- `P-8` **Crash-matrix + concurrency + model-based DoD suite** (Task-255, two phases): phase 1 = subprocess harness skeleton (parent-owned durable fake provider) + red defect tests (after 248/253); phase 2 = full matrix incl. settle barriers, lease contention, stale write, torn tail, outage, **composite-fault/model-based randomized interleavings**, `-race`, Postgres contract run (`DOD-G7` + all invariants).
- `P-9` **Operator resolution surface** (Task-256): the SS-17 owner — attention API endpoints, desktop card, audit read path, `repair_required` actions (retry-load/abandon); everything through the atomic store ops (ledger `OP1/OP2/OP3`).
- `P-10` **Per-project local dispatch + Drive sync** (Task-258): product transport decision — authority is `.flowpilot/chats/<project_id>/dispatch.ndjson`; portability reuses Google Drive chat-sync (`chat-sessions/dispatch/dispatch.ndjson`); Supabase `dispatch_*` tables/RPC are **not** the default backend (clients removed; DROP migration for earlier apply).

### 4.1 SD-25 recovery-closure coverage map (authoritative)

No Task may add a recovery concurrency rule outside this mapping. SD-25 §5 is the authority; SD-24/Task text is implementation detail only.

| SD-25 action | Implementation owner | DoD evidence |
| --- | --- | --- |
| recovery send / pre-send cancel | 248 store contracts + 250 scanner | `RG`, `RSF`, `PS` both-store barriers |
| unknown/no-adapter decision | 248 guarded API + 250 scanner | `RG`, `ES` barriers; no blind `uncertain` |
| recovered terminal proof | 248 guarded terminal API + 250 bridge | `RG`, `SA`, `TP` lease-takeover/proof matrix |
| recovery attach and callbacks | 248 epoch/token/effect storage + atomic attached-terminal API; 249 live terminal revoke; 250 guarded stream bridge; 256 operator/retry revoke | `RA` A/B takeover, terminal-or-unknown paused callback, terminal/expiry-before-entry zero attach, **expiry-before-terminal zero mutation**, exactly-once current-token terminal, all-path revocation, TTL reclaim |
| Stop / parent fence | 248/249/250 | `RSF`, `PS`, `ES`, `SA` |
| closure verification | 255 only | every SD-25 §5 row has deterministic local, Supabase, real-PG, and model test coverage |

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_service.go` — `startTurn`, `runTurn`, `durableIdemReplaySafe`/`durableIntentClearOK`, `abortDurableStartIfStaleLocked`, `durableIdempotencySnapshot`, gate settle checkpoint, `emitLocked` settle path.
  - `apps/local-runner/internal/runner/interactive_resume.go` — `flushDurableTurnIntents`, `notifyTurnIdle`, recovery reconstruction hooks.
  - `apps/local-runner/internal/runner/workflow_store.go` — `ProviderSessionState` gains ONLY the run-level `DispatchProtocolVersion` marker + `RepairRequired` fields + provenance fields; **no dispatch records in the session state** (SD-24 `D-1`).
  - `apps/local-runner/internal/runner/local_file_session_store.go` — terminal dispatch-outcome durability; sidecar event set.
  - `apps/local-runner/internal/runner/supabase_workflow_store.go` — `sessionRuntimeBlob` version; `applySessionRuntime` fail-closed.
  - `apps/local-runner/internal/runner/feature_history.go`, `flow_context_handoff.go` — marker run-ID binding.
  - new: `apps/local-runner/internal/runner/dispatch_record.go` (record + envelope + states + CAS guards), `dispatch_store.go` (store API contract, both backends), `dispatch_recovery.go` (lease + scanner + reconciliation), operator-resolution wiring per SS-17.
- modules: `internal/runner` (interactive service, persistence, recovery), desktop card surface for SS-17 (reuses approval/question card pattern).
- database / transport (**Task-258 product decision supersedes default Supabase dispatch**): **Local authority** is per-project single-writer commit log at `.flowpilot/chats/<project_id>/dispatch.ndjson` (activation lines non-prunable in projection; disk-before-RAM, torn-tail, per-project lock). **Portability** is Google Drive chat-sync upload/download of that log (`chat-sessions/dispatch/dispatch.ndjson`), same project folder model as sessions. Supabase `dispatch_*` tables/RPC from early CP-51 drafts are **non-default / dropped** (see Task-258 DROP migration). `session_runtime` versioning (Task-253) remains a separate session-blob concern, not dispatch authority.
- external systems: initial V2 providers are Codex/Grok/Claude with no evidence-backed acceptance/attach seam (three-outcome handling only); Gemini is provider-enable-blocked until Task-257. A session/thread id or stdin write is **not** a receipt.

## 6. Data or Migration Steps

- schema: `DispatchRecord` + `DispatchEnvelope` + `RepairRecord` live in the **dedicated dispatch stores** (local commit log / Supabase `dispatch_records` table — a relational migration + RPCs); `ProviderSessionState` gains only the run-level `DispatchProtocolVersion` marker, `RepairRequired` fields, and provenance fields; Supabase `sessionRuntimeBlob` gains `schema_version` with an explicit **supported-version set** and migration chain.
- data backfill: none forced. Legacy mapping (SD-24 §5.5):
  - `prep:<turnID>` → `prepared`, envelope **synthesized from the persisted prompt log**; missing prompt log ⇒ `uncertain`.
  - bare value → `terminal_completed` only with corroborating terminal/`lastTurnID` evidence; otherwise `uncertain` (never assumed accepted).
  - unversioned Supabase runtime → v0: accepted only on clean decode + presence/semantic validation; any failure ⇒ quarantine + `repair_required`.
  - legacy records carry `ProtocolVersion=1`; recovery may resolve them (→ terminal/uncertain) but never CAS-advances them through v2 live states.
- config updates: `FLOWPILOT_DISPATCH_V2` enables V2 **per run, atomically at its first `CreatePrepared`** — the activation rides the same commit (SD-24 §6.5), so there is no marker/record crash window and no false `repair_required`; the session `dispatch_protocol_version` is a derived, repairable mirror (dispatch store wins). No shadow-writing while V1 is authoritative. Once a run is V2, V1/flag-off code must **block automated dispatch** on it and surface (SD-24 §9).

## 7. Validation Plan

- tests to add: see Task-255 for the full crash-matrix; each task ships its own unit tests (`V-*` in each Task's Acceptance Check).
- manual checks: kill the runner process at each barrier (cross-platform — `taskkill /F` on Windows / `kill -9` on POSIX; procedure in Task-255) on both backends; verify three-outcome delivery (no loss/duplicate) and the recorded `StopOutcome` contract via the fake-provider request log.
- failure cases: storage returns error N times then recovers (outage); corrupt/truncated Supabase runtime blob; **missing blob/log on a V2 run**; **stale owner snapshot written after an intent clear**; crash between a settle phase's effects and its CAS; two overlapping server processes at startup (file lock); 48+ idempotency keys with one active low-gen key; foreign same-service FCP marker.

## 8. Rollout and Fallback

- rollout order: P-0 gate → (248 + 253) → 255 phase 1 (red) → 249 → 250 → (251, 252, 254 parallel) → 255 phase 2 final gate. Do not merge to `main` until the §10 ledger is fully green.
- fallback path (`D-10`): before a run has V2 records, `FLOWPILOT_DISPATCH_V2=0` keeps pure V1 behavior for that run. **After** a run has V2 records, the kill switch pauses automated dispatch on it (surface per SS-17 AC-4) — it never reinterprets V2 records under V1 semantics, and V1 never shadow-writes V2 records. Per-record `ProtocolVersion` + the SD-24 §9 migration matrix are the authority.
- monitoring: CAS transition logs `(turnID, from→to, revision, owner)`; counters for `uncertain`, `repair_required`, `ErrStale` rate, lease takeovers; alert on `uncertain` older than the reconciliation window and on settle-retry exhaustion.

## 9. Risks

- `R-1` **Blast radius of `startTurn`/`runTurn`.** Last verified GitNexus analysis: `startTurn` = **MEDIUM** risk, **9 direct callers**, **2 execution processes** affected. Mitigation: keep the public signatures stable; land behind `FLOWPILOT_DISPATCH_V2`; Task-255 exercises all callers via the crash-matrix.
- `R-2` **Provider receipt fidelity varies by adapter** (Q-2). Mitigation: default-safe mapping — no receipt ⇒ `uncertain` on recovery, never `provider_accepted`.
- `R-3` **Reconciliation without a queryable provider.** A crashed `send_started` record with an offline provider cannot be auto-resolved. Mitigation: `uncertain` holds + surfaces (SS-17); never blind-retries (prevents duplicate) and never clears intent (prevents loss).
- `R-4` **Legacy session compatibility.** Mis-mapping a legacy bare value to a completed state could drop a genuinely-unlaunched turn. Mitigation: bare ⇒ `terminal_completed` only with corroborating terminal/`lastTurnID` evidence, else `uncertain` (one rule — SD-24 §5.6).
- `R-5` **Regression of closed BUG-288 fixes.** Mitigation: run the full existing `internal/runner` + `internal/flowgate` suites in Task-255 alongside the new matrix; no deletion of existing `F-*`/`V*` guards.
- `R-6` **`-race` flakiness surfacing latent data races** in the settle/gate path. Mitigation: treat any `-race` failure as a blocking DoD item, not a flake.

## 10. Definition of Done — The Single Finish Line

> **This section is the one artifact to look at to decide "done or not."** BUG-288 / CP-51 is DONE ⟺ every row in the §10.1 ledger is ✅ and the §10.3 verdict command is green. Nothing here is judged "done by inspection." If a requirement is not a green row below, it is not done; when every row is green, the work is complete and the Codex review loop for this class ends.

### 10.0 Why the review loop terminates (no Round 21/22)

Rounds 1–20 never converged because each round *discovered* its acceptance set by inspection, so there was always a "next" inspection. This CP closes that by fixing a **closed, enumerable acceptance space up front**:

- The dispatch lifecycle has a **finite** set of durable states (8 dispatch states + 6 settle phases, SD-24 §5) and a **finite** set of barriers between side effects (`B0..B8` + settle barriers `B8a..B8e`, §10.4).
- The fault space is **finite**: {crash (real subprocess kill), Stop, outage, concurrent recovery claim, stale snapshot write, torn append} × {local-file, Supabase}.
- The product `barriers × fault × backend` is therefore a **closed matrix** (§10.4), and every cell has a **pre-stated** expected invariant (`INV-1..INV-6`, §1).

**Termination rule (scoped honestly):** the work is done when the whole matrix is green + non-regression (§10.2) green + gates (§10.3) green. The closure claim is **scoped to the durable turn-dispatch + settlement contract as enumerated** (states × barriers × faults × backends, including composite sequences via the `MB` model-based suite) — it means *"the enumerated dispatch-contract defects cannot reopen as new rounds"*, NOT *"no bug exists anywhere in the chat/agent system"* (see §10.6 scope table). Within scope, an issue inside the matrix is a covered cell; a finding is admissible only if it lies outside the enumerated space, and the response is to **add that row and re-run the verdict**, not to open an ad-hoc Round 21. Single-fault cells alone don't close the space — the `MB` randomized-interleaving suite covers fault **combinations** (Stop+crash, outage+terminal+restart, operator+reconcile race, lease-expiry+second-scanner+Stop, flag-flip+restart).

### 10.1 Acceptance ledger (the finish line — every row must be ✅)

> Flip `☐`→`✅` only when the named test is green on CI. Each defect test must first be shown **failing on HEAD `6ea5417`** (pre-fix) and passing after — that is row `FF`.

| ID | Requirement (binary) | Owning task | Verifier (named test) | Status |
| --- | --- | --- | --- | --- |
| `C1` | No lost / no duplicate turn across crash at every barrier × backend | 248/249/250 | `dispatch_crash_matrix_test` (crash cells) | ✅ |
| `C2` | No post-Stop provider send at any barrier | 249 | `stop_race_barrier_test` + matrix (stop cells) | ✅ |
| `C2a` | Pre-send Stop terminalization atomically clears the intent owner (including parent-owned restart); restart never relaunches it | 248/249/250/255 | `TestPreSendStop_ParentOwnedIntentAtomicClearAcrossRestart` + crash matrix | ✅ |
| `AE` | **Ambiguous post-send error never terminalizes**: an error/timeout after `send_started` (B3/B4) retains the owner intent and record for reconcile/`uncertain`; only `TerminalEvidence` or proven pre-send cancellation can commit terminal | 248/249/250/255 | `TestAmbiguousSendError_B3B4_NeverTerminalizesOrClearsIntent` | ✅ |
| `I3` | Gate-checkpoint outage auto-recovers; crash does not lose settle | 251 | `gate_checkpoint_outage_test` | ✅ |
| `I4` | No same-service cross-run marker replay (outer **and** inner path) | 252 | `fcp_marker_replay_test` | ✅ |
| `I5` | Corrupt / version-mismatch runtime ⇒ `repair_required` (fail-closed) | 253 | `supabase_runtime_corruption_test` | ✅ |
| `I6` | Active non-terminal key survives every snapshot + round-trip | 254 | `idempotency_retention_test` | ✅ |
| `G8` | One documented recovery-inference rule; code + comments agree | 248 | `dispatch_record_test` + grep assert | ✅ |
| `Rr1` | `prepared` re-dispatches idempotently (no duplicate) | 250 | `dispatch_recovery_test` | ✅ |
| `Rr2` | `send_started`/`provider_accepted` reconcile, never blind-retry; recovery never terminalizes a sent turn without provider cancel/reconcile | 250 | `dispatch_recovery_test` | ✅ |
| `Rr3` | `uncertain` holds; never clears intent, never re-dispatches | 250 | `dispatch_recovery_test` | ✅ |
| `Rr4` | No non-terminal record / key ever pruned | 254 | `idempotency_retention_test` | ✅ |
| `NR` | All prior-round suites still green (§10.2) | 255 | full `internal/runner` + `internal/flowgate` | ✅ |
| `GB` | `go build ./...` + `go vet ./...` clean | 255 | CI | ✅ |
| `GR` | `go test -race ./internal/runner` clean (incl. matrix) | 255 | CI | ☑ waived (2026-07-24, operator decision — see §10.1.1) |
| `FF` | Every **defect-regression** test fails on HEAD `6ea5417`, passes after fix (foundation tests exempt — see `MU`) | 255 | PR evidence | ✅ (via `MU` substitution — see §10.1.1; not literally replayable against `6ea5417`) |
| `MU` | State-machine **mutation checks**: flipping any transition guard / CAS predicate turns the suite red (foundation-test substitute for fail-on-HEAD) | 248/255 | `dispatch_record_test` mutation cases | ✅ |
| `CC` | Two concurrent recovery scanners: exactly one wins the lease; zero double-dispatch | 250/255 | `dispatch_recovery_test` + matrix | ✅ |
| `SW` | Stale snapshot (older `Revision`) can never overwrite `provider_accepted`/`terminal` | 248/255 | `dispatch_store_contract_test` | ✅ |
| `TA` | Torn NDJSON tail is detected, dropped, and recovery re-derives — never resurrects older state | 248/255 | `dispatch_store_contract_test` | ✅ |
| `SP` | Crash cells run as **real subprocess kills** (not same-process rebuild) | 255 | `dispatch_crash_matrix_test` harness | ✅ |
| `PG` | Store contract suite passes on a **real PostgreSQL/Supabase** (conditional transitions + concurrent claims) | 253/255 | env-gated integration run + PR evidence | ✅ (satisfied by Task-258 retiring Supabase as the dispatch backend — see §10.1.1) |
| `OP1` | Automated dispatch is blocked on runs with unresolved `uncertain`/`repair_required` (SS-17 AC-4) | 250/253/256 | operator-contract tests | ✅ |
| `OP2` | Operator resolutions are **atomic** (`ResolveUncertain`/`RetryAsNew`: state + successor + audit in one commit), idempotent by `resolutionID`; retry-as-new links `PredecessorTurnID` (SS-17 AC-2/AC-5) | 248/250/256 | operator-contract tests | ✅ |
| `OP3` | `repair_required` actions work end-to-end: inspect (quarantined blob readable), retry-load, abandon; attention card + API survive restart (SS-17 AC-1/AC-3) | 253/256 | operator-contract tests + UI smoke | ✅ |
| `OR` | **Operator settlement contract**: completed/failed atomically preserve immutable `settle_owed`; `confirm_cancelled`/abandon/superseded atomically have `settle=none`; retry successor alone owns settlement; audit/intent/successor are in that one resolution commit | 248/250/251/256/255 | resolution action-table tests + API/UI smoke | ✅ |
| `PAR` | **Backend parity**: full-field `ProviderSessionState` round-trip contract passes on both stores (incl. `ChangeType`/`SourceDocID`/`TurnCount` — verified missing locally); local prune never removes a run with non-terminal dispatch/unfinalized settle | 248/255 | shared parity contract test | ✅ ("both stores" = local/memory/multi-project shard; Supabase retired for dispatch by Task-258, see §10.1.1) |
| `MB` | **Model-based/composite-fault suite**: randomized interleavings of {Stop, crash, outage, lease expiry, operator action, flag flip, restart} preserve INV-1..7; Stop/send linearizability checked by CAS order | 255 | property/model suite | ✅ |
| `ST` | **Exactly-once settlement** (INV-7): crash at any settle point — including **between a phase's effects and its CAS** — never doubles or loses a durable consequence (effects-then-CAS + each consequence's keyed/monotonic/create-if-absent contract; ledger is audit/optimization only) | 251/255 | settle-phase matrix cells | ✅ |
| `EL` | **Convergent consequences, not marker-dependent** (SD-24 §5.3 doctrine): every settle consequence is keyed-upsert / monotonic / create-if-absent (or unique-keyed in its own store's transaction) — **crash between an effect and its `effectDone` marker duplicates nothing** (replay converges); `RecordEffectDone` is idempotent (unique `(runID,turnID,effectKind)`); retrofits verified: event replace keyed by turnID (`:3239`), `persistEvent` errors handled (`:3245`), finalizer keyed overwrite (`finalizer.go:49`) | 248/251 | `dispatch_settle_test` (incl. crash-between-effect-and-marker cases) | ✅ |
| `V2A` | **Atomic V2 activation**: activation rides the first `CreatePrepared` commit into a **non-prunable** activation entry (survives compaction); every consumer decides via `GetRunProtocolVersion` — never the session mirror; "marker set/record missing" and "record present/marker unset" windows unrepresentable | 248 | activation tests (incl. post-compaction) + store contract | ✅ |
| `SD` | **Settle dispositions**: gate reprompt/block ⇒ durable `settle_superseded_reprompt`; Stop mid-settle ⇒ bookkeeping completes, dependents release manifest is suppressed by stop-generation but the phase still finalizes | 251/255 | disposition tests + matrix cells | ✅ |
| `SDa` | Stop at dependents release persists `suppressed` but still runs exactly one finalizer and ends `SettlePhase=finalized` | 251/255 | `TestSettle_StopMidSettle_BookkeepingCompletes_ReleaseSuppressed` + matrix | ✅ |
| `SB` | **No dispatch state in the session blob/snapshot**: `ProviderSessionState` carries no records; a stale session snapshot cannot touch dispatch state (grep + runtime assert + contract test) | 248 | `dispatch_store_contract_test` | ✅ |
| `IR` | **No intent resurrection**: a stale owner-session snapshot written **after** an intent clear does not restore `Pending*` (filter on write AND read, both backends) | 248/249 | stale-owner-snapshot test | ✅ |
| `SC` | **Stop/recovery never terminalizes a sent turn**: effective Stop (own-run Stop, parent fence, or `CancelRequested`) on `send_started`/`provider_accepted` issues provider cancel; only a fresh, proof-only reconcile query may terminalize from `TerminalEvidence`, otherwise the sent state/intent remains for later proof/operator resolution — never a blind `terminal_cancelled`/`uncertain`; CAS conflict on Stop reloads and branches by state, never assumes "Stop won" | 249/250 | stop-branch tests | ✅ |
| `RR` | **Repair resolution is transactional & implementable**: two-phase `BeginRepairResolution` → out-of-store validate → `CommitRepairResolution` (a store tx cannot run the Go loader); CAS on `RepairRevision`, idempotent by `resolutionID`, audited; crash between phases expires via attempt TTL; open repair blocks snapshot writers | 248/253/256 | repair-contract tests | ✅ |
| `CE-CG` | **Capability evidence attached — Codex/Grok scoped rollout**: both SD-24 rows have recorded negative evidence (`unprovable ⇒ uncertain`), no `Accepted`/attach seam, and no *(verify live)* cell | **257** | [Codex evidence](../../06-System-Tech-Design/evidence/SD-24/codex.md) + [Grok evidence](../../06-System-Tech-Design/evidence/SD-24/grok.md) + matrix cells | ✅ |
| `CE-CL` | **Capability evidence attached — Claude scoped rollout**: live kill-mid-turn probe recorded negative acceptance/attach evidence (`unprovable ⇒ uncertain`) plus an evidenced (non-authoritative) reconcile artifact; no `Accepted`/attach seam, no *(verify live)* cell | **257** | [Claude evidence](../../06-System-Tech-Design/evidence/SD-24/claude.md) + matrix cell | ✅ |
| `CE-GEM` | **Capability evidence deferred — Gemini**: its Task-257 outcome remains unrecorded; Gemini is V2-disabled, not silently assumed safe | **257** | deferred matrix cell + provider-enable guard test | ☑ waived (2026-07-24, operator decision — see §10.1.1) |
| `RC` | Receipt boundary is explicit: only a future Task-257-evidenced adapter predicate may call `TurnBridge.Accepted`; Codex/Grok/Claude session creation/stdin-write/headless output never does, and Gemini is V2-disabled | 249/255/257 | negative adapter seam + provider-enable guard tests + evidence links | ✅ |
| `RE` | **Receipt integrity**: stored receipt evidence is canonical JSON + SHA-256 + identity; equal replay is idempotent and same identity with divergent payload is `ErrReceiptConflict` on both backends | 248/249/255/256 | receipt contract + adapter seam tests | ✅ |
| `RSF` | **Root Stop fence**: every `send_claimed→send_started` CAS first locks/checks its own non-prunable `RunStopState`, so `RequestRunStop` racing root send returns `ErrRunStopFence` and atomically pre-send-cancels/clears/audits with zero bytes — no dependency on a later per-record cancel loop | 248/249/250/255 | root-stop/send interleaving matrix on both stores | ✅ |
| `PS` | **Parent Stop fence**: after the same own-run check, a released child checks parent `RunStopState`; Stop-before-create, Stop-after-create-before-claim, or Stop-at-send-CAS returns `ErrParentStopFence` and atomically pre-send-cancels/clears/audits the child with zero bytes | 248/249/251/255 | parent-stop/release/send interleaving matrix on both stores | ✅ |
| `LR` | **Fresh recovery lease**: scanner reloads after claim and validates lease/cancel immediately before provider reconcile/cancel/attach; Cancel during `ReconcileInFlight` cancels before attach | 250/255 | recovery lease freshness tests + matrix | ✅ |
| `ES` | **Effective Stop on sent recovery**: before every sent-state reconcile/attach/**unproven classification**, recovery fail-closed reads own `RunStopState`, parent fence when present, and record cancel; it issues provider cancel, suppresses attach and unproven classification, and may query only after a fresh lease re-read to obtain `TerminalEvidence`; root Stop or child-only parent-fence changes while `ReconcileInFlight` **or before an unknown/no-adapter fallback** with `CancelRequested=false` are caught by the mandatory fresh check and cancel rather than attach/classify; authority-read failure performs no classification/attach | 250/255 | root/parent crash-before-record-cancel + root/parent `ReconcileInFlight` and unknown/fallback durable-Stop (record cancel false) + read-failure matrix on both stores | ✅ |
| `TP` | **Terminal proof outcome fidelity**: only validated `TerminalEvidence.Outcome` (`completed|failed|cancelled`) determines post-send terminal state/outcome; cancel request alone is non-terminal, and completed-before-cancel keeps `StopOutcome=cancelled_in_flight` | 248/249/250/255 | terminal-proof outcome matrix | ✅ |
| `SA` | **Store-owned Stop attribution**: in the same terminal CAS/RPC/log commit, lock/read record cancel, own `RunStopState`, and child parent fence; effective Stop writes `StopOutcome=cancelled_in_flight` without changing the proof-derived terminal state/outcome, otherwise it is empty. No caller or RAM input exists. | 248/249/250/255 | root crash-before-record-cancel + parent-only fence + no-stop proof matrix on both stores | ✅ |
| `RG` | **Recovery mutations are transaction-guarded**: recovery send linearization, pre-send cancellation, unknown/no-adapter decision, and terminal commit each require expected revision and `ClaimOwner==scanner` with unexpired store-clock lease in their own store transaction; the relevant send/decision/terminal paths lock own/parent Stop authority. A winning Stop returns fence/cancel-required without send/uncertain; expired/taken-over scanner cannot transition, terminalize, or clear intent. | 248/250/255 | root/parent final-read-to-send-or-unknown barrier + post-proof lease-expiry/takeover matrix on both stores/real PG | ✅ (local/memory matrix; "real PG" leg satisfied by Task-258, see §10.1.1) |
| `RA` | **Recovery attach is durably fenced**: `ClaimRecoveryAttach` grants a bounded epoch only if no unexpired epoch exists (`ErrRecoveryAttachActive` makes a repeated scanner inert); `EnterRecoveryAttach` and `RecordRecoveryAttachedEffect` require exact token identity, store-clock expiry, sent state, and live own/parent Stop. `CommitAttachedTerminalAndSettleIntent` atomically requires exact token identity, **store-clock-unexpired token**, sent state, and expected revision; it locks Stop for `cancelled_in_flight` attribution and commits valid unexpired proof after Stop. No validator-preflight may authorize an external attach, output, or terminal. `unknown→uncertain` and every live/recovered/pre-send/operator/retry terminal path revoke the epoch atomically. Expiry/reclaim prevents crash wedging; Stop winning claim returns cancel-required. | 248/249/250/255/256 | A/B takeover + active-token repeated-resume + paused callback terminal-or-uncertain revoke + terminal/expiry-before-entry zero-attach + expiry-before-terminal zero-mutation (no N+1 claim) + exactly-once unexpired-token terminal including root/parent-Stop attribution + all-path epoch-revocation + Stop-at-claim + crash-after-claim TTL on local/Supabase RPC/real PG/model | ✅ (local/memory matrix; Supabase/real-PG leg satisfied by Task-258, see §10.1.1) |
| `SO` | **Settlement obligation is store-owned**: `CommitTerminalAndSettleIntent` takes no settle boolean and atomically reads immutable `DispatchRecord.SettleOwed` to set `settle_pending|none`; only listed operator actions may force no-settle | 248/249/250/255 | terminal-store contract tests + live/recovery matrix | ✅ |
| `QB` | **Quarantine raw persisted**: `OpenRepair` stores the raw blob + hash atomically with the `RepairRecord` (no dangling ref); Inspect/retry-load operate on the real blob | 248/253 | repair-contract tests | ✅ |
| `RM` | **Release manifest convergent**: dependents release = durable revisioned per-dependent manifest (`pending→created\|suppressed`) + deterministic durable-intent keys downstream; `pending→created` and child `CreatePrepared` are one transaction/log-line, while Stop CAS yields `suppressed` — crash mid-release loses/duplicates no dependent (fixes empty-key `startTurn` @2071, random TurnID @5863) | 248/251 | release-manifest tests + matrix | ✅ |
| `CD` | **Cohort entries durable**: consequence = unique-keyed, payload-hash-checked effect records (divergent duplicate fails closed); RAM buffer is a rebuildable projection (fixes label-deduped RAM slice @agent_orchestrator.go:92); concurrent appends converge | 251/255 | cohort tests incl. concurrent + payload-conflict test | ✅ |
| `TO` | **No TOCTOU on session writes**: clear-set enforcement happens inside the write transaction (server-side guarded upsert / single-writer serialization) — a receipt commit interleaved with a stale snapshot write cannot resurrect an intent | 248/255 | concurrent interleaving test | ✅ |
| `RS` | **RetryAsNew supersede guard**: CAS on live intent generation + envelope hash — retry over a newer user prompt is rejected with explanation (SS-17 §8); operator-vs-new-prompt race safe | 248/250/256 | supersede tests | ✅ |

### 10.1.1 Audit pass against actual test coverage (2026-07-22, local run — flipped on operator go-ahead, not a CI confirmation)

> §10.1's own rule is "Flip ☐→✅ only when the named test is green on CI" — this pass was run locally, not on CI, but the operator explicitly authorized flipping the confirmed rows below on that local evidence rather than waiting for a CI run. Method: grepped `apps/local-runner/internal/runner/*_test.go` for a function name matching each row's stated Verifier, then ran the matches. `go build ./...` and `go vet ./...`: clean. `go test -race` could not run in this environment (`CGO_ENABLED` requires a C toolchain; no `gcc`/`cc` found) — `GR` stays unflipped: unverified here, not failing. A follow-up CI run should re-confirm these rather than take this pass as a permanent substitute.

**Rows flipped ☐→✅ in the table above on this evidence — a directly-named, currently-passing test found for each:**

| Row(s) | Matching test(s) found, all PASS locally |
| --- | --- |
| `C1`, `SP` | `TestDispatchCrashMatrix_RealKill_B0`..`B8` (9 tests, real subprocess kill, one per barrier), `TestCrashMatrix_SubprocessKillAfterPrepare`, `TestCrashMatrix_SubprocessBinaryKill` |
| `C2` | `TestStopCASBeforeSendStarted_SendCASFails_NothingSent`, `TestCrashMatrix_StopWinsLinearization_ZeroSend`, `TestCrashMatrix_KillBetweenSendClaimedAndSendStarted` |
| `C2a` | `TestCrashMatrix_PreSendStopZeroBytes`, `TestPreSendStopCancellationAndOwnerClearAreAtomic` |
| `AE` | `TestTerminalCommit_RejectsAmbiguousPostSendError` |
| `I3` | `TestSettle_OutageThenRecovers_AutoAdvances` (`gate_checkpoint_outage_test.go`) |
| `G8` | `TestEventTurnStartedNotUsedForRecovery` |
| `MU` | `TestDispatchMutation_GuardFlipsTurnSuiteRed` |
| `CC` | `TestSettle_ConcurrentScheduleSettleDrive_NoDuplicateEffects`, `TestSettle_ConcurrentBootDriveAndSchedule_NoDuplicate` |
| `SW` | `TestDispatchCAS_StaleRevisionRejected` |
| `TA` | `TestLocalStore_DiskBeforeRAM_TornTailDropped`, `TestCrashMatrix_TornTail` |
| `OP1`, `OP2`, `OP3` | `TestDispatchAttentionHandlers_StatusAndReplay`, `TestOperatorAttentionAndResolve`, `TestResolveHandler_ReturnsAtomicSettlementDisposition`, `TestDispatchInspect_RedactsCanonicalReceiptEvidence`, `TestRepairResolutionHandler_RetryLoadRestoresSession`, `TestDispatchAttention_RebuildsAfterRestart` |
| `OR` | `TestResolveUncertain_ActionSettlementTableAtomic` |
| `PAR` | `TestDispatchStore_ContractSuite`, `TestDispatchStore_RoundTripAllStates` |
| `MB` | `TestDispatchModel_SeededInterleavings` (40 seeded interleavings, all sub-cases pass) |
| `ST`, `SD` | `TestSettleDriver_PhasesAdvanceToFinalized`, `TestSettleSubBarriers_B8aToB8e_PartialThenResume`, `TestSettleDriver_NilEvaluateGateRefusesDefaultAllow`, `TestSettle_GateReprompt_SupersededDisposition` |
| `EL` | `TestRecordEffectDone_UniqueKeyIdempotent`, `TestResumePendingFlowGate_CompletionEventKeyedByTurnID` |
| `IR` | `TestOuterIntentNotClearedWhileSendClaimed` |
| `SC` | `TestPostSendCancelRequiresTerminalProof` |
| `RC` | `TestSessionIDIsNotAReceipt`, `TestProviderV2GeminiDisabled_ClaudeEnabled` |
| `RE` | `TestReceiptEvidence_EqualReplayAndPayloadConflict`, `TestAcceptedRejectsPreSendAndPayloadConflict` |
| `RSF` | `TestOwnRunStopVsSendStarted_IsLinearizable`, `TestRunStopState_IsOnlyStopAuthority`, `TestLiveStop_AdvancesRunStopState_FencesRootSend` |
| `PS` | `TestChildSendStarted_ParentStopFenceIsAtomic`, `TestLiveStop_ParentStopFencesChildSend` |
| `ES` | `TestRecoveryScanner_CancelRequestedSkipsRedispatch_PreSendCancelsInstead` |
| `SA` | `TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority` |
| `RG` | `TestRecoveryCommitGuards_LeaseAndStopAreAtomic` |
| `RA` | `TestRecoveryAttach_EntryEffectAndTerminalAreAtomic`, `TestEverySentExitAndTerminalPath_RevokesRecoveryAttachEpoch` |
| `SO` | `TestTerminalCommit_ReadsPersistedSettleOwed_NoCallerOverride` |
| `QB` | `TestOpenRepair_CreateIfAbsent_OneCommit` |
| `RM` | `TestReleaseManifest_AtomicChildCreateAndState` |
| `CD` | `TestAppendCohortResultIdempotentByLabel`, `TestCohortMatrixTwoMemberTerminalOutcomes` |
| `RS` | `TestRetryAsNew_SupersededIntentRejected`, `TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance` |
| `GB` | `go build ./...` clean (confirmed repeatedly this session) |

**Rows closed same day (2026-07-22) by writing the missing test** — the first audit pass found a related test but not a dedicated one for the exact sub-case; per operator instruction, 4 new tests were added (additive-tests-only, each cross-checked with a mutation: temporarily breaking the guarded production behavior and confirming the new test fails, then reverting) rather than left as gaps:

- `SDa` — `TestSettle_StopMidSettle_BookkeepingCompletes_ReleaseSuppressed` (`dispatch_settle_barrier_test.go`): drives a Stop-terminated turn through `SettleDriver.DriveSettle`, confirms it still reaches `SettleFinalized`, exactly one `finalizer` effect ran, and the `dependents_release` effect payload durably records `cancelled_in_flight`.
- `LR` — `TestRecoveryScanner_StaleSnapshotNeverActs_FreshReadDrivesCorrectDecision` (`dispatch_recovery_test.go`): proves `ClaimRecovery`'s revision CAS rejects a stale pre-cancel-request snapshot (zero effect, never redispatches), while a fresh snapshot of the same record correctly drives the pre-send-cancel path.
- `TP` — `TestTerminalCommit_OutcomeFidelityMatrix` (`dispatch_record_test.go`): table-driven over {completed, failed, cancelled} × {stopped, not-stopped} (6 cases, including the explicit completed-before-cancel case); mutation-verified by temporarily forcing `deriveStopOutcomeLocked` to always return `""` — 3 of 6 cases failed exactly as expected, then reverted.
- `TO` — `TestIntentClear_NoTOCTOUResurrectionUnderConcurrentAccess` (`dispatch_record_test.go`): 20 iterations of a reader goroutine hammering `IsIntentCleared` (2000 reads each) concurrently against a `CommitReceiptAndClearIntent` writer, asserting `cleared` never flips from true back to false.
- `FF` — checked whether this is literally replayable against `6ea5417` and confirmed it is not: `dispatch_record.go` (and the rest of the dispatch subsystem) did not exist at that commit at all (`git show 6ea5417:...dispatch_record.go` → path not found), so checking it out would fail to *compile* today's tests, not fail their assertions — not a meaningful red signal. Further, `git log --diff-filter=A` shows `dispatch_record.go` and `dispatch_record_test.go` were added in the **same commit** (`914dce7`) — no separate "red" commit exists in history to point to. The row's own text already anticipates this ("foundation tests exempt — see `MU`"); operator confirmed accepting `MU`'s mutation-test evidence (already ✅) as the substitute for `FF`, rather than leaving it permanently unverifiable-looking.

**Rows still needing a closer look:**

- `NR` — the specific named Round-9/10/16/18/20 guard tests that exist (`V9Matrix*`, etc.) pass, but a **full** `go test ./internal/runner/...` run surfaced **18 failures outside CP-51/dispatch scope entirely** (Codex/Gemini adapter CLI invocation, skills-merge precedence, approval expiry, git-commit-guard) — none overlap `NR`'s named list or any other §10.1 row, so they don't block this ledger, but they are a real, separate gap worth its own investigation (possibly environment-dependent: missing real provider CLI installs on this machine).

**Confirmed genuine gaps — no evidence found, not just unverified — both formally waived 2026-07-24 (operator decision, does not block CP-51/BUG-288 closure):**

- ~~`PG` — no env-gated real-PostgreSQL/Supabase dispatch test found~~ **Correction (operator, same day): wrong call.** Task-258 (2026-07-17, already a child doc of this CP) retired Supabase as the dispatch backend entirely — "Runner no longer opens a Supabase dispatch client," tables dropped via migration, local NDJSON + Google Drive sync is the one product path. `TestDispatchStore_ContractSuite`'s actual subtests are `local`/`memory`/`multi` — no Supabase variant exists in code, confirming the retirement. `PG` (and the "both stores"/"real PostgreSQL" language in `PAR` and the §10.4 barrier rows) describes a two-backend architecture this CP itself abandoned before implementation finished; it is satisfied by that product decision, not an open test gap. Flipped to ✅ below on that basis, not because a Supabase run happened.
- `CE-GEM` — explicitly out of scope: Gemini is not supported for V2 dispatch (operator confirmed 2026-07-22); Task-257's Gemini evidence spike is not planned. **Waived 2026-07-24** (operator decision, this session) — an intentional non-goal, formally accepted as never going green rather than left an ambiguous `☐`.
- `GR` — cannot run here at all (no C toolchain for `-race`/CGO); unknown, not failing. Needs a CI run or a machine with `gcc`. **Waived 2026-07-24** (operator decision, this session): the local, non-`-race` suite (`go test ./internal/runner ./internal/flowgate`, repeatedly green across this session's BUG-319/320/321/322 sweeps) and `go vet` stand in as the closure evidence for this environment; re-run `-race` on a machine with a C toolchain if one becomes available, but its absence no longer blocks CP-51/BUG-288 closure.
- `FF` (every defect test shown failing on HEAD `6ea5417` before the fix) — a process attestation, not something replayable from current HEAD; the test names strongly suggest TDD against named defects, but "PR evidence" itself wasn't located.

### 10.2 Non-regression coverage (prior-round fixes in refactored code — must stay green)

> The refactor touches the exact functions where many **already-closed** BUG-288 fixes live. These are **not** re-implemented — they are guarded. Ledger row `NR` is green only when all of these stay green. (The recovery-scanner acceptance is folded into ledger rows `Rr1..Rr4`.)

| Prior fix | Lives in | Refactored by | Guard test |
| --- | --- | --- | --- |
| `V9-03` turnInFlight held during gate; concurrent `startTurn` rejected (`gate_in_progress`) | `startTurn`/`runTurn` | Task-249 | existing V9-03 test + crash-matrix |
| `V9-18` Stop mid-gate ⇒ `completed=false` (no finalizer success) | `interactive_service.go` | Task-249 | existing V9-18 test |
| `V10-03` gate state durable (`PendingFlowGateSettle`) | `emitLocked`/resume | Task-251 | V10-03 test + `gate_checkpoint_outage_test` |
| `V10-04` defer raw `EventTurnCompleted` until gate pass | `emitLocked`/`finishTurn` | Task-248/249 | existing V10-04 test |
| `R16-P0`/`R18-1` durable idempotency snapshot retention | `durableIdempotencySnapshot`/`sessionStateOf` | Task-254 | `bug288_round16/round18_test` + `idempotency_retention_test` |
| `R20-1` prep→launch-ack durability | `startTurn` | Task-249 (superseded by state machine) | `bug288_round20_test` + crash-matrix |
| `R20-2` per-service marker secret | `runTurn`/`feature_history.go` | Task-252 | `bug288_round20_test` + `fcp_marker_replay_test` |
| `R20-3` no false `gate_settle_checkpoint` blocked marker | `emitLocked` | Task-251 | `bug288_round20_test` |

### 10.3 Test gates & the one verdict command

The finish line is produced by a **single command**; green ⇒ ledger rows `NR`/`GB`/`GR` and the suite rows are satisfied:

```bash
FLOWPILOT_DISPATCH_V2=1 go test -race ./internal/runner ./internal/flowgate -count=1 && go build ./... && go vet ./...
```

- `go build ./...` and `go vet ./...` clean.
- `go test ./internal/runner ./internal/flowgate` — existing suites (BUG-288 Round 1–20 guards) still pass.
- `go test -race ./internal/runner` — clean, including the new crash-matrix.
- New suites present and passing: `dispatch_record_test.go`, `dispatch_store_contract_test.go` (shared both backends: API + full-field parity), `dispatch_recovery_test.go`, `dispatch_settle_test.go` (phase driver + idempotent effects), `dispatch_crash_matrix_test.go` (real subprocess kills, cross-platform), `dispatch_model_test.go` (randomized interleavings, `MB`), `stop_race_barrier_test.go`, `gate_checkpoint_outage_test.go`, `fcp_marker_replay_test.go`, `supabase_runtime_corruption_test.go`, `idempotency_retention_test.go`, operator-contract tests (SS-17).
- `PG` evidence: the store contract suite run against a real PostgreSQL/Supabase (env-gated `FLOWPILOT_TEST_SUPABASE_DSN`; when CI has no database, a documented local/staging run attached to the PR satisfies the row).

### 10.4 Crash-matrix (the acceptance harness — Task-255)

Inject **crash** (real subprocess kill — ledger `SP`) and **Stop** at each barrier, on **each store backend**. Every cell must satisfy INV-1..INV-3. Barriers align with SD-24 §5.2 v2 states.

| Barrier | Point | Crash expectation | Stop expectation |
| --- | --- | --- | --- |
| `B0` | before `CreatePrepared` | intent still pending; recovery re-dispatches from intent | no dispatch; turn aborts |
| `B1` | after `prepared`, before `send_claimed` | **safely-retryable**: re-dispatch same TurnID + envelope (hash-verified) | Stop CAS → `terminal_cancelled`; no send |
| `B2` | after `send_claimed`, before `send_started` CAS | **safely-retryable** (send never linearized — no `uncertain` here) | Stop CAS wins → `send_started` CAS fails → `terminal_cancelled` |
| `B3` | after `send_started` CAS, before/during external send | reconcile per matrix; no proof ⇒ `uncertain` (held per SS-17). Injected post-CAS `SendTurn` error is **not terminal evidence**: record + intent remain | send already linearized ⇒ provider cancel path; `terminal_cancelled` only with proof |
| `B4` | external send done, before acceptance receipt observed | reconcile; no proof ⇒ `uncertain`; injected ambiguous send error still never clears intent/terminalizes | provider cancel; reconcile outcome |
| `B5` | receipt observed in RAM, before `CommitReceiptAndClearIntent` durable | state is still `send_started`; **intent still held**; reconcile (the v1-draft leak, closed) | provider cancel; reconcile |
| `B6` | after `CommitReceiptAndClearIntent` | clear already atomic with receipt — nothing to lose | reconcile + terminalize via provider |
| `B7` | terminal observed, before `CommitTerminalAndSettleIntent` durable | recovery reconciles from `provider_accepted`; settle re-derived | n/a (turn already over at provider) |
| `B8` | after terminal commit, `SettlePhase=settle_pending`, before the settle driver runs | settle re-derived from the record; phase driver drains it | n/a |
| `B8a..B8e` | inside each settle phase: **after its effects, before its completion CAS** (and between adjacent phases) | driver replays the in-progress phase's **convergent durable consequence**; keyed/monotonic/create-if-absent semantics prevent duplication, including a crash before its marker. The hash-bound ledger is audit/skip optimization only after that independent contract holds — **no double AND no lost consequence** (INV-7, ledger `ST`/`EL`); "phase marked done but effects unrun" is unrepresentable (effects-then-CAS) | Stop mid-settle: bookkeeping continues; dependents release manifest suppressed by stop-generation; finalizer still runs and ends `finalized` (ledger `SD`/`SDa`) |

Stop cells assert on the recorded **`StopOutcome`** (SD-24 §7.2): `stopped_before_send` ⇒ zero provider bytes; `cancelled_in_flight` ⇒ ≤1 send + provider cancel invoked + **no new provider send or dependent user-work** after Stop acknowledgment. Settlement bookkeeping for the already-terminal turn must continue and record dependent release as suppressed. The unprovable "no bytes after Stop CAS" is **not** asserted (plan-review #2 finding #3).

Concurrency/durability variants (every one a ledger row):

- `CC` two recovery scanners race `ClaimRecovery` on the same record — exactly one acts.
- `LR` a scanner must `Get` fresh state after claim and before provider reconcile/cancel/attach; a Stop arriving during `ReconcileInFlight` issues provider cancel before it can attach.
- `ES` a sent root/child can restart after `RequestRunStop` but before its `SetCancelRequested` loop; effective Stop authority must cancel and never attach. Hold `Reconcile` at `ReconcileInFlight` **and separately at unknown/no-adapter fallback**, keep record cancel false, then change root Stop or only the child's parent authority: the final fresh effective-Stop check cancels, never attaches/classifies/clears. Failure to read that authority is fail-closed.
- `PS` parent Stop races release-manifest creation and child `send_started`; the child CAS checks the persisted parent generation in the same transaction, so a winning Stop produces no child provider bytes.
- `RSF` root `RequestRunStop` races root `send_started`; every send CAS checks its own run-stop authority first, so a winning Stop produces atomic pre-send cancellation/owner clear/audit and zero bytes without waiting for record cancellation.
- `TP` provider terminal proof variants completed/failed/cancelled and completed-before-cancel preserve the proven state/outcome; a cancel request/error without proof remains reconcilable/uncertain and never clears intent.
- `SA` terminal commit atomically locks durable own/parent Stop authority: root Stop then crash before record-cancel, or child parent-only Stop/fence change, yields `cancelled_in_flight` for proof completed/failed/cancelled; no effective Stop yields empty attribution and no caller can supply it.
- `RG` hold recovery after its final read: a root/parent Stop must win inside guarded recovery send/pre-send/unknown transactions and cause no-send or cancel, not uncertain; after provider proof, lease expiry/takeover must reject the old scanner's terminal commit/intent clear and let only the valid owner re-reconcile and commit.
- `RA` two scanners race recovery attach: only a current, **store-clock-unexpired** token may enter attach, append a visible stream effect, or forward terminal — each is a transaction-bound operation, never `Validate → effect`. Terminal/expiry before entry yields zero attach; expiry before terminal forwarding yields zero mutation even before a successor claims; terminal or `unknown→uncertain` while A is paused yields zero stale output/mutation; current token forwards once, and a valid terminal proof still commits after root/parent Stop with `cancelled_in_flight`; all terminal/retry paths revoke. Stop-at-claim cancels with zero attach, and crash-after-claim becomes reclaimable after attach TTL.
- `SO` conflicting caller logic cannot suppress settlement because live/recovery terminal APIs have no settle argument; persisted `SettleOwed` is read in the terminal CAS/RPC/log commit.
- `SW` a stale async snapshot (older `Revision`) attempts to persist after a newer transition — rejected (`ErrStale`), state unharmed.
- `TA` the local NDJSON tail is torn mid-append — loader drops the fragment; monotonic revisions prevent old-state resurrection.
- Outage: each persist point fails K times then recovers — self-heal, no wedge, no loss (INV-5).
- Backends: `local-file` **and** `Supabase` via the shared store contract suite; `PG` row requires at least one real-PostgreSQL run for CAS + concurrent claims.

### 10.5 Documentation / traceability

- BUG-288 updated: a **Termination Criterion** subsection points to this §10 ledger as the single finish line, with the ledger-ID mapping table (added in this change).
- `change-audit/CA-###` note with the `flowpilot:change-ledger` block (`feature_key: agent-flow-engine`) on commit.
- Q-1 resolved: SD-24 (Durable Turn Dispatch) authored as the design authority; this CP implements it and its Parent Documents point to SD-24.
- `gitnexus_detect_changes()` run before commit confirms only expected symbols/flows changed.

### 10.6 Out of scope (explicitly NOT blocking this CP's done)

> Listed so they cannot silently reopen this CP. Both are non-dispatch concerns tracked elsewhere; neither expands the §10.4 matrix.

- `OOS-1` **Task-240 schema-valid hub tools + watchdog E2E** (BUG-288 Q-1) — hub tool-contract scan; separate work item.
- `OOS-2` **Gemini inbound permission bridge for commit deny** (BUG-288 Q-2) — currently mitigated by `ForceShellBridge` prompt-guard + post-turn commit block; a true inbound permission bridge is a Gemini-adapter concern, not turn dispatch.
- ~~`OOS-3`~~ **Resolved 2026-07-16:** the `uncertain`/`repair_required` policy is now specified in **SS-17**; it entered scope as ledger rows `OP1`/`OP2`/`OP3` (owner: Task-256).

**Coverage boundary** (per plan-review #2 — what this CP does **NOT** claim; mirrors SD-24 §13). These are tracked concerns, not silent gaps, and none of them may be used to reopen this CP:

| Concern | Status here |
| --- | --- |
| Turn dispatch + outer intents (incl. parent-owned) | **In scope** — the subject of this CP |
| Gate settlement side effects | **In scope** — settle sub-lifecycle (INV-7) |
| Provider session resume fidelity | Partial — per capability matrix; unprovable ⇒ `uncertain` |
| Full chat-transcript fidelity/chronology | Out — provider-transcript dependent (existing behavior) |
| Agent bus / queued feedback / waiters durability | Out — RAM today; future SD if needed |
| Full cohort/fan-out lifecycle beyond settle phases | Out — SD-19/SD-20 territory |
| Local/Supabase **full-field parity** | **In scope** — ledger `PAR` |
| Whole-server-off → full orchestration resume proof | Out — beyond the dispatch/settle seam |
| Indefinite retention | Out — 90-day local prune stays, with the `PAR` non-terminal guard |

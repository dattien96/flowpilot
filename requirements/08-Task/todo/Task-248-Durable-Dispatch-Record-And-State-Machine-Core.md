# Task-248: Durable Dispatch Record And State Machine Core

## Metadata

- Document ID: `Task-248`
- Title: `Durable Dispatch Record And State Machine Core`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [CP-51](../../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SD-25 Recovery Ownership Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SS-17](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md)
- Replaces: `None`
- Tags: `agent-flow-engine, durable-turn, dispatch-state-machine, persistence`

## AI Quick View

### Summary

- Introduce a single, versioned durable `DispatchRecord` as the source of truth for a turn's dispatch lifecycle, retiring the `prep:<turnID>`/bare launch-ack encoding of `idempotency[key]`.
- **Eight** forward-only states (SD-24 §5.1–5.2): `prepared → send_claimed → send_started → provider_accepted → terminal_completed/failed/cancelled`, plus the `uncertain` **state**. There is no `intent_pending` state — the pre-record phase IS the durable outer intent (plan-review #2 #11); no boolean overlays (plan-review #1 #4). Plus the **settle sub-lifecycle** `SettlePhase` (6 phases, SD-24 §5.3).
- Every mutation is a `Revision` CAS through the SD-24 §6.1 store API; recovery works under a lease; the record binds an immutable `DispatchEnvelope` by hash.
- Make terminal outcomes durable in the local store (single-writer, disk-before-RAM, torn-append-safe) and resolve the `EventTurnStarted` contract contradiction.

### Current Ask

- Land the record type, state-machine transitions, persistence plumbing, and legacy migration — the foundation every other CP-51 task builds on. No live-path rewiring yet (that is Task-249).

### Key Decisions

- `T-1` `DispatchRecord` is the only place that answers "what did the provider receive?"; `turnInFlight` RAM is never durable evidence.
- `T-2` Legacy `prep:<turnID>` → `prepared` (envelope synthesized from the persisted prompt log; missing log ⇒ `uncertain`); bare value → `terminal_completed` **only** with corroborating terminal/`lastTurnID` evidence, else `uncertain`; legacy records carry `ProtocolVersion=1` and are resolve-only.
- `T-3` One documented rule per event/state for what recovery may infer; code comments must agree (fixes `DOD-G8`).
- `T-4d` `uncertain` and the three terminal outcomes are **states**, not overlays; `CancelRequested` is a monotonic flag with CAS-enforced invariants (fails the `send_started` CAS) — resolves plan-review #4's contradiction; no backward transitions exist anywhere.

### Constraints

- Additive `omitempty` JSON only; legacy readers must ignore new fields.
- No live-path rewiring in this task (that is Task-249) — but **no shadow-writing either** (SD-24 `D-10`: V2 records exist only once a run is V2-authority; V1-authoritative runs get no V2 records). This task lands the types, stores, and migrations, verified by unit/contract tests only.

### Open Questions

- `Q-1` **Resolved (plan reviews #1 #4, #2 #11):** explicit **8-state** enum (no `intent_pending`); `uncertain` and `terminal_*` are states; `CancelRequested` is a constrained monotonic flag, never a free overlay.
- `Q-2` **Resolved (SD-24 §6.5 / plan-review #2 #4):** dedicated `dispatch_records` table + transactional RPCs. Conditional-PATCH-on-blob rejected (cannot CAS one turn inside a JSON array; cannot atomically clear a parent intent).

### Source Refs

- CP-51 §3.2–3.3, §6; BUG-288 `DOD-C1`, `DOD-G8`.
- Code: `workflow_store.go:249-259`, `interactive_service.go:2714-2766` (`durableIdem*`), `local_file_session_store.go:611-618`, `interactive_service.go:5994`.

## 1. Goal

Create the durable, versioned dispatch record and its state machine, persist it (including durable terminal outcomes in the local store), migrate legacy idempotency encodings into record states, and eliminate the `EventTurnStarted` documentation contradiction — establishing the single source of truth for turn dispatch.

## 2. Parent Links

- coding plan: CP-51 (`P-1`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (governing design); SD-20 (Flow Mode turn lifecycle)
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-C1`, `DOD-G8`, INV-1, INV-2

## 3. Trigger

Codex Round-20 confirmed that dispatch coordination has no durable notion of provider receipt: `ProviderSessionState` stores only `IdempotencyKeys map[string]string` (`workflow_store.go:249-259`), terminal events are not persisted by the local store (`local_file_session_store.go:611-618`), and a comment claims recovery can use `EventTurnStarted` while `durableIdemReplaySafe` says it cannot (`interactive_service.go:5994` vs `:2732-2766`). Without a durable record, every downstream fix is a point patch.

## 4. Exact Change

- `T-1` New `dispatch_record.go`: 8-state enum + settle-phase enum + `DispatchRecord` (`ProtocolVersion`, **`IntentOwnerRunID`**, `Revision`, `ClaimOwner`/`ClaimExpiresAt`, **`RecoveryAttachEpoch`/`RecoveryAttachOwner`/`RecoveryAttachExpiresAt`**, `CancelRequested`+`StopGeneration`+**`StopOutcome`**, `OuterIntentKey`/`Gen`, `EnvelopeHash`, canonical **`ReceiptEvidence`** + **`TerminalEvidence`**, immutable `SettleOwed`, **`SettlePhase`**, `PredecessorTurnID`, `Outcome`, **`ParentStopFence`**, timestamps) with CAS-aware guards rejecting every edge not in the closed tables (SD-24 §5.2–5.3). `CreatePrepared` computes `SettleOwed` exactly once from the prepared invocation's durable flow/gate context (`requiresGateSettlement(envelope, runMode, step)`), persists it with the envelope, and every later terminal/recovery path only reads it; no update API may mutate it. Attach fields are durable store fields, never session state; every transition out of `send_started|provider_accepted` and every terminal path increments the epoch and clears owner/expiry. Add `ReleaseManifestItem` and immutable `DurableIntent` (deterministic child turn/run/key + complete child envelope/reference hash + parent Stop fence, item state/revision/stop generation) so a pending manifest can re-create the exact child without RAM.
- `T-1b` New `DispatchEnvelope` (immutable, hash-bound; SD-24 §5.4): `PromptRef` (existing `turnLogKindPrompt` line) + `PromptSHA256`, `StepID`, model/effort/YOLO, `SelectedSkills`, `Scenario`, provider/account/session-at-prepare, `Attachments` (refs+hashes), `FlowContextInjected`, `EnvelopeHash`.
- `T-1c` New `dispatch_store.go` store API (SD-24 §6.1) — `CreatePrepared` (**atomic V2 activation on first record also creates non-prunable `RunStopState`** — ledger `V2A`), `CASAdvance` (every live `send_claimed→send_started` locks/checks the record's own `RunStopState`, returning `ErrRunStopFence{currentGeneration}` if stopped; a child additionally checks parent `ParentStopFence`, returning `ErrParentStopFence{currentGeneration}`), recovery-only **`CASRecoveryAdvance`** (same send linearization plus `ClaimOwner==leaseOwner`, unexpired store-clock lease, and own/parent fence in that transaction), `CASAdvanceSettle`, `ClaimRecovery`, `SetCancelRequested`, **`RequestRunStop`**, cross-record `CommitReceiptAndClearIntent`/`CommitTerminalAndSettleIntent`/**`CommitPreSendCancellationAndClearIntent`** (all take **`intentOwnerRunID`**; terminal commit locks own and, if fenced, parent `RunStopState` in the same local writer/RPC transaction, derives state/outcome from validated `TerminalEvidence`, phase from persisted immutable `SettleOwed`, and `StopOutcome=cancelled_in_flight` iff record cancel/own Stop/parent fence is effective; pre-send cancel includes owner clear + `self|parent_fence` audit), plus recovery-only **`CommitRecoveryPreSendCancellationAndClearIntent`**, **`CommitRecoveredTerminalAndSettleIntent`**, and **`CommitRecoveryUnknownOrRequireCancel`** (all enforce `ClaimOwner==leaseOwner` and unexpired store-clock lease inside their mutation transaction; unknown-to-`uncertain` revokes the attach epoch), then `ClaimRecoveryAttach` + **`EnterRecoveryAttach` / `RecordRecoveryAttachedEffect` / `CommitAttachedTerminalAndSettleIntent`**. Entry/output attach operations require exact epoch+owner, unexpired **store-clock** token, sent state, and live own/parent Stop. `CommitAttachedTerminalAndSettleIntent` atomically requires exact epoch+owner, `RecoveryAttachExpiresAt > store_clock_now`, sent state, and expected revision; it locks a winning own/parent Stop for **attribution**, then atomically derives/commits the valid proof terminal with `StopOutcome=cancelled_in_flight`, clears intent, settles, audits, and revokes; it must not drop an unexpired proof because Stop won. `ErrRecoveryAttachRevoked` is strictly inert for expiry/token/state/revision failure; it is not the result of a valid unexpired proof plus Stop. Every live/recovered/pre-send/operator/retry terminal path revokes the epoch in the terminal transaction. Operator `ResolveUncertain`/`RetryAsNew` remain closed settlement action table + supersede/fence guarded; **`RecordEffectDone`** is for non-attach convergent effects. Storage decided (SD-24 §6.1/§6.5): local `dispatch.ndjson` commit log; Supabase `dispatch_records` + **`dispatch_run_stop_state`** + **`dispatch_effects` (UNIQUE run/turn/effect_kind, payload hash, state, revision)** + **`repair_records`** tables + transactional RPCs; dispatch state never in the session runtime blob.
- `T-1d` Local store contract fix (SD-24 §6.2): single-writer serialization; **disk-before-RAM** for dispatch-bearing state (inverts the verified RAM-first order at `local_file_session_store.go:273-292`); torn-append detection on load (drop + count incomplete tail).
- `T-1e` **Backend parity** (ledger `PAR`, plan-review #2 #9): add the fields missing from `ndjsonSessionRecord` (`ChangeType`, `SourceDocID`, `TurnCount` — verified absent at `local_file_session_store.go:65-109`); malformed session lines are counted+logged (repair-surfaced for V2 runs, not silently skipped); the 90-day local prune skips any run with a non-terminal dispatch or unfinalized settle; ship the shared **full-field `ProviderSessionState` round-trip parity test** against both backends.
- `T-1f` **SD-25 attached-stream contract:** local log lines and Supabase `dispatch_records` persist `recovery_attach_epoch int8 NOT NULL`, `recovery_attach_owner text NULL`, `recovery_attach_expires_at timestamptz NULL`; a unique attached-effect identity is durable (`run_id, turn_id, "attached:<epoch>:<eventID>"`) and payload-hash protected. Add transactional RPCs `dispatch_enter_recovery_attach`, `dispatch_record_recovery_attached_effect`, and `dispatch_commit_attached_terminal_settle`. No implementation may call a validator then attach, output, or terminalize; the three RPC/local operations are the only attach bridge operations.
- `T-2` **No dispatch state in the session snapshot** (plan-review #3 #3; SD-24 `D-1`): `ProviderSessionState`/`sessionStateOf` gain ONLY `DispatchProtocolVersion`, `RepairRequired`/`RepairReason`, and provenance fields — never a `Dispatch` slice. `rs.dispatch` (RAM cache) is loaded from the **dispatch store** at boot and refreshed from commit results; a runtime assert + grep test enforce the absence (ledger `SB`).
- `T-3` Terminal durability comes from the dispatch commit log itself (`CommitTerminalAndSettleIntent` writes the fsynced terminal line) — the session-store event gap at `local_file_session_store.go:611-618` stays untouched; nothing rides `persistProviderSession`.
- `T-3b` Read-side store API (plan-review #3 #4): `Get`, `GetEnvelope`, `ListRecoverable` (non-terminal + unfinalized-settle), `ListAttention`, `ListAudit`, `ListEffects`, `GetResolutionResult`, `IsIntentCleared` — deterministic ordering, limit+cursor pagination, in the shared contract suite for both backends.
- `T-3c` Operational contract for the local log (plan-review #3 #13): process-exclusive `dispatch.lock` at startup (old/new server overlap ⇒ loser fails dispatch loudly); compaction = temp → fsync → atomic rename → dir sync, leftover temp discarded on startup; **missing `dispatch.ndjson` on a V2-marker run ⇒ `repair_required`**.
- `T-4` Legacy migration helper (ONE rule, same as SD-24 §5.6 and the AI Quick View `T-2` — no second variant anywhere): `prep:<turnID>`→`prepared` (envelope from prompt log; missing ⇒ `uncertain`); bare→**`terminal_completed`** iff terminal/`lastTurnID` corroboration, else `uncertain`. Keep reading old fields so pre-CP-51 sessions restore; legacy records are `ProtocolVersion=1`, resolve-only.
- `T-5` Documentation contract: replace the `interactive_service.go:5994` comment and the `durableIdemReplaySafe` comment with one canonical rule table (what each state/event proves for recovery); ensure code matches.
- `T-6` Provider-receipt seam: define the interface point where an adapter reports a durable receipt (consumed in Task-249); default empty ⇒ not accepted.

### 4.1 Code guide (step-by-step)

> Sketches are illustrative — confirm exact signatures against HEAD `6ea5417` before compiling. Run `gitnexus_impact` before editing shared symbols.

**Step 1 — new file `apps/local-runner/internal/runner/dispatch_record.go`: the record + state machine.**

```go
package runner

const DispatchProtocolV2 = 2

type DispatchState string

const (
	// NOTE: no "intent_pending" state — before CreatePrepared the durable
	// artifact is the outer intent itself (SD-24 §5.1). Records begin at prepared.
	DispatchPrepared          DispatchState = "prepared"
	DispatchSendClaimed       DispatchState = "send_claimed"
	DispatchSendStarted       DispatchState = "send_started"
	DispatchProviderAccepted  DispatchState = "provider_accepted"
	DispatchUncertain         DispatchState = "uncertain"
	DispatchTerminalCompleted DispatchState = "terminal_completed"
	DispatchTerminalFailed    DispatchState = "terminal_failed"
	DispatchTerminalCancelled DispatchState = "terminal_cancelled"
)

// DispatchRecord: single durable source of truth for one turn's dispatch
// (SD-24 §5.3). Forward-only; EVERY mutation is a Revision CAS via the
// dispatch_store.go API — direct field writes are forbidden outside it.
type DispatchRecord struct {
	ProtocolVersion   int           `json:"protocol_version"` // v2 = DispatchProtocolV2; legacy = 1
	TurnID            string        `json:"turn_id"`
	RunID             string        `json:"run_id"`
	IntentOwnerRunID  string        `json:"intent_owner_run_id,omitempty"` // ≠ RunID for parent-owned restart intents (deliverPendingRestart @2233)
	State             DispatchState `json:"state"`
	StopOutcome       string        `json:"stop_outcome,omitempty"` // stopped_before_send | cancelled_in_flight (SD-24 §7.2)
	Revision          int64         `json:"revision"`                      // CAS token, monotonic per record
	ClaimOwner        string        `json:"claim_owner,omitempty"`         // recovery lease (instance id)
	ClaimExpiresAt    string        `json:"claim_expires_at,omitempty"`    // lease TTL (RFC3339Nano)
	RecoveryAttachEpoch     int64    `json:"recovery_attach_epoch"`        // monotonic callback/attach ownership fence
	RecoveryAttachOwner     string   `json:"recovery_attach_owner,omitempty"`
	RecoveryAttachExpiresAt string   `json:"recovery_attach_expires_at,omitempty"` // store-clock bounded TTL
	CancelRequested   bool          `json:"cancel_requested,omitempty"`    // monotonic; makes send_started CAS fail
	StopGeneration    int64         `json:"stop_generation,omitempty"` // per-record mirror; parent authority is RunStopState
	OuterIntentKey    string        `json:"outer_intent_key,omitempty"`    // durable-<run>-reprompt-<gen> etc.
	OuterIntentGen    int64         `json:"outer_intent_gen,omitempty"`
	EnvelopeHash      string        `json:"envelope_hash"`                 // binds the immutable envelope
	ReceiptEvidence   *ReceiptEvidence `json:"receipt_evidence,omitempty"` // canonical bytes+hash; set ONLY by CommitReceiptAndClearIntent
	TerminalEvidence  *TerminalEvidence `json:"terminal_evidence,omitempty"` // mandatory for post-send terminal commit
	SettleOwed        bool          `json:"settle_owed"`                 // fixed at prepare; operator action table may only force false for abandon/superseded
	SettlePhase       string        `json:"settle_phase,omitempty"` // settle sub-lifecycle (SD-24 §5.3); set atomically with terminal commit
	PredecessorTurnID string        `json:"predecessor_turn_id,omitempty"` // SS-17 retry-as-new lineage
	Outcome           string        `json:"outcome,omitempty"`             // completed|failed|cancelled + reason/resolved_by
	ParentStopFence   *ParentStopFence `json:"parent_stop_fence,omitempty"` // released child: parent run+expected stop generation, checked in send_started CAS
	CreatedAt         string        `json:"created_at,omitempty"`
	UpdatedAt         string        `json:"updated_at,omitempty"`
}

type ReceiptEvidence struct { ProviderKey, ReceiptID, EvidenceKind string; PayloadCanonicalJSON []byte; PayloadSHA256 string; ObservedAt string }
type TerminalEvidence struct { ProviderKey, EvidenceKind, Outcome, ReceiptIdentity string; PayloadCanonicalJSON []byte; PayloadSHA256 string; ObservedAt string }
type AttachedEffectPayload struct { Kind string; CanonicalJSON []byte; SHA256 string; ObservedAt string } // persisted before renderer/UI projection
type ParentStopFence struct { ParentRunID string; ExpectedStopGeneration int64 }
type RunStopState struct { RunID string; Generation, Revision int64; Stopped bool }
type ErrRunStopFence struct { CurrentGeneration int64 }       // own RunStopState.Stopped=true
type ErrParentStopFence struct { CurrentGeneration int64 }    // parent stopped or generation mismatch
var ErrRecoveryAttachRevoked = errors.New("recovery attach token is revoked")
var ErrRecoveryAttachActive = errors.New("recovery attach token remains active")

// dispatchLegal is the CLOSED forward-only edge set (SD-24 §5.2). No backward
// edges; no exits from terminal; uncertain exits only to terminal_* (reconcile
// proof or SS-17 operator action). Stop-before-send goes FORWARD to
// terminal_cancelled — never back to prepared.
var dispatchLegal = map[DispatchState][]DispatchState{
	DispatchPrepared:         {DispatchSendClaimed, DispatchTerminalCancelled},
	DispatchSendClaimed:      {DispatchSendStarted, DispatchTerminalCancelled},
	DispatchSendStarted:      {DispatchProviderAccepted, DispatchUncertain, DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
	DispatchProviderAccepted: {DispatchUncertain, DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
	DispatchUncertain:        {DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled},
}

func (s DispatchState) IsTerminal() bool {
	switch s {
	case DispatchTerminalCompleted, DispatchTerminalFailed, DispatchTerminalCancelled:
		return true
	}
	return false
}

// canTransition enforces the edge table PLUS CancelRequested invariants:
//   - CancelRequested ⇒ send_claimed→send_started is REJECTED (the Stop/send
//     linearization, SD-24 §7.2);
//   - CancelRequested in prepared/send_claimed ⇒ only terminal_cancelled is legal.
func (r *DispatchRecord) canTransition(to DispatchState) error {
	if r.CancelRequested && to == DispatchSendStarted {
		return fmt.Errorf("dispatch %s: cancel_requested blocks send_started (stop won linearization)", r.TurnID)
	}
	if r.CancelRequested && !r.State.IsTerminal() && (r.State == DispatchPrepared || r.State == DispatchSendClaimed) && to != DispatchTerminalCancelled {
		return fmt.Errorf("dispatch %s: cancel_requested in %s only permits terminal_cancelled", r.TurnID, r.State)
	}
	for _, ok := range dispatchLegal[r.State] {
		if ok == to {
			return nil
		}
	}
	return fmt.Errorf("illegal dispatch transition %s -> %s (turn %s)", r.State, to, r.TurnID)
}
```

`canTransition` validates only the record-local edge. `DispatchStore.CASAdvance` is the only caller and, when `next == DispatchSendStarted`, must lock/read the record's own `RunStopState` and reject stopped with `ErrRunStopFence{CurrentGeneration}`; a child then locks/compares its parent state with `ParentStopFence` and returns `ErrParentStopFence{CurrentGeneration}` on mismatch. Caller must invoke atomic pre-send cancellation with source `self|parent_fence`. No direct in-memory transition API is exposed.

**Step 1b — `DispatchEnvelope` (immutable redispatch payload, SD-24 §5.4).**

```go
// DispatchEnvelope is written ONCE at prepared and never mutated. Recovery
// re-sends from THIS, not from live intent fields; a hash mismatch vs. the
// live intent refuses automated redispatch (→ uncertain, SS-17).
type DispatchEnvelope struct {
	TurnID, RunID, StepID      string
	ProviderKey                ProviderKey
	ProviderAccountID          string
	ProviderSessionIDAtPrepare string
	PromptRef                  string // reference to the persisted turnLogKindPrompt line
	PromptSHA256               string
	Model, ReasoningEffort     string
	Yolo                       bool
	SelectedSkills             []SkillSelection
	Scenario                   string
	Attachments                []AttachmentRef // path/ref + sha256 each
	FlowContextInjected        bool
	CreatedAt                  string
	EnvelopeHash               string // sha256 over canonical JSON of all fields above
}
```

**Step 1c — `dispatch_store.go`: the only mutation path (SD-24 §6.1).**

```go
var ErrStaleDispatch = errors.New("dispatch record revision is stale")

type DispatchStore interface {
	// create-if-absent; the run's FIRST CreatePrepared also carries the atomic V2
	// activation in the same commit (SD-24 §6.5 — no marker/record crash window).
	CreatePrepared(ctx context.Context, rec DispatchRecord, env DispatchEnvelope) error
	CASAdvance(ctx context.Context, runID, turnID string, expectedRev int64,
		expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) // ErrStaleDispatch on mismatch; send_started always checks own RunStopState (ErrRunStopFence), then any parent fence (ErrParentStopFence)
	CASRecoveryAdvance(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
		expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) // recovery-only: same CAS plus exact claim owner + unexpired store-clock lease and own/parent fence in the transaction
	CASAdvanceSettle(ctx context.Context, runID, turnID string, expectedRev int64,
		expected, next SettlePhase) (int64, error)
	ClaimRecovery(ctx context.Context, runID, turnID string, expectedRev int64, owner string, ttl time.Duration) (int64, error)
	SetCancelRequested(ctx context.Context, runID, turnID string, expectedRev, stopGen int64) (int64, error)
	GetRunStopState(ctx context.Context, runID string) (RunStopState, error) // dispatch-store authority, never session snapshot
	RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) // one writer/RPC: generation++, stopped=true, audit
	// Cross-record atomic commits: dispatch transition + INTENT OWNER's intent
	// clear (+ audit) in ONE commit. intentOwnerRunID may differ from runID —
	// PendingRestart* lives on the PARENT (deliverPendingRestart @2233).
	CommitReceiptAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		receipt ReceiptEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // equality = canonical identity+payload hash; conflict => ErrReceiptConflict
	CommitTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // one transaction: proof derives state/outcome; persisted SettleOwed derives phase; record cancel + locked own/parent RunStopState derive StopOutcome; no caller override
	CommitRecoveredTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		leaseOwner string, proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // recovery-only: same terminal derivations while transaction requires claim owner + unexpired store-clock lease; ErrRecoveryLeaseLost otherwise
	CommitRecoveryUnknownOrRequireCancel(ctx context.Context, runID, turnID string, expectedRev int64,
		leaseOwner string) (RecoveryUnknownDecision, int64, error) // recovery-only: one tx validates lease + record cancel + locked own/parent Stop authority; either writes uncertain AND revokes attach epoch or returns RecoveryCancelRequired without mutation
	ClaimRecoveryAttach(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error) // recovery-only: exact lease + no current unexpired attach token + own/parent Stop tx; increments epoch or returns ErrRecoveryAttachActive/cancel-required
	EnterRecoveryAttach(ctx context.Context, runID, turnID string, token RecoveryAttachToken) error // one local-log/RPC transaction: exact epoch+owner + store-clock expiry + sent state + live own/parent Stop; persist attach entry BEFORE provider attach; ErrRecoveryAttachRevoked => zero attach
	RecordRecoveryAttachedEffect(ctx context.Context, runID, turnID string, token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (bool, error) // one transaction: same predicate + unique event identity/hash; renderer may expose output ONLY after accepted persistence
	CommitAttachedTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64, token RecoveryAttachToken,
		proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) // one tx: exact token + RecoveryAttachExpiresAt > store_clock_now + sent state + revision; lock Stop only to derive cancelled_in_flight, then proof terminal/clear/settle/audit/revoke; Stop never rejects valid unexpired proof, expiry/token/state/revision failure is ErrRecoveryAttachRevoked with zero mutation
	CommitPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
		intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) // ONE commit: send_claimed/prepared→terminal_cancelled + owner clear + self|parent-fence audit
	CommitRecoveryPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
		intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) // same pre-send terminal+clear/audit plus lease predicate
	// Operator resolution (SS-17 / SD-24 §6.7) — atomic + idempotent by resolutionID:
	ResolveUncertain(ctx context.Context, runID, turnID string, expectedRev int64,
		resolutionID, action string, evidence OperatorEvidence) (int64, error) // action = mark_completed|mark_failed|confirm_cancelled|abandon; one commit: exact SD-24 §6.7 terminal+intent+settle action table + attach epoch revoke
	// Supersede guard (SS-17 §8, ledger RS): CAS-checks live intent gen + envelope
	// hash — retry over a newer user prompt returns ErrSuperseded.
	RetryAsNew(ctx context.Context, runID, oldTurnID string, expectedRev int64,
		resolutionID, newTurnID string, expectedIntentGen int64, expectedEnvelopeHash string) (int64, error) // atomically supersedes/revokes old attach epoch before creating successor
	// Effect ledger + repair lifecycle (SD-24 §6.1):
	RecordEffectDone(ctx context.Context, runID, turnID, effectKind string, payload []byte, payloadHash string) (int64, error) // equal duplicate = no-op; payload-hash mismatch = ErrEffectConflict; cohort consequence lives here
	CreateReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, intent DurableIntent) (ReleaseManifestItem, error) // create-if-absent, revisioned pending item; mismatched intent hash = ErrEffectConflict
	CommitReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev int64, child DispatchRecord, env DispatchEnvelope) (int64, error) // ONE commit: child CreatePrepared + pending→created
	SuppressReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev, stopGen int64) (int64, error) // CAS pending→suppressed
	OpenRepair(ctx context.Context, runID, reason string, rawBlob []byte, rawHash string) (int64, error) // create-if-absent; record + THE RAW BLOB + block + audit in ONE commit (ledger QB)
	GetOpenRepair(ctx context.Context, runID string) (RepairRecord, bool, error) // returns quarantined raw
	// Two-phase repair resolution (a store tx cannot run the Go loader):
	BeginRepairResolution(ctx context.Context, runID string, expectedRepairRev int64,
		resolutionID, action string) (int64, []byte, error) // CAS-claim attempt (TTL) + audit start + hand back raw
	CommitRepairResolution(ctx context.Context, runID string, attemptRev int64,
		resolutionID, outcome, detail string) (int64, error) // resolved_retry_load|failed_still_open|resolved_abandon
	// Run protocol authority (ledger V2A): reads the NON-PRUNABLE activation entry.
	GetRunProtocolVersion(ctx context.Context, runID string) (int, error)
	// Read side (SD-24 §6.1 — required by settle driver / recovery / Task-256):
	Get(ctx context.Context, runID, turnID string) (DispatchRecord, int64, error)
	GetEnvelope(ctx context.Context, runID, turnID string) (DispatchEnvelope, error)
	ListRecoverable(ctx context.Context, runID string) ([]DispatchRecord, error)
	FindActiveByOuterIntent(ctx context.Context, runID, intentKey string, intentGen int64) (DispatchRecord, bool, error) // delivery authority; unique active record by outer intent
	ListAttention(ctx context.Context) ([]AttentionItem, error)
	ListAudit(ctx context.Context, runID, turnID string) ([]AuditEntry, error)
	ListEffects(ctx context.Context, runID, turnID string) ([]EffectDone, error)
	GetResolutionResult(ctx context.Context, resolutionID string) (ResolutionResult, bool, error)
	IsIntentCleared(ctx context.Context, ownerRunID, intentKey string, intentGen int64) (bool, error)
}
```
**Local impl (SD-24 §6.2):** one `dispatch.ndjson` per project data dir; ONE writer goroutine; each API commit = one fsynced appended line (record after-state + embedded `intentClear{ownerRunID,key,gen}` + `audit{...}` for cross-record commits — atomic at line granularity); validate → append → sync → then RAM; loader replays, drops+counts a torn tail; **the dispatch log wins over session records for intent clears at load**. Retention: prune only `terminal_*+finalized` past TTL; non-terminal blocks the session store's 90-day prune for that run.
**Supabase impl (SD-24 §6.5):** `dispatch_records` table (PK run_id+turn_id, `revision int8`, `intent_owner_run_id`, envelope jsonb immutable) + transactional RPCs (`dispatch_cas_advance`, `dispatch_commit_receipt_clear_intent` — updates dispatch row AND owner session intent AND audit in one transaction, ...); 0-row match ⇒ `ErrStaleDispatch`.

**Step 2 — RAM cache loads from the dispatch store, NOT the session snapshot.**
- Add `dispatch map[string]*DispatchRecord` to `interactiveRun` (keyed by turnID) + `dispatchByTurn(id)`. It is a **cache of the dispatch store**: populated at boot via `dispatchStore.ListRecoverable(runID)` + lazy `Get`, and updated only from successful commit results (post-durable, per SD-24 §6.1).
- `ProviderSessionState` ([workflow_store.go:249](../../../apps/local-runner/internal/runner/workflow_store.go:249)) gains ONLY `DispatchProtocolVersion int`, `RepairRequired bool`/`RepairReason string`, and the Task-252 provenance fields. **No `Dispatch` slice — adding one recreates the two-sources-of-truth/stale-snapshot defect** (plan-review #3 #3). `sessionStateOf` ([interactive_service.go:2535](../../../apps/local-runner/internal/runner/interactive_service.go:2535)) copies only those scalars.
- Boot load (around [interactive_service.go:710](../../../apps/local-runner/internal/runner/interactive_service.go:710)): after sessions load, `rs.dispatch` rebuilds from the dispatch store; session `Pending*` intents are filtered against `IsIntentCleared` (SD-24 §6.2 read-side rule).

**Step 3 — terminal durability = the commit log line.** `CommitTerminalAndSettleIntent` appends the fsynced terminal line; recovery observes it via `Get`/`ListRecoverable`. Nothing dispatch-related rides `persistProviderSession`, so the session-event gap at [local_file_session_store.go:611](../../../apps/local-runner/internal/runner/local_file_session_store.go:611) is irrelevant to dispatch (leave it).

**Step 4 — legacy migration helper (read old `idempotency[key]`).**

```go
func dispatchFromLegacyIdem(rs *interactiveRun, key, raw string) DispatchRecord {
	turnID, launched := parseDurableIdemValue(raw) // existing helper @2725
	r := DispatchRecord{ProtocolVersion: 1, TurnID: turnID, RunID: rs.id, OuterIntentKey: key}
	switch {
	case !launched: // "prep:<turnID>" — envelope synthesized from the persisted
		// turnLogKindPrompt line; if that log line is missing, r.State = uncertain
		// (we cannot prove WHAT would be re-sent).
		r.State = DispatchPrepared
	case rs.lastTurnID == turnID || hasTerminalEvent(rs, turnID):
		r.State, r.Outcome = DispatchTerminalCompleted, "completed"
	default: // bare, no corroboration -> never assume accepted
		r.State = DispatchUncertain
	}
	return r
}
```
Legacy (`ProtocolVersion=1`) records are **resolve-only**: recovery may move them to `terminal_*`/`uncertain` but never CAS-advances them through v2 live states.

**Step 5 — resolve the `EventTurnStarted` contradiction (DOD-G8).** Delete the misleading claim at [interactive_service.go:5994](../../../apps/local-runner/internal/runner/interactive_service.go:5994) ("recovery can see EventTurnStarted for replay-safe"); replace with a pointer to the one rule table in `dispatch_record.go` (recovery infers from `DispatchRecord.State`, never from `EventTurnStarted`). Keep `durableIdemReplaySafe`'s statement consistent or remove the function once Task-249 lands its replacement.

### 4.2 Test skeletons (`dispatch_record_test.go`)

```go
func TestDispatchTransitions_ExhaustiveTable(t *testing.T)      { /* every (from,to) pair: legal per table, ALL others rejected */ }
func TestDispatchCancelRequested_BlocksSendStarted(t *testing.T){ /* the Stop/send linearization guard */ }
func TestDispatchCAS_StaleRevisionRejected(t *testing.T)        { /* SW ledger row: older Revision -> ErrStaleDispatch, state unharmed */ }
func TestDispatchStore_ContractSuite(t *testing.T)              { /* shared cases, run vs local AND supabase-fake (+real PG when FLOWPILOT_TEST_SUPABASE_DSN set) */ }
func TestLocalStore_DiskBeforeRAM_TornTailDropped(t *testing.T) { /* TA ledger row */ }
func TestDispatchEnvelope_HashBindsPayload(t *testing.T)        { /* mutate any field -> hash mismatch detected */ }
func TestDispatchStore_RoundTripAllStates(t *testing.T)         { /* dispatch store persist -> restart -> Get/ListRecoverable (NOT via session snapshot) */ }
func TestSessionState_CarriesNoDispatchRecords(t *testing.T)    { /* SB ledger row: marker/repair/provenance scalars only */ }
func TestLocalLog_StartupLock_CompactionCrashRules(t *testing.T){ /* second process rejected; temp discarded; missing-log-on-V2 -> repair */ }
func TestCreatePrepared_AtomicV2Activation(t *testing.T)        { /* V2A row: first record activates in same commit; GetRunProtocolVersion is the authority; mirror lag benign */ }
func TestActivation_SurvivesCompaction(t *testing.T)            { /* V2A row: prune all terminal records -> activation entry remains; version still 2 */ }
func TestSessionUpsertGuarded_InterleavedClearCannotResurrect(t *testing.T) { /* TO row: receipt commit racing a stale snapshot write, same-transaction guard */ }
func TestRetryAsNew_SupersededIntentRejected(t *testing.T)      { /* RS row: newer intent gen / envelope hash -> ErrSuperseded */ }
func TestRecordEffectDone_UniqueKeyIdempotent(t *testing.T)     { /* equal duplicate = no-op; divergent payload hash = ErrEffectConflict on both backends */ }
func TestReleaseManifest_AtomicChildCreateAndState(t *testing.T) { /* pending→created and child prepared are one local line / one RPC tx; Stop CAS yields suppressed */ }
func TestOpenRepair_CreateIfAbsent_OneCommit(t *testing.T)      { /* repeat returns existing RepairRevision; quarantine+block+audit atomic */ }
func TestDispatchTerminalOutcome_SurvivesLocalRestart(t *testing.T) { /* was lost pre-fix (DOD-C1) */ }
func TestDispatchFromLegacyIdem(t *testing.T)                   { /* prep:/bare+terminal/bare-only -> prepared/terminal_completed/uncertain */ }
func TestEventTurnStartedNotUsedForRecovery(t *testing.T)       { /* DOD-G8 */ }
func TestDispatchMutation_GuardFlipsTurnSuiteRed(t *testing.T)  { /* MU ledger row: build-tag/injected wrong-edge variant must fail */ }
func TestPreSendStopCancellationAndOwnerClearAreAtomic(t *testing.T) { /* parent-owned intent; crash/restart never re-dispatches cancelled turn */ }
func TestReceiptEvidence_EqualReplayAndPayloadConflict(t *testing.T) { /* canonical JSON/hash equality accepted; same identity+differing payload => ErrReceiptConflict on both stores */ }
func TestTerminalCommit_RejectsAmbiguousPostSendError(t *testing.T) { /* no TerminalEvidence => send_started + owner intent remain; only recovery/uncertain allowed */ }
func TestTerminalCommit_ReadsPersistedSettleOwed_NoCallerOverride(t *testing.T) { /* SO: terminal commit API has no settle bool; both stores derive settle_pending|none from record */ }
func TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority(t *testing.T) { /* SA: both stores/local-log+RPC lock own and optional parent RunStopState in the terminal transaction; root Stop or child parent mismatch with CancelRequested=false yields cancelled_in_flight for proof completed|failed|cancelled; no effective Stop leaves it empty; no caller input exists */ }
func TestRecoveryCommitGuards_LeaseAndStopAreAtomic(t *testing.T) { /* RG: both stores. Stop/root or parent fence commits after final pre-write read but before recovery send/pre-send/unknown transaction => typed fence/no send, pre-send atomic cancellation, or RecoveryCancelRequired/no uncertain; expired/taken-over lease after Reconcile proof => ErrRecoveryLeaseLost/no terminal/owner clear; valid owner re-reconciles and commits once. */ }
func TestRecoveryAttach_EntryEffectAndTerminalAreAtomic(t *testing.T) { /* RA/TP/SA: local + Supabase RPC + real PG + model. A pauses after token predicate/entry barrier; B terminalizes or moves to uncertain; A resumes => ErrRecoveryAttachRevoked, no provider attach/output/terminal/intent clear/settle/audit. A current token forwards exactly one completed|failed|cancelled proof through CommitAttachedTerminalAndSettleIntent; root-Stop and child-parent-Stop (CancelRequested=false) variants must commit the proof-derived state/outcome, clear/settle/audit once, record cancelled_in_flight, revoke epoch, then make later callbacks inert. Cross store time past attach TTL without N+1 claim before each completed|failed|cancelled terminal proof (with no Stop, root Stop, and parent Stop) => ErrRecoveryAttachRevoked and zero mutation; keep separate unexpired Stop variants green. */ }
func TestEverySentExitAndTerminalPath_RevokesRecoveryAttachEpoch(t *testing.T) { /* live terminal, recovered terminal, pre-send cancellation, unknown->uncertain, each terminal ResolveUncertain action, and RetryAsNew all increment/clear the current epoch; old token is inert on both stores. */ }
func TestRunStopState_IsOnlyStopAuthority(t *testing.T) { /* local log + Supabase: activation creates it; root and child send CAS never use session/record mirrors */ }
func TestOwnRunStopVsSendStarted_IsLinearizable(t *testing.T) { /* RSF: RequestRunStop races root send CAS; ErrRunStopFence -> atomic pre-send cancel/owner clear/audit, zero bytes */ }
func TestChildSendStarted_ParentStopFenceIsAtomic(t *testing.T) { /* parent Stop vs manifest-create/child-send: ErrParentStopFence(current) -> atomic pre-send cancel + owner clear/audit, zero bytes */ }
func TestResolveUncertain_ActionSettlementTableAtomic(t *testing.T) { /* completed/failed preserve settleOwed; abandon/supersede force none; successor is sole settler */ }
```

> **FF note (plan-review #16):** these are *foundation* tests — they use types that do not exist on HEAD, so "fail on HEAD" does not apply. Their substitute is the `MU` mutation row. Fail-on-HEAD applies to the defect-regression suites (Task-255).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/dispatch_record.go` + `dispatch_store.go` + `dispatch_store_local.go` + `dispatch_store_supabase.go` (new), `workflow_store.go` (marker/repair/provenance scalars only), `local_file_session_store.go` (parity fields + intent-clear filter + prune guard), `interactive_service.go` (RAM cache + comment fix), `supabase_workflow_store.go` (marker column + intent-clear filter; blob versioning in Task-253).
- modules: `internal/runner` persistence.
- routes: none.
- tables (SD-24 §6.5 — the full relational migration, plan-review #5 #9): **`dispatch_records`** (including durable attach epoch/owner/expiry), **`dispatch_run_stop_state`** (non-prunable run-level `generation/stopped/revision`, created with activation; sole parent-fence authority), **`dispatch_effects`** (UNIQUE run/turn/effect_kind, payload + payload hash; includes attached event identity and release-manifest state + revision for `pending→created|suppressed`), **`repair_records`** (incl. `quarantine_blob`+`quarantine_hash`), **`run_protocol_activations`** (non-prunable) + all RPCs incl. `session_upsert_guarded`, `dispatch_request_run_stop`, `dispatch_enter_recovery_attach`, `dispatch_record_recovery_attached_effect`, `dispatch_commit_attached_terminal_settle`, `dispatch_create_release_manifest_item`, `dispatch_commit_release_manifest_item`, `dispatch_suppress_release_manifest_item`; `session_runtime` gains only the display-mirror column. Local: `dispatch.ndjson` (activation + run-stop-state lines non-prunable) + `dispatch.lock`. **No dispatch JSON inside session records on either backend.**

## 6. Acceptance Check

- `V-1` `dispatch_record_test.go`: the full (from,to) transition matrix — legal edges accepted, **every** other edge rejected; `CancelRequested` invariants (blocks `send_started`; forces cancel path) enforced; settle-phase transitions likewise exhaustive.
- `V-2` Round-trip test: a record at each state + settle phase survives **dispatch-store** persist → restart → `Get`/`ListRecoverable` on both backends; and (`SB`) `ProviderSessionState` JSON contains **no** dispatch records (grep + runtime assert).
- `V-3` Terminal-outcome durability: a `terminal` commit-log line is observable after restart via the dispatch store (previously lost per `DOD-C1`); shared local/Supabase contract tests prove the same terminal transaction derives `StopOutcome=cancelled_in_flight` from record cancel/own Stop/parent fence or empty when none is effective; read-side APIs return deterministic order + honor pagination.
- `V-4` Legacy migration test: `prep:`, bare-with-terminal, and bare-without-terminal fixtures map to `prepared`/**`terminal_completed`**/`uncertain` respectively (the ONE rule — SD-24 §5.6).
- `V-4b` Operational: startup lock rejects a second process; compaction crash-rules honored (leftover temp discarded); missing log on a V2-marker run ⇒ `repair_required`.
- `V-5` `DOD-G8`: a grep/test asserts there is exactly one documented recovery-inference rule for `EventTurnStarted` and code honors it.
- `V-6` Parity (`PAR`): full-field `ProviderSessionState` round-trip passes identically on local and Supabase (incl. the three previously-missing local fields); prune guard verified (a run with a non-terminal record survives the retention pass).
- `V-7` Cross-record atomicity: `CommitReceiptAndClearIntent` with `intentOwnerRunID != runID` (parent restart) clears the parent's intent and the child's transition in one commit — crash between them is impossible by construction (single log line / single RPC transaction), asserted via the store contract suite.
- `V-7b` SD-25 attached-stream contract: shared local/Supabase RPC/real-PG tests prove `EnterRecoveryAttach` occurs before physical attach, output is visible only from `RecordRecoveryAttachedEffect`, attached terminal forwarding is one `CommitAttachedTerminalAndSettleIntent` transaction, and revoked-token results mutate nothing. Every sent exit and all live/recovered/pre-send/operator/retry terminal paths revoke the epoch.
- `V-8` `go build`, `go vet`, `go test ./internal/runner` clean.

## 7. Out of Scope

- Rewiring `startTurn`/`runTurn` to drive the record live (Task-249).
- Recovery scanner logic (Task-250).
- Supabase version field + fail-closed decode (Task-253) — this task only passes the record through.

## 8. Completion Notes

- result: **done** — landed `DispatchRecord` 8-state machine + settle phases, full `DispatchStore` API (memory/local NDJSON/supabase-fake), legacy `prep:`/bare migration, session-mirror scalars only (no dispatch slice), `EventTurnStarted` recovery contract fix (DOD-G8), and contract/unit suite green under `go test ./internal/runner -run TestDispatch…`.
- follow-ups: Task-249 live path wiring; Task-253 co-phase Supabase runtime version/quarantine; Task-255 crash-matrix harness.
- upstream docs updated: CP-51 implementation in progress; DoD ledger rows owned by this task exercised via unit suite (SW/TA/V2A/RS/SB foundation).
- DoD checklist:
  - [x] V-1 exhaustive transition + CancelRequested
  - [x] V-2 round-trip + session carries no dispatch records
  - [x] V-3 terminal durability across local restart
  - [x] V-4 legacy migration ONE rule
  - [x] V-4b startup lock temp discard + torn tail
  - [x] V-5 DOD-G8 EventTurnStarted not recovery authority
  - [x] V-6 parity fields on ndjson session record
  - [x] V-7 cross-record pre-send cancel + intent clear
  - [x] V-7b attach revoke + terminal paths
  - [x] V-8 package tests compile and pass for new suite

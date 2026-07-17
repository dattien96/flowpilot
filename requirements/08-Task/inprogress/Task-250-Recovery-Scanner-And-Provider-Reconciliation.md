# Task-250: Recovery Scanner And Provider Reconciliation

## Metadata

- Document ID: `Task-250`
- Title: `Recovery Scanner And Provider Reconciliation`
- Phase: `task`
- Status: `in_progress` (audit 2026-07-17: boot enumeration + redispatch fixed; provider reconcile/attach engine (T-4) still absent — see §8 Audit Gap)
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SD-25 Recovery Ownership Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SS-17](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [Task-248](./Task-248-Durable-Dispatch-Record-And-State-Machine-Core.md), [Task-249](./Task-249-Live-Dispatch-Integration-And-Stop-Fences.md)
- Replaces: `None`
- Tags: `agent-flow-engine, durable-turn, crash-recovery, reconciliation`

## AI Quick View

### Summary

- On restart, scan non-terminal `DispatchRecord`s **under a `ClaimRecovery` lease** (two concurrent scanners: exactly one acts — CC ledger row) and act per SD-24 §6.4: `prepared`/`send_claimed` are **safely-retryable** (same TurnID + envelope, hash-verified); `send_started`/`provider_accepted` reconcile via the capability matrix; `uncertain` surfaces per SS-17.
- Wire the SS-17 operator-resolution actions (inspect / retry-as-new via `PredecessorTurnID` / mark-outcome / abandon), idempotent + audited, and the SS-17 AC-4 dispatch-block on unresolved runs.
- Outer intents clear only via the atomic commits; no non-terminal record is ever pruned.
- **Depends on Task-249** (live transitions must exist before recovery can reconcile them) — plan-review #15.

### Current Ask

- Implement the recovery scanner + per-adapter reconciliation hook and integrate it into the existing resume/reconstruct path.

### Key Decisions

- `T-1` Recovery — not the live process — owns incomplete dispatch: a ghost launch-ack must never cause a clear or a blind relaunch.
- `T-2` `uncertain` is terminal-for-automation: hold, surface for reconciliation, never auto-clear intent, never re-dispatch (prevents both loss and duplication).

### Constraints

- Reconciliation must be idempotent and safe to run repeatedly (resume can fire more than once).
- Provider query support varies; when a provider cannot confirm, downgrade to `uncertain` rather than guess.

### Open Questions

- `Q-3` **Resolved by SS-17:** hold + surface; operator actions only (AC-2); no timer-based auto-terminalize (AC-6).

### Source Refs

- CP-51 §3.2, §10.2, §10.4 (crash cells); BUG-288 `DOD-C1` (recovery side).
- Code: `interactive_resume.go` (resume/reconstruct entry, `reconstructPendingChildSessions:1588`), `interactive_service.go` (`durableIdemReplaySafe` retirement).

## 1. Goal

Guarantee that after any crash, every non-terminal dispatch is resolved without losing or duplicating a turn: re-dispatch only what is provably un-launched, reconcile what may be in flight, and block what cannot be proven.

## 2. Parent Links

- coding plan: CP-51 (`P-3`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.4 recovery scanner contract); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-C1`, INV-1, INV-2, recovery-scanner acceptance §10.2

## 3. Trigger

Today recovery relies on `durableIdemReplaySafe` returning false for incomplete keys so the launch path re-enters; but outer-intent clearing already happened live on RAM `turnInFlight` (Task-249 fixes the live side). The recovery side needs an explicit lease-based scanner that reconciles `send_started`/`provider_accepted` records against the provider instead of relying on event presence (terminal events were not even durable pre-Task-248), and that hands terminal-but-unsettled records to the settle driver.

## 4. Exact Change

- `T-1` New `dispatch_recovery.go`: `reconcileDispatchRecordsOnResume(runID)` iterating `dispatchStore.ListRecoverable(runID)` (the read API — never a session-snapshot copy), acting only under `ClaimRecovery` leases **followed immediately by a fresh `Get` and lease-owner/expiry check**; no provider action may use the list snapshot. Every recovery mutation uses a store transaction that rechecks `ClaimOwner` and unexpired store-clock lease; a pre-write `Get` is never treated as a lease lock.
- `T-2` **Branch by state first; `CancelRequested` never blind-terminalizes a sent turn** (plan-review #3 #7): `prepared`/`send_claimed` + local cancel, **stopped own `RunStopState`**, or invalidated `ParentStopFence` ⇒ atomic pre-send `terminal_cancelled`/owner-clear/audit (nothing was sent); `send_started`/`provider_accepted` + any effective Stop (record cancel, own Stop, or parent fence) ⇒ issue/continue **provider cancel**. Thereafter recovery may make a fresh, lease-guarded reconcile query **only to commit validated `TerminalEvidence`**; it never attaches or writes `uncertain` merely because Stop was requested. Never a bare CAS to cancelled/uncertain on a sent turn because Stop alone is not terminal proof (ledger `SC`/`ES`/`RSF`/`PS`/`TP`).
- `T-3` `prepared`/`send_claimed` (no cancel) → safely-retryable: envelope-hash-verified re-dispatch, same TurnID.
- `T-4` `send_started`/`provider_accepted` → adapter `Reconcile(turnID, ReceiptEvidence)`: terminal-proof with `TerminalEvidence.Outcome ∈ {completed,failed,cancelled}` ⇒ **`CommitRecoveredTerminalAndSettleIntent`** (one transaction validates scanner lease, locks Stop authority, derives proof state/outcome/settle/StopOutcome, and revokes the attach epoch); in-flight ⇒ **`ClaimRecoveryAttach`**, then **`EnterRecoveryAttach` before physical provider attach**. Each nonterminal stream callback first calls **`RecordRecoveryAttachedEffect`**; only the accepted durable event reaches the renderer/bridge. A callback terminal uses only **`CommitAttachedTerminalAndSettleIntent`**, which atomically validates exact epoch+owner, `RecoveryAttachExpiresAt > store_clock_now`, sent state, and expected revision before terminalizing — never `Validate → Commit`; it locks Stop for `cancelled_in_flight` attribution and must still commit valid **unexpired** proof after root or parent Stop. Unknown/no-adapter ⇒ **`CommitRecoveryUnknownOrRequireCancel`**, which revokes attach when it changes sent state to `uncertain`. Exact epoch+owner, store-clock expiry, sent state, and live own/parent Stop are mandatory for attach entry/output; terminal has the same expiry/token/state guard but Stop is attribution, not a rejection. `ErrRecoveryAttachRevoked` produces no attach/output/terminal/clear/settlement/audit for expiry/token/state/revision failure. An unexpired active token is respected on repeated resume; expiry reopens recovery after crash. `effectiveStopRequested` reads own `RunStopState`, parent fence authority, and record `CancelRequested`; if Stop is effective it issues/continues provider cancel and suppresses attach and unproven classification. Store-read failure is fail-closed. That request alone is never terminal.
- `T-5` `uncertain` → surface per SS-17; resolutions only via `ResolveUncertain`/`RetryAsNew`. Terminal records with unfinalized settle → hand to the settle driver. No recovery-side intent clear exists (clears live in the atomic commits + load/write filters).
- `T-6` Scanner runs from the resume/reconstruct hooks (parent + `reconstructPendingChildSessions`), idempotent across repeated resumes; legacy `ProtocolVersion=1` records resolve-only.

### 4.1 Code guide (step-by-step)

> Depends on Task-248 (record) + Task-249 (live transitions). Sketches illustrative.

**Step 1 — new file `apps/local-runner/internal/runner/dispatch_recovery.go`: the scanner.**

```go
package runner

// reconcileDispatchRecordsOnResume resolves every non-terminal dispatch on restart
// (SD-24 §6.4). Idempotent — safe to run on every resume. Caller holds no lock.
func (s *InteractiveService) reconcileDispatchRecordsOnResume(runID string) {
	// READ API, not a session-snapshot copy (plan-review #3 #4): includes
	// non-terminal records AND terminal records with unfinalized settle.
	pending, err := s.dispatchStore.ListRecoverable(ctx, runID)
	if err != nil { return }

	for _, r := range pending {
		// LEASE FIRST (SD-24 §6.4): two scanners -> exactly one acts (CC row).
		_, err := s.dispatchStore.ClaimRecovery(ctx, runID, r.TurnID, r.Revision, s.instanceID, recoveryLeaseTTL)
		if err != nil { continue } // someone else owns it, or revision moved — skip, don't act
		// Claim changes the revision. Never use the ListRecoverable snapshot after
		// this point: Stop/cancel or another valid transition may have landed.
		fresh, rev, err := s.dispatchStore.Get(ctx, runID, r.TurnID)
		if err != nil || fresh.ClaimOwner != s.instanceID || leaseExpired(fresh.ClaimExpiresAt) { continue }
		r = fresh

		// STATE FIRST — CancelRequested must never blind-terminalize a SENT turn
		// (plan-review #3 #7; ledger SC).
		switch {
		case r.State.IsTerminal() && r.SettlePhase != SettleFinalized && !isSettleDisposition(r.SettlePhase):
			s.driveSettle(runID, r.TurnID) // Task-251 phase driver (effects-then-CAS + effect ledger)
		case r.State == DispatchPrepared || r.State == DispatchSendClaimed:
			// Nothing was sent. Check BOTH the record-local Stop and a released
			// child's RunStopState authority before choosing redispatch. A parent
			// fence mismatch uses the same atomic terminal+owner-clear path as the
			// live ErrParentStopFence route; it must never leave send_claimed live.
			source, stopGen, blocked, berr := s.preSendStopBlock(ctx, r)
			if berr != nil { continue } // cannot prove permission; no redispatch
			if !blocked {
				s.redispatchFromEnvelope(runID, r, rev) // helper rechecks lease/fence immediately before its send CAS
				continue
			}
			_, err := s.dispatchStore.CommitRecoveryPreSendCancellationAndClearIntent(ctx, runID, r.TurnID, rev, s.instanceID,
				r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen, stopGen, source)
			if err != nil { log.Printf("[dispatch-recovery] pre-send cancel commit failed run=%s turn=%s: %v", runID, r.TurnID, err) }
		case r.State == DispatchSendStarted || r.State == DispatchProviderAccepted:
			// Sent (or possibly sent): reconcileWithProvider first evaluates the
			// durable effective-Stop authority; a true result is cancel-only.
			s.reconcileWithProvider(runID, r, rev)
		case r.State == DispatchUncertain:
			// A Stop can arrive after an earlier scanner transaction had honestly
			// classified unknown. It remains a sent record: route the durable Stop
			// to provider cancel before surfacing, never re-dispatch.
			stopped, serr := s.effectiveStopRequested(ctx, r)
			if serr != nil { continue }
			if stopped {
				if cerr := s.issueProviderCancel(runID, r.TurnID); cerr != nil { log.Printf("[dispatch-recovery] cancel surfaced uncertain failed: %v", cerr) }
			}
			s.surfaceUncertainPerSS17(runID, r) // hold; block automated dispatch (AC-4)
		}
	}
}
```

`preSendStopBlock` is a required helper, not a cache lookup: it first calls `GetRunStopState(r.RunID)` for **every** record and returns `(PreSendStopSelf, state.Generation, true, nil)` when the own run is stopped (or when record-local `CancelRequested` is true). Only then, for a fenced child, it calls `GetRunStopState(fence.ParentRunID)` and returns `(PreSendStopParentFence, state.Generation, true, nil)` when `state.Stopped || state.Generation != fence.ExpectedStopGeneration`. Store-read failure returns an error and blocks redispatch. `redispatchFromEnvelope` repeats the fresh lease + `preSendStopBlock` check immediately before `CASRecoveryAdvance(send_claimed→send_started, leaseOwner=s.instanceID)`; that transaction revalidates the unexpired lease and both Stop authorities at the actual send linearization. Its typed fence/lease error performs no external send; a fence routes through `CommitRecoveryPreSendCancellationAndClearIntent`, and a lease loss leaves the next claimant to act.

**Step 2 — provider reconcile hook (opt-in adapter capability).** Extend the adapter surface without breaking `SendTurn` ([provider_registry.go:77](../../../apps/local-runner/internal/runner/provider_registry.go:77)):

```go
type ReconcilableAdapter interface {
	// Reconcile reports whether a previously-dispatched turn completed, is still
	// in flight, or is unknown. Adapters that cannot answer return ReconcileUnknown.
	Reconcile(ctx context.Context, turnID string, receipt *ReceiptEvidence) (ReconcileOutcome, *TerminalEvidence, error) // terminal proof's Outcome is completed|failed|cancelled; do not infer it from ReconcileOutcome
}
type ReconcileOutcome string
const ( ReconcileTerminalProof ReconcileOutcome = "terminal_proof"; ReconcileInFlight ReconcileOutcome = "in_flight"; ReconcileUnknown ReconcileOutcome = "unknown" )

// Runs ONLY under a held lease; every transition is a CAS with the lease
// revision. Takes runID (matches the scanner — plan-review #4 #9); resolves the
// run/adapter internally. NO discarded errors: ErrStaleDispatch => someone else
// acted, stop; persist failure => release nothing, the lease TTL + next scan
// retries (fail-closed contract).
func (s *InteractiveService) reconcileWithProvider(runID string, r DispatchRecord, rev int64) {
	// Re-read and validate the lease immediately before every provider side
	// effect. A claim is not a permanent lock: Stop may have landed while an
	// earlier Reconcile call was in flight.
	fresh, freshRev, err := s.dispatchStore.Get(ctx, runID, r.TurnID)
	if err != nil || fresh.ClaimOwner != s.instanceID || leaseExpired(fresh.ClaimExpiresAt) { return }
	r, rev = fresh, freshRev
	stopped, serr := s.effectiveStopRequested(ctx, r)
	if serr != nil { return } // cannot prove live authority => no provider action/attach
	stopActive := stopped
	if stopped {
		if err := s.issueProviderCancel(runID, r.TurnID); err != nil { return }
		// Re-read after cancel. A stopped sent record may query only for a real
		// TerminalEvidence; it must never attach or become uncertain by inference.
		fresh, freshRev, err = s.dispatchStore.Get(ctx, runID, r.TurnID)
		if err != nil || fresh.ClaimOwner != s.instanceID || leaseExpired(fresh.ClaimExpiresAt) { return }
		r, rev = fresh, freshRev
	}
	ra, ok := s.adapterForRun(runID).(ReconcilableAdapter)
	if !ok {
		if stopActive { return } // no proof source after Stop: retain sent record/intent
		decision, _, derr := s.dispatchStore.CommitRecoveryUnknownOrRequireCancel(ctx, runID, r.TurnID, rev, s.instanceID)
		if derr != nil { return } // stale/lease loss => another scanner/retry owns it
		if decision == RecoveryCancelRequired {
			if cerr := s.issueProviderCancel(runID, r.TurnID); cerr != nil { log.Printf("[dispatch-recovery] cancel before fallback classification failed: %v", cerr) }
		}
		return
	}
	out, terminalProof, err := ra.Reconcile(ctx, r.TurnID, r.ReceiptEvidence)
	switch {
	case err == nil && out == ReconcileTerminalProof && terminalProof != nil:
		// Terminal via the ATOMIC commit (owner's intent + settle mark) — never a
		// bare state write, never a direct intent clear.
		if !isClosedTerminalOutcome(terminalProof.Outcome) { // completed|failed|cancelled only
			log.Printf("[dispatch-recovery] invalid terminal proof outcome run=%s turn=%s", runID, r.TurnID)
			return // leave reconcilable; next scan may get valid proof
		}
		if _, cerr := s.dispatchStore.CommitRecoveredTerminalAndSettleIntent(ctx, runID, r.TurnID, rev, s.instanceID,
			*terminalProof, r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen); cerr != nil {
			log.Printf("[dispatch-recovery] terminal commit failed run=%s turn=%s: %v (retry on next scan)", runID, r.TurnID, cerr)
		}
	case err == nil && out == ReconcileInFlight:
		if stopActive { return }
		token, attached, aerr := s.dispatchStore.ClaimRecoveryAttach(ctx, runID, r.TurnID, rev, s.instanceID, recoveryAttachTTL)
		if errors.Is(aerr, ErrRecoveryCancelRequired) {
			if cerr := s.issueProviderCancel(runID, r.TurnID); cerr != nil { log.Printf("[dispatch-recovery] cancel at attach claim failed: %v", cerr) }
			return
		}
		if aerr != nil { return } // ErrRecoveryAttachActive/stale/lease loss => no attach by this scanner; active owner remains sole consumer
		// This is the last durable gate BEFORE an external subscription. A claim
		// alone does not authorize attach after terminal/expiry/unknown races.
		if eerr := s.dispatchStore.EnterRecoveryAttach(ctx, runID, attached.TurnID, token); eerr != nil {
			return // ErrRecoveryAttachRevoked => zero provider attach; no fallback output
		}
		s.resumeStreaming(runID, attached, token) // bridge uses the two guarded operations below
	default:
		if stopActive { return } // no terminal proof after Stop: retain for later proof/operator
		// Unknown can race a durable Stop/lease takeover. The store decision locks
		// those authorities with this mutation; it alone may write uncertain.
		decision, _, derr := s.dispatchStore.CommitRecoveryUnknownOrRequireCancel(ctx, runID, r.TurnID, rev, s.instanceID)
		if derr != nil { return } // stale/lease loss => no mutation by this scanner
		if decision == RecoveryCancelRequired {
			if cerr := s.issueProviderCancel(runID, r.TurnID); cerr != nil { log.Printf("[dispatch-recovery] cancel before unknown classification failed: %v", cerr) }
		}
	}
}
```
Default-safe (SD-24 §6.3): cannot confirm ⇒ `uncertain`, never assumed accepted. **There is no recovery-side intent clear**: clears happen only inside the atomic commits (plan-review #2 #5).

`effectiveStopRequested(ctx, r)` is one shared fail-closed helper for **sent-state recovery**: read `GetRunStopState(r.RunID)`; return true when it is stopped or `r.CancelRequested`; if `r.ParentStopFence != nil`, read its parent state and return true when stopped or generation differs. Any read error is returned (caller performs no reconcile classification, cancel, or attach); it never consults session/RAM mirrors. `preSendStopBlock` reuses the same authority logic for pre-send recovery, but sent state must call this helper independently because a crash can land after `RequestRunStop` and before `SetCancelRequested`.

**Attached stream bridge contract — required implementation, not pseudocode:** `resumeStreaming(runID, record, token)` is called only after successful `EnterRecoveryAttach`. Before any nonterminal provider callback is emitted to the UI/event bus, call `RecordRecoveryAttachedEffect(ctx, runID, turnID, token, providerEventID, payload)`. It appends a unique durable event in the same transaction that checks exact token owner/epoch, store-clock expiry, `State ∈ {send_started, provider_accepted}`, and own/parent Stop; only `inserted=true` is rendered (equal duplicate is replay-safe). A callback carrying `TerminalEvidence` instead calls `CommitAttachedTerminalAndSettleIntent(ctx, runID, turnID, recordRevision, token, proof, intent...)`; it atomically checks exact token owner/epoch **and `RecoveryAttachExpiresAt > store_clock_now`**, sent state, and revision, locks own/parent Stop only to derive `StopOutcome=cancelled_in_flight`, and commits valid unexpired proof even if Stop won. It must not emit a final output first and must never perform a standalone validation followed by an ordinary terminal API. `ErrRecoveryAttachRevoked` is deliberately swallowed only for stale/**expired**/superseded/uncertain/terminal token/state/revision failure. If terminal/unknown/expiry wins after `ClaimRecoveryAttach` but before `EnterRecoveryAttach`, the latter returns that error and `resumeStreaming` is never invoked.

**Step 3 — wire into the boot/resume path.** Call `reconcileDispatchRecordsOnResume(runID)` where sessions are loaded on startup (around [interactive_service.go:710](../../../apps/local-runner/internal/runner/interactive_service.go:710), `ListAllProviderSessions`) and from `reconstructPendingChildSessions` ([interactive_resume.go:1588](../../../apps/local-runner/internal/runner/interactive_resume.go:1588)). Ensure it runs after `rs.dispatch` is rebuilt (Task-248 Step 2).

**Step 4 — settle + operator handoff.** Terminal records with `SettlePhase != finalized` are handed to the settle phase driver (Task-251). `uncertain` records surface per SS-17 and route resolutions through the atomic ops `ResolveUncertain`/`RetryAsNew` (Task-248 API; surface = Task-256). At load, session `Pending*` intents are reconciled against the dispatch log's embedded clear markers (SD-24 §6.2) — recovery itself never touches intent fields directly.

**Step 5 — never prune non-terminal (guard).** The store's compaction/retention (SD-24 §6.2) already excludes non-terminal + unfinalized-settle records; add a scanner-side assertion that `ListRecoverable` results never contain a record the retention pass could have dropped (belt-and-suspenders with Task-254).

### 4.2 Test skeletons (`dispatch_recovery_test.go`)

```go
func TestRecovery_CancelOnSentTurn_ProviderCancelNotBlindTerminal(t *testing.T) { /* SC row: send_started+CancelRequested -> provider cancel; only valid TerminalEvidence may terminalize, no proof never becomes bare terminal_cancelled/uncertain */ }
func TestRecovery_TerminalWithUnfinalizedSettle_DrivesSettle(t *testing.T)      { /* ListRecoverable includes it; driver invoked */ }
func TestRecovery_PreparedRedispatchesOnce(t *testing.T)            { /* no loss (INV-1); envelope-hash verified */ }
func TestRecovery_SendClaimedIsSafelyRetryable(t *testing.T)        { /* B2: no uncertain, no duplicate */ }
func TestRecovery_ProviderAcceptedNoRedispatch(t *testing.T)        { /* countingAdapter.sends==0 (INV-2) */ }
func TestRecovery_SendStartedReconciles(t *testing.T)               { /* fake ReconcilableAdapter: completed/in-flight/unknown */ }
func TestRecovery_UnknownBecomesUncertain_HoldsIntent(t *testing.T) {}
func TestRecovery_TwoScannersOneLeaseWinner(t *testing.T)           { /* CC ledger row */ }
func TestRecovery_ExpiredLeaseTakeover(t *testing.T)                { /* INV-5: crashed owner doesn't wedge */ }
func TestRecovery_EnvelopeMismatchRefusesRedispatch(t *testing.T)   { /* F-11 -> uncertain */ }
func TestOperator_RetryAsNew_LinksPredecessor(t *testing.T)         { /* SS-17 AC-2; OP2 row */ }
func TestOperator_RetryAsNew_SupersededByNewPrompt_Rejected(t *testing.T) { /* RS row: live intent gen/envelope hash moved -> ErrSuperseded + abandon offered (SS-17 §8) */ }
func TestOperator_ResolutionIdempotent_Audited(t *testing.T)        { /* SS-17 AC-5; OP2 row */ }
func TestUncertainRun_BlocksAutomatedDispatch(t *testing.T)         { /* SS-17 AC-4; OP1 row */ }
func TestRecovery_IsIdempotentAcrossDoubleResume(t *testing.T)      {}
func TestRecovery_ClaimReloadsFreshRevisionBeforeProviderAction(t *testing.T) { /* LR: Stop/revision after Claim; stale list snapshot causes no external action */ }
func TestRecovery_CancelArrivesDuringReconcile_CancelsBeforeAttach(t *testing.T) { /* LR: ReconcileInFlight then CancelRequested => provider cancel, never resumeStreaming */ }
func TestRecovery_RootRunStopBeforeRecordCancel_CancelsNeverAttaches(t *testing.T) { /* ES: RequestRunStop durable, crash before SetCancelRequested, restart sent turn -> provider cancel/no attach */ }
func TestRecovery_ParentStopBeforeChildRecordCancel_CancelsNeverAttaches(t *testing.T) { /* ES: only parent RunStopState changes after child send -> cancel/no attach */ }
func TestRecovery_RunStopChangesDuringReconcileInFlight_CancelsNeverAttaches(t *testing.T) { /* ES: fake Reconcile signals that it will return ReconcileInFlight, waits on a test barrier, and returns that result only after the test keeps CancelRequested=false then (a) sets root RunStopState.Stopped=true or (b) for a child changes only parent RunStopState generation/stopped. The mandatory post-Reconcile fresh lease + effectiveStopRequested check must call provider cancel, must not resumeStreaming or classify uncertain, must neither clear owner intent nor write a false terminal, and must use no session/RAM mirror. */ }
func TestRecovery_AttachClaimLeaseFenceAndStopBoundary(t *testing.T) { /* RA: A pauses after in-flight result; B takes expired recovery/attach lease and claims next epoch; release A => stale token cannot attach, emit output, or forward terminal. B alone attaches. Stop/fence winning ClaimRecoveryAttach returns cancel-required and zero attach. Crash after claim is recovered after attach TTL. */ }
func TestRecovery_AttachEntryAndCallbacksAreTransactionGuarded(t *testing.T) { /* RA/TP/SA: local/Supabase RPC/real PG/model. A pauses after ClaimRecoveryAttach and separately after a callback token predicate; B terminalizes or writes uncertain. Releasing A makes EnterRecoveryAttach/RecordRecoveryAttachedEffect/CommitAttachedTerminalAndSettleIntent return ErrRecoveryAttachRevoked: no provider attach, output, terminal mutation, intent clear, settlement, or terminal audit. Cross store time past attach TTL without N+1 claim, then deliver each completed|failed|cancelled terminal proof with no Stop/root Stop/parent Stop: every call is inert. Separately, root Stop and child-parent Stop after a **current unexpired** terminal callback's token check (CancelRequested=false) do NOT make it inert: each proof commits once with proof-derived state/outcome, owner clear/settlement/audit, cancelled_in_flight, epoch revoke, and inert later callbacks. */ }
func TestRecovery_CurrentAttachToken_ForwardsTerminalExactlyOnce(t *testing.T) { /* RA: current epoch submits one proof through the attached-terminal API; duplicate callback is idempotent/stale after revoke and cannot create a second terminal/settle. */ }
func TestRecovery_RepeatedResume_RespectsActiveAttachToken(t *testing.T) { /* RA: before attach TTL, a new scanner does not steal/create a second consumer; after expiry it claims a higher epoch and the old owner is inert. */ }
func TestRecovery_RunStopChangesBeforeUnknownClassification_CancelsNotUncertain(t *testing.T) { /* ES: fake Reconcile signals it will return unknown then waits; with CancelRequested=false set root Stop or only child parent authority, release it, and prove the mandatory fresh effectiveStopRequested check calls provider cancel instead of CASAdvance(...uncertain), attaching, clearing, or fabricating a terminal. Repeat no-adapter fallback with the same barrier. */ }
func TestRecovery_GuardedUnknownAndTerminalWrites_RejectStopOrLeaseRace(t *testing.T) { /* RG: barrier after final pre-write read. Root/parent Stop wins before CommitRecoveryUnknownOrRequireCancel => RecoveryCancelRequired/provider cancel/no uncertain. After Reconcile yields proof, let lease expire and second scanner claim: old CommitRecoveredTerminalAndSettleIntent => ErrRecoveryLeaseLost/no intent clear; valid scanner commits once. */ }
func TestRecovery_EffectiveStopReadFailure_FailsClosed(t *testing.T) { /* ES: own/parent RunStopState read error => no reconcile classification, cancel or attach */ }
func TestRecovery_CompletedWithoutTerminalEvidence_BecomesUncertain(t *testing.T) { /* proof label alone cannot terminalize or clear intent */ }
func TestRecovery_TerminalProofOutcomePassedThrough(t *testing.T) { /* TP/SA: provider proof completed|failed|cancelled maps exactly to state/outcome; terminal store commit independently derives cancelled_in_flight from record cancel/own Stop/parent fence, never hard-coded completed or caller attribution */ }
func TestRecovery_CancelRequestWithoutTerminalProof_RemainsReconciliable(t *testing.T) { /* provider cancel request is non-terminal; intent held until proof/uncertain */ }
func TestRecovery_TerminalCommitCannotOverridePersistedSettleOwed(t *testing.T) { /* SO: recovery passes no settle bool; record's immutable obligation controls phase */ }
func TestOperator_ResolveUncertain_SettlementActionsAtomic(t *testing.T) { /* OR: completed/failed preserve settle owed; abandon/supersede do not settle; retry successor alone settles */ }
```

## 5. Touched Areas

- files: `dispatch_recovery.go` (new), `interactive_resume.go` (wire into resume/reconstruct), `interactive_service.go` (retire `durableIdemReplaySafe` recovery reliance), adapter files (`Reconcile` hook).
- modules: `internal/runner` recovery + adapters.
- routes: none.
- tables: none new.

## 6. Acceptance Check

- `V-1` `dispatch_recovery_test.go`: for each seeded non-terminal state, the scanner takes the correct action (dispatch / reconcile / block) exactly once.
- `V-2` No-duplicate test: a `provider_accepted` record on recovery never re-dispatches; a fake adapter asserts zero new sends.
- `V-3` No-loss test: a `prepared` record on recovery re-dispatches and delivers the intent.
- `V-4` `uncertain` test: neither clears the outer intent nor re-dispatches; surfaces a blocked state.
- `V-5` Idempotency: running the scanner twice yields the same result (no double action).
- `V-6` `go build`, `go vet`, `go test ./internal/runner` clean.
- `V-7` `LR`: a scanner re-reads after claim and before every provider side effect; a new cancel prevents attach and is sent to provider cancel first.
- `V-8` `OR`: the SS-17/SD-24 operator action table is applied in one store transaction and recovery starts exactly the prescribed settlement/no-settlement work.
- `V-9` `TP`: recovery passes through provider-proven completed/failed/cancelled exactly; cancellation request without a terminal proof keeps intent/reconciliation state and cannot write a false completion.
- `V-10` `SA`: when proof arrives after recovery observes durable root/parent Stop (including crash before record cancel), `CommitTerminalAndSettleIntent` keeps proof-derived state/outcome and derives `StopOutcome=cancelled_in_flight` from store authority; no authority leaves it empty.
- `V-11` `RG`: recovery send linearization/pre-send cancellation/unknown-no-adapter decision are lease- and Stop-authority-guarded store mutations, not `Get` then mutate; a Stop winning its barrier yields no send, atomic pre-send cancellation, or cancel-required. A recovery terminal commit whose lease expired/took over after external reconcile is rejected without intent clear, and the valid lease holder commits exactly once.

## 7. Out of Scope

- Live-path transitions and Stop fences (Task-249).
- Gate-checkpoint retry worker (Task-251).
- The full crash-matrix harness (Task-255) — this task ships unit tests for the scanner; the end-to-end kill-at-barrier matrix is Task-255.

## 8. Completion Notes

- result: **boot enumeration + redispatch fixed (2026-07-17); provider reconciliation (T-4) still NOT done.** `RecoveryScanner.ScanRun` (claim + fresh `Get` re-read) now drives real action for prepared/send_claimed via `EnsureLiveAndRedispatch`; boot enumeration and wiring both fixed per Codex review.
- **[FIXED 2026-07-17 — Codex gap #1] Boot enumerator now finds everything:** `ScanAllRecoverable` (`dispatch_recovery.go`) now calls `Store.ListRecoverable(ctx, "")` (empty runID = every non-terminal record across every run/project on both memory and multi-project-local stores — this method already existed, just wasn't used here) instead of `ListAttention` (uncertain-only). Test: `TestScanAllRecoverable_EnumeratesEveryNonTerminalState_NotJustUncertain` proves prepared/send_claimed/send_started are all visited.
- **[FIXED 2026-07-17 — Codex gap #2] prepared/send_claimed now really redispatches:** `RecoveryScanner.EnsureLiveAndRedispatch` (new nil-safe hook field) is called from `reconcileOne` when a prepared/send_claimed record has no pending cancel. `InteractiveService.ensureLiveAndRedispatch` (`dispatch_live.go`) reconstructs the run into RAM via the existing `loadPersistedRun` path if it is not already live, then calls the existing `flushDurableTurnIntents` — which redrives `startTurn` with the record's own durable idempotency key (`startTurn`'s own `durableIdemReplaySafe`/`preparedReuseTurnID` logic hash-verifies the envelope and reuses the same TurnID). This reuses the existing, already-correct relaunch channel rather than inventing new provider-launch plumbing. Tests: `TestRecoveryScanner_PreparedSafelyRetryable_TriggersRedispatch`, `TestRecoveryScanner_CancelRequestedSkipsRedispatch_PreSendCancelsInstead`, and the end-to-end `TestScanDispatchRecoveryOnBoot_ReconstructsAndRedispatches` (a run with NO live RAM state, simulating a real process restart, gets reconstructed and its durable intent flushed by the boot scan).
- **[FIXED 2026-07-17 — T-6 wiring] Boot caller now exists:** `InteractiveService.ScanDispatchRecoveryOnBoot(ctx)` (`dispatch_live.go`) is called via `go interactive.ScanDispatchRecoveryOnBoot(ctx)` in `cli/root.go`, right after `SetDispatchStore`, mirroring the existing `ScanPersistedChatsForSummaries` best-effort boot-pass pattern.
- **Still NOT done — Provider reconciliation (T-4) absent:** no `ReconcilableAdapter`/`Reconcile` hook; `send_started`/`provider_accepted` still goes straight to `CommitRecoveryUnknownOrRequireCancel` and never queries the provider (this is the CORRECT behavior per the Task-257 evidenced guarantee class — Codex/Grok/Claude have no query-by-operation-id — but the attach/reconcile machinery the store exposes for a future evidence-backed provider is still uncalled).
- **Still NOT done — Effective-Stop authority partial:** only `PreSendStopSelf`; no parent-fence / `effectiveStopRequested` helper (doesn't exist).
- Tests: `dispatch_recovery_test.go` now exists (4 new tests, above) plus the 2 pre-existing `TestRecoveryScanner_*` tests in `cp51_tasks_test.go`; still short of the 32 named §4.2 tests (attach-token, operator retry-as-new interplay, TP/SA proof pass-through remain unwritten). Acceptance: boot-enumeration + redispatch items now pass; provider-reconcile/attach items remain 0.
- follow-ups: implement provider reconcile + attach engine (T-4) for any future evidence-backed provider; add `effectiveStopRequested`; extend `dispatch_recovery_test.go` toward the full §4.2 list.
- upstream docs updated: task status (this audit + Codex-suggested fixes)

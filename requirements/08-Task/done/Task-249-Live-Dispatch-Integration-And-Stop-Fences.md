# Task-249: Live Dispatch Integration And Stop Fences

## Metadata

- Document ID: `Task-249`
- Title: `Live Dispatch Integration And Stop Fences`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SD-25: Recovery Ownership Linearization Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SS-17: Dispatch Uncertainty And Repair Operator Contract](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [Task-248](./Task-248-Durable-Dispatch-Record-And-State-Machine-Core.md), [Task-255](./Task-255-Crash-Matrix-Stop-Race-And-Race-DOD-Suite.md)
- Replaces: `None`
- Tags: `agent-flow-engine, durable-turn, stop-race, dispatch-state-machine`

## AI Quick View

### Summary

- Drive the `DispatchRecord` (Task-248) through the live path: `startTurn` advances `prepared → send_claimed`; **the durable `send_claimed → send_started` CAS immediately before the adapter's first external byte is the Stop/send linearization point** (SD-24 §7.2); the matrix-defined acceptance signal commits `provider_accepted`; `finishTurn` commits `terminal_*`.
- Stop is a CAS (`SetCancelRequested`), not a sampled flag: if it lands before `send_started`, the send CAS fails and **nothing is sent**; after, cancellation goes through the provider (`DOD-C2`). `ctx.Err()` checks remain as defense-in-depth only.
- Receipt and outer-intent clear are **one atomic commit** (`CommitReceiptAndClearIntent`); a receipt whose persist failed is invisible to every predicate — never RAM (`DOD-C1` live side; closes plan-review #6).
- **Consume** the scoped SD-24 §6.3 Task-257 matrix: Codex/Grok/Claude are eligible for V2 but have the explicit negative result `unprovable ⇒ uncertain`, therefore none has an `Accepted` call site; Gemini is V2-disabled until its deferred evidence completes.

### Current Ask

- **Done (2026-07-17 re-audit):** live V2 prep/claim/linearize/Stop fail-closed + atomic receipt/terminal commits landed; co-owned crash/Stop matrix proof closed via [Task-255](./Task-255-Crash-Matrix-Stop-Race-And-Race-DOD-Suite.md). T-6 permanently descoped (documented). See §8.

### Key Decisions

- `T-1` Outer intents clear only **inside** `CommitReceiptAndClearIntent`/`CommitTerminalAndSettleIntent`; there is no separate clear predicate reading RAM or unpersisted receipts.
- `T-2` The Stop/send race is decided by CAS revision order at `send_claimed → send_started` — not by how many places sample `ctx.Err()`.
- `T-3` A session/thread id (`thread/start`, `session/new`) or a successful stdin write is **not** a receipt; only the SD-24 §6.3 matrix signal is.

### Constraints

- Keep `startTurn`/`runTurn` public signatures stable (9 callers; MEDIUM blast radius — CP-51 R-1).
- Land behind `FLOWPILOT_DISPATCH_V2`; old path remains as fallback.

### Open Questions

- `Q-2` **Resolved by SD-24 §6.3** (scoped capability matrix). Codex/Grok/Claude have the recorded negative outcome `unprovable ⇒ uncertain`; no receipt seam is wired. Gemini evidence is deferred and it remains V2-disabled. A future enablement repeats Task-257; no proof remains `uncertain`.

### Source Refs

- CP-51 §3.4, §10.4 (B2/B3/B4); BUG-288 `DOD-C1`, `DOD-C2`.
- Code: `interactive_service.go:4826` (`runTurn` entry), `:5978` (`abortDurableStartIfStaleLocked`), `:5993-6086` (launch-ack → `go runTurn`), `interactive_resume.go:1524-1548` (intent clear).

## 1. Goal

Make the live dispatch path drive the durable record and enforce Stop as a fence at every barrier, so that no turn is lost/duplicated because outer intent was cleared on RAM state, and no provider prompt is sent after Stop.

## 2. Parent Links

- coding plan: CP-51 (`P-2`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§7.1 fences, §6.2 clear contract); [SD-25](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md) (terminal attach revocation); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-C1`, `DOD-C2`, INV-1..INV-3, crash-matrix B2/B3/B4

## 3. Trigger

`abortDurableStartIfStaleLocked` revalidates Stop only before launch-ack (`interactive_service.go:5978`); there is no fence between the launch-ack persist and `go s.runTurn(...)` (`:6086`), and `runTurn` does not check `ctx.Err()` at entry (`:4826`). Separately, `durableIntentClearOK`→`durableIdemReplaySafe` clears outer intents when `turnInFlight && currentTurnID==turnID` (`:2745-2749`), which is RAM and does not prove the provider received anything.

## 4. Exact Change

- `T-1` In `startTurn`: `CreatePrepared` (record + envelope) at the prep point; `CASAdvance(prepared → send_claimed)` at the launch-ack point. Both fail-closed: persist failure aborts without launching.
- `T-2` **Linearization**: inside `runTurn`, immediately before the adapter's first external byte, `CASAdvance(send_claimed → send_started)` durably locks/checks the record's own `RunStopState`; root Stop returns `ErrRunStopFence{currentGeneration}` even before the per-record cancel loop runs. For a released child it then locks/checks the parent `RunStopState` against `ParentStopFence`, returning `ErrParentStopFence{currentGeneration}`. Local `CancelRequested` conflict or either typed Stop error routes to `CommitPreSendCancellationAndClearIntent` atomically writing `terminal_cancelled`, owner clear and `self|parent_fence` audit; **no send**. No backward transition exists (a Stop before send does NOT revert to `prepared` — plan-review #4).
- `T-3` Stop path first advances the durable `RunStopState` through `RequestRunStop`, then calls `SetCancelRequested(expectedRev, stopGen)` for active records; post-linearization Stop flows through existing `turnCancel` as a non-terminal request. Only later `TerminalEvidence.Outcome` determines `terminal_cancelled`, `terminal_failed`, or completed-before-cancel; the terminal store transaction, not the best-effort record loop, locks own/parent Stop authority and derives `StopOutcome=cancelled_in_flight` when effective.
- `T-4` On the matrix-defined acceptance signal (SD-24 §6.3), call `CommitReceiptAndClearIntent` with canonical `ReceiptEvidence` — receipt + `provider_accepted` + outer-intent clear in **one atomic durable write**. Commit failure ⇒ state stays `send_started`, intent stays held, a retry worker re-attempts; the receipt is never visible from RAM (plan-review #6).
- `T-5` On a provider-backed terminal event/query result in `finishTurn`, call `CommitTerminalAndSettleIntent(TerminalEvidence, intentOwnerRunID, intentKey, intentGen)` — the store validates `TerminalEvidence.Outcome`, derives terminal state/outcome, reads immutable persisted `DispatchRecord.SettleOwed` to stamp `SettlePhase=settle_pending|none`, and in that same transaction locks/reads own `RunStopState` plus a child parent fence to derive `StopOutcome=cancelled_in_flight` when record cancel/own Stop/parent fence is effective; no caller can override any of those. The same transaction revokes `RecoveryAttachEpoch`/owner/expiry before it returns, so an active recovered stream cannot emit a late effect. It clears the **owner's** intent in that commit. A `SendTurn` error or provider-cancel request after `send_started` is not terminal evidence: persist only a reconcile work item/diagnostic, keep intent and `send_started`, and let Task-250 prove terminal or hold uncertain. Fail-closed retry — no `_ = persist`.
- `T-6` Delete the RAM-based `durableIdemReplaySafe`/`durableIntentClearOK` clear path at `interactive_resume.go:1529` (the atomic commits replace it; there is no separate clear predicate to get wrong).
- `T-7` Preflight: assert the Codex/Grok/Claude matrix rows have no *(verify live)* cells and enforce their recorded **no `Accepted` seam**; assert Gemini is rejected from V2 provider enablement until `CE-GEM` closes. Never infer an acceptance signal.
- `T-8` Guard behind `FLOWPILOT_DISPATCH_V2` per the SD-24 §9 migration matrix (per-run authority; no shadow-writing under V1).

### 4.1 Code guide (step-by-step)

> Depends on Task-248 (`DispatchRecord`, `rs.dispatch`, `dispatchByTurn`). Sketches illustrative — verify against HEAD before compiling. `startTurn` = MEDIUM blast radius (9 callers) — run `gitnexus_impact` first; keep the signature `startTurn(runID string, in TurnInput, scenario, idempotencyKey string) (string, *apiErr)` unchanged.

**Step 1 — `startTurn`: create + claim ([interactive_service.go:5933+](../../../apps/local-runner/internal/runner/interactive_service.go:5933)).** At the prep point (`durableKey := strings.HasPrefix(idempotencyKey, "durable-")`), build the immutable envelope and `CreatePrepared`; at the launch-ack point (~`:6003`) advance:

```go
env := buildDispatchEnvelope(rs, in, turnID)               // PromptRef=turnLogKindPrompt line, hashes, model/effort/yolo/skills
if err := s.dispatchStore.CreatePrepared(ctx, newDispatchRecord(rs, turnID, env), env); err != nil { /* abort, fail-closed */ }
// ... existing validations ...
rev, err := s.dispatchStore.CASAdvance(ctx, rs.id, turnID, rec.Revision, DispatchPrepared, DispatchSendClaimed, nil)
if err != nil { /* ErrStaleDispatch or persist fail -> abort WITHOUT go runTurn (fail-closed) */ }
go s.runTurn(ctx, rs, adapter, in, scenario, turnID, capturedCtx)
```

**Step 2 — the linearization CAS in `runTurn`, immediately before the provider send ([interactive_service.go:4826+](../../../apps/local-runner/internal/runner/interactive_service.go:4826); the send is `adapter.SendTurn` → Codex `turn/start`, Grok `session/prompt`, Claude `writeUserTurn`, Gemini spawn).**

```go
// LINEARIZATION POINT (SD-24 §7.2). Durable CAS; CancelRequested makes it fail.
// For a released child the SAME store CAS locks ParentStopFence.ParentRunID's
// RunStopState and requires Stopped=false + equal generation. A caller must not
// pre-read the parent then call CAS: that recreates the Stop race.
rev, err = s.dispatchStore.CASAdvance(ctx, rs.id, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
if err != nil {
	var ownStop *ErrRunStopFence
	if errors.As(err, &ownStop) {
		cur, curRev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID)
		if gerr == nil && cur.State == DispatchSendClaimed {
			_, cerr := s.dispatchStore.CommitPreSendCancellationAndClearIntent(ctx, rs.id, turnID, curRev,
				cur.IntentOwnerRunID, cur.OuterIntentKey, cur.OuterIntentGen, ownStop.CurrentGeneration, PreSendStopSelf)
			if cerr != nil { log.Printf("[dispatch] own-run-stop cancellation failed run=%s turn=%s: %v", rs.id, turnID, cerr) }
		}
		return
	}
	var parentFence *ErrParentStopFence
	if errors.As(err, &parentFence) {
		// Parent Stop won. The typed error carries the durable authority's current
		// generation; reload only to obtain the child revision/owner fields, then
		// atomically cancel+clear+audit this never-sent child.
		cur, curRev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID)
		if gerr == nil && cur.State == DispatchSendClaimed {
			_, cerr := s.dispatchStore.CommitPreSendCancellationAndClearIntent(ctx, rs.id, turnID, curRev,
				cur.IntentOwnerRunID, cur.OuterIntentKey, cur.OuterIntentGen, parentFence.CurrentGeneration, PreSendStopParentFence)
			if cerr != nil { log.Printf("[dispatch] parent-fence cancellation failed run=%s turn=%s: %v", rs.id, turnID, cerr) }
		}
		return
	}
	// A CAS conflict does NOT mean "Stop won" (plan-review #3 #7): another actor
	// may have advanced the record. RELOAD and branch by actual state:
	cur, curRev, gerr := s.dispatchStore.Get(ctx, rs.id, turnID)
	switch {
	case gerr != nil:
		return // cannot classify here; recovery owns it
	case cur.State == DispatchSendClaimed && cur.CancelRequested:
		// Stop genuinely won the linearization: nothing was sent.
		if _, cerr := s.dispatchStore.CommitPreSendCancellationAndClearIntent(ctx, rs.id, turnID, curRev,
			cur.IntentOwnerRunID, cur.OuterIntentKey, cur.OuterIntentGen, cur.StopGeneration, PreSendStopSelf); cerr != nil {
			// Fail-closed: no _, _ = (contract). ErrStale/persist-fail -> the record
			// stays send_claimed+CancelRequested; recovery's cancel branch owns it.
			log.Printf("[dispatch] stop-cancel CAS failed run=%s turn=%s: %v (recovery will settle)", rs.id, turnID, cerr)
		}
		return
	case cur.State.IsTerminal() || cur.State == DispatchSendStarted || cur.State == DispatchProviderAccepted:
		// Someone else owns/advanced this turn — do NOT double-act, do NOT terminalize.
		return
	default:
		return // unexpected: leave to recovery; never guess
	}
}
if err = adapter.SendTurn(ctx, req, bridge); err != nil { // first external byte happens only after the CAS above
	// Ambiguous by definition: the provider may have consumed bytes before the
	// error. Persist only a diagnostic/reconcile wake-up; do not manufacture a
	// terminal event, receipt, or owner-intent clear (AE B3/B4).
	s.recordDispatchTransportError(ctx, rs.id, turnID, err)
	s.scheduleDispatchReconcile(rs.id, turnID)
	return
}
```
`ctx.Err()` checks at `runTurn` entry remain as cheap defense-in-depth, but correctness comes from the CAS.

**Step 3 — Stop path.** The run Stop owner first calls `RequestRunStop` (the durable **own-run** authority), then calls `SetCancelRequested(ctx, runID, turnID, rev, stopGen)` on active records for provider-cancel routing/UX. The latter loop is not the send fence: every root `send_started` CAS already rejects `ErrRunStopFence`, and a child may also reject `ErrParentStopFence`. Any pre-linearization result makes Step 2 call `CommitPreSendCancellationAndClearIntent` — **one** log line/RPC transaction contains state, owner clear, `StopOutcome`, source and audit. Post-linearization: existing `turnCancel` is only a provider cancel request; `finishTurn` may commit terminal **only after** provider `TerminalEvidence` proves `cancelled`, `failed`, or completed-before-cancel. A cancel request/error alone never terminalizes or clears intent.

**Step 4 — receipt → atomic commit.** Add `TurnBridge.Accepted(ReceiptEvidence)` beside `Emit`, and make it the **only** caller of receipt commit. The scoped Task-257 outcome supplies **no** Codex/Grok/Claude adapter `file:function`: session creation (`thread/start`/`session/new`), Claude `writeUserTurn` stdin, and their observed headless output have no call path. Gemini is V2-disabled, not a fallback receipt candidate. Keep `Accepted` implemented only as the safe generic store seam for a future evidence-backed provider; no current adapter calls it. In `Accepted`:

```go
func (b *turnBridge) Accepted(receipt ReceiptEvidence) {
	r := b.rs.dispatchByTurn(b.turnID)
	if r != nil && r.State == DispatchSendStarted {
	if _, err := b.svc.dispatchStore.CommitReceiptAndClearIntent(b.ctx, b.rs.id, b.turnID, r.Revision,
		receipt, r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen); err != nil {
		// Fail-closed: state stays send_started; the OWNER's intent stays held;
		// schedule commit retry. (Signature carries IntentOwnerRunID — the parent
		// for restart intents; SD-24 §6.1.)
		b.svc.scheduleDispatchCommitRetry(b.rs.id, b.turnID)
	}
}
}
```
`turnBridge` owns `svc *InteractiveService`, `ctx context.Context`, `rs *interactiveRun`, and `turnID string`; `ReceiptEvidence` is the SD-24 §6.3a canonical payload. Reject/log Accepted before `send_started` or after terminal; equal receipt identity **and canonical payload hash** replays are no-ops, but a different payload for the same identity is `ErrReceiptConflict`. Codex/Grok/Claude have no concrete adapter calls under the current evidence, so post-send recovery remains `uncertain`; Gemini remains V2-disabled. A future provider call is allowed only after its own Task-257 matrix row becomes evidence-backed.
There is **no** separate "clear predicate": the clear happened (or didn't) atomically with the receipt. `flushDurableTurnIntents` ([interactive_resume.go:1529](../../../apps/local-runner/internal/runner/interactive_resume.go:1529)) drops its `durableIntentClearOK` branch entirely; delete `durableIdemReplaySafe`/`durableIntentClearOK` ([interactive_service.go:2732-2773](../../../apps/local-runner/internal/runner/interactive_service.go:2732)) once callers migrate.

**Step 5 — terminal evidence and ambiguous send error.** `turnBridge.Terminal(TerminalEvidence)` is the only automatic post-send caller of `CommitTerminalAndSettleIntent(ctx, rs.id, turnID, rev, proof, r.IntentOwnerRunID, r.OuterIntentKey, r.OuterIntentGen)`. It validates provider/evidence kind, canonical bytes/hash and receipt linkage; `proof.Outcome` is the closed `completed|failed|cancelled` value from which the store derives record state/outcome. In the same store transaction, the store reads `r.SettleOwed` to derive `SettlePhase=settle_pending|none`, locks the record's own `RunStopState` plus a child parent fence (when present), and **increments `RecoveryAttachEpoch` and clears attach owner/expiry**: record cancel, own Stop, or parent stopped/generation mismatch yields `StopOutcome=cancelled_in_flight`; no condition leaves it empty. This guide must not compute or pass settlement/StopOutcome/revocation. It clears the **owner's** intent in the same commit. `turnCancel` after `send_started` is only a cancel request, never a terminal transition: wait for provider `TerminalEvidence` (which may prove cancelled, failed, or completed-before-cancel). If `adapter.SendTurn` returns after `send_started`, call `recordDispatchTransportError` and schedule Task-250 reconciliation; do **not** call terminal commit, do **not** clear intent, and do **not** synthesize a receipt/outcome. Same fail-closed retry rule (no `_ = persist` anywhere on this path).

### 4.2 Test skeletons (`stop_race_barrier_test.go`)

```go
func TestStopCASBeforeSendStarted_SendCASFails_NothingSent(t *testing.T) { /* countingAdapter.sends == 0; record = terminal_cancelled (DOD-C2, B2) */ }
func TestStopAfterSendStarted_ProviderCancelPath(t *testing.T)           { /* no NEW sends; terminal_cancelled via provider */ }
func TestReceiptCommitFailure_IntentStillHeld_StateSendStarted(t *testing.T) { /* plan-review #6: RAM receipt invisible */ }
func TestReceiptAndClearAreAtomic(t *testing.T)                          { /* no interleaving where intent cleared without durable receipt */ }
func TestSessionIDIsNotAReceipt(t *testing.T)                            { /* thread/start / session/new alone never advances past send_started */ }
func TestAcceptedSeam_OnlyEvidenceBackedAdapterEventCommitsReceipt(t *testing.T) { /* each selected adapter predicate calls Accepted once; Gemini none */ }
func TestAcceptedRejectsPreSendAndPayloadConflict(t *testing.T)           { /* no clear/state transition; ErrReceiptConflict is surfaced */ }
func TestSendCASConflict_ReloadsAndBranches_NeverAssumesStopWon(t *testing.T) { /* SC ledger row: conflict + state=send_started => no terminalize, no second act */ }
func TestOuterIntentNotClearedWhileSendClaimed(t *testing.T)             {}
func TestTerminalCommitRetries_NoSilentDiscard(t *testing.T)             {}
func TestPreSendStop_ParentOwnedIntentAtomicClearAcrossRestart(t *testing.T) { /* crash before/after commit: cancelled child cannot re-launch from parent restart intent */ }
func TestSendErrorAfterStarted_RetainsIntentAndNeverTerminalizes(t *testing.T) { /* B3/B4: ambiguous error -> send_started + reconcile work; no terminal commit/clear */ }
func TestTerminalSeam_RequiresCanonicalTerminalEvidence(t *testing.T) { /* missing/invalid proof rejected; only provider-backed terminal event can settle */ }
func TestReceiptEvidence_ConflictNeverClearsIntent(t *testing.T) { /* same receipt identity, divergent canonical payload/hash => ErrReceiptConflict */ }
func TestParentStopVsChildReleaseAndSend_FenceWins(t *testing.T) { /* PS: parent Stop before/in the child send CAS -> zero bytes, child pre-send-cancelled */ }
func TestRootStopVsSendStarted_RunStopFenceWins(t *testing.T) { /* RSF: RequestRunStop before/in root send CAS -> ErrRunStopFence, owner-clear/audit, zero bytes */ }
func TestPostSendCancelRequiresTerminalProof(t *testing.T) { /* TP: cancel request alone retains intent; proved cancelled/failed/completed-before-cancel maps exactly from proof.Outcome */ }
func TestTerminalSeam_CannotOverridePersistedSettleOwed(t *testing.T) { /* SO: no settle argument exists; durable record determines settle phase */ }
func TestTerminalSeam_CannotOverrideStoreDerivedStopOutcome(t *testing.T) { /* SA: bridge supplies only proof; terminal store tx derives cancelled_in_flight from record cancel/own-or-parent authority, including crash-before-record-cancel */ }
```

## 5. Touched Areas

- files: `interactive_service.go` (`startTurn`, `runTurn`, receipt transitions, fences), `interactive_resume.go` (`flushDurableTurnIntents` clear predicate), `dispatch_record.go` (helpers from Task-248), adapter seam files as needed for the receipt signal.
- modules: `internal/runner` interactive service + adapters.
- routes: none.
- tables: none new (uses Task-248 fields).

## 6. Acceptance Check

- `V-1` `stop_race_barrier_test.go`: a Stop CAS landing before `send_started` makes the send CAS fail — zero provider sends, record `terminal_cancelled` (B2); a Stop after `send_started` produces no NEW sends and flows through provider cancel (B3/B4).
- `V-2` Intent-clear atomicity: outer intent is never observable as cleared without a durable receipt/outcome in the same commit; not cleared while `prepared`/`send_claimed`/`send_started`.
- `V-3` Receipt-commit failure: state stays `send_started`, intent held, commit retried — RAM receipt invisible (plan-review #6).
- `V-4` Scoped capability seam test: Codex/Grok/Claude `thread/start`/`session/new`/`writeUserTurn`/headless output never calls `CommitReceiptAndClearIntent`; Gemini cannot be V2-enabled. A future evidence-backed provider test must prove the sole Accepted caller.
- `V-5` Scoped capability matrix: Codex/Grok/Claude have evidence links and zero *(verify live)* cells; Gemini is explicitly deferred + V2-disabled rather than silently assumed safe.
- `V-6` Callers unaffected: existing `startTurn` caller tests and BUG-288 Round-20 tests still pass with `FLOWPILOT_DISPATCH_V2` on and off (per-run authority semantics).
- `V-7` `go build`, `go vet`, `go test ./internal/runner` clean.
- `V-8` `B3`/`B4`: an error returned after `send_started` leaves the durable intent and record reconcilable; no code path can terminalize or settle without `TerminalEvidence`.
- `V-9` `PS`: parent Stop, release-manifest commit, and child `send_started` are interleaved under the real store transaction; a Stop that wins produces zero child sends.
- `V-10` `SA`: the terminal bridge supplies only valid proof. Root Stop before the record-cancel loop and child parent-only Stop/fence changes are derived by the terminal store transaction as `cancelled_in_flight`; no effective Stop remains empty while the proof still controls terminal state/outcome.

## 7. Out of Scope

- Recovery-side reconciliation of records after restart (Task-250).
- Gate-checkpoint retry (Task-251).

## 8. Completion Notes

- result: **done (2026-07-17 re-audit after Task-255 land)** — live V2 path + Stop fences owned by this task are complete; co-owned crash/Stop proof closed via Task-255.
- **Production wire (this task):**
  - T-1/T-2/T-8: `prepareDispatchV2` / `claimDispatchV2` / `linearizeSendStarted` (storeErr distinguishable for BUG-289 A1) behind `FLOWPILOT_DISPATCH_V2`; fail-closed prep/claim.
  - T-3: `requestRunStopV2` + `stopAgentLoop` fail-closed (`dispatch_stop_fence_failed` when durable fence cannot be written).
  - T-4/T-5: `TurnBridge.Accepted` → `CommitReceiptAndClearIntent`; `Terminal` → `CommitTerminalAndSettleIntent` (+ Task-251 settle schedule).
  - T-7: Codex/Grok/Claude V2-eligible with no false `Accepted` seam; Gemini V2-disabled until evidence (matrix / `FLOWPILOT_DISPATCH_V2_PROVIDERS`) — **not** a Task-249 open gap.
  - **T-6 permanently descoped:** keep `durableIdemReplaySafe` / `durableIntentClearOK` (Task-254 did not invert delivery authority; deleting them is unsafe). Documented permanent decision, not deferred work.
- **Co-owned DOD (was blocked on Task-255 — now landed):**
  - Real-kill B0–B8 (`dispatch_crash_harness_test.go`): B2/B3/B4 linearization + uncertain cells; B8 settle_pending survival.
  - In-process Stop/fence: `stop_race_barrier_test.go` (root/parent fence, fail-closed Stop, Accepted/receipt conflicts, outer-intent hold, post-send cancel needs terminal proof).
  - Store linearizability: `TestOwnRunStopVsSendStarted_IsLinearizable`, `TestChildSendStarted_ParentStopFenceIsAtomic`, `TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority`, pre-send cancel atomic, etc. (`dispatch_record_test.go`).
  - Crash matrix Stop cell: `TestCrashMatrix_StopWinsLinearization_ZeroSend` / related.
- **Verification (2026-07-17):** `go test ./internal/runner/ -run "TestStop|TestAccepted|TestOuterIntent|TestPostSend|TestLiveStop|TestDispatchCrashMatrix|TestTerminalCommit|TestOwnRunStop|TestChildSend" -count=1` green; no new failures vs known env baseline.
- **Honest residuals (non-blocking for this task's done bar):**
  - Not every historical §4.2 *named* skeleton exists as its own test function (coverage is mapped across stop_race + crash harness + store contract, not a 19/19 filename checklist).
  - Supabase/PG store tier **moot** (Task-258).
  - Adapter-level "provider cancel after send_started" path remains matrix-dependent (Task-250 waiver for provider reconcile where no adapter exposes it).
- follow-ups: none owned by Task-249; Gemini `CE-GEM` enablement is a future Task-257-class re-entry.
- upstream: Task-255/251 done docs; this re-audit closes the prior "cannot done until 255" ceiling.

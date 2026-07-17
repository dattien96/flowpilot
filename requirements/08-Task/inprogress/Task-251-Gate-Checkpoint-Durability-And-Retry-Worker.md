# Task-251: Gate Checkpoint Durability And Retry Worker

## Metadata

- Document ID: `Task-251`
- Title: `Gate Checkpoint Durability And Retry Worker`
- Phase: `task`
- Status: `in_progress` (2026-07-17: 2 contained fixes landed (event keyed-upsert, persist-error observability); full T-1 driver refactor + T-3 retry-worker wiring **handed off to [BUG-289](../../09-BugFix/todo/BUG-289-Flow-Mode-Invariant-Audit-25-Unhandled-Bug-And-Edge-Cases.md) `F-7`** — do not duplicate that work here; this task closes only once BUG-289 `F-7` lands with tests, see §8)
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md), [SS-17: Dispatch Uncertainty And Repair Operator Contract](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [BUG-289](../../09-BugFix/todo/BUG-289-Flow-Mode-Invariant-Audit-25-Unhandled-Bug-And-Edge-Cases.md) (`A3`/`F-7` — owns the remaining T-1/T-3 work below; 2026-07-17 decision: do it once there, not twice)
- Replaces: `None`
- Tags: `agent-flow-engine, flow-gate, crash-recovery, retry-backoff`

## AI Quick View

### Summary

- Today: after three failed settle persists the state is RAM-only, retried once; `notifyTurnIdle` treats settle as busy and never retries → wedge on recovered outage, loss on crash. Worse, settlement is **seven side effects** (`signalChild`, event persist, broadcast, cohort append, node DONE, finalizer, dependents release — verified `interactive_service.go:3226-3266`), so a naive replay double-runs them and an early clear loses the tail.
- This task builds the **settle phase driver** on the durable `SettlePhase` sub-lifecycle (SD-24 §5.3): **effects first (each a replay-convergent consequence), phase CAS after** — exactly-once *outcomes* without depending on marker durability (INV-7, ledger `ST`/`EL`), with dispositions for reprompt/block/Stop (`SD`) and a backoff retry worker for persist outages.

### Current Ask

- Make gate-settle durability self-healing: no permanent wedge on transient outage, no loss on crash.

### Key Decisions

- `T-1` Settle durability rides the dispatch record (`D-9`): `SettlePhase=settle_pending` is stamped in the same `CommitTerminalAndSettleIntent` CAS as the terminal outcome — no RAM-only settle state exists. Total-outage answer: if that commit never became durable, the record is still `send_started`/`provider_accepted` and recovery reconciles → settle re-derives.
- `T-2` Settlement is a durable phase lifecycle (SD-24 §5.3), not one boolean: the phase driver (refactor of `resumePendingFlowGate`'s body, preserving its single-flight/epoch/stop-generation guards) runs each phase's **effects first, then CAS-marks the phase completed** (`SettlePhase` = last *completed* phase). Correctness rests on the SD-24 **idempotency doctrine** — every consequence is keyed-upsert / monotonic / create-if-absent, so replay after ANY crash (including between an effect and its `effectDone` marker) converges instead of duplicating. `notifyTurnIdle` is not involved (verified: it only flushes resume/reprompt intents).

### Constraints

- Do not change gate rule semantics; only the durability/retry of the settle lifecycle.
- Backoff bounded (cap the delay). **Stop policy (SD-24 §5.3, plan-review #4 #7):** settle of an already-terminal turn is bookkeeping — the worker does **not** exit on Stop; it completes the phases, with only the dependents-release suppressed by stop-generation and recorded as `suppressed_by_stop` **audit metadata after `SettlePhase=finalized`**. A terminal dispatch must never park with an unfinalized settle forever.

### Open Questions

- `Q-1` Should repeated settle-persist exhaustion escalate to `repair_required` after N cycles? Default: keep retrying with capped backoff + alert; escalate only if unbounded.

### Source Refs

- CP-51 §10.1 `DOD-I3`; BUG-288 P1-16 / R15–R20 settle checkpoint.
- Code: `interactive_service.go:3363-3390` (three-persist + RAM blocked), `:5147-5176` (one immediate retry), `interactive_resume.go:1573-1581` (`notifyTurnIdle` busy check).

## 1. Goal

Ensure a pending gate settle survives storage outages and crashes: retried automatically with backoff when storage recovers, and never held only in RAM in a way a crash can lose.

## 2. Parent Links

- coding plan: CP-51 (`P-4`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§8 F-4); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-I3`, INV-5

## 3. Trigger

After three failed settle persists, `emitLocked` sets `intentBlockedKind="gate_settle_checkpoint"` + `gateCheckpointNotDurable=true` in RAM and returns false (`interactive_service.go:3369-3379`). `runTurn` retries once (`:5151-5176`). `notifyTurnIdle` computes `busy = turnInFlight || pendingFlowGateSettle || postTurnGateCancel != nil` and returns without flushing when settle is pending (`interactive_resume.go:1573-1581`). There is no backoff worker, so a storage recovery does not re-drive the settle → the run wedges; a crash loses the RAM-only mark.

## 4. Exact Change

- `T-1` **Settle phase driver** (`dispatch_settle.go`): refactor `resumePendingFlowGate`'s body into per-phase executors mapped to SD-24 §5.3. **Order is effects-first, CAS-after**: run the next phase's effects, then `CASAdvanceSettle` marks the phase completed. Crash anywhere ⇒ replay the phase's effects — safe because **every consequence is replay-convergent by construction** (the SD-24 doctrine), NOT because a ledger check preceded it (a pre-check cannot be atomic with the effect — plan-review #4 #1). `SettlePhase` semantics = *last completed phase*.
- `T-2` **Per-effect convergence contracts** (ledger `EL`/`CD`/`RM`/`PS`): completion event = keyed upsert by (runID, turnID) (fix the type-only replace at `interactive_service.go:3239`; handle `persistEvent` errors `:3245`); **cohort entry = payload-bearing durable effect record** `RecordEffectDone(kind="cohort:<cohortID>:<childRunID>", payload, payloadHash)` — the current RAM/label-deduped `appendCohortResult` (`agent_orchestrator.go:92`) becomes only a projection rebuilt from the store; **dependents release = revisioned durable manifest** (one `pending→created|suppressed` item per dependent: `CommitReleaseManifestItem` transaction-locks the parent's `RunStopState{generation,stopped}` authority; Stop-first marks `suppressed`, release-first creates a deterministic-key child with that generation copied to `ParentStopFence`; the child's later `send_started` CAS locks/rechecks the same authority and its typed fence error is routed to pre-send cancellation); finalizer = keyed artifact overwrite (replaces the RAM map `finalizer.go:49`); `signalChild` waiter wakeups best-effort per INV-7 scope. The `effectDone` marker is **skip-optimization + audit only** — correctness never depends on it.
- `T-2b` **Dispositions** (plan-review #3 #8; ledger `SD`): gate reprompt/block ⇒ CAS `settle_superseded_reprompt` (durable terminal disposition; the reprompt/block flow owns the next dispatch — recovery does nothing further). Stop mid-settle ⇒ bookkeeping phases continue (not "new user work"); `dependents_released` CAS-records its manifest item `suppressed_by_stop`, then the driver still enters `finalized` and performs the keyed finalizer exactly once. `suppressed_by_stop` is terminal audit metadata, never an early phase exit.
- `T-3` **Retry worker** for persist failures at any phase CAS: capped exponential backoff, single-flight per run, cancel-safe (Stop / parent stop-generation — preserve the BUG-288 P1-04/P1-06 guards), driving the **phase driver** — not `notifyTurnIdle` (verified: it only flushes resume/reprompt intents, `interactive_resume.go:1563-1582`).
- `T-4` **Total-outage answer** (plan-review #2 #7): there is no RAM-only settle state to lose — `SettlePhase=settle_pending` is written atomically inside `CommitTerminalAndSettleIntent` (Task-249). If that commit itself never became durable, the record is still `provider_accepted` and recovery reconciles → settle re-derives. Delete the `gateCheckpointNotDurable` RAM flag and its three-persist dance (`interactive_service.go:3363-3390`, `:5147-5176`) once the driver lands.

### 4.1 Code guide (step-by-step)

> **Consumes** Task-248's record/store API and Task-249's terminal commit (see Out of Scope — this task is the settle consumer, NOT independent). Sketches illustrative.

**Step 1 — the phase driver (`dispatch_settle.go`, new).** Refactor `resumePendingFlowGate`'s settle body (the verified sequence at [interactive_service.go:3226-3266](../../../apps/local-runner/internal/runner/interactive_service.go:3226)) into a driver over `SettlePhase`:

```go
// driveSettle resumes from the LAST COMPLETED phase (SettlePhase semantics).
// Order per SD-24 §5.3: run the next phase's EFFECTS FIRST (each invokes its
// own convergent durable operation, never a pre-ledger check), THEN CAS the
// phase as completed — crash between them replays the same convergent operation
// (INV-7). Preserves the
// resumePendingFlowGate guards: single-flight, gate epoch, parent
// stop-generation (BUG-288 P1-04/P1-06).
func (s *InteractiveService) driveSettle(runID, turnID string) {
	for {
		r, rev, err := s.dispatchStore.Get(ctx, runID, turnID)
		if err != nil || r.SettlePhase == SettleFinalized || isSettleDisposition(r.SettlePhase) { return }
		next, effects := settlePlan[r.SettlePhase] // closed table per SD-24 §5.3 (incl. disposition exits)
		// 1) EFFECTS FIRST. Every consequence is replay-CONVERGENT (keyed upsert /
		//    monotonic / unique-keyed effect record) — replay after ANY crash,
		//    including between an effect and its effectDone marker, converges
		//    instead of duplicating. The ledger (ListEffects) is only a skip
		//    optimization + audit trail.
		if disposition, err := effects(s, runID, turnID); err != nil {
			s.scheduleSettleRetry(runID, turnID) // persist/effect failure -> backoff, single-flight
			return
		} else if disposition != "" {
			// Only gate reprompt/block exits early. Stop suppression is persisted on
			// the release item and MUST retain next=dependents_released so finalizer runs.
			next = disposition
		}
		// 2) CAS AFTER — marks the phase COMPLETED. Crash between (1) and (2)
		//    replays (1) safely. The inverse order would lose effects (#1).
		if _, err := s.dispatchStore.CASAdvanceSettle(ctx, runID, turnID, rev, r.SettlePhase, next); err != nil {
			s.scheduleSettleRetry(runID, turnID) // ErrStale -> reload on retry
			return
		}
	}
}
```

**Step 2 — per-effect convergence contracts (the doctrine, SD-24 §5.3 — a ledger check alone is NOT the mechanism, plan-review #4 #1).**
- completion event → **keyed upsert** by (runID, turnID): fix the type-only replace at [interactive_service.go:3239-3243](../../../apps/local-runner/internal/runner/interactive_service.go:3239); check the persist at [:3245](../../../apps/local-runner/internal/runner/interactive_service.go:3245) (retry via the worker, never `_ =`).
- cohort entry → `RecordEffectDone(kind="cohort:<cohortID>:<childRunID>", payload, payloadHash)` **in the dispatch store**; equal replay is a no-op, divergent payload is `ErrEffectConflict`; `appendCohortResult` reads/rebuilds only this durable projection.
- child status / node DONE → already **monotonic** (guards exist) — verify, don't reinvent.
- dependents release → `CreateReleaseManifestItem` first; then `CommitReleaseManifestItem` transaction-locks the parent's durable `RunStopState`: stopped ⇒ `pending→suppressed`; live generation ⇒ atomically creates the deterministic-key child `prepared` record **with that exact `ParentStopFence`** and advances `pending→created`. The child may send only through Task-248's `send_started` CAS, which locks/rechecks the same authority; `ErrParentStopFence` must immediately route to the atomic pre-send cancellation. Neither manifest creation nor a local cache check is treated as permission to send. Replay reads the item; it never infer-skips from a marker alone.
- finalizer → **keyed overwrite** artifacts by (runID, turnID) (replaces the RAM map at [finalizer.go:49](../../../apps/local-runner/internal/runner/finalizer.go:49)); chat summary debounced/keyed.
- After a consequence external to the dispatch store: write its hash-bound `RecordEffectDone` marker (equal duplicate only; divergent payload = `ErrEffectConflict`) as **skip-optimization + audit only**. A cohort/release item is already its own durable consequence and is not written a second time as a blank marker.

**Step 3 — retry worker.** `scheduleSettleRetry` = single-flight per (runID, turnID), capped exponential backoff; each attempt re-enters `driveSettle` (re-reads the durable phase — no RAM carry-over). **The worker does not exit on Stop** (Constraints): Stop only flips the dependents-release decision via stop-generation; the machine always reaches `finalized` or a durable disposition.

**Step 4 — delete the legacy dance.** Remove `gateCheckpointNotDurable` + the three-persist retry at [interactive_service.go:3363-3390](../../../apps/local-runner/internal/runner/interactive_service.go:3363) and the checkpoint-blocked branch at [:5147-5176](../../../apps/local-runner/internal/runner/interactive_service.go:5147); `notifyTurnIdle` keeps flushing only resume/reprompt (unchanged — settle is no longer its concern). Recovery hands unfinalized settles to `driveSettle` (Task-250 Step 4).

### 4.2 Test skeletons (`gate_checkpoint_outage_test.go`)

```go
func TestSettleDriver_ResumesFromEachPhase(t *testing.T)          { /* seed each durable phase; driver completes exactly the remaining effects */ }
func TestSettleEffects_ConvergentOnReplay(t *testing.T)           { /* run a phase's effects twice: one event, one cohort entry, one release decision, one finalize (INV-7, ST) */ }
func TestSettle_CrashBetweenEffectAndMarker_NoDuplicate(t *testing.T) { /* EL row: effect done, marker lost -> replay converges (keyed/unique), never doubles */ }
func TestSettle_StopMidSettle_BookkeepingCompletes_ReleaseSuppressed(t *testing.T) { /* Stop at release: zero child dispatch, manifest=suppressed, exactly one event/cohort/node/finalizer, phase=finalized */ }
func TestCohortEntries_DurableProjection_ConcurrentConverge(t *testing.T) { /* CD row: effect records unique-keyed; RAM rebuilt on load; concurrent appends converge */ }
func TestReleaseManifest_CrashMidRelease_NoLostNoDuplicateDependent(t *testing.T) { /* RM row: pending re-dispatched via deterministic key; created skipped; suppressed honored */ }
func TestSettle_OutageThenRecovers_AutoAdvances(t *testing.T)     { /* CAS fails K then ok; no manual nudge (DOD-I3) */ }
func TestSettle_CrashBetweenPhases_NoDoubleNoLoss(t *testing.T)   { /* B8a..B8e cells (delegated to matrix; unit variant here) */ }
func TestSettle_StopAndParentStopGenGuardsPreserved(t *testing.T) { /* BUG-288 P1-04/P1-06 non-regression */ }
func TestSettle_TotalOutage_RederivedFromRecord(t *testing.T)     { /* terminal commit never durable -> reconcile path re-derives settle */ }
func TestReleaseManifest_ParentStopFencePreventsChildSend(t *testing.T) { /* PS: Stop racing pending→created / child claim / child send has zero child bytes when Stop wins */ }
```

## 5. Touched Areas

- files: `dispatch_settle.go` (new driver), `interactive_service.go` (settle path removal + event keyed-replace), `agent_orchestrator.go` (cohort projection + release manifest consumption), `finalizer.go` (keyed overwrite), `interactive_resume.go`.
- modules: `internal/runner` flow-gate settle + orchestrator consequences.
- routes: none.
- tables: **consumes `dispatch_effects`** (Task-248 schema) for cohort entries + release-manifest items + effect markers; no other new tables. The legacy `gateCheckpointNotDurable` fields are deleted.

## 6. Acceptance Check

- `V-1` `gate_checkpoint_outage_test.go`: persist fails K times then succeeds → the worker drives the settle to durable and the run advances with no manual action.
- `V-2` Wedge test: with the pre-fix behavior a settle-pending run never advances after recovery; post-fix it does (test fails on old code).
- `V-3` Crash test: settle state is recoverable after a real crash at every point — between phases AND between an effect and its `effectDone` marker (replay converges; ledger `ST`/`EL`).
- `V-4` Stop policy: Stop mid-settle **completes bookkeeping** (event/cohort/node/finalizer) and suppresses only the dependents release (manifest item `suppressed_by_stop`, final `SettlePhase=finalized`); no terminal dispatch is ever left with an unfinalized settle; no new user work is started (SS-16 `BR-6`-consistent).
- `V-5` Disposition tests: gate reprompt/block ⇒ `settle_superseded_reprompt`, recovery takes no further settle action (ledger `SD`).
- `V-6` `go build`, `go vet`, `go test -race ./internal/runner` clean for the settle path.

## 7. Out of Scope

- The record/store API definitions (Task-248) and the terminal commit that stamps `settle_pending` (Task-249) — this task **consumes** them (it is NOT orthogonal to the dispatch record; it is its settle consumer).
- Supabase versioning (Task-253); operator surface (Task-256).

## 8. Completion Notes

- result: **stub + two contained fixes (still NOT done)** — `SettleDriver.DriveSettle` + `planNext` + `CASAdvanceSettle` + `RetrySettleWithBackoff` remain a standalone, unwired unit in `dispatch_settle.go`.
- **2026-07-17 — deliberate decision NOT to attempt the full T-1 driver refactor in this pass.** `resumePendingFlowGate` is ~350 lines carrying the accumulated fixes of ~10 separate BUG-288 rounds (P1-04, P1-05, P1-06, P1-16, R11 #4, R15–R17, R19, R20-3, …), each guarding a specific crash/Stop/race window. Rewiring it into a phase-driver with 7 real convergent effects (keyed event upsert, durable cohort projection, release-manifest with parent-stop-fence, keyed finalizer) while preserving every one of those guards is a genuine multi-day task requiring line-by-line cross-reference against each historical round. Attempting it in this pass — under session time pressure — would risk silently reopening a bug that took 20 rounds to close, which is a worse outcome than an honest "not done." This is the same class of judgment call as Task-254's T-1 descope, but here the underlying task (T-1/T-2/T-3/T-4) genuinely IS still required by CP-51 (unlike Task-254's T-1) — it is deferred, not waived.
- **What WAS fixed (real, contained, tested, verified against the actual current code — not the stub):**
  - **The "type-only replace" bug (ledger `EL`), fixed for real:** the completion-event upsert at the actual current line (`interactive_service.go`, inside `resumePendingFlowGate`'s gate-pass branch) compared only `rs.events[n-1].Type == EventTurnCompleted`, so a DIFFERENT turn's completion event ending up last (e.g. a fast-completing sibling) would be silently overwritten instead of appended. Now keyed by `(Type, ProviderTurnID)`. New regression test `TestResumePendingFlowGate_CompletionEventKeyedByTurnID` seeds a prior turn's completion as the last event and proves both survive.
  - **Persist-error swallow removed:** `_ = s.persistEvent(completedEv)` now logs the error instead of silently discarding it. (Not the full retry-worker fix — T-3 remains not built — but strictly better than before: the failure is now observable.)
  - Both fixes verified against the FULL BUG-288/gate/settle/V9/V10 regression suite — zero regressions — and are surgical (a few lines each), not part of the architectural swap, so they carry materially lower risk than T-1.
- **Still NOT done (genuinely large, deferred as a dedicated future session, not silently dropped):**
  - T-1: `resumePendingFlowGate` not refactored into the phase driver; `SettleDriver` remains unwired (0 production callers).
  - T-2 (remainder): cohort entry is still the RAM/label-dedup `appendCohortResult`, not a durable `RecordEffectDone` projection; dependents-release is not the durable revisioned manifest with `ParentStopFence`; finalizer is not a keyed-overwrite durable effect.
  - T-3: no retry worker (`scheduleSettleRetry` exists on the stub driver only, unwired).
  - T-4: the legacy `gateCheckpointNotDurable` + three-persist dance is fully intact — NOT deleted (deleting it before T-1/T-3 land would remove the only durability mechanism currently protecting this path).
  - Tests: `gate_checkpoint_outage_test.go` still absent; 1/11 named skeletons (the pre-existing happy path) + the 1 new regression test above (differently named). Acceptance 0/6 for the full V-1..V-6 (V-6 build/vet pass; the rest require the undone driver).
- **2026-07-17 ownership decision — T-1/T-3 handed off to BUG-289, not duplicated here:** BUG-289's own re-audit of the whole Flow Mode engine independently found the exact same gap and gave it a concrete user-visible consequence this task's notes didn't fully spell out — `A3`: because `SettleDriver`/`CASAdvanceSettle` has no production caller, every turn's `SettlePending` obligation never resolves, so `HasNonTerminal` never goes false, permanently blocking the 90-day session prune and leaking `dispatch.ndjson` growth forever. BUG-289 `F-7` proposes wiring `SettleDriver` a production caller (boot scanner / post-terminal) as part of its own fix batch (alongside `A2`, the CP-51 Task-250-adjacent "Stop-then-crash record only gets `log.Printf`'d, never a real recovery action" gap, which has the same permanent-non-terminal / never-pruned consequence). Rather than doing the same T-1/T-3 refactor twice under two different documents, it is now owned and delivered **once**, under BUG-289 `F-7`. This task stays `in_progress` — it does **not** close until BUG-289 `F-7` lands with its own tests (a real production caller for `SettleDriver`, verified to unblock `HasNonTerminal`/prune); at that point both this task and BUG-289's `A3`/`F-7` close together, referencing the same commit.
- follow-ups: none owned directly by this task anymore for T-1/T-3 — track via [BUG-289](../../09-BugFix/todo/BUG-289-Flow-Mode-Invariant-Audit-25-Unhandled-Bug-And-Edge-Cases.md) `F-7`/`A3`. Until that lands, the legacy checkpoint mechanism (T-4) must stay in place.
- upstream docs updated: task status (this audit + the two 2026-07-17 contained fixes + the 2026-07-17 BUG-289 hand-off decision).

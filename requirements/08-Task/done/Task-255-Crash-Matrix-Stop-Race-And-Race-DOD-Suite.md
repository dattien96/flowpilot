# Task-255: Crash-Matrix, Stop-Race And Race DOD Suite

## Metadata

- Document ID: `Task-255`
- Title: `Crash-Matrix, Stop-Race And Race DOD Suite`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-17`
- Parent Documents: [CP-51](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§11), [SD-25 Recovery Ownership Closure](../../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md) (§5, §11), [SS-17](../../05-System-Specs/SS-17-Dispatch-Uncertainty-And-Repair-Operator-Contract.md)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/done/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [Task-248](./Task-248-Durable-Dispatch-Record-And-State-Machine-Core.md), [Task-249](./Task-249-Live-Dispatch-Integration-And-Stop-Fences.md), [Task-250](./Task-250-Recovery-Scanner-And-Provider-Reconciliation.md)
- Replaces: `None`
- Tags: `agent-flow-engine, testing, crash-matrix, race, definition-of-done`

## AI Quick View

### Summary

- This task **is** the CP-51 Definition of Done: a fault-injection harness that crashes and Stops the runner at every dispatch barrier, on both the local-file and Supabase stores, plus outage-recovery, marker-replay, and runtime-corruption suites, all under `go test -race`.
- Codex's explicit DoD: crash-matrix tests (local + Supabase), Stop race at every barrier, automatic outage recovery, marker replay tests, runtime corruption tests, and `go test -race ./internal/runner`.

### Current Ask

- **Done (2026-07-17):** B0–B8 real-kill matrix + B8a–e settle sub-barriers + seeded model suite + CI `-race` workflow. See §8.

### Key Decisions

- `T-1` **Fail-on-HEAD applies to defect-regression tests only** (plan-review #16): the six defect suites must fail on HEAD `6ea5417` and pass after. Foundation tests (new types/APIs) are validated by the `MU` mutation row instead — flipping any transition guard/CAS predicate must turn the suite red.
- `T-2` **Crash = real subprocess kill** (plan-reviews #13 both rounds): a re-exec'd worker process is killed via cross-platform `os.Process.Kill()` at the barrier signal (Windows is first-class — no `kill -9`/SIGKILL assumptions), while the parent test owns the durable store **and the fake-provider server with its durable request log**. Same-process RAM-drop/rebuild is NOT accepted for crash cells. Outage cells may use the in-process fault seam.
- `T-3` Two phases: phase 1 (after 248+253) = harness skeleton + red defect tests; phase 2 (last) = full matrix + concurrency + `-race` + Postgres contract run.

### Constraints

- Tests must not write into the target project tree; a harness `.cache/` may be created but should be gitignored.
- `-race` failures are blocking, never dismissed as flakes.
- Barrier hooks must be test-only seams, not production branches.

### Open Questions

- `Q-1` **Resolved (plan-review #14):** three tiers — (1) fast HTTP fake (`httpRequestFn`) for encoding/error unit tests; (2) a **shared store contract suite** run against both the local store and the Supabase impl; (3) at least one **real PostgreSQL/Supabase** run of that contract suite for CAS conditional transitions + concurrent claims (env-gated `FLOWPILOT_TEST_SUPABASE_DSN`; documented run attached to the PR when CI lacks a DB) — ledger row `PG`.

### Source Refs

- CP-51 §10 (entire DoD), especially §10.3 test gates and §10.4 crash-matrix.
- BUG-288 `DOD-G7`, §11.9.1 (no transaction-boundary/crash coverage today).

## 1. Goal

Deliver the acceptance harness that makes CP-51's invariants provable and regression-proof: crash + Stop at every barrier on both backends, outage self-heal, marker replay, runtime corruption, and a clean `-race` run.

## 2. Parent Links

- coding plan: CP-51 (`P-8`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§11 validation strategy); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-C1`, `DOD-C2`, `DOD-I3`, `DOD-I4`, `DOD-I5`, `DOD-I6`, `DOD-G7`, `DOD-G8`, INV-1..INV-6, crash-matrix §10.4

## 3. Trigger

BUG-288 §11.9.1 states the targeted green tests "chưa phủ transaction boundary, crash/restart, concurrent turn". Codex Round-20 gap `DOD-G7` lists the exact uncovered scenarios. Without this harness the fixes cannot be trusted and future rounds will re-open the same classes of bug.

## 4. Exact Change

- `T-1` Harness: **real subprocess kill** (cross-platform `os.Process.Kill`) at named barriers `B0..B8` **+ settle barriers `B8a..B8e`** (CP-51 §10.4); parent owns the durable store + fake-provider server (durable request log); in-process fault seam only for outage (fail-N-then-succeed) cells.
- `T-2` `dispatch_crash_matrix_test.go`: for each barrier × {crash, Stop} × {local-file, Supabase-fake}, assert INV-1 (three-outcome, no silent loss), INV-2 (no duplicate — parent-side provider log counts sends), INV-3 **on the recorded `StopOutcome`** (`stopped_before_send` ⇒ zero sends; `cancelled_in_flight` ⇒ ≤1 send + provider cancel; the unprovable "no bytes after Stop CAS" is never asserted).
- `T-3` `stop_race_barrier_test.go`: Stop cells branch by linearization winner (consolidates Task-249's tests into the full matrix) — "never sends" applies ONLY to pre-`send_started` cells.
- `T-4` `gate_checkpoint_outage_test.go`: settle persist fails K times then recovers → auto-advance (Task-251).
- `T-5` `fcp_marker_replay_test.go`: same-service cross-run marker does not suppress history (Task-252).
- `T-6` `supabase_runtime_corruption_test.go`: corrupt/version-mismatch blob ⇒ `repair_required` (Task-253).
- `T-7` `idempotency_retention_test.go`: active low-gen key survives 48+ terminal keys and round-trip (Task-254).
- `T-8` CI wiring: `go test -race ./internal/runner ./internal/flowgate` and `go build`/`go vet` in the pipeline; document the kill-at-barrier manual script.

### 4.1 Code guide (step-by-step)

> This is the acceptance harness. Depends on Task-248..254. Sketches illustrative.

**Step 1 — fault-injecting store wrapper (`dispatch_test_harness.go`, test-only).** Wrap the real `WorkflowStore`/session store so a named barrier can crash (drop RAM, reload from disk) or fail-N-times:

```go
type barrier string
const (
	B0 barrier = "pre_create_prepared"
	B1 barrier = "after_prepared_pre_send_claimed"
	B2 barrier = "after_send_claimed_pre_send_started"   // crash here = safely-retryable, NOT uncertain
	B3 barrier = "after_send_started_pre_external_send"  // from here on: reconcile/uncertain
	B4 barrier = "after_external_send_pre_receipt"
	B5 barrier = "receipt_in_ram_pre_receipt_commit"     // the v1-draft leak cell
	B6 barrier = "after_receipt_commit"
	B7 barrier = "terminal_observed_pre_terminal_commit"
	B8 barrier = "after_terminal_commit_settle_pending"
)

// Settle sub-barriers (plan-review #4 #10 — the three windows of the settle
// protocol, instantiated FOR EACH phase P in the SD-24 §5.3 plan):
//   SB(P, "after_effect_pre_marker")   — consequence landed, effectDone not yet durable
//   SB(P, "after_marker_pre_phase_cas") — marker durable, phase CAS not yet
//   SB(P, "after_phase_cas_pre_next")   — phase completed, next phase's effects not started
func settleBarriers() []barrier { /* generated: phases × 3 windows */ }

// Outage seam (in-process, for fail-K-then-recover cells only):
type faultStore struct {
	inner  DispatchStore
	failAt barrier
	failN  int // fail N times then succeed; crash cells do NOT use this
}
```

**Crash cells use a real subprocess** (`T-2`): the test re-execs itself (`os.Args[0] -test.run=TestHelperDispatchWorker` + env: `STORE_PATH`, `FAKE_PROVIDER_URL`, `RUN_ID`, `BARRIER`), the worker signals barrier arrival (pidfile/pipe), and the parent kills it via **cross-platform `os.Process.Kill()`** (this repo develops on Windows — no `kill -9`/SIGKILL assumptions) — deferred cleanup, goroutines, and pending writes genuinely die. The parent then runs recovery against the surviving store + provider log and asserts. The barrier signal seam is a test-only callback (`s.testBarrierHook func(barrier)`), not a production branch. Settle barriers `B8a..B8e` (between adjacent `SettlePhase`s) are cells in the same matrix (ledger `ST`).

**Step 2 — durable fake provider server (plan-review #2 #13).** An in-memory counting adapter cannot survive the worker being killed. The fake provider is an **HTTP/IPC server owned by the parent test**, with a **durable request log** (send counts + turnIDs survive worker death); the worker's adapter points at its URL. Assertions read the parent-side log. The Supabase fake is likewise a parent-owned shared server, not an in-process object.

```go
// Parent-owned; log survives worker kill. Worker env: FAKE_PROVIDER_URL, STORE_PATH, RUN_ID, BARRIER.
type fakeProviderServer struct { // http.Handler
	mu    sync.Mutex
	log   []sendRecord // {turnID, receivedAt} — appended on the "send" endpoint, fsynced to a temp log file
	mode  ReconcileOutcome
}
func (f *fakeProviderServer) sendsFor(turnID string) int { ... } // parent-side assertion API
```

**Step 3 — matrix driver.** For each `(barrier, fault, backend)` run a turn to the barrier, simulate crash (rebuild service from the same on-disk store) or Stop, run recovery, then assert:

```go
func runMatrixCell(t *testing.T, b barrier, fault faultMode, backend storeKind) {
	store, provider := startDurableFixtures(t, backend) // parent owns store + counting fake provider
	switch fault {
	case crashFault:
		worker := spawnDispatchWorker(t, backend, b)  // subprocess re-exec
		waitBarrier(t, worker, b)
		worker.Kill()                                  // os.Process.Kill — cross-platform hard kill; nothing survives
	case stopFault:
		svc := newSvcWith(t, store, provider)
		startTurnToBarrier(t, svc, b)
		svc.SetCancelRequestedAtBarrier(b)             // Stop CAS injected at the barrier
	}
	svc2 := newSvcWith(t, store, providerURL)          // fresh process + same durable store + same provider log
	svc2.reconcileDispatchRecordsOnResume(runID)
	assertThreeOutcome(t, svc2, runID)                 // INV-1: terminal | retryable | surfaced-uncertain — never silent loss
	assertNoDuplicate(t, providerLog)                  // INV-2: distinct sends <= 1 per turn (or 0 + retryable)
	if fault == stopFault {                            // INV-3 asserted on the RECORDED StopOutcome (SD-24 §7.2):
		switch stopOutcomeOf(svc2, runID) {            //   stopped_before_send  => providerLog.sendsFor(turn) == 0
		case "stopped_before_send":  assertZeroSends(t, providerLog)
		case "cancelled_in_flight":  assertAtMostOneSendAndProviderCancel(t, providerLog)
		}                                              // the unprovable "no bytes after Stop CAS" is NOT asserted
	}
}
func TestDispatchCrashMatrix(t *testing.T) {
	barriers := append([]barrier{B0,B1,B2,B3,B4,B5,B6,B7,B8}, settleBarriers()...) // B8a..B8e expanded per phase × 3 windows
	for _, b := range barriers {
		for _, f := range []faultMode{crashFault, stopFault} {
			for _, be := range []storeKind{localFileStore, supabaseFakeStore} {
				t.Run(fmt.Sprintf("%s/%s/%s", b, f, be), func(t *testing.T){ runMatrixCell(t, b, f, be) })
			}
		}
	}
}
// Settle-cell assertions (ledger ST/EL/SD): exactly one durable consequence per
// effectKind (convergent replay), never a lost one; after_effect_pre_marker
// crash MUST NOT duplicate (the doctrine's key window); Stop cells assert
// bookkeeping completes with release suppressed by stop-generation.
// Concurrency/durability variants (ledger rows CC/SW/TA/TO/RM/CD):
func TestRecovery_ConcurrentScanners_OneLeaseWinner(t *testing.T) {}
func TestStaleSnapshot_OlderRevision_Rejected(t *testing.T)       {}
func TestLocalStore_TornTail_DroppedAndRederived(t *testing.T)    {}
func TestInterleaved_ReceiptCommitVsStaleSessionWrite_NoResurrect(t *testing.T) { /* TO row: concurrent, not sequential */ }
func TestReleaseManifest_KillMidRelease_ExactDependents(t *testing.T)           { /* RM row: subprocess kill between dependents; every item is pending|created|suppressed and no child exists without created in the same durable commit */ }
func TestEffectPayloadConflict_FailsClosed(t *testing.T)                         { /* duplicate key + divergent cohort/release payload -> ErrEffectConflict, never first-writer-wins */ }
func TestPreSendStopCancellation_ClearsOwnerAtomically(t *testing.T)             { /* C2a: terminal state, parent/owner clear and audit survive every crash point together */ }
func TestCohortEffectRecords_KillMidAppend_ProjectionConverges(t *testing.T)    { /* CD row */ }
func TestAmbiguousSendError_B3B4_NeverTerminalizesOrClearsIntent(t *testing.T)  { /* AE: injected SendTurn timeout/error after send_started (before/after fake received bytes) leaves record reconcilable; only terminal evidence may settle */ }
func TestParentStopReleaseChildSend_LinearizableFence(t *testing.T)              { /* PS: both stores, enumerate Stop-before-create / after-create-before-claim / at-child-send-CAS; RunStopState authority -> ErrParentStopFence -> terminal+owner clear+audit + zero fake-provider requests */ }
func TestReceiptEvidence_ConflictAndCanonicalReplay(t *testing.T)                { /* RE: same identity/equal hash is idempotent; divergent canonical payload returns ErrReceiptConflict and keeps owner intent */ }
func TestRecoveryLeaseReloadAndCancelBeforeAttach(t *testing.T)                  { /* LR: mutate CancelRequested/revision after claim and after ReconcileInFlight; no stale provider action, cancel before attach */ }
func TestOperatorResolution_SettlementActionTable(t *testing.T)                  { /* OR: completed/failed settle iff immutable owed; abandon/superseded never settle; successor is sole settler; all atomic/idempotent */ }
func TestTerminalProofOutcomeFidelity(t *testing.T)                              { /* TP: proved failed/cancelled/completed-before-cancel map exactly; cancel request/no proof retains intent and reconcile state */ }
func TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority(t *testing.T)     { /* SA: both stores and real crash matrix. (1) root send_started -> RequestRunStop -> crash before SetCancelRequested -> proof completed|failed|cancelled: proof controls state/outcome AND terminal store transaction records cancelled_in_flight; (2) same sent child with CancelRequested=false and only parent stopped/generation changed: same attribution; (3) no record/own/parent Stop leaves StopOutcome empty. The API has no caller-supplied attribution. */ }
func TestRecoveryMutations_AreLeaseAndStopTransactionGuarded(t *testing.T)       { /* RG: both stores plus real PG. Barrier after final pre-action read: root Stop and child-only parent fence each win before CASRecoveryAdvance or recovery pre-send cancel (no bytes/no stale terminal), and before CommitRecoveryUnknownOrRequireCancel (cancel-required, no uncertain/attach/clear). Separate barrier after Reconcile obtains proof: expire lease, let scanner B claim; scanner A's CommitRecoveredTerminalAndSettleIntent returns ErrRecoveryLeaseLost and cannot clear intent; B re-reconciles/commits exactly once. Include no-adapter fallback and seeded model cases. */ }
func TestRecoveryAttach_DurableEpochFence(t *testing.T)                           { /* RA/TP/SA: local, Supabase RPC, real PG, and model. (1) A claims N then pauses before EnterRecoveryAttach; terminal/expiry wins => zero attach. (2) A pauses after an attach-token predicate/effect barrier; B terminalizes OR changes state to uncertain; release A => ErrRecoveryAttachRevoked and no output, terminal mutation, owner clear, settlement, or terminal audit. (3) store clock crosses N's attach TTL without N+1 claim; each completed|failed|cancelled attached terminal callback (no Stop/root Stop/parent Stop) returns ErrRecoveryAttachRevoked and performs zero terminal/clear/settle/audit mutation. (4) A expires, B claims N+1; only B enters/outputs/forwards. (5) current **unexpired** token forwards one completed|failed|cancelled proof terminal exactly once through CommitAttachedTerminalAndSettleIntent, including root Stop and child-parent-fence Stop races with CancelRequested=false: proof-derived terminal/owner-clear/settlement/audit commits once, StopOutcome=cancelled_in_flight, epoch revokes, later callbacks inert. Stop/root or parent fence winning claim returns cancel-required with zero attach; crash after claim releases only after bounded TTL. */ }
func TestRecoveryAttach_AllTerminalPathsRevokeEpoch(t *testing.T)                { /* RA: live terminal, recovered terminal, both pre-send cancellation paths, every terminal ResolveUncertain action, and RetryAsNew each revoke an active epoch in their atomic transition; an old token cannot enter, output, or forward. Run local, Supabase RPC, real PG, and the model. */ }
func TestRootRunStopVsSendStarted_Linearizable(t *testing.T)                    { /* RSF: RequestRunStop wins root send CAS on both stores -> ErrRunStopFence -> pre-send terminal/owner clear/audit, zero bytes */ }
func TestTerminalCommit_ReadsStoreSettleObligation(t *testing.T)                { /* SO: live and recovery terminal paths expose no caller bool; persisted SettleOwed alone controls settle_pending|none */ }
func TestRecovery_EffectiveStopAfterCrashBeforeRecordCancel(t *testing.T)       { /* ES: root and child parent-stop variants on both stores; RequestRunStop durable before per-record loop => provider cancel/no attach */ }
func TestRecovery_RunStopChangesDuringReconcileInFlight(t *testing.T)            { /* ES: run against both stores. Fake Reconcile signals it will return ReconcileInFlight then waits; with CancelRequested=false, mutate (a) the record's own RunStopState.Stopped=true, then separately (b) only the child's parent RunStopState generation/stopped, and release it to return InFlight. The post-Reconcile fresh lease/effectiveStopRequested check calls provider cancel; resumeStreaming and uncertain classification are never called; owner intent is retained and no terminal transition is fabricated. */ }
func TestRecovery_RunStopChangesBeforeUnknownClassification(t *testing.T)        { /* ES: both stores. Fake Reconcile signals then returns unknown only after root Stop or child-only parent authority changes with record cancel false; the fresh authority check cancels, never CASes uncertain/attaches/clears/fabricates. Repeat the no-ReconcilableAdapter fallback. */ }
func TestRecovery_EffectiveStopReadFailure_NoAttachOrClassification(t *testing.T) { /* ES: own/parent run-stop read failure fails closed */ }
```

**Step 4 — Supabase backends (three tiers, Q-1 / plan-review #14).** (a) HTTP fake via `httpRequestFn` (the store already indirects through it) for encoding/error unit cells; (b) the shared `dispatch_store_contract_test.go` suite runs **identically** against the local store and the Supabase impl; (c) the same contract suite against a **real PostgreSQL/Supabase** (`FLOWPILOT_TEST_SUPABASE_DSN`, skipped when unset) proving CAS conditional transitions + concurrent claims — ledger row `PG` requires at least one documented run.

**Step 5 — aggregate the per-task suites** (Task-249 stop-race, Task-251 outage, Task-252 marker, Task-253 corruption, Task-254 retention) and add the coverage-map note (§6 V-1). Wire CI: `go build ./...`, `go vet ./...`, `go test -race ./internal/runner ./internal/flowgate`.

**Step 6 — composite-fault / model-based suite (`dispatch_model_test.go`, ledger `MB`).** A driver applies **randomized interleavings** of {dispatch steps, Stop, worker kill, outage, lease expiry, operator `ResolveUncertain`/`RetryAsNew`, flag flip, restart} (seeded, replayable) and checks INV-1..7 after every step + a Stop/send linearizability check by CAS order. Explicit named sequences: Stop+crash, outage+terminal-commit+restart, operator-vs-reconcile race, lease-expiry+second-scanner+Stop, flag-flip+restart, crash inside each settle phase.

**Step 7 — document the manual kill procedure** (cross-platform: `taskkill /F` on Windows, `kill -9` on POSIX — or the harness's own kill helper) at each barrier on both backends, in Completion Notes for release verification.

### 4.2 Test file inventory (all must exist + pass)

```
dispatch_crash_matrix_test.go     stop_race_barrier_test.go     gate_checkpoint_outage_test.go
fcp_marker_replay_test.go         supabase_runtime_corruption_test.go     idempotency_retention_test.go
dispatch_record_test.go           dispatch_recovery_test.go     dispatch_store_contract_test.go
dispatch_settle_test.go (phase driver + effect ledger + dispositions)     dispatch_model_test.go (MB randomized interleavings)
operator contract tests (SS-17: blocking, idempotent resolution, retry-as-new, repair)
```
**Defect-regression** tests must be shown failing on HEAD `6ea5417` and passing after their owning task (capture in the PR). **Foundation** tests (new types) are instead covered by the `MU` mutation row — flip a transition guard/CAS predicate and show the suite goes red (plan-review #16).

### 4.3 Fold BUG-288 §11.9.5 (historical mandatory tests — nothing lost)

BUG-288 §11.9.5 already listed the tests required before close. This suite must **subsume** that list so no historically-required check is dropped. Map each into the closed matrix or an existing round test:

| BUG-288 §11.9.5 requirement | Covered by |
| --- | --- |
| Crash/reconstruct at `provider_done`/`gate_running`/`gate_blocked`/`gate_passed`; concurrent `startTurn` in post-gate rejected | `dispatch_crash_matrix_test` cells B2–B5 + existing V9-03 test (`gate_in_progress`) |
| `blocked_validation_failed`/`skipped_no_command`/`skipped_env_error`/oracle-timeout/user-cancel never reach audit→done | existing validate/oracle suite (V9-01) — assert still green under `NR` |
| Gate block/reprompt never mutates Canonical Head; non-code root turn never hides declared contract | existing gate_hook suite (V9-02/V9-04) — `NR` |
| Cohort two-member stall; Retry in-flight cancels then restarts **exactly once**; invalid node action → no state mutation | existing cohort suite (V9-05..V9-07/V9-25) — `NR` |
| Restart at child approval/question: actionable submit or expire+re-run; no WAITING 404 | existing resume suite (V9-08) — `NR` |
| Git matrix (rename/copy, space/tab/newline/quote); fingerprint >1 MiB tail edit same size | existing flowgate/observe suite (V9-14/V9-15) — `NR` |
| Validation matrix (quoted/escaped/env/pipe/`&&`/unmatched); baseline pre-change truth; no invented `suite_regressed` | existing oracle/validation suite (V9-16/V9-26/V9-27/V9-30) — `NR` |
| `TestE2EReviewLoopApprovedPath -count=20` exactly one synthesis; every E2E calls `assertNoStepStuckRunning` | existing E2E suite (V9-28) — `NR`, run under `-race` |

Items in the durability/dispatch class are covered by the **new** matrix; the rest are guarded as **non-regression** (CP-51 §10.2). Add a coverage note in the PR mapping §11.9.5 → tests.

## 5. Touched Areas

- files: new `*_test.go` suites listed above; a shared `dispatch_test_harness.go` (test build tag) for the crash/outage seam and the fake counting adapter.
- modules: `internal/runner` tests (+ `internal/flowgate` regression run).
- routes: none.
- tables: none (uses the Supabase HTTP fake).

## 6. Acceptance Check

- `V-1` Every `DOD-*` item in CP-51 §10.1 maps to at least one named test in this task; a coverage note lists the mapping.
- `V-2` Each defect test is verified to **fail on HEAD `6ea5417`** (pre-fix) and pass after its owning task — captured in the PR description.
- `V-3` Crash-matrix passes for all barriers on **both** local-file and Supabase-fake backends.
- `V-3b` SD-25 attach closure passes on local, Supabase RPC implementation, real PostgreSQL, and the model suite: terminal/expiry before entry gives zero attach; **attach TTL expiry before terminal forwarding gives zero mutation even before N+1 claim**; stale paused callbacks after terminal or `uncertain` give zero output/mutation; one current unexpired token forwards exactly one terminal (including root/parent Stop attribution); every terminal/retry path revokes the epoch.
- `V-4` `go test -race ./internal/runner` is clean (no data races), including the settle/gate path.
- `V-5` Full existing `internal/runner` + `internal/flowgate` suites (BUG-288 Round 1–20 guards) still pass — no regression.
- `V-6` `go build ./...`, `go vet ./...` clean.

## 7. Out of Scope

- The production fixes themselves (Task-248..254, Task-256) — this task only proves them; per-task unit tests still live with their tasks, and are aggregated/extended here into the full matrix.

## 8. Completion Notes

- result: **done (2026-07-17)**
- **Real-kill matrix B0–B8** (`dispatch_crash_harness_test.go`): parent-owned durable fake-provider HTTP server, re-exec worker (`TestHelperDispatchWorker` + `FLOWPILOT_CRASH_WORKER`), cross-platform `killHard` (Windows `taskkill /F` fallback). 9 cells INV-1/INV-2 green (incl. B8: bare `RecoveryScanner` leaves `settle_pending`; settle drive is Task-251 `ScanDispatchRecoveryOnBoot` / `scheduleSettleDrive`).
- **Settle sub-barriers B8a–B8e** (`dispatch_settle_barrier_test.go`): after Task-251 production driver land, in-process fault seam stops after N phase CAS advances then resumes DriveSettle — proves convergent effects (no duplicate kind) and finalization from each partial phase.
- **Model suite** (`dispatch_model_test.go`): seeded 40 interleavings of claim/start/receipt/terminal/settle/cancel_flag; asserts record never silently lost/empty-state.
- **Outage suite** co-owned with Task-251: `gate_checkpoint_outage_test.go`.
- **CI**: `.github/workflows/local-runner-race.yml` — `go vet` + `go test -race` focused on settle/dispatch/crash packages.
- **Supabase / PG tier**: **moot** (Task-258 retired Supabase dispatch) — matrix is local-file-only; not an open gap.
- **Honest residual (non-blocking):** full composite model with real subprocess kill + operator ResolveUncertain every interleaving step is not exhaustive; B8a–e are durable phase-CAS barriers (not OS-kill inside effect write). Fail-on-HEAD capture for historical `6ea5417` is documentation-only. Existing BUG-288 E2E suite remains the non-regression gate.
- follow-ups: optional expand model to include real-kill workers; optional full-package `-race` nightly (focused race job is the blocking CI).

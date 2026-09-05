# CA-742 — gate oracle cannot hang; bounded gate-busy lets hub_stalled fire (run-540927)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-354
change_type: bugfix
summary: executeSuite hard-bounds its wait (process-group kill + 2s grace, EnvError on abandon) so a pipe-holding grandchild can no longer pin the post-turn gate forever, and the hub watchdog's gate-busy signal is bounded (postTurnGateStartedAt + 6m bound + child-ghost rule) so a dead gate finally surfaces hub_stalled instead of pinning implement RUNNING
# --->8---

## Problem

Live run-540927 (CP-58 Phiên 1, gate-sandbox, task-harness): the coder child's retry turn ended 22:58:45 UTC and its post-turn gate ran `go test -v ./calc/...` — the oracle never returned. No log, no result, no watchdog escape. TUI pinned `implement RUNNING` for 10+ minutes; operator `/stop` was the only way out. Forensics: `[gate] scoped oracle run` logged 22:58:45, then silence; no `go test` process; 5-minute `context.WithTimeout` in `executeSuite` never yielded because `cmd.Wait()` blocks until inherited output pipes close (grandchild holding them).

Second layer: even with the oracle dead, `checkAndBlockStalledHub` could not fire — `rs.postTurnGateCancel != nil` is unbounded busy, and `hasActiveFlowChild` counts the child's lingering `RunStatusRunning`/`pendingFlowGateSettle` as active. The two defense layers each deferred to the other.

## Root cause

`exec.CommandContext` kills the direct child but `cmd.Wait()` returns only after pipe-holding descendants close the pipes; and both hub watchdog busy signals (parent gate-cancel, child Running/settle) were unbounded in time.

## Changes

- `apps/local-runner/internal/flowgate/oracle.go` — `executeSuite`: suite runs in its own process group; `cmd.Run()` moved to a goroutine; on ctx cancel/timeout the group is SIGKILLed and the executor waits ≤ `suiteGraceAfterCancel` (2s) before returning `EnvError` regardless. Added `[gate] suite start/end` logs (duration, aborted, outputBytes) so future hangs are visible.
- `apps/local-runner/internal/flowgate/suite_proc_unix.go` (new) — Setpgid + group SIGKILL (`//go:build !windows`).
- `apps/local-runner/internal/flowgate/suite_proc_windows.go` (new) — `taskkill /T /F /PID` equivalent (`//go:build windows`), fire-and-forget (review F4: a hung taskkill must not block executeSuite past the grace).
- `apps/local-runner/internal/runner/interactive_service.go` — `interactiveRun.postTurnGateStartedAt`; stamped at both production arm sites (resumePendingFlowGate + post-turn gate in runTurn) alongside `postTurnGateCancel`.
- `apps/local-runner/internal/runner/hub_stall.go` — `postTurnGateBusyBound = 6m` (oracle 5m deadline + slack) + `gateCancelLive(startedAt, cancel)` (zero stamp = legacy unbounded busy); consumed by the parent busy check AND `hasActiveFlowChild`; child-ghost rule: `status==Running` no longer counts active when the turn is over, the gate cancel aged out, and nothing is pending; stale `pendingFlowGateSettle` without a live gate mirrors the hub-level H-C contract (run-1618).
- **F1 (sub-agent review round):** the first cut left `turnInFlight` unbounded busy — but V9-03 (`interactive_service.go` finishTurn) holds `turnInFlight=true` for the ENTIRE post-turn gate window, so the live run-540927 shape (turn held by a never-returning gate) would have kept both parent `busy` and `hasActiveFlowChild` true forever regardless of the cancel bound. Fix: while `postTurnGateCancel != nil`, `gateCancelLive` owns the busy decision (`turnBusy = turnInFlight && postTurnGateCancel == nil` on both parent and child paths); `turnInFlight` without an armed gate stays busy unconditionally (CA-361 intact). Safety: `startTurn` already rejects overlapping turns while the gate is settling (`gate_in_progress`), so a stale gate cannot race a new turn.

## Cross-provider parity (R2)

Provider-agnostic (Case 1): `executeSuite`, `RunOracleContext`, `checkAndBlockStalledHub`, `hasActiveFlowChild`, `gateCancelLive` take no `providerKey` and never branch on one — grep across the changed functions and their callers shows zero `ProviderKey`/per-provider constants. Watchdog and oracle behavior is therefore identical for Claude/Codex/Grok; one representative test set covers all three by construction (no provider parameterization needed, stated per cross-provider-parity Case 1).

## Prior CA claims — intact

- CA-361 (hub stall must not cancel a running child): `turnInFlight` remains unconditionally active; `TestRun333HubStallDoesNotCancelRunningChild` green.
- CA-355 / run-1618 H-C (stale settle ≠ busy; live gate = busy): mirrored, not weakened — `TestHubStallFiresDespiteStalePendingGateSettle` and `TestHubStallStillBusyDuringLivePostTurnGate` (zero stamp → legacy busy) green.
- CA-616 (terminal child ≠ active): terminal skip untouched.
- CA-741 (park-cancel non-terminal): `TestRun203966*` all green.
- CA-642 (child ask_user → root card): `TestChildAskQuestionForwardedToFlowRootStream` green.
- Oracle regression semantics: cancelled suite still `EnvError`, never a fake regression — `TestRunOracleContextCanceledIsNotRegression` green; old oracle suite (`go test ./internal/flowgate/ -count=1`) fully green.

## Tests added (new files only)

- `internal/flowgate/run540927_oracle_timeout_test.go`:
  - `Test540927OracleGroupKillReturnsDespitePipeHoldingChild` — repro shape: grandchild holds the pipes; deadline → returns <10s, `EnvError`, no regression.
  - `Test540927OracleSigtermIgnoringSuiteBounded` — TERM-trapping suite cannot pin the executor past deadline + grace.
  - `Test540927OracleGraceAbandonWhenKillMissesPipeHolder` — setsid-escaped pipe-holder cannot pin the executor past the 2s grace (P3, documented in CA-743).
  - `Test540927OracleCleanSuiteStillPasses` — group-kill plumbing leaves the happy path intact.
- `internal/runner/run540927_gate_hang_watchdog_test.go`:
  - `Test540927HubStallFiresWhenGateCancelStale` — stale gate (10m) → `hub_stalled` (repro).
  - `Test540927HubStallBusyWhileGateCancelFresh` — fresh gate (30s) → still busy (near-miss).
  - `Test540927ChildGateStaleDoesNotKeepHubBusy` — stale child gate + ghost child → hub fires.
  - `Test540927ChildTurnInFlightStillBusy` — CA-361 guard: live child turn (no gate cancel) stays active, never cancelled.
  - F1 review probes (V9-03 shape — turnInFlight held by the gate window):
    - `Test540927ChildTurnInFlightStaleGateStillFires` — child `turnInFlight=true` + stale gate (10m) → `hub_stalled` (the live run-540927 shape).
    - `Test540927ChildTurnInFlightFreshGateStillBusy` — same shape, fresh gate (30s) → still busy.
    - `Test540927HubTurnStaleGateStillFires` — the hub's own `turnInFlight` held by a stale gate → `hub_stalled`.

## Verification

- `go test ./internal/flowgate/ -count=1` → ok.
- `go test ./internal/runner/ -count=1 -run 'Test540927|TestHubStall|TestRun333|TestRun1618|TestRun43831|TestBug289|TestRun2047|TestRun203966|TestChildAskQuestion'` → all pass.
- `go vet ./internal/flowgate/ ./internal/runner/` clean.
- Full runner package stash-diff (with `-u` so untracked new files stash too): baseline flaky set (18 failures incl. TestRun144900/147126 family) identical with and without this change; both candidate regressions (`TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`, `TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt`) reproduce at baseline under `-count=3` → pre-existing flaky, not this change.

## Residual risks / out of scope

- ~~`interactive_resume.go` + `cohort_stall.go` unbounded gate-busy~~ — SUPERSEDED by CA-743 P2/P2-R2: those three consumers now use the same `gateCancelLive` bound (+ V9-03 ownership rule). Remaining unbounded on purpose: `startTurn` `gate_in_progress` reject (needs `gateEpoch` bump at startTurn — own review round; escape = `hub_stalled` card → Stop) and `hubShouldSkipProseEscalate` BUG-226 prose-escalate skip (pre-existing, no hang — hub_stalled still fires; own probe harness needed, deferred per round-4 review).
- ~~BUG-354 C2 ask_user ghost RUNNING~~ — SUPERSEDED by CA-743 P1: `AskQuestionCtx` + `dispatchCtx(r.Context())`, late answer → 409 `question_expired`.
- Invariant to keep: oracle deadline (5m) < `postTurnGateBusyBound` (6m); both constants must move together.

# BUG-354 — Post-turn gate oracle hangs forever: implement stays RUNNING with no card, watchdog cannot fire (run-540927)

## Metadata

- Document ID: `BUG-354`
- Title: `Post-turn gate oracle hangs forever — implement stays RUNNING with no actionable card, hub watchdog cannot fire (run-540927)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Parent Documents: [CP-58: Task/BUG/CP Harness Dual Review Loops](../../07-Coding-Plan/inprogress/CP-58-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [CA-742](../../../change-audit/CA-742-gate-oracle-cannot-hang.md), [CA-741](../../../change-audit/CA-741-audit-park-cancel-nonterminal.md) (same run family, CP-58 S2), [CA-361](../../../change-audit/CA-361-run333-hub-stall-respects-active-child.md), [CA-355](../../../change-audit/CA-355-run1618-entry-fail-hub-hang.md) (H-C stale-settle contract), run-540927 (`.flowpilot/cli-runner.log`, 2026-09-04 22:55–23:10 UTC), `flowgate/oracle.go`, `runner/hub_stall.go`
- Replaces: `none`
- Tags: `agent-flow-engine, flow-gate, oracle, hub-stall, watchdog, hang, running-forever, severity-high`

## AI Quick View

### Summary

- Live `run-540927` (gate-sandbox, opencode `omen-alpha`, task-harness, CP-58 Phiên 1 GCD prompt): the implement (coder) child finished its retry turn at 22:58:45 UTC; the post-turn child gate started `go test -v ./calc/...` and **never returned** — no result, no log line, no watchdog escape. The TUI pinned `implement RUNNING` with no card for 10+ minutes; the operator had to `/stop` manually.
- Forensics: `[gate] scoped oracle run` logged at 22:58:45, then silence. The oracle's own 5-minute deadline (`context.WithTimeout` in `executeSuite`) never yielded a result: `cmd.Run()` blocks inside `cmd.Wait()` until the suite's inherited stdout/stderr pipes close — a grandchild holding the pipes pins the wait past the deadline, so `CommandContext`'s kill never converts into a return.
- Watchdog blind spot (second layer): the hub stall watchdog (`checkAndBlockStalledHub`) treats a live `postTurnGateCancel` as busy, and `hasActiveFlowChild` treats the child's lingering `RunStatusRunning` + `pendingFlowGateSettle` as active — a **dead gate shields the flow from hub_stalled forever** (CA-361/CA-616 conservative fields), exactly the hang shape.
- Near-miss (same run): the model's `flowpilot_ask_user` question timed out on the MCP client side (~72s) while the runner-side question TTL is 10 minutes — the card stayed answerable after the model already escalated and ended its turn; answering late stamped the run `RUNNING` again (ghost). **Fixed by CA-743 (P1)**: the shared MCP dispatch now carries `r.Context()`; client disconnect expires the question (late answer → 409, no ghost). The dead-gate busy shield on the cohort/resume consumers is also bounded (CA-743 P2), and the 2s grace abandon has its own probe (P3).

### Current Ask

- The post-turn gate oracle must **always return** within its deadline + a small kill grace (pass / fail / `EnvError`) — no shape may pin `cmd.Wait` forever.
- A gate that never returns must stop counting as hub/child busy after a bounded window so the watchdog can surface an actionable `hub_stalled` card instead of a silent `RUNNING` pin.

### Key Decisions

- `D-1` Process-group kill: the suite command runs in its own process group (`Setpgid`); on cancel/timeout the **whole group** is SIGKILLed so pipe-holding grandchildren die with it. Windows uses `taskkill /T /F`.
- `D-2` Hard wait bound: `cmd.Run()` moves to a goroutine; after ctx cancel the executor waits at most `suiteGraceAfterCancel` (2s) for `Wait` to drain, then returns an `EnvError` regardless. `executeSuite` cannot block indefinitely by construction.
- `D-3` Bounded gate-busy: new `postTurnGateStartedAt` stamps when `postTurnGateCancel` is armed (both production arm sites); `gateCancelLive(startedAt, cancel)` counts the gate busy only while `age < postTurnGateBusyBound` (6m = oracle 5m deadline + 1m slack). A zero stamp keeps legacy unbounded-busy behavior so pre-stamp/reconstructed state never regresses.
- `D-4` Child ghost detection: `hasActiveFlowChild` no longer lets a child whose turn is over, whose gate cancel aged out, and which holds **no** pending work (prompt/approval/question/live gate) shield the hub. Stale `pendingFlowGateSettle` without a live gate mirrors the hub-level H-C contract (run-1618): settle counts busy only when the gate cancel is live. `turnInFlight` stays active unconditionally (CA-361 intact).
- `D-5` (sub-agent review F1) While the post-turn gate is armed, `gateCancelLive` owns the busy decision: V9-03 holds `turnInFlight=true` for the entire gate window, so `turnInFlight` with a stale gate cancel must not stay busy past the bound — that was the live run-540927 shape the first cut missed. `turnInFlight` **without** an armed gate stays busy (CA-361); `startTurn`'s `gate_in_progress` reject prevents a new turn from racing a stale gate.

### Constraints

- additive-tests-only: new tests only (`run540927_*_test.go`, `suite_proc_*.go`); zero pre-existing test edits (verified by stash-diff baseline comparison).
- oracle-rule: baseline-failing/flaky old tests were proven pre-existing (stash run) — not "fixed" by editing.
- The oracle's 5-minute deadline and regression semantics are untouched: a cancelled suite still returns `EnvError`, never a fake regression (old `TestRunOracleContextCanceledIsNotRegression` stays green).
- `interactive_resume.go` / `cohort_stall.go` consume `postTurnGateCancel` for their own busy checks — out of scope here (residual risk below); hub watchdog + active-child are the claim surface.
- Cross-provider: watchdog + oracle take no `providerKey` (grep evidence in CA-742) — provider-agnostic (R2 Case 1).

### Open Questions

- `Q-1` What exactly held the pipes in run-540927 (the suite failed in ~1s on the first gate)? Forensics could not capture a goroutine dump before the operator stopped the run; the fix bounds every shape, so the specific culprit is no longer load-bearing. A repro with `SIGQUIT` dump would answer it.
- `Q-2` Cohort member watchdog same gate-age bound — DONE in P2 (same session): `cohort_stall.go` gate shield + restart park bounded via `gateCancelLive`; `interactive_resume.go` reinvoke drain bounded.
- `Q-3` P1/C2 ask_user card lifecycle — DONE in P1 (same session): `AskQuestionCtx` + `dispatchCtx(r.Context())`; MCP client disconnect expires the question (late answer → 409 `question_expired`, no ghost RUNNING). Optional-interface design, zero old-test compile break.

### Source Refs

- `run-540927` forensic (`.flowpilot/cli-runner.log`):
  - 22:57:31 first gate `go test -v ./calc/...` → result → retry 1/3 prompt 22:57:33.
  - 22:57:33 retry turn + `flowpilot_ask_user` (3 options) — model reports `ask_user timed out` ~72s later, escalates, keeps change as-is.
  - 22:58:45 coder `end_turn` (`stopReason end_turn`) → second `[gate] scoped oracle run: go test -v ./calc/...` — **last gate log; no suite end, no watchdog, no hub_stalled afterward**.
  - 23:03:20 only `chat-history-open` (operator opened the child transcript); runner HTTP alive, gate goroutine gone silent.
  - No `go test` process alive at 23:06–23:12; `opencode acp` child process alive but idle.
- `flowgate/oracle.go` `executeSuite` — `context.WithTimeout(ctx, 5*time.Minute)` + bare `cmd.Run()` (pre-fix).
- `runner/hub_stall.go` — busy check line ~327 and `hasActiveFlowChild` line ~75 (pre-fix): unbounded `postTurnGateCancel != nil` / `status == RunStatusRunning` / `pendingFlowGateSettle`.

## Root Cause

1. **Oracle layer**: `exec.CommandContext` kills only the direct child on deadline, but `cmd.Wait()` returns only after the pipes inherited by **grandchildren** close. A pipe-holding descendant therefore pins `executeSuite` past its own 5-minute deadline with no log and no return.
2. **Watchdog layer**: the hub stall watchdog's busy signal (`postTurnGateCancel != nil`) and the active-child signal (`status Running`, `pendingFlowGateSettle`) are unbounded — a gate that can never return also never stops being "busy", so BUG-289 F-0 (`hub_stalled`) cannot fire. Two defensive layers each deferred to the other; neither bounded.

## Fix

- `flowgate/oracle.go`: `executeSuite` runs the suite in its own process group, streams output as before, and bounds its wait: on ctx cancel/timeout it kills the group, waits ≤2s for `Wait`, then returns `EnvError`. Start/end logs (`[gate] suite start/end` with duration) make future hangs visible in the runner log.
- `flowgate/suite_proc_unix.go` / `suite_proc_windows.go`: process-group helpers (Setpgid + group SIGKILL / `taskkill /T /F`).
- `runner/interactive_service.go`: `interactiveRun.postTurnGateStartedAt`; both production arm sites (resume gate + post-turn gate) stamp it.
- `runner/hub_stall.go`: `postTurnGateBusyBound` (6m) + `gateCancelLive`; parent busy and `hasActiveFlowChild` both consume the bounded predicate, plus the child-ghost rule above. Sub-agent review F1: while the gate is armed, `gateCancelLive` owns busy — `turnInFlight` (held true by V9-03 for the whole gate window) with a stale gate cancel ages out; `turnInFlight` without an armed gate stays busy (CA-361).

## Validation

- New tests (all green):
  - `flowgate/run540927_oracle_timeout_test.go`: pipe-holding grandchild + deadline → returns <10s with `EnvError`, no regression; TERM-trapping stubborn suite bounded; clean green suite unaffected (happy path).
  - `runner/run540927_gate_hang_watchdog_test.go`: stale gate cancel (10m) → `hub_stalled` fires (repro); fresh gate cancel (30s) → still busy (near-miss); stale **child** gate → hub fires despite ghost child; child `turnInFlight` → CA-361 intact, no cancel.
- Old suites: `go test ./internal/flowgate/ -count=1` ok; hub-stall family (`TestHubStall*`, `TestRun333*`, `TestRun1618*`, `TestBug289*`, `TestRun2047*`, `TestRun43831*`) all green; CA-741 + CA-642 tests green; `go vet` clean.
- Full-package stash-diff: the runner package's baseline flaky set (18 tests incl. `TestRun144900/147126` family, `TestDetectProviders*`, `TestTryAdvance*`, `TestFlowCodingPrompt*`) is identical with and without this change — zero new failures attributable to the fix (both candidate tests reproduce at baseline under `-count=3`).

## Residual Risks

- `startTurn` (~`interactive_service.go:8035`) still rejects new turns while `postTurnGateCancel != nil` unbounded — deliberately kept: `gateEpoch` is bumped only by Stop, so racing a leaked gate goroutine could apply stale side effects. Escape = watchdog `hub_stalled` card → Stop. Narrowed by P0 to the rare post-oracle wedged case.
- A suite legitimately running longer than 6 minutes (post-turn gate window) could trip the watchdog while the oracle is still inside its own 5m deadline — not reachable today (oracle deadline 5m < bound 6m) but the invariant is deadline < bound and must be kept if either constant changes.
- Review F2/F3/F7 (deadline-race EnvError classification, leaked `cmd.Wait` goroutine after grace abandon, `cmd.Process` nil race) — bounded, conservative, documented in CA-742; no code change.
- `ask_user` on the Claude stdio control_request path (`claude_adapter.handleInbound`) keeps its turn-ctx cancellation; the HTTP MCP path (all three providers' model ask_user) is the ctx-bound one fixed in P1.

# CA-743 — ask_user MCP client death expires the question; all gate-cancel busy consumers bounded (BUG-354 P1/P2/P3)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-354
change_type: bugfix
summary: the model ask_user MCP call no longer outlives its HTTP client (r.Context() plumbed into the shared MCP dispatch; client disconnect expires the pending question and returns an error tool-result so a late human answer gets 409 question_expired instead of stamping ghost RUNNING), the remaining unbounded postTurnGateCancel busy consumers (cohort member stall shield/restart park, notifyTurnIdle reinvoke drain) adopt the same 6m gateCancelLive bound, and a setsid-escaped pipe-holder probe locks the 2s grace abandon
# --->8---

## Problem (P1/C2 — the run-540927 near-miss)

Live run-540927: the coder's `flowpilot_ask_user` (3 options, contract conflict) was answered by the operator ~3 minutes after the model had already reported `ask_user timed out` (the MCP client's ~60s timeout) and ended its turn. The runner-side question TTL is 10 minutes and the pending question lived on `bridge.ctx` (turn ctx), which outlives the HTTP request — so the card stayed answerable, the late `AnswerQuestion` flipped the run back to `RUNNING`, and the already-dead turn showed as live. All three providers route model `ask_user` through this ONE shared MCP server (claude `--mcp-config` http; grok reuses `claudeMCPServer`, Task-209; opencode `session/load` mcpServers http — live log), so one fix covers Claude/Codex/Grok (R2 shared-server by construction).

## Changes

- `runner/interactive_service.go` — `turnBridge.AskQuestion` refactored into `askQuestion(extraCtx, …)`; new `AskQuestionCtx(ctx, …)` method (NOT a TurnBridge interface change — the TurnBridge fakes in pre-existing tests keep compiling, R1). Extra `ctx.Done()` (nil-safe via a blocking nil channel) expires the pending question and returns `ask_user aborted (client disconnected): …`.
- `runner/claude_permission_mcp.go` — optional `askUserCtxBridge` capability interface + `handleClaudeAskUserCtx`; bridges without the capability fall back to legacy `handleClaudeAskUser` (old `fakeClaudeBridge` callers untouched).
- `runner/claude_mcp_server.go` — new `dispatchCtx(ctx, …)`; `dispatch` kept as a `context.Background()` wrapper (grok_mcp_test calls `dispatch` directly 6× — signature preserved); `ServeHTTP` passes `r.Context()`; `tools/call ask_user` routes through `handleClaudeAskUserCtx`.
- `runner/cohort_stall.go` — member stall shield (`hasGate`) and member-restart park (`gateBusy`) bound the gate cancel with `gateCancelLive` (dead gate stops shielding a member from `member_stalled`; a live gate keeps shielding, T-11(a)/BUG-288 R13-06 intact).
- `runner/interactive_resume.go` — `notifyTurnIdle` reinvoke drain bounds the gate cancel (a dead gate no longer strands `pendingHubReinvoke` forever, run-1618 family).
- `internal/flowgate/run540927_oracle_timeout_test.go` — new grace-abandon probe (below).

## Tests added (new files only)

- `runner/run540927_ask_user_mcp_cancel_test.go`:
  - `Test540927AskUserMCPDisconnectExpiresQuestion` — client disconnect mid-question → dispatch returns "ask_user aborted…" promptly, record expired, `pendingQuestionID` cleared, late `AnswerQuestion` rejected (no ghost RUNNING).
  - `Test540927AskUserAnsweredBeforeDisconnectStillResolves` — happy path through the ctx-bound dispatch still returns the choice.
  - `Test540927AskUserLegacyBridgeFallbackStillWorks` — capability-less bridge (pre-existing fake shape) still works through `dispatchCtx`.
- `runner/run540927_cohort_resume_gate_bound_test.go`:
  - `Test540927CohortMemberStaleGateStillStalls` / `Test540927CohortMemberFreshGateStillShielded` — member_stalled vs T-11(a) shield on stale/fresh gate.
  - `Test540927NotifyTurnIdleDrainsReinvokePastStaleGate` — stale gate no longer strands the reinvoke; fresh gate still defers the drain.
- `flowgate/run540927_oracle_timeout_test.go` (P3, review F6): `Test540927OracleGraceAbandonWhenKillMissesPipeHolder` — a `setsid`-escaped child still holding the pipes cannot pin the executor; the 2s grace abandons it with `EnvError`, no regression.

## Prior CA claims — intact

- CA-642 (child ask_user mirrors to the root stream) — the mirror block is unchanged inside `askQuestion`; `TestChildAskQuestionForwardedToFlowRootStream` green.
- BUG-288 P1-07/P1-08/P2-02 (persist-before-wait, durable TTL, typed resolve) — same body, only an extra exit case; BUG-289 H4 expiry-flip untouched (`expireQuestion` reused).
- CA-742 P0 (bounded gate-busy, oracle hard-bound) — watchdog layer untouched; `postTurnGateBusyBound` reused, not redefined.
- T-11(a)/BUG-288 R13-06 (gate visible → wait) — a LIVE gate still shields cohort members; only the dead (>6m) gate ages out.

## Verification

- `go test ./internal/flowgate/ ./internal/runner/ -count=1 -run 'Test540927|TestHubStall|TestRun333|TestRun1618|TestRun43831|TestBug289|TestRun2047|TestRun203966|TestChildAskQuestion|AskUser|Question|WorkflowDriven'` → 71 pass / 0 fail.
- `go vet` clean on both packages; `go build` clean.
- Full runner package delta vs the CA-742 baseline flaky set: no new attributable failures (candidates `TestMultiWorkspaceRunsIndependent`, `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`, `TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt` reproduce at baseline under `-count=3`).

## Residual risks

- `startTurn`'s `gate_in_progress` reject (`pendingFlowGateSettle || postTurnGateCancel != nil`) is still unbounded — after the watchdog parks a dead-gate flow, a child Retry could be rejected until restart. Left out deliberately: letting a new turn start over a possibly-still-running gate goroutine requires a gateEpoch bump at startTurn; needs its own review round.
- The grace-abandon probe asserts bounded return, not that the setsid child deterministically held the pipes past the kill (OS-dependent timing); the TERM-trap + pipe-holder probes carry the deterministic coverage.
- `AskQuestionCtx` is optional-capability: a future bridge that implements `TurnBridge` but forgets `AskQuestionCtx` silently gets the legacy 10-minute card. Acceptable (default = pre-BUG-354 behavior); a compile-time enforcement would change the interface (R1 trade-off, documented).

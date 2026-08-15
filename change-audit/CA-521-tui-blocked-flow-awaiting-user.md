---
id: CA-521
feature_key: cli-tui
title: blocked (awaiting-user) flow reopen must not arm [stop]; add /continue /stop
date: 2026-08-15
status: COMPLETE
---

## Problem

run-189839: after reopening a flow whose orchestrator loop is `blocked`
(loopState), the TUI status line shows `[stop]` even though the flow is parked
awaiting a user Continue/Stop decision. The transcript says "Flow status: done."
(synthesis agent text) and all F2 steps are DONE, but the TUI keeps the flow
"live-running".

## Root cause (TUI-only, not a runner regression)

The runner reports a blocked flow's history status as `blocked` and the loop is
a deliberate non-terminal "awaiting user" pause (cap reached / escalate /
member_stalled) — Desktop BUG-231/run-63960. The TUI lacked that semantics:

- `turnIsActive()` (app.go) treated a `blocked` loop + orch SSE + stale
  `runHandle.Status=="running"` as live work → armed `[stop]` (BUG-231: the very
  user the flow is waiting on has no way to respond).
- `/open` hydrated agent **runs** (`cmdHydrateAgentRuns` → `ListAgentRuns`) but
  never fetched the full **agent graph** (`GET /agent-graph`), so loopState
  (`blocked`/`done`) was not seeded — the TUI could not know the loop was parked.
- A freeform turn against a blocked loop answers `409 flow_awaiting_user`
  (interactive_service.go startTurn gate). The TUI's `SendTurn` retry classified
  it as a transient 409 conflict (all 409s were retryable) and, when it
  surfaced, `turnStreamClosedMsg` dropped the run handle to a dead error state —
  the opposite of Desktop, which parks the flow and keeps Continue/Stop.
- No TUI affordance existed to unblock (Desktop `FlowAwaitingUserCard` with
  Continue/Stop + `continueFlow`/`pauseAgentLoop`).
- `AgentGraphMsg`/`agent_graph_updated` applied any graph unconditionally, so a
  late blocked refresh could overwrite a newer Continue/Stop result.

## Fix (TUI-only)

**A. Client (`tui/client/client.go`)**
- `AgentLoopState` gains `Cap`, `GateReason`, `OpenIssues`, `BlockReason`,
  `ActiveNode` (Desktop `AgentLoopState` parity).
- `GetAgentGraph(ctx, runID)` → `GET .../agent-graph`.
- `ContinueFlow(ctx, runID)` → `POST .../agent-loop/continue`.
- `IsFlowAwaitingUserError(err)` classifies 409 `flow_awaiting_user` (or 409
  whose message carries that meaning). `IsRetryableAPIError` now returns false
  for it — a parked state must never be retried (plain 409 `turn_in_progress` /
  `gate_in_progress` stay retryable).

**B. App (`app/step_runtime.go`, `app/app.go`)**
- `flowLoopBlocked()` — loop `blocked` with no live child, no turn/focus stream.
  A running child still wins (BUG-231 legacy), keeping `[stop]` armed.
- `turnIsActive()` returns false for a parked blocked flow (no `[stop]`).
- `applyAgentGraph()` centralizes loop+agents+blockReason+banner application,
  with a **stale-parent guard**: graphs for a different `ParentRunID` are
  ignored (`AgentGraphMsg`, `agent_graph_updated`).
- `showBlockedBanner()` — warn banner "Flow is waiting for you (blocked:
  <reason>) — /continue or /stop" + `ConnWaiting` "awaiting your decision"
  (Desktop `FlowAwaitingUserCard` parity).
- `cmdHydrateAgentGraph()` — one-shot `GET /agent-graph` on `/open` of a flow
  run (Desktop `refreshAgentGraph` on history open), so loop state seeds.
- `turnStreamClosedMsg` 409 awaiting-user → park (keep handle, blocked + banner
  + hydrate graph/runs/steps), not error/drop-handle. Non-awaiting-user errors
  (500, 409 invalid_request, 500-with-decision-wording) still fail+drop.
- `/continue` slash → `cmdContinueFlow` (POST agent-loop/continue, apply graph);
  `/stop` remains valid on a parked blocked flow; `StoppedMsg` seals loop as
  `stopped`.

## Provider impact

Provider-agnostic. The loop-state/awaiting-user semantics are run-level, not
provider-adapter logic. All new matrix tests parameterize Claude / Codex / Grok
(cross-provider-parity Case 1 + parameterized guard).

## Tests

New additive file `app/tui_flow_awaiting_user_test.go` (mirrors Desktop
`store.flow-blocked-terminal` / `store.flow-awaiting-user-409` /
`store.flow-awaiting-user-full-matrix`):

- `turnIsActive` blocked (no `[stop]`) × claude/codex/grok.
- Blocked loop + running child still arms `[stop]` (BUG-231 legacy).
- `AgentGraphMsg` blocked sets loop+blockReason+banner, no settle, no `[stop]`
  × 3 providers × blockReason cap/escalate/member_stalled.
- Blocked → done graph settles to ConnIdle "done".
- Stale-parent graph ignored (Desktop "switch discards late graph" guard).
- `turnStreamClosed` 409 `flow_awaiting_user` parks (keeps handle, blocked,
  warn, refresh) × 3 providers × 3 blockReasons; non-awaiting errors still fail
  (500, 409 invalid_request, 500-with-decision-wording).
- `/continue` unparks (POST continue, loop→running) × 3 providers.
- `/stop` on parked blocked seals loop `stopped`.
- Client wire contract: `GetAgentGraph` decodes blocked loop fields;
  `ContinueFlow` returns running; `IsFlowAwaitingUserError` true +
  `IsRetryableAPIError` false for 409 flow_awaiting_user; plain 409
  turn_in_progress stays retryable.

## Contract matrix

| Surface | Coverage |
|---|---|
| Terminal-state reopen parity (completed) | CA-515/516/520 tests untouched+green |
| Awaiting-user (blocked) reopen parity | `turnIsActive` blocked / `AgentGraphMsg` blocked / `turnStreamClosed` park / `/continue` / `/stop` |
| running child wins over blocked loop | `TestTurnIsActive_BlockedFlowWithRunningChildStillArmsStop` |
| stale/late graph guard | `TestAgentGraphMsg_StaleParentIgnored` |
| freeform 409 mapping | awaiting-user parks; invalid_request/500 still fail |
| Providers | claude/codex/grok parameterized |

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green).
- New tests + app/client race-clean: `go test ./internal/tui/app ./internal/tui/client -race`.

## Out of scope / residual

- Runner `startTurn` 409 gate and `historyStatusForLiveRun` unchanged.
- Pre-existing unrelated suites (`internal/runner` flaky, `internal/structure`,
  `internal/changecontract` platform path-separator on darwin) unchanged.
- `/continue` uses feedback="continue"; no per-memberAction (retry/skip) — a
  future member_stalled refinement can extend `ContinueFlow` body.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: The TUI now treats a blocked (awaiting-user) flow loop as parked, not live: /open hydrates the agent graph to seed loopState, turnIsActive no longer arms [stop] for a parked blocked flow, a 409 flow_awaiting_user parks instead of failing, a stale-parent graph guard prevents late blocked refreshes from clobbering results, and /continue + /stop unblock/seal the loop (Desktop FlowAwaitingUser parity, run-189839 / BUG-231)
# --->8---

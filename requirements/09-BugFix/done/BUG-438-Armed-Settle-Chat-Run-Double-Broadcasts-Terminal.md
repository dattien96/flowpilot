# BUG-438: chat run with armed pendingFlowGateSettle double-broadcasts `turn_completed` to live subscribers

## Metadata

- Document ID: `BUG-438`
- Title: `armed-settle chat run emits live turn_completed AND materialized turn_completed — SSE subscribers see a duplicate terminal`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Feature Keys: `durable-replay`, `workflow-runtime`
- Parent Documents: BUG-375 (Devin file_changed), BUG-435 (replay terminal ordering)
- Tags: `sse, terminal-dup, gate-settle, live-evidence`

## Bug report

Live verification of the Devin fixes (lt-verify-devin, run-1, devin/swe-2-max):
after the reprompted turn-42 passed the post-turn gate, the SSE stream carried
**two identical `turn_completed` events** for turn-42 (same occurredAt
`2026-09-22T17:07:45.963128Z`, same finalMessage) back-to-back.

## Reproduction

1. Chat run (normal_chat, no parent, not flowEngineDriven).
2. Turn emits `file_changed` (non-doc path) + `turn_completed` →
   `hasCode=true` → `markPendingFlowGateSettleLocked` arms the deferred settle.
3. `emitLocked` computes `deferGateCompleted=false` for chat roots (neither
   parent-of-flow-child nor flowEngineDriven) → the live terminal is persisted
   AND broadcast immediately.
4. Post-turn gate passes → the armed-settle pass branch materializes a second
   `completedEv` (same occurredAt via `pendingFlowGateOccurredAt`) and
   broadcasts it unconditionally to `rs.subs`.

Result: every live subscriber receives `turn_completed` twice for the same
providerTurnID. In-memory `rs.events` stays correct (keyed upsert dedups), so
this is a broadcast-only duplication — but SSE/TUI consumers see a double
terminal.

## Root cause

The materialization broadcast exists for the DEFERRED case (flow children /
flow-driven roots) where `emitLocked` suppressed the live broadcast. For armed
chat runs the live broadcast already happened, and nothing records that fact —
the pass branch re-broadcasts unconditionally.

Pre-existing for all providers on chat runs with code changes + gate pass;
latent on Devin until BUG-375 made `file_changed` actually emit.

## Evidence

- SSE capture `/tmp/lt-verify-sse.log`: two `turn_completed` frames for
  `providerTurnId=turn-42`, identical `occurredAt`/`finalMessage`.
- Runner log: `[settle] finalized run=run-1 turn=turn-42` once — dispatch
  ledger is fine; only the event broadcast doubles.

## Expected

Exactly one `turn_completed` reaches subscribers per provider turn, on both
the deferred (flow) and non-deferred (chat) paths.

## Severity

medium — event-stream correctness; UI may render duplicate terminal /
double-fire completion handlers on every gated chat turn that touches code.

## Completion Notes (implemented 2026-09-22, CA-916)

- Fix: `terminalBroadcastTurns` per-run set on `interactiveRun` — emitLocked
  records a turnID when its `turn_completed` actually reaches subscribers
  (non-deferred). Both post-gate materialization broadcasts (live pass branch
  and `resumePendingFlowGate`) skip re-broadcasting a marked turn. RAM-only:
  post-restart materialization still emits since no live broadcast happened
  in-process.
- Files: `internal/runner/interactive_service.go`.
- Tests: `bug438_terminal_dup_test.go` — live subscriber sees exactly one
  `turn_completed` on an armed-settle chat run (was 2).
- Provider-agnostic (no providerKey branch); found live on Devin but latent
  for every provider on gated chat turns that touch code.

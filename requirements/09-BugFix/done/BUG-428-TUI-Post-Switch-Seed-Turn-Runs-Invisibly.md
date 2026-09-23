# BUG-428: TUI post-switch seed turn runs invisibly — pending approval unreachable, `/stop` no-ops

## Metadata

- Document ID: `BUG-428`
- Title: `applyChatSwitched sets seedTurnActive=true which suppresses the seed turn's UI — real tool calls/approvals proceed with no surface`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: evidence `~/fp-beds/lt-evidence/ui/RESULT.md` (BUG-LIVE-UI-5)
- Feature Keys: `tui`, `provider-switch`, `seed-turn`, `approval-card`

## AI Quick View

### Summary

- After a `/provider` switch, the TUI suppresses the seed turn's UI via `seedTurnActive=true` — but the seed turn is a **real turn** that performs tool calls and requests approvals.
- Live: `run-786` (opencode leg) sat `status=running`/`waiting_approval` with `pendingApproval appr-793 → appr-811` for >20 s while the TUI showed `ready · prompt cleared`, `turnActive=false`, and **no approval card**.
- `/stop` no-ops (nothing "active" to stop — it's an API-level interrupt only); recovery was possible only via `POST /interrupt`.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

After switching provider mid-chat, the first (seed) turn on the new leg runs with no visible surface: no approval card for its pending approvals, status line shows `ready`, and `/stop` cannot interrupt it.

### Expected

Either the seed turn does not perform approvable work, or its approvals/activity render like any other turn — an approval card appears and `/stop`/interrupt reaches it.

### Actual

- `pendingApproval appr-793 → appr-811` pending >20 s invisible to the user; the run sat `waiting_approval`.
- TUI: `ready · prompt cleared`, `turnActive=false`, no card; `~/.flowpilot/tui.log` shows `input-watchdog`, `turnActive=false`.
- `/stop` is an API interrupt that only acts on "active" turns → no-op here; only `POST /interrupt` recovered the session.

### Impact

Severe TUI wedge: a provider switch can strand the chat in an invisible `waiting_approval` state the user cannot see, approve, deny, or stop — the session appears ready but is actually blocked.

## Reproduction

1. In TUI, run a chat with provider A, then `/provider <B>` (or a posture switch that routes through provider switch).
2. The seed turn on the new leg starts a run that requests a tool approval.
3. Observe: TUI shows `ready`, no approval card; `GET /client/workflow-runs/<run>` shows `waiting_approval` + `pendingApproval`; `/stop` does nothing; `POST /interrupt` is required.

## Root cause

- `apps/local-runner/internal/tui/app/chat_switch.go:354` — `applyChatSwitched` sets `m.seedTurnActive = true`, which suppresses the seed turn's rendering (comment at :353 notes intent: suppress a spurious You-box) — but suppression also hides the turn's approval surface and marks no turn active.
- `/stop` path only interrupts active turns → no-op against a suppressed (`turnActive=false`) but server-side `waiting_approval` turn.

## Evidence

- `~/fp-beds/lt-evidence/ui/RESULT.md` — BUG-LIVE-UI-5: `api-run786-pending.json`, `ui8-suppressed-approval*.txt`, `~/.flowpilot/tui.log` (`input-watchdog`, `turnActive=false`); run-786 `waiting_approval` `appr-793 → appr-811` >20 s.
- Verified on main worktree HEAD `435e336b`: `chat_switch.go:354` `m.seedTurnActive = true` (evidence cited the same line).

## Severity

- `medium` (severe for TUI UX) — invisible blocking approval state; recoverable only via raw API interrupt.

## Completion Notes (implemented 2026-09-23, CA-926)

- Root cause corrected vs. the initial report: `seedTurnActive` suppresses
  only the seed's assistant envelope — `permission_required` was never gated
  on it. The real defect was stream attachment: `applyChatSwitched` reset
  `turnStream` but left `orchStream` bound to the old leg, so
  `cmdStartOrchestrationStream` bailed (`orchStream != nil`) and the new
  leg's seed events had no consumer. Additionally a stale in-flight poll
  could deliver `orchStreamClosedMsg` after the new stream opened and kill
  it.
- Fix: `applyChatSwitched` now calls `stopOrchestrationStream()` before
  adopting the new handle; `orchStreamState` carries its `ctx` (cancelled on
  stop) and orch stream messages are tagged with their stream pointer so
  `Update` drops stale events/closes (`turn_stream.go`, `app.go`,
  `chat_switch.go`).
- Tests: `internal/tui/app/bug428_post_switch_stream_test.go` — stream
  reattach on switch, approval surfacing on the new leg, `/stop` reaching
  the seed turn, stale-msg drop.
- Live: mid-run `/provider devin` on the live bed → seed turn streamed
  (`▸ 2 tool calls`), `/stop` → `Stopped.` — verified via pty driver.

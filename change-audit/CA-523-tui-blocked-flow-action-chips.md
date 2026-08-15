---
id: CA-523
feature_key: cli-tui
title: blocked (awaiting-user) flow shows clickable Continue/Stop chips above composer
date: 2026-08-15
status: COMPLETE
---

## Problem

A flow whose orchestrator loop is `blocked` (awaiting a user Continue/Stop
decision, BUG-231 / run-189839) is surfaced in the TUI only as a **text** warn
banner ("Flow is waiting for you (blocked) — /continue or /stop"). The user must
know and type a slash command to act. Desktop shows a `FlowAwaitingUserCard` with
clickable **Continue** / **Stop** buttons; the TUI had no clickable equivalent.

## Fix (TUI-only)

Mirrors the existing Approve/Deny and dispatch-attention chip pattern — a
clickable action bar rendered above the composer when the flow is parked, no new
slash command required (`/continue` and `/stop` still work as before).

**`app/step_runtime.go`**
- `renderBlockedBar()` — when `flowLoopBlocked()` is true, returns a two-line
  action bar above the composer:
  `flow [blocked] awaiting your decision (<reason>)` then `[Continue]  [Stop]  click`.
  Returns `""` when not blocked so no extra input-bar row is allocated.

**`app/app.go`** (`renderInputLine`)
- Appends the `renderBlockedBar()` output to the input-block inner lines (after
  the dispatch-attention bar), so the chips sit above the composer exactly like
  the Approve/Deny and attention chips.

**`app/mouse.go`**
- `hitBlockedChrome(c, x, y)` — maps a click in the blocked action bar to
  `"continue"` / `"stop"` (token hit-test against the actual rendered input row).
- `clickTargetAt` checks it after the approve/deny chrome.
- `dispatchMouseClick`:
  - `target == "stop"` now also fires `cmdStopTurn()` when `flowLoopBlocked()`
    (the existing case only fired on `turnIsActive()`; a parked blocked flow has
    `turnIsActive()==false` but must still be stoppable).
  - new `target == "continue"` → `cmdContinueFlow(runID)` (POST
    `/agent-loop/continue`, apply graph) when `flowLoopBlocked()`.

## Provider impact

Provider-agnostic (Case 1): all touched logic reads loop state / run handle, never
`providerKey`. All new chip tests parameterize Claude / Codex / Grok.

## Tests

New additive file `app/tui_blocked_action_chips_test.go`:

- `renderBlockedBar` renders `[Continue]` and `[Stop]` chips when blocked ×
  claude/codex/grok.
- No chips when loop is `running` / `done`.
- No chips when a blocked loop has a live child (`flowLoopBlocked()` false —
  BUG-231 "running child wins").
- Clicking `[Continue]` POSTs `/agent-loop/continue` and applies the returned
  running graph (Desktop Continue parity, no slash) × 3 providers.
- Clicking `[Stop]` POSTs `/agent-loop/stop` and returns `StoppedMsg` × 3
  providers.
- `[Continue]` chip disappears once the graph reports the loop no longer blocked.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `gofmt` clean on all changed files (pre-existing misaligned test files
  untouched).
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green).
- New tests + app/client race-clean: `go test ./internal/tui/app ./internal/tui/client -race`.

## Out of scope / residual

- Runner 409 `flow_awaiting_user` gate and `historyStatusForLiveRun` unchanged
  (CA-521 already handles the park semantics).
- `/continue` uses feedback="continue"; per-memberAction (retry/skip) refinement
  remains a future extension (unchanged from CA-521).
- Pre-existing unrelated suites (`internal/runner` flaky, `internal/structure`,
  `internal/changecontract` platform path-separator on darwin) unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: A blocked (awaiting-user) flow loop now surfaces clickable [Continue]/[Stop] action chips above the composer (renderBlockedBar + hitBlockedChrome + dispatchMouseClick), mirroring the Approve/Deny + dispatch-attention chip pattern and the Desktop FlowAwaitingUserCard — no slash command required (run-189839 / BUG-231)
# --->8---

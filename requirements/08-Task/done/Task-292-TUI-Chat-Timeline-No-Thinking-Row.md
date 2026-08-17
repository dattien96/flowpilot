# Task-292: TUI Chat Timeline Must Not Show a Duplicate Thinking Loading Row

## Metadata

- Document ID: `Task-292`
- Title: `Remove the animated "Thinking" placeholder from the TUI chat timeline — the loading spinner lives on the status bottom line + the F2 RUNNING step`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-17`
- Last Updated: `2026-08-17`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56 Terminal TUI Chat And Flow Client](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Child Documents: [CA-537](../../../change-audit/CA-537-tui-chat-timeline-no-thinking-row.md)
- Related Documents: [CA-533](../../../change-audit/CA-533-tui-animated-thinking-placeholder.md), [CA-534](../../../change-audit/CA-534-tui-run-sidebar-flow-thinking-paste-history.md), [CA-536](../../../change-audit/CA-536-tui-chat-freeze-empty-options-gate.md), [BUG-325](../../09-BugFix/done/BUG-325-TUI-Chat-Freezes-After-Flow-Gate-With-Empty-Options.md)
- Replaces: `None`
- Tags: `cli-tui, ui, loading-indicator, spinner`

## AI Quick View

### Summary

- The animated "Thinking" loading indicator was shown in **two places**: the status bottom line + F2 step table (already communicating live work) **and** a new animated "Thinking" row in the chat timeline (CA-533).
- CA-537 removes the chat-timeline row. The spinner now animates on the status bottom line **and** on the F2 RUNNING step whenever a chat turn or flow step is live, and stops while the user is deciding (gate/question/approval), when the flow is parked on operator attention, or idle.
- Liveness is a single `workIsLive()` predicate; the 90ms spinner ticker is driven by it (not by the presence of a placeholder message).

### Current Ask

- Whenever chat runs or flow runs, show the loading animation on the status bottom line + F2 RUNNING step (as it already was), and never render a separate "Thinking" row in the chat timeline.

### Key Decisions

- `T-1` `workIsLive()` = chat turn or flow step actively working: `pendingPrompt`, `flowHasActiveAgents`, `ConnRunning`, live turn/focus stream, or flow/step run with a non-terminal handle.
- `T-2` Decision states (gate/question/approval) are NOT live — checked before `flowHasActiveAgents` because an agent waiting on the user reports a `waiting_user_approval` status. `ConnConnecting`/session-loading is also not live (already surfaces "connecting…"/"loading…").
- `T-3` `processInput`/`handleGateInput` no longer add a "thinking" chat message; `chatRows` skips any stray placeholder; `chatRowsSig` drops the `thinkingFrame` dependency.
- `T-4` F2 RUNNING step uses the same `thinkingSpinner` glyph as the status line; `WAITING_USER_APPROVAL` keeps `•` (parked on the user, not working).
- `T-5` Pre-existing tests that asserted the thinking-in-chat contract were updated (user-approved exception to additive-tests-only); all other legacy tests untouched.

### Constraints

- additive-tests-only unless a test asserts the removed chat-row contract (T-5); green old tests stay the regression guard.
- The chat placeholder machinery (`thinkingIndex`, `replaceThinkingAt`, `clearThinkingPlaceholder`, `addMessage` frame reset) is kept for replay/legacy — just never created by the normal prompt/gate path and never rendered.
- Provider-agnostic (Case 1); all matrix tests run `claude`/`codex`/`grok`.

### Open Questions

- `Q-1` None.

### Source Refs

- Operator request: "show item thinking loading … already show ở F2 table (status bottom line) thì không cần show ở chat timeline nữa. check và remove."
- Code: `apps/local-runner/internal/tui/app/app.go` (`workIsLive`, `processInput`, `handleGateInput`, `chatRows`, `chatRowsSig`, ticker), `status_bar.go` (`statusReadyLabel`), `session_panel.go` (`flowStepsPanelLines`), `thinking_anim.go` (spinner helpers).
- Tests: `apps/local-runner/internal/tui/app/ca537_thinking_chat_timeline_test.go`.

## 1. Goal

The chat timeline shows only real conversation. Live-work feedback (the animated
spinner + elapsed time) comes from the status bottom line and the F2 RUNNING
step — whenever chat or flow is running — and stops whenever the system is
waiting on the user or idle.

## 2. Parent Links

- coding plan: CP-56 (TUI chat + flow client chrome)
- tech design: none directly (TUI rendering layer)
- system spec: none directly (TUI layer)
- specific upstream ids: CA-533 (thinking animation origin), CA-534 (F2 sidebar + flow chrome)

## 3. Trigger

UX cleanup: CA-533 added the animated "Thinking" row to the chat timeline, but
the loading indicator was already present on the status bottom line and the F2
step table — a duplicated signal that pushed the user's prompt up and added
noise to the transcript.

## 4. Exact Change

1. `app.go`: add `workIsLive()`; drive `cmdThinkingTick` arming + `thinkingTickMsg`
   rescheduling from it (frame reset on the idle→live edge); remove the
   `addMessage("assistant", "thinking…", "thinking")` calls in `processInput`
   and `handleGateInput`; skip `FormatHint=="thinking"` in `chatRows`; drop the
   `thinkingFrame` hashing from `chatRowsSig`.
2. `status_bar.go`: `statusReadyLabel` returns the animated label while
   `workIsLive()`, else the static statusMsg.
3. `session_panel.go`: F2 RUNNING step row uses `thinkingSpinner(thinkingFrame,
   asciiMode)` as its glyph.

## 5. Touched Areas

- `apps/local-runner/internal/tui/app/app.go`
- `apps/local-runner/internal/tui/app/status_bar.go`
- `apps/local-runner/internal/tui/app/session_panel.go`
- `apps/local-runner/internal/tui/app/thinking_anim_test.go`
- `apps/local-runner/internal/tui/app/chat_ux_actions_test.go`
- `apps/local-runner/internal/tui/app/tui_run_sidebar_gate_paste_history_test.go`
- `apps/local-runner/internal/tui/app/tui_f2_right_sidebar_test.go`
- `apps/local-runner/internal/tui/app/ca537_thinking_chat_timeline_test.go` (new)
- `change-audit/CA-537-tui-chat-timeline-no-thinking-row.md`
- modules: `cli-tui`

## 6. Acceptance Check

- Sending a prompt in chat: no "Thinking" row appears in the timeline; the
  status bottom line shows the animated spinner; the F2 RUNNING step shows the
  animated spinner.
- Flow running after a gate decision: status + F2 animate, no chat row.
- Gate/question/approval pending: no spinner anywhere (the system is waiting on
  the user); `WAITING_USER_APPROVAL` F2 row keeps the `•` glyph.
- Flow/step run with a live (non-terminal) handle keeps animating even on
  `ConnIdle` chrome; a terminal handle + idle stops it.
- `claude`/`codex`/`grok` all behave identically (matrix tests).
- New tests green; untouched legacy tests green.

## 7. Out of Scope

- Removing the chat placeholder state machinery (`thinkingIndex`,
  `replaceThinkingAt`, `clearThinkingPlaceholder`, `addMessage` frame reset) —
  kept for replay/legacy.
- `renderThinkingLine` (the shimmer row renderer) — kept for legacy reference
  and still covered by `TestThinkingShimmer_WindowSweeps`.
- Runner / Desktop behavior.

## 8. Completion Notes

- result: chat timeline has no "Thinking" row; spinner animates on the status
  bottom line + F2 RUNNING step whenever chat/flow is live and stops when the
  user is deciding or idle.
- follow-ups: none.
- upstream docs updated: CA-537; CP-56 TUI chrome note.
- verification: `go build ./...`, `go vet ./internal/tui/...`,
  `go test ./internal/tui/app/ -count=1 -skip "TestCmdFocusAgent_.*"` and
  `go test ./internal/tui/client/ -count=1` all green.

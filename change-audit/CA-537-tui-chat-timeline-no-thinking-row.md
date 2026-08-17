---
id: CA-537
feature_key: cli-tui
title: Remove chat-timeline thinking row — spinner lives on status line + F2 RUNNING step
date: 2026-08-17
status: COMPLETE
---

## Problem

The animated "Thinking" loading indicator was duplicated: the status bottom
line and the F2 step table already communicate that work is in progress, and
CA-533 also injected an animated "Thinking" row into the chat timeline on every
prompt send and gate decision. The chat row duplicated the loading signal and
pushed the user's prompt up as the placeholder sat under it.

## Fix (TUI-only)

1. **`workIsLive()` predicate** (`app.go`) — true whenever a chat turn or flow
   step is actively working:
   - `pendingPrompt`, `flowHasActiveAgents`, `connStatus == ConnRunning`,
     live `turnStream`/`focusStream`, or a flow/step run with a non-terminal
     handle (`shouldPollStepsRuntime`).
   - **Not live** while the user is deciding (gate/question/approval), when the
     flow is parked on operator attention (`hasUnresolvedAttention`), when a
     blocked loop is paused, or idle. `ConnConnecting`/session-loading is also
     not live — those phases already show "connecting…"/"loading…".
2. **Spinner ticker driven by `workIsLive()`** instead of `thinkingIndex()`.
   The 90ms tick arms on the idle→live edge (resetting `thinkingFrame` so the
   elapsed clock does not race ahead) and self-cancels when work ends.
3. **No thinking row in the chat timeline**:
   - `processInput` and `handleGateInput` no longer call
     `addMessage("assistant", "thinking…", "thinking")` (statusMsg stays
     "thinking…" for legacy tests).
   - `chatRows` skips any stray `FormatHint=="thinking"` placeholder (legacy
     replay safety) so it never renders.
   - `chatRowsSig` no longer hashes `thinkingFrame` — the chat row cache is
     frame-independent.
   - The placeholder machinery (`thinkingIndex`, `replaceThinkingAt`,
     `clearThinkingPlaceholder`, `appendAssistantDelta`/`ensureAssistantMessage`
     replacement, `addMessage` frame reset) is kept for replay/legacy.
4. **Status line** — `statusReadyLabel` returns the animated
   `thinkingLabelText` (spinner + "Thinking" + elapsed) whenever `workIsLive()`,
   else the static statusMsg.
5. **F2 step table** — the RUNNING step row uses the same animated
   `thinkingSpinner` glyph instead of the static `•`; `WAITING_USER_APPROVAL`
   keeps `•` (it is parked on the user, not working).

## Provider impact

Provider-agnostic (Case 1). All matrix tests parameterize Claude / Codex / Grok.

## Tests

Updated (user-approved, the pre-existing tests that asserted the thinking-in-chat
contract):

- `thinking_anim_test.go` — `TestThinkingRow_NotRenderedInChatView`,
  `TestChatRowsSig_ThinkingPlaceholderDoesNotInvalidate`,
  `TestStatusReadyLabel_ShowsAnimatedSpinnerWhenLive`, and
  `TestThinkingTick_AdvancesAndSelfStops` now drive liveness with
  `connStatus` instead of the placeholder message.
- `chat_ux_actions_test.go` — `TestProcessInput_NoThinkingChatRow`.
- `tui_run_sidebar_gate_paste_history_test.go` —
  `TestFlowMode_SpinnerOnStatusLineNotChat`,
  `TestGateDecision_SpinnerOnStatusLineNotChat`.
- `tui_f2_right_sidebar_test.go` — `TestStepTodoRestyle_KeepsLegacyTokens`
  asserts the ascii spinner `[|]` (frame 0) instead of `[•]`.

New additive file `tui/app/ca537_thinking_chat_timeline_test.go`:

- `TestChatTimeline_NoThinkingRow` — a thinking placeholder never renders.
- `TestWorkIsLive_Matrix` — pendingPrompt / ConnRunning / streams / active
  agents / live flow handle → live; gate/question/approval (even with a
  waiting_user_approval agent) / ConnConnecting / ConnWaiting-without-flow /
  terminal handle → not live.
- `TestStatusSpinner_LiveWhileChatTurn` — prompt send: no chat row, status
  spinner live, tick advances + reschedules.
- `TestStatusSpinner_StopsWhenIdle` / `TestStatusSpinner_StopsDuringGate`.
- `TestCursorTick_ArmsTickerOnLiveEdge` — arms + resets frame on idle→live edge.
- `TestF2RunningStep_SpinnerGlyph` — F2 RUNNING row uses the spinner glyph and
  advances with the frame.

## Verification

- `go build ./...` clean; `go vet ./internal/tui/...` clean.
- gofmt clean (checked via LF-normalized temp copies; repo files are CRLF);
  new files written CRLF.
- `go test ./internal/tui/app/ -count=1` green (excluding the two known
  pre-existing flaky `TestCmdFocusAgent_*` network tests).
- `go test ./internal/tui/client/ -count=1` green.
- Untouched legacy thinking-machinery tests still green:
  `bug324_thinking_placeholder_test.go`, `turn_stream_closed_thinking_test.go`,
  `TestThinkingTick_ResetsOnFreshPlaceholder`.

## Out of scope / residual

- The chat placeholder state machinery stays for replay/legacy; it is simply
  never created by the normal prompt/gate path and never rendered if present.
- `renderThinkingLine` (the shimmer row renderer) remains for legacy reference
  and is still exercised by `TestThinkingShimmer_WindowSweeps`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-537
change_type: refactor
summary: Remove the animated Thinking row from the chat timeline (it duplicated the status line + F2 step loading indicator) and drive the spinner on the status bottom line + F2 RUNNING step from a workIsLive predicate so it animates whenever a chat turn or flow step is live and stops during gate/question/approval, parked attention, or idle
# --->8---

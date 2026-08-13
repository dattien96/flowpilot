# Task-290: TUI Long Chat Load-Earlier Windowing (CP-56 P-8c)

## Metadata

- Document ID: `Task-290`
- Title: `TUI Long Chat Load-Earlier Windowing`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-13`
- Feature Keys: `cli-tui, chat-history`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-8c; P-8 polish), [CP-56-Test-Steps](../../07-Coding-Plan/inprogress/CP-56-Test-Steps.md), [Task-287](./Task-287-TUI-Resume-Headless-And-Session-Reset.md), [Task-289](../done/Task-289-TUI-Codex-Style-Assistant-Markdown.md), [Task-062](../done/Task-062-Admin-Workflow-Run-Long-Chat-Session-Performance.md)
- Child Documents: `none`
- Related Documents: Desktop `Timeline.tsx` (`TIMELINE_PAGE_SIZE = 6`, `sliceTimelineFromPrompt`, `↑ Load earlier prompts`), [CA-462](../../change-audit/CA-462-tui-scroll-markdown-cache.md), [CA-086](../../change-audit/CA-086-admin-workflow-run-chat-session-windowing.md)
- Replaces: `None`
- Tags: `cli-tui, chat-history, performance, windowing`

## AI Quick View

### Summary

- Long TUI chats currently replay from seq 0 and paint every message (`cmdOpenChat` cap 8000; `buildChatRows` walks the full `messages` slice).
- Desktop already windows the timeline: last 6 user prompts first, `↑ Load earlier prompts (N)` to page older groups in.
- This slice adds the same recent-first window to `flowpilot chat` so a long session stays usable.

### Current Ask

Implement P-8c. Do not start until this task is picked up; capture only.

### Key Decisions

- `T-1` Window by user-prompt groups from the newest end, page size 6, matching Desktop `TIMELINE_PAGE_SIZE`.
- `T-2` Hidden older groups stay in memory for follow-up turns; they are not rendered until Load earlier.
- `T-3` Same window for live chat growth and `/history` `/open` `/resume` replay.
- `T-4` A pending approval/question/thinking row must stay visible even if its prompt would otherwise be paged out.
- `T-5` Thin client only: no new runner history-page API in this slice (Desktop also windows render-side).

### Constraints

- CP-56 D-1/D-7/D-8: no runner business edits; additive TUI tests only.
- Do not undo Task-289 markdown cache or CA-462 `chatRows` cache.
- Do not change resume cursor semantics from Task-287 (`replay until lastEventSeq`, then live).
- GitNexus impact before editing `buildChatRows` / `cmdOpenChat` / `View` when those tools are available. This capture thread had no GitNexus MCP.

### Open Questions

- `Q-1` If seq-0 SSE replay itself is the bottleneck on very large runs, a later task may add `afterSeq` tail-fetch. Out of this slice unless proven after render windowing.
- `Q-2` Exact Load-earlier key: clickable row vs a slash vs `PgUp` at top. Default: a transcript header control equivalent to Desktop's button, plus scroll-to-top revealing it.

### Source Refs

- CP-56 P-8, D-1, D-7; Task-287 T-5; Task-289 T-2; Desktop `apps/desktop-flowpilot/src/components/Timeline.tsx`; Task-062 / CA-086 (same recent-first idea, admin-web).

---

## 1. Goal

A long TUI chat (live or resumed) initially shows the newest prompt groups, not the full history from turn 1, with an explicit Load earlier control — Desktop Timeline parity — so markdown/layout cost stays bounded.

## 2. Parent Links

- coding plan: CP-56 P-8c (narrows P-8 polish; does not replace Task-287 resume or Task-289 markdown)
- tech design: SD-02 (TUI is a parallel client; presentation-only)
- system spec: none dedicated; client chrome only
- specific upstream ids: CP-56 D-1, D-7; Task-287 T-3/T-5; Desktop `TIMELINE_PAGE_SIZE`

## 3. Trigger

Operator: long chats must not load/render from the beginning; use Desktop-style load more for performance.

Current TUI (`history.go` `cmdOpenChat`) always `StreamRun(..., afterSeq=0)` until `LastEventSeq` (hard cap 8000) and `replayHistoryMessages` into the full `messages` list. `buildChatRows` then styles every assistant bubble. Desktop keeps the full store timeline but only renders `sliceTimelineFromPrompt(timeline, visiblePromptCount)` with page size 6.

## 4. Exact Change

- `T-1` Add a visible-prompt window (newest 6 user prompts + their following assistant/tool/system rows). Page older groups in batches of 6.
- `T-2` Show a Load earlier control when `hiddenPromptCount > 0`, copy-aligned with Desktop (`↑ Load earlier prompts (N)`).
- `T-3` Apply the window after resume replay and as the live transcript grows past 6 prompts. Reset the window on `/new`.
- `T-4` Always include in-flight thinking, the active turn, and unresolved approval/question rows in the visible slice.
- `T-5` Keep `chatRows` / markdown caches; do not re-render hidden messages on scroll or blink.
- `T-6` Additive tests only (new file preferred): window math, load-earlier expansion, pending-gate visibility, `/new` reset.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tui/app/history.go` (`cmdOpenChat` replay still allowed; window after `replayHistoryMessages`)
  - `apps/local-runner/internal/tui/app/app.go` (`buildChatRows` / `View` / chat list)
  - new `apps/local-runner/internal/tui/app/chat_windowing.go` (optional extract, Desktop `sliceTimelineFromPrompt` analog)
  - new `apps/local-runner/internal/tui/app/chat_windowing_test.go`
- modules: `cli-tui`
- routes: none (existing `/client/workflow-runs/{id}/events/stream`)
- tables: none

## 6. Acceptance Check

- Open or resume a chat with more than 6 user prompts: first paint shows the newest 6 groups, not turn 1.
- Load earlier reveals the previous page; repeating reaches the start.
- Follow-up send still appends at the bottom; scroll stays on the live tail.
- Unresolved approval/question remains visible.
- New unit tests green; related old TUI tests untouched and green.
- Manual: same long chat on Desktop still uses `↑ Load earlier prompts`.

## 7. Out of Scope

- New runner pagination endpoints or changing `afterSeq` replay contract.
- Admin-web session windowing (already Task-062).
- Markdown renderer rewrite (Task-289).
- Headless `-p` transcript printing.
- Sub-agent child transcripts (unless they reuse the same message list helper — then window that list the same way).

## 8. Completion Notes

- result: Captured only. Not implemented in this turn.
- follow-ups: If first paint is still slow after render windowing, measure seq-0 SSE replay and consider a tail-fetch task (`Q-1`).
- upstream docs updated: CP-56 P-8c + Task Cut; CP-56-Test-Steps A8.16–A8.19 stubs.
- verification: none run (no code in this turn).

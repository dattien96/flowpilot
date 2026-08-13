# Task-290: TUI Long Chat Load-Earlier Windowing (CP-56 P-8c)

## Metadata

- Document ID: `Task-290`
- Title: `TUI Long Chat Load-Earlier Windowing`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-13`
- Feature Keys: `cli-tui, chat-history`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-8c; P-8 polish), [CP-56-Test-Steps](../../07-Coding-Plan/inprogress/CP-56-Test-Steps.md), [Task-287](./Task-287-TUI-Resume-Headless-And-Session-Reset.md), [Task-289](./Task-289-TUI-Codex-Style-Assistant-Markdown.md), [Task-062](./Task-062-Admin-Workflow-Run-Long-Chat-Session-Performance.md)
- Child Documents: [CA-475](../../change-audit/CA-475-tui-long-chat-load-earlier-windowing.md)
- Related Documents: Desktop `Timeline.tsx` (`TIMELINE_PAGE_SIZE = 6`, `sliceTimelineFromPrompt`, `↑ Load earlier prompts`), [CA-462](../../change-audit/CA-462-tui-scroll-markdown-cache.md), [CA-086](../../change-audit/CA-086-admin-workflow-run-chat-session-windowing.md)
- Replaces: `None`
- Tags: `cli-tui, chat-history, performance, windowing`

## AI Quick View

### Summary

- Long TUI chats window to the newest six user-prompt groups on render (Desktop Timeline parity).
- `↑ Load earlier prompts (N)` at the transcript top pages six older groups per click.
- Full transcript stays in memory after open; older chunks load on demand via tail SSE replay (Q-1).

### Current Ask

Done. P-8c landed in `chat_windowing.go` + `buildChatRows` windowing.

### Key Decisions

- `T-1` Window by user-prompt groups from the newest end, page size 6, matching Desktop `TIMELINE_PAGE_SIZE`.
- `T-2` Hidden older groups stay in memory for follow-up turns; they are not rendered until Load earlier.
- `T-3` Same window for live chat growth and `/history` `/open` `/resume` replay.
- `T-4` A pending approval/question/thinking row must stay visible even if its prompt would otherwise be paged out.
- `T-5` Thin client only: tail SSE replay via existing `afterSeq` stream cursor; no new runner endpoint.
- `T-6` On open, replay from `tailAfterSeq ≈ lastEventSeq - 400` instead of seq 0; Load earlier fetches older chunks when in-memory window is exhausted.

### Constraints

- CP-56 D-1/D-7/D-8: no runner business edits; additive TUI tests only.
- Do not undo Task-289 markdown cache or CA-462 `chatRows` cache.
- Do not change live follow cursor semantics from Task-287 (replay through `lastEventSeq`, then live).

### Open Questions

- `Q-1` RESOLVED: tail SSE replay on open + chunked fetch on Load earlier when memory exhausted (existing `afterSeq` API).
- `Q-2` RESOLVED: clickable `↑ Load earlier prompts` row at transcript top (mouse); scroll up to reveal.

### Source Refs

- CP-56 P-8c, D-1, D-7; Task-287 T-5; Task-289 T-2; Desktop `apps/desktop-flowpilot/src/components/Timeline.tsx`; CA-475.

---

## 1. Goal

A long TUI chat (live or resumed) initially shows the newest prompt groups, not the full history from turn 1, with an explicit Load earlier control — Desktop Timeline parity — so markdown/layout cost stays bounded.

## 2. Parent Links

- coding plan: CP-56 P-8c
- tech design: SD-02 (TUI is a parallel client; presentation-only)
- system spec: none dedicated; client chrome only
- specific upstream ids: CP-56 D-1, D-7; Task-287 T-3/T-5; Desktop `TIMELINE_PAGE_SIZE`

## 3. Trigger

Operator: long chats must not load/render from the beginning; use Desktop-style load more for performance.

## 4. Exact Change

- `T-1` Added `visiblePromptCount` + `sliceMessagesFromPrompt` (page size 6).
- `T-2` `buildChatRows` renders from `windowStartIndex()`; prepends styled load-earlier row.
- `T-3` Sync on chat open, user prompt append, agent focus restore; reset on `/new`.
- `T-4` `minVisibleIndexForPending()` pulls in thinking/approval/question/gate groups.
- `T-5` `chatRowsSig` includes `visiblePromptCount`; scroll still reuses row cache.
- `T-6` `chat_history_replay.go`: tail `afterSeq`, chunked older fetch, prepend merge.
- `T-7` `chat_windowing.go` / `history.go`: Load earlier triggers async fetch when at memory start.
- `T-8` `chat_history_replay_test.go` + `chat_windowing_test.go` cover A8.16–A8.22.

## 5. Touched Areas

- `apps/local-runner/internal/tui/app/chat_history_replay.go`
- `apps/local-runner/internal/tui/app/chat_history_replay_test.go`
- `apps/local-runner/internal/tui/app/chat_windowing.go`
- `apps/local-runner/internal/tui/app/chat_windowing_test.go`
- `apps/local-runner/internal/tui/app/history.go`
- `apps/local-runner/internal/tui/app/app.go` (`buildChatRows`, `addMessage`, `/new`, `ChatOpenedMsg`, `HistoryChunkMsg`)
- `apps/local-runner/internal/tui/app/model.go`
- `apps/local-runner/internal/tui/app/mouse.go`
- `apps/local-runner/internal/tui/app/agents_focus.go`
- `change-audit/CA-475-tui-long-chat-load-earlier-windowing.md`

## 6. Acceptance Check

- Open or resume a chat with more than 6 user prompts: first paint shows the newest 6 groups, not turn 1.
- Load earlier reveals the previous page; repeating reaches the start; further clicks fetch older SSE chunks when needed.
- Follow-up send still appends at the bottom; scroll stays on the live tail.
- Unresolved approval/question remains visible.
- New unit tests green; related old TUI tests untouched and green.

## 7. Out of Scope

- New runner pagination endpoints (client uses existing `afterSeq` stream cursor only).
- Admin-web session windowing (already Task-062).
- Markdown renderer rewrite (Task-289).
- Headless `-p` transcript printing.

## 8. Completion Notes

- result: Render-side prompt-group windowing plus tail SSE replay and chunked Load earlier fetch.
- follow-ups: Tune `chatReplayTailEventBudget` if operators report missing turns at open.
- upstream docs updated: CP-56-Test-Steps A8.16–A8.19 marked done; CA-475.
- verification: `go test ./internal/tui/app/ -run 'TestChatWindow_|TestSliceMessagesFromPrompt|TestChatRows_'` green.

---
id: CA-475
feature_key: cli-tui
title: TUI long-chat load-earlier prompt windowing
date: 2026-08-13
status: COMPLETE
---

## Change

Task-290: long TUI chats no longer render every prompt group on first paint. Mirrors Desktop `Timeline.tsx` (`TIMELINE_PAGE_SIZE = 6`): newest six user-prompt groups plus followers, `↑ Load earlier prompts (N)` row at the top (clickable), page six more on click. Pending thinking/approval/question/gate rows force their prompt group into the visible slice. `/new`, chat open, and agent focus restore reset/re-sync the window. Render-side only — seq-0 replay unchanged.

## Provider impact

Case 1 (agnostic). TUI presentation only.

## Tests

New file `chat_windowing_test.go`: slice math, newest-six window, load-earlier expansion, pending gate visibility, `/new` reset, cache invalidation. CP-56 A8.16–A8.19.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-290
change_type: feature
summary: TUI windows long chats to newest six prompt groups with load-earlier paging like Desktop
# --->8---

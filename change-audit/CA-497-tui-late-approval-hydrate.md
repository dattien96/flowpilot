---
id: CA-497
feature_key: cli-tui
title: TUI surfaces late and reopened pending approvals
date: 2026-08-14
status: COMPLETE
---

## Change

run-97624: YOLO=off write was approved and the user turn completed (`done`), then
the runner started a chat-mode gate-reprompt turn that needed a second shell
approval. TUI stopped listening after the first `turn_completed` and `/open`
cleared `m.approval` without reading the run snapshot, so a follow-up `hello`
POSTed a new turn and hit runner `409 awaiting_user`.

F-1: after a successful user-turn stream close, always start the existing
orchestration SSE (`cmdStartOrchestrationStream`) for **plain chat** as well as
flow. `shouldPollStepsRuntime` is unchanged (flow/step UI poll only).

F-2: `/open` still replays the tail transcript, then hydrates live pending
state from `GET /client/workflow-runs/{id}` (`GetRun`). Mount approval/question
only when status is `waiting_approval` / `waiting_question` (CA-089 twin).

F-3: after turn close, also `GetRun` as a safety net if the late SSE event is
missed.

`turnIsActive` no longer treats a plain-chat orch listener as an in-flight turn
(would have left `[stop]` armed forever). Flow/step orch + running children
still count as active.

YOLO=on late cards still auto-approve without mounting (CA-476). Waiting copy
unchanged (CA-477). Chat gate settlement on the runner is unchanged (BUG-298).

## Provider impact

Case 1 (agnostic). TUI chrome only — no `providerKey` branch. Snapshot hydrate
table-tested for grok/claude/codex.

## Tests

New `run97624_tui_late_approval_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: run-97624
change_type: bugfix
summary: TUI listens after chat turns and hydrates pending approval on /open
# --->8---

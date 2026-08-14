---
id: CA-498
feature_key: cli-tui
title: TUI /open keeps post-tool assistant frames
date: 2026-08-14
status: COMPLETE
---

## Change

run-97624 /open showed only the first Grok assistant frame
(`Creating tesssst.txt…`) and dropped `Đã tạo file … với nội dung`.

Grok `chat_history.jsonl` (and seed replay) emits one `message_completed`
before the write tool and another after. `replayHistoryMessages` filled the
assistant bubble from the first completed and ignored later completed /
`turn_completed` once the bubble was non-empty (that guard exists so a
step-complete stub cannot wipe streamed text).

Now later frames merge: append if distinct, take the longer superset, never
replace with `isStepCompleteStub`. Deltas still concatenate. CA-475 tail
windowing unchanged.

## Provider impact

Case 1 (agnostic). Replay helper takes no `providerKey`. Table-tested
claude/codex/grok event shapes.

## Tests

New `run97624_tui_replay_assistant_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: run-97624
change_type: bugfix
summary: TUI /open merges post-tool assistant frames instead of keeping only the first
# --->8---

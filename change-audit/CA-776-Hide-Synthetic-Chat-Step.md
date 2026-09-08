# CA-776 — Hide synthetic chat step until real flow nodes exist

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-326
change_type: bugfix
summary: Do not render the mint chat-run step in TUI notices or the steps panel; show blank until vibe-ingest nodes arrive
# --->8---

## Why

Live run-211960: first paint was `[RUNNING] chat` / `Now: chat` while `startResolvedFlow` was still async. Operator Stopped thinking there was no flow. Real nodes (`ingest_reader`…`ss_lock`) appeared later.

## Change

TUI only. `isSyntheticChatStep` (`StepType=chat` or `chat-*` id) skipped in `formatStepChatNotices`, `activeStepName`, `flowStepsPanelLinesMax`. Runner still mints the hub step.

## Tests

`ca776_hide_synthetic_chat_step_test.go`. Old step-notice tests untouched.

## Will not undo

CA-775 bare vibe-ingest resolve.

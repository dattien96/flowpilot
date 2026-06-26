# CA-029: Workflow Run Detail Follow-Up UX

## Scope

This audit captures the later follow-up UX fixes on the workflow run detail page, including bubble behavior and state handling.

## Completed

- Preserved optimistic follow-up chat bubbles so they do not disappear immediately after send.
- Kept the run detail view responsive while follow-up execution is running in the background.
- Added a safer merge path for follow-up decisions derived from runtime logs and local optimistic state.

## Verification

- Verified the prompt bubble stays visible after sending a follow-up message.
- Confirmed the loading state no longer wipes the user message from the feed.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-029
change_type: feature
summary: Workflow Run Detail Follow-Up UX
# --->8---

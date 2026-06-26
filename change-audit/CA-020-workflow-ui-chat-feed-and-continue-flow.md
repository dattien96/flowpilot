# CA-020: Workflow UI Chat Feed and Continue Flow

## Scope

This audit covers the first workflow UI revamp that converted the run detail page into a chat-like feed and allowed follow-up on completed steps.

## Completed

- Reworked the workflow run detail layout into a chat-style timeline feed.
- Allowed continuation from completed steps so users can submit follow-up prompts after the initial run ends.
- Added a more conversational approval/decision flow in the run detail page.
- Relaxed the workflow run status checks so the continuation box appears consistently.

## Verification

- Verified the chat-style layout renders step history in order.
- Confirmed follow-up actions appear when the run is in a continueable state.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-020
change_type: feature
summary: Workflow UI Chat Feed and Continue Flow
# --->8---

# CA-033: Workflow Run History Delete Flow

## Scope

This audit documents the workflow run history delete flow and the related gateway updates.

## Completed

- Added bulk delete support for workflow run history entries.
- Added per-row delete actions on the workflow run list page.
- Updated the workflow gateway implementations so deletes cascade through related run data.

## Verification

- Verified selected runs and full run history can be deleted from the UI.
- Confirmed the backing gateways remove dependent records consistently.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-033
change_type: feature
summary: Workflow Run History Delete Flow
# --->8---

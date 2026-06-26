# CA-034: Workflow Run Title and Summary Fixes

## Scope

This audit captures the run title and summary fixes that made the workflow history and detail pages easier to read.

## Completed

- Added prompt-summary-based run titles so the execution instance header uses the prompt instead of a raw UUID when possible.
- Improved the initial prompt extraction so the page can show the actual run prompt text from logs or local files.
- Kept the run summary display consistent across completed and follow-up runs.

## Verification

- Verified the run detail header resolves from prompt data when available.
- Confirmed the UUID fallback only appears when no prompt text can be derived.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-034
change_type: feature
summary: Workflow Run Title and Summary Fixes
# --->8---

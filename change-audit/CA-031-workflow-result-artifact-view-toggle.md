# CA-031: Workflow Result Artifact View Toggle

## Scope

This audit captures the later workflow result artifact view toggle cleanup that simplified the artifact tab.

## Completed

- Removed the inline `Expand`, `Reload`, and preview controls from the artifact tab.
- Kept `Open in new tab` as the single entry point for artifact inspection.
- Reduced the amount of inline artifact content rendered in the run detail page.

## Verification

- Verified the artifact tab now exposes only the metadata and the new-tab action.
- Confirmed the preview section no longer loads automatically.


# ---8<--- flowpilot:change-ledger
feature_key: artifacts
source_doc_id: CA-031
change_type: feature
summary: Workflow Result Artifact View Toggle
# --->8---

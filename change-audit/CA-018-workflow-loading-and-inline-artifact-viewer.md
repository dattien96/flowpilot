# CA-018: Workflow Loading and Inline Artifact Viewer

## Scope

This audit covers the execution loading polish and inline artifact preview work added to the workflow run detail page and the local runner file access path.

## Completed

- Added a clearer loading layer while execution details are being resolved so users see active progress instead of a blank panel.
- Introduced inline artifact content viewing on the workflow run detail page so markdown output can be inspected without leaving the page.
- Added a local runner `/files/read` endpoint to read generated artifact content from the local workspace.
- Updated the frontend local runner gateway to fetch artifact files directly for preview rendering.
- Fixed artifact markdown links so invalid `/abs/path/` prefixes are stripped before rendering.

## Verification

- Verified the loader behavior in the execution panel.
- Verified artifact file reads through the local runner flow.
- Confirmed markdown links render without the invalid absolute-path prefix.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-018
change_type: feature
summary: Workflow Loading and Inline Artifact Viewer
# --->8---

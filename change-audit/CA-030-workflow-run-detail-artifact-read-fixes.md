# CA-030: Workflow Run Detail Artifact Read Fixes

## Scope

This audit covers the local-runner file-read path fixes that resolved artifact loading failures on macOS and other environments.

## Completed

- Normalized local runner file paths so relative artifact paths resolve against the workspace root.
- Added a timeout to frontend artifact reads so the UI cannot hang forever on a stalled file request.
- Changed the browser-side local runner default base URL to `127.0.0.1` to avoid localhost resolution issues.

## Verification

- Verified the file-read path works against `.flowpilot` artifact locations.
- Confirmed the artifact tab shows a normal error instead of hanging indefinitely when the file cannot be read.


# ---8<--- flowpilot:change-ledger
feature_key: artifacts
source_doc_id: CA-030
change_type: feature
summary: Workflow Run Detail Artifact Read Fixes
# --->8---

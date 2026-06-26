# CA-027: Workflow Artifact Preview and Viewer

## Scope

This audit documents the artifact preview and browser-tab viewing workflow added to the run detail page.

## Completed

- Added an artifact tab in the workflow run detail page that exposes the generated output artifact.
- Added an `Open in new tab` action so markdown artifacts can be viewed in a full browser tab.
- Added the markdown preview helper that renders the artifact in a dedicated preview page.
- Removed the inline preview controls once the new-tab workflow proved sufficient.

## Verification

- Verified the artifact can be opened in a new tab and renders as markdown.
- Confirmed the old inline preview controls no longer appear.


# ---8<--- flowpilot:change-ledger
feature_key: artifacts
source_doc_id: CA-027
change_type: feature
summary: Workflow Artifact Preview and Viewer
# --->8---

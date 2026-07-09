# CA-267: Smooth Workflow Settings Selection and Sync Feedback

## Scope

Reduced reload thrash in the desktop `Workflows/Steps` settings screen so tapping between workflow and step rows no longer full-refreshes the whole settings payload. Save/create/delete/clone paths still refresh the list after mutations, and those refreshes now show a blocking loading modal.

## Changes

- `WorkflowsSettings.tsx`: split selection from refresh. Row taps now update local workflow/step state directly, while mutation paths continue to call `refresh()` so saved data and generated derived state stay authoritative.
- `WorkflowsSettings.tsx`: added a centered loading modal overlay for `busy` state so sync operations clearly communicate that the list is being refreshed.
- `styles.css`: added the modal-specific loading styles and pulse indicator animation.

## Verification

- `rtk npm run typecheck` (`tsc --noEmit`) — clean after the UI change.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-179
change_type: bugfix
summary: reduce Workflows/Steps selection reload thrash and show a blocking loading modal during save sync
# --->8---

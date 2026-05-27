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


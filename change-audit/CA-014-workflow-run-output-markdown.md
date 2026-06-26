# CA-014 Workflow Run Output Markdown Support

## Scope

Added support for storing and rendering markdown outputs directly inside artifact runs. Previously, users could not see the actual text result of a workflow run because the schema lacked a direct text payload field for step runs that do not output files.

## Completed

- Updated [supabase-gateway-bundle.ts](file:///c:/working/flowpilot/apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts):
  - Handled mapping and parsing of `content_markdown` property returned from Supabase `artifact_runs` queries.
- Updated [workflow-runs/$runId.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx):
  - Updated the detail timeline view to fetch and render `content_markdown` inside a styled output card if present.
- Updated [workflow-start-runtime.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/usecase/workflow-engine/workflow-start-runtime.ts):
  - Configured step run execution to populate the output content inside the artifact run entity database record.
- Added database migration:
  - `20260522150000_add_content_markdown_to_artifact_runs.sql` which adds the `content_markdown` column to `artifact_runs`.
- Updated Coding Plan documentation:
  - Staged `CP-15-02-Project-Launch-And-Workspace-UX.md`, `CP-15-03-Step-Definition-Execution-Contract.md`, and `CP-15-04-Workflow-Start-And-Runner-Runtime.md` splitting out the execution and runtime designs.

## Verification

- Verified the workflow run detail screen correctly renders outputs.
- Executed unit tests in `apps/admin-web` via `npm run test` (all tests passed).

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CP-15
change_type: feature
summary: Workflow Run Output Markdown Support
# --->8---

# CA-013 Project Workspace Bindings

## Scope

Implemented local project-to-workspace directory bindings to support linking local file systems and path directories to specific FlowPilot projects. This allows the local runner to correctly map workspace resources, definitions, and configurations dynamically based on the project selected in the UI.

## Completed

- Created [project-workspace-binding.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/model/entity/project-workspace-binding.ts) and [project-workspace-binding-payload.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/model/payload/project-workspace-binding-payload.ts):
  - Defined the domain model entity for workspace bindings (e.g. `id`, `projectId`, `localPath`, `isDefault`, `createdAt`, `updatedAt`).
- Updated [project-gateway.ts](file:///c:/working/flowpilot/apps/admin-web/src/domain/gateway/project-gateway.ts) and [supabase-gateway-bundle.ts](file:///c:/working/flowpilot/apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts):
  - Exposed and implemented gateway methods `getWorkspaceBindings`, `createWorkspaceBinding`, and `deleteWorkspaceBinding`.
  - Updated the Supabase repository bundle to fetch and mutate workspace bindings from the Supabase backend.
- Updated [projects/$projectId.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/projects/$projectId.tsx):
  - Added a dedicated "Directory Bindings" panel section displaying current path links.
- Created [directory-bindings.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/projects/$projectId/directory-bindings.tsx):
  - Built the interface components for viewing, adding, and deleting local directory bindings for a project.
- Added database migration:
  - `20260522100000_add_project_workspace_bindings.sql` containing the `project_workspace_bindings` table, schema, references, and RLS policies.
- Updated Coding Plan documentation:
  - Staged `CP-15-01-Project-Workspace-Bindings.md` outlining the detailed integration.

## Verification

- Ran unit tests in `apps/admin-web` via `npm run test` (including project workspace bindings tests, all passed).

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CP-15
change_type: feature
summary: Project Workspace Bindings
# --->8---

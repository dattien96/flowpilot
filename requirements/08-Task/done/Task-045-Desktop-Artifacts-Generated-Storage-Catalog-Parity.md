# Task-045: Desktop Artifacts Generated / Storage / Catalog Parity

## Metadata

- Document ID: `Task-045`
- Title: `Desktop Artifacts Generated / Storage / Catalog Parity`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [CP-06-01: Artifacts Sync](../../07-Coding-Plan/done/CP-06-01-Artifacts-Sync.md)
- Child Documents: `none`
- Related Documents: [SD-08: Artifact Management](../../06-System-Tech-Design/SD-08-Artifact-Management.md), [SS-07: Workflow Artifact](../../05-System-Specs/SS-07-Workflow-Artifact.md), [R3-Phase1-Review-Capture-Issues](../../10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md), [R3-Phase1-Route-Mapping](../../10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Route-Mapping.md)
- Replaces: `none`
- Tags: `artifacts, desktop, settings, generated, storage, catalog, R3-phase1`

## AI Quick View

### Summary

- Desktop `ArtifactsSettings.tsx` was missing three key concepts from the web `/artifacts?tab=generated` page: Local/Remote sub-tabs, a Sync artifacts button, and collapsible grouping by project → provider → workflow run.
- The Storage tab was showing a raw driver form instead of a two-column project-list / storage-config layout that conditionally shows Supabase info or Google Drive driver fields.
- The Catalog tab was missing the create-in-new-page pattern consistent with other settings tabs; the old inline Create button jumped directly into an edit state.
- This task rewrites `ArtifactsSettings.tsx` to fix all three tabs and appends matching CSS to `styles.css`.

### Current Ask

- All three artifact tab issues from `R3-Phase1-Review-Capture-Issues.md` item 7 are addressed in this task.

### Key Decisions

- `T-1` Generated tab: classify `LocalRunnerArtifact` items and `ArtifactRun` rows with `syncStatus === "local_only"` as the local section; remaining `ArtifactRun` rows (synced/syncing/failed) as the remote section.
- `T-2` Remote section groups by `project name → storageProvider.toUpperCase() → workflowRunId` using `useMemo`; each workflow-run row is collapsible via a `Set<string>` toggle state.
- `T-3` Sync button calls `POST /artifacts/${artifactId}/sync` on the runner directly via `RUNNER_URL` for each visible local artifact — no change to the `ArtifactRepository` interface required.
- `T-4` Storage tab restructured to `settings-two-column`: left = project list, right = conditional Supabase info panel or Google Drive driver form depending on `storagePreference`.
- `T-5` Catalog tab: `catalogView: "list" | "create"` state; Create button navigates to a separate create view with Back button; `saveDefinition` returns to list only when called from the create view.
- `T-6` Delete for catalog definitions is not implemented — no `deleteDefinition` method exists in `ArtifactCatalogRepository`; recorded as a follow-up.

### Constraints

- Do not modify `ArtifactRepository` interface or any shared core package — desktop-only change.
- Preserve all existing data-fetch logic (`listLocalArtifacts`, `listRuns`, `listDefinitions`, `getStorageDriver`).
- TypeScript must compile with zero errors.

### Open Questions

- None for this slice.

### Source Refs

- CP-06-01 §9 (UI Expectations), §10 (Acceptance Criteria)
- Web screenshot: `requirements/10-Refactor/Migrate-Web-To-Desktop/image-12.png`
- R3-Phase1-Review-Capture-Issues item 7 (7.1 Catalog, 7.2 Storage, 7.3 Generated)

## 1. Goal

Bring the desktop Artifacts page (`ArtifactsSettings.tsx`) into parity with the web `/artifacts` page for all three tabs:

- **Generated**: Local/Not Synced vs Remote/Sync sub-tabs, Sync artifacts button, collapsible grouping by project → provider → workflow run.
- **Storage**: Two-column layout (project list left, storage config right) with conditional Supabase info panel or Google Drive driver form.
- **Catalog**: Create-in-new-page pattern with Back navigation; inline edit stays on list view.

## 2. Parent Links

- coding plan: [CP-06-01: Artifacts Sync](../../07-Coding-Plan/done/CP-06-01-Artifacts-Sync.md)
- tech design: [SD-08: Artifact Management](../../06-System-Tech-Design/SD-08-Artifact-Management.md)
- system spec: [SS-07: Workflow Artifact](../../05-System-Specs/SS-07-Workflow-Artifact.md)
- specific upstream ids: CP-06-01 §9.2 (Global artifact page), CP-06-01 §10 (Acceptance Criteria)

## 3. Trigger

Issue 7 in `R3-Phase1-Review-Capture-Issues.md` identifies three sub-issues found during Phase 1 desktop testing:

- 7.1 Catalog: create form should be a separate page, no delete button.
- 7.2 Storage: completely wrong — should be two-column with project list and conditional provider config.
- 7.3 Generated: missing Local/Remote tab concept, missing Sync artifacts button, missing collapsible grouping.

## 4. Exact Change

- `T-1` Rewrite `apps/desktop-flowpilot/src/components/settings/ArtifactsSettings.tsx`:
  - Add `GeneratedSubTab`, `CatalogView` state types and corresponding state variables.
  - Add `syncBusy` state; `syncArtifacts()` calls `POST /artifacts/${id}/sync` via `RUNNER_URL`.
  - Add `remoteGroups` `useMemo` that groups remote `ArtifactRun`s by project → provider → workflowRunId.
  - Add `toggleRun(runId)` to expand/collapse individual workflow run rows.
  - Render Generated tab with `artifact-subtab` sub-tabs and collapsible remote groups.
  - Render Storage tab as `settings-two-column` with project list left and conditional provider config right.
  - Render Catalog tab with `catalogView` state for create-in-page navigation.
- `T-2` Append artifact browser CSS to `apps/desktop-flowpilot/src/styles.css`:
  - `.settings-list-empty`, `.artifact-subtabs`, `.artifact-subtab`, `.artifact-project-group`, `.artifact-group-header`, `.artifact-group-name`, `.artifact-group-count`, `.artifact-provider-group`, `.artifact-provider-header`, `.artifact-run-row`, `.artifact-run-header`, `.artifact-run-chevron`, `.artifact-run-label`, `.artifact-run-id`, `.artifact-run-count`, `.artifact-status-badge`, `.artifact-status-synced`, `.artifact-status-syncing`, `.artifact-status-failed`, `.artifact-run-items`, `.artifact-info-panel`.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/settings/ArtifactsSettings.tsx` (rewritten)
  - `apps/desktop-flowpilot/src/styles.css` (appended)
- modules:
  - Desktop Artifacts settings panel — all three tabs
- routes:
  - Runner: `POST /artifacts/{artifactId}/sync` (called directly via `RUNNER_URL` from sync button)

## 6. Acceptance Check

- Generated tab shows "Local / Not Synced (N)" and "Remote / Sync (N)" sub-tab buttons.
- Local sub-tab lists `LocalRunnerArtifact` rows + `ArtifactRun` rows with `syncStatus === "local_only"`.
- Remote sub-tab groups rows by project name → provider → workflowRunId with collapse/expand toggle.
- "Sync artifacts (N)" button is visible in the Generated tab header area; N reflects local artifact count.
- Storage tab shows project list on the left; selecting a project shows provider selector on the right.
- When Supabase is selected, the right side shows an info panel (no driver form).
- When Google Drive is selected, the right side shows the driver configuration form.
- Catalog tab "Create" button opens a new create view with a Back button.
- Saving a new definition from the create view returns to the list view.
- Editing an existing definition from the list view saves in place (no navigation).
- TypeScript compiles with zero errors.

## 7. Out of Scope

- `deleteDefinition` for catalog definitions — no backend method exists in `ArtifactCatalogRepository`; follow-up required.
- Artifact Storage tab: full Google Drive connection status card (image-11.png) — that full OAuth status panel is covered by `GoogleDriveSettings.tsx` (Task-043); the Storage tab shows the lightweight driver config only.
- Runner-side changes — no Go or shared-core modifications.
- Automated tests.

## 8. Completion Notes

- result: Implemented. `ArtifactsSettings.tsx` rewritten (~260 lines), CSS appended (~160 lines). TypeScript compiles with zero errors.
- follow-ups: (a) Add `deleteDefinition` to `ArtifactCatalogRepository` and Supabase implementation, then wire up delete button in Catalog tab. (b) Verify sync button behavior in a running desktop app — runner must be active and `/artifacts/{id}/sync` must return 200.
- upstream docs updated: none — CP-06-01 §9.2 and §10 already describe the required UI behavior; this task executes the desktop portion.

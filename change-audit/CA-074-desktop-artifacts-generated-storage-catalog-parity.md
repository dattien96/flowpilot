# CA-074: Desktop Artifacts Generated / Storage / Catalog Parity

## Scope

Desktop `ArtifactsSettings.tsx` — all three artifact tabs brought into parity with the web `/artifacts` page. CSS artifact browser classes appended to `styles.css`. No runner, shared-core, or Supabase changes.

## Completed

### Generated tab (7.3)
- Added `generatedSubTab: "local" | "remote"` state.
- **Local sub-tab** shows `LocalRunnerArtifact` rows (runner-local, not yet synced) plus `ArtifactRun` rows with `syncStatus === "local_only"`.
- **Remote sub-tab** shows `ArtifactRun` rows (synced/syncing/failed) grouped in a three-level collapsible hierarchy: project name → storage provider → workflowRunId. Each workflow-run row can be expanded/collapsed via a `Set<string>` toggle.
- Added `artifact-subtab` pill buttons above the list matching the web "Local / Not Synced (N) | Remote / Sync (N)" tabs.
- Added **Sync artifacts (N)** button that calls `POST /artifacts/${artifactId}/sync` on the runner directly via `RUNNER_URL` for each visible local artifact.

### Storage tab (7.2)
- Restructured to `settings-two-column` layout: project list on the left, storage config on the right.
- Clicking a project on the left updates the storage preference selector on the right.
- If `storagePreference === "supabase"`: right side shows an info panel ("Supabase Storage is active. No additional driver configuration required.").
- If `storagePreference === "google_drive"`: right side shows the existing driver fields (Remote Root, Folder Name, Enabled checkbox).
- Save button persists both the project's `artifactStoragePreference` and the driver config.

### Catalog tab (7.1)
- Added `catalogView: "list" | "create"` state.
- **Create button** navigates to a separate create view (within the same tab) with a Back button — consistent with the "create in new page" pattern used by Projects, Workflows, Teams tabs.
- Saving a new definition from the create view returns to the list.
- Editing an existing definition from the list view saves in-place without any navigation.
- Delete is **not implemented** — `ArtifactCatalogRepository` has no `deleteDefinition` method; logged as follow-up.

### CSS additions (`styles.css`)
New classes: `.settings-list-empty`, `.artifact-subtabs`, `.artifact-subtab`, `.artifact-project-group`, `.artifact-group-header`, `.artifact-group-name`, `.artifact-group-count`, `.artifact-provider-group`, `.artifact-provider-header`, `.artifact-run-row`, `.artifact-run-header`, `.artifact-run-chevron`, `.artifact-run-label`, `.artifact-run-id`, `.artifact-run-count`, `.artifact-status-badge`, `.artifact-status-{synced,syncing,failed}`, `.artifact-run-items`, `.artifact-info-panel`.

## Verification

- `npx tsc --noEmit` in `apps/desktop-flowpilot` → **zero TypeScript errors**.
- Manual UI testing not performed (desktop app not launched in this session). Follow-up: open the Artifacts tab in a running desktop instance with runner active and verify sync button, sub-tabs, and collapsible rows.

## Residual Notes

- **Delete artifact definitions**: `ArtifactCatalogRepository.deleteDefinition` does not exist. Adding it requires a Supabase repository change and interface update. Low priority; add as a follow-up task.
- **Sync button**: calls the runner endpoint directly via `RUNNER_URL`. If the runner is not running, the sync button will surface an error in the feedback banner — expected behavior.
- **Storage tab Google Drive status card** (image-11.png from review doc): the full OAuth account status panel (connected account, folder ID, last validated, etc.) is handled by `GoogleDriveSettings.tsx` (Task-043). The Storage tab shows only the lightweight driver config, which is appropriate for per-project storage preference selection.
- **Task document**: `requirements/08-Task/done/Task-045-Desktop-Artifacts-Generated-Storage-Catalog-Parity.md`

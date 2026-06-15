# Task-043: Desktop Google Drive Setup Tab

## Metadata

- Document ID: `Task-043`
- Title: `Desktop Google Drive Setup Tab`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [CP-30: Google Drive Account Connection And Artifact Folder Binding](../../07-Coding-Plan/done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)
- Child Documents: `none`
- Related Documents: [CP-29: MCP Proxy Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md), [Task-025: Drive MCP Auth Flow](./Task-025-Drive-MCP-Auth-Flow.md), [Task-023: Sync Artifact With Google](./Task-023-Sync-Artifact-With-Google.md), [R3-Phase1-Review-Capture-Issues](../../10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md)
- Replaces: `none`
- Tags: `google-drive, desktop, settings, mcp, oauth, R3-phase1`

## AI Quick View

### Summary

- The desktop app's Google Drive tab (`SettingsShell` section `google-drive`) was rendering `<McpSettings mode="google-drive" />`, a minimal placeholder with no OAuth credential forms, no account connection flow, and no setup wizard.
- The web app's `/settings/google-drive-setup` has a comprehensive 7-step wizard covering Google Cloud project setup, OAuth consent, API enablement, OAuth client credentials, Picker API key, Google account connection, and proxy MCP provider status.
- This task creates a new `GoogleDriveSettings.tsx` component that mirrors the web app's setup page, calling the local runner directly at `RUNNER_URL` (http://127.0.0.1:4317) instead of through Next.js API proxies.
- `SettingsShell.tsx` is updated to render `GoogleDriveSettings` for the `google-drive` section.

### Current Ask

- Implement the full Google Drive setup wizard in the desktop app to match the web app's `/settings/google-drive-setup` page.

### Key Decisions

- `T-1` Call the runner directly at `RUNNER_URL` for all Google Drive config APIs — no admin-web proxy layer in the desktop.
- `T-2` Inline all Google Drive type definitions (`GoogleDriveWorkspaceConfigResponse`, `GoogleDriveMcpStatus`, `GoogleDriveAccountStatus`, etc.) in `GoogleDriveSettings.tsx` since they are not exported from `@flowpilot/client-core`.
- `T-3` Use existing desktop CSS classes (`settings-panel`, `settings-subpanel`, `settings-field`, `primary-btn`, `secondary-btn`, etc.) and add a minimal new CSS section in `styles.css` for stepper, badges, and code-row display.
- `T-4` Account OAuth popup uses `window.open(connectUrl, "_blank")` matching the web app approach.

### Constraints

- Do not store Google refresh tokens outside the runner secret store.
- Preserve all existing desktop components — only add new file and update the import in `SettingsShell.tsx`.
- Runner endpoint paths must match what the runner actually serves (confirmed from web API proxy routes).

### Open Questions

- `window.open` for OAuth in Electron may need `shell.openExternal` IPC in future if Electron blocks popup windows — acceptable as follow-up.

### Source Refs

- CP-30 `P-9`: Update admin-web UI to show account connection and provider MCP config status.
- Web app source: `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- Runner endpoints confirmed via: `apps/admin-web/src/app/api/runtime/google-drive-config/_shared.ts` and sibling route files.

## 1. Goal

Replace the placeholder `<McpSettings mode="google-drive" />` in the desktop app's Google Drive settings tab with a full 7-step setup wizard matching the web app's `/settings/google-drive-setup` page.

The wizard covers:
1. Google Cloud project creation guide
2. OAuth consent screen configuration guide
3. API enablement guide
4. Web OAuth client credentials (Client ID, Client Secret, Redirect URI)
5. Google Picker API key
6. Google account connection and MCP account selection
7. Proxy MCP provider config status

## 2. Parent Links

- coding plan: [CP-30: Google Drive Account Connection And Artifact Folder Binding](../../07-Coding-Plan/done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)
- tech design: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- system spec: [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md)
- specific upstream ids: CP-30 `P-9`

## 3. Trigger

Issue 6 in `R3-Phase1-Review-Capture-Issues.md` identifies the Google Drive tab in the desktop app as completely missing the setup wizard that exists in the web app (`/settings/google-drive-setup`). The desktop tab was showing a minimal MCP integration panel with no credential forms, no OAuth connect flow, and no step-by-step guidance.

## 4. Exact Change

- `T-1` Create `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx` — full 7-step Google Drive setup wizard that calls the runner directly.
- `T-2` Update `apps/desktop-flowpilot/src/components/SettingsShell.tsx` — import `GoogleDriveSettings` and render it for the `google-drive` section instead of `<McpSettings mode="google-drive" />`.
- `T-3` Append Google Drive CSS classes to `apps/desktop-flowpilot/src/styles.css` — `.gdrive-badge`, `.gdrive-stepper`, `.gdrive-step`, `.gdrive-step-circle`, `.gdrive-step-header`, `.gdrive-guide`, `.gdrive-code-row`, `.gdrive-secret-row`, `.gdrive-status-grid`, `.gdrive-status-row`, etc.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx` (new)
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules:
  - desktop settings panel
  - Google Drive runner config APIs (read-only — no runner changes)
- routes:
  - Runner: `GET/PUT/DELETE /google-drive-config`, `POST /google-drive-config/validate`, `POST /google-drive-config/accounts/connect-sessions`, `DELETE /google-drive-config/accounts/{accountId}`

## 6. Acceptance Check

- Opening Google Drive tab in the desktop app shows the 7-step setup wizard (not the old MCP integration panel).
- Status overview shows artifact sync status, MCP status, and runner reachability.
- Step 4 form saves OAuth client credentials to the runner.
- Step 5 form saves Picker API key to the runner.
- Step 6 loads connected accounts from the runner, allows selecting the proxy MCP account, and opens the OAuth popup for new account connection.
- Step 7 shows provider MCP config status from the runner.
- Validate setup and Reset config buttons work.
- TypeScript compiles with zero errors.

## 7. Out of Scope

- Runner-side changes — all runner endpoints already exist from CP-30 and Task-025 work.
- Project-level artifact folder binding UI (separate per-project settings page).
- Electron `shell.openExternal` IPC for OAuth popup — using `window.open` same as web app.
- Automated tests for the new component.

## 8. Completion Notes

- result: Implemented. `GoogleDriveSettings.tsx` created (≈ 450 lines), `SettingsShell.tsx` updated, CSS appended. TypeScript compiles with zero errors.
- follow-ups: Verify in a running desktop app that the runner APIs respond correctly and the OAuth popup opens the connect URL. Electron popup handling may need `shell.openExternal` if popup is blocked.
- upstream docs updated: none — CP-30 `P-9` already describes the UI requirement; this task executes it.

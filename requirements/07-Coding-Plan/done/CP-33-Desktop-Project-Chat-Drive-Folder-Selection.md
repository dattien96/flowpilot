# CP-33: Desktop Project Chat Drive Folder Selection

## Metadata

- Document ID: `CP-33`
- Title: `Desktop Project Chat Drive Folder Selection`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `TBD`
- Related Documents: [CP-30: Google Drive Account Connection And Artifact Folder Binding](../done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md), [Task-043: Desktop Google Drive Setup Tab](../../08-Task/done/Task-043-Desktop-Google-Drive-Setup-Tab.md), [Task-045: Desktop Artifacts Generated / Storage / Catalog Parity](../../08-Task/done/Task-045-Desktop-Artifacts-Generated-Storage-Catalog-Parity.md), [Task-069: Cross-PC Sync for Non-Supabase Users](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [Task-073: Cross-PC Non-Supabase Chat Sync DOD](../../08-Task/todo/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md), [Task-074: Cross-PC Non-Supabase Chat Sync Test Signatures](../../08-Task/todo/Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md), [Task-075: Cross-Account And Cross-PC Chat E2E Test Guide](../../08-Task/todo/Task-075-Cross-Account-And-Cross-PC-Chat-E2E-Test-Guide.md)
- Replaces: `None`
- Tags: `desktop, project-detail, google-drive, chat-sync, folder-binding, local-runner`

## AI Quick View

### Summary

- Desktop chat sync already has sync, restore, and remote-list actions, but it currently resolves its Drive root through the artifact Google Drive connection.
- Artifact storage is pending for this slice, so chat sync needs its own project-level Drive target folder binding.
- The new control belongs on Project detail, not the global Artifacts settings page, because the target is per project and affects chat history sync for that project only.
- The implementation mirrors the web artifact storage folder-picker flow, but only changes the chat sync target folder under the already linked Google Drive account.

### Current Ask

- Plan a scoped implementation so a user can re-select the exact Google Drive folder used for desktop chat sync without regressing SD-14 cross-account resume, cross-PC sync, restore, or artifact behavior.
- Provide a detailed implementation and review checklist that can be used later to request code and test verification.

### Key Decisions

- `P-1` Add a dedicated `projectId -> googleDriveAccountId -> chatFolderId` chat sync binding instead of reusing or mutating the artifact storage binding.
- `P-2` Place the folder selection UI inside desktop Project detail as a new collapsible panel.
- `P-3` Reuse the existing Google Drive account connection and Picker/OAuth capability, but persist the selected folder to chat sync state only.
- `P-4` Update chat sync, remote list, and restore to resolve the chat sync folder first, with a compatibility fallback to the legacy artifact binding only when no chat binding exists.
- `P-5` Keep this task limited to changing the target folder; changing the connected Google account, artifact provider, artifact folder, background sync policy, and deletion propagation remain out of scope.

### Constraints

- Do not route this through the artifact feature as the source of truth; artifact sync can remain pending.
- Do not store Google refresh tokens in Supabase or desktop renderer state.
- Do not change the SD-14 provider-session file model, manifest format, hash validation, or cross-account relocation behavior unless a test proves it is required.
- Before implementation edits to Go or TypeScript symbols, run GitNexus impact analysis for every modified function, class, or method.
- Existing synced chats must remain discoverable from the old artifact-backed Drive folder until the user selects a new chat folder.

### Open Questions

- Should the compatibility fallback to the artifact binding be permanent or removed after one migration release?
- Should selecting a new chat folder optionally copy the existing `chat-sessions/_index/sessions.ndjson` from the old folder, or should the UI state clearly show that the new folder starts with its own remote history?

### Source Refs

- `SD-14` D-1 through D-4, Section 6.3 Optional Cloud Sync Contracts, Section 7 Cross-Account and Cross-PC flows.
- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
- `apps/desktop-flowpilot/src/components/settings/ArtifactsSettings.tsx`
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/admin-web/src/presentation/components/artifacts/artifact-cloud-storage-panel.tsx`

## 1. Goal

Let the desktop user choose or change the Google Drive folder used for chat sync for a specific project.

The target behavior is:

- Project detail shows the current chat sync Drive folder for the selected project.
- The user can select another folder in the same connected Google Drive account.
- Future chat sync, remote chat listing, and restore for that project use the selected chat folder.
- Existing artifact settings are not changed.
- Existing cross-account chat resume remains unchanged because local provider-session relocation is separate from Drive folder selection.

## 2. Input Documents

- [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [CP-30: Google Drive Account Connection And Artifact Folder Binding](../done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)
- [Task-043: Desktop Google Drive Setup Tab](../../08-Task/done/Task-043-Desktop-Google-Drive-Setup-Tab.md)
- [Task-045: Desktop Artifacts Generated / Storage / Catalog Parity](../../08-Task/done/Task-045-Desktop-Artifacts-Generated-Storage-Catalog-Parity.md)
- [Task-069: Cross-PC Sync for Non-Supabase Users](../../08-Task/todo/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- [Task-074: Cross-PC Non-Supabase Chat Sync Test Signatures](../../08-Task/todo/Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md)

## 3. Implementation Strategy

- overall approach:
  - Treat chat sync folder selection as a project detail setting backed by runner-local Google Drive state.
  - Add chat sync folder status and picker-session endpoints to the local runner.
  - Update SD-14 chat sync resolution so `syncChatRunToDrive`, `listRemoteChatSessions`, and `restoreChatRunFromDrive` use the chat sync binding.
  - Mirror the web artifact folder picker interaction pattern in desktop, but label and persist it as chat sync only.

- sequencing logic:
  - First add runner state and compatibility readers.
  - Then add runner HTTP contracts for status, connect/picker session, and folder save.
  - Then update chat sync root resolution.
  - Then add desktop Project detail UI and client types.
  - Finally update tests and manual verification docs.

- dependencies:
  - Existing Google Drive account connection from CP-30 and Task-043.
  - Existing Google Picker support in `artifact_google_drive_connection.go`.
  - Existing chat sync API methods in `RunnerClient`.

## 4. Work Breakdown

- `P-1` Define runner-local chat sync folder binding state.
  - Add a `chatSyncGoogleDriveConnections` or equivalent map keyed by `projectId` in the existing Google Drive state file.
  - Store `projectId`, `accountId`, `accountEmail`, `folderId`, `folderName`, `status`, `connectedAt`, `updatedAt`, `lastValidatedAt`, and `lastError`.
  - Keep credential material in the existing account secret store only.
  - Add compatibility lookup: if no chat sync binding exists, read the existing artifact Google Drive connection for that project as a legacy source.

- `P-2` Add runner APIs for chat sync folder status and folder selection.
  - `GET /client/projects/{projectId}/chat-sync/google-drive/status`
    - Returns the chat binding, effective source (`chat_sync` or `artifact_legacy`), connected account metadata, and readiness.
  - `POST /client/projects/{projectId}/chat-sync/google-drive/connect-session`
    - Starts a folder-picker session using the already selected or requested Google Drive account.
  - `POST /client/chat-sync/google-drive/folder-selection`
    - Saves the selected folder into chat sync binding state only.
  - Reuse Picker token serving where practical, but keep session purpose distinct so artifact folder selection cannot consume a chat sync session by mistake.

- `P-3` Update chat sync root resolution.
  - Replace `ensureChatSessionDriveRoot(projectID)` internals with a resolver that prefers the chat sync binding.
  - Preserve current artifact-backed behavior only as fallback.
  - Make errors distinguish:
    - no Google Drive account connected
    - no chat sync folder selected
    - selected folder no longer readable
    - missing required Drive scope
  - Ensure `ChatSessionSyncRequest.googleDriveFolderId` is either removed from runtime use or validated against the selected project binding before use.

- `P-4` Add desktop Project detail UI.
  - Extend `ProjectsSettings.tsx` with a new `ProjectPanelKey` such as `chatSync`.
  - Add a collapsible panel named `Chat Sync`.
  - Show selected account email, folder name, folder id, status, last validated time, and effective source.
  - Add actions:
    - Refresh status.
    - Select Drive Folder.
    - Open Google Drive setup when no account is connected.
  - The picker action opens the runner-hosted folder picker just like the web artifact storage flow, then polls status until connected.
  - Do not place this control in the Artifacts tab; leave `ArtifactsSettings.tsx` artifact-focused.

- `P-5` Update desktop client and store usage.
  - Add typed status/session/folder-selection contracts in `apps/desktop-flowpilot/src/types/contract.ts` or a local Project settings type if the runner client does not need global exposure.
  - Add small `RUNNER_URL` fetch helpers in Project settings for the new endpoints, following `GoogleDriveSettings.tsx`.
  - Update `syncHistoryRun` only if needed to pass the selected project id, not raw folder ids.
  - Keep `Navigator` sync buttons unchanged from the user's perspective; they should automatically use the selected project chat folder.

- `P-6` Preserve remote history and restore behavior.
  - `listRemoteChatSessions(projectId)` reads from the selected chat sync folder.
  - If no chat sync binding exists, it reads from the legacy artifact folder so current users do not lose visibility.
  - Restore uses the same effective folder as listing.
  - If a user changes folders, the new folder becomes the source of truth for future list/restore. Old-folder migration or copy is not automatic unless the follow-up question is approved.

- `P-7` Add regression tests.
  - Runner unit tests in `chat_session_sync_test.go`:
    - resolver prefers chat sync binding over artifact binding
    - resolver falls back to artifact binding when chat binding is absent
    - sync writes `chat-sessions/_index/sessions.ndjson` under the selected chat folder
    - list/restore read from the selected chat folder
    - changing the chat folder does not mutate artifact connection state
  - Runner handler tests:
    - status returns effective source and folder metadata
    - folder selection rejects missing project, missing account, non-folder item, and wrong-purpose session
  - Desktop tests:
    - Project detail renders chat sync panel
    - no account state shows setup action
    - connected state shows folder and allows selection
    - sync button path still calls `syncChatRun` without passing stale folder ids.

- `P-8` Manual verification pass.
  - Connect Google Drive account in desktop Google Drive settings.
  - Open Project detail and select folder `A` for Chat Sync.
  - Start or open a chat and sync it; verify Drive writes under folder `A/chat-sessions/...`.
  - Re-select folder `B`; sync another chat; verify writes under folder `B/chat-sessions/...`.
  - Confirm artifact storage preference and artifact folder settings did not change.
  - Confirm SD-14 cross-account resume still works locally after switching Codex accounts.
  - Confirm restore on another machine/account uses the folder currently selected for that project.

### 4.1 Detailed Execution Checklist

Use this checklist as the implementation source of truth. Every checked item should be backed by code, tests, or a clear verification note.

- [ ] `C-01` Run GitNexus impact analysis before editing each modified symbol.
  - [ ] `C-01.1` Run impact for chat sync resolver functions before changing `chat_session_sync.go`.
  - [ ] `C-01.2` Run impact for Google Drive state/session functions before changing `artifact_google_drive_connection.go` or `google_drive_config.go`.
  - [ ] `C-01.3` Run impact for desktop Project settings component functions before changing `ProjectsSettings.tsx`.
  - [ ] `C-01.4` Report any HIGH or CRITICAL blast radius before editing.

- [ ] `C-02` Add runner-local chat sync binding model.
  - [ ] `C-02.1` Define chat sync binding record fields: `projectId`, `accountId`, `accountEmail`, `folderId`, `folderName`, `status`, `connectedAt`, `updatedAt`, `lastValidatedAt`, `lastError`.
  - [ ] `C-02.2` Persist chat sync bindings in runner-local Google Drive state, not Supabase.
  - [ ] `C-02.3` Keep refresh/access tokens in the existing secret store only.
  - [ ] `C-02.4` Add backward-compatible JSON loading when the new field is missing.
  - [ ] `C-02.5` Add state save logic that does not rewrite or delete existing artifact connection records.

- [ ] `C-03` Add effective chat Drive root resolver.
  - [ ] `C-03.1` Prefer dedicated chat sync binding when present and connected.
  - [ ] `C-03.2` Fall back to artifact Google Drive connection only when no chat binding exists.
  - [ ] `C-03.3` Return effective source as `chat_sync` or `artifact_legacy`.
  - [ ] `C-03.4` Refresh the access token through the binding account.
  - [ ] `C-03.5` Validate folder id is non-empty before returning root.
  - [ ] `C-03.6` Preserve typed errors for no account, no folder, unreadable folder, missing scope, and token refresh failure.

- [ ] `C-04` Wire resolver into chat sync operations.
  - [ ] `C-04.1` Update `syncChatRunToDrive` to write under selected chat folder.
  - [ ] `C-04.2` Update `listRemoteChatSessions` to read selected chat folder index.
  - [ ] `C-04.3` Update `restoreChatRunFromDrive` to read selected chat folder manifest/provider file.
  - [ ] `C-04.4` Ensure remote paths remain `chat-sessions/runs/...` relative to the selected folder.
  - [ ] `C-04.5` Ensure manifest schema, SHA-256 validation, provider file restore, and run id collision handling are unchanged.
  - [ ] `C-04.6` Ensure `ChatSessionSyncRequest.googleDriveFolderId` cannot silently override the project binding.

- [ ] `C-05` Add runner HTTP endpoints.
  - [ ] `C-05.1` Register `GET /client/projects/{projectId}/chat-sync/google-drive/status`.
  - [ ] `C-05.2` Register `POST /client/projects/{projectId}/chat-sync/google-drive/connect-session`.
  - [ ] `C-05.3` Register `POST /client/chat-sync/google-drive/folder-selection`.
  - [ ] `C-05.4` Return typed JSON errors consistent with existing runner handlers.
  - [ ] `C-05.5` Include current binding, effective source, account metadata, readiness, and last error in status response.
  - [ ] `C-05.6` Prevent artifact folder-selection sessions from saving chat bindings, and prevent chat sessions from saving artifact bindings.

- [ ] `C-06` Mirror folder picker flow for chat sync.
  - [ ] `C-06.1` Reuse existing Google Picker token flow where possible.
  - [ ] `C-06.2` Label picker page/action for chat sync, not artifacts.
  - [ ] `C-06.3` Save selected folder id/name to chat sync binding only.
  - [ ] `C-06.4` Poll status after the picker closes or returns.
  - [ ] `C-06.5` Show reconnect/setup guidance if account scopes or credentials are not ready.

- [ ] `C-07` Add Project detail UI.
  - [ ] `C-07.1` Add `chatSync` to `ProjectPanelKey` and default collapsed panel state.
  - [ ] `C-07.2` Render `Chat Sync` panel under selected project detail.
  - [ ] `C-07.3` Show account email, folder name, folder id, status, effective source, last validated time, and last error.
  - [ ] `C-07.4` Add `Refresh` action.
  - [ ] `C-07.5` Add `Select Drive Folder` action.
  - [ ] `C-07.6` Add `Open Google Drive Setup` action when no usable account is connected.
  - [ ] `C-07.7` Keep the Artifacts tab unchanged for this slice.
  - [ ] `C-07.8` Ensure the panel layout fits desktop widths without overlapping text or controls.

- [ ] `C-08` Update desktop contracts and store usage.
  - [ ] `C-08.1` Add TypeScript types for chat sync status, connect session, and folder selection responses.
  - [ ] `C-08.2` Use `RUNNER_URL` direct fetch helpers or `RunnerClient` methods consistently with nearby desktop settings code.
  - [ ] `C-08.3` Ensure `Navigator` single-sync and sync-all buttons need no new user action once the project folder is selected.
  - [ ] `C-08.4` Ensure `syncHistoryRun` passes project context only, not raw stale folder ids.
  - [ ] `C-08.5` Reload remote chat sessions after successful folder selection.

- [ ] `C-09` Preserve non-regression constraints.
  - [ ] `C-09.1` Artifact storage preference is not changed by chat folder selection.
  - [ ] `C-09.2` Artifact Google Drive folder binding is not changed by chat folder selection.
  - [ ] `C-09.3` Google Drive account setup remains in Google Drive settings.
  - [ ] `C-09.4` Local cross-account resume logic remains unchanged.
  - [ ] `C-09.5` Delete chat history behavior remains unchanged.

- [ ] `C-10` Update task docs after implementation.
  - [ ] `C-10.1` Create child task document under `requirements/08-Task` if implementation starts from this CP.
  - [ ] `C-10.2` Link the child task in CP-33 `Child Documents`.
  - [ ] `C-10.3` Update Task-073, Task-074, or Task-075 only if their checklist/test-guide expectations change.
  - [ ] `C-10.4` Add a change-audit note if the repo workflow requires one for desktop/runner behavior changes.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/chat_session_sync.go`
  - `apps/local-runner/internal/runner/chat_session_sync_test.go`
  - `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
  - `apps/local-runner/internal/runner/google_drive_config.go`
  - `apps/local-runner/internal/runner/google_drive_config_test.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/styles.css`
- modules:
  - local runner Google Drive state and picker sessions
  - local runner chat sync
  - desktop Project detail settings
  - desktop chat history sync commands
- database:
  - none expected; this is runner-local state.
- external systems:
  - Google Drive API
  - Google Picker

## 6. Data or Migration Steps

- schema:
  - No Supabase migration.
  - Add runner-local JSON state fields for chat sync folder bindings.

- data backfill:
  - Do not rewrite existing state at startup.
  - Runtime resolver treats an existing artifact Google Drive project connection as `artifact_legacy` until the user selects a chat sync folder.
  - First successful chat sync folder selection writes a dedicated chat binding for that project.

- config updates:
  - No new OAuth scopes expected if current account connection already supports folder picker and file write.
  - If scope validation fails, Project detail must direct the user to reconnect in Google Drive settings.

## 7. Validation Plan

- tests to add:
  - Go unit and handler tests listed in `P-7`.
  - Desktop Project settings tests for the new panel and status states.
  - Store/client test to ensure sync uses project-level resolution rather than stale folder request payloads.

- manual checks:
  - Folder A to folder B re-selection.
  - Sync all chats button in Navigator.
  - Single chat sync button in Navigator.
  - Remote chat list reload after folder selection.
  - Restore from the selected folder on a clean runner home.

- failure cases:
  - No Google Drive account connected.
  - Account connected but no chat sync folder selected.
  - Folder removed or access revoked in Drive.
  - User selects a non-folder item.
  - Old artifact-backed folder exists but new chat folder has no index yet.
  - Cross-account Codex resume after a folder change.

### 7.1 Code And Test Review Checklist

Use this checklist when requesting a code and test review for the implementation.

- [ ] `RV-01` Impact and scope review.
  - [ ] `RV-01.1` Confirm GitNexus impact was run for every changed symbol.
  - [ ] `RV-01.2` Confirm no HIGH or CRITICAL impact was ignored.
  - [ ] `RV-01.3` Confirm modified files match the CP-33 touched-area list or have a documented reason.
  - [ ] `RV-01.4` Confirm no unrelated refactor or artifact-sync expansion was introduced.

- [ ] `RV-02` Runner state review.
  - [ ] `RV-02.1` Chat sync binding persists separately from artifact binding.
  - [ ] `RV-02.2` Existing Google Drive state loads when chat binding field is absent.
  - [ ] `RV-02.3` Saving chat sync binding cannot erase artifact connections, accounts, sessions, or secrets.
  - [ ] `RV-02.4` Token material never appears in API responses, desktop state, logs, or Supabase rows.

- [ ] `RV-03` Chat sync behavior review.
  - [ ] `RV-03.1` Sync writes to selected chat folder.
  - [ ] `RV-03.2` List reads from selected chat folder.
  - [ ] `RV-03.3` Restore reads from selected chat folder.
  - [ ] `RV-03.4` Legacy artifact folder fallback works before a dedicated chat folder is selected.
  - [ ] `RV-03.5` Changing chat folder does not make local history unusable.
  - [ ] `RV-03.6` Manifest integrity and provider file hash checks remain enforced.

- [ ] `RV-04` Endpoint review.
  - [ ] `RV-04.1` Status endpoint returns enough state for UI without leaking secrets.
  - [ ] `RV-04.2` Connect-session endpoint validates project and account.
  - [ ] `RV-04.3` Folder-selection endpoint rejects invalid session purpose.
  - [ ] `RV-04.4` Folder-selection endpoint rejects empty folder id and non-folder Drive items.
  - [ ] `RV-04.5` All new endpoint errors are typed and user-actionable.

- [ ] `RV-05` Desktop UI review.
  - [ ] `RV-05.1` Project detail shows a `Chat Sync` panel only for the selected project.
  - [ ] `RV-05.2` Empty, loading, connected, legacy fallback, failed, and reconnect-required states render clearly.
  - [ ] `RV-05.3` Select-folder flow opens the runner picker and refreshes status after completion.
  - [ ] `RV-05.4` Sync buttons in Navigator still work without new visible inputs.
  - [ ] `RV-05.5` Text and controls do not overlap on narrow desktop windows.

- [ ] `RV-06` Test coverage review.
  - [ ] `RV-06.1` Go resolver tests cover chat binding preferred over artifact fallback.
  - [ ] `RV-06.2` Go resolver tests cover artifact fallback when chat binding is missing.
  - [ ] `RV-06.3` Go sync/list/restore tests verify selected folder paths.
  - [ ] `RV-06.4` Go tests prove artifact connection state is unchanged by chat folder selection.
  - [ ] `RV-06.5` Handler tests cover status, connect-session, folder-selection success and errors.
  - [ ] `RV-06.6` Desktop tests cover Project detail panel states and folder action.
  - [ ] `RV-06.7` Store/client tests cover sync without stale raw folder id payloads.

- [ ] `RV-07` Manual verification review.
  - [ ] `RV-07.1` Folder `A` sync creates `chat-sessions/runs/...` and `_index/sessions.ndjson`.
  - [ ] `RV-07.2` Folder `B` re-selection sends later syncs to folder `B`.
  - [ ] `RV-07.3` Artifact folder and provider preference are unchanged after `A -> B`.
  - [ ] `RV-07.4` Remote list shows the selected folder's remote chats.
  - [ ] `RV-07.5` Restore from selected folder works on a clean local runner/account home.
  - [ ] `RV-07.6` Cross-account Codex resume still works after folder selection.
  - [ ] `RV-07.7` Failure states are visible for revoked Drive access and missing folder.

## 8. Rollout and Fallback

- rollout order:
  - Ship runner resolver fallback first.
  - Ship runner folder-selection APIs.
  - Ship desktop Project detail UI.
  - Ship tests and update manual E2E checklist.

- fallback path:
  - If new chat binding is absent or invalid, show a clear Project detail error.
  - If no chat binding exists, continue using the old artifact-backed folder for compatibility.
  - If a newly selected folder causes issues, the user can select the previous folder again.

- monitoring:
  - Use chat row `syncStatus`, `unavailableReason`, and runner typed errors to surface failures.
  - Inspect Drive for `chat-sessions/runs/.../manifest.json` and `chat-sessions/_index/sessions.ndjson` under the selected folder.

## 9. Risks

- `R-1` Reusing artifact picker code may accidentally update artifact connection state. Mitigation: add explicit session purpose and tests that artifact state is unchanged.
- `R-2` Changing the folder can make older remote chats invisible if they remain in the previous folder. Mitigation: retain fallback before first dedicated chat folder and clearly document that folder migration is separate.
- `R-3` Desktop Project detail can become too dense. Mitigation: use a collapsed `Chat Sync` panel with compact status rows and one primary folder action.
- `R-4` Drive scope differences may make folder selection succeed but sync fail. Mitigation: validate account capability in status and before saving the binding.
- `R-5` Cross-account local resume could be mistaken for Drive sync. Mitigation: keep SD-14 local relocation code untouched and test both paths separately.

## 10. Definition of Done

- Project detail contains a `Chat Sync` panel for the selected project.
- The panel shows the effective Google Drive account and folder used for chat sync.
- The user can select a new Drive folder for chat sync using the current linked Google account.
- `syncChatRunToDrive`, `listRemoteChatSessions`, and `restoreChatRunFromDrive` use the selected chat sync folder.
- Changing the chat sync folder does not change artifact provider preference or artifact folder binding.
- Existing users with only artifact-backed chat sync continue working until they select a chat sync folder.
- Automated runner and desktop tests cover the new resolver, endpoints, and Project detail panel.
- Manual verification confirms sync to folder A, reselect to folder B, restore from folder B, and SD-14 cross-account resume with no regression.

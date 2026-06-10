# CP-30: Google Drive Account Connection And Artifact Folder Binding

## Metadata

- Document ID: `CP-30`
- Title: `Google Drive Account Connection And Artifact Folder Binding`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-09`
- Last Updated: `2026-06-10`
- Parent Documents: [CP-29: FlowPilot Proxy MCP Server For Google Drive](./CP-29-MCP-Proxy-Google-Drive.md), [CP-06-01: Artifacts Sync](../done/CP-06-01-Artifacts-Sync.md), [CP-05-03: Google Drive MCP Current Implementation Notes](../done/CP-05-03-Driver-Mcp.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-08: Artifact Management](../../06-System-Tech-Design/SD-08-Artifact-Management.md), [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md), [SS-07: Workflow Artifact](../../05-System-Specs/SS-07-Workflow-Artifact.md)
- Child Documents: `TBD`
- Related Documents: [CP-27: Google Cloud Setting Manually](../done/CP-27-Google-Cloud-Setting-Manually.md), [CP-28: Google Cloud Config With UI Auto](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md), [Task-025: Drive MCP Auth Flow](../../08-Task/done/Task-025-Drive-MCP-Auth-Flow.md)
- Replaces: `None`
- Tags: `google-drive, mcp, artifact-sync, oauth, local-runner, admin-web, account-connection`

## AI Quick View

### Summary

- Current Google Drive proxy MCP reuses artifact-sync project connections as its auth source, which breaks when the runner has more than one connected FlowPilot project.
- The root issue is a model coupling: artifact sync needs `projectId -> folderId`, while MCP needs `accountId -> broad Drive token`.
- CP-30 introduces an account-level Google Drive connection model and keeps artifact folder selection as a per-project binding under that account.
- MCP read tools must use the selected account token and must not be limited by the artifact folder.
- Artifact sync must use the same account token plus the project folder binding for upload/open/sync behavior.

### Current Ask

- Define a detailed implementation plan to separate Google Drive account login from artifact folder binding across model, local runner, and admin UI.
- Preserve artifact sync behavior while enabling MCP to read the connected account's broader Drive data.
- Include a concrete Definition of Done and manual Test Guide.

### Key Decisions

- `P-0` Update or explicitly approve upstream specs/design before implementation because CP-30 changes Google Drive MCP from folder-scoped access to account-scoped read access.
- `P-1` Add a runner-local Google Drive account connection store keyed by `accountId`, not by FlowPilot `projectId`.
- `P-2` Store refresh tokens only in the runner secret store; store account metadata and project folder bindings in local runner state.
- `P-3` Model artifact sync as `projectId -> accountId -> folderId`, not as `projectId -> refreshToken -> folderId`.
- `P-4` Make project/workflow proxy MCP select a Google Drive `accountId` explicitly through runtime context; allow a runner default only for settings-page diagnostics.
- `P-5` MCP read tools use the account token without applying the artifact folder as a read filter.
- `P-6` Artifact sync write/open flows use account token plus the project folder binding.
- `P-7` Broad MCP reads require an OAuth flow with scopes beyond the current artifact-only `drive.file` grant.
- `P-8` Migrate existing project-scoped artifact connections into account records plus project bindings without losing selected folders.

### Constraints

- Supabase must not store Google refresh tokens, access tokens, Desktop OAuth JSON contents, or local token-file paths.
- Existing artifact sync behavior must keep working for projects that already selected a Google Drive folder.
- The current `SS-02` wording says Google Drive MCP can read/write files in a selected folder or shared drive; broad account-level MCP read must be reflected in upstream spec/design before approval.
- Implementation must not start until upstream documents either approve account-scoped MCP reads or explicitly constrain this CP back to folder-scoped MCP.
- The local runner remains the owner of Google OAuth, token refresh, folder validation, Drive API calls, and proxy MCP execution.
- The first implementation should be local-runner state only; cross-machine Google account sync is out of scope.

### Open Questions

- None for implementation readiness.
- Deferred product question: remove or hide the legacy raw `@piotr-agier/google-drive-mcp` setup only after account-backed proxy MCP is proven stable.

### Source Refs

- CP-29 `P-8`: proxy MCP currently planned to reuse artifact-sync OAuth/token infrastructure.
- CP-06-01 Section 6: artifact sync currently asks the user to OAuth and select one destination folder per project.
- CP-05-03 Sections 3-6: current raw Google Drive MCP uses separate Desktop OAuth JSON and token files.
- SD-11 Section 6: provider secrets belong in the local runner, not normal project config.
- SD-08 Section 6: artifact sync is separate from workflow execution.
- SS-02 Sections 3-7: MCP must create a real usable connection and credentials must stay local to the runner.
- SS-07 Sections 5-7: remote artifact storage is secondary to local-first execution.

## 1. Goal

Refactor Google Drive auth and storage state so FlowPilot can support both of these workflows without conflict:

- User connects one Google Drive account once and MCP can read Drive data available to that account, subject to granted OAuth scopes.
- Each FlowPilot project can bind artifact sync to a specific folder inside a connected Google Drive account.

The target design separates identity from folder routing:

```text
Google Drive account connection
  accountId
  accountEmail
  grantedScopes
  refreshToken stored in runner secret store

Project artifact folder binding
  projectId
  accountId
  folderId
  folderName
  status
```

This fixes the observed proxy MCP ambiguity:

- Current state: `.flowpilot/settings/artifact-storage-google-drive.json` can contain multiple connected `projectId` records.
- Current proxy MCP behavior: `proxyArtifactConnection()` accepts exactly one connected project and fails when multiple are present.
- Correct behavior: MCP chooses an account/token; artifact sync chooses a project folder binding.

This plan is a draft until upstream spec/design documents approve broad account-level MCP read behavior. The implementation must not silently treat artifact folder selection as the source of truth for MCP read scope.

## 2. Input Documents

- [CP-29: FlowPilot Proxy MCP Server For Google Drive](./CP-29-MCP-Proxy-Google-Drive.md)
- [CP-06-01: Artifacts Sync](../done/CP-06-01-Artifacts-Sync.md)
- [CP-05-03: Google Drive MCP Current Implementation Notes](../done/CP-05-03-Driver-Mcp.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- [SD-08: Artifact Management](../../06-System-Tech-Design/SD-08-Artifact-Management.md)
- [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md)
- [SS-07: Workflow Artifact](../../05-System-Specs/SS-07-Workflow-Artifact.md)

## 3. Implementation Strategy

- overall approach:
  - Introduce an account-level Google Drive connection state before changing proxy MCP selection.
  - Keep existing artifact folder binding behavior, but make it reference an account connection instead of owning the credential directly.
  - Update proxy MCP to resolve credentials from account state and remove the "exactly one artifact-sync project" shortcut.
  - Update admin-web to show Google Drive account connection and per-project artifact folder binding as separate steps.

- sequencing logic:
  - First add model/state types and compatibility readers.
  - Then migrate runner artifact sync to use `accountId + folderId`.
  - Then migrate proxy MCP to use `accountId`.
  - Then update UI/API surfaces.
  - Finally add migration/backfill and manual validation.

- dependencies:
  - Upstream spec/design approval for account-scoped MCP reads.
  - OAuth implementation follows the capability matrix in Section 3.3.
  - Existing local runner secret store remains available.
  - Existing Google OAuth client config remains valid for local callback flows.

### 3.1 Problem Details

The current implementation mixes three different meanings into one artifact-sync project connection:

- Google account identity.
- Google refresh token.
- FlowPilot project artifact folder.

That works only while there is exactly one connected project. It fails when a runner has multiple FlowPilot projects connected to Drive, even if those projects use the same Google Cloud OAuth client.

Example current state:

```text
Project A -> Google account X -> folder X
Project B -> Google account Y -> folder Y
```

MCP wants to know:

```text
Which Google account should I use?
```

Artifact sync wants to know:

```text
For this project, which folder should I write artifacts into?
```

The current proxy MCP asks artifact sync for a single connected project. With two projects, it cannot know whether to use account X or account Y, so it fails. That failure is correct for the current model, but the model is wrong for the desired product behavior.

### 3.2 Target Runtime Model

The new model has two separate runtime flows:

```text
MCP read flow
  provider CLI
  -> FlowPilot Google Drive proxy MCP
  -> explicit Google Drive accountId from workflow/project runtime context
  -> selected Google Drive account token
  -> Google Drive APIs across granted account scope
```

```text
Artifact sync flow
  artifact sync request for projectId
  -> project Google Drive folder binding
  -> linked Google Drive account token
  -> selected Drive folder
  -> upload/open/sync artifact files
```

The artifact folder remains important for artifact sync, but it is not a read boundary for MCP.

Account resolution rules:

- Project/workflow MCP execution must pass `accountId` explicitly through runtime environment, such as `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID`.
- Provider MCP config should stay generic where possible; runtime launch env supplies the account for the current project/workflow.
- The proxy MCP may accept `--google-drive-account-id` for diagnostics, but runtime env wins when both are set.
- A runner-level default account is allowed only for settings-page test calls and local diagnostics.
- The proxy must never choose an account by scanning artifact project bindings and guessing.
- If multiple accounts exist and no account is selected, MCP preflight and tool calls fail with an actionable "select Google Drive account" error.

### 3.3 OAuth Scope Strategy

The current artifact flow requests:

```text
https://www.googleapis.com/auth/drive.file openid email
```

That scope is appropriate for app-created or picker-authorized files, but it is not a reliable "read all Drive data" scope.

The new account connection flow must request scopes based on capability:

- Account identity: `openid email`
- Artifact folder binding and artifact writes: `https://www.googleapis.com/auth/drive.file`
- MCP broad read: `https://www.googleapis.com/auth/drive.readonly`
- Future structured Google Docs API reads: add `https://www.googleapis.com/auth/documents.readonly` only if Drive export no longer satisfies the tool contract.

The implementation must persist `grantedScopes` in account state and surface capability status in the UI:

- `artifactReady`
- `mcpReadReady`
- `mcpWriteReady`
- `reconnectRequired`

Capability rules:

| Capability | Required state | Behavior when missing |
|---|---|---|
| `accountReady` | refresh token exists and refresh succeeds | Account status is `reconnect_required` or `failed`. |
| `artifactReady` | `accountReady`, `drive.file`, and valid project folder binding | Artifact sync is disabled for that project and asks user to reconnect or select folder. |
| `mcpReadReady` | `accountReady` and `drive.readonly` | MCP read preflight fails with missing-scope reconnect message. |
| `mcpWriteReady` | `accountReady`, `drive.file`, and `read_write` workflow/tool policy | MCP write tools remain disabled or fail before write approval. |

Write-mode scope limits:

- CP-30 does not approve arbitrary full-Drive writes.
- `read_write` MCP may create docs/folders under an explicit `parentFolderId`.
- If a project workflow has a Google Drive artifact binding and `parentFolderId` is omitted, the proxy may default write parents to that project's bound artifact folder.
- Outside a project workflow, write tools must require `parentFolderId` unless a later spec approves a runner default write folder.
- Updating an existing Drive file is allowed only when Google API permission and granted scopes permit it; otherwise the tool must fail with a clear access/scope error.

If broad read scopes are missing, MCP should fail preflight with a clear message asking the user to reconnect the account with MCP read access.

## 4. Work Breakdown

- `P-0` Align upstream product and design documents.
  - Update or explicitly approve `SS-02` to distinguish account-scoped MCP reads from project artifact folder binding.
  - Update or explicitly approve `SD-11` to describe account-level Google Drive OAuth state and project-level folder binding.
  - Update or supersede CP-29 where it says proxy MCP should directly reuse artifact-sync OAuth/token state.
  - Confirm the capability matrix and write-mode default-parent rules before coding runner changes.

- `P-1` Define account-level runner models.
  - Add `googleDriveAccountState` with `Accounts`, `Sessions`, and compatibility metadata.
  - Add `googleDriveAccountRecord` with `AccountID`, `AccountEmail`, `AccountSubject`, `DisplayName`, `OAuthClientID`, `GrantedScopes`, `Status`, `ConnectedAt`, `UpdatedAt`, `LastValidatedAt`, and `NeedsReview`.
  - Add secret keys such as `google-drive:account:<accountId>:credential`.
  - Derive new account IDs from Google account subject plus OAuth client ID when available; never derive new account IDs from project ID.
  - Keep refresh tokens out of JSON state.

- `P-2` Define project folder binding models.
  - Replace credential ownership in `artifactStorageGoogleDriveConnectionRecord` with `AccountID`.
  - Keep `ProjectID`, `FolderID`, `FolderName`, `Status`, timestamps, and last error.
  - Treat the binding as artifact-specific routing, not as MCP auth state.
  - Preserve old JSON loading through compatibility migration.

- `P-3` Add account connection runner APIs.
  - Add endpoints for account connect session creation, OAuth redirect, OAuth callback, account status, account list, account reconnect, and account disconnect.
  - Store token data via the runner secret store.
  - Parse and persist account subject/email from ID token or Google userinfo response.
  - Persist granted OAuth scopes and compute capability flags from the matrix in Section 3.3.
  - Validate account capability by refreshing the access token and calling a lightweight Drive API.

- `P-4` Refactor artifact Google Drive connection flow.
  - Change artifact connection session creation to accept `projectId` and either an existing `accountId` or a "connect new account" option.
  - If `accountId` exists, skip OAuth and go directly to folder picker using a short-lived access token.
  - If `accountId` is new, run account OAuth first, then folder picker.
  - Save `projectId -> accountId -> folderId` binding after folder selection.

- `P-5` Refactor artifact sync and open flows.
  - Update upload/open helpers in artifact cloud storage to resolve `projectId` to a binding, then resolve the linked account token.
  - Keep existing remote path and manifest behavior.
  - Mark only the project binding as `reconnect_required` when account token refresh fails, and mark the account status so all dependent bindings show the shared auth problem.

- `P-6` Refactor proxy MCP account resolution.
  - Add `--google-drive-account-id` or equivalent proxy MCP argument.
  - Add `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID` runtime environment selection.
  - Make runtime env override static CLI args when both are present.
  - Replace `proxyArtifactConnection()` with account lookup.
  - Make `authGetStatus` report account source, account email, granted scopes, and whether artifact folder binding is present for the current project.
  - Keep artifact folder out of read queries.

- `P-7` Update provider MCP config generation.
  - Keep Codex, Claude, and Gemini MCP server config generic so it can serve different projects.
  - Inject `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID` into the provider process environment at workflow launch time.
  - Use a runner default account only for setup-page diagnostics outside workflow execution.
  - Keep `read_only` and `read_write` tool allowlists from CP-29.
  - Fail provider config preflight if proxy mode is enabled and no MCP-ready account exists.
  - For project workflow execution, pass the project-selected account from the project binding or explicit MCP account setting.

- `P-8` Update admin-web domain models and gateways.
  - Add TypeScript models for Google Drive account records, account status, capabilities, and project folder binding.
  - Add local-runner gateway methods for account list/connect/reconnect/disconnect and project folder binding actions.
  - Keep existing artifact storage status APIs compatible while adding `accountId` and account capability fields.

- `P-9` Update admin-web UI.
  - Split Google Drive setup UI into account connection status and provider MCP config status.
  - Update project artifact settings to choose a connected account and then choose a folder.
  - Show which account a project artifact folder uses.
  - Show MCP readiness independently from artifact folder readiness.
  - Show missing-scope/reconnect actions when the connected account does not grant broad MCP read scopes.

- `P-10` Add migration/backfill.
  - On runner startup or first state read, detect legacy project-scoped credentials.
  - Validate each legacy refresh token before linking it to a new account record.
  - Create account records from Google account subject plus OAuth client ID when validation returns subject metadata.
  - Fall back to normalized account email plus OAuth client ID only for legacy tokens that cannot return subject metadata.
  - Link each legacy project connection to the derived account record.
  - If two legacy connections appear to be the same account but token validation disagrees or metadata is incomplete, create separate `NeedsReview` account records and require user review.
  - Do not delete legacy project secrets until the migrated account token refreshes successfully and every dependent project binding validates.

- `P-11` Add tests.
  - Cover account state load/save, secret lookup, and migration from legacy artifact state.
  - Cover artifact sync resolution through `projectId -> accountId -> folderId`.
  - Cover proxy MCP with multiple project bindings and one selected account.
  - Cover missing scope, revoked token, and no selected account errors.
  - Cover runtime provider env injection for `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID`.
  - Cover UI status rendering for account-ready, folder-missing, MCP-scope-missing, and reconnect-required states.

- `P-12` Update documentation and preflight messages.
  - Update setup text to explain account connection versus project folder binding.
  - Replace "multiple artifact-sync Google Drive connections" with actionable account selection errors.
  - Document that MCP reads are account-scoped by OAuth permission, not artifact-folder-scoped.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/types.go`
  - `apps/local-runner/internal/runner/google_drive_config.go`
  - `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
  - `apps/local-runner/internal/runner/artifact_google_drive_auth.go`
  - `apps/local-runner/internal/runner/artifact_cloud_storage.go`
  - `apps/local-runner/internal/runner/google_drive_proxy_mcp.go`
  - `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
  - `apps/local-runner/internal/cli/root.go`
  - `apps/admin-web/src/domain/model/entity/local-runner.ts`
  - `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`
  - `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
  - `apps/admin-web/src/app/api/runtime/google-drive-config/**`
  - `apps/admin-web/src/app/api/local-runner/artifact-storage/google-drive/**`
  - `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
  - `apps/admin-web/src/routes/_authenticated/settings/components/-GoogleDriveProviderConfigCard.tsx`
  - `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.tsx`
  - `apps/admin-web/src/features/artifacts/artifact-storage-connection.ts`

- modules:
  - local-runner Google Drive OAuth and secret storage
  - local-runner artifact cloud storage
  - local-runner proxy MCP server
  - provider MCP config generation
  - admin-web runtime Google Drive setup APIs
  - admin-web project artifact settings

- database:
  - No required Supabase token storage.
  - Optional future Supabase metadata may mirror non-secret account labels and binding readiness, but not in this CP's MVP.

- external systems:
  - Google OAuth
  - Google Drive API
  - Google Docs export/read API path
  - Google Picker
  - Codex, Claude, and Gemini MCP client configs

## 6. Data or Migration Steps

- schema:
  - Add runner-local state file `.flowpilot/settings/google-drive-accounts.json`.
  - Extend `.flowpilot/settings/artifact-storage-google-drive.json` connection records with `accountId`.
  - Keep existing state file names where possible to reduce user disruption.

- data backfill:
  - Read existing `artifact-storage-google-drive.json` connections.
  - For each connected project, read its legacy credential from `google-drive:project:<projectId>` secret key.
  - Refresh the legacy token and fetch account subject/email metadata where possible.
  - Create or reuse an account record using `accountSubject + OAuthClientID`.
  - If subject is unavailable, use `normalizedAccountEmail + OAuthClientID` and set `NeedsReview=true`.
  - Save refresh token under `google-drive:account:<accountId>:credential`.
  - Update the project connection with `accountId`.
  - Preserve `folderId`, `folderName`, `accountEmail`, `connectedAt`, and `updatedAt`.
  - Leave legacy secrets in place for one release or until successful validation, then add a cleanup task.

- config updates:
  - Add a selected MCP account setting for provider config generation.
  - Add an account capability status to `/google-drive-config`.
  - Add `accountId` to artifact storage Google Drive status responses.
  - Add proxy MCP CLI args and parser support for selected account.
  - Add workflow/provider launch env injection for `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID`.

## 7. Validation Plan

- tests to add:
  - `TestGoogleDriveAccountStateSaveLoad`
  - `TestGoogleDriveAccountMigrationFromLegacyProjectCredentials`
  - `TestArtifactGoogleDriveConnectionStoresAccountBinding`
  - `TestArtifactSyncUsesProjectBindingAccountCredential`
  - `TestProxyMcpUsesSelectedAccountWhenMultipleProjectBindingsExist`
  - `TestProxyMcpFailsWhenSelectedAccountMissingMcpReadScope`
  - `TestProxyMcpFailsWhenMultipleAccountsExistWithoutSelectedAccount`
  - `TestProxyMcpUsesProjectArtifactFolderAsWriteDefaultOnlyWhenProjectBindingExists`
  - `TestProviderLaunchEnvIncludesGoogleDriveAccountId`
  - `TestLegacyMigrationCreatesNeedsReviewAccountWhenSubjectIsUnavailable`
  - admin-web route tests for account status and project binding status
  - component tests for account connection, folder binding, reconnect-required, and missing-scope states

- manual checks:
  - Connect one Google account and verify MCP read status is configured.
  - Bind Project A to folder X under that account.
  - Bind Project B to folder Y under that same account.
  - Run MCP `authGetStatus`, `search`, `listSharedDrives`, and `readGoogleDoc`; verify reads are not limited to folder X or Y.
  - Sync artifacts for Project A and verify files land in folder X.
  - Sync artifacts for Project B and verify files land in folder Y.
  - Revoke Google access and verify both MCP and artifact binding surfaces show reconnect-required.

- failure cases:
  - No connected account exists.
  - Account exists but lacks MCP read scope.
  - Project has no artifact folder binding.
  - Project binding references a deleted account.
  - Token refresh fails with `invalid_grant`.
  - Folder was deleted or permission was removed.
  - Multiple accounts exist and no MCP account is selected.
  - Legacy migration sees duplicate email with conflicting or unverifiable token metadata.

### 7.1 Manual Test Guide

1. Start with a clean runner state or back up `.flowpilot/settings/google-drive-config.json`, `.flowpilot/settings/google-drive-accounts.json`, and `.flowpilot/settings/artifact-storage-google-drive.json`.
2. Open Google Drive setup in admin-web.
3. Connect Google account A with MCP read capability.
4. Confirm the account row shows `connected`, the account email, and MCP read readiness.
5. Open Project A settings or artifact storage UI.
6. Select account A and choose folder X for artifact sync.
7. Confirm Project A shows `google_drive connected`, account A, and folder X.
8. Open Project B settings or artifact storage UI.
9. Select account A again and choose folder Y for artifact sync.
10. Confirm Project B shows `google_drive connected`, account A, and folder Y.

11. Run a Google Drive MCP read-only workflow for Project A.
12. Ask the provider to list or search for a file outside folder X but inside account A's Drive.

13. Confirm MCP can read the file if OAuth scopes permit it.
14. Generate an artifact in Project A and run manual sync.
15. Confirm the remote artifact appears under folder X only.
16. Generate an artifact in Project B and run manual sync.
17. Confirm the remote artifact appears under folder Y only.
18. Connect Google account B.
19. Run MCP diagnostics or a workflow with runtime account B selected.
20. Confirm MCP reads now use account B while Project A and Project B artifact folder bindings remain on account A.
21. Rebind Project B artifact sync to account B and choose folder Z.
22. Confirm Project B artifact sync writes to folder Z while Project A still writes to folder X.
23. Switch MCP runtime account back to account A and confirm MCP reads use account A, not Project B's account B binding.
24. Revoke account A from Google Account permissions.
25. Refresh runner status.
26. Confirm Project A artifact binding reports reconnect-required and MCP only reports reconnect-required if account A is the selected MCP account.
27. Confirm Project B artifact binding remains connected through account B.
28. Reconnect account A.
29. Confirm existing Project A folder binding recovers without asking the user to select folder X again.

## 8. Rollout and Fallback

- rollout order:
  - Complete `P-0` upstream approval before enabling implementation.
  - Ship account state and compatibility migration behind a feature flag such as `FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_CONNECTIONS`.
  - Enable account-backed artifact sync while still reading legacy project credentials.
  - Enable account-backed proxy MCP selection.
  - Update UI after runner APIs are stable.
  - Remove legacy raw MCP UI only after proxy MCP account flow is proven.

- fallback path:
  - If migration fails, keep reading legacy `projectId` credential secrets for artifact sync.
  - If account-backed proxy MCP fails, allow disabling `FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP` and using the raw MCP token-file path from CP-05-03.
  - Preserve existing artifact folder bindings so users do not need to reconnect projects after rollback.

- monitoring:
  - Log account migration results without token values.
  - Surface account status, granted capability status, project binding status, and last error in admin-web.
  - Add clear runner errors for missing account, missing binding, missing scope, and reconnect-required.

## 9. Risks

- `R-1` OAuth scope expansion may require Google Cloud OAuth consent verification or user re-consent.
- `R-2` Migrating project-scoped credentials to account records can accidentally merge different Google identities if account metadata is incomplete.
- `R-3` Broad Drive read access is more sensitive than artifact-folder write access and needs explicit product/security approval.
- `R-4` Provider launch code must inject project-specific account selection reliably because account-home-scoped MCP config should remain generic.
- `R-5` Existing users may have raw MCP token files and artifact sync connections configured differently, creating confusing status combinations during transition.
- `R-6` Multiple accounts plus multiple project folder bindings can make UI state dense unless the setup pages clearly separate account readiness from project binding readiness.

## 10. Definition of Done

- Model:
  - [x] Runner has account-level Google Drive state with non-secret metadata.
  - [x] Runner stores account refresh tokens only in the secret store.
  - [x] Runner supports `projectId -> accountId -> folderId` artifact binding.
  - [x] Legacy project-scoped artifact connections migrate or remain readable through compatibility fallback.

- Runner:
  - [x] Account OAuth connect, reconnect, status, and disconnect APIs exist.
  - [x] Artifact sync resolves credentials through project folder binding and account token.
  - [x] Proxy MCP resolves credentials through selected account, not through "only connected artifact project".
  - [x] Proxy MCP read tools do not filter by artifact folder.
  - [x] Proxy MCP reports missing account, missing scope, and reconnect-required clearly.
  - [x] Provider launch environment passes selected Google Drive account context to Codex, Claude, and Gemini.

- UI:
  - [x] Google Drive setup separates account connection status from provider MCP config status.
  - [x] Project artifact settings let the user select a connected account and choose a folder.
  - [x] UI displays account email, folder name, MCP readiness, artifact readiness, and reconnect-required states.
  - [x] UI does not imply that artifact folder selection limits MCP read access.

- Tests:
  - [x] Local runner unit tests cover account state, migration, artifact binding, proxy MCP account selection, and scope failures.
  - [x] Admin-web route tests cover account and binding APIs.
  - [x] Admin-web component tests cover ready, missing-scope, missing-folder, and reconnect-required states.
  - [x] Existing artifact sync tests still pass.
  - [x] Existing Google Drive MCP provider config tests are updated and pass.

- Manual validation:
  - [x] One account can power MCP reads across Drive data allowed by OAuth scope. Covered by deterministic runner proxy/account-selection tests in this revision; live browser smoke remains recommended.
  - [x] Two projects can use the same account with different artifact folders. Covered by deterministic runner multi-binding tests in this revision; live browser smoke remains recommended.
  - [x] MCP reads are not limited to either artifact folder. Covered by deterministic selected-account proxy tests in this revision; live browser smoke remains recommended.
  - [x] Artifact sync writes still land in each project's selected folder. Covered by existing artifact-sync Google Drive regression tests in this revision; live browser smoke remains recommended.
  - [x] Multiple connected accounts require explicit MCP account selection and never fall back to guessing. Covered by deterministic proxy preflight and access-token tests in this revision; live browser smoke remains recommended.
  - [x] Revoked Google access produces reconnect-required for all dependent surfaces. Covered by deterministic reconnect-required runner/proxy/artifact-sync tests in this revision; live browser smoke remains recommended.

- Documentation and compliance:
  - [x] `SS-02` and `SD-11` are updated or explicitly approved to allow account-scoped broad MCP reads.
  - [x] CP-29 is updated or superseded where it says proxy MCP should directly reuse artifact-sync OAuth/token state.
  - [x] The OAuth capability matrix is implemented and visible in runner/API/UI status.
  - [x] User-facing setup copy explains account connection versus project folder binding.

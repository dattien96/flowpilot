# CP-06-01: Artifacts Sync

Companion setup guide: [CP-06-02-Artifacts-Sync-Setup-Guide.md](C:/working/flowpilot/requirements/07-Coding-Plan/priority/CP-06-02-Artifacts-Sync-Setup-Guide.md)

## 1. Goal

Sync local artifact snapshots to shared online storage while preserving version history and stable canonical access.

- The Go runner on the current PC is the backend for local sync state, auth/session handling, and manifest persistence.
- Supabase Storage is the default shared provider.
- A project may switch to Google Drive after the user completes OAuth login and selects one destination folder on that PC.
- The `/artifacts` page provides a QR code and normal browser link for the Google Drive connection flow on the current host.
- Artifacts must be visible in both project detail pages and the global artifact page, with manual sync available from both places.
- Both pages must present artifact data as two sections: `Local` and `Remote/Synced`.
- Both pages should expose one bulk sync action for the current filtered scope instead of requiring per-row sync clicks.
- FlowPilot must automatically sync eligible local artifacts when possible, while still keeping manual retry available.
- Supabase stores shared sync state so local-only versus remotely synced artifacts can be read consistently across app sessions and PCs.
- Synced artifact bytes live in the shared destination. Secrets and machine-specific state stay local to the runner host.

## 2. Current-State Findings

### 2.1 Existing `/artifacts` panel

`ArtifactStoragePanel` currently configures a runner-local filesystem driver:

```text
driverKey = filesystem
remoteRootPath
remoteFolderName
enabled
```

This is an existing local-copy configuration surface, not an online-provider model. It can remain as an advanced backup path, but it is not the primary cloud-sync path.

### 2.2 Existing project preference

The repository already has:

```text
projects.artifact_storage_preference default 'supabase'
Project.artifactStoragePreference: "supabase" | "google_drive"
```

The database column needs a check constraint and a working provider implementation.

### 2.3 Existing Google Drive MCP code

The runner can prepare a Google Drive MCP backend for tool context. MCP backend installation and OAuth state are separate from artifact-storage authorization. Artifact sync requires its own selected folder, refresh-token handling, and Drive API adapter.

### 2.4 Existing local artifact layouts

Sync must continue to support:

1. Declared workflow output with a local canonical file and `.snapshots/<artifactId>/`.
2. Snapshot-only fallback workflow output.
3. Standalone runner `prompt_execution` artifacts without `.snapshots`.

Local layouts remain unchanged.

## 3. Provider Model

### 3.1 Shared providers

```ts
type ArtifactStorageProvider = "supabase" | "google_drive";
```

Each project selects one shared provider through `projects.artifact_storage_preference`. The local runner on each PC owns its own auth/session state for that provider.

Supabase Storage:
- Is the default.
- Uses a private `flowpilot-artifacts` bucket.
- Stores deterministic object paths.
- Creates signed URLs only when an artifact is opened.

Google Drive:
- Is optional.
- Requires a connected project-scoped storage connection on the current PC.
- Uses a user-selected root folder.
- Stores Drive folder and file IDs for idempotent retry and canonical updates.

### 3.2 Local artifacts

Local artifacts already live under the FlowPilot artifact workspace:

```text
.flowpilot/artifacts/<projectId>/<workflowRunId>/<workflowStepKey>/
```

This local artifact tree remains the local source of truth before cloud sync. CP-06-01 does not require an additional filesystem mirror or backup copy, though the existing runner-local filesystem driver may remain as an advanced local backup path.

## 4. Cloud Storage Layout

All providers use one portable logical path:

```text
projects/<projectId>/runs/<workflowRunId>/steps/<workflowStepKey>/
  <outputFilename>
  .snapshots/<artifactId>/
    manifest.json
    prompt.md
    actual-prompt.md        # when present
    stdout.txt
    stderr.txt
    command.txt
    <outputFilename>
```

The `.snapshots/<artifactId>/` folder is immutable history except for a retried partial upload. The convenience file at the step root is canonical.

The canonical file must represent the newest successfully synced snapshot for:

```text
<projectId>/<workflowRunId>/<workflowStepKey>/<outputFilename>
```

Retrying an older snapshot must not replace a newer successfully synced canonical file.

## 5. Sync Contract

### 5.1 Runner responsibility

The local runner:

- Discovers artifacts through local `manifest.json`.
- Exports the selected artifact as a byte-safe sync bundle.
- Persists cloud sync results back into the local manifest.
- Receives explicit sync requests from the UI and automatic sync requests from admin-web/server orchestration.
- Reports enough metadata for admin-web to mirror sync state into Supabase after each attempt.

### 5.2 Local runner responsibility

The local runner API on the current PC:

1. Loads the local artifact and project preference.
2. Requests a byte-safe bundle from the runner.
3. Resolves the selected online provider.
4. Uploads snapshot history.
5. Promotes the canonical file from the newest successfully synced snapshot.
6. Persists local manifest success or failure through the runner.
7. Updates Supabase sync metadata after each success or failure.

### 5.3 Shared sync state

Supabase is the durable shared sync-state store. Local manifests are still the local runner source of truth for artifact bytes and immediate retry state, but the app must not depend only on local files to know whether a generated artifact is local-only or synced remotely.

Persist sync state in Supabase for every workflow artifact run:

- `artifact_runs.storage_provider`
- `artifact_runs.remote_path`
- `artifact_runs.remote_object_id`
- `artifact_runs.sync_status`
- `artifact_runs.updated_at`

Sync state meanings:

| Status | Meaning |
|---|---|
| `local_only` | Artifact exists locally but has no successful remote sync recorded. |
| `syncing` | A manual or automatic sync attempt is in progress. |
| `synced` | Artifact bytes were uploaded and remote metadata was saved. |
| `failed` | Last sync attempt failed; previous successful remote metadata must be preserved when available. |

The UI must combine Supabase state with local runner state and partition the result into two sections:

- `Local`: artifacts discovered on the current runner that are valid for display but do not yet have a remotely synced representation.
- `Remote/Synced`: artifacts that already have durable Supabase sync metadata and remote provider/path state.

Only valid local artifacts may appear in the `Local` section. A valid local artifact must:

- have enough manifest metadata to identify `projectId` and `workflowRunId`
- map to an existing `workflow_runs` record in Supabase
- belong to the current page scope, for example the selected project on the project Artifact tab

Local artifacts that cannot be tied back to workflow-run data must be hidden from the artifact pages until their metadata is repaired or they are resynced through a supported flow.

The primary sync action on these pages is a bulk `Sync artifacts` button. It should enqueue every eligible local or unsynced artifact in the current filtered scope. Individual row-level sync buttons are optional and should only exist if a debug-only workflow still needs them.

### 5.4 Metadata

Persist durable metadata:

- `storageProvider`
- `remotePath`
- `remoteObjectId` when the provider exposes one
- `syncStatus`
- `updatedAt`

Do not persist:

- Supabase signed URLs
- Google OAuth access tokens
- Google OAuth refresh tokens

`remoteUrl` is optional for cloud providers. The UI must open cloud artifacts through a local runner route that creates a fresh signed URL or Drive redirect.

### 5.5 Automatic sync

FlowPilot must automatically sync eligible artifacts to the selected project storage provider when remote sync is possible.

Automatic sync triggers:

1. **Server start reconciliation**
   - When admin-web/server starts, load local runner artifacts and Supabase artifact sync state.
   - Ignore orphan local artifacts that do not map to existing workflow-run data.
   - Find valid local artifacts whose Supabase sync state is missing, `local_only`, or `failed`.
   - Attempt sync for each eligible artifact if the selected provider is ready.
   - Skip artifacts whose provider is not ready, for example Google Drive without a connected folder on the current runner host.

2. **Workflow completion debounce**
   - After a workflow finishes and produces artifacts, schedule sync after 5 minutes.
   - If the user sends another prompt or starts another workflow action for the same run/project before the 5 minutes expires, reset the timer to 5 minutes.
   - When the timer fires, sync the latest local artifacts for that run/project.
   - Manual sync remains available while waiting for the timer.

Automatic sync must be idempotent. Retrying an older artifact snapshot must not overwrite a newer canonical remote artifact.

Automatic sync should mark Supabase state as `syncing` before upload, then `synced` or `failed` after the runner returns. If the process exits while `syncing`, the next server-start reconciliation should treat stale `syncing` rows as retryable.

## 6. Google Drive Connection Flow

### 6.1 Connect session

From `/artifacts`, the user selects a project and clicks `Connect Google Drive`.

The local runner on the current PC creates a short-lived, one-time connect session and returns a FlowPilot URL. The UI:

- Displays a QR code containing that URL.
- Displays the same URL as a clickable link.
- Polls connection status until completion, expiry, or failure.

The QR connect token and OAuth `state` are separate single-use values. Store only their hashes in the local runner secret store.

### 6.2 OAuth

Opening the connect URL starts Google OAuth authorization-code flow:

- Request `access_type=offline`.
- Request `https://www.googleapis.com/auth/drive.file`.
- Validate OAuth `state`.
- Exchange the authorization code through the local runner.
- Store the refresh token only through the local runner secret store on that PC.

### 6.3 Folder picker

After OAuth succeeds, the browser opens Google Picker on the current PC:

- Use a Drive folders view.
- Enable folder selection.
- Provide only a short-lived access token required by Picker.
- Persist the selected `folderId`, `folderName`, and connection status locally on the runner host.
- Never place refresh tokens in browser state.

The initiating `/artifacts` panel updates when polling observes `connected`.

## 7. Supabase Storage Contract

- Create a private `flowpilot-artifacts` bucket.
- Apply RLS policies for authenticated project access.
- Use deterministic object paths.
- Allow retry to overwrite partial snapshot objects.
- Upsert the canonical convenience file after history upload completes.
- Generate time-limited signed URLs only from the server open route.

For large artifact payloads, use resumable upload rather than standard upload.

## 8. Security Requirements

- `integrations.config_encrypted` may store non-secret metadata only. It is JSONB, not an encrypted secret vault.
- Keep Google client secret, refresh tokens, and any Supabase secret keys in the local runner secret store on the current PC.
- Use short-lived, one-time QR connect sessions.
- Do not put OAuth tokens in query strings, logs, browser storage, or local manifests.
- Keep the Picker access token in memory only and discard it after folder selection.
- Validate the selected Drive folder belongs to the authorized picker result before marking the connection ready.
- Keep the Supabase bucket private.

## 9. UI Expectations

Artifacts must be shown in two places.

### 9.1 Project Artifact tab

Each project detail page has an Artifact tab that shows only artifacts belonging to that project.

The project Artifact tab must show:

- A `Local` section containing valid local runner artifacts for the project that are not yet represented as remotely synced artifacts.
- A `Remote/Synced` section containing artifacts for the project that already have durable Supabase sync metadata.
- Sync state for each artifact: `local_only`, `syncing`, `synced`, or `failed`.
- Remote provider when synced: `supabase` or `google_drive`.
- Remote path/object metadata when available.
- A bulk `Sync artifacts` action for the current filtered scope.
- `Open artifact` action that uses the fresh server-generated open route.
- A `Go to artifact storage settings` action that navigates to the global artifact/storage page.
- Exclude orphan local artifacts that do not map to an existing workflow run.

### 9.2 Global artifact page

The global `/artifacts` page must show all generated artifacts for all projects.

The global artifact page must show:

- A project selector or equivalent project context for the current runner host.
- A `Local` section containing valid local runner artifacts from all projects that are not yet represented as remotely synced artifacts.
- A `Remote/Synced` section containing all artifacts with durable Supabase sync metadata across projects.
- Project name or project ID for each artifact.
- Sync state for each artifact: `local_only`, `syncing`, `synced`, or `failed`.
- Remote provider when synced: `supabase` or `google_drive`.
- Remote path/object metadata when available.
- A bulk `Sync artifacts` action for the current filtered scope.
- `Open artifact` action that uses the fresh server-generated open route.
- Supabase Storage as the default provider.
- Google Drive connection and selected-folder status.
- QR code and clickable link while Google Drive authorization is pending.
- Last validation, last sync, and last error state.
- Exclude orphan local artifacts that do not map to an existing workflow run.

Project settings must allow:

- `supabase` selection without Google authorization.
- `google_drive` selection only when a connected Drive folder exists on the current runner host.

Artifact browser actions must:

- Bulk sync eligible artifacts in the current filtered scope.
- Show local, syncing, synced, and failed state.
- Show whether the artifact is local-only or synced remotely.
- Show which remote provider was used when synced.
- Open a synced cloud artifact through a fresh server-generated URL.

## 10. Acceptance Criteria

- New projects default to Supabase Storage.
- Project Artifact tabs show only artifacts for that project.
- The global `/artifacts` page shows artifacts across all projects.
- Both artifact pages separate results into `Local` and `Remote/Synced`.
- The `Local` section shows only valid local artifacts that map to existing workflow runs.
- Orphan local artifacts without workflow-run backing are not shown in either page.
- Both project Artifact tabs and the global artifact page support a bulk `Sync artifacts` action.
- The project Artifact tab includes a `Go to artifact storage settings` action that navigates to the global artifact/storage page.
- Artifact rows show local-only versus remotely synced state.
- Synced artifact rows show the remote provider: `supabase` or `google_drive`.
- Supabase stores durable sync state for every workflow artifact run.
- Server start reconciles unsynced or failed local artifacts and syncs eligible artifacts automatically.
- Workflow completion schedules automatic sync after 5 minutes.
- New prompts or workflow activity within the 5-minute window reset the sync timer.
- A private Supabase bucket stores snapshot history and canonical files.
- Supabase signed URLs are generated on demand and are not persisted.
- A user can scan a QR code or click a link, log in with Google, select a Drive folder on the current PC, and return to a connected state.
- Google Drive sync writes below the selected folder.
- Google Drive refresh tokens are stored only in the local runner secret store.
- Google Drive MCP setup is not treated as artifact-storage readiness.
- Every supported local artifact layout remains syncable.
- Newer successfully synced snapshots update the canonical file.
- Older retries do not regress the canonical file.
- Failed uploads remain retryable and update local and Supabase status with failure details.
- Two PCs can sync the same project to the same shared destination, but each PC keeps its own local auth/session state.

## 11. Implementation Checklist

### 11.1 Artifact Surfaces

- [ ] Global `/artifacts` page lists all generated artifacts across all projects.
- [ ] Project detail Artifact tab lists only artifacts for the current project.
- [ ] Both artifact pages separate items into `Local` and `Remote/Synced` sections.
- [ ] The `Local` section contains only valid runner-local artifacts that map to an existing `workflow_runs` record.
- [ ] Orphan local artifacts without workflow-run backing are excluded from the UI.
- [ ] Both artifact pages show `local_only`, `syncing`, `synced`, and `failed`.
- [ ] Both artifact pages show synced remote provider: `supabase` or `google_drive`.
- [ ] Both artifact pages show remote path and remote object metadata when available.
- [ ] Both artifact pages have a bulk `Sync artifacts` action for the current filtered scope.
- [ ] Both artifact pages have an `Open artifact` action through the fresh open route.
- [ ] Project Artifact tab has `Go to artifact storage settings` linking to global `/artifacts`.

### 11.2 Sync State

- [ ] Supabase stores durable artifact sync state in `artifact_runs`.
- [ ] Persist `storage_provider`, `remote_path`, `remote_object_id`, `sync_status`, and `updated_at`.
- [ ] Mark sync as `syncing` before upload.
- [ ] Mark sync as `synced` after upload and metadata persistence succeed.
- [ ] Mark sync as `failed` when upload or Supabase metadata persistence fails.
- [ ] Preserve previous successful remote metadata on failed retry.
- [ ] Merge local runner artifacts with Supabase sync state for UI display.
- [ ] Partition merged artifact data into `Local` and `Remote/Synced`.
- [ ] Treat workflow-run-backed local artifacts without remote sync metadata as `Local`.

### 11.3 Automatic Sync

- [ ] On server start, load local runner artifacts and Supabase sync state.
- [ ] Skip orphan local artifacts that do not map to workflow-run data.
- [ ] Auto-sync artifacts with missing, `local_only`, `failed`, or stale `syncing` state.
- [ ] Skip auto-sync when the selected provider is not ready.
- [ ] After workflow completion, schedule auto-sync after 5 minutes.
- [ ] Reset the 5-minute timer when another prompt or workflow action occurs for the same run/project.
- [ ] Keep manual sync available during the 5-minute wait.
- [ ] Ensure auto-sync is idempotent and does not regress canonical files.

### 11.4 Verification

- [ ] Test global artifact list shows all projects.
- [ ] Test project Artifact tab filters by project.
- [ ] Test both pages partition results into `Local` and `Remote/Synced`.
- [ ] Test orphan local artifacts are excluded.
- [ ] Test only workflow-run-backed local artifacts appear in `Local`.
- [ ] Test bulk sync from both pages updates Supabase state.
- [ ] Test open action generates a fresh URL or redirect.
- [ ] Test server-start reconciliation retries eligible artifacts.
- [ ] Test workflow completion debounce and timer reset.
- [ ] Test sync state survives app restart.

## 12. Implementation

This section consolidates the implementation record for the runner-local CP-06-01 rollout. `implementation_plan.md` now points here.

### 12.1 Implementation Summary

This section is the source of truth for the runner-local CP-06-01 rollout. `implementation_plan.md` has been merged into this section and now points here.

Implement Supabase Storage as the default shared provider and add optional Google Drive folder sync while keeping `.flowpilot/artifacts` as the local source of truth on the current PC.

Merged review findings and corrections:

1. The current `/artifacts` panel is a filesystem-driver configuration, not an online provider selector.
2. `projects.artifact_storage_preference` already exists and defaults to `supabase`, but the actual sync backend must live in the runner on each PC.
3. Existing Google Drive MCP setup is unrelated to artifact uploads and must not be reused as the storage provider.
4. Google Drive connection must use browser OAuth plus Google Picker folder selection.
5. The QR code should open a short-lived FlowPilot connect URL that continues into OAuth and folder selection on the current host.
6. Google Drive refresh tokens require local runner secret storage. `integrations.config_encrypted` is not sufficient.
7. Supabase private-bucket signed URLs expire and must be generated on demand.
8. Online upload orchestration belongs in the Go runner on the current PC. The UI and web app are thin local clients.
9. The runner keeps ownership of local artifact bytes, bundle export, manifest persistence, and local sync state.

### 12.2 Impact Review

Planning guidance from prior review:

| Symbol or route | Risk | Impact |
|---|---|---|
| `ArtifactStoragePanel` | LOW | Local UI surface only |
| `ArtifactBrowserPanel` | LOW | Local UI surface only |
| `Runner` artifact methods | MEDIUM | Core sync behavior |
| `WorkflowGateway` | CRITICAL | Avoid broad interface changes |
| `WorkflowEngineGateway` | HIGH | Avoid broad interface changes |

Implementation should use focused artifact-storage contracts rather than widening workflow gateways.

### 12.3 Architecture Decision

Keep local artifact ownership and sync orchestration in the Go runner on the current PC:

```text
local runner artifact bundle
  -> local runner provider adapter
  -> shared provider destination
     -> Supabase Storage (default)
     -> Google Drive selected folder (optional)
  -> persist local manifest result
  -> persist Supabase artifact sync state
```

This keeps Google refresh tokens and local connect state machine-local while synced artifact bytes and durable sync state remain shared.

### 12.4 Workstreams

#### Phase 1 - Local runner state

- Store connect sessions, OAuth state, and refresh-token references on the current runner host.
- Keep provider selection and last sync status in local runner state.
- Preserve the existing `.flowpilot/artifacts` layout as the source of truth for local snapshots.

#### Phase 2 - Runner bundle contract

- Add a runner endpoint that exports one artifact snapshot as a byte-safe ZIP sync bundle.
- Add a runner endpoint that persists a cloud sync success or failure result into the local manifest.
- Add matching local-runner gateway methods and cloud metadata fields in TypeScript.

#### Phase 3 - Supabase provider in the runner

- Implement a Supabase Storage adapter in the runner.
- Upload immutable snapshot history to deterministic paths.
- Upsert the canonical convenience file after snapshot upload succeeds.
- Generate signed open URLs only when requested.

#### Phase 4 - Google Drive provider in the runner

- Add runner-local Google OAuth authorization-code flow with offline access.
- Request `https://www.googleapis.com/auth/drive.file`.
- Store the refresh token in the local runner secret store on that PC.
- Add a short-lived connect session usable from a QR code or browser link.
- Open Google Picker with folder selection enabled and persist the selected folder ID and name locally.
- Upload snapshots and canonical files below the selected Drive folder.

#### Phase 5 - UI and local proxy routes

- Keep the admin-web `/artifacts` experience as the UI for the current runner.
- Add the artifact browser to the global `/artifacts` page so it lists all generated artifacts from all projects, partitioned into `Local` and `Remote/Synced`.
- Add or update the project Artifact tab so it lists only artifacts for the current project, partitioned into `Local` and `Remote/Synced`.
- Add a bulk `Sync artifacts` action in both the global artifact page and project Artifact tab.
- Add `Go to artifact storage settings` from the project Artifact tab to the global `/artifacts` page.
- Replace the filesystem-only primary panel with provider cards and connect state where provider settings are shown.
- Show Supabase Storage as the default ready provider.
- Show Google Drive states: not connected, waiting for login, waiting for folder selection, connected, and failed.
- Make project storage preference editable from project settings.
- Filter the `Local` section so only runner artifacts backed by existing workflow-run records are visible.

#### Phase 6 - Shared sync state and automation

- Persist sync status, storage provider, remote path, and remote object ID to Supabase after each sync attempt.
- Merge local runner artifacts with Supabase sync state in both artifact list surfaces.
- Partition merged results into `Local` and `Remote/Synced`, hiding orphan local artifacts.
- Add server-start reconciliation for `local_only`, missing, failed, and stale `syncing` artifacts.
- Add workflow-completion debounce that waits 5 minutes before automatic sync.
- Reset the 5-minute debounce when new prompt/workflow activity occurs for the same run/project.
- Ensure automatic sync skips provider-not-ready cases without blocking manual retry.

#### Phase 7 - Verification

- Add focused runner, proxy route, OAuth callback, picker-selection, and UI tests.
- Verify Supabase default sync, Google Drive sync, retry, canonical promotion, expired connect sessions, OAuth denial, and revoked refresh tokens.
- Verify project Artifact tab filtering, global artifact aggregation, sync-state display, and bulk sync from both surfaces.
- Verify server-start reconciliation and workflow-completion debounce behavior.
- Verify Supabase sync-state persistence survives app restart.
- Run a manual QR-code connection pass in a second browser or phone on the same PC host.

### 12.5 Detailed Changes

#### Local Runner

- Add runner-local artifact storage interfaces and provider adapters in Go.
- Keep bundle export and sync-result persistence in the runner.
- Add runner-local connect-session and token storage for Google Drive.
- Keep the filesystem mirror as an advanced local-backup path if it remains enabled separately.

#### Shared synced data

- Store artifact bytes in Supabase Storage or Google Drive.
- Keep the durable provider path and remote object ID available to any PC that needs to open the shared artifact.
- Store durable sync state in Supabase so UI state survives app restarts and can be read outside the local runner manifest.
- Do not rely on one PC's local manifest to understand another PC's sync state or auth state.

#### Admin-web

- Treat admin-web as the UI and thin local proxy to the current runner host.
- Use the existing artifact browser and storage panel to show provider state, connect status, artifact list state, and last sync/error state.
- Render the artifact browser in both the global artifact page and the project Artifact tab, with project filtering applied in the project tab.
- Own automatic sync scheduling and Supabase sync-state reconciliation.
- Keep the open route on a fresh redirect path so cloud-backed artifacts are opened through the current provider state.

#### Tests

- Runner bundle export and result persistence.
- ZIP limit and traversal checks.
- Supabase path mapping, canonical promotion, retry, and fresh signed URL generation.
- Google OAuth state validation, callback token handling, folder selection, revoked-token failure, and recovery.
- Sync route provider selection and local-state persistence.
- `/artifacts` provider cards, QR pending state, and polling completion.
- Project Artifact tab shows only project artifacts.
- Global artifact page shows all project artifacts.
- Bulk sync from both artifact pages updates Supabase sync state.
- Server-start auto sync retries missing, `local_only`, failed, and stale `syncing` artifacts.
- Workflow completion auto sync runs after a 5-minute debounce and resets on new activity.

### 12.6 Open Prerequisites

- `GOOGLE_DRIVE_CLIENT_ID`
- `GOOGLE_DRIVE_CLIENT_SECRET`
- `GOOGLE_DRIVE_REDIRECT_URI`
- `GOOGLE_PICKER_API_KEY`
- `FLOWPILOT_PUBLIC_ORIGIN`

These values are for the runner host that owns the local Google Drive connection.

## 13. Non-Goals

- Migrating existing local artifact layouts.
- Deleting snapshot history.
- Reusing Google Drive MCP credentials for artifact storage.
- Persisting expiring Supabase signed URLs.
- Supporting shared drives in the first implementation.
- Supporting multiple Google Drive folders per project in the first implementation.

---

## 14. Q&A

### Q1: What are "Supabase signed URLs" and why are they generated on demand and not persisted?

**What they are:**

Supabase signed URLs are temporary, time-limited URLs that provide direct access to files stored in Supabase Storage. They look like:

```
https://your-project.supabase.co/storage/v1/object/sign/flowpilot-artifacts/projects/abc/runs/xyz/Response.md?token=eyJhbGc...
```

These URLs expire after a configured time period (e.g., 1 hour, 24 hours, 1 week).

**Why generate on demand (not persist):**

| Approach                     | Problem                                                                                       |
| ------------------------------| -----------------------------------------------------------------------------------------------|
| ❌ **Store in database**      | URL becomes invalid after expiration; creates security risk if leaked URLs remain in database |
| ✅ **Generate fresh on open** | URL is always valid; minimizes exposure window; automatic cleanup through expiration          |

**The flow:**

```
User clicks "Open Artifact" in UI
  ↓
Admin-web calls local runner route
  ↓
Runner calls Supabase Storage API: storage.from('flowpilot-artifacts').createSignedUrl(path, expiresIn)
  ↓
Supabase returns fresh signed URL (valid for X hours)
  ↓
Browser redirects to signed URL
  ↓
File opens/downloads
```

**Security benefits:**

- Expired URLs automatically become invalid (no manual cleanup needed)
- Leaked URLs have limited lifetime
- Each access creates an audit trail
- No stale URLs accumulate in the database

**Implementation note:**

The `artifact_runs.remote_url` field should remain empty or contain only a stable reference path (not the signed URL). The actual signed URL is generated through a dedicated "open artifact" API route that calls the runner's Supabase adapter.

---

### Q2: What is the "short-lived QR code" for Google Drive connection?

**What it is:**

A temporary authorization link that enables Google Drive connection from any device on the same network. The QR code contains a short-lived FlowPilot URL pointing to the local runner.

**Example QR code content:**

```
http://192.168.1.100:3000/runner/connect/google-drive?token=short_lived_abc123
```

**About the token in the URL:**

The `token=short_lived_abc123` parameter is a **connect token created by the Go runner**:

| Aspect | Details |
|--------|---------|
| **Created by** | Go runner's local-runner service when user clicks "Connect Google Drive" |
| **Purpose** | Secure the QR code link and validate the connection request came from FlowPilot |
| **Stored as** | `hash(token)` in runner's local secret store (not the plaintext token) |
| **Lifetime** | 5-10 minutes |
| **One-time use** | Marked as "used" after first successful validation |
| **Scope** | Tied to specific projectId |

**Token creation flow:**

```
User clicks "Connect Google Drive" button
  ↓
Admin-web → POST /api/local-runner/connect/google-drive
  ↓
┌─────────────────────────────────────────────────────┐
│ Go Runner creates connect session:                  │
│                                                      │
│ 1. token = generateSecureRandomString(32)           │
│    // e.g., "short_lived_abc123xyz789..."           │
│                                                      │
│ 2. Store in runner secret store:                    │
│    hash = sha256(token)                             │
│    store {                                           │
│      hash: hash,                                     │
│      projectId: "...",                               │
│      createdAt: now(),                               │
│      expiresAt: now() + 10 minutes,                 │
│      used: false                                     │
│    }                                                 │
│                                                      │
│ 3. Build connect URL:                               │
│    url = "http://192.168.1.100:3000/runner/         │
│           connect/google-drive?token=" + token      │
│                                                      │
│ 4. Return { connectUrl: url, expiresAt: ... }       │
└─────────────────────────────────────────────────────┘
  ↓
UI displays QR code + clickable link
```

**Token validation flow:**

```
User scans QR code or clicks link
  ↓
Browser opens: .../connect/google-drive?token=short_lived_abc123
  ↓
┌─────────────────────────────────────────────────────┐
│ Go Runner validates connect token:                  │
│                                                      │
│ 1. Extract token from query param                   │
│ 2. hash = sha256(token)                             │
│ 3. Look up hash in secret store                     │
│ 4. Validate:                                         │
│    ✓ Hash exists?                                   │
│    ✓ Not expired?                                   │
│    ✓ Not already used?                              │
│                                                      │
│ 5. If valid:                                         │
│    - Mark token as "used"                           │
│    - Generate OAuth state parameter                 │
│    - Redirect to Google OAuth                       │
│                                                      │
│ 6. If invalid:                                       │
│    - Return error: "Connection link expired"        │
└─────────────────────────────────────────────────────┘
```

**Connect Token vs OAuth State (Two separate security tokens):**

| Token Type | Created By | Purpose | Lifetime | Stored As |
|------------|-----------|---------|----------|-----------|
| **Connect Token** | Go Runner | Secure QR code link, validate connection request | 5-10 min | `hash(token)` in runner secret store |
| **OAuth State** | Go Runner | Prevent CSRF in OAuth flow | ~30 min | `hash(state)` in runner secret store |
| **OAuth Code** | Google | Exchange for tokens | ~10 min | Not stored (used immediately) |
| **Access Token** | Google | Temporary API access | 1 hour | Memory only (for Picker) |
| **Refresh Token** | Google | Long-term API access | Forever | Encrypted in runner secret store |

Both the connect token and OAuth state are single-use, cryptographically random values that protect different stages of the authorization flow.

**The complete flow:**

```
┌─────────────────────────────────────────────────────────────┐
│ Step 1: User clicks "Connect Google Drive" on /artifacts   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Local runner creates connect session:                       │
│ - Generates random token (valid 5-10 minutes)               │
│ - Stores hash(token) in local secret store                  │
│ - Returns URL: http://[local-ip]:3000/runner/connect/...   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ UI displays:                                                 │
│                                                              │
│   ╔═══════════════════════════════╗                        │
│   ║  Connect Google Drive          ║                        │
│   ╠═══════════════════════════════╣                        │
│   ║                                ║                        │
│   ║   ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓         ║                        │
│   ║   ▓▓   QR CODE       ▓▓        ║  ← Scan on phone      │
│   ║   ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓         ║                        │
│   ║                                ║                        │
│   ║   Or click:                    ║                        │
│   ║   http://192.168.1.100:3000... ║  ← Click same PC      │
│   ║                                ║                        │
│   ║   ⏱️  Expires in 4:32          ║  ← Countdown          │
│   ║                                ║                        │
│   ║   Status: Waiting for login... ║                        │
│   ╚═══════════════════════════════╝                        │
│                                                              │
│   [UI polls status every 2-3 seconds]                       │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 2: User scans QR or clicks link                        │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Runner validates connect token:                              │
│ - Check hash(token) exists in secret store                   │
│ - Check not expired                                          │
│ - Check not already used                                     │
│ - Generate OAuth state parameter                             │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 3: Redirect to Google OAuth                            │
│                                                              │
│ https://accounts.google.com/o/oauth2/v2/auth?               │
│   client_id=...                                              │
│   redirect_uri=http://localhost:3000/runner/oauth/callback  │
│   response_type=code                                         │
│   scope=https://www.googleapis.com/auth/drive.file          │
│   access_type=offline  ← Request refresh token              │
│   state=[random_state_hash]                                  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 4: User logs in with Google account                    │
│ User grants drive.file permission                            │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 5: OAuth callback                                       │
│                                                              │
│ Runner receives: code + state                                │
│ - Validates state matches                                    │
│ - Exchanges code for access_token + refresh_token            │
│ - Stores refresh_token in local runner secret store         │
│ - Marks connect token as used                                │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 6: Google Picker for folder selection                  │
│                                                              │
│ Opens picker UI in browser:                                 │
│ - Shows Drive folder tree                                   │
│ - User selects "FlowPilot Artifacts" folder                 │
│ - Returns folderId                                           │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Step 7: Runner persists connection                           │
│                                                              │
│ Local runner stores:                                         │
│ - projectId                                                  │
│ - folderId (from picker)                                     │
│ - folderName (from picker)                                   │
│ - connectionStatus: "connected"                              │
│ - refresh_token (already in secret store)                    │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ Original /artifacts page polling detects status change:     │
│                                                              │
│   ✓ Connected to Google Drive                               │
│   📁 Folder: FlowPilot Artifacts                            │
│   ✓ Ready to sync                                           │
└─────────────────────────────────────────────────────────────┘
```

**Why short-lived (5-10 minutes):**

| Security aspect | Reason |
|-----------------|--------|
| **Interception risk** | If someone captures the QR code screenshot, it expires quickly |
| **One-time use** | Token invalidated after first successful connection |
| **No replay attacks** | Cannot reuse old QR codes |
| **Network boundary** | Only works on local network (localhost/LAN IP) - phone must be on same subnet as PC |

**Key security properties:**

- Connect token ≠ OAuth state (two separate single-use values)
- Only token hash stored in runner (not plaintext)
- Token expires after 5-10 minutes OR first use
- OAuth state validates the authorization flow came from FlowPilot
- Refresh token never leaves local runner secret store
- Short-lived access token used only for Picker, then discarded

**Multiple device support:**

The QR code enables this workflow:
- Desktop PC runs local runner (e.g., at IP `192.168.1.100`)
- User scans QR on phone **on the same local network/subnet**
- User logs in with Google on phone
- User selects folder on phone
- Desktop PC receives connection confirmation
- Both devices now can sync to same Google Drive folder

**Network requirement:** The phone must be on the same local network (subnet) as the PC running the runner, because the QR code contains a local IP address (e.g., `http://192.168.1.100:3000/...`) that is only reachable on the same network. The phone cannot connect to the runner if it's on a different network (e.g., mobile data, different WiFi).

---

### Q3: Does artifact storage work across runner restarts?

**Short answer:** Yes, artifact storage is fully persistent across runner restarts.

**What persists (survives restart):**

| Data | Storage Location | Persists? |
|------|-----------------|-----------|
| Local artifacts | `.flowpilot/artifacts/` on disk | ✅ Yes |
| Artifact manifests | `manifest.json` in each snapshot folder | ✅ Yes |
| Sync status | `syncStatus` field in manifests | ✅ Yes |
| Remote paths | `remotePath`, `remoteObjectId` in manifests | ✅ Yes |
| Storage driver config | `.flowpilot/settings/storage-driver.json` | ✅ Yes |
| Google Drive refresh tokens | Runner's local secret store (encrypted on disk) | ✅ Yes |
| Google Drive folder connections | Runner's local state (on disk) | ✅ Yes |
| Google Drive connect session records | `.flowpilot/artifact-storage-google-drive.json` | ✅ Yes |
| Hashed connect/state tokens | Runner's local secret store | ✅ Yes |
| Supabase artifacts | Supabase Storage (cloud) | ✅ Yes |
| Google Drive artifacts | Google Drive (cloud) | ✅ Yes |

**What does NOT persist (lost on restart):**

| Data | Storage Location | Persists? |
|------|-----------------|-----------|
| Supabase signed URLs | Not stored (generated on demand) | ❌ No |
| Google Drive access tokens | Memory (1-hour lifetime, regenerated from refresh token) | ❌ No |

**After restart behavior:**

```
Runner restarts
  ↓
┌────────────────────────────────────────────────┐
│ What the runner reloads from disk:             │
│                                                 │
│ ✓ All local artifacts (.flowpilot/artifacts/)  │
│ ✓ Artifact manifests with sync status          │
│ ✓ Storage driver configuration                 │
│ ✓ Google Drive refresh tokens (secret store)   │
│ ✓ Google Drive folder connections              │
│ ✓ Google Drive connect session records         │
│                                                 │
│ Result: Sync functionality fully restored      │
└────────────────────────────────────────────────┘
  ↓
User can immediately:
  ✓ Open synced artifacts (runner generates fresh signed URL)
  ✓ Sync new artifacts (uses stored refresh token for Drive)
  ✓ View sync status (from local manifests)
  ✓ Resume a still-valid Google Drive connect session
  
❌ User CANNOT continue:
  ✗ Use an expired Google Drive connect session
```

**Complete restart scenario:**

```
Before restart:
  - Project A synced 10 artifacts to Supabase Storage ✓
  - Project B connected to Google Drive folder ✓
  - All artifacts have remotePath, syncStatus="synced" in manifests ✓

Runner crashes and restarts

After restart:
  - All 10 artifacts still show "synced" status ✓
  - Project B still connected to Drive (refresh token persists) ✓
  - User clicks "Open Artifact":
    → Runner reads remotePath from manifest
    → Runner generates fresh Supabase signed URL ✓
    → Browser redirects to artifact
  - User syncs new artifact to Drive:
    → Runner uses stored Drive refresh token ✓
    → Gets new access token automatically ✓
    → Uploads successfully ✓
```

**Edge case - interrupted connection flow:**

```
User clicks "Connect Google Drive"
  → QR code displayed (token valid for 5 minutes)
  → User scanning QR...
  
Runner crashes/restarts

After restart:
  ✅ Session record still exists on disk
  ✅ Token hashes still exist in runner secret store
  → If the session has not expired yet, user can continue the connect flow
  → If the session expired while the runner was down, user must start a new one
  
But:
  ✓ Previously connected projects still work normally
  ✓ No need to reconnect existing Google Drive folders
```

**Persistence mechanisms:**

1. **Local artifacts**: File system storage at `.flowpilot/artifacts/<projectId>/<runId>/<stepKey>/.snapshots/<artifactId>/`
2. **Runner secret store**: OS-backed secret storage (contains refresh tokens, hashed session tokens)
3. **Storage driver config**: JSON file at `.flowpilot/settings/storage-driver.json`
4. **Google Drive connections**: Runner state file `.flowpilot/artifact-storage-google-drive.json` tracking `projectId → folderId → connectionStatus` mappings and connect session records

**Key takeaway:** All sync functionality survives restarts because critical data (artifacts, auth tokens, connection state, and short-lived connect session records) is persisted locally. The only temporary value that must be regenerated after restart is the Google access token used for API calls and Picker.

---

### Q4: Can multiple PCs sync to the same project?

**Short answer:** Yes, but the setup differs between Supabase Storage and Google Drive.

**Scenario:** PC A syncs artifacts to Supabase/Drive. Can PC B read those artifacts and sync new ones?

#### Part 1: Can PC B read artifacts synced by PC A?

**Supabase Storage: ✅ YES**

```
PC A:
  - Creates local artifact
  - Syncs to Supabase Storage (project abc-123)
  - Artifact stored at: projects/abc-123/runs/xyz/steps/step1/Response.md

PC B:
  ✅ Can read the artifact if:
     • PC B has Supabase credentials (deployment-level, shared across PCs)
     • PC B's admin-web queries artifact_runs table for project abc-123
     • PC B's runner generates fresh signed URL for the remotePath
     
  ✅ How PC B reads it:
     1. Admin-web queries: SELECT * FROM artifact_runs WHERE project_id='abc-123'
     2. Gets remotePath: "projects/abc-123/runs/xyz/steps/step1/Response.md"
     3. PC B's runner calls: Supabase.storage.createSignedUrl(remotePath)
     4. Returns signed URL (valid for X hours)
     5. PC B's browser opens the artifact ✓
```

**Google Drive: ❌ NO (unless PC B also connects)**

```
PC A:
  - Connected to Google Drive (has refresh token for user@gmail.com)
  - Syncs artifact to Drive: "FlowPilot Artifacts/projects/abc-123/..."

PC B:
  ❌ Cannot read the artifact UNLESS:
     • PC B connects to the SAME Google account (user@gmail.com)
     • PC B selects the SAME "FlowPilot Artifacts" folder
     • Then PC B gets its own refresh token for that account
     
  ❌ Why PC B can't read without connection:
     • PC A's refresh token is stored locally on PC A only
     • PC B doesn't have any Google Drive credentials
     • Can't access the Drive folder without auth
```

#### Part 2: Can PC B sync NEW artifacts?

**Supabase Storage: ✅ YES (works immediately)**

```
Project Settings:
  artifactStoragePreference: "supabase"

PC A:
  ✅ Can sync (has Supabase credentials)

PC B:
  ✅ Can sync immediately because:
     • Supabase credentials are deployment-level (shared)
     • No per-PC setup required
     
  How it works:
  1. PC B creates local artifact
  2. PC B's runner checks project preference: "supabase"
  3. PC B's runner uses deployment Supabase credentials
  4. Uploads to: projects/abc-123/runs/new-run/steps/step2/NewArtifact.md
  5. Updates local manifest + artifact_runs table
  ✅ Success - artifact visible to all PCs!
```

**Google Drive: ❌ NO (must connect first)**

```
Project Settings:
  artifactStoragePreference: "google_drive"

PC A:
  ✅ Connected to Drive (has refresh token)
  ✅ Can sync

PC B:
  ❌ NOT connected yet
  ❌ Cannot sync!
  
  What happens when PC B tries to sync:
  1. PC B creates local artifact
  2. PC B's runner checks project preference: "google_drive"
  3. PC B's runner looks for Drive connection for this project
  4. ❌ No connection found (no refresh token stored locally)
  5. PC B's runner returns error:
     "Google Drive not connected for this project on this PC.
      Please connect Google Drive in /artifacts settings."
  
  What PC B must do:
  ┌────────────────────────────────────────────────┐
  │ 1. Go to /artifacts page on PC B               │
  │ 2. Click "Connect Google Drive" for project    │
  │ 3. Scan QR / click link                        │
  │ 4. Log in with Google                          │
  │    (same account as PC A, or different)        │
  │ 5. Select folder                               │
  │    (ideally same "FlowPilot Artifacts" folder) │
  │ 6. PC B now has its own refresh token          │
  │ 7. ✅ PC B can now sync to same Drive folder   │
  └────────────────────────────────────────────────┘
```

**Clarification:** Multiple PCs can target the same Google Drive folder, but each runner must first complete its own local Google Drive connection flow and store its own refresh token.

#### Summary Table

| Scenario | Supabase Storage | Google Drive |
|----------|-----------------|--------------|
| **PC B reads artifacts from PC A** | ✅ YES (uses shared deployment credentials) | ❌ NO (unless PC B connects to same Google account + folder) |
| **PC B syncs NEW artifacts** | ✅ YES (works immediately, no setup) | ❌ NO (must connect Google Drive on PC B first) |
| **Credentials scope** | Deployment-level (shared across PCs) | Per-PC (each PC needs its own connection) |
| **Setup required on PC B** | None (uses existing credentials) | Must complete full Google Drive connection flow |

#### Design Rationale

From **Section 10 - Acceptance Criteria**:
> "Two PCs can sync the same project to the same shared destination, but each PC keeps its own local auth/session state."

This design choice means:

**Supabase Storage:**
- ✅ Auth is deployment-level (SUPABASE_URL, SUPABASE_ANON_KEY environment variables)
- ✅ Same credentials work on all PCs in the deployment
- ✅ PC B works out of the box with no additional setup

**Google Drive:**
- ✅ Auth is per-PC (refresh token stored locally on each PC)
- ✅ Each PC has its own independent Google authorization
- ✅ Allows different team members to use their own Google accounts
- ❌ Requires each PC to connect separately (one-time setup)

#### Multi-PC Collaboration Flow

```
Team scenario: 2 developers working on same project

PC A (Alice's laptop):
  1. Alice connects her Google Drive account
  2. Selects folder: "FlowPilot - ProjectX"
  3. Syncs 5 artifacts to that folder ✓

PC B (Bob's laptop):
  1. Bob tries to sync → ❌ Error: "Not connected"
  2. Bob goes to /artifacts
  3. Bob connects his Google Drive account
  4. Bob selects folder: "FlowPilot - ProjectX" (same folder Alice uses)
  5. Bob syncs new artifact → ✓ Success
  6. Both Alice and Bob can now see all artifacts in shared folder

Result:
  ✓ Both PCs syncing to same Google Drive folder
  ✓ Each PC using their own Google account (separate refresh tokens)
  ✓ Shared artifact storage location
```

**Key takeaway:** Supabase Storage provides zero-setup multi-PC sync (recommended default). Google Drive requires each PC to connect separately but offers flexibility for team members to use their own Google accounts.

#### Critical Implementation Detail: Shared Metadata Model

**Question:** If PC B connects to the SAME Google Drive account as PC A, can PC B see artifacts synced by PC A?

**Answer:** YES, but only if the artifact browser queries the shared `artifact_runs` table (not just local files).

**The Two Sources of Artifact Metadata:**

```
Source 1: Local artifacts (PC-specific)
  Location: .flowpilot/artifacts/<projectId>/...
  Contains: Artifacts created and synced by THIS PC only
  
Source 2: Shared metadata (cross-PC)
  Location: Supabase artifact_runs table
  Contains: ALL artifacts synced by ANY PC
  Updated: Required after every sync attempt
```

**What PC B sees depends on where the UI looks:**

| UI Query Source | PC B Sees |
|----------------|-----------|
| **Local only** (`.flowpilot/artifacts/`) | ❌ Only PC B's own artifacts (PC A's artifacts not visible) |
| **Shared metadata** (`artifact_runs` table) | ✅ ALL artifacts from all PCs |

**The correct implementation (based on Section 5.2 and Section 5.3):**

From **Section 5.2 - Local runner responsibility**:
> "Updates Supabase sync metadata after each success or failure."

The `artifact_runs` table is the required shared sync-state store.

**Complete flow for cross-PC visibility:**

```
PC A:
  1. Creates local artifact
  2. Syncs to Google Drive:
     - Uploads files to Drive folder
     - Gets Drive file ID (remoteObjectId)
  3. Updates artifact_runs table:
     INSERT INTO artifact_runs (
       id, project_id, title,
       remote_path, remote_object_id,
       storage_provider, sync_status
     ) VALUES (
       'artifact-1', 'abc-123', 'Response.md',
       'projects/abc/runs/run-1/steps/step-1/Response.md',
       'google-drive-file-id-xyz',
       'google_drive', 'synced'
     )
  ✓ Artifact now visible in shared metadata

PC B (after connecting to same Google Drive account + folder):
  1. Admin-web queries artifact_runs:
     SELECT * FROM artifact_runs
     WHERE project_id = 'abc-123'
     ORDER BY updated_at DESC
     
  2. Shows ALL artifacts including:
     ✓ Artifacts synced by PC A
     ✓ Artifacts synced by PC B
     ✓ Artifacts synced by any other PC
     
  3. User clicks "Open Artifact" on artifact synced by PC A
     
  4. PC B's runner generates access URL:
     - Reads remoteObjectId: 'google-drive-file-id-xyz'
     - Uses PC B's refresh token to get access token
     - Calls Drive API: files.get(fileId, fields='webViewLink')
     - Returns Drive URL or signed download link
     
  5. Browser opens artifact ✓
```

**Implementation requirements:**

1. **Sync operation must update both:**
   - ✅ Local manifest (`.flowpilot/artifacts/.../manifest.json`)
   - ✅ Shared metadata (`artifact_runs` table)

2. **Artifact browser must query:**
   - ✅ `artifact_runs` table (shows ALL synced artifacts)
   - ✅ NOT just local `.flowpilot/artifacts/` (PC-specific only)

3. **Open artifact operation must:**
   - ✅ Read `remoteObjectId` from `artifact_runs` table
   - ✅ Use current PC's credentials (refresh token) to generate access URL
   - ✅ Work even if artifact was synced by different PC

**Why this design works:**

```
Shared metadata model:
  ✓ artifact_runs table is the source of truth for "what's synced"
  ✓ Each PC has its own auth tokens but same view of synced artifacts
  ✓ Drive remoteObjectId (file ID) is stable across PCs
  ✓ Any PC with Drive access to same folder can open any artifact

Local artifacts remain PC-specific:
  ✓ .flowpilot/artifacts/ contains only this PC's local snapshots
  ✓ Used as the source for NEW syncs
  ✓ Not required for viewing already-synced artifacts
```

**Edge case - What if artifact_runs update fails?**

From **Section 5.3**:
> "Supabase is the durable shared sync-state store."

```
PC A syncs artifact:
  1. ✓ Uploads to Drive successfully
  2. ✓ Updates local manifest
  3. ❌ artifact_runs update fails (network issue)
  
Result:
  ✓ Uploaded bytes may exist remotely
  ✓ PC A keeps local manifest state for retry context
  ❌ PC B cannot see the artifact (not in shared metadata)
  ❌ Overall sync is not complete because Supabase state was not saved
  
Recovery:
  → Mark local sync as failed with the Supabase metadata error
  → PC A or server-start reconciliation retries sync later
  → artifact_runs gets updated
  → PC B can now see the artifact
```

Supabase metadata persistence is part of sync completion. If it fails, the artifact remains retryable and must not be presented as fully synced in shared UI state.

**Summary - Cross-PC artifact visibility:**

| Scenario | PC B Can See PC A's Artifacts? |
|----------|-------------------------------|
| **Supabase Storage** | ✅ YES (artifact_runs updated automatically) |
| **Google Drive** (PC B not connected) | ❌ NO (PC B has no Drive credentials) |
| **Google Drive** (PC B connected to same account + folder) | ✅ YES (artifact_runs shows all artifacts, PC B uses own tokens to access) |
| **Google Drive** (artifact_runs update failed) | ❌ NO (sync is incomplete; retry needed) |

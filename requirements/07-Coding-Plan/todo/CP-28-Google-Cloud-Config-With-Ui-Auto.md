# CP-28: Google Cloud Config With UI Auto

## Status

Planning.

## Depends On

- [CP-27: Google Cloud Setting](C:/working/flowpilot/requirements/07-Coding-Plan/todo/CP-27-Google-Cloud-Setting.md)
- [CP-05-03: Google Drive MCP](C:/working/flowpilot/requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md)
- [Task-023: Sync Artifact With Google Drive](C:/working/flowpilot/requirements/08-Task/Task-023-Sync-Artifact-With-Google.md)
- `Task-014 Supabase Connection Config` as the reference pattern for runner-managed local configuration

---

## 1. Goal

Create a FlowPilot setup UI that removes repeated local manual setup after the user finishes the required Google Cloud Console work.

This plan does not try to automate Google Cloud project creation, OAuth consent setup, API enablement, OAuth client creation, or API key creation. Those steps must still happen in Google Cloud Console.

This plan automates the local FlowPilot side:

- guide the user through the required Google Cloud Console steps
- collect the Google values/files after the user creates them
- save artifact-sync OAuth config locally through the runner
- save the Picker API key locally through the runner
- copy the MCP Desktop OAuth JSON into a stable local config path
- inject the MCP credential/token paths into the MCP process
- validate readiness for artifact sync and Google Drive MCP separately
- avoid repeated manual `.env` edits

The implementation should feel like the Supabase setup flow from `Task-014`: guided UI, runner-managed persistence, validation, and clear status.

---

## 2. Non-Goals

Do not build these in CP-28:

- automatic Google Cloud project creation
- automatic OAuth consent screen creation
- automatic Google API enablement
- automatic OAuth client creation
- automatic API key creation
- Google Cloud service-account setup
- storing Google refresh tokens in Supabase
- rewriting `.env` as the main product configuration path

Manual Google Cloud setup remains documented in CP-27. CP-28 only makes the local application setup productized and repeatable.

---

## 3. Key Design Decisions

### 3.1 Keep Google Cloud manual, automate local config

Google Cloud Console must remain manual because:

- users must own or choose their own Google Cloud project
- OAuth consent configuration is tied to the user's Google account/project
- Google controls the OAuth client and API key creation UI
- personal MVP usage does not justify a full Google Cloud provisioning integration

FlowPilot should still reduce friction after this point by saving local config and validating it.

### 3.2 Do not auto-edit `.env`

Do not implement this feature by writing `.env` automatically.

Reason:

- `.env` is repo-level developer configuration
- UI-driven `.env` writes are brittle on different machines
- it mixes local runtime state with project source configuration
- the repo already has a better pattern in the Supabase setup flow
- secrets should be managed by the runner secret store, not by browser code

Recommended behavior:

- runner-managed local config is the primary source
- environment variables remain a fallback for developer/manual setup
- UI can show an optional export/debug snippet later, but it must not depend on `.env`

### 3.3 One Google Cloud project can power both features

One Google Cloud project can be used for:

- artifact sync OAuth
- Picker API key
- Google Drive MCP OAuth

But it requires different credential types:

- artifact sync needs a Web OAuth client because the runner handles an HTTP callback
- Picker needs an API key because the browser Picker UI needs a browser-side key
- MCP usually needs a Desktop OAuth client because the MCP package performs local installed-app OAuth

This is still one Google Cloud project. It is not one credential.

### 3.4 Runtime config must actually be used by existing code

Current artifact Google Drive code reads env variables directly. CP-28 is incomplete unless implementation changes runtime config resolution.

Required rule:

- saved runner config + secret store is preferred when present and complete
- environment variables are used only as fallback

This affects artifact sync connect, OAuth callback, token refresh, Picker config, and Google Drive sync execution.

### 3.5 MCP upload must connect to MCP launch

Uploading `gcp-oauth.keys.json` is not enough by itself.

CP-28 must also ensure the runner passes these environment variables when launching or verifying the MCP package:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=/absolute/path/to/gcp-oauth.keys.json
GOOGLE_DRIVE_MCP_TOKEN_PATH=/absolute/path/to/tokens.json
```

On Windows example:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=C:\Users\dat.nguyen\.config\google-drive-mcp\gcp-oauth.keys.json
GOOGLE_DRIVE_MCP_TOKEN_PATH=C:\Users\dat.nguyen\.config\google-drive-mcp\tokens.json
```

---

## 4. Current Code Context

### 4.1 Supabase reference pattern

Use the existing Supabase config flow as the model.

Relevant runner pattern:

- `apps/local-runner/internal/runner/supabase_config.go`
- saves browser-safe config under `.flowpilot/settings`
- stores secrets through the runner secret store
- validates config before save
- rolls back secret writes if config persistence fails
- returns status without leaking secret values

Relevant admin-web pattern:

- `apps/admin-web/src/routes/setup.supabase.tsx`
- `apps/admin-web/src/app/api/runtime/supabase-config`

CP-28 should mirror this pattern for Google Drive.

### 4.2 Artifact sync current env-only areas

Current Google Drive artifact sync code has env-only config access.

Known areas to refactor:

- `apps/local-runner/internal/runner/artifact_google_drive_auth.go`
- `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
- `apps/local-runner/internal/runner/artifact_cloud_storage.go`

Current env values to replace with runner config resolution:

```env
GOOGLE_DRIVE_CLIENT_ID
GOOGLE_DRIVE_CLIENT_SECRET
GOOGLE_DRIVE_REDIRECT_URI
GOOGLE_PICKER_API_KEY
```

Required implementation change:

- create a runner method that resolves Google Drive artifact config from saved config/secret store first
- keep env fallback for developers and backward compatibility
- refactor current free functions or call sites so they can access runner-managed config

Expected precedence:

1. complete saved Google Drive runner config
2. complete environment variable config
3. missing-config error with clear field-level status

Important behavior:

- if saved config exists but is incomplete, report which saved fields are missing
- do not silently mix partial saved secrets with unrelated env secrets unless explicitly designed and documented
- fallback to env should mainly apply when no saved config exists

### 4.3 MCP current backend spec area

Current MCP backend spec is registered in:

- `apps/local-runner/internal/runner/runner.go`

Known MCP package:

```text
@piotr-agier/google-drive-mcp
```

CP-28 must extend the runner MCP launch/install/verify flow so Google Drive MCP receives:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=<resolved credential path>
GOOGLE_DRIVE_MCP_TOKEN_PATH=<resolved token path>
```

This should apply to:

- package verification
- backend startup
- any MCP auth helper action added by CP-05-03

---

## 5. User Experience Plan

### 5.1 Route

Recommended first route:

```text
/settings/google-drive-setup
```

Optional later unauthenticated route:

```text
/setup/google-drive
```

The first implementation should live inside authenticated settings unless the existing setup flow already supports unauthenticated runner configuration pages.

Current settings navigation also exposes a parent MCP page for related runtime management:

```text
/settings/mcp-servers
/settings/mcp-servers/jira-link
```

This parent page is a navigation surface for MCP-related settings, while the Google Drive setup page remains the guided CP-28 configuration flow.

### 5.2 Page sections

The page should show six sections:

1. Create Google Cloud project
2. Configure OAuth consent screen
3. Enable APIs
4. Create Web OAuth client for artifact sync
5. Create Desktop OAuth client for Google Drive MCP
6. Create API key for Picker

Behavior:

- sections 1 to 3 are guide-only
- sections 4 to 6 are guide plus local config actions
- every section shows status
- every local save action is independent
- user can leave and return without losing progress

### 5.3 Section status values

Use status values that are easy for the UI and runner to share:

```ts
type GoogleDriveSetupStepStatus =
  | "not_started"
  | "needs_input"
  | "configured"
  | "warning"
  | "failed";
```

Suggested status meaning:

- `not_started`: no local data exists
- `needs_input`: user must paste/upload something
- `configured`: required local config exists and basic validation passes
- `warning`: config exists but a deeper check is missing, such as MCP token not created yet
- `failed`: validation or file access failed

---

## 6. Detailed UI Step Plan

### 6.1 Step 1: Create Google Cloud project

Manual Google Cloud work:

1. Open Google Cloud Console.
2. Select or create one project for FlowPilot.
3. Confirm the active project in the top project selector.

FlowPilot UI:

- show guide text only
- show CP-27 reference link
- show a "Done, continue" button
- do not persist any local value

Validation:

- no runner validation
- UI-only completion flag may be stored in local UI state if useful

### 6.2 Step 2: Configure OAuth consent screen

Manual Google Cloud work:

1. Open Google Cloud Console.
2. Go to `Google Auth Platform`.
3. Open `Audience`.
4. Choose `External` for personal/friend testing unless the app is only for one Google Workspace organization.
5. In `Testing`, add test users for all Google accounts that need to connect.
6. Move to `Production` only when ready to avoid 7-day testing refresh-token expiration behavior.

FlowPilot UI:

- show guide text only
- explain `Testing` versus `Production`
- explain that refresh tokens can expire in Testing mode
- link to CP-27 for the complete Google Cloud guide

Validation:

- no direct runner validation is possible
- later OAuth failures should surface reconnect-required messages

### 6.3 Step 3: Enable APIs

Manual Google Cloud work:

Enable the APIs needed by the selected feature set.

Required for artifact sync:

- Google Drive API
- Google Picker API

Required for MCP if the MCP package accesses these file types:

- Google Drive API
- Google Docs API
- Google Sheets API
- Google Slides API

Optional if MCP tools need calendar access later:

- Google Calendar API

FlowPilot UI:

- show checklist
- show which APIs are required for which feature
- do not claim FlowPilot can verify API enablement until a real Google request is attempted

Validation:

- basic local validation only
- API-disabled errors from Google should be mapped to a clear setup error when feature calls fail

### 6.4 Step 4: Create Web OAuth client for artifact sync

Manual Google Cloud work:

1. Go to `Google Auth Platform > Clients`.
2. Click `Create client`.
3. Choose application type `Web application`.
4. Use a clear name, for example `FlowPilot Artifact Sync Local`.
5. Add this authorized redirect URI:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

6. Save.
7. Copy `Client ID`.
8. Copy `Client Secret`.

FlowPilot UI:

- show redirect URI as read-only with copy button
- show `Client ID` input
- show `Client Secret` password input
- show save button
- after save, clear the secret field and show only `configured` state

Runner save behavior:

- persist redirect URI in local config
- persist client ID and client secret in runner secret store
- never return client secret to browser after save

Runtime behavior after save:

- artifact sync OAuth connect must use saved client ID, client secret, and redirect URI
- OAuth callback token exchange must use saved client ID and client secret
- access-token refresh must use saved client ID and client secret
- environment variables remain fallback only if no saved config exists

### 6.5 Step 5: Create Desktop OAuth client for MCP

Manual Google Cloud work:

1. Go to `Google Auth Platform > Clients`.
2. Click `Create client`.
3. Choose application type `Desktop app`.
4. Use a clear name, for example `FlowPilot Google Drive MCP Local`.
5. Save.
6. Download the OAuth JSON file.

FlowPilot UI:

- show upload/dropzone for the downloaded JSON file
- show expected target path before upload
- show resolved credential path after upload
- show resolved token path
- show MCP auth status separately from credential-file status

Runner upload behavior:

- validate uploaded file is JSON
- validate it looks like a Google Desktop OAuth client JSON
- copy it to the resolved MCP credential path as `gcp-oauth.keys.json`
- ensure parent directory exists
- do not store the OAuth JSON inside the repo workspace

Default MCP credential path:

- Windows:
  - `%USERPROFILE%\.config\google-drive-mcp\gcp-oauth.keys.json`
- Linux/macOS:
  - `~/.config/google-drive-mcp/gcp-oauth.keys.json`

Default MCP token path:

- Windows:
  - `%USERPROFILE%\.config\google-drive-mcp\tokens.json`
- Linux/macOS:
  - `~/.config/google-drive-mcp/tokens.json`

Runtime behavior after save:

- runner must launch Google Drive MCP with `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
- runner must launch Google Drive MCP with `GOOGLE_DRIVE_MCP_TOKEN_PATH`
- token file may not exist until the user completes MCP auth
- missing token file is `needs_auth`, not credential setup failure

### 6.6 Step 6: Create API key for Picker

Manual Google Cloud work:

1. Go to `APIs & Services > Credentials`.
2. Click `Create credentials`.
3. Choose `API key`.
4. Name it clearly, for example `FlowPilot Picker API Key`.
5. In `APIs that can be accessed using this key`, choose API restriction.
6. Select only `Google Picker API`.
7. Do not check `Authenticate API calls through a service account`.
8. Under application restrictions, use the safest option that works for local MVP.
9. For easiest personal MVP testing, use `None`.
10. Save.
11. Copy the API key.

FlowPilot UI:

- show `Google Picker API Key` password input
- explain this key is used by the browser Picker UI, not by OAuth token refresh
- show save button
- after save, clear the key field and show only `configured` state

Runner save behavior:

- persist Picker API key in runner secret store
- never return key value after save

Runtime behavior after save:

- artifact sync frontend/runtime config endpoint must expose only what the browser needs
- if the browser needs the Picker API key to initialize Picker, return it only through a deliberate endpoint and treat it as browser-exposed by design
- do not confuse Picker API key with OAuth client secret

---

## 7. Persistence Model

### 7.1 Local config file

Recommended workspace config path:

```text
<workspace>/.flowpilot/settings/google-drive-config.json
```

Recommended file content:

```json
{
  "version": 1,
  "artifactSync": {
    "redirectUri": "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback",
    "hasClientId": true,
    "hasClientSecret": true,
    "hasPickerApiKey": true
  },
  "mcp": {
    "credentialPath": "C:\\Users\\dat.nguyen\\.config\\google-drive-mcp\\gcp-oauth.keys.json",
    "tokenPath": "C:\\Users\\dat.nguyen\\.config\\google-drive-mcp\\tokens.json"
  },
  "updatedAt": "2026-06-04T00:00:00Z"
}
```

Rules:

- file contains no raw client secret
- file contains no raw Picker API key unless the team explicitly decides this browser-exposed key can be plain config
- file contains no refresh token
- file contains paths and status metadata only

### 7.2 Runner secret store keys

Recommended secret keys:

```text
google-drive:artifact-sync:client-id
google-drive:artifact-sync:client-secret
google-drive:artifact-sync:picker-api-key
```

Notes:

- client ID is not highly sensitive, but storing it with the rest of the Google setup values keeps the implementation simple
- client secret must never be returned to admin-web after save
- Picker key may be browser-exposed for Picker, but should still not be casually returned in status responses

### 7.3 MCP local files

Recommended user-local directory:

```text
~/.config/google-drive-mcp
```

Required files:

```text
gcp-oauth.keys.json
tokens.json
```

Meaning:

- `gcp-oauth.keys.json` is the Desktop OAuth client credential JSON downloaded from Google Cloud
- `tokens.json` is created by the MCP auth flow after the user grants Google Drive permission

Important:

- `tokens.json` is not created during OAuth JSON upload
- `tokens.json` is machine/user-specific
- do not store it inside the repo
- do not sync it to Supabase

---

## 8. Runner API Plan

Implement a runner module similar to Supabase config.

Suggested file:

```text
apps/local-runner/internal/runner/google_drive_config.go
```

### 8.1 Runner data types

Suggested internal resolved config:

```go
type googleDriveArtifactRuntimeConfig struct {
    ClientID     string
    ClientSecret string
    RedirectURI  string
    PickerAPIKey string
    Source       string // "saved" or "env"
}
```

Suggested public status response:

```ts
type GoogleDriveConfigStatus = {
  artifactSync: {
    configured: boolean;
    source: "saved" | "env" | "missing";
    hasClientId: boolean;
    hasClientSecret: boolean;
    redirectUri: string | null;
    hasPickerApiKey: boolean;
    missingFields: string[];
  };
  mcp: {
    configured: boolean;
    needsAuth: boolean;
    credentialPath: string | null;
    credentialFileExists: boolean;
    credentialFileValid: boolean;
    tokenPath: string | null;
    tokenFileExists: boolean;
    backendPackageAvailable: boolean | null;
    missingFields: string[];
  };
  runnerReachable: boolean;
  lastError: string | null;
};
```

### 8.2 Runner endpoints

Suggested local runner routes:

```text
GET    /google-drive-config
PUT    /google-drive-config
POST   /google-drive-config/validate
POST   /google-drive-config/mcp-oauth-upload
POST   /google-drive-config/mcp-auth/start
DELETE /google-drive-config
```

Endpoint behavior:

- `GET /google-drive-config`
  - returns status only
  - does not return client secret
  - does not return Picker API key by default

- `PUT /google-drive-config`
  - saves artifact sync `clientId`, `clientSecret`, `redirectUri`
  - saves Picker `apiKey` if provided
  - allows partial section save but status must show missing fields

- `POST /google-drive-config/validate`
  - re-loads config from disk and secret store
  - validates artifact sync and MCP readiness
  - may run package-level MCP verification if safe

- `POST /google-drive-config/mcp-oauth-upload`
  - accepts uploaded Google Desktop OAuth JSON
  - validates JSON shape
  - copies to resolved credential path
  - returns status

- `POST /google-drive-config/mcp-auth/start`
  - starts the MCP auth flow if CP-05-03 supports a runner-managed auth action
  - must pass `GOOGLE_DRIVE_OAUTH_CREDENTIALS` and `GOOGLE_DRIVE_MCP_TOKEN_PATH`
  - returns an operation/status object, not token contents

- `DELETE /google-drive-config`
  - removes saved local config and related secrets
  - should not delete token file by default unless request includes an explicit `deleteMcpTokens` flag

### 8.3 Config resolution rules

Artifact sync config resolver must use this order:

1. saved runner config and secret store if complete
2. environment variables if no saved config exists
3. missing config error

Required env fallback variables:

```env
GOOGLE_DRIVE_CLIENT_ID
GOOGLE_DRIVE_CLIENT_SECRET
GOOGLE_DRIVE_REDIRECT_URI
GOOGLE_PICKER_API_KEY
```

Default redirect URI:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

Resolver rules:

- saved config must be validated as a unit
- do not combine a saved client ID with an env client secret unless explicitly supported
- error messages must list missing fields
- status response must show whether config came from `saved`, `env`, or `missing`

---

## 9. Artifact Sync Runtime Changes

### 9.1 Refactor env-only config access

Current helper functions that read env directly must be changed or wrapped so they can use runner-managed config.

Required implementation options:

- convert free helper functions into runner methods where they need config access
- or pass a resolved config object into the helper functions

Expected targets:

- OAuth authorization URL creation
- OAuth callback token exchange
- access token refresh
- Picker API key/config endpoint
- sync upload/download execution if it needs Google config

### 9.2 Required behavior after CP-28

After the user saves CP-28 config:

- artifact sync connect can start without `.env`
- OAuth callback can exchange code without `.env`
- token refresh can refresh access token without `.env`
- Picker can initialize without `.env`
- remote artifact tab behavior from Task-023 still filters by active storage provider

### 9.3 Token expiration handling remains required

CP-28 only configures the OAuth client values. It does not remove token lifecycle handling.

Task-023 still must implement/improve:

- if access token fails, try refresh token
- if refresh token is expired/revoked, mark connection as reconnect required
- UI should show reconnect form/action
- do not present this as a generic sync failure

---

## 10. MCP Runtime Changes

### 10.1 MCP credential path resolution

Runner should resolve these paths:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=<user-config-dir>/google-drive-mcp/gcp-oauth.keys.json
GOOGLE_DRIVE_MCP_TOKEN_PATH=<user-config-dir>/google-drive-mcp/tokens.json
```

Windows example:

```text
C:\Users\dat.nguyen\.config\google-drive-mcp\gcp-oauth.keys.json
C:\Users\dat.nguyen\.config\google-drive-mcp\tokens.json
```

Linux/macOS example:

```text
~/.config/google-drive-mcp/gcp-oauth.keys.json
~/.config/google-drive-mcp/tokens.json
```

### 10.2 MCP launcher env injection

When the runner launches or verifies `@piotr-agier/google-drive-mcp`, it must include:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=<resolved credential path>
GOOGLE_DRIVE_MCP_TOKEN_PATH=<resolved token path>
```

Apply this to:

- install verification command
- runtime MCP process
- any explicit auth command

### 10.3 MCP auth state

MCP setup has two separate states:

- credential file exists
- user has completed auth and token file exists

UI must not mark MCP fully connected just because the OAuth JSON file exists.

Status rules:

- missing credential JSON: `needs_input`
- credential JSON exists but token file missing: `needs_auth`
- token file exists but backend verification fails: `failed`
- token file exists and backend verification passes: `configured`

### 10.4 Token expiration handling remains required

CP-05-03 still must implement:

- access-token refresh using refresh token when possible
- reconnect-required state when refresh token is expired or revoked
- clear UI/action to rerun MCP auth
- no storage of MCP tokens in Supabase or project source files

---

## 11. Admin-Web API Plan

Mirror the Supabase runtime-config API pattern.

Suggested admin API routes:

```text
GET    /api/runtime/google-drive-config
PUT    /api/runtime/google-drive-config
POST   /api/runtime/google-drive-config/validate
POST   /api/runtime/google-drive-config/mcp-oauth-upload
POST   /api/runtime/google-drive-config/mcp-auth/start
DELETE /api/runtime/google-drive-config
```

Rules:

- proxy to local runner
- do not persist secrets in admin-web
- do not log secret values
- return browser-safe status
- upload route should stream file content to runner or forward multipart safely
- routes should work the same way as Supabase runtime config routes

Required frontend behavior:

- after every save/upload/auth action, re-fetch status
- handle runner unreachable with a setup error
- show field-level errors from runner

---

## 12. UI Component Plan

Suggested route file:

```text
apps/admin-web/src/routes/settings.google-drive-setup.tsx
```

Use existing routing conventions if the route naming differs.

Required components:

- Google Drive setup page shell
- MCP Servers parent page
- Jira MCP link page
- guide section card
- step status badge
- artifact sync credential form
- Picker API key form
- MCP OAuth JSON upload area
- MCP auth action/status panel
- validation results panel

Required UI states:

- runner unreachable
- config missing
- partial artifact sync config
- artifact sync configured
- MCP credential missing
- MCP credential uploaded but auth missing
- MCP configured
- validation failed with actionable message
- MCP settings page overview with child links to Driver and Jira runtime pages

Important UI copy:

- "Google Cloud setup is manual."
- "FlowPilot saves the local values after you create them in Google Cloud."
- "The Web OAuth client is for artifact sync."
- "The Desktop OAuth client JSON is for Google Drive MCP."
- "The Picker API key is not the OAuth client secret."
- "MCP Servers is the parent settings surface for Google Drive runtime links."

---

## 13. Validation Plan

### 13.1 Artifact sync validation checks

Required checks:

- `artifact_client_id_present`
- `artifact_client_secret_present`
- `artifact_redirect_uri_present`
- `artifact_redirect_uri_format`
- `artifact_redirect_uri_matches_runner_callback`
- `picker_api_key_present`
- `saved_config_reloadable`
- `secret_store_values_reloadable`
- `runtime_config_resolves`

Optional deeper checks:

- start OAuth connect URL generation
- verify generated Google OAuth URL includes expected redirect URI and scopes

Do not require a live Google token for basic local config validation.

### 13.2 MCP validation checks

Required checks:

- `mcp_credential_path_resolved`
- `mcp_credential_file_exists`
- `mcp_credential_file_readable`
- `mcp_credential_json_shape_valid`
- `mcp_token_path_resolved`
- `mcp_token_parent_dir_writable`
- `mcp_token_file_exists`

Recommended deeper checks:

- run `npx -y @piotr-agier/google-drive-mcp --help`
- run the existing runner MCP backend verification path
- if token file exists, attempt lightweight MCP startup if safe

Validation result must distinguish:

- missing credential file
- invalid credential JSON
- missing token file / auth not completed
- package unavailable
- auth expired / reconnect required

---

## 14. Implementation Phases

### Phase 1: Runner persistence and status API

Goal:

- add Google Drive local config persistence similar to Supabase config

Tasks:

- create `google_drive_config.go`
- define config/status structs
- write/read `.flowpilot/settings/google-drive-config.json`
- save/load Google secrets in runner secret store
- implement local config status
- implement field-level validation
- add route handlers for `GET`, `PUT`, `POST validate`, and `DELETE`

Done when:

- runner can save Google artifact sync values
- runner can report browser-safe status
- runner does not return secrets
- env fallback status is represented

### Phase 2: Artifact sync config resolver refactor

Goal:

- make existing artifact sync code use saved Google Drive config

Tasks:

- add `resolveGoogleDriveArtifactRuntimeConfig`
- refactor env-only helper usage
- update OAuth connect flow to use resolved config
- update OAuth callback token exchange to use resolved config
- update token refresh to use resolved config
- update Picker config access to use resolved config
- preserve env fallback when no saved config exists

Done when:

- artifact sync works without Google `.env` values after CP-28 config is saved
- old `.env` setup still works when no saved config exists
- missing fields return clear setup errors

### Phase 3: MCP OAuth JSON upload and path management

Goal:

- make the UI-uploaded Desktop OAuth JSON usable by MCP

Tasks:

- implement `POST /google-drive-config/mcp-oauth-upload`
- validate uploaded JSON shape
- create `~/.config/google-drive-mcp` if missing
- copy file as `gcp-oauth.keys.json`
- store/report resolved credential path
- store/report resolved token path
- detect token file existence

Done when:

- credential JSON upload creates the expected file
- status shows credential and token paths
- status distinguishes credential-ready from auth-ready

### Phase 4: MCP launcher integration

Goal:

- ensure MCP receives the uploaded credential path and token path

Tasks:

- update Google Drive MCP backend spec/environment builder
- pass `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
- pass `GOOGLE_DRIVE_MCP_TOKEN_PATH`
- apply env to verify/start/auth commands
- add or connect `POST /google-drive-config/mcp-auth/start` if CP-05-03 uses runner-managed auth
- map MCP auth/token errors to reconnect-required status

Done when:

- MCP launch/verify uses CP-28 paths
- token missing is shown as auth-needed, not generic failure
- expired/revoked token is shown as reconnect required

### Phase 5: Admin-web runtime API proxy

Goal:

- expose runner Google Drive config actions to the UI

Tasks:

- add `/api/runtime/google-drive-config` routes
- proxy `GET`, `PUT`, `POST validate`, `POST upload`, `POST auth/start`, and `DELETE`
- preserve multipart upload if used
- hide secret values
- handle runner unreachable cleanly

Done when:

- admin-web can save, upload, validate, and delete through runner
- no secret is logged or returned in status

### Phase 6: Settings UI

Goal:

- implement the guided Google Drive setup page

Tasks:

- create `/settings/google-drive-setup`
- add six guided sections
- add artifact sync credential form
- add Picker API key form
- add MCP OAuth JSON upload
- add MCP auth/status panel
- add validation panel
- add copy buttons for redirect URI and paths
- reload status after each action

Done when:

- user can follow CP-27 in UI
- user can complete local setup without editing `.env`
- user sees exactly what remains missing

### Phase 7: Tests and verification

Goal:

- prove the config flow is reliable before implementation is considered complete

Tasks:

- unit test config file save/load
- unit test secret store save/load behavior
- unit test env fallback behavior
- unit test no-secret status response
- unit test artifact sync resolver precedence
- unit test invalid/missing MCP JSON validation
- unit test MCP token missing status
- integration test runner config endpoints if existing test pattern supports it
- frontend test status rendering and form submission if existing pattern supports it

Done when:

- tests cover saved config, env fallback, missing config, invalid MCP JSON, and secret hiding
- manual verification confirms artifact sync and MCP read the saved config paths

---

## 15. Security Rules

Do:

- keep secrets in runner-managed local storage
- keep MCP OAuth JSON in user-local config storage
- keep token files local to the current machine/user
- validate uploaded file shape before copying
- hide stored secrets in all status responses
- avoid logging credentials or token file contents

Do not:

- rewrite source-controlled `.env` as primary configuration
- return stored client secret to the browser after save
- store Google refresh tokens in Supabase
- store MCP token file inside the repo workspace
- treat an API key as an OAuth client secret
- treat OAuth JSON upload as completed MCP auth

---

## 16. Error Handling Requirements

Artifact sync setup errors:

- missing client ID
- missing client secret
- missing redirect URI
- redirect URI mismatch
- missing Picker API key
- Google OAuth callback rejected
- refresh token expired or revoked
- Google API disabled

MCP setup errors:

- missing OAuth JSON
- invalid OAuth JSON
- credential path not writable
- token path parent not writable
- token missing / auth not completed
- token expired or revoked
- MCP package not installed or cannot run

UI behavior:

- errors must say which step fixes the issue
- reconnect-required must show a reconnect/auth action
- do not show raw Google tokens or secrets

---

## 17. Acceptance Criteria

CP-28 is complete when:

- `[Done]` user can open one guided FlowPilot page for Google Drive setup
- `[Done]` user can open the MCP Servers parent settings page for related runtime links
- `[Done]` steps 1 to 3 provide clear Google Cloud guide text from CP-27
- `[Done]` step 4 lets the user paste artifact-sync Web OAuth client ID and secret
- `[Done]` step 4 saves artifact-sync config locally through the runner
- `[Done]` step 4 makes current artifact sync runtime use saved config without `.env`
- `[Done]` step 5 lets the user upload Desktop OAuth JSON for MCP
- `[Done]` step 5 copies the file into the correct OS-specific MCP path
- `[Done]` step 5 reports credential-file status and token/auth status separately
- `[Done]` step 6 lets the user paste the Picker API key
- `[Done]` step 6 saves Picker API key locally through the runner
- `[Done]` Google Drive MCP launch/verify receives `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
- `[Done]` Google Drive MCP launch/verify receives `GOOGLE_DRIVE_MCP_TOKEN_PATH`
- `[Done]` runner can report readiness status for artifact sync and MCP separately
- `[Done]` artifact sync still supports existing env-based setup when no saved config exists
- `[Done]` secrets are not returned to the browser after save
- `[Done]` missing/expired tokens show reconnect-required state instead of generic failure
- `[Done]` UI clearly shows what is still missing before Google Drive features can run
- `[Done]` tests cover config save/load, env fallback, secret hiding, MCP JSON validation, and runtime resolver behavior

---

## 18. Test Guide

Use this guide to test the implemented CP-28 flow yourself.

### 18.1 Prerequisites

Before testing in FlowPilot, prepare the Google Cloud values first. Use this exact sequence.

1. Create or select one Google Cloud project.
   - Open [Google Cloud Console](https://console.cloud.google.com/).
   - Use the project selector at the top.
   - If you do not already have one, click `New Project`.
   - Give it a clear name such as `FlowPilot Local`.
   - Wait until the project is created, then make sure it is the active project.

2. Configure the OAuth consent screen.
   - In Google Cloud Console, open `Google Auth Platform`.
   - Open `Audience`.
   - Choose `External` for personal MVP / friend testing.
   - Fill the required app information.
   - Add every Google account you will use under test users if the app is still in Testing mode.
   - If you want refresh tokens to stop expiring every 7 days during testing, move the app to `Production` when ready.

3. Enable the required APIs.
   - In Google Cloud Console, open `APIs & Services > Library`.
   - Enable `Google Drive API`.
   - Enable `Google Picker API`.
   - If you want broader Google Drive MCP coverage, also enable:
     - `Google Docs API`
     - `Google Sheets API`
     - `Google Slides API`
   - Optional later:
     - `Google Calendar API`

4. Create the Web OAuth client for artifact sync.
   - Open `Google Auth Platform > Clients`.
   - Click `Create client`.
   - Choose `Web application`.
   - Example name: `FlowPilot Artifact Sync Local`.
   - Under redirect URIs, add:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

   - Save the client.
   - Copy these two values:
     - `Client ID`
     - `Client Secret`
   - These two values are used in FlowPilot step 4.

5. Create the Desktop OAuth client for Google Drive MCP.
   - Open `Google Auth Platform > Clients`.
   - Click `Create client`.
   - Choose `Desktop app`.
   - Example name: `FlowPilot Google Drive MCP Local`.
   - Save the client.
   - Download the OAuth JSON file from this Desktop client.
   - Do not use the Web client JSON here.
   - This downloaded JSON file is used in FlowPilot step 5.

6. Create the Google Picker API key.
   - Open `APIs & Services > Credentials`.
   - Click `Create Credentials > API key`.
   - Example name: `FlowPilot Picker API Key`.
   - In `APIs that can be accessed using this key`, restrict it to `Google Picker API`.
   - Do not enable `Authenticate API calls through a service account`.
   - For simplest local MVP testing, keep application restriction as `None`.
   - Save the key.
   - Copy the API key value.
   - This API key is used in FlowPilot step 6.

You should now have these three inputs ready for FlowPilot:

1. Web OAuth `Client ID`
2. Web OAuth `Client Secret`
3. Desktop OAuth JSON file
4. Google Picker API key

Also make sure:

- local runner is running
- admin-web is running
- Node.js and `npx` are available on the machine if you want to test MCP

### 18.2 Open the setup page

1. Start FlowPilot locally.
2. Open:

```text
/settings/google-drive-setup
```

3. Confirm you can see:
   - guide sections for steps 1 to 3
   - artifact sync form
   - MCP OAuth JSON upload section
   - Picker API key section
   - current status panel

Expected result:

- page loads successfully
- runner status is shown
- MCP credential path and token path are shown

### 18.3 Test artifact sync config save

1. In step 4, paste:
   - Web OAuth client ID
   - Web OAuth client secret
   - redirect URI
2. Click `Validate setup`.
3. Click `Save artifact sync`.

Expected result:

- validation passes for artifact client ID, client secret, redirect URI, and Picker key if already provided
- save succeeds
- client secret input is cleared after save
- page status shows artifact sync is configured or partially configured depending on whether Picker key is already saved

### 18.4 Test Picker API key save

1. In step 6, paste the Picker API key.
2. Click `Save picker key`.

Expected result:

- save succeeds
- Picker API key input is cleared after save
- artifact sync status becomes fully configured if step 4 was already completed

### 18.5 Test MCP OAuth JSON upload

1. In step 5, upload the Desktop OAuth JSON downloaded from Google Cloud.
2. Confirm the uploaded JSON is from the Desktop OAuth client, not the Web OAuth client.

Expected result:

- upload succeeds
- status shows the MCP credential file exists
- status shows MCP token path
- MCP usually shows `needs_auth` until tokens are created

Negative test:

1. Try uploading the Web OAuth JSON instead.

Expected result:

- upload is rejected
- MCP credential status does not move to configured

### 18.6 Test MCP auth-needed state

1. Upload the Desktop OAuth JSON.
2. Do not authenticate MCP yet.
3. Click `Refresh MCP status`.

Expected result:

- MCP status shows `needs_auth`
- UI message tells you auth still needs to be completed

### 18.7 Test MCP configured state

1. Complete the Google Drive MCP auth flow so `tokens.json` is created.
2. Return to the setup page.
3. Click `Refresh MCP status`.

Expected result:

- MCP status becomes `configured` if token validation succeeds
- runner reports MCP as ready

### 18.8 Test artifact sync real connection flow

1. Open a project that supports artifact sync.
2. Change artifact storage to Google Drive.
3. Start the connect flow for Google Drive.
4. Sign in with Google.
5. Use Picker to choose a Drive folder.
6. Run a workflow that produces artifacts.
7. Sync artifacts.

Expected result:

- OAuth connect starts without requiring `.env`
- folder selection succeeds
- artifact sync succeeds
- files appear under the selected Google Drive folder

### 18.9 Test reconnect-required behavior

You should test both artifact sync and MCP reconnect handling.

Artifact sync reconnect test:

1. Connect Google Drive successfully first.
2. Revoke the app or refresh token from Google Account permissions, or replace the stored token with an invalid one.
3. Try to sync again or fetch a Picker token again.

Expected result:

- operation fails with reconnect-required behavior
- connection/session status moves to `reconnect_required`
- UI should not show only a generic failure

MCP reconnect test:

1. Complete MCP auth successfully first.
2. Revoke the app from Google Account permissions, or delete/replace the valid refresh token in `tokens.json`.
3. Click `Refresh MCP status`.

Expected result:

- MCP status becomes `reconnect_required`
- UI indicates you must authenticate again

### 18.10 Test env fallback

This checks backward compatibility when no saved runner config exists.

1. Reset Google Drive config from the setup page.
2. Set these env vars manually before starting the runner:

```env
GOOGLE_DRIVE_CLIENT_ID=...
GOOGLE_DRIVE_CLIENT_SECRET=...
GOOGLE_DRIVE_REDIRECT_URI=http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
GOOGLE_PICKER_API_KEY=...
```

3. Start the runner again.
4. Reload the setup page.

Expected result:

- artifact sync status source is env fallback
- artifact sync can still work without saved runner config

### 18.11 Test saved-config precedence over env

1. Keep the env vars set.
2. Save a different Google Drive config through the setup UI.
3. Run the artifact connect flow again.

Expected result:

- saved runner config is used instead of env fallback
- artifact sync behavior matches the saved UI config, not the old env values

### 18.12 Files you can inspect manually

If you want to inspect what FlowPilot created locally:

- workspace config:

```text
<workspace>/.flowpilot/settings/google-drive-config.json
```

- MCP credential file:

```text
~/.config/google-drive-mcp/gcp-oauth.keys.json
```

- MCP token file:

```text
~/.config/google-drive-mcp/tokens.json
```

On Windows, these usually resolve to paths like:

```text
C:\Users\<your-user>\.config\google-drive-mcp\gcp-oauth.keys.json
C:\Users\<your-user>\.config\google-drive-mcp\tokens.json
```

What to confirm:

- workspace config file exists
- workspace config file does not contain raw client secret
- workspace config file does not contain raw Picker API key
- MCP files are stored in the user config directory, not inside the repo workspace

### 18.13 Current verification note

Already verified in implementation work:

- targeted runner tests for Google Drive config, runtime resolution, reconnect handling, and MCP validation passed

Not yet fully verified by automated frontend coverage in this task:

- full admin-web test/build pass for the setup page
- full manual end-to-end UI smoke test across every branch above
## 19. Recommended Execution Order

1. Implement runner config persistence and status API.
2. Refactor artifact sync to use runner-resolved config with env fallback.
3. Implement MCP OAuth JSON upload and path status.
4. Inject MCP credential/token paths into Google Drive MCP launch/verify/auth.
5. Add admin-web runtime API proxy.
6. Add `/settings/google-drive-setup` UI.
7. Add validation panel and tests.

This order avoids building a UI that saves values but does not affect the actual artifact sync or MCP runtime.

---

## 20. Product Note

The key improvement is not "automate Google Cloud".

The key improvement is:

- keep Google Cloud manual
- automate local FlowPilot configuration after Google Cloud is done
- ensure saved config is actually consumed by artifact sync and MCP
- make setup behave like a product flow instead of a developer `.env` ritual

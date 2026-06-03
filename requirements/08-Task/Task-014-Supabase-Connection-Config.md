# Task-014: Supabase Connection Config

**Status:** planned
**Type:** product + platform foundation
**Primary modules:** `apps/admin-web`, `apps/local-runner`

---

## 1. Goal

Remove the current requirement to rely on hard-coded Supabase connection values in `.env` for normal product usage.

The user should be able to:

1. open a Supabase connection settings page,
2. enter the FlowPilot workspace's Supabase values,
3. validate and save them,
4. use FlowPilot against that configured Supabase project without editing source-controlled env files.

Runtime resolution must prefer saved config first, and only fall back to `.env` when config is absent.

The saved Supabase connection is **local to each PC**. It is workstation-specific runtime configuration, not shared project data.

This page must be reachable:

- from the authenticated Settings menu,
- from the unauthenticated login flow before sign-in.

---

## 2. Requirement Summary

Current MVP assumption:

```env
SUPABASE_API_URL=Value
SUPABASE_API_KEY=Value
SUPABASE_API_EDGE_FUNCTION_URL=Value
SUPABASE_SERVICE_ROLE_KEY=Value
```

Desired behavior:

- each user can bring their own Supabase project,
- connection values are entered in the UI instead of being hard-coded in `.env`,
- saved values become the active backend connection for that local PC/workspace,
- FlowPilot can then operate on that user's own scoped data.

Important clarification for MVP:

- "scoped-data" should mean **workspace-scoped active Supabase connection**, not per-FlowPilot-project connection switching.
- One workspace should have one active Supabase connection at a time.
- That connection is stored locally on the current PC.
- Per-project or multi-profile switching is a later phase.

Reason:

- the current architecture assumes a single active Supabase bootstrap path,
- changing that to per-project switching would have a much larger blast radius across auth, gateway factories, realtime subscriptions, and API handlers.

---

## 3. 4C Summary

### 3.1 Context

Relevant current code:

- browser env access: `apps/admin-web/src/lib/env/browser-env.ts`
- server env access: `apps/admin-web/src/lib/env/app-env.ts`
- browser client bootstrap: `apps/admin-web/src/data/supabase/client.ts`
- server client bootstrap: `apps/admin-web/src/data/datasource/supabase/client.ts`
- server gateway selection: `apps/admin-web/src/data/repository/factory.ts`
- login page: `apps/admin-web/src/routes/login.tsx`
- auth bootstrap: `apps/admin-web/src/features/auth/auth-provider.tsx`
- auth route guard: `apps/admin-web/src/features/auth/require-auth.ts`
- settings navigation: `apps/admin-web/src/components/layout/app-nav.ts`
- existing local persisted settings pattern: `apps/local-runner/internal/runner/artifacts.go`
- existing local secret storage pattern: `apps/local-runner/internal/runner/secret_store.go`

### 3.2 Command

Build a new Supabase connection configuration flow with:

- persistent storage,
- validation,
- login-page access,
- authenticated settings-page access,
- runtime bootstrap support for both browser and server code.

### 3.3 Constraints

- `SUPABASE_SERVICE_ROLE_KEY` must never be exposed to the browser UI after save.
- The current app supports demo mode when Supabase is unavailable; this fallback should remain.
- Existing `.env` support should remain as a fallback for local development and CI.
- The current Supabase bootstrap entrypoints have a large blast radius.

Impact findings from GitNexus:

- `createSupabaseBrowserClient` upstream impact: `CRITICAL`
  - direct callers: `11`
  - affected processes: `19`
- `createGatewayBundle` upstream impact: `CRITICAL`
  - direct callers: `40`
  - affected processes: `36`

This means the implementation must be staged carefully around bootstrap boundaries, not scattered feature-by-feature.

### 3.4 Criteria

Done means:

- a new user can configure Supabase from the UI without editing `.env`,
- the login screen can guide an unconfigured workspace to setup,
- saved config becomes the active local-PC workspace backend,
- browser code only receives browser-safe connection values,
- server-only flows can still use the service-role key,
- existing env-based developer workflow still works,
- demo mode still works when no env and no saved connection exist.

---

## 4. Design Options

| Option | Summary | Pros | Cons |
|---|---|---|---|
| A. UI writes `.env` | Settings page edits workspace `.env` file directly | Minimal new runtime plumbing | Brittle, source-control pollution, poor UX, restart-heavy, risky on shared repos |
| B. Workspace config + secret store | Save browser-safe config in workspace settings JSON and server-only secret in keyring/local runner | Best fit for current architecture, secure enough, no repo pollution, reusable for future connection screens | Requires bootstrap refactor |
| C. Save config in Supabase itself | Store connection info inside a Supabase table | Centralized after bootstrap | Circular dependency: cannot use Supabase until Supabase is configured |

### Recommendation

Choose **Option B**. -> APPROVED

This is the only option that:

- works before login,
- avoids editing repo env files,
- avoids circular bootstrap,
- keeps configuration local to each machine,
- fits existing local-runner settings and secret-store patterns.

---

## 5. Recommended MVP Scope

### In scope

- one active workspace-level Supabase connection
- local per-PC persistence for that connection
- a new Supabase settings page in authenticated Settings
- a setup entry from the login page when Supabase is not configured
- local persistence for non-secret config
- secure local persistence for the service-role key
- runtime config resolution for browser and server code
- validation and status display
- fallback to demo mode when nothing is configured

### Out of scope

- multiple saved Supabase profiles
- per-project Supabase switching
- team/shared remote connection registry
- automatic migration of existing `.env` into saved settings
- rotating service-role keys from the browser after save

---

## 6. Data Model

### 6.1 Workspace config file

Store browser-safe and metadata fields in:

`<workspace>/.flowpilot/settings/supabase-config.json`

This file is local runtime state for the current PC. It must not be treated as shared team configuration.

Example shape:

```json
{
  "version": 1,
  "apiUrl": "https://xyzcompany.supabase.co",
  "anonKey": "eyJ...",
  "edgeFunctionUrl": "https://xyzcompany.supabase.co/functions/v1",
  "projectRef": "xyzcompany",
  "status": "configured",
  "updatedAt": "2026-06-03T00:00:00.000Z"
}
```

Notes:

- `anonKey` is browser-safe and may be stored here.
- `projectRef` can be derived from `apiUrl` for UI display.
- `status` is optional convenience metadata, not source of truth.
- the file should be ignored from version control if `.flowpilot/settings/` is not already ignored.

### 6.2 Secret storage

Store the service-role key in the existing local-runner secret store using a normalized key such as:

`supabase:workspace:service-role-key`

Do not return this value from browser-facing APIs.
This secret is also local to the current PC.

### 6.3 Runtime status response

Provide a browser-safe bootstrap payload:

```ts
type SupabaseRuntimeStatus = {
  mode: "config" | "env" | "demo";
  configured: boolean;
  apiUrl: string | null;
  anonKey: string | null;
  edgeFunctionUrl: string | null;
  hasServiceRoleKey: boolean;
  projectRef: string | null;
};
```

---

## 7. Resolution Rules

Use this precedence everywhere:

1. saved workspace config
2. explicit process env
3. demo mode fallback

Reason:

- product users should rely on saved settings by default,
- `.env` remains available as a compatibility fallback,
- each PC can keep its own local Supabase connection without affecting other machines,
- the app remains usable in demo mode when neither exists.

Apply the same precedence to:

- browser Supabase bootstrap
- server Supabase bootstrap
- login mode selection
- gateway bundle selection

---

## 8. UX Flow

### 8.1 First-time workspace

1. user opens FlowPilot
2. app detects no saved workspace config and no env fallback
3. login screen shows "Configure Supabase" instead of only sign-in inputs
4. user opens `/setup/supabase`
5. user enters:
   - Supabase URL
   - Supabase anon key
   - Edge Function URL
   - Service role key
6. user clicks `Validate`
7. user clicks `Save`
8. app reloads runtime config
9. user returns to login and signs in

Result:

- that PC now uses its locally saved Supabase connection,
- another PC can configure a different Supabase project independently.

### 8.2 Authenticated workspace

1. user opens `Settings > Supabase`
2. page shows current source:
   - `workspace config`
   - `env fallback`
   - `demo mode`
3. user can update values and revalidate
4. app prompts for reload after save because browser bootstrap client must be recreated safely

### 8.3 Failure states

- invalid URL format
- missing anon key
- missing service-role key
- unreachable Supabase URL
- invalid service-role key
- edge URL mismatch with API URL

Each should render an actionable message, not a generic "save failed".

---

## 9. Backend Implementation Plan

### 9.1 Local runner: persisted config support

Add a dedicated runner module for Supabase workspace config, following the same persistence pattern as storage-driver settings.

Suggested new file:

- `apps/local-runner/internal/runner/supabase_config.go`

Responsibilities:

- load saved workspace config JSON
- save workspace config JSON
- delete/reset workspace config JSON
- save/load/delete service-role key via `SecretStore`
- derive `projectRef` from URL
- validate config shape before persistence
- keep all persisted connection data local to the current workstation

Suggested functions:

```go
type SupabaseWorkspaceConfig struct {
    Version         int    `json:"version"`
    APIURL          string `json:"apiUrl"`
    AnonKey         string `json:"anonKey"`
    EdgeFunctionURL string `json:"edgeFunctionUrl"`
    ProjectRef      string `json:"projectRef,omitempty"`
    UpdatedAt       string `json:"updatedAt"`
}
```

```go
func (r *Runner) LoadSupabaseWorkspaceConfig() (SupabaseWorkspaceConfig, error)
func (r *Runner) SaveSupabaseWorkspaceConfig(input SupabaseWorkspaceConfig, serviceRoleKey string) error
func (r *Runner) ResetSupabaseWorkspaceConfig() error
func (r *Runner) ValidateSupabaseWorkspaceConfig(input SupabaseWorkspaceConfig, serviceRoleKey string) (SupabaseValidationResult, error)
```

### 9.2 Local runner HTTP surface

Expose runner endpoints for the admin app to call.

Suggested routes:

- `GET /supabase-config`
- `PUT /supabase-config`
- `POST /supabase-config/validate`
- `DELETE /supabase-config`

Response rules:

- `GET` returns browser-safe config plus `hasServiceRoleKey`
- `PUT` accepts service-role key but never returns it
- `DELETE` removes both JSON config and stored secret

### 9.3 Validation rules

Validation should include:

- `apiUrl` parses as HTTPS URL
- `anonKey` is non-empty
- `edgeFunctionUrl` parses as HTTPS URL
- `edgeFunctionUrl` belongs to the same Supabase project as `apiUrl`
- service-role key is non-empty
- optional connectivity checks:
  - create anon client and verify base reachability
  - create admin client and perform a harmless auth-admin verification call

Validation should not depend on application tables already existing.

If saved config exists and validates, it should be treated as the active runtime source even when `.env` is also present.

---

## 10. Admin-Web Implementation Plan

### 10.1 Add runtime config API layer

Admin-web needs its own runtime-facing API surface because the login/setup page must work before authenticated app flows.

Suggested route family:

- `apps/admin-web/src/app/api/runtime/supabase-config/route.ts`
- `apps/admin-web/src/app/api/runtime/supabase-config/validate/route.ts`

Behavior:

- proxy to local-runner Supabase config endpoints, or
- resolve directly from workspace config if that is simpler for server runtime

Security rule:

- browser-safe fields may be returned
- service-role key must never be returned

### 10.2 Replace env-only browser bootstrap

Current problem:

- `apps/admin-web/src/data/supabase/client.ts` creates a browser client from `import.meta.env` at module load time
- this cannot support runtime-entered config safely

Required refactor:

- replace import-time singleton assumptions with lazy runtime bootstrap
- load browser-safe config from runtime API before creating the browser client
- cache the browser client after bootstrap
- add a reset path after config save so a page reload recreates the client cleanly

Suggested shape:

```ts
type BrowserSupabaseConfig = {
  apiUrl: string;
  anonKey: string;
  edgeFunctionUrl: string;
};

async function loadBrowserSupabaseConfig(): Promise<BrowserSupabaseConfig | null>
function getBrowserSupabaseClient(): Promise<SupabaseClient | DemoClient>
function resetBrowserSupabaseClient(): void
```

### 10.3 Replace env-only server bootstrap

Refactor:

- `apps/admin-web/src/lib/env/app-env.ts`
- `apps/admin-web/src/data/datasource/supabase/client.ts`
- `apps/admin-web/src/lib/supabase/admin.ts`

Server runtime must resolve:

- workspace config + stored service-role key first
- otherwise env fallback
- otherwise demo mode or explicit failure depending on call site

### 10.4 Refactor auth mode detection

Current mode detection is mostly:

- `hasSupabaseEnv()` => Supabase mode
- otherwise demo mode

Replace with:

- `hasSupabaseRuntimeConfigOrEnvFallback()` => Supabase mode
- otherwise demo mode

This change must be applied consistently to:

- `apps/admin-web/src/features/auth/auth-provider.tsx`
- `apps/admin-web/src/features/auth/require-auth.ts`
- `apps/admin-web/src/routes/login.tsx`
- `apps/admin-web/src/data/auth/session.ts`

### 10.5 Add setup and settings pages

#### Detailed UI Specifications:

1. **Unauthenticated Connection Setup Page**
   - **Route File**: `apps/admin-web/src/routes/setup.supabase.tsx`
   - **Path**: `/setup/supabase`
   - **Visual Layout**: A premium centered card panel mimicking the style of `login.tsx` with a noise background, rounded borders (`rounded-[2rem]`), and a backdrop blur panel. Includes clear titles ("Configure Supabase Workspace") and a description ("Configure a dedicated Supabase connection to store and persist workspace data").
   - **State and Fields**:
     - `apiUrl`: HTTPS URL input. Typing a URL will automatically append `/functions/v1` and auto-populate the Edge Function URL unless overridden by the user.
     - `anonKey`: Password-masked toggleable text input.
     - `edgeFunctionUrl`: HTTPS URL input.
     - `serviceRoleKey`: Password-masked toggleable text input with a security badge warning that this key must remain confidential.
   - **Interactive Actions**:
     - **Test Connection** button: Submits values to `/api/runtime/supabase-config/validate` to verify connection details. Shows inline validation items:
       - 1. URL formatting checks
       - 2. Anon client reachability check
       - 3. Admin service-role key reachability check
     - **Save & Apply** button: Saves settings. Disabled until a connection test completes successfully. Triggers configuration storage and redirects to `/login`.
     - **Back to Login** link: Navigates back to `/login` to bypass configuration and proceed to demo mode.

2. **Authenticated Settings Page**
   - **Route File**: `apps/admin-web/src/routes/_authenticated/settings/supabase.tsx`
   - **Path**: `/settings/supabase`
   - **Visual Layout**: Wrapped in a standard project `<PageFrame title="Supabase Settings" description="Manage database connections, API keys, and workspace secrets." />` structure.
   - **Environment Override Notice**:
     - Displays a status bar indicating the current active source: `env`, `workspace`, or `demo`.
     - If the configuration is overridden via environment variables, shows a prominent warning banner: *"Active settings are loaded from local environment variables (.env). Direct UI configuration is read-only."* Under this condition, inputs are set to read-only and actions are hidden/disabled.
   - **Form Fields**: Same fields as the setup page.
     - Note on Service Role Key: The actual key value is never returned to the UI; if already configured, the input renders a masked placeholder `••••••••••••••••` with a note: *"Leave empty to keep existing key, or enter a new key to overwrite."*
   - **Interactive Actions**:
     - **Test Connection**: Sends details for backend validation.
     - **Save Changes**: Persists the browser-safe settings and writes the service role key to the secret store. Because the client wrapper is a module-level singleton, a successful save will display a warning banner or modal asking the user to refresh the page: *"Configuration updated successfully. A page reload is required to apply the changes."* along with a primary **Reload Now** button.
     - **Danger Zone**: A red-accented bottom panel containing a **Reset to Demo Mode** action. Clicking this opens a confirmation modal and, on approval, calls `DELETE /api/runtime/supabase-config` to revert back to an unconfigured state.

### 10.6 Entry points and Login Integration

1. **Login Page Integration (`apps/admin-web/src/routes/login.tsx`)**
   - **Check Config Status**: On load, queries `/api/runtime/supabase-config` to determine if a workspace-level Supabase setup is present.
   - **State: Unconfigured Workspace**:
     - Hides the email/password credential fields.
     - Shows a clear warning card: *"Database connection required to log in. Proceed to setup your database connection, or continue in Demo Mode."*
     - Displays two CTA buttons:
       - **Configure Supabase** (Primary accent button; links to `/setup/supabase`).
       - **Continue in Demo Mode** (Secondary outline button; initializes demo session and redirects directly to `/dashboard`).
   - **State: Configured Workspace**:
     - Renders the normal email/password login form.
     - Displays a subtle link below the form: **Database Settings** (links to `/setup/supabase` for re-configuring or troubleshooting connections).

2. **Sidebar Navigation Integration (`apps/admin-web/src/components/layout/app-nav.ts`)**
   - Imports the `Database` icon from `lucide-react`.
   - Appends a new route link to the `settingsNavItems` list:
     ```ts
     { to: "/settings/supabase", label: "Supabase", icon: Database }
     ```

---

## 11. Incremental Delivery Plan

Because the Supabase bootstrap points are `CRITICAL` impact, implement in this order:

### Phase 1: local persistence and validation

- add local-runner config storage
- add local-runner validation endpoint
- add admin-web runtime proxy route
- add tests for save/load/validate/reset

No auth/bootstrap consumers changed yet.

### Phase 2: browser runtime bootstrap

- refactor browser client creation to runtime config
- keep env override behavior
- update auth-provider and login logic
- add reload/reset mechanics

### Phase 3: settings and setup UI

- add unauthenticated setup page
- add authenticated settings page
- add navigation links
- add login screen empty-state CTA

### Phase 4: server runtime adoption

- refactor server/client/admin helpers to resolve workspace config
- verify gateway bundle behavior
- verify admin-only flows that need service-role access

### Phase 5: cleanup

- consolidate old env helpers
- remove duplicated branching
- document final runtime contract

---

## 12. Files In Scope

### Existing files likely to change

- `apps/admin-web/src/lib/env/browser-env.ts`
- `apps/admin-web/src/lib/env/app-env.ts`
- `apps/admin-web/src/data/supabase/client.ts`
- `apps/admin-web/src/data/datasource/supabase/client.ts`
- `apps/admin-web/src/data/repository/factory.ts`
- `apps/admin-web/src/lib/supabase/admin.ts`
- `apps/admin-web/src/features/auth/auth-provider.tsx`
- `apps/admin-web/src/features/auth/require-auth.ts`
- `apps/admin-web/src/data/auth/session.ts`
- `apps/admin-web/src/routes/login.tsx`
- `apps/admin-web/src/components/layout/app-nav.ts`
- `apps/local-runner/internal/runner/secret_store.go`

### New files likely needed

- `apps/local-runner/internal/runner/supabase_config.go`
- `apps/admin-web/src/app/api/runtime/supabase-config/route.ts`
- `apps/admin-web/src/app/api/runtime/supabase-config/validate/route.ts`
- `apps/admin-web/src/routes/_authenticated/settings/supabase.tsx`
- `apps/admin-web/src/routes/setup.supabase.tsx`
- supporting tests for each new boundary

---

## 13. Testing Plan

### Local-runner tests

- save config writes JSON file under `.flowpilot/settings/`
- service-role key is stored in secret store, not JSON
- load returns expected browser-safe values
- reset removes both JSON state and secret
- validate rejects malformed URLs
- validate reports missing service-role key
- runtime resolution prefers saved config over env fallback

### Admin-web unit tests

- runtime resolver chooses `config > env > demo`
- browser bootstrap returns demo client when unconfigured
- browser bootstrap returns Supabase client when runtime config exists
- auth provider uses runtime-config mode rather than env-only mode
- login page shows setup CTA when unconfigured
- login page shows sign-in form when configured

### API route tests

- runtime config GET omits service-role key
- PUT saves config successfully
- validate route returns field-specific failures
- reset route clears saved config

### Integration checks

- configure from login page, reload, then sign in
- open authenticated settings page and update config
- server-side routes that need admin client still work after switching from env to saved config
- demo mode still works when both env and saved config are absent

---

## 14. Risks And Mitigations

### Risk 1: import-time browser client assumptions

Problem:

- many files assume the browser client exists at module import time

Mitigation:

- centralize lazy runtime bootstrap,
- require reload after config save,
- migrate auth/bootstrap consumers first before secondary feature screens.

### Risk 2: mixing public and privileged keys

Problem:

- `SUPABASE_API_KEY` is browser-safe but `SUPABASE_SERVICE_ROLE_KEY` is not

Mitigation:

- persist them separately,
- never return the service-role key from runtime APIs,
- keep privileged client creation server-only.

### Risk 3: breaking demo mode

Problem:

- demo mode is part of the current local workflow

Mitigation:

- keep explicit demo fallback in the resolver,
- add tests that cover no-env/no-workspace-config behavior.

### Risk 5: env unexpectedly masking saved config

Problem:

- if `.env` stays higher priority, users will think settings changes do not work

Mitigation:

- make saved config the primary source,
- show the active source in the UI,
- add tests for `config > env > demo`.

### Risk 6: local config accidentally committed or assumed shared

Problem:

- users may assume the saved connection is shared with teammates or checked into the repo

Mitigation:

- document clearly that config is per-PC local state,
- store secrets in local secret storage,
- keep config under local runtime state paths,
- ensure local settings paths are ignored by git.

### Risk 4: service-role validation depends on product schema

Problem:

- validation could fail if it queries app tables before migrations exist

Mitigation:

- validate against generic Supabase auth/admin reachability, not app tables.

---

## 15. Definition Of Done

- [ ] User can configure Supabase from the UI without editing `.env`
- [ ] Login flow provides a clear setup path before sign-in
- [ ] Authenticated Settings includes a Supabase page
- [ ] Workspace config persists locally on each PC outside source-controlled env files
- [ ] Service-role key is never exposed to the browser after save
- [ ] Runtime resolution uses `workspace config > env > demo`
- [ ] Browser and server Supabase bootstrap both support saved workspace config
- [ ] Demo mode still works when nothing is configured
- [ ] Tests cover persistence, validation, bootstrap, login states, and reset behavior

---

## 16. Recommended Next Implementation Ticket Split

If this is implemented as smaller PRs, split it into:

1. `Task-014A` local-runner persisted Supabase config + validation API
2. `Task-014B` admin-web runtime bootstrap refactor
3. `Task-014C` login/setup/settings UI
4. `Task-014D` cleanup, documentation, and regression coverage

---

## 17. Detailed UI Implementation Plan

### Overview
Provide a dedicated, workspace-scoped Supabase connection configuration user interface in the Admin app. This will allow users to configure their own Supabase database without hard-coding credentials in environment variables, while maintaining seamless fallback to environment variable overrides (for development/CI) and local demo mode.

### User Review Required

> [!IMPORTANT]
> **Page Reload Requirement:** Since the Supabase client (`supabase` in `@/data/supabase/client.ts`) is currently a module-level singleton, changing connection configuration at runtime requires a page reload (`window.location.reload()`) to cleanly re-instantiate the client wrapper. We must explicitly prompt the user for a reload after a successful save.
> 
> **Environment Variables Precedence:** If environment variables (`SUPABASE_API_URL` and `SUPABASE_API_KEY`) are present, they override workspace configuration. The UI will show a prominent warning and mark inputs as read-only.

### Proposed Changes

#### Admin Web Frontend

##### [NEW] [setup.supabase.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/setup.supabase.tsx)
An unauthenticated route mapping to `/setup/supabase` for onboarding unconfigured workspaces.

- **Layout**: Full-screen centered card layout utilizing the project's signature premium design (`noise-bg`, rounded border panel, backdrop blur).
- **Form Fields**:
  - **Supabase URL**: HTTPS text input. Text updates will automatically suggest/fill the Edge Function URL as `<api-url>/functions/v1`.
  - **Anon Key**: Password-style input with a toggle to reveal plain text.
  - **Edge Function URL**: HTTPS text input.
  - **Service Role Key**: Password-style input with a toggle. Highlighted with a warning badge ("Privileged Secret").
- **Validation**:
  - A "Test Connection" button that calls the `/api/runtime/supabase-config/validate` endpoint.
  - Shows testing steps (URL Check, Base Auth Check, Admin Auth Check) with visual success/error states.
- **Actions**:
  - "Save & Configure" button: Disabled until connection test succeeds. Saves credentials via `PUT /api/runtime/supabase-config` and triggers a redirect back to `/login`.
  - "Return to Login (Demo Mode)" link: Navigates back to `/login` to bypass configuration.

##### [NEW] [supabase.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/settings/supabase.tsx)
An authenticated route mapping to `/settings/supabase` within the sidebar shell.

- **Layout**: Wrapped in `<PageFrame title="Supabase Settings" description="Configure database endpoints, public keys, and administrator secrets." />`.
- **Environment Override Notice**:
  - If the active mode is `env` (overridden via `.env`), displays a prominent banner: *"Settings are currently governed by environment variables. This panel is read-only."*
  - Disables all inputs, and hides the "Save" and "Reset" buttons.
- **Form Fields**:
  - Same inputs as the Setup page.
  - For the **Service Role Key**, the actual key is never returned by the backend. The UI will render a masked placeholder `••••••••••••••••` if already configured, with a note: *"Leave empty to keep existing key, or enter a new key to overwrite."*
- **Actions**:
  - "Test Connection": Performs live verification.
  - "Save Changes": Saves updated settings. On success, displays a modal or page banner requesting a reload: *"Database configuration updated. A page reload is required to apply the changes."* with a "Reload Now" button.
  - **Danger Zone**: A red-accented panel containing a "Reset to Demo Mode" button. Triggering this opens a confirmation modal before sending a `DELETE /api/runtime/supabase-config` request to clear configuration.

##### [MODIFY] [login.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/login.tsx)
Update the login page to direct the user based on the database configuration status.

- **Runtime Check**:
  - On mount, query `/api/runtime/supabase-config` to check if Supabase is active (or if we are in demo mode).
- **Conditional Layout**:
  - **Unconfigured State**:
    - Hide/disable email and password inputs.
    - Show an alert panel: *"Supabase database is not configured. Realtime features, persistence, and team controls are unavailable in Demo Mode."*
    - Render a prominent primary button: **Configure Supabase** (links to `/setup/supabase`).
    - Render a secondary button: **Continue in Demo Mode** (logs the user in automatically with demo credentials).
  - **Configured State**:
    - Display the normal login form.
    - Render a small, subtle link at the bottom: **Database Settings** (links to `/setup/supabase` if configuration needs updates).

##### [MODIFY] [app-nav.ts](file:///c:/working/flowpilot/apps/admin-web/src/components/layout/app-nav.ts)
Add the settings navigation entry.

- Import the `Database` icon from `lucide-react`.
- Add the item to `settingsNavItems`:
  ```ts
  { to: "/settings/supabase", label: "Supabase", icon: Database }
  ```

### Verification Plan

#### Automated Tests
- Create a test file `apps/admin-web/src/routes/_authenticated/settings/supabase.test.tsx` to verify:
  - Render state when configuration is overridden by environment variables (read-only mode).
  - Form validation rules for URLs.
  - Test Connection behavior and API mock integration.
  - Reset database configuration action triggers confirmation dialog.

#### Manual Verification
1. **Unconfigured Initial Load**:
   - Clear `.env` values for Supabase (`SUPABASE_API_URL` and `SUPABASE_API_KEY`).
   - Load `/login` and verify that the page displays the database configuration warning and redirection buttons.
2. **Setup Workspace Connection**:
   - Click "Configure Supabase" to navigate to `/setup/supabase`.
   - Enter invalid parameters (e.g. malformed URLs or missing keys) and click "Test Connection" -> check that error states appear.
   - Enter valid details, click "Test Connection" -> check that the success state appears and "Save & Configure" becomes active.
   - Save the configuration -> check that the app redirects to `/login`.
3. **Database Login**:
   - Log in with valid Supabase user credentials.
   - Verify that you are redirected to the Dashboard.
4. **Settings Page Inspection**:
   - Navigate to `/settings/supabase`.
   - Update a parameter, verify that saving displays the "Reload Required" warning.
   - Verify that clicking "Reload Now" refreshes the browser and the new settings persist.
5. **Danger Zone Reset**:
   - Navigate to `/settings/supabase` and click "Reset to Demo Mode".
   - Confirm the dialog and verify that the config is cleared and you are returned to `/login` showing the unconfigured state.

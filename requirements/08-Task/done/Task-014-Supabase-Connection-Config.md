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
- The runtime config endpoints must work before login and must not depend on Supabase auth, `assertAdminApiSession()`, or `createGatewayBundle()`.
- The admin app currently has two routing surfaces:
  - UI routes under `apps/admin-web/src/routes` using TanStack Router.
  - API route handlers under `apps/admin-web/src/app/api` using Next-style route modules.
- `.flowpilot` is already ignored by git, so the proposed workspace config path is local-only by default.

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

## 5.1 Planning Review Issues Found

Review date: 2026-06-03.

### Issue 1: precedence contradiction

Earlier sections correctly define runtime precedence as:

1. saved workspace config
2. explicit process env
3. demo mode fallback

The detailed UI section previously said environment variables override workspace configuration and make the settings panel read-only. That conflicts with the product goal. The corrected rule is:

- saved workspace config is active whenever it exists,
- env is shown as an available fallback only when no saved config exists,
- the UI remains editable in both `config` and `env` modes,
- saving from `env` mode creates workspace config and makes `config` active after reload.

### Issue 2: runtime config API circular dependency

The setup/login flow must be available before authentication. Therefore the runtime config API must not call:

- `assertAdminApiSession()`
- `requireAdminSession()`
- `createGatewayBundle()`
- Supabase client factories that depend on the same runtime config being resolved

The API route should read env directly and proxy local-runner config endpoints directly through a small runner request helper.

### Issue 3: async browser bootstrap affects more than the client factory

`apps/admin-web/src/data/supabase/client.ts` exports a module-level `supabase` singleton. Current consumers include auth, login, edge function auth token retrieval, artifact cloud storage UI, and realtime workflow screens.

Changing only `createSupabaseBrowserClient()` is not enough. The implementation must introduce an async runtime-status/client boundary and migrate direct singleton consumers to that boundary.

### Issue 4: route surfaces need to stay separate

The UI files should be added under `apps/admin-web/src/routes`.
The proxy/runtime API files should be added under `apps/admin-web/src/app/api`.

Do not add UI pages under `src/app`, and do not expect TanStack route generation to pick up API files.

### Issue 5: edge function URL completeness

Current `hasSupabaseEnv()` only checks `SUPABASE_API_URL` and `SUPABASE_API_KEY`, while `getSupabaseEdgeFunctionUrl()` can still throw if `SUPABASE_API_EDGE_FUNCTION_URL` is missing.

The runtime status must distinguish:

- core Supabase auth/data readiness,
- edge-function URL readiness,
- service-role key readiness.

### Issue 6: partial service-role updates

The settings UI cannot receive the saved service-role key. Save semantics must explicitly support:

- empty service-role input means keep the existing secret when `hasServiceRoleKey` is true,
- non-empty service-role input overwrites the secret,
- empty service-role input fails validation when no existing secret is present.

### Issue 7: runner offline behavior

If the local runner is offline, saved workspace config cannot be read. Runtime resolution should then fall back to env if env is complete; otherwise demo mode should remain available with an actionable "runner offline" status for the setup/settings UI.

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
  runnerReachable: boolean;
  envAvailable: boolean;
  savedConfigAvailable: boolean;
  edgeFunctionsReady: boolean;
  lastError: string | null;
};
```

Rules:

- `configured` means the active runtime can create normal Supabase browser/server clients.
- `edgeFunctionsReady` is separate because some flows can use Supabase auth/data even when the Edge Function URL is missing or invalid.
- `hasServiceRoleKey` is a boolean only; the service-role key value must never be serialized in this payload.
- `mode: "env"` means no saved workspace config was available and env fallback is active.
- `mode: "demo"` means neither saved config nor complete env fallback is available.

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

Important:

- environment variables do not override saved workspace config,
- env is only the active source when saved config is absent or unreadable,
- the UI should show env fallback availability, but it should still allow saving a workspace config so users can move from env mode to config mode.

Apply the same precedence to:

- browser Supabase bootstrap
- server Supabase bootstrap
- login mode selection
- gateway bundle selection
- Edge Function URL resolution
- admin/service-role client creation

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
- route registration should be added in `apps/local-runner/internal/cli/root.go` near `/storage-driver`
- `PUT` should support "keep existing service role secret" semantics when the incoming service-role field is blank and a secret already exists
- validation responses should include field-level results, not just a single error string

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

Suggested validation response:

```go
type SupabaseValidationResult struct {
    Valid             bool                         `json:"valid"`
    ProjectRef        string                       `json:"projectRef,omitempty"`
    Checks            []SupabaseValidationCheck    `json:"checks"`
    BrowserSafeConfig SupabaseWorkspaceConfig      `json:"browserSafeConfig,omitempty"`
    HasServiceRoleKey bool                         `json:"hasServiceRoleKey"`
}

type SupabaseValidationCheck struct {
    Key     string `json:"key"`
    Status  string `json:"status"` // "passed" | "failed" | "skipped"
    Message string `json:"message"`
}
```

Validation check keys:

- `api_url_format`
- `anon_key_present`
- `edge_url_format`
- `edge_url_project_match`
- `service_role_present`
- `anon_reachability`
- `service_role_reachability`

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

Implementation constraints:

- do not call `assertAdminApiSession()` because setup must work before login,
- do not call `createGatewayBundle()` because gateway creation currently depends on Supabase mode resolution,
- reuse or extract the small `localRunnerRequest()`/`readRunnerError()` helper pattern from `apps/admin-web/src/app/api/local-runner/provider-accounts/_shared.ts`,
- return `mode: "config" | "env" | "demo"` using the same precedence as Section 7,
- include `runnerReachable` and a browser-safe `lastError` for UI messaging,
- use `cache: "no-store"` for all runtime config reads.

Suggested route behavior:

| Method/path | Auth | Responsibility |
|---|---:|---|
| `GET /api/runtime/supabase-config` | none | Return `SupabaseRuntimeStatus` plus browser-safe saved/env config fields |
| `PUT /api/runtime/supabase-config` | none for setup, session optional for settings | Validate payload shape, proxy save to runner, return browser-safe status |
| `POST /api/runtime/supabase-config/validate` | none | Run field-level validation without persisting |
| `DELETE /api/runtime/supabase-config` | authenticated preferred, setup-safe if no session exists | Clear saved config and service-role secret |

Open security decision before implementation:

- `PUT` and `DELETE` are intentionally unauthenticated for first-time setup. Limit them to local dev/desktop usage through the local-runner trust boundary and avoid exposing this API on a public hosted deployment without an additional local-only guard.

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

Migration plan:

1. Add `apps/admin-web/src/lib/supabase/runtime-config.ts` for `loadSupabaseRuntimeStatus()`, config caching, and `invalidateSupabaseRuntimeStatus()`.
2. Change `apps/admin-web/src/data/supabase/client.ts` to expose async `getBrowserSupabaseClient()` and `resetBrowserSupabaseClient()`.
3. Keep a temporary compatibility export only if needed, but do not use it in auth/login code.
4. Migrate direct singleton consumers:
   - `apps/admin-web/src/features/auth/auth-provider.tsx`
   - `apps/admin-web/src/features/auth/require-auth.ts`
   - `apps/admin-web/src/routes/login.tsx`
   - `apps/admin-web/src/data/datasource/supabase/edge-function-client.ts`
   - `apps/admin-web/src/presentation/components/artifacts/artifact-cloud-storage-panel.tsx`
   - `apps/admin-web/src/features/workflow-engine/use-workflow-realtime.ts`
   - workflow-run pages that call `createSupabaseBrowserClient()` synchronously
5. Update tests that mock `@/data/supabase/client` so they mock the async getter instead of a static `supabase` object.

### 10.3 Replace env-only server bootstrap

Refactor:

- `apps/admin-web/src/lib/env/app-env.ts`
- `apps/admin-web/src/data/datasource/supabase/client.ts`
- `apps/admin-web/src/lib/supabase/admin.ts`

Server runtime must resolve:

- workspace config + stored service-role key first
- otherwise env fallback
- otherwise demo mode or explicit failure depending on call site

Recommended implementation split:

- add a pure server resolver such as `resolveSupabaseRuntimeConfig()` in `apps/admin-web/src/lib/supabase/runtime-config.server.ts`,
- have `createSupabaseServerClient()` use the resolved anon key config,
- have `createSupabaseAdminClient()` use the resolved service-role key config,
- make callers that require service-role access fail with a clear "service role key is not configured" error instead of silently using demo mode,
- update `createGatewayBundle()` to branch on runtime status rather than `hasSupabaseEnv()`.

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

Auth-specific details:

- auth provider must start in a loading state while runtime status is fetched,
- demo session should be used only when runtime status is `mode: "demo"` or when the user explicitly clicks "Continue in Demo Mode",
- route guards should await runtime status before deciding between demo and Supabase mode,
- login should not call `hasSupabaseEnv()` directly after this refactor,
- sign-out should obtain the active client through `getBrowserSupabaseClient()` only when runtime status is not demo.

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
     - If active source is `env`, shows an informational banner: *"FlowPilot is currently using local environment variables because no saved workspace config exists. Saving this form will create workspace config and make it the active source after reload."*
     - If active source is `workspace`, shows that workspace config has priority over any env fallback.
     - If active source is `demo`, shows that no saved config or complete env fallback is available.
   - **Form Fields**: Same fields as the setup page.
     - Note on Service Role Key: The actual key value is never returned to the UI; if already configured, the input renders a masked placeholder `••••••••••••••••` with a note: *"Leave empty to keep existing key, or enter a new key to overwrite."*
   - **Interactive Actions**:
     - **Test Connection**: Sends details for backend validation.
     - **Save Changes**: Persists the browser-safe settings and writes the service role key to the secret store. Because the client wrapper is a module-level singleton, a successful save will display a warning banner or modal asking the user to refresh the page: *"Configuration updated successfully. A page reload is required to apply the changes."* along with a primary **Reload Now** button.

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
- add admin-web runtime proxy route that does not depend on auth or gateway creation
- add runtime status resolver with `config > env > demo` precedence
- add tests for save/load/validate/reset

No auth/bootstrap consumers changed yet.

### Phase 2: browser runtime bootstrap

- refactor browser client creation to runtime config
- keep env fallback behavior
- update auth-provider and login logic
- add reload/reset mechanics

### Phase 3: server runtime adoption

- refactor server/client/admin helpers to resolve workspace config
- verify gateway bundle behavior
- verify admin-only flows that need service-role access
- verify Edge Function client resolution

### Phase 4: settings and setup UI

- add unauthenticated setup page
- add authenticated settings page
- add navigation links
- add login screen empty-state CTA

### Phase 5: cleanup

- consolidate old env helpers
- remove duplicated branching
- document final runtime contract
- remove or deprecate static `supabase` singleton exports once consumers are migrated

---

## 12. Files In Scope

### Existing files likely to change

- `apps/admin-web/src/lib/env/browser-env.ts`
- `apps/admin-web/src/lib/env/app-env.ts`
- `apps/admin-web/src/data/supabase/client.ts`
- `apps/admin-web/src/data/datasource/supabase/client.ts`
- `apps/admin-web/src/data/datasource/supabase/edge-function-client.ts`
- `apps/admin-web/src/data/repository/factory.ts`
- `apps/admin-web/src/lib/supabase/admin.ts`
- `apps/admin-web/src/features/auth/auth-provider.tsx`
- `apps/admin-web/src/features/auth/require-auth.ts`
- `apps/admin-web/src/data/auth/session.ts`
- `apps/admin-web/src/routes/login.tsx`
- `apps/admin-web/src/components/layout/app-nav.ts`
- `apps/admin-web/src/components/layout/app-shell.tsx`
- `apps/admin-web/src/routes/route-map.test.ts`
- `apps/local-runner/internal/cli/root.go`
- `apps/local-runner/internal/runner/types.go`
- `apps/local-runner/internal/runner/secret_store.go`

### New files likely needed

- `apps/local-runner/internal/runner/supabase_config.go`
- `apps/local-runner/internal/runner/supabase_config_test.go`
- `apps/admin-web/src/lib/supabase/runtime-config.ts`
- `apps/admin-web/src/lib/supabase/runtime-config.server.ts`
- `apps/admin-web/src/app/api/runtime/supabase-config/_shared.ts`
- `apps/admin-web/src/app/api/runtime/supabase-config/route.ts`
- `apps/admin-web/src/app/api/runtime/supabase-config/validate/route.ts`
- `apps/admin-web/src/routes/_authenticated/settings/supabase.tsx`
- `apps/admin-web/src/routes/setup.supabase.tsx`
- supporting tests for each new boundary

Generated files:

- `apps/admin-web/src/routeTree.gen.ts` should be regenerated by the TanStack route generator after adding UI routes.

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
- blank service-role input keeps existing secret when one exists
- blank service-role input fails when no existing secret exists
- validation produces field-level check results

### Admin-web unit tests

- runtime resolver chooses `config > env > demo`
- runtime resolver reports runner-offline fallback behavior
- browser bootstrap returns demo client when unconfigured
- browser bootstrap returns Supabase client when runtime config exists
- auth provider uses runtime-config mode rather than env-only mode
- auth provider stays loading while runtime status is being resolved
- login page shows setup CTA when unconfigured
- login page shows sign-in form when configured
- Edge Function client reads the active runtime Edge Function URL

### API route tests

- runtime config GET omits service-role key
- PUT saves config successfully
- PUT with blank service-role key keeps an existing secret
- validate route returns field-specific failures
- reset route clears saved config
- runtime routes do not require an authenticated Supabase session

### Integration checks

- configure from login page, reload, then sign in
- save workspace config while env fallback exists and verify workspace config becomes active
- open authenticated settings page and update config
- server-side routes that need admin client still work after switching from env to saved config
- demo mode still works when both env and saved config are absent
- local runner offline: env fallback still works if env is complete; otherwise login shows demo/setup status

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

### Risk 4: env unexpectedly masking saved config

Problem:

- if `.env` stays higher priority, users will think settings changes do not work

Mitigation:

- make saved config the primary source,
- show the active source in the UI,
- add tests for `config > env > demo`.

### Risk 5: local config accidentally committed or assumed shared

Problem:

- users may assume the saved connection is shared with teammates or checked into the repo

Mitigation:

- document clearly that config is per-PC local state,
- store secrets in local secret storage,
- keep config under local runtime state paths,
- ensure local settings paths are ignored by git.

### Risk 6: service-role validation depends on product schema

Problem:

- validation could fail if it queries app tables before migrations exist

Mitigation:

- validate against generic Supabase auth/admin reachability, not app tables.

---

## 15. Definition Of Done

- [x] User can configure Supabase from the UI without editing `.env`
- [x] Login flow provides a clear setup path before sign-in
- [x] Authenticated Settings includes a Supabase page
- [x] Workspace config persists locally on each PC outside source-controlled env files
- [x] The doc and UI clearly communicate that saved Supabase config is per-PC local runtime state, not shared project/team configuration
- [x] Service-role key is never exposed to the browser after save
- [x] Browser-safe config and privileged service-role secret are persisted separately
- [x] Runtime resolution uses `workspace config > env > demo`
- [x] Environment variables never override an existing saved workspace config
- [x] Env fallback remains usable when no saved workspace config exists
- [x] Browser and server Supabase bootstrap both support saved workspace config
- [x] Runtime setup/status APIs work before login without auth or gateway circular dependencies
- [x] Runtime setup/status APIs do not call `assertAdminApiSession()`, `requireAdminSession()`, `createGatewayBundle()`, or Supabase client factories as part of config discovery
- [x] Saving config while env fallback exists makes workspace config active after reload
- [x] The browser runtime no longer depends on a permanent import-time `supabase` singleton for auth/login/runtime-sensitive flows
- [x] Auth, login, and route-guard flows wait for runtime status/client resolution before deciding between Supabase mode and demo mode
- [x] Demo mode still works when nothing is configured
- [x] Runner-offline state falls back to env or demo with actionable UI status
- [x] UI route files live under `apps/admin-web/src/routes` and runtime/API handlers live under `apps/admin-web/src/app/api`
- [x] Runtime status distinguishes core Supabase readiness, Edge Function URL readiness, and service-role key readiness
- [x] Edge Function URL resolution works with saved workspace config and does not fail silently behind partial env checks
- [x] Leaving the service-role field blank keeps the existing secret only when one already exists; otherwise save/validation fails clearly
- [x] Validation returns actionable field/check-specific failures rather than a single generic save error
- [x] Validation checks generic Supabase reachability and does not depend on FlowPilot product tables or migrations already existing
- [x] Import-time browser client assumptions are removed or isolated behind lazy runtime bootstrap and explicit reload/reset behavior
- [x] The active runtime source is visible in the UI so users can tell whether `config`, `env`, or `demo` is active
- [x] Local settings paths remain ignored by git so workspace config is not accidentally committed
- [ ] Tests cover persistence, validation, bootstrap, login states, and reset behavior

---

## 16. Recommended Next Implementation Ticket Split

If this is implemented as smaller PRs, split it into:

1. `Task-014A` local-runner persisted Supabase config + validation API + unauthenticated admin runtime proxy
2. `Task-014B` browser runtime bootstrap + auth/login mode refactor
3. `Task-014C` server runtime adoption + gateway/admin/edge resolution
4. `Task-014D` cleanup, documentation, and regression coverage
5. `Task-014E` login/setup/settings UI and route navigation

---

## 17. Detailed UI Implementation Plan

### Overview
Provide a dedicated, workspace-scoped Supabase connection configuration UI in the Admin app. Users can configure their own Supabase database without hard-coding credentials in environment variables, while retaining env fallback for development/CI and demo mode when neither source exists.

### User Review Required

> [!IMPORTANT]
> **Page Reload Requirement:** The current app has module-level Supabase client assumptions. Even after the async bootstrap refactor, successful config save should prompt for a page reload so auth, realtime subscriptions, and cached clients are recreated from the new source.
> 
> **Precedence:** Workspace config has priority over env. Env is fallback only. If env is active, saving the form creates workspace config and makes workspace config active after reload.

### Proposed Changes

#### Admin Web Frontend

##### [NEW] `apps/admin-web/src/routes/setup.supabase.tsx`

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

##### [NEW] `apps/admin-web/src/routes/_authenticated/settings/supabase.tsx`

An authenticated route mapping to `/settings/supabase` within the sidebar shell.

- **Layout**: Wrapped in `<PageFrame title="Supabase Settings" description="Configure database endpoints, public keys, and administrator secrets." />`.
- **Source Notice**:
  - `config`: show workspace config is active and env is ignored unless config is reset.
  - `env`: show env fallback is active because no saved config exists; inputs remain editable.
  - `demo`: show no saved config or complete env fallback exists; inputs remain editable.
  - runner offline: show runner cannot read/write saved config; saving is disabled until runner is reachable, but env/demo status remains visible.
- **Form Fields**:
  - Same inputs as the Setup page.
  - For the **Service Role Key**, the actual key is never returned by the backend. The UI will render a masked placeholder `••••••••••••••••` if already configured, with a note: *"Leave empty to keep existing key, or enter a new key to overwrite."*
- **Actions**:
  - "Test Connection": Performs live verification.
  - "Save Changes": Saves updated settings. On success, displays a modal or page banner requesting a reload: *"Database configuration updated. A page reload is required to apply the changes."* with a "Reload Now" button.

##### [MODIFY] `apps/admin-web/src/routes/login.tsx`

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

##### [MODIFY] `apps/admin-web/src/components/layout/app-nav.ts`

Add the settings navigation entry.

- Import the `Database` icon from `lucide-react`.
- Add the item to `settingsNavItems`:
  ```ts
  { to: "/settings/supabase", label: "Supabase", icon: Database }
  ```

### Verification Plan

#### Automated Tests
- Create a test file `apps/admin-web/src/routes/_authenticated/settings/supabase.test.tsx` to verify:
  - render state for `config`, `env`, `demo`, and runner-offline status.
  - Form validation rules for URLs.
  - Test Connection behavior and API mock integration.
  - Blank service-role key keeps the existing secret only when `hasServiceRoleKey` is true.

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
5. **Env Fallback Promotion**:
   - Configure valid env values and clear saved workspace config.
   - Verify `/settings/supabase` shows `env` as active and remains editable.
   - Save workspace config, reload, and verify `config` becomes active.

---

## 18. GUide to Test feature

Use this guide to manually verify the full Supabase runtime-config feature after the new code changes.

### Test data to prepare

- One valid Supabase project URL
- One valid anon key for that project
- One valid service-role key for that project
- The matching Edge Function base URL (`<supabase-url>/functions/v1`)
- One invalid URL string for negative testing
- One invalid or truncated key for negative testing

### Use case 1: Unconfigured workspace shows setup path

1. Ensure there is no saved local runner config for this workspace on the current PC.
2. Remove or disable the Supabase env fallback values used by `apps/admin-web`.
3. Open `/login`.
4. Verify the login form does not proceed as a normal Supabase login flow.
5. Verify the page explains that Supabase is not configured.
6. Verify the page shows a primary action to configure Supabase.

### Use case 2: Setup page validates bad input

1. From `/login`, open `/setup/supabase`.
2. Enter an invalid Supabase URL and leave required keys blank.
3. Click `Test Connection`.
4. Verify validation errors are shown per failed check instead of a single generic message.
5. Verify failed checks clearly identify URL format issues and missing key issues.
6. Verify `Save & Configure` stays disabled until validation passes.

### Use case 3: Setup page accepts valid config

1. On `/setup/supabase`, enter a valid Supabase URL.
2. Verify the Edge Function URL auto-suggests to `<supabase-url>/functions/v1` before manual override.
3. Enter a valid anon key.
4. Enter the matching valid service-role key.
5. Click `Test Connection`.
6. Verify the checks report:
   - Supabase URL is valid
   - anon key is present
   - Edge Function URL is valid
   - Edge Function URL matches the Supabase project
   - service-role key is present
   - anon API is reachable
   - service-role/admin validation runs and passes
7. Verify the success message appears.
8. Verify `Save & Configure` becomes enabled.
9. Click `Save & Configure`.
10. Verify the app returns to `/login`.

### Use case 4: Login works after runtime config save

1. After saving valid config, stay on `/login`.
2. Verify the normal Supabase login experience is available.
3. Sign in with a valid Supabase user.
4. Verify navigation proceeds into the authenticated app.

### Use case 5: Authenticated settings page shows active source and secret behavior

1. Open `/settings/supabase`.
2. Verify the page shows the active runtime source as `config`, `env`, or `demo`.
3. If `config` is active, verify the page says saved workspace config has priority over env.
4. Verify the service-role field does not reveal the stored secret value returned from the backend.
5. If a service-role key is already stored, verify the field presents the keep-or-overwrite guidance.
6. Verify there is no `Danger Zone` block on this page.

### Use case 6: Blank service-role save keeps an existing secret

1. Start from a workspace that already has a saved service-role key.
2. Open `/settings/supabase`.
3. Change a non-secret field such as the Edge Function URL.
4. Leave the service-role field blank.
5. Click `Test Connection`.
6. Verify validation does not fail just because the visible service-role input is blank.
7. Save the form.
8. Verify the save succeeds and the existing stored service-role secret is preserved.

### Use case 7: Missing service-role save fails when no secret exists yet

1. Start from a workspace with no saved service-role key.
2. Open `/setup/supabase` or `/settings/supabase` in a state where a new config must be saved.
3. Fill the Supabase URL, anon key, and Edge Function URL, but leave service-role blank.
4. Click `Test Connection`.
5. Verify validation reports `Service role key is required.`
6. Verify the admin/service-role reachability check is skipped until the key is present.
7. Verify the save action cannot complete successfully in this state.

### Use case 8: Env fallback is visible and can be promoted to saved config

1. Clear any saved workspace config for this PC.
2. Restore valid Supabase env fallback values.
3. Open `/settings/supabase`.
4. Verify the active source shows `env`.
5. Verify the page explains env is being used only because no saved workspace config exists.
6. Change one or more values and save them as workspace config.
7. Reload the page when prompted.
8. Verify the active source changes from `env` to `config`.

### Use case 9: Reload-required flow recreates runtime clients

1. From `/settings/supabase`, change the config to another valid value set for the same project or a different test project.
2. Save the form.
3. Verify the success banner says a reload is required.
4. Click `Reload Now`.
5. Verify the page reloads cleanly.
6. Verify subsequent authenticated Supabase actions still work after reload.
7. Verify there is no stale login/setup/runtime state left over from the previous config source.

### Use case 10: Runner offline state is handled clearly

1. Stop the local runner while keeping the admin web app open.
2. Open `/settings/supabase`.
3. Verify the page reports that the runner is offline.
4. Verify saving is disabled while the runner is unreachable.
5. Verify the page still communicates whether the app is currently in `env` or `demo` mode.
6. Restart the runner and confirm the page can recover after refresh.

### Use case 11: Edge Function URL mismatch is caught

1. Enter a valid Supabase URL.
2. Enter an Edge Function URL from a different Supabase project or domain.
3. Fill the remaining required keys.
4. Click `Test Connection`.
5. Verify validation reports that the Edge Function URL does not match the Supabase project.
6. Correct the URL and verify the check passes on the next test.

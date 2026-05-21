# CP-03: Supabase Integration

**Maps from:** CP-01, CP-09, CP-12, SD-01, SD-04, SD-08
**Phase:** Parallel foundation after CP-01, before deeper storage and Edge Function rollout
**Status:** implemented

---

## 1. Goal

Make Supabase the explicit application backend contract for `admin-web`, with clear separation between:

- browser auth and low-privilege reads
- authenticated user-scoped application data access
- Edge Function invocation
- Storage bucket operations

This plan should follow the same boundary split used in the Go reference from `D:\working\BEMplan\data`:

- `SupabaseClient`
- `SupabaseEdgeFunctionClient`
- `NewSupabaseStorageClient`

The TypeScript implementation should mirror the same intent even if the concrete APIs differ.

---

## 2. Current Project State

The repo already contains a partial Supabase integration:

- `apps/admin-web/src/data/supabase/client.ts` creates the browser client and falls back to demo mode when Vite env is missing.
- `apps/admin-web/src/lib/env/browser-env.ts` reads browser-safe Supabase variables.
- `apps/admin-web/src/lib/env/app-env.ts` should only read browser-safe API values plus the Edge Function URL for admin-web server boundaries.
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts` already persists projects, features, context sources, workflow runs, approvals, outputs, and logs through Supabase.
- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql` already defines the current admin MVP schema.

What is still missing as a first-class integration contract:

- a documented root `.env` contract that connects browser and server usage cleanly
- a dedicated Edge Function client boundary
- a dedicated Storage client boundary
- explicit guidance on when to use anon credentials versus service-role credentials

---

## 3. Root Env Contract

Use the repository root `.env` as the single source of truth.

Required server env:

```env
SUPABASE_API_URL=...
SUPABASE_API_KEY=...
SUPABASE_API_EDGE_FUNCTION_URL=...
```

Optional:

```env
FLOWPILOT_RUNNER_URL=http://127.0.0.1:4317
VITE_LOCAL_RUNNER_URL=http://localhost:9100
```

Rules:

- `SUPABASE_API_KEY` is the only browser-safe Supabase credential used by admin-web.
- `SUPABASE_API_EDGE_FUNCTION_URL` is browser-safe only when the corresponding Edge Functions validate JWT auth and do not rely on privileged secrets in the caller.
- `SUPABASE_API_SERVICE_ROLE_KEY` must not be part of the admin-web runtime contract.
- `SUPABASE_API_SERVICE_ROLE_KEY`, when needed at all, belongs only in trusted runtimes such as Go/internal tools or Supabase-managed function environments.

---

## 4. Client Boundaries

| Boundary | Responsibility | Credential |
|---|---|---|
| Browser Supabase client | login, logout, session restore, user-scoped reads/writes guarded by RLS | `SUPABASE_API_KEY` |
| Admin-web server boundary | request/session validation and forwarding with the same low-privilege project credentials | `SUPABASE_API_KEY` |
| Edge Function client | invoke trusted `/functions/v1/*` endpoints such as embeddings or AI helpers | `SUPABASE_API_KEY` plus user JWT when required |
| Storage client | not owned by admin-web runtime; handled by trusted backend or internal tools | not exposed to admin-web |

Mirror the Go reference design:

- `SupabaseClient` -> browser or request-scoped data access wrapper
- `SupabaseEdgeFunctionClient` -> HTTP client with shared auth/header handling
- `SupabaseStorageClient` -> bucket/object helper with consistent error handling in trusted runtimes, not in admin-web

---

## 5. Files In Scope

Existing files:

- `apps/admin-web/src/data/supabase/client.ts`
- `apps/admin-web/src/data/datasource/supabase/client.ts`
- `apps/admin-web/src/lib/env/browser-env.ts`
- `apps/admin-web/src/lib/env/app-env.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts`
- `apps/admin-web/src/app/api/auth/login/route.ts`
- `apps/admin-web/src/app/api/auth/logout/route.ts`
- `apps/admin-web/src/data/auth/session.ts`
- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql`

Planned additions or hardening points:

- `apps/admin-web/src/data/datasource/supabase/edge-function-client.ts`
- `apps/admin-web/src/data/datasource/supabase/storage-client.ts`
- shared Supabase error mapper or response guard helpers if duplication appears

---

## 6. Implementation Plan

### 6.1 Normalize Client Creation

Keep one authoritative Supabase datasource module that exports:

- browser client factory
- server client factory

Requirements:

- browser creation must fail fast only when the app is expected to run in Supabase mode
- demo mode fallback must remain available for local exploration when env is absent
- admin-web must not create a service-role client

### 6.2 Keep Auth In The Browser Boundary

Use the browser client for:

- `signInWithPassword`
- `signOut`
- `getSession`
- auth state subscriptions
- route guards for authenticated views

Do not expose service-role credentials to the browser for convenience.

### 6.3 Keep Repository Persistence User-Scoped

Route admin-web persistence through user-scoped Supabase access governed by RLS, or through authenticated Edge Functions when the action cannot safely be performed from the browser.

Requirements:

- repository code used by admin-web must not depend on service-role credentials
- IDs, timestamps, and workflow state transitions stay centralized in the repository layer
- server routes, if retained, should forward authenticated user context rather than bypassing RLS with a role key

### 6.4 Add A Dedicated Edge Function Client

Create a small datasource wrapper modeled after the Go `SupabaseEdgeFunctionClient`.

Responsibilities:

- build endpoint URLs from `SUPABASE_API_EDGE_FUNCTION_URL`
- send JSON requests with shared auth headers
- handle timeout, response parsing, and structured errors
- validate required response fields for each function contract

Primary consumers:

- CP-09 AI orchestration endpoints
- CP-12 embedding generation and retrieval helpers
- future long-running AI helper functions

### 6.5 Keep Storage Outside Admin-Web Trust Boundary

Do not make admin-web a storage service-role owner.

Responsibilities:

- let trusted backends or internal tools upload artifacts to named buckets
- let admin-web consume storage metadata or signed/public URLs returned by trusted boundaries
- centralize bucket path conventions and content type handling outside the browser runtime

Primary consumers:

- Go/internal tooling
- Edge Functions
- future backend-owned upload workflows

### 6.6 Security And RLS Rules

Keep the trust model explicit:

- browser reads and writes must rely on RLS
- admin-web must not require or expose a service-role key
- Edge Functions that perform privileged work should validate auth before acting
- Storage bucket policies must be aligned with the same project and team boundaries as database rows

Never solve an authorization problem by moving more power into the browser.

---

## 7. Verification Checklist

1. Root `.env` contains `SUPABASE_API_URL`, `SUPABASE_API_KEY`, and `SUPABASE_API_EDGE_FUNCTION_URL` for admin-web.
2. `apps/admin-web` starts in Supabase mode when env is present and demo mode when env is absent.
3. Login and logout work through the browser client without service-role leakage.
4. Admin-web CRUD flows persist through RLS-safe Supabase access or authenticated Edge Functions.
5. Edge Function calls can be invoked through a dedicated datasource wrapper with clear error messages.
6. Storage operations remain outside the admin-web browser trust boundary.
7. No admin-web runtime path requires `SUPABASE_API_SERVICE_ROLE_KEY`.

---

## 8. Definition Of Done

- [ ] Supabase env usage is documented and consistent across browser and server code.
- [ ] Browser auth uses anon credentials only.
- [ ] Admin-web runtime does not require service-role credentials.
- [ ] Edge Function calls are centralized behind a dedicated client wrapper.
- [ ] Storage bucket operations are handled by trusted backend/internal boundaries rather than the browser runtime.
- [ ] Existing Supabase-backed project, feature, context, workflow, approval, output, and log flows continue to work.
- [ ] Security boundaries between admin-web, Edge Functions, and trusted internal tools are explicit and testable.


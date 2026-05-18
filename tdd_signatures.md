# TDD Signatures - CP-01 Foundation Setup

Date: 2026-05-18

Rules:

- signatures only
- no implementation code
- focus on foundation risk

## 1. Environment And Tooling

### File

`apps/admin-web/src/lib/env/app-env.test.ts`

#### `describe("Vite env contract")`

- `it("reads VITE_SUPABASE_URL and VITE_SUPABASE_ANON_KEY from import.meta.env")`
- `it("detects when Supabase env is unavailable")`

### File

`apps/admin-web/package.json`

#### `describe("foundation scripts")`

- `it("exposes dev, build, preview, and test scripts for the Vite workflow")`

## 2. Auth Foundation

### File

`apps/admin-web/src/features/auth/require-auth.test.ts`

#### `describe("requireAuth")`

- `it("returns the current session when Supabase has an authenticated session")`
- `it("redirects guests to /login when no session exists")`

### File

`apps/admin-web/src/features/auth/auth-provider.test.tsx`

#### `describe("AuthProvider")`

- `it("hydrates auth state from the current Supabase session on mount")`
- `it("subscribes to auth state changes and updates stored session data")`

## 3. Router And Shell

### File

`apps/admin-web/src/routes/route-map.test.ts`

#### `describe("CP-01 route tree")`

- `it("maps /login to the public login route")`
- `it("maps / to a redirect into the authenticated landing flow")`
- `it("registers all authenticated project child routes required by CP-01")`
- `it("registers ai-runs and settings child routes")`

### File

`apps/admin-web/src/components/layout/app-shell.test.tsx`

#### `describe("AppShell")`

- `it("renders sidebar, header, and main outlet regions")`
- `it("renders navigation entries for the foundation route set")`

### File

`apps/admin-web/src/app.test.tsx`

#### `describe("App composition")`

- `it("wraps the router with query client and auth providers")`
- `it("renders TanStack router content without Next.js layout dependencies")`

## 4. Domain Carryover

### File

`apps/admin-web/src/domain/constant/status.test.ts`

#### `describe("status constants")`

- `it("exports the workflow and approval status values required by the migrated app")`

### File

`apps/admin-web/src/domain/model/entity/project.test.ts`

#### `describe("domain entity portability")`

- `it("keeps project entity typing intact after the foundation migration")`

## 5. Build Safety

### File

`apps/admin-web/src/routes/login.test.tsx`

#### `describe("login route")`

- `it("renders without importing Next.js-only modules")`

### File

`apps/admin-web/src/routes/_authenticated.test.tsx`

#### `describe("authenticated layout route")`

- `it("uses the auth guard and renders AppShell with an outlet")`

### File

`apps/admin-web/vite-build-smoke.test.ts`

#### `describe("foundation smoke checks")`

- `it("keeps the active app entry free from next/* imports")`
- `it("keeps src/app/api and old app-router files out of the active build path")`

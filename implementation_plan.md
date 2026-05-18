# Implementation Plan - Next.js to Vite + TanStack Refactor

Date: 2026-05-18

## 1. Objective

Move `apps/admin-web` off Next.js and onto Vite + React + TanStack Router, while keeping the existing clean architecture and preserving demo mode, Supabase mode, and the product routes defined in R1.

## 2. Architecture Rules

- Presentation owns routes, page composition, and interactive UI.
- Domain owns entities, payloads, use cases, and business rules.
- Data owns Supabase clients, repository adapters, auth/session helpers, and demo adapters.
- Route components should fetch through React Query hooks or route loaders, not through ad hoc data access inside deeply nested UI.
- The new app must not rely on Next.js file conventions, server components, or `src/app/api/**`.

## 3. Current Migration Targets

- `src/app/layout.tsx` -> Vite HTML shell and `src/main.tsx`
- `src/app/page.tsx` -> TanStack Router redirect to `/dashboard`
- `src/app/(auth)/login/page.tsx` -> `src/routes/login.tsx`
- `src/app/(protected)/**` -> `src/routes/_authenticated/**`
- `src/app/api/**` -> remove or replace with client-side Supabase calls and Edge Functions where needed
- `src/data/auth/session.ts` -> client-side auth/session helper
- `src/data/datasource/supabase/**` -> browser `createClient()` setup
- `src/data/repository/**` -> browser-safe repository bundle and demo fallback

## 4. Phase 1 - Scaffold The Vite App

1. Back up the current Next.js implementation so the migration can be validated without losing the original reference.
2. Create the Vite React TypeScript app in `apps/admin-web`.
3. Install the R1 dependency set:
   - `@tanstack/react-router`
   - `@tanstack/react-query`
   - `@supabase/supabase-js`
   - Tailwind 4 + shadcn support packages
   - `vitest` and testing libraries as needed
4. Add the Vite config, Tailwind config, and environment wiring.
5. Keep the root environment contract aligned with the Vite stack, not Next.js.

## 5. Phase 2 - Port The Reusable Layers

1. Copy the domain layer into the new app structure.
2. Preserve entity, payload, response, gateway, and use case code that is framework-agnostic.
3. Update `status.ts` so `WorkflowStepStatus` includes `skipped`, per R1.
4. Rewrite Supabase client creation to browser-only `createClient()`.
5. Rewrite auth/session helpers to use `supabase.auth.getSession()` and client-side session state.
6. Rewrite repository bundle selection so the demo bundle is used when Supabase env is missing and the Supabase bundle is used when it is available.
7. Keep the mock workflow executor behind the same gateway contract.

## 6. Phase 3 - Port UI And Routing

1. Copy the shared UI primitives and feature components that remain valid.
2. Replace `next/link` with `@tanstack/react-router` `Link`.
3. Replace `next/navigation` redirects with TanStack Router navigation and guards.
4. Rewrite `src/presentation/components/layout/app-shell.tsx` into a TanStack-aware shell.
5. Create the new router tree:
   - `/login`
   - `/_authenticated/dashboard`
   - `/_authenticated/projects`
   - `/_authenticated/projects/$projectId`
   - `/_authenticated/projects/$projectId/features`
   - `/_authenticated/projects/$projectId/workflows`
   - `/_authenticated/projects/$projectId/approvals`
   - `/_authenticated/projects/$projectId/outputs`
   - `/_authenticated/projects/$projectId/artifacts`
   - `/_authenticated/projects/$projectId/artifacts/$artifactId`
   - `/_authenticated/ai-runs`
   - `/_authenticated/settings/integrations`
   - `/_authenticated/settings/prompt-templates`
6. Wire auth protection with router guards so protected routes redirect to `/login` when no session exists.

## 7. Phase 4 - Data Fetching And Query Hooks

1. Introduce React Query client setup in the app root.
2. Create query hooks for the key list and detail pages:
   - projects
   - features
   - workflow definitions
   - workflow runs
   - approvals
   - outputs
   - logs
   - artifact memories
   - workflow prompt context items
3. Keep query keys stable and colocated with each feature module.
4. Move page-level data loading into route loaders or page components that call the hooks.

## 8. Phase 5 - Clean Up Next.js Artifacts

1. Remove or stop using `next.config.ts`, `next-env.d.ts`, and Next-specific app files once the Vite routes are verified.
2. Remove `src/app/api/**` after the equivalent flows are covered by the new stack.
3. Remove imports from `next/link`, `next/navigation`, `next/headers`, and `next/font` across the migrated app.
4. Keep legacy code only if it is still needed as a temporary reference during the branch.

## 9. Verification Order

1. Confirm the Vite app starts.
2. Confirm login and protected route redirects work.
3. Confirm demo fallback still renders.
4. Confirm the key route tree loads.
5. Confirm React Query data flows for at least one list and one detail screen.
6. Confirm the build and test scripts pass.

## 10. Done Criteria

- The app is no longer Next.js-driven.
- The domain and data layers still work through the new router/app shell.
- The route tree matches the R1 mapping.
- Demo mode and Supabase mode both remain viable.
- The migration is clean enough for the later feature phases to build on top of it.

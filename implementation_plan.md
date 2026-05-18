# Implementation Plan - CP-01 Foundation Setup

Date: 2026-05-18

## 1. Objective

Convert `apps/admin-web` from the current mixed Next.js/Vite state into a clean Vite + React + TanStack foundation that satisfies `requirements/07-Coding-Plan/CP-01-Foundation-Setup.md`.

## 2. Execution Boundaries

- This phase is foundation only.
- Deliver the Vite shell, router, auth guard, query client, layout shell, and baseline route tree.
- Port the reusable domain contracts and constants needed as stable inputs for later phases.
- Do not attempt full feature completion for every screen in later phases of the product plan.
- Remove or isolate Next.js-only code paths that would block the Vite app from building and routing correctly.

## 3. Current State To Correct

- `apps/admin-web` still contains `src/app/**`, `next.config.ts`, and `src/app/api/**`.
- Vite files already exist, but the app structure is not yet aligned with the target folder layout.
- Auth, routing, and component locations are split across old and new conventions.
- The plan requirement to back up the current Next.js codebase has not been satisfied with a dedicated `apps/admin-web-nextjs-backup` directory.

## 4. Phase Breakdown

### Phase A - Preserve Current App Snapshot

1. Copy the current `apps/admin-web` tree into `apps/admin-web-nextjs-backup`.
2. Keep the backup untouched after creation so it remains the migration reference.

### Phase B - Normalize Tooling And Dependencies

1. Update `apps/admin-web/package.json` to reflect a Vite-first stack.
2. Ensure the required dependency groups exist:
   - runtime: `react`, `react-dom`, `@tanstack/react-router`, `@tanstack/react-query`, `@supabase/supabase-js`, `zod`, `zustand`, `class-variance-authority`, `clsx`, `tailwind-merge`, `lucide-react`
   - dev: `vite`, `@vitejs/plugin-react`, `@tanstack/router-plugin`, `@tanstack/router-devtools`, `@tanstack/react-query-devtools`, `tailwindcss`, `@tailwindcss/postcss`, `postcss`, `autoprefixer`, `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `jsdom`
3. Remove dependencies that are only required by the old Next.js runtime when they are no longer used by the migrated foundation.
4. Keep scripts aligned to the Vite workflow: `dev`, `build`, `preview`, `test`.

### Phase C - Establish Target App Skeleton

1. Make `src/main.tsx` the browser entry point.
2. Make `src/app.tsx` own `QueryClientProvider`, auth provider, and router provider composition.
3. Make `src/router.tsx` own the TanStack Router instance.
4. Create `src/routes/**` matching the CP-01 target tree, using placeholders where feature implementation is deferred.
5. Create `src/components/layout/app-shell.tsx` as the authenticated shell with sidebar, header, and content outlet.
6. Move shared utilities into `src/lib/**` and Vite env typing into `src/types/env.d.ts` or equivalent Vite-safe declarations.

### Phase D - Port Stable Domain Contracts

1. Copy `src/domain/constant/status.ts`.
2. Copy all entity, payload, response, and gateway interface files from the current app into the new structure.
3. Preserve existing TypeScript shapes unless a Vite or browser-only constraint requires change.
4. Leave broader use-case migration for later unless a file is required by the foundation screens or auth wiring.

### Phase E - Rewrite Browser-Only Data And Auth Setup

1. Implement `src/data/supabase/client.ts` using browser `createClient()`.
2. Implement a Vite-safe auth provider in `src/features/auth/auth-provider.tsx`.
3. Implement auth state storage in `src/features/auth/use-auth.ts`.
4. Implement `src/features/auth/require-auth.ts` using TanStack Router `beforeLoad`.
5. Keep the guard behavior simple: redirect unauthenticated users to `/login`, return session for authenticated users.

### Phase F - Route Foundation

1. Implement `src/routes/__root.tsx`.
2. Implement `src/routes/index.tsx` redirecting to `/dashboard`.
3. Implement `src/routes/login.tsx`.
4. Implement `src/routes/_authenticated.tsx` with `beforeLoad: requireAuth`.
5. Implement the authenticated child route files required by CP-01:
   - `dashboard.tsx`
   - `projects/index.tsx`
   - `projects/$projectId.tsx`
   - `projects/$projectId/business-logic.tsx`
   - `projects/$projectId/tech-specs.tsx`
   - `projects/$projectId/coding-plan.tsx`
   - `projects/$projectId/master-schedule.tsx`
   - `projects/$projectId/tasks.tsx`
   - `projects/$projectId/members.tsx`
   - `projects/$projectId/workflows.tsx`
   - `projects/$projectId/settings.tsx`
   - `ai-runs.tsx`
   - `settings/integrations.tsx`
   - `settings/prompt-templates.tsx`
6. Use lightweight placeholders where downstream feature content is not yet migrated.

### Phase G - Tailwind And UI Base

1. Ensure `vite.config.ts`, `postcss.config.mjs`, and Tailwind configuration are aligned with the migrated source tree.
2. Normalize the shared `cn()` utility and shadcn-compatible UI primitives under `src/components/ui`.
3. Keep the foundation visually functional, but avoid broad UI rewrites outside shell and route placeholders.

### Phase H - Remove Blocking Next.js Runtime Usage

1. Eliminate imports from `next/link`, `next/navigation`, `next/font`, `next/headers`, and `next/server` in files that remain active in the Vite build.
2. Remove or exclude `src/app/api/**` from the active app path for this phase.
3. Remove or isolate `src/app/**` and `next.config.ts` so the Vite build is the sole app entry.

## 5. Verification Sequence

1. Install dependencies successfully.
2. Run the Vite test suite for foundation coverage.
3. Run the production build.
4. Confirm the root route redirects correctly.
5. Confirm guest access to protected routes redirects to `/login`.
6. Confirm authenticated layout renders the shell and nested route outlet.

## 6. Done Criteria

- `apps/admin-web-nextjs-backup` exists as a backup of the pre-migration app.
- `apps/admin-web` builds as a Vite app.
- TanStack Router file-based routing is active.
- React Query is configured at the app root.
- Supabase browser client and auth guard are wired.
- `_authenticated` routes render inside `AppShell`.
- Domain entities, payloads, responses, gateways, and constants are ported into the active structure.
- Active code no longer depends on the Next.js app runtime.

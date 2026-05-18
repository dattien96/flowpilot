# 4C Summary - Next.js to Vite + TanStack Refactor

Date: 2026-05-18

## Context

- Workspace scope for this branch is `apps/admin-web`, plus any supporting root docs and Supabase migrations needed by the refactor.
- The current app is a Next.js 16 App Router admin shell with server layouts, server auth, route handlers, and a mixed demo/Supabase gateway bundle.
- `requirements/10-Refactor/R1-Refactor-To-React-Plan.md` defines the target state: migrate `apps/admin-web` to Vite + React + TanStack Router while preserving the existing domain/usecase/gateway layering and demo fallback.
- The plan also adds new route surfaces for artifact management, artifact memory detail, `ai-runs`, and prompt templates.

## Current Codebase Snapshot

- Presentation entry points live in `apps/admin-web/src/app/**`.
- Shared UI and shell components live in `apps/admin-web/src/presentation/**`.
- Domain entities, payloads, gateway interfaces, and use cases live in `apps/admin-web/src/domain/**`.
- Data adapters, auth/session helpers, and gateway bundles live in `apps/admin-web/src/data/**`.
- Current Next.js-specific dependencies and conventions still exist:
  - `src/app/layout.tsx`
  - `src/app/(protected)/layout.tsx`
  - `src/app/(auth)/login/page.tsx`
  - `src/app/api/**`
  - `next.config.ts`
  - `next-env.d.ts`

## Hard Constraints

- Preserve clean architecture:
  - presentation -> domain use cases -> gateway interfaces -> data adapters
- Keep demo mode usable when Supabase env is absent.
- Replace Next.js page and route conventions with Vite plus TanStack Router.
- Remove Next.js server-action and API-route dependence from the admin web app.
- Keep the refactor incremental enough to validate route parity before deleting legacy Next.js artifacts.

## Main Risks

- Auth and route-guard changes can lock users out if session handling is not re-created correctly.
- Import rewrites from `next/link` and `next/navigation` to TanStack Router can break navigation silently.
- The repository bundle factory must keep demo fallback deterministic while switching the browser stack.
- UI copy-forward work can accidentally retain server-only assumptions from the Next.js layout and protected pages.
- Route migration has a broad blast radius because many current pages are server-rendered by default.

## Decision

- Use Vite as the app shell.
- Use TanStack Router for route structure and protected navigation.
- Use React Query for server-state fetching.
- Keep the reusable domain/usecase/data logic intact and rewrite only the app-shell and data-access boundaries that are tied to Next.js.

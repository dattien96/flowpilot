# Task - Execute R1 Next.js to Vite + TanStack Migration

Date: 2026-05-18
Type: Planning, Architecture, and TDD
Primary module: `apps/admin-web`

## Goal

Migrate the current Next.js-based admin app to the Vite + React + TanStack stack described in `requirements/10-Refactor/R1-Refactor-To-React-Plan.md`, while preserving the reusable domain/usecase/data layers, demo fallback, and the route surfaces called out in the refactor plan.

## In Scope

- Back up or preserve the current Next.js app while scaffolding the new Vite app in `apps/admin-web`.
- Install and configure the Vite, TanStack Router, TanStack Query, Tailwind 4, shadcn, Supabase client, and Vitest stack.
- Copy or reuse the domain layer, payloads, entities, gateway interfaces, and use cases that are still valid.
- Rewrite auth and Supabase access for the browser-first Vite model.
- Rewrite the repository bundle selection so demo mode remains available when Supabase env is absent.
- Port the presentation layer to TanStack Router file routes.
- Replace `next/link`, `next/navigation`, Next layout, and API routes with the new stack.
- Add the route surfaces required by R1:
  - artifact management
  - artifact memory detail
  - `ai-runs`
  - prompt templates settings
- Add the query hooks required by R1, including `artifact-memories` and `workflow-prompt-context-items`.
- Update docs and setup instructions for the new stack.

## Non-Goals

- Do not implement unrelated product expansion beyond the R1 migration surface.
- Do not add new backend worker infrastructure.
- Do not replace the domain model with framework-specific state.
- Do not introduce AI provider execution beyond what the current shell already mocks or exposes.
- Do not remove demo mode.

## Success Criteria

- `apps/admin-web` runs as a Vite app.
- Existing domain and data logic still works through the new browser-first app shell.
- Protected navigation works in the new router.
- Demo fallback still renders usable seeded data.
- The new route tree matches the R1 plan.
- The app can be built and tested from the new stack without Next.js runtime assumptions.

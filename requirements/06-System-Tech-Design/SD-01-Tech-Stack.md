# Stack Decision

## Frontend

Selected:
- React 19
- Vite (dev server + production build)
- TypeScript
- Tailwind CSS 4
- shadcn/ui
- TanStack Router (type-safe file-based routing)
- TanStack Query (server-state management)
- Zustand (lightweight UI state)
- Zod (input validation)

**Previously considered:** Next.js 16. Rejected because:
- The admin-web is an internal SPA dashboard, not a public SEO-heavy website.
- SSR, Server Components, and Server Actions add unnecessary complexity.
- Auth/authorization should not depend on frontend routing — real protection must come from Supabase RLS and Edge Functions.
- React + Vite gives a lighter, more explicit architecture where every data fetch is a visible query hook.

## Backend
Supabase (Postgres + Auth + Storage + Realtime + Edge Functions)

## Runner
Golang + Docker

We use Golang Cobra CLI tool to receive our prompts provided by backend and execute them.

### Why:
- fast MVP development,
- good type safety,
- easy form validation,
- reusable UI components,
- good admin dashboard ecosystem.

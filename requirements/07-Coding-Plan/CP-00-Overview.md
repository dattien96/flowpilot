# FlowPilot Coding Plan - Master Index

This folder translates the tech designs in `06-System-Tech-Design/` into actionable, phase-by-phase coding plans for the **admin-web** frontend. The plans reflect the crucial tech stack shift from Next.js to **React + Vite + TanStack Router + TanStack Query**.

---

## Stack Decision (Final)

| Layer | Old (Current) | New (Target) |
|-------|---------------|--------------|
| Framework | Next.js 16 (SSR, Server Components) | Vite + React 19 (SPA) |
| Routing | Next.js App Router (`app/` folder convention) | TanStack Router (file-based, type-safe) |
| Data Fetching | Next.js Server Actions + `@supabase/ssr` | TanStack Query + `@supabase/supabase-js` (client only) |
| Auth | Supabase SSR cookies + Next.js middleware | Supabase Auth (client SDK) + RLS + Edge Functions |
| Styling | Tailwind CSS 4 + shadcn/ui | Tailwind CSS 4 + shadcn/ui (kept) |
| Validation | Zod 4 | Zod 4 (kept) |
| State | React context | Zustand (lightweight UI) + TanStack Query (server) |
| Build | `next build` | `vite build` |

## Why The Shift

The admin-web is an internal SPA dashboard, not a public SEO-heavy website. Next.js adds unnecessary complexity:

- SSR/Server Components are not needed for an authenticated admin panel.
- Server Actions couple backend logic to the frontend framework.
- Next.js middleware is the wrong place for authorization (real security = Supabase RLS + Edge Functions).

React + Vite + TanStack gives us a lighter, more explicit architecture where every data fetch is visible as a query hook and all security is enforced at the database/Edge Function level.

---

## Document Index

| Doc | Title | Maps From |
|-----|-------|-----------|
| [CP-01](./CP-01-Foundation-Setup.md) | Foundation Setup (Vite + TanStack + Auth) | SD-01, SD-02 |
| [CP-02](./CP-02-Admin-Web-Hot-Reload.md) | Admin-Web Docker Hot Reload | DX hardening for local development |
| [CP-03](./CP-03-Supabase-Integration.md) | Supabase Integration | CP-01, CP-09, CP-12, DX + data boundary hardening |
| [CP-04](./CP-04-Project-Management.md) | Project & Team CRUD | SD-04 |
| [CP-05](./CP-05-Project-Mcp-Context.md) | Project MCP Context | SS-01, SS-02, SD-05, SD-10 |
| [CP-06](./CP-06-Document-Workflow.md) | Business Logic -> Tech Spec -> Coding Plan | SD-05, SD-06 |
| [CP-07](./CP-07-Workflow-Engine-UI.md) | Workflow Engine UI & Execution Dashboard | SD-05, SD-09 |
| [CP-08](./CP-08-Master-Schedule-Tasks.md) | Master Schedule & Task Board | SS-03, SS-04 §3.5.9, SD-04 |
| [CP-09](./CP-09-AI-Orchestration.md) | AI Prompt Templates & Execution Logs | SD-06, SD-07, SD-08 |
| [CP-10](./CP-10-Integrations-Hardening.md) | Integration Hardening, Security & Audit | SD-04 Section 4, SD-03 |
| [CP-11](./CP-11-Go-Runner-Implementation.md) | Go-Runner CLI, Providers, Skills & Process Isolation | SD-03, SD-05 Section 3-5, SD-06, SD-07 |
| [CP-12](./CP-12-Artifact-Memory-RAG.md) | Artifact Memory, RAG & Prompt Context | SS-09, SD-10 |

---

## Implementation Phases (Build Order)

```text
Phase 1  -> CP-01: Scaffold Vite project, auth, layout, routing
Phase Parallel -> CP-02: Docker hot reload for admin-web (`just docker-up`) before heavy frontend iteration
Phase Parallel -> CP-03: Supabase env contract, client boundaries, storage, and Edge Function integration
Phase 2  -> CP-04: UUID project/team/member baseline and settings shell
Phase 3  -> CP-05: Project MCP context configuration and `integrations` table
Phase 4  -> CP-07: Canonical workflow schema, builder, execution dashboard, approval gates
Phase 4A -> CP-11A: Basic Go-Runner after workflow run/step state exists
Phase 5  -> CP-06: Business Logic / Tech Spec / Coding Plan artifact editors
Phase 6  -> CP-08: Master Schedule generator, task board
Phase 7  -> CP-09: AI prompt templates, execution log viewer, Edge Functions
Phase 8  -> CP-12: Artifact memory + RAG context resolver
Phase 8A -> CP-11B: Full Go-Runner context memory integration
Phase 9  -> CP-10: Integration hardening, RLS hardening, audit trails
```

## Migration Strategy

The current Next.js codebase in `apps/admin-web/` will be replaced in-place:

1. Back up the existing `apps/admin-web/` to `apps/admin-web-nextjs-backup/`.
2. Scaffold a new Vite + React + TS project in `apps/admin-web/`.
3. Port reusable layers: `domain/` (entities, gateways, constants, usecases) -> copy as-is.
4. Port UI components: `presentation/components/` -> move to `src/components/`.
5. Rewrite routing: Next.js `app/` routes -> TanStack Router file routes.
6. Rewrite data layer: Server Actions -> TanStack Query hooks + Supabase client SDK.

See [CP-01 Migration](./CP-01-Foundation-Setup.md) for the detailed refactor steps.

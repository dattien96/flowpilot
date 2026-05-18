# FlowPilot Coding Plan — Master Index

This folder translates the tech designs in `06-System-Tech-Design/` into actionable, phase-by-phase coding plans for the **admin-web** frontend. The plans reflect the **crucial tech stack shift** from Next.js to **React + Vite + TanStack Router + TanStack Query**.

---

## Stack Decision (Final)

| Layer | Old (Current) | New (Target) |
|-------|---------------|--------------|
| Framework | Next.js 16 (SSR, Server Components) | Vite + React 19 (SPA) |
| Routing | Next.js App Router (`app/` folder convention) | TanStack Router (file-based, type-safe) |
| Data Fetching | Next.js Server Actions + `@supabase/ssr` | TanStack Query + `@supabase/supabase-js` (client only) |
| Auth | Supabase SSR cookies + Next.js middleware | Supabase Auth (client SDK) + RLS + Edge Functions |
| Styling | Tailwind CSS 4 + shadcn/ui | Tailwind CSS 4 + shadcn/ui (**kept**) |
| Validation | Zod 4 | Zod 4 (**kept**) |
| State | React context | Zustand (lightweight UI) + TanStack Query (server) |
| Build | `next build` | `vite build` |

## Why The Shift

The admin-web is an **internal SPA dashboard**, not a public SEO-heavy website. Next.js adds unnecessary complexity:
- SSR/Server Components are not needed for an authenticated admin panel.
- Server Actions couple backend logic to the frontend framework.
- Next.js middleware is the wrong place for authorization (real security = Supabase RLS + Edge Functions).

React + Vite + TanStack gives us a lighter, more explicit architecture where every data fetch is visible as a query hook and all security is enforced at the database/Edge Function level.

---

## Document Index

| Doc | Title | Maps From |
|-----|-------|-----------|
| [CP-01](./CP-01-Foundation-Setup.md) | Foundation Setup (Vite + TanStack + Auth) | SD-01, SD-02 |
| [CP-02](./CP-02-Project-Management.md) | Project & Team CRUD | SD-04 |
| [CP-03](./CP-03-Document-Workflow.md) | Business Logic → Tech Spec → Coding Plan | SD-05, SD-06 |
| [CP-04](./CP-04-Workflow-Engine-UI.md) | Workflow Engine UI & Execution Dashboard | SD-05, SD-09 |
| [CP-05](./CP-05-Master-Schedule-Tasks.md) | Master Schedule & Task Board | Google Doc §4.6–4.7 |
| [CP-06](./CP-06-AI-Orchestration.md) | AI Prompt Templates & Execution Logs | SD-06, SD-07, SD-08 |
| [CP-07](./CP-07-Integrations-Hardening.md) | MCP/Jira Integration & Security Hardening | SD-04 §4, SD-03 |
| [CP-08](./CP-08-Go-Runner-Implementation.md) | Go-Runner CLI, Providers, Skills & Process Isolation | SD-03, SD-05 §3–5, SD-06, SD-07 |

---

## Implementation Phases (Build Order)

```
Phase 1 → CP-01: Scaffold Vite project, auth, layout, routing
Phase 2 → CP-02: Projects + Members CRUD
Phase 3 → CP-03: Business Logic / Tech Spec / Coding Plan editors
Phase 4 → CP-04: Workflow builder, execution dashboard, approval gates
Phase 5 → CP-05: Master Schedule generator, task board
Phase 6 → CP-06: AI prompt templates, execution log viewer
Phase 7 → CP-07: Jira/MCP integration, RLS hardening, audit trails
Phase ∥ → CP-08: Go-Runner CLI (cross-cutting, built alongside Phases 4–6)
```

## Migration Strategy

The current Next.js codebase in `apps/admin-web/` will be **replaced in-place**:
1. Back up the existing `apps/admin-web/` to `apps/admin-web-nextjs-backup/`.
2. Scaffold a new Vite + React + TS project in `apps/admin-web/`.
3. Port reusable layers: `domain/` (entities, gateways, constants, usecases) → copy as-is.
4. Port UI components: `presentation/components/` → move to `src/components/`.
5. Rewrite routing: Next.js `app/` routes → TanStack Router file routes.
6. Rewrite data layer: Server Actions → TanStack Query hooks + Supabase client SDK.

See [CP-01 §Migration](./CP-01-Foundation-Setup.md) for the detailed refactor steps.

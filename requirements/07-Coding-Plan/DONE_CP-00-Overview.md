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

# FlowPilot Admin Web

Vite-based admin shell for the FlowPilot MVP workflow skeleton. It runs in two modes:

- Demo mode when Supabase env is missing. In-memory demo repositories keep local exploration working.
- Supabase mode when all required root env values are present. Data persists through Supabase and protected pages require a Supabase Auth session.

## Required Env

Set these values in the repository root `.env` to enable Supabase mode.

The admin web runs from `apps/admin-web`, so Vite is configured to load env from the repo root rather than an app-local `.env` file.

```bash
SUPABASE_API_URL=...
SUPABASE_API_KEY=...
```

Optional when calling Supabase Edge Functions directly:

```bash
SUPABASE_API_EDGE_FUNCTION_URL=...
```

Optional local runner override:

```bash
FLOWPILOT_RUNNER_URL=http://127.0.0.1:4317
```

## Local Run

```bash
cd apps/admin-web
npm install
npm run dev
```

Open `http://localhost:3001`. If Supabase mode is enabled, create or invite a Supabase Auth user and sign in at `/login`.

## Docker Run

```bash
just docker-up
```

Open `http://localhost:3001`.

This Docker workflow is the default frontend dev loop:

- Edit files under `apps/admin-web/src/**` on the host machine.
- Vite runs inside Docker with polling enabled for bind-mounted files.
- Component and style changes should apply through HMR.
- Route or config changes may trigger a full-page reload instead.

## Database

Apply all SQL files under the repository root `supabase/migrations/` directory in timestamp order.

Today that includes:

- `supabase/migrations/20260515050000_admin_mvp_skeleton.sql`
- `supabase/migrations/20260519060000_cp02_project_team_management.sql`

Together they create the admin MVP tables, team-management tables, RLS policies, indexes, and seed data used by the current app.

## Current MVP Boundaries

- The workflow executor is intentionally mocked in both demo and Supabase modes.
- AI outputs, approvals, logs, and workflow run state are persisted through the active repository implementation.
- Local runner artifact sync remains a separate server-side boundary and is protected by the same admin session guard.

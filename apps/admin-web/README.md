# FlowPilot Admin Web

Next.js admin shell for the FlowPilot MVP workflow skeleton. It runs in two modes:

- Demo mode when Supabase env is missing. In-memory demo repositories keep local exploration working.
- Supabase mode when all required root env values are present. Data persists through Supabase and protected pages require a Supabase Auth session.

## Required Env

Set these values in the repository root `.env` to enable Supabase mode:

```bash
SUPABASE_API_URL=...
SUPABASE_API_KEY=...
SUPABASE_SERVICE_ROLE_KEY=...
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

Open `http://localhost:3000`. If Supabase mode is enabled, create or invite a Supabase Auth user and sign in at `/login`.

## Database

Apply `supabase/migrations/20260515050000_admin_mvp_skeleton.sql`. It creates the admin MVP tables, RLS policies, indexes, and seed data for projects, features, context sources, and the default workflow definition.

## Current MVP Boundaries

- The workflow executor is intentionally mocked in both demo and Supabase modes.
- AI outputs, approvals, logs, and workflow run state are persisted through the active repository implementation.
- Local runner artifact sync remains a separate server-side boundary and is protected by the same admin session guard.

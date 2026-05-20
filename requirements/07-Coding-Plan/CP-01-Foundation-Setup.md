# CP-01: Foundation Setup — Vite + TanStack + Auth

**Maps from:** SD-01 (Tech Stack), SD-02 (Architecture)
**Phase:** 1 (must be done first)
**Status:** implemented
---

## 1. Scaffold New Vite Project

### 1.1 Back Up Current Next.js Codebase
```bash
# From workspace root
mv apps/admin-web apps/admin-web-nextjs-backup
```

### 1.2 Create New Vite + React + TypeScript Project
```bash
npm create vite@latest apps/admin-web -- --template react-ts
cd apps/admin-web
npm install
```

### 1.3 Install Core Dependencies
```bash
# Routing
npm install @tanstack/react-router
npm install -D @tanstack/router-plugin @tanstack/router-devtools

# Data fetching
npm install @tanstack/react-query
npm install -D @tanstack/react-query-devtools

# Supabase (client-only, NO @supabase/ssr)
npm install @supabase/supabase-js

# UI
npm install tailwindcss @tailwindcss/postcss postcss autoprefixer
npm install class-variance-authority clsx tailwind-merge lucide-react

# Validation
npm install zod

# State (lightweight UI state)
npm install zustand

# Testing
npm install -D vitest @testing-library/react @testing-library/jest-dom jsdom
```

### 1.4 Initialize Tailwind CSS 4
```bash
npx tailwindcss init -p
```

### 1.5 Initialize shadcn/ui
```bash
npx shadcn@latest init
npx shadcn@latest add button input textarea card dialog sheet tabs badge table dropdown-menu form select checkbox command popover calendar toast
```

---

## 2. Folder Structure (Target)

```
apps/admin-web/
├── index.html
├── vite.config.ts
├── tsconfig.json
├── tailwind.config.ts
├── postcss.config.mjs
├── src/
│   ├── main.tsx                          # React entry point
│   ├── app.tsx                           # <RouterProvider> + <QueryClientProvider>
│   ├── router.tsx                        # TanStack Router instance
│   │
│   ├── routes/                           # TanStack Router file-based routes
│   │   ├── __root.tsx                    # Root layout (sidebar, header)
│   │   ├── index.tsx                     # Redirect to /dashboard
│   │   ├── login.tsx                     # Public login page
│   │   ├── _authenticated.tsx            # Auth guard layout route
│   │   ├── _authenticated/
│   │   │   ├── dashboard.tsx
│   │   │   ├── projects/
│   │   │   │   ├── index.tsx             # Project list
│   │   │   │   ├── $projectId.tsx        # Project detail layout
│   │   │   │   ├── $projectId/
│   │   │   │   │   ├── business-logic.tsx
│   │   │   │   │   ├── tech-specs.tsx
│   │   │   │   │   ├── coding-plan.tsx
│   │   │   │   │   ├── master-schedule.tsx
│   │   │   │   │   ├── tasks.tsx
│   │   │   │   │   ├── members.tsx
│   │   │   │   │   ├── workflows.tsx
│   │   │   │   │   └── settings.tsx
│   │   │   ├── ai-runs.tsx
│   │   │   ├── settings/
│   │   │   │   ├── integrations.tsx
│   │   │   │   └── prompt-templates.tsx
│   │
│   ├── components/                       # Shared UI components
│   │   ├── ui/                           # shadcn/ui primitives
│   │   ├── layout/                       # AppShell, Sidebar, Header
│   │   ├── project/                      # ProjectCard, ProjectForm
│   │   ├── workflow/                     # WorkflowBuilder, StepCard
│   │   ├── task/                         # TaskBoard, TaskCard
│   │   ├── schedule/                     # ScheduleTimeline, MilestoneCard
│   │   ├── ai/                           # AIRunPanel, PromptEditor
│   │   └── common/                       # StatusBadge, MarkdownViewer, EmptyState
│   │
│   ├── features/                         # Feature-specific hooks & query logic
│   │   ├── auth/
│   │   │   ├── use-auth.ts               # Zustand auth store
│   │   │   ├── auth-provider.tsx         # AuthContext with Supabase onAuthStateChange
│   │   │   └── require-auth.ts           # TanStack Router beforeLoad guard
│   │   ├── projects/
│   │   │   ├── queries.ts                # useProjects, useProject query hooks
│   │   │   └── mutations.ts              # useCreateProject, useUpdateProject
│   │   ├── business-logic/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── tech-specs/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── coding-plans/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── master-schedule/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── tasks/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── members/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── workflows/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   ├── ai-runs/
│   │   │   ├── queries.ts
│   │   │   └── mutations.ts
│   │   └── approvals/
│   │       ├── queries.ts
│   │       └── mutations.ts
│   │
│   ├── domain/                           # ← PORTED from existing Next.js codebase
│   │   ├── constant/
│   │   │   └── status.ts                 # WorkflowRunStatus, ApprovalStatus, etc.
│   │   ├── model/
│   │   │   ├── entity/                   # Project, Feature, Workflow, etc.
│   │   │   ├── payload/                  # Create/update request types
│   │   │   └── response/                 # Aggregated response types
│   │   ├── gateway/                      # Gateway interfaces (abstract)
│   │   └── usecase/                      # Business logic use cases
│   │
│   ├── data/                             # ← PORTED & REWRITTEN
│   │   ├── supabase/
│   │   │   ├── client.ts                 # createClient() with env vars
│   │   │   └── realtime.ts               # Realtime subscription helpers
│   │   └── repository/                   # Gateway implementations using Supabase client SDK
│   │       ├── project-repository.ts
│   │       ├── workflow-repository.ts
│   │       ├── feature-repository.ts
│   │       └── ...
│   │
│   ├── lib/
│   │   ├── utils.ts                      # cn() helper, etc.
│   │   ├── query-client.ts               # TanStack QueryClient config
│   │   └── validators/                   # Zod schemas
│   │
│   └── types/
│       └── env.d.ts                      # Vite env type declarations
```

---

## 3. Key Configuration Files

### 3.1 vite.config.ts
```typescript
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { TanStackRouterVite } from '@tanstack/router-plugin/vite'
import path from 'path'

export default defineConfig({
  plugins: [
    TanStackRouterVite(),
    react(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
})
```

### 3.2 Supabase Client (Client-Only)
```typescript
// src/data/supabase/client.ts
import { createClient } from '@supabase/supabase-js'

export const supabase = createClient(
  import.meta.env.VITE_SUPABASE_URL,
  import.meta.env.VITE_SUPABASE_ANON_KEY
)
```

**Key difference from Next.js:** No `@supabase/ssr`, no cookie-based sessions, no server-side client. All auth uses the browser's built-in Supabase session management.

### 3.3 Auth Guard (TanStack Router `beforeLoad`)
```typescript
// src/features/auth/require-auth.ts
import { redirect } from '@tanstack/react-router'
import { supabase } from '@/data/supabase/client'

export async function requireAuth() {
  const { data: { session } } = await supabase.auth.getSession()
  if (!session) {
    throw redirect({ to: '/login' })
  }
  return session
}
```

### 3.4 Root Layout Route
```typescript
// src/routes/__root.tsx
import { Outlet, createRootRoute } from '@tanstack/react-router'
import { TanStackRouterDevtools } from '@tanstack/router-devtools'

export const Route = createRootRoute({
  component: () => (
    <>
      <Outlet />
      {import.meta.env.DEV && <TanStackRouterDevtools />}
    </>
  ),
})
```

### 3.5 Authenticated Layout Route
```typescript
// src/routes/_authenticated.tsx
import { Outlet, createFileRoute } from '@tanstack/react-router'
import { requireAuth } from '@/features/auth/require-auth'
import { AppShell } from '@/components/layout/app-shell'

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: requireAuth,
  component: () => (
    <AppShell>
      <Outlet />
    </AppShell>
  ),
})
```

---

## 4. Migration Checklist — What To Port

### 4.1 Copy As-Is (No Changes Needed)
| Source (Next.js) | Destination (Vite) | Notes |
|---|---|---|
| `src/domain/model/entity/*.ts` | `src/domain/model/entity/*.ts` | All 5 entity files: Project, Feature, ContextSource, Workflow, LocalRunner |
| `src/domain/model/payload/*.ts` | `src/domain/model/payload/*.ts` | Request payloads |
| `src/domain/model/response/*.ts` | `src/domain/model/response/*.ts` | Response types |
| `src/domain/constant/status.ts` | `src/domain/constant/status.ts` | Status enums |
| `src/domain/gateway/*.ts` | `src/domain/gateway/*.ts` | 5 gateway interfaces |

### 4.2 Rewrite Required
| Source (Next.js) | Destination (Vite) | Rewrite Reason |
|---|---|---|
| `src/data/auth/session.ts` | `src/features/auth/use-auth.ts` | SSR cookie session → client-side Supabase auth |
| `src/data/datasource/supabase/*` | `src/data/supabase/client.ts` | `createServerClient` → `createClient` |
| `src/data/repository/supabase/*` | `src/data/repository/*.ts` | Remove SSR patterns, use client SDK directly |
| `src/app/(protected)/layout.tsx` | `src/routes/_authenticated.tsx` | Next.js layout → TanStack Router layout route |
| `src/app/(auth)/login/page.tsx` | `src/routes/login.tsx` | Next.js page → TanStack route component |
| `src/app/(protected)/projects/page.tsx` | `src/routes/_authenticated/projects/index.tsx` | Next.js page → TanStack route |
| `src/app/(protected)/dashboard/page.tsx` | `src/routes/_authenticated/dashboard.tsx` | Next.js page → TanStack route |
| All `app/(protected)/*/page.tsx` | Corresponding `routes/_authenticated/*.tsx` | Same pattern for all routes |

### 4.3 UI Components — Port With Minor Changes
| Source (Next.js) | Destination (Vite) | Change |
|---|---|---|
| `src/presentation/components/ui/*` | `src/components/ui/*` | No changes (shadcn primitives) |
| `src/presentation/components/layout/*` | `src/components/layout/*` | Remove `next/font` imports → use CSS `@import` for Google Fonts |
| `src/presentation/components/projects/*` | `src/components/project/*` | Remove `next/link` → use `<Link>` from TanStack Router |
| `src/presentation/components/workflow-runs/*` | `src/components/workflow/*` | Same link migration |

### 4.4 Delete (Not Needed in Vite)
- `next.config.ts`
- `next-env.d.ts`
- `src/app/api/` (API routes → use Supabase Edge Functions)
- `@supabase/ssr` dependency
- `src/data/auth/session.ts` (SSR session helper)

---

## 5. Environment Variables

```env
# .env.local
VITE_SUPABASE_URL=http://localhost:54321
VITE_SUPABASE_ANON_KEY=your-anon-key
VITE_LOCAL_RUNNER_URL=http://localhost:9100
```

**Note:** Vite uses `VITE_` prefix instead of `NEXT_PUBLIC_`.

---

## 6. Development Commands

```bash
# Dev server
npm run dev

# Build
npm run build

# Preview production build
npm run preview

# Test
npm run test
```

---

## 7. Definition of Done — Phase 1

- [ ] Vite project scaffolded with React 19 + TypeScript
- [ ] TanStack Router configured with file-based routing
- [ ] TanStack Query configured with QueryClientProvider
- [ ] Supabase client SDK initialized (client-only)
- [ ] Auth flow working: login → session → protected routes → logout
- [ ] `_authenticated` layout route with `beforeLoad` guard
- [ ] AppShell layout: left sidebar, top header, main content area
- [ ] shadcn/ui initialized with core components
- [ ] All domain entities, gateways, and constants ported from Next.js backup
- [ ] Environment variables configured
- [ ] Dev server runs at `localhost:5173`

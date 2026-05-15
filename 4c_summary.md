# 4C Summary - Admin MVP Skeleton

Date: 2026-05-15

## Context

- Workspace scope: `apps/admin-web` and `supabase`
- Current repo state: both areas are present but effectively empty; `supabase` only contains placeholder `functions/` and `migrations/`
- Confirmed stack from `README.md`: Next.js, TypeScript, Tailwind CSS, shadcn/ui, Supabase Auth, Supabase Postgres, Supabase Realtime, Supabase Storage, optional Supabase Edge Functions
- Source of truth documents:
  - `README.md`
  - Google Doc: `API_FLOW - 04 - Short-term Implementation Plan - Admin MVP Skeleton First`
- Stable entities that must anchor the design:
  - `project`
  - `feature`
  - `context_source`
  - `workflow_definition`
  - `workflow_run`
  - `workflow_step`
  - `ai_output`
  - `approval`
  - `ai_call_log`

## Command

- Produce planning-only deliverables at the project root:
  - `4c_summary.md`
  - `task.md`
  - `implementation_plan.md`
  - `tdd_signatures.md`
- Plan the Admin MVP Skeleton as a Next.js admin shell with Supabase persistence and a mock workflow executor
- Do not design for real AI orchestration yet
- Preserve a swappable service boundary so a future real backend can replace the mock executor without changing the UI contract

## Constraints

- Use the current confirmed stack, not the older React/Vite wording inside the Google Doc
- Keep the workflow model aligned to the stable entities listed above
- Treat persistence and workflow state as first-class, not temporary scaffolding
- Avoid direct UI coupling to mock execution details
- Do not include Jira, Firebase, Context7, RTK/GitNexus, RAG, Telegram, MCP, or real provider calls in the MVP skeleton
- Write all deliverables directly to the project root

## Criteria

- The plan must cover Planning, Architecture, and TDD only
- The architecture must define:
  - package and route structure for `apps/admin-web`
  - Supabase ownership for auth, persistence, and realtime
  - a stable API/service contract for workflow operations
  - a mock executor implementation that can later be swapped for a real orchestrator
- The TDD artifact must define signatures for the key flows:
  - auth and shell protection
  - project CRUD
  - feature intake
  - context source management
  - workflow definition seed/load
  - workflow run creation
  - mock execution progression
  - approval actions
  - outputs library
  - ai call log visibility

## Design Options

### Option A - Client-heavy app using direct Supabase calls from React components

- Fastest initial build
- Weak replacement path later because UI becomes coupled to Supabase schemas and mock workflow mechanics
- Rejected for this task

### Option B - Next.js route handlers plus internal services, with Supabase behind the server boundary

- UI talks to stable app-owned contracts
- Supabase stays private to server-side repositories/services
- Mock executor can later be replaced by a backend adapter without changing page-level data contracts
- Recommended

### Option C - Supabase Edge Functions as the primary app API from day one

- Clean separation for workflow execution
- Adds operational complexity too early for an otherwise empty workspace
- Useful later, but too much ceremony for the initial skeleton

## Recommended Direction

Use Option B.

Reason:

- It matches the confirmed Next.js stack
- It keeps the UI contract stable
- It supports fast MVP delivery while preserving the path to a future real backend
- It avoids throwaway component logic and avoids overcommitting to Supabase-specific client code

## Explicit Assumptions

- Next.js App Router will be used
- Workflow execution mutations will go through Next.js route handlers or server actions backed by service interfaces
- The initial mock executor may run in-process on the server and can optionally move to Supabase Edge Functions later without changing the UI contract
- Realtime is used for run status and approval visibility only after the core persistence model is in place

## Definition Of Done For This Planning Pass

- A clear root-level execution task file exists
- A Next.js-first implementation plan exists
- A TDD signature file exists
- All artifacts maintain the stable entity vocabulary and the swappable executor boundary

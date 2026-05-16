# 4C Summary - Admin MVP Skeleton Pending Features

Date: 2026-05-15

## Context

- Workspace scope for implementation: `apps/admin-web`, `supabase`, root `.env`, root `README.md`
- Current app shape already follows a useful coarse boundary:
  - presentation: `src/app`, `src/presentation`
  - domain: `src/domain`
  - data: `src/data`
- Current persistence state:
  - `supabase/migrations/20260515050000_admin_mvp_skeleton.sql` defines base tables
  - `createGatewayBundle()` still returns the demo bundle even when Supabase env exists
  - current env detection uses `NEXT_PUBLIC_SUPABASE_URL` and `NEXT_PUBLIC_SUPABASE_ANON_KEY`
  - requested implementation must instead wire root `.env` values `SUPABASE_API_URL`, `SUPABASE_API_KEY`, `SUPABASE_SERVICE_ROLE_KEY`, while preserving demo fallback
- Current auth state:
  - `(protected)` routes are not actually protected
  - login page explicitly says auth is not wired yet
  - Supabase SSR client exists but is not used for route/page protection
- Current product gaps in code:
  - feature create form hardcodes `project_meal_suggestion`
  - context sources page is read-only and also hardcodes one project id
  - no workflow definitions page
  - approval center is list-only and links to run detail
  - workflow run detail shows only latest output, timeline, and three bare decision forms
  - outputs page is artifact-browser oriented, not AI-output-library oriented
  - logs page is an unfiltered flat table
  - workflow gateway contracts are missing richer query and mutation methods for the requested UX
  - no meaningful automated tests are present yet in `apps/admin-web`
- GitNexus constraint awareness:
  - attempted lookup for repo `flowpilot` failed
  - available indexed repos in this session do not include this project
  - planning should not depend on GitNexus availability

## Command

Produce planning artifacts only, written directly to the project root:

- `4c_summary.md`
- `task.md`
- `implementation_plan.md`
- `tdd_signatures.md`

The plan must cover this implementation scope:

- wire real Supabase persistence using `SUPABASE_API_URL`, `SUPABASE_API_KEY`, `SUPABASE_SERVICE_ROLE_KEY`
- preserve demo fallback when required env is missing
- add Supabase Auth and session protection for protected routes and protected APIs
- add context source CRUD
- replace hardcoded project selection in feature intake
- add workflow definitions page
- improve approval center with output preview, comments, request-changes behavior, and decision history
- improve output library with AI output detail, version, approval, and export support
- improve workflow run detail with selected context, per-step outputs, logs/metadata, and resume/cancel placeholders
- add logs/cost dashboard filters and summaries
- update RLS and seed migrations
- add meaningful use case and route tests
- fix local-runner path normalization regression/test
- update docs and README

## Constraints

- Respect:
  - `AGENTS.md`
  - `.agents/AGENTS.md`
  - `.agents/skills/project/flowpilot-admin-rules.md`
  - React/Next best-practice guidance already loaded for this repo
- Keep clean architecture strict:
  - presentation -> domain use cases -> gateway interfaces -> data adapters
- Do not introduce:
  - real AI provider calls
  - NestJS
  - backend worker
  - Jira, Firebase, Context7, RAG, Telegram, MCP integrations
- Keep workflow state, approvals, outputs, and logs persisted in Supabase when env is available
- Keep demo mode working when Supabase env is absent or intentionally disabled
- Keep Next.js-specific implementation practical for the current App Router structure
- Planning artifacts must be actionable for this repository, not generic product docs

## Criteria

- Planning artifact quality:
  - grounded in the actual files and gaps in `apps/admin-web`
  - explicit about server/client boundaries in Next.js
  - explicit about Supabase client ownership and env usage
  - explicit about fallback behavior between Supabase mode and demo mode
- Architecture quality:
  - protected pages and API routes authenticate server-side
  - route handlers stay thin and delegate to domain use cases
  - repository/gateway contracts expand only where needed for the MVP scope
  - output/approval/workflow pages can evolve later without coupling UI to the mock executor
- TDD quality:
  - covers use case behavior, route protection, repository persistence rules, and the local-runner normalization fix
  - includes approval `changes_requested` behavior and decision history
  - includes logs summaries/filters and output versioning/detail cases

## Current Risks

- `createGatewayBundle()` is misleading today because Supabase env detection does not change repository behavior
- `SubmitApprovalDecisionUseCase` currently locates approvals by iterating all runs; that will not scale and is not a clean protected-route contract
- current schema lacks enough metadata for history, richer output detail, and operational logs/dashboard queries
- current pages perform server rendering directly from gateway bundles without auth/session guardrails
- current outputs UX is centered on local runner artifacts rather than persisted `ai_outputs`

## Recommended Direction

- Use Supabase-backed repositories behind the existing gateway bundle factory
- Split environment ownership into:
  - public/SSR auth client based on `SUPABASE_API_URL` + `SUPABASE_API_KEY`
  - server-only admin/service client based on `SUPABASE_SERVICE_ROLE_KEY`
- Keep demo bundle as the fallback implementation selected by the factory
- Add a small auth/session layer in data/presentation, not in domain
- Expand domain contracts to support list/detail/filter/update operations needed by approvals, outputs, workflow definitions, and logs
- Treat workflow execution as still mock/in-process, but persist all run/step/output/log effects through the active repository implementation

## Definition Of Done For This Planning Pass

- `task.md` states the concrete implementation mission and boundaries for this repo
- `implementation_plan.md` gives a Next.js-specific build sequence and architecture
- `tdd_signatures.md` defines the target test inventory and expected behavior signatures
- all planning artifacts acknowledge GitNexus is unavailable here and do not rely on it

# Task - Implement Pending Admin MVP Skeleton Features

Date: 2026-05-15
Type: Planning, Architecture, and TDD
Primary module: `apps/admin-web`
Supporting modules: `supabase`, root docs, root env handling

## Goal

Prepare the implementation handoff for the pending Admin MVP Skeleton work already implied by the docs and current codebase. The implementation must move the admin app from a demo-only shell to a Supabase-backed MVP with protected routes, persistent workflow state, richer approval/output/workflow views, and meaningful test coverage, while preserving demo fallback.

## In Scope

- Supabase persistence wiring using root `.env`:
  - `SUPABASE_API_URL`
  - `SUPABASE_API_KEY`
  - `SUPABASE_SERVICE_ROLE_KEY`
- Demo fallback when Supabase env is unavailable
- Supabase Auth session protection for protected pages and protected route handlers
- Context source CRUD
- Feature intake project selection sourced from real projects instead of hardcoded ids
- Workflow definitions listing/detail page
- Approval center improvements:
  - output preview
  - decision comments
  - request-changes behavior
  - decision history
- Output library improvements:
  - persisted AI outputs
  - output detail
  - version/approval state
  - export support
- Workflow run detail improvements:
  - selected context
  - per-step outputs
  - logs and metadata
  - resume/cancel placeholders
- Logs/cost dashboard summaries and filters
- RLS and seed migration updates
- Meaningful use case and route tests
- Local-runner path normalization test fix
- README and setup docs updates

## Non-Goals

- No real AI provider execution
- No NestJS service
- No background worker
- No Jira, Firebase, Context7, RAG, Telegram, or MCP integrations
- No redesign of the clean architecture layering already present

## Current State Snapshot

- Supabase tables exist in one migration, but repository factory still serves demo repositories only
- auth is not enforced on `(protected)` routes or protected APIs
- pages for approvals, outputs, workflow runs, and logs are minimal
- context sources cannot be edited or deleted
- feature intake hardcodes one project id
- no workflow definitions page exists
- tests are effectively absent in `apps/admin-web`

## Architecture Guardrails

- Presentation may depend on domain use cases only
- Domain may depend on gateway interfaces only
- Data owns Supabase, auth/session, and persistence details
- Mock workflow execution remains behind gateway/use-case boundaries
- Page and route contracts must not assume demo-only data access

## Delivery Order

1. Foundation
2. Auth protection
3. Supabase repositories and fallback factory
4. Context/project/feature workflow
5. Workflow definitions and run detail enhancements
6. Approval and output library enhancements
7. Logs summaries and migration hardening
8. Test coverage and docs

## Expected Implementation Outcome

After implementation, the admin app should be able to run in two modes:

- Supabase mode:
  - authenticated users can access protected routes
  - workflow data persists across refreshes
  - approvals, outputs, and logs reflect real database state
- Demo mode:
  - the current seeded experience still works for local exploration when Supabase env is not configured

## Planning Output For This Task

- `4c_summary.md`: problem framing and repo constraints
- `implementation_plan.md`: actionable Next.js implementation architecture and sequence
- `tdd_signatures.md`: target behavior signatures for tests and regression coverage

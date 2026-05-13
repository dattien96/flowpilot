# Task - Admin MVP Skeleton

Date: 2026-05-15
Type: New feature
Primary module: `apps/admin-web`
Supporting module: `supabase`

## Goal

Build the planning baseline for an Admin MVP Skeleton that proves workflow state, approval gates, output history, and context visibility using Next.js and Supabase, with a mock workflow executor behind a replaceable service boundary.

## Phase Breakdown

### Phase 1 - Planner

- Confirm scope from `README.md` and the Google Doc
- Freeze the stable entity vocabulary
- Translate the old React/Vite implementation notes into a Next.js-first plan
- Choose the boundary strategy for future backend replacement

### Phase 2 - Architecture

- Define app structure for `apps/admin-web`
- Define route groups, shared layout, and protected shell boundaries
- Define server-side modules for repositories, services, and workflow execution
- Define Supabase schema ownership and realtime usage
- Define the contract between UI and workflow services

### Phase 3 - TDD

- Identify the initial contract tests for route handlers and service behavior
- Identify repository and orchestration logic paths
- Identify approval and workflow progression cases
- Document test signatures only, without implementation

### Phase 4 - Coding Handoff

- Bootstrap Next.js admin app structure
- Add Supabase environment and client/server wiring
- Create initial SQL migrations and seed data
- Implement protected shell, CRUD, workflow run flows, approvals, outputs, and logs

### Phase 5 - Review Handoff

- Validate entity naming consistency
- Validate the UI contract is not coupled to mock executor internals
- Validate refresh persistence, approval pauses, and workflow resume behavior
- Validate the MVP excludes real AI orchestration concerns

## Implementation Streams

### Stream A - Platform foundation

- Next.js app bootstrap
- Tailwind and shadcn/ui setup
- Auth/session plumbing
- shared app shell

### Stream B - Data foundation

- Supabase schema and RLS
- seed workflow definition
- seed sample project and feature

### Stream C - Core product flows

- project CRUD
- feature intake
- context source management
- workflow run creation
- mock workflow execution
- approval center
- outputs library
- log placeholder

### Stream D - Replacement-safe boundaries

- UI-facing API contracts
- service interfaces for workflow execution
- repository interfaces for persistence access
- adapter slot for future real orchestrator

## Key Non-Goals

- real AI provider integration
- queue workers
- external MCP integrations
- RAG or long-running orchestration
- CI/CD or production incident automation

## Handoff Notes

- The implementation should favor server-owned mutations over direct client writes for workflow operations
- The workflow executor must be swappable without requiring page or component contract changes
- The stable entities in this file and the other planning artifacts should be treated as v0.1 vocabulary

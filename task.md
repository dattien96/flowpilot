# Task - Execute CP-01 Foundation Setup

Date: 2026-05-18
Type: Planning, Architecture, and TDD
Primary module: `apps/admin-web`

## Goal

Execute `requirements/07-Coding-Plan/CP-01-Foundation-Setup.md` exactly enough to establish the new frontend foundation for later phases:

- Vite + React + TypeScript app shell
- TanStack Router route tree
- TanStack Query app wiring
- Supabase browser auth setup
- authenticated layout shell
- ported domain contracts and constants

## In Scope

- Backup of the current `apps/admin-web`
- Vite-first dependency and config normalization
- Route skeleton creation for all CP-01 pages
- browser-only auth/session rewrite
- shell layout and shared utility normalization
- domain constants, entities, payloads, responses, and gateways carryover
- removal or isolation of active Next.js runtime dependencies

## Non-Goals

- Full business implementation for every route
- Complete data repository migration for all feature modules
- Backend or Edge Function work
- Product-scope expansion beyond the CP-01 route surface

## Success Criteria

- The active app is a Vite app, not a Next.js runtime app.
- Protected routing works through TanStack Router guards.
- The shell renders authenticated pages inside a common layout.
- The base route tree required by CP-01 exists.
- The project builds and the foundation tests pass.

# 4C Summary - CP-01 Foundation Setup

Date: 2026-05-18

## Context

- The requested execution target is `requirements/07-Coding-Plan/CP-01-Foundation-Setup.md`.
- The active frontend lives in `apps/admin-web`.
- The current app is partially migrated already, but it still mixes Vite files with Next.js runtime structure.
- The foundation phase must establish a stable Vite + TanStack base before later feature migrations continue.

## Current Codebase Snapshot

- `apps/admin-web` contains both Vite files (`vite.config.ts`, `src/main.tsx`, `src/app.tsx`) and legacy Next.js files (`src/app/**`, `next.config.ts`, `src/app/api/**`).
- Some auth and layout files already exist outside the old Next.js tree, but the target CP-01 folder structure is not yet complete.
- Domain contracts already exist and can be ported with low semantic risk.
- The largest technical risk is not missing business logic; it is leaving conflicting app-entry and routing assumptions active in the same module.

## Constraints

- Use a browser-only Supabase client.
- Use TanStack Router file-based routing.
- Keep demo-safe, framework-agnostic domain contracts portable.
- Preserve a backup of the current Next.js app before reshaping the active tree.
- Make the active build path free of Next.js runtime requirements.

## Concerns

- The existing mixed tree may hide dormant imports from `next/*` that will break the Vite build.
- Reorganizing route files has a broad file-level blast radius.
- Over-migrating feature logic in this phase would slow delivery and increase breakage risk.

## Course Of Action

- Build the minimum solid foundation first.
- Use placeholder route components where downstream features are not yet ready.
- Port stable types and interfaces immediately.
- Defer deeper feature rewrites until later coding plans.

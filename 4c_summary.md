# 4C Summary - CP-02 Project & Team Management

Date: 2026-05-18

## Context

- The requested execution target is `requirements/07-Coding-Plan/CP-02-Project-Management.md`.
- The active app is `apps/admin-web`.
- The current project surface already includes project listing and project detail pages, but the detail experience still centers on features and workflow runs rather than teams, members, and project settings.
- The phase requires extending the existing project domain with team management, project-team linking, and project settings while keeping the current admin shell consistent.

## Current Codebase Snapshot

- `apps/admin-web/src/domain/gateway/project-gateway.ts` currently exposes only project list/detail/create operations.
- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.ts` maps projects, features, context sources, and workflow artifacts, but has no team or integration persistence.
- `apps/admin-web/src/app/(protected)/projects/[projectId]/page.tsx` still renders feature and workflow cards.
- The project area is already central to the app, so changes here will propagate through domain use cases, gateway bundles, and the protected route tree.

## Constraints

- Preserve existing project behavior while expanding the domain for teams and settings.
- Keep the new schema aligned with Supabase/Postgres conventions used by the repository.
- Avoid duplicating CP-07 integration storage rules; only surface the project settings hook points required by CP-02.
- Prefer incremental route expansion over a full visual redesign.

## Concerns

- The project gateway is a shared abstraction used by multiple use cases and repositories.
- The existing detail page is feature-centric, so introducing a team/settings layout will require careful route and component reshaping.
- Database schema changes need to be reflected consistently across domain entities, Supabase mappings, and any table-backed queries.

## Course Of Action

- Extend the domain model first with teams, team members, and updated project fields.
- Add project-team linkage and team CRUD on the gateway layer.
- Update the Supabase bundle to map the new tables and project columns.
- Rework the project detail route into a tabbed project-management shell with placeholder tabs where later phases will deepen behavior.
- Add the project members and settings surfaces required by CP-02 while leaving MCP installation details for CP-07.

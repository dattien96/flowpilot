# Task - Execute CP-05 Project & Team Management

Date: 2026-05-18
Type: Planning, Architecture, and TDD
Primary module: `apps/admin-web`

## Goal

Execute `requirements/07-Coding-Plan/CP-05-Project-Management.md` to establish the project management foundation:

- teams and team members domain support
- project-team linking
- updated project entity fields
- project detail tab layout for management surfaces
- project members and settings routes
- Supabase schema and repository support for the new tables

## In Scope

- design-level migration plan for the new schema
- domain entity and gateway expansion
- Supabase repository mapping updates
- project detail route reshaping
- project members and settings route surfaces
- query hook shape updates where needed for the project management UI

## Non-Goals

- CP-10 integration installation flow
- full Jira/Figma/Google Drive connector configuration logic
- broad workflow or feature domain rewrites unrelated to project management
- unnecessary redesign of unrelated screens

## Success Criteria

- the design artifacts fully cover the CP-05 schema, domain, gateway, and UI surfaces
- the plan preserves existing project behavior while adding team management support
- the project detail experience includes the required management tabs
- the project settings surface exposes storage preference and MCP context status placeholders


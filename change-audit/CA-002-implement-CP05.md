# CA-002 Implement CP-05

## Scope

This audit records the completed implementation work for:

- `CP-05-Project-Mcp-Context`
- `CP-05-01-Jira-Mcp-Api-Token`
- `CP-05-02-Mcp-Test-Console`

It covers the product model split between:

- MCP type
- MCP instance
- project link

It also covers the supporting admin-web, local-runner, and Supabase changes required to make that model real.

## Completed

- [x] Updated CP-05 product architecture to use MCP type, MCP instance, and project-link separation. Summary: split the old project-centric MCP model into global type inventory, reusable instances, and project-only links.
- [x] Updated CP-05 system design to reflect remote versus local MCP categories. Summary: defined two MCP categories and clarified which ones enable, install, or create reusable instances.
- [x] Updated the Jira MCP API token plan. Summary: moved Jira token handling into the new instance flow so fresh machines can reconnect without recreating the MCP.
- [x] Updated the MCP Test Console plan for the dedicated test route and Jira-first actions. Summary: moved test execution to its own route and defined collapsible Jira actions for bugs, user stories, and ticket content.
- [x] Added provider follow-up plans for Driver, Firebase, and Tele MCPs. Summary: documented the remaining MCP types so the same type/instance model can expand beyond Jira.
- [x] Updated workflow and implementation planning docs to match the new MCP model. Summary: aligned roadmap language, execution phases, and task breakdowns with the new MCP ownership model.
- [x] Added a shared MCP integration configuration module for provider fields and defaults.
- [x] Added integration domain entities and payload models for global MCP management.
- [x] Added an `IntegrationGateway` contract for CRUD and project-link operations.
- [x] Added local-runner domain contracts for MCP backend and runner health.
- [x] Added demo repository support for MCP integrations.
- [x] Added Supabase repository support for MCP integrations.
- [x] Added local-runner HTTP repository support for MCP actions.
- [x] Added app shell and navigation updates for MCP and runner settings entries. Summary: added the new settings routes to the navigation shell so MCP and runner pages are reachable from the UI.
- [x] Added the global MCP servers page with type inventory and instance management sections.
- [x] Added MCP type enable and disable controls.
- [x] Added the disabled-state grayout treatment for MCP type cards.
- [x] Moved MCP creation into a dedicated `/settings/mcp-servers/create` route.
- [x] Restricted the provider dropdown to enabled MCP types only.
- [x] Added the dedicated `/settings/mcp-servers/mcp-connect-test` route.
- [x] Added the Jira-first MCP Test Console with collapsible action groups.
- [x] Added the dedicated `/settings/runner` page for runner reachability.
- [x] Split project listing into `/projects` and `/projects/create`.
- [x] Kept project settings focused on linking and unlinking existing MCP instances only.
- [x] Hid project settings actions that should no longer be visible when already linked.
- [x] Added route generation and route-map updates for the new admin-web pages.
- [x] Added local runner CLI plumbing for MCP execution and management.
- [x] Added local runner secret-store support for stored credentials.
- [x] Expanded the Go runner with Jira test templates, Jira instance handling, and better error reporting.
- [x] Added local runner tests for Jira list, ticket content, verify, and delete flows.
- [x] Added schema migrations for MCP context, project MCP links, and MCP type enablement.
- [x] Added the combined dev helper command in `Justfile`.
- [x] Verified the admin-web build after the routing and MCP page changes.
- [x] Verified the local runner test suite after the MCP execution changes.
- [x] Refreshed GitNexus indexing after the implementation pass.

## Verification

Verified during the implementation pass:

- `npm run build` in `apps/admin-web`
- targeted admin-web Vitest coverage for MCP-related routes and repositories
- `go test ./internal/runner ./internal/cli` in `apps/local-runner`
- GitNexus reindex after the code changes

## Residual Notes

- The implemented work aligns the product around:
  - MCP types as global capabilities
  - MCP instances as reusable configured connections
  - project settings as link-only assignment
- The dedicated MCP Test Console now covers Jira first and leaves the other type sections ready for follow-up work.
- The `change-audit` note is intentionally implementation-focused and does not replace the formal coding-plan docs.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05
change_type: feature
summary: Implement CP-05
# --->8---

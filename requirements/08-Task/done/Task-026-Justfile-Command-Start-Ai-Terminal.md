# Task-026: Justfile Command Start AI Terminal

## Metadata

- Document ID: `Task-026`
- Title: `Justfile Command Start AI Terminal`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot team`
- Created: `2026-06-09`
- Last Updated: `2026-06-09`
- Parent Documents:
  - [CP-26: Env-Liked Proxy System](../../07-Coding-Plan/done/CP-26-Env-Liked-Proxy-System.md)
  - [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
  - [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `None`
- Related Documents:
  - [Task-025: Drive MCP Auth Flow](../done/Task-025-Drive-MCP-Auth-Flow.md)
  - [CP-05-03: Google Drive MCP Implementation Plan](../../07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md)
- Replaces: `None`
- Tags: `justfile`, `local-runner`, `provider-accounts`, `terminal`

## AI Quick View

### Summary

- Added a `Justfile` recipe that starts the provider terminal flow without starting the web server.
- Added a `providers terminal` Cobra command that lists connected AI provider accounts, shows usage metadata, and prompts for a selection.
- Reused the existing runner terminal-launch path so the selected account opens in the same way the dashboard launcher does.

### Current Ask

- Document the completed Justfile-driven terminal launch flow and its implementation scope.

### Key Decisions

- `T-1` Keep the flow serverless: the recipe runs `go run ./cmd/flowpilot providers terminal` directly.
- `T-2` Prompt from the local runner account inventory instead of hard-coding provider/account choices.
- `T-3` Show account usage context inline so the selection step matches the dashboard experience.

### Constraints

- Do not start the HTTP server for this flow.
- Only offer connected provider accounts for launch.
- Preserve the existing runner terminal behavior for the selected account.

### Open Questions

- None.

### Source Refs

- [CP-26: Env-Liked Proxy System](../../07-Coding-Plan/done/CP-26-Env-Liked-Proxy-System.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [CP-05-03: Google Drive MCP Implementation Plan](../../07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md)

## 1. Goal

Let a user run one command from `Justfile` to choose a connected AI provider account, review its usage limits, and open a terminal for that account without starting the local server.

The intended flow is:

- user runs the Justfile recipe
- the runner lists connected provider accounts with usage context
- user selects the desired account
- the runner opens the same provider terminal flow used by the dashboard launcher

## 2. Parent Links

- coding plan: [CP-26: Env-Liked Proxy System](../../07-Coding-Plan/done/CP-26-Env-Liked-Proxy-System.md)
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `CP-26`, `SD-06`, `SS-11`, `CP-05-03`

## 3. Trigger

The dashboard already exposed a provider-account terminal launcher, but that flow required going through the UI and the local server.

This task exists to give the same practical behavior from the command line so a user can:

- skip the server startup path
- inspect connected accounts and their usage limits first
- launch the selected account directly from `Justfile`

## 4. Exact Change

- `T-1` Added `runner-provider-terminal` to `Justfile` so the terminal-launch flow can be started with one command from the repo root.
- `T-2` Added a `providers terminal` Cobra subcommand in the local runner that reads provider accounts, filters to connected accounts, prints usage context, prompts for a selection, and opens the terminal for the chosen account.
- `T-3` Added focused CLI tests for selection parsing and provider usage-summary formatting so the prompt flow stays stable.
- `T-4` Kept the existing dashboard and runner launch behavior intact instead of introducing a separate terminal-launch path.

## 5. Touched Areas

- files:
  - `Justfile`
  - `apps/local-runner/internal/cli/root.go`
  - `apps/local-runner/internal/cli/provider_account_terminal.go`
  - `apps/local-runner/internal/cli/provider_account_terminal_test.go`
- modules:
  - local runner CLI
  - provider account discovery and selection
- routes:
  - none
- tables:
  - none

## 6. Acceptance Check

- `just runner-provider-terminal` appears in `just --list`.
- `go run ./cmd/flowpilot providers terminal --help` exposes the new command and flags.
- The command lists connected provider accounts and shows usage metadata before selection.
- The command opens the same terminal flow as the dashboard launcher for the selected account.
- `go test ./internal/cli/...` passes.

## 7. Out of Scope

- Starting or supervising the local HTTP server.
- Redesigning the dashboard provider launcher UI.
- Changing provider account discovery rules or usage-metadata computation beyond what the launcher needs.
- Adding new provider account types.

## 8. Completion Notes

- result: implemented and verified locally.
- follow-ups: none required for this slice.
- upstream docs updated: none. This task is a downstream implementation note only; it does not change upstream business intent.

# CA-116: Fix Project Codex Agent Catalog Precedence

## Scope

- Native Codex TOML agent discovery
- Project-over-account duplicate resolution
- Active provider-account filtering

## Completed

- Added `.toml` support to the agent catalog alongside existing Markdown support.
- Parsed Codex model, reasoning effort, tools, and `developer_instructions`.
- Limited provider-home discovery to active connected accounts after runner attachment.
- Preserved precedence: project > active provider account > built-in.
- Added regression coverage for a project and account TOML agent with the same name.

## GitNexus Impact

- `listAgents`: MEDIUM risk, 6 direct callers including picker and child-run creation.
- `discoverProjectAgents`: LOW risk, 1 direct caller.
- `agentsFromDir`: LOW risk in the current index.
- `AttachRunner`: LOW risk, 1 direct caller and 2 indexed CLI flows.
- No HIGH or CRITICAL impact was reported.

## Verification

- Focused agent-catalog and spawn tests — pass.
- Full `go test ./internal/runner -count=1` — pass.
- Live project agent endpoint verification — pass.
- `git diff --check` — pass.

## Residual Notes

- Malformed Codex TOML agent files are skipped.
- Model availability is still validated by Codex when the child turn starts.
- Existing unrelated worktree changes were preserved.

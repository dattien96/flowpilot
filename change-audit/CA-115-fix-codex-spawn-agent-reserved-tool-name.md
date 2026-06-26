# CA-115: Fix Codex Spawn-Agent Reserved Tool Name

## Scope

- Codex dynamic tool registration
- Codex inbound tool-call routing
- Codex tool-event normalization
- Legacy Codex rollout migration before thread resume

## Completed

- Updated `apps/local-runner/internal/runner/codex_adapter.go`.
  - Added a Codex-only tool alias constant: `flowpilot_spawn_agent`.
  - Registered the alias instead of raw `spawn_agent` in Codex `dynamicTools`.
  - Kept the inbound tool-call handler backward-compatible with both names.
- Updated `apps/local-runner/internal/runner/codex_event_mapper.go`.
  - Normalized the Codex alias back to `spawn_agent` in provider events so UI/history semantics stay unchanged.
- Updated `apps/local-runner/internal/runner/codex_appserver_process.go`.
  - Bound the active account's `CODEX_HOME` to the shared Codex adapter.
- Updated `apps/local-runner/internal/runner/session_file_locator.go`.
  - Added an idempotent migration for legacy rollout `session_meta.dynamic_tools`.
  - Rewrites through a sibling temporary file and rename.
  - Preserves file permissions and all records after `session_meta`.
- Updated tests:
  - `apps/local-runner/internal/runner/codex_appserver_test.go`
  - `apps/local-runner/internal/runner/codex_event_mapper_test.go`
  - Added alias round-trip and legacy-resume migration coverage.
  - Asserted that Codex payloads and migrated rollout metadata no longer advertise raw `spawn_agent`.

## GitNexus Impact

- `codexThreadStartParams`: LOW risk, helper-only impact in current index.
- `codexThreadResumeParams`: LOW risk, helper-only impact in current index.
- `codexSpawnAgentDynamicTool`: LOW risk, 1 direct caller (`SendTurn`).
- `newCodexAdapter`: LOW risk, 1 direct caller (`ensureCodexAppServer`) in the current index.
- `ensureCodexAppServer`: LOW risk, no indexed upstream callers.
- `LocateSessionFile`: LOW risk, no indexed upstream callers.
- GitNexus MCP tools were unavailable in this session; the local GitNexus CLI index was used instead.

## Verification

- `go test ./internal/runner -run 'TestCodexAdapter(SpawnAgentDynamicToolAliasRoundTrip|ResumeMigratesLegacySpawnAgentTool|ResumedTurnRoutesAskUserDynamicTool)|TestMapCodexNotification' -count=1` — pass.
- `go test ./internal/runner -count=1` — pass.
- `FLOWPILOT_CODEX_E2E=1 go test ./internal/runner -run TestCodexAskUserEndToEnd -count=1 -v` — pass against the real Codex app-server.
- Live legacy-session replay: `run-190` migrated its stored tool name to `flowpilot_spawn_agent` and completed the resumed turn without the reserved-tool `400`.
- `git diff --check` — pass.

## Residual Notes

- This fix is intentionally Codex-only. Claude continues to use MCP `spawn_agent`.
- Existing Codex rollout files are migrated lazily on their first resume after upgrade.
- Existing user edits in `AGENTS.md` and `CLAUDE.md` were preserved.

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: CA-115
change_type: fix
summary: Fix Codex Spawn-Agent Reserved Tool Name
# --->8---

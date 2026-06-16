# CA-083 — Fix Claude FlowPilot MCP Availability: ask_user in YOLO=on + Google Drive at Runtime (BUG-073)

## Scope

Runner — Claude provider adapter + per-turn MCP config:

- `apps/local-runner/internal/runner/claude_adapter.go`
- `apps/local-runner/internal/runner/claude_permission_mcp.go`
- `apps/local-runner/internal/runner/claude_mcp_server.go`
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
- `apps/local-runner/internal/runner/provider_registry.go`
- tests: `claude_adapter_test.go`, `claude_mcp_server_test.go`, `claude_mcp_live_test.go`

## Completed

Two coupled defects on the Claude provider path, both rooted in how the per-turn
`--mcp-config` was assembled alongside the always-present `--strict-mcp-config` flag.

### Defect 1 — YOLO=on had zero FlowPilot MCP tools (ask_user broken)

`SendTurn` registered the turn bridge and wrote the per-turn `--mcp-config` **only** when
`!posture.RunnerAutoApprove` (YOLO=off), and `claudeArgs` added `--mcp-config` under the same
condition. With `--strict-mcp-config` always present, a YOLO=on turn launched Claude with no
`--mcp-config` at all → **no** FlowPilot MCP tools, so `mcp__flowpilot__ask_user` did not exist
and Claude could not surface a question card in YOLO=on.

**Fix:**
- `SendTurn` now registers the bridge + writes the per-turn `--mcp-config` for **all** YOLO
  states. The fail-closed guards (missing base URL / config write error) remain conditional on
  `!RunnerAutoApprove`: YOLO=off still fails closed (never runs ungated); YOLO=on degrades
  gracefully to the in-stream `control_request` fallback.
- `claudeArgs` splits the two flags: `--mcp-config` is added whenever the config path is
  non-empty (all YOLO states); `--permission-prompt-tool` is added only when YOLO=off.

### Defect 2 — Google Drive MCP ignored at runtime despite being configured

`EnsureGoogleDriveMcpProviderConfig` writes the google-drive server into the account's
`~/.claudeHome<N>/.claude.json` (parity with Codex's `config.toml`). But `--strict-mcp-config`
makes the Claude CLI load MCP servers **only** from the command-line `--mcp-config` and ignore
`.claude.json`. The per-turn `--mcp-config` carried only the `flowpilot` permission server, so
Claude never saw google-drive (it reported only `WebFetch`/`ToolSearch`). Codex has no
equivalent strict flag, so it reads `config.toml` natively — hence the asymmetry.

**Fix:**
- New `writeClaudeMCPConfig(baseURL, token, extra)` merges FlowPilot-managed servers next to
  `flowpilot` (and refuses to let an extra entry shadow the permission route).
- New `(*Runner).flowpilotClaudeExtraMCPServers(accountHome, yolo)` reads the google-drive entry
  the settings page already wrote to `.claude.json` and **recomputes** it with the turn's YOLO +
  fresh proxy OAuth/runtime via the existing `expectedClaudeGoogleDriveMcpServer` SSOT. Gated:
  only injects when the user configured google-drive for Claude AND proxy auth is configured.
- Adapter gains `extraMCPServers func(yolo bool) map[string]claudeMcpServer`, wired in
  `provider_registry.go` from the resolved `account.HomePath` (empty/no-op for the API-key path).

### Hardening

- Single-sourced the server name: `claudeMCPServerName = "flowpilot"` with
  `claudeApproveToolName`/`claudeAskUserToolName` derived from it via const concatenation. All
  four prior literals (`--mcp-config` key, shadow guard, stdio config, `serverInfo`) now
  reference the const so a rename can't silently desync the tool names or the guard.

## Verification

- `go build ./...` clean in `apps/local-runner`.
- Targeted: `go test ./internal/runner -run "TestWriteClaudeMCPConfig|TestClaudeArgs|TestClaudeAdapter|TestHandleClaude|TestEnsureClaudeConfig"` — all pass, including the new
  `TestWriteClaudeMCPConfigMergesExtraServers` and the YOLO=true + non-empty mcpConfig case in
  `TestClaudeArgsYoloPosture`.
- Full package: `go test ./internal/runner` shows the **identical 25 pre-existing failures** on
  both the clean tree and the changed tree (verified by stash + diff, ignoring timing). All 25
  are environmental (Google Drive OAuth/credentials/launcher not present in this test env, `sh`
  not on PATH on Windows, skill-source env). No new regressions.
- NOT verified this session: a live `claude` binary run against real Google Drive auth. The
  config-file mechanism is unit-proven; an end-to-end live smoke check remains open.

## Residual Notes

- **Process model is spawn-per-turn** (07 plan "Final Decision"): each prompt spawns a fresh
  `claude` process and reloads all MCP servers; continuity is `--resume <session-id>`. So the
  google-drive **stdio** server is (re)started every prompt (20s `startup_timeout_sec` budget),
  while the `flowpilot` permission server is runner-hosted HTTP and persists. This per-turn stdio
  cost is inherent to the existing model, not introduced here.
- **Future improvement (not done):** host google-drive as a **runner-hosted HTTP MCP** (like
  `flowpilot`) so it runs once and persists across turns, removing per-turn stdio startup
  latency while keeping the per-turn token model, per-turn YOLO, and spawn-per-turn process model
  intact. This is the architecturally sound version of "start the MCP once"; a full warm-process
  per session is rejected because it breaks per-turn token routing, per-turn YOLO posture, and
  Drive credential freshness.
- GitNexus interactive tools (`gitnexus_impact`/`context`/`query`) were not available in this
  thread; edits relied on direct call-graph inspection (`SendTurn` → `claudeArgs` →
  `writeClaudeMCPConfig` → `provider_registry` factory → `claudeProcessPool`).

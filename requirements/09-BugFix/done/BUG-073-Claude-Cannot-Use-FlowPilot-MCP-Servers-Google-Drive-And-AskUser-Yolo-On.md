# BUG-073: Claude Cannot Use FlowPilot MCP Servers (Google Drive at Runtime; ask_user in YOLO=on)

## Metadata

- Document ID: `BUG-073`
- Title: `Claude Cannot Use FlowPilot MCP Servers (Google Drive at Runtime; ask_user in YOLO=on)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [CP-29: MCP Proxy Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md), [07: Claude Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)
- Child Documents: `none`
- Related Documents: [BUG-069: Claude YOLO=off Always Auto-Approves](./BUG-069-Claude-YOLO-Off-Always-Auto-Approves.md), [BUG-032: Codex Google Drive MCP Model And Approval](./BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [Task-043: Desktop Google Drive Setup Tab](../../08-Task/done/Task-043-Desktop-Google-Drive-Setup-Tab.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [CA-083: Fix Claude FlowPilot MCP Availability](../../../change-audit/CA-083-fix-claude-flowpilot-mcp-availability-yolo-and-google-drive.md)
- Replaces: `none`
- Tags: `claude, mcp, google-drive, ask_user, yolo, strict-mcp-config, runner, regression, high`

## AI Quick View

### Summary

- In desktop chat, when the user mentions the Google Drive MCP, Claude reports it has no such tools (only `WebFetch`/`ToolSearch`) — even though Google Drive setup step 7 ("Configure AI Providers") configured it for Claude.
- FlowPilot DOES write the google-drive server to the Claude account's `.claude.json` (parity with Codex's `config.toml`), but `claudeArgs` always passes `--strict-mcp-config`, which makes the Claude CLI ignore `.claude.json` and load MCP servers ONLY from the per-turn `--mcp-config` — which carried only the `flowpilot` permission server.
- Coupled defect: the per-turn `--mcp-config` (and the turn bridge registration) was gated on YOLO=off, so YOLO=on Claude turns had NO FlowPilot MCP tools at all — `mcp__flowpilot__ask_user` did not exist and Claude could not surface a question card in YOLO=on.
- Codex is unaffected because it has no `--strict-mcp-config` equivalent and reads `config.toml` natively.

### Current Ask

- Done. The per-turn `--mcp-config` is now built for all YOLO states (so `ask_user` works in YOLO=on), and FlowPilot-managed servers (google-drive) are merged into it (so `--strict-mcp-config` no longer hides them).

### Key Decisions

- `V-1` Build the per-turn `--mcp-config` + register the bridge for ALL YOLO states; keep the fail-closed guards conditional on YOLO=off only.
- `V-2` In `claudeArgs`, decouple `--mcp-config` (added whenever a config path exists) from `--permission-prompt-tool` (added only for YOLO=off).
- `V-3` Merge FlowPilot-managed servers into the per-turn `--mcp-config` via `flowpilotClaudeExtraMCPServers`, recomputed per turn with the turn's YOLO + fresh proxy OAuth; gate on the user having configured google-drive for Claude AND proxy auth being configured.
- `V-4` Keep `--strict-mcp-config` (ambient `~/.claude` isolation) — solve availability by explicit merge, not by relaxing isolation.
- `V-5` Single-source the MCP server name as `claudeMCPServerName` and derive the tool names from it; no re-typed `"flowpilot"` literals.

### Constraints

- No change to the desktop approval/question card vocabulary, the runner policy engine, or the YOLO resolver (SSOT).
- Codex and Gemini provider paths are unaffected.
- `--strict-mcp-config` stays: the user's ambient `~/.claude` MCP servers must remain excluded from FlowPilot runs.

### Open Questions

- Live end-to-end confirmation with a real `claude` binary against real Google Drive auth is not yet run (unit-proven config mechanism only).
- Per-turn google-drive stdio startup latency is inherent to the spawn-per-turn process model; the HTTP-hosted google-drive optimization (see §10) is deferred.

### Source Refs

- User report `2026-06-16`: Claude in desktop chat said Google Drive MCP tools were not available ("only WebFetch loaded"); Codex got the correct MCP config in `config.toml` (step 7) but Claude did not behave the same.
- Spike finding in `claude_permission_mcp.go` / [07 Claude Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md): permission/ask_user are external MCP tools passed via `--mcp-config`; `--strict-mcp-config` loads ONLY that config (tool names `mcp__<srv>__<tool>`).
- `EnsureGoogleDriveMcpProviderConfig` → `ensureClaudeGoogleDriveMcpConfig` writes google-drive into `~/.claudeHome<N>/.claude.json`.

## 1. Issue Summary

When the user references the Google Drive MCP in desktop chat with the Claude provider, Claude responds that it has no Google Drive tools available — only `WebFetch` and `ToolSearch`. This happens even though Google Drive setup step 7 ("Configure AI Providers") reported Claude as configured and wrote the google-drive MCP server into the Claude account's `.claude.json`, exactly as it does for Codex's `config.toml`. Separately, when YOLO is on, Claude has no FlowPilot MCP tools at all, so the `ask_user` question card never appears.

## 2. Parent Links

- impacted coding plan: [CP-29: MCP Proxy Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md); design detail in [07: Claude Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)
- impacted tech design: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode + `apps/local-runner` with the Claude provider; Google Drive proxy MCP configured for the Claude account via setup step 7.
- reproduction steps:
  1. Configure Google Drive proxy MCP and run step 7 "Configure AI Providers" (Claude account gets google-drive in its `.claude.json`).
  2. Open desktop chat with Claude.
  3. Ask Claude to list or read a file from Google Drive (or mention the google-drive MCP).
  4. Observe: Claude reports the google-drive tools are not available; it only sees `WebFetch`/`ToolSearch`.
  5. Separately, with YOLO on, prompt something that should trigger a clarifying question — observe no `ask_user` question card appears.
- frequency: deterministic — every Claude turn (google-drive always hidden by `--strict-mcp-config`; `ask_user` always absent in YOLO=on).

## 4. Expected vs Actual

- expected: Claude has the FlowPilot-configured google-drive MCP tools (`mcp__google-drive__search`, `listFolder`, `readGoogleDoc`, …) available — like Codex — and has `mcp__flowpilot__ask_user` available in both YOLO states.
- actual: under `--strict-mcp-config` the per-turn `--mcp-config` carried only `flowpilot`, so google-drive (written to `.claude.json`) was ignored; and in YOLO=on no `--mcp-config` was written at all, so `ask_user` was absent.

## 5. Impact

- users affected: all desktop chat (and provider-run) users using Claude with a FlowPilot-configured MCP (Google Drive), and all Claude users relying on `ask_user` in YOLO=on.
- workflows affected: any Claude turn that needs Google Drive access; any YOLO=on Claude turn that needs a structured user question.
- severity: high — a configured integration is silently unavailable on one provider, and an interaction primitive (`ask_user`) is missing in a whole YOLO mode.

## 6. Root Cause

- hypothesis: Claude ignores the google-drive server FlowPilot wrote to `.claude.json`, and YOLO=on skips MCP config setup entirely.
- confirmed cause:
  - `claudeArgs` always emits `--strict-mcp-config`, which directs the Claude CLI to load MCP servers ONLY from the command-line `--mcp-config` and ignore the account's `.claude.json` `mcpServers`. The per-turn `--mcp-config` (`writeClaudeMCPConfig`) contained only the `flowpilot` permission server, so the google-drive server in `.claude.json` was never loaded.
  - `SendTurn` registered the bridge and wrote the per-turn `--mcp-config` only when `!posture.RunnerAutoApprove`, and `claudeArgs` added `--mcp-config` under the same condition. Combined with the always-present `--strict-mcp-config`, a YOLO=on turn launched with zero FlowPilot MCP tools, so `mcp__flowpilot__ask_user` did not exist.
- evidence:
  - 07 Claude Adapter Plan: "external MCP via `--mcp-config` (stdio/http/sse), `--strict-mcp-config`; tool names `mcp__<srv>__<tool>`"; `ask_user` is "served as a real external MCP server … passed via `--mcp-config`".
  - `ensureClaudeGoogleDriveMcpConfig` writes the google-drive server to `~/.claudeHome<N>/.claude.json` (the `getProviderConfigPath("claude", …)` location) — confirming the config is present but unused at runtime.
  - New `TestWriteClaudeMCPConfigMergesExtraServers` fails before the merge change (google-drive absent from the per-turn config) and passes after.
  - `TestClaudeArgsYoloPosture` (extended) asserts YOLO=true + non-empty mcpConfig now yields `--mcp-config` WITHOUT `--permission-prompt-tool`.

## 7. Fix Strategy

- `F-1` `SendTurn`: register the turn bridge + write the per-turn `--mcp-config` for ALL YOLO states; keep the fail-closed guards (missing base URL / write error) conditional on `!RunnerAutoApprove` only (YOLO=off never runs ungated; YOLO=on degrades to the in-stream fallback).
- `F-2` `claudeArgs`: add `--mcp-config` whenever the config path is non-empty; add `--permission-prompt-tool` only when YOLO=off (decoupled from `--mcp-config`).
- `F-3` `writeClaudeMCPConfig(baseURL, token, extra)`: merge FlowPilot-managed servers next to `flowpilot`; never let an extra entry shadow the `flowpilot` permission route.
- `F-4` New `(*Runner).flowpilotClaudeExtraMCPServers(accountHome, yolo)`: read the google-drive entry from the account's `.claude.json`, recompute it with the turn's YOLO + fresh proxy OAuth via `expectedClaudeGoogleDriveMcpServer`; return empty unless the user configured google-drive for Claude AND proxy auth is configured.
- `F-5` Wire `extraMCPServers` on the adapter (`claude_adapter.go`) and in the registry factory (`provider_registry.go`) from the resolved `account.HomePath` (empty/no-op for the API-key path); `SendTurn` passes the resolved extras into `writeClaudeMCPConfig`.
- `F-6` Single-source `claudeMCPServerName = "flowpilot"` and derive `claudeApproveToolName`/`claudeAskUserToolName` from it; replace all re-typed `"flowpilot"` literals (config key, shadow guard, stdio config, `serverInfo`) with the const.

## 8. Validation

- `V-1` `go build ./...` clean in `apps/local-runner`.
- `V-2` `go test ./internal/runner -run "TestWriteClaudeMCPConfig|TestClaudeArgs|TestClaudeAdapter|TestHandleClaude|TestEnsureClaudeConfig" -count=1` — all pass (incl. new `TestWriteClaudeMCPConfigMergesExtraServers` and the YOLO=true mcpConfig assertion in `TestClaudeArgsYoloPosture`).
- `V-3` Full 2×2 matrix proven with the per-turn MCP server WIRED (production path): `TestClaudeSendTurnMcpAvailabilityMatrix` (YOLO on/off → `--mcp-config` + `--strict-mcp-config` present AND config merges both `flowpilot` and `google-drive`; `--permission-prompt-tool` present iff YOLO=off; `--permission-mode` tracks YOLO) and `TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade` (YOLO=off + missing base URL → fail closed, no spawn; YOLO=on → degrades, spawns without `--mcp-config`).
- `V-4` Full-package regression: `go test ./internal/runner` shows the IDENTICAL 25 pre-existing failures on both the clean tree and the changed tree, INCLUDING after the new matrix tests were added (verified via git stash + name-level diff). All 25 are environmental (Google Drive OAuth/credentials/launcher absent in test env, `sh` not on PATH on Windows, skill-source env). No new failures introduced.
- `V-5` NOT run this session: a live `claude` binary against real Google Drive auth. The config-file mechanism is unit-proven; the live end-to-end smoke check is left open (see §10 / Open Questions).

## 9. Regression Guard

- tests (full {YOLO on/off} × {command-tool gating, MCP availability} matrix):
  - `TestClaudeSendTurnMcpAvailabilityMatrix` (new — SendTurn with MCP server wired: both YOLO states get `--mcp-config` with `flowpilot`+`google-drive` merged; `--permission-prompt-tool` iff YOLO=off).
  - `TestClaudeSendTurnBaseURLMissingFailClosedVsDegrade` (new — YOLO=off fails closed / no spawn; YOLO=on degrades without `--mcp-config`).
  - `TestWriteClaudeMCPConfigMergesExtraServers` (new — helper-level merge + no-shadow guard).
  - `TestClaudeArgsYoloPosture` (extended — YOLO=true keeps `--mcp-config` and drops `--permission-prompt-tool`; empty mcpConfig emits neither flag).
  - `TestClaudeArgsIncludesStrictMcpConfig` (unchanged — strict flag always present).
  - `TestClaudeAdapterYoloTrueNoPermission` (unchanged — YOLO=on surfaces no permission_required).
  - `TestClaudeAdapterAskUserRoundTrip` / `TestClaudeAdapterApprovalRoundTrip` (unchanged — bridge round-trips via in-stream fallback).
  - `TestEnsureClaudeConfigSettingsClearsAllowRules` (BUG-069 guard, unchanged).
- alerts: none.
- audit checks: [CA-083](../../../change-audit/CA-083-fix-claude-flowpilot-mcp-availability-yolo-and-google-drive.md) records the change; clean-vs-changed failure diff captured to confirm no new regressions.

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - [07: Claude Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md) — the plan describes the per-turn `--mcp-config` as carrying only `approve` + `ask_user` and being built for the gated (YOLO=off) path. That is now broadened: the per-turn `--mcp-config` is built for ALL YOLO states, and it merges FlowPilot-managed external servers (google-drive). Flag this delta in the plan; `--permission-prompt-tool` (not `--mcp-config`) is the YOLO-conditional part.
- notes left unchanged on purpose:
  - `--strict-mcp-config` remains the isolation mechanism; availability is solved by explicit merge, not by relaxing isolation.
  - SS-08 / SD-09 (approval + YOLO product behavior) are unchanged — this is a runtime MCP-availability fix, not a change to the approval gate semantics.
  - The in-stream `control_request` fallback (used when `mcpServer` is nil, offline/tests, and now as the YOLO=on graceful-degrade path when base URL is unavailable) is unchanged.
- deferred improvement (NOT done; documented for later): the process model is spawn-per-turn (07 "Final Decision"), so the google-drive stdio server restarts every prompt (20s startup budget) while the HTTP-hosted `flowpilot` server persists. Hosting google-drive as a runner-hosted HTTP MCP (like `flowpilot`) would run it once and remove per-turn stdio startup latency while preserving the per-turn token model, per-turn YOLO posture, and spawn-per-turn process model. A full warm-process-per-session is rejected because it breaks per-turn token routing, per-turn YOLO posture, and Drive credential freshness.

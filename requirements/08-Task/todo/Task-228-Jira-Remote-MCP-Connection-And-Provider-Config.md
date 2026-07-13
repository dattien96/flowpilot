# Task-228: Jira Remote-MCP Connection And Provider Config

## Metadata

- Document ID: `Task-228`
- Title: `Jira Remote-MCP Connection And Provider Config`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-05-06: Jira MCP As A Context Artifact Source](../../07-Coding-Plan/todo/CP-05-06-Jira-MCP.md) (`P-1`, `P-2`), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md) (§3.2)
- Child Documents: `None`
- Related Documents: [CP-05-01: Jira MCP API Token Flow](../../07-Coding-Plan/done/CP-05-01-Jira-Mcp-Api-Token.md) (legacy token path — fallback), [CP-05-03: Google Drive MCP Current Implementation Notes](../../07-Coding-Plan/done/CP-05-03-Driver-Mcp.md) (mẫu provider-config injection), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md)
- Replaces: `None`
- Tags: `mcp`, `jira`, `atlassian`, `oauth`, `remote-mcp`, `provider-config`, `connection`

## AI Quick View

### Summary

- Nối Jira qua **Atlassian official remote MCP** (`https://mcp.atlassian.com/v1/mcp/authv2`, OAuth trình duyệt) đúng SD-11 §3.2. `mcpBackendSpec` jira đã khai transport `remote` + endpoint ([runner.go:2302](../../../apps/local-runner/internal/runner/runner.go:2302)) nhưng **chưa có code thực sự nói remote-MCP setup**.
- Viết **provider-config injection** (mirror `google_drive_mcp_provider_config.go`): thêm entry remote MCP server `jira` vào config của Claude/Codex/Gemini/Grok (+ Claude `--strict-mcp-config` extra server), để provider CLI làm MCP client.
- Onboarding UI trong `McpSettings.tsx` (mode `jira`): Atlassian guide link + OAuth trigger; status `pending → awaiting_oauth → connected`; token chỉ ở keyring.

### Current Ask

- Cho user connect Jira qua Atlassian remote MCP + OAuth, và inject config để ≥1 provider CLI thấy MCP server `jira`.

### Key Decisions

- `T-1` Transport = remote MCP + OAuth (SD-11 §3.2); **không** dùng Go MCP client cho execution. Legacy REST/token (CP-05-01) chỉ là fallback/diagnostic.
- `T-2` OAuth token/refresh chỉ ở runner keyring (`jira:<integrationId>`, mirror `jiraCredential` [runner.go:2377](../../../apps/local-runner/internal/runner/runner.go:2377)); **không** vào Supabase `config_encrypted`.
- `T-3` Provider-config injection per-account-home (không commit vào repo), theo CP-05-03 §11.4; stale detection như Drive.
- `T-4` Duplicate-guard theo normalized Atlassian site URL (CP-05-01 §3.2).

### Constraints

- Secret boundary SD-11 §6 / CP-05-01 §5.
- Chỉ đổi status `connected` sau verify thật (không false-connected).
- Node.js cần cho local proxy path của Atlassian (SD-11 §3.2).
- Read-only phase (không cấu hình write tools).

### Open Questions

- `Q-1` Có cần Go client nói remote-MCP transport để verify/list không, hay verify dùng lại REST path CP-05-01? Đề xuất reuse REST cho verify + pick.
- `Q-2` Giữ song song token path (headless) hay retire CP-05-01? Chờ owner (CP-05-06 `Q-4`).

### Source Refs

- `CP-05-06` `P-1`, `P-2`, `R-1`, `R-5`.
- `SD-11` §2, §3.2, §6.
- `CP-05-03` §11.3 (provider config examples), §11.4 (account home), §11.7 (config install).
- current code: `runner.go` (`mcpBackendSpecs`, `jiraCredential`, `detectJiraBackend`, `VerifyMcpBackend`), `google_drive_mcp_provider_config.go` (mẫu), `McpSettings.tsx`.

## 1. Goal

Connect Jira qua Atlassian remote MCP + OAuth và cấu hình provider CLI để nó dùng được MCP server `jira`, với token an toàn trong keyring và status phản ánh verify thật.

## 2. Parent Links

- coding plan: `CP-05-06` (`P-1`, `P-2`)
- tech design: `SD-11` (§3.2), `SD-23` (`D-5`)
- system spec: `SS-14`
- specific upstream ids: `CP-05-06 P-1/P-2`, `SD-11 §3.2`

## 3. Trigger

Jira context source (Task-229) không dùng được nếu chưa có connection + provider-config để CLI thấy MCP server `jira`. Đây là lớp connection nền cho CP-05-06.

## 4. Exact Change

- `T-1` Bổ sung code remote-MCP setup cho Jira backend (hiện chỉ có spec khai báo).
- `T-2` OAuth flow: trigger từ `McpSettings.tsx` mode `jira`; store token ở keyring; status transitions.
- `T-3` `jira_mcp_provider_config.go` (+ test): `Ensure*JiraMcpConfig`/`check*`/`detectStale*` cho ≥1 provider (Claude hoặc Codex) + Claude `--strict-mcp-config` extra server.
- `T-4` Verify path đổi status `connected` (reuse REST verify `detectJiraBackend`/`VerifyMcpBackend` theo `Q-1`).
- `T-5` Duplicate site URL guard.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/runner.go` (jira backend/verify), mới `jira_mcp_provider_config.go` (+ test), `apps/local-runner/internal/cli/root.go` (endpoint nếu cần), `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx`, `settingsHelpers.ts` (jira fields cho OAuth)
- modules: runner MCP backend + provider-config; desktop MCP settings
- routes: MCP connect/OAuth endpoints
- tables: `integrations` (type `jira`, đã hợp lệ), `project_mcp_links`

## 6. Acceptance Check

- Connect Jira qua OAuth → status `connected`; token chỉ ở keyring (không trong Supabase/log — kiểm bằng test).
- Provider config cho ≥1 CLI chứa MCP server `jira` trỏ remote endpoint; stale detection có test.
- Duplicate site URL bị chặn.
- `go test ./apps/local-runner/internal/runner/...` xanh (provider-config tests).

## 7. Out of Scope

- Không register `jira.issue` context source (Task-229).
- Không runtime target picker / prompt-note (Task-229).
- Không write tools.

## 8. Completion Notes

- result: **partially done — code + tests land, real OAuth handshake does not (documented limitation, not a gap I could close in this environment)**.
  - Shipped: `apps/local-runner/internal/runner/jira_mcp_provider_config.go` — `EnsureClaudeJiraMcpConfig` (writes/updates `mcpServers.jira` in `.claude.json` as a **remote** `{"type":"http","url":"https://mcp.atlassian.com/v1/mcp/authv2","headers":{"Authorization": authHeaderValue}}` entry, preserving every other config key via a generic-map splice — mirrors the Grok "preserve other keys" technique already in `google_drive_mcp_provider_config.go`, applied to JSON instead of TOML); `checkClaudeJiraMcpConfig` (read-only staleness check); `PreflightJiraMcp` (reuses existing `detectJiraBackend`/keyring credential — CP-05-01's already-shipped connection layer — to decide "is Jira connected", then checks provider config staleness); `buildJiraMcpInstructions` (prompt block, mirrors Drive's shape, read-only v1, failure codes `MCP_UNAVAILABLE`/`MCP_AUTH_REQUIRED`/`JIRA_CONTENT_NOT_FOUND`). Registered `"jira"` into Task-227's `mcpInstructionSpecs` — `InjectRequiredMcpInstructions(..., []string{"jira"}, ...)` now dispatches to Jira's own block, proving the Task-227 seam generalizes to a real second MCP.
  - Shipped: `apps/desktop-flowpilot` duplicate-site-URL guard — `normalizeAtlassianSiteUrl`/`findDuplicateJiraIntegration` in `settingsHelpers.ts`, wired into `McpSettings.tsx`'s `createIntegration` (CP-05-01 §3.2 duplicate rule, previously undone — no such guard existed before this task).
  - Tests: `jira_mcp_provider_config_test.go` (10 new cases — write/idempotent/preserve-other-keys/stale-detection/preflight ready-not-ready/provider-configured-not-configured) + `TestInjectRequiredMcpInstructionsJiraProducesJiraBlockNotDrive`; `settingsHelpers.test.ts` (8 new cases for the duplicate-guard). `go build ./...` clean; `go test ./internal/runner/...` — 1403 passed / 16 failed vs. a 1396 passed / 21 failed baseline on the same unmodified code (confirmed via `git stash`) — net **fewer** failures, zero regressions, all 16 remaining failures are pre-existing/unrelated (codex-resume timing, skills-merge, session-path — same profile CP-45 already documented). `tsc --noEmit` in `apps/desktop-flowpilot` clean.
  - **NOT done (explicit, by design, not an oversight):** the actual Atlassian OAuth 2.1 browser handshake that produces the `authHeaderValue` bearer token `EnsureClaudeJiraMcpConfig` expects. That requires a registered Atlassian OAuth app + a live browser redirect flow — infrastructure this sandboxed session cannot stand up or verify. Until that lands, `authHeaderValue` has no real producer; the existing CP-05-01 API-token flow (email+token, Basic auth against Jira REST) remains fully working for Test Console/verify/list, completely unaffected by this task. `PreflightJiraMcp` correctly reports "not ready" whenever the OAuth wiring is absent — it never fabricates a connected state (SD-11's explicit non-goal).
  - `Codex`/`Gemini`/`Grok` provider-config writers for Jira were **not** built (Task-228 scope said "≥1 provider (Claude hoặc Codex)"; Claude covers that bar). Extending to the others is mechanical repetition of the same pattern once needed.
- follow-ups: (1) implement the Atlassian OAuth 2.1 handshake (separate follow-up task — needs a registered OAuth app, redirect URI, and a UI "Start Auth" trigger mirroring Google Drive's `StartGoogleDriveMcpAuth`); (2) extend provider-config writers to Codex/Gemini/Grok once needed; (3) Task-229 should call `mcpBoundedFetch` (Task-226) for its own Jira context-source fetches.
- upstream docs updated: none required — CP-05-06 `P-1`/`P-2` already anticipated the OAuth gap as the primary risk (`R-1`); this completion note is the record of what's real vs. pending.

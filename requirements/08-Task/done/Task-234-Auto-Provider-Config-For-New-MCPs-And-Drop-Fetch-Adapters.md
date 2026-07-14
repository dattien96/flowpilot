# Task-234: Auto Provider-Config For New MCPs And Drop Redundant Fetch Adapters

## Metadata

- Document ID: `Task-234`
- Title: `Auto Provider-Config For New MCPs (Jira/Firebase/Telegram) Across All 4 Providers And Drop Redundant Fetch Adapters`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-05-06: Jira MCP](../../07-Coding-Plan/done/CP-05-06-Jira-MCP.md) (`P-1`, `P-2`), [CP-05-04: Firebase MCP](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md) (`P-1`, `P-2`), [CP-05-05: Telegram MCP](../../07-Coding-Plan/done/CP-05-05-Tele-Mcp.md) (`P-1`, `P-2`), [CP-05-03: Driver MCP](../../07-Coding-Plan/done/CP-05-03-Driver-Mcp.md) (`§11` provider-CLI-is-client, Step-7 provider-config loop)
- Child Documents: `None`
- Related Documents: [Task-228: Jira Remote-MCP Connection And Provider Config](./Task-228-Jira-Remote-MCP-Connection-And-Provider-Config.md), [Task-229: Jira Issue Context Source And Runtime Target Picker](./Task-229-Jira-Issue-Context-Source-And-Runtime-Target-Picker.md), [Task-230: Firebase Crashlytics MCP Connection And Provider Config](./Task-230-Firebase-Crashlytics-MCP-Connection-And-Provider-Config.md), [Task-231: Firebase Crashlytics Context Source And Runtime Target](./Task-231-Firebase-Crashlytics-Context-Source-And-Runtime-Target.md), [Task-232: Telegram Artifact Type And Bot-API Proxy MCP](./Task-232-Telegram-Artifact-Type-And-Bot-API-Proxy-MCP.md), [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](./Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md)
- Replaces: `None`
- Tags: `mcp`, `jira`, `firebase`, `telegram`, `provider-config`, `multi-provider`, `ui`, `cleanup`

## AI Quick View

### Summary

- Task-228/230/232 shipped `EnsureClaude{Jira,Firebase,Telegram}McpConfig` writing `.claude.json`, but Claude launches with `--strict-mcp-config` and **ignores** that file at turn time — only `flowpilotClaudeExtraMCPServers` (merged into per-turn `--mcp-config`) is honored, and it currently injects **only** `google-drive`. So the three new MCP servers **never reach a live Claude turn**. Fix: extend that merge for jira/firebase/telegram, gated on the integration being connected.
- Provider-config was Claude-only. The other three CLIs (Codex/Gemini/Grok) read their own static config and got nothing. Add their config-ensure for the three new MCPs, mirroring Drive's `ensureCodex/ensureGemini/ensureGrok` shape.
- Replace the manual single-account "Configure AI Provider" panel (added mid-session) with a **"Configure Providers" loop button per MCP page** that iterates every discovered account of all four providers and writes config into each — mirroring Google Drive Step-7 (`configureAiProviders`).
- Drop the redundant deterministic **content-fetch adapters** (`jiraRestIssueAdapter`, `jiraRestSprintAdapter`, `firebaseToolsMcpAdapter`): the live path filters these sources out of collect and injects only a prompt note, so the AI's own configured MCP does the fetch. CP-05-06 `R-1` scoped REST to the pick-list only, never live content — keep the prompt-note path, remove the fetch code from the primary wiring.

### Current Ask

- Make MCP provider-config for Jira/Firebase/Telegram automatic across all four provider CLIs (one loop-button per MCP page, plus the Claude per-turn merge), and delete the redundant REST/MCP-client fetch adapters so the live path stays prompt-note-only like `mcp.driver`.

### Key Decisions

- `T-1` Config injection is not manual-per-account. Claude is served automatically at turn time via `flowpilotClaudeExtraMCPServers`; Codex/Gemini/Grok via a per-account static-config loop.
- `T-2` The Claude per-turn merge gate for these three is **"integration connected" (keyring credential present)**, not "entry present in `.claude.json`" — there is no per-run OAuth handshake like Drive's proxy, so no static enable-flag write is required for Claude.
- `T-3` Live AI-turn context path stays prompt-note-only (sources already filtered from collect in `flow_executor.go`). No deterministic content fetch in the primary flow.
- `T-4` Jira bearer token is still required to write the remote-MCP `Authorization` header; it is captured at connect/config time, never persisted to Supabase (SD-11 §6).

### Constraints

- `flowpilotClaudeExtraMCPServers` and the `extraMCPServers` closure are **shared** by the Claude adapter and the Grok ACP adapter ([provider_registry.go:347](../../../apps/local-runner/internal/runner/provider_registry.go:347), [provider_registry.go:514](../../../apps/local-runner/internal/runner/provider_registry.go:514)) and gate two already-shipped provider paths — changes must be additive and behavior-preserving for Google Drive.
- Do not regress the existing Drive Step-7 flow or its `/google-drive-config/mcp-provider-config/ensure` route.
- Deterministic/no-vector context invariant and `PackageID` stability (BUG-236) unaffected — this task removes fetch code, does not add sources.
- Removing the adapters must not break the "no adapter configured" degrade contract for a source that is (incorrectly) left in collect.

### Open Questions

- `Q-1` Fully delete the adapter files (`firebase_tools_mcp_client.go`, the `jiraRest*Adapter` types) or leave them dormant/unwired like Drive keeps `googleDriveDriverAdapter` for a possible future Test-Console direct-collect path? Default: remove from wiring; keep code only if a concrete non-live consumer is named.
- `Q-2` Codex/Gemini remote-HTTP MCP (Jira) support — confirm each CLI's config schema for an `http` transport server before writing it (Firebase/Telegram are stdio and already proven via Drive's stdio shape).
- `Q-3` Where does the per-provider static-config loop run for Codex/Gemini/Grok — on demand via the new button only, or also eagerly at integration-connect time?

### Source Refs

- `CP-05-06` `P-1` (provider CLI is MCP client; legacy REST = fallback/diagnostic), `P-2` (provider-config injection), `R-1` (REST only for pick-list, not live content).
- `CP-05-04` `P-1/P-2`, `CP-05-05` `P-1/P-2`.
- `CP-05-03` `§11` (provider-CLI-is-client), Step-7 provider-config loop UI.
- current code: `google_drive_mcp_provider_config.go` (`flowpilotClaudeExtraMCPServers`, `ensureCodex/Gemini/Grok...`, `resolveGoogleDriveMcpProviderStatuses`), `provider_registry.go` (extraMCPServers binding), `flow_executor.go:353-356` (collect filter), `interactive_service.go:1637-1645` (adapter wiring), `GoogleDriveSettings.tsx` (`configureAiProviders` Step-7 loop), `McpSettings.tsx` (the manual panel to replace).

## 1. Goal

Give Jira/Firebase/Telegram MCP servers the same automatic, all-provider config reach that Google Drive already has: served to a live Claude turn via the per-turn `--mcp-config` merge, written into Codex/Gemini/Grok static config across all discovered accounts via one loop-button per MCP page — and remove the redundant deterministic fetch adapters so the live context path is prompt-note-only, matching `mcp.driver`.

## 2. Parent Links

- coding plan: `CP-05-06` (`P-1`, `P-2`, `R-1`), `CP-05-04` (`P-1`, `P-2`), `CP-05-05` (`P-1`, `P-2`), `CP-05-03` (`§11`, Step-7 loop)
- tech design: `SD-11` (§3.2 remote MCP, §6 secret boundary), `SD-22`/`SD-23` (context source registry)
- system spec: `SS-14`
- specific upstream ids: `CP-05-06 P-1/P-2/R-1`, `CP-05-03 §11.7`

## 3. Trigger

Review of the mid-session implementation found the new MCP provider-config path was (a) non-functional for live Claude turns (`--strict-mcp-config` ignores the written `.claude.json`; the per-turn merge only knows google-drive), (b) Claude-only, (c) driven by a manual single-account button instead of Drive's all-account loop, and (d) carrying redundant REST/MCP-client fetch adapters the live path never invokes. This task corrects all four to match the established Drive pattern.

## 4. Exact Change

- `T-1` Extend `flowpilotClaudeExtraMCPServers` to also return jira/firebase/telegram server entries when the corresponding integration is connected (keyring credential resolves). Remote Jira = `type:http` + Authorization header; Firebase/Telegram = the stdio entries from their `expectedClaude*McpServer` builders. Behavior-preserving for google-drive.
- `T-2` Add `ensureCodex/ensureGemini/ensureGrok` config writers for the three new MCP servers (mirror the Google Drive equivalents), and route them through each MCP's `Ensure{Jira,Firebase,Telegram}McpProviderConfig` dispatch switch (currently only `case "claude"`).
- `T-3` Add a per-provider status/enumeration path (mirror `resolveGoogleDriveMcpProviderStatuses` → `DiscoverProviderAccountHomes` over codex/gemini/claude/grok) so the UI can list all target accounts for the loop.
- `T-4` UI: replace the manual single-account "Configure AI Provider" panel in `McpSettings.tsx` with a "Configure Providers" button (per jira/firebase/telegram page) that loops all discovered accounts and calls the ensure route for each — mirror `configureAiProviders` in `GoogleDriveSettings.tsx`. Keep the Jira bearer-token field feeding the header write.
- `T-5` Remove the deterministic fetch adapters from the primary wiring: unwire `SetJiraIssueAdapter`/`SetJiraSprintAdapter`/`SetFirebaseCrashlyticsAdapter` in `interactive_service.go` and (per `Q-1`) delete `jiraRestIssueAdapter`/`jiraRestSprintAdapter`/`firebase_tools_mcp_client.go` unless a non-live consumer is named. Keep the prompt-note path (`appendJira*TargetPrompt`, `appendFirebaseCrashTargetPrompt`) and the collect filter intact.
- `T-6` Update DOD/notes in CP-05-04 `DOD-4` and CP-05-05, and Task-229/231/233 completion notes, to reflect that the deterministic-fetch approach was dropped in favor of the prompt-note + auto-provider-config design.

## 5. Touched Areas

- files: `google_drive_mcp_provider_config.go` (extend `flowpilotClaudeExtraMCPServers`), `jira_mcp_provider_config.go` / `firebase_mcp_provider_config.go` / `telegram_mcp_provider_config.go` (provider dispatch switch + codex/gemini/grok writers), `provider_registry.go` (verify shared closure still safe), `interactive_service.go` (unwire adapters), `context_source_jira.go` / `firebase_tools_mcp_client.go` (remove adapter code per `Q-1`), `cli/root.go` (status/enumeration route if needed), `McpSettings.tsx` (replace panel with loop button), `settingsHelpers.ts`
- modules: runner provider-config, per-turn MCP merge, desktop MCP settings
- routes: reuse `/{jira,firebase,telegram}-config/mcp-provider-config/ensure`; possibly add a `.../mcp-provider-config/status` GET for account enumeration
- tables: none

## 6. Acceptance Check

- With a connected Jira/Firebase/Telegram integration and a running Claude turn, the corresponding server appears in the turn's `--mcp-config` (verify via the merge output), without any manual per-account click.
- "Configure Providers" button on each MCP page writes config into every discovered Codex/Gemini/Grok account (loop, not single pick), reporting per-account success/failure like Drive.
- Live flow with `jira.issue`/`firebase.crashlytics` bound INPUT still injects only a bounded prompt note; package contains no fetched content; no adapter is invoked.
- `EnsureXMcpProviderConfig` returns success for all four provider keys (no more "not implemented for provider" on codex/gemini/grok).
- `go build ./...`, `go vet ./internal/runner/...`, `go test ./internal/runner/... ./internal/cli/...` — green vs. known baseline; `tsc --noEmit` clean in desktop-flowpilot + flowpilot-client-core.

## 7. Out of Scope

- Atlassian OAuth browser handshake to auto-mint the Jira bearer token (still manual paste; separate follow-up).
- SonarQube MCP (CP-05-07).
- Any new context source or artifact type.
- Live end-to-end against real Jira/Firebase/Telegram infrastructure (needs real credentials/projects).

## 8. Completion Notes

- result: **done**, with `Q-1`/`Q-2` resolved as explicit scope boundaries (documented below, not silently skipped).
  - `T-1`: `flowpilotClaudeExtraMCPServers` (`google_drive_mcp_provider_config.go`) now also returns jira/firebase/telegram entries — `jiraLiveMCPServer()`, `firebaseLiveMCPServer()`, `telegramLiveMCPServer()` (one helper per provider file, each gated on "integration connected" via the existing keyring resolvers, no accountHomePath dependency). `claudeMcpServer` gained additive `URL`/`Headers` fields (both `omitempty`) so the same struct now carries Jira's remote-HTTP shape alongside the pre-existing stdio fields — behavior-preserving for Google Drive (new fields are zero-value/omitted on its entries). Verified via `TestFlowpilotClaudeExtraMCPServersIncludesJiraWhenBearerTokenPersisted`, `...OmitsJiraWhenNoBearerToken`, `...IncludesFirebaseAndTelegramWhenConnected`, `...OmitsFirebaseTelegramWhenNotConnected`.
  - Grok gets firebase/telegram automatically: `grokACPExtraMCPServers` forwards stdio entries (`Command` set). **G2 follow-up (2026-07-13, CA-300)** also forwards HTTP entries so **Jira** reaches Grok ACP — see `TestGrokACPExtraMCPServersForwardsStdioAndHTTPEntries` (replaces the old "drops HTTP" test).
  - `T-2`: `EnsureJiraMcpProviderConfig`/`EnsureFirebaseMcpProviderConfig`/`EnsureTelegramMcpProviderConfig` dispatch switches extended. Firebase/Telegram (stdio) support **all 4 providers**. Jira (remote HTTP) initially shipped **Claude, Codex, Gemini**; **Grok added in G2** via `ensureGrokJiraMcpConfig` (`config.toml` `url`+`headers` per xAI docs) + `PreflightJiraMcp("grok")`.
  - Jira bearer token is now **persisted** (`jiraCredential.BearerToken`, via `persistConnectedJiraBearerToken`/`resolveConnectedJiraBearerToken`) onto the same "one connected Jira integration" keyring record `resolveConnectedJiraCredential` already reads — so it survives across provider-config calls and the per-turn merge without re-entry after the first paste. `EnsureJiraMcpProviderConfig` persists a non-empty token and falls back to the persisted one when the request omits it (idempotent re-runs). Verified via `TestEnsureJiraMcpProviderConfigReusesPersistedBearerTokenOnRerun`.
  - `T-3`/`T-4`: `McpSettings.tsx`'s "Configure AI Provider" single-account panel replaced with **"Configure Providers"** — lists every account from the existing `/provider-accounts` endpoint (no new enumeration route needed; reused rather than duplicating `resolveGoogleDriveMcpProviderStatuses`'s per-provider `DiscoverProviderAccountHomes` scan, since `/provider-accounts` already covers registered accounts across all 4 providers) and loops the ensure call across all of them on one button, mirroring `GoogleDriveSettings.tsx`'s `configureAiProviders` (same per-account error aggregation, same `PROVIDER_LABELS` map). Jira's bearer-token field now feeds the same value into every account in the loop; the runner persists it once so later loop iterations (and future turns) reuse it.
  - `T-5`: `SetJiraIssueAdapter`/`SetJiraSprintAdapter`/`SetFirebaseCrashlyticsAdapter` calls removed from `AttachRunner` (`interactive_service.go`) — commented out with rationale, not deleted. Per explicit instruction, the adapter files themselves (`jiraRestIssueAdapter`/`jiraRestSprintAdapter` in `context_source_jira.go`, `firebase_tools_mcp_client.go`) are **kept intact**, resolving `Q-1` as "keep dormant" rather than delete.
  - `T-6`: CP-05-04 `DOD-3`/`DOD-4`, CP-05-05 `DOD-3`/`DOD-6`, and Task-229/231/233 completion notes all updated with addenda describing this revisit.
  - `Q-2` **CLOSED 2026-07-13 (G2 / CA-300):** Grok remote-HTTP Jira is wired end-to-end for provider-config + live turns.
    - Static: `ensureGrokJiraMcpConfig` writes `[mcp_servers.jira]` with `url = https://mcp.atlassian.com/v1/mcp/authv2`, `headers.Authorization`, `enabled = true` (xAI `config.toml` remote MCP shape).
    - Live: `grokACPExtraMCPServers` forwards HTTP-shaped `claudeMcpServer` entries (`type`/`name`/`url`/`headers`) into ACP `session/new`, reusing `jiraLiveMCPServer()` from the shared extra map.
    - Preflight: `PreflightJiraMcp("grok", accountHome)` checks non-stale on-disk entry (`checkGrokJiraMcpConfig`).
    - `grokMcpServer` gained additive `URL`/`Headers` (`omitempty`); stdio Firebase/Telegram/Drive writers unchanged.
    - Tests: `TestEnsureJiraMcpProviderConfigDispatchesToGrok`, `TestEnsureGrokJiraMcpConfigPreservesOtherTopLevelSections`, `TestEnsureGrokJiraMcpConfigIsIdempotent`, `TestGrokServerToMapRoundTripsHTTPServer`, `TestGrokACPExtraMCPServersForwardsStdioAndHTTPEntries`, `TestPreflightJiraMcpReadyWhenGrokProviderConfigured`, `TestPreflightJiraMcpFailsWhenGrokProviderMissing`, `TestPreflightJiraMcpFailsWhenGrokProviderStale`.
    - UI: `McpSettings.tsx` Configure Providers help text lists Grok for Jira.
  - `Q-3` resolved: the per-provider static-config loop runs **on demand only**, via the "Configure Providers" button — no eager write at integration-connect time (matches Drive's own behavior; connecting an integration and configuring AI providers are already treated as separate steps in this codebase).
  - Verification (Task-234 baseline + G2): `go test ./internal/runner -run 'Jira|GrokACP|GrokServer|ExtraMCP|PreflightJira|EnsureGrok'` PASS; Grok×Firebase/Telegram stdio regression still green.
- follow-ups: (1) Atlassian OAuth browser handshake **won't-do** (G1); (2) ~~Grok remote-HTTP Jira (`Q-2`)~~ **done G2**; (3) ~~Telegram per-send approval queue scope-threading (G3)~~ **won't-do** — v1 = auto-approve toggle only (CA-318); (4) optional: `PreflightJiraMcp` for `codex`/`gemini` (Ensure writers exist; preflight still Claude+Grok only); (5) credential re-resolve caching on live MCP helpers if merge frequency grows.
- upstream docs updated: CP-05-04 `DOD-3`/`DOD-4`, CP-05-05 `DOD-3`/`DOD-6`, Task-229 (§9 addendum), Task-231 (§10 addendum), Task-233 (§10 addendum); **G2:** CP-05-06 `DOD-4`/`DOD-6` notes, manual E2E runbook, CA-300.

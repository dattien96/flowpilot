# Task-302: Opencode Settings — Detect / Install / Models / MCP / Account

## Metadata

- Document ID: `Task-302`
- Title: `Opencode Settings — Detect / Install / Models / MCP / Account`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-29` (done — CP-57 §10.2 verification record; CA-679..CA-683, BUG-329)
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md), [Task-300: Opencode ACP Transport](./Task-300-Opencode-ACP-Transport-And-Process-Dispatcher.md), [Task-301: Opencode Controlled Adapter MVP](./Task-301-Opencode-Controlled-Adapter-MVP.md)
- Child Documents: `None`
- Related Documents: [Task-213: Auto-Detect And Sync Provider Models](../done/Task-213-Auto-Detect-And-Sync-Provider-Models.md), [Task-215: Per-Model Reasoning-Effort Detection](../done/Task-215-Per-Model-Reasoning-Effort-Detection-And-Model-Aware-UI.md), [Task-210: Grok Account Model](../done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- Replaces: `None`
- Tags: `opencode, settings, ai-providers, detection, install, models, mcp, account`

## AI Quick View

### Summary

- Make the **Settings page** work for Opencode exactly like Codex/Claude/Grok: `AI Providers` row shows installed `1.18.23` vs compat `CompatTestedOpencodeVersion`, one-click `Install OpenCode CLI`, `Detect models` imports `opencode/*` + `opencode-go/*` (both your xAi and Go plans) into `ai_supported_models` with `source:"detected"` + provenance, and `CheckVersionSettings` **4th row** (currently 3 rows: Claude/Codex/Grok at `CheckVersionSettings.tsx:219-233`).
- One-click **MCP** Google Drive / Jira → writes `~/.config/opencode/opencode.json` `mcpServers` entry (not Claude `settings.json` nor Codex `config.toml`), with per-managed-account `OPENCODE_CONFIG` isolation. Firebase/Telegram MCP remain **out of scope** in this task (future separate task; CP-57 `P-7` scope is Google/Jira only).
- **Account panel** shows Opencode zen account (`opencode providers` + `opencode stats`): email/plan/models active, usage `cost`/`tokens` from `stats`, **token limit `N/A`** tooltip (zen proxy does not expose upstream `monthlyLimit`) — hides `ModelContextWindow` progress bar when `nil`.

### Current Ask

- Wire runner detection/install/model-probe/MCP-config/account-metadata for `opencode` and drive them from the existing desktop `AiProvidersSettings.tsx` / `ProviderAccountsPanel.tsx` / `McpSettings.tsx` / `GoogleDriveSettings.tsx` flows without forking their generic loops.

### Key Decisions

- `T-1` **Reuse `Task-213` pattern** for model sync: runner `GET /providers` `provider.models[]` is the source of truth; desktop diffs `detectedModel.id` vs `ai_supported_models.model_id` for `provider_key='opencode'` and inserts only missing rows as `source:"detected"`, `detection_method:"opencode_models"`, `detected_cli_version:<detectedVersion>`, `last_detected_at:now` — never overwrites existing `is_enabled/display_name/sort_order/source`.
- `T-2` **Detect via `opencode models`** (not `opencode --version` alone): `detectOpencodeModels()` parses `opencode models` text (or `--format json` if available) into `ProviderModel{ID,DisplayName,Source:"opencode_models", SupportedReasoningEfforts:[...variant levels], ContextWindowTokens, MaxContextWindowTokens}`; failure keeps provider `Installed=true` but `Models=[]` + `LastError` (like Grok fallback to static list `Task-213 DOD`).
- `T-3` **Install via `npm i -g opencode-ai@latest`** (primary) fallback `irm https://opencode.ai/install | iex` on Windows; `providerInstallCommand` opencode case special-cases binary existence check via `lookPathFn("opencode")`.
- `T-4` **MCP writes to `opencode.json`**, not shared Codex/Grok `config.toml`: new `opencode_mcp_provider_config.go` implements `EnsureOpencodeMcpProviderConfig` + `getOpencodeMcpConfigPath(home)` = `filepath.Join(home, ".config", "opencode", "opencode.json")` (plus `~/.config/opencode/config.json` fallback observed live) with `writeFile+rename` atomically; validates schema before write.
- `T-5` **Account single zen**: unlike `GROK_HOME` multi-home, Opencode is one zen token → `DetectDefaultAccountHomePath` returns `~/.config/opencode`; managed homes `.opencodeHomeN` map to `OPENCODE_CONFIG` per account for isolation; `loadOpencodeAccountMetadata` reads `opencode providers` (email/plan + subscription vs free) + `opencode stats` (cost/tokens) and populates `ProviderAccountSummary{ProviderKey:"opencode", Remaining5hPercent=nil, UsageDetailLines:["cost: $x", "tokens: y/..."]}`.
- `T-6` **Token limit `N/A`**: do not fabricate `monthlyLimit`; Settings `ProviderAccountsPanel.tsx:67 compactUsageLines` stays data-driven; desktop hides progress bar when `ModelContextWindow==nil` and shows tooltip `"Opencode is a zen proxy — limit depends on upstream provider (e.g. opencode/claude-opus-5), see opencode stats for usage/cost"` (verified live `opencode stats` is per-session cost, not account-wide, so do not aggregate across sessions).

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-57 `P-0`):** additive-only; `providerSpecs()` (`runner.go:1669`), `provider_accounts.go` `managedProviderHomePrefix:640`, `runner.go` `providerInstallCommand`/`getEnvForExecution:1155`, `compat.go:20`, `AiProvidersSettings.tsx:239` install-button generalization must keep Codex/Claude/Grok/Gemini branches byte-identical — covered by `providerKeyFromModel` / `providerSpecs` / `provider_registry` unit tests (P-0 regression), not `domain_hardcode_guard_test.go:16`.
- Generic `McpSettings.tsx:324 for (account of providerAccounts)` loop already iterates all providers — only `PROVIDER_LABELS` needs the new entry.
- `ai_supported_models` `CHECK provider_key` constraint already allows free-text per `Task-213` migration `20260710100000_allow_grok_supported_models.sql` — no migration needed for `opencode`, but add `sort_order` seeding.
- Do not log `auth.json` tokens or `mcpServers` headers.

### Open Questions

- `Q-1` Where exactly does `opencode models` emit per-model `context_window`/`supported_variant` metadata? If not in CLI output, fall back to static variant list `minimal/low/medium/high/max`.
- `Q-2` Does `opencode.json` `mcpServers` require `command`→`opencode mcp` subcommand or HTTP MCP entry like Claude/Grok FlowPilot server? Confirm via live `opencode mcp` help during implementation.

### Source Refs

- `CP-57` Work Breakdown `P-7` (Key Decisions `P-7`, `P-11`), exhaustive scan rows 5-10, 16, 25.
- `Task-213`, `Task-210`, `Task-215` (pattern to port).
- Runner: `apps/local-runner/internal/runner/runner.go:1669 providerSpecs`, `354 DetectProviders`, `419 InstallProvider`, `provider_accounts.go:640 managedProviderHomePrefix`, `727 discover*`, `compat.go:20 CompatTestedGrokVersion`, `google_drive_mcp_provider_config.go:1039 flowpilotClaudeExtraMCPServers`, `apps/local-runner/internal/tooling/check.go`
- Desktop: `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx:108 detectModels, 239 installProvider, 327 <option>`, `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx:22 PROVIDER_LABELS`, `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx:95`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx:7 PROVIDERS`, `packages/.../adminModels.ts:89`, `apps/desktop-flowpilot/src/types/contract.ts:11`, `packages/flowpilot-client-core/src/domain/adminLogic.ts:3 resolveProviderKeyForModel`

## 1. Goal

Settings can manage Opencode end-to-end: detect installed `1.18.23`, install missing CLI, `Detect models` imports both `opencode/*` + `opencode-go/*` into the catalog, one-click MCP writes the correct `opencode.json` path per managed account, and the account card shows zen usage with `N/A` for per-model token limit.

## 2. Parent Links

- coding plan: `CP-57` Work Breakdown `P-7` (Key Decisions `P-7`, `P-11`)
- tech design: `SD-06`, `SD-11`, `SD-14`
- system spec: `SS-05`
- specific upstream ids: `CP-57 Work Breakdown P-7` (Settings detect/install/models/MCP/account)

## 3. Trigger

Task-301 makes Opencode runnable but invisible in Settings — detection/install/models/MCP/account wiring is the Settings DOD half.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/runner.go:1669 func providerSpecs() []providerSpec` — Current: `return []providerSpec{ {Key:"codex",... Models:[]ProviderModel{ID:"gpt-5.5"...}}, {Key:"claude",...}, {Key:"gemini", BinaryName:"agy", Models:defaultGeminiProviderModels()}, {Key:"grok", BinaryName:"grok" ... Models:defaultGrokProviderModels()} }` (codex/claude/gemini/grok appended last per CP-46 P-0, grok block at `:1700-1712`). Delta: append **last** `{Key:"opencode", Label:"Opencode", BinaryName:"opencode", InstallHint:"npm i -g opencode-ai@latest or https://opencode.ai/install"}` (no reorder). Copy `runner.go:1877 func detectGrokModels() ([]ProviderModel,error)` (reads `~/.grok/models_cache.json` + `types.go:28 ProviderModel`): add `func detectOpencodeModels(ctx context.Context) ([]ProviderModel,error)` at same file, called from `resolveProviderModels`/`detectProvider` **append case last** like `detectGrokModels`: `exec.CommandContext(ctx, opencodeBinaryName(),"models")` try `--format json` (if exit 0 parse `[{id,displayName,contextWindow}]`), fallback text parse `opencode models` lines (40+ `opencode/*` + `opencode-go/*`), map each to `ProviderModel{ID:"opencode/gpt-5.6-terra", DisplayName:"GPT 5.6 Terra (Opencode)", Available:true, Source:"opencode_models", SupportedReasoningEfforts:[]string{"minimal","low","medium","high","max"}, DefaultReasoningEffort:"medium", ContextWindowTokens:int(parsed or 0), MaxContextWindowTokens:int}`; include both namespaces; skip unsupported `opencode/deepseek-v4-flash-free` with `LastError="ProviderModelNotFoundError Did you mean..."`. Test: `TestDetectOpencodeModelsParsesBothNamespaces` asserts `opencode/`+`opencode-go/` ≥40, `SupportedReasoningEfforts` non-empty.
- `T-2` `apps/local-runner/internal/runner/provider_accounts.go:640 func managedProviderHomePrefix(providerKey string) (string,bool)` — Current: `switch strings.ToLower(providerKey){ case "codex":".codexHome", case "claude":".claudeHome", case "gemini":".geminiHome", case "grok":".grokHome" (CP-46 P-0 appended last) default:"",false }` (`:640-653`). Delta: append `case "opencode": return ".opencodeHome",true` **last** before default. Add `func isValidOpencodeAccountPath(path string) bool` checking `filepath.Join(path,".config","opencode","opencode.json")` exists OR `Join(path,".config","opencode","config.json")` plus `auth` file if any (mirror `isValidGrokAccountPath` at `:919` checking `config.toml`/`auth.json`). Add `func discoverOpencodeAccountHomes() []string` + append `"opencode"` to `DiscoverProviderAccountHomes()` switch at `:727 discover*` (copy `discoverGrokAccountHomes`), to `NextAccountHomePath` prefix branch at `runner.go:4200 NextAccountHomePath`, to `syncProviderAccounts` loop, to `HasLocalAuthAtPath` case (check `~/.config/opencode/auth.json` OR `opencode.json` with `"auth"` key parsed), to `defaultAuthCandidates` (`runner.go:2124`) / `accountAuthPaths` (`:2155`) / `hasValidProviderAuthFile` (`:4280`) — all **appended last** (P-0). Test: `TestDiscoverOpencodeAccountHomes` asserts default `~/.config/opencode` + managed `~/.opencodeHome2` found.
- `T-3` `apps/local-runner/internal/runner/runner.go:4237 func providerEnvSetCommand` / `runner.go:1155 func getEnvForExecution` / `runner.go:4167 func StartInteractiveAuth` / `runner.go:3943 func AuthenticateProvider` — Current: `providerEnvSetCommand` emits `export GROK_HOME=...` per provider; `getEnvForExecution` at `:1155` switches `HOME`/`XDG_CONFIG_HOME`/`GEMINI_HOME`/`GROK_HOME`/`CLAUDE_CONFIG_DIR` per `managedProviderHomePrefix` (copy `provider_registry.go:270 Codex CODEX_HOME, 322 Claude, 395 Gemini, 460 Grok`); `StartInteractiveAuth` launches `grok login --device-auth` / `claude auth login`. Delta: add `case "opencode": env["OPENCODE_HOME"]=account.HomePath; env["OPENCODE_CONFIG"]=Join(account.HomePath,".config","opencode"); env["HOME"]=account.HomePath; env["XDG_CONFIG_HOME"]=Join(account.HomePath,".config")` plus Windows `USERPROFILE/APPDATA/LOCALAPPDATA/HOMEDRIVE/HOMEPATH` via `windowsHomeDriveAndPath(account.HomePath)` like Grok `provider_registry.go:460` block (copy). Add `StartInteractiveAuth` opencode case → `exec.CommandContext(ctx, opencodeBinaryName(), "auth", "login")` (fallback `"opencode login"` if `auth` subcommand missing, probe `opencode --help`); `AuthenticateProvider` opencode case → `opencode providers` verify. Do NOT inherit host `OPENCODE_API_KEY` beyond scope (strip `os.Environ` then inject only `scopeKey` env).
- `T-4` `apps/local-runner/internal/runner/runner.go:419 func InstallProvider` / `runner.go:2320 func providerInstallCommand` / `runner.go:2319 runProviderInstallCommand` — Current: `InstallProvider` → `providerInstallCommand` switch `codex→"npm i -g @openai/codex"`, `claude→"npm i -g @anthropic-ai/claude-code"`, `gemini→"npm i -g @antigravity/cli"`, `grok→"irm https://x.ai/cli/install.ps1 | iex"` (Windows) else `curl ...`. Delta: append `case "opencode": if lookPathFn("npm")==nil && runtime.GOOS=="windows" { return `powershell -Command "irm https://opencode.ai/install | iex"` } else { return `npm i -g opencode-ai@latest` }` (check `lookPathFn` at `runner.go:31 var lookPathFn=exec.LookPath`); `runProviderInstallCommand` executes `exec.CommandContext(ctx, "sh","-c", cmd)` or `powershell` on Windows. `DetectProvidersCached` (`:372`) invalidation already generic (`providersCacheTTL=3s`): no change, but add `invalidateProvidersCache()` call after opencode install like grok.
- `T-5` `apps/local-runner/internal/runner/opencode_mcp_provider_config.go` (new) — **copy `google_drive_mcp_provider_config.go:1039 flowpilotClaudeExtraMCPServers` + `claude_mcp_provider_config.go` atomic writer**. Current: `google_drive_mcp_provider_config.go:1039 func (r *Runner) flowpilotClaudeExtraMCPServers(accountHomePath string, yolo bool) map[string]claudeMcpServer` merges `jira`/`firebase`/`telegram` when connected; `EnsureGoogleDriveMcpConfig` does `os.WriteFile(tmp)+os.Rename` atomic, validates JSON via `json.Unmarshal`, preserves unrelated keys. Delta: implement `func getOpencodeMcpConfigPath(home string) string { return filepath.Join(home,".config","opencode","opencode.json") }` (fallback `filepath.Join(home,".config","opencode","config.json")` if `opencode.json` missing, observed live `~/.config/opencode/opencode.json` + `auth.json`). Implement `func EnsureOpencodeGoogleDriveMcpConfig(home string, yolo bool) error` + `EnsureOpencodeJiraMcpConfig` — each reads existing `opencode.json` (if missing create `{"mcpServers":{}}`), sets `mcpServers["google-drive"] = map[string]any{"command":"flowpilot-mcp","env":{"FLOWPILOT_MCP_TOKEN":token},"args":[]}` (or actual `claudeMCP` HTTP entry like `grokACPExtraMCPServers`), validates `json.Valid`, writes to `tmp+rename` (preserve unrelated `permission`/`agent.build.model` keys), merges `flowpilotClaudeExtraMCPServers(yolo)` per-turn if `mcpServers` HTTP. **Scope Google/Jira only** — Firebase/Telegram out of scope (future separate task, CP-57 P-7). Test: `TestEnsureOpencodeGoogleDriveMcpConfig` asserts file contains `mcpServers.google-drive.command=="flowpilot-mcp"`; `TestEnsureOpencodeMcpConfigAtomicWrite` kills mid-write and asserts no corrupt `opencode.json`.
- `T-6` `apps/local-runner/internal/runner/compat.go:20` + `apps/local-runner/internal/runner/compat_config_test.go` — Current: `compat.go:20 const CompatTestedClaudeVersion="2.1.179", CompatTestedCodexVersion="0.140.0", CompatTestedGrokVersion="0.2.93"` (`:20-24`) + `CompatConfig struct { TestedClaudeVersion, TestedCodexVersion, TestedGrokVersion, Installed... }` (`:214-231`) + `RunCompatCheck` at `compat.go:459 func compatRunVersion`. Delta: append **last** `const CompatTestedOpencodeVersion = "1.18.23"` (P-0); add to `CompatConfig` `TestedOpencodeVersion, InstalledOpencodeVersion string; OpencodeVersionOk bool` **appended last** (JSON shape for Codex/Claude/Grok unchanged, `CheckVersionSettings.tsx` row order preserved); in `RunCompatCheck` add `config.TestedOpencodeVersion = CompatTestedOpencodeVersion` + `installed,ok:=compatRunVersion(ctx,"opencode","--version")`. Desktop `CheckVersionSettings.tsx:219-233` currently 3 `VersionRow` (Claude/Codex/Grok) → add 4th `VersionRow{label:"Opencode", tested:info.testedOpencodeVersion, installed:info.installedOpencodeVersion}`. Test: `compatConfig_test.go` add opencode branch `TestCompatConfigTestedOpencodeVersion` asserts `TestedOpencodeVersion=="1.18.23"`; `go test -run CompatConfig` green.
- `T-7` `apps/local-runner/internal/tooling/check.go:33 func CheckTool(name string, repoDir string) ToolStatus` — Current: `func CheckTool(name string, repoDir string) ToolStatus { lookPath, runCommand " --version", ToolStatus{Status:"ok"/"missing", Version} }` (called via `tooling/check.go:CheckTool("grok",repoDir)`). Delta: ensure `CheckTool("opencode", repoDir)` works via `lookPathFn("opencode")` (`runner.go:31 var lookPathFn`) + `runCommandFn(ctx,"opencode","--version")` (mockable); `tooling.json` entry `{"tool":"opencode","name":"opencode","status":"ok","version":"1.18.23"}` or `"missing"` with `installHint:"npm i -g opencode-ai"`. Do NOT add `os.Getenv("OPENCODE")` special case.
- `T-8` `apps/local-runner/internal/runner/provider_registry.go:221-247 DefaultProviderRegistry` + `:455 ProviderRegistryFor` — Current: `DefaultProviderRegistry` registers `codex` available, `claude`/`gemini` placeholder only — **no grok/opencode**; `ProviderRegistryFor` does `if grokAgentEnabled(){ reg.register(...ProviderKeyGrok...) }` at `:455` (closure `newAdapterForTurn` with `scopeKey=account.ID`, `env[HOME/XDG/GROK_HOME]`). Delta: **DEFER** `opencodeAgentEnabled()` gate — owned by **Task-301 T-10** (`ProviderRegistryFor` live only). This task only ensures `DefaultProviderRegistry` has **no** opencode entry (like Grok `provider_registry.go:221-247`); do NOT duplicate `if opencodeAgentEnabled(){...}` wiring here (would duplicate T-10). Add `// DEFERRED to Task-301` comment in Touched Areas.
- `T-9` Desktop `apps/desktop-flowpilot/src/types/contract.ts:11 type ProviderKey = "codex"|"claude"|"gemini"|"grok"` + `packages/flowpilot-client-core/src/domain/adminLogic.ts:3 func resolveProviderKeyForModel` + `packages/.../adminModels.ts:89` — Current: `contract.ts:11` union lacks opencode; `adminLogic.ts:3 resolveProviderKeyForModel(modelId,supportedModels)` has `if modelId.startsWith("gpt-") return "codex"; if gemini→"gemini"; if claude→"claude"; if grok→"grok"||"grok-build" return "grok"; return supportedModels.find(...)?providerKey:null` (`:3-15`); `adminModels.ts:89 SupportedModel.providerKey:"codex"|"claude"|"gemini"|"grok"`. Delta: append **last** `| "opencode"` to `ProviderKey` union (`contract.ts:11`); in `adminLogic.ts:3` add `if (modelId.startsWith("opencode/")||modelId.startsWith("opencode-go/")) return "opencode"` **before fallback** (grok prefix `:12` stays last before fallback, P-0); in `adminModels.ts:89` add `| "opencode"` to `SupportedModel.providerKey` union `| "opencode"`. Test: `adminLogic.test.ts` asserts `resolveProviderKeyForModel("opencode/gpt-5.6-terra")== "opencode"` and `"grok-4.5"== "grok"` unchanged.
- `T-10` Desktop `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx:22 const PROVIDER_LABELS` + `:239 installProvider` + `:327 <option>` — Current: `AiProvidersSettings.tsx:22 PROVIDER_LABELS={codex:"Codex",claude:"Claude",gemini:"Gemini",grok:"Grok"}`; `:239 installProvider(providerKey)` has `providerKey==="gemini" ? "AGY CLI" : providerKey` (gemini-only special); `:327` draft `<option>` lacks opencode; `:108 detectModels`, `:120 detectModels` data-driven. Delta: add `opencode:"OpenCode"` **last** to `PROVIDER_LABELS`; generalize `:239` to `providerKey==="gemini"?"AGY CLI":providerKey==="opencode"?"OpenCode CLI":providerKey` (keep gemini special); add `298 Local Providers` row `provider.key==="opencode"` label branch; add `<option value="opencode">opencode</option>` **last** at `:327` (appended, P-0); `:120 detectModels` no code change (data-driven) but ensure `supportedReasoningEfforts`/`contextWindowTokens` stamped from `ProviderModel` (like `detectGrokModels` `Task-215`). Test: `AiProvidersSettings.test.tsx` `it('generalizes install label for opencode')`.
- `T-11` Desktop `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx:22` + `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx:95` + `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx:7` — Current: `McpSettings.tsx:22 const PROVIDER_LABELS={codex:"Codex",gemini:"Gemini",claude:"Claude",grok:"Grok"}`; `GoogleDriveSettings.tsx:95` same loop `for (account of providerAccounts)` already iterates all; `ProviderAccountsPanel.tsx:7 const PROVIDERS=[{key:"claude"}, {key:"codex"}, {key:"gemini"}, {key:"grok"}] as const`. Delta: append **last** `opencode:"OpenCode"` to both `PROVIDER_LABELS`; append `{key:"opencode",label:"OpenCode"}` **last** to `PROVIDERS` (P-0); `packages/.../adminModels.ts` `source:"detected"` badge already generic (no change). Test: `McpSettings` snapshot shows opencode label after grok.
- `T-12` Desktop `apps/desktop-flowpilot/src/state/store.ts:2256 export function providerLabel` + `store.ts:187 func pickDefaultModel` — Current: `providerLabel` at `:2256` switches `codex→"Codex", claude→"Claude", gemini→"Gemini", grok→"Grok"`, default `return providerKey`; `pickDefaultModel` at `:187` has `grok` branch `if provider==="grok" return supportedModels.find(m=>m.providerKey==="grok"&&m.isEnabled)?.modelId`. Delta: append **last** `if(providerKey==="opencode") return "OpenCode"` to `providerLabel` (P-0); `pickDefaultModel` opencode heuristic is **Task-303** (this task only ensures `providerLabel("opencode")` does NOT fall through to raw `"opencode"` for account card `formatAuthLabel`).
- `T-13` `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx:67 func compactUsageLines` + `apps/local-runner/internal/runner/opencode_account.go` (new helper) — Current: `compactUsageLines` at `:67` stays data-driven (`Remaining5hPercent`/`Remaining7dPercent` nil → hide progress bar, `usageDetailLines` generic); `opencode_account.go` does not exist. Delta: keep `compactUsageLines` generic (only `gemini` filters `3.1` per `store.ts:2863`); add `opencode_account.go` with `func loadOpencodeAccountMetadata(home string) (ProviderAccountSummary,error)` reading `opencode providers` (`email`/`plan`) + `opencode stats` (`cost`/`tokens`) via `exec.CommandContext(ctx, opencodeBinaryName(),"providers")` + `... "stats"` (both JSON if available, fallback text parse), populating `ProviderAccountSummary{ ProviderKey:"opencode", AccountID:deterministicProviderAccountID("opencode",home), DisplayName:"OpenCode", Remaining5hPercent:nil, Remaining7dPercent:nil, UsageDetailLines:[]string{fmt.Sprintf("cost: $%.2f",cost), fmt.Sprintf("tokens: %d",tokens)} }`; on `stats` failure, return metadata sans usage with `LastError` (degrade, not crash). Settings hides `ModelContextWindow` progress bar when `nil` with tooltip `"Opencode is a zen proxy — limit depends on upstream provider (e.g. opencode/claude-opus-5), see opencode stats for usage/cost"` (verified `stats` per-session not account-wide, do NOT aggregate).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/runner.go:1669` (`providerSpecs`, `detectOpencodeModels`, `resolveProviderModels`, `providerInstallCommand`, `getEnvForExecution:1155`, `providerEnvSetCommand`, `StartInteractiveAuth`, `DetectDefaultAccountHomePath`), `apps/local-runner/internal/runner/provider_accounts.go:640` (`managedProviderHomePrefix`, `Discover*`, `isValidOpencodeAccountPath`, `syncProviderAccounts`), `apps/local-runner/internal/runner/provider_event.go` (already in Task-301 — no change here), `apps/local-runner/internal/runner/compat.go:20`, `apps/local-runner/internal/tooling/check.go` (`apps/local-runner/internal/tooling/check.go`), `apps/local-runner/internal/runner/opencode_mcp_provider_config.go` (new, Google/Jira only), `apps/local-runner/internal/runner/opencode_account.go` (new helpers), `apps/desktop-flowpilot/src/types/contract.ts:11`, `packages/flowpilot-client-core/src/domain/adminLogic.ts:3`, `packages/.../adminModels.ts:89`, `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx:239`, `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx:22`, `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx:95`, `apps/desktop-flowpilot/src/components/ProviderAccountsPanel.tsx:7`, `apps/desktop-flowpilot/src/state/store.ts:2256`
- modules: provider detection/install, supported-model sync, MCP provider config, account management
- routes: existing `GET /providers`, `POST /providers/install`, `POST /providers/import` (via `detectModels`), `GET /provider-accounts`, `POST /provider-accounts/*`, `GET /compat`
- tables: `ai_supported_models` (insert `provider_key='opencode'` rows `source:"detected"`), no schema migration needed

## 6. Acceptance Check

- Settings `Local Providers` row shows `Opencode — Installed 1.18.23 (tested 1.18.23)`; missing binary → `missing` + `Install OpenCode CLI` button works (`npm i -g opencode-ai`).
- `Detect models` for Opencode imports 40+ `opencode/*` + `opencode-go/*` rows as `source:"detected"` with `detection_method:"opencode_models"` and `supportedReasoningEfforts`; re-run is idempotent (skips existing ids).
- Google Drive / Jira `Connect` in Settings writes `~/.config/opencode/opencode.json` `mcpServers.google-drive` / `mcpServers.jira` and `opencode mcp` lists it; managed-home `.opencodeHomeN` writes to that home's `opencode.json`.
- Account panel shows `OpenCode` account from `opencode providers`, `cost`/`tokens` from `opencode stats`, token-limit chip shows `N/A` with tooltip, progress bar hidden when `ModelContextWindow==nil`.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `detectOpencodeModels` returns 40+ models (`opencode/`+`opencode-go/`) with `ProviderModel{SupportedReasoningEfforts,DefaultReasoningEffort}` populated. (`TestDetectOpencodeModelsParsesBothNamespaces`.)
- [ ] `DOD-2` `DetectProviders` Opencode row `Installed=true` when `opencode --version` succeeds, `Installed=false` otherwise, `LastError` set. (`TestDetectOpencodeInstalled`.)
- [ ] `DOD-3` `InstallProvider("opencode")` executes correct command and flips to `Installed=true`. (`TestInstallOpencodeProvider` with mocked `runProviderInstallCommand`.)
- [ ] `DOD-4` Desktop `Detect models` idempotent insert: first click inserts `opencode/gpt-5.6-terra` etc., second click inserts zero new rows. (`TestOpencodeModelSyncIdempotent` + `AiProvidersSettings.detectModels` integration.)
- [ ] `DOD-5` MCP one-click: `EnsureOpencodeGoogleDriveMcpConfig` writes `opencode.json` atomically; `checkOpencodeMcpConfig` reads it. (`TestEnsureOpencodeGoogleDriveMcpConfig`, `TestEnsureOpencodeJiraMcpConfig`.)
- [ ] `DOD-6` Managed home `.opencodeHomeN` isolation: `NextAccountHomePath("opencode", 2)=="~/.opencodeHome2"` + `DiscoverOpencodeAccountHomes` finds both default and managed.
- [ ] `DOD-7` Account metadata: `loadOpencodeAccountMetadata` populates email/plan from `opencode providers` + cost/tokens from `opencode stats`; failure degrades without crash. (`TestLoadOpencodeAccountMetadata`.)
- [ ] `DOD-8` Version baseline: `CompatTestedOpencodeVersion="1.18.23"` + `CompatVersionInfo` fields + `CheckVersionSettings` **4th row** (`CheckVersionSettings.tsx:219-233`) renders; `go test ./internal/runner -run CompatConfig` green.
- [ ] `DOD-9` Base-regression: `providerSpecs` order unchanged (`codex,claude,gemini,grok,opencode` appended at `runner.go:1669`), `providerKeyFromModel` + `defaultModelForProvider` for old providers unchanged, `providerKeyFromModel` / `providerSpecs` / `provider_registry` unit tests green (not `domain_hardcode_guard_test.go:16`).
- [ ] `DOD-10` Manual: Settings screenshots show Installed, model list, MCP `opencode.json` on disk, account `N/A` tooltip.

### 6.2 Test Signatures

```go
// runner/runner_test.go (append last, per P-0)
func TestDetectOpencodeModelsParsesBothNamespaces(t *testing.T)
func TestDetectOpencodeModelsHandlesJsonAndTextFallback(t *testing.T)
func TestDetectOpencodeModelsMissingBinaryReturnsError(t *testing.T)
func TestResolveProviderModelsOpencodeUsesDetectedCache(t *testing.T)
func TestDetectOpencodeInstalled(t *testing.T)
func TestInstallOpencodeProvider(t *testing.T)

// runner/provider_accounts_test.go
func TestDiscoverOpencodeAccountHomes(t *testing.T)
func TestIsValidOpencodeAccountPath(t *testing.T)
func TestNextAccountHomePathOpencode(t *testing.T)
func TestHasLocalAuthAtPathOpencode(t *testing.T)

// runner/opencode_mcp_provider_config_test.go (Google/Jira only in this task)
func TestEnsureOpencodeGoogleDriveMcpConfig(t *testing.T)
func TestEnsureOpencodeJiraMcpConfig(t *testing.T)
func TestCheckOpencodeMcpConfig(t *testing.T)
func TestEnsureOpencodeMcpConfigAtomicWrite(t *testing.T)

// runner/opencode_account_test.go
func TestLoadOpencodeAccountMetadata(t *testing.T)
func TestLoadOpencodeAccountMetadataHandlesStatsFailureGracefully(t *testing.T)

// desktop AiProvidersSettings.test.tsx (if exists, TS not Go)
it('detects Opencode models and inserts both namespaces', async () => {})
```

## 7. Out of Scope

- `ProviderRuntimeAdapter` streaming/permission/MCP per-turn wiring — Task-301/303.
- `ReasoningEffort → variant` mapping beyond `SupportedReasoningEfforts` catalog rows — Task-301/303.
- `LocateSessionFile` / `handoff_context` / `summarizer` for Opencode — Task-303.
- Pruning stale `opencode/deepseek-v4-flash-free` row — do in Task-302 `DOD-4` as idempotent skip, not delete.

## 8. Completion Notes

- result:
- implementation notes: `runner.go` `detectOpencodeModels` + `providerSpecs` opencode, `provider_accounts.go` `.opencodeHome` prefix, `opencode_mcp_provider_config.go` atomic `opencode.json` writer, `compat.go` version, desktop `contract.ts`/`adminModels.ts`/`AiProvidersSettings.tsx`/`ProviderAccountsPanel.tsx` opencode branches.
- verification:
- follow-ups: live `opencode models --format json` flag existence (fallback to text parse already covered).
- upstream docs updated: `CP-57` Work Breakdown `P-7`, scan rows 5-10, 16, 25

# Task-402: Devin Settings: Detect, Install, Models, MCP, and Account

## Metadata

- Document ID: `Task-402`
- Title: `Devin Settings: Detect, Install, Models, MCP, and Account`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-20`
- Last Updated: `2026-09-20`
- Parent Documents: [CP-70: Devin Provider Integration](../../07-Coding-Plan/todo/CP-70-Devin-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-302: Opencode Settings](../../08-Task/done/Task-302-Opencode-Settings-Detect-Install-Models-MCP-Account.md), [Task-401: Devin Adapter MVP](../../08-Task/todo/Task-401-Devin-Controlled-Adapter-MVP.md), [CP-70-note.md](../../07-Coding-Plan/note/CP-70-note.md), [CP-70-Test-Steps.md](../../07-Coding-Plan/todo/CP-70-Test-Steps.md)
- Replaces: `None`
- Tags: `devin, settings, cli-detection, 1-click-install, model-catalog, mcp-config, desktop-ui`

## AI Quick View

### Summary

- Wire Devin CLI into the FlowPilot Settings ecosystem across both the Go local-runner and the TypeScript desktop application.
- Implement CLI detection (`devin --version`), one-click CLI installer (`curl -fsSL https://cli.devin.ai/install.sh | bash` on macOS/Linux, `irm https://static.devin.ai/cli/setup.ps1 | iex` on Windows — F-12; **không có brew cask**), and compatibility version check (`CompatTestedDevinVersion = "3000.10.31"`).
- Build live model catalog probe (`detectDevinModels()`) in `resolveProviderModels` và sync available models vào `ai_supported_models` **luôn kèm prefix `devin/`** (alias Devin `opus`/`gpt`/`codex`/`swe` trùng tên provider khác — F-9) without changing DB schema (`schema: none`); fallback static list khi chưa login (`devin models list` cần auth).
- Implement 1-click MCP configuration write (`devin_mcp_config.go`): write Google Drive and Jira MCP server entries vào `~/.config/devin/mcp_config.json` key `mcpServers` (dedicated file từ v3000.3 — F-11), hoặc `devin mcp add -s user`; `session/new{mcpServers}` chỉ hỗ trợ stdio nên remote MCP bắt buộc đi qua file (F-2).
- Integrate Devin into Desktop Account Management: add `"devin"` to `PROVIDERS` in `ProviderAccountsPanel.tsx`, support multi-account managed homes (`.devinHome` với `credentials.toml` dưới `.local/share/devin/`), display auth status (`devin auth status`) + usage từ `turn_completed._meta` (không có quota CLI riêng — Issue 17).

### Current Ask

- Implement runner endpoints and desktop UI components for Devin configuration, installation, catalog discovery, account management, and MCP wiring.

### Key Decisions

- `T-1` No Schema Change: As established in CP-70 §6, do not create a Supabase migration; populate `ai_supported_models` through CLI detection and contract types.
- `T-2` Standalone MCP Config Helper: Create `devin_mcp_config.go` to handle JSON config read/write for Devin, avoiding coupling with Claude or Opencode MCP writers.
- `T-3` Multi-Account Home Isolation: Support `.devinHome` per account with `isValidDevinAccountPath` checking for valid auth tokens.
- `T-4` Desktop Compatibility Row: Place Devin as the 5th row on `CheckVersionSettings.tsx` without disrupting existing provider row ordering.

### Constraints

- Tham chiếu theo **symbol name** (line numbers drift): `resolvePromptExecutionAdapter`, `providerSpecs`, `runProviderInstallCommand`, `getEnvForExecution`, `resolveProviderModels`, `hasValidProviderAuthFile`, `defaultAuthCandidates`, `StartInteractiveAuth`, `managedProviderHomePrefix`.
- Do not break existing provider installation scripts or settings rendering.

### Open Questions

- `Q-1` (đã trả lời): `devin auth status` tồn tại (text); credentials `~/.local/share/devin/credentials.toml` (keys: `windsurf_api_key`, `api_server_url`, `devin_webapp_host`, `devin_api_url`). **Quirk observed**: file có key 189 chars nhưng `auth status` báo "Not logged in" trong khi ACP `authenticate` PKCE thành công → check existence + non-empty `windsurf_api_key` là primary signal cho REPL path; cho ACP path, `authenticate` probe mới là truth.

### Source Refs

- `CP-70` §4 Work Breakdown `P-7`; §5.2 Rows 5, 6, 7, 8, 9, 11, 16, 25.
- Reference code: `apps/local-runner/internal/runner/runner.go:1666, 1871, 2445`, `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`.

---

## 1. Goal

Provide complete administrative and settings capabilities for Devin: detect installed CLI, install via 1-click, probe and synchronize available models, configure external MCP servers, and manage Devin accounts.

---

## 2. Parent Links

- Coding Plan: [CP-70: Devin Provider Integration](../../07-Coding-Plan/todo/CP-70-Devin-Provider-Integration.md) (Work Breakdown `P-7`)
- Tech Design: `SD-06`
- Upstream Task Reference: [Task-302: Opencode Settings](../../08-Task/done/Task-302-Opencode-Settings-Detect-Install-Models-MCP-Account.md)

---

## 3. Trigger

Users need a straightforward GUI flow to check whether Devin is installed, install it if missing, connect their credentials, discover models, and enable Google Drive / Jira MCP tools.

---

## 4. Exact Change

- `T-1` **Runner Provider Specs & Inventory (`runner.go:1666`)**:
  - In `providerSpecs()`:
  - Add `{Key: "devin", Binary: "devin", DisplayName: "Devin"}` to the inventory slice.
  - Update `internal/tooling/check.go` to support `CheckTool("devin")`.

- `T-2` **1-Click Installation Command (`runProviderInstallCommand`)**:
  - Add `case "devin"`:
    - macOS/Linux/WSL: `curl -fsSL https://cli.devin.ai/install.sh | bash` (installs `~/.local/bin/devin` → `_versions/current/bin/devin` symlink)
    - Windows: `irm https://static.devin.ai/cli/setup.ps1 | iex` (hoặc `https://cli.devin.ai/install.ps1`)
    - Upgrade path: `devin update [--force]` (self-managed installs only)

- `T-3` **Model Catalog Detection (`resolveProviderModels`)**:
  - Implement `detectDevinModels() ([]SupportedModel, error)`:
    - **Nguồn ưu tiên (F-21)**: ACP `session/new` trả `configOptions.model.options` (~380 entries, gồm `_meta.supportsImages`) — không phụ thuộc REPL auth (`devin models list` cần `devin auth login` riêng và đang fail trên máy dev dù ACP authenticate OK).
    - Fallback: `devin models list --format json` nếu REPL đã login; cuối cùng static list nhỏ (`devin/swe-2-high`, `devin/adaptive`, `devin/swe-1-6-fast`).
    - Map models with prefix `devin/*` — **bắt buộc**, vì alias Devin (`opus`/`codex`/`gpt`/`swe`) trùng tên provider khác (F-9).
    - If CLI not found, return empty list gracefully without error.
  - In `resolveProviderModels`:
    - Add `case "devin": return detectDevinModels()`.

- `T-4` **Auth File Verification & Account Login (`hasValidProviderAuthFile`, `defaultAuthCandidates`, `StartInteractiveAuth`)**:
  - In `hasValidProviderAuthFile`:
    - Add `case "devin":` check existence and validity of `~/.local/share/devin/credentials.toml` (path in ra bởi `devin auth status` — F-6); Windows `%APPDATA%\devin\credentials.toml`.
  - In `defaultAuthCandidates`:
    - Add default candidate paths for Devin (`<home>/.local/share/devin/credentials.toml`).
  - In `StartInteractiveAuth`:
    - Add `case "devin":` launch `devin auth login` (`--force-manual-token-flow` cho remote/SSH) hoặc `devin setup` trong interactive pseudo-terminal. **`devin configure` không tồn tại** (F-10).
    - Lưu ý: hai credential store tách biệt — REPL (`devin auth status`, `credentials.toml`) vs ACP (`authenticate` PKCE per-process, F-17). ACP authenticate đã verify chạy được khi browser session app.devin.ai còn hiệu lực, kể cả khi REPL báo "Not logged in".

- `T-5` **MCP 1-Click Configuration Writer (`devin_mcp_config.go`)**:
  - Implement `WriteDevinMCPConfig(accountHome string, mcpServers map[string]any) error`:
    - Target file: `<home>/.config/devin/mcp_config.json` (dedicated file từ v3000.3 — F-11; `_meta.mcpConfigPath` trong initialize confirm path). KHÔNG ghi vào `config.json`.
    - Merge `mcpServers` entries (Google Drive, Jira) without erasing existing settings; preserve unknown keys.
    - Write atomically back to disk (tmp + rename).
    - Phương án thay thế: `devin mcp add -s user <name> <url>` (CLI-managed, tự động chọn scope file).
  - In `GoogleDriveSettings.tsx`:
    - Update `PROVIDER_LABELS["devin"] = "Devin"`.

- `T-6` **Compatibility Version Tracking (`compat.go`, `CheckVersionSettings.tsx`)**:
  - In `compat.go`:
    - Add `CompatTestedDevinVersion = "3000.10.31"` (live-verified baseline trên máy dev).
    - Add `TestedDevinVersion string` + `InstalledDevinVersion string` to version payload.
  - In `CheckVersionSettings.tsx`:
    - Render 5th row for Devin version verification.

- `T-7` **Desktop Provider & Accounts Panel Integration**:
  - In `apps/desktop-flowpilot/src/types/contract.ts`:
    - Add `"devin"` to `ProviderKey` type union.
  - In `apps/desktop-flowpilot/src/components/settings/ProviderAccountsPanel.tsx`:
    - Add `{key: "devin", label: "Devin"}` to `PROVIDERS` array.
    - Support account path validation and switching.
  - In `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`:
    - Add Devin card with status badge (`Installed`, `Missing`), Install button, and `Detect models` action.

- `T-8` **Managed Account Homes (`managedProviderHomePrefix`)**:
  - In `managedProviderHomePrefix`:
    - Add `case "devin": return ".devinHome"`.
  - Managed home giữ layout XDG bên trong: `.config/devin/` + `.local/share/devin/` (credentials.toml, cli/sessions.db) để Devin resolve đúng qua `HOME`/`XDG_*` env; permissions `0700`.

- `T-9` **Account Status & Usage (`provider_accounts.go`)**:
  - Account status: `devin auth status` (text output) + check `credentials.toml` non-empty.
  - Usage: đọc `turn_completed._meta` (tokens/cost — Task-401 Q-1); không có per-account quota CLI riêng (Issue 17).

---

## 5. Touched Areas

- **files (runner, new):**
  - `apps/local-runner/internal/runner/devin_mcp_config.go`
  - `apps/local-runner/internal/runner/devin_mcp_config_test.go`
- **files (runner, extend):**
  - `apps/local-runner/internal/runner/runner.go` (`providerSpecs`, `runProviderInstallCommand`, `resolveProviderModels`, `hasValidProviderAuthFile`, `defaultAuthCandidates`)
  - `apps/local-runner/internal/runner/provider_accounts.go` (`StartInteractiveAuth`, `managedProviderHomePrefix`)
  - `apps/local-runner/internal/runner/compat.go`
- **files (desktop, extend):**
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/ProviderAccountsPanel.tsx`
  - `apps/desktop-flowpilot/src/components/settings/CheckVersionSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/GoogleDriveSettings.tsx`
- **modules:** Runner settings & Desktop settings UI

---

## 6. Acceptance Check

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` `CheckTool("devin")` correctly detects binary presence and reports version (`TestDevinToolingCheck`).
- [ ] `DOD-2` `runProviderInstallCommand("devin")` executes correct OS-specific command (`TestDevinInstallCommandArgs`).
- [ ] `DOD-3` `detectDevinModels()` returns catalog with `devin/*` prefixes (`TestDevinDetectModels`).
- [ ] `DOD-4` `hasValidProviderAuthFile("devin")` returns true when `~/.local/share/devin/credentials.toml` exists và non-empty (`TestDevinAuthFileCheck`).
- [ ] `DOD-5` `WriteDevinMCPConfig` merges Google Drive and Jira MCP servers into Devin config without corrupting existing keys (`TestDevinMcpConfigWrite`).
- [ ] `DOD-6` `CheckVersionSettings.tsx` renders Devin as 5th row with version badge without visual regression on existing rows.
- [ ] `DOD-7` `ProviderAccountsPanel.tsx` includes Devin in `PROVIDERS` list and allows adding/switching accounts.
- [ ] `DOD-8` `AiProvidersSettings.tsx` displays Devin card with accurate status, 1-click install, and detect models button.
- [ ] `DOD-9` Base regression: existing settings for Claude, Codex, Gemini, Grok, Opencode function identically.
- [ ] `DOD-10` `managedProviderHomePrefix("devin")` → `.devinHome` với layout `.config/devin` + `.local/share/devin` được tạo (`TestDevinManagedHomeLayout`).

### 6.2 Test Signatures

```go
// runner_test.go
func TestDevinToolingCheck(t *testing.T)
func TestDevinInstallCommandArgs(t *testing.T)
func TestDevinDetectModels(t *testing.T)
func TestDevinAuthFileCheck(t *testing.T)

// devin_mcp_config_test.go
func TestDevinMcpConfigWrite(t *testing.T)
func TestDevinMcpConfigPreservesCustomSettings(t *testing.T)
```

```typescript
// AiProvidersSettings.test.tsx
test("renders Devin provider card with install button when missing", () => {})
test("triggers model detection for Devin on button click", () => {})

// CheckVersionSettings.test.tsx
test("renders Devin version row at 5th position", () => {})
```

### 6.3 Code Signatures

```go
// runner.go
func (r *Runner) detectDevinModels() ([]SupportedModel, error)
func (r *Runner) hasValidDevinAuthFile(home string) bool

// devin_mcp_config.go
func WriteDevinMCPConfig(configFilePath string, servers map[string]any) error
func ReadDevinMCPConfig(configFilePath string) (map[string]any, error)
```

---

## 7. Out of Scope

- Chat and Flow mode execution — Task-401 and Task-403.
- Approval Gate cards in chat timeline — Task-403.
- Sub-agent spawning and execution — Task-403.

---

## 8. Completion Notes

- result: pending implementation
- implementation notes:
- verification:
- follow-ups: Unblocks Task-403
- upstream docs updated: `CP-70` Work Breakdown `P-7`

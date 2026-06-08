# CP-05-03: Google Drive MCP Current Implementation Notes

## Status

Partially implemented.

This document was refreshed after comparing the original plan with the latest code. Treat the current code as the source of truth.

Related work:

- [CP-27: Google Cloud Setting](../done/CP-27-Google-Cloud-Setting-Manually.md)
- [CP-28: Google Cloud Config With UI Auto](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)
- [Task-025: Drive MCP Auth Flow](../../08-Task/Task-025-Drive-MCP-Auth-Flow.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)

CP-27 and CP-28 now own the Google Cloud setup and local credential upload flow. CP-05-03 only describes the Google Drive MCP backend behavior that remains relevant after those tasks.

---

## 1. Current Goal

Google Drive MCP is a runner-owned local integration. FlowPilot should let the user:

- upload the Desktop OAuth JSON through `/settings/google-drive-setup`
- verify that `npx` can launch `@piotr-agier/google-drive-mcp`
- start the MCP package's OAuth flow from the Google Drive MCP block
- create `~/.config/google-drive-mcp/tokens.json` after completing Google sign-in
- refresh MCP status after auth completes
- keep Google OAuth tokens local to the runner/MCP package

The current implementation separates three states that used to be mixed together:

- launcher/package readiness
- Desktop OAuth JSON readiness
- user OAuth token readiness

---

## 2. Implemented Code Surface

Current runner/backend code:

- `apps/local-runner/internal/runner/runner.go`
  - `mcpBackendSpecs`
  - `InstallMcpBackend`
  - `VerifyMcpBackend`
  - `StartGoogleDriveMcpAuth`
- `apps/local-runner/internal/runner/google_drive_config.go`
  - `googleDriveMcpRuntimeConfig`
  - `resolveGoogleDriveMcpStatus`
  - `googleDriveMcpCommandEnv`
  - `UploadGoogleDriveMcpOAuthCredentials`
- `apps/local-runner/internal/cli/root.go`
  - `/google-drive-config`
  - `/google-drive-config/mcp-oauth-upload`
  - `/google-drive-config/mcp-auth/start`
  - `/mcp-backends/{backend}/install`
  - `/mcp-backends/{backend}/action`

Current admin-web code:

- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- `apps/admin-web/src/app/api/runtime/google-drive-config/mcp-auth/start/route.ts`

Current tests:

- `TestInstallMcpBackendRunsExplicitCommandForGoogleDrive`
- `TestStartGoogleDriveMcpAuthLaunchesTerminalWithManagedPaths`
- Google Drive runtime config tests in `apps/local-runner/internal/runner/google_drive_config_test.go`

---

## 3. MCP Package Contract

FlowPilot uses `@piotr-agier/google-drive-mcp`.

Current contract:

- OAuth client type: Desktop app
- credential file name: `gcp-oauth.keys.json`
- credential path: `~/.config/google-drive-mcp/gcp-oauth.keys.json`
- credential env var: `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
- auth command: `npx -y @piotr-agier/google-drive-mcp auth`
- token path: `~/.config/google-drive-mcp/tokens.json`
- token env var: `GOOGLE_DRIVE_MCP_TOKEN_PATH`
- launcher/package probe: `npx -y @piotr-agier/google-drive-mcp --version`
- verify/help probe: `npx -y @piotr-agier/google-drive-mcp --help`

Important behavior:

- uploading `gcp-oauth.keys.json` does not create `tokens.json`
- `tokens.json` is created only after the MCP auth flow completes
- install/verify probes require a valid Desktop OAuth JSON path but no longer require `tokens.json`
- status remains `needs_auth` until the token file exists and passes validation

---

## 4. Current User Flow

1. User completes CP-27 Google Cloud setup.
2. User completes CP-28 local setup on `/settings/google-drive-setup`.
3. User uploads the Desktop OAuth JSON.
4. Runner copies it to `~/.config/google-drive-mcp/gcp-oauth.keys.json`.
5. User clicks `Install` in the Google Drive MCP block.
6. Runner runs the package launcher probe with:

```bash
GOOGLE_DRIVE_OAUTH_CREDENTIALS="$HOME/.config/google-drive-mcp/gcp-oauth.keys.json" \
GOOGLE_DRIVE_MCP_TOKEN_PATH="$HOME/.config/google-drive-mcp/tokens.json" \
npx -y @piotr-agier/google-drive-mcp --version
```

7. User clicks `Start Auth`.
8. Runner opens a terminal and runs:

```bash
GOOGLE_DRIVE_OAUTH_CREDENTIALS="$HOME/.config/google-drive-mcp/gcp-oauth.keys.json" \
GOOGLE_DRIVE_MCP_TOKEN_PATH="$HOME/.config/google-drive-mcp/tokens.json" \
npx -y @piotr-agier/google-drive-mcp auth
```

9. User completes Google sign-in in the browser/terminal flow.
10. MCP package creates `~/.config/google-drive-mcp/tokens.json`.
11. User clicks `Refresh MCP status`.
12. Runner refreshes MCP runtime status.

---

## 5. Current Runner Status Model

Google Drive MCP runtime status is resolved locally by the runner.

Current statuses:

- `not_started`: no meaningful local MCP setup has happened yet
- `needs_input`: Desktop OAuth JSON is missing
- `failed`: Desktop OAuth JSON exists but is invalid
- `needs_auth`: Desktop OAuth JSON is valid, but token file is missing or requires auth
- `configured`: token file exists and refresh-token validation succeeds
- `reconnect_required`: refresh token is expired, revoked, or invalid
- `warning`: token or backend status exists but cannot be fully validated

Current state ownership:

- `.flowpilot/mcp-backend-state.json` stores package/backend launcher state such as installed, last checked, and last error
- Google Drive workspace config stores the MCP credential and token paths
- the MCP package owns `tokens.json`
- Supabase must not store Google access tokens or refresh tokens

---

## 6. Current Admin-Web Behavior

The Google Drive MCP block now lives inside `/settings/google-drive-setup`.

Current buttons:

- `Install`: verifies package launcher readiness
- `Verify`: re-runs the package help/verify probe after install
- `Start Auth`: opens the interactive MCP auth flow in a new terminal
- `Refresh MCP status`: reloads runner status without launching auth

Current prerequisite:

- `Start Auth` is disabled until the uploaded Desktop OAuth JSON is valid

Expected UX detail:

- Step 5 can still show `needs_auth` after upload, because upload only saves the Desktop OAuth JSON
- status should move away from `needs_auth` only after `tokens.json` is created by the MCP auth flow

---

## 7. What Is No Longer Accurate From The Old Plan

Removed as stale:

- "exact MCP OAuth setup instructions are missing"
- "persistent runner state shape is undefined"
- "token lifecycle handling is missing"
- "admin-web does not show the Google OAuth setup path"
- "connection API should run the OAuth flow directly as part of `POST /integrations/:id/connection`"
- "install should fail until OAuth tokens already exist"

Current code already handles those differently:

- OAuth setup is in `/settings/google-drive-setup`
- CP-28 saves the Desktop OAuth JSON
- Task-025 added `Start Auth`
- runner status distinguishes missing JSON, invalid JSON, missing token, configured, reconnect-required, and warning
- launcher/package install is separate from user OAuth completion

---

## 8. Remaining Gaps

These items are still not fully proven by the current code and remain valid future work.

### 8.1 Provider CLI MCP Runtime Verification

Current verification runs package probes such as `--version` and `--help`. It does not yet prove that Claude, Codex, or Gemini can see and use the Google Drive MCP server during a normal FlowPilot prompt execution.

Remaining requirement:

- install or update provider-specific MCP config for `google-drive`
- verify the selected provider CLI can load the configured MCP server
- verify a workflow prompt that requires `google_drive` can trigger real MCP tool usage through the provider CLI
- only treat Google Drive MCP as runtime-ready for a provider after the provider-side config check passes

### 8.2 Supabase Integration Mirroring

Current runner status is local and machine-specific. The original plan mentioned mirroring visible integration status to Supabase, but the latest implemented flow mainly surfaces local runner status in the setup UI.

Remaining decision:

- decide whether Google Drive MCP should create/update a Supabase `integrations` row after local auth succeeds
- avoid storing local token material or local-only credential paths in Supabase

### 8.3 Account Metadata

The current MCP status does not persist a verified Google account email for the MCP connection.

Remaining requirement:

- if needed, obtain account metadata only after verified MCP/Google access
- keep it as display metadata, not token material

---

## 9. Current Acceptance Criteria

Already implemented:

- CP-28 can save the Desktop OAuth JSON to the runner-managed MCP path
- runner validates that the MCP OAuth JSON is a Desktop client JSON
- runner injects `GOOGLE_DRIVE_OAUTH_CREDENTIALS` and `GOOGLE_DRIVE_MCP_TOKEN_PATH` for package install/verify
- install/verify package probes no longer require a completed token file
- admin-web exposes `Start Auth` in the Google Drive MCP block
- runner opens a terminal and runs `npx -y @piotr-agier/google-drive-mcp auth`
- token file is created only after the MCP auth flow completes
- runner status distinguishes `needs_auth`, `configured`, `reconnect_required`, and `warning`
- automated runner tests cover the launcher/env behavior and auth launch command

Still required before calling Google Drive MCP fully connected:

- install/update Claude, Codex, and Gemini MCP config for the local Google Drive MCP server
- verify at least one provider CLI can load the `google-drive` MCP server during normal prompt execution
- inject strict MCP usage instructions when a workflow step declares `requiredMcps: ["google_drive"]`
- decide and implement any Supabase integration-row mirroring
- add tests for provider config generation, prompt augmentation, and provider-side MCP verification

---

## 10. Validation Checklist

Manual validation for the current implementation:

1. Complete CP-27 Google Cloud setup.
2. Upload the Desktop OAuth JSON through CP-28 Step 5.
3. Confirm `~/.config/google-drive-mcp/gcp-oauth.keys.json` exists.
4. Click `Install` in the Google Drive MCP block.
5. Confirm install moves the backend to installed/verify state.
6. Click `Start Auth`.
7. Complete Google sign-in in the opened terminal/browser flow.
8. Confirm `~/.config/google-drive-mcp/tokens.json` exists.
9. Click `Refresh MCP status`.
10. Confirm status moves away from `needs_auth`.

Failure validation:

- remove `gcp-oauth.keys.json` and confirm auth start is blocked
- upload a Web OAuth JSON and confirm validation rejects it
- remove `tokens.json` and confirm status returns to `needs_auth`
- revoke Google access and confirm status becomes `reconnect_required` or `needs_auth`
- remove Node.js/`npx` from PATH and confirm package launch reports a clear failure

---

## 11. Detailed Runtime Plan: Provider CLI Owns MCP Tool Calls

This section replaces the old direct-runner MCP client plan.

The correct FlowPilot runtime shape is:

```text
workflow step requires google_drive
-> runner validates local Google Drive MCP setup/auth
-> runner ensures selected AI provider account has google-drive MCP config
-> runner improves the workflow prompt with strict MCP usage instructions
-> runner launches Claude/Codex/Gemini using the existing provider CLI path
-> provider CLI starts/connects to google-drive-mcp from its own MCP config
-> provider CLI calls Drive MCP tools
-> provider CLI returns output to runner
```

Primary design decision:

- The AI provider CLI is the MCP client.
- The runner remains the orchestrator, setup manager, preflight checker, and prompt builder.
- The runner should not implement a Go MCP client for normal workflow execution unless provider-side MCP support is unavailable or unreliable.

Why this matches FlowPilot:

- `ExecutePrompt`, `StartSession`, and `SendMessage` already launch Codex, Claude, and Gemini through terminal-like provider adapters.
- FlowPilot already improves prompts and passes them to provider CLIs.
- Claude, Codex, and Gemini each have their own MCP configuration surface.
- Letting each provider CLI own MCP tool calls avoids duplicating MCP protocol handling in the runner.
- The runner still controls readiness, guardrails, prompt instructions, and audit artifacts.

### 11.1 Source Contract And Provider Support

Upstream Google Drive MCP package:

- GitHub: `https://github.com/piotr-agier/google-drive-mcp`
- README: `https://raw.githubusercontent.com/piotr-agier/google-drive-mcp/master/README.md`
- Runtime package: `@piotr-agier/google-drive-mcp`
- Recommended runtime command: `npx @piotr-agier/google-drive-mcp`
- Manual auth command: `npx @piotr-agier/google-drive-mcp auth`
- Default transport: stdio
- HTTP transport exists but is not needed for the first FlowPilot implementation
- Credential file: `gcp-oauth.keys.json`
- Recommended credential path: `~/.config/google-drive-mcp/gcp-oauth.keys.json`
- Token path: `~/.config/google-drive-mcp/tokens.json`
- Docker is optional and should not be the first FlowPilot path

Provider MCP support:

- Claude Code supports local stdio MCP servers through `claude mcp add ... -- <command> [args...]`, project `.mcp.json`, local/user config in `~/.claude.json`, and MCP server management commands such as `claude mcp list` and `claude mcp get`.
- Claude Code MCP tools use names like `mcp__<server-name>__<tool-name>`. For server `google-drive`, examples are `mcp__google-drive__search` and `mcp__google-drive__readGoogleDoc`.
- Claude Code CLI supports `--allowedTools`, and FlowPilot should use it for required MCP tools when running non-interactive prompts.
- Gemini CLI uses `mcpServers` in `settings.json`, with `command`, `args`, `env`, `timeout`, `trust`, `includeTools`, and `excludeTools`.
- Gemini settings can live in user config `~/.gemini/settings.json` or workspace config `.gemini/settings.json`.
- Codex supports MCP servers in `config.toml` under `[mcp_servers.<name>]`, including `command`, `args`, `env`, `startup_timeout_sec`, `tool_timeout_sec`, `enabled_tools`, `disabled_tools`, and approval modes.
- In FlowPilot account-isolated execution, provider config must be written under the account home path used by `getEnvForExecution`, not blindly under the real OS home.

### 11.2 Canonical Internal Names

Use stable internal and external names:

```text
FlowPilot internal MCP key: google_drive
Provider MCP server name: google-drive
Package command: npx
Package args: ["-y", "@piotr-agier/google-drive-mcp"]
Transport: stdio
Credential env var: GOOGLE_DRIVE_OAUTH_CREDENTIALS
Token env var: GOOGLE_DRIVE_MCP_TOKEN_PATH
```

Important naming rule:

- `requiredMcps` keeps the FlowPilot internal key: `google_drive`.
- Provider config uses the MCP server name: `google-drive`.
- Prompt instructions should mention both the requirement and the provider-visible server name.
- Claude tool names should use the provider-visible server name, for example `mcp__google-drive__search`.

Recommended internal config shape:

```json
{
  "key": "google_drive",
  "serverName": "google-drive",
  "transport": "stdio",
  "command": "npx",
  "args": ["-y", "@piotr-agier/google-drive-mcp"],
  "env": {
    "GOOGLE_DRIVE_OAUTH_CREDENTIALS": "/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json",
    "GOOGLE_DRIVE_MCP_TOKEN_PATH": "/Users/example/.config/google-drive-mcp/tokens.json"
  }
}
```

Do not use `driver` as the provider-facing name. The existing UI shell may call the section "Driver", but runtime config should use `google-drive` to avoid ambiguity.

### 11.3 Provider Config Examples

Claude Code project or local config shape:

```json
{
  "mcpServers": {
    "google-drive": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@piotr-agier/google-drive-mcp"],
      "env": {
        "GOOGLE_DRIVE_OAUTH_CREDENTIALS": "/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json",
        "GOOGLE_DRIVE_MCP_TOKEN_PATH": "/Users/example/.config/google-drive-mcp/tokens.json"
      },
      "timeout": 600000
    }
  }
}
```

Claude CLI add command:

```bash
claude mcp add --transport stdio --scope local \
  --env GOOGLE_DRIVE_OAUTH_CREDENTIALS=/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json \
  --env GOOGLE_DRIVE_MCP_TOKEN_PATH=/Users/example/.config/google-drive-mcp/tokens.json \
  google-drive -- npx -y @piotr-agier/google-drive-mcp
```

Gemini `settings.json` shape:

```json
{
  "mcpServers": {
    "google-drive": {
      "command": "npx",
      "args": ["-y", "@piotr-agier/google-drive-mcp"],
      "env": {
        "GOOGLE_DRIVE_OAUTH_CREDENTIALS": "/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json",
        "GOOGLE_DRIVE_MCP_TOKEN_PATH": "/Users/example/.config/google-drive-mcp/tokens.json"
      },
      "timeout": 600000,
      "trust": false,
      "includeTools": [
        "authGetStatus",
        "authListScopes",
        "authTestFileAccess",
        "search",
        "listFolder",
        "listSharedDrives",
        "readGoogleDoc",
        "readGoogleDocPaginated",
        "getGoogleDocContent",
        "getGoogleDocContentPaginated"
      ]
    }
  }
}
```

Codex `config.toml` shape:

```toml
[mcp_servers.google-drive]
command = "npx"
args = ["-y", "@piotr-agier/google-drive-mcp"]
startup_timeout_sec = 20
tool_timeout_sec = 120
enabled = true
enabled_tools = [
  "authGetStatus",
  "authListScopes",
  "authTestFileAccess",
  "search",
  "listFolder",
  "listSharedDrives",
  "readGoogleDoc",
  "readGoogleDocPaginated",
  "getGoogleDocContent",
  "getGoogleDocContentPaginated"
]
default_tools_approval_mode = "prompt"

[mcp_servers.google-drive.env]
GOOGLE_DRIVE_OAUTH_CREDENTIALS = "/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json"
GOOGLE_DRIVE_MCP_TOKEN_PATH = "/Users/example/.config/google-drive-mcp/tokens.json"
```

Tool allowlist policy:

- Phase A and Phase B should allow read-only and diagnostic tools only.
- Phase C can add write tools only when the workflow step explicitly allows writes.
- Permission and approval behavior is provider-specific, so the runner should configure the strictest available option per provider.

Read-only allowlist:

```text
authGetStatus
authListScopes
authTestFileAccess
search
listFolder
listSharedDrives
readGoogleDoc
readGoogleDocPaginated
getGoogleDocContent
getGoogleDocContentPaginated
```

Write tools to keep disabled until Phase C:

```text
createTextFile
updateTextFile
deleteItem
renameItem
moveItem
copyFile
createFolder
uploadFile
downloadFile
createGoogleDoc
updateGoogleDoc
addPermission
updatePermission
removePermission
shareFile
createGoogleSheet
updateGoogleSheet
createGoogleSlides
updateGoogleSlides
calendar write tools
```

### 11.4 Account Home And Config Ownership

FlowPilot provider execution is account-aware.

Current runner behavior:

- `getEnvForExecution` can override `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, `XDG_CONFIG_HOME`, and `CODEX_HOME` for provider accounts.
- Codex gets `CODEX_HOME=<accountHomePath>`.
- Claude and Gemini get `HOME=<accountHomePath>` and `XDG_CONFIG_HOME=<accountHomePath>/.config`.
- Provider auth detection already checks per-provider account homes.

Therefore provider MCP config must be written against the same account home that will execute the prompt.

Recommended config locations:

- Claude local scope:
  - use `claude mcp add --scope local` with the provider account home env
  - expected storage is under that account's `~/.claude.json`
- Claude project scope:
  - `.mcp.json` in the workspace
  - useful for team-shared config, but risky for local credential paths
- Gemini account/user scope:
  - `<accountHomePath>/.gemini/settings.json`
- Gemini workspace scope:
  - `<workspace>/.gemini/settings.json`
  - useful only if config does not contain machine-specific secrets or paths
- Codex account/user scope:
  - `<accountHomePath>/config.toml` when `CODEX_HOME=<accountHomePath>`
- Codex project scope:
  - `<workspace>/.codex/config.toml`
  - only for trusted projects and only if config can avoid local secrets

Recommended FlowPilot default:

- Write provider config into the provider account home, not the project repository.
- Keep workspace/project config optional and user-controlled.
- Do not commit `.mcp.json`, `.gemini/settings.json`, or `.codex/config.toml` if they include local credential paths.

Rationale:

- Google Drive MCP tokens are runner-local and user-account-specific.
- Project-scoped config can leak local paths and machine assumptions.
- FlowPilot already models provider accounts and account home paths.

### 11.5 New Runner Responsibilities

The runner should own four responsibilities:

1. Google Drive MCP setup/auth status.
2. Provider MCP config installation and verification.
3. Prompt augmentation for workflow steps with `requiredMcps`.
4. Runtime result/error interpretation after the provider CLI returns.

The runner should not normally:

- speak MCP JSON-RPC to Google Drive MCP for workflow execution
- directly call `tools/list` or `tools/call` during a normal workflow step
- parse Drive content itself before the provider model sees it
- act as a long-running MCP proxy

Direct runner MCP calls can still exist as a fallback diagnostic path, but they are not the primary product flow.

### 11.6 Provider Config Status Model

Add a provider config status model so FlowPilot can distinguish "Google Drive MCP is authenticated" from "provider can use it".

Suggested local status fields:

```json
{
  "backendKey": "google_drive",
  "serverName": "google-drive",
  "credentialPath": "/Users/example/.config/google-drive-mcp/gcp-oauth.keys.json",
  "tokenPath": "/Users/example/.config/google-drive-mcp/tokens.json",
  "providerConfigs": [
    {
      "providerKey": "claude",
      "accountHomePath": "/Users/example/.claudeHome1",
      "configPath": "/Users/example/.claudeHome1/.claude.json",
      "status": "configured",
      "lastCheckedAt": "2026-06-07T00:00:00Z",
      "lastError": null
    },
    {
      "providerKey": "codex",
      "accountHomePath": "/Users/example/.codexHome1",
      "configPath": "/Users/example/.codexHome1/config.toml",
      "status": "configured",
      "lastCheckedAt": "2026-06-07T00:00:00Z",
      "lastError": null
    },
    {
      "providerKey": "gemini",
      "accountHomePath": "/Users/example/.geminiHome1",
      "configPath": "/Users/example/.geminiHome1/.gemini/settings.json",
      "status": "configured",
      "lastCheckedAt": "2026-06-07T00:00:00Z",
      "lastError": null
    }
  ]
}
```

Recommended statuses:

- `not_started`: provider config was never checked
- `provider_missing`: provider CLI binary is not installed
- `account_missing`: no provider account home/auth is selected
- `config_missing`: provider config file does not include `google-drive`
- `config_stale`: provider config exists but command/env/path differs from current Google Drive MCP runtime config
- `configured`: provider config contains the expected MCP server
- `blocked_by_policy`: provider config cannot be used because provider policy blocks MCP or this server
- `failed`: runner could not read/write/validate provider config
- `unknown`: runner cannot verify the provider config format safely

### 11.7 Config Installation Strategy

Expose one runner operation:

```text
EnsureGoogleDriveMcpProviderConfig(providerKey, accountHomePath, scope)
```

Input:

```json
{
  "providerKey": "codex",
  "accountHomePath": "/Users/example/.codexHome1",
  "scope": "account",
  "mode": "read_only"
}
```

Output:

```json
{
  "providerKey": "codex",
  "serverName": "google-drive",
  "configPath": "/Users/example/.codexHome1/config.toml",
  "status": "configured",
  "changed": true,
  "lastError": null
}
```

Rules:

- Always validate Google Drive MCP runtime status first.
- Do not configure providers while Google Drive MCP is `needs_input`, `failed`, `needs_auth`, or `reconnect_required`.
- For `warning`, allow configuration but mark runtime verification as not fully proven.
- Do not overwrite unrelated provider config.
- Preserve existing provider config keys.
- If a `google-drive` server already exists and points to a different command/path, update only that server entry after validating it is safe to own.
- If a provider config file exists but is invalid JSON/TOML, return `failed` and do not rewrite it.
- Store local credential paths as absolute paths.
- Keep one canonical server name, `google-drive`.

Implementation approaches:

1. Use provider CLI commands.
   - Claude: `claude mcp add ...`
   - Codex: `codex mcp add ...`
   - Gemini: no universal add command is documented for all cases, so file edit is still needed.
   - Pros: provider handles exact format and future changes.
   - Cons: command behavior may be interactive and version-dependent.
2. Edit provider config files directly.
   - Pros: deterministic, testable, supports account-home isolation.
   - Cons: must preserve user config carefully and handle JSON/TOML parsing.
3. Hybrid.
   - Use direct file edits for Codex and Gemini.
   - Prefer `claude mcp add` for Claude local scope, with fallback to `.mcp.json`/`.claude.json` parsing only when needed.

Recommendation:

- Use hybrid for Phase A.
- Implement config file editing where the file format is stable and testable.
- Avoid relying on interactive commands for noninteractive runner paths.
- Keep manual provider CLI commands available in the UI as troubleshooting copy.

### 11.8 Provider-Specific Implementation Detail

Claude:

- Primary account-local path: `~/.claude.json` under the provider account home.
- Alternative project path: `.mcp.json` in workspace.
- Local stdio server command:
  - `npx -y @piotr-agier/google-drive-mcp`
- FlowPilot launch change:
  - when `requiredMcps` includes `google_drive`, add `--allowedTools` entries for the read-only MCP tools or update permission settings if the CLI invocation supports it better.
- Expected tool names:
  - `mcp__google-drive__authGetStatus`
  - `mcp__google-drive__search`
  - `mcp__google-drive__listFolder`
  - `mcp__google-drive__readGoogleDoc`

Gemini:

- Account-local path: `<accountHomePath>/.gemini/settings.json`.
- Workspace path: `<workspace>/.gemini/settings.json`.
- Add or merge `mcpServers.google-drive`.
- Use `includeTools` for read-only mode.
- Use `excludeTools` to block destructive/write tools if needed.
- Keep `trust: false` by default unless FlowPilot has a separate approval model.
- Add validation command later:
  - `gemini mcp list`
  - `/mcp` is interactive and less useful for automation.

Codex:

- Account-local path with FlowPilot `CODEX_HOME`: `<accountHomePath>/config.toml`.
- Project path: `<workspace>/.codex/config.toml` for trusted projects only.
- Add `[mcp_servers.google-drive]`.
- Use `enabled_tools` for read-only mode.
- Use `disabled_tools` for write/destructive tools if needed.
- Use `default_tools_approval_mode = "prompt"` for Phase A/B.
- FlowPilot launch can keep using existing Codex provider paths; Codex loads MCP config from `CODEX_HOME`/project config.

### 11.9 Config Verification Strategy

Verification has two levels.

Level 1: static config verification.

- Google Drive MCP credential JSON exists and is valid.
- Google Drive MCP token file exists and passes current validation.
- Provider CLI binary exists.
- Provider account home exists or is created.
- Provider config file contains `google-drive`.
- Provider config command/args/env match the current runtime config.
- Read-only tool allowlist is present where supported.

Level 2: provider runtime verification.

- Launch the provider CLI in a bounded test using the same account home.
- Send a test prompt that requires the provider to use `google-drive`.
- Confirm the provider reports MCP access or returns a result that proves tool use.
- Save stdout/stderr/output under `.flowpilot/mcp-tests` or a provider-config test directory.

Recommended Phase A runtime test prompt:

```text
This is a FlowPilot MCP verification task.

Use the MCP server named `google-drive`.
Call the Google Drive MCP auth/status diagnostic tool if available.
If that tool is unavailable, list up to 3 files from the Drive root folder.

Return only:
- MCP server used
- tool used
- status
- any error message

If the MCP server or tool is unavailable, say `MCP_UNAVAILABLE` and explain the exact cause.
Do not invent Drive content.
```

Why this test is better than runner-direct `tools/call`:

- It verifies the same path a workflow will use.
- It catches provider config errors.
- It catches provider permission/approval behavior.
- It catches prompt/tool-name mismatch.

### 11.10 Prompt Augmentation Contract

Mentioning `Please use MCP Drive` is not strong enough.

Required prompt augmentation should be structured, specific, and failure-aware.

Add a runner function:

```text
injectRequiredMcpInstructions(prompt, requiredMcps, providerKey, allowWrite)
```

Input:

- base improved prompt
- step `requiredMcps`
- selected provider key
- write permission state
- optional workflow step MCP intent, such as search query or document ID

Output:

- final provider prompt with a stable MCP section inserted before task-specific instructions or immediately after the task header

Recommended injected section for read-only Google Drive steps:

```markdown
## Required MCP Usage

This workflow step requires FlowPilot MCP `google_drive`.
The configured provider MCP server name is `google-drive`.

Before producing the final answer, use Google Drive MCP tools from `google-drive` when Drive context is needed for this task.

Preferred read-only tools:
- `authGetStatus` or equivalent auth/status diagnostic, when checking availability
- `search` for locating files
- `listFolder` for folder contents
- `readGoogleDoc` or paginated document read tools for Google Docs content

Rules:
- Do not invent Google Drive content.
- If `google-drive` is unavailable, stop and report `MCP_UNAVAILABLE`.
- If auth is missing or expired, stop and report `MCP_AUTH_REQUIRED`.
- If the required Drive file or folder cannot be found, report `DRIVE_CONTENT_NOT_FOUND`.
- Include the file name and file ID for every Drive item used.
- Use read-only tools only unless this step explicitly allows writes.
```

Recommended injected section when the step has a specific Drive intent:

```markdown
## Required MCP Usage

Use provider MCP server `google-drive`.
Search Google Drive for: "project roadmap".
Read the most relevant Google Doc if available.
Use only read-only Google Drive MCP tools.

Return the file names and file IDs used.
If the server, auth, or matching content is unavailable, stop and report the exact failure code.
```

Recommended failure codes:

- `MCP_UNAVAILABLE`: provider CLI cannot see the server or tools
- `MCP_AUTH_REQUIRED`: Google Drive MCP auth/token is missing or invalid
- `MCP_TOOL_BLOCKED`: provider policy or approval settings block the tool
- `MCP_TOOL_FAILED`: tool was available but returned an error
- `DRIVE_CONTENT_NOT_FOUND`: query/path/file ID did not match usable content
- `DRIVE_WRITE_NOT_ALLOWED`: task asks for write but step does not allow writes

Prompt rules:

- Always use exact server name `google-drive`.
- Prefer exact tool names when known.
- Do not rely on vague wording such as "use drive MCP".
- Ask for evidence of tool use in the output: file names, IDs, document URLs, or tool status.
- Keep the MCP section short enough that provider CLIs do not ignore it.
- If `requiredMcps` does not include `google_drive`, do not inject Google Drive MCP instructions.

### 11.11 Required MCP Preflight

Before running a step with `requiredMcps: ["google_drive"]`, the runner should preflight:

1. Google Drive MCP backend spec exists.
2. `npx` is available.
3. OAuth credential JSON exists and is valid.
4. Token file exists.
5. Token status is `configured` or recoverable.
6. Selected provider CLI is installed.
7. Selected provider account is authenticated.
8. Selected provider account has `google-drive` MCP config.
9. Provider config is not stale.

Failure behavior:

- Do not run the provider prompt if preflight fails.
- Return a clear step error.
- Point the user to `/settings/google-drive-setup` for Google Drive setup/auth problems.
- Point the user to provider setup/config for provider-specific config problems.

Preflight error mapping:

- Google credential missing:
  - `Google Drive MCP credential JSON is missing. Upload the Desktop OAuth JSON first.`
- Google auth missing:
  - `Google Drive MCP auth is incomplete. Run Start Auth, complete sign-in, then refresh status.`
- Google reconnect required:
  - `Google Drive MCP token requires reconnect. Start Auth again.`
- Provider missing:
  - `The selected AI provider CLI is not installed on this runner.`
- Provider auth missing:
  - `The selected AI provider account is not authenticated.`
- Provider config missing:
  - `The selected AI provider is not configured with the google-drive MCP server.`
- Provider config stale:
  - `The selected AI provider has stale Google Drive MCP config. Re-run Configure Providers.`

### 11.12 Workflow Runtime Behavior

When a workflow step starts:

```text
step.requiredMcps = ["google_drive"]
provider = selected step/provider model
accountHomePath = selected provider account home

runner:
  load Google Drive MCP status
  load provider config status
  fail early if required setup is missing
  inject required MCP instructions into the final prompt
  launch provider CLI through existing ExecutePrompt or session API
  collect stdout/stderr/output
  inspect output for MCP failure codes
  save actual prompt and result artifacts
```

Important distinction:

- `requiredMcps` means "this step must have provider-side MCP access".
- It does not mean "runner directly fetches context before the model runs".
- A future structured `mcpContextRequests` field can improve prompt specificity, but it should still drive provider-side tool usage first.

Recommended first runtime behavior:

- Keep workflow data model unchanged.
- Use existing `requiredMcps`.
- Inject strict MCP usage instructions.
- Add provider config preflight.
- Add optional test prompts for provider verification.

Future enhancement:

```json
{
  "requiredMcps": ["google_drive"],
  "mcpUsage": [
    {
      "provider": "google_drive",
      "mode": "read_only",
      "serverName": "google-drive",
      "intent": "search_and_read",
      "query": "project roadmap",
      "maxResults": 5,
      "requireEvidence": true
    }
  ]
}
```

The future field should describe what the provider should do, not force the runner to fetch the content itself.

### 11.13 Phase A: Provider Config And Smoke Test

Goal:

- Configure Google Drive MCP for each selected provider account.
- Verify the provider CLI can see and use `google-drive`.
- Keep all tool usage read-only.

Deliverables:

- Runner provider config installer.
- Runner provider config status endpoint or expanded Google Drive config response.
- UI action to configure providers.
- MCP test route action that runs through provider CLI, not direct runner MCP calls.
- Prompt augmentation helper.
- Tests for config generation and preflight.

Phase A completion checklist:

- [x] Runner provider config installer is implemented for Codex, Gemini, and Claude account-local config paths.
- [x] Google Drive config status response includes provider config status rows and stale/failed detection.
- [x] Google Drive setup UI exposes provider configuration status and configure actions.
- [x] `/mcp-tests` supports provider-driven Google Drive smoke tests through the selected provider CLI.
- [x] Prompt augmentation and Google Drive MCP preflight are implemented for provider verification.
- [x] Read-only tool allowlists and provider-config regression coverage were added for Phase A behavior.

Implementation steps:

1. Add provider config status types.
2. Add `EnsureGoogleDriveMcpProviderConfig`.
3. Implement provider-specific config writers:
   - Claude local/project config
   - Gemini settings JSON
   - Codex config TOML
4. Add config diff/stale detection.
5. Add read-only tool allowlist per provider where supported.
6. Add UI "Configure AI Providers" action under Google Drive MCP setup.
7. Add `/mcp-tests` provider-driven test action:
   - `google_drive_provider_auth_status`
   - `google_drive_provider_list_root`
8. Add strict verification prompt.
9. Save test artifacts under `.flowpilot/mcp-tests`.

Preferred Phase A smoke test:

```text
backendKey: google_drive
providerType: google_drive
providerKey: codex | claude | gemini
templateKey: google_drive_provider_auth_status
allowWrite: false
prompt: generated verification prompt
```

This may require extending `McpTestRequest` to include `providerKey`, `modelName`, and `accountHomePath`, because the test now verifies provider-side MCP usage.

Current `McpTestRequest` is backend-focused:

```text
backendKey
providerType
projectId
integrationId
templateKey
allowWrite
prompt
timeoutMs
```

Recommended extension:

```text
aiProviderKey
aiModelName
providerAccountId
accountHomePath
workingDirectory
```

Do not overload `providerType`; keep it as MCP provider type (`google_drive`).

### 11.14 Phase B: Workflow Prompt Runtime

Goal:

- Make real workflow steps with `requiredMcps: ["google_drive"]` use provider-side MCP tools.

Deliverables:

- Required MCP preflight in workflow run path.
- Prompt augmentation before provider execution.
- Actual prompt artifact includes MCP instructions.
- Step fails early when config/auth is missing.
- Provider output is inspected for MCP failure codes.

Runtime prompt example:

```markdown
You are executing the "Research source docs" workflow step.

## Required MCP Usage

This step requires FlowPilot MCP `google_drive`.
The configured provider MCP server name is `google-drive`.

Use Google Drive MCP tools to search Drive for files related to "Q1 roadmap".
Read the most relevant Google Doc if one exists.
Use only read-only tools.
Include file names and file IDs used.
If unavailable, stop and report the exact MCP failure code.

## Task

Summarize the source material into implementation risks and open questions.
```

Acceptance:

- Step does not run when Google Drive setup/auth is incomplete.
- Step does not run when selected provider lacks `google-drive` config.
- Step actual prompt contains required MCP section.
- Provider output includes evidence of Drive file usage or a failure code.
- Write tools remain unavailable or blocked for read-only steps.

Checklist:

- [x] Add workflow prompt preflight for `requiredMcps: ["google_drive"]`.
- [x] Inject the Phase B MCP instruction block into the actual prompt before provider execution.
- [x] Persist the actual prompt artifact with the injected MCP instructions.
- [x] Fail the step early when Google Drive auth or provider config is missing or stale.
- [x] Detect explicit MCP failure codes in provider output and mark the workflow step failed.
- [x] Keep read-only workflow steps from enabling write tools.
- [x] Add regression tests for preflight, prompt injection, failure-code detection, and workflow runtime retry behavior.

### 11.15 Phase C: Write Workflows Through Provider CLI

Goal:

- Allow selected workflow steps to create/update Google Drive artifacts through the provider CLI and Google Drive MCP.

Note:

- This phase is still required even with artifact sync in place, because artifact sync only mirrors FlowPilot-owned artifacts. Phase C is the runtime write path for AI providers like Claude Code, Gemini, and Codex to create or edit existing files in Google Drive during a workflow run.

Initial write use cases:

- Create Google Doc from workflow result.
- Update Google Doc with final deliverable.
- Create a Google Sheet for structured run output.
- Create a Slides deck from a planning/research result.

Required guardrails:

- Step must explicitly allow writes.
- Prompt augmentation must say writes are allowed and list exact target operations.
- Provider config must enable only the required write tools for that run or provider account.
- Destructive tools such as `deleteItem`, permission changes, and public sharing remain disabled until a separate approval design exists.
- Result must include created/updated file IDs and URLs.
- Runner should save write audit metadata in artifacts.

Write prompt example:

```markdown
## Required MCP Usage

This step is allowed to write to Google Drive through provider MCP server `google-drive`.

Allowed operation:
- Create one Google Doc from the final workflow output.

Not allowed:
- Delete files
- Change sharing permissions
- Move unrelated files
- Create calendar events

After the write, return the created document name, document ID, and URL.
If the write tool is unavailable or blocked, report `MCP_TOOL_BLOCKED` or `MCP_TOOL_FAILED`.
```

Write audit artifact:

```json
{
  "provider": "google_drive",
  "serverName": "google-drive",
  "aiProvider": "claude",
  "workflowRunId": "run-id",
  "stepKey": "publish_doc",
  "allowWrite": true,
  "requestedTools": ["createGoogleDoc"],
  "createdObjects": [
    {
      "type": "google_doc",
      "id": "doc-id",
      "url": "https://docs.google.com/document/d/doc-id"
    }
  ]
}
```

### 11.16 UI Changes

Google Drive setup page:

- Keep existing upload/install/start-auth/refresh behavior.
- Add a provider config panel:
  - Claude: configured/missing/stale/failed
  - Codex: configured/missing/stale/failed
  - Gemini: configured/missing/stale/failed
- Add action:
  - `Configure AI Providers`
- Add action:
  - `Verify Provider MCP`
- Show provider-specific config path and last error.
- Explain that provider CLIs call Drive MCP during workflow execution.

MCP connect test page:

- Change Google Drive section from direct MCP test to provider-driven test.
- Let user select:
  - project
  - AI provider
  - provider account
  - model
  - test action
- Test actions:
  - `Auth/status through provider`
  - `List root through provider`
  - `Search Drive through provider`
- Show actual prompt used.
- Show provider stdout/stderr.
- Show parsed failure code when present.

Workflow builder:

- Existing `requiredMcps` remains valid.
- Show `google_drive` as a required MCP.
- Warn if selected provider account is not configured for `google-drive`.
- Future UI can add structured MCP intent fields.

### 11.17 Backend/API Surface

Recommended new or expanded runner APIs:

```text
GET /google-drive-config
```

Include provider config statuses:

```json
{
  "mcp": {
    "status": "configured",
    "credentialPath": "...",
    "tokenPath": "...",
    "providerConfigs": []
  }
}
```

```text
POST /google-drive-config/mcp-provider-config/ensure
```

Request:

```json
{
  "providerKey": "codex",
  "accountHomePath": "/Users/example/.codexHome1",
  "scope": "account",
  "mode": "read_only"
}
```

Response:

```json
{
  "providerKey": "codex",
  "serverName": "google-drive",
  "status": "configured",
  "changed": true,
  "configPath": "/Users/example/.codexHome1/config.toml",
  "lastError": null
}
```

```text
POST /mcp-tests
```

Extend existing request for provider-driven Google Drive test:

```json
{
  "backendKey": "google_drive",
  "providerType": "google_drive",
  "projectId": "project-id",
  "integrationId": "local-google-drive-mcp",
  "templateKey": "google_drive_provider_auth_status",
  "allowWrite": false,
  "prompt": "generated verification prompt",
  "timeoutMs": 60000,
  "aiProviderKey": "codex",
  "aiModelName": "gpt-5-codex",
  "accountHomePath": "/Users/example/.codexHome1"
}
```

No new direct MCP `tools/call` API is required for Phase A.

### 11.18 File Checklist

Runner files:

- `apps/local-runner/internal/runner/google_drive_config.go`
  - Add provider config status aggregation.
  - Reuse credential/token/env resolution.
- `apps/local-runner/internal/runner/runner.go`
  - Add provider-driven Google Drive MCP test path.
  - Add prompt augmentation hook if prompt execution stays here.
- `apps/local-runner/internal/runner/sessions.go`
  - Add prompt augmentation for session-based workflow execution if required MCPs are passed at session message time.
  - Add Claude `--allowedTools` behavior for required MCP tools if needed.
- `apps/local-runner/internal/runner/types.go`
  - Add provider config status/request/response types.
  - Extend `McpTestRequest` only if provider-driven tests use `/mcp-tests`.
- New likely files:
  - `google_drive_mcp_provider_config.go`
  - `google_drive_mcp_provider_config_test.go`
  - `mcp_prompt_instructions.go`
  - `mcp_prompt_instructions_test.go`

CLI/API files:

- `apps/local-runner/internal/cli/root.go`
  - Add provider config ensure endpoint.
  - Keep `/mcp-tests` but support provider-driven request fields.

Admin-web files:

- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
  - Add provider config panel and actions.
- `apps/admin-web/src/routes/_authenticated/settings/mcp-servers/mcp-connect-test.tsx`
  - Make Google Drive section provider-driven.
- `apps/admin-web/src/domain/model/entity/local-runner.ts`
  - Add provider config status/request types.
  - Extend MCP test request if needed.
- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`
  - Add ensure-provider-config method.
  - Pass provider-driven test fields.
- Workflow execution/usecase files where prompts are assembled:
  - Add required MCP prompt augmentation and preflight.

### 11.19 Test Plan

Runner config tests:

- Writes Codex `config.toml` with `[mcp_servers.google-drive]`.
- Preserves unrelated Codex config.
- Updates stale Codex Google Drive config.
- Writes Gemini `settings.json` with `mcpServers.google-drive`.
- Preserves unrelated Gemini settings.
- Writes or verifies Claude local/project config.
- Fails without rewriting invalid JSON/TOML.
- Produces absolute credential/token paths.
- Adds read-only tool allowlist.
- Does not include write tools in read-only mode.

Runner preflight tests:

- Required Google Drive MCP fails when credential is missing.
- Required Google Drive MCP fails when token is missing.
- Required Google Drive MCP fails when provider config is missing.
- Required Google Drive MCP fails when provider config is stale.
- Required Google Drive MCP passes when backend auth and provider config are valid.

Prompt tests:

- No MCP instructions are injected when `requiredMcps` is empty.
- Google Drive instructions are injected when `requiredMcps` includes `google_drive`.
- Instructions include `google-drive` server name.
- Instructions include read-only rules.
- Instructions include failure codes.
- Write instructions are included only when the step explicitly allows writes.

Provider-driven MCP test tests:

- `/mcp-tests` with `google_drive_provider_auth_status` calls provider prompt execution, not direct MCP JSON-RPC.
- Test prompt includes MCP usage section.
- Result artifacts include actual prompt, stdout, stderr, and output.
- Failure code is surfaced when provider output contains `MCP_UNAVAILABLE` or `MCP_AUTH_REQUIRED`.

Admin-web tests:

- Google Drive setup page shows provider config statuses.
- Configure action calls the new runner endpoint.
- Google Drive MCP test page requires selected AI provider/account.
- Test action sends `aiProviderKey`, `aiModelName`, and `accountHomePath`.
- Disabled states distinguish Google auth missing from provider config missing.

Manual validation:

1. Upload Desktop OAuth JSON.
2. Install/verify Google Drive MCP package.
3. Start auth and confirm `tokens.json`.
4. Configure Codex provider MCP.
5. Run provider MCP test through Codex.
6. Configure Claude provider MCP.
7. Run provider MCP test through Claude.
8. Configure Gemini provider MCP.
9. Run provider MCP test through Gemini.
10. Run a workflow step with `requiredMcps: ["google_drive"]`.
11. Confirm actual prompt contains required MCP section.
12. Confirm provider output includes Drive evidence or explicit failure code.
13. Remove provider config and confirm preflight blocks the step.
14. Remove `tokens.json` and confirm preflight points to Start Auth.

### 11.20 Security And Privacy Requirements

Hard requirements:

- Supabase must not store Google access tokens.
- Supabase must not store Google refresh tokens.
- Supabase must not store MCP token file contents.
- Supabase must not store Desktop OAuth JSON contents.
- Provider config should store paths, not token contents.
- Test artifacts must not include token or credential file contents.
- Runner logs must redact token-like values.
- Read-only phases must not enable write tools where provider config supports tool filtering.
- Write phases must require explicit workflow step write permission.

Provider config safety:

- Account-local config is preferred over project config.
- Project config must not be committed if it includes local machine paths.
- Existing provider config must be preserved.
- Invalid config files must not be overwritten automatically.

Prompt safety:

- Instruct provider not to invent Drive content.
- Require file names and IDs for evidence.
- Require explicit failure codes.
- Keep write instructions separate from read-only instructions.

### 11.21 Supabase Mirroring

Recommended Phase A behavior:

- No Supabase mirroring required for provider config.
- Runner-local status is the source of truth.

Recommended Phase B behavior:

- Supabase can store visible integration metadata only if workflows need project-visible setup state.
- Store provider type, label, status, last verified timestamp, and optional account email.
- Do not store local account home paths unless there is a clear runner-machine identity model.
- Do not store secrets.

Important risk:

- A Supabase row saying `connected` can be wrong on another machine where provider config or Google tokens do not exist.

Mitigation:

- Always run local runner preflight before workflow execution.
- Treat Supabase as display/config only.

### 11.22 Definition Of Done By Phase

Phase A is done when:

- Google Drive MCP setup/auth still works.
- Runner can configure `google-drive` MCP for at least Codex account-local config.
- Runner can configure Claude and Gemini or clearly mark them unsupported/pending.
- Provider-driven MCP smoke test runs through a real provider CLI.
- Read-only tool allowlists are applied where supported.
- Test artifacts include the prompt and provider result.
- No normal workflow execution uses a direct runner MCP client.

Phase B is done when:

- Workflow step preflight checks Google Drive MCP status and selected provider MCP config.
- `requiredMcps: ["google_drive"]` injects strict MCP usage instructions.
- Provider execution sees the actual MCP server name `google-drive`.
- Step fails early when setup/auth/config is missing.
- Step output includes Drive evidence or an MCP failure code.
- Tests cover preflight, prompt injection, and provider-driven test request shape.

Phase C is done when:

- Write tools are enabled only for write-approved steps.
- Provider prompt clearly states exact allowed write operation.
- Created/updated Drive object IDs are captured.
- Destructive and permission-changing tools stay disabled unless separately approved.
- Audit artifacts record requested tools and resulting Drive objects.

### 11.23 Recommended First Slice

Build this first:

1. Add provider config status model for Google Drive MCP.
2. Implement Codex `config.toml` writer for `google-drive`.
3. Implement prompt augmentation for `requiredMcps: ["google_drive"]`.
4. Add preflight that blocks the step if Google auth or Codex MCP config is missing.
5. Add a provider-driven MCP test prompt for Codex.
6. Add UI status/action for "Configure Codex MCP".
7. Add tests for config writing, preflight, and prompt injection.

Then expand:

1. Add Gemini `settings.json` writer.
2. Add Claude config writer or `claude mcp add` integration.
3. Add provider selection in MCP test UI.
4. Add workflow-level provider config validation for all selected providers.

Do not start with a Go MCP client unless provider-side MCP verification fails and cannot be made reliable.

## 12. Test Guide For Implemented Phase A + B Cases

Use this guide to verify the behavior that is already implemented in Phase A and Phase B.

### 12.1 Prerequisites

Before testing, confirm:

- Google Cloud project setup from CP-27 is complete.
- The local Google Drive setup page can save the Desktop OAuth JSON.
- `npx` is available on the runner machine.
- The selected AI provider CLI is installed and signed in on the account you plan to test.
- The account home path used by the provider matches the path stored by FlowPilot.
- You know where each provider stores its MCP config file:
  - Codex: `config.toml`
  - Gemini: `.gemini/settings.json`
  - Claude: `.claude.json`

If any of those are missing, stop and fix setup first. Do not treat provider runtime failures as proof that Phase A or Phase B is broken unless the prerequisite checks passed.

### 12.2 Phase A Test Cases

Verify provider config and provider-driven MCP smoke testing first.

1. Open `/settings/google-drive-setup`.
2. Upload the Desktop OAuth JSON.
3. Confirm the runner stores `gcp-oauth.keys.json` under the managed MCP config path.
4. Click `Install` for Google Drive MCP.
5. Confirm the backend moves to installed or verify state.
6. Click `Verify` and confirm the package probe runs with the Google Drive MCP env vars.
7. Click `Start Auth`.
8. Complete the Google sign-in flow in the opened terminal or browser.
9. Confirm `tokens.json` exists under the managed MCP config path.
10. Click `Refresh MCP status`.
11. Confirm status changes from `needs_auth` to `configured`, `warning`, or `reconnect_required` depending on token state.
12. Click `Configure AI Providers` after MCP status is ready.
13. Confirm the runner writes the provider config file for the selected account home path:
  - Codex writes `config.toml`
  - Gemini writes `.gemini/settings.json`
  - Claude writes `.claude.json`
14. Run the provider-driven MCP smoke test for Codex, Claude, and Gemini.
15. Confirm each smoke test shows the injected prompt, provider stdout/stderr, and parsed MCP failure code or success evidence.

Expected Phase A results:

- Google Drive MCP install and verify succeed with the managed credential/token paths.
- Provider config is written under the account-local home path, not into the repo.
- The config file names match the provider:
  - Codex: `config.toml`
  - Gemini: `.gemini/settings.json`
  - Claude: `.claude.json`
- The provider-driven MCP test reaches the provider CLI, not a direct runner MCP call.
- The smoke test fails clearly if the provider account is not configured.

### 12.3 Phase B Test Cases

Verify workflow runtime behavior with `requiredMcps: ["google_drive"]`.

1. Create or open a workflow step that declares `requiredMcps: ["google_drive"]`.
2. Run the step with Google Drive auth incomplete.
3. Confirm the step stops before provider launch.
4. Confirm the error points to `/settings/google-drive-setup` and the Start Auth flow.
5. Fix Google Drive auth, then remove the provider MCP config.
6. Run the step again.
7. Confirm the step stops before provider launch and reports missing `google-drive` provider config.
8. Restore provider config and run the step normally.
9. Confirm the actual prompt artifact contains the `## Required MCP Usage` section.
10. Confirm the prompt mentions the internal key `google_drive` and the provider server name `google-drive`.
11. Confirm provider output contains file evidence or an explicit MCP failure code.
12. Confirm `actualPromptText` is persisted in the prompt execution result and artifacts.
13. Simulate provider output that returns `MCP_FAILURE_CODE: MCP_AUTH_REQUIRED`.
14. Confirm the workflow step fails.
15. Simulate provider output that only quotes the failure-code guidance.
16. Confirm the workflow step stays successful and is not downgraded by the parser.

Expected Phase B results:

- Steps fail early when auth or provider config is missing.
- The injected prompt survives into the actual prompt artifact.
- A real explicit MCP failure marker produces a failed step.
- Quoted instructions alone do not produce a false failure.

### 12.4 Retry And Recovery Cases

Verify the retry wrapper behavior after the Phase B runtime changes.

1. Trigger a `session_dead` path that represents a real transport/session loss.
2. Confirm the wrapper retries or reconnects as designed.
3. Trigger a deterministic setup error such as missing `accountHomePath` or missing Google Drive config.
4. Confirm the wrapper does not treat it as a recoverable `session_dead` event.
5. Trigger a provider reply that includes an explicit marker at the end of the final line.
6. Confirm the wrapper records the MCP failure and returns a failed result.
7. Trigger a provider reply that only mentions the marker in quoted guidance.
8. Confirm the wrapper does not misclassify the run as failed.

### 12.5 Minimal Regression Checklist

If you only have time for a short regression pass, run these cases:

- Google Drive MCP install, verify, auth, and refresh
- Provider config write for at least one provider account
- Provider-driven MCP smoke test
- Workflow step with `requiredMcps: ["google_drive"]`
- Missing auth preflight
- Missing provider config preflight
- Explicit MCP failure marker handling
- Quoted guidance false-positive check

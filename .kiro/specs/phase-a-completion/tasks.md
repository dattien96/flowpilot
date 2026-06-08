# Tasks: Phase A Completion - Google Drive MCP Provider Integration

## Overview

This document defines the implementation tasks for completing Phase A of CP-05-03 Google Drive MCP Provider Configuration. Phase A addresses 4 critical gaps to enable AI providers to use Google Drive MCP tools during workflow execution.

**Total Tasks**: 8 main tasks covering backend implementation (Tasks 1-5) and frontend integration (Tasks 6-7) plus provider verification (Task 8)

**Estimated Effort**: 3-4 days for full Phase A completion

**Key Dependencies**: Google Drive MCP backend must be installed and authenticated

---

# Implementation Plan:

## Phases and Sequence

## Implementation Plan

### Phase 1 - Backend Foundation (Tasks 1-5)
1. **Task 1**: Provider account discovery system
2. **Task 2**: Provider config status resolution
3. **Task 3**: Extend API response with provider configs
4. **Task 4**: Stale detection in preflight checks
5. **Task 5**: Expose configuration via HTTP endpoint

### Phase 2 - Frontend Integration (Tasks 6-7)
6. **Task 6**: TypeScript types and gateway methods
7. **Task 7**: Provider configuration UI card

### Phase 3 - Provider Verification (Task 8)
8. **Task 8**: Provider-driven MCP test implementation

### Implementation Sequence
- Tasks 1-3 should be completed in order (dependencies)
- Task 4 depends on Task 2
- Task 5 depends on Task 3
- Tasks 6-7 depend on Task 5
- Task 8 depends on Tasks 1-3

---

## Task Dependency Graph

```json
{
  "waves": [
    ["1"],
    ["2"],
    ["3"],
    ["4", "5", "8"],
    ["6"],
    ["7"]
  ]
}
```

---

## Notes

- All file paths are relative to repository root
- Frontend implementation assumes TypeScript 5.0+
- Backend implementation uses Go 1.21+
- All new tests must follow existing test conventions
- Error messages should be user-friendly and actionable
- Configuration operations must be idempotent
- No breaking changes to existing API contracts

---

## Tasks

---

## 1. Implement Provider Account Discovery

**Status**: Queued

**Category**: Backend

**Depends On**: (none)

**Blocked By**: (none)

**Points**: 5

### Description

Extend the provider inventory system to discover and expose provider account home paths. This enables the UI to auto-populate account selection without manual path entry.

### Acceptance Criteria

1. **Codex Discovery**: 
   - [x] Check `CODEX_HOME` environment variable
   - [~] Check default paths: `~/.codexHome`, `~/codex-accounts/*`
   - [~] Validate discovered paths by checking for `config.toml` or `.codex/` directory
   - [~] Return list of discovered account home paths

2. **Gemini Discovery**:
   - [~] Check `~/.gemini/settings.json` (user config)
   - [~] Check `GEMINI_HOME` environment variable if set
   - [~] Validate paths by checking for existing Gemini config files
   - [ ] Return list of discovered account home paths

3. **Claude Discovery**:
   - [x] Check `~/.claude.json` (user config)
   - [x] Check for multiple Claude account directories if they exist
   - [x] Validate paths by checking for existing Claude config files
   - [ ] Return list of discovered account home paths

4. **Provider Type**: Update `Provider` struct to include accounts:
   - [x] Add `Accounts []ProviderAccount` field to `Provider` type
   - [x] Add `ProviderAccount` struct with `ID`, `HomePath`, `Label` fields
   - [x] Ensure `getProviderInventory()` populates accounts for each provider

5. **API Exposure**:
   - [x] Accounts available via existing `/providers` inventory endpoint
   - [x] Each account includes `homePath` field
   - [x] Accounts are discovered at runtime (no caching)

### Implementation Notes

- Use helper function: `discoverProviderAccountHomes(providerKey string) ([]string, error)`
- Return only valid, accessible account paths
- Handle missing directories gracefully (return empty list, not error)
- Support multiple accounts per provider for future multi-account workflows
- Document default paths for each provider in code comments

### Files to Create/Modify

- `apps/local-runner/internal/runner/types.go` - Add `ProviderAccount` struct to `Provider`
- `apps/local-runner/internal/runner/provider_discovery.go` - New file with discovery logic
- `apps/local-runner/internal/runner/runner.go` - Update `getProviderInventory()` to use discovery
- `apps/admin-web/src/types/gateway.ts` - Add TypeScript `ProviderAccount` interface

### Test Requirements

Unit tests in `apps/local-runner/internal/runner/provider_discovery_test.go`:
- [~] Test Codex account discovery with CODEX_HOME set
- [~] Test Codex account discovery with default paths
- [~] Test Gemini account discovery with user config
- [~] Test Claude account discovery with user config
- [~] Test discovery with no accounts (empty list)
- [~] Test discovery with invalid/missing directories (graceful handling)
- [~] Test all providers together in single call

---

## 2. Implement Provider Config Status Resolution

**Status**: Queued

**Category**: Backend

**Depends On**: Task 1

**Blocked By**: (none)

**Points**: 8

### Description

Implement logic to check provider configuration status for each AI provider. This determines whether each provider is ready to use Google Drive MCP or needs configuration.

### Acceptance Criteria

1. **Status Resolution Function**:
   - [~] Implement `resolveGoogleDriveMcpProviderStatuses() []GoogleDriveMcpProviderConfigStatus`
   - [~] Function checks all three providers: codex, gemini, claude
   - [~] Returns status for each provider even if account not found

2. **Status Values**:
   - [~] `not_started`: Config file doesn't exist
   - [~] `configured`: Config file exists and contains google-drive server with correct paths
   - [~] `config_stale`: Config file exists but credential/token paths differ from runtime
   - [~] `failed`: Config file exists but is invalid (unparseable JSON/TOML)

3. **Stale Detection**:
   - [~] Compare credential path in provider config with runtime config
   - [~] Compare token path in provider config with runtime config
   - [~] Return `config_stale` if either path differs
   - [~] Include last checked timestamp
   - [~] Include last error message if applicable

4. **Provider Config Paths**:
   - [ ] Codex: `<accountHomePath>/config.toml`
   - [ ] Gemini: `<accountHomePath>/.gemini/settings.json`
   - [~] Claude: `<accountHomePath>/.claude.json`

5. **Config Path Parsing**:
   - [~] For Codex: Parse TOML, extract `mcp_servers.google-drive.env`
   - [ ] For Gemini: Parse JSON, extract `mcpServers.google-drive.env`
   - [~] For Claude: Parse JSON, extract `mcpServers.google-drive.env`
   - [~] Handle missing server entry (not_started)
   - [ ] Handle invalid JSON/TOML (failed)

6. **Error Handling**:
   - [ ] Log parsing errors without crashing
   - [ ] Return failed status with error message
   - [~] Continue checking other providers if one fails

### Implementation Notes

- Use existing `googleDriveMcpRuntimeConfig()` for credential/token paths
- Use existing provider discovery from Task 1
- For each provider, loop through discovered account home paths
- Store status with: `ProviderKey`, `AccountHomePath`, `ConfigPath`, `Status`, `LastCheckedAt`, `LastError`
- Timestamp should be ISO 8601 format (use `time.Now().UTC().Format(time.RFC3339)`)

### Files to Create/Modify

- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` - Add `resolveGoogleDriveMcpProviderStatuses()`
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` - Add `detectStaleConfig()` helper

### Test Requirements

Unit tests in `apps/local-runner/internal/runner/google_drive_mcp_provider_config_test.go`:
- [~] Test all providers return proper statuses
- [ ] Test not_started when config doesn't exist
- [ ] Test configured when paths match
- [ ] Test config_stale when credential path differs
- [ ] Test config_stale when token path differs
- [~] Test failed when config JSON is invalid
- [~] Test failed when config TOML is invalid
- [~] Test error message included in failed status
- [~] Test timestamp is populated for each status check

---

## 3. Add Provider Configs to Workspace Config Response

**Status**: Queued

**Category**: Backend

**Depends On**: Tasks 1-2

**Blocked By**: (none)

**Points**: 3

### Description

Extend the Google Drive workspace config API endpoint to include provider configuration statuses, enabling the UI to display provider readiness.

### Acceptance Criteria

1. **Response Type Extension**:
   - [~] Add `ProviderConfigs []GoogleDriveMcpProviderConfigStatus` field to `GoogleDriveWorkspaceConfigResponse`
   - [~] Field is populated from `resolveGoogleDriveMcpProviderStatuses()`
   - [~] Array contains all 3 providers (even if status is failed)

2. **Endpoint Integration**:
   - [~] Update `LoadGoogleDriveWorkspaceConfig()` to call status resolver
   - [~] Populate `ProviderConfigs` in response
   - [~] Handle errors gracefully (populate empty array if resolution fails)
   - [~] Maintain backward compatibility (field is optional in old clients)

3. **TypeScript Interface**:
   - [~] Extend `GoogleDriveWorkspaceConfigResponse` in `apps/admin-web/src/types/gateway.ts`
   - [~] Add `ProviderConfigs?: GoogleDriveMcpProviderConfigStatus[]` field
   - [~] Add `GoogleDriveMcpProviderConfigStatus` interface with all fields
   - [~] Include status type union: `'not_started' | 'configured' | 'config_stale' | 'failed'`

### Implementation Notes

- Endpoint: `GET /google-drive-config`
- Keep provider config resolution fast (no external calls, only file I/O)
- Resolution should complete in <1 second
- Timestamp captures when status was checked, not when config was changed

### Files to Create/Modify

- `apps/local-runner/internal/runner/types.go` - Extend `GoogleDriveWorkspaceConfigResponse`
- `apps/local-runner/internal/runner/google_drive_config.go` - Update `LoadGoogleDriveWorkspaceConfig()`
- `apps/admin-web/src/types/gateway.ts` - Add TypeScript types

### Test Requirements

Unit tests in `apps/local-runner/internal/runner/google_drive_config_test.go`:
- [~] Test LoadGoogleDriveWorkspaceConfig includes provider configs
- [~] Test response includes all 3 providers
- [~] Test response populated correctly with statuses
- [~] Test handles resolution errors gracefully

---

## 4. Implement Stale Config Detection in Preflight

**Status**: Queued

**Category**: Backend

**Depends On**: Tasks 1-2

**Blocked By**: (none)

**Points**: 2

### Description

Extend the Google Drive MCP preflight check to detect and report stale provider configurations, blocking workflow execution when config paths don't match current runtime.

### Acceptance Criteria

1. **Preflight Enhancement**:
   - [~] Update `PreflightGoogleDriveMcp()` to check for stale config
   - [~] Run after provider config existence check
   - [~] Return early with clear error if stale config detected

2. **Stale Detection**:
   - [ ] Compare credential paths: `configCredPath != runtimeConfig.CredentialPath`
   - [~] Compare token paths: `configTokenPath != runtimeConfig.TokenPath`
   - [~] Return error if either path differs

3. **Error Messages**:
   - [ ] Clear, actionable message: `"Provider has stale Google Drive MCP config. Re-run Configure Providers."`
   - [~] Include paths in debug output (not user-facing)

### Implementation Notes

- Use existing `detectStaleConfig()` helper from Task 2
- Integration point: Called before workflow execution with provider key and account home
- Fail-fast: Stop checking when stale config found
- Reuse response type: `MCPPreflightCheck`

### Files to Create/Modify

- `apps/local-runner/internal/runner/mcp_prompt_instructions.go` - Update `PreflightGoogleDriveMcp()`

### Test Requirements

Unit tests in `apps/local-runner/internal/runner/mcp_prompt_instructions_test.go`:
- [~] Test preflight passes with configured provider
- [~] Test preflight fails with not_started status
- [~] Test preflight fails with stale credential path
- [~] Test preflight fails with stale token path
- [~] Test preflight fails with failed provider config
- [ ] Test error message is populated on failure

---

## 5. Implement Provider Config Ensure Endpoint

**Status**: Queued

**Category**: Backend

**Depends On**: Tasks 1-3

**Blocked By**: (none)

**Points**: 3

### Description

Expose the existing `EnsureGoogleDriveMcpProviderConfig()` through an HTTP endpoint to allow the UI to configure providers.

### Acceptance Criteria

1. **HTTP Endpoint**:
   - [~] POST `/google-drive-config/mcp-provider-config/ensure`
   - [~] Accepts `GoogleDriveMcpProviderConfigRequest` with fields: `providerKey`, `accountHomePath`, `scope`, `mode`
   - [~] Returns `GoogleDriveMcpProviderConfigResponse` with: `providerKey`, `serverName`, `status`, `changed`, `configPath`, `lastError`

2. **Request Validation**:
   - [~] Validate `providerKey` is one of: codex, gemini, claude
   - [~] Validate `accountHomePath` is not empty and is an accessible directory
   - [ ] Default `mode` to `read_only` if not provided
   - [ ] Return 400 with clear error message on validation failure

3. **Response Status**:
   - [~] Always return HTTP 200 (even on validation errors, with error in response body)
   - [~] Include `lastError` field if operation failed
   - [ ] Include `changed` flag to indicate if config was modified

4. **Error Cases**:
   - [~] Google Drive MCP not configured: Return error message
   - [~] Invalid provider key: Return 400 with error
   - [ ] Invalid account home path: Return 400 with error
   - [ ] Config file unreadable: Return error with details
   - [~] Config file invalid (JSON/TOML): Return error without overwriting

### Implementation Notes

- Endpoint should use existing `EnsureGoogleDriveMcpProviderConfig()` function
- Add route handler in `apps/local-runner/internal/cli/root.go`
- Parse request body as JSON
- Write response as JSON
- Log all configuration attempts for auditing

### Files to Create/Modify

- `apps/local-runner/internal/cli/root.go` - Add HTTP route and handler
- `apps/local-runner/internal/runner/types.go` - Request/response types already exist

### Test Requirements

Integration tests in `apps/local-runner/internal/cli/root_test.go` or new file:
- [ ] Test successful provider configuration
- [~] Test configuration returns changed=true on first run
- [~] Test configuration returns changed=false on second run (idempotent)
- [~] Test returns error if Google Drive MCP not configured
- [~] Test returns error if invalid provider key
- [~] Test returns error if invalid account home path
- [~] Test preserves existing config for other providers
- [~] Test handles all three providers (codex, gemini, claude)

---

## 6. Extend Frontend Types and Gateway

**Status**: Queued

**Category**: Frontend

**Depends On**: Tasks 3-5

**Blocked By**: (none)

**Points**: 2

### Description

Add TypeScript types and gateway methods for provider configuration to the admin-web frontend.

### Acceptance Criteria

1. **TypeScript Types**:
   - [ ] Add `GoogleDriveMcpProviderConfigStatus` interface with all fields
   - [~] Add `GoogleDriveMcpProviderConfigRequest` interface for request
   - [~] Add `GoogleDriveMcpProviderConfigResponse` interface for response
   - [~] Extend `GoogleDriveWorkspaceConfigResponse` to include `providerConfigs?`
   - [~] Include provider account type: `ProviderAccount` with `id`, `homePath`, `label`

2. **Gateway Methods**:
   - [~] Add `ensureGoogleDriveMcpProviderConfig()` method to `HttpLocalRunnerGateway`
   - [ ] Add `refreshGoogleDriveWorkspaceConfig()` method (wraps existing GET call)
   - [ ] Both methods handle errors gracefully and type responses properly

3. **Type Safety**:
   - [~] All methods are fully typed with proper return types
   - [~] Request/response types match backend exactly
   - [ ] Status field is properly typed as union of valid statuses

### Implementation Notes

- File: `apps/admin-web/src/types/gateway.ts`
- Gateway: `apps/admin-web/src/services/HttpLocalRunnerGateway.ts`
- Keep naming consistent with backend (snake_case in JSON, camelCase in TypeScript)

### Files to Create/Modify

- `apps/admin-web/src/types/gateway.ts` - Add types
- `apps/admin-web/src/services/HttpLocalRunnerGateway.ts` - Add gateway methods

### Test Requirements

No unit tests required (TypeScript types are compile-time only). Manual verification:
- [ ] TypeScript compiler accepts all types with no errors
- [ ] IDE autocomplete shows all fields and methods
- [~] Request/response objects round-trip correctly

---

## 7. Add Provider Configuration Card to UI

**Status**: Queued

**Category**: Frontend

**Depends On**: Tasks 6

**Blocked By**: (none)

**Points**: 5

### Description

Add "AI Provider Configuration" card to the Google Drive setup page to display provider statuses and enable one-click configuration.

### Acceptance Criteria

1. **Provider Status Card**:
   - [~] Display on `/settings/google-drive-setup` page
   - [~] Title: "AI Provider Configuration"
   - [~] Subtitle: "Configure Codex, Gemini, and Claude to use Google Drive MCP"
   - [~] Located below the Google Drive MCP status card

2. **Status Badges**:
   - [~] Display badge for each provider: Codex, Gemini, Claude
   - [~] Badge colors indicate status:
     - `not_started`: Gray (unconfigured)
     - `configured`: Green (ready)
     - `config_stale`: Yellow/Orange (warning)
     - `failed`: Red (error)
   - [~] Badge shows status text and icon

3. **Provider Details**:
   - [~] Show provider name clearly
   - [~] Show account home path if available
   - [~] Show config file path when status changes
   - [~] Show error message when status is failed

4. **Configure Button**:
   - [~] Button label: "Configure AI Providers"
   - [~] Button disabled if Google Drive MCP status is not `configured`
   - [~] Tooltip: "Google Drive MCP must be configured before setting up providers"
   - [~] Clicking button initiates configuration flow

5. **Configuration Flow**:
   - [ ] Discover available provider accounts automatically
   - [ ] Show provider account selection dropdown (if multiple accounts)
   - [~] Call `ensureGoogleDriveMcpProviderConfig()` for each provider
   - [~] Show loading state during configuration
   - [~] Show success toast on completion
   - [~] Show error toast if any provider fails
   - [~] Refresh provider statuses after configuration

6. **Stale Config Handling**:
   - [~] Display warning icon for `config_stale` status
   - [~] Show tooltip: "Configuration is outdated. Click Configure to update."
   - [~] Allow user to re-configure stale config

7. **Test Button** (optional, can skip if not MVP):
   - [~] Button label: "Test Provider Connection"
   - [~] Enabled only if provider status is `configured`
   - [~] Calls MCP test endpoint with provider config
   - [~] Shows test result (success/failure)
   - [~] Shows tool name used if successful

### Implementation Notes

- Component file: `apps/admin-web/src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.tsx`
- Add component to `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- Use existing UI components from app component library
- Reuse styling from Google Drive MCP status card
- Use toast notifications for user feedback
- Implement loading states with spinners

### User Experience Flow

```
1. User sees provider config card with all statuses
2. If any status is not_started or config_stale:
   - User clicks "Configure AI Providers"
   - System discovers available provider accounts
   - System configures each provider (3 calls, ~1-2 seconds total)
   - System shows success toast
   - Statuses refresh to show "configured"
3. If user changes Google Drive MCP credential path:
   - On next status refresh, providers show "config_stale"
   - User clicks "Configure AI Providers" to update
4. If user wants to verify provider works:
   - User clicks "Test Provider Connection"
   - System runs provider-driven MCP test
   - Shows pass/fail with tool name
```

### Files to Create/Modify

- `apps/admin-web/src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.tsx` - New component
- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx` - Add card to page
- `apps/admin-web/src/services/HttpLocalRunnerGateway.ts` - Already updated in Task 6

### Test Requirements

Component tests in `apps/admin-web/src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.test.tsx`:
- [ ] Test card renders all 3 provider badges
- [ ] Test badge colors match status values
- [ ] Test Configure button disabled when Google Drive MCP not ready
- [ ] Test Configure button triggers configuration
- [ ] Test loading state during configuration
- [~] Test success toast shown on completion
- [~] Test error toast shown on failure
- [ ] Test statuses refresh after configuration
- [ ] Test stale config displays warning icon
- [ ] Test manual testing shows error message when not configured

---

## 8. Implement Provider-Driven MCP Test

**Status**: Queued

**Category**: Backend

**Depends On**: Tasks 1-3

**Blocked By**: (none)

**Points**: 8

### Description

Implement verification that AI providers can actually use Google Drive MCP tools during workflow execution, not just verify package availability.

### Acceptance Criteria

1. **Extended McpTestRequest**:
   - [~] Add field `UseProviderCLI bool` to toggle provider-driven test
   - [~] Add field `AIProviderKey string` for provider selection
   - [~] Add field `AIModelName string` for model selection
   - [~] Add field `AccountHomePath string` for provider account home
   - [~] Add field `WorkingDirectory string` for execution context

2. **Extended McpTestResult**:
   - [~] Add field `AIProviderKey string` - which provider executed the test
   - [ ] Add field `AIModelName string` - which model was used
   - [ ] Add field `McpServerName string` - MCP server name (google-drive)
   - [ ] Add field `McpToolUsed string` - first tool called by provider
   - [ ] Add field `McpFailureCode string` - failure code if any (MCP_UNAVAILABLE, etc.)

3. **Test Execution Path**:
   - [ ] When `UseProviderCLI=true`, route to provider-driven test
   - [ ] Run preflight checks (Google Drive MCP status, provider config)
   - [ ] Generate verification prompt with MCP instructions
   - [ ] Execute prompt via provider CLI (reuse ExecutePrompt logic)
   - [ ] Capture provider output (stdout, stderr, model response)

4. **Verification Prompt**:
   - [~] Inject standard MCP instructions using `InjectRequiredMcpInstructions()`
   - [~] Request provider to call `authGetStatus` or similar diagnostic tool
   - [~] Fallback to listing Drive files if diagnostic tool unavailable
   - [~] Request provider return structured response: MCP server, tool, status, error
   - [~] Explicitly forbid invented Drive content

5. **MCP Failure Code Detection**:
   - [~] Detect `MCP_UNAVAILABLE` in output (server not found)
   - [~] Detect `MCP_AUTH_REQUIRED` in output (auth/token missing)
   - [~] Detect `MCP_TOOL_BLOCKED` in output (policy blocked tool)
   - [~] Detect `MCP_TOOL_FAILED` in output (tool error)
   - [~] Detect `DRIVE_CONTENT_NOT_FOUND` in output (no Drive content)
   - [~] Populate `McpFailureCode` field if code detected
   - [~] Return status `failed` if any failure code found

6. **Artifact Storage**:
   - [~] Create directory: `.flowpilot/mcp-tests/{runID}/`
   - [~] Save `request.json` - original test request
   - [~] Save `prompt.txt` - generated prompt with MCP instructions
   - [~] Save `stdout.txt` - provider CLI stdout
   - [~] Save `stderr.txt` - provider CLI stderr
   - [~] Save `output.md` - parsed output from provider
   - [~] Save `result.json` - test result summary
   - [~] Include all artifacts in response `ArtifactPaths` array

7. **Result Determination**:
   - [~] Status `success` if: provider executed, MCP tools called, no failure codes, valid response
   - [~] Status `failed` if: provider execution failed, MCP unavailable, failure code found, invalid response
   - [~] Populate `ErrorMessage` with clear failure reason
   - [~] Include timestamps: `StartedAt`, `CompletedAt`

### Implementation Notes

- Endpoint: `POST /mcp-test` (existing, extend with new fields)
- Provider-driven tests follow same timeout behavior as normal provider execution
- Default timeout: 10 minutes for provider-driven test (respect `TimeoutMs`)
- Verification prompt should be deterministic (same each time for same provider)
- Failure codes are case-sensitive exact matches in output
- MCP failure codes are documented in `mcp_prompt_instructions.go`

### Files to Create/Modify

- `apps/local-runner/internal/runner/types.go` - Extend `McpTestRequest` and `McpTestResult`
- `apps/local-runner/internal/runner/mcp_test.go` - New file with test execution logic
- `apps/local-runner/internal/cli/root.go` - Route provider-driven tests

### Test Requirements

Integration tests in `apps/local-runner/internal/runner/mcp_test_test.go`:
- [~] Test preflight check blocks when Google Drive MCP not ready
- [~] Test preflight check blocks when provider not configured
- [~] Test prompt includes MCP instructions section
- [~] Test provider CLI execution completes successfully
- [~] Test MCP_UNAVAILABLE code detected in output
- [~] Test MCP_AUTH_REQUIRED code detected in output
- [~] Test MCP tool usage detected (tool name extracted)
- [~] Test artifacts saved to correct directory
- [~] Test result contains all required fields
- [~] Test status is success when tools work
- [~] Test status is failed when MCP codes present
- [~] Test error message populated on failure
- [~] Test idempotent (multiple runs don't interfere)

---

## Summary

### Execution Order

**Phase 1 - Backend Foundation** (Tasks 1-5):
1. Provider account discovery (Task 1)
2. Provider status resolution (Task 2)
3. Extend API response (Task 3)
4. Stale detection in preflight (Task 4)
5. Expose configuration endpoint (Task 5)

**Phase 2 - Frontend Integration** (Tasks 6-7):
6. TypeScript types and gateway (Task 6)
7. Provider configuration UI card (Task 7)

**Phase 3 - Provider Verification** (Task 8):
8. Provider-driven MCP test (Task 8) - Can start after Tasks 1-3

### Key Milestones

- **After Task 3**: Provider statuses visible via `/google-drive-config` endpoint
- **After Task 5**: Providers configurable via API endpoint
- **After Task 7**: Full UI workflow for configuring and verifying providers
- **After Task 8**: Can verify providers can actually use Google Drive MCP

### Risk Mitigation

- **Config Preservation**: Never overwrite existing provider config on errors
- **Idempotent Operations**: Safe to retry configuration multiple times
- **Fail-Fast Preflight**: Block execution early if prerequisites missing
- **Clear Error Messages**: Users know exactly what to fix
- **Comprehensive Testing**: Unit, integration, and manual test coverage

### Not Included (Future Phases)

- UI for provider-driven MCP test execution
- Write operation support (Phase C)
- Multi-tenant provider account management
- Automated config recovery from backup

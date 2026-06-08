# Requirements Document

## Introduction

This document specifies the requirements for completing Phase A of the CP-05-03 Google Drive MCP Provider Configuration system. Phase A is currently 67% complete with a solid backend foundation. This document addresses the 4 critical gaps that prevent Phase A from being truly complete per section 11.23 specification.

## Glossary

- **Runner**: The FlowPilot local-runner service that orchestrates MCP setup and workflow execution
- **Provider_CLI**: An AI provider command-line interface (Codex, Gemini, or Claude)
- **MCP_Server**: A Model Context Protocol server (google-drive in this context)
- **Provider_Config**: Provider-specific configuration files (config.toml, settings.json, .claude.json)
- **Account_Home**: The home directory path for an isolated provider account
- **Preflight_Check**: Validation performed before workflow execution to ensure MCP readiness
- **Provider_Driven_Test**: A test where the Provider_CLI calls MCP tools, not the Runner directly
- **Stale_Config**: Provider configuration that references outdated credential or token paths

## Requirements

### Requirement 1: Provider-Driven MCP Test

**User Story:** As a developer, I want to verify that AI providers can actually use Google Drive MCP tools during workflow execution, so that I can trust the integration is working correctly.

#### Acceptance Criteria

1. THE Runner SHALL extend McpTestRequest type with provider-driven fields (AIProviderKey, AIModelName, AccountHomePath, WorkingDirectory, UseProviderCLI flag)
2. WHEN UseProviderCLI is true, THE Runner SHALL generate a verification prompt using InjectRequiredMcpInstructions
3. WHEN the verification prompt is generated, THE Runner SHALL execute it via the Provider_CLI using ExecutePrompt logic
4. WHEN Provider_CLI execution completes, THE Runner SHALL capture provider-specific artifacts (actual prompt, stdout, stderr)
5. THE Runner SHALL detect MCP failure codes in Provider_CLI output (MCP_UNAVAILABLE, MCP_AUTH_REQUIRED, MCP_TOOL_BLOCKED, MCP_TOOL_FAILED, DRIVE_CONTENT_NOT_FOUND)
6. THE Runner SHALL save test artifacts under `.flowpilot/mcp-tests/{runID}/` directory
7. WHEN Provider_CLI successfully uses google-drive MCP tools, THE test SHALL return status "success"
8. IF Provider_CLI cannot access google-drive MCP tools, THEN THE test SHALL return status "failed" with clear error message

### Requirement 2: Provider Config Status Visibility

**User Story:** As a user, I want to see which AI providers are configured for Google Drive MCP, so that I know which providers are ready for workflow execution.

#### Acceptance Criteria

1. THE Runner SHALL implement resolveGoogleDriveMcpProviderStatuses() function
2. FOR EACH provider (codex, gemini, claude), THE Runner SHALL check the provider's config file for google-drive server entry
3. THE Runner SHALL return status for each provider: not_started, configured, failed, or config_stale
4. THE GoogleDriveWorkspaceConfigResponse SHALL include ProviderConfigs field containing provider status array
5. WHEN LoadGoogleDriveWorkspaceConfig() is called, THE Runner SHALL populate provider statuses
6. THE admin-web TypeScript interface SHALL extend GoogleDriveWorkspaceConfigResponse with providerConfigs field
7. THE google-drive-setup page SHALL display "AI Provider Configuration" card showing Codex, Gemini, and Claude status badges
8. THE google-drive-setup page SHALL include "Configure AI Providers" button
9. WHEN "Configure AI Providers" button is clicked, THE UI SHALL call /mcp-provider-config/ensure for each available provider
10. THE UI SHALL show success/error toasts with clear messages after configuration attempts
11. THE UI SHALL refresh provider status after configuration

### Requirement 3: Provider Account Discovery

**User Story:** As a developer, I want the system to automatically discover provider account home paths, so that the UI can configure providers without manual path entry.

#### Acceptance Criteria

1. THE Runner SHALL extend provider detection to discover account home paths
2. FOR Codex accounts, THE Runner SHALL detect CODEX_HOME environment variable or default account paths
3. FOR Gemini accounts, THE Runner SHALL detect .gemini/ config locations
4. FOR Claude accounts, THE Runner SHALL detect .claude.json locations
5. THE Provider inventory API response SHALL expose home paths for each detected provider account
6. THE HttpLocalRunnerGateway SHALL include ensureGoogleDriveMcpProviderConfig method
7. THE local-runner TypeScript types SHALL include GoogleDriveMcpProviderConfigRequest and GoogleDriveMcpProviderConfigResponse interfaces
8. THE local-runner TypeScript types SHALL include GoogleDriveMcpProviderConfigStatus interface with providerKey, accountHomePath, configPath, status, lastCheckedAt, and lastError fields

### Requirement 4: Stale Config Detection

**User Story:** As a user, I want to be notified if my provider configuration is stale, so that I can update it when credential or token paths change.

#### Acceptance Criteria

1. THE Runner SHALL implement stale detection in PreflightGoogleDriveMcp function
2. WHEN checking provider config, THE Runner SHALL compare provider config paths with current Google Drive MCP runtime config
3. IF credential path in provider config differs from current runtime credential path, THEN THE Runner SHALL mark config as stale
4. IF token path in provider config differs from current runtime token path, THEN THE Runner SHALL mark config as stale
5. THE PreflightGoogleDriveMcp SHALL return clear error message when config is stale: "Provider has stale Google Drive MCP config. Re-run Configure Providers."
6. THE resolveGoogleDriveMcpProviderStatuses SHALL detect stale config and return status "config_stale"
7. THE UI SHALL display stale config status with actionable error message directing user to reconfigure

## Parser and Serializer Requirements

N/A - This feature does not involve parsing or serialization beyond standard JSON/TOML operations already implemented.

## Special Requirements Guidance

### Provider-Driven Test Design

The provider-driven MCP test is critical because:

- Phase B workflow execution will go through Provider_CLI
- Direct Runner JSON-RPC tests don't verify the actual integration path
- Provider_CLI config errors won't be caught without provider-driven tests
- Tool name mismatches (e.g., `mcp__google-drive__search` vs `search`) must be detected

### Idempotency

All operations MUST be idempotent:
- Configuring providers twice should not create duplicate config
- Checking provider status should not modify state
- Running MCP tests should not affect provider configuration

### Account-Home Isolation

Provider account discovery MUST respect account-home isolation:
- Config files MUST be written to provider account home, not workspace
- Multiple provider accounts MUST be supported
- Default OS home MUST NOT be assumed

### Prerequisite Blocking

UI MUST enforce prerequisites:
- "Configure AI Providers" button disabled until MCP status is "configured"
- Provider-driven test blocked until provider config exists
- Clear tooltip messages explaining why actions are blocked

## Non-Functional Requirements

### Performance
- Provider status resolution MUST complete within 2 seconds
- Provider account discovery MUST complete within 5 seconds
- Provider-driven MCP test MUST respect timeout (default 10 minutes)

### Security
- Provider config files MUST store paths, not secrets
- Token files MUST remain in MCP-managed locations
- Account home paths MUST be validated before use

### Usability
- Error messages MUST be actionable
- Status badges MUST use clear visual indicators
- Button states MUST reflect current system state

### Compatibility
- MUST work on Windows, macOS, and Linux
- MUST support Codex, Gemini, and Claude providers
- MUST preserve existing provider config files

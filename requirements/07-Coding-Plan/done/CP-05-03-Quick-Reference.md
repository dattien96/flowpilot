# Google Drive MCP Provider Config - Quick Reference

## For Backend Developers

### Configure a Provider for Google Drive MCP

```go
import "flowpilot-runner/internal/runner"

// Ensure Codex is configured for Google Drive MCP
req := runner.GoogleDriveMcpProviderConfigRequest{
    ProviderKey:     "codex",
    AccountHomePath: "/Users/example/.codexHome1",
    Scope:           "account",
    Mode:            "read_only",
}

resp, err := runnerInstance.EnsureGoogleDriveMcpProviderConfig(req)
if err != nil {
    // Handle error
}

// resp.Status: "configured", "failed"
// resp.Changed: true if config was updated
// resp.ConfigPath: where config was written
```

### Add MCP Instructions to a Prompt

```go
import "flowpilot-runner/internal/runner"

// Inject MCP usage instructions
basePrompt := "Summarize the Q1 roadmap document."
requiredMcps := []string{"google_drive"}
providerKey := "codex"
allowWrite := false

augmentedPrompt := runner.InjectRequiredMcpInstructions(
    basePrompt,
    requiredMcps,
    providerKey,
    allowWrite,
)

// augmentedPrompt now includes:
// - MCP server name (google-drive)
// - Read-only tool list
// - Failure code instructions
// - Original prompt
```

### Preflight Check Before Execution

```go
import "flowpilot-runner/internal/runner"

// Check if Google Drive MCP and provider are ready
result := runnerInstance.PreflightGoogleDriveMcp("codex", accountHomePath)

if !result.GoogleDriveReady {
    // Block execution
    return fmt.Errorf(result.ErrorMessage)
}

if !result.ProviderConfigured {
    // Block execution
    return fmt.Errorf(result.ErrorMessage)
}

// Safe to proceed with provider execution
```

## For Frontend Developers

### Configure Providers API

**Endpoint**: `POST /google-drive-config/mcp-provider-config/ensure`

**Request**:
```json
{
  "providerKey": "codex",
  "accountHomePath": "/Users/example/.codexHome1",
  "scope": "account",
  "mode": "read_only"
}
```

**Response (Success)**:
```json
{
  "providerKey": "codex",
  "serverName": "google-drive",
  "status": "configured",
  "changed": true,
  "configPath": "/Users/example/.codexHome1/config.toml",
  "lastError": ""
}
```

**Response (Error)**:
```json
{
  "providerKey": "codex",
  "serverName": "google-drive",
  "status": "failed",
  "changed": false,
  "configPath": "/Users/example/.codexHome1/config.toml",
  "lastError": "Google Drive MCP must be configured and authenticated before configuring providers (current status: needs_auth)"
}
```

### Error Handling

**Common Errors**:

| Error Message | User Action |
|---------------|-------------|
| `Google Drive MCP credential JSON is missing` | Upload Desktop OAuth JSON |
| `Google Drive MCP auth is incomplete` | Click "Start Auth", complete sign-in |
| `Google Drive MCP token requires reconnect` | Click "Start Auth" again |
| `The selected AI provider is not configured` | Click "Configure AI Providers" |

## Provider Config Locations

| Provider | Config File | Location (Account Home) |
|----------|-------------|-------------------------|
| Codex | `config.toml` | `<accountHome>/config.toml` |
| Gemini | `settings.json` | `<accountHome>/.gemini/settings.json` |
| Claude | `.claude.json` | `<accountHome>/.claude.json` |

## MCP Server Details

| Property | Value |
|----------|-------|
| Internal Key | `google_drive` |
| Server Name | `google-drive` |
| Command | `npx` |
| Args | `["-y", "@piotr-agier/google-drive-mcp"]` |
| Transport | `stdio` |
| Credential Env | `GOOGLE_DRIVE_OAUTH_CREDENTIALS` |
| Token Env | `GOOGLE_DRIVE_MCP_TOKEN_PATH` |

## Read-Only Tools (Phase A)

Phase A allows only these 10 tools:

```
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

Write tools are blocked until Phase C.

## MCP Failure Codes

Instruct provider models to return these codes:

| Code | Meaning |
|------|---------|
| `MCP_UNAVAILABLE` | Provider CLI cannot see server/tools |
| `MCP_AUTH_REQUIRED` | Google Drive auth missing/expired |
| `MCP_TOOL_BLOCKED` | Provider policy blocks the tool |
| `MCP_TOOL_FAILED` | Tool available but returned error |
| `DRIVE_CONTENT_NOT_FOUND` | Query/path/ID didn't match content |
| `DRIVE_WRITE_NOT_ALLOWED` | Write attempted on read-only step |

## Workflow Integration (Phase B - Not Yet Implemented)

**Planned usage in workflow execution**:

```go
// Pseudocode for Phase B
func executeWorkflowStep(step WorkflowStep) error {
    if len(step.RequiredMcps) > 0 {
        // 1. Preflight
        preflight := runner.PreflightGoogleDriveMcp(
            step.ProviderKey,
            step.AccountHomePath,
        )
        if !preflight.GoogleDriveReady || !preflight.ProviderConfigured {
            return preflight.ErrorMessage
        }
        
        // 2. Augment prompt
        augmentedPrompt := runner.InjectRequiredMcpInstructions(
            step.Prompt,
            step.RequiredMcps,
            step.ProviderKey,
            step.AllowWrite,
        )
        
        // 3. Execute via provider CLI
        result := executeProviderCLI(augmentedPrompt)
        
        // 4. Check for MCP failure codes
        if strings.Contains(result.Output, "MCP_UNAVAILABLE") {
            return errors.New("MCP server unavailable")
        }
        
        return nil
    }
}
```

## Testing

**Run all Google Drive MCP tests**:
```bash
cd apps/local-runner
go test ./internal/runner -run "TestEnsure.*GoogleDrive|TestInjectRequiredMcp|TestPreflight" -v
```

**Test specific provider**:
```bash
go test ./internal/runner -run "TestEnsureCodexGoogleDrive" -v
go test ./internal/runner -run "TestEnsureGeminiGoogleDrive" -v
go test ./internal/runner -run "TestEnsureClaudeGoogleDrive" -v
```

**Test prompt injection**:
```bash
go test ./internal/runner -run "TestInjectRequiredMcp" -v
```

**Test preflight**:
```bash
go test ./internal/runner -run "TestPreflight" -v
```

## Example: Full Provider Config Flow

```go
package main

import (
    "fmt"
    "flowpilot-runner/internal/runner"
)

func configureCodexForGoogleDrive(r *runner.Runner, accountHome string) error {
    // 1. Check Google Drive MCP status
    gdStatus, err := r.LoadGoogleDriveWorkspaceConfig()
    if err != nil {
        return err
    }
    
    if gdStatus.MCP.Status != "configured" {
        return fmt.Errorf("Google Drive MCP not ready: %s", gdStatus.MCP.Status)
    }
    
    // 2. Configure Codex
    req := runner.GoogleDriveMcpProviderConfigRequest{
        ProviderKey:     "codex",
        AccountHomePath: accountHome,
        Scope:           "account",
        Mode:            "read_only",
    }
    
    resp, err := r.EnsureGoogleDriveMcpProviderConfig(req)
    if err != nil {
        return fmt.Errorf("failed to configure Codex: %w", err)
    }
    
    if resp.Status != "configured" {
        return fmt.Errorf("Codex config failed: %s", resp.LastError)
    }
    
    fmt.Printf("Codex configured for Google Drive MCP\n")
    fmt.Printf("Config path: %s\n", resp.ConfigPath)
    fmt.Printf("Changed: %v\n", resp.Changed)
    
    return nil
}

func runWorkflowWithMcp(r *runner.Runner, prompt string, providerKey string, accountHome string) (string, error) {
    // 3. Preflight
    preflight := r.PreflightGoogleDriveMcp(providerKey, accountHome)
    if !preflight.GoogleDriveReady || !preflight.ProviderConfigured {
        return "", fmt.Errorf("preflight failed: %s", preflight.ErrorMessage)
    }
    
    // 4. Augment prompt
    augmented := runner.InjectRequiredMcpInstructions(
        prompt,
        []string{"google_drive"},
        providerKey,
        false, // read-only
    )
    
    // 5. Execute (this would call provider CLI in real implementation)
    fmt.Printf("Would execute with prompt:\n%s\n", augmented)
    
    return "success", nil
}
```

## Troubleshooting

**Q: Config endpoint returns "needs_auth"**
- A: Google Drive MCP tokens.json is missing. Run Start Auth first.

**Q: Config file exists but status is "stale"**
- A: Credential/token paths changed. Re-run configure to update.

**Q: Provider CLI doesn't see google-drive server**
- A: Check account home path matches provider execution env.

**Q: Tests fail with "invalid TOML"**
- A: Existing config file is corrupted. Delete and reconfigure.

**Q: Prompt doesn't contain MCP instructions**
- A: Ensure `requiredMcps` includes "google_drive" (not "google-drive").

## Related Documentation

- [CP-05-03 Full Plan](./CP-05-03-Driver-Mcp.md)
- [Phase A Implementation Summary](./CP-05-03-Implementation-Phase-A.md)
- [CP-27: Google Cloud Setup](../done/CP-27-Google-Cloud-Setting-Manually.md)
- [CP-28: OAuth Upload](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)

package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectRequiredMcpInstructions_NoMcps(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{}, "codex", false)

	if result != prompt {
		t.Error("Prompt should be unchanged when no MCPs required")
	}
}

func TestInjectRequiredMcpInstructions_NoGoogleDrive(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"other_mcp"}, "codex", false)

	if result != prompt {
		t.Error("Prompt should be unchanged when google_drive is not required")
	}
}

func TestInjectRequiredMcpInstructions_GoogleDriveReadOnly(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", false)

	if !strings.Contains(result, "## Required MCP Usage") {
		t.Error("Result should contain MCP instructions header")
	}

	if !strings.Contains(result, "google-drive") {
		t.Error("Result should mention server name 'google-drive'")
	}

	if !strings.Contains(result, "authGetStatus") {
		t.Error("Result should list read-only tools")
	}

	if !strings.Contains(result, "MCP_UNAVAILABLE") {
		t.Error("Result should mention failure codes")
	}

	if !strings.Contains(result, "Use read-only tools only") {
		t.Error("Result should include read-only restriction for allowWrite=false")
	}

	if !strings.Contains(result, "Original prompt") {
		t.Error("Result should still contain original prompt")
	}
}

func TestInjectRequiredMcpInstructions_GoogleDriveWrite(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", true)

	if !strings.Contains(result, "## Required MCP Usage") {
		t.Error("Result should contain MCP instructions header")
	}

	if !strings.Contains(result, "allowed to perform read and write operations") {
		t.Error("Result should mention write operations are allowed")
	}

	if strings.Contains(result, "Use read-only tools only") {
		t.Error("Result should not restrict to read-only when allowWrite=true")
	}
}

func TestPreflightGoogleDriveMcp_NoCredential(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir}

	// Create an explicit workspace config with non-existent paths to override defaults
	flowpilotDir := filepath.Join(tmpDir, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	// Use paths that definitely don't exist
	nonExistentCredPath := filepath.Join(tmpDir, "nonexistent", "cred.json")
	nonExistentTokenPath := filepath.Join(tmpDir, "nonexistent", "token.json")

	workspaceConfig := `{
		"version": 1,
		"artifactSync": {},
		"mcp": {
			"credentialPath": "` + nonExistentCredPath + `",
			"tokenPath": "` + nonExistentTokenPath + `"
		}
	}`

	wsConfigPath := filepath.Join(flowpilotDir, "google-drive-config.json")
	if err := os.WriteFile(wsConfigPath, []byte(workspaceConfig), 0o644); err != nil {
		t.Fatalf("Failed to write workspace config: %v", err)
	}

	// Don't check provider config - just check Google Drive readiness
	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Error("Google Drive should not be ready without credentials")
	}

	if result.ErrorMessage == "" {
		t.Error("Expected an error message, got empty string")
	}

	if !strings.Contains(result.ErrorMessage, "credential") {
		t.Errorf("Expected credential-related error, got: %s", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ConfiguredWithNoProvider(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir}

	// Create valid MCP credential and token files
	mcpConfigDir := filepath.Join(tmpDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(mcpConfigDir, 0o755); err != nil {
		t.Fatalf("Failed to create MCP config dir: %v", err)
	}

	credPath := filepath.Join(mcpConfigDir, "gcp-oauth.keys.json")
	validCred := `{"installed":{"client_id":"test-id","client_secret":"test-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`
	if err := os.WriteFile(credPath, []byte(validCred), 0o600); err != nil {
		t.Fatalf("Failed to write credential: %v", err)
	}

	tokenPath := filepath.Join(mcpConfigDir, "tokens.json")
	validToken := `{"access_token":"test-access","refresh_token":"test-refresh"}`
	if err := os.WriteFile(tokenPath, []byte(validToken), 0o600); err != nil {
		t.Fatalf("Failed to write token: %v", err)
	}

	// Check without provider - should pass if Google Drive is ready
	result := runner.PreflightGoogleDriveMcp("", "")

	if !result.GoogleDriveReady {
		t.Errorf("Google Drive should be ready, error: %s", result.ErrorMessage)
	}

	if !result.ProviderConfigured {
		t.Error("Provider should be considered configured when not checked")
	}
}

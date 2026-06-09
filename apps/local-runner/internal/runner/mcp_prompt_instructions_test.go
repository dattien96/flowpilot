package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeValidGoogleDriveWorkspaceConfig(t *testing.T, workspace string) (string, string) {
	t.Helper()

	mcpConfigDir := filepath.Join(workspace, ".config", "google-drive-mcp")
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

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	wsConfig := map[string]interface{}{
		"version": 1,
		"artifactSync": map[string]interface{}{
			"clientId":    "artifact-client-id",
			"redirectUri": googleDriveDefaultRedirectURI,
		},
		"mcp": map[string]interface{}{
			"credentialPath": credPath,
			"tokenPath":      tokenPath,
		},
	}

	wsConfigBytes, err := json.Marshal(wsConfig)
	if err != nil {
		t.Fatalf("Failed to marshal workspace config: %v", err)
	}

	wsConfigPath := filepath.Join(flowpilotDir, "google-drive-config.json")
	if err := os.WriteFile(wsConfigPath, wsConfigBytes, 0o644); err != nil {
		t.Fatalf("Failed to write workspace config: %v", err)
	}

	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("Failed to save artifact sync client secret: %v", err)
	}

	return credPath, tokenPath
}

func TestInjectRequiredMcpInstructions_NoMcps(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{}, "codex", false, false)

	if result != prompt {
		t.Error("Prompt should be unchanged when no MCPs required")
	}
}

func TestInjectRequiredMcpInstructions_NoGoogleDrive(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"other_mcp"}, "codex", false, false)

	if result != prompt {
		t.Error("Prompt should be unchanged when google_drive is not required")
	}
}

func TestInjectRequiredMcpInstructions_GoogleDriveReadOnly(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", false, false)

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

	if strings.Contains(result, "Read and write MCP tool calls require provider-side approval before execution") {
		t.Error("Read-only manual instructions should not mention write approval guidance")
	}

	if !strings.Contains(result, "Original prompt") {
		t.Error("Result should still contain original prompt")
	}
}

func TestInjectRequiredMcpInstructions_GoogleDriveWrite(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", true, false)

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

func TestInjectRequiredMcpInstructions_GoogleDriveReadOnlyYolo(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", false, true)

	if !strings.Contains(result, "yolo_auto_approve") {
		t.Error("Result should mention yolo auto-approve mode")
	}

	if !strings.Contains(result, "Read tools can be called without waiting for user approval") {
		t.Error("Result should explain read-only yolo behavior")
	}

	if strings.Contains(result, "policy-allowed write tools") {
		t.Error("Read-only yolo instructions should not mention write tools")
	}
}

func TestInjectRequiredMcpInstructions_GoogleDriveWriteYolo(t *testing.T) {
	prompt := "Original prompt"
	result := InjectRequiredMcpInstructions(prompt, []string{"google_drive"}, "codex", true, true)

	if !strings.Contains(result, "yolo_auto_approve") {
		t.Error("Result should mention yolo auto-approve mode")
	}

	if !strings.Contains(result, "policy-allowed write tools can be called without waiting for user approval") {
		t.Error("Write-enabled yolo instructions should mention auto-approved write tools")
	}

	if strings.Contains(result, "Use read-only tools only") {
		t.Error("Write-enabled yolo instructions should not mention read-only restriction")
	}
}

func TestPreflightGoogleDriveMcp_NoCredential(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

	// Create an explicit workspace config with non-existent paths to override defaults
	flowpilotDir := filepath.Join(tmpDir, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	// Use paths that definitely don't exist
	nonExistentCredPath := filepath.Join(tmpDir, "nonexistent", "cred.json")
	nonExistentTokenPath := filepath.Join(tmpDir, "nonexistent", "token.json")

	// Build workspace config using JSON marshaling for proper escaping (handles Windows paths)
	wsConfig := map[string]interface{}{
		"version":      1,
		"artifactSync": map[string]interface{}{},
		"mcp": map[string]interface{}{
			"credentialPath": nonExistentCredPath,
			"tokenPath":      nonExistentTokenPath,
		},
	}

	wsConfigBytes, err := json.Marshal(wsConfig)
	if err != nil {
		t.Fatalf("Failed to marshal workspace config: %v", err)
	}

	wsConfigPath := filepath.Join(flowpilotDir, "google-drive-config.json")
	if err := os.WriteFile(wsConfigPath, wsConfigBytes, 0o644); err != nil {
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
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

	writeValidGoogleDriveWorkspaceConfig(t, tmpDir)

	// Check without provider - should pass if Google Drive is ready
	result := runner.PreflightGoogleDriveMcp("", "")

	if !result.GoogleDriveReady {
		t.Errorf("Google Drive should be ready, error: %s", result.ErrorMessage)
	}

	if !result.ProviderConfigured {
		t.Error("Provider should be considered configured when not checked")
	}
}

func TestPreparePromptForRequiredMcps_RequiresAccountHomePath(t *testing.T) {
	runner := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	_, err := runner.preparePromptForRequiredMcps(
		"Original prompt",
		[]string{"google_drive"},
		"codex",
		"",
		false,
		false,
	)
	if err == nil {
		t.Fatal("expected missing accountHomePath error")
	}
	if !strings.Contains(err.Error(), "accountHomePath is required") {
		t.Fatalf("expected accountHomePath error, got %v", err)
	}
}

func TestPreparePromptForRequiredMcps_InjectsAfterSuccessfulPreflight(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeValidGoogleDriveWorkspaceConfig(t, workspace)

	accountHomePath := t.TempDir()
	_, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHomePath,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("ensure provider config: %v", err)
	}

	result, err := runner.preparePromptForRequiredMcps(
		"Original prompt",
		[]string{"google_drive"},
		"codex",
		accountHomePath,
		false,
		false,
	)
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}

	if !strings.Contains(result, "## Required MCP Usage") {
		t.Fatal("expected injected MCP section")
	}
	if !strings.Contains(result, "google-drive") {
		t.Fatal("expected injected server name")
	}
	if !strings.Contains(result, "Original prompt") {
		t.Fatal("expected original prompt to remain in output")
	}
}

func TestApplyRequiredMcpFailureStatus_IgnoresQuotedFailureGuidance(t *testing.T) {
	result := PromptExecutionResult{
		Status:         "success",
		OutputMarkdown: "Drive search succeeded.\nQuoted guidance: If auth fails later, return `MCP_AUTH_REQUIRED`.",
	}

	applyRequiredMcpFailureStatus(&result, []string{"google_drive"})

	if result.Status != "success" {
		t.Fatalf("expected status to remain success, got %q", result.Status)
	}
	if result.ErrorMessage != "" {
		t.Fatalf("expected empty error message, got %q", result.ErrorMessage)
	}
}

func TestApplyRequiredMcpFailureStatus_IgnoresQuotedExplicitMarkerGuidance(t *testing.T) {
	result := PromptExecutionResult{
		Status:         "success",
		OutputMarkdown: "Drive search succeeded.\nQuoted guidance: end the response with `MCP_FAILURE_CODE: MCP_AUTH_REQUIRED`.",
	}

	applyRequiredMcpFailureStatus(&result, []string{"google_drive"})

	if result.Status != "success" {
		t.Fatalf("expected status to remain success, got %q", result.Status)
	}
	if result.ErrorMessage != "" {
		t.Fatalf("expected empty error message, got %q", result.ErrorMessage)
	}
}

func TestApplyRequiredMcpFailureStatus_DetectsExplicitMarkerAfterExplanation(t *testing.T) {
	result := PromptExecutionResult{
		Status:         "success",
		OutputMarkdown: "I could not authenticate with Google Drive. MCP_FAILURE_CODE: MCP_AUTH_REQUIRED",
	}

	applyRequiredMcpFailureStatus(&result, []string{"google_drive"})

	if result.Status != "failed" {
		t.Fatalf("expected status to be failed, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "mcp_auth_required") {
		t.Fatalf("expected MCP failure code in error message, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_NeedsAuthWithoutToken(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	_, tokenPath := writeValidGoogleDriveWorkspaceConfig(t, workspace)
	if err := os.Remove(tokenPath); err != nil {
		t.Fatalf("remove token file: %v", err)
	}

	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Fatal("expected Google Drive to require auth when token is missing")
	}
	if !strings.Contains(result.ErrorMessage, "auth is incomplete") {
		t.Fatalf("expected needs_auth error, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ReconnectRequired(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeValidGoogleDriveWorkspaceConfig(t, workspace)

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		return 400, []byte(`{"error":"invalid_grant","error_description":"token revoked"}`), nil
	}

	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Fatal("expected Google Drive to require reconnect when refresh token is revoked")
	}
	if !strings.Contains(result.ErrorMessage, "requires reconnect") {
		t.Fatalf("expected reconnect error, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ConfigStale(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	_, tokenPath := writeValidGoogleDriveWorkspaceConfig(t, workspace)

	accountHomePath := t.TempDir()
	_, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHomePath,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("ensure provider config: %v", err)
	}

	configPath := filepath.Join(accountHomePath, "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read provider config: %v", err)
	}
	staleTokenPath := tokenPath + ".stale"
	updated := strings.Replace(string(raw), tokenPath, staleTokenPath, 1)
	if updated == string(raw) {
		t.Fatal("expected to replace token path in provider config")
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write stale provider config: %v", err)
	}

	result := runner.PreflightGoogleDriveMcp("codex", accountHomePath)

	if result.ProviderConfigured {
		t.Fatal("expected stale provider config to fail preflight")
	}
	if !strings.Contains(result.ErrorMessage, "stale Google Drive MCP config") {
		t.Fatalf("expected stale config error, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ProxyPathDoesNotRequireLegacyDesktopMcpAuth(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeArtifactSyncOnlyPreflightConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")

	result := runner.PreflightGoogleDriveMcp("", "")

	if !result.GoogleDriveReady {
		t.Fatalf("expected proxy path to be ready with artifact-sync auth only, got error: %s", result.ErrorMessage)
	}
	if !result.ProviderConfigured {
		t.Fatal("expected providerConfigured to remain true when no provider config is requested")
	}
}

func TestPreflightGoogleDriveMcp_ProxyPathFailsWithoutArtifactConnection(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeArtifactSyncOnlyPreflightConfig(t, runner, workspace)

	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Fatal("expected proxy preflight to fail when no artifact-sync connection exists")
	}
	if !strings.Contains(result.ErrorMessage, "no artifact-sync Google Drive connection") {
		t.Fatalf("expected missing connection error, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ProxyPathFailsWithMultipleArtifactConnections(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeArtifactSyncOnlyPreflightConfig(t, runner, workspace)

	if err := runner.saveGoogleDriveCredentialByProject("project-1", googleDriveCredential{
		RefreshToken: "refresh-1",
		AccountEmail: "project-1@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject(project-1) failed: %v", err)
	}
	if err := runner.saveGoogleDriveCredentialByProject("project-2", googleDriveCredential{
		RefreshToken: "refresh-2",
		AccountEmail: "project-2@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject(project-2) failed: %v", err)
	}
	if err := runner.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			Status:       "connected",
			FolderID:     "folder-1",
			AccountEmail: "project-1@example.com",
		}
		current.Connections["project-2"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-2",
			Status:       "connected",
			FolderID:     "folder-2",
			AccountEmail: "project-2@example.com",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Fatal("expected proxy preflight to fail when multiple artifact-sync connections exist")
	}
	if !strings.Contains(result.ErrorMessage, "multiple artifact-sync Google Drive connections") {
		t.Fatalf("expected multiple connection error, got %q", result.ErrorMessage)
	}
}

func TestPreflightGoogleDriveMcp_ProxyPathFailsWithoutStoredRefreshToken(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeArtifactSyncOnlyPreflightConfig(t, runner, workspace)

	if err := runner.ensureSecretStore().Set(
		googleDriveProjectCredentialKey("project-1"),
		`{"accountEmail":"project-1@example.com"}`,
	); err != nil {
		t.Fatalf("failed to store incomplete credential: %v", err)
	}
	if err := runner.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			Status:       "connected",
			FolderID:     "folder-1",
			AccountEmail: "project-1@example.com",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	httpCalled := false
	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		httpCalled = true
		return 500, nil, nil
	}

	result := runner.PreflightGoogleDriveMcp("", "")

	if result.GoogleDriveReady {
		t.Fatal("expected proxy preflight to fail when refresh token is missing")
	}
	if !strings.Contains(result.ErrorMessage, "refresh token is not configured") {
		t.Fatalf("expected missing refresh token error, got %q", result.ErrorMessage)
	}
	if httpCalled {
		t.Fatal("expected proxy preflight to validate locally without refreshing tokens")
	}
}

func writeArtifactSyncOnlyPreflightConfig(t *testing.T, runner *Runner, workspace string) {
	t.Helper()

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	wsConfig := map[string]interface{}{
		"version": 1,
		"artifactSync": map[string]interface{}{
			"clientId":    "artifact-client-id",
			"redirectUri": googleDriveDefaultRedirectURI,
		},
		"mcp": map[string]interface{}{},
	}

	wsConfigBytes, err := json.Marshal(wsConfig)
	if err != nil {
		t.Fatalf("Failed to marshal workspace config: %v", err)
	}

	wsConfigPath := filepath.Join(flowpilotDir, "google-drive-config.json")
	if err := os.WriteFile(wsConfigPath, wsConfigBytes, 0o644); err != nil {
		t.Fatalf("Failed to write workspace config: %v", err)
	}

	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("Failed to save artifact sync client secret: %v", err)
	}
}

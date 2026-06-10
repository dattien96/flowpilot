package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestEnsureCodexGoogleDriveMcpConfig(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	accountHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	// Setup runner with valid Google Drive MCP
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

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

	// Create workspace config to specify MCP paths
	flowpilotDir := filepath.Join(tmpDir, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	// Build workspace config using JSON marshaling for proper escaping (handles Windows paths)
	wsConfig := map[string]interface{}{
		"version":      1,
		"artifactSync": map[string]interface{}{},
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

	// Configure Codex
	req := GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	}

	resp, err := runner.EnsureGoogleDriveMcpProviderConfig(req)
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
	}

	if resp.Status != "configured" {
		t.Errorf("Expected status 'configured', got '%s'", resp.Status)
	}

	if !resp.Changed {
		t.Error("Expected Changed to be true for new config")
	}

	if resp.ServerName != "google-drive" {
		t.Errorf("Expected serverName 'google-drive', got '%s'", resp.ServerName)
	}

	// Verify config file was created
	configPath := filepath.Join(accountHome, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Codex config.toml was not created")
	}

	// Parse and verify config content
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config codexConfig
	if err := toml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("Failed to parse TOML: %v", err)
	}

	server, exists := config.McpServers["google-drive"]
	if !exists {
		t.Fatal("google-drive server not found in config")
	}

	if server.Command != "npx" {
		t.Errorf("Expected command 'npx', got '%s'", server.Command)
	}

	if len(server.Args) != 2 || server.Args[0] != "-y" || server.Args[1] != "@piotr-agier/google-drive-mcp" {
		t.Errorf("Unexpected args: %v", server.Args)
	}

	if server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != credPath {
		t.Errorf("Wrong credential path in env")
	}

	if server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != tokenPath {
		t.Errorf("Wrong token path in env")
	}

	// Verify read-only tools are configured
	if len(server.EnabledTools) == 0 {
		t.Error("No enabled tools configured for read_only mode")
	}
	if server.ApprovalMode != "approve" {
		t.Errorf("Expected read_only approval mode 'approve', got '%s'", server.ApprovalMode)
	}

	// Run again to verify idempotency
	resp2, err := runner.EnsureGoogleDriveMcpProviderConfig(req)
	if err != nil {
		t.Fatalf("Second run failed: %v", err)
	}

	if resp2.Changed {
		t.Error("Expected Changed to be false for unchanged config")
	}
}

func TestEnsureGeminiGoogleDriveMcpConfig(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	accountHome := filepath.Join(tmpDir, "gemini-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	// Setup runner with valid Google Drive MCP
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

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

	// Create workspace config to specify MCP paths
	flowpilotDir := filepath.Join(tmpDir, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	// Build workspace config using JSON marshaling for proper escaping (handles Windows paths)
	wsConfig := map[string]interface{}{
		"version":      1,
		"artifactSync": map[string]interface{}{},
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

	// Configure Gemini
	req := GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "gemini",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	}

	resp, err := runner.EnsureGoogleDriveMcpProviderConfig(req)
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
	}

	if resp.Status != "configured" {
		t.Errorf("Expected status 'configured', got '%s'", resp.Status)
	}

	// Verify config file was created
	configPath := filepath.Join(accountHome, ".gemini", "settings.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Gemini settings.json was not created")
	}

	// Parse and verify config content
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config geminiSettings
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	server, exists := config.McpServers["google-drive"]
	if !exists {
		t.Fatal("google-drive server not found in config")
	}

	if server.Command != "npx" {
		t.Errorf("Expected command 'npx', got '%s'", server.Command)
	}

	// Verify includeTools for read-only mode
	if len(server.IncludeTools) == 0 {
		t.Error("No includeTools configured for read_only mode")
	}
}

func TestEnsureClaudeGoogleDriveMcpConfig(t *testing.T) {
	// Setup temp directory
	tmpDir := t.TempDir()
	accountHome := filepath.Join(tmpDir, "claude-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	// Setup runner with valid Google Drive MCP
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

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

	// Create workspace config to specify MCP paths
	flowpilotDir := filepath.Join(tmpDir, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot dir: %v", err)
	}

	// Build workspace config using JSON marshaling for proper escaping (handles Windows paths)
	wsConfig := map[string]interface{}{
		"version":      1,
		"artifactSync": map[string]interface{}{},
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

	// Configure Claude
	req := GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	}

	resp, err := runner.EnsureGoogleDriveMcpProviderConfig(req)
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
	}

	if resp.Status != "configured" {
		t.Errorf("Expected status 'configured', got '%s'", resp.Status)
	}

	// Verify config file was created
	configPath := filepath.Join(accountHome, ".claude.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Claude .claude.json was not created")
	}

	// Parse and verify config content
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	server, exists := config.McpServers["google-drive"]
	if !exists {
		t.Fatal("google-drive server not found in config")
	}

	if server.Type != "stdio" {
		t.Errorf("Expected type 'stdio', got '%s'", server.Type)
	}
}

func TestEnsureCodexGoogleDriveMcpConfig_ProxyPathUsesYoloApproval(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	accountHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	t.Setenv(googleDriveProxyMcpFlag, "true")

	testCases := []struct {
		name     string
		yoloMode bool
	}{
		{name: "manual", yoloMode: false},
		{name: "yolo", yoloMode: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
				ProviderKey:       "codex",
				AccountHomePath:   accountHome,
				Scope:             "account",
				Mode:              "read_write",
				YoloMode:          tc.yoloMode,
				WorkflowRunID:     "run-123",
				WorkflowStepRunID: "step-456",
				ProcessKey:        "proc-789",
			})
			if err != nil {
				t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
			}
			if resp.Status != "configured" {
				t.Fatalf("expected configured response, got %q", resp.Status)
			}

			raw, err := os.ReadFile(filepath.Join(accountHome, "config.toml"))
			if err != nil {
				t.Fatalf("Failed to read config: %v", err)
			}

			var config codexConfig
			if err := toml.Unmarshal(raw, &config); err != nil {
				t.Fatalf("Failed to parse TOML: %v", err)
			}

			server, exists := config.McpServers[googleDriveMcpServerName]
			if !exists {
				t.Fatal("google-drive server not found in config")
			}

			expectedArgs := googleDriveProxyMcpArgs(tmpDir, accountHome, "read_write", tc.yoloMode)
			if len(server.Args) != len(expectedArgs) {
				t.Fatalf("unexpected args length: got %v want %v", server.Args, expectedArgs)
			}
			for i := range expectedArgs {
				if server.Args[i] != expectedArgs[i] {
					t.Fatalf("unexpected args: got %v want %v", server.Args, expectedArgs)
				}
			}

			expectedApproval := googleDriveProxyMcpApprovalMode(tc.yoloMode)
			if server.ApprovalMode != expectedApproval {
				t.Fatalf("unexpected approval mode: got %q want %q", server.ApprovalMode, expectedApproval)
			}
			expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
			if server.Command != expectedCommand {
				t.Fatalf("unexpected command: got %q want %q", server.Command, expectedCommand)
			}
			assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
			if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
				t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
			}
			if got := server.Env[googleDriveProxyWorkflowRunIDEnv]; got != "run-123" {
				t.Fatalf("expected workflow run id env, got %q", got)
			}
			if got := server.Env[googleDriveProxyWorkflowStepIDEnv]; got != "step-456" {
				t.Fatalf("expected workflow step run id env, got %q", got)
			}
			if got := server.Env[googleDriveProxyProcessKeyEnv]; got != "proc-789" {
				t.Fatalf("expected process key env, got %q", got)
			}
		})
	}
}

func TestEnsureCodexGoogleDriveMcpConfig_ProxyPathInjectsSelectedAccountID(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)
	t.Setenv(googleDriveProxyMcpFlag, "true")

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	runtimeStatus, err := runner.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("LoadGoogleDriveWorkspaceConfig() failed: %v", err)
	}
	if len(runtimeStatus.Accounts) == 0 {
		t.Fatal("expected at least one Google Drive account to be discovered")
	}
	selectedAccountID := runtimeStatus.Accounts[0].AccountID

	if _, err := runner.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		MCPAccountID: selectedAccountID,
	}); err != nil {
		t.Fatalf("SaveGoogleDriveWorkspaceConfig() failed: %v", err)
	}

	accountHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
	}
	if resp.Status != "configured" {
		t.Fatalf("expected configured response, got %q", resp.Status)
	}

	raw, err := os.ReadFile(filepath.Join(accountHome, "config.toml"))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	var config codexConfig
	if err := toml.Unmarshal(raw, &config); err != nil {
		t.Fatalf("Failed to parse TOML: %v", err)
	}

	server := config.McpServers[googleDriveMcpServerName]
	if got := strings.TrimSpace(server.Env[googleDriveProxyAccountIDEnv]); got != selectedAccountID {
		t.Fatalf("expected selected account id env to be injected, got %q", got)
	}
	if got := strings.TrimSpace(server.Env[googleDriveClientIDEnv]); got != "artifact-client-id" {
		t.Fatalf("expected Google Drive client id env to be injected, got %q", got)
	}
	if got := strings.TrimSpace(server.Env[googleDriveClientSecretEnv]); got != "artifact-client-secret" {
		t.Fatalf("expected Google Drive client secret env to be injected, got %q", got)
	}
	if got := strings.TrimSpace(server.Env[googleDriveProxyRefreshTokenEnv]); got != "artifact-refresh-token" {
		t.Fatalf("expected Google Drive refresh token env to be injected, got %q", got)
	}
}

func TestGoogleDriveProxyMcpServerEnvIncludesApprovalScope(t *testing.T) {
	env := googleDriveProxyMcpServerEnv(googleDriveMcpRuntimeConfig{
		AccountID:         "account-1",
		WorkflowRunID:     "run-123",
		WorkflowStepRunID: "step-456",
		ProcessKey:        "proc-789",
		ProxyClientID:     "client-id",
		ProxyClientSecret: "client-secret",
		ProxyRefreshToken: "refresh-token",
	})

	expected := map[string]string{
		googleDriveProxyAccountIDEnv:      "account-1",
		googleDriveProxyWorkflowRunIDEnv:  "run-123",
		googleDriveProxyWorkflowStepIDEnv: "step-456",
		googleDriveProxyProcessKeyEnv:     "proc-789",
		googleDriveClientIDEnv:            "client-id",
		googleDriveClientSecretEnv:        "client-secret",
		googleDriveProxyRefreshTokenEnv:   "refresh-token",
	}
	for key, want := range expected {
		if got := env[key]; got != want {
			t.Fatalf("expected env[%s] = %q, got %q", key, want, got)
		}
	}
}

func TestEnvMatchesAllowsRuntimeApprovalScopeExtras(t *testing.T) {
	expected := map[string]string{
		googleDriveProxyAccountIDEnv: "account-1",
		googleDriveClientIDEnv:       "client-id",
	}
	existing := map[string]string{
		googleDriveProxyAccountIDEnv:      "account-1",
		googleDriveClientIDEnv:            "client-id",
		googleDriveProxyWorkflowRunIDEnv:  "run-123",
		googleDriveProxyWorkflowStepIDEnv: "step-456",
		googleDriveProxyProcessKeyEnv:     "proc-789",
	}

	if !envMatches(existing, expected) {
		t.Fatal("expected runtime approval scope env extras to be ignored")
	}
}

func TestEnvMatchesRejectsMismatchedExpectedApprovalScope(t *testing.T) {
	expected := map[string]string{
		googleDriveProxyProcessKeyEnv: "proc-new",
	}
	existing := map[string]string{
		googleDriveProxyProcessKeyEnv: "proc-old",
	}

	if envMatches(existing, expected) {
		t.Fatal("expected mismatched approval scope env to be rejected")
	}
}

func TestEnvMatchesRejectsUnknownExtraEnv(t *testing.T) {
	expected := map[string]string{
		googleDriveProxyAccountIDEnv: "account-1",
	}
	existing := map[string]string{
		googleDriveProxyAccountIDEnv: "account-1",
		"FLOWPILOT_UNKNOWN":          "value",
	}

	if envMatches(existing, expected) {
		t.Fatal("expected unknown extra env to be rejected")
	}
}

func TestEnsureGeminiAndClaudeGoogleDriveMcpConfig_ProxyPathIncludesAccountHome(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	t.Setenv(googleDriveProxyMcpFlag, "true")

	testCases := []struct {
		name         string
		providerKey  string
		accountHome  string
		configPathFn func(string) string
	}{
		{name: "gemini", providerKey: "gemini", accountHome: filepath.Join(tmpDir, "gemini-home"), configPathFn: func(home string) string { return filepath.Join(home, ".gemini", "settings.json") }},
		{name: "claude", providerKey: "claude", accountHome: filepath.Join(tmpDir, "claude-home"), configPathFn: func(home string) string { return filepath.Join(home, ".claude.json") }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.accountHome, 0o755); err != nil {
				t.Fatalf("Failed to create account home: %v", err)
			}

			resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
				ProviderKey:     tc.providerKey,
				AccountHomePath: tc.accountHome,
				Scope:           "account",
				Mode:            "read_only",
				YoloMode:        true,
			})
			if err != nil {
				t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
			}
			if resp.Status != "configured" {
				t.Fatalf("expected configured response, got %q", resp.Status)
			}

			raw, err := os.ReadFile(tc.configPathFn(tc.accountHome))
			if err != nil {
				t.Fatalf("Failed to read config: %v", err)
			}

			if tc.providerKey == "gemini" {
				var config geminiSettings
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("Failed to parse JSON: %v", err)
				}
				server, exists := config.McpServers[googleDriveMcpServerName]
				if !exists {
					t.Fatal("google-drive server not found in config")
				}
				expectedArgs := googleDriveProxyMcpArgs(tmpDir, tc.accountHome, "read_only", true)
				if len(server.Args) != len(expectedArgs) {
					t.Fatalf("unexpected args length: got %v want %v", server.Args, expectedArgs)
				}
				for i := range expectedArgs {
					if server.Args[i] != expectedArgs[i] {
						t.Fatalf("unexpected args: got %v want %v", server.Args, expectedArgs)
					}
				}
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("unexpected command: got %q want %q", server.Command, expectedCommand)
				}
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				if len(server.IncludeTools) != len(googleDriveMcpReadOnlyTools) {
					t.Fatalf("expected read_only tools, got %v", server.IncludeTools)
				}
				if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
					t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
				}
				return
			}

			var config claudeConfig
			if err := json.Unmarshal(raw, &config); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}
			server, exists := config.McpServers[googleDriveMcpServerName]
			if !exists {
				t.Fatal("google-drive server not found in config")
			}
			expectedArgs := googleDriveProxyMcpArgs(tmpDir, tc.accountHome, "read_only", true)
			if len(server.Args) != len(expectedArgs) {
				t.Fatalf("unexpected args length: got %v want %v", server.Args, expectedArgs)
			}
			for i := range expectedArgs {
				if server.Args[i] != expectedArgs[i] {
					t.Fatalf("unexpected args: got %v want %v", server.Args, expectedArgs)
				}
			}
			expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
			if server.Command != expectedCommand {
				t.Fatalf("unexpected command: got %q want %q", server.Command, expectedCommand)
			}
			assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
			if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
				t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
			}
		})
	}
}

func TestEnsureGoogleDriveMcpProviderConfig_ProxyConfigsIncludeSharedAccountHome(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	testCases := []struct {
		name        string
		providerKey string
		accountHome string
	}{
		{
			name:        "codex",
			providerKey: "codex",
			accountHome: filepath.Join(tmpDir, "codex-home"),
		},
		{
			name:        "gemini",
			providerKey: "gemini",
			accountHome: filepath.Join(tmpDir, "gemini-home"),
		},
		{
			name:        "claude",
			providerKey: "claude",
			accountHome: filepath.Join(tmpDir, "claude-home"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.MkdirAll(tc.accountHome, 0o755); err != nil {
				t.Fatalf("Failed to create account home: %v", err)
			}

			resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
				ProviderKey:     tc.providerKey,
				AccountHomePath: tc.accountHome,
				Scope:           "account",
				Mode:            "read_only",
				YoloMode:        false,
			})
			if err != nil {
				t.Fatalf("EnsureGoogleDriveMcpProviderConfig(%s) failed: %v", tc.providerKey, err)
			}
			if resp.Status != "configured" {
				t.Fatalf("expected configured status for %s, got %q", tc.providerKey, resp.Status)
			}

			var raw []byte
			var readErr error
			switch tc.providerKey {
			case "codex":
				raw, readErr = os.ReadFile(filepath.Join(tc.accountHome, "config.toml"))
			case "gemini":
				raw, readErr = os.ReadFile(filepath.Join(tc.accountHome, ".gemini", "settings.json"))
			case "claude":
				raw, readErr = os.ReadFile(filepath.Join(tc.accountHome, ".claude.json"))
			}
			if readErr != nil {
				t.Fatalf("Failed to read %s config: %v", tc.providerKey, readErr)
			}

			expectedArgs := googleDriveProxyMcpArgs(tmpDir, tc.accountHome, "read_only", false)
			switch tc.providerKey {
			case "codex":
				var config codexConfig
				if err := toml.Unmarshal(raw, &config); err != nil {
					t.Fatalf("Failed to parse Codex TOML: %v", err)
				}
				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
					t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
				}
				if server.ApprovalMode != "approve" {
					t.Fatalf("expected approve approval for proxy path, got %q", server.ApprovalMode)
				}
			case "gemini":
				var config geminiSettings
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("Failed to parse Gemini JSON: %v", err)
				}
				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
					t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
				}
			case "claude":
				var config claudeConfig
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("Failed to parse Claude JSON: %v", err)
				}
				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if _, ok := server.Env["FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"]; ok {
					t.Fatalf("expected proxy env flag to be omitted, got %#v", server.Env)
				}
			}
		})
	}
}

func TestEnsureGoogleDriveMcpProviderConfig_ProxyCodexApprovalAndToolSurfaceFollowInputs(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	tmpDir := t.TempDir()
	accountHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	testCases := []struct {
		name         string
		mode         string
		yoloMode     bool
		wantApproval string
		wantTools    []string
	}{
		{
			name:         "read_only_approve",
			mode:         "read_only",
			yoloMode:     false,
			wantApproval: "approve",
			wantTools:    googleDriveMcpReadOnlyTools,
		},
		{
			name:         "read_write_approve",
			mode:         "read_write",
			yoloMode:     true,
			wantApproval: "approve",
			wantTools:    googleDriveMcpReadWriteTools,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
				ProviderKey:     "codex",
				AccountHomePath: accountHome,
				Scope:           "account",
				Mode:            tc.mode,
				YoloMode:        tc.yoloMode,
			})
			if err != nil {
				t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
			}
			if resp.Status != "configured" {
				t.Fatalf("expected configured status, got %q", resp.Status)
			}

			raw, err := os.ReadFile(filepath.Join(accountHome, "config.toml"))
			if err != nil {
				t.Fatalf("Failed to read Codex config: %v", err)
			}

			var config codexConfig
			if err := toml.Unmarshal(raw, &config); err != nil {
				t.Fatalf("Failed to parse TOML: %v", err)
			}

			server := config.McpServers[googleDriveMcpServerName]
			expectedArgs := googleDriveProxyMcpArgs(tmpDir, accountHome, tc.mode, tc.yoloMode)
			assertStringSliceEqual(t, server.Args, expectedArgs)
			if server.ApprovalMode != tc.wantApproval {
				t.Fatalf("expected approval %q, got %q", tc.wantApproval, server.ApprovalMode)
			}
			if !toolListsMatch(server.EnabledTools, tc.wantTools) {
				t.Fatalf("expected tools %v, got %v", tc.wantTools, server.EnabledTools)
			}
		})
	}
}

func TestEnsureGoogleDriveMcpProviderConfig_ProxyPathDoesNotRequireLegacyDesktopMcpAuth(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	tmpDir := t.TempDir()
	accountHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(accountHome, 0o755); err != nil {
		t.Fatalf("Failed to create account home: %v", err)
	}

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	writeArtifactSyncOnlyGoogleDriveWorkspaceConfig(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	resp, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig failed: %v", err)
	}
	if resp.Status != "configured" {
		t.Fatalf("expected configured status, got %q", resp.Status)
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_WithProxyFieldDriftedConfig(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)
	mcpStatus := googleDriveMcpRuntimeConfig{
		CredentialPath: credPath,
		TokenPath:      tokenPath,
	}

	codexHome := filepath.Join(tmpDir, ".codexHome")
	codexServer := expectedCodexGoogleDriveMcpServer(tmpDir, codexHome, "read_write", true, mcpStatus)
	codexServer.ApprovalMode = "prompt"
	codexConfigBytes, err := toml.Marshal(codexConfig{
		McpServers: map[string]codexMcpServer{
			googleDriveMcpServerName: codexServer,
		},
	})
	if err != nil {
		t.Fatalf("toml.Marshal(codex proxy config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(codexHome, "config.toml"), string(codexConfigBytes))

	geminiServer := expectedGeminiGoogleDriveMcpServer(tmpDir, tmpDir, "read_only", false, mcpStatus)
	geminiServer.Args[4] = filepath.Join(tmpDir, "wrong-gemini-home")
	geminiConfigBytes, err := json.Marshal(geminiSettings{
		McpServers: map[string]geminiMcpServer{
			googleDriveMcpServerName: geminiServer,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(gemini proxy config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(tmpDir, ".gemini", "settings.json"), string(geminiConfigBytes))

	claudeServer := expectedClaudeGoogleDriveMcpServer(tmpDir, tmpDir, "read_only", true, mcpStatus)
	claudeServer.Args[4] = filepath.Join(tmpDir, "wrong-claude-home")
	claudeConfigBytes, err := json.Marshal(claudeConfig{
		McpServers: map[string]claudeMcpServer{
			googleDriveMcpServerName: claudeServer,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(claude proxy config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(tmpDir, ".claude.json"), string(claudeConfigBytes))

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	for _, providerKey := range []string{"codex", "gemini", "claude"} {
		status := findProviderConfigStatus(t, statuses, providerKey)
		if status.Status != "config_stale" {
			t.Fatalf("expected %s to be config_stale for proxy drift, got %q", providerKey, status.Status)
		}
	}
}

func TestEnsureGoogleDriveMcpProviderConfig_RewritesInvalidExistingConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	stubGoogleDriveOAuthTokenRefresh(t)

	testCases := []struct {
		name         string
		providerKey  string
		accountHome  string
		configPath   string
		invalidBody  string
		validateFile func(*testing.T, string, string, string)
	}{
		{
			name:        "codex",
			providerKey: "codex",
			accountHome: filepath.Join(tmpDir, ".codexHome"),
			configPath:  filepath.Join(tmpDir, ".codexHome", "config.toml"),
			invalidBody: "not = [valid",
			validateFile: func(t *testing.T, configPath string, expectedCred string, expectedToken string) {
				t.Helper()
				raw, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("ReadFile(%q) failed: %v", configPath, err)
				}

				var config codexConfig
				if err := toml.Unmarshal(raw, &config); err != nil {
					t.Fatalf("expected rewritten codex config to be valid TOML: %v", err)
				}

				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				expectedArgs := googleDriveProxyMcpArgs(tmpDir, filepath.Join(tmpDir, ".codexHome"), "read_only", false)
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if server.ApprovalMode != "approve" {
					t.Fatalf("expected proxy approval mode 'approve', got %q", server.ApprovalMode)
				}
				if server.Env[googleDriveProxyAccountIDEnv] == "" {
					t.Fatalf("expected account id env to be populated, got %#v", server.Env)
				}
			},
		},
		{
			name:        "gemini",
			providerKey: "gemini",
			accountHome: filepath.Join(tmpDir, "gemini-home"),
			configPath:  filepath.Join(tmpDir, "gemini-home", ".gemini", "settings.json"),
			invalidBody: "{not-json",
			validateFile: func(t *testing.T, configPath string, expectedCred string, expectedToken string) {
				t.Helper()
				raw, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("ReadFile(%q) failed: %v", configPath, err)
				}

				var config geminiSettings
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("expected rewritten gemini config to be valid JSON: %v", err)
				}

				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				expectedArgs := googleDriveProxyMcpArgs(tmpDir, filepath.Join(tmpDir, "gemini-home"), "read_only", false)
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if server.Env[googleDriveProxyAccountIDEnv] == "" {
					t.Fatalf("expected account id env to be populated, got %#v", server.Env)
				}
			},
		},
		{
			name:        "claude",
			providerKey: "claude",
			accountHome: filepath.Join(tmpDir, "claude-home"),
			configPath:  filepath.Join(tmpDir, "claude-home", ".claude.json"),
			invalidBody: "{not-json",
			validateFile: func(t *testing.T, configPath string, expectedCred string, expectedToken string) {
				t.Helper()
				raw, err := os.ReadFile(configPath)
				if err != nil {
					t.Fatalf("ReadFile(%q) failed: %v", configPath, err)
				}

				var config claudeConfig
				if err := json.Unmarshal(raw, &config); err != nil {
					t.Fatalf("expected rewritten claude config to be valid JSON: %v", err)
				}

				server := config.McpServers[googleDriveMcpServerName]
				expectedCommand, expectedPrefix := googleDriveProxyMcpCommand(tmpDir)
				if server.Command != expectedCommand {
					t.Fatalf("expected command %q, got %q", expectedCommand, server.Command)
				}
				expectedArgs := googleDriveProxyMcpArgs(tmpDir, filepath.Join(tmpDir, "claude-home"), "read_only", false)
				assertStringSliceEqual(t, server.Args[:len(expectedPrefix)], expectedPrefix)
				assertStringSliceEqual(t, server.Args, expectedArgs)
				if server.Env[googleDriveProxyAccountIDEnv] == "" {
					t.Fatalf("expected account id env to be populated, got %#v", server.Env)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mustWriteTestFile(t, tc.configPath, tc.invalidBody)

			response, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
				ProviderKey:     tc.providerKey,
				AccountHomePath: tc.accountHome,
				Scope:           "account",
				Mode:            "read_only",
			})
			if err != nil {
				t.Fatalf("EnsureGoogleDriveMcpProviderConfig(%q) failed: %v", tc.providerKey, err)
			}

			if response.Status != "configured" {
				t.Fatalf("expected configured status for %s, got %q", tc.providerKey, response.Status)
			}
			if !response.Changed {
				t.Fatalf("expected rewritten config for %s to be marked changed", tc.providerKey)
			}

			tc.validateFile(t, tc.configPath, credPath, tokenPath)
		})
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted(t *testing.T) {
	// Setup temp directory with no provider configs
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)
	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}

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

	// Resolve statuses
	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses failed: %v", err)
	}

	// Should have 3 providers
	if len(statuses) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(statuses))
	}

	// All should be not_started because no provider accounts/configs exist
	for _, status := range statuses {
		if status.Status != "not_started" {
			t.Errorf("Expected status 'not_started' for %s, got '%s'", status.ProviderKey, status.Status)
		}
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_WithConfiguredProvider(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	mustWriteTestFile(t, filepath.Join(tmpDir, ".claude.json"), `{"version":"1"}`)

	_, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: tmpDir,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig() failed: %v", err)
	}

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	claudeStatus := findProviderConfigStatus(t, statuses, "claude")
	if claudeStatus.Status != "configured" {
		t.Fatalf("expected claude to be configured, got %q", claudeStatus.Status)
	}
	if claudeStatus.AccountHomePath != tmpDir {
		t.Fatalf("expected claude account home %q, got %q", tmpDir, claudeStatus.AccountHomePath)
	}
	if claudeStatus.ConfigPath != filepath.Join(tmpDir, ".claude.json") {
		t.Fatalf("expected claude config path to be populated, got %q", claudeStatus.ConfigPath)
	}
	if claudeStatus.LastCheckedAt == "" {
		t.Fatal("expected LastCheckedAt to be populated")
	}
}

func TestGoogleDriveProxyMcpCommandWithLookup(t *testing.T) {
	tmpDir := t.TempDir()
	runnerDir := filepath.Join(tmpDir, "apps", "local-runner")
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		t.Fatalf("Failed to create runner dir: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(runnerDir, "go.mod"), "module flowpilot-runner\n")

	t.Run("prefers flowpilot binary", func(t *testing.T) {
		command, args := googleDriveProxyMcpCommandWithLookup(tmpDir, func(bin string) (string, error) {
			if bin == "flowpilot" {
				return "/usr/local/bin/flowpilot", nil
			}
			if bin == "go" {
				return "/usr/local/go/bin/go", nil
			}
			return "", os.ErrNotExist
		})

		if command != "flowpilot" {
			t.Fatalf("expected flowpilot command, got %q", command)
		}
		if len(args) != 0 {
			t.Fatalf("expected no command prefix args, got %v", args)
		}
	})

	t.Run("falls back to go run in dev workspace", func(t *testing.T) {
		command, args := googleDriveProxyMcpCommandWithLookup(tmpDir, func(bin string) (string, error) {
			if bin == "go" {
				return "/usr/local/go/bin/go", nil
			}
			return "", os.ErrNotExist
		})

		if command != "go" {
			t.Fatalf("expected go command, got %q", command)
		}
		assertStringSliceEqual(t, args, []string{"-C", runnerDir, "run", "./cmd/flowpilot"})
	})
}

func TestParseGoogleDriveProxyMcpInvocation_AcceptsGoRunFallback(t *testing.T) {
	workspace, accountHomePath, mode, yoloMode, ok := parseGoogleDriveProxyMcpInvocation("go", []string{
		"-C",
		"/tmp/workspace/apps/local-runner",
		"run",
		"./cmd/flowpilot",
		"google-drive-mcp",
		"--workspace",
		"/tmp/workspace",
		"--account-home",
		"/tmp/account",
		"--mode",
		"read_write",
		"--yolo-mode",
	})
	if !ok {
		t.Fatal("expected go run proxy invocation to parse")
	}
	if workspace != "/tmp/workspace" {
		t.Fatalf("unexpected workspace: %q", workspace)
	}
	if accountHomePath != "/tmp/account" {
		t.Fatalf("unexpected account home: %q", accountHomePath)
	}
	if mode != "read_write" {
		t.Fatalf("unexpected mode: %q", mode)
	}
	if !yoloMode {
		t.Fatal("expected yolo mode to be true")
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_WithStaleConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	accountHome := filepath.Join(tmpDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(accountHome, "config.toml"), `[core]
version = "1.0"`)

	_, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHome,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig() failed: %v", err)
	}

	newMcpDir := filepath.Join(tmpDir, ".config2", "google-drive-mcp")
	if err := os.MkdirAll(newMcpDir, 0o755); err != nil {
		t.Fatalf("Failed to create alternate MCP config dir: %v", err)
	}

	newCredPath := filepath.Join(newMcpDir, "gcp-oauth.keys.json")
	validCred := `{"installed":{"client_id":"test-id","client_secret":"test-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`
	if err := os.WriteFile(newCredPath, []byte(validCred), 0o600); err != nil {
		t.Fatalf("Failed to write alternate credential: %v", err)
	}

	newTokenPath := filepath.Join(newMcpDir, "tokens.json")
	validToken := `{"access_token":"test-access","refresh_token":"test-refresh"}`
	if err := os.WriteFile(newTokenPath, []byte(validToken), 0o600); err != nil {
		t.Fatalf("Failed to write alternate token: %v", err)
	}

	writeTestGoogleDriveWorkspaceConfig(t, runner, tmpDir, newCredPath, newTokenPath)

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	codexStatus := findProviderConfigStatus(t, statuses, "codex")
	if codexStatus.Status != "config_stale" {
		t.Fatalf("expected codex to be config_stale, got %q", codexStatus.Status)
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_WithShapeDriftedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
	mcpStatus := googleDriveMcpRuntimeConfig{
		CredentialPath: credPath,
		TokenPath:      tokenPath,
	}

	codexHome := filepath.Join(tmpDir, ".codexHome")
	codexServer := expectedCodexGoogleDriveMcpServer(tmpDir, filepath.Join(tmpDir, ".codexHome"), googleDriveMcpStatusMode, false, mcpStatus)
	codexServer.EnabledTools = []string{"search"}
	codexConfigBytes, err := toml.Marshal(codexConfig{
		McpServers: map[string]codexMcpServer{
			googleDriveMcpServerName: codexServer,
		},
	})
	if err != nil {
		t.Fatalf("toml.Marshal(codex config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(codexHome, "config.toml"), string(codexConfigBytes))

	geminiServer := expectedGeminiGoogleDriveMcpServer(tmpDir, "", googleDriveMcpStatusMode, false, mcpStatus)
	geminiServer.Command = "node"
	geminiConfigBytes, err := json.Marshal(geminiSettings{
		McpServers: map[string]geminiMcpServer{
			googleDriveMcpServerName: geminiServer,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(gemini config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(tmpDir, ".gemini", "settings.json"), string(geminiConfigBytes))

	claudeServer := expectedClaudeGoogleDriveMcpServer(tmpDir, "", googleDriveMcpStatusMode, false, mcpStatus)
	claudeServer.Type = "sse"
	claudeConfigBytes, err := json.Marshal(claudeConfig{
		McpServers: map[string]claudeMcpServer{
			googleDriveMcpServerName: claudeServer,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(claude config) failed: %v", err)
	}
	mustWriteTestFile(t, filepath.Join(tmpDir, ".claude.json"), string(claudeConfigBytes))

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	for _, providerKey := range []string{"codex", "gemini", "claude"} {
		status := findProviderConfigStatus(t, statuses, providerKey)
		if status.Status != "config_stale" {
			t.Fatalf("expected %s to be config_stale for shape drift, got %q", providerKey, status.Status)
		}
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_WithFieldDriftedConfig(t *testing.T) {
	testCases := []struct {
		name        string
		providerKey string
		writeConfig func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig)
	}{
		{
			name:        "codex startup timeout drift",
			providerKey: "codex",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedCodexGoogleDriveMcpServer(tmpDir, filepath.Join(tmpDir, ".codexHome"), googleDriveMcpStatusMode, false, mcpStatus)
				server.StartupTimeoutSec++
				raw, err := toml.Marshal(codexConfig{
					McpServers: map[string]codexMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("toml.Marshal(codex startup timeout drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".codexHome", "config.toml"), string(raw))
			},
		},
		{
			name:        "codex tool timeout drift",
			providerKey: "codex",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedCodexGoogleDriveMcpServer(tmpDir, filepath.Join(tmpDir, ".codexHome"), googleDriveMcpStatusMode, false, mcpStatus)
				server.ToolTimeoutSec++
				raw, err := toml.Marshal(codexConfig{
					McpServers: map[string]codexMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("toml.Marshal(codex tool timeout drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".codexHome", "config.toml"), string(raw))
			},
		},
		{
			name:        "codex enabled drift",
			providerKey: "codex",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedCodexGoogleDriveMcpServer(tmpDir, filepath.Join(tmpDir, ".codexHome"), googleDriveMcpStatusMode, false, mcpStatus)
				server.Enabled = false
				raw, err := toml.Marshal(codexConfig{
					McpServers: map[string]codexMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("toml.Marshal(codex enabled drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".codexHome", "config.toml"), string(raw))
			},
		},
		{
			name:        "codex approval mode drift",
			providerKey: "codex",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedCodexGoogleDriveMcpServer(tmpDir, filepath.Join(tmpDir, ".codexHome"), googleDriveMcpStatusMode, false, mcpStatus)
				server.ApprovalMode = "never"
				raw, err := toml.Marshal(codexConfig{
					McpServers: map[string]codexMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("toml.Marshal(codex approval mode drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".codexHome", "config.toml"), string(raw))
			},
		},
		{
			name:        "gemini timeout drift",
			providerKey: "gemini",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedGeminiGoogleDriveMcpServer(tmpDir, "", googleDriveMcpStatusMode, false, mcpStatus)
				server.Timeout++
				raw, err := json.Marshal(geminiSettings{
					McpServers: map[string]geminiMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("json.Marshal(gemini timeout drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".gemini", "settings.json"), string(raw))
			},
		},
		{
			name:        "gemini trust drift",
			providerKey: "gemini",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedGeminiGoogleDriveMcpServer(tmpDir, "", googleDriveMcpStatusMode, false, mcpStatus)
				server.Trust = true
				raw, err := json.Marshal(geminiSettings{
					McpServers: map[string]geminiMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("json.Marshal(gemini trust drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".gemini", "settings.json"), string(raw))
			},
		},
		{
			name:        "claude timeout drift",
			providerKey: "claude",
			writeConfig: func(t *testing.T, tmpDir string, mcpStatus googleDriveMcpRuntimeConfig) {
				server := expectedClaudeGoogleDriveMcpServer(tmpDir, "", googleDriveMcpStatusMode, false, mcpStatus)
				server.Timeout++
				raw, err := json.Marshal(claudeConfig{
					McpServers: map[string]claudeMcpServer{googleDriveMcpServerName: server},
				})
				if err != nil {
					t.Fatalf("json.Marshal(claude timeout drift) failed: %v", err)
				}
				mustWriteTestFile(t, filepath.Join(tmpDir, ".claude.json"), string(raw))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			setDiscoveryTestHome(t, tmpDir)

			runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
			_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)
			mcpStatus := googleDriveMcpRuntimeConfig{
				CredentialPath: credPath,
				TokenPath:      tokenPath,
			}

			tc.writeConfig(t, tmpDir, mcpStatus)

			statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
			if err != nil {
				t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
			}

			status := findProviderConfigStatus(t, statuses, tc.providerKey)
			if status.Status != "config_stale" {
				t.Fatalf("expected %s to be config_stale for %s, got %q", tc.providerKey, tc.name, status.Status)
			}
		})
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_ReturnsMultipleAccountsPerProvider(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)

	defaultCodexHome := filepath.Join(tmpDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(defaultCodexHome, "config.toml"), `[core]
version = "1.0"`)

	secondCodexHome := filepath.Join(tmpDir, ".codexHome1")
	mustWriteTestFile(t, filepath.Join(secondCodexHome, "config.toml"), `[core]
version = "1.0"`)

	_, err := runner.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: defaultCodexHome,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("EnsureGoogleDriveMcpProviderConfig() for default home failed: %v", err)
	}

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	codexStatuses := make([]GoogleDriveMcpProviderConfigStatus, 0)
	for _, status := range statuses {
		if status.ProviderKey == "codex" {
			codexStatuses = append(codexStatuses, status)
		}
	}

	if len(codexStatuses) != 2 {
		t.Fatalf("expected 2 codex statuses, got %d: %#v", len(codexStatuses), codexStatuses)
	}

	foundConfigured := false
	foundNotStarted := false
	for _, status := range codexStatuses {
		switch status.AccountHomePath {
		case defaultCodexHome:
			foundConfigured = status.Status == "configured"
		case secondCodexHome:
			foundNotStarted = status.Status == "not_started"
		}
	}

	if !foundConfigured {
		t.Fatalf("expected configured status for %s", defaultCodexHome)
	}
	if !foundNotStarted {
		t.Fatalf("expected not_started status for %s", secondCodexHome)
	}
}

func TestResolveGoogleDriveMcpProviderStatuses_ManagedSlotWithoutAuthIsNotStarted(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir, secretStore: newMemorySecretStore()}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, runner, tmpDir)

	managedCodexHome := filepath.Join(tmpDir, ".codexHome1")
	if err := os.MkdirAll(managedCodexHome, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", managedCodexHome, err)
	}

	statuses, err := runner.resolveGoogleDriveMcpProviderStatuses()
	if err != nil {
		t.Fatalf("resolveGoogleDriveMcpProviderStatuses() failed: %v", err)
	}

	foundManagedHome := false
	for _, status := range statuses {
		if status.ProviderKey == "codex" && status.AccountHomePath == managedCodexHome {
			foundManagedHome = true
			if status.Status != "not_started" {
				t.Fatalf("expected managed codex slot to be not_started, got %q", status.Status)
			}
		}
	}

	if !foundManagedHome {
		t.Fatalf("expected managed codex slot %q in provider statuses: %#v", managedCodexHome, statuses)
	}
}

func writeTestGoogleDriveMcpRuntime(t *testing.T, runner *Runner, workspace string) (string, string, string) {
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

	writeTestGoogleDriveWorkspaceConfig(t, runner, workspace, credPath, tokenPath)
	return mcpConfigDir, credPath, tokenPath
}

func writeTestGoogleDriveWorkspaceConfig(t *testing.T, runner *Runner, workspace string, credPath string, tokenPath string) {
	t.Helper()
	stubGoogleDriveProxyLauncherAvailable(t)

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot settings dir: %v", err)
	}

	wsConfig := googleDriveWorkspaceConfigFile{
		Version: 1,
		ArtifactSync: googleDriveWorkspaceArtifactConfig{
			ClientID:    "artifact-client-id",
			RedirectURI: googleDriveDefaultRedirectURI,
		},
		MCP: googleDriveWorkspaceMcpConfig{
			CredentialPath: credPath,
			TokenPath:      tokenPath,
		},
	}

	wsConfigBytes, err := json.MarshalIndent(wsConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal workspace config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowpilotDir, "google-drive-config.json"), wsConfigBytes, 0o644); err != nil {
		t.Fatalf("Failed to write workspace config: %v", err)
	}

	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("Failed to save artifact sync client secret: %v", err)
	}
	writeSingleProxyArtifactConnection(t, runner, "project-1")
}

func writeArtifactSyncOnlyGoogleDriveWorkspaceConfig(t *testing.T, runner *Runner, workspace string) {
	t.Helper()
	stubGoogleDriveProxyLauncherAvailable(t)

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot settings dir: %v", err)
	}

	wsConfig := googleDriveWorkspaceConfigFile{
		Version: 1,
		ArtifactSync: googleDriveWorkspaceArtifactConfig{
			ClientID:    "artifact-client-id",
			RedirectURI: googleDriveDefaultRedirectURI,
		},
		MCP: googleDriveWorkspaceMcpConfig{},
	}

	wsConfigBytes, err := json.MarshalIndent(wsConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal workspace config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowpilotDir, "google-drive-config.json"), wsConfigBytes, 0o644); err != nil {
		t.Fatalf("Failed to write workspace config: %v", err)
	}

	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("Failed to save artifact sync client secret: %v", err)
	}
	writeSingleProxyArtifactConnection(t, runner, "project-1")
}

func stubGoogleDriveOAuthTokenRefresh(t *testing.T) {
	t.Helper()

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(_ context.Context, _ string, endpoint string, _ map[string]string, _ []byte) (int, []byte, error) {
		if endpoint != "https://oauth2.googleapis.com/token" {
			return 500, []byte(`unexpected Google Drive OAuth request`), nil
		}
		return 200, []byte(`{"access_token":"test-access-token"}`), nil
	}
}

func stubGoogleDriveProxyLauncherAvailable(t *testing.T) {
	t.Helper()

	originalLookPath := lookPathFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
	})
	lookPathFn = func(file string) (string, error) {
		if file == "flowpilot" {
			return "/usr/local/bin/flowpilot", nil
		}
		return "", os.ErrNotExist
	}
}

func findProviderConfigStatus(
	t *testing.T,
	statuses []GoogleDriveMcpProviderConfigStatus,
	providerKey string,
) GoogleDriveMcpProviderConfigStatus {
	t.Helper()

	for _, status := range statuses {
		if status.ProviderKey == providerKey {
			return status
		}
	}

	t.Fatalf("provider status %q not found", providerKey)
	return GoogleDriveMcpProviderConfigStatus{}
}

func assertStringSliceEqual(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected args length: got %d want %d; got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected args at index %d: got %q want %q; got=%v want=%v", i, got[i], want[i], got, want)
		}
	}
}

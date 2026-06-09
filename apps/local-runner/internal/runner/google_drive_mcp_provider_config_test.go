package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestEnsureGoogleDriveMcpProviderConfig_RewritesInvalidExistingConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	runner := &Runner{workspace: tmpDir}
	_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, tmpDir)

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
				if server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expectedCred {
					t.Fatalf("expected credential path %q, got %q", expectedCred, server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"])
				}
				if server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expectedToken {
					t.Fatalf("expected token path %q, got %q", expectedToken, server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"])
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
				if server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expectedCred {
					t.Fatalf("expected credential path %q, got %q", expectedCred, server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"])
				}
				if server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expectedToken {
					t.Fatalf("expected token path %q, got %q", expectedToken, server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"])
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
				if server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expectedCred {
					t.Fatalf("expected credential path %q, got %q", expectedCred, server.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"])
				}
				if server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expectedToken {
					t.Fatalf("expected token path %q, got %q", expectedToken, server.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"])
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

	runner := &Runner{workspace: tmpDir}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, tmpDir)
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

func TestResolveGoogleDriveMcpProviderStatuses_WithStaleConfig(t *testing.T) {
	tmpDir := t.TempDir()
	setDiscoveryTestHome(t, tmpDir)

	runner := &Runner{workspace: tmpDir}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, tmpDir)
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

	writeTestGoogleDriveWorkspaceConfig(t, tmpDir, newCredPath, newTokenPath)

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

	runner := &Runner{workspace: tmpDir}
	_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, tmpDir)
	mcpStatus := googleDriveMcpRuntimeConfig{
		CredentialPath: credPath,
		TokenPath:      tokenPath,
	}

	codexHome := filepath.Join(tmpDir, ".codexHome")
	codexServer := expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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

	geminiServer := expectedGeminiGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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

	claudeServer := expectedClaudeGoogleDriveMcpServer(mcpStatus)
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
				server := expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedGeminiGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedGeminiGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus)
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
				server := expectedClaudeGoogleDriveMcpServer(mcpStatus)
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

			runner := &Runner{workspace: tmpDir}
			_, credPath, tokenPath := writeTestGoogleDriveMcpRuntime(t, tmpDir)
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

	runner := &Runner{workspace: tmpDir}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, tmpDir)

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

	runner := &Runner{workspace: tmpDir}
	_, _, _ = writeTestGoogleDriveMcpRuntime(t, tmpDir)

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

func writeTestGoogleDriveMcpRuntime(t *testing.T, workspace string) (string, string, string) {
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

	writeTestGoogleDriveWorkspaceConfig(t, workspace, credPath, tokenPath)
	return mcpConfigDir, credPath, tokenPath
}

func writeTestGoogleDriveWorkspaceConfig(t *testing.T, workspace string, credPath string, tokenPath string) {
	t.Helper()

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create .flowpilot settings dir: %v", err)
	}

	wsConfig := googleDriveWorkspaceConfigFile{
		Version:      1,
		ArtifactSync: googleDriveWorkspaceArtifactConfig{},
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

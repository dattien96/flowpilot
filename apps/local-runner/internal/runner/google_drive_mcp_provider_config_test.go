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

	workspaceConfig := `{
		"version": 1,
		"artifactSync": {},
		"mcp": {
			"credentialPath": "` + credPath + `",
			"tokenPath": "` + tokenPath + `"
		}
	}`

	wsConfigPath := filepath.Join(flowpilotDir, "google-drive-config.json")
	if err := os.WriteFile(wsConfigPath, []byte(workspaceConfig), 0o644); err != nil {
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

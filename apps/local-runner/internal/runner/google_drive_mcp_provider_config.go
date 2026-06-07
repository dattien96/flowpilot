package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Provider config status for Google Drive MCP
type GoogleDriveMcpProviderConfigStatus struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
	ConfigPath      string `json:"configPath"`
	Status          string `json:"status"`
	LastCheckedAt   string `json:"lastCheckedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
}

// Request to ensure provider config
type GoogleDriveMcpProviderConfigRequest struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
	Scope           string `json:"scope"` // "account" or "workspace"
	Mode            string `json:"mode"`  // "read_only" or "read_write"
}

// Response from ensuring provider config
type GoogleDriveMcpProviderConfigResponse struct {
	ProviderKey string `json:"providerKey"`
	ServerName  string `json:"serverName"`
	Status      string `json:"status"`
	Changed     bool   `json:"changed"`
	ConfigPath  string `json:"configPath"`
	LastError   string `json:"lastError,omitempty"`
}

// Extended Google Drive config response with provider configs
type GoogleDriveConfigWithProviders struct {
	GoogleDriveWorkspaceConfigResponse
	ProviderConfigs []GoogleDriveMcpProviderConfigStatus `json:"providerConfigs,omitempty"`
}

const (
	googleDriveMcpServerName = "google-drive"
)

// Read-only tool allowlist for Phase A
var googleDriveMcpReadOnlyTools = []string{
	"authGetStatus",
	"authListScopes",
	"authTestFileAccess",
	"search",
	"listFolder",
	"listSharedDrives",
	"readGoogleDoc",
	"readGoogleDocPaginated",
	"getGoogleDocContent",
	"getGoogleDocContentPaginated",
}

// Codex config TOML structures
type codexConfig struct {
	McpServers map[string]codexMcpServer `toml:"mcp_servers"`
}

type codexMcpServer struct {
	Command           string            `toml:"command"`
	Args              []string          `toml:"args"`
	StartupTimeoutSec int               `toml:"startup_timeout_sec,omitempty"`
	ToolTimeoutSec    int               `toml:"tool_timeout_sec,omitempty"`
	Enabled           bool              `toml:"enabled"`
	EnabledTools      []string          `toml:"enabled_tools,omitempty"`
	ApprovalMode      string            `toml:"default_tools_approval_mode,omitempty"`
	Env               map[string]string `toml:"env"`
}

// Gemini settings JSON structures
type geminiSettings struct {
	McpServers map[string]geminiMcpServer `json:"mcpServers"`
}

type geminiMcpServer struct {
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	Env          map[string]string `json:"env"`
	Timeout      int               `json:"timeout,omitempty"`
	Trust        bool              `json:"trust"`
	IncludeTools []string          `json:"includeTools,omitempty"`
}

// Claude config JSON structures
type claudeConfig struct {
	McpServers map[string]claudeMcpServer `json:"mcpServers"`
}

type claudeMcpServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Timeout int               `json:"timeout,omitempty"`
}

// EnsureGoogleDriveMcpProviderConfig configures a provider for Google Drive MCP
func (r *Runner) EnsureGoogleDriveMcpProviderConfig(req GoogleDriveMcpProviderConfigRequest) (GoogleDriveMcpProviderConfigResponse, error) {
	// Validate Google Drive MCP status first
	mcpStatus, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to get Google Drive MCP status: %w", err)
	}

	// Block if Google Drive MCP is not ready
	if mcpStatus.Status == "needs_input" || mcpStatus.Status == "failed" ||
		mcpStatus.Status == "needs_auth" || mcpStatus.Status == "reconnect_required" {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf(
			"Google Drive MCP must be configured and authenticated before configuring providers (current status: %s)",
			mcpStatus.Status,
		)
	}

	// Validate provider key
	providerKey := strings.TrimSpace(strings.ToLower(req.ProviderKey))
	if providerKey == "" {
		return GoogleDriveMcpProviderConfigResponse{}, errors.New("providerKey is required")
	}

	// Validate account home path
	accountHomePath := strings.TrimSpace(req.AccountHomePath)
	if accountHomePath == "" {
		return GoogleDriveMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "read_only"
	}

	// Route to provider-specific implementation
	switch providerKey {
	case "codex":
		return r.ensureCodexGoogleDriveMcpConfig(accountHomePath, mode, mcpStatus)
	case "gemini":
		return r.ensureGeminiGoogleDriveMcpConfig(accountHomePath, mode, mcpStatus)
	case "claude":
		return r.ensureClaudeGoogleDriveMcpConfig(accountHomePath, mode, mcpStatus)
	default:
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

// ensureCodexGoogleDriveMcpConfig configures Codex config.toml
func (r *Runner) ensureCodexGoogleDriveMcpConfig(accountHomePath string, mode string, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
	configPath := filepath.Join(accountHomePath, "config.toml")

	// Load existing config or create new
	var config codexConfig
	changed := false

	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to read Codex config: %w", err)
	}

	if err == nil {
		// Parse existing config
		if err := toml.Unmarshal(raw, &config); err != nil {
			return GoogleDriveMcpProviderConfigResponse{
				ProviderKey: "codex",
				ServerName:  googleDriveMcpServerName,
				Status:      "failed",
				ConfigPath:  configPath,
				LastError:   "invalid TOML config",
			}, fmt.Errorf("existing Codex config is invalid TOML: %w", err)
		}
	}

	// Initialize mcp_servers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]codexMcpServer)
	}

	// Build expected server config
	expectedServer := codexMcpServer{
		Command:           "npx",
		Args:              []string{"-y", "@piotr-agier/google-drive-mcp"},
		StartupTimeoutSec: 20,
		ToolTimeoutSec:    120,
		Enabled:           true,
		ApprovalMode:      "prompt",
		Env: map[string]string{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": mcpStatus.CredentialPath,
			"GOOGLE_DRIVE_MCP_TOKEN_PATH":    mcpStatus.TokenPath,
		},
	}

	// Add read-only tools for read_only mode
	if mode == "read_only" {
		expectedServer.EnabledTools = googleDriveMcpReadOnlyTools
	}

	// Check if config needs update
	existingServer, exists := config.McpServers[googleDriveMcpServerName]
	if !exists || !codexServerConfigMatches(existingServer, expectedServer) {
		config.McpServers[googleDriveMcpServerName] = expectedServer
		changed = true
	}

	// Write config if changed
	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}

		out, err := toml.Marshal(config)
		if err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return GoogleDriveMcpProviderConfigResponse{
		ProviderKey: "codex",
		ServerName:  googleDriveMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// codexServerConfigMatches checks if existing server config matches expected
func codexServerConfigMatches(existing, expected codexMcpServer) bool {
	if existing.Command != expected.Command {
		return false
	}
	if len(existing.Args) != len(expected.Args) {
		return false
	}
	for i, arg := range existing.Args {
		if arg != expected.Args[i] {
			return false
		}
	}
	if existing.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expected.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] {
		return false
	}
	if existing.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expected.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] {
		return false
	}
	return true
}

// ensureGeminiGoogleDriveMcpConfig configures Gemini settings.json
func (r *Runner) ensureGeminiGoogleDriveMcpConfig(accountHomePath string, mode string, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
	configDir := filepath.Join(accountHomePath, ".gemini")
	configPath := filepath.Join(configDir, "settings.json")

	// Load existing config or create new
	var config geminiSettings
	changed := false

	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to read Gemini config: %w", err)
	}

	if err == nil {
		// Parse existing config
		if err := json.Unmarshal(raw, &config); err != nil {
			return GoogleDriveMcpProviderConfigResponse{
				ProviderKey: "gemini",
				ServerName:  googleDriveMcpServerName,
				Status:      "failed",
				ConfigPath:  configPath,
				LastError:   "invalid JSON config",
			}, fmt.Errorf("existing Gemini config is invalid JSON: %w", err)
		}
	}

	// Initialize mcpServers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]geminiMcpServer)
	}

	// Build expected server config
	expectedServer := geminiMcpServer{
		Command: "npx",
		Args:    []string{"-y", "@piotr-agier/google-drive-mcp"},
		Env: map[string]string{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": mcpStatus.CredentialPath,
			"GOOGLE_DRIVE_MCP_TOKEN_PATH":    mcpStatus.TokenPath,
		},
		Timeout: 600000,
		Trust:   false,
	}

	// Add read-only tools for read_only mode
	if mode == "read_only" {
		expectedServer.IncludeTools = googleDriveMcpReadOnlyTools
	}

	// Check if config needs update
	existingServer, exists := config.McpServers[googleDriveMcpServerName]
	if !exists || !geminiServerConfigMatches(existingServer, expectedServer) {
		config.McpServers[googleDriveMcpServerName] = expectedServer
		changed = true
	}

	// Write config if changed
	if changed {
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}

		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return GoogleDriveMcpProviderConfigResponse{
		ProviderKey: "gemini",
		ServerName:  googleDriveMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// geminiServerConfigMatches checks if existing server config matches expected
func geminiServerConfigMatches(existing, expected geminiMcpServer) bool {
	if existing.Command != expected.Command {
		return false
	}
	if len(existing.Args) != len(expected.Args) {
		return false
	}
	for i, arg := range existing.Args {
		if arg != expected.Args[i] {
			return false
		}
	}
	if existing.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expected.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] {
		return false
	}
	if existing.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expected.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] {
		return false
	}
	return true
}

// ensureClaudeGoogleDriveMcpConfig configures Claude .claude.json
func (r *Runner) ensureClaudeGoogleDriveMcpConfig(accountHomePath string, mode string, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
	configPath := filepath.Join(accountHomePath, ".claude.json")

	// Load existing config or create new
	var config claudeConfig
	changed := false

	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to read Claude config: %w", err)
	}

	if err == nil {
		// Parse existing config
		if err := json.Unmarshal(raw, &config); err != nil {
			return GoogleDriveMcpProviderConfigResponse{
				ProviderKey: "claude",
				ServerName:  googleDriveMcpServerName,
				Status:      "failed",
				ConfigPath:  configPath,
				LastError:   "invalid JSON config",
			}, fmt.Errorf("existing Claude config is invalid JSON: %w", err)
		}
	}

	// Initialize mcpServers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]claudeMcpServer)
	}

	// Build expected server config
	expectedServer := claudeMcpServer{
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "@piotr-agier/google-drive-mcp"},
		Env: map[string]string{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": mcpStatus.CredentialPath,
			"GOOGLE_DRIVE_MCP_TOKEN_PATH":    mcpStatus.TokenPath,
		},
		Timeout: 600000,
	}

	// Check if config needs update
	existingServer, exists := config.McpServers[googleDriveMcpServerName]
	if !exists || !claudeServerConfigMatches(existingServer, expectedServer) {
		config.McpServers[googleDriveMcpServerName] = expectedServer
		changed = true
	}

	// Write config if changed
	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}

		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return GoogleDriveMcpProviderConfigResponse{
		ProviderKey: "claude",
		ServerName:  googleDriveMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// claudeServerConfigMatches checks if existing server config matches expected
func claudeServerConfigMatches(existing, expected claudeMcpServer) bool {
	if existing.Type != expected.Type || existing.Command != expected.Command {
		return false
	}
	if len(existing.Args) != len(expected.Args) {
		return false
	}
	for i, arg := range existing.Args {
		if arg != expected.Args[i] {
			return false
		}
	}
	if existing.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != expected.Env["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] {
		return false
	}
	if existing.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != expected.Env["GOOGLE_DRIVE_MCP_TOKEN_PATH"] {
		return false
	}
	return true
}

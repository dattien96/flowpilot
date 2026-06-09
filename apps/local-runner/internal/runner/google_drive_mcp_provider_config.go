package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	googleDriveMcpStatusMode = "read_only"
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
			config = codexConfig{}
			changed = true
		}
	}

	// Initialize mcp_servers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]codexMcpServer)
	}

	expectedServer := expectedCodexGoogleDriveMcpServer(mode, mcpStatus)

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

// toolListsMatch compares two tool lists, handling nil and empty slices
func toolListsMatch(existing, expected []string) bool {
	// Both empty or nil - they match
	if len(existing) == 0 && len(expected) == 0 {
		return true
	}
	// One is empty and other is not - no match
	if len(existing) != len(expected) {
		return false
	}
	// Compare elements
	for i, tool := range expected {
		if i >= len(existing) || existing[i] != tool {
			return false
		}
	}
	return true
}

// codexServerConfigMatches checks if existing server config matches expected
func codexServerConfigMatches(existing, expected codexMcpServer) bool {
	if existing.Command != expected.Command {
		return false
	}
	if existing.StartupTimeoutSec != expected.StartupTimeoutSec {
		return false
	}
	if existing.ToolTimeoutSec != expected.ToolTimeoutSec {
		return false
	}
	if existing.Enabled != expected.Enabled {
		return false
	}
	if existing.ApprovalMode != expected.ApprovalMode {
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
	// Compare enabled tools for read-only mode
	if !toolListsMatch(existing.EnabledTools, expected.EnabledTools) {
		return false
	}
	return true
}

func expectedCodexGoogleDriveMcpServer(mode string, mcpStatus googleDriveMcpRuntimeConfig) codexMcpServer {
	server := codexMcpServer{
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

	if mode == "read_only" {
		server.EnabledTools = googleDriveMcpReadOnlyTools
		server.ApprovalMode = "approve"
	}

	return server
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
			config = geminiSettings{}
			changed = true
		}
	}

	// Initialize mcpServers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]geminiMcpServer)
	}

	expectedServer := expectedGeminiGoogleDriveMcpServer(mode, mcpStatus)

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
	if existing.Timeout != expected.Timeout {
		return false
	}
	if existing.Trust != expected.Trust {
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
	// Compare includeTools for read-only mode
	if !toolListsMatch(existing.IncludeTools, expected.IncludeTools) {
		return false
	}
	return true
}

func expectedGeminiGoogleDriveMcpServer(mode string, mcpStatus googleDriveMcpRuntimeConfig) geminiMcpServer {
	server := geminiMcpServer{
		Command: "npx",
		Args:    []string{"-y", "@piotr-agier/google-drive-mcp"},
		Env: map[string]string{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": mcpStatus.CredentialPath,
			"GOOGLE_DRIVE_MCP_TOKEN_PATH":    mcpStatus.TokenPath,
		},
		Timeout: 600000,
		Trust:   false,
	}

	if mode == "read_only" {
		server.IncludeTools = googleDriveMcpReadOnlyTools
	}

	return server
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
			config = claudeConfig{}
			changed = true
		}
	}

	// Initialize mcpServers if not present
	if config.McpServers == nil {
		config.McpServers = make(map[string]claudeMcpServer)
	}

	expectedServer := expectedClaudeGoogleDriveMcpServer(mcpStatus)

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
	if existing.Timeout != expected.Timeout {
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

func expectedClaudeGoogleDriveMcpServer(mcpStatus googleDriveMcpRuntimeConfig) claudeMcpServer {
	return claudeMcpServer{
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "@piotr-agier/google-drive-mcp"},
		Env: map[string]string{
			"GOOGLE_DRIVE_OAUTH_CREDENTIALS": mcpStatus.CredentialPath,
			"GOOGLE_DRIVE_MCP_TOKEN_PATH":    mcpStatus.TokenPath,
		},
		Timeout: 600000,
	}
}

// resolveGoogleDriveMcpProviderStatuses checks provider config status for all providers
func (r *Runner) resolveGoogleDriveMcpProviderStatuses() ([]GoogleDriveMcpProviderConfigStatus, error) {
	// Get runtime config for credential and token paths
	mcpStatus, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Google Drive MCP runtime config: %w", err)
	}

	statuses := make([]GoogleDriveMcpProviderConfigStatus, 0, 3)
	now := time.Now().UTC().Format(time.RFC3339)
	accounts, _ := r.ListProviderAccounts()

	// Check each provider
	for _, providerKey := range []string{"codex", "gemini", "claude"} {
		accountHomes, discoverErr := DiscoverProviderAccountHomes(providerKey)
		if discoverErr != nil {
			statuses = append(statuses, GoogleDriveMcpProviderConfigStatus{
				ProviderKey:   providerKey,
				Status:        "failed",
				LastCheckedAt: now,
				LastError:     discoverErr.Error(),
			})
			continue
		}

		orderedHomes := orderProviderStatusHomes(providerKey, accountHomes, accounts)
		if len(orderedHomes) == 0 {
			statuses = append(statuses, GoogleDriveMcpProviderConfigStatus{
				ProviderKey:   providerKey,
				Status:        "not_started",
				LastCheckedAt: now,
			})
			continue
		}

		for _, accountHomePath := range orderedHomes {
			configPath := getProviderConfigPath(providerKey, accountHomePath)

			// Check if config file exists
			exists := fileExists(configPath)
			if !exists {
				statuses = append(statuses, GoogleDriveMcpProviderConfigStatus{
					ProviderKey:     providerKey,
					AccountHomePath: accountHomePath,
					ConfigPath:      configPath,
					Status:          "not_started",
					LastCheckedAt:   now,
				})
				continue
			}

			// Try to parse config and check for google-drive server
			status, detailedErr := r.checkProviderGoogleDriveMcpConfig(providerKey, accountHomePath, configPath, mcpStatus)
			if detailedErr != nil {
				status.LastError = detailedErr.Error()
			}
			status.LastCheckedAt = now
			statuses = append(statuses, status)
		}
	}

	return statuses, nil
}

func orderProviderStatusHomes(
	providerKey string,
	discoveredHomes []string,
	accounts []ProviderAccount,
) []string {
	if len(discoveredHomes) == 0 {
		return nil
	}

	canonicalHomes := make(map[string]string, len(discoveredHomes))
	for _, homePath := range discoveredHomes {
		canonicalHomes[canonicalPathKey(homePath)] = filepath.Clean(homePath)
	}

	orderedHomes := make([]string, 0, len(discoveredHomes))
	seen := make(map[string]struct{}, len(discoveredHomes))
	appendHome := func(homePath string) {
		cleanHome := canonicalPathKey(homePath)
		if _, alreadySeen := seen[cleanHome]; alreadySeen {
			return
		}
		canonicalHome, ok := canonicalHomes[cleanHome]
		if !ok {
			return
		}
		seen[cleanHome] = struct{}{}
		orderedHomes = append(orderedHomes, canonicalHome)
	}

	for _, account := range accounts {
		cleanHome := canonicalPathKey(account.HomePath)
		if account.ProviderKey == providerKey && account.IsActive && account.AuthStatus == "connected" {
			appendHome(cleanHome)
		}
	}

	for _, account := range accounts {
		cleanHome := canonicalPathKey(account.HomePath)
		if account.ProviderKey == providerKey && account.AuthStatus == "connected" {
			appendHome(cleanHome)
		}
	}

	for _, homePath := range discoveredHomes {
		appendHome(homePath)
	}

	return orderedHomes
}

// getProviderConfigPath returns the config file path for a provider
func getProviderConfigPath(providerKey string, accountHomePath string) string {
	switch providerKey {
	case "codex":
		return filepath.Join(accountHomePath, "config.toml")
	case "gemini":
		return filepath.Join(accountHomePath, ".gemini", "settings.json")
	case "claude":
		return filepath.Join(accountHomePath, ".claude.json")
	default:
		return ""
	}
}

// checkProviderGoogleDriveMcpConfig checks the config status for a specific provider
func (r *Runner) checkProviderGoogleDriveMcpConfig(
	providerKey string,
	accountHomePath string,
	configPath string,
	mcpStatus googleDriveMcpRuntimeConfig,
) (GoogleDriveMcpProviderConfigStatus, error) {
	result := GoogleDriveMcpProviderConfigStatus{
		ProviderKey:     providerKey,
		AccountHomePath: accountHomePath,
		ConfigPath:      configPath,
		Status:          "failed",
	}

	switch providerKey {
	case "codex":
		return r.checkCodexGoogleDriveMcpConfig(result, configPath, mcpStatus)
	case "gemini":
		return r.checkGeminiGoogleDriveMcpConfig(result, configPath, mcpStatus)
	case "claude":
		return r.checkClaudeGoogleDriveMcpConfig(result, configPath, mcpStatus)
	default:
		return result, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

// checkCodexGoogleDriveMcpConfig checks Codex config.toml for google-drive MCP
func (r *Runner) checkCodexGoogleDriveMcpConfig(
	result GoogleDriveMcpProviderConfigStatus,
	configPath string,
	mcpStatus googleDriveMcpRuntimeConfig,
) (GoogleDriveMcpProviderConfigStatus, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("failed to read Codex config: %w", err)
	}

	var config codexConfig
	if err := toml.Unmarshal(raw, &config); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("invalid TOML config: %w", err)
	}

	server, exists := config.McpServers[googleDriveMcpServerName]
	if !exists {
		result.Status = "not_started"
		return result, nil
	}

	// Check if paths match current runtime config
	if detectStaleCodexConfig(server, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleCodexConfig detects if Codex config drifts from the expected MCP server shape.
func detectStaleCodexConfig(server codexMcpServer, mcpStatus googleDriveMcpRuntimeConfig) bool {
	return !codexServerConfigMatches(server, expectedCodexGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus))
}

// checkGeminiGoogleDriveMcpConfig checks Gemini settings.json for google-drive MCP
func (r *Runner) checkGeminiGoogleDriveMcpConfig(
	result GoogleDriveMcpProviderConfigStatus,
	configPath string,
	mcpStatus googleDriveMcpRuntimeConfig,
) (GoogleDriveMcpProviderConfigStatus, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("failed to read Gemini config: %w", err)
	}

	var config geminiSettings
	if err := json.Unmarshal(raw, &config); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("invalid JSON config: %w", err)
	}

	server, exists := config.McpServers[googleDriveMcpServerName]
	if !exists {
		result.Status = "not_started"
		return result, nil
	}

	// Check if paths match current runtime config
	if detectStaleGeminiConfig(server, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleGeminiConfig detects if Gemini config drifts from the expected MCP server shape.
func detectStaleGeminiConfig(server geminiMcpServer, mcpStatus googleDriveMcpRuntimeConfig) bool {
	return !geminiServerConfigMatches(server, expectedGeminiGoogleDriveMcpServer(googleDriveMcpStatusMode, mcpStatus))
}

// checkClaudeGoogleDriveMcpConfig checks Claude .claude.json for google-drive MCP
func (r *Runner) checkClaudeGoogleDriveMcpConfig(
	result GoogleDriveMcpProviderConfigStatus,
	configPath string,
	mcpStatus googleDriveMcpRuntimeConfig,
) (GoogleDriveMcpProviderConfigStatus, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("failed to read Claude config: %w", err)
	}

	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("invalid JSON config: %w", err)
	}

	server, exists := config.McpServers[googleDriveMcpServerName]
	if !exists {
		result.Status = "not_started"
		return result, nil
	}

	// Check if paths match current runtime config
	if detectStaleClaudeConfig(server, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleClaudeConfig detects if Claude config drifts from the expected MCP server shape.
func detectStaleClaudeConfig(server claudeMcpServer, mcpStatus googleDriveMcpRuntimeConfig) bool {
	return !claudeServerConfigMatches(server, expectedClaudeGoogleDriveMcpServer(mcpStatus))
}

// detectProviderConfigStale checks if provider config paths are stale
func (r *Runner) detectProviderConfigStale(providerKey string, configPath string, mcpStatus googleDriveMcpRuntimeConfig) (bool, error) {
	switch providerKey {
	case "codex":
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return false, err
		}
		var config codexConfig
		if err := toml.Unmarshal(raw, &config); err != nil {
			return false, nil // If invalid, not necessarily stale
		}
		server, exists := config.McpServers[googleDriveMcpServerName]
		if !exists {
			return false, nil // Not configured
		}
		return detectStaleCodexConfig(server, mcpStatus), nil

	case "gemini":
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return false, err
		}
		var config geminiSettings
		if err := json.Unmarshal(raw, &config); err != nil {
			return false, nil // If invalid, not necessarily stale
		}
		server, exists := config.McpServers[googleDriveMcpServerName]
		if !exists {
			return false, nil // Not configured
		}
		return detectStaleGeminiConfig(server, mcpStatus), nil

	case "claude":
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return false, err
		}
		var config claudeConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			return false, nil // If invalid, not necessarily stale
		}
		server, exists := config.McpServers[googleDriveMcpServerName]
		if !exists {
			return false, nil // Not configured
		}
		return detectStaleClaudeConfig(server, mcpStatus), nil

	default:
		return false, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

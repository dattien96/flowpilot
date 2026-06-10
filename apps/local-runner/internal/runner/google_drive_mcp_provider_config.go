package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Provider config status for Google Drive MCP
type GoogleDriveMcpProviderConfigStatus struct {
	ProviderKey     string   `json:"providerKey"`
	AccountHomePath string   `json:"accountHomePath"`
	ConfigPath      string   `json:"configPath"`
	Status          string   `json:"status"`
	ConfigKind      string   `json:"configKind,omitempty"`
	Command         string   `json:"command,omitempty"`
	Args            []string `json:"args,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	ApprovalMode    string   `json:"approvalMode,omitempty"`
	LastCheckedAt   string   `json:"lastCheckedAt,omitempty"`
	LastError       string   `json:"lastError,omitempty"`
}

// Request to ensure provider config
type GoogleDriveMcpProviderConfigRequest struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
	Scope           string `json:"scope"` // "account" or "workspace"
	Mode            string `json:"mode"`  // "read_only" or "read_write"
	YoloMode        bool   `json:"yoloMode,omitempty"`
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
	googleDriveMcpServerName     = "google-drive"
	googleDriveMcpStatusMode     = "read_only"
	googleDriveProxyMcpFlag      = "FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"
	googleDriveProxyAccountIDEnv = "FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID"
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

var googleDriveMcpReadWriteTools = []string{
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
	"createGoogleDoc",
	"updateGoogleDoc",
	"createFolder",
}

func flowpilotGoogleDriveProxyMcpEnabled() bool {
	return true
}

func (r *Runner) googleDriveProxyMcpAuthReady() (bool, error) {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	status := r.resolveGoogleDriveMcpStatus(configFile)
	return status.Configured, nil
}

func (r *Runner) validateGoogleDriveProxyMcpPrerequisites() error {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	selection, err := r.resolveGoogleDriveProxyAccountSelection(configFile)
	if err != nil {
		return err
	}
	status := r.resolveGoogleDriveMcpStatus(configFile)
	if !status.Configured {
		if !status.BackendPackageAvailable {
			return errors.New("Google Drive MCP launcher is not available; install FlowPilot or Go so the proxy launcher can start the Google Drive MCP package")
		}
		if status.AccountSelectionRequired && len(selection.Accounts) == 0 {
			return errors.New("no connected Google Drive account is available; connect an account in Google Drive setup first")
		}
		if status.AccountSelectionRequired && len(selection.Accounts) > 1 && strings.TrimSpace(status.AccountID) == "" {
			return errors.New("select an active Google account in Google Drive setup before using the proxy MCP")
		}
		if len(status.MissingScopes) > 0 {
			return fmt.Errorf(
				"selected Google Drive account is missing MCP read scope (%s); reconnect the account in Google Drive setup and grant broad read access",
				strings.Join(status.MissingScopes, ", "),
			)
		}
		for _, field := range status.MissingFields {
			if field == "refreshToken" {
				return errors.New("refresh token is not configured")
			}
		}
		if status.Status == "failed" {
			return errors.New("FlowPilot proxy Google Drive auth is incomplete")
		}
		return errors.New("FlowPilot proxy Google Drive auth must be configured before configuring providers")
	}
	return nil
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

func classifyGoogleDriveServer(command string, args []string) (configKind string, mode string) {
	command = strings.TrimSpace(command)
	if _, _, parsedMode, _, ok := parseGoogleDriveProxyMcpInvocation(command, args); ok {
		return "proxy", parsedMode
	}
	if command == "npx" && len(args) >= 2 && args[0] == "-y" && args[1] == "@piotr-agier/google-drive-mcp" {
		return "legacy_raw", "read_only"
	}
	return "unknown", ""
}

func applyDetectedServerStatus(
	result GoogleDriveMcpProviderConfigStatus,
	command string,
	args []string,
	approvalMode string,
) GoogleDriveMcpProviderConfigStatus {
	configKind, mode := classifyGoogleDriveServer(command, args)
	result.ConfigKind = configKind
	result.Command = strings.TrimSpace(command)
	result.Args = append([]string(nil), args...)
	result.Mode = mode
	result.ApprovalMode = strings.TrimSpace(approvalMode)
	return result
}

// EnsureGoogleDriveMcpProviderConfig configures a provider for Google Drive MCP
func (r *Runner) EnsureGoogleDriveMcpProviderConfig(req GoogleDriveMcpProviderConfigRequest) (GoogleDriveMcpProviderConfigResponse, error) {
	configFile, configErr := r.loadGoogleDriveWorkspaceConfigFile()
	if configErr != nil && !errors.Is(configErr, os.ErrNotExist) {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to get Google Drive workspace config: %w", configErr)
	}
	mcpStatus := r.resolveGoogleDriveMcpStatus(configFile)
	runtimeMcpStatus := googleDriveMcpStatusRuntimeConfig(mcpStatus)
	if flowpilotGoogleDriveProxyMcpEnabled() {
		if !mcpStatus.Configured {
			return GoogleDriveMcpProviderConfigResponse{}, errors.New(
				"FlowPilot proxy Google Drive auth must be configured before configuring providers",
			)
		}
	} else if mcpStatus.Status == "needs_input" || mcpStatus.Status == "failed" ||
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
		return r.ensureCodexGoogleDriveMcpConfig(accountHomePath, mode, req.YoloMode, runtimeMcpStatus)
	case "gemini":
		return r.ensureGeminiGoogleDriveMcpConfig(accountHomePath, mode, req.YoloMode, runtimeMcpStatus)
	case "claude":
		return r.ensureClaudeGoogleDriveMcpConfig(accountHomePath, mode, req.YoloMode, runtimeMcpStatus)
	default:
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

// ensureCodexGoogleDriveMcpConfig configures Codex config.toml
func (r *Runner) ensureCodexGoogleDriveMcpConfig(accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
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

	expectedServer := expectedCodexGoogleDriveMcpServer(r.workspace, accountHomePath, mode, yoloMode, mcpStatus)

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

func envMatches(existing, expected map[string]string) bool {
	if len(existing) != len(expected) {
		return false
	}

	for key, expectedValue := range expected {
		if existing[key] != expectedValue {
			return false
		}
	}

	return true
}

func googleDriveProxyMcpArgs(workspace string, accountHomePath string, mode string, yoloMode bool) []string {
	args := []string{
		"google-drive-mcp",
		"--workspace",
		strings.TrimSpace(filepath.Clean(workspace)),
		"--account-home",
		strings.TrimSpace(accountHomePath),
		"--mode",
		mode,
	}
	if yoloMode {
		args = append(args, "--yolo-mode")
	}
	return args
}

func googleDriveProxyMcpCommand(workspace string) (string, []string) {
	return googleDriveProxyMcpCommandWithLookup(workspace, exec.LookPath)
}

func googleDriveProxyMcpCommandWithLookup(workspace string, lookPath func(string) (string, error)) (string, []string) {
	if _, err := lookPath("flowpilot"); err == nil {
		return "flowpilot", nil
	}

	runnerDir := filepath.Join(strings.TrimSpace(filepath.Clean(workspace)), "apps", "local-runner")
	if _, err := os.Stat(filepath.Join(runnerDir, "go.mod")); err == nil {
		if _, err := lookPath("go"); err == nil {
			return "go", []string{"-C", runnerDir, "run", "./cmd/flowpilot"}
		}
	}

	return "flowpilot", nil
}

func googleDriveProxyMcpApprovalMode(yoloMode bool) string {
	if yoloMode {
		return "approve"
	}
	return "prompt"
}

func parseGoogleDriveProxyMcpArgs(args []string) (workspace string, accountHomePath string, mode string, yoloMode bool, ok bool) {
	if len(args) != 7 && len(args) != 8 {
		return "", "", "", false, false
	}
	if args[0] != "google-drive-mcp" ||
		args[1] != "--workspace" ||
		args[3] != "--account-home" ||
		args[5] != "--mode" {
		return "", "", "", false, false
	}

	workspace = strings.TrimSpace(filepath.Clean(args[2]))
	accountHomePath = strings.TrimSpace(args[4])
	mode = strings.ToLower(strings.TrimSpace(args[6]))
	if mode != "read_only" && mode != "read_write" {
		return "", "", "", false, false
	}

	if len(args) == 8 {
		if args[7] != "--yolo-mode" {
			return "", "", "", false, false
		}
		yoloMode = true
	}

	return workspace, accountHomePath, mode, yoloMode, true
}

func parseGoogleDriveProxyMcpInvocation(
	command string,
	args []string,
) (workspace string, accountHomePath string, mode string, yoloMode bool, ok bool) {
	command = strings.TrimSpace(command)
	switch command {
	case "flowpilot":
		return parseGoogleDriveProxyMcpArgs(args)
	case "go":
		if len(args) < 5 || args[0] != "-C" || args[2] != "run" || args[3] != "./cmd/flowpilot" {
			return "", "", "", false, false
		}
		return parseGoogleDriveProxyMcpArgs(args[4:])
	default:
		return "", "", "", false, false
	}
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
	if !envMatches(existing.Env, expected.Env) {
		return false
	}
	// Compare enabled tools for read-only mode
	if !toolListsMatch(existing.EnabledTools, expected.EnabledTools) {
		return false
	}
	return true
}

func expectedCodexGoogleDriveMcpServer(workspace string, accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) codexMcpServer {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		command, argsPrefix := googleDriveProxyMcpCommand(workspace)
		server := codexMcpServer{
			Command:           command,
			Args:              append(argsPrefix, googleDriveProxyMcpArgs(workspace, accountHomePath, mode, yoloMode)...),
			StartupTimeoutSec: 20,
			ToolTimeoutSec:    120,
			Enabled:           true,
			Env:               map[string]string{},
		}
		if strings.TrimSpace(mcpStatus.AccountID) != "" {
			server.Env[googleDriveProxyAccountIDEnv] = strings.TrimSpace(mcpStatus.AccountID)
		}

		server.ApprovalMode = googleDriveProxyMcpApprovalMode(yoloMode)
		if mode == "read_only" {
			server.EnabledTools = googleDriveMcpReadOnlyTools
		} else {
			server.EnabledTools = googleDriveMcpReadWriteTools
		}
		return server
	}

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
func (r *Runner) ensureGeminiGoogleDriveMcpConfig(accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
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

	expectedServer := expectedGeminiGoogleDriveMcpServer(r.workspace, accountHomePath, mode, yoloMode, mcpStatus)

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
	if !envMatches(existing.Env, expected.Env) {
		return false
	}
	// Compare includeTools for read-only mode
	if !toolListsMatch(existing.IncludeTools, expected.IncludeTools) {
		return false
	}
	return true
}

func expectedGeminiGoogleDriveMcpServer(workspace string, accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) geminiMcpServer {
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

	if flowpilotGoogleDriveProxyMcpEnabled() {
		server.Command, server.Args = googleDriveProxyMcpCommand(workspace)
		server.Args = append(server.Args, googleDriveProxyMcpArgs(workspace, accountHomePath, mode, yoloMode)...)
		server.Trust = false
		server.IncludeTools = googleDriveMcpReadOnlyTools
		if mode != "read_only" {
			server.IncludeTools = googleDriveMcpReadWriteTools
		}
		server.Env = map[string]string{}
		if strings.TrimSpace(mcpStatus.AccountID) != "" {
			server.Env[googleDriveProxyAccountIDEnv] = strings.TrimSpace(mcpStatus.AccountID)
		}
		return server
	}

	if mode == "read_only" {
		server.IncludeTools = googleDriveMcpReadOnlyTools
	}

	return server
}

// ensureClaudeGoogleDriveMcpConfig configures Claude .claude.json
func (r *Runner) ensureClaudeGoogleDriveMcpConfig(accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
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

	expectedServer := expectedClaudeGoogleDriveMcpServer(r.workspace, accountHomePath, mode, yoloMode, mcpStatus)

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
	if !envMatches(existing.Env, expected.Env) {
		return false
	}
	return true
}

func expectedClaudeGoogleDriveMcpServer(workspace string, accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) claudeMcpServer {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		command, argsPrefix := googleDriveProxyMcpCommand(workspace)
		server := claudeMcpServer{
			Type:    "stdio",
			Command: command,
			Args:    append(argsPrefix, googleDriveProxyMcpArgs(workspace, accountHomePath, mode, yoloMode)...),
			Env:     map[string]string{},
			Timeout: 600000,
		}
		if strings.TrimSpace(mcpStatus.AccountID) != "" {
			server.Env[googleDriveProxyAccountIDEnv] = strings.TrimSpace(mcpStatus.AccountID)
		}
		return server
	}

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
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to get Google Drive workspace config: %w", err)
	}
	mcpStatus := r.resolveGoogleDriveMcpStatus(configFile)
	runtimeMcpStatus := googleDriveMcpStatusRuntimeConfig(mcpStatus)

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
			status, detailedErr := r.checkProviderGoogleDriveMcpConfig(providerKey, accountHomePath, configPath, runtimeMcpStatus)
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

func googleDriveMcpStatusRuntimeConfig(status GoogleDriveMcpStatus) googleDriveMcpRuntimeConfig {
	return googleDriveMcpRuntimeConfig{
		CredentialPath:           status.CredentialPath,
		TokenPath:                status.TokenPath,
		CredentialExists:         status.CredentialFileExists,
		CredentialValid:          status.CredentialFileValid,
		TokenExists:              status.TokenFileExists,
		TokenRefreshValid:        status.TokenRefreshValid,
		BackendPackageAvailable:  status.BackendPackageAvailable,
		AccountID:                status.AccountID,
		AccountEmail:             status.AccountEmail,
		AccountSelectionRequired: status.AccountSelectionRequired,
		Status:                   status.Status,
	}
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
	result = applyDetectedServerStatus(result, server.Command, server.Args, server.ApprovalMode)

	// Check if paths match current runtime config
	if detectStaleCodexConfig(server, r.workspace, filepath.Dir(configPath), mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleCodexConfig detects if Codex config drifts from the expected MCP server shape.
func detectStaleCodexConfig(server codexMcpServer, workspace string, accountHomePath string, mcpStatus googleDriveMcpRuntimeConfig) bool {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		parsedWorkspace, parsedAccountHome, parsedMode, parsedYoloMode, ok := parseGoogleDriveProxyMcpInvocation(server.Command, server.Args)
		if !ok {
			return true
		}
		if strings.TrimSpace(filepath.Clean(workspace)) != parsedWorkspace || strings.TrimSpace(accountHomePath) != parsedAccountHome {
			return true
		}
		expected := expectedCodexGoogleDriveMcpServer(workspace, accountHomePath, parsedMode, parsedYoloMode, mcpStatus)
		expected.Command = server.Command
		expected.Args = append([]string(nil), server.Args...)
		return !codexServerConfigMatches(server, expected)
	}
	return !codexServerConfigMatches(server, expectedCodexGoogleDriveMcpServer(workspace, "", googleDriveMcpStatusMode, false, mcpStatus))
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
	result = applyDetectedServerStatus(result, server.Command, server.Args, "")

	// Check if paths match current runtime config
	if detectStaleGeminiConfig(server, r.workspace, result.AccountHomePath, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleGeminiConfig detects if Gemini config drifts from the expected MCP server shape.
func detectStaleGeminiConfig(server geminiMcpServer, workspace string, accountHomePath string, mcpStatus googleDriveMcpRuntimeConfig) bool {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		parsedWorkspace, parsedAccountHome, parsedMode, parsedYoloMode, ok := parseGoogleDriveProxyMcpInvocation(server.Command, server.Args)
		if !ok {
			return true
		}
		if strings.TrimSpace(filepath.Clean(workspace)) != parsedWorkspace || strings.TrimSpace(accountHomePath) != parsedAccountHome {
			return true
		}
		expected := expectedGeminiGoogleDriveMcpServer(workspace, accountHomePath, parsedMode, parsedYoloMode, mcpStatus)
		expected.Command = server.Command
		expected.Args = append([]string(nil), server.Args...)
		return !geminiServerConfigMatches(server, expected)
	}
	return !geminiServerConfigMatches(server, expectedGeminiGoogleDriveMcpServer(workspace, "", googleDriveMcpStatusMode, false, mcpStatus))
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
	result = applyDetectedServerStatus(result, server.Command, server.Args, "")

	// Check if paths match current runtime config
	if detectStaleClaudeConfig(server, r.workspace, result.AccountHomePath, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleClaudeConfig detects if Claude config drifts from the expected MCP server shape.
func detectStaleClaudeConfig(server claudeMcpServer, workspace string, accountHomePath string, mcpStatus googleDriveMcpRuntimeConfig) bool {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		parsedWorkspace, parsedAccountHome, parsedMode, parsedYoloMode, ok := parseGoogleDriveProxyMcpInvocation(server.Command, server.Args)
		if !ok {
			return true
		}
		if strings.TrimSpace(filepath.Clean(workspace)) != parsedWorkspace || strings.TrimSpace(accountHomePath) != parsedAccountHome {
			return true
		}
		expected := expectedClaudeGoogleDriveMcpServer(workspace, accountHomePath, parsedMode, parsedYoloMode, mcpStatus)
		expected.Command = server.Command
		expected.Args = append([]string(nil), server.Args...)
		return !claudeServerConfigMatches(server, expected)
	}
	return !claudeServerConfigMatches(server, expectedClaudeGoogleDriveMcpServer(workspace, "", googleDriveMcpStatusMode, false, mcpStatus))
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
		return detectStaleCodexConfig(server, "", filepath.Dir(configPath), mcpStatus), nil

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
		return detectStaleGeminiConfig(server, "", filepath.Dir(filepath.Dir(configPath)), mcpStatus), nil

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
		return detectStaleClaudeConfig(server, "", filepath.Dir(configPath), mcpStatus), nil

	default:
		return false, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

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
	ProviderKey       string `json:"providerKey"`
	AccountHomePath   string `json:"accountHomePath"`
	Scope             string `json:"scope"` // "account" or "workspace"
	Mode              string `json:"mode"`  // "read_only" or "read_write"
	YoloMode          bool   `json:"yoloMode,omitempty"`
	WorkflowRunID     string `json:"workflowRunId,omitempty"`
	WorkflowStepRunID string `json:"workflowStepRunId,omitempty"`
	ProcessKey        string `json:"processKey,omitempty"`
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
	googleDriveMcpServerName        = "google-drive"
	googleDriveMcpStatusMode        = "read_only"
	googleDriveProxyMcpFlag         = "FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP"
	googleDriveProxyAccountIDEnv    = "FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID"
	googleDriveProxyRefreshTokenEnv = "FLOWPILOT_GOOGLE_DRIVE_REFRESH_TOKEN"
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
		if status.ReconnectRequired || status.Status == "reconnect_required" {
			return errors.New("Google Drive MCP requires reconnect; reconnect the selected Google account in Google Drive setup")
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
	Command           string            `toml:"command,omitempty"`
	Args              []string          `toml:"args,omitempty"`
	StartupTimeoutSec int               `toml:"startup_timeout_sec,omitempty"`
	ToolTimeoutSec    int               `toml:"tool_timeout_sec,omitempty"`
	Enabled           bool              `toml:"enabled"`
	EnabledTools      []string          `toml:"enabled_tools,omitempty"`
	ApprovalMode      string            `toml:"default_tools_approval_mode,omitempty"`
	Env               map[string]string `toml:"env,omitempty"`
	// URL/BearerTokenEnvVar (Task-234) are additive fields for a
	// Streamable-HTTP remote MCP entry (Jira) — Codex selects transport by
	// which keys are present ("command" => stdio, "url" => http), reading
	// the bearer value from the env var this names rather than an inline
	// header. Both omitempty so stdio entries (Google Drive/Firebase/
	// Telegram) are unaffected.
	URL               string `toml:"url,omitempty"`
	BearerTokenEnvVar string `toml:"bearer_token_env_var,omitempty"`
	// HTTPHeaders carries a full Authorization value (e.g. Basic …) when
	// bearer_token_env_var would incorrectly wrap it as Bearer (Rovo personal
	// API token path). omitempty keeps stdio entries clean.
	HTTPHeaders map[string]string `toml:"http_headers,omitempty"`
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
	// HTTPURL/Headers (Task-234) are additive fields for a Streamable-HTTP
	// remote MCP entry (Jira) — Gemini's documented shape: `httpUrl` selects
	// the Streamable HTTP transport (distinct from `url`, which Gemini treats
	// as SSE), with `headers` carrying the Authorization value directly.
	// Both omitempty so stdio entries are unaffected.
	HTTPURL string            `json:"httpUrl,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
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
	// URL/Headers are additive fields (Task-234 T-1) for remote HTTP MCP
	// servers (Jira) merged into the per-turn --mcp-config alongside the
	// existing stdio-shaped entries (Firebase/Telegram/Google Drive). Both
	// omitempty so they never appear on the stdio entries this struct
	// already served before this change.
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Grok config.toml uses the same [mcp_servers.<name>] shape as Codex, but its config.toml
// routinely carries other top-level sections ([cli], [marketplace], [models], [ui],
// [plugins]) that a McpServers-only struct would silently drop on a Marshal round-trip. So
// unlike codexConfig, we never decode the whole document into a typed struct — the
// ensure/check functions below round-trip the document as a generic map and only touch the
// mcp_servers.google-drive sub-table.
type grokMcpServer struct {
	// Command/Args use omitempty so a remote HTTP entry (Jira: empty command,
	// no args) does NOT serialize `command = ""` / `args = []` into config.toml.
	// Grok treats a present `command` as a stdio server and tries to launch it;
	// an empty command made the Jira entry show as `[unavailable]` in Grok CLI.
	// This mirrors codexMcpServer, whose Command/Args are already omitempty so
	// its remote jira entry stays stdio-field-free (see the Codex assertion in
	// jira_mcp_provider_config_test.go).
	Command           string            `toml:"command,omitempty"`
	Args              []string          `toml:"args,omitempty"`
	Enabled           bool              `toml:"enabled"`
	StartupTimeoutSec int               `toml:"startup_timeout_sec,omitempty"`
	ToolTimeoutSec    int               `toml:"tool_timeout_sec,omitempty"`
	Env               map[string]string `toml:"env"`
	// URL/Headers are additive (G2 / Task-234 Q-2) for remote HTTP MCP
	// servers such as Jira. omitempty keeps stdio entries (Drive/Firebase/
	// Telegram) free of empty url/headers keys in config.toml.
	URL     string            `toml:"url,omitempty"`
	Headers map[string]string `toml:"headers,omitempty"`
}

// grokServerToMap converts a typed server into the generic map shape needed to splice into
// a document decoded as map[string]interface{}.
func grokServerToMap(server grokMcpServer) (map[string]interface{}, error) {
	raw, err := toml.Marshal(server)
	if err != nil {
		return nil, err
	}
	m := map[string]interface{}{}
	if err := toml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// grokServerFromMap converts a generic mcp_servers.google-drive entry back into a typed
// struct for comparison. Returns ok=false if the entry doesn't decode as a Grok MCP server.
func grokServerFromMap(m map[string]interface{}) (grokMcpServer, bool) {
	raw, err := toml.Marshal(m)
	if err != nil {
		return grokMcpServer{}, false
	}
	var server grokMcpServer
	if err := toml.Unmarshal(raw, &server); err != nil {
		return grokMcpServer{}, false
	}
	return server, true
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
	runtimeMcpStatus.WorkflowRunID = strings.TrimSpace(req.WorkflowRunID)
	runtimeMcpStatus.WorkflowStepRunID = strings.TrimSpace(req.WorkflowStepRunID)
	runtimeMcpStatus.ProcessKey = strings.TrimSpace(req.ProcessKey)
	r.hydrateGoogleDriveProxyOAuthRuntimeConfig(&runtimeMcpStatus)
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
	case "grok":
		return r.ensureGrokGoogleDriveMcpConfig(accountHomePath, mode, req.YoloMode, runtimeMcpStatus)
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
	for key, expectedValue := range expected {
		if existing[key] != expectedValue {
			return false
		}
	}

	for key := range existing {
		if _, ok := expected[key]; ok {
			continue
		}
		if isGoogleDriveProxyApprovalScopeEnv(key) {
			continue
		}
		return false
	}

	return true
}

func isGoogleDriveProxyApprovalScopeEnv(key string) bool {
	switch key {
	case googleDriveProxyWorkflowRunIDEnv,
		googleDriveProxyWorkflowStepIDEnv,
		googleDriveProxyProcessKeyEnv:
		return true
	default:
		return false
	}
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

// googleDriveProxyMcpApprovalMode is the proxy MCP gate, keyed on YOLO (CP-29,
// 04-04). The old hack pinned this to "approve" unconditionally, decoupling it from
// YOLO; approval mode is now YOLO-derived per the SSOT resolver so the proxy stays
// in lockstep with the rest of the run. YOLO=true → "approve" (gating disabled, the
// proxy auto-runs); YOLO=false → "prompt" (Codex surfaces the request, which
// flows through the FlowPilot approval bridge / policy engine). The proxy still
// gates internally via its --yolo-mode arg.
func googleDriveProxyMcpApprovalMode(yoloMode bool) string {
	if resolveYoloPosture(yoloMode).RunnerAutoApprove {
		return "approve"
	}
	return "prompt"
}

func googleDriveProxyMcpServerEnv(mcpStatus googleDriveMcpRuntimeConfig) map[string]string {
	env := map[string]string{}
	if strings.TrimSpace(mcpStatus.AccountID) != "" {
		env[googleDriveProxyAccountIDEnv] = strings.TrimSpace(mcpStatus.AccountID)
	}
	if strings.TrimSpace(mcpStatus.WorkflowRunID) != "" {
		env[googleDriveProxyWorkflowRunIDEnv] = strings.TrimSpace(mcpStatus.WorkflowRunID)
	}
	if strings.TrimSpace(mcpStatus.WorkflowStepRunID) != "" {
		env[googleDriveProxyWorkflowStepIDEnv] = strings.TrimSpace(mcpStatus.WorkflowStepRunID)
	}
	if strings.TrimSpace(mcpStatus.ProcessKey) != "" {
		env[googleDriveProxyProcessKeyEnv] = strings.TrimSpace(mcpStatus.ProcessKey)
	}
	if strings.TrimSpace(mcpStatus.ProxyClientID) != "" {
		env[googleDriveClientIDEnv] = strings.TrimSpace(mcpStatus.ProxyClientID)
	}
	if strings.TrimSpace(mcpStatus.ProxyClientSecret) != "" {
		env[googleDriveClientSecretEnv] = strings.TrimSpace(mcpStatus.ProxyClientSecret)
	}
	if strings.TrimSpace(mcpStatus.ProxyRefreshToken) != "" {
		env[googleDriveProxyRefreshTokenEnv] = strings.TrimSpace(mcpStatus.ProxyRefreshToken)
	}
	return env
}

func (r *Runner) hydrateGoogleDriveProxyOAuthRuntimeConfig(mcpStatus *googleDriveMcpRuntimeConfig) {
	if mcpStatus == nil || !flowpilotGoogleDriveProxyMcpEnabled() {
		return
	}
	config, err := r.resolveGoogleDriveProxyOAuthConfig()
	if err != nil {
		return
	}
	mcpStatus.ProxyClientID = strings.TrimSpace(config.clientID)
	mcpStatus.ProxyClientSecret = strings.TrimSpace(config.clientSecret)
	if strings.TrimSpace(mcpStatus.AccountID) != "" {
		if creds, err := r.loadGoogleDriveCredentialByAccount(mcpStatus.AccountID); err == nil {
			mcpStatus.ProxyRefreshToken = strings.TrimSpace(creds.RefreshToken)
		}
	}
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
			Env:               googleDriveProxyMcpServerEnv(mcpStatus),
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
		server.Env = googleDriveProxyMcpServerEnv(mcpStatus)
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
			Env:     googleDriveProxyMcpServerEnv(mcpStatus),
			Timeout: 600000,
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

// ensureGrokGoogleDriveMcpConfig configures Grok's config.toml. See the grokMcpServer
// comment: the document is round-tripped as a generic map so unrelated top-level sections
// (marketplace, models, ui, ...) survive the write.
func (r *Runner) ensureGrokGoogleDriveMcpConfig(accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) (GoogleDriveMcpProviderConfigResponse, error) {
	configPath := filepath.Join(accountHomePath, "config.toml")

	doc := map[string]interface{}{}
	changed := false

	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to read Grok config: %w", err)
	}
	if err == nil {
		if unmarshalErr := toml.Unmarshal(raw, &doc); unmarshalErr != nil {
			doc = map[string]interface{}{}
			changed = true
		}
	}

	mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
	}

	expectedServer := expectedGrokGoogleDriveMcpServer(r.workspace, accountHomePath, mode, yoloMode, mcpStatus)

	existingMatches := false
	if existingRaw, exists := mcpServers[googleDriveMcpServerName]; exists {
		if existingMap, ok := existingRaw.(map[string]interface{}); ok {
			if existingServer, ok := grokServerFromMap(existingMap); ok {
				existingMatches = grokServerConfigMatches(existingServer, expectedServer)
			}
		}
	}
	if !existingMatches {
		expectedMap, mapErr := grokServerToMap(expectedServer)
		if mapErr != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to build Grok MCP server entry: %w", mapErr)
		}
		mcpServers[googleDriveMcpServerName] = expectedMap
		doc["mcp_servers"] = mcpServers
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}

		out, err := toml.Marshal(doc)
		if err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}

		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return GoogleDriveMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return GoogleDriveMcpProviderConfigResponse{
		ProviderKey: "grok",
		ServerName:  googleDriveMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// grokServerConfigMatches checks if existing server config matches expected
func grokServerConfigMatches(existing, expected grokMcpServer) bool {
	if existing.Command != expected.Command {
		return false
	}
	if existing.Enabled != expected.Enabled {
		return false
	}
	if existing.StartupTimeoutSec != expected.StartupTimeoutSec {
		return false
	}
	if existing.ToolTimeoutSec != expected.ToolTimeoutSec {
		return false
	}
	if existing.URL != expected.URL {
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
	if !envMatches(existing.Headers, expected.Headers) {
		return false
	}
	return true
}

func expectedGrokGoogleDriveMcpServer(workspace string, accountHomePath string, mode string, yoloMode bool, mcpStatus googleDriveMcpRuntimeConfig) grokMcpServer {
	command, argsPrefix := googleDriveProxyMcpCommand(workspace)
	return grokMcpServer{
		Command:           command,
		Args:              append(argsPrefix, googleDriveProxyMcpArgs(workspace, accountHomePath, mode, yoloMode)...),
		Enabled:           true,
		StartupTimeoutSec: 20,
		ToolTimeoutSec:    120,
		Env:               googleDriveProxyMcpServerEnv(mcpStatus),
	}
}

// flowpilotClaudeExtraMCPServers returns FlowPilot-managed MCP servers to merge into a
// Claude turn's --mcp-config (keyed by server name). claude is launched with
// --strict-mcp-config and only the flowpilot permission server in its --mcp-config, so it
// ignores the google-drive server that EnsureGoogleDriveMcpProviderConfig wrote to the
// account's .claude.json. To give claude the same Google Drive access Codex gets natively,
// we re-derive that server here and hand it back for merging.
//
// Gate: only inject when the user actually configured google-drive for Claude (an entry is
// present in .claude.json). The server is recomputed with the turn's yolo + fresh proxy
// OAuth/runtime so its approval mode and refresh token stay current (the on-disk entry may
// have been written under a different yolo). Best-effort: any failure returns no extras so a
// turn never breaks over an optional MCP.
func (r *Runner) flowpilotClaudeExtraMCPServers(accountHomePath string, yolo bool) map[string]claudeMcpServer {
	out := map[string]claudeMcpServer{}

	// Task-234 T-1 + G2: jira/firebase/telegram are merged the same way
	// regardless of accountHomePath — derived from "integration connected"
	// (runner keyring). Shared by Claude (--mcp-config) and Grok ACP
	// (grokACPExtraMCPServers forwards both stdio and HTTP shapes).
	if server, ok := r.jiraLiveMCPServer(); ok {
		out[jiraMcpServerName] = server
	}
	if server, ok := r.firebaseLiveMCPServer(); ok {
		out[firebaseMcpServerName] = server
	}
	if server, ok := r.telegramLiveMCPServer(); ok {
		out[telegramMcpServerName] = server
	}

	accountHomePath = strings.TrimSpace(accountHomePath)
	if accountHomePath == "" {
		return out
	}

	configPath := filepath.Join(accountHomePath, ".claude.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return out
	}
	var cfg claudeConfig
	if json.Unmarshal(raw, &cfg) != nil {
		return out
	}
	existing, ok := cfg.McpServers[googleDriveMcpServerName]
	if !ok {
		return out // user did not configure Google Drive for Claude
	}

	// Parse the configured mode; for a non-proxy/legacy shape pass the entry through as-is.
	_, _, mode, _, parsed := parseGoogleDriveProxyMcpInvocation(existing.Command, existing.Args)
	if !parsed {
		out[googleDriveMcpServerName] = existing
		return out
	}

	configFile, ferr := r.loadGoogleDriveWorkspaceConfigFile()
	if ferr != nil && !errors.Is(ferr, os.ErrNotExist) {
		out[googleDriveMcpServerName] = existing
		return out
	}
	mcpStatus := r.resolveGoogleDriveMcpStatus(configFile)
	if !mcpStatus.Configured {
		// Auth no longer configured — skip rather than launch a server that will fail auth.
		return out
	}
	runtime := googleDriveMcpStatusRuntimeConfig(mcpStatus)
	r.hydrateGoogleDriveProxyOAuthRuntimeConfig(&runtime)

	out[googleDriveMcpServerName] = expectedClaudeGoogleDriveMcpServer(r.workspace, accountHomePath, mode, yolo, runtime)
	return out
}

// resolveGoogleDriveMcpProviderStatuses checks provider config status for all providers
func (r *Runner) resolveGoogleDriveMcpProviderStatuses() ([]GoogleDriveMcpProviderConfigStatus, error) {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to get Google Drive workspace config: %w", err)
	}
	mcpStatus := r.resolveGoogleDriveMcpStatus(configFile)
	runtimeMcpStatus := googleDriveMcpStatusRuntimeConfig(mcpStatus)
	r.hydrateGoogleDriveProxyOAuthRuntimeConfig(&runtimeMcpStatus)

	statuses := make([]GoogleDriveMcpProviderConfigStatus, 0, 4)
	now := time.Now().UTC().Format(time.RFC3339)
	accounts, _ := r.ListProviderAccounts()

	// Check each provider
	for _, providerKey := range []string{"codex", "gemini", "claude", "grok"} {
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
	case "grok":
		return filepath.Join(accountHomePath, "config.toml")
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
	case "grok":
		return r.checkGrokGoogleDriveMcpConfig(result, configPath, mcpStatus)
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

// checkGrokGoogleDriveMcpConfig checks Grok config.toml for google-drive MCP. See the
// grokMcpServer comment: the document is decoded as a generic map so unrelated sections
// aren't mistaken for parse failures.
func (r *Runner) checkGrokGoogleDriveMcpConfig(
	result GoogleDriveMcpProviderConfigStatus,
	configPath string,
	mcpStatus googleDriveMcpRuntimeConfig,
) (GoogleDriveMcpProviderConfigStatus, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return result, fmt.Errorf("failed to read Grok config: %w", err)
	}

	doc := map[string]interface{}{}
	if err := toml.Unmarshal(raw, &doc); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("invalid TOML config: %w", err)
	}

	mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
	rawServer, exists := mcpServers[googleDriveMcpServerName]
	if !exists {
		result.Status = "not_started"
		return result, nil
	}
	serverMap, ok := rawServer.(map[string]interface{})
	if !ok {
		result.Status = "failed"
		return result, fmt.Errorf("invalid Grok mcp_servers.%s entry", googleDriveMcpServerName)
	}
	server, ok := grokServerFromMap(serverMap)
	if !ok {
		result.Status = "failed"
		return result, fmt.Errorf("invalid Grok mcp_servers.%s entry", googleDriveMcpServerName)
	}
	result = applyDetectedServerStatus(result, server.Command, server.Args, "")

	// Check if paths match current runtime config
	if detectStaleGrokConfig(server, r.workspace, result.AccountHomePath, mcpStatus) {
		result.Status = "config_stale"
		return result, nil
	}

	result.Status = "configured"
	return result, nil
}

// detectStaleGrokConfig detects if Grok config drifts from the expected MCP server shape.
func detectStaleGrokConfig(server grokMcpServer, workspace string, accountHomePath string, mcpStatus googleDriveMcpRuntimeConfig) bool {
	parsedWorkspace, parsedAccountHome, parsedMode, parsedYoloMode, ok := parseGoogleDriveProxyMcpInvocation(server.Command, server.Args)
	if !ok {
		return true
	}
	if strings.TrimSpace(filepath.Clean(workspace)) != parsedWorkspace || strings.TrimSpace(accountHomePath) != parsedAccountHome {
		return true
	}
	expected := expectedGrokGoogleDriveMcpServer(workspace, accountHomePath, parsedMode, parsedYoloMode, mcpStatus)
	expected.Command = server.Command
	expected.Args = append([]string(nil), server.Args...)
	return !grokServerConfigMatches(server, expected)
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

	case "grok":
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return false, err
		}
		doc := map[string]interface{}{}
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return false, nil // If invalid, not necessarily stale
		}
		mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
		rawServer, exists := mcpServers[googleDriveMcpServerName]
		if !exists {
			return false, nil // Not configured
		}
		serverMap, ok := rawServer.(map[string]interface{})
		if !ok {
			return false, nil
		}
		server, ok := grokServerFromMap(serverMap)
		if !ok {
			return false, nil
		}
		return detectStaleGrokConfig(server, "", filepath.Dir(configPath), mcpStatus), nil

	default:
		return false, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

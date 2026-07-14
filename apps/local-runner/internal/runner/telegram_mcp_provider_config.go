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

// Task-232 (CP-05-05 P-1/P-2): connection layer + Claude provider-config
// injection for the Telegram Bot-API proxy MCP (telegram_proxy_mcp.go).
const telegramMcpServerName = "flowpilot_telegram"

// telegramCredential is the runner-managed secret for a connected Telegram
// integration — mirrors jiraCredential/firebaseCredential's shape/lifecycle.
type telegramCredential struct {
	BotToken  string `json:"botToken"`
	ChannelID string `json:"channelId"`
	// AutoApprove gates whether the proxy MCP (telegram_proxy_mcp.go) will
	// actually send — default false (Task-233 P-5: an irreversible,
	// outward-facing action must not send by default).
	AutoApprove bool `json:"autoApprove"`
}

func telegramCredentialKey(integrationID string) string {
	return normalizeSecretKey("telegram", integrationID)
}

func (r *Runner) saveTelegramCredential(integrationID string, creds telegramCredential) error {
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return r.ensureSecretStore().Set(telegramCredentialKey(integrationID), string(raw))
}

func (r *Runner) loadTelegramCredential(secretKey string) (telegramCredential, error) {
	if strings.TrimSpace(secretKey) == "" {
		return telegramCredential{}, errors.New("telegram bot token is not configured")
	}
	raw, err := r.ensureSecretStore().Get(secretKey)
	if err != nil {
		return telegramCredential{}, err
	}
	var creds telegramCredential
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return telegramCredential{}, err
	}
	return creds, nil
}

func (r *Runner) deleteTelegramCredential(integrationID string) error {
	return r.ensureSecretStore().Delete(telegramCredentialKey(integrationID))
}

// resolveConnectedTelegramCredential mirrors resolveConnectedJiraCredential/
// resolveConnectedFirebaseCredential: one active Telegram connection per
// workspace/runner.
func (r *Runner) resolveConnectedTelegramCredential() (telegramCredential, error) {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return telegramCredential{}, fmt.Errorf("load MCP backend records: %w", err)
	}
	secretKey := strings.TrimSpace(records["telegram"].SecretKey)
	if secretKey == "" {
		return telegramCredential{}, errors.New("telegram is not connected for this workspace")
	}
	return r.loadTelegramCredential(secretKey)
}

func validateTelegramCredentialFields(botToken, channelID string) error {
	if strings.TrimSpace(botToken) == "" {
		return errors.New("a bot token is required for Telegram")
	}
	if strings.TrimSpace(channelID) == "" {
		return errors.New("a channel id is required for Telegram")
	}
	return nil
}

// resolveTelegramCredential mirrors resolveJiraCredential's keyring fallback:
// a "test"/"retry" call against an ALREADY-CONNECTED integration can't resend
// the bot token, because SD-11 §6 strips secret fields (botToken) from
// Supabase config_encrypted before it's ever read back — the desktop's "Test"
// button on an existing integration row resends that stripped config
// verbatim. Without this fallback, re-testing an already-connected Telegram
// integration always failed with "a bot token is required for Telegram" even
// though a token was pasted at Create time.
//
// AutoApprove uses *bool on the request: nil leaves the stored flag alone
// (plain re-Test), non-nil sets it (Create / Save with checkbox, or the
// dedicated Enable/Disable auto-approve control). The flag lives only in the
// runner keyring credential JSON — not Supabase secrets (SD-11 §6).
func (r *Runner) resolveTelegramCredential(integrationID string, request IntegrationConnectionRequest) (telegramCredential, error) {
	secretKey := telegramCredentialKey(integrationID)
	botToken := strings.TrimSpace(request.BotToken)
	channelID := strings.TrimSpace(request.ChannelID)

	if botToken != "" && channelID != "" {
		autoApprove := false
		if request.TelegramAutoApprove != nil {
			autoApprove = *request.TelegramAutoApprove
		}
		return telegramCredential{
			BotToken:    botToken,
			ChannelID:   channelID,
			AutoApprove: autoApprove,
		}, nil
	}

	creds, err := r.loadTelegramCredential(secretKey)
	if err != nil {
		return telegramCredential{}, errors.New("a bot token and channel id are required for Telegram")
	}
	if botToken != "" {
		creds.BotToken = botToken
	}
	if channelID != "" {
		creds.ChannelID = channelID
	}
	if request.TelegramAutoApprove != nil {
		creds.AutoApprove = *request.TelegramAutoApprove
	}
	if err := validateTelegramCredentialFields(creds.BotToken, creds.ChannelID); err != nil {
		return telegramCredential{}, err
	}
	return creds, nil
}

func (r *Runner) detectTelegramBackend(record mcpBackendRecord) McpBackend {
	backend := McpBackend{
		Key:          "telegram",
		ProviderType: "telegram",
		Label:        "Telegram MCP",
		Transport:    "launcher",
		Launcher:     "flowpilot",
		Command:      "flowpilot telegram-mcp",
		InstallHint:  "Add a Telegram bot token and channel id so the runner can connect the Telegram MCP.",
		State:        "missing",
		Action:       "install",
		ActionLabel:  "Create MCP",
		SecretKey:    record.SecretKey,
	}
	if strings.TrimSpace(record.SecretKey) == "" {
		backend.LastError = "No Telegram bot token is stored for the runner yet."
		return backend
	}
	creds, err := r.loadTelegramCredential(record.SecretKey)
	if err != nil {
		backend.LastError = err.Error()
		return backend
	}
	backend.Installed = true
	backend.State = "installed"
	backend.Action = "verify"
	backend.ActionLabel = "Verify"
	backend.LastCheckedAt = record.LastCheckedAt
	backend.LastError = record.LastError
	if strings.TrimSpace(creds.ChannelID) != "" {
		backend.InstallHint = fmt.Sprintf("Connected to Telegram channel %s.", creds.ChannelID)
	}
	return backend
}

// TelegramMcpProviderConfigResponse mirrors JiraMcpProviderConfigResponse's shape.
type TelegramMcpProviderConfigResponse struct {
	ProviderKey string `json:"providerKey"`
	ServerName  string `json:"serverName"`
	Status      string `json:"status"`
	Changed     bool   `json:"changed"`
	ConfigPath  string `json:"configPath"`
}

// expectedClaudeTelegramMcpServer launches the FlowPilot binary's own
// `telegram-mcp` subcommand (mirrors the google-drive-mcp subcommand
// pattern) rather than an `npx` package — this proxy IS FlowPilot code.
func expectedClaudeTelegramMcpServer(flowpilotBinaryPath string) claudeMcpServer {
	return claudeMcpServer{
		Type:    "stdio",
		Command: flowpilotBinaryPath,
		Args:    []string{"telegram-mcp"},
	}
}

// EnsureClaudeTelegramMcpConfig writes/updates Claude's `mcpServers.telegram`
// stdio entry, reusing the existing typed claudeConfig/claudeMcpServer
// structs (same pattern as EnsureClaudeFirebaseMcpConfig).
func (r *Runner) EnsureClaudeTelegramMcpConfig(accountHomePath string) (TelegramMcpProviderConfigResponse, error) {
	accountHomePath = strings.TrimSpace(accountHomePath)
	if accountHomePath == "" {
		return TelegramMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	if _, err := r.resolveConnectedTelegramCredential(); err != nil {
		return TelegramMcpProviderConfigResponse{}, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("resolve flowpilot binary path: %w", err)
	}

	configPath := filepath.Join(accountHomePath, ".claude.json")
	var config claudeConfig
	changed := false

	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to read Claude config: %w", readErr)
	}
	if readErr == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			config = claudeConfig{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]claudeMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(telegramMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := expectedClaudeTelegramMcpServer(binaryPath)
	existing, exists := config.McpServers[telegramMcpServerName]
	if !exists || !claudeServerConfigMatches(existing, expected) {
		config.McpServers[telegramMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return TelegramMcpProviderConfigResponse{
		ProviderKey: "claude",
		ServerName:  telegramMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// TelegramMcpProviderConfigRequest is the HTTP-facing request for
// EnsureTelegramMcpProviderConfig, mirroring JiraMcpProviderConfigRequest.
type TelegramMcpProviderConfigRequest struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
}

// EnsureTelegramMcpProviderConfig is the dispatch entry point mirroring
// EnsureJiraMcpProviderConfig — the production call site that was missing
// entirely before this change (only EnsureClaudeTelegramMcpConfig existed,
// called from tests only). All four providers are wired — Telegram is a
// local stdio launcher (this repo's own binary), so the same shape proven
// for Claude round-trips cleanly into Codex/Gemini/Grok's config formats.
func (r *Runner) EnsureTelegramMcpProviderConfig(req TelegramMcpProviderConfigRequest) (TelegramMcpProviderConfigResponse, error) {
	providerKey := strings.ToLower(strings.TrimSpace(req.ProviderKey))
	if providerKey == "" {
		return TelegramMcpProviderConfigResponse{}, errors.New("providerKey is required")
	}
	accountHomePath := strings.TrimSpace(req.AccountHomePath)
	if accountHomePath == "" {
		return TelegramMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	switch providerKey {
	case "claude":
		return r.EnsureClaudeTelegramMcpConfig(accountHomePath)
	case "codex":
		return r.ensureCodexTelegramMcpConfig(accountHomePath)
	case "gemini":
		return r.ensureGeminiTelegramMcpConfig(accountHomePath)
	case "grok":
		return r.ensureGrokTelegramMcpConfig(accountHomePath)
	default:
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("Telegram MCP provider config is not yet implemented for provider: %s", providerKey)
	}
}

// telegramLiveMCPServer builds the per-turn claudeMcpServer entry for
// Telegram when connected, for flowpilotClaudeExtraMCPServers (Task-234
// T-1). Its stdio shape reaches Grok automatically too via
// grokACPExtraMCPServers.
func (r *Runner) telegramLiveMCPServer() (claudeMcpServer, bool) {
	if _, err := r.resolveConnectedTelegramCredential(); err != nil {
		return claudeMcpServer{}, false
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return claudeMcpServer{}, false
	}
	return expectedClaudeTelegramMcpServer(binaryPath), true
}

func (r *Runner) ensureCodexTelegramMcpConfig(accountHomePath string) (TelegramMcpProviderConfigResponse, error) {
	if _, err := r.resolveConnectedTelegramCredential(); err != nil {
		return TelegramMcpProviderConfigResponse{}, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("resolve flowpilot binary path: %w", err)
	}
	configPath := filepath.Join(accountHomePath, "config.toml")

	var config codexConfig
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to read Codex config: %w", readErr)
	}
	if readErr == nil {
		if tomlErr := toml.Unmarshal(raw, &config); tomlErr != nil {
			config = codexConfig{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]codexMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(telegramMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := codexMcpServer{
		Command:           binaryPath,
		Args:              []string{"telegram-mcp"},
		StartupTimeoutSec: 20,
		ToolTimeoutSec:    120,
		Enabled:           true,
	}
	existing, exists := config.McpServers[telegramMcpServerName]
	if !exists || !codexServerConfigMatches(existing, expected) {
		config.McpServers[telegramMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(config)
		if err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return TelegramMcpProviderConfigResponse{
		ProviderKey: "codex",
		ServerName:  telegramMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

func (r *Runner) ensureGeminiTelegramMcpConfig(accountHomePath string) (TelegramMcpProviderConfigResponse, error) {
	if _, err := r.resolveConnectedTelegramCredential(); err != nil {
		return TelegramMcpProviderConfigResponse{}, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("resolve flowpilot binary path: %w", err)
	}
	configDir := filepath.Join(accountHomePath, ".gemini")
	configPath := filepath.Join(configDir, "settings.json")

	var config geminiSettings
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to read Gemini config: %w", readErr)
	}
	if readErr == nil {
		if jsonErr := json.Unmarshal(raw, &config); jsonErr != nil {
			config = geminiSettings{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]geminiMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(telegramMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := geminiMcpServer{
		Command: binaryPath,
		Args:    []string{"telegram-mcp"},
		Timeout: 600000,
		Trust:   false,
	}
	existing, exists := config.McpServers[telegramMcpServerName]
	if !exists || !geminiServerConfigMatches(existing, expected) {
		config.McpServers[telegramMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return TelegramMcpProviderConfigResponse{
		ProviderKey: "gemini",
		ServerName:  telegramMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

func (r *Runner) ensureGrokTelegramMcpConfig(accountHomePath string) (TelegramMcpProviderConfigResponse, error) {
	if _, err := r.resolveConnectedTelegramCredential(); err != nil {
		return TelegramMcpProviderConfigResponse{}, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("resolve flowpilot binary path: %w", err)
	}
	configPath := filepath.Join(accountHomePath, "config.toml")

	doc := map[string]interface{}{}
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to read Grok config: %w", readErr)
	}
	if readErr == nil {
		if tomlErr := toml.Unmarshal(raw, &doc); tomlErr != nil {
			doc = map[string]interface{}{}
			changed = true
		}
	}

	mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(telegramMcpServerName); legacy != "" {
		if _, ok := mcpServers[legacy]; ok {
			delete(mcpServers, legacy)
			doc["mcp_servers"] = mcpServers
			changed = true
		}
	}

	expected := grokMcpServer{
		Command: binaryPath,
		Args:    []string{"telegram-mcp"},
		Enabled: true,
	}
	existingMatches := false
	if existingRaw, exists := mcpServers[telegramMcpServerName]; exists {
		if existingMap, ok := existingRaw.(map[string]interface{}); ok {
			if existingServer, ok := grokServerFromMap(existingMap); ok {
				existingMatches = grokServerConfigMatches(existingServer, expected)
			}
		}
	}
	if !existingMatches {
		expectedMap, mapErr := grokServerToMap(expected)
		if mapErr != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to build Grok MCP server entry: %w", mapErr)
		}
		mcpServers[telegramMcpServerName] = expectedMap
		doc["mcp_servers"] = mcpServers
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(doc)
		if err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return TelegramMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return TelegramMcpProviderConfigResponse{
		ProviderKey: "grok",
		ServerName:  telegramMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// checkClaudeTelegramMcpConfig is a read-only staleness check, mirroring
// checkClaudeFirebaseMcpConfig.
func (r *Runner) checkClaudeTelegramMcpConfig(accountHomePath string) (configured bool, stale bool, err error) {
	configPath := filepath.Join(strings.TrimSpace(accountHomePath), ".claude.json")
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to read Claude config: %w", readErr)
	}
	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return false, false, fmt.Errorf("failed to parse Claude config: %w", err)
	}
	existing, ok := config.McpServers[telegramMcpServerName]
	if !ok {
		return false, false, nil
	}
	if existing.Type != "stdio" || strings.TrimSpace(existing.Command) == "" || len(existing.Args) == 0 || existing.Args[0] != "telegram-mcp" {
		return true, true, nil
	}
	return true, false, nil
}

// PreflightTelegramMcp mirrors PreflightFirebaseMcp: checks Telegram is
// connected, then (when a provider/account is given) whether that
// provider's config has a non-stale telegram MCP server entry.
func (r *Runner) PreflightTelegramMcp(providerKey string, accountHomePath string) MCPPreflightCheck {
	result := MCPPreflightCheck{}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to check Telegram MCP status: %v", err)
		return result
	}
	backend := r.detectTelegramBackend(records["telegram"])
	if !backend.Installed {
		if strings.TrimSpace(backend.LastError) != "" {
			result.ErrorMessage = backend.LastError
		} else {
			result.ErrorMessage = "Telegram is not connected yet. Add a bot token and channel id in MCP Servers settings."
		}
		return result
	}
	result.GoogleDriveReady = true

	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	accountHomePath = strings.TrimSpace(accountHomePath)
	if providerKey == "" || accountHomePath == "" {
		result.ProviderConfigured = true
		return result
	}

	switch providerKey {
	case "claude":
		configured, stale, err := r.checkClaudeTelegramMcpConfig(accountHomePath)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to check Claude Telegram MCP config: %v", err)
			return result
		}
		if !configured {
			result.ErrorMessage = "The selected AI provider is not configured with the telegram MCP server. Run Configure Providers first."
			return result
		}
		if stale {
			result.ErrorMessage = "Provider has stale Telegram MCP config. Re-run Configure Providers."
			return result
		}
	default:
		result.ErrorMessage = fmt.Sprintf("Telegram MCP provider config is not yet implemented for provider: %s", providerKey)
		return result
	}

	result.ProviderConfigured = true
	return result
}

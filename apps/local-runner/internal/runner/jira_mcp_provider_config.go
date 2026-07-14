package runner

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Task-228 (CP-05-06 P-1/P-2) + Jira-MCP-API-Token-Auth: Jira connects through
// Atlassian's official Rovo remote MCP server — not a runner-launched local
// process. Auth primary path is personal API token (Basic email:apiToken →
// /v1/mcp) per Atlassian headless docs; optional Bearer override for service
// accounts / OAuth-style paste uses /v1/mcp/authv2.
const (
	jiraMcpServerName = "jira"
	// jiraMcpAPITokenRemoteURL is the official personal-API-token / headless endpoint.
	jiraMcpAPITokenRemoteURL = "https://mcp.atlassian.com/v1/mcp"
	// jiraMcpOAuthRemoteURL is the authv2 endpoint used for Bearer override path.
	jiraMcpOAuthRemoteURL = "https://mcp.atlassian.com/v1/mcp/authv2"
	// Deprecated alias kept so older test strings / comments that referenced a
	// single URL still compile if anything imported it — prefer the two consts above.
	jiraMcpRemoteURL = jiraMcpOAuthRemoteURL
)

// jiraMcpAuthConfig is the resolved remote-MCP URL + Authorization header value
// written into every provider config and the per-turn live merge.
type jiraMcpAuthConfig struct {
	Authorization string
	URL           string
	// Mode is "basic" (personal API token) or "bearer" (explicit override).
	Mode string
}

// resolveJiraMcpAuth picks Authorization + URL for Rovo MCP.
// Precedence:
//  1. non-empty requestBearer (persisted as override)
//  2. persisted creds.BearerToken
//  3. connected Email+ApiToken → Basic base64(email:apiToken) + /v1/mcp
func (r *Runner) resolveJiraMcpAuth(requestBearer string) (jiraMcpAuthConfig, error) {
	requestBearer = strings.TrimSpace(requestBearer)
	if requestBearer != "" {
		if err := r.persistConnectedJiraBearerToken(requestBearer); err != nil {
			return jiraMcpAuthConfig{}, fmt.Errorf("persist jira bearer token: %w", err)
		}
		return jiraMcpAuthConfig{
			Authorization: normalizeJiraBearerAuthorization(requestBearer),
			URL:           jiraMcpOAuthRemoteURL,
			Mode:          "bearer",
		}, nil
	}
	if token, err := r.resolveConnectedJiraBearerToken(); err == nil && strings.TrimSpace(token) != "" {
		return jiraMcpAuthConfig{
			Authorization: normalizeJiraBearerAuthorization(token),
			URL:           jiraMcpOAuthRemoteURL,
			Mode:          "bearer",
		}, nil
	}
	creds, err := r.resolveConnectedJiraCredential()
	if err != nil {
		return jiraMcpAuthConfig{}, err
	}
	email := strings.TrimSpace(creds.Email)
	apiToken := strings.TrimSpace(creds.ApiToken)
	if email == "" || apiToken == "" {
		return jiraMcpAuthConfig{}, errors.New("jira MCP auth requires a connected email+apiToken (or an optional Authorization override)")
	}
	basic := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
	return jiraMcpAuthConfig{
		Authorization: "Basic " + basic,
		URL:           jiraMcpAPITokenRemoteURL,
		Mode:          "basic",
	}, nil
}

// normalizeJiraBearerAuthorization leaves values that already start with
// "Basic " or "Bearer " alone; bare tokens get a Bearer prefix (service-account
// / OAuth access token paste).
func normalizeJiraBearerAuthorization(raw string) string {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "bearer ") || strings.HasPrefix(lower, "basic ") {
		return raw
	}
	return "Bearer " + raw
}

// JiraMcpProviderConfigResponse mirrors GoogleDriveMcpProviderConfigResponse's
// shape (ProviderKey/ServerName/Status/Changed/ConfigPath) so callers already
// familiar with the Drive provider-config contract can read this the same
// way (CP-05-03 §11.7's Ensure*/Response shape).
type JiraMcpProviderConfigResponse struct {
	ProviderKey string `json:"providerKey"`
	ServerName  string `json:"serverName"`
	Status      string `json:"status"`
	Changed     bool   `json:"changed"`
	ConfigPath  string `json:"configPath"`
}

// claudeRemoteMcpServer is Claude Code's documented remote-MCP-server config
// shape (`type: "http"` + `url` + optional `headers`) — distinct from the
// stdio `claudeMcpServer` struct in google_drive_mcp_provider_config.go
// (Command/Args/Env), which this file deliberately does not touch or reuse:
// mixing a remote-only field set into that stdio-only struct would leak
// unused fields into Google Drive's own config writes.
type claudeRemoteMcpServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

// EnsureClaudeJiraMcpConfig writes/updates Claude's `mcpServers.jira` entry
// to point at Atlassian's remote MCP endpoint, preserving every other key in
// the config document (including any `mcpServers.google-drive` entry) via a
// generic-map splice — the same "touch only our one sub-key" discipline
// google_drive_mcp_provider_config.go uses for Grok's config.toml, applied
// here to Claude's JSON document instead of assuming the whole document
// matches the typed claudeConfig struct.
//
// EnsureClaudeJiraMcpConfig writes mcpServers.jira for Claude. Prefer
// EnsureJiraMcpProviderConfig which resolves Basic vs Bearer. This helper
// still picks the URL from the Authorization value so direct callers cannot
// put Bearer tokens on the API-token endpoint (or Basic on authv2).
func (r *Runner) EnsureClaudeJiraMcpConfig(accountHomePath string, authHeaderValue string) (JiraMcpProviderConfigResponse, error) {
	return r.ensureClaudeJiraMcpConfigWithURL(accountHomePath, authHeaderValue, jiraRemoteURLForAuthorization(authHeaderValue))
}

// jiraRemoteURLForAuthorization maps Basic → /v1/mcp, Bearer/other → /authv2.
func jiraRemoteURLForAuthorization(authHeaderValue string) string {
	lower := strings.ToLower(strings.TrimSpace(authHeaderValue))
	if strings.HasPrefix(lower, "basic ") {
		return jiraMcpAPITokenRemoteURL
	}
	return jiraMcpOAuthRemoteURL
}

func (r *Runner) ensureClaudeJiraMcpConfigWithURL(accountHomePath string, authHeaderValue string, remoteURL string) (JiraMcpProviderConfigResponse, error) {
	accountHomePath = strings.TrimSpace(accountHomePath)
	if accountHomePath == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	authHeaderValue = strings.TrimSpace(authHeaderValue)
	if authHeaderValue == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("authHeaderValue is required")
	}
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		remoteURL = jiraMcpAPITokenRemoteURL
	}

	configPath := filepath.Join(accountHomePath, ".claude.json")

	doc := map[string]any{}
	raw, err := os.ReadFile(configPath)
	changed := false
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to read Claude config: %w", err)
		}
		changed = true
	} else if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to parse Claude config: %w", err)
		}
	}

	mcpServersRaw, _ := doc["mcpServers"].(map[string]any)
	if mcpServersRaw == nil {
		mcpServersRaw = map[string]any{}
	}

	expected := claudeRemoteMcpServer{
		Type:    "http",
		URL:     remoteURL,
		Headers: map[string]string{"Authorization": authHeaderValue},
	}

	if existing, ok := mcpServersRaw[jiraMcpServerName]; !ok || !claudeRemoteServerMatches(existing, expected) {
		expectedMap, marshalErr := remoteServerToMap(expected)
		if marshalErr != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to build jira MCP server entry: %w", marshalErr)
		}
		mcpServersRaw[jiraMcpServerName] = expectedMap
		changed = true
	}
	doc["mcpServers"] = mcpServersRaw

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal Claude config: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to write Claude config: %w", err)
		}
	}

	return JiraMcpProviderConfigResponse{
		ProviderKey: "claude",
		ServerName:  jiraMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// claudeJiraMcpConfigStatus is what PreflightJiraMcp needs to know about an
// existing Claude config: does mcpServers.jira exist, and does it still
// point at the expected remote endpoint (stale detection mirrors
// detectStaleClaudeConfig's URL/command comparison for Drive).
type claudeJiraMcpConfigStatus struct {
	Configured bool
	Stale      bool
}

// JiraMcpProviderConfigRequest is the HTTP-facing request for
// EnsureJiraMcpProviderConfig. BearerToken is an *optional* Authorization
// override (service-account Bearer or OAuth access token). When empty, the
// runner derives Basic auth from the connected email+apiToken (official
// Rovo MCP personal API token path).
type JiraMcpProviderConfigRequest struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
	BearerToken     string `json:"bearerToken"`
}

// EnsureJiraMcpProviderConfig writes Rovo remote MCP into the selected
// provider account. Primary auth = connected API token Basic → /v1/mcp;
// optional BearerToken override → authv2. G2 Grok HTTP path preserved.
func (r *Runner) EnsureJiraMcpProviderConfig(req JiraMcpProviderConfigRequest) (JiraMcpProviderConfigResponse, error) {
	providerKey := strings.ToLower(strings.TrimSpace(req.ProviderKey))
	if providerKey == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("providerKey is required")
	}
	accountHomePath := strings.TrimSpace(req.AccountHomePath)
	if accountHomePath == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}

	auth, err := r.resolveJiraMcpAuth(req.BearerToken)
	if err != nil {
		return JiraMcpProviderConfigResponse{}, err
	}

	switch providerKey {
	case "claude":
		return r.ensureClaudeJiraMcpConfigWithURL(accountHomePath, auth.Authorization, auth.URL)
	case "codex":
		return r.ensureCodexJiraMcpConfig(accountHomePath, auth)
	case "gemini":
		return r.ensureGeminiJiraMcpConfig(accountHomePath, auth)
	case "grok":
		return r.ensureGrokJiraMcpConfig(accountHomePath, auth)
	default:
		return JiraMcpProviderConfigResponse{}, fmt.Errorf("Jira MCP provider config is not yet implemented for provider: %s", providerKey)
	}
}

// persistConnectedJiraBearerToken stores the Atlassian OAuth bearer token
// onto the same keyring record resolveConnectedJiraCredential already reads
// (the "one connected Jira integration per workspace" model), so it survives
// across provider-config calls and per-turn merges without re-entry.
func (r *Runner) persistConnectedJiraBearerToken(token string) error {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return fmt.Errorf("load MCP backend records: %w", err)
	}
	secretKey := strings.TrimSpace(records["jira"].SecretKey)
	if secretKey == "" {
		return errors.New("jira is not connected for this workspace")
	}
	creds, err := r.loadJiraCredential(secretKey)
	if err != nil {
		return err
	}
	creds.BearerToken = strings.TrimSpace(token)
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return r.ensureSecretStore().Set(secretKey, string(raw))
}

// resolveConnectedJiraBearerToken reads back the token persistConnectedJiraBearerToken
// stored, for the per-turn merge and for idempotent re-runs of EnsureJiraMcpProviderConfig.
func (r *Runner) resolveConnectedJiraBearerToken() (string, error) {
	creds, err := r.resolveConnectedJiraCredential()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(creds.BearerToken), nil
}

// jiraLiveMCPServer builds the per-turn claudeMcpServer entry for Jira when
// connected credentials can resolve Rovo MCP auth (Basic apiToken or Bearer
// override), for flowpilotClaudeExtraMCPServers + Grok ACP HTTP forward.
func (r *Runner) jiraLiveMCPServer() (claudeMcpServer, bool) {
	auth, err := r.resolveJiraMcpAuth("")
	if err != nil || auth.Authorization == "" {
		return claudeMcpServer{}, false
	}
	return claudeMcpServer{
		Type:    "http",
		URL:     auth.URL,
		Headers: map[string]string{"Authorization": auth.Authorization},
	}, true
}

// ensureCodexJiraMcpConfig writes Codex mcp_servers.jira. Bearer override uses
// bearer_token_env_var (Codex prepends Bearer semantics). Basic personal API
// token uses http_headers with the full Authorization value so Codex does not
// wrap "Basic …" as a Bearer token.
func (r *Runner) ensureCodexJiraMcpConfig(accountHomePath string, auth jiraMcpAuthConfig) (JiraMcpProviderConfigResponse, error) {
	configPath := filepath.Join(accountHomePath, "config.toml")

	var config codexConfig
	changed := false
	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to read Codex config: %w", err)
	}
	if err == nil {
		if tomlErr := toml.Unmarshal(raw, &config); tomlErr != nil {
			config = codexConfig{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]codexMcpServer)
	}

	var expected codexMcpServer
	if auth.Mode == "basic" {
		expected = codexMcpServer{
			Enabled:     true,
			URL:         auth.URL,
			HTTPHeaders: map[string]string{"Authorization": auth.Authorization},
		}
	} else {
		// Strip leading "Bearer " for env var — Codex bearer_token_env_var adds it.
		token := strings.TrimSpace(auth.Authorization)
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		expected = codexMcpServer{
			Enabled:           true,
			URL:               auth.URL,
			BearerTokenEnvVar: "JIRA_BEARER_TOKEN",
			Env:               map[string]string{"JIRA_BEARER_TOKEN": token},
		}
	}
	existing, exists := config.McpServers[jiraMcpServerName]
	if !exists || existing.URL != expected.URL || existing.BearerTokenEnvVar != expected.BearerTokenEnvVar || !envMatches(existing.Env, expected.Env) || !envMatches(existing.HTTPHeaders, expected.HTTPHeaders) {
		config.McpServers[jiraMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(config)
		if err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return JiraMcpProviderConfigResponse{
		ProviderKey: "codex",
		ServerName:  jiraMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// ensureGrokJiraMcpConfig writes Grok config.toml [mcp_servers.jira] with
// url + headers.Authorization (Basic or Bearer) — G2 generic-map splice.
func (r *Runner) ensureGrokJiraMcpConfig(accountHomePath string, auth jiraMcpAuthConfig) (JiraMcpProviderConfigResponse, error) {
	configPath := filepath.Join(accountHomePath, "config.toml")

	doc := map[string]interface{}{}
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to read Grok config: %w", readErr)
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

	expected := grokMcpServer{
		Enabled: true,
		URL:     auth.URL,
		Headers: map[string]string{"Authorization": auth.Authorization},
	}
	existingMatches := false
	if existingRaw, exists := mcpServers[jiraMcpServerName]; exists {
		if existingMap, ok := existingRaw.(map[string]interface{}); ok {
			if existingServer, ok := grokServerFromMap(existingMap); ok {
				existingMatches = grokServerConfigMatches(existingServer, expected)
			}
			// Heal an older config that wrote stale stdio keys (command = "" /
			// args = []) onto this remote HTTP entry — the typed comparison
			// above treats an empty command as a match, so force a rewrite when
			// those keys are physically present so they get dropped.
			if _, hasCommand := existingMap["command"]; hasCommand {
				existingMatches = false
			}
			if _, hasArgs := existingMap["args"]; hasArgs {
				existingMatches = false
			}
		}
	}
	if !existingMatches {
		expectedMap, mapErr := grokServerToMap(expected)
		if mapErr != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to build Grok jira MCP server entry: %w", mapErr)
		}
		mcpServers[jiraMcpServerName] = expectedMap
		doc["mcp_servers"] = mcpServers
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(doc)
		if err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return JiraMcpProviderConfigResponse{
		ProviderKey: "grok",
		ServerName:  jiraMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// ensureGeminiJiraMcpConfig writes Gemini settings.json mcpServers.jira with
// httpUrl + headers.Authorization.
func (r *Runner) ensureGeminiJiraMcpConfig(accountHomePath string, auth jiraMcpAuthConfig) (JiraMcpProviderConfigResponse, error) {
	configDir := filepath.Join(accountHomePath, ".gemini")
	configPath := filepath.Join(configDir, "settings.json")

	var config geminiSettings
	changed := false
	raw, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to read Gemini config: %w", err)
	}
	if err == nil {
		if jsonErr := json.Unmarshal(raw, &config); jsonErr != nil {
			config = geminiSettings{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]geminiMcpServer)
	}

	expected := geminiMcpServer{HTTPURL: auth.URL, Headers: map[string]string{"Authorization": auth.Authorization}}
	existing, exists := config.McpServers[jiraMcpServerName]
	if !exists || existing.HTTPURL != expected.HTTPURL || existing.Headers["Authorization"] != auth.Authorization {
		config.McpServers[jiraMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return JiraMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return JiraMcpProviderConfigResponse{
		ProviderKey: "gemini",
		ServerName:  jiraMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// checkClaudeJiraMcpConfig reads (without writing) the current
// mcpServers.jira entry to answer "is Claude ready to use Jira MCP right
// now" — used by PreflightJiraMcp so preflight never has side effects.
func (r *Runner) checkClaudeJiraMcpConfig(accountHomePath string) (claudeJiraMcpConfigStatus, error) {
	configPath := filepath.Join(strings.TrimSpace(accountHomePath), ".claude.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return claudeJiraMcpConfigStatus{}, nil
		}
		return claudeJiraMcpConfigStatus{}, fmt.Errorf("failed to read Claude config: %w", err)
	}

	doc := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return claudeJiraMcpConfigStatus{}, fmt.Errorf("failed to parse Claude config: %w", err)
		}
	}
	mcpServersRaw, _ := doc["mcpServers"].(map[string]any)
	existing, ok := mcpServersRaw[jiraMcpServerName]
	if !ok {
		return claudeJiraMcpConfigStatus{}, nil
	}

	server, ok := remoteServerFromAny(existing)
	if !ok {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	if server.Type != "http" {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	// Accept either API-token (/v1/mcp) or Bearer override (/v1/mcp/authv2).
	if server.URL != jiraMcpAPITokenRemoteURL && server.URL != jiraMcpOAuthRemoteURL {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	if strings.TrimSpace(server.Headers["Authorization"]) == "" {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	return claudeJiraMcpConfigStatus{Configured: true, Stale: false}, nil
}

// remoteServerToMap round-trips a typed remote MCP server entry through JSON
// into a generic map, so it splices cleanly into a document decoded as
// map[string]any (mirrors grokServerToMap's generic-map-splice technique in
// google_drive_mcp_provider_config.go, applied to JSON instead of TOML).
func remoteServerToMap(server claudeRemoteMcpServer) (map[string]any, error) {
	raw, err := json.Marshal(server)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// remoteServerFromAny converts a generic mcpServers.jira entry back into a
// typed struct for comparison. ok=false means the entry doesn't decode as a
// remote MCP server shape at all (e.g. a stdio entry left by some other
// integration under the same key), which callers treat as stale/misconfigured
// rather than erroring.
func remoteServerFromAny(v any) (claudeRemoteMcpServer, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return claudeRemoteMcpServer{}, false
	}
	var server claudeRemoteMcpServer
	if err := json.Unmarshal(raw, &server); err != nil {
		return claudeRemoteMcpServer{}, false
	}
	return server, true
}

func claudeRemoteServerMatches(existing any, expected claudeRemoteMcpServer) bool {
	server, ok := remoteServerFromAny(existing)
	if !ok {
		return false
	}
	if server.Type != expected.Type || server.URL != expected.URL {
		return false
	}
	for key, value := range expected.Headers {
		if server.Headers[key] != value {
			return false
		}
	}
	return true
}

// PreflightJiraMcp checks whether Jira is connected (an existing credential
// per detectJiraBackend, CP-05-01's already-shipped connection flow) and,
// when a provider/account is given, whether that provider's config has a
// non-stale jira MCP server entry. It never writes config — only
// EnsureClaudeJiraMcpConfig does that — so preflight stays side-effect-free
// (CP-05-03 §11.11's preflight contract).
//
// Registered into mcpInstructionSpecs alongside google_drive
// (mcp_prompt_instructions.go) as a compile-time built-in, same as Google
// Drive — Jira's prompt/preflight pair is FlowPilot code, not a
// runtime-registered external adapter.
func (r *Runner) PreflightJiraMcp(providerKey string, accountHomePath string) MCPPreflightCheck {
	result := MCPPreflightCheck{}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to check Jira MCP status: %v", err)
		return result
	}
	backend := r.detectJiraBackend(records["jira"])
	if !backend.Installed {
		if strings.TrimSpace(backend.LastError) != "" {
			result.ErrorMessage = backend.LastError
		} else {
			result.ErrorMessage = "Jira is not connected yet. Open the Jira MCP form and connect a workspace."
		}
		return result
	}
	// MCPPreflightCheck's field name predates this generalization (Task-227) —
	// GoogleDriveReady is reused generically here to mean "this MCP is ready".
	result.GoogleDriveReady = true

	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	accountHomePath = strings.TrimSpace(accountHomePath)
	if providerKey == "" || accountHomePath == "" {
		result.ProviderConfigured = true
		return result
	}

	switch providerKey {
	case "claude":
		status, err := r.checkClaudeJiraMcpConfig(accountHomePath)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to check Claude Jira MCP config: %v", err)
			return result
		}
		if !status.Configured {
			result.ErrorMessage = "The selected AI provider is not configured with the jira MCP server. Run Configure Providers first."
			return result
		}
		if status.Stale {
			result.ErrorMessage = "Provider has stale Jira MCP config. Re-run Configure Providers."
			return result
		}
	case "grok":
		// G2: static config.toml is what Configure Providers writes; live ACP
		// also gets jira via grokACPExtraMCPServers, but preflight still
		// requires the on-disk entry so "Configure Providers" is the clear
		// setup step (mirrors Claude checking .claude.json).
		status, err := r.checkGrokJiraMcpConfig(accountHomePath)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to check Grok Jira MCP config: %v", err)
			return result
		}
		if !status.Configured {
			result.ErrorMessage = "The selected AI provider is not configured with the jira MCP server. Run Configure Providers first."
			return result
		}
		if status.Stale {
			result.ErrorMessage = "Provider has stale Jira MCP config. Re-run Configure Providers."
			return result
		}
	default:
		// codex/gemini Ensure writers exist; preflight for those providers is
		// still a follow-up (not G2). Empty providerKey already returned above.
		result.ErrorMessage = fmt.Sprintf("Jira MCP provider config is not yet implemented for provider: %s", providerKey)
		return result
	}

	result.ProviderConfigured = true
	return result
}

// checkGrokJiraMcpConfig is a read-only check of config.toml mcp_servers.jira
// (G2). Configured=true when the entry exists; Stale=true when it exists but
// URL is wrong, disabled, or Authorization header is empty.
func (r *Runner) checkGrokJiraMcpConfig(accountHomePath string) (claudeJiraMcpConfigStatus, error) {
	configPath := filepath.Join(strings.TrimSpace(accountHomePath), "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return claudeJiraMcpConfigStatus{}, nil
		}
		return claudeJiraMcpConfigStatus{}, fmt.Errorf("failed to read Grok config: %w", err)
	}
	doc := map[string]interface{}{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return claudeJiraMcpConfigStatus{}, fmt.Errorf("failed to parse Grok config: %w", err)
		}
	}
	mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
	if mcpServers == nil {
		return claudeJiraMcpConfigStatus{}, nil
	}
	existingRaw, ok := mcpServers[jiraMcpServerName]
	if !ok {
		return claudeJiraMcpConfigStatus{}, nil
	}
	existingMap, ok := existingRaw.(map[string]interface{})
	if !ok {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	server, ok := grokServerFromMap(existingMap)
	if !ok {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	if !server.Enabled || strings.TrimSpace(server.Headers["Authorization"]) == "" {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	if server.URL != jiraMcpAPITokenRemoteURL && server.URL != jiraMcpOAuthRemoteURL {
		return claudeJiraMcpConfigStatus{Configured: true, Stale: true}, nil
	}
	return claudeJiraMcpConfigStatus{Configured: true, Stale: false}, nil
}

// buildJiraMcpInstructions creates the "## Required MCP Usage" prompt block
// for Jira, mirroring buildGoogleDriveMcpInstructions's shape/rules
// (CP-05-03 §11.10) but scoped to read-only Jira operations (CP-05-06 P-7:
// read-only v1) and the failure codes CP-05-06 P-4's target-note contract
// expects (MCP_UNAVAILABLE, MCP_AUTH_REQUIRED, JIRA_CONTENT_NOT_FOUND).
func buildJiraMcpInstructions(providerKey string, allowWrite bool, yoloMode bool) string {
	var sb strings.Builder
	sb.WriteString("## Required MCP Usage\n\n")
	sb.WriteString("This workflow step requires FlowPilot MCP `jira`.\n")
	sb.WriteString("The configured provider MCP server name is `jira`.\n\n")
	sb.WriteString("This step is restricted to `read_only` Jira operations.\n")
	sb.WriteString("Before producing the final answer, use Jira MCP tools from `jira` to fetch the ticket/sprint context this run requires.\n\n")
	sb.WriteString("Atlassian bootstrap sequence:\n")
	sb.WriteString("- First call `getAccessibleAtlassianResources` and use the returned resource `id` as `cloudId`. Never send an empty `cloudId`.\n")
	sb.WriteString("- If you need the current Atlassian user, call `atlassianUserInfo` first and use its `account_id` as the Teamwork Graph `objectIdentifier`.\n")
	sb.WriteString("- Do not use `objectIdentifier: \"current\"` with `getTeamworkGraphContext`.\n")
	sb.WriteString("- Use Teamwork Graph tools only when you need relationships or linked entities. Prefer direct Jira issue/sprint/project tools for basic ticket reads.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Do not invent Jira ticket or sprint content.\n")
	sb.WriteString("- Only read the ticket(s)/sprint already selected for this run; do not run a broader Jira search.\n")
	sb.WriteString("- If `jira` is unavailable, stop and end the response with `MCP_FAILURE_CODE: MCP_UNAVAILABLE`.\n")
	sb.WriteString("- If auth is missing or expired, stop and end the response with `MCP_FAILURE_CODE: MCP_AUTH_REQUIRED`.\n")
	sb.WriteString("- If the required ticket or sprint cannot be found, end the response with `MCP_FAILURE_CODE: JIRA_CONTENT_NOT_FOUND`.\n")
	sb.WriteString("- Include the issue key (e.g. `SCRUM-123`) for every Jira item used.\n")
	sb.WriteString("- Destructive or write operations are not allowed in this MCP server (read-only v1).\n")
	return sb.String()
}

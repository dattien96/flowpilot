package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Task-228 (CP-05-06 P-1/P-2): Jira connects through Atlassian's official
// remote MCP server (SD-11 §3.2), not a runner-launched local process — so
// unlike Google Drive's stdio launcher config, the provider-config entry
// here is a *remote* MCP server (URL + auth header), and the runner never
// spawns anything for it.
const (
	jiraMcpServerName = "jira"
	jiraMcpRemoteURL  = "https://mcp.atlassian.com/v1/mcp/authv2"
)

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
// authHeaderValue is the exact `Authorization` header value to send with
// every request to the remote MCP endpoint (e.g. "Bearer <oauth-token>").
// Task-228 ships the provider-config plumbing only: acquiring a real
// Atlassian OAuth bearer token requires a registered Atlassian OAuth app and
// a live browser redirect, which this runner cannot stand up or verify in
// this environment — that handshake is a separate follow-up. Until it lands,
// callers should treat an empty authHeaderValue as "not ready" and avoid
// calling this with one (see PreflightJiraMcp, which gates on Jira being
// connected before provider config is attempted).
func (r *Runner) EnsureClaudeJiraMcpConfig(accountHomePath string, authHeaderValue string) (JiraMcpProviderConfigResponse, error) {
	accountHomePath = strings.TrimSpace(accountHomePath)
	if accountHomePath == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	authHeaderValue = strings.TrimSpace(authHeaderValue)
	if authHeaderValue == "" {
		return JiraMcpProviderConfigResponse{}, errors.New("authHeaderValue is required")
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
		URL:     jiraMcpRemoteURL,
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
	if server.Type != "http" || server.URL != jiraMcpRemoteURL {
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
	default:
		result.ErrorMessage = fmt.Sprintf("Jira MCP provider config is not yet implemented for provider: %s", providerKey)
		return result
	}

	result.ProviderConfigured = true
	return result
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

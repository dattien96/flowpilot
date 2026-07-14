package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// connectTestJiraBackend seeds a fully "connected" Jira backend (credential +
// persisted mcp-backend-state.json SecretKey pointer) the same way
// TestTriggerIntegrationConnectionUsesJiraApiToken does, so PreflightJiraMcp
// sees Jira as installed/connected without needing a live Atlassian call.
func connectTestJiraBackend(t *testing.T, instance *Runner) {
	t.Helper()
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() { executeJiraRequestFn = originalRequest })
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		return []byte(`{"id":"10000","key":"SCRUM"}`), nil
	}

	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-jira", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "jira",
		Action:       "test",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
	})
	if err != nil {
		t.Fatalf("connect test jira backend: %v", err)
	}
}

func TestEnsureJiraMcpProviderConfigDispatchesToClaudeInProduction(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	accountHome := t.TempDir()

	resp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: accountHome,
		BearerToken:     "Bearer test-token",
	})
	if err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig: %v", err)
	}
	if resp.ServerName != "jira" || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	raw, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	if !strings.Contains(string(raw), "mcp.atlassian.com") {
		t.Fatalf("expected jira remote MCP url in config, got: %s", raw)
	}
}

// TestEnsureJiraMcpProviderConfigReusesPersistedBearerTokenOnRerun verifies
// Task-234 T-1: a bearer token pasted once is persisted (via
// persistConnectedJiraBearerToken) and reused automatically on a later call
// that omits it — e.g. a re-run of a "Configure Providers" loop button.
func TestEnsureJiraMcpProviderConfigReusesPersistedBearerTokenOnRerun(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)

	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: t.TempDir(),
		BearerToken:     "Bearer persisted-token",
	}); err != nil {
		t.Fatalf("first EnsureJiraMcpProviderConfig: %v", err)
	}

	accountHome2 := t.TempDir()
	resp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: accountHome2,
	})
	if err != nil {
		t.Fatalf("second EnsureJiraMcpProviderConfig (no bearer token): %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(accountHome2, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	if !strings.Contains(string(raw), "Bearer persisted-token") {
		t.Fatalf("expected persisted bearer token reused, got: %s", raw)
	}
	if !resp.Changed {
		t.Fatalf("expected config written for the new account home")
	}
}

func TestEnsureJiraMcpProviderConfigDispatchesToCodexAndGemini(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)

	codexHome := t.TempDir()
	codexResp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: codexHome,
		BearerToken:     "Bearer test-token",
	})
	if err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig(codex): %v", err)
	}
	if codexResp.ServerName != "jira" || !codexResp.Changed {
		t.Fatalf("unexpected codex response: %+v", codexResp)
	}
	codexRaw, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatalf("read codex config: %v", err)
	}
	if !strings.Contains(string(codexRaw), "mcp.atlassian.com") || !strings.Contains(string(codexRaw), "bearer_token_env_var") {
		t.Fatalf("expected jira remote MCP url + bearer_token_env_var in codex config, got: %s", codexRaw)
	}

	geminiHome := t.TempDir()
	geminiResp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "gemini",
		AccountHomePath: geminiHome,
		BearerToken:     "Bearer test-token",
	})
	if err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig(gemini): %v", err)
	}
	if geminiResp.ServerName != "jira" || !geminiResp.Changed {
		t.Fatalf("unexpected gemini response: %+v", geminiResp)
	}
	geminiRaw, err := os.ReadFile(filepath.Join(geminiHome, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("read gemini config: %v", err)
	}
	if !strings.Contains(string(geminiRaw), "httpUrl") || !strings.Contains(string(geminiRaw), "mcp.atlassian.com") {
		t.Fatalf("expected httpUrl + mcp.atlassian.com in gemini config, got: %s", geminiRaw)
	}
}

// TestEnsureJiraMcpProviderConfigDispatchesToGrok closes G2: Grok gets a
// remote-HTTP mcp_servers.jira entry in config.toml (url + headers + enabled).
func TestEnsureJiraMcpProviderConfigDispatchesToGrok(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	grokHome := t.TempDir()

	resp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
		BearerToken:     "Bearer test-token",
	})
	if err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig(grok): %v", err)
	}
	if resp.ServerName != "jira" || resp.ProviderKey != "grok" || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	raw, err := os.ReadFile(filepath.Join(grokHome, "config.toml"))
	if err != nil {
		t.Fatalf("read grok config: %v", err)
	}
	doc := map[string]interface{}{}
	if err := toml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse grok config: %v", err)
	}
	servers, _ := doc["mcp_servers"].(map[string]interface{})
	jiraRaw, ok := servers["jira"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected mcp_servers.jira map, got: %s", raw)
	}
	server, ok := grokServerFromMap(jiraRaw)
	if !ok {
		t.Fatalf("grokServerFromMap failed: %+v", jiraRaw)
	}
	if server.URL != jiraMcpRemoteURL {
		t.Fatalf("url = %q, want %q", server.URL, jiraMcpRemoteURL)
	}
	if server.Headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want Bearer test-token", server.Headers["Authorization"])
	}
	if !server.Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestPreflightJiraMcpReadyWhenGrokProviderConfigured(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	grokHome := t.TempDir()
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
		BearerToken:     "Bearer ready-token",
	}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	check := instance.PreflightJiraMcp("grok", grokHome)
	if check.ErrorMessage != "" || !check.ProviderConfigured || !check.GoogleDriveReady {
		t.Fatalf("expected ready preflight, got %+v", check)
	}
}

func TestPreflightJiraMcpFailsWhenGrokProviderMissing(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	check := instance.PreflightJiraMcp("grok", t.TempDir())
	if check.ErrorMessage == "" || check.ProviderConfigured {
		t.Fatalf("expected not-configured error, got %+v", check)
	}
	if !strings.Contains(check.ErrorMessage, "Configure Providers") {
		t.Fatalf("expected Configure Providers guidance, got %q", check.ErrorMessage)
	}
}

func TestPreflightJiraMcpFailsWhenGrokProviderStale(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	grokHome := t.TempDir()
	stale := `[mcp_servers.jira]
url = "https://example.com/wrong"
enabled = true
`
	if err := os.WriteFile(filepath.Join(grokHome, "config.toml"), []byte(stale), 0o644); err != nil {
		t.Fatalf("write stale config: %v", err)
	}
	check := instance.PreflightJiraMcp("grok", grokHome)
	if check.ErrorMessage == "" || check.ProviderConfigured {
		t.Fatalf("expected stale error, got %+v", check)
	}
	if !strings.Contains(strings.ToLower(check.ErrorMessage), "stale") {
		t.Fatalf("expected stale message, got %q", check.ErrorMessage)
	}
}

func TestEnsureGrokJiraMcpConfigPreservesOtherTopLevelSections(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	grokHome := t.TempDir()
	seed := `[ui]
theme = "dark"

[models]
default = "grok-build"

[mcp_servers.filesystem]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
enabled = true
`
	if err := os.WriteFile(filepath.Join(grokHome, "config.toml"), []byte(seed), 0o644); err != nil {
		t.Fatalf("seed config.toml: %v", err)
	}

	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
		BearerToken:     "Bearer keep-sections",
	}); err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig(grok): %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(grokHome, "config.toml"))
	if err != nil {
		t.Fatalf("read grok config: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "theme") || !strings.Contains(text, "dark") {
		t.Fatalf("expected [ui] theme preserved, got: %s", text)
	}
	if !strings.Contains(text, "grok-build") {
		t.Fatalf("expected [models] preserved, got: %s", text)
	}
	if !strings.Contains(text, "filesystem") || !strings.Contains(text, "server-filesystem") {
		t.Fatalf("expected existing mcp_servers.filesystem preserved, got: %s", text)
	}
	if !strings.Contains(text, "mcp.atlassian.com") {
		t.Fatalf("expected jira entry added, got: %s", text)
	}
}

func TestEnsureGrokJiraMcpConfigIsIdempotent(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	grokHome := t.TempDir()

	first, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
		BearerToken:     "Bearer same-token",
	})
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if !first.Changed {
		t.Fatal("first ensure should write config")
	}
	second, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "grok",
		AccountHomePath: grokHome,
		BearerToken:     "Bearer same-token",
	})
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if second.Changed {
		t.Fatal("second ensure with same token should be idempotent (Changed=false)")
	}
}

func TestEnsureJiraMcpProviderConfigRejectsUnsupportedProvider(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "unknown-provider",
		AccountHomePath: t.TempDir(),
		BearerToken:     "Bearer test-token",
	}); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestGrokServerToMapRoundTripsHTTPServer(t *testing.T) {
	expected := grokMcpServer{
		Enabled: true,
		URL:     jiraMcpRemoteURL,
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	}
	m, err := grokServerToMap(expected)
	if err != nil {
		t.Fatalf("grokServerToMap: %v", err)
	}
	got, ok := grokServerFromMap(m)
	if !ok {
		t.Fatalf("grokServerFromMap failed for map: %+v", m)
	}
	if !grokServerConfigMatches(got, expected) {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, expected)
	}
}

func TestEnsureJiraMcpProviderConfigRequiresProviderKeyAndAccountHome(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{AccountHomePath: t.TempDir()}); err == nil {
		t.Fatal("expected error for missing providerKey")
	}
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{ProviderKey: "claude"}); err == nil {
		t.Fatal("expected error for missing accountHomePath")
	}
}

func TestEnsureJiraMcpProviderConfigRequiresAuthWhenNoneConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: t.TempDir(),
	}); err == nil {
		t.Fatal("expected error when Jira is not connected (no apiToken and no override)")
	}
}

func TestEnsureJiraMcpProviderConfigUsesConnectedApiTokenBasic(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	accountHome := t.TempDir()

	resp, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: accountHome,
	})
	if err != nil {
		t.Fatalf("EnsureJiraMcpProviderConfig: %v", err)
	}
	if !resp.Changed {
		t.Fatal("expected config write")
	}
	raw, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	jira, _ := doc["mcpServers"].(map[string]any)["jira"].(map[string]any)
	if jira["url"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("url = %v, want %s", jira["url"], jiraMcpAPITokenRemoteURL)
	}
	headers, _ := jira["headers"].(map[string]any)
	auth, _ := headers["Authorization"].(string)
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("name@company.com:secret-token"))
	if auth != wantBasic {
		t.Fatalf("Authorization = %q, want %q", auth, wantBasic)
	}
}

func TestEnsureClaudeJiraMcpConfigWritesRemoteServerEntry(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	accountHome := t.TempDir()

	resp, err := instance.EnsureClaudeJiraMcpConfig(accountHome, "Bearer test-token")
	if err != nil {
		t.Fatalf("EnsureClaudeJiraMcpConfig: %v", err)
	}
	if !resp.Changed || resp.Status != "configured" || resp.ServerName != "jira" {
		t.Fatalf("unexpected response: %#v", resp)
	}

	raw, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal claude config: %v", err)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	jira, ok := servers["jira"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers.jira entry, got %#v", servers)
	}
	// Bearer header → authv2 endpoint (not /v1/mcp).
	if jira["type"] != "http" || jira["url"] != jiraMcpOAuthRemoteURL {
		t.Fatalf("unexpected jira server entry: %#v", jira)
	}
	headers, _ := jira["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %#v", headers)
	}
}

func TestEnsureClaudeJiraMcpConfigBasicUsesAPITokenURL(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	accountHome := t.TempDir()
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("a@b.com:tok"))
	if _, err := instance.EnsureClaudeJiraMcpConfig(accountHome, basic); err != nil {
		t.Fatalf("EnsureClaudeJiraMcpConfig: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	jira, _ := doc["mcpServers"].(map[string]any)["jira"].(map[string]any)
	if jira["url"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("url = %v, want %s", jira["url"], jiraMcpAPITokenRemoteURL)
	}
}

func TestEnsureJiraMcpProviderConfigBasicNoOverrideForCodexGeminiGrok(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("name@company.com:secret-token"))

	// Codex
	codexHome := t.TempDir()
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey: "codex", AccountHomePath: codexHome,
	}); err != nil {
		t.Fatalf("codex: %v", err)
	}
	codexRaw, _ := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	var codexDoc map[string]any
	if err := toml.Unmarshal(codexRaw, &codexDoc); err != nil {
		t.Fatalf("parse codex: %v", err)
	}
	codexJira, _ := codexDoc["mcp_servers"].(map[string]any)["jira"].(map[string]any)
	if codexJira["url"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("codex url = %v", codexJira["url"])
	}
	if v, ok := codexJira["command"]; ok && strings.TrimSpace(fmt.Sprint(v)) != "" {
		t.Fatalf("codex remote jira must not serialize stdio command, got %v", v)
	}
	if v, ok := codexJira["args"]; ok {
		switch vv := v.(type) {
		case []any:
			if len(vv) > 0 {
				t.Fatalf("codex remote jira must not serialize stdio args, got %v", vv)
			}
		case []string:
			if len(vv) > 0 {
				t.Fatalf("codex remote jira must not serialize stdio args, got %v", vv)
			}
		default:
			t.Fatalf("codex remote jira args has unexpected shape: %#v", v)
		}
	}
	if v, ok := codexJira["env"]; ok {
		switch vv := v.(type) {
		case map[string]any:
			if len(vv) > 0 {
				t.Fatalf("codex remote jira must not serialize stdio env, got %v", vv)
			}
		case map[string]string:
			if len(vv) > 0 {
				t.Fatalf("codex remote jira must not serialize stdio env, got %v", vv)
			}
		default:
			t.Fatalf("codex remote jira env has unexpected shape: %#v", v)
		}
	}
	if codexJira["bearer_token_env_var"] != nil && codexJira["bearer_token_env_var"] != "" {
		t.Fatalf("codex must not use bearer_token_env_var for Basic, got %v", codexJira["bearer_token_env_var"])
	}
	headers, _ := codexJira["http_headers"].(map[string]any)
	if headers["Authorization"] != wantBasic {
		t.Fatalf("codex Authorization = %v, want %s", headers["Authorization"], wantBasic)
	}

	// Gemini
	geminiHome := t.TempDir()
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey: "gemini", AccountHomePath: geminiHome,
	}); err != nil {
		t.Fatalf("gemini: %v", err)
	}
	geminiRaw, _ := os.ReadFile(filepath.Join(geminiHome, ".gemini", "settings.json"))
	var geminiDoc map[string]any
	if err := json.Unmarshal(geminiRaw, &geminiDoc); err != nil {
		t.Fatalf("parse gemini: %v", err)
	}
	geminiJira, _ := geminiDoc["mcpServers"].(map[string]any)["jira"].(map[string]any)
	if geminiJira["httpUrl"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("gemini httpUrl = %v", geminiJira["httpUrl"])
	}
	gHeaders, _ := geminiJira["headers"].(map[string]any)
	if gHeaders["Authorization"] != wantBasic {
		t.Fatalf("gemini Authorization = %v, want %s", gHeaders["Authorization"], wantBasic)
	}

	// Grok
	grokHome := t.TempDir()
	if _, err := instance.EnsureJiraMcpProviderConfig(JiraMcpProviderConfigRequest{
		ProviderKey: "grok", AccountHomePath: grokHome,
	}); err != nil {
		t.Fatalf("grok: %v", err)
	}
	grokRaw, _ := os.ReadFile(filepath.Join(grokHome, "config.toml"))
	var grokDoc map[string]any
	if err := toml.Unmarshal(grokRaw, &grokDoc); err != nil {
		t.Fatalf("parse grok: %v", err)
	}
	grokJira, _ := grokDoc["mcp_servers"].(map[string]any)["jira"].(map[string]any)
	if grokJira["url"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("grok url = %v", grokJira["url"])
	}
	// headers may be map after toml round-trip
	if gh, ok := grokJira["headers"].(map[string]any); ok {
		if gh["Authorization"] != wantBasic {
			t.Fatalf("grok Authorization = %v, want %s", gh["Authorization"], wantBasic)
		}
	} else {
		// pelletier may leave typed map[string]string-ish values
		server, ok := grokServerFromMap(mustStringMap(grokJira))
		if !ok || server.Headers["Authorization"] != wantBasic {
			t.Fatalf("grok headers not Basic: %+v ok=%v", grokJira, ok)
		}
	}
}

func mustStringMap(m map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestEnsureClaudeJiraMcpConfigPreservesOtherKeysAndServers(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	accountHome := t.TempDir()

	seed := map[string]any{
		"someUnrelatedTopLevelKey": "keep-me",
		"mcpServers": map[string]any{
			"google-drive": map[string]any{
				"type":    "stdio",
				"command": "npx",
				"args":    []any{"-y", "@piotr-agier/google-drive-mcp"},
			},
		},
	}
	raw, _ := json.Marshal(seed)
	if err := os.WriteFile(filepath.Join(accountHome, ".claude.json"), raw, 0o644); err != nil {
		t.Fatalf("seed claude config: %v", err)
	}

	if _, err := instance.EnsureClaudeJiraMcpConfig(accountHome, "Bearer test-token"); err != nil {
		t.Fatalf("EnsureClaudeJiraMcpConfig: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["someUnrelatedTopLevelKey"] != "keep-me" {
		t.Fatalf("expected unrelated top-level key preserved, got %#v", doc)
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if _, ok := servers["google-drive"]; !ok {
		t.Fatalf("expected google-drive server entry preserved, got %#v", servers)
	}
	if _, ok := servers["jira"]; !ok {
		t.Fatalf("expected jira server entry added, got %#v", servers)
	}
}

func TestEnsureClaudeJiraMcpConfigIsIdempotent(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	accountHome := t.TempDir()

	if _, err := instance.EnsureClaudeJiraMcpConfig(accountHome, "Bearer test-token"); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	resp, err := instance.EnsureClaudeJiraMcpConfig(accountHome, "Bearer test-token")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if resp.Changed {
		t.Fatalf("expected no change on second identical ensure, got %#v", resp)
	}
}

func TestCheckClaudeJiraMcpConfigDetectsStaleURL(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	accountHome := t.TempDir()

	seed := map[string]any{
		"mcpServers": map[string]any{
			"jira": map[string]any{
				"type": "http",
				"url":  "https://old-endpoint.example.com/mcp",
			},
		},
	}
	raw, _ := json.Marshal(seed)
	if err := os.WriteFile(filepath.Join(accountHome, ".claude.json"), raw, 0o644); err != nil {
		t.Fatalf("seed claude config: %v", err)
	}

	status, err := instance.checkClaudeJiraMcpConfig(accountHome)
	if err != nil {
		t.Fatalf("checkClaudeJiraMcpConfig: %v", err)
	}
	if !status.Configured || !status.Stale {
		t.Fatalf("expected configured+stale, got %#v", status)
	}
}

func TestPreflightJiraMcpFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result := instance.PreflightJiraMcp("", "")
	if result.GoogleDriveReady || strings.TrimSpace(result.ErrorMessage) == "" {
		t.Fatalf("expected not-ready with an error message, got %#v", result)
	}
}

func TestPreflightJiraMcpReadyWhenConnectedNoProvider(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)

	result := instance.PreflightJiraMcp("", "")
	if !result.GoogleDriveReady || !result.ProviderConfigured {
		t.Fatalf("expected ready with no provider check requested, got %#v", result)
	}
	if strings.TrimSpace(result.ErrorMessage) != "" {
		t.Fatalf("expected no error message, got %q", result.ErrorMessage)
	}
}

func TestPreflightJiraMcpFailsWhenProviderNotConfigured(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	accountHome := t.TempDir() // no .claude.json at all

	result := instance.PreflightJiraMcp("claude", accountHome)
	if result.ProviderConfigured || strings.TrimSpace(result.ErrorMessage) == "" {
		t.Fatalf("expected provider-not-configured error, got %#v", result)
	}
}

func TestPreflightJiraMcpReadyWhenProviderConfigured(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	accountHome := t.TempDir()
	if _, err := instance.EnsureClaudeJiraMcpConfig(accountHome, "Bearer test-token"); err != nil {
		t.Fatalf("EnsureClaudeJiraMcpConfig: %v", err)
	}

	result := instance.PreflightJiraMcp("claude", accountHome)
	if !result.GoogleDriveReady || !result.ProviderConfigured {
		t.Fatalf("expected fully ready, got %#v", result)
	}
}

// TestInjectRequiredMcpInstructionsJiraProducesJiraBlockNotDrive proves the
// Task-227 registry seam dispatches to Jira's own block when requiredMcps
// asks for "jira" (CP-05-06 P-6).
func TestInjectRequiredMcpInstructionsJiraProducesJiraBlockNotDrive(t *testing.T) {
	result := InjectRequiredMcpInstructions("Task.\n\nDo the thing.", []string{"jira"}, "codex", false, false)
	if !strings.Contains(result, "FlowPilot MCP `jira`") {
		t.Errorf("expected jira MCP instructions injected, got: %s", result)
	}
	if strings.Contains(result, "google_drive") || strings.Contains(result, "Google Drive") {
		t.Errorf("jira injection must not pull in Google Drive instructions, got: %s", result)
	}
	if !strings.Contains(result, "JIRA_CONTENT_NOT_FOUND") {
		t.Errorf("expected jira failure codes present, got: %s", result)
	}
	if !strings.Contains(result, "getAccessibleAtlassianResources") {
		t.Errorf("expected jira bootstrap instructions to require getAccessibleAtlassianResources first, got: %s", result)
	}
	if !strings.Contains(result, "Never send an empty `cloudId`") {
		t.Errorf("expected jira bootstrap instructions to forbid empty cloudId, got: %s", result)
	}
	if !strings.Contains(result, "atlassianUserInfo") || !strings.Contains(result, "`account_id`") {
		t.Errorf("expected jira bootstrap instructions to require atlassianUserInfo for current user resolution, got: %s", result)
	}
	if !strings.Contains(result, "Do not use `objectIdentifier: \"current\"`") {
		t.Errorf("expected jira bootstrap instructions to forbid objectIdentifier=current, got: %s", result)
	}
}

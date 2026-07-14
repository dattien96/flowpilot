package runner

import (
	"encoding/base64"
	"testing"
)

// Task-234 T-1: flowpilotClaudeExtraMCPServers must merge jira/firebase/
// telegram into the per-turn --mcp-config once each integration is connected
// — this is the fix for the gap where EnsureClaude{Jira,Firebase,Telegram}McpConfig
// wrote a .claude.json entry that a live Claude turn (--strict-mcp-config)
// never actually read.

func TestFlowpilotClaudeExtraMCPServersIncludesJiraWhenBearerTokenPersisted(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	if err := instance.persistConnectedJiraBearerToken("Bearer live-token"); err != nil {
		t.Fatalf("persistConnectedJiraBearerToken: %v", err)
	}

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	server, ok := extra["jira"]
	if !ok {
		t.Fatal("expected jira in extra MCP servers")
	}
	if server.Type != "http" || server.URL != jiraMcpOAuthRemoteURL || server.Headers["Authorization"] != "Bearer live-token" {
		t.Fatalf("unexpected jira server entry: %+v", server)
	}
}

// With connected email+apiToken, live merge includes Jira via Basic → /v1/mcp
// (no separate bearer required). Exact Basic value asserted.
func TestFlowpilotClaudeExtraMCPServersIncludesJiraWhenApiTokenConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("name@company.com:secret-token"))

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	server, ok := extra["jira"]
	if !ok {
		t.Fatal("expected jira in extra MCP servers when email+apiToken are connected")
	}
	if server.URL != jiraMcpAPITokenRemoteURL {
		t.Fatalf("url = %q, want %s", server.URL, jiraMcpAPITokenRemoteURL)
	}
	if server.Headers["Authorization"] != wantBasic {
		t.Fatalf("Authorization = %q, want %q", server.Headers["Authorization"], wantBasic)
	}
}

func TestGrokACPExtraMCPServersForwardsBasicJiraWhenApiTokenConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("name@company.com:secret-token"))

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	acpEntries := grokACPExtraMCPServers(extra)
	var jira map[string]interface{}
	for _, entry := range acpEntries {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if m["name"] == "jira" {
			jira = m
			break
		}
	}
	if jira == nil {
		t.Fatal("expected jira HTTP entry in Grok ACP servers for Basic path")
	}
	if jira["type"] != "http" || jira["url"] != jiraMcpAPITokenRemoteURL {
		t.Fatalf("unexpected jira entry: %+v", jira)
	}
	headers, ok := jira["headers"].([]interface{})
	if !ok {
		t.Fatalf("jira headers should be an ACP HttpHeader[] array, got %T (%v)", jira["headers"], jira["headers"])
	}
	if v, found := acpHeaderValue(headers, "Authorization"); !found || v != wantBasic {
		t.Fatalf("Authorization = %v, want %s", v, wantBasic)
	}
}

// acpHeaderValue looks up a header value in an ACP HttpHeader[] array
// ([]{"name":..,"value":..}) as sent in session/new mcpServers[].headers.
func acpHeaderValue(headers []interface{}, name string) (string, bool) {
	for _, raw := range headers {
		h, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if h["name"] == name {
			v, _ := h["value"].(string)
			return v, true
		}
	}
	return "", false
}

func TestFlowpilotClaudeExtraMCPServersOmitsJiraWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	if _, ok := extra["jira"]; ok {
		t.Fatal("did not expect jira in extra MCP servers when Jira is not connected")
	}
}

func TestFlowpilotClaudeExtraMCPServersIncludesFirebaseAndTelegramWhenConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestFirebaseBackend(t, instance)
	connectTestTelegramBackend(t, instance)

	extra := instance.flowpilotClaudeExtraMCPServers("", false)

	firebase, ok := extra["firebase"]
	if !ok || firebase.Command != "npx" {
		t.Fatalf("expected firebase stdio entry, got: %+v (ok=%v)", firebase, ok)
	}
	telegram, ok := extra["telegram"]
	if !ok || len(telegram.Args) == 0 || telegram.Args[0] != "telegram-mcp" {
		t.Fatalf("expected telegram stdio entry, got: %+v (ok=%v)", telegram, ok)
	}
}

func TestFlowpilotClaudeExtraMCPServersOmitsFirebaseTelegramWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	if _, ok := extra["firebase"]; ok {
		t.Fatal("did not expect firebase in extra MCP servers when not connected")
	}
	if _, ok := extra["telegram"]; ok {
		t.Fatal("did not expect telegram in extra MCP servers when not connected")
	}
}

// TestGrokACPExtraMCPServersForwardsStdioAndHTTPEntries (G2): stdio
// firebase/telegram and HTTP jira all reach Grok ACP mcpServers[].
func TestGrokACPExtraMCPServersForwardsStdioAndHTTPEntries(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestJiraBackend(t, instance)
	if err := instance.persistConnectedJiraBearerToken("Bearer live-token"); err != nil {
		t.Fatalf("persistConnectedJiraBearerToken: %v", err)
	}
	connectTestFirebaseBackend(t, instance)
	connectTestTelegramBackend(t, instance)

	extra := instance.flowpilotClaudeExtraMCPServers("", false)
	acpEntries := grokACPExtraMCPServers(extra)

	byName := map[string]map[string]interface{}{}
	for _, entry := range acpEntries {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		if name != "" {
			byName[name] = m
		}
	}
	if byName["firebase"] == nil || byName["telegram"] == nil {
		t.Fatalf("expected firebase and telegram stdio entries, got: %+v", byName)
	}
	jira, ok := byName["jira"]
	if !ok {
		t.Fatalf("expected jira HTTP entry in Grok ACP servers, got: %+v", byName)
	}
	if jira["type"] != "http" {
		t.Fatalf("jira type = %v, want http", jira["type"])
	}
	if jira["url"] != jiraMcpOAuthRemoteURL {
		t.Fatalf("jira url = %v, want %s", jira["url"], jiraMcpOAuthRemoteURL)
	}
	headers, ok := jira["headers"].([]interface{})
	if !ok {
		t.Fatalf("jira headers should be an ACP HttpHeader[] array, got %T (%v)", jira["headers"], jira["headers"])
	}
	if v, found := acpHeaderValue(headers, "Authorization"); !found || v != "Bearer live-token" {
		t.Fatalf("jira Authorization = %v, want Bearer live-token", v)
	}
}

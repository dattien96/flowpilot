package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if jira["type"] != "http" || jira["url"] != jiraMcpRemoteURL {
		t.Fatalf("unexpected jira server entry: %#v", jira)
	}
	headers, _ := jira["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer test-token" {
		t.Fatalf("expected Authorization header, got %#v", headers)
	}
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
}

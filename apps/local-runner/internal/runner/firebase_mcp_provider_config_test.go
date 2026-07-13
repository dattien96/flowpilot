package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validFirebaseServiceAccountJSON = `{
  "type": "service_account",
  "project_id": "flowpilot-test",
  "private_key": "-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n",
  "client_email": "runner@flowpilot-test.iam.gserviceaccount.com"
}`

func connectTestFirebaseBackend(t *testing.T, instance *Runner) {
	t.Helper()
	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-firebase", IntegrationConnectionRequest{
		ProjectID:           "project-alpha",
		ProviderType:        "firebase",
		Action:              "test",
		FirebaseProjectID:   "flowpilot-test",
		FirebaseEnvironment: "production",
		ServiceAccountJSON:  validFirebaseServiceAccountJSON,
	})
	if err != nil {
		t.Fatalf("connect test firebase backend: %v", err)
	}
}

func TestValidateFirebaseServiceAccountJSONAcceptsValidKey(t *testing.T) {
	parsed, err := validateFirebaseServiceAccountJSON(validFirebaseServiceAccountJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.ProjectID != "flowpilot-test" {
		t.Errorf("ProjectID = %q, want flowpilot-test", parsed.ProjectID)
	}
	if parsed.ClientEmail != "runner@flowpilot-test.iam.gserviceaccount.com" {
		t.Errorf("ClientEmail = %q", parsed.ClientEmail)
	}
}

func TestValidateFirebaseServiceAccountJSONRejectsEmpty(t *testing.T) {
	if _, err := validateFirebaseServiceAccountJSON(""); err == nil {
		t.Fatal("expected error for empty service account JSON")
	}
}

func TestValidateFirebaseServiceAccountJSONRejectsMalformedJSON(t *testing.T) {
	if _, err := validateFirebaseServiceAccountJSON("{not json"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestValidateFirebaseServiceAccountJSONRejectsWrongType(t *testing.T) {
	_, err := validateFirebaseServiceAccountJSON(`{"type":"authorized_user","project_id":"x","private_key":"y","client_email":"z"}`)
	if err == nil {
		t.Fatal("expected error for non-service_account type")
	}
}

func TestValidateFirebaseServiceAccountJSONRejectsMissingFields(t *testing.T) {
	_, err := validateFirebaseServiceAccountJSON(`{"type":"service_account","project_id":"x"}`)
	if err == nil {
		t.Fatal("expected error for missing private_key/client_email")
	}
}

func TestTriggerIntegrationConnectionFirebaseRejectsInvalidServiceAccount(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-firebase", IntegrationConnectionRequest{
		ProjectID:          "project-alpha",
		ProviderType:       "firebase",
		Action:             "test",
		ServiceAccountJSON: "{not json",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.IntegrationStatus != "failed" || result.RequestStatus != "rejected" {
		t.Fatalf("expected rejected/failed result, got %#v", result)
	}
}

func TestTriggerIntegrationConnectionFirebaseConnectsWithValidServiceAccount(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-firebase", IntegrationConnectionRequest{
		ProjectID:           "project-alpha",
		ProviderType:        "firebase",
		Action:              "test",
		FirebaseProjectID:   "flowpilot-test",
		FirebaseEnvironment: "production",
		ServiceAccountJSON:  validFirebaseServiceAccountJSON,
	})
	if err != nil {
		t.Fatalf("connect firebase: %v", err)
	}
	if result.IntegrationStatus != "connected" {
		t.Fatalf("expected connected status, got %#v", result)
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}
	found := false
	for _, backend := range backends {
		if backend.ProviderType != "firebase" {
			continue
		}
		found = true
		if !backend.Installed || backend.State != "installed" {
			t.Fatalf("expected firebase backend installed, got %#v", backend)
		}
	}
	if !found {
		t.Fatal("expected a firebase backend in ListMcpBackends")
	}
}

func TestEnsureClaudeFirebaseMcpConfigWritesStdioEntryAndCredentialFile(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestFirebaseBackend(t, instance)
	accountHome := t.TempDir()

	resp, err := instance.EnsureClaudeFirebaseMcpConfig(accountHome)
	if err != nil {
		t.Fatalf("EnsureClaudeFirebaseMcpConfig: %v", err)
	}
	if !resp.Changed || resp.ServerName != "firebase" || resp.Status != "configured" {
		t.Fatalf("unexpected response: %#v", resp)
	}

	raw, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("unmarshal claude config: %v", err)
	}
	server, ok := config.McpServers["firebase"]
	if !ok {
		t.Fatalf("expected mcpServers.firebase entry, got %#v", config.McpServers)
	}
	if server.Command != "npx" || server.Type != "stdio" {
		t.Fatalf("unexpected server entry: %#v", server)
	}
	credPath := server.Env["GOOGLE_APPLICATION_CREDENTIALS"]
	if credPath == "" {
		t.Fatal("expected GOOGLE_APPLICATION_CREDENTIALS env var set")
	}
	writtenCred, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("expected credential file to exist at %q: %v", credPath, err)
	}
	if !strings.Contains(string(writtenCred), "flowpilot-test") {
		t.Fatalf("expected credential file to contain the service account JSON, got %q", string(writtenCred))
	}
}

func TestEnsureClaudeFirebaseMcpConfigIsIdempotent(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestFirebaseBackend(t, instance)
	accountHome := t.TempDir()

	if _, err := instance.EnsureClaudeFirebaseMcpConfig(accountHome); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	resp, err := instance.EnsureClaudeFirebaseMcpConfig(accountHome)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if resp.Changed {
		t.Fatalf("expected no change on second identical ensure, got %#v", resp)
	}
}

func TestEnsureClaudeFirebaseMcpConfigFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.EnsureClaudeFirebaseMcpConfig(t.TempDir()); err == nil {
		t.Fatal("expected error when firebase is not connected")
	}
}

func TestPreflightFirebaseMcpFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result := instance.PreflightFirebaseMcp("", "")
	if result.GoogleDriveReady || strings.TrimSpace(result.ErrorMessage) == "" {
		t.Fatalf("expected not-ready with an error message, got %#v", result)
	}
}

func TestPreflightFirebaseMcpReadyWhenConnectedAndProviderConfigured(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestFirebaseBackend(t, instance)
	accountHome := t.TempDir()
	if _, err := instance.EnsureClaudeFirebaseMcpConfig(accountHome); err != nil {
		t.Fatalf("EnsureClaudeFirebaseMcpConfig: %v", err)
	}

	result := instance.PreflightFirebaseMcp("claude", accountHome)
	if !result.GoogleDriveReady || !result.ProviderConfigured {
		t.Fatalf("expected fully ready, got %#v", result)
	}
}

func TestPreflightFirebaseMcpFailsWhenProviderNotConfigured(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestFirebaseBackend(t, instance)

	result := instance.PreflightFirebaseMcp("claude", t.TempDir())
	if result.ProviderConfigured || strings.TrimSpace(result.ErrorMessage) == "" {
		t.Fatalf("expected provider-not-configured error, got %#v", result)
	}
}

// TestInjectRequiredMcpInstructionsFirebaseProducesFirebaseBlockNotDrive
// proves the Task-227 registry seam dispatches to Firebase's own block
// (CP-05-04 P-1).
func TestInjectRequiredMcpInstructionsFirebaseProducesFirebaseBlockNotDrive(t *testing.T) {
	result := InjectRequiredMcpInstructions("Task.\n\nDo the thing.", []string{"firebase"}, "codex", false, false)
	if !strings.Contains(result, "FlowPilot MCP `firebase`") {
		t.Errorf("expected firebase MCP instructions injected, got: %s", result)
	}
	if !strings.Contains(result, "crashlytics_get_issue") {
		t.Errorf("expected Crashlytics tool names present, got: %s", result)
	}
	if strings.Contains(result, "google_drive") || strings.Contains(result, "Google Drive") {
		t.Errorf("firebase injection must not pull in Google Drive instructions, got: %s", result)
	}
	if !strings.Contains(result, "FIREBASE_CONTENT_NOT_FOUND") {
		t.Errorf("expected firebase failure codes present, got: %s", result)
	}
}

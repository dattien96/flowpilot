package runner

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type memorySecretStore struct {
	values map[string]string
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{values: map[string]string{}}
}

func (s *memorySecretStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func (s *memorySecretStore) Get(key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (s *memorySecretStore) Delete(key string) error {
	delete(s.values, key)
	return nil
}

func TestTriggerIntegrationConnectionAcceptsValidRequest(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "telegram",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("trigger integration connection: %v", err)
	}

	if result.RequestStatus != "accepted" {
		t.Fatalf("expected accepted request status, got %q", result.RequestStatus)
	}
	if result.IntegrationID != "integration-1" {
		t.Fatalf("expected integration id to round-trip, got %q", result.IntegrationID)
	}
	if result.IntegrationStatus != "pending" {
		t.Fatalf("expected pending integration status, got %q", result.IntegrationStatus)
	}
	if result.Message == nil || !strings.Contains(*result.Message, "project-alpha") {
		t.Fatalf("expected message to mention project id, got %#v", result.Message)
	}
}

func TestTriggerIntegrationConnectionUsesJiraApiToken(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	var observedEndpoint string
	var observedMethod string
	var observedCreds jiraCredential
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		observedEndpoint = endpoint
		observedMethod = method
		observedCreds = creds
		return []byte(`{"id":"10000","key":"SCRUM"}`), nil
	}

	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-jira", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "jira",
		Action:       "test",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		BoardID:      "1",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
	})
	if err != nil {
		t.Fatalf("trigger Jira integration connection: %v", err)
	}

	if result.RequestStatus != "accepted" {
		t.Fatalf("expected accepted request status, got %q", result.RequestStatus)
	}
	if result.IntegrationStatus != "connected" {
		t.Fatalf("expected connected integration status, got %q", result.IntegrationStatus)
	}
	if observedEndpoint != "https://flowpilot899.atlassian.net/rest/api/3/project/SCRUM" {
		t.Fatalf("expected Jira project verification endpoint, got %q", observedEndpoint)
	}
	if observedMethod != http.MethodGet {
		t.Fatalf("expected Jira verify GET request, got %q", observedMethod)
	}
	if observedCreds.Email != "name@company.com" || observedCreds.ApiToken != "secret-token" {
		t.Fatalf("expected stored Jira credentials to be used, got %#v", observedCreds)
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after Jira connect: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if !backend.Installed {
			t.Fatal("expected Jira backend to be marked installed after connect")
		}
		if backend.State != "installed" {
			t.Fatalf("expected Jira backend state installed, got %q", backend.State)
		}
		if backend.Action != "verify" {
			t.Fatalf("expected Jira backend action verify, got %q", backend.Action)
		}
		break
	}

	verified, err := instance.VerifyMcpBackend(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-jira",
	)
	if err != nil {
		t.Fatalf("verify Jira MCP backend: %v", err)
	}
	if !verified.Installed {
		t.Fatal("expected Jira backend to remain installed after verify")
	}
	if verified.Action != "verify" {
		t.Fatalf("expected Jira backend action verify, got %q", verified.Action)
	}
}

func TestVerifyMcpBackendUsesRequestedJiraIntegrationScope(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-legacy", jiraCredential{
		ProjectID:    "project-legacy",
		Email:        "legacy@company.com",
		ApiToken:     "legacy-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "LEGACY",
	}); err != nil {
		t.Fatalf("save legacy jira credential: %v", err)
	}
	if err := instance.saveJiraCredential("integration-target", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "target@company.com",
		ApiToken:     "target-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save target jira credential: %v", err)
	}
	if err := instance.saveMcpBackendRecord(McpBackend{
		Key:       "jira",
		SecretKey: jiraCredentialKey("integration-legacy"),
	}); err != nil {
		t.Fatalf("save jira backend record: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	var observedCreds jiraCredential
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		observedCreds = creds
		return []byte(`{"id":"10000","key":"SCRUM"}`), nil
	}

	backend, err := instance.VerifyMcpBackend(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-target",
	)
	if err != nil {
		t.Fatalf("verify jira backend with scoped integration: %v", err)
	}

	if observedCreds.Email != "target@company.com" || observedCreds.ApiToken != "target-token" {
		t.Fatalf("expected verify to use requested integration credential, got %#v", observedCreds)
	}
	if backend.SecretKey != jiraCredentialKey("integration-target") {
		t.Fatalf("expected backend secret key to switch to target integration, got %q", backend.SecretKey)
	}
}

func TestTriggerIntegrationConnectionFailsWhenRequiredBackendLauncherIsMissing(t *testing.T) {
	t.Setenv("PATH", "")

	instance := &Runner{workspace: t.TempDir()}

	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("trigger integration connection: %v", err)
	}

	if result.RequestStatus != "rejected" {
		t.Fatalf("expected rejected request status, got %q", result.RequestStatus)
	}
	if result.IntegrationStatus != "failed" {
		t.Fatalf("expected failed integration status, got %q", result.IntegrationStatus)
	}
	if result.Message == nil || !strings.Contains(strings.ToLower(*result.Message), "install") {
		t.Fatalf("expected install hint in message, got %#v", result.Message)
	}
}

func TestTriggerIntegrationConnectionRejectsInvalidInput(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	_, err := instance.TriggerIntegrationConnection(context.Background(), "", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err == nil {
		t.Fatal("expected integration id validation error")
	}

	_, err = instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err == nil {
		t.Fatal("expected project id validation error")
	}

	_, err = instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "invalid",
	})
	if err == nil {
		t.Fatal("expected action validation error")
	}
}

func TestListMcpBackendsReportsMissingLaunchers(t *testing.T) {
	t.Setenv("PATH", "")

	instance := &Runner{workspace: t.TempDir()}
	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}

	if len(backends) == 0 {
		t.Fatal("expected allowlisted MCP backends")
	}

	for _, backend := range backends {
		if backend.Installed {
			t.Fatalf("expected backend %q to be missing when PATH is empty", backend.Key)
		}
		if backend.LastError == "" {
			t.Fatalf("expected backend %q to report a missing launcher or credential error", backend.Key)
		}
	}
}

func TestListMcpBackendsDetectsAllowlistedBackends(t *testing.T) {
	originalLookPath := lookPathFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
	})

	lookPathFn = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", errors.New("launcher not found")
	}
	instance := &Runner{workspace: t.TempDir()}
	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}

	if len(backends) < 2 {
		t.Fatalf("expected allowlisted MCP backends, got %d", len(backends))
	}

	for _, backend := range backends {
		switch backend.ProviderType {
		case "google_drive":
			if backend.Installed {
				t.Fatalf("expected backend %q to be launcher-available but not installed before any backend action", backend.Key)
			}
			if backend.State != "launcher_available" {
				t.Fatalf("expected backend %q to report launcher_available state, got %q", backend.Key, backend.State)
			}
			if backend.Action != "install" {
				t.Fatalf("expected backend %q to default to install action before setup, got %q", backend.Key, backend.Action)
			}
		case "jira":
			if backend.Installed {
				t.Fatalf("expected Jira backend %q to be missing before token setup", backend.Key)
			}
			if backend.State != "missing" {
				t.Fatalf("expected Jira backend %q to report missing state, got %q", backend.Key, backend.State)
			}
			if backend.ActionLabel != "Create MCP" {
				t.Fatalf("expected Jira backend %q to default to Create MCP action before setup, got %q", backend.Key, backend.ActionLabel)
			}
		}
	}
}

func TestInstallMcpBackendRunsExplicitCommandForGoogleDrive(t *testing.T) {
	originalLookPath := lookPathFn
	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
		runCommandFn = originalRunCommand
	})

	lookPathFn = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", errors.New("launcher not found")
	}
	commandArgs := make([][]string, 0, 2)
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		commandArgs = append(commandArgs, append([]string(nil), args...))
		if name != "/usr/bin/npx" {
			t.Fatalf("expected explicit install to use resolved launcher path, got %q", name)
		}
		if len(args) == 0 {
			t.Fatal("expected explicit install to pass backend command args")
		}
		return []byte("ok"), nil
	}

	instance := &Runner{workspace: t.TempDir()}
	backend, err := instance.InstallMcpBackend(context.Background(), "google_drive")
	if err != nil {
		t.Fatalf("install MCP backend: %v", err)
	}
	if !backend.Installed {
		t.Fatal("expected backend to be installed after explicit install")
	}
	if backend.Action != "verify" {
		t.Fatalf("expected backend action to switch to verify, got %q", backend.Action)
	}
	if len(commandArgs) != 1 {
		t.Fatalf("expected one explicit install command, got %d", len(commandArgs))
	}
	if want := []string{"-y", "@piotr-agier/google-drive-mcp", "--version"}; !slices.Equal(commandArgs[0], want) {
		t.Fatalf("expected install args %v, got %v", want, commandArgs[0])
	}

	verified, err := instance.VerifyMcpBackend(context.Background(), "google_drive", "", "")
	if err != nil {
		t.Fatalf("verify MCP backend: %v", err)
	}
	if !verified.Installed {
		t.Fatal("expected backend to remain installed after explicit verify")
	}
	if verified.Action != "verify" {
		t.Fatalf("expected backend action to stay verify after verification, got %q", verified.Action)
	}
	if len(commandArgs) != 2 {
		t.Fatalf("expected install and verify commands to both run, got %d", len(commandArgs))
	}
	if want := []string{"-y", "@piotr-agier/google-drive-mcp", "--help"}; !slices.Equal(commandArgs[1], want) {
		t.Fatalf("expected verify args %v, got %v", want, commandArgs[1])
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after install: %v", err)
	}

	for _, listed := range backends {
		if listed.Key != "google_drive" {
			continue
		}
		if !listed.Installed {
			t.Fatal("expected installed backend state to persist for google_drive")
		}
		if listed.State != "installed" {
			t.Fatalf("expected persisted state to be installed, got %q", listed.State)
		}
		if listed.Action != "verify" {
			t.Fatalf("expected persisted action to be verify, got %q", listed.Action)
		}
		return
	}

	t.Fatal("expected google_drive backend to be present after install")
}

func TestDeleteIntegrationConnectionRemovesJiraSecret(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	originalProbe := probeRemoteBackendFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
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
		t.Fatalf("prime Jira connection: %v", err)
	}

	if err := instance.DeleteIntegrationConnection(context.Background(), "integration-jira"); err != nil {
		t.Fatalf("delete integration connection: %v", err)
	}

	if _, err := instance.ensureSecretStore().Get(jiraCredentialKey("integration-jira")); err == nil {
		t.Fatal("expected Jira secret to be removed from keyring store")
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after delete: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if backend.Installed {
			t.Fatal("expected Jira backend to be missing after delete")
		}
		if backend.State != "missing" {
			t.Fatalf("expected Jira backend state missing after delete, got %q", backend.State)
		}
		return
	}

	t.Fatal("expected Jira backend to be present after delete")
}

func TestDeleteIntegrationConnectionSucceedsWhenLocalJiraSecretIsAlreadyMissing(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveMcpBackendRecord(McpBackend{
		Key:           "jira",
		ProviderType:  "jira",
		Label:         "Atlassian MCP",
		Transport:     "remote",
		Launcher:      "remote",
		InstallHint:   "Open the Jira MCP form, read the Atlassian guide, and paste an API token so the runner can connect to the Atlassian remote MCP server.",
		State:         "installed",
		Action:        "verify",
		ActionLabel:   "Verify",
		LastCheckedAt: "2026-05-19T08:00:00.000Z",
		SecretKey:     jiraCredentialKey("integration-jira"),
	}); err != nil {
		t.Fatalf("save jira backend record: %v", err)
	}

	if err := instance.DeleteIntegrationConnection(context.Background(), "integration-jira"); err != nil {
		t.Fatalf("delete integration connection without local secret: %v", err)
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after delete: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if backend.Installed {
			t.Fatal("expected Jira backend to be missing after delete")
		}
		if backend.State != "missing" {
			t.Fatalf("expected Jira backend state missing after delete, got %q", backend.State)
		}
		return
	}

	t.Fatal("expected Jira backend to be present after delete")
}

func TestInstallMcpBackendRejectsUnsupportedBackend(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	_, err := instance.InstallMcpBackend(context.Background(), "unsupported")
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestRunMcpTestRejectsInvalidInput(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	_, err := instance.RunMcpTest(context.Background(), McpTestRequest{})
	if err == nil || !strings.Contains(err.Error(), "backendKey") {
		t.Fatalf("expected backendKey validation error, got %v", err)
	}

	_, err = instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "google_drive",
		ProjectID:     "project-1",
		IntegrationID: "integration-1",
		Prompt:        "List open bugs.",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match backend") {
		t.Fatalf("expected provider mismatch error, got %v", err)
	}
}

func TestRunMcpTestUsesStoredJiraCredentialAndWritesArtifacts(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		BoardID:      "1",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira search GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/search/jql") {
			t.Fatalf("expected Jira search endpoint, got %q", endpoint)
		}
		if creds.Email != "name@company.com" || creds.ApiToken != "secret-token" {
			t.Fatalf("expected stored Jira credential to be used, got %#v", creds)
		}
		return []byte(`{"issues":[{"key":"FLOW-101","fields":{"summary":"Login fails on mobile","status":{"name":"In Progress"},"assignee":{"displayName":"Taylor"}}}]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		AllowWrite:    false,
		Prompt:        "Find all open bugs in Project Alpha.",
		TimeoutMs:     1000,
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if result.RunID == "" || !strings.HasPrefix(result.RunID, "mcp_") {
		t.Fatalf("expected MCP run id, got %q", result.RunID)
	}
	if !strings.Contains(result.Command, "/rest/api/3/search/jql") {
		t.Fatalf("unexpected command %q", result.Command)
	}
	if !strings.Contains(result.StdoutSummary, "Found 1 open bug issues") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "FLOW-101") {
		t.Fatalf("expected prompt in output markdown, got %q", result.OutputMarkdown)
	}
	if len(result.ArtifactPaths) != 5 {
		t.Fatalf("expected five artifact paths, got %d", len(result.ArtifactPaths))
	}

	for _, path := range result.ArtifactPaths {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected artifact %q to exist: %v", path, statErr)
		}
	}

	runs, err := instance.ListMcpTestRuns(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-jira",
		10,
	)
	if err != nil {
		t.Fatalf("list MCP test runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run summary, got %d", len(runs))
	}
	if runs[0].RunID != result.RunID {
		t.Fatalf("expected run id %q, got %q", result.RunID, runs[0].RunID)
	}
	if runs[0].ProjectID != "project-alpha" {
		t.Fatalf("expected project id to round-trip, got %q", runs[0].ProjectID)
	}
	if runs[0].ArtifactDir != filepath.Dir(result.ArtifactPaths[0]) {
		t.Fatalf("expected artifact dir %q, got %q", filepath.Dir(result.ArtifactPaths[0]), runs[0].ArtifactDir)
	}
}

func TestRunMcpTestListsJiraUserStories(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira user story GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "issuetype+in+%28Story%2C+%22User+Story%22%29") {
			t.Fatalf("expected Jira user story endpoint, got %q", endpoint)
		}
		return []byte(`{"issues":[{"key":"SCRUM-12","fields":{"summary":"Onboarding wizard","status":{"name":"To Do"},"assignee":{"displayName":"Avery"}}}]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_list_user_stories",
		Prompt:        "Get a list of user stories in project SCRUM.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "Found 1 user stories") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "SCRUM-12") {
		t.Fatalf("expected story key in output markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestLoadsJiraTicketContent(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira issue GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/issue/SCRUM-7") {
			t.Fatalf("expected Jira ticket endpoint, got %q", endpoint)
		}
		return []byte(`{"key":"SCRUM-7","fields":{"summary":"Broken login button","status":{"name":"In Progress"},"description":"Investigate mobile web regression","assignee":{"displayName":"Jordan"}}}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_get_ticket_content",
		Prompt:        "Get the content of Jira ticket SCRUM-7.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "Loaded Jira ticket SCRUM-7") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "Broken login button") {
		t.Fatalf("expected ticket content in output markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestBackfillsLegacyJiraProjectScope(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if creds.ProjectID != "project-alpha" {
			t.Fatalf("expected legacy credential to be backfilled with project scope, got %#v", creds)
		}
		return []byte(`{"issues":[]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		Prompt:        "Find all open bugs in Project Alpha.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}

	creds, err := instance.loadJiraCredential(jiraCredentialKey("integration-jira"))
	if err != nil {
		t.Fatalf("load jira credential: %v", err)
	}
	if creds.ProjectID != "project-alpha" {
		t.Fatalf("expected saved credential project id project-alpha, got %q", creds.ProjectID)
	}
}

func TestRunMcpTestCreatesStoryWhenPromptRequestsIt(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "POST" {
			t.Fatalf("expected Jira create POST request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/issue") {
			t.Fatalf("expected Jira issue endpoint, got %q", endpoint)
		}
		if !strings.Contains(string(payload), "Redesign onboarding") {
			t.Fatalf("expected create payload to include story title, got %s", string(payload))
		}
		return []byte(`{"key":"SCRUM-42","self":"https://flowpilot899.atlassian.net/rest/api/3/issue/SCRUM-42"}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    true,
		Prompt:        "Create a story titled 'Redesign onboarding'.",
		TimeoutMs:     1000,
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "SCRUM-42") {
		t.Fatalf("expected created issue key in stdout, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "Redesign onboarding") {
		t.Fatalf("expected story title in markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestReturnsFailedResultWhenJiraVerificationFails(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		return nil, errors.New("remote backend authorization failed: 401 Unauthorized")
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    true,
		Prompt:        "Create a story titled Redesign onboarding.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "401 Unauthorized") {
		t.Fatalf("expected error message to mention failure, got %q", result.ErrorMessage)
	}
	if !strings.Contains(result.StderrSummary, "401 Unauthorized") {
		t.Fatalf("expected stderr summary to mention failure, got %q", result.StderrSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "could not create the Jira story") {
		t.Fatalf("expected failure markdown, got %q", result.OutputMarkdown)
	}
}

func TestVerifyJiraCredentialRejectsNon2xxStatuses(t *testing.T) {
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		return nil, errors.New("jira request failed: 429 rate limited")
	}

	err := (&Runner{}).verifyJiraCredential(context.Background(), jiraCredential{
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected non-2xx verification failure, got %v", err)
	}
}

func TestRunMcpTestRejectsWriteTemplateWithoutAllowWrite(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    false,
		Prompt:        "Create a story titled 'Redesign onboarding'.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "allowWrite") {
		t.Fatalf("expected allowWrite failure, got %q", result.ErrorMessage)
	}
}

func TestRunMcpTestAllowsProjectLinkReuse(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	executeJiraRequestFn = func(
		_ context.Context,
		_ string,
		_ string,
		_ jiraCredential,
		_ []byte,
	) ([]byte, error) {
		return []byte(`{"issues":[]}`), nil
	}
	t.Cleanup(func() {
		executeJiraRequestFn = func(
			ctx context.Context,
			method string,
			endpoint string,
			creds jiraCredential,
			payload []byte,
		) ([]byte, error) {
			auth := base64.StdEncoding.EncodeToString([]byte(creds.Email + ":" + creds.ApiToken))
			headers := map[string]string{
				"Authorization": "Basic " + auth,
				"Accept":        "application/json",
			}
			if payload != nil {
				headers["Content-Type"] = "application/json"
			}

			statusCode, responseBody, err := httpRequestFn(ctx, method, endpoint, headers, payload)
			if err != nil {
				return nil, err
			}
			if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
				return nil, fmt.Errorf("remote backend authorization failed: %d", statusCode)
			}
			if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
				return nil, fmt.Errorf("jira request failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
			}

			return responseBody, nil
		}
	})

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-beta",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		AllowWrite:    false,
		Prompt:        "Find all open bugs in Project Alpha.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q (%s)", result.Status, result.ErrorMessage)
	}
}

func TestListMcpTestRunsAppliesFilterAndLimit(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	baseDir := filepath.Join(instance.workspace, ".flowpilot", "mcp-tests")
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101010_0001"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101011_0002"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101012_0003"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	writeResult := func(dir string, result McpTestResult) {
		t.Helper()
		if err := writeJSONFile(filepath.Join(dir, "result.json"), result); err != nil {
			t.Fatalf("write result.json: %v", err)
		}
	}

	writeResult(filepath.Join(baseDir, "mcp_20260519_101010_0001"), McpTestResult{
		Status:        "success",
		RunID:         "mcp_20260519_101010_0001",
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-a",
		StartedAt:     "2026-05-19T10:10:10Z",
		CompletedAt:   "2026-05-19T10:10:11Z",
	})
	writeResult(filepath.Join(baseDir, "mcp_20260519_101011_0002"), McpTestResult{
		Status:        "failed",
		RunID:         "mcp_20260519_101011_0002",
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-b",
		StartedAt:     "2026-05-19T10:10:11Z",
		CompletedAt:   "2026-05-19T10:10:12Z",
	})
	writeResult(filepath.Join(baseDir, "mcp_20260519_101012_0003"), McpTestResult{
		Status:        "success",
		RunID:         "mcp_20260519_101012_0003",
		BackendKey:    "google_drive",
		ProviderType:  "google_drive",
		ProjectID:     "project-beta",
		IntegrationID: "integration-c",
		StartedAt:     "2026-05-19T10:10:12Z",
		CompletedAt:   "2026-05-19T10:10:13Z",
	})

	runs, err := instance.ListMcpTestRuns(context.Background(), "jira", "project-alpha", "", 1)
	if err != nil {
		t.Fatalf("list MCP test runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one Jira run after limit, got %d", len(runs))
	}
	if runs[0].RunID != "mcp_20260519_101011_0002" {
		t.Fatalf("expected most recent Jira run, got %q", runs[0].RunID)
	}
	if !strings.HasSuffix(runs[0].ArtifactDir, "mcp_20260519_101011_0002") {
		t.Fatalf("expected artifact dir to point at latest Jira run, got %q", runs[0].ArtifactDir)
	}
}

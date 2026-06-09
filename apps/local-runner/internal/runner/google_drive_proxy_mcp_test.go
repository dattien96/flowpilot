package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProxyMcpAccessToken_UsesArtifactSyncCredentialWhenProxyEnabled(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		if !strings.Contains(url, "oauth2.googleapis.com/token") {
			t.Fatalf("unexpected URL: %s", url)
		}
		return 200, []byte(`{"access_token":"artifact-access-token"}`), nil
	}

	server := &proxyMcpServer{runner: runner}
	token, err := server.accessToken()
	if err != nil {
		t.Fatalf("accessToken() failed: %v", err)
	}
	if token != "artifact-access-token" {
		t.Fatalf("unexpected access token: %q", token)
	}
}

func TestProxyMcpAccessToken_FailsWhenMultipleArtifactConnectionsExist(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")
	if err := runner.saveGoogleDriveCredentialByProject("project-2", googleDriveCredential{
		RefreshToken: "refresh-2",
		AccountEmail: "project-2@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject(project-2) failed: %v", err)
	}
	if err := runner.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-2"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-2",
			Status:       "connected",
			FolderID:     "folder-2",
			AccountEmail: "project-2@example.com",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	server := &proxyMcpServer{runner: runner}
	_, err := server.accessToken()
	if err == nil {
		t.Fatal("expected multiple connection error")
	}
	if !strings.Contains(err.Error(), "multiple artifact-sync Google Drive connections") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProxyMcpAuthStatusText_ReportsArtifactSyncSourceWhenProxyEnabled(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")

	server := &proxyMcpServer{runner: runner}
	statusText := server.authStatusText()
	if !strings.Contains(statusText, "source=artifact_sync") {
		t.Fatalf("expected artifact sync source in auth status, got %q", statusText)
	}
	if !strings.Contains(statusText, "projectId=project-1") {
		t.Fatalf("expected project id in auth status, got %q", statusText)
	}
}

func TestHandleWriteTool_RetainsApprovedStatusAfterTransientFailure(t *testing.T) {
	t.Setenv(googleDriveProxyMcpFlag, "true")

	workspace := t.TempDir()
	runner := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeProxyArtifactSyncConfig(t, runner, workspace)
	writeSingleProxyArtifactConnection(t, runner, "project-1")

	args := map[string]any{"name": "Docs"}
	canonicalArgsJSON, argumentsHash, err := canonicalizeGoogleDriveProxyArguments(args)
	if err != nil {
		t.Fatalf("canonicalizeGoogleDriveProxyArguments() failed: %v", err)
	}
	state := googleDriveProxyApprovalState{
		Version: 1,
		Records: map[string]googleDriveProxyApprovalRecord{
			"approval-1": {
				ID:                "approval-1",
				WorkflowRunID:     "run-1",
				WorkflowStepRunID: "step-1",
				ProcessKey:        "proc-1",
				ToolName:          "createFolder",
				CanonicalArgsJSON: canonicalArgsJSON,
				ArgumentsHash:     argumentsHash,
				Status:            "approved",
				DecisionMode:      "manual",
				RequestedAt:       time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
				DecidedAt:         time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339Nano),
				ExpiresAt:         time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	}
	if err := runner.saveGoogleDriveProxyApprovalState(state); err != nil {
		t.Fatalf("saveGoogleDriveProxyApprovalState() failed: %v", err)
	}

	requestCount := 0
	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(ctx context.Context, method, url string, headers map[string]string, body []byte) (int, []byte, error) {
		requestCount++
		if requestCount == 1 {
			return 0, nil, errors.New("temporary token refresh failure")
		}
		return 200, []byte(`{"access_token":"artifact-access-token"}`), nil
	}

	server := &proxyMcpServer{
		runner:            runner,
		mode:              "read_write",
		workflowRunID:     "run-1",
		workflowStepRunID: "step-1",
		processKey:        "proc-1",
	}
	_, err = server.handleWriteTool("createFolder", args, func(accessToken string) (string, string, string, error) {
		if accessToken != "artifact-access-token" {
			t.Fatalf("expected refreshed access token, got %q", accessToken)
		}
		return `{"status":"ok"}`, "drive-1", "https://drive.google.com/file/d/drive-1/view", nil
	})
	if err == nil || !strings.Contains(err.Error(), "temporary token refresh failure") {
		t.Fatalf("expected transient failure, got %v", err)
	}

	storedState, err := runner.loadGoogleDriveProxyApprovalState()
	if err != nil {
		t.Fatalf("loadGoogleDriveProxyApprovalState() failed: %v", err)
	}
	if got := storedState.Records["approval-1"].Status; got != "approved" {
		t.Fatalf("expected approval to remain approved after transient failure, got %q", got)
	}

	result, err := server.handleWriteTool("createFolder", args, func(accessToken string) (string, string, string, error) {
		if accessToken != "artifact-access-token" {
			t.Fatalf("expected refreshed access token, got %q", accessToken)
		}
		return `{"status":"ok"}`, "drive-1", "https://drive.google.com/file/d/drive-1/view", nil
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if !strings.Contains(mustJSON(result), `{\"status\":\"ok\"}`) {
		t.Fatalf("expected successful tool result, got %v", result)
	}

	storedState, err = runner.loadGoogleDriveProxyApprovalState()
	if err != nil {
		t.Fatalf("loadGoogleDriveProxyApprovalState() failed: %v", err)
	}
	if got := storedState.Records["approval-1"].Status; got != "executed" {
		t.Fatalf("expected approval to become executed after retry, got %q", got)
	}
}

func TestResolveWriteApproval_UsesLiveSessionDeadlineWhenShorterThanTwoHours(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{
		workspace: workspace,
		sessions:  map[string]*LiveSession{},
	}

	lastUsedAt := time.Now().UTC()
	runner.sessions["proc-short"] = &LiveSession{
		SessionID:  "session-short",
		ProcessKey: "proc-short",
		Status:     "active",
		LastUsedAt: lastUsedAt,
		IdleTTL:    30 * time.Minute,
	}

	server := &proxyMcpServer{
		runner:            runner,
		mode:              "read_write",
		workflowRunID:     "run-short",
		workflowStepRunID: "step-short",
		processKey:        "proc-short",
	}

	record, err := server.resolveWriteApproval("createFolder", map[string]any{"name": "Docs"})
	if err != nil {
		t.Fatalf("resolveWriteApproval() failed: %v", err)
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		t.Fatalf("parse ExpiresAt: %v", err)
	}
	expected := lastUsedAt.Add(30 * time.Minute)
	if diff := expiresAt.Sub(expected); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("expected approval expiry near session deadline %s, got %s", expected, expiresAt)
	}
}

func TestResolveWriteApproval_UsesLiveSessionDeadlineWhenLongerThanTwoHours(t *testing.T) {
	workspace := t.TempDir()
	runner := &Runner{
		workspace: workspace,
		sessions:  map[string]*LiveSession{},
	}

	lastUsedAt := time.Now().UTC()
	runner.sessions["proc-long"] = &LiveSession{
		SessionID:  "session-long",
		ProcessKey: "proc-long",
		Status:     "active",
		LastUsedAt: lastUsedAt,
		IdleTTL:    3 * time.Hour,
	}

	server := &proxyMcpServer{
		runner:            runner,
		mode:              "read_write",
		workflowRunID:     "run-long",
		workflowStepRunID: "step-long",
		processKey:        "proc-long",
	}

	record, err := server.resolveWriteApproval("createFolder", map[string]any{"name": "Docs"})
	if err != nil {
		t.Fatalf("resolveWriteApproval() failed: %v", err)
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if err != nil {
		t.Fatalf("parse ExpiresAt: %v", err)
	}
	expected := lastUsedAt.Add(3 * time.Hour)
	if diff := expiresAt.Sub(expected); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("expected approval expiry near session deadline %s, got %s", expected, expiresAt)
	}
}

func TestResolveWriteApproval_RejectsMissingScopingIdentifiers(t *testing.T) {
	testCases := []struct {
		name              string
		workflowRunID     string
		workflowStepRunID string
		processKey        string
		wantError         string
	}{
		{
			name:              "missing workflow run id",
			workflowStepRunID: "step-1",
			processKey:        "proc-1",
			wantError:         "workflow run id is required",
		},
		{
			name:          "missing workflow step run id",
			workflowRunID: "run-1",
			processKey:    "proc-1",
			wantError:     "workflow step run id is required",
		},
		{
			name:              "missing process key",
			workflowRunID:     "run-1",
			workflowStepRunID: "step-1",
			wantError:         "process key is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			runner := &Runner{
				workspace: workspace,
				sessions:  map[string]*LiveSession{},
			}
			server := &proxyMcpServer{
				runner:            runner,
				mode:              "read_write",
				workflowRunID:     tc.workflowRunID,
				workflowStepRunID: tc.workflowStepRunID,
				processKey:        tc.processKey,
			}

			_, err := server.resolveWriteApproval("createFolder", map[string]any{"name": "Docs"})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
			}

			state, err := runner.loadGoogleDriveProxyApprovalState()
			if err != nil {
				t.Fatalf("loadGoogleDriveProxyApprovalState() failed: %v", err)
			}
			if len(state.Records) != 0 {
				t.Fatalf("expected no approvals to be persisted, got %d", len(state.Records))
			}
		})
	}
}

func writeProxyArtifactSyncConfig(t *testing.T, runner *Runner, workspace string) {
	t.Helper()

	flowpilotDir := filepath.Join(workspace, ".flowpilot", "settings")
	if err := os.MkdirAll(flowpilotDir, 0o755); err != nil {
		t.Fatalf("Failed to create settings dir: %v", err)
	}

	wsConfig := map[string]any{
		"version": 1,
		"artifactSync": map[string]any{
			"clientId":    "artifact-client-id",
			"redirectUri": googleDriveDefaultRedirectURI,
		},
		"mcp": map[string]any{},
	}
	wsConfigBytes, err := json.Marshal(wsConfig)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(flowpilotDir, "google-drive-config.json"), wsConfigBytes, 0o644); err != nil {
		t.Fatalf("write workspace config: %v", err)
	}
	if err := runner.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, "artifact-client-secret"); err != nil {
		t.Fatalf("save artifact sync client secret: %v", err)
	}
}

func writeSingleProxyArtifactConnection(t *testing.T, runner *Runner, projectID string) {
	t.Helper()

	if err := runner.saveGoogleDriveCredentialByProject(projectID, googleDriveCredential{
		RefreshToken: "artifact-refresh-token",
		AccountEmail: projectID + "@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject() failed: %v", err)
	}
	if err := runner.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections[projectID] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    projectID,
			Status:       "connected",
			FolderID:     "folder-" + projectID,
			AccountEmail: projectID + "@example.com",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}
}

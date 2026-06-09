package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGoogleDriveWorkspaceConfigSaveAndLoad(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}

	status, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "client-id-1",
		ClientSecret: "client-secret-1",
		RedirectURI:  googleDriveDefaultRedirectURI,
		PickerAPIKey: "picker-api-key-1",
	})
	if err != nil {
		t.Fatalf("save google drive config: %v", err)
	}
	if !status.ArtifactSync.Configured {
		t.Fatalf("expected artifact sync to be configured, got %#v", status.ArtifactSync)
	}
	if status.ArtifactSync.Source != "saved" {
		t.Fatalf("expected saved source, got %#v", status.ArtifactSync.Source)
	}

	loaded, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("load google drive config: %v", err)
	}
	if !loaded.ArtifactSync.Configured {
		t.Fatalf("expected loaded artifact sync to be configured, got %#v", loaded.ArtifactSync)
	}
	if loaded.ArtifactSync.ClientID != "client-id-1" {
		t.Fatalf("unexpected client id: %#v", loaded.ArtifactSync.ClientID)
	}
	if loaded.ArtifactSync.RedirectURI != googleDriveDefaultRedirectURI {
		t.Fatalf("unexpected redirect uri: %#v", loaded.ArtifactSync.RedirectURI)
	}
	if loaded.ArtifactSync.HasClientSecret != true || loaded.ArtifactSync.HasPickerAPIKey != true {
		t.Fatalf("expected secrets to be stored, got %#v", loaded.ArtifactSync)
	}
}

func TestUploadGoogleDriveMcpOAuthCredentialsWritesCredentialFile(t *testing.T) {
	workspace := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	status, err := instance.UploadGoogleDriveMcpOAuthCredentials(GoogleDriveMcpOAuthUploadRequest{
		FileName: "gcp-oauth.keys.json",
		Content:  `{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	})
	if err != nil {
		t.Fatalf("upload google drive oauth credentials: %v", err)
	}

	credentialPath := filepath.Join(homeDir, ".config", "google-drive-mcp", "gcp-oauth.keys.json")
	raw, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if !strings.Contains(string(raw), `"client_id":"client-id"`) {
		t.Fatalf("unexpected credential file content: %s", string(raw))
	}
	if !status.MCP.CredentialFileExists || !status.MCP.CredentialFileValid {
		t.Fatalf("expected credential file status to be valid, got %#v", status.MCP)
	}
	if status.MCP.CredentialPath != credentialPath {
		t.Fatalf("unexpected credential path: %#v", status.MCP.CredentialPath)
	}
}

func TestUploadGoogleDriveMcpOAuthCredentialsPrefersXdgConfigHome(t *testing.T) {
	workspace := t.TempDir()
	homeDir := t.TempDir()
	xdgConfigHome := filepath.Join(t.TempDir(), "xdg-config")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	status, err := instance.UploadGoogleDriveMcpOAuthCredentials(GoogleDriveMcpOAuthUploadRequest{
		FileName: "gcp-oauth.keys.json",
		Content:  `{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	})
	if err != nil {
		t.Fatalf("upload google drive oauth credentials with XDG_CONFIG_HOME: %v", err)
	}

	credentialPath := filepath.Join(xdgConfigHome, "google-drive-mcp", "gcp-oauth.keys.json")
	if _, err := os.Stat(credentialPath); err != nil {
		t.Fatalf("expected credential file in XDG_CONFIG_HOME, got %v", err)
	}
	if status.MCP.CredentialPath != credentialPath {
		t.Fatalf("unexpected credential path with XDG_CONFIG_HOME: %#v", status.MCP.CredentialPath)
	}
}

func TestLoadGoogleDriveWorkspaceConfigUsesEnvFallback(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "env-client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "env-client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", googleDriveDefaultRedirectURI)
	t.Setenv("GOOGLE_PICKER_API_KEY", "env-picker-api-key")
	t.Setenv("FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP", "true")

	status, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("load google drive config from env: %v", err)
	}
	if status.ArtifactSync.Source != "env" {
		t.Fatalf("expected env source, got %#v", status.ArtifactSync.Source)
	}
	if !status.ArtifactSync.Configured {
		t.Fatalf("expected env artifact sync to be configured, got %#v", status.ArtifactSync)
	}
	if !status.MCP.ProxyMcpEnabled {
		t.Fatalf("expected proxy MCP flag to be reflected in status, got %#v", status.MCP)
	}
	if status.ArtifactSync.ClientID != "env-client-id" {
		t.Fatalf("unexpected env client id: %#v", status.ArtifactSync.ClientID)
	}
	if !status.ArtifactSync.HasClientSecret || !status.ArtifactSync.HasPickerAPIKey {
		t.Fatalf("expected env-backed secrets to be reported as present, got %#v", status.ArtifactSync)
	}
}

func TestResolveGoogleDriveArtifactRuntimeConfigPrefersSavedConfigOverEnv(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "saved-client-id",
		ClientSecret: "saved-client-secret",
		RedirectURI:  googleDriveDefaultRedirectURI,
		PickerAPIKey: "saved-picker-api-key",
	}); err != nil {
		t.Fatalf("save google drive config: %v", err)
	}

	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "env-client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "env-client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/other-callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "env-picker-api-key")

	config, err := instance.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		t.Fatalf("resolve google drive runtime config: %v", err)
	}
	if config.Source != "saved" {
		t.Fatalf("expected saved config source, got %#v", config.Source)
	}
	if config.ClientID != "saved-client-id" || config.ClientSecret != "saved-client-secret" {
		t.Fatalf("expected saved oauth client to win, got %#v", config)
	}
	if config.RedirectURI != googleDriveDefaultRedirectURI {
		t.Fatalf("expected saved redirect URI to win, got %#v", config.RedirectURI)
	}
	if config.PickerAPIKey != "saved-picker-api-key" {
		t.Fatalf("expected saved picker API key to win, got %#v", config.PickerAPIKey)
	}
}

func TestGoogleDriveWorkspaceStatusHidesSecretsFromJSONResponse(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "client-id-1",
		ClientSecret: "super-secret-client",
		RedirectURI:  googleDriveDefaultRedirectURI,
		PickerAPIKey: "super-secret-picker",
	}); err != nil {
		t.Fatalf("save google drive config: %v", err)
	}

	status, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("load google drive status: %v", err)
	}

	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal google drive status: %v", err)
	}
	if strings.Contains(string(raw), "super-secret-client") || strings.Contains(string(raw), "super-secret-picker") {
		t.Fatalf("status response leaked secret values: %s", string(raw))
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal google drive status payload: %v", err)
	}
	artifact, ok := payload["artifactSync"].(map[string]any)
	if !ok {
		t.Fatalf("artifactSync payload missing: %#v", payload)
	}
	if _, ok := artifact["clientSecret"]; ok {
		t.Fatalf("artifactSync payload exposed clientSecret: %#v", artifact)
	}
	if _, ok := artifact["pickerApiKey"]; ok {
		t.Fatalf("artifactSync payload exposed pickerApiKey: %#v", artifact)
	}
}

func TestUploadGoogleDriveMcpOAuthCredentialsRejectsWebClientJSON(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	_, err := instance.UploadGoogleDriveMcpOAuthCredentials(GoogleDriveMcpOAuthUploadRequest{
		FileName: "gcp-oauth.keys.json",
		Content:  `{"web":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	})
	if err == nil || !strings.Contains(err.Error(), "Desktop OAuth client") {
		t.Fatalf("expected Desktop OAuth validation error, got %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(homeDir, ".config", "google-drive-mcp", "gcp-oauth.keys.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected rejected web client JSON to stay unwritten, got %v", statErr)
	}
}

func TestLoadGoogleDriveWorkspaceConfigDoesNotTreatInvalidMcpCredentialFileAsReady(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	configDir := filepath.Join(homeDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir google drive mcp config dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "gcp-oauth.keys.json"),
		[]byte(`{"web":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`),
		0o600,
	); err != nil {
		t.Fatalf("write invalid google drive credential file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tokens.json"), []byte(`{"refresh_token":"token"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	status, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("load google drive config: %v", err)
	}
	if !status.MCP.CredentialFileExists {
		t.Fatalf("expected MCP credential file to exist, got %#v", status.MCP)
	}
	if status.MCP.CredentialFileValid {
		t.Fatalf("expected Web OAuth JSON to be invalid for MCP, got %#v", status.MCP)
	}
	if status.MCP.Configured {
		t.Fatalf("expected MCP to stay unconfigured when the credential JSON is invalid, got %#v", status.MCP)
	}
	if status.MCP.Status != "failed" {
		t.Fatalf("expected MCP status failed for invalid credential JSON, got %#v", status.MCP.Status)
	}
	if status.MCP.NeedsAuth {
		t.Fatalf("expected invalid credential JSON to fail before auth-needed state, got %#v", status.MCP)
	}
}

func TestValidateGoogleDriveWorkspaceConfigSkipsMcpTokenChecksUntilAuthFlowRuns(t *testing.T) {
	workspace := t.TempDir()
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	if _, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "client-id-1",
		ClientSecret: "client-secret-1",
		RedirectURI:  googleDriveDefaultRedirectURI,
		PickerAPIKey: "picker-api-key-1",
	}); err != nil {
		t.Fatalf("save google drive config: %v", err)
	}
	if _, err := instance.UploadGoogleDriveMcpOAuthCredentials(GoogleDriveMcpOAuthUploadRequest{
		FileName: "gcp-oauth.keys.json",
		Content:  `{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`,
	}); err != nil {
		t.Fatalf("upload google drive oauth credentials: %v", err)
	}

	result, err := instance.ValidateGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{})
	if err != nil {
		t.Fatalf("validate google drive config: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected validation to pass before token auth, got %#v", result.Checks)
	}

	tokenFileCheck := findGoogleDriveValidationCheck(result.Checks, "mcp_token_file_exists")
	if tokenFileCheck.Status != "skipped" {
		t.Fatalf("expected token file check to be skipped before auth flow, got %#v", tokenFileCheck)
	}

	tokenRefreshCheck := findGoogleDriveValidationCheck(result.Checks, "mcp_token_refresh_valid")
	if tokenRefreshCheck.Status != "skipped" {
		t.Fatalf("expected token refresh check to be skipped before auth flow, got %#v", tokenRefreshCheck)
	}
}

func TestValidateGoogleDriveWorkspaceConfigFailsWhenTokenFileExistsWithoutRefreshToken(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	if _, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "client-id-1",
		ClientSecret: "client-secret-1",
		RedirectURI:  googleDriveDefaultRedirectURI,
		PickerAPIKey: "picker-api-key-1",
	}); err != nil {
		t.Fatalf("save google drive config: %v", err)
	}

	configDir := filepath.Join(homeDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir google drive mcp config dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "gcp-oauth.keys.json"),
		[]byte(`{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`),
		0o600,
	); err != nil {
		t.Fatalf("write desktop oauth credential file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "tokens.json"), []byte(`{"access_token":"token-without-refresh"}`), 0o600); err != nil {
		t.Fatalf("write token file without refresh token: %v", err)
	}

	result, err := instance.ValidateGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{})
	if err != nil {
		t.Fatalf("validate google drive config: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected validation to fail when stored token is unusable, got %#v", result.Checks)
	}

	tokenFileCheck := findGoogleDriveValidationCheck(result.Checks, "mcp_token_file_exists")
	if tokenFileCheck.Status != "passed" {
		t.Fatalf("expected token file check to pass when tokens.json exists, got %#v", tokenFileCheck)
	}

	tokenRefreshCheck := findGoogleDriveValidationCheck(result.Checks, "mcp_token_refresh_valid")
	if tokenRefreshCheck.Status != "failed" {
		t.Fatalf("expected token refresh check to fail when refresh token is missing, got %#v", tokenRefreshCheck)
	}
}

func findGoogleDriveValidationCheck(checks []GoogleDriveValidationCheck, key string) GoogleDriveValidationCheck {
	for _, check := range checks {
		if check.Key == key {
			return check
		}
	}
	return GoogleDriveValidationCheck{}
}

func TestHandleGoogleDriveArtifactOAuthCallbackMarksReconnectRequiredOnInvalidGrant(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id-1")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret-1")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", googleDriveDefaultRedirectURI)
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-api-key-1")

	now := time.Now().UTC()
	sessionID := "session-1"
	stateToken := "state-token"
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Sessions[sessionID] = artifactStorageGoogleDriveSessionRecord{
			SessionID: sessionID,
			ProjectID: "project-1",
			Status:    string(ArtifactStorageGoogleDriveSessionAwaitingOAuth),
			CreatedAt: now.Format(time.RFC3339Nano),
			ExpiresAt: now.Add(time.Hour).Format(time.RFC3339Nano),
		}
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			Status:       string(ArtifactStorageGoogleDriveSessionConnected),
			FolderID:     "folder-1",
			FolderName:   "FlowPilot Root",
			AccountEmail: "owner@example.com",
			ConnectedAt:  now.Format(time.RFC3339Nano),
			UpdatedAt:    now.Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("seed google drive session state: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveSessionToken(sessionID, "state", stateToken); err != nil {
		t.Fatalf("seed oauth state token: %v", err)
	}

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(_ context.Context, method string, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		if endpoint != "https://oauth2.googleapis.com/token" {
			t.Fatalf("unexpected oauth endpoint: %s %s", method, endpoint)
		}
		return 400, []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`), nil
	}

	_, err := instance.HandleGoogleDriveArtifactOAuthCallback(sessionID+"."+stateToken, "auth-code", "")
	if err == nil || !strings.Contains(err.Error(), "reconnect Google Drive") {
		t.Fatalf("expected reconnect-required oauth callback error, got %v", err)
	}

	status, err := instance.GetGoogleDriveArtifactConnectionStatus("project-1", sessionID)
	if err != nil {
		t.Fatalf("load google drive connection status: %v", err)
	}
	if status.Connection.Status != "reconnect_required" {
		t.Fatalf("expected connection to move to reconnect_required, got %#v", status.Connection)
	}
	if !strings.Contains(status.Connection.LastError, "reconnect Google Drive") {
		t.Fatalf("expected connection last error to request reconnect, got %#v", status.Connection)
	}
	if status.Session == nil || string(status.Session.Status) != "reconnect_required" {
		t.Fatalf("expected session to move to reconnect_required on invalid_grant, got %#v", status.Session)
	}
	if status.Session == nil || !strings.Contains(status.Session.LastError, "reconnect Google Drive") {
		t.Fatalf("expected session error to request reconnect, got %#v", status.Session)
	}
}

func TestLoadArtifactStorageGoogleDriveStateFallsBackToLegacyPath(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	legacyPath := filepath.Join(instance.workspace, ".flowpilot", "artifact-storage-google-drive.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("create legacy state dir: %v", err)
	}

	payload := `{
  "sessions": {
    "session-1": {
      "sessionId": "session-1",
      "projectId": "project-1",
      "status": "connected",
      "createdAt": "2026-06-01T00:00:00Z",
      "expiresAt": "2026-06-01T01:00:00Z"
    }
  },
  "connections": {
    "project-1": {
      "projectId": "project-1",
      "status": "connected",
      "folderId": "folder-1",
      "folderName": "FlowPilot Root"
    }
  }
}`
	if err := os.WriteFile(legacyPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	state, err := instance.loadArtifactStorageGoogleDriveState()
	if err != nil {
		t.Fatalf("load artifact storage google drive state: %v", err)
	}
	if state.Connections["project-1"].FolderID != "folder-1" {
		t.Fatalf("expected legacy connection to load, got %#v", state.Connections["project-1"])
	}

	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:  "project-1",
			Status:     "connected",
			FolderID:   "folder-2",
			FolderName: "FlowPilot Settings",
		}
	}); err != nil {
		t.Fatalf("save migrated google drive state: %v", err)
	}

	newPath := filepath.Join(instance.workspace, ".flowpilot", "settings", "artifact-storage-google-drive.json")
	raw, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read migrated state: %v", err)
	}
	if !strings.Contains(string(raw), `"folderId": "folder-2"`) {
		t.Fatalf("expected migrated state to be written to new path, got %s", string(raw))
	}
}

func TestLoadGoogleDriveWorkspaceConfigDetectsMcpReconnectRequiredOnInvalidGrant(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	configDir := filepath.Join(homeDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir google drive mcp config dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "gcp-oauth.keys.json"),
		[]byte(`{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`),
		0o600,
	); err != nil {
		t.Fatalf("write desktop oauth credential file: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "tokens.json"),
		[]byte(`{"refresh_token":"refresh-token-1"}`),
		0o600,
	); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(_ context.Context, method string, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		if endpoint != "https://oauth2.googleapis.com/token" {
			t.Fatalf("unexpected oauth endpoint: %s %s", method, endpoint)
		}
		if method != "POST" {
			t.Fatalf("expected POST for google drive token refresh, got %s", method)
		}
		if headers["content-type"] != "application/x-www-form-urlencoded" {
			t.Fatalf("unexpected token refresh content type: %s", headers["content-type"])
		}
		if !strings.Contains(string(body), "refresh_token=refresh-token-1") {
			t.Fatalf("unexpected token refresh body: %s", string(body))
		}
		return 400, []byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`), nil
	}

	status, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("load google drive config: %v", err)
	}
	if status.MCP.Status != "reconnect_required" {
		t.Fatalf("expected MCP reconnect_required status, got %#v", status.MCP)
	}
	if status.MCP.Configured {
		t.Fatalf("expected MCP reconnect_required to stay unconfigured, got %#v", status.MCP)
	}
	if !status.MCP.NeedsAuth {
		t.Fatalf("expected reconnect_required to require auth again, got %#v", status.MCP)
	}
}

package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCreateGoogleDriveArtifactConnectSessionStoresHashedToken(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	session, err := instance.CreateGoogleDriveArtifactConnectSession(ArtifactStorageGoogleDriveConnectRequest{
		ProjectID: "project-1",
		BaseURL:   "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveArtifactConnectSession() failed: %v", err)
	}
	if session.ProjectID != "project-1" || session.Status != ArtifactStorageGoogleDriveSessionPending {
		t.Fatalf("unexpected session payload: %#v", session)
	}
	if !strings.Contains(session.ConnectURL, session.SessionID) {
		t.Fatalf("expected connect URL to include session id, got %q", session.ConnectURL)
	}
	if strings.Contains(session.ConnectURL, "artifact-storage:google-drive:session") {
		t.Fatalf("unexpected secret key leak in connect URL: %q", session.ConnectURL)
	}
}

func TestCreateGoogleDriveArtifactConnectSessionUsesSelectedAccountPickerFlow(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	stubGoogleDriveOAuthTokenRefresh(t)

	if err := instance.saveGoogleDriveCredentialByAccount("account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount() failed: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Accounts["account-1"] = artifactStorageGoogleDriveAccountRecord{
			AccountID:     "account-1",
			AccountEmail:  "owner@example.com",
			GrantedScopes: googleDriveArtifactRequestedScopes(),
			Status:        "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	session, err := instance.CreateGoogleDriveArtifactConnectSession(ArtifactStorageGoogleDriveConnectRequest{
		ProjectID: "project-1",
		BaseURL:   "http://127.0.0.1:4317",
		AccountID: "account-1",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveArtifactConnectSession() failed: %v", err)
	}
	if session.Status != ArtifactStorageGoogleDriveSessionAwaitingFolderPicker {
		t.Fatalf("expected awaiting folder picker status, got %#v", session)
	}
	if !strings.Contains(session.ConnectURL, "/artifact-storage/google-drive/picker?sessionId=") {
		t.Fatalf("expected direct picker URL, got %q", session.ConnectURL)
	}
	if session.AccountID != "account-1" {
		t.Fatalf("expected session account id to be preserved, got %#v", session)
	}

	state, err := instance.loadArtifactStorageGoogleDriveState()
	if err != nil {
		t.Fatalf("loadArtifactStorageGoogleDriveState() failed: %v", err)
	}
	stored, ok := state.Sessions[session.SessionID]
	if !ok {
		t.Fatalf("expected stored session %q, got %#v", session.SessionID, state.Sessions)
	}
	if stored.Status != string(ArtifactStorageGoogleDriveSessionAwaitingFolderPicker) {
		t.Fatalf("expected stored session to await folder picker, got %#v", stored)
	}
	if stored.AccountID != "account-1" {
		t.Fatalf("expected stored account id, got %#v", stored)
	}
}

func TestBuildGoogleDriveArtifactConnectRedirectFailsFastOnInvalidClientSecret(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		_ map[string]string,
		body []byte,
	) (int, []byte, error) {
		if endpoint != "https://oauth2.googleapis.com/token" {
			t.Fatalf("unexpected request: %s %s", method, endpoint)
		}
		values, _ := url.ParseQuery(string(body))
		if values.Get("code") != "flowpilot-oauth-client-validation-probe" {
			t.Fatalf("expected oauth client validation probe, got code %q", values.Get("code"))
		}
		return http.StatusUnauthorized, []byte(`{"error":"invalid_client","error_description":"The provided client secret is invalid."}`), nil
	}

	session, err := instance.CreateGoogleDriveAccountConnectSession(GoogleDriveAccountConnectRequest{
		BaseURL: "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveAccountConnectSession() failed: %v", err)
	}
	connectToken := queryValue(session.ConnectURL, "token")

	_, err = instance.BuildGoogleDriveArtifactConnectRedirect(session.SessionID, connectToken)
	if err == nil || !strings.Contains(err.Error(), "google drive OAuth client credentials are invalid") {
		t.Fatalf("expected invalid-client error before redirect, got %v", err)
	}
}

func TestGoogleDriveAccountConnectFlowPersistsGrantedScopes(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		_ map[string]string,
		body []byte,
	) (int, []byte, error) {
		if endpoint != "https://oauth2.googleapis.com/token" {
			t.Fatalf("unexpected request: %s %s", method, endpoint)
		}
		values, _ := url.ParseQuery(string(body))
		switch values.Get("grant_type") {
		case "authorization_code":
			return 200, []byte(`{"refresh_token":"refresh-token-1","scope":"https://www.googleapis.com/auth/drive.file https://www.googleapis.com/auth/drive.readonly openid email","id_token":"` + fakeGoogleIDToken("owner@example.com") + `"}`), nil
		case "refresh_token":
			return 200, []byte(`{"access_token":"access-token-1"}`), nil
		default:
			t.Fatalf("unexpected google token grant type: %q", values.Get("grant_type"))
		}
		return 0, nil, nil
	}

	session, err := instance.CreateGoogleDriveAccountConnectSession(GoogleDriveAccountConnectRequest{
		BaseURL: "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveAccountConnectSession() failed: %v", err)
	}
	connectToken := queryValue(session.ConnectURL, "token")
	redirectURL, err := instance.BuildGoogleDriveArtifactConnectRedirect(session.SessionID, connectToken)
	if err != nil {
		t.Fatalf("BuildGoogleDriveArtifactConnectRedirect() failed: %v", err)
	}
	rawState := queryValue(redirectURL, "state")

	updatedSession, err := instance.HandleGoogleDriveArtifactOAuthCallback(rawState, "oauth-code-1", "")
	if err != nil {
		t.Fatalf("HandleGoogleDriveArtifactOAuthCallback() failed: %v", err)
	}
	if updatedSession.Status != ArtifactStorageGoogleDriveSessionConnected {
		t.Fatalf("expected connected account session, got %#v", updatedSession)
	}
	if updatedSession.ProjectID != "" {
		t.Fatalf("expected account connect flow to avoid project binding, got %#v", updatedSession)
	}

	accounts, err := instance.ListGoogleDriveAccounts()
	if err != nil {
		t.Fatalf("ListGoogleDriveAccounts() failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one connected account, got %#v", accounts)
	}
	account := accounts[0]
	if account.AccountID != updatedSession.AccountID {
		t.Fatalf("expected persisted account id %q, got %#v", updatedSession.AccountID, account)
	}
	if !account.AccountReady || !account.McpReadReady || !account.McpWriteReady {
		t.Fatalf("expected account capabilities to be ready, got %#v", account)
	}
	if !strings.Contains(strings.Join(account.GrantedScopes, ","), googleDriveScopeDriveReadonly) {
		t.Fatalf("expected granted scopes to include drive.readonly, got %#v", account.GrantedScopes)
	}
	if !strings.Contains(strings.Join(account.GrantedScopes, ","), googleDriveScopeDriveFile) {
		t.Fatalf("expected granted scopes to include drive.file, got %#v", account.GrantedScopes)
	}
}

func TestDisconnectGoogleDriveAccountClearsSelectionAndMarksBindingsReconnectRequired(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	if _, err := instance.SaveGoogleDriveWorkspaceConfig(GoogleDriveWorkspaceConfigRequest{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURI:  "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback",
		PickerAPIKey: "picker-key",
		MCPAccountID: "account-1",
	}); err != nil {
		t.Fatalf("SaveGoogleDriveWorkspaceConfig() failed: %v", err)
	}
	if err := instance.saveGoogleDriveCredentialByAccount("account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount() failed: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Accounts["account-1"] = artifactStorageGoogleDriveAccountRecord{
			AccountID:     "account-1",
			AccountEmail:  "owner@example.com",
			GrantedScopes: googleDriveAccountRequestedScopes(),
			Status:        "connected",
		}
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "folder-1",
			FolderName:   "Artifacts",
			Status:       "connected",
		}
		current.Sessions["session-1"] = artifactStorageGoogleDriveSessionRecord{
			SessionID:  "session-1",
			ProjectID:  "project-1",
			AccountID:  "account-1",
			Status:     string(ArtifactStorageGoogleDriveSessionConnected),
			FolderID:   "folder-1",
			FolderName: "Artifacts",
		}
		current.ChatSyncConnections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "chat-folder-1",
			FolderName:   "Chat Sync",
			Status:       "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	if err := instance.DisconnectGoogleDriveAccount("account-1"); err != nil {
		t.Fatalf("DisconnectGoogleDriveAccount() failed: %v", err)
	}

	accounts, err := instance.ListGoogleDriveAccounts()
	if err != nil {
		t.Fatalf("ListGoogleDriveAccounts() failed: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected disconnected account to be removed, got %#v", accounts)
	}

	status, err := instance.GetGoogleDriveArtifactConnectionStatus("project-1", "")
	if err != nil {
		t.Fatalf("GetGoogleDriveArtifactConnectionStatus() failed: %v", err)
	}
	if status.Connection.Status != "reconnect_required" {
		t.Fatalf("expected project binding to require reconnect, got %#v", status.Connection)
	}
	if !strings.Contains(status.Connection.LastError, "disconnected on this runner") {
		t.Fatalf("expected disconnect reason on project binding, got %#v", status.Connection)
	}

	state, err := instance.loadArtifactStorageGoogleDriveState()
	if err != nil {
		t.Fatalf("loadArtifactStorageGoogleDriveState() failed: %v", err)
	}
	if state.Sessions["session-1"].Status != "reconnect_required" {
		t.Fatalf("expected active session to become reconnect_required, got %#v", state.Sessions["session-1"])
	}
	if state.ChatSyncConnections["project-1"].Status != "reconnect_required" {
		t.Fatalf("expected chat sync binding to become reconnect_required, got %#v", state.ChatSyncConnections["project-1"])
	}

	workspaceStatus, err := instance.LoadGoogleDriveWorkspaceConfig()
	if err != nil {
		t.Fatalf("LoadGoogleDriveWorkspaceConfig() failed: %v", err)
	}
	if workspaceStatus.MCP.AccountID != "" {
		t.Fatalf("expected MCP account selection to be cleared, got %#v", workspaceStatus.MCP)
	}
}

func TestGetGoogleDriveChatSyncConnectionStatusFallsBackToArtifactBinding(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	stubGoogleDriveOAuthTokenRefresh(t)

	if err := instance.saveGoogleDriveCredentialByAccount("account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount() failed: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Accounts["account-1"] = artifactStorageGoogleDriveAccountRecord{
			AccountID:     "account-1",
			AccountEmail:  "owner@example.com",
			GrantedScopes: googleDriveArtifactRequestedScopes(),
			Status:        "connected",
		}
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "artifact-folder-1",
			FolderName:   "Artifacts",
			Status:       "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	status, err := instance.GetGoogleDriveChatSyncConnectionStatus("project-1", "")
	if err != nil {
		t.Fatalf("GetGoogleDriveChatSyncConnectionStatus() failed: %v", err)
	}
	if status.EffectiveSource != "artifact_legacy" {
		t.Fatalf("expected artifact legacy fallback, got %#v", status)
	}
	if status.Connection.FolderID != "artifact-folder-1" || !status.Ready {
		t.Fatalf("expected artifact legacy folder to stay readable, got %#v", status)
	}
}

func TestCreateGoogleDriveChatSyncConnectSessionUsesSelectedAccountPickerFlow(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")
	stubGoogleDriveOAuthTokenRefresh(t)

	if err := instance.saveGoogleDriveCredentialByAccount("account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount() failed: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Accounts["account-1"] = artifactStorageGoogleDriveAccountRecord{
			AccountID:     "account-1",
			AccountEmail:  "owner@example.com",
			GrantedScopes: googleDriveArtifactRequestedScopes(),
			Status:        "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	session, err := instance.CreateGoogleDriveChatSyncConnectSession("project-1", ChatSyncGoogleDriveConnectRequest{
		BaseURL:   "http://127.0.0.1:4317",
		AccountID: "account-1",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveChatSyncConnectSession() failed: %v", err)
	}
	if session.Status != ArtifactStorageGoogleDriveSessionAwaitingFolderPicker {
		t.Fatalf("expected awaiting folder picker status, got %#v", session)
	}
	if !strings.Contains(session.ConnectURL, "/artifact-storage/google-drive/picker?sessionId=") {
		t.Fatalf("expected direct picker URL, got %q", session.ConnectURL)
	}

	state, err := instance.loadArtifactStorageGoogleDriveState()
	if err != nil {
		t.Fatalf("loadArtifactStorageGoogleDriveState() failed: %v", err)
	}
	if got := state.Sessions[session.SessionID].FlowKind; got != googleDriveSessionFlowChatSyncBinding {
		t.Fatalf("expected chat sync flow kind, got %q", got)
	}
}

func TestSaveGoogleDriveChatSyncFolderSelectionDoesNotMutateArtifactBinding(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		_ map[string]string,
		body []byte,
	) (int, []byte, error) {
		switch {
		case endpoint == "https://oauth2.googleapis.com/token":
			values, _ := url.ParseQuery(string(body))
			switch values.Get("grant_type") {
			case "authorization_code":
				return 200, []byte(`{"refresh_token":"refresh-token-1","id_token":"` + fakeGoogleIDToken("owner@example.com") + `"}`), nil
			case "refresh_token":
				return 200, []byte(`{"access_token":"access-token-1"}`), nil
			default:
				t.Fatalf("unexpected google token grant type: %q", values.Get("grant_type"))
			}
		case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files/chat-folder-1"):
			return 200, []byte(`{"id":"chat-folder-1","name":"Chat Sync Folder","mimeType":"application/vnd.google-apps.folder"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", method, endpoint)
		}
		return 0, nil, nil
	}

	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "artifact-folder-1",
			FolderName:   "Artifacts",
			Status:       "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	session, err := instance.CreateGoogleDriveChatSyncConnectSession("project-1", ChatSyncGoogleDriveConnectRequest{
		BaseURL: "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("CreateGoogleDriveChatSyncConnectSession() failed: %v", err)
	}
	connectToken := queryValue(session.ConnectURL, "token")
	redirectURL, err := instance.BuildGoogleDriveArtifactConnectRedirect(session.SessionID, connectToken)
	if err != nil {
		t.Fatalf("BuildGoogleDriveArtifactConnectRedirect() failed: %v", err)
	}
	rawState := queryValue(redirectURL, "state")
	if _, err := instance.HandleGoogleDriveArtifactOAuthCallback(rawState, "oauth-code-1", ""); err != nil {
		t.Fatalf("HandleGoogleDriveArtifactOAuthCallback() failed: %v", err)
	}

	status, err := instance.SaveGoogleDriveChatSyncFolderSelection(ArtifactStorageGoogleDriveFolderSelectionRequest{
		SessionID:    session.SessionID,
		FolderID:     "chat-folder-1",
		FolderName:   "Chat Sync Folder",
		AccountEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatalf("SaveGoogleDriveChatSyncFolderSelection() failed: %v", err)
	}
	if status.EffectiveSource != "chat_sync" || status.Connection.FolderID != "chat-folder-1" {
		t.Fatalf("expected dedicated chat sync binding, got %#v", status)
	}

	artifactStatus, err := instance.GetGoogleDriveArtifactConnectionStatus("project-1", "")
	if err != nil {
		t.Fatalf("GetGoogleDriveArtifactConnectionStatus() failed: %v", err)
	}
	if artifactStatus.Connection.FolderID != "artifact-folder-1" {
		t.Fatalf("expected artifact binding to stay unchanged, got %#v", artifactStatus.Connection)
	}
}

func TestRenderGoogleDriveChatSyncPickerHTMLUsesChatSyncEndpoints(t *testing.T) {
	html := RenderGoogleDriveChatSyncPickerHTML("session-1")
	for _, expected := range []string{
		`/client/chat-sync/google-drive/picker-token`,
		`/client/chat-sync/google-drive/folder-selection`,
		`Select a FlowPilot chat sync folder`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected picker HTML to contain %q", expected)
		}
	}
}

func TestListGoogleDriveAccountsAggregatesMultipleProjectBindingsForSameAccount(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	stubGoogleDriveOAuthTokenRefresh(t)

	if err := instance.saveGoogleDriveCredentialByAccount("account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount() failed: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Accounts["account-1"] = artifactStorageGoogleDriveAccountRecord{
			AccountID:     "account-1",
			AccountEmail:  "owner@example.com",
			GrantedScopes: googleDriveAccountRequestedScopes(),
			Status:        "connected",
		}
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "folder-1",
			FolderName:   "Artifacts A",
			Status:       "connected",
		}
		current.Connections["project-2"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-2",
			AccountID:    "account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "folder-2",
			FolderName:   "Artifacts B",
			Status:       "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState() failed: %v", err)
	}

	accounts, err := instance.ListGoogleDriveAccounts()
	if err != nil {
		t.Fatalf("ListGoogleDriveAccounts() failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one shared account, got %#v", accounts)
	}
	if accounts[0].ProjectCount != 2 {
		t.Fatalf("expected shared account project count 2, got %#v", accounts[0])
	}

	project1, err := instance.GetGoogleDriveArtifactConnectionStatus("project-1", "")
	if err != nil {
		t.Fatalf("GetGoogleDriveArtifactConnectionStatus(project-1) failed: %v", err)
	}
	if project1.Connection.Status != "connected" || project1.Connection.FolderID != "folder-1" {
		t.Fatalf("expected project-1 binding to stay connected, got %#v", project1.Connection)
	}

	project2, err := instance.GetGoogleDriveArtifactConnectionStatus("project-2", "")
	if err != nil {
		t.Fatalf("GetGoogleDriveArtifactConnectionStatus(project-2) failed: %v", err)
	}
	if project2.Connection.Status != "connected" || project2.Connection.FolderID != "folder-2" {
		t.Fatalf("expected project-2 binding to stay connected, got %#v", project2.Connection)
	}
}

func TestRenderGoogleDriveArtifactPickerHTMLUsesSelectableFolderView(t *testing.T) {
	html := RenderGoogleDriveArtifactPickerHTML("session-1")
	for _, expected := range []string{
		`const folderMimeType = "application/vnd.google-apps.folder";`,
		`const pickerOrigin = window.location.protocol + "//" + window.location.host;`,
		`const pickerRelayUrl = pickerOrigin + "/artifact-storage/google-drive/picker-relay";`,
		`new google.picker.DocsView()`,
		`.setMimeTypes(folderMimeType)`,
		`.setSelectFolderEnabled(true)`,
		`.setOrigin(pickerOrigin)`,
		`.setRelayUrl(pickerRelayUrl)`,
		`.setSelectableMimeTypes(folderMimeType)`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected picker HTML to contain %q", expected)
		}
	}
	if strings.Contains(html, "google.picker.ViewId.FOLDERS") {
		t.Fatalf("picker HTML should not use ViewId.FOLDERS because it keeps folder selection disabled")
	}
}

func TestRenderGoogleDriveArtifactPickerRelayHTMLReturnsDocument(t *testing.T) {
	html := RenderGoogleDriveArtifactPickerRelayHTML()
	for _, expected := range []string{
		`<!doctype html>`,
		`<title>FlowPilot Google Drive Picker Relay</title>`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected relay HTML to contain %q", expected)
		}
	}
}

func TestGoogleDriveArtifactOAuthAndFolderSelectionFlow(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		switch {
		case endpoint == "https://oauth2.googleapis.com/token":
			values, _ := url.ParseQuery(string(body))
			switch values.Get("grant_type") {
			case "authorization_code":
				return 200, []byte(`{"refresh_token":"refresh-token-1","id_token":"` + fakeGoogleIDToken("owner@example.com") + `"}`), nil
			case "refresh_token":
				return 200, []byte(`{"access_token":"access-token-1"}`), nil
			default:
				t.Fatalf("unexpected google token grant type: %q", values.Get("grant_type"))
			}
		case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files/folder-1"):
			return 200, []byte(`{"id":"folder-1","name":"FlowPilot Drive","mimeType":"application/vnd.google-apps.folder"}`), nil
		default:
			t.Fatalf("unexpected request: %s %s %#v", method, endpoint, headers)
		}
		return 0, nil, nil
	}

	session, err := instance.CreateGoogleDriveArtifactConnectSession(ArtifactStorageGoogleDriveConnectRequest{
		ProjectID: "project-1",
		BaseURL:   "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("create connect session: %v", err)
	}
	connectToken := queryValue(session.ConnectURL, "token")
	redirectURL, err := instance.BuildGoogleDriveArtifactConnectRedirect(session.SessionID, connectToken)
	if err != nil {
		t.Fatalf("BuildGoogleDriveArtifactConnectRedirect() failed: %v", err)
	}
	rawState := queryValue(redirectURL, "state")

	updatedSession, err := instance.HandleGoogleDriveArtifactOAuthCallback(rawState, "oauth-code-1", "")
	if err != nil {
		t.Fatalf("HandleGoogleDriveArtifactOAuthCallback() failed: %v", err)
	}
	if updatedSession.Status != ArtifactStorageGoogleDriveSessionAwaitingFolderPicker {
		t.Fatalf("expected awaiting folder picker status, got %#v", updatedSession)
	}

	pickerToken, err := instance.GetGoogleDriveArtifactPickerToken(session.SessionID)
	if err != nil {
		t.Fatalf("GetGoogleDriveArtifactPickerToken() failed: %v", err)
	}
	if pickerToken.AccessToken != "access-token-1" || pickerToken.ApiKey != "picker-key" {
		t.Fatalf("unexpected picker token payload: %#v", pickerToken)
	}

	status, err := instance.SaveGoogleDriveArtifactFolderSelection(ArtifactStorageGoogleDriveFolderSelectionRequest{
		SessionID:    session.SessionID,
		FolderID:     "folder-1",
		FolderName:   "FlowPilot Drive",
		AccountEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatalf("SaveGoogleDriveArtifactFolderSelection() failed: %v", err)
	}
	if status.Connection.Status != "connected" || status.Connection.FolderID != "folder-1" {
		t.Fatalf("unexpected connection status: %#v", status)
	}
	if status.Session == nil || status.Session.Status != ArtifactStorageGoogleDriveSessionConnected {
		t.Fatalf("expected connected session state, got %#v", status.Session)
	}
}

func TestSaveGoogleDriveArtifactFolderSelectionFallsBackToPickerPayloadOnLookupFailure(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		_ context.Context,
		method string,
		endpoint string,
		_ map[string]string,
		body []byte,
	) (int, []byte, error) {
		switch {
		case endpoint == "https://oauth2.googleapis.com/token":
			values, _ := url.ParseQuery(string(body))
			switch values.Get("grant_type") {
			case "authorization_code":
				return 200, []byte(`{"refresh_token":"refresh-token-1","id_token":"` + fakeGoogleIDToken("owner@example.com") + `"}`), nil
			case "refresh_token":
				return 200, []byte(`{"access_token":"access-token-1"}`), nil
			default:
				t.Fatalf("unexpected google token grant type: %q", values.Get("grant_type"))
			}
		case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files/folder-404"):
			return 404, []byte(`{"error":{"code":404,"message":"File not found"}}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", method, endpoint)
		}
		return 0, nil, nil
	}

	session, err := instance.CreateGoogleDriveArtifactConnectSession(ArtifactStorageGoogleDriveConnectRequest{
		ProjectID: "project-1",
		BaseURL:   "http://127.0.0.1:4317",
	})
	if err != nil {
		t.Fatalf("create connect session: %v", err)
	}
	connectToken := queryValue(session.ConnectURL, "token")
	redirectURL, err := instance.BuildGoogleDriveArtifactConnectRedirect(session.SessionID, connectToken)
	if err != nil {
		t.Fatalf("BuildGoogleDriveArtifactConnectRedirect() failed: %v", err)
	}
	rawState := queryValue(redirectURL, "state")
	if _, err := instance.HandleGoogleDriveArtifactOAuthCallback(rawState, "oauth-code-1", ""); err != nil {
		t.Fatalf("HandleGoogleDriveArtifactOAuthCallback() failed: %v", err)
	}

	status, err := instance.SaveGoogleDriveArtifactFolderSelection(ArtifactStorageGoogleDriveFolderSelectionRequest{
		SessionID:    session.SessionID,
		FolderID:     "folder-404",
		FolderName:   "FlowPilot_Artifacts",
		AccountEmail: "owner@example.com",
	})
	if err != nil {
		t.Fatalf("SaveGoogleDriveArtifactFolderSelection() failed: %v", err)
	}
	if status.Connection.Status != "connected" {
		t.Fatalf("expected connected status, got %#v", status.Connection)
	}
	if status.Connection.FolderID != "folder-404" || status.Connection.FolderName != "FlowPilot_Artifacts" {
		t.Fatalf("expected picker payload fallback to persist selection, got %#v", status.Connection)
	}
}

func fakeGoogleIDToken(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(map[string]string{"email": email})
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func queryValue(rawURL, key string) string {
	parsed, _ := url.Parse(rawURL)
	return parsed.Query().Get(key)
}

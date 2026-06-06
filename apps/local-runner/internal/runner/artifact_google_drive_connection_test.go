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

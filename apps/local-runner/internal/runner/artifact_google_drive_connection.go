package runner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	googleDriveArtifactConnectPath         = "/artifact-storage/google-drive/connect"
	googleDriveArtifactOAuthCallbackPath   = "/artifact-storage/google-drive/oauth/callback"
	googleDriveArtifactPickerPath          = "/artifact-storage/google-drive/picker"
	googleDriveArtifactPickerRelayPath     = "/artifact-storage/google-drive/picker-relay"
	googleDriveArtifactSessionSecretPrefix = "artifact-storage:google-drive:session"
)

type artifactStorageGoogleDriveState struct {
	Sessions    map[string]artifactStorageGoogleDriveSessionRecord    `json:"sessions"`
	Connections map[string]artifactStorageGoogleDriveConnectionRecord `json:"connections"`
}

type artifactStorageGoogleDriveSessionRecord struct {
	SessionID    string `json:"sessionId"`
	ProjectID    string `json:"projectId"`
	Status       string `json:"status"`
	CreatedAt    string `json:"createdAt"`
	ExpiresAt    string `json:"expiresAt"`
	ConnectedAt  string `json:"connectedAt,omitempty"`
	AccountEmail string `json:"accountEmail,omitempty"`
	FolderID     string `json:"folderId,omitempty"`
	FolderName   string `json:"folderName,omitempty"`
	LastError    string `json:"lastError,omitempty"`
}

type artifactStorageGoogleDriveConnectionRecord struct {
	ProjectID       string `json:"projectId"`
	Status          string `json:"status"`
	FolderID        string `json:"folderId,omitempty"`
	FolderName      string `json:"folderName,omitempty"`
	AccountEmail    string `json:"accountEmail,omitempty"`
	LastError       string `json:"lastError,omitempty"`
	LastValidatedAt string `json:"lastValidatedAt,omitempty"`
	ConnectedAt     string `json:"connectedAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
}

type googleDriveOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func (r *Runner) CreateGoogleDriveArtifactConnectSession(request ArtifactStorageGoogleDriveConnectRequest) (ArtifactStorageGoogleDriveSession, error) {
	projectID := strings.TrimSpace(request.ProjectID)
	if projectID == "" {
		return ArtifactStorageGoogleDriveSession{}, errors.New("projectId is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		return ArtifactStorageGoogleDriveSession{}, errors.New("baseUrl is required")
	}
	if _, err := r.resolveGoogleDriveArtifactRuntimeConfig(); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	connectToken := newArtifactStorageGoogleDriveToken()
	state := artifactStorageGoogleDriveSessionRecord{
		SessionID: newArtifactStorageGoogleDriveSessionID(),
		ProjectID: projectID,
		Status:    string(ArtifactStorageGoogleDriveSessionPending),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt: time.Now().UTC().Add(time.Duration(artifactStorageGoogleDriveDefaultSessionTTL) * time.Second).Format(time.RFC3339Nano),
	}

	if err := r.saveArtifactStorageGoogleDriveSessionToken(state.SessionID, "connect", connectToken); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		if current.Sessions == nil {
			current.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
		}
		current.Sessions[state.SessionID] = state
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	connectURL := fmt.Sprintf(
		"%s%s?sessionId=%s&token=%s",
		baseURL,
		googleDriveArtifactConnectPath,
		url.QueryEscape(state.SessionID),
		url.QueryEscape(connectToken),
	)
	return mapArtifactStorageGoogleDriveSession(state, connectURL), nil
}

func (r *Runner) BuildGoogleDriveArtifactConnectRedirect(sessionID, connectToken string) (string, error) {
	session, err := r.getArtifactStorageGoogleDriveSession(sessionID)
	if err != nil {
		return "", err
	}
	if session.Status == string(ArtifactStorageGoogleDriveSessionExpired) {
		return "", errors.New("google drive connect session has expired")
	}
	if artifactStorageGoogleDriveSessionExpired(session.ExpiresAt) {
		_ = r.updateArtifactStorageGoogleDriveSession(session.SessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
			record.Status = string(ArtifactStorageGoogleDriveSessionExpired)
			record.LastError = "Connection session expired before Google OAuth started."
		})
		return "", errors.New("google drive connect session has expired")
	}
	valid, err := r.verifyArtifactStorageGoogleDriveSessionToken(sessionID, "connect", connectToken)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", errors.New("google drive connect token is invalid")
	}

	stateToken := newArtifactStorageGoogleDriveToken()
	if err := r.saveArtifactStorageGoogleDriveSessionToken(sessionID, "state", stateToken); err != nil {
		return "", err
	}
	if err := r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
		record.Status = string(ArtifactStorageGoogleDriveSessionAwaitingOAuth)
		record.LastError = ""
	}); err != nil {
		return "", err
	}

	config, err := r.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", config.RedirectURI)
	query.Set("response_type", "code")
	query.Set("scope", "https://www.googleapis.com/auth/drive.file openid email")
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	query.Set("include_granted_scopes", "true")
	query.Set("state", sessionID+"."+stateToken)

	return "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode(), nil
}

func (r *Runner) HandleGoogleDriveArtifactOAuthCallback(rawState, code, providerError string) (ArtifactStorageGoogleDriveSession, error) {
	sessionID, stateToken, err := splitArtifactStorageGoogleDriveState(rawState)
	if err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	session, err := r.getArtifactStorageGoogleDriveSession(sessionID)
	if err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	if providerError != "" {
		_ = r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
			record.Status = string(ArtifactStorageGoogleDriveSessionFailed)
			record.LastError = providerError
		})
		return ArtifactStorageGoogleDriveSession{}, errors.New(providerError)
	}
	valid, err := r.verifyArtifactStorageGoogleDriveSessionToken(sessionID, "state", stateToken)
	if err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	if !valid {
		return ArtifactStorageGoogleDriveSession{}, errors.New("google drive oauth state is invalid")
	}
	if strings.TrimSpace(code) == "" {
		return ArtifactStorageGoogleDriveSession{}, errors.New("google drive oauth code is required")
	}

	tokenResponse, err := r.exchangeGoogleDriveOAuthCode(code)
	if err != nil {
		_ = r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
			record.Status = "reconnect_required"
			record.LastError = err.Error()
		})
		r.markGoogleDriveArtifactReconnectRequired(session.ProjectID, sessionID, err)
		return ArtifactStorageGoogleDriveSession{}, err
	}

	refreshToken := strings.TrimSpace(tokenResponse.RefreshToken)
	if refreshToken == "" {
		previous, previousErr := r.loadGoogleDriveCredentialByProject(session.ProjectID)
		if previousErr != nil {
			_ = r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
				record.Status = string(ArtifactStorageGoogleDriveSessionFailed)
				record.LastError = "google drive oauth response did not include a refresh token"
			})
			return ArtifactStorageGoogleDriveSession{}, errors.New("google drive oauth response did not include a refresh token")
		}
		refreshToken = previous.RefreshToken
	}

	accountEmail := parseGoogleDriveIDTokenEmail(tokenResponse.IDToken)
	if err := r.saveGoogleDriveCredentialByProject(session.ProjectID, googleDriveCredential{
		RefreshToken: refreshToken,
		AccountEmail: accountEmail,
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	if err := r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
		record.Status = string(ArtifactStorageGoogleDriveSessionAwaitingFolderPicker)
		record.AccountEmail = accountEmail
		record.LastError = ""
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	session, err = r.getArtifactStorageGoogleDriveSession(sessionID)
	if err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	return mapArtifactStorageGoogleDriveSession(session, ""), nil
}

func (r *Runner) GetGoogleDriveArtifactPickerToken(sessionID string) (ArtifactStorageGoogleDrivePickerToken, error) {
	session, err := r.getArtifactStorageGoogleDriveSession(sessionID)
	if err != nil {
		return ArtifactStorageGoogleDrivePickerToken{}, err
	}
	if session.Status != string(ArtifactStorageGoogleDriveSessionAwaitingFolderPicker) && session.Status != string(ArtifactStorageGoogleDriveSessionConnected) {
		return ArtifactStorageGoogleDrivePickerToken{}, errors.New("google drive picker is not ready for this session")
	}
	creds, err := r.loadGoogleDriveCredentialByProject(session.ProjectID)
	if err != nil {
		return ArtifactStorageGoogleDrivePickerToken{}, err
	}
	accessToken, err := r.refreshGoogleDriveAccessToken(creds.RefreshToken)
	if err != nil {
		r.markGoogleDriveArtifactReconnectRequired(session.ProjectID, sessionID, err)
		return ArtifactStorageGoogleDrivePickerToken{}, err
	}
	config, err := r.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		return ArtifactStorageGoogleDrivePickerToken{}, err
	}
	return ArtifactStorageGoogleDrivePickerToken{
		AccessToken: accessToken,
		ApiKey:      config.PickerAPIKey,
	}, nil
}

func (r *Runner) SaveGoogleDriveArtifactFolderSelection(request ArtifactStorageGoogleDriveFolderSelectionRequest) (ArtifactStorageGoogleDriveConnectionStatus, error) {
	sessionID := strings.TrimSpace(request.SessionID)
	if sessionID == "" {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("sessionId is required")
	}
	session, err := r.getArtifactStorageGoogleDriveSession(sessionID)
	if err != nil {
		return ArtifactStorageGoogleDriveConnectionStatus{}, err
	}

	folderID := strings.TrimSpace(request.FolderID)
	if folderID == "" {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("folderId is required")
	}
	creds, err := r.loadGoogleDriveCredentialByProject(session.ProjectID)
	if err != nil {
		return ArtifactStorageGoogleDriveConnectionStatus{}, err
	}
	accessToken, err := r.refreshGoogleDriveAccessToken(creds.RefreshToken)
	if err != nil {
		r.markGoogleDriveArtifactReconnectRequired(session.ProjectID, sessionID, err)
		return ArtifactStorageGoogleDriveConnectionStatus{}, err
	}
	folderInfo, err := fetchGoogleDriveFileByID(accessToken, folderID)
	if err != nil {
		if !shouldFallbackToPickedGoogleDriveFolder(err) {
			return ArtifactStorageGoogleDriveConnectionStatus{}, err
		}
		// Google Picker already constrained the selection to folders. Some
		// drive.file refresh tokens cannot immediately re-read an existing folder
		// by ID even though the user just picked it, so we trust the picker payload.
		folderInfo = googleDriveFile{
			ID:       folderID,
			Name:     firstNonEmptyGoogleDriveValue(request.FolderName, folderID),
			MimeType: googleDriveFolderMimeType,
		}
	}
	if folderInfo.MimeType != googleDriveFolderMimeType {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("selected Google Drive item is not a folder")
	}

	accountEmail := strings.TrimSpace(request.AccountEmail)
	if accountEmail == "" {
		accountEmail = creds.AccountEmail
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		if current.Connections == nil {
			current.Connections = map[string]artifactStorageGoogleDriveConnectionRecord{}
		}
		current.Connections[session.ProjectID] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:       session.ProjectID,
			Status:          string(ArtifactStorageGoogleDriveSessionConnected),
			FolderID:        folderInfo.ID,
			FolderName:      folderInfo.Name,
			AccountEmail:    accountEmail,
			LastError:       "",
			LastValidatedAt: now,
			ConnectedAt:     now,
			UpdatedAt:       now,
		}
		if current.Sessions == nil {
			current.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
		}
		record := current.Sessions[sessionID]
		record.Status = string(ArtifactStorageGoogleDriveSessionConnected)
		record.FolderID = folderInfo.ID
		record.FolderName = folderInfo.Name
		record.AccountEmail = accountEmail
		record.ConnectedAt = now
		record.LastError = ""
		current.Sessions[sessionID] = record
	}); err != nil {
		return ArtifactStorageGoogleDriveConnectionStatus{}, err
	}

	return r.GetGoogleDriveArtifactConnectionStatus(session.ProjectID, sessionID)
}

func (r *Runner) GetGoogleDriveArtifactConnectionStatus(projectID, sessionID string) (ArtifactStorageGoogleDriveConnectionStatus, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("projectId is required")
	}
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return ArtifactStorageGoogleDriveConnectionStatus{}, err
	}

	connection := ArtifactStorageGoogleDriveConnection{
		ProjectID: projectID,
		Status:    "disconnected",
	}
	if record, ok := state.Connections[projectID]; ok {
		connection = mapArtifactStorageGoogleDriveConnection(record)
	}

	var session *ArtifactStorageGoogleDriveSession
	if trimmedSessionID := strings.TrimSpace(sessionID); trimmedSessionID != "" {
		if record, ok := state.Sessions[trimmedSessionID]; ok && record.ProjectID == projectID {
			mapped := mapArtifactStorageGoogleDriveSession(record, "")
			session = &mapped
		}
	}

	return ArtifactStorageGoogleDriveConnectionStatus{
		Connection: connection,
		Session:    session,
	}, nil
}

func (r *Runner) getArtifactStorageGoogleDriveSession(sessionID string) (artifactStorageGoogleDriveSessionRecord, error) {
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return artifactStorageGoogleDriveSessionRecord{}, err
	}
	record, ok := state.Sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return artifactStorageGoogleDriveSessionRecord{}, os.ErrNotExist
	}
	return record, nil
}

func (r *Runner) updateArtifactStorageGoogleDriveSession(sessionID string, apply func(*artifactStorageGoogleDriveSessionRecord)) error {
	return r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		record := current.Sessions[sessionID]
		apply(&record)
		current.Sessions[sessionID] = record
	})
}

func (r *Runner) loadArtifactStorageGoogleDriveState() (artifactStorageGoogleDriveState, error) {
	path := r.artifactStorageGoogleDriveStatePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return artifactStorageGoogleDriveState{
				Sessions:    map[string]artifactStorageGoogleDriveSessionRecord{},
				Connections: map[string]artifactStorageGoogleDriveConnectionRecord{},
			}, nil
		}
		return artifactStorageGoogleDriveState{}, err
	}

	var state artifactStorageGoogleDriveState
	if err := json.Unmarshal(raw, &state); err != nil {
		return artifactStorageGoogleDriveState{}, err
	}
	if state.Sessions == nil {
		state.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
	}
	if state.Connections == nil {
		state.Connections = map[string]artifactStorageGoogleDriveConnectionRecord{}
	}
	return state, nil
}

func (r *Runner) saveArtifactStorageGoogleDriveState(apply func(*artifactStorageGoogleDriveState)) error {
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return err
	}
	apply(&state)
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := r.artifactStorageGoogleDriveStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (r *Runner) artifactStorageGoogleDriveStatePath() string {
	return filepath.Join(r.workspace, ".flowpilot", "artifact-storage-google-drive.json")
}

func (r *Runner) saveArtifactStorageGoogleDriveSessionToken(sessionID, keyType, rawValue string) error {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawValue)))
	return r.ensureSecretStore().Set(
		artifactStorageGoogleDriveSessionSecretKey(sessionID, keyType),
		hex.EncodeToString(sum[:]),
	)
}

func (r *Runner) verifyArtifactStorageGoogleDriveSessionToken(sessionID, keyType, rawValue string) (bool, error) {
	expectedHash, err := r.ensureSecretStore().Get(artifactStorageGoogleDriveSessionSecretKey(sessionID, keyType))
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawValue)))
	return expectedHash == hex.EncodeToString(sum[:]), nil
}

func artifactStorageGoogleDriveSessionSecretKey(sessionID, keyType string) string {
	return normalizeSecretKey(googleDriveArtifactSessionSecretPrefix, keyType, sessionID)
}

func artifactStorageGoogleDriveSessionExpired(expiresAt string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(expiresAt))
	if err != nil {
		return true
	}
	return time.Now().UTC().After(parsed)
}

func mapArtifactStorageGoogleDriveSession(record artifactStorageGoogleDriveSessionRecord, connectURL string) ArtifactStorageGoogleDriveSession {
	return ArtifactStorageGoogleDriveSession{
		SessionID:    record.SessionID,
		ProjectID:    record.ProjectID,
		Status:       ArtifactStorageGoogleDriveSessionStatus(record.Status),
		ConnectURL:   connectURL,
		ExpiresAt:    record.ExpiresAt,
		ConnectedAt:  record.ConnectedAt,
		AccountEmail: record.AccountEmail,
		FolderID:     record.FolderID,
		FolderName:   record.FolderName,
		LastError:    record.LastError,
	}
}

func mapArtifactStorageGoogleDriveConnection(record artifactStorageGoogleDriveConnectionRecord) ArtifactStorageGoogleDriveConnection {
	return ArtifactStorageGoogleDriveConnection{
		ProjectID:       record.ProjectID,
		Status:          record.Status,
		FolderID:        record.FolderID,
		FolderName:      record.FolderName,
		AccountEmail:    record.AccountEmail,
		LastError:       record.LastError,
		LastValidatedAt: record.LastValidatedAt,
		ConnectedAt:     record.ConnectedAt,
		UpdatedAt:       record.UpdatedAt,
	}
}

func (r *Runner) markGoogleDriveArtifactReconnectRequired(projectID, sessionID string, err error) {
	if !googleDriveReconnectRequired(err) {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		trimmedProjectID := strings.TrimSpace(projectID)
		if trimmedProjectID != "" {
			record := current.Connections[trimmedProjectID]
			record.ProjectID = trimmedProjectID
			record.Status = "reconnect_required"
			record.LastError = err.Error()
			record.LastValidatedAt = now
			record.UpdatedAt = now
			current.Connections[trimmedProjectID] = record
		}

		trimmedSessionID := strings.TrimSpace(sessionID)
		if trimmedSessionID == "" {
			return
		}

		record, ok := current.Sessions[trimmedSessionID]
		if !ok {
			return
		}
		record.Status = "reconnect_required"
		record.LastError = err.Error()
		current.Sessions[trimmedSessionID] = record
	})
}

func newArtifactStorageGoogleDriveSessionID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("gdrv-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func newArtifactStorageGoogleDriveToken() string {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func splitArtifactStorageGoogleDriveState(rawState string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(rawState), ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("google drive oauth state is invalid")
	}
	return parts[0], parts[1], nil
}

func exchangeGoogleDriveOAuthCode(code string) (googleDriveOAuthTokenResponse, error) {
	config, err := readGoogleDriveArtifactConfig()
	if err != nil {
		return googleDriveOAuthTokenResponse{}, err
	}
	redirectURI := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_REDIRECT_URI"))
	form := url.Values{}
	form.Set("client_id", config.clientID)
	form.Set("client_secret", config.clientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)
	form.Set("code", strings.TrimSpace(code))

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		"POST",
		"https://oauth2.googleapis.com/token",
		map[string]string{"content-type": "application/x-www-form-urlencoded"},
		[]byte(form.Encode()),
	)
	if requestErr != nil {
		return googleDriveOAuthTokenResponse{}, requestErr
	}
	if statusCode < 200 || statusCode >= 300 {
		return googleDriveOAuthTokenResponse{}, fmt.Errorf("google drive oauth code exchange failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}
	var response googleDriveOAuthTokenResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return googleDriveOAuthTokenResponse{}, err
	}
	return response, nil
}

func parseGoogleDriveIDTokenEmail(idToken string) string {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Email)
}

func shouldFallbackToPickedGoogleDriveFolder(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "google drive file lookup by id failed: 403") ||
		strings.Contains(message, "google drive file lookup by id failed: 404")
}

func firstNonEmptyGoogleDriveValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func RenderGoogleDriveArtifactPickerHTML(sessionID string) string {
	return fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>FlowPilot Google Drive Folder Picker</title>
    <script src="https://apis.google.com/js/api.js"></script>
    <script src="https://accounts.google.com/gsi/client" async defer></script>
    <style>
      body { font-family: sans-serif; padding: 24px; background: #f6f7f9; color: #111827; }
      .card { max-width: 640px; margin: 32px auto; background: white; border-radius: 16px; padding: 24px; box-shadow: 0 12px 32px rgba(0,0,0,.08); }
      .muted { color: #6b7280; }
      .error { color: #b91c1c; }
      .debug { display: none; margin-top: 16px; padding: 12px; border-radius: 12px; background: #f3f4f6; font-size: 12px; line-height: 1.5; white-space: pre-wrap; word-break: break-word; }
      button { border: none; border-radius: 999px; padding: 12px 18px; font-weight: 600; cursor: pointer; background: #111827; color: white; }
    </style>
  </head>
  <body>
    <div class="card">
      <h1>Select a Google Drive folder</h1>
      <p class="muted">FlowPilot will store synced artifacts below the folder you choose on this runner host.</p>
      <p id="status">Preparing Google Picker…</p>
      <button id="retry" style="display:none">Retry picker</button>
      <pre id="debug" class="debug"></pre>
    </div>
    <script>
      const sessionId = %q;
      const pickerOrigin = window.location.protocol + "//" + window.location.host;
      const pickerRelayUrl = pickerOrigin + %q;
      const statusEl = document.getElementById("status");
      const retryButton = document.getElementById("retry");
      const debugEl = document.getElementById("debug");
      let pickerReady = false;
      let oauthPayload = null;

      function setStatus(message, isError) {
        statusEl.textContent = message;
        statusEl.className = isError ? "error" : "";
      }

      function setDebug(value) {
        if (!debugEl) {
          return;
        }
        if (!value) {
          debugEl.style.display = "none";
          debugEl.textContent = "";
          return;
        }
        debugEl.style.display = "block";
        debugEl.textContent = typeof value === "string" ? value : JSON.stringify(value, null, 2);
      }

      async function loadPicker() {
        try {
          const response = await fetch("/artifact-storage/google-drive/picker-token?sessionId=" + encodeURIComponent(sessionId), { cache: "no-store" });
          const payload = await response.json();
          if (!response.ok) {
            throw new Error(payload.error || "Unable to prepare the Google Picker token.");
          }
          oauthPayload = payload;
          await new Promise((resolve) => window.gapi.load("picker", resolve));
          pickerReady = true;
          openPicker();
        } catch (error) {
          setStatus(error instanceof Error ? error.message : "Unable to prepare Google Picker.", true);
          retryButton.style.display = "inline-flex";
        }
      }

      function openPicker() {
        if (!pickerReady || !oauthPayload) {
          return;
        }
        retryButton.style.display = "none";
        setStatus("Waiting for folder selection…", false);
        const folderMimeType = "application/vnd.google-apps.folder";
        const view = new google.picker.DocsView()
          .setIncludeFolders(true)
          .setMimeTypes(folderMimeType)
          .setSelectFolderEnabled(true);
        const picker = new google.picker.PickerBuilder()
          .setDeveloperKey(oauthPayload.apiKey)
          .setOAuthToken(oauthPayload.accessToken)
          .setOrigin(pickerOrigin)
          .setRelayUrl(pickerRelayUrl)
          .addView(view)
          .setSelectableMimeTypes(folderMimeType)
          .setTitle("Select a FlowPilot artifact folder")
          .setCallback(async (data) => {
            const action = data[google.picker.Response.ACTION] || data.action;
            const docs = data[google.picker.Response.DOCUMENTS] || data.docs || [];
            setDebug({
              action,
              docs: docs.map((doc) => ({
                id: doc[google.picker.Document.ID] || doc.id || "",
                name: doc[google.picker.Document.NAME] || doc.name || "",
                mimeType: doc[google.picker.Document.MIME_TYPE] || doc.mimeType || "",
              })),
            });
            if (action === google.picker.Action.PICKED && docs.length > 0) {
              const folder = docs[0];
              const folderId = folder[google.picker.Document.ID] || folder.id || "";
              const folderName = folder[google.picker.Document.NAME] || folder.name || "";
              try {
                setStatus("Saving the selected folder…", false);
                const saveResponse = await fetch("/artifact-storage/google-drive/folder-selection", {
                  method: "POST",
                  headers: { "content-type": "application/json" },
                  body: JSON.stringify({
                    sessionId,
                    folderId,
                    folderName,
                  }),
                });
                const saveBody = await saveResponse.text();
                let savePayload = {};
                try {
                  savePayload = saveBody ? JSON.parse(saveBody) : {};
                } catch (parseError) {
                  savePayload = { raw: saveBody, parseError: parseError instanceof Error ? parseError.message : "Unable to parse response." };
                }
                setDebug({
                  action,
                  folderId,
                  folderName,
                  saveStatus: saveResponse.status,
                  savePayload,
                });
                if (!saveResponse.ok) {
                  throw new Error(savePayload.error || "Unable to save the selected folder.");
                }
                picker.setVisible(false);
                setStatus("Google Drive is connected. You can close this tab.", false);
                if (window.opener) {
                  window.opener.postMessage({ type: "flowpilot-google-drive-connected", projectId: savePayload.connection?.projectId || "" }, "*");
                }
                window.setTimeout(() => window.close(), 350);
              } catch (error) {
                setStatus(error instanceof Error ? error.message : "Unable to save the selected folder.", true);
              }
            } else if (action === google.picker.Action.CANCEL) {
              setStatus("Folder selection was cancelled. You can retry below.", true);
              retryButton.style.display = "inline-flex";
            } else {
              setStatus("Picker returned " + String(action || "an unknown action") + ".", true);
            }
          })
          .build();
        picker.setVisible(true);
      }

      retryButton.addEventListener("click", () => {
        if (pickerReady) {
          openPicker();
        } else {
          void loadPicker();
        }
      });

      void loadPicker();
    </script>
  </body>
</html>`, sessionID, googleDriveArtifactPickerRelayPath)
}

func RenderGoogleDriveArtifactPickerRelayHTML() string {
	return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>FlowPilot Google Drive Picker Relay</title>
  </head>
  <body></body>
</html>`
}

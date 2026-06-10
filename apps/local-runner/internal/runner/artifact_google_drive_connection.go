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
	"sort"
	"strings"
	"time"
)

const (
	googleDriveArtifactConnectPath         = "/artifact-storage/google-drive/connect"
	googleDriveAccountConnectCompletePath  = "/artifact-storage/google-drive/account-complete"
	googleDriveArtifactOAuthCallbackPath   = "/artifact-storage/google-drive/oauth/callback"
	googleDriveArtifactPickerPath          = "/artifact-storage/google-drive/picker"
	googleDriveArtifactPickerRelayPath     = "/artifact-storage/google-drive/picker-relay"
	googleDriveArtifactSessionSecretPrefix = "artifact-storage:google-drive:session"
	googleDriveSessionFlowAccount          = "account"
	googleDriveSessionFlowArtifactBinding  = "artifact_binding"
)

type artifactStorageGoogleDriveState struct {
	Sessions           map[string]artifactStorageGoogleDriveSessionRecord    `json:"sessions"`
	Connections        map[string]artifactStorageGoogleDriveConnectionRecord `json:"connections"`
	Accounts           map[string]artifactStorageGoogleDriveAccountRecord    `json:"accounts"`
	SuppressedAccounts map[string]bool                                       `json:"suppressedAccounts,omitempty"`
}

type artifactStorageGoogleDriveAccountRecord struct {
	AccountID       string   `json:"accountId"`
	AccountEmail    string   `json:"accountEmail,omitempty"`
	AccountSubject  string   `json:"accountSubject,omitempty"`
	OAuthClientID   string   `json:"oauthClientId,omitempty"`
	GrantedScopes   []string `json:"grantedScopes,omitempty"`
	Status          string   `json:"status"`
	LastError       string   `json:"lastError,omitempty"`
	LastValidatedAt string   `json:"lastValidatedAt,omitempty"`
	ConnectedAt     string   `json:"connectedAt,omitempty"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
}

type artifactStorageGoogleDriveSessionRecord struct {
	SessionID       string   `json:"sessionId"`
	ProjectID       string   `json:"projectId"`
	FlowKind        string   `json:"flowKind,omitempty"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"createdAt"`
	ExpiresAt       string   `json:"expiresAt"`
	ConnectedAt     string   `json:"connectedAt,omitempty"`
	AccountID       string   `json:"accountId,omitempty"`
	AccountEmail    string   `json:"accountEmail,omitempty"`
	RequestedScopes []string `json:"requestedScopes,omitempty"`
	FolderID        string   `json:"folderId,omitempty"`
	FolderName      string   `json:"folderName,omitempty"`
	LastError       string   `json:"lastError,omitempty"`
}

type artifactStorageGoogleDriveConnectionRecord struct {
	ProjectID       string `json:"projectId"`
	Status          string `json:"status"`
	FolderID        string `json:"folderId,omitempty"`
	FolderName      string `json:"folderName,omitempty"`
	AccountID       string `json:"accountId,omitempty"`
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
	Scope        string `json:"scope,omitempty"`
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

	state := artifactStorageGoogleDriveSessionRecord{
		SessionID:       newArtifactStorageGoogleDriveSessionID(),
		ProjectID:       projectID,
		FlowKind:        googleDriveSessionFlowArtifactBinding,
		Status:          string(ArtifactStorageGoogleDriveSessionPending),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt:       time.Now().UTC().Add(time.Duration(artifactStorageGoogleDriveDefaultSessionTTL) * time.Second).Format(time.RFC3339Nano),
		RequestedScopes: googleDriveArtifactRequestedScopes(),
	}

	selectedAccountID := strings.TrimSpace(request.AccountID)
	if selectedAccountID != "" {
		accountStatus, err := r.googleDriveAccountStatusByID(selectedAccountID)
		if err != nil {
			return ArtifactStorageGoogleDriveSession{}, err
		}
		if !accountStatus.AccountReady {
			if accountStatus.ReconnectRequired {
				return ArtifactStorageGoogleDriveSession{}, errors.New("selected Google Drive account needs reconnect before folder binding")
			}
			return ArtifactStorageGoogleDriveSession{}, errors.New("selected Google Drive account is not ready for folder binding")
		}
		if !accountStatus.McpWriteReady {
			return ArtifactStorageGoogleDriveSession{}, errors.New("selected Google Drive account is missing the drive.file scope required for artifact sync")
		}

		state.Status = string(ArtifactStorageGoogleDriveSessionAwaitingFolderPicker)
		state.AccountID = accountStatus.AccountID
		state.AccountEmail = accountStatus.AccountEmail
		if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
			if current.Sessions == nil {
				current.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
			}
			current.Sessions[state.SessionID] = state
		}); err != nil {
			return ArtifactStorageGoogleDriveSession{}, err
		}
		return mapArtifactStorageGoogleDriveSession(
			state,
			fmt.Sprintf("%s%s?sessionId=%s", baseURL, googleDriveArtifactPickerPath, url.QueryEscape(state.SessionID)),
		), nil
	}

	if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		if current.Sessions == nil {
			current.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
		}
		current.Sessions[state.SessionID] = state
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	connectToken := newArtifactStorageGoogleDriveToken()
	if err := r.saveArtifactStorageGoogleDriveSessionToken(state.SessionID, "connect", connectToken); err != nil {
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

func (r *Runner) CreateGoogleDriveAccountConnectSession(request GoogleDriveAccountConnectRequest) (ArtifactStorageGoogleDriveSession, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if baseURL == "" {
		return ArtifactStorageGoogleDriveSession{}, errors.New("baseUrl is required")
	}
	if _, err := r.resolveGoogleDriveArtifactRuntimeConfig(); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	connectToken := newArtifactStorageGoogleDriveToken()
	state := artifactStorageGoogleDriveSessionRecord{
		SessionID:       newArtifactStorageGoogleDriveSessionID(),
		FlowKind:        googleDriveSessionFlowAccount,
		Status:          string(ArtifactStorageGoogleDriveSessionPending),
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExpiresAt:       time.Now().UTC().Add(time.Duration(artifactStorageGoogleDriveDefaultSessionTTL) * time.Second).Format(time.RFC3339Nano),
		AccountID:       normalizeGoogleDriveStoredAccountID(strings.TrimSpace(request.AccountID)),
		RequestedScopes: googleDriveAccountRequestedScopes(),
	}
	if strings.TrimSpace(state.AccountID) != "" {
		if accountStatus, err := r.googleDriveAccountStatusByID(state.AccountID); err == nil {
			state.AccountEmail = accountStatus.AccountEmail
		}
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
	if err := validateGoogleDriveArtifactOAuthClientConfig(config); err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("client_id", config.ClientID)
	query.Set("redirect_uri", config.RedirectURI)
	query.Set("response_type", "code")
	requestedScopes := session.RequestedScopes
	if len(requestedScopes) == 0 {
		requestedScopes = googleDriveArtifactRequestedScopes()
	}
	query.Set("scope", strings.Join(requestedScopes, " "))
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

	accountIdentity := parseGoogleDriveIDTokenIdentity(tokenResponse.IDToken)
	accountEmail := strings.TrimSpace(accountIdentity.Email)
	if accountEmail == "" {
		accountEmail = session.AccountEmail
	}
	runtimeConfig, runtimeErr := r.resolveGoogleDriveArtifactRuntimeConfig()
	if runtimeErr != nil {
		return ArtifactStorageGoogleDriveSession{}, runtimeErr
	}
	accountID := buildGoogleDriveAccountID(accountIdentity.Subject, accountEmail, runtimeConfig.ClientID)
	if strings.TrimSpace(accountID) == "" {
		accountID = firstNonEmptyGoogleDriveValue(strings.TrimSpace(session.AccountID), session.ProjectID, accountEmail)
	}
	grantedScopes := normalizeGoogleDriveScopes(parseGoogleDriveScopeList(tokenResponse.Scope))
	if len(grantedScopes) == 0 {
		grantedScopes = normalizeGoogleDriveScopes(session.RequestedScopes)
	}
	refreshToken := strings.TrimSpace(tokenResponse.RefreshToken)
	if refreshToken == "" {
		previous, previousErr := r.loadGoogleDriveCredentialByAccount(accountID)
		if previousErr != nil {
			_ = r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
				record.Status = string(ArtifactStorageGoogleDriveSessionFailed)
				record.LastError = "google drive oauth response did not include a refresh token"
			})
			return ArtifactStorageGoogleDriveSession{}, errors.New("google drive oauth response did not include a refresh token")
		}
		refreshToken = previous.RefreshToken
	}
	if err := r.saveGoogleDriveCredentialByAccount(accountID, googleDriveCredential{
		RefreshToken: refreshToken,
		AccountEmail: accountEmail,
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}
	if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		if current.Accounts == nil {
			current.Accounts = map[string]artifactStorageGoogleDriveAccountRecord{}
		}
		delete(current.SuppressedAccounts, accountID)
		record := current.Accounts[accountID]
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if strings.TrimSpace(record.ConnectedAt) == "" {
			record.ConnectedAt = now
		}
		record.AccountID = accountID
		record.AccountEmail = accountEmail
		record.AccountSubject = strings.TrimSpace(accountIdentity.Subject)
		record.OAuthClientID = strings.TrimSpace(runtimeConfig.ClientID)
		record.GrantedScopes = grantedScopes
		record.Status = "connected"
		record.LastError = ""
		record.LastValidatedAt = now
		record.UpdatedAt = now
		current.Accounts[accountID] = record
	}); err != nil {
		return ArtifactStorageGoogleDriveSession{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if session.FlowKind == googleDriveSessionFlowAccount || strings.TrimSpace(session.ProjectID) == "" {
		if err := r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
			record.Status = string(ArtifactStorageGoogleDriveSessionConnected)
			record.AccountID = accountID
			record.AccountEmail = accountEmail
			record.ConnectedAt = now
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

	if err := r.updateArtifactStorageGoogleDriveSession(sessionID, func(record *artifactStorageGoogleDriveSessionRecord) {
		record.Status = string(ArtifactStorageGoogleDriveSessionAwaitingFolderPicker)
		record.AccountID = accountID
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
	creds, err := r.loadGoogleDriveCredentialBySession(session.SessionID)
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
	if strings.TrimSpace(session.ProjectID) == "" {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("google drive folder binding session is missing projectId")
	}

	folderID := strings.TrimSpace(request.FolderID)
	if folderID == "" {
		return ArtifactStorageGoogleDriveConnectionStatus{}, errors.New("folderId is required")
	}
	creds, err := r.loadGoogleDriveCredentialBySession(session.SessionID)
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
	accountID := firstNonEmptyGoogleDriveValue(session.AccountID, normalizeGoogleDriveAccountID(accountEmail))
	if strings.TrimSpace(accountID) == "" {
		accountID = normalizeGoogleDriveAccountID(session.ProjectID)
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
			AccountID:       accountID,
			AccountEmail:    accountEmail,
			LastError:       "",
			LastValidatedAt: now,
			ConnectedAt:     now,
			UpdatedAt:       now,
		}
		if current.Accounts == nil {
			current.Accounts = map[string]artifactStorageGoogleDriveAccountRecord{}
		}
		if strings.TrimSpace(accountID) != "" {
			delete(current.SuppressedAccounts, accountID)
			accountRecord := current.Accounts[accountID]
			accountRecord.AccountID = accountID
			accountRecord.AccountEmail = accountEmail
			accountRecord.Status = "connected"
			accountRecord.LastError = ""
			accountRecord.LastValidatedAt = now
			if strings.TrimSpace(accountRecord.ConnectedAt) == "" {
				accountRecord.ConnectedAt = now
			}
			accountRecord.UpdatedAt = now
			current.Accounts[accountID] = accountRecord
		}
		if current.Sessions == nil {
			current.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
		}
		record := current.Sessions[sessionID]
		record.Status = string(ArtifactStorageGoogleDriveSessionConnected)
		record.FolderID = folderInfo.ID
		record.FolderName = folderInfo.Name
		record.AccountID = accountID
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
		accountID := normalizeGoogleDriveStoredAccountID(firstNonEmptyGoogleDriveValue(connection.AccountID, connection.AccountEmail))
		if accountID != "" && !strings.EqualFold(strings.TrimSpace(connection.Status), "reconnect_required") && !strings.EqualFold(strings.TrimSpace(connection.Status), "failed") {
			accountStatus, statusErr := r.googleDriveAccountStatusByID(accountID)
			switch {
			case statusErr != nil:
				connection.Status = "reconnect_required"
				if strings.TrimSpace(connection.LastError) == "" {
					connection.LastError = statusErr.Error()
				}
			case accountStatus.ReconnectRequired:
				connection.Status = "reconnect_required"
				connection.LastError = firstNonEmptyGoogleDriveValue(accountStatus.LastError, connection.LastError)
			case !accountStatus.AccountReady:
				connection.Status = "failed"
				connection.LastError = firstNonEmptyGoogleDriveValue(accountStatus.LastError, connection.LastError)
			case !accountStatus.McpWriteReady:
				connection.Status = "reconnect_required"
				connection.LastError = "Connected Google Drive account is missing the drive.file scope required for artifact sync. Reconnect the account and grant artifact access."
			}
		}
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

func (r *Runner) ListGoogleDriveAccounts() ([]GoogleDriveAccountStatus, error) {
	return r.resolveGoogleDriveAccountStatuses()
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
			legacyRaw, legacyErr := os.ReadFile(r.legacyArtifactStorageGoogleDriveStatePath())
			if legacyErr == nil {
				raw = legacyRaw
			} else if !errors.Is(legacyErr, os.ErrNotExist) {
				return artifactStorageGoogleDriveState{}, legacyErr
			} else {
				return artifactStorageGoogleDriveState{
					Sessions:           map[string]artifactStorageGoogleDriveSessionRecord{},
					Connections:        map[string]artifactStorageGoogleDriveConnectionRecord{},
					SuppressedAccounts: map[string]bool{},
				}, nil
			}
		} else {
			return artifactStorageGoogleDriveState{}, err
		}
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
	if state.Accounts == nil {
		state.Accounts = map[string]artifactStorageGoogleDriveAccountRecord{}
	}
	if state.SuppressedAccounts == nil {
		state.SuppressedAccounts = map[string]bool{}
	}
	normalizeGoogleDriveArtifactState(&state)
	return state, nil
}

func (r *Runner) resolveGoogleDriveAccountStatuses() ([]GoogleDriveAccountStatus, error) {
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return nil, err
	}

	projectCounts := map[string]int{}
	for _, connection := range state.Connections {
		accountID := normalizeGoogleDriveAccountID(connection.AccountID)
		if accountID == "" {
			accountID = normalizeGoogleDriveAccountID(connection.AccountEmail)
		}
		if accountID == "" {
			accountID = normalizeGoogleDriveAccountID(connection.ProjectID)
		}
		if accountID != "" {
			projectCounts[accountID]++
		}
	}

	accounts := make([]GoogleDriveAccountStatus, 0, len(state.Accounts))
	for accountID, record := range state.Accounts {
		normalizedAccountID := normalizeGoogleDriveStoredAccountID(firstNonEmptyGoogleDriveValue(accountID, record.AccountID, record.AccountEmail))
		if normalizedAccountID == "" {
			continue
		}
		record.AccountID = normalizedAccountID
		accounts = append(accounts, r.googleDriveAccountStatusFromRecord(record, projectCounts[normalizedAccountID]))
	}

	sort.Slice(accounts, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(accounts[i].AccountEmail))
		right := strings.ToLower(strings.TrimSpace(accounts[j].AccountEmail))
		if left == right {
			return accounts[i].AccountID < accounts[j].AccountID
		}
		return left < right
	})

	return accounts, nil
}

func (r *Runner) googleDriveAccountStatusByID(accountID string) (GoogleDriveAccountStatus, error) {
	accounts, err := r.resolveGoogleDriveAccountStatuses()
	if err != nil {
		return GoogleDriveAccountStatus{}, err
	}
	account, ok := findGoogleDriveAccountStatus(accounts, accountID)
	if !ok {
		return GoogleDriveAccountStatus{}, fmt.Errorf("google drive account %q is not connected on this runner", strings.TrimSpace(accountID))
	}
	return account, nil
}

func (r *Runner) googleDriveAccountStatusFromRecord(record artifactStorageGoogleDriveAccountRecord, projectCount int) GoogleDriveAccountStatus {
	accountID := normalizeGoogleDriveStoredAccountID(firstNonEmptyGoogleDriveValue(record.AccountID, record.AccountEmail))
	grantedScopes := normalizeGoogleDriveScopes(record.GrantedScopes)
	status := GoogleDriveAccountStatus{
		AccountID:      accountID,
		AccountEmail:   strings.TrimSpace(record.AccountEmail),
		AccountSubject: strings.TrimSpace(record.AccountSubject),
		OAuthClientID:  strings.TrimSpace(record.OAuthClientID),
		GrantedScopes:  grantedScopes,
		Status:         firstNonEmptyGoogleDriveValue(record.Status, "connected"),
		ProjectCount:   projectCount,
		ConnectedAt:    record.ConnectedAt,
		UpdatedAt:      record.UpdatedAt,
		LastError:      record.LastError,
	}

	creds, err := r.loadGoogleDriveCredentialByAccount(accountID)
	switch {
	case err == nil:
		if _, refreshErr := r.refreshGoogleDriveAccessToken(creds.RefreshToken); refreshErr == nil {
			status.AccountReady = true
		} else {
			status.LastError = refreshErr.Error()
			if googleDriveReconnectRequired(refreshErr) {
				status.Status = "reconnect_required"
				status.ReconnectRequired = true
			} else {
				status.Status = "failed"
			}
		}
	case googleDriveCredentialNeedsAuth(err):
		status.Status = "needs_auth"
		status.LastError = err.Error()
	default:
		status.Status = "failed"
		status.LastError = err.Error()
	}

	status.McpReadReady = status.AccountReady && googleDriveHasScope(grantedScopes, googleDriveScopeDriveReadonly)
	status.McpWriteReady = status.AccountReady && googleDriveHasScope(grantedScopes, googleDriveScopeDriveFile)
	status.MissingScopes = googleDriveMissingScopes(grantedScopes, []string{googleDriveScopeDriveReadonly, googleDriveScopeDriveFile})
	return status
}

func googleDriveCredentialNeedsAuth(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "refresh token is not configured") ||
		strings.Contains(lower, "not connected on this runner") ||
		strings.Contains(lower, "secret not found")
}

func (r *Runner) DisconnectGoogleDriveAccount(accountID string) error {
	trimmedAccountID := normalizeGoogleDriveStoredAccountID(accountID)
	if trimmedAccountID == "" {
		return errors.New("accountId is required")
	}
	if err := r.ensureSecretStore().Delete(googleDriveAccountCredentialKey(trimmedAccountID)); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return err
	}
	if err := r.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if current.SuppressedAccounts == nil {
			current.SuppressedAccounts = map[string]bool{}
		}
		current.SuppressedAccounts[trimmedAccountID] = true
		delete(current.Accounts, trimmedAccountID)
		for projectID, connection := range current.Connections {
			if !strings.EqualFold(normalizeGoogleDriveStoredAccountID(connection.AccountID), trimmedAccountID) {
				continue
			}
			connection.Status = "reconnect_required"
			connection.LastError = "Google Drive account was disconnected on this runner."
			connection.LastValidatedAt = now
			connection.UpdatedAt = now
			current.Connections[projectID] = connection
		}
		for sessionID, session := range current.Sessions {
			if !strings.EqualFold(normalizeGoogleDriveStoredAccountID(session.AccountID), trimmedAccountID) {
				continue
			}
			session.Status = "reconnect_required"
			session.LastError = "Google Drive account was disconnected on this runner."
			current.Sessions[sessionID] = session
		}
	}); err != nil {
		return err
	}

	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err == nil && strings.EqualFold(strings.TrimSpace(configFile.MCP.AccountID), trimmedAccountID) {
		configFile.MCP.AccountID = ""
		configFile.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return r.persistGoogleDriveWorkspaceConfigFile(configFile)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r *Runner) saveArtifactStorageGoogleDriveState(apply func(*artifactStorageGoogleDriveState)) error {
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return err
	}
	normalizeGoogleDriveArtifactState(&state)
	apply(&state)
	normalizeGoogleDriveArtifactState(&state)
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
	return filepath.Join(r.workspace, ".flowpilot", "settings", "artifact-storage-google-drive.json")
}

func (r *Runner) legacyArtifactStorageGoogleDriveStatePath() string {
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
		AccountID:    record.AccountID,
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
		AccountID:       record.AccountID,
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
		if current.Accounts == nil {
			current.Accounts = map[string]artifactStorageGoogleDriveAccountRecord{}
		}
		for key, record := range current.Accounts {
			if strings.TrimSpace(record.AccountID) == "" {
				record.AccountID = key
			}
			if strings.TrimSpace(record.Status) == "" {
				record.Status = "connected"
			}
			current.Accounts[key] = record
		}
		trimmedProjectID := strings.TrimSpace(projectID)
		if trimmedProjectID != "" {
			record := current.Connections[trimmedProjectID]
			record.ProjectID = trimmedProjectID
			record.Status = "reconnect_required"
			if strings.TrimSpace(record.AccountID) == "" {
				record.AccountID = normalizeGoogleDriveAccountID(record.AccountEmail)
			}
			record.LastError = err.Error()
			record.LastValidatedAt = now
			record.UpdatedAt = now
			current.Connections[trimmedProjectID] = record
			if strings.TrimSpace(record.AccountID) != "" {
				accountRecord := current.Accounts[record.AccountID]
				accountRecord.AccountID = record.AccountID
				accountRecord.AccountEmail = firstNonEmptyGoogleDriveValue(accountRecord.AccountEmail, record.AccountEmail)
				accountRecord.Status = "reconnect_required"
				accountRecord.LastError = err.Error()
				accountRecord.LastValidatedAt = now
				accountRecord.UpdatedAt = now
				if strings.TrimSpace(accountRecord.ConnectedAt) == "" {
					accountRecord.ConnectedAt = now
				}
				current.Accounts[record.AccountID] = accountRecord
			}
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

func normalizeGoogleDriveAccountID(value string) string {
	return normalizeGoogleDriveStoredAccountID(value)
}

func normalizeGoogleDriveArtifactState(state *artifactStorageGoogleDriveState) {
	if state.Sessions == nil {
		state.Sessions = map[string]artifactStorageGoogleDriveSessionRecord{}
	}
	if state.Connections == nil {
		state.Connections = map[string]artifactStorageGoogleDriveConnectionRecord{}
	}
	if state.Accounts == nil {
		state.Accounts = map[string]artifactStorageGoogleDriveAccountRecord{}
	}
	if state.SuppressedAccounts == nil {
		state.SuppressedAccounts = map[string]bool{}
	}

	for projectID, record := range state.Connections {
		normalizedRecord := record
		accountID := firstNonEmptyGoogleDriveValue(
			normalizedRecord.AccountID,
			normalizeGoogleDriveAccountID(normalizedRecord.AccountEmail),
			normalizeGoogleDriveAccountID(projectID),
		)
		normalizedRecord.AccountID = accountID
		if strings.TrimSpace(normalizedRecord.ProjectID) == "" {
			normalizedRecord.ProjectID = projectID
		}
		state.Connections[projectID] = normalizedRecord
		if accountID == "" || state.SuppressedAccounts[accountID] {
			continue
		}
		accountRecord := state.Accounts[accountID]
		accountRecord.AccountID = accountID
		accountRecord.AccountEmail = firstNonEmptyGoogleDriveValue(accountRecord.AccountEmail, normalizedRecord.AccountEmail)
		accountRecord.Status = firstNonEmptyGoogleDriveValue(normalizedRecord.Status, accountRecord.Status)
		accountRecord.LastError = firstNonEmptyGoogleDriveValue(normalizedRecord.LastError, accountRecord.LastError)
		accountRecord.LastValidatedAt = firstNonEmptyGoogleDriveValue(normalizedRecord.LastValidatedAt, accountRecord.LastValidatedAt)
		accountRecord.ConnectedAt = firstNonEmptyGoogleDriveValue(normalizedRecord.ConnectedAt, accountRecord.ConnectedAt)
		accountRecord.UpdatedAt = firstNonEmptyGoogleDriveValue(normalizedRecord.UpdatedAt, accountRecord.UpdatedAt)
		accountRecord.GrantedScopes = normalizeGoogleDriveScopes(
			firstNonEmptyGoogleDriveScopes(accountRecord.GrantedScopes, googleDriveArtifactRequestedScopes()),
		)
		if strings.TrimSpace(accountRecord.Status) == "" {
			accountRecord.Status = "connected"
		}
		state.Accounts[accountID] = accountRecord
	}

	for sessionID, record := range state.Sessions {
		normalizedRecord := record
		accountID := firstNonEmptyGoogleDriveValue(
			normalizedRecord.AccountID,
			normalizeGoogleDriveAccountID(normalizedRecord.AccountEmail),
			normalizeGoogleDriveAccountID(normalizedRecord.ProjectID),
		)
		normalizedRecord.AccountID = accountID
		if strings.TrimSpace(normalizedRecord.SessionID) == "" {
			normalizedRecord.SessionID = sessionID
		}
		state.Sessions[sessionID] = normalizedRecord
		if accountID == "" || state.SuppressedAccounts[accountID] {
			continue
		}
		accountRecord := state.Accounts[accountID]
		accountRecord.AccountID = accountID
		accountRecord.AccountEmail = firstNonEmptyGoogleDriveValue(accountRecord.AccountEmail, normalizedRecord.AccountEmail)
		accountRecord.GrantedScopes = normalizeGoogleDriveScopes(
			firstNonEmptyGoogleDriveScopes(
				accountRecord.GrantedScopes,
				normalizedRecord.RequestedScopes,
				googleDriveArtifactRequestedScopes(),
			),
		)
		if strings.TrimSpace(accountRecord.Status) == "" {
			accountRecord.Status = "connected"
		}
		if strings.TrimSpace(accountRecord.ConnectedAt) == "" {
			accountRecord.ConnectedAt = normalizedRecord.ConnectedAt
		}
		state.Accounts[accountID] = accountRecord
	}
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

type googleDriveIDTokenIdentity struct {
	Email   string `json:"email"`
	Subject string `json:"sub"`
}

func parseGoogleDriveIDTokenIdentity(idToken string) googleDriveIDTokenIdentity {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return googleDriveIDTokenIdentity{}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleDriveIDTokenIdentity{}
	}
	var payload googleDriveIDTokenIdentity
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return googleDriveIDTokenIdentity{}
	}
	payload.Email = strings.TrimSpace(payload.Email)
	payload.Subject = strings.TrimSpace(payload.Subject)
	return payload
}

func shouldFallbackToPickedGoogleDriveFolder(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "google drive file lookup by id failed: 403") ||
		strings.Contains(message, "google drive file lookup by id failed: 404")
}

func buildGoogleDriveAccountID(subject, email, oauthClientID string) string {
	normalizedSubject := normalizeGoogleDriveStoredAccountID(subject)
	normalizedEmail := normalizeGoogleDriveStoredAccountID(email)
	normalizedClientID := normalizeGoogleDriveStoredAccountID(oauthClientID)
	switch {
	case normalizedSubject != "" && normalizedClientID != "":
		return normalizedSubject + "::" + normalizedClientID
	case normalizedSubject != "":
		return normalizedSubject
	case normalizedEmail != "" && normalizedClientID != "":
		return normalizedEmail + "::" + normalizedClientID
	default:
		return firstNonEmptyGoogleDriveValue(normalizedEmail, normalizedClientID)
	}
}

func normalizeGoogleDriveStoredAccountID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseGoogleDriveScopeList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Fields(strings.TrimSpace(raw))
}

func normalizeGoogleDriveScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func googleDriveHasScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(strings.TrimSpace(scope), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}

func googleDriveMissingScopes(scopes []string, expected []string) []string {
	missing := make([]string, 0, len(expected))
	for _, scope := range expected {
		if !googleDriveHasScope(scopes, scope) {
			missing = append(missing, scope)
		}
	}
	return missing
}

func firstNonEmptyGoogleDriveScopes(candidates ...[]string) []string {
	for _, candidate := range candidates {
		if len(candidate) > 0 {
			return candidate
		}
	}
	return nil
}

func googleDriveArtifactRequestedScopes() []string {
	return []string{
		googleDriveScopeDriveFile,
		googleDriveScopeOpenID,
		googleDriveScopeEmail,
	}
}

func googleDriveAccountRequestedScopes() []string {
	return []string{
		googleDriveScopeDriveFile,
		googleDriveScopeDriveReadonly,
		googleDriveScopeOpenID,
		googleDriveScopeEmail,
	}
}

func firstNonEmptyGoogleDriveValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func RenderGoogleDriveAccountConnectedHTML() string {
	return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>FlowPilot Google Drive Account Connected</title>
  </head>
  <body style="font-family:sans-serif;padding:24px;background:#f6f7f9;color:#111827;">
    <div style="max-width:560px;margin:32px auto;background:white;border-radius:16px;padding:24px;box-shadow:0 12px 32px rgba(0,0,0,.08);">
      <h1 style="margin-top:0;">Google Drive account connected</h1>
      <p>FlowPilot saved the Google account on this runner. You can close this tab and return to Google Drive setup.</p>
    </div>
    <script>
      if (window.opener) {
        window.opener.postMessage({ type: "flowpilot-google-drive-account-connected" }, "*");
      }
      window.setTimeout(() => window.close(), 350);
    </script>
  </body>
</html>`
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

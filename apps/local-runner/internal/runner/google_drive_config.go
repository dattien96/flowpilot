package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	googleDriveArtifactSyncClientSecretKey    = "google-drive:artifact-sync:client-secret"
	googleDriveArtifactSyncPickerAPIKeySecret = "google-drive:artifact-sync:picker-api-key"
	googleDriveDefaultRedirectURI             = "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback"
	googleDriveOAuthCredentialsFileName       = "gcp-oauth.keys.json"
	googleDriveMcpTokenFileName               = "tokens.json"
	googleDriveScopeOpenID                    = "openid"
	googleDriveScopeEmail                     = "email"
	googleDriveScopeDriveFile                 = "https://www.googleapis.com/auth/drive.file"
	googleDriveScopeDriveReadonly             = "https://www.googleapis.com/auth/drive.readonly"
)

type googleDriveWorkspaceConfigFile struct {
	Version      int                                `json:"version"`
	ArtifactSync googleDriveWorkspaceArtifactConfig `json:"artifactSync"`
	MCP          googleDriveWorkspaceMcpConfig      `json:"mcp"`
	UpdatedAt    string                             `json:"updatedAt,omitempty"`
}

type googleDriveWorkspaceArtifactConfig struct {
	ClientID    string `json:"clientId,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
}

type googleDriveWorkspaceMcpConfig struct {
	CredentialPath string `json:"credentialPath,omitempty"`
	TokenPath      string `json:"tokenPath,omitempty"`
	AccountID      string `json:"accountId,omitempty"`
}

type googleDriveArtifactEnvConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	PickerAPIKey string
}

type googleDriveArtifactRuntimeConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	PickerAPIKey string
	Source       string
}

type googleDriveProxyAccountSelection struct {
	AccountID    string
	AccountEmail string
	Accounts     []GoogleDriveAccountStatus
	Required     bool
}

type googleDriveOAuthClientJSON struct {
	Installed *googleDriveOAuthClientData `json:"installed"`
	Web       *googleDriveOAuthClientData `json:"web"`
}

type googleDriveOAuthClientData struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

type googleDriveOAuthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type googleDriveOAuthRequestError struct {
	Operation         string
	StatusCode        int
	ProviderError     string
	ProviderMessage   string
	RawMessage        string
	ReconnectRequired bool
}

func (e *googleDriveOAuthRequestError) Error() string {
	if e == nil {
		return ""
	}
	if e.ReconnectRequired {
		if strings.TrimSpace(e.ProviderError) != "" {
			return fmt.Sprintf(
				"google drive authorization expired or was revoked (%s); reconnect Google Drive and try again",
				strings.TrimSpace(e.ProviderError),
			)
		}
		return "google drive authorization expired or was revoked; reconnect Google Drive and try again"
	}

	detail := strings.TrimSpace(e.ProviderError)
	if message := strings.TrimSpace(e.ProviderMessage); message != "" {
		if detail != "" {
			detail += ": " + message
		} else {
			detail = message
		}
	}
	if detail == "" {
		detail = strings.TrimSpace(e.RawMessage)
	}
	if detail == "" {
		return fmt.Sprintf("%s failed: %d", e.Operation, e.StatusCode)
	}
	return fmt.Sprintf("%s failed: %d %s", e.Operation, e.StatusCode, detail)
}

func (r *Runner) googleDriveWorkspaceConfigPath() string {
	return filepath.Join(r.workspace, ".flowpilot", "settings", "google-drive-config.json")
}

func googleDriveMcpConfigDir() string {
	if configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); configHome != "" {
		return filepath.Join(configHome, "google-drive-mcp")
	}
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".config", "google-drive-mcp")
	}
	if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
		return filepath.Join(profile, ".config", "google-drive-mcp")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".config", "google-drive-mcp")
	}
	return filepath.Join(".", ".config", "google-drive-mcp")
}

func googleDriveMcpCredentialPath() string {
	return filepath.Join(googleDriveMcpConfigDir(), googleDriveOAuthCredentialsFileName)
}

func googleDriveMcpTokenPath() string {
	return filepath.Join(googleDriveMcpConfigDir(), googleDriveMcpTokenFileName)
}

func (r *Runner) LoadGoogleDriveWorkspaceConfig() (GoogleDriveWorkspaceConfigResponse, error) {
	return r.resolveGoogleDriveWorkspaceStatus()
}

func (r *Runner) SaveGoogleDriveWorkspaceConfig(input GoogleDriveWorkspaceConfigRequest) (GoogleDriveWorkspaceConfigResponse, error) {
	current, err := r.loadGoogleDriveWorkspaceConfigFile()
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	if !hasCurrent && strings.TrimSpace(input.ClientID) == "" && strings.TrimSpace(input.RedirectURI) == "" && strings.TrimSpace(input.ClientSecret) == "" && strings.TrimSpace(input.PickerAPIKey) == "" {
		return GoogleDriveWorkspaceConfigResponse{}, errors.New("at least one Google Drive config value is required")
	}
	previousSecretState, err := r.googleDriveSecretState()
	if err != nil {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	if hasCurrent && googleDriveClientIDChangeNeedsNewSecret(current, input, previousSecretState.hasClientSecret) {
		return GoogleDriveWorkspaceConfigResponse{}, errors.New("clientSecret is required when clientId changes so Google OAuth does not reuse an older secret")
	}

	artifactConfig := current.ArtifactSync
	if strings.TrimSpace(input.ClientID) != "" {
		artifactConfig.ClientID = strings.TrimSpace(input.ClientID)
	}
	if strings.TrimSpace(input.RedirectURI) != "" {
		artifactConfig.RedirectURI = normalizeGoogleDriveRedirectURI(input.RedirectURI)
	}
	if artifactConfig.RedirectURI == "" {
		artifactConfig.RedirectURI = googleDriveDefaultRedirectURI
	}

	mcpConfig := current.MCP
	if strings.TrimSpace(mcpConfig.CredentialPath) == "" {
		mcpConfig.CredentialPath = googleDriveMcpCredentialPath()
	}
	if strings.TrimSpace(mcpConfig.TokenPath) == "" {
		mcpConfig.TokenPath = googleDriveMcpTokenPath()
	}
	if strings.TrimSpace(input.MCPAccountID) != "" {
		mcpConfig.AccountID = strings.TrimSpace(input.MCPAccountID)
	}

	nextConfig := googleDriveWorkspaceConfigFile{
		Version:      1,
		ArtifactSync: artifactConfig,
		MCP:          mcpConfig,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := r.saveGoogleDriveSecretIfProvided(googleDriveArtifactSyncClientSecretKey, input.ClientSecret); err != nil {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	if err := r.saveGoogleDriveSecretIfProvided(googleDriveArtifactSyncPickerAPIKeySecret, input.PickerAPIKey); err != nil {
		r.restoreGoogleDriveSecretState(previousSecretState)
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	if err := r.persistGoogleDriveWorkspaceConfigFile(nextConfig); err != nil {
		r.restoreGoogleDriveSecretState(previousSecretState)
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	return r.resolveGoogleDriveWorkspaceStatus()
}

func (r *Runner) ResetGoogleDriveWorkspaceConfig() error {
	err := os.Remove(r.googleDriveWorkspaceConfigPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if deleteErr := r.ensureSecretStore().Delete(googleDriveArtifactSyncClientSecretKey); deleteErr != nil {
		return deleteErr
	}
	if deleteErr := r.ensureSecretStore().Delete(googleDriveArtifactSyncPickerAPIKeySecret); deleteErr != nil {
		return deleteErr
	}

	return nil
}

func (r *Runner) ValidateGoogleDriveWorkspaceConfig(input GoogleDriveWorkspaceConfigRequest) (GoogleDriveValidationResult, error) {
	status, err := r.buildGoogleDriveValidationStatus(input)
	if err != nil {
		return GoogleDriveValidationResult{}, err
	}

	checks := make([]GoogleDriveValidationCheck, 0, 10)
	artifact := status.ArtifactSync
	checks = append(checks, googleDriveCheck("artifact_client_id_present", artifact.ClientID != "", "Artifact sync client ID is present.", "Artifact sync client ID is required."))
	checks = append(checks, googleDriveCheck("artifact_client_secret_present", artifact.HasClientSecret, "Artifact sync client secret is present.", "Artifact sync client secret is required."))
	checks = append(checks, googleDriveCheck("artifact_redirect_uri_present", artifact.RedirectURI != "", "Artifact sync redirect URI is present.", "Artifact sync redirect URI is required."))
	checks = append(checks, googleDriveCheck("artifact_redirect_uri_matches_runner_callback", strings.EqualFold(strings.TrimSpace(artifact.RedirectURI), googleDriveDefaultRedirectURI), "Artifact sync redirect URI matches the runner callback.", "Artifact sync redirect URI must match the runner callback URL."))
	checks = append(checks, googleDriveCheck("picker_api_key_present", artifact.HasPickerAPIKey, "Google Picker API key is present.", "Google Picker API key is required."))

	mcp := status.MCP
	if mcp.ProxyMcpEnabled {
		checks = append(checks, googleDriveCheck("mcp_account_selected", !mcp.AccountSelectionRequired && strings.TrimSpace(mcp.AccountID) != "", "Active Google account is selected for the FlowPilot proxy MCP.", "Select an active Google account for the FlowPilot proxy MCP."))
		checks = append(checks, googleDriveCheck("mcp_account_ready", mcp.TokenRefreshValid, "Selected Google account auth is ready for the proxy MCP.", "Reconnect the selected Google account before configuring the proxy MCP."))
		checks = append(checks, googleDriveCheck("mcp_backend_available", mcp.BackendPackageAvailable, "Google Drive MCP launcher is available.", "Install FlowPilot or Go so the proxy launcher can start the Google Drive MCP package."))
	} else {
		checks = append(checks, googleDriveCheck("mcp_credential_path_resolved", strings.TrimSpace(mcp.CredentialPath) != "", "MCP credential path is resolved.", "MCP credential path is required."))
		checks = append(checks, googleDriveCheck("mcp_credential_file_exists", mcp.CredentialFileExists, "MCP credential JSON exists.", "Upload the Google Drive MCP OAuth JSON file."))
		checks = append(checks, googleDriveCheck("mcp_credential_json_valid", mcp.CredentialFileValid, "MCP credential JSON is valid.", "The uploaded OAuth JSON is invalid."))
		checks = append(checks, googleDriveCheck("mcp_token_path_resolved", strings.TrimSpace(mcp.TokenPath) != "", "MCP token path is resolved.", "MCP token path is required."))
		if !mcp.TokenFileExists {
			checks = append(checks, googleDriveSkippedCheck("mcp_token_file_exists", "MCP token file will be created when you complete the MCP auth flow."))
			checks = append(checks, googleDriveSkippedCheck("mcp_token_refresh_valid", "MCP OAuth token validation will run after the MCP auth flow creates the token file."))
		} else {
			checks = append(checks, googleDrivePassedCheck("mcp_token_file_exists", "MCP token file exists."))
			switch mcp.Status {
			case "configured":
				checks = append(checks, googleDrivePassedCheck("mcp_token_refresh_valid", "MCP OAuth token is usable."))
			case "warning":
				checks = append(checks, googleDriveSkippedCheck("mcp_token_refresh_valid", "The stored MCP OAuth token exists, but FlowPilot could not fully validate token health."))
			default:
				checks = append(checks, googleDriveFailedCheck("mcp_token_refresh_valid", "The stored MCP OAuth token needs reconnect or re-authentication."))
			}
		}
		checks = append(checks, googleDriveCheck("mcp_backend_available", mcp.BackendPackageAvailable, "Google Drive MCP launcher is available.", "Install Node.js so npx can launch the Google Drive MCP package."))
	}

	valid := true
	for _, item := range checks {
		if item.Status == "failed" {
			valid = false
			break
		}
	}

	return GoogleDriveValidationResult{
		Valid:  valid,
		Checks: checks,
		Status: status,
	}, nil
}

func (r *Runner) resolveGoogleDriveWorkspaceStatus() (GoogleDriveWorkspaceConfigResponse, error) {
	status := GoogleDriveWorkspaceConfigResponse{
		RunnerReachable: true,
	}

	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		status.LastError = err.Error()
		return status, nil
	}

	// Resolve provider configs
	providerConfigs, _ := r.resolveGoogleDriveMcpProviderStatuses()
	status.ProviderConfigs = providerConfigs
	if accounts, accountsErr := r.resolveGoogleDriveAccountStatuses(); accountsErr == nil {
		status.Accounts = accounts
	} else if status.LastError == "" {
		status.LastError = accountsErr.Error()
	}

	if err == nil {
		artifact, artifactErr := r.resolveGoogleDriveArtifactStatusFromSavedConfig(configFile)
		if artifactErr != nil {
			status.LastError = artifactErr.Error()
		}
		status.ArtifactSync = artifact
		status.MCP = r.resolveGoogleDriveMcpStatus(configFile)
		status.UpdatedAt = configFile.UpdatedAt
		return status, nil
	}

	artifact, artifactErr := r.resolveGoogleDriveArtifactStatusFromEnv()
	if artifactErr != nil {
		status.LastError = artifactErr.Error()
	}
	status.ArtifactSync = artifact
	status.MCP = r.resolveGoogleDriveMcpStatus(googleDriveWorkspaceConfigFile{})
	return status, nil
}

func (r *Runner) resolveGoogleDriveArtifactRuntimeConfig() (googleDriveArtifactRuntimeConfig, error) {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err == nil {
		secretState, stateErr := r.googleDriveSecretState()
		if stateErr != nil {
			return googleDriveArtifactRuntimeConfig{}, stateErr
		}
		return r.buildGoogleDriveArtifactRuntimeConfigFromSaved(configFile, secretState)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return googleDriveArtifactRuntimeConfig{}, err
	}
	envConfig, envErr := readGoogleDriveArtifactEnvConfig()
	if envErr != nil {
		return googleDriveArtifactRuntimeConfig{}, envErr
	}
	return googleDriveArtifactRuntimeConfig{
		ClientID:     envConfig.ClientID,
		ClientSecret: envConfig.ClientSecret,
		RedirectURI:  envConfig.RedirectURI,
		PickerAPIKey: envConfig.PickerAPIKey,
		Source:       "env",
	}, nil
}

func (r *Runner) resolveGoogleDriveProxyOAuthConfig() (googleDriveArtifactConfig, error) {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err == nil {
		clientID := strings.TrimSpace(configFile.ArtifactSync.ClientID)
		clientSecret, secretErr := r.ensureSecretStore().Get(googleDriveArtifactSyncClientSecretKey)
		if secretErr != nil {
			return googleDriveArtifactConfig{}, secretErr
		}
		clientSecret = strings.TrimSpace(clientSecret)
		if clientID == "" || clientSecret == "" {
			return googleDriveArtifactConfig{}, errors.New("FlowPilot proxy Google Drive auth is incomplete")
		}
		return googleDriveArtifactConfig{clientID: clientID, clientSecret: clientSecret}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return googleDriveArtifactConfig{}, err
	}
	return readGoogleDriveArtifactConfig()
}

func (r *Runner) exchangeGoogleDriveOAuthCode(code string) (googleDriveOAuthTokenResponse, error) {
	config, err := r.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		return googleDriveOAuthTokenResponse{}, err
	}

	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", strings.TrimSpace(code))
	form.Set("redirect_uri", config.RedirectURI)

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		"https://oauth2.googleapis.com/token",
		map[string]string{
			"content-type": "application/x-www-form-urlencoded",
		},
		[]byte(form.Encode()),
	)
	if requestErr != nil {
		return googleDriveOAuthTokenResponse{}, requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return googleDriveOAuthTokenResponse{}, googleDriveOAuthFailure(
			"google drive oauth code exchange",
			statusCode,
			responseBody,
		)
	}

	var response googleDriveOAuthTokenResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return googleDriveOAuthTokenResponse{}, err
	}
	if strings.TrimSpace(response.AccessToken) == "" && strings.TrimSpace(response.RefreshToken) == "" {
		return googleDriveOAuthTokenResponse{}, errors.New("google drive oauth code exchange returned an empty token response")
	}

	return response, nil
}

func (r *Runner) validateGoogleDriveArtifactOAuthClient() error {
	config, err := r.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		return err
	}
	return validateGoogleDriveArtifactOAuthClientConfig(config)
}

func validateGoogleDriveArtifactOAuthClientConfig(config googleDriveArtifactRuntimeConfig) error {
	form := url.Values{}
	form.Set("client_id", config.ClientID)
	form.Set("client_secret", config.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", "flowpilot-oauth-client-validation-probe")
	form.Set("redirect_uri", config.RedirectURI)

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		"https://oauth2.googleapis.com/token",
		map[string]string{
			"content-type": "application/x-www-form-urlencoded",
		},
		[]byte(form.Encode()),
	)
	if requestErr != nil {
		return requestErr
	}
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return nil
	}

	err := googleDriveOAuthFailure("google drive oauth client validation", statusCode, responseBody)
	var oauthErr *googleDriveOAuthRequestError
	if errors.As(err, &oauthErr) {
		switch strings.ToLower(strings.TrimSpace(oauthErr.ProviderError)) {
		case "invalid_grant":
			return nil
		case "invalid_client":
			return fmt.Errorf(
				"google drive OAuth client credentials are invalid for clientId %q; save the matching Web OAuth client secret and try again",
				strings.TrimSpace(config.ClientID),
			)
		}
	}

	return err
}

func (r *Runner) refreshGoogleDriveAccessToken(refreshToken string) (string, error) {
	config, err := r.resolveGoogleDriveProxyOAuthConfig()
	if err != nil {
		return "", err
	}

	return googleDriveRefreshAccessToken(config.clientID, config.clientSecret, refreshToken)
}

func googleDriveRefreshAccessToken(clientID, clientSecret, refreshToken string) (string, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("client_secret", strings.TrimSpace(clientSecret))
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))

	statusCode, responseBody, requestErr := httpRequestFn(
		context.Background(),
		http.MethodPost,
		"https://oauth2.googleapis.com/token",
		map[string]string{
			"content-type": "application/x-www-form-urlencoded",
		},
		[]byte(form.Encode()),
	)
	if requestErr != nil {
		return "", requestErr
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return "", googleDriveOAuthFailure("google drive token refresh", statusCode, responseBody)
	}

	var response googleDriveTokenResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", errors.New("google drive token refresh returned an empty access token")
	}

	return strings.TrimSpace(response.AccessToken), nil
}

func (r *Runner) googleDriveMcpRuntimeConfig() (googleDriveMcpRuntimeConfig, error) {
	configFile, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return googleDriveMcpRuntimeConfig{}, err
	}

	credentialPath := googleDriveMcpCredentialPath()
	tokenPath := googleDriveMcpTokenPath()
	if err == nil {
		if strings.TrimSpace(configFile.MCP.CredentialPath) != "" {
			credentialPath = configFile.MCP.CredentialPath
		}
		if strings.TrimSpace(configFile.MCP.TokenPath) != "" {
			tokenPath = configFile.MCP.TokenPath
		}
	}

	credentialExists := fileExists(credentialPath)
	tokenExists := fileExists(tokenPath)
	backendAvailable := googleDriveMcpBackendAvailable()
	credentialValid := false
	tokenRefreshValid := false
	if credentialExists {
		credentialValid = validateGoogleDriveOAuthCredentialJSONFile(credentialPath) == nil
	}

	status := "not_started"
	if !credentialExists {
		status = "needs_input"
	} else if !credentialValid {
		status = "failed"
	} else if !tokenExists {
		status = "needs_auth"
	} else {
		tokenRefreshValid, err = validateGoogleDriveMcpStoredRefreshToken(credentialPath, tokenPath)
		switch {
		case err == nil && !backendAvailable:
			status = "warning"
		case err == nil:
			status = "configured"
		case googleDriveReconnectRequired(err):
			status = "reconnect_required"
		case errors.Is(err, errGoogleDriveMcpTokenNeedsAuth):
			status = "needs_auth"
		default:
			status = "warning"
		}
	}

	return googleDriveMcpRuntimeConfig{
		CredentialPath:          credentialPath,
		TokenPath:               tokenPath,
		CredentialExists:        credentialExists,
		CredentialValid:         credentialValid,
		TokenExists:             tokenExists,
		TokenRefreshValid:       tokenRefreshValid,
		BackendPackageAvailable: backendAvailable,
		Status:                  status,
	}, nil
}

func (r *Runner) resolveGoogleDriveMcpStatus(configFile googleDriveWorkspaceConfigFile) GoogleDriveMcpStatus {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		return r.resolveGoogleDriveProxyMcpStatus(configFile)
	}

	mcpConfig, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		return GoogleDriveMcpStatus{
			Status:          "failed",
			Configured:      false,
			ProxyMcpEnabled: flowpilotGoogleDriveProxyMcpEnabled(),
			MissingFields:   []string{"mcp_runtime"},
		}
	}
	status := GoogleDriveMcpStatus{
		Status:                   mcpConfig.Status,
		Configured:               mcpConfig.Status == "configured",
		ProxyMcpEnabled:          flowpilotGoogleDriveProxyMcpEnabled(),
		CredentialPath:           mcpConfig.CredentialPath,
		TokenPath:                mcpConfig.TokenPath,
		CredentialFileExists:     mcpConfig.CredentialExists,
		CredentialFileValid:      mcpConfig.CredentialValid,
		TokenFileExists:          mcpConfig.TokenExists,
		NeedsAuth:                mcpConfig.Status == "needs_auth" || mcpConfig.Status == "reconnect_required",
		BackendPackageAvailable:  mcpConfig.BackendPackageAvailable,
		AccountID:                mcpConfig.AccountID,
		AccountEmail:             mcpConfig.AccountEmail,
		AccountSelectionRequired: mcpConfig.AccountSelectionRequired,
	}

	if strings.TrimSpace(configFile.MCP.CredentialPath) != "" || strings.TrimSpace(configFile.MCP.TokenPath) != "" {
		status.CredentialPath = configFile.MCP.CredentialPath
		status.TokenPath = configFile.MCP.TokenPath
	}

	status.MissingFields = googleDriveMcpMissingFields(status)
	return status
}

func (r *Runner) resolveGoogleDriveProxyMcpStatus(configFile googleDriveWorkspaceConfigFile) GoogleDriveMcpStatus {
	artifactConfig, err := r.resolveGoogleDriveProxyOAuthConfig()
	if err != nil {
		return GoogleDriveMcpStatus{
			Status:                   "failed",
			Configured:               false,
			ProxyMcpEnabled:          true,
			AccountSelectionRequired: true,
			MissingFields:            []string{"artifactSync", "accountId"},
		}
	}

	selection, err := r.resolveGoogleDriveProxyAccountSelection(configFile)
	if err != nil {
		return GoogleDriveMcpStatus{
			Status:                   "failed",
			Configured:               false,
			ProxyMcpEnabled:          true,
			AccountSelectionRequired: true,
			MissingFields:            []string{"accountId"},
			AccountID:                strings.TrimSpace(configFile.MCP.AccountID),
		}
	}

	status := GoogleDriveMcpStatus{
		Status:                   "needs_input",
		Configured:               false,
		ProxyMcpEnabled:          true,
		CredentialPath:           googleDriveMcpCredentialPath(),
		TokenPath:                googleDriveMcpTokenPath(),
		CredentialFileExists:     fileExists(googleDriveMcpCredentialPath()),
		CredentialFileValid:      fileExists(googleDriveMcpCredentialPath()) && validateGoogleDriveOAuthCredentialJSONFile(googleDriveMcpCredentialPath()) == nil,
		TokenFileExists:          fileExists(googleDriveMcpTokenPath()),
		NeedsAuth:                true,
		BackendPackageAvailable:  googleDriveProxyMcpBackendAvailable(r.workspace),
		AccountID:                selection.AccountID,
		AccountEmail:             selection.AccountEmail,
		AccountSelectionRequired: selection.Required,
	}

	if strings.TrimSpace(configFile.MCP.CredentialPath) != "" || strings.TrimSpace(configFile.MCP.TokenPath) != "" {
		status.CredentialPath = configFile.MCP.CredentialPath
		status.TokenPath = configFile.MCP.TokenPath
		status.CredentialFileExists = fileExists(status.CredentialPath)
		status.CredentialFileValid = status.CredentialFileExists && validateGoogleDriveOAuthCredentialJSONFile(status.CredentialPath) == nil
		status.TokenFileExists = fileExists(status.TokenPath)
	}
	if strings.TrimSpace(configFile.MCP.AccountID) != "" {
		status.AccountID = strings.TrimSpace(configFile.MCP.AccountID)
		if account, ok := findGoogleDriveAccountStatus(selection.Accounts, status.AccountID); ok {
			status.AccountEmail = account.AccountEmail
		}
	}

	if status.AccountSelectionRequired {
		status.Status = "needs_input"
		status.MissingFields = googleDriveMcpMissingFields(status)
		return status
	}

	accountStatus, ok := findGoogleDriveAccountStatus(selection.Accounts, status.AccountID)
	if !ok {
		status.Status = "needs_auth"
		status.NeedsAuth = true
		status.MissingFields = googleDriveMcpMissingFields(status)
		return status
	}

	status.AccountEmail = accountStatus.AccountEmail
	status.GrantedScopes = normalizeGoogleDriveScopes(accountStatus.GrantedScopes)
	status.MissingScopes = googleDriveMissingScopes(status.GrantedScopes, []string{googleDriveScopeDriveReadonly})
	status.AccountReady = accountStatus.AccountReady
	status.McpReadReady = accountStatus.McpReadReady
	status.McpWriteReady = accountStatus.McpWriteReady
	status.ReconnectRequired = accountStatus.ReconnectRequired
	status.TokenRefreshValid = accountStatus.AccountReady
	status.ArtifactBindingPresent = r.googleDriveAccountHasArtifactBinding(status.AccountID)
	status.ArtifactReady = status.AccountReady && googleDriveHasScope(status.GrantedScopes, googleDriveScopeDriveFile) && status.ArtifactBindingPresent

	switch {
	case accountStatus.ReconnectRequired:
		status.Status = "reconnect_required"
		status.NeedsAuth = true
	case !accountStatus.AccountReady:
		status.Status = "needs_auth"
		status.NeedsAuth = true
	case len(status.MissingScopes) > 0:
		status.Status = "needs_auth"
		status.NeedsAuth = true
	case status.BackendPackageAvailable:
		status.Status = "configured"
		status.Configured = true
		status.NeedsAuth = false
	default:
		status.Status = "warning"
		status.NeedsAuth = false
	}

	_ = artifactConfig
	status.MissingFields = googleDriveMcpMissingFields(status)
	return status
}

func (r *Runner) resolveGoogleDriveArtifactStatusFromSavedConfig(configFile googleDriveWorkspaceConfigFile) (GoogleDriveArtifactSyncStatus, error) {
	secretState, err := r.googleDriveSecretState()
	if err != nil {
		return GoogleDriveArtifactSyncStatus{}, err
	}

	artifact := GoogleDriveArtifactSyncStatus{
		Source:          "saved",
		ClientID:        strings.TrimSpace(configFile.ArtifactSync.ClientID),
		RedirectURI:     normalizeGoogleDriveRedirectURI(configFile.ArtifactSync.RedirectURI),
		HasClientSecret: secretState.hasClientSecret,
		HasPickerAPIKey: secretState.hasPickerAPIKey,
	}
	artifact.MissingFields = googleDriveArtifactMissingFields(artifact)
	artifact.Configured = len(artifact.MissingFields) == 0
	artifact.Status = googleDriveArtifactStatus(artifact)
	return artifact, nil
}

func (r *Runner) resolveGoogleDriveArtifactStatusFromEnv() (GoogleDriveArtifactSyncStatus, error) {
	envConfig, err := readGoogleDriveArtifactEnvConfig()
	if err != nil {
		artifact := GoogleDriveArtifactSyncStatus{Source: "missing"}
		artifact.MissingFields = googleDriveArtifactMissingFields(artifact)
		artifact.Status = googleDriveArtifactStatus(artifact)
		return artifact, nil
	}

	artifact := GoogleDriveArtifactSyncStatus{
		Source:          "env",
		ClientID:        envConfig.ClientID,
		RedirectURI:     normalizeGoogleDriveRedirectURI(envConfig.RedirectURI),
		HasClientSecret: strings.TrimSpace(envConfig.ClientSecret) != "",
		HasPickerAPIKey: strings.TrimSpace(envConfig.PickerAPIKey) != "",
	}
	artifact.MissingFields = googleDriveArtifactMissingFields(artifact)
	artifact.Configured = len(artifact.MissingFields) == 0
	artifact.Status = googleDriveArtifactStatus(artifact)
	return artifact, nil
}

func (r *Runner) buildGoogleDriveValidationStatus(input GoogleDriveWorkspaceConfigRequest) (GoogleDriveWorkspaceConfigResponse, error) {
	current, err := r.loadGoogleDriveWorkspaceConfigFile()
	hasCurrent := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	artifact := GoogleDriveArtifactSyncStatus{Source: "missing"}
	if hasCurrent {
		currentArtifact, artifactErr := r.resolveGoogleDriveArtifactStatusFromSavedConfig(current)
		if artifactErr != nil {
			return GoogleDriveWorkspaceConfigResponse{}, artifactErr
		}
		artifact = currentArtifact
	} else {
		currentArtifact, _ := r.resolveGoogleDriveArtifactStatusFromEnv()
		artifact = currentArtifact
	}

	if strings.TrimSpace(input.ClientID) != "" {
		artifact.ClientID = strings.TrimSpace(input.ClientID)
	}
	if strings.TrimSpace(input.RedirectURI) != "" {
		artifact.RedirectURI = normalizeGoogleDriveRedirectURI(input.RedirectURI)
	}
	if strings.TrimSpace(input.ClientSecret) != "" {
		artifact.HasClientSecret = true
	}
	if strings.TrimSpace(input.PickerAPIKey) != "" {
		artifact.HasPickerAPIKey = true
	}
	if hasCurrent && googleDriveClientIDChangeNeedsNewSecret(current, input, artifact.HasClientSecret) {
		artifact.HasClientSecret = false
	}
	artifact.MissingFields = googleDriveArtifactMissingFields(artifact)
	artifact.Configured = len(artifact.MissingFields) == 0
	artifact.Status = googleDriveArtifactStatus(artifact)

	if strings.TrimSpace(input.MCPAccountID) != "" {
		current.MCP.AccountID = strings.TrimSpace(input.MCPAccountID)
	}

	mcp := r.resolveGoogleDriveMcpStatus(current)
	return GoogleDriveWorkspaceConfigResponse{
		ArtifactSync:    artifact,
		MCP:             mcp,
		RunnerReachable: true,
	}, nil
}

func (r *Runner) loadGoogleDriveWorkspaceConfigFile() (googleDriveWorkspaceConfigFile, error) {
	raw, err := os.ReadFile(r.googleDriveWorkspaceConfigPath())
	if err != nil {
		return googleDriveWorkspaceConfigFile{}, err
	}

	var config googleDriveWorkspaceConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		return googleDriveWorkspaceConfigFile{}, err
	}
	if config.Version == 0 {
		config.Version = 1
	}
	return config, nil
}

func (r *Runner) persistGoogleDriveWorkspaceConfigFile(config googleDriveWorkspaceConfigFile) error {
	path := r.googleDriveWorkspaceConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0o644)
}

func (r *Runner) googleDriveSecretState() (googleDriveSecretState, error) {
	state := googleDriveSecretState{}
	clientSecret, err := r.ensureSecretStore().Get(googleDriveArtifactSyncClientSecretKey)
	if err == nil {
		state.clientSecret = strings.TrimSpace(clientSecret)
		state.hasClientSecret = state.clientSecret != ""
	}
	pickerAPIKey, err := r.ensureSecretStore().Get(googleDriveArtifactSyncPickerAPIKeySecret)
	if err == nil {
		state.pickerAPIKey = strings.TrimSpace(pickerAPIKey)
		state.hasPickerAPIKey = state.pickerAPIKey != ""
	}
	return state, nil
}

func (r *Runner) saveGoogleDriveSecretIfProvided(key string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	secretStore := r.ensureSecretStore()
	if err := secretStore.Set(key, trimmed); err != nil {
		return err
	}
	persisted, err := secretStore.Get(key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(persisted) != trimmed {
		return fmt.Errorf("%s verification failed", key)
	}
	return nil
}

func (r *Runner) restoreGoogleDriveSecretState(previous googleDriveSecretState) {
	if previous.hasClientSecret {
		_ = r.ensureSecretStore().Set(googleDriveArtifactSyncClientSecretKey, previous.clientSecret)
	} else {
		_ = r.ensureSecretStore().Delete(googleDriveArtifactSyncClientSecretKey)
	}
	if previous.hasPickerAPIKey {
		_ = r.ensureSecretStore().Set(googleDriveArtifactSyncPickerAPIKeySecret, previous.pickerAPIKey)
	} else {
		_ = r.ensureSecretStore().Delete(googleDriveArtifactSyncPickerAPIKeySecret)
	}
}

func findGoogleDriveAccountStatus(accounts []GoogleDriveAccountStatus, accountID string) (GoogleDriveAccountStatus, bool) {
	trimmedAccountID := strings.TrimSpace(accountID)
	for _, account := range accounts {
		if strings.EqualFold(strings.TrimSpace(account.AccountID), trimmedAccountID) {
			return account, true
		}
	}
	return GoogleDriveAccountStatus{}, false
}

func (r *Runner) googleDriveAccountHasArtifactBinding(accountID string) bool {
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return false
	}
	normalizedAccountID := normalizeGoogleDriveStoredAccountID(accountID)
	for _, connection := range state.Connections {
		if !strings.EqualFold(normalizeGoogleDriveStoredAccountID(connection.AccountID), normalizedAccountID) {
			continue
		}
		if strings.TrimSpace(connection.FolderID) != "" {
			return true
		}
	}
	return false
}

func (r *Runner) resolveGoogleDriveProxyAccountSelection(configFile googleDriveWorkspaceConfigFile) (googleDriveProxyAccountSelection, error) {
	accounts, err := r.resolveGoogleDriveAccountStatuses()
	if err != nil {
		return googleDriveProxyAccountSelection{}, err
	}

	selectedAccountID := strings.TrimSpace(configFile.MCP.AccountID)
	if selectedAccountID == "" {
		selectedAccountID = strings.TrimSpace(os.Getenv("FLOWPILOT_GOOGLE_DRIVE_ACCOUNT_ID"))
	}
	if selectedAccountID != "" {
		if account, ok := findGoogleDriveAccountStatus(accounts, selectedAccountID); ok {
			return googleDriveProxyAccountSelection{
				AccountID:    account.AccountID,
				AccountEmail: account.AccountEmail,
				Accounts:     accounts,
				Required:     false,
			}, nil
		}
		return googleDriveProxyAccountSelection{
			AccountID: selectedAccountID,
			Accounts:  accounts,
			Required:  false,
		}, nil
	}

	if len(accounts) == 1 {
		return googleDriveProxyAccountSelection{
			AccountID:    accounts[0].AccountID,
			AccountEmail: accounts[0].AccountEmail,
			Accounts:     accounts,
			Required:     false,
		}, nil
	}

	return googleDriveProxyAccountSelection{
		Accounts: accounts,
		Required: true,
	}, nil
}

func readGoogleDriveArtifactEnvConfig() (googleDriveArtifactEnvConfig, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_SECRET"))
	redirectURI := normalizeGoogleDriveRedirectURI(os.Getenv("GOOGLE_DRIVE_REDIRECT_URI"))
	pickerAPIKey := strings.TrimSpace(os.Getenv("GOOGLE_PICKER_API_KEY"))
	if clientID == "" {
		return googleDriveArtifactEnvConfig{}, errors.New("missing required environment variable: GOOGLE_DRIVE_CLIENT_ID")
	}
	if clientSecret == "" {
		return googleDriveArtifactEnvConfig{}, errors.New("missing required environment variable: GOOGLE_DRIVE_CLIENT_SECRET")
	}
	if redirectURI == "" {
		return googleDriveArtifactEnvConfig{}, errors.New("missing required environment variable: GOOGLE_DRIVE_REDIRECT_URI")
	}
	if pickerAPIKey == "" {
		return googleDriveArtifactEnvConfig{}, errors.New("missing required environment variable: GOOGLE_PICKER_API_KEY")
	}

	return googleDriveArtifactEnvConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		PickerAPIKey: pickerAPIKey,
	}, nil
}

func normalizeGoogleDriveRedirectURI(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func googleDriveClientIDChangeNeedsNewSecret(current googleDriveWorkspaceConfigFile, input GoogleDriveWorkspaceConfigRequest, hasStoredSecret bool) bool {
	if !hasStoredSecret {
		return false
	}
	nextClientID := strings.TrimSpace(input.ClientID)
	if nextClientID == "" {
		return false
	}
	currentClientID := strings.TrimSpace(current.ArtifactSync.ClientID)
	if currentClientID == "" || strings.EqualFold(currentClientID, nextClientID) {
		return false
	}
	return strings.TrimSpace(input.ClientSecret) == ""
}

func googleDriveArtifactMissingFields(status GoogleDriveArtifactSyncStatus) []string {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(status.ClientID) == "" {
		missing = append(missing, "clientId")
	}
	if strings.TrimSpace(status.RedirectURI) == "" {
		missing = append(missing, "redirectUri")
	}
	if !status.HasClientSecret {
		missing = append(missing, "clientSecret")
	}
	if !status.HasPickerAPIKey {
		missing = append(missing, "pickerApiKey")
	}
	return missing
}

func googleDriveArtifactStatus(status GoogleDriveArtifactSyncStatus) string {
	switch {
	case status.Configured:
		return "configured"
	case status.Source == "env":
		return "needs_input"
	case status.Source == "saved":
		return "needs_input"
	default:
		return "not_started"
	}
}

func googleDriveCheck(key string, ok bool, passed string, failed string) GoogleDriveValidationCheck {
	if ok {
		return googleDrivePassedCheck(key, passed)
	}
	return googleDriveFailedCheck(key, failed)
}

func googleDrivePassedCheck(key string, message string) GoogleDriveValidationCheck {
	return GoogleDriveValidationCheck{Key: key, Status: "passed", Message: message}
}

func googleDriveFailedCheck(key string, message string) GoogleDriveValidationCheck {
	return GoogleDriveValidationCheck{Key: key, Status: "failed", Message: message}
}

func googleDriveSkippedCheck(key string, message string) GoogleDriveValidationCheck {
	return GoogleDriveValidationCheck{Key: key, Status: "skipped", Message: message}
}

func googleDriveMcpMissingFields(status GoogleDriveMcpStatus) []string {
	missing := make([]string, 0, 4)
	if status.ProxyMcpEnabled {
		if status.AccountSelectionRequired || strings.TrimSpace(status.AccountID) == "" {
			missing = append(missing, "accountId")
		}
		if !status.AccountReady {
			missing = append(missing, "refreshToken")
		}
		if len(status.MissingScopes) > 0 {
			missing = append(missing, "mcpReadScope")
		}
		if status.Status == "reconnect_required" {
			missing = append(missing, "tokenReconnect")
		}
		if !status.BackendPackageAvailable {
			missing = append(missing, "backendPackage")
		}
		return missing
	}

	if strings.TrimSpace(status.CredentialPath) == "" {
		missing = append(missing, "credentialPath")
	}
	if !status.CredentialFileExists {
		missing = append(missing, "credentialFile")
	}
	if status.CredentialFileExists && !status.CredentialFileValid {
		missing = append(missing, "credentialJson")
	}
	if strings.TrimSpace(status.TokenPath) == "" {
		missing = append(missing, "tokenPath")
	}
	if !status.TokenFileExists {
		missing = append(missing, "tokenFile")
	}
	if status.Status == "reconnect_required" {
		missing = append(missing, "tokenReconnect")
	}
	if !status.BackendPackageAvailable {
		missing = append(missing, "backendPackage")
	}
	return missing
}

func googleDriveMcpBackendAvailable() bool {
	_, err := lookPathFn("npx")
	return err == nil
}

func googleDriveProxyMcpBackendAvailable(workspace string) bool {
	if _, err := lookPathFn("flowpilot"); err == nil {
		return true
	}

	runnerDir := filepath.Join(strings.TrimSpace(filepath.Clean(workspace)), "apps", "local-runner")
	if _, err := os.Stat(filepath.Join(runnerDir, "go.mod")); err == nil {
		_, err := lookPathFn("go")
		return err == nil
	}

	return false
}

func validateGoogleDriveOAuthCredentialJSONFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = validateGoogleDriveOAuthCredentialJSON(raw)
	return err
}

func loadGoogleDriveOAuthCredentialJSONFile(path string) (googleDriveOAuthClientData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return googleDriveOAuthClientData{}, err
	}
	return validateGoogleDriveOAuthCredentialJSON(raw)
}

func validateGoogleDriveOAuthCredentialJSON(raw []byte) (googleDriveOAuthClientData, error) {
	var payload googleDriveOAuthClientJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return googleDriveOAuthClientData{}, err
	}

	if payload.Web != nil {
		return googleDriveOAuthClientData{}, errors.New("google drive MCP OAuth JSON must use a Desktop OAuth client (installed); Web OAuth clients are not supported for this flow")
	}
	data := payload.Installed
	if data == nil {
		return googleDriveOAuthClientData{}, errors.New("google drive MCP OAuth JSON must contain an installed client object")
	}
	if strings.TrimSpace(data.ClientID) == "" {
		return googleDriveOAuthClientData{}, errors.New("google drive OAuth JSON is missing client_id")
	}
	if strings.TrimSpace(data.ClientSecret) == "" {
		return googleDriveOAuthClientData{}, errors.New("google drive OAuth JSON is missing client_secret")
	}
	if strings.TrimSpace(data.AuthURI) == "" || strings.TrimSpace(data.TokenURI) == "" {
		return googleDriveOAuthClientData{}, errors.New("google drive OAuth JSON is missing auth_uri or token_uri")
	}

	return *data, nil
}

func (r *Runner) UploadGoogleDriveMcpOAuthCredentials(payload GoogleDriveMcpOAuthUploadRequest) (GoogleDriveWorkspaceConfigResponse, error) {
	trimmedContent := strings.TrimSpace(payload.Content)
	if trimmedContent == "" {
		return GoogleDriveWorkspaceConfigResponse{}, errors.New("content is required")
	}

	credentialData, err := validateGoogleDriveOAuthCredentialJSON([]byte(trimmedContent))
	if err != nil {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	credentialPath := googleDriveMcpCredentialPath()
	tokenPath := googleDriveMcpTokenPath()
	if err := os.MkdirAll(filepath.Dir(credentialPath), 0o755); err != nil {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	if err := os.WriteFile(credentialPath, []byte(trimmedContent), 0o600); err != nil {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	config, err := r.loadGoogleDriveWorkspaceConfigFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	config.Version = 1
	config.MCP.CredentialPath = credentialPath
	config.MCP.TokenPath = tokenPath
	config.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := r.persistGoogleDriveWorkspaceConfigFile(config); err != nil {
		_ = os.Remove(credentialPath)
		return GoogleDriveWorkspaceConfigResponse{}, err
	}

	status, err := r.resolveGoogleDriveWorkspaceStatus()
	if err != nil {
		_ = os.Remove(credentialPath)
		return GoogleDriveWorkspaceConfigResponse{}, err
	}
	status.MCP.CredentialFileValid = status.MCP.CredentialFileValid && strings.TrimSpace(credentialData.ClientID) != ""
	return status, nil
}

func (r *Runner) googleDriveMcpCommandEnv() ([]string, error) {
	config, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		return nil, err
	}

	if !config.CredentialExists || !config.CredentialValid {
		return nil, errors.New("google drive MCP OAuth credentials are not configured")
	}

	if err := os.MkdirAll(filepath.Dir(config.TokenPath), 0o755); err != nil {
		return nil, err
	}

	return []string{
		"GOOGLE_DRIVE_OAUTH_CREDENTIALS=" + config.CredentialPath,
		"GOOGLE_DRIVE_MCP_TOKEN_PATH=" + config.TokenPath,
	}, nil
}

var runCommandWithEnvFn = func(ctx context.Context, name string, env []string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		command.Env = append(os.Environ(), env...)
	}
	return command.CombinedOutput()
}

func (r *Runner) executeGoogleDriveMcpCommand(ctx context.Context, launcherPath string, args []string) error {
	env, err := r.googleDriveMcpCommandEnv()
	if err != nil {
		return err
	}
	output, runErr := runCommandWithEnvFn(ctx, launcherPath, env, args...)
	if runErr != nil {
		return errors.New(backendErrorMessage(runErr, output))
	}
	return nil
}

type googleDriveSecretState struct {
	clientSecret    string
	pickerAPIKey    string
	hasClientSecret bool
	hasPickerAPIKey bool
}

type googleDriveMcpRuntimeConfig struct {
	CredentialPath           string
	TokenPath                string
	CredentialExists         bool
	CredentialValid          bool
	TokenExists              bool
	TokenRefreshValid        bool
	BackendPackageAvailable  bool
	AccountID                string
	AccountEmail             string
	AccountSelectionRequired bool
	Status                   string
}

type googleDriveMcpTokenFile struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiryDate   int64  `json:"expiry_date"`
}

var errGoogleDriveMcpTokenNeedsAuth = errors.New("google drive MCP token requires re-authentication")

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readGoogleDriveMcpTokenFile(path string) (googleDriveMcpTokenFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return googleDriveMcpTokenFile{}, err
	}

	var tokenFile googleDriveMcpTokenFile
	if err := json.Unmarshal(raw, &tokenFile); err != nil {
		return googleDriveMcpTokenFile{}, err
	}
	return tokenFile, nil
}

func validateGoogleDriveMcpStoredRefreshToken(credentialPath, tokenPath string) (bool, error) {
	credentialData, err := loadGoogleDriveOAuthCredentialJSONFile(credentialPath)
	if err != nil {
		return false, err
	}

	tokenFile, err := readGoogleDriveMcpTokenFile(tokenPath)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(tokenFile.RefreshToken) == "" {
		return false, errGoogleDriveMcpTokenNeedsAuth
	}

	_, err = googleDriveRefreshAccessToken(
		credentialData.ClientID,
		credentialData.ClientSecret,
		tokenFile.RefreshToken,
	)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (r *Runner) buildGoogleDriveArtifactRuntimeConfigFromSaved(config googleDriveWorkspaceConfigFile, secretState googleDriveSecretState) (googleDriveArtifactRuntimeConfig, error) {
	result := googleDriveArtifactRuntimeConfig{
		ClientID:     strings.TrimSpace(config.ArtifactSync.ClientID),
		RedirectURI:  normalizeGoogleDriveRedirectURI(config.ArtifactSync.RedirectURI),
		ClientSecret: "",
		PickerAPIKey: "",
		Source:       "saved",
	}

	if result.RedirectURI == "" {
		result.RedirectURI = googleDriveDefaultRedirectURI
	}

	if secretState.hasClientSecret {
		secret, err := r.ensureSecretStore().Get(googleDriveArtifactSyncClientSecretKey)
		if err != nil {
			return googleDriveArtifactRuntimeConfig{}, err
		}
		result.ClientSecret = strings.TrimSpace(secret)
	}
	if secretState.hasPickerAPIKey {
		secret, err := r.ensureSecretStore().Get(googleDriveArtifactSyncPickerAPIKeySecret)
		if err != nil {
			return googleDriveArtifactRuntimeConfig{}, err
		}
		result.PickerAPIKey = strings.TrimSpace(secret)
	}

	if strings.TrimSpace(result.ClientID) == "" || strings.TrimSpace(result.ClientSecret) == "" || strings.TrimSpace(result.RedirectURI) == "" || strings.TrimSpace(result.PickerAPIKey) == "" {
		return googleDriveArtifactRuntimeConfig{}, googleDriveArtifactConfigMissingError(result)
	}

	return result, nil
}

func googleDriveArtifactConfigMissingError(config googleDriveArtifactRuntimeConfig) error {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(config.ClientID) == "" {
		missing = append(missing, "clientId")
	}
	if strings.TrimSpace(config.ClientSecret) == "" {
		missing = append(missing, "clientSecret")
	}
	if strings.TrimSpace(config.RedirectURI) == "" {
		missing = append(missing, "redirectUri")
	}
	if strings.TrimSpace(config.PickerAPIKey) == "" {
		missing = append(missing, "pickerApiKey")
	}
	return fmt.Errorf("google drive artifact config is incomplete: missing %s", strings.Join(missing, ", "))
}

func googleDriveArtifactConfigFromEnvConfig(env googleDriveArtifactEnvConfig) googleDriveArtifactRuntimeConfig {
	return googleDriveArtifactRuntimeConfig{
		ClientID:     env.ClientID,
		ClientSecret: env.ClientSecret,
		RedirectURI:  env.RedirectURI,
		PickerAPIKey: env.PickerAPIKey,
		Source:       "env",
	}
}

func googleDriveOAuthFailure(operation string, statusCode int, responseBody []byte) error {
	responseText := strings.TrimSpace(string(responseBody))
	parsed := googleDriveOAuthErrorResponse{}
	if err := json.Unmarshal(responseBody, &parsed); err == nil {
		providerError := strings.TrimSpace(parsed.Error)
		providerMessage := strings.TrimSpace(parsed.ErrorDescription)
		return &googleDriveOAuthRequestError{
			Operation:         operation,
			StatusCode:        statusCode,
			ProviderError:     providerError,
			ProviderMessage:   providerMessage,
			RawMessage:        responseText,
			ReconnectRequired: strings.EqualFold(providerError, "invalid_grant"),
		}
	}

	return &googleDriveOAuthRequestError{
		Operation:         operation,
		StatusCode:        statusCode,
		RawMessage:        responseText,
		ReconnectRequired: strings.Contains(strings.ToLower(responseText), "invalid_grant"),
	}
}

func googleDriveReconnectRequired(err error) bool {
	var oauthErr *googleDriveOAuthRequestError
	return errors.As(err, &oauthErr) && oauthErr.ReconnectRequired
}

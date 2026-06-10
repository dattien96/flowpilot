package runner

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type googleDriveCredential struct {
	RefreshToken string `json:"refreshToken"`
	AccountEmail string `json:"accountEmail,omitempty"`
}

func googleDriveCredentialKey(integrationID string) string {
	return normalizeSecretKey("google-drive", integrationID)
}

func googleDriveProjectCredentialKey(projectID string) string {
	return normalizeSecretKey("artifact-storage", "google-drive", projectID)
}

func googleDriveAccountCredentialKey(accountID string) string {
	return normalizeSecretKey("google-drive", "account", accountID)
}

func (r *Runner) saveGoogleDriveCredential(integrationID string, creds googleDriveCredential) error {
	if strings.TrimSpace(integrationID) == "" {
		return errors.New("google drive integration id is required")
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("google drive refresh token is required")
	}

	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	return r.ensureSecretStore().Set(googleDriveCredentialKey(integrationID), string(raw))
}

func (r *Runner) saveGoogleDriveCredentialByAccount(accountID string, creds googleDriveCredential) error {
	trimmedAccountID := strings.TrimSpace(accountID)
	if trimmedAccountID == "" {
		return errors.New("google drive account id is required")
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("google drive refresh token is required")
	}

	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	return r.ensureSecretStore().Set(googleDriveAccountCredentialKey(trimmedAccountID), string(raw))
}

func (r *Runner) loadGoogleDriveCredential(integrationID string) (googleDriveCredential, error) {
	if strings.TrimSpace(integrationID) == "" {
		return googleDriveCredential{}, errors.New("google drive integration id is required")
	}

	raw, err := r.ensureSecretStore().Get(googleDriveCredentialKey(integrationID))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return googleDriveCredential{}, errors.New("google drive is not connected on this runner yet")
		}
		return googleDriveCredential{}, err
	}

	var creds googleDriveCredential
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return googleDriveCredential{}, err
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return googleDriveCredential{}, errors.New("google drive refresh token is not configured")
	}

	return creds, nil
}

func (r *Runner) loadGoogleDriveCredentialBySecretKey(secretKey string) (googleDriveCredential, error) {
	trimmedKey := strings.TrimSpace(secretKey)
	if trimmedKey == "" {
		return googleDriveCredential{}, errors.New("google drive credential key is required")
	}

	raw, err := r.ensureSecretStore().Get(trimmedKey)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return googleDriveCredential{}, errors.New("google drive is not connected on this runner yet")
		}
		return googleDriveCredential{}, err
	}

	var creds googleDriveCredential
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return googleDriveCredential{}, err
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return googleDriveCredential{}, errors.New("google drive refresh token is not configured")
	}
	return creds, nil
}

func (r *Runner) saveGoogleDriveCredentialByProject(projectID string, creds googleDriveCredential) error {
	if strings.TrimSpace(projectID) == "" {
		return errors.New("google drive project id is required")
	}
	if strings.TrimSpace(creds.RefreshToken) == "" {
		return errors.New("google drive refresh token is required")
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return r.ensureSecretStore().Set(googleDriveProjectCredentialKey(projectID), string(raw))
}

func (r *Runner) loadGoogleDriveCredentialBySession(sessionID string) (googleDriveCredential, error) {
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID == "" {
		return googleDriveCredential{}, errors.New("google drive session id is required")
	}

	state, err := r.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return googleDriveCredential{}, err
	}
	record, ok := state.Sessions[trimmedSessionID]
	if !ok {
		return googleDriveCredential{}, errors.New("google drive connect session is not available")
	}

	accountID := strings.TrimSpace(record.AccountID)
	if accountID == "" {
		accountID = strings.ToLower(strings.TrimSpace(record.AccountEmail))
	}
	if accountID != "" {
		creds, accountErr := r.loadGoogleDriveCredentialByAccount(accountID)
		if accountErr == nil {
			return creds, nil
		}
		if !strings.Contains(strings.ToLower(accountErr.Error()), "not connected") {
			return googleDriveCredential{}, accountErr
		}
	}

	return r.loadGoogleDriveCredentialByProject(record.ProjectID)
}

func (r *Runner) loadGoogleDriveCredentialByAccount(accountID string) (googleDriveCredential, error) {
	trimmedAccountID := strings.TrimSpace(accountID)
	if trimmedAccountID == "" {
		return googleDriveCredential{}, errors.New("google drive account id is required")
	}

	if creds, err := r.loadGoogleDriveCredentialBySecretKey(googleDriveAccountCredentialKey(trimmedAccountID)); err == nil {
		return creds, nil
	}

	state, err := r.loadArtifactStorageGoogleDriveState()
	if err == nil {
		for _, account := range state.Accounts {
			if strings.EqualFold(strings.TrimSpace(account.AccountID), trimmedAccountID) {
				for projectID, connection := range state.Connections {
					candidateAccountID := strings.TrimSpace(connection.AccountID)
					if candidateAccountID == "" {
						candidateAccountID = strings.ToLower(strings.TrimSpace(connection.AccountEmail))
					}
					if candidateAccountID == "" {
						candidateAccountID = strings.ToLower(strings.TrimSpace(projectID))
					}
					if strings.EqualFold(candidateAccountID, trimmedAccountID) {
						return r.loadGoogleDriveCredentialBySecretKey(googleDriveProjectCredentialKey(projectID))
					}
				}
				for sessionID, session := range state.Sessions {
					candidateAccountID := strings.TrimSpace(session.AccountID)
					if candidateAccountID == "" {
						candidateAccountID = strings.ToLower(strings.TrimSpace(session.AccountEmail))
					}
					if candidateAccountID == "" {
						candidateAccountID = strings.ToLower(strings.TrimSpace(session.ProjectID))
					}
					if strings.EqualFold(candidateAccountID, trimmedAccountID) {
						return r.loadGoogleDriveCredentialBySecretKey(googleDriveProjectCredentialKey(session.ProjectID))
					}
					_ = sessionID
				}
			}
		}
	}

	return r.loadGoogleDriveCredentialBySecretKey(googleDriveAccountCredentialKey(trimmedAccountID))
}

func (r *Runner) loadGoogleDriveCredentialByProject(projectID string) (googleDriveCredential, error) {
	if strings.TrimSpace(projectID) == "" {
		return googleDriveCredential{}, errors.New("google drive project id is required")
	}
	state, err := r.loadArtifactStorageGoogleDriveState()
	if err == nil {
		if record, ok := state.Connections[strings.TrimSpace(projectID)]; ok {
			accountID := strings.TrimSpace(record.AccountID)
			if accountID == "" {
				accountID = strings.ToLower(strings.TrimSpace(record.AccountEmail))
			}
			if accountID != "" {
				if creds, accountErr := r.loadGoogleDriveCredentialByAccount(accountID); accountErr == nil {
					return creds, nil
				}
			}
		}
	}

	return r.loadGoogleDriveCredentialBySecretKey(googleDriveProjectCredentialKey(projectID))
}

type googleDriveArtifactConfig struct {
	clientID     string
	clientSecret string
}

func readGoogleDriveArtifactConfig() (googleDriveArtifactConfig, error) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_SECRET"))
	if clientID == "" {
		return googleDriveArtifactConfig{}, errors.New("missing required environment variable: GOOGLE_DRIVE_CLIENT_ID")
	}
	if clientSecret == "" {
		return googleDriveArtifactConfig{}, errors.New("missing required environment variable: GOOGLE_DRIVE_CLIENT_SECRET")
	}
	return googleDriveArtifactConfig{
		clientID:     clientID,
		clientSecret: clientSecret,
	}, nil
}

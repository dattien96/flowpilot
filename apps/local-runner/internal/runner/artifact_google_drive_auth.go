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

func (r *Runner) loadGoogleDriveCredentialByProject(projectID string) (googleDriveCredential, error) {
	if strings.TrimSpace(projectID) == "" {
		return googleDriveCredential{}, errors.New("google drive project id is required")
	}
	raw, err := r.ensureSecretStore().Get(googleDriveProjectCredentialKey(projectID))
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

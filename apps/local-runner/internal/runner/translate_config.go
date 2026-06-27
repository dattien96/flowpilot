package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	translateAPIKeySecret   = "translate:libre:api-key"
	translateConfigFileName = "translate-config.json"
)

type translateConfigFile struct {
	BaseURL string `json:"baseUrl,omitempty"`
}

// TranslateConfigRequest is the PUT body for /translate-config.
type TranslateConfigRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey,omitempty"`
}

// TranslateConfigStatus is returned by GET /translate-config.
type TranslateConfigStatus struct {
	BaseURL   string `json:"baseUrl"`
	HasAPIKey bool   `json:"hasApiKey"`
}

func (r *Runner) translateConfigPath() string {
	cwd := r.Health().Cwd
	if cwd == "" {
		cwd = "."
	}
	return filepath.Join(cwd, ".flowpilot", translateConfigFileName)
}

func (r *Runner) loadTranslateConfigFile() translateConfigFile {
	cfg := translateConfigFile{}
	if raw, err := os.ReadFile(r.translateConfigPath()); err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultTranslateBaseURL()
	}
	return cfg
}

// LoadTranslateConfig returns the current config status (safe to send to the frontend).
func (r *Runner) LoadTranslateConfig() (TranslateConfigStatus, error) {
	cfg := r.loadTranslateConfigFile()
	apiKey, _ := r.ensureSecretStore().Get(translateAPIKeySecret)
	return TranslateConfigStatus{
		BaseURL:   cfg.BaseURL,
		HasAPIKey: strings.TrimSpace(apiKey) != "",
	}, nil
}

// SaveTranslateConfig persists the base URL and, when non-empty, updates the API key.
// An empty APIKey in the request leaves the existing stored key unchanged.
func (r *Runner) SaveTranslateConfig(req TranslateConfigRequest) (TranslateConfigStatus, error) {
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = defaultTranslateBaseURL()
	}

	cfg := translateConfigFile{BaseURL: baseURL}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return TranslateConfigStatus{}, err
	}
	if err := os.MkdirAll(filepath.Dir(r.translateConfigPath()), 0o755); err != nil {
		return TranslateConfigStatus{}, err
	}
	if err := os.WriteFile(r.translateConfigPath(), raw, 0o644); err != nil {
		return TranslateConfigStatus{}, err
	}

	if trimmed := strings.TrimSpace(req.APIKey); trimmed != "" {
		if err := r.ensureSecretStore().Set(translateAPIKeySecret, trimmed); err != nil {
			return TranslateConfigStatus{}, err
		}
	}

	storedKey, _ := r.ensureSecretStore().Get(translateAPIKeySecret)
	return TranslateConfigStatus{
		BaseURL:   baseURL,
		HasAPIKey: strings.TrimSpace(storedKey) != "",
	}, nil
}

func defaultTranslateBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("FLOWPILOT_LIBRETRANSLATE_URL")); value != "" {
		return value
	}
	port := strings.TrimSpace(os.Getenv("FLOWPILOT_LIBRETRANSLATE_PORT"))
	if port == "" {
		port = "5001"
	}
	return fmt.Sprintf("http://127.0.0.1:%s", port)
}

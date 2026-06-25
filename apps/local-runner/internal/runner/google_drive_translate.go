package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TranslateRequest is the input for a translation call.
type TranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
}

// TranslateResponse is returned by the /translate endpoint.
type TranslateResponse struct {
	TranslatedText string `json:"translatedText"`
	Source         string `json:"source"`
	Target         string `json:"target"`
}

// TranslateText calls the LibreTranslate POST /translate API using the
// configured base URL and optional API key from the runner secret store.
func (r *Runner) TranslateText(req TranslateRequest) (TranslateResponse, error) {
	q := strings.TrimSpace(req.Q)
	if q == "" {
		return TranslateResponse{}, fmt.Errorf("q is required")
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = "vi"
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "auto"
	}

	cfg, err := r.LoadTranslateConfig()
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("could not load translate config: %w", err)
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")

	payload := map[string]string{
		"q":      q,
		"source": source,
		"target": target,
		"format": "text",
	}
	if cfg.HasAPIKey {
		if key, _ := r.ensureSecretStore().Get(translateAPIKeySecret); strings.TrimSpace(key) != "" {
			payload["api_key"] = strings.TrimSpace(key)
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return TranslateResponse{}, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(baseURL+"/translate", "application/json", bytes.NewReader(body))
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("translation request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return TranslateResponse{}, fmt.Errorf("LibreTranslate: %s", apiErr.Error)
		}
		return TranslateResponse{}, fmt.Errorf("LibreTranslate returned status %d", resp.StatusCode)
	}

	var result struct {
		TranslatedText string `json:"translatedText"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.TranslatedText == "" {
		return TranslateResponse{}, fmt.Errorf("empty translation in response")
	}

	return TranslateResponse{
		TranslatedText: result.TranslatedText,
		Source:         source,
		Target:         target,
	}, nil
}

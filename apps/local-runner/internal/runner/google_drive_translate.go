package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// TranslateText calls the Google Cloud Translation Basic API (v2) using the
// stored Picker API key. The same key works for translations when the Cloud
// Translation API is enabled on the same Google Cloud project.
func (r *Runner) TranslateText(req TranslateRequest) (TranslateResponse, error) {
	q := strings.TrimSpace(req.Q)
	if q == "" {
		return TranslateResponse{}, fmt.Errorf("q is required")
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = "vi"
	}

	secrets, err := r.googleDriveSecretState()
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("could not load Google API key: %w", err)
	}
	if !secrets.hasPickerAPIKey {
		return TranslateResponse{}, fmt.Errorf("no Google API key configured — add one in Settings → Google")
	}

	params := url.Values{}
	params.Set("key", secrets.pickerAPIKey)
	params.Set("q", q)
	params.Set("target", target)
	params.Set("format", "text")
	if src := strings.TrimSpace(req.Source); src != "" {
		params.Set("source", src)
	}

	apiURL := "https://translation.googleapis.com/language/translate/v2?" + params.Encode()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("translation request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			return TranslateResponse{}, fmt.Errorf("Google Translate API %d: %s", apiErr.Error.Code, apiErr.Error.Message)
		}
		return TranslateResponse{}, fmt.Errorf("Google Translate API returned status %d", resp.StatusCode)
	}

	// v2 response: {"data":{"translations":[{"translatedText":"...","detectedSourceLanguage":"..."}]}}
	var result struct {
		Data struct {
			Translations []struct {
				TranslatedText         string `json:"translatedText"`
				DetectedSourceLanguage string `json:"detectedSourceLanguage"`
			} `json:"translations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return TranslateResponse{}, fmt.Errorf("failed to parse translation response: %w", err)
	}
	if len(result.Data.Translations) == 0 {
		return TranslateResponse{}, fmt.Errorf("empty translations array in response")
	}

	detected := result.Data.Translations[0].DetectedSourceLanguage
	if detected == "" {
		detected = req.Source
	}
	return TranslateResponse{
		TranslatedText: result.Data.Translations[0].TranslatedText,
		Source:         detected,
		Target:         target,
	}, nil
}

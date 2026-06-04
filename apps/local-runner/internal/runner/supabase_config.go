package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const supabaseServiceRoleSecretKey = "supabase:workspace:service-role-key"

func (r *Runner) LoadSupabaseWorkspaceConfig() (SupabaseWorkspaceConfigResponse, error) {
	config, err := r.loadSupabaseWorkspaceConfig()
	if err != nil {
		return SupabaseWorkspaceConfigResponse{}, err
	}

	hasSecret, err := r.hasSupabaseServiceRoleKey()
	if err != nil {
		return SupabaseWorkspaceConfigResponse{}, err
	}

	return SupabaseWorkspaceConfigResponse{
		SupabaseWorkspaceConfig: config,
		HasServiceRoleKey:       hasSecret,
	}, nil
}

func (r *Runner) LoadSupabaseWorkspaceConfigWithSecret() (SupabaseWorkspaceConfigResponse, error) {
	response, err := r.LoadSupabaseWorkspaceConfig()
	if err != nil {
		return SupabaseWorkspaceConfigResponse{}, err
	}

	secret, err := r.ensureSecretStore().Get(supabaseServiceRoleSecretKey)
	if err != nil {
		return response, nil
	}
	response.ServiceRoleKey = secret
	response.HasServiceRoleKey = strings.TrimSpace(secret) != ""
	return response, nil
}

func (r *Runner) SaveSupabaseWorkspaceConfig(input SupabaseWorkspaceConfigRequest) (SupabaseWorkspaceConfigResponse, error) {
	serviceRoleKey := strings.TrimSpace(input.ServiceRoleKey)
	hasExistingSecret, err := r.hasSupabaseServiceRoleKey()
	if err != nil {
		return SupabaseWorkspaceConfigResponse{}, err
	}
	previousServiceRoleKey := ""
	if hasExistingSecret {
		previousServiceRoleKey, err = r.ensureSecretStore().Get(supabaseServiceRoleSecretKey)
		if err != nil {
			return SupabaseWorkspaceConfigResponse{}, err
		}
	}
	if serviceRoleKey == "" && !hasExistingSecret {
		return SupabaseWorkspaceConfigResponse{}, errors.New("service role key is required")
	}

	validation, err := r.ValidateSupabaseWorkspaceConfig(input)
	if err != nil {
		return SupabaseWorkspaceConfigResponse{}, err
	}
	if !validation.Valid {
		return SupabaseWorkspaceConfigResponse{}, errors.New(firstFailedSupabaseCheck(validation.Checks))
	}

	config := validation.BrowserSafeConfig
	config.Version = 1
	config.Status = "configured"
	config.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if serviceRoleKey != "" {
		secretStore := r.ensureSecretStore()
		if err := secretStore.Set(supabaseServiceRoleSecretKey, serviceRoleKey); err != nil {
			return SupabaseWorkspaceConfigResponse{}, err
		}
		persistedSecret, err := secretStore.Get(supabaseServiceRoleSecretKey)
		if err != nil {
			return SupabaseWorkspaceConfigResponse{}, err
		}
		if strings.TrimSpace(persistedSecret) != serviceRoleKey {
			return SupabaseWorkspaceConfigResponse{}, errors.New("service role key verification failed")
		}
	}
	if err := r.persistSupabaseWorkspaceConfig(config); err != nil {
		r.rollbackSupabaseServiceRoleKey(hasExistingSecret, previousServiceRoleKey)
		return SupabaseWorkspaceConfigResponse{}, err
	}

	return SupabaseWorkspaceConfigResponse{
		SupabaseWorkspaceConfig: config,
		HasServiceRoleKey:       true,
	}, nil
}

func (r *Runner) rollbackSupabaseServiceRoleKey(hadPrevious bool, previousValue string) {
	if hadPrevious {
		_ = r.ensureSecretStore().Set(supabaseServiceRoleSecretKey, previousValue)
		return
	}
	_ = r.ensureSecretStore().Delete(supabaseServiceRoleSecretKey)
}

func (r *Runner) ResetSupabaseWorkspaceConfig() error {
	err := os.Remove(r.supabaseWorkspaceConfigPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.ensureSecretStore().Delete(supabaseServiceRoleSecretKey)
}

func (r *Runner) ValidateSupabaseWorkspaceConfig(input SupabaseWorkspaceConfigRequest) (SupabaseValidationResult, error) {
	config := normalizeSupabaseWorkspaceConfig(input.SupabaseWorkspaceConfig)
	serviceRoleKey := strings.TrimSpace(input.ServiceRoleKey)
	hasExistingSecret, err := r.hasSupabaseServiceRoleKey()
	if err != nil {
		return SupabaseValidationResult{}, err
	}
	hasServiceRoleKey := serviceRoleKey != "" || hasExistingSecret

	checks := make([]SupabaseValidationCheck, 0, 7)
	apiURL, apiOK := parseHTTPSURL(config.APIURL)
	checks = append(checks, check("api_url_format", apiOK, "Supabase URL is a valid HTTPS URL.", "Enter a valid HTTPS Supabase URL."))
	checks = append(checks, check("anon_key_present", strings.TrimSpace(config.AnonKey) != "", "Anon key is present.", "Anon key is required."))
	edgeURL, edgeOK := parseHTTPSURL(config.EdgeFunctionURL)
	checks = append(checks, check("edge_url_format", edgeOK, "Edge Function URL is a valid HTTPS URL.", "Enter a valid HTTPS Edge Function URL."))

	projectMatch := apiOK && edgeOK && supabaseProjectRefFromHost(apiURL.Host) == supabaseProjectRefFromHost(edgeURL.Host)
	checks = append(checks, check("edge_url_project_match", projectMatch, "Edge Function URL matches the Supabase project.", "Edge Function URL must belong to the same Supabase project as the Supabase URL."))
	checks = append(checks, check("service_role_present", hasServiceRoleKey, "Service role key is present.", "Service role key is required."))

	if apiOK && strings.TrimSpace(config.AnonKey) != "" {
		checks = append(checks, r.probeSupabaseEndpoint("anon_reachability", config.APIURL, config.AnonKey, "Anon API is reachable."))
	} else {
		checks = append(checks, skippedCheck("anon_reachability", "Skipped until Supabase URL and anon key are valid."))
	}

	if apiOK && hasServiceRoleKey {
		keyForProbe := serviceRoleKey
		if keyForProbe == "" {
			keyForProbe, _ = r.ensureSecretStore().Get(supabaseServiceRoleSecretKey)
		}
		checks = append(checks, r.probeSupabaseEndpoint("service_role_reachability", config.APIURL, keyForProbe, "Service role API is reachable."))
	} else {
		checks = append(checks, skippedCheck("service_role_reachability", "Skipped until Supabase URL and service role key are valid."))
	}

	valid := true
	for _, item := range checks {
		if item.Status == "failed" {
			valid = false
			break
		}
	}

	if apiOK {
		config.ProjectRef = supabaseProjectRefFromHost(apiURL.Host)
	}
	config.Version = 1
	config.Status = "configured"

	return SupabaseValidationResult{
		Valid:             valid,
		ProjectRef:        config.ProjectRef,
		Checks:            checks,
		BrowserSafeConfig: config,
		HasServiceRoleKey: hasServiceRoleKey,
	}, nil
}

func (r *Runner) loadSupabaseWorkspaceConfig() (SupabaseWorkspaceConfig, error) {
	raw, err := os.ReadFile(r.supabaseWorkspaceConfigPath())
	if err != nil {
		return SupabaseWorkspaceConfig{}, err
	}

	var config SupabaseWorkspaceConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return SupabaseWorkspaceConfig{}, err
	}
	return normalizeSupabaseWorkspaceConfig(config), nil
}

func (r *Runner) persistSupabaseWorkspaceConfig(config SupabaseWorkspaceConfig) error {
	path := r.supabaseWorkspaceConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (r *Runner) supabaseWorkspaceConfigPath() string {
	return filepath.Join(r.workspace, ".flowpilot", "settings", "supabase-config.json")
}

func (r *Runner) hasSupabaseServiceRoleKey() (bool, error) {
	value, err := r.ensureSecretStore().Get(supabaseServiceRoleSecretKey)
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(value) != "", nil
}

func (r *Runner) probeSupabaseEndpoint(key string, baseURL string, apiKey string, successMessage string) SupabaseValidationCheck {
	endpoint := strings.TrimRight(baseURL, "/") + "/auth/v1/settings"
	statusCode, body, err := httpRequestFn(context.Background(), http.MethodGet, endpoint, map[string]string{
		"apikey":        apiKey,
		"Authorization": "Bearer " + apiKey,
	}, nil)
	if err != nil {
		return failedCheck(key, fmt.Sprintf("Unable to reach Supabase: %s", err.Error()))
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return failedCheck(key, fmt.Sprintf("Supabase responded with %d: %s", statusCode, strings.TrimSpace(string(body))))
	}
	return passedCheck(key, successMessage)
}

func normalizeSupabaseWorkspaceConfig(config SupabaseWorkspaceConfig) SupabaseWorkspaceConfig {
	config.APIURL = strings.TrimRight(strings.TrimSpace(config.APIURL), "/")
	config.AnonKey = strings.TrimSpace(config.AnonKey)
	config.EdgeFunctionURL = strings.TrimRight(strings.TrimSpace(config.EdgeFunctionURL), "/")
	if config.ProjectRef == "" {
		if parsed, ok := parseHTTPSURL(config.APIURL); ok {
			config.ProjectRef = supabaseProjectRefFromHost(parsed.Host)
		}
	}
	return config
}

func parseHTTPSURL(value string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, false
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return nil, false
	}
	return parsed, true
}

func supabaseProjectRefFromHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimSuffix(host, ".supabase.co")
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func check(key string, ok bool, passed string, failed string) SupabaseValidationCheck {
	if ok {
		return passedCheck(key, passed)
	}
	return failedCheck(key, failed)
}

func passedCheck(key string, message string) SupabaseValidationCheck {
	return SupabaseValidationCheck{Key: key, Status: "passed", Message: message}
}

func failedCheck(key string, message string) SupabaseValidationCheck {
	return SupabaseValidationCheck{Key: key, Status: "failed", Message: message}
}

func skippedCheck(key string, message string) SupabaseValidationCheck {
	return SupabaseValidationCheck{Key: key, Status: "skipped", Message: message}
}

func firstFailedSupabaseCheck(checks []SupabaseValidationCheck) string {
	for _, item := range checks {
		if item.Status == "failed" {
			return item.Message
		}
	}
	return "Supabase configuration validation failed"
}

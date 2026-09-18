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
	"sort"
	"strings"
	"time"
)

const supabaseServiceRoleSecretKey = "supabase:workspace:service-role-key"
const supabaseManagementAPIBaseURL = "https://api.supabase.com"

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

func (r *Runner) ApplySupabaseMigrations(input SupabaseSchemaApplyRequest) (SupabaseSchemaApplyResponse, error) {
	apiURL := strings.TrimSpace(input.APIURL)
	projectRef := strings.TrimSpace(input.ProjectRef)
	accessToken := strings.TrimSpace(input.AccessToken)
	if projectRef == "" {
		if parsed, ok := parseHTTPSURL(apiURL); ok {
			projectRef = supabaseProjectRefFromHost(parsed.Host)
		}
	}
	if projectRef == "" {
		return SupabaseSchemaApplyResponse{}, errors.New("project ref is required to apply repo migrations")
	}
	if accessToken == "" {
		return SupabaseSchemaApplyResponse{}, errors.New("management API access token is required")
	}

	migrations, err := r.loadRepoSupabaseMigrations()
	if err != nil {
		return SupabaseSchemaApplyResponse{}, err
	}
	if len(migrations) == 0 {
		return SupabaseSchemaApplyResponse{}, errors.New("no repo Supabase migrations were found")
	}

	if _, err := runSupabaseManagementQuery(projectRef, accessToken, ensureSupabaseMigrationHistorySQL(), false); err != nil {
		return SupabaseSchemaApplyResponse{}, err
	}

	appliedVersions, err := loadAppliedSupabaseMigrationVersions(projectRef, accessToken)
	if err != nil {
		return SupabaseSchemaApplyResponse{}, err
	}

	result := SupabaseSchemaApplyResponse{
		ProjectRef: projectRef,
		Migrations: make([]SupabaseSchemaMigrationResult, 0, len(migrations)),
	}

	for _, migration := range migrations {
		if _, alreadyApplied := appliedVersions[migration.Version]; alreadyApplied {
			result.SkippedCount++
			result.Migrations = append(result.Migrations, SupabaseSchemaMigrationResult{
				Version: migration.Version,
				Name:    migration.Name,
				Status:  "skipped",
				Message: "already applied remotely",
			})
			continue
		}

		if _, err := runSupabaseManagementQuery(
			projectRef,
			accessToken,
			buildSupabaseMigrationApplyQuery(migration),
			false,
		); err != nil {
			return SupabaseSchemaApplyResponse{}, fmt.Errorf(
				"apply migration %s_%s: %w",
				migration.Version,
				migration.Name,
				err,
			)
		}

		appliedVersions[migration.Version] = struct{}{}
		result.AppliedCount++
		result.Migrations = append(result.Migrations, SupabaseSchemaMigrationResult{
			Version: migration.Version,
			Name:    migration.Name,
			Status:  "applied",
			Message: "migration applied and recorded",
		})
	}

	return result, nil
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

func (r *Runner) LoginSupabaseWithPassword(input SupabasePasswordLoginRequest) (SupabasePasswordLoginResponse, error) {
	config, err := r.loadSupabaseWorkspaceConfig()
	if err != nil {
		return SupabasePasswordLoginResponse{}, err
	}

	email := strings.TrimSpace(input.Email)
	password := strings.TrimSpace(input.Password)
	if email == "" || password == "" {
		return SupabasePasswordLoginResponse{}, errors.New("email and password are required")
	}

	payload, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return SupabasePasswordLoginResponse{}, err
	}

	endpoint := strings.TrimRight(config.APIURL, "/") + "/auth/v1/token?grant_type=password"
	statusCode, responseBody, err := httpRequestFn(context.Background(), http.MethodPost, endpoint, map[string]string{
		"Content-Type":  "application/json",
		"apikey":        config.AnonKey,
		"Authorization": "Bearer " + config.AnonKey,
	}, payload)
	if err != nil {
		return SupabasePasswordLoginResponse{}, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return SupabasePasswordLoginResponse{}, fmt.Errorf("Supabase auth responded with %d: %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		User         struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return SupabasePasswordLoginResponse{}, fmt.Errorf("unable to decode Supabase auth response: %w", err)
	}
	if strings.TrimSpace(result.AccessToken) == "" || strings.TrimSpace(result.RefreshToken) == "" {
		return SupabasePasswordLoginResponse{}, errors.New("Supabase auth response did not include session tokens")
	}
	if strings.TrimSpace(result.User.ID) == "" {
		return SupabasePasswordLoginResponse{}, errors.New("Supabase auth response did not include a user id")
	}

	return SupabasePasswordLoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		UserID:       result.User.ID,
		Email:        result.User.Email,
	}, nil
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
	primaryPath := r.supabaseWorkspaceConfigPath()
	raw, err := os.ReadFile(primaryPath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		if fallbackPath := globalSupabaseConfigPath(); fallbackPath != "" && fallbackPath != primaryPath {
			raw, err = os.ReadFile(fallbackPath)
		}
	}
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
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return err
	}

	// Also mirror to global user settings for cross-workspace standalone usage
	if globalPath := globalSupabaseConfigPath(); globalPath != "" && globalPath != path {
		_ = os.MkdirAll(filepath.Dir(globalPath), 0o755)
		_ = os.WriteFile(globalPath, raw, 0o644)
	}

	return nil
}

func globalSupabaseConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".flowpilot", "settings", "supabase-config.json")
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

type repoSupabaseMigration struct {
	Version string
	Name    string
	Query   string
}

func (r *Runner) loadRepoSupabaseMigrations() ([]repoSupabaseMigration, error) {
	if strings.TrimSpace(r.workspace) == "" {
		return nil, errors.New("runner workspace is not configured")
	}

	migrationsDir := filepath.Join(r.workspace, "supabase", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read repo migrations dir: %w", err)
	}

	migrations := make([]repoSupabaseMigration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		version, name, found := strings.Cut(baseName, "_")
		if !found {
			name = baseName
		}
		version = strings.TrimSpace(version)
		name = strings.TrimSpace(name)
		if version == "" {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		raw, err := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		query := strings.TrimSpace(string(raw))
		if query == "" {
			continue
		}

		migrations = append(migrations, repoSupabaseMigration{
			Version: version,
			Name:    name,
			Query:   query,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	return migrations, nil
}

func ensureSupabaseMigrationHistorySQL() string {
	return strings.TrimSpace(`
create schema if not exists supabase_migrations;
create table if not exists supabase_migrations.schema_migrations (
  version text primary key,
  statements text[],
  name text
);
alter table supabase_migrations.schema_migrations add column if not exists statements text[];
alter table supabase_migrations.schema_migrations add column if not exists name text;
`)
}

func buildSupabaseMigrationApplyQuery(migration repoSupabaseMigration) string {
	return strings.Join([]string{
		"begin;",
		migration.Query,
		fmt.Sprintf(
			"insert into supabase_migrations.schema_migrations(version, name, statements) values (%s, %s, array[%s]) on conflict (version) do nothing;",
			pgDollarQuote(migration.Version, "fp_ver"),
			pgDollarQuote(migration.Name, "fp_name"),
			pgDollarQuote(migration.Query, "fp_sql"),
		),
		"commit;",
	}, "\n")
}

func pgDollarQuote(value string, prefix string) string {
	tag := prefix
	for strings.Contains(value, "$"+tag+"$") {
		tag += "_x"
	}
	return "$" + tag + "$" + value + "$" + tag + "$"
}

func loadAppliedSupabaseMigrationVersions(projectRef string, accessToken string) (map[string]struct{}, error) {
	body, err := runSupabaseManagementQuery(
		projectRef,
		accessToken,
		"select version from supabase_migrations.schema_migrations order by version;",
		true,
	)
	if err != nil {
		return nil, err
	}

	rows, err := parseSupabaseManagementRows(body)
	if err != nil {
		return nil, err
	}

	versions := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		version := strings.TrimSpace(fmt.Sprint(row["version"]))
		if version == "" || version == "<nil>" {
			continue
		}
		versions[version] = struct{}{}
	}
	return versions, nil
}

func runSupabaseManagementQuery(
	projectRef string,
	accessToken string,
	query string,
	readOnly bool,
) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"read_only": readOnly,
	})
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(supabaseManagementAPIBaseURL, "/") +
		"/v1/projects/" + url.PathEscape(projectRef) + "/database/query"
	statusCode, body, err := httpRequestFn(
		context.Background(),
		http.MethodPost,
		endpoint,
		map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer " + accessToken,
			"User-Agent":    "FlowPilot Desktop Schema Init",
		},
		payload,
	)
	if err != nil {
		return nil, err
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("Supabase Management API responded with %d: %s", statusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func parseSupabaseManagementRows(body []byte) ([]map[string]any, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var directRows []map[string]any
	if err := json.Unmarshal(body, &directRows); err == nil {
		return directRows, nil
	}

	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode Supabase query response: %w", err)
	}

	for _, key := range []string{"rows", "result", "data"} {
		rows, ok := envelope[key]
		if !ok {
			continue
		}
		decoded, err := normalizeSupabaseManagementRows(rows)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	return nil, nil
}

func normalizeSupabaseManagementRows(value any) ([]map[string]any, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("Supabase query response rows were not an array")
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("Supabase query response row was not an object")
		}
		rows = append(rows, row)
	}
	return rows, nil
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

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupabaseWorkspaceConfigSaveLoadAndReset(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	restoreHTTPProbe(t)

	saved, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co/",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1/",
		},
		ServiceRoleKey: "service-role",
	})
	if err != nil {
		t.Fatalf("SaveSupabaseWorkspaceConfig() failed: %v", err)
	}
	if !saved.HasServiceRoleKey {
		t.Fatal("expected service-role key flag")
	}
	if saved.ServiceRoleKey != "" {
		t.Fatal("service-role key must not be returned in browser-safe save response")
	}
	if saved.ProjectRef != "demo-ref" {
		t.Fatalf("expected derived project ref, got %q", saved.ProjectRef)
	}

	raw, err := os.ReadFile(filepath.Join(instance.workspace, ".flowpilot", "settings", "supabase-config.json"))
	if err != nil {
		t.Fatalf("config was not written: %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("invalid persisted JSON: %v", err)
	}
	if _, ok := persisted["serviceRoleKey"]; ok {
		t.Fatal("service-role key must not be persisted in JSON")
	}

	loaded, err := instance.LoadSupabaseWorkspaceConfig()
	if err != nil {
		t.Fatalf("LoadSupabaseWorkspaceConfig() failed: %v", err)
	}
	if loaded.APIURL != "https://demo-ref.supabase.co" || loaded.EdgeFunctionURL != "https://demo-ref.supabase.co/functions/v1" {
		t.Fatalf("unexpected normalized config: %#v", loaded)
	}
	if !loaded.HasServiceRoleKey || loaded.ServiceRoleKey != "" {
		t.Fatalf("unexpected loaded secret metadata: %#v", loaded)
	}

	withSecret, err := instance.LoadSupabaseWorkspaceConfigWithSecret()
	if err != nil {
		t.Fatalf("LoadSupabaseWorkspaceConfigWithSecret() failed: %v", err)
	}
	if withSecret.ServiceRoleKey != "service-role" {
		t.Fatalf("expected secret for server resolver, got %q", withSecret.ServiceRoleKey)
	}

	if err := instance.ResetSupabaseWorkspaceConfig(); err != nil {
		t.Fatalf("ResetSupabaseWorkspaceConfig() failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(instance.workspace, ".flowpilot", "settings", "supabase-config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected config file removed, got err=%v", err)
	}
	hasSecret, err := instance.hasSupabaseServiceRoleKey()
	if err != nil {
		t.Fatalf("hasSupabaseServiceRoleKey() failed: %v", err)
	}
	if hasSecret {
		t.Fatal("expected reset to clear service-role secret")
	}
}

func TestSupabaseWorkspaceConfigValidationAndBlankSecretSemantics(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	restoreHTTPProbe(t)

	result, err := instance.ValidateSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "http://bad.supabase.co",
			AnonKey:         "",
			EdgeFunctionURL: "https://other.supabase.co/functions/v1",
		},
	})
	if err != nil {
		t.Fatalf("ValidateSupabaseWorkspaceConfig() failed: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if !hasFailedCheck(result.Checks, "api_url_format") || !hasFailedCheck(result.Checks, "service_role_present") {
		t.Fatalf("expected field-level failures, got %#v", result.Checks)
	}

	if _, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
	}); err == nil {
		t.Fatal("expected save without service-role key to fail when no secret exists")
	}

	if _, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
		ServiceRoleKey: "service-role",
	}); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	if _, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key-2",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
	}); err != nil {
		t.Fatalf("blank service-role key should keep existing secret: %v", err)
	}
}

func TestSupabaseWorkspaceConfigSecretFailureDoesNotWriteConfig(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: failingSetSecretStore{}}
	restoreHTTPProbe(t)

	_, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
		ServiceRoleKey: "service-role",
	})
	if err == nil {
		t.Fatal("expected secret-store failure")
	}
	if _, statErr := os.Stat(filepath.Join(instance.workspace, ".flowpilot", "settings", "supabase-config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no config file after secret-store failure, got err=%v", statErr)
	}
}

func TestSupabaseWorkspaceConfigPersistFailureRollsBackSecret(t *testing.T) {
	secretStore := newMemorySecretStore()
	instance := &Runner{workspace: t.TempDir(), secretStore: secretStore}
	restoreHTTPProbe(t)

	if err := os.WriteFile(filepath.Join(instance.workspace, ".flowpilot"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("failed to create persistence blocker: %v", err)
	}

	_, err := instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
		ServiceRoleKey: "service-role",
	})
	if err == nil {
		t.Fatal("expected config persistence failure")
	}
	if _, getErr := secretStore.Get(supabaseServiceRoleSecretKey); getErr == nil {
		t.Fatal("expected new service-role secret to be rolled back")
	}

	secretStore.values[supabaseServiceRoleSecretKey] = "previous-service-role"
	_, err = instance.SaveSupabaseWorkspaceConfig(SupabaseWorkspaceConfigRequest{
		SupabaseWorkspaceConfig: SupabaseWorkspaceConfig{
			APIURL:          "https://demo-ref.supabase.co",
			AnonKey:         "anon-key",
			EdgeFunctionURL: "https://demo-ref.supabase.co/functions/v1",
		},
		ServiceRoleKey: "replacement-service-role",
	})
	if err == nil {
		t.Fatal("expected config persistence failure with existing secret")
	}
	restored, getErr := secretStore.Get(supabaseServiceRoleSecretKey)
	if getErr != nil {
		t.Fatalf("expected previous secret to be restored: %v", getErr)
	}
	if restored != "previous-service-role" {
		t.Fatalf("expected previous secret restored, got %q", restored)
	}
}

func TestApplySupabaseMigrationsSkipsAppliedVersionsAndAppliesRemaining(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace}
	migrationsDir := filepath.Join(workspace, "supabase", "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "20260515050000_admin_mvp_skeleton.sql"), []byte("create table if not exists public.projects(id uuid primary key);"), 0o644); err != nil {
		t.Fatalf("write migration 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(migrationsDir, "20260519070000_projects_uuid_baseline.sql"), []byte("alter table public.projects add column if not exists legacy_project_id text;"), 0o644); err != nil {
		t.Fatalf("write migration 2: %v", err)
	}

	original := httpRequestFn
	callCount := 0
	httpRequestFn = func(_ context.Context, method string, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		callCount++
		if method != http.MethodPost {
			return 0, nil, fmt.Errorf("unexpected method %s", method)
		}
		if endpoint != "https://api.supabase.com/v1/projects/demo-ref/database/query" {
			return 0, nil, fmt.Errorf("unexpected endpoint %s", endpoint)
		}
		if got := headers["Authorization"]; got != "Bearer mgmt-token" {
			return 0, nil, fmt.Errorf("unexpected auth header %q", got)
		}
		query := extractSupabaseQuery(t, body)
		switch callCount {
		case 1:
			if !strings.Contains(query, "create schema if not exists supabase_migrations") {
				return 0, nil, fmt.Errorf("expected ensure-history query, got %q", query)
			}
			return http.StatusCreated, []byte(`{}`), nil
		case 2:
			if !strings.Contains(query, "select version from supabase_migrations.schema_migrations") {
				return 0, nil, fmt.Errorf("expected migration-history query, got %q", query)
			}
			return http.StatusCreated, []byte(`[{"version":"20260515050000"}]`), nil
		case 3:
			if !strings.Contains(query, "legacy_project_id") || !strings.Contains(query, "20260519070000") {
				return 0, nil, fmt.Errorf("expected apply query for second migration, got %q", query)
			}
			return http.StatusCreated, []byte(`{}`), nil
		default:
			return 0, nil, fmt.Errorf("unexpected extra management-api call %d", callCount)
		}
	}
	t.Cleanup(func() {
		httpRequestFn = original
	})

	result, err := instance.ApplySupabaseMigrations(SupabaseSchemaApplyRequest{
		APIURL:      "https://demo-ref.supabase.co",
		AccessToken: "mgmt-token",
	})
	if err != nil {
		t.Fatalf("ApplySupabaseMigrations() failed: %v", err)
	}
	if result.ProjectRef != "demo-ref" {
		t.Fatalf("projectRef = %q, want demo-ref", result.ProjectRef)
	}
	if result.AppliedCount != 1 || result.SkippedCount != 1 {
		t.Fatalf("unexpected result counts: %+v", result)
	}
	if len(result.Migrations) != 2 {
		t.Fatalf("migrations len = %d, want 2", len(result.Migrations))
	}
	if result.Migrations[0].Status != "skipped" || result.Migrations[1].Status != "applied" {
		t.Fatalf("unexpected migration statuses: %+v", result.Migrations)
	}
}

type failingSetSecretStore struct{}

func (s failingSetSecretStore) Set(key, value string) error {
	return errors.New("secret store unavailable")
}

func (s failingSetSecretStore) Get(key string) (string, error) {
	return "", errors.New("not found")
}

func (s failingSetSecretStore) Delete(key string) error {
	return nil
}

func restoreHTTPProbe(t *testing.T) {
	t.Helper()
	original := httpRequestFn
	httpRequestFn = func(ctx context.Context, method string, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		return http.StatusOK, []byte(`{}`), nil
	}
	t.Cleanup(func() {
		httpRequestFn = original
	})
}

func hasFailedCheck(checks []SupabaseValidationCheck, key string) bool {
	for _, item := range checks {
		if item.Key == key && item.Status == "failed" {
			return true
		}
	}
	return false
}

func extractSupabaseQuery(t *testing.T, body []byte) string {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	query, _ := payload["query"].(string)
	return query
}

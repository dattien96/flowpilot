package runner

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupOrphanedWorkflowArtifactDirectoriesRemovesRunsMissingFromSupabase(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	projectID := "392355a6-5573-44f1-9aa5-4313502a5816"
	keptRunID := "11111111-1111-1111-1111-111111111111"
	removedRunID := "180cac0e-a709-4f52-9e11-44087dd2d8c5"

	keptRunDir := filepath.Join(instance.artifactRoot(), projectID, keptRunID)
	removedRunDir := filepath.Join(instance.artifactRoot(), projectID, removedRunID)
	localPromptRunDir := filepath.Join(instance.artifactRoot(), "local", "prompt-execution", removedRunID)

	writeWorkflowArtifactFiles(t, filepath.Join(keptRunDir, "business_idea"), keptRunID)
	writeWorkflowArtifactFiles(t, filepath.Join(removedRunDir, "business_idea"), removedRunID)
	writePromptArtifactFiles(t, filepath.Join(localPromptRunDir, "prompt_execution", "artifact-a"), removedRunID)

	t.Setenv("SUPABASE_API_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key")

	originalHTTPRequest := httpRequestFn
	t.Cleanup(func() {
		httpRequestFn = originalHTTPRequest
	})
	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		if method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", method)
		}
		if headers["Authorization"] != "Bearer service-role-key" || headers["apikey"] != "service-role-key" {
			t.Fatal("expected Supabase service role headers")
		}

		parsed, err := url.Parse(endpoint)
		if err != nil {
			t.Fatalf("parse endpoint: %v", err)
		}
		if parsed.Path != "/rest/v1/workflow_runs" {
			t.Fatalf("expected workflow_runs endpoint, got %s", parsed.Path)
		}
		if parsed.Query().Get("project_id") != "eq."+projectID {
			t.Fatalf("expected project filter, got %q", parsed.Query().Get("project_id"))
		}
		if !strings.Contains(parsed.Query().Get("id"), keptRunID) ||
			!strings.Contains(parsed.Query().Get("id"), removedRunID) {
			t.Fatalf("expected both run ids in lookup, got %q", parsed.Query().Get("id"))
		}

		return http.StatusOK, []byte(`[{"id":"` + keptRunID + `"}]`), nil
	}

	result, err := instance.CleanupOrphanedWorkflowArtifactDirectories(context.Background())
	if err != nil {
		t.Fatalf("cleanup orphaned workflow artifacts: %v", err)
	}

	if result.CheckedRunDirectories != 2 {
		t.Fatalf("expected 2 checked workflow run directories, got %d", result.CheckedRunDirectories)
	}
	if result.RemovedRunDirectories != 1 {
		t.Fatalf("expected 1 removed workflow run directory, got %d", result.RemovedRunDirectories)
	}
	if _, err := os.Stat(removedRunDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected removed workflow run directory to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(keptRunDir); err != nil {
		t.Fatalf("expected existing workflow run directory to stay, got err=%v", err)
	}
	if _, err := os.Stat(localPromptRunDir); err != nil {
		t.Fatalf("expected local prompt execution directory to stay, got err=%v", err)
	}
}

func TestCleanupOrphanedWorkflowArtifactDirectoriesRequiresSupabaseConfigBeforeDeleting(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	runDir := filepath.Join(
		instance.artifactRoot(),
		"392355a6-5573-44f1-9aa5-4313502a5816",
		"180cac0e-a709-4f52-9e11-44087dd2d8c5",
	)
	writeWorkflowArtifactFiles(t, filepath.Join(runDir, "business_idea"), "180cac0e-a709-4f52-9e11-44087dd2d8c5")

	t.Setenv("SUPABASE_API_URL", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")

	if _, err := instance.CleanupOrphanedWorkflowArtifactDirectories(context.Background()); err == nil {
		t.Fatal("expected missing Supabase config error")
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("expected workflow run directory to stay when config is missing, got err=%v", err)
	}
}

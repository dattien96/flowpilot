package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEngineInitAndStatusEndpoints(t *testing.T) {
	targetRepo := createGitRepoForEngineTest(t)
	runnerWorkspace := t.TempDir()

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: runnerWorkspace})

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/project-1/engine/init", map[string]any{
		"workingDirectory": targetRepo,
		"trigger":          "manual",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}

	var initResponse EngineStatusResponse
	if err := json.Unmarshal(body, &initResponse); err != nil {
		t.Fatalf("decode init response: %v", err)
	}
	if initResponse.ProjectID != "project-1" {
		t.Fatalf("projectId=%q", initResponse.ProjectID)
	}
	if initResponse.LastInit == nil || initResponse.LastInit.Status != "success" {
		t.Fatalf("lastInit=%+v", initResponse.LastInit)
	}
	if !initResponse.Initialized {
		t.Fatal("expected initialized=true")
	}

	expectFileExists(t, filepath.Join(targetRepo, ".claude", "skills", "git-commit-format", "SKILL.md"))
	expectFileExists(t, filepath.Join(targetRepo, ".agents", "skills", "git-commit-format", "SKILL.md"))
	expectFileExists(t, filepath.Join(targetRepo, ".flowpilot", "tooling.json"))
	expectFileExists(t, filepath.Join(targetRepo, ".flowpilot", "engine-init.json"))
	expectFileExists(t, filepath.Join(targetRepo, ".flowpilot", "ledger", "feature_history.ndjson"))
	expectFileExists(t, filepath.Join(targetRepo, ".flowpilot", "catalog", "features.ndjson"))

	status, body = doJSON(t, http.MethodGet, server.URL+"/client/projects/project-1/engine/status?workingDirectory="+targetRepo, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}

	var statusResponse EngineStatusResponse
	if err := json.Unmarshal(body, &statusResponse); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusResponse.LastInit == nil {
		t.Fatal("expected lastInit in status response")
	}
	if len(statusResponse.Tooling) == 0 {
		t.Fatal("expected tooling entries")
	}
}

func TestGlobalEngineToolingStatusEndpoint(t *testing.T) {
	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: t.TempDir()})

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status, body := doJSON(t, http.MethodGet, server.URL+"/client/engine/tooling/status", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}

	var response EngineGlobalToolingStatusResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode global tooling response: %v", err)
	}
	if len(response.Tooling) != 3 {
		t.Fatalf("expected 3 global tooling entries, got %d", len(response.Tooling))
	}
	for _, tool := range response.Tooling {
		if tool.Tool == "skill_pack" {
			t.Fatalf("global tooling response must not include skill_pack: %+v", response.Tooling)
		}
	}
}

func TestEngineInitAllowsRunnerWorkspace(t *testing.T) {
	runnerWorkspace := createGitRepoForEngineTest(t)

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: runnerWorkspace})

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/project-1/engine/init", map[string]any{
		"workingDirectory": runnerWorkspace,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !containsJSONField(body, "\"projectId\":\"project-1\"") {
		t.Fatalf("unexpected body=%s", body)
	}
}

func TestEngineInitSkipsCurrentBindTrigger(t *testing.T) {
	targetRepo := createGitRepoForEngineTest(t)

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: t.TempDir()})

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	if status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/project-1/engine/init", map[string]any{
		"workingDirectory": targetRepo,
		"trigger":          "manual",
	}, nil); status != http.StatusOK {
		t.Fatalf("manual init status=%d body=%s", status, body)
	}

	status, body := doJSON(t, http.MethodPost, server.URL+"/client/projects/project-1/engine/init", map[string]any{
		"workingDirectory": targetRepo,
		"trigger":          "bind",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}

	var response EngineStatusResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode bind response: %v", err)
	}
	if response.LastInit == nil || !response.LastInit.Skipped {
		t.Fatalf("lastInit=%+v", response.LastInit)
	}
}

func createGitRepoForEngineTest(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.email", "flowpilot@example.com")
	runGit(t, repoDir, "config", "user.name", "FlowPilot")

	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# engine test\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}

	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "init")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func expectFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func containsJSONField(body []byte, needle string) bool {
	return strings.Contains(string(body), needle)
}

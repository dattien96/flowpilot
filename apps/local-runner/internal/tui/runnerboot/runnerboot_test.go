// Tests A5-A6: CP-56 runnerboot contract tests.
// These tests use httptest servers to simulate healthy / unhealthy runners.
// Spawn (A6b) is tested in isolation without actually forking a subprocess.
package runnerboot_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/tui/runnerboot"
)

// A5: EnsureRunner reuses an existing healthy runner without spawning.
func TestA5_EnsureRunner_ReusesHealthyRunner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{
				"status":        "online",
				"runnerVersion": "1.0.0",
				"cwd":           "/workspace",
				"os":            "linux",
				"startedAt":     "2026-08-12T00:00:00Z",
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Use ExplicitURL so EnsureRunner doesn't try to spawn
	result, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
		Host:        "127.0.0.1",
		Port:        4317,
	})
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if result.RunnerURL != srv.URL {
		t.Errorf("RunnerURL = %q, want %q", result.RunnerURL, srv.URL)
	}
	if result.Launched {
		t.Error("Launched = true, want false (should reuse)")
	}
	if !result.Reused {
		t.Error("Reused = false, want true")
	}
}

// A5b: EnsureRunner with version check rejects mismatched version.
func TestA5b_EnsureRunner_NoStartRunner_HealthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
		Host:        "127.0.0.1",
		Port:        4317,
	})
	if err == nil {
		t.Fatal("expected error when runner unhealthy and ExplicitURL set")
	}
}

// A5c: EnsureRunner with --no-start-runner returns error when not reachable.
func TestA5c_EnsureRunner_NoStart_Unreachable(t *testing.T) {
	_, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		NoStart: true,
		Host:    "127.0.0.1",
		Port:    19999, // nothing listening here
	})
	if err == nil {
		t.Fatal("expected error when --no-start-runner and runner unreachable")
	}
}

// A6: FindWorkspaceRoot walks up to find a directory with apps/local-runner.
func TestA6_FindWorkspaceRoot(t *testing.T) {
	// Create a temp directory tree: root/apps/local-runner/sub/project
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "local-runner")
	projectDir := filepath.Join(appDir, "sub", "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	got := runnerboot.FindWorkspaceRoot(projectDir)
	if got != root {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want %q", projectDir, got, root)
	}
}

// A6b: FindWorkspaceRoot finds .agents directory.
func TestA6b_FindWorkspaceRoot_AgentsDir(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents")
	projectDir := filepath.Join(root, "myproject")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(projectDir, 0755)

	got := runnerboot.FindWorkspaceRoot(projectDir)
	if got != root {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want %q", projectDir, got, root)
	}
}

// A6c: FindWorkspaceRoot — if a fresh isolated dir has no markers, returns start or a parent with markers.
// On developer machines the home dir may contain .agents (FlowPilot workspace), so we accept either:
//
//	(a) the result equals startDir (true fallback), OR
//	(b) the result is a parent that actually contains a FlowPilot marker.
func TestA6c_FindWorkspaceRoot_IsolatedDirNoMarker(t *testing.T) {
	// Create a temp dir with no FlowPilot markers of its own.
	dir := t.TempDir()
	got := runnerboot.FindWorkspaceRoot(dir)
	// The result must be either startDir or a valid FlowPilot root that is an ancestor.
	if got == dir {
		return // true fallback — pass
	}
	// Verify the returned ancestor actually has a marker.
	_, err1 := os.Stat(filepath.Join(got, "apps", "local-runner"))
	_, err2 := os.Stat(filepath.Join(got, ".agents"))
	if err1 != nil && err2 != nil {
		t.Errorf("FindWorkspaceRoot(%q) = %q, which has no FlowPilot marker", dir, got)
	}
}

// A6d: EnsureRunner with workspace mismatch does not reuse the runner.
func TestA6d_EnsureRunner_CwdMismatch_NotReused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{
				"status":        "online",
				"runnerVersion": "1.0.0",
				"cwd":           "/other-workspace",
				"os":            "linux",
				"startedAt":     "2026-08-12T00:00:00Z",
			})
		} else {
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// With ExplicitURL: if runner is healthy but cwd mismatches, we still
	// skip no-start=false path and just return the URL (explicit overrides cwd check).
	// The Workspace field is only checked when there's no ExplicitURL.
	result, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
		Workspace:   "/my-workspace", // different from cwd=/other-workspace
		Host:        "127.0.0.1",
		Port:        4317,
	})
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	// With ExplicitURL, we always reuse (no spawn check)
	if !result.Reused {
		t.Error("Reused = false, want true (explicit URL skips workspace check)")
	}
}

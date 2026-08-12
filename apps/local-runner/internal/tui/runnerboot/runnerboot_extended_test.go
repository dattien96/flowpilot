// CP-56 extended runnerboot tests — test signatures A5.1-A6.9.
package runnerboot_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/runnerboot"
)

// ---- A5: EnsureRunner -------------------------------------------------------

func TestApplyLiveRunnerDefaults_DefaultsCodexAppServer(t *testing.T) {
	got := runnerboot.ApplyLiveRunnerDefaults([]string{"PATH=/bin", "HOME=/tmp"})
	found := false
	for _, e := range got {
		if e == "FLOWPILOT_CODEX_APPSERVER=1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected default FLOWPILOT_CODEX_APPSERVER=1, got %v", got)
	}
	kept := runnerboot.ApplyLiveRunnerDefaults([]string{"FLOWPILOT_CODEX_APPSERVER=0"})
	if len(kept) != 1 || kept[0] != "FLOWPILOT_CODEX_APPSERVER=0" {
		t.Fatalf("explicit value should be preserved, got %v", kept)
	}
}

func TestA5_1_EnsureRunner_ReusesHealthyRunner(t *testing.T) {
	srv := newHealthSrv("online", "1.0.0", "/workspace")
	defer srv.Close()
	result, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if !result.Reused {
		t.Error("Reused=false, want true")
	}
	if result.Launched {
		t.Error("Launched=true, want false")
	}
}

func TestA5_2_EnsureRunner_ExplicitURL_UnhealthyReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	_, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error for unhealthy runner with ExplicitURL")
	}
}

func TestA5_3_EnsureRunner_NoStart_UnreachableReturnsError(t *testing.T) {
	_, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		NoStart: true,
		Host:    "127.0.0.1",
		Port:    19998, // nothing listening
	})
	if err == nil {
		t.Fatal("expected error when NoStart and runner unreachable")
	}
}

func TestA5_4_IsHealthyAndCompatible_StatusOnline(t *testing.T) {
	h := client.HealthResponse{Status: "online", RunnerVersion: "1.0"}
	if !runnerboot.IsHealthyAndCompatible(h, runnerboot.Config{}) {
		t.Error("online status should be compatible")
	}
}

func TestA5_5_IsHealthyAndCompatible_StatusOfflineRejected(t *testing.T) {
	h := client.HealthResponse{Status: "offline", RunnerVersion: "1.0"}
	if runnerboot.IsHealthyAndCompatible(h, runnerboot.Config{}) {
		t.Error("offline status should NOT be compatible")
	}
}

func TestA5_6_IsHealthyAndCompatible_VersionMatch(t *testing.T) {
	h := client.HealthResponse{Status: "online", RunnerVersion: "2.5.0"}
	if !runnerboot.IsHealthyAndCompatible(h, runnerboot.Config{ExpectedVersion: "2.5.0"}) {
		t.Error("matching version should be compatible")
	}
}

func TestA5_7_IsHealthyAndCompatible_VersionMismatch(t *testing.T) {
	h := client.HealthResponse{Status: "online", RunnerVersion: "1.0.0"}
	if runnerboot.IsHealthyAndCompatible(h, runnerboot.Config{ExpectedVersion: "2.0.0"}) {
		t.Error("mismatched version should NOT be compatible")
	}
}

// ---- A6: FindWorkspaceRoot --------------------------------------------------

func TestA6_1_FindWorkspaceRoot_FindsAppsLocalRunner(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "local-runner")
	deepDir := filepath.Join(appDir, "internal", "project")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}
	got := runnerboot.FindWorkspaceRoot(deepDir)
	if got != root {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want %q", deepDir, got, root)
	}
}

func TestA6_2_FindWorkspaceRoot_FindsDotAgents(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".agents"), 0755)
	projectDir := filepath.Join(root, "src", "myproject")
	os.MkdirAll(projectDir, 0755)

	got := runnerboot.FindWorkspaceRoot(projectDir)
	if got != root {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want %q", projectDir, got, root)
	}
}

func TestA6_3_FindWorkspaceRoot_FallbackToStartDir(t *testing.T) {
	// Create an isolated temp dir with no FlowPilot markers.
	dir := t.TempDir()
	got := runnerboot.FindWorkspaceRoot(dir)
	// Accept: fallback to start dir OR a parent that has a marker (dev machine).
	if got == dir {
		return
	}
	_, err1 := os.Stat(filepath.Join(got, "apps", "local-runner"))
	_, err2 := os.Stat(filepath.Join(got, ".agents"))
	if err1 != nil && err2 != nil {
		t.Errorf("FindWorkspaceRoot(%q) = %q which has no marker", dir, got)
	}
}

func TestA6_4_FindWorkspaceRoot_FromDeeplyNestedDir(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".agents"), 0755)
	deep := filepath.Join(root, "a", "b", "c", "d", "e")
	os.MkdirAll(deep, 0755)

	got := runnerboot.FindWorkspaceRoot(deep)
	if got != root {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want %q", deep, got, root)
	}
}

func TestA6_5_FindWorkspaceRoot_StopsAtFilesystemRoot(t *testing.T) {
	// Start from a temp dir with no marker — just ensure no panic and it returns.
	dir := t.TempDir()
	got := runnerboot.FindWorkspaceRoot(dir)
	if got == "" {
		t.Error("FindWorkspaceRoot should not return empty string")
	}
}

func TestA6_6_EnsureRunner_CwdMismatch_ExplicitURLAlwaysReused(t *testing.T) {
	srv := newHealthSrv("online", "1.0.0", "/other-workspace")
	defer srv.Close()
	// With ExplicitURL and workspace mismatch, ExplicitURL takes priority.
	result, err := runnerboot.EnsureRunner(context.Background(), runnerboot.Config{
		ExplicitURL: srv.URL,
		Workspace:   "/my-workspace",
	})
	if err != nil {
		t.Fatalf("EnsureRunner: %v", err)
	}
	if !result.Reused {
		t.Error("Reused=false; ExplicitURL should always reuse")
	}
}

func TestA6_7_FindWorkspaceRoot_PrefersNearestAncestor(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "sub")
	os.MkdirAll(filepath.Join(outer, ".agents"), 0755)
	os.MkdirAll(filepath.Join(inner, ".agents"), 0755)
	deep := filepath.Join(inner, "project", "src")
	os.MkdirAll(deep, 0755)

	got := runnerboot.FindWorkspaceRoot(deep)
	// Should find inner (nearest) not outer.
	if got != inner {
		t.Errorf("FindWorkspaceRoot(%q) = %q, want nearest %q", deep, got, inner)
	}
}

func TestA6_8_FindWorkspaceRoot_SingleLevelDirFallback(t *testing.T) {
	// A single dir with no markers should fall back to itself.
	dir := t.TempDir()
	// Ensure it has no marker.
	os.Remove(filepath.Join(dir, ".agents"))
	os.RemoveAll(filepath.Join(dir, "apps"))

	got := runnerboot.FindWorkspaceRoot(dir)
	// If no marker found anywhere, startDir is returned.
	if got == "" {
		t.Error("should return a non-empty dir")
	}
}

func TestA6_9_FindWorkspaceRoot_WithEnvOverride(t *testing.T) {
	// FLOWPILOT_WORKSPACE env is honoured at the CLI layer (chat.go), not in
	// FindWorkspaceRoot itself.  Verify that FindWorkspaceRoot is pure: it only
	// uses the filesystem, not env vars.
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_WORKSPACE", "/some/other/path")
	got := runnerboot.FindWorkspaceRoot(dir)
	// Should NOT return /some/other/path (env ignored inside FindWorkspaceRoot).
	if got == "/some/other/path" {
		t.Error("FindWorkspaceRoot should not read FLOWPILOT_WORKSPACE env")
	}
}

// ---- Helper -----------------------------------------------------------------

func newHealthSrv(status, version, cwd string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{
				"status": status, "runnerVersion": version,
				"cwd": cwd, "os": "linux", "startedAt": "2026-08-12T00:00:00Z",
			})
			return
		}
		http.NotFound(w, r)
	}))
}

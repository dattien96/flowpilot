package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/runner"
)

func TestResolveWorkspaceWalksUpToAgentsRoot(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents: %v", err)
	}

	nested := filepath.Join(root, "apps", "local-runner")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalWD); chdirErr != nil {
			t.Fatalf("restore cwd: %v", chdirErr)
		}
	})

	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir nested: %v", err)
	}

	resolved, err := runner.ResolveWorkspace("")
	if err != nil {
		t.Fatalf("resolve workspace: %v", err)
	}

	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval symlinks root: %v", err)
	}
	if resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
	}
}

func TestWithCORSAllowsDeletePreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/integrations/integration-1/connection", nil)
	req.Header.Set("Origin", "http://localhost:3002")
	recorder := httptest.NewRecorder()

	withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach wrapped handler")
	})).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204 preflight response, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, DELETE, OPTIONS" {
		t.Fatalf("unexpected allow methods header %q", got)
	}
}

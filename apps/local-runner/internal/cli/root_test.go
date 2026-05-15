package cli

import (
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

	if resolved != root {
		t.Fatalf("expected %q, got %q", root, resolved)
	}
}

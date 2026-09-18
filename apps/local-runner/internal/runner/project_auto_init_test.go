package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateProject_AutoInitsEngine(t *testing.T) {
	svc, srv := newTestServer(t)
	svc.AttachRunner(&Runner{workspace: t.TempDir()})
	tmpDir := t.TempDir()

	status, body := doJSON(t, "POST", srv.URL+"/client/projects", CreateProjectInput{
		Name:          "MobileApp",
		DirectoryPath: tmpDir,
		Platform:      "reactjs",
	}, nil)

	if status != 201 {
		t.Fatalf("expected status 201, got %d: %s", status, body)
	}

	// Verify requirements/ folder was scaffolded
	ssFormat := filepath.Join(tmpDir, "requirements", "05-System-Specs", "FORMAT-REFERENCE-SS.md")
	if _, err := os.Stat(ssFormat); err != nil {
		t.Fatalf("expected FORMAT-REFERENCE-SS.md to be scaffolded: %v", err)
	}

	// Verify .flowpilot/settings/gate-config.json was written
	gateConfig := filepath.Join(tmpDir, ".flowpilot", "settings", "gate-config.json")
	if _, err := os.Stat(gateConfig); err != nil {
		t.Fatalf("expected gate-config.json to be written: %v", err)
	}

	// Verify skills were installed
	skillDir := filepath.Join(tmpDir, ".claude", "skills", "git-commit-format")
	if _, err := os.Stat(skillDir); err != nil {
		t.Fatalf("expected skills to be installed: %v", err)
	}
}

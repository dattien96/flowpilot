package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/skillpack"
)

// Re-init on a project whose skills are already at the current pack version
// must not read like a failed install: "skipped" means "already current".
func TestEngineInitReinstallReportsAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	if _, err := skillpack.Install(dir, "golang"); err != nil {
		t.Fatalf("seed Install: %v", err)
	}

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: t.TempDir()})

	_, initState := svc.runEngineInit("project-1", dir, "golang", "manual", "all")
	if initState == nil {
		t.Fatalf("initState = nil")
	}
	if len(initState.Install.InstalledPaths) != 0 {
		t.Fatalf("InstalledPaths = %d, want 0 on a current re-init", len(initState.Install.InstalledPaths))
	}
	if len(initState.Install.SkippedPaths) == 0 {
		t.Fatalf("SkippedPaths empty, want all current files reported")
	}

	step := findEngineStep(initState.Steps, "skillpack_install")
	if step == nil {
		t.Fatalf("skillpack_install step missing")
	}
	if !strings.Contains(step.Detail, "already current") {
		t.Fatalf("skillpack_install detail = %q, want wording that reads as up-to-date (\"already current\")", step.Detail)
	}
}

package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// BUG-423: /standardize must operate on the runner's configured workspace,
// not the process cwd. A service whose runner is attached to workspace A must
// resolve A even when the process cwd is B.
func TestBug423_StandardizeRootComesFromAttachedRunner(t *testing.T) {
	workspace := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(cwd) == filepath.Clean(workspace) {
		t.Fatalf("test precondition: cwd equals workspace")
	}

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})

	if got := svc.standardizeWorkspaceRoot(); filepath.Clean(got) != filepath.Clean(workspace) {
		t.Fatalf("standardizeWorkspaceRoot = %q; want attached runner workspace %q", got, workspace)
	}
}

// BUG-423: an explicit SetStandardizeWorkspaceRoot pin still wins over the
// attached runner workspace (tests rely on pinned sandboxes).
func TestBug423_ExplicitPinStillWins(t *testing.T) {
	workspace := t.TempDir()
	pinned := t.TempDir()

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})
	svc.SetStandardizeWorkspaceRoot(pinned)

	if got := svc.standardizeWorkspaceRoot(); filepath.Clean(got) != filepath.Clean(pinned) {
		t.Fatalf("standardizeWorkspaceRoot = %q; want explicit pin %q", got, pinned)
	}
}

// BUG-423 e2e: ExecuteStandardize scans/writes under the runner workspace's
// requirements/ tree even when the process cwd has none.
func TestBug423_ExecuteStandardizeUsesRunnerWorkspace(t *testing.T) {
	workspace := t.TempDir()
	stubGitNexus(t, gitnexusFixtureJSON, nil)

	writeSandboxFile(t, workspace, "requirements/05-System-Specs/SS-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, workspace, "requirements/06-System-Tech-Design/SD-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, workspace, "features/auth/login.go", "package auth\n")

	// Safety net: before the fix this test runs the reverse-doc branch against
	// the process cwd and writes drafts into <cwd>/requirements — remove them.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwdReq := filepath.Join(cwd, "requirements")
	if _, err := os.Stat(cwdReq); os.IsNotExist(err) {
		t.Cleanup(func() { _ = os.RemoveAll(cwdReq) })
	}

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: workspace})

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/auth"})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.Mode != StandardizeModeConformance {
		t.Fatalf("Mode = %q; want conformance — the workspace's existing docs were ignored (wrong root)", res.Mode)
	}
	if res.ScanReport == nil || res.ScanReport.TotalFilesScanned != 2 {
		t.Fatalf("ScanReport = %+v; want 2 files scanned under the workspace requirements tree", res.ScanReport)
	}
}

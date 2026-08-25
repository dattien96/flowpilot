package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// CA-638: the runner auto-runs `gitnexus analyze` against a target project
// when it has no .gitnexus index yet — once per process per workspace — so
// scope-drift HighSeverity (B14) and source.dependence (B12) get real
// dependents without the operator indexing manually. Additive file.
//
// The actual `gitnexus analyze` spawn is covered with a stub binary on PATH;
// provider-agnostic (fires on run creation regardless of provider).

func ca638TestService() *InteractiveService {
	return &InteractiveService{
		mu:                  sync.Mutex{},
		gitnexusAnalyzeOnce: map[string]bool{},
	}
}

func ca638GitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

// ca638StubGitNexus installs a fake `gitnexus` on PATH: --version prints a
// version (so tooling.CheckTool passes) and analyze writes the marker file.
func ca638StubGitNexus(t *testing.T, marker string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  --version) echo 1.4.8 ;;\n  analyze) echo ran >> \"$MARKER\" ;;\n  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gitnexus"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARKER", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func ca638WaitMarker(t *testing.T, marker string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("gitnexus analyze was not invoked (marker %s missing)", marker)
}

func TestEnsureGitNexusIndex_AutoAnalyzesOnce(t *testing.T) {
	dir := ca638GitRepo(t)
	marker := filepath.Join(t.TempDir(), "analyzed.marker")
	ca638StubGitNexus(t, marker)

	svc := ca638TestService()
	svc.ensureGitNexusIndexAsync(dir)
	svc.ensureGitNexusIndexAsync(dir) // second call must be a no-op

	ca638WaitMarker(t, marker)
	b, _ := os.ReadFile(marker)
	if got := strings.Count(string(b), "ran"); got != 1 {
		t.Fatalf("analyze ran %d times, want exactly 1", got)
	}
}

func TestEnsureGitNexusIndex_AlreadyIndexedNoAnalyze(t *testing.T) {
	dir := ca638GitRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".gitnexus"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "never.marker")
	ca638StubGitNexus(t, marker)

	svc := ca638TestService()
	svc.ensureGitNexusIndexAsync(dir)
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("must not analyze when .gitnexus already exists")
	}
}

func TestEnsureGitNexusIndex_NotGitRepoNoAnalyze(t *testing.T) {
	dir := t.TempDir() // no .git
	marker := filepath.Join(t.TempDir(), "never.marker")
	ca638StubGitNexus(t, marker)

	svc := ca638TestService()
	svc.ensureGitNexusIndexAsync(dir)
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("must not analyze a non-git workspace")
	}
}

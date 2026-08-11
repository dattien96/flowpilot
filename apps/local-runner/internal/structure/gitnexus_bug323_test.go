package structure

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoNameFromDirUsesBasename(t *testing.T) {
	if got := repoNameFromDir(`C:\working\flowpilot`); got != "flowpilot" {
		t.Fatalf("got %q, want flowpilot", got)
	}
	if got := repoNameFromDir("/home/dev/flowpilot"); got != "flowpilot" {
		t.Fatalf("got %q, want flowpilot", got)
	}
}

func TestGitNexusResponseErrorExtractsTopLevelError(t *testing.T) {
	msg := gitNexusResponseError(`{"error":"Target 'foo' not found"}`)
	if msg != "Target 'foo' not found" {
		t.Fatalf("got %q", msg)
	}
}

func TestGitNexusResponseErrorEmptyForSuccessPayload(t *testing.T) {
	msg := gitNexusResponseError(`{"impactedCount":0,"affected_processes":[]}`)
	if msg != "" {
		t.Fatalf("expected no error message, got %q", msg)
	}
}

func TestTryParseGitNexusImpactV2RealSchema(t *testing.T) {
	input := `{
	  "impactedCount": 4,
	  "affected_processes": [
	    {"name": "runFlowGateAtEpoch"},
	    {"name": "runChildArtifactOutputGateAtEpoch"}
	  ],
	  "affected_modules": [
	    {"name": "Runner"},
	    {"name": "Flowgate"}
	  ],
	  "byDepth": {
	    "1": [{"name": "prepareChangeContract"}],
	    "2": [{"name": "captureChangeContract"}]
	  }
	}`

	summary, ok := tryParseGitNexusImpactV2(input)
	if !ok {
		t.Fatal("expected v2 parser to accept real CLI schema")
	}
	if summary.Count != 4 {
		t.Fatalf("Count: got %d, want 4", summary.Count)
	}
	if len(summary.Flows) != 2 {
		t.Fatalf("Flows: got %v", summary.Flows)
	}
	if !containsString(summary.Nearest, "prepareChangeContract") {
		t.Fatalf("Nearest missing prepareChangeContract: %v", summary.Nearest)
	}
	if !containsString(summary.Nearest, "Runner") {
		t.Fatalf("Nearest missing Runner: %v", summary.Nearest)
	}
}

func TestTryParseGitNexusImpactV2ZeroDependentsIsLegitimateEmpty(t *testing.T) {
	input := `{"impactedCount":0,"affected_processes":[],"affected_modules":[],"byDepth":{}}`
	summary, ok := tryParseGitNexusImpactV2(input)
	if !ok {
		t.Fatal("expected zero-hit v2 payload to parse")
	}
	if summary.Count != 0 {
		t.Fatalf("Count: got %d, want 0", summary.Count)
	}
	if len(summary.Nearest) != 0 {
		t.Fatalf("Nearest: got %v", summary.Nearest)
	}
}

func TestParseGitNexusOutputPrefersV2OverLegacyEmptyDecode(t *testing.T) {
	input := `{"impactedCount":2,"affected_processes":[{"name":"proc-a"}],"byDepth":{"1":[{"name":"sym-a"}]}}`
	summary := parseGitNexusOutput(input)
	if summary.Count != 2 {
		t.Fatalf("Count: got %d, want 2", summary.Count)
	}
	if !containsString(summary.Flows, "proc-a") {
		t.Fatalf("Flows: got %v", summary.Flows)
	}
}

func TestGitNexusDependentsReturnsErrorForMissingTarget(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	repoRoot := findRepoRoot(t)
	gp := &gitNexusProvider{repoDir: repoRoot}
	_, err := gp.Dependents(context.Background(), "NonExistentSymbolXYZ")
	if err == nil {
		t.Fatal("expected error for missing target")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitNexusDependentsSmokeScopeDiff(t *testing.T) {
	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not available")
	}

	repoRoot := findRepoRoot(t)
	gp := &gitNexusProvider{repoDir: repoRoot}
	summary, err := gp.Dependents(context.Background(), "ScopeDiff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Count == 0 {
		t.Fatalf("expected non-zero blast radius for ScopeDiff, got %+v", summary)
	}
	if len(summary.Flows) == 0 {
		t.Fatalf("expected affected process names, got %+v", summary)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Skip("no working directory")
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			cmd := exec.Command("git", "rev-parse", "--show-toplevel")
			cmd.Dir = dir
			if out, gitErr := cmd.Output(); gitErr == nil {
				return strings.TrimSpace(string(out))
			}
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skip("could not locate git repo root for gitnexus smoke test")
	return ""
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

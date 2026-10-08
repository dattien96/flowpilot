package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// BUG-644 (live run-523131, contract c86945a3): the sprint-1 freeze note
// claimed core/vault-core/src/main/cpp/CMakeLists.txt was declared, but the
// stored contract JSON omitted it — NormalizeDeclaredCodePaths silently
// drops any path IsConcreteCodeTarget rejects, and CMakeLists.txt (.txt)
// classified as doc/audit. Prose record and machine contract diverged; every
// downstream gate evaluated the JSON while humans/adjudications read prose.

// Build files are code-adjacent build config — a legitimately declared
// build file must survive normalization into the stored contract.
func TestBug644_CMakeListsSurvivesFreezeNormalization(t *testing.T) {
	dir := t.TempDir()
	draft := changecontract.PreflightContractDraft{
		FeatureKey: "vault-core",
		Intent:     "split the panic nuke engine",
		DeclaredPaths: []string{
			"core/vault-core/src/main/cpp/CMakeLists.txt",
			"core/vault-core/src/main/cpp/src/Engine.cpp",
		},
	}
	rec, err := changecontract.FreezeContract(dir, "run-644", "planner", "coder", draft, "", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("FreezeContract: %v", err)
	}
	for _, want := range draft.DeclaredPaths {
		found := false
		for _, got := range rec.DeclaredPaths {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("declared path %q dropped from the stored contract — prose claims it, JSON omits it (the live divergence)", want)
		}
	}
}

// Genuinely non-code draft entries (docs, audit files) still drop — but the
// drop must be auditable: droppedFreezePaths names every prose-claimed path
// that did not land in the contract.
func TestBug644_DroppedDraftPathsAreAudited(t *testing.T) {
	dir := t.TempDir()
	draft := []string{
		"core/vault-core/src/Engine.cpp",   // concrete code — stored
		"change-audit/CA-156.md",           // audit doc — dropped
		"core/vault-core/CMakeLists.txt",   // build config — stored after the fix
		"core/vault-core/src/Engine.cpp",   // duplicate of the stored path — not a drop
	}
	stored, err := changecontract.NormalizeDeclaredCodePaths(dir, draft)
	if err != nil {
		t.Fatalf("NormalizeDeclaredCodePaths: %v", err)
	}
	dropped := droppedFreezePaths(dir, draft, stored)
	joined := strings.Join(dropped, ",")
	if strings.Contains(joined, "CMakeLists.txt") {
		t.Fatalf("CMakeLists.txt flagged as dropped — it must normalize into scope, got dropped=%v", dropped)
	}
	if !strings.Contains(joined, "CA-156.md") {
		t.Fatalf("audit doc not reported as dropped — the divergence stays silent: %v", dropped)
	}
	// A duplicate entry is a dedupe, not a claimed-but-dropped path.
	count := 0
	for _, d := range dropped {
		if d == "core/vault-core/src/Engine.cpp" {
			count++
		}
	}
	if count != 0 {
		t.Fatalf("stored path misreported as dropped %d time(s): %v", count, dropped)
	}
}

// The diag event is the human/audit record — freeze_summary_mismatch must be
// written to the run's flow diag stream when the drop set is non-empty.
func TestBug644_MismatchEventWrittenToDiagLog(t *testing.T) {
	diagDir := t.TempDir()
	t.Setenv("FLOWPILOT_FLOW_DIAG_DIR", diagDir)
	svc, runID := clusterFService(t)
	emitFreezeSummaryMismatch(svc, runID, "freeze", "c-abc", []string{"change-audit/CA-156.md"})
	emitFreezeSummaryMismatch(svc, runID, "freeze", "c-abc", nil) // no-op, must write nothing extra

	data, err := os.ReadFile(filepath.Join(diagDir, runID+".ndjson"))
	if err != nil {
		t.Fatalf("diag file not written: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "freeze_summary_mismatch") {
		t.Fatalf("freeze_summary_mismatch event missing from diag:\n%s", body)
	}
	if !strings.Contains(body, "CA-156.md") {
		t.Fatalf("dropped path not named in the event:\n%s", body)
	}
	if strings.Count(body, "freeze_summary_mismatch") != 1 {
		t.Fatalf("event must fire once per non-empty drop set, got:\n%s", body)
	}
}

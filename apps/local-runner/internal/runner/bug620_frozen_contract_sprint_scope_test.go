package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// BUG-620 (live run-150388): vibe sprints reuse the same node ids, so a
// later sprint's freeze lands as a NEWER version for the same
// (runID, "coder") step — e.g. sprint-2's legit Task-032 freeze vs the
// fabricated sprint-3 preflight's Task-033 freeze at 11:25Z. Every consumer
// of "the frozen contract" resolved the newest record and sprint 2's legs
// read — and contracted — the wrong task.
//
// Rule: inside vibe-sprint topology, contract resolution prefers the active
// record whose source_doc_id matches the sprint's own task doc
// (vibeTaskPlan[vibeSprintIndex-1]). Non-sprint callers and sprints whose
// own doc has no record keep the existing newest-active behavior.

func saveFrozenRecordForDoc(t *testing.T, dir, runID, stepID, docID string, version int, at time.Time, paths ...string) {
	t.Helper()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatalf("NewFrozenStore: %v", err)
	}
	rec := changecontract.FrozenContractRecord{
		ContractID:    runID + ":" + stepID + ":v" + docID,
		Version:       version,
		RunID:         runID,
		CoderStepID:   stepID,
		FeatureKey:    "crypto-ndk",
		Intent:        "sprint contract",
		SourceDocID:   docID,
		DeclaredPaths: paths,
		DeclaredAt:    at,
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatalf("SaveFrozen(%s): %v", docID, err)
	}
}

func TestBUG620_LatestContractPrefersSprintTaskDoc(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()

	// Sprint 2 legit freeze (Task-032), then the fabricated sprint-3 preflight
	// froze Task-033 on top of the same "coder" step — newest wins by version.
	saveFrozenRecordForDoc(t, dir, "run-s", "coder", "Task-032", 1, now, "vault_container_032.go")
	saveFrozenRecordForDoc(t, dir, "run-s", "coder", "Task-033", 2, now.Add(time.Hour), "key_guard_033.go")

	c, ok := latestContractForRunScoped(dir, "run-s", "Task-032")
	if !ok {
		t.Fatal("scoped resolution must find the sprint's own contract")
	}
	if len(c.DeclaredPaths) != 1 || c.DeclaredPaths[0] != "vault_container_032.go" {
		t.Fatalf("scoped contract paths=%v want [vault_container_032.go] — resolution followed ledger head", c.DeclaredPaths)
	}

	// Back-compat: an unscoped caller still resolves the newest record.
	c2, ok := latestContractForRun(dir, "run-s")
	if !ok || len(c2.DeclaredPaths) != 1 || c2.DeclaredPaths[0] != "key_guard_033.go" {
		t.Fatalf("unscoped resolution must keep newest-active semantics, got %+v ok=%v", c2.DeclaredPaths, ok)
	}
}

func TestBUG620_FrozenContractForRunPrefersSprintTaskDoc(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armVibeSprintRun(t, svc)
	dir := t.TempDir()
	now := time.Now().UTC()

	saveFrozenRecordForDoc(t, dir, runID, "coder", "Task-032", 1, now, "vault_container_032.go")
	saveFrozenRecordForDoc(t, dir, runID, "coder", "Task-033", 2, now.Add(time.Hour), "key_guard_033.go")

	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeTaskPlan = []string{
		"requirements/08-Task/done/Task-031.md",
		"requirements/08-Task/todo/Task-032.md",
		"requirements/08-Task/todo/Task-033.md",
	}
	rs.vibeSprintIndex = 2
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	rec, ok := svc.frozenContractForRun(dir, runID)
	if !ok {
		t.Fatal("sprint-scoped resolution must find a contract")
	}
	if rec.SourceDocID != "Task-032" {
		t.Fatalf("frozenContractForRun returned %q — sprint 2 must resolve its own task doc, want Task-032", rec.SourceDocID)
	}
	if len(rec.DeclaredPaths) != 1 || rec.DeclaredPaths[0] != "vault_container_032.go" {
		t.Fatalf("resolved paths=%v want Task-032's scope", rec.DeclaredPaths)
	}
}

// A sprint whose own task doc has no frozen record falls back to the newest
// active record — fail-open for sprint-1 era runs frozen before the doc was
// stamped (or replayed state where the plan is not yet loaded).
func TestBUG620_NoSprintDocMatchFallsBackToNewest(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	saveFrozenRecordForDoc(t, dir, "run-x", "coder", "Task-033", 1, now, "only_033.go")

	c, ok := latestContractForRunScoped(dir, "run-x", "Task-032")
	if !ok || len(c.DeclaredPaths) != 1 || c.DeclaredPaths[0] != "only_033.go" {
		t.Fatalf("no doc match must fall back to newest active, got %+v ok=%v", c.DeclaredPaths, ok)
	}
}

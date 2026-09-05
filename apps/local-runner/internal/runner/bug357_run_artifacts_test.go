package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-357: flow doc-writer/coder completion records per-run file_artifact
// OUTPUT instances so the artifacts panel/API surfaces what was written.

// Builder shapes one deterministic record per path.
func TestBuildFlowChildArtifactRecords_SetsFields(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	arts := buildFlowChildArtifactRecords("run-548341", "plan_writer",
		[]string{"requirements/Task-910-calc-core-lcm.md", " ", "requirements/Task-910-calc-core-lcm.md"}, now)
	if len(arts) != 1 {
		t.Fatalf("want 1 record (blank skipped, dup deduped), got %d: %+v", len(arts), arts)
	}
	a := arts[0]
	if a.ID != "run-548341:artifact:plan_writer:0" {
		t.Fatalf("ID = %q", a.ID)
	}
	if a.RunID != "run-548341" || a.Kind != runArtifactKindFile {
		t.Fatalf("RunID/Kind = %q/%q", a.RunID, a.Kind)
	}
	if a.Name != "Task-910-calc-core-lcm.md" {
		t.Fatalf("Name = %q", a.Name)
	}
	if a.Path != "requirements/Task-910-calc-core-lcm.md" || a.NodeID != "plan_writer" {
		t.Fatalf("Path/NodeID = %q/%q", a.Path, a.NodeID)
	}
	if a.Preview == "" || a.CreatedAt == "" {
		t.Fatalf("Preview/CreatedAt must be set: %+v", a)
	}
	// Deterministic IDs must not collide with finalizer (run:turn:kind) or
	// fake (run-final/run-diff) id shapes.
	for _, bad := range []string{":final", ":diff", ":summary", ":rag", "run-final", "run-diff"} {
		if strings.Contains(a.ID, bad) {
			t.Fatalf("ID %q collides with legacy shape %q", a.ID, bad)
		}
	}
}

func TestBuildFlowChildArtifactRecords_Empty(t *testing.T) {
	if arts := buildFlowChildArtifactRecords("r", "n", nil, time.Now()); len(arts) != 0 {
		t.Fatalf("want nil, got %+v", arts)
	}
}

// Store upserts by ID and returns copies.
func TestRunArtifactStore_RecordUpsertsAndCopies(t *testing.T) {
	st := newRunArtifactStore()
	st.record("run-1", []Artifact{{ID: "a", RunID: "run-1", Name: "one.md"}})
	st.record("run-1", []Artifact{{ID: "a", RunID: "run-1", Name: "one-v2.md"}, {ID: "b", Name: "two.md"}})
	got, ok := st.forRun("run-1")
	if !ok || len(got) != 2 {
		t.Fatalf("want 2 records, got %+v ok=%v", got, ok)
	}
	if got[0].Name != "one-v2.md" || got[1].RunID != "run-1" {
		t.Fatalf("upsert/fill broken: %+v", got)
	}
	got[0].Name = "mutated"
	again, _ := st.forRun("run-1")
	if again[0].Name != "one-v2.md" {
		t.Fatalf("forRun must return a copy: %+v", again)
	}
	if _, ok := st.forRun("run-unknown"); ok {
		t.Fatal("unknown run must report false")
	}
	var nilStore *runArtifactStore
	nilStore.record("r", []Artifact{{ID: "x"}})
	if _, ok := nilStore.forRun("r"); ok {
		t.Fatal("nil store must be safe")
	}
}

// Only files actually present on disk are recorded (gate-identical
// existence semantics; escapes and missing files are dropped).
func TestRecordFlowChildArtifacts_OnlyExistingFiles(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "requirements"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join("requirements", "Task-910-calc-core-lcm.md")
	if err := os.WriteFile(filepath.Join(cwd, real), []byte("# plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{runArtifacts: newRunArtifactStore()}
	svc.recordFlowChildArtifacts("run-548341", "plan_writer", cwd,
		[]string{real, "requirements/missing.md", "../escape.md", ""})
	got, ok := svc.runArtifacts.forRun("run-548341")
	if !ok || len(got) != 1 {
		t.Fatalf("want exactly the existing file, got %+v ok=%v", got, ok)
	}
	if got[0].Path != real || got[0].NodeID != "plan_writer" || got[0].Kind != runArtifactKindFile {
		t.Fatalf("record wrong: %+v", got[0])
	}
}

// Snapshot resolves the live parent topology node (label == node id) and
// merges plain + structured required OUTPUT paths.
func TestSnapshotFlowChildArtifactPathsLocked_ResolvesNodeOutputs(t *testing.T) {
	svc := &InteractiveService{runs: map[string]*interactiveRun{}}
	svc.runs["run-parent"] = &interactiveRun{
		id: "run-parent",
		activeFlowNodes: []agentpack.FlowNode{
			{
				ID: "plan_writer",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true,
						ConfigJSON: map[string]any{"paths": []any{"docs/plan.md"}}},
					{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: false,
						ConfigJSON: map[string]any{"paths": []any{"docs/optional.md"}}},
					{Direction: "input", ArtifactTypeID: ArtifactTypeFile, Required: true,
						ConfigJSON: map[string]any{"paths": []any{"docs/input.md"}}},
				},
			},
		},
	}
	child := &interactiveRun{id: "run-child", label: "plan_writer", parentRunID: "run-parent", workspaceCwd: "/tmp/wd"}
	runID, nodeID, cwd, paths := svc.snapshotFlowChildArtifactPathsLocked(child)
	if runID != "run-parent" || nodeID != "plan_writer" || cwd != "/tmp/wd" {
		t.Fatalf("routing keys = %q/%q/%q", runID, nodeID, cwd)
	}
	if len(paths) != 1 || paths[0] != "docs/plan.md" {
		t.Fatalf("want only the required output path, got %v", paths)
	}
	// Unknown label → no paths (no recording), keys still routed.
	child.label = "no-such-node"
	_, _, _, paths = svc.snapshotFlowChildArtifactPathsLocked(child)
	if len(paths) != 0 {
		t.Fatalf("unknown node must yield no paths, got %v", paths)
	}
}

// Handler merges recorded instances ahead of finalizer rows; the fake
// catalog only serves when both are absent.
func TestHandleListArtifacts_MergesRecordedBeforeFinalizer(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := "run-357"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.runArtifacts.record(runID, []Artifact{
		{ID: runID + ":artifact:plan_writer:0", RunID: runID, Kind: runArtifactKindFile,
			Name: "Task-910-calc-core-lcm.md", Path: "requirements/Task-910-calc-core-lcm.md",
			NodeID: "plan_writer", CreatedAt: now},
	})
	if err := svc.finalizer.Finalize(finalizeInput{RunID: runID, TurnID: "t1", FinalMessage: "done"}); err != nil {
		t.Fatal(err)
	}
	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+runID+"/artifacts", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var arts []Artifact
	if err := json.Unmarshal(body, &arts); err != nil {
		t.Fatal(err)
	}
	if len(arts) != 5 {
		t.Fatalf("want 1 recorded + 4 finalizer rows, got %d: %+v", len(arts), arts)
	}
	if arts[0].Kind != runArtifactKindFile || arts[0].Path == "" || arts[0].NodeID != "plan_writer" {
		t.Fatalf("recorded instance must come first with path+node: %+v", arts[0])
	}
}

package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func ca793Write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCA793_CommitRequiresExistingFile(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	dir := t.TempDir()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	rs.vibeLockedSS = "requirements/05-System-Specs/SS-18-x.md"
	svc.mu.Unlock()

	if svc.tryCommitVibeCheckpoint(parent.RunID, vibeSSLockNodeID) {
		t.Fatal("must not commit history when SS file is missing")
	}
	ca793Write(t, dir, "requirements/05-System-Specs/SS-18-x.md", "# SS\n")
	if !svc.tryCommitVibeCheckpoint(parent.RunID, vibeSSLockNodeID) {
		t.Fatal("commit after file exists")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.vibeCheckpointNode != vibeSSLockNodeID {
		t.Fatalf("node=%q", rs.vibeCheckpointNode)
	}
}

func TestCA793_EmptyFileDoesNotCommit(t *testing.T) {
	dir := t.TempDir()
	ca793Write(t, dir, "requirements/07-Coding-Plan/todo/CP-60-x.md", "")
	if vibeWorkspaceFileExists(dir, "requirements/07-Coding-Plan/todo/CP-60-x.md") {
		t.Fatal("zero-byte file must not count as exist")
	}
}

func TestCA793_DeleteCPDemotesToSS(t *testing.T) {
	dir := t.TempDir()
	ss := "requirements/05-System-Specs/SS-18-x.md"
	cp := "requirements/07-Coding-Plan/todo/CP-60-x.md"
	ca793Write(t, dir, ss, "# SS\n")
	ca793Write(t, dir, cp, "# CP\n")
	rs := &interactiveRun{workingMode: workingmode.Vibe, workspaceCwd: dir, vibeLockedSS: ss, vibeLockedCP: cp}
	node, arts := demoteVibeCheckpoint(dir, vibeCpWriterNodeID, []string{cp}, rs)
	if node != vibeCpWriterNodeID {
		t.Fatalf("with CP present node=%q", node)
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(cp))); err != nil {
		t.Fatal(err)
	}
	node, arts = demoteVibeCheckpoint(dir, vibeCpWriterNodeID, []string{cp}, rs)
	if node != vibeSSLockNodeID {
		t.Fatalf("after CP delete node=%q want ss_lock arts=%v", node, arts)
	}
}

func TestCA793_DeleteAllClearsCheckpoint(t *testing.T) {
	dir := t.TempDir()
	ss := "requirements/05-System-Specs/SS-18-x.md"
	ca793Write(t, dir, ss, "# SS\n")
	rs := &interactiveRun{
		workingMode: workingmode.Vibe, workspaceCwd: dir,
		vibeLockedSS: ss, vibeCheckpointNode: vibeSSLockNodeID,
		vibeCheckpointArtifacts: []string{ss},
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(ss))); err != nil {
		t.Fatal(err)
	}
	applyVibeCheckpointFromDisk(rs)
	if rs.vibeCheckpointNode != "" || len(rs.vibeCheckpointArtifacts) != 0 {
		t.Fatalf("node=%q arts=%v", rs.vibeCheckpointNode, rs.vibeCheckpointArtifacts)
	}
}

func TestCA793_ReconstructDemotesDeletedTask(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	dir := t.TempDir()
	task := "requirements/08-Task/todo/Task-1-x.md"
	cp := "requirements/07-Coding-Plan/todo/CP-60-x.md"
	ca793Write(t, dir, task, "# T\n")
	ca793Write(t, dir, cp, "# CP\n")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	rs.vibeCheckpointNode = vibeTaskSlicerNodeID
	rs.vibeCheckpointArtifacts = []string{task}
	rs.vibeLockedCP = cp
	st := sessionStateOf(rs)
	st.WorkingDirectory = dir
	svc.mu.Unlock()

	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(task))); err != nil {
		t.Fatal(err)
	}
	got, recErr := svc.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstruct: %v", recErr)
	}
	if got.vibeCheckpointNode != vibeCpWriterNodeID {
		t.Fatalf("node=%q want cp_writer after Task delete", got.vibeCheckpointNode)
	}
}

func TestCA793_AliasSprintSlicerDemotesToCP(t *testing.T) {
	dir := t.TempDir()
	task := "requirements/08-Task/todo/Task-1-x.md"
	cp := "requirements/07-Coding-Plan/todo/CP-60-x.md"
	ca793Write(t, dir, task, "# T\n")
	ca793Write(t, dir, cp, "# CP\n")
	rs := &interactiveRun{workingMode: workingmode.Vibe, workspaceCwd: dir, vibeLockedCP: cp}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(task))); err != nil {
		t.Fatal(err)
	}
	node, _ := demoteVibeCheckpoint(dir, vibeSprintSlicerNodeID, []string{task}, rs)
	if node != vibeCpWriterNodeID {
		t.Fatalf("sprint_slicer alias node=%q want cp_writer", node)
	}
}

func TestCA793_AliasCpLockDemotesToSS(t *testing.T) {
	dir := t.TempDir()
	ss := "requirements/05-System-Specs/SS-18-x.md"
	cp := "requirements/07-Coding-Plan/todo/CP-60-x.md"
	ca793Write(t, dir, ss, "# SS\n")
	ca793Write(t, dir, cp, "# CP\n")
	rs := &interactiveRun{workingMode: workingmode.Vibe, workspaceCwd: dir, vibeLockedSS: ss, vibeLockedCP: cp}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(cp))); err != nil {
		t.Fatal(err)
	}
	node, _ := demoteVibeCheckpoint(dir, vibeCpLockNodeID, []string{cp}, rs)
	if node != vibeSSLockNodeID {
		t.Fatalf("cp_lock alias node=%q want ss_lock", node)
	}
}

func TestCA793_CommitStoresCanonicalLayer(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	dir := t.TempDir()
	ca793Write(t, dir, "requirements/08-Task/todo/Task-1-x.md", "# T\n")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	svc.mu.Unlock()
	if !svc.tryCommitVibeCheckpoint(parent.RunID, vibeSprintSlicerNodeID) {
		t.Fatal("commit sprint_slicer alias")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[parent.RunID].vibeCheckpointNode != vibeTaskSlicerNodeID {
		t.Fatalf("stored=%q want task_slicer", svc.runs[parent.RunID].vibeCheckpointNode)
	}
}

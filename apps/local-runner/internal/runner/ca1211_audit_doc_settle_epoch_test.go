package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// CA-1211 (live run-262417): the audit node's task-doc settle resolved the
// sprint's doc from the LIVE vibeSprintIndex. A settle landing after the
// boundary take advanced the index moved the NEW sprint's todo doc into
// done/ before its work began — done/ progress counting then reported a
// sprint as delivered that had just mounted. The settle is now epoch-gated:
// the index captured at audit-body start must still be current at
// rename+repoint time, verified under one s.mu hold.
func TestCA1211_StaleEpochRefusesToMoveNextSprintDoc(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	doc042 := "requirements/08-Task/todo/Task-042-scanner.md"
	docPath := filepath.Join(ws, filepath.FromSlash(doc042))
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("task doc"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	// Boundary already advanced: sprint-041's audit body is landing LATE.
	rs.vibeSprintIndex = 2
	rs.vibeTaskPlan = []string{"requirements/08-Task/done/Task-041-rasp.md", doc042}
	svc.mu.Unlock()

	settled := svc.settleVibeSprintTaskDocForEpoch(parent.RunID, "audit", ws, doc042, 1)
	if settled != "" {
		t.Fatalf("stale-epoch settle moved a doc: %q", settled)
	}
	if _, err := os.Stat(docPath); err != nil {
		t.Fatal("sprint-042's todo doc must not be moved by a sprint-041 audit")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.vibeTaskPlan[1] != doc042 {
		t.Fatalf("plan entry must stay pointed at the todo doc, got %q", rs.vibeTaskPlan[1])
	}
}

func TestCA1211_CurrentEpochSettlesOwnDoc(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	doc041 := "requirements/08-Task/todo/Task-041-rasp.md"
	docPath := filepath.Join(ws, filepath.FromSlash(doc041))
	if err := os.MkdirAll(filepath.Dir(docPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(docPath, []byte("task doc"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeSprintIndex = 1
	rs.vibeTaskPlan = []string{doc041, "requirements/08-Task/todo/Task-042-scanner.md"}
	svc.mu.Unlock()

	settled := svc.settleVibeSprintTaskDocForEpoch(parent.RunID, "audit", ws, doc041, 1)
	want := "requirements/08-Task/done/Task-041-rasp.md"
	if settled != want {
		t.Fatalf("current-epoch settle must move the doc, got %q want %q", settled, want)
	}
	if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(want))); err != nil {
		t.Fatal("done/ doc missing after settle")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.vibeTaskPlan[0] != want {
		t.Fatalf("plan entry must be re-pointed at done/, got %q", rs.vibeTaskPlan[0])
	}
}

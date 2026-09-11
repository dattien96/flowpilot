package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-365: live run-646702 — the operator locked at ss_lock, but the SS files
// stayed `status: draft` on disk because the lock was in-memory only.
// cp_writer read the draft and asked the operator to fix the status first
// instead of writing the CP; no CP/Task ever existed and the slicer parked.
// The lock must stamp the SS list (SS-*.md + SPRINT-PLAN*.md, never FORMAT-*)
// approved before cp_writer runs.
func TestBUG365_SSLockStampsApprovedOnDisk(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	specDir := filepath.Join(cwd, "requirements", "05-System-Specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ssBody := strings.Join([]string{
		"---",
		"id: SS-100",
		"title: Snake MVP",
		"status: draft",
		"---",
		"",
		"## Metadata",
		"",
		"- Document ID: `SS-100`",
		"- Status: `draft`",
		"",
		"SS-100 status can move from `draft` → `approved` (locked) by operator.",
		"",
	}, "\n")
	sprintBody := "---\nid: SPRINT-PLAN-snake-mvp\nstatus: draft\n---\n\n- Status: `draft`\n"
	formatBody := "---\nid: FORMAT-REFERENCE-SS\nstatus: draft\n---\n\n- Status: `draft | reviewing | approved | superseded`\n"
	for name, body := range map[string]string{
		"SS-100-snake-mvp.md":      ssBody,
		"SPRINT-PLAN-snake-mvp.md": sprintBody,
		"FORMAT-REFERENCE-SS.md":   formatBody,
	} {
		if err := os.WriteFile(filepath.Join(specDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui", Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeAwaitingLock = true
	rs.vibeLockNodeID = vibeSSLockNodeID
	rs.vibeLockPath = "requirements/05-System-Specs/SS-100-snake-mvp.md"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: vibeSSLockNodeID, To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()

	if _, ok := svc.resumeVibeLock(parent.RunID, "continue", AgentGraphSnapshot{}); !ok {
		t.Fatal("resumeVibeLock")
	}

	read := func(name string) string {
		t.Helper()
		b, readErr := os.ReadFile(filepath.Join(specDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		return string(b)
	}
	ss := read("SS-100-snake-mvp.md")
	if !strings.Contains(ss, "status: approved") {
		t.Fatalf("frontmatter not stamped approved:\n%s", ss)
	}
	if !strings.Contains(ss, "- Status: `approved`") {
		t.Fatalf("metadata status not stamped approved:\n%s", ss)
	}
	if !strings.Contains(ss, "can move from `draft` → `approved`") {
		t.Fatalf("prose mentioning draft must not be rewritten:\n%s", ss)
	}
	if sp := read("SPRINT-PLAN-snake-mvp.md"); !strings.Contains(sp, "status: approved") || !strings.Contains(sp, "- Status: `approved`") {
		t.Fatalf("sprint plan not stamped approved:\n%s", sp)
	}
	if f := read("FORMAT-REFERENCE-SS.md"); !strings.Contains(f, "status: draft") {
		t.Fatalf("FORMAT-* reference must not be stamped:\n%s", f)
	}
}

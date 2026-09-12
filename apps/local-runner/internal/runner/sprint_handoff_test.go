package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// Task-342 (CP-62 P-6): sprint handoff written from verified state, consumed
// by the next sprint's entry prompt, graceful fallback when missing.

func handoffRun(t *testing.T) (*InteractiveService, *interactiveRun, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	cwd := t.TempDir()
	rs.workspaceCwd = cwd
	rs.vibeSprintIndex = 2
	rs.vibeTaskName = "requirements/08-Task/todo/Task-904-string-reverse.md"
	rs.lastFlowVerdicts = []VerdictRow{
		{ACID: "AC-1", Verdict: "pass", Note: "verified by signature test"},
		{ACID: "AC-3", Verdict: "fail", Note: "unicode collation deferred"},
	}
	svc.mu.Unlock()
	_ = cwd
	return svc, rs, cwd
}

// Scenario: Node Audit ghi thành công file YAML handoff đúng định dạng sprint_handoff.v1
func TestSprintHandoff_AuditNodeWritesValidYaml(t *testing.T) {
	svc, rs, cwd := handoffRun(t)
	path := svc.emitSprintHandoff(rs)
	if path == "" {
		t.Fatalf("handoff not written")
	}
	wantPath := filepath.Join(cwd, "requirements", ".flowpilot", "vibe", "handoffs", "handoff-sprint-2.yaml")
	if path != wantPath {
		t.Fatalf("path = %q want %q", path, wantPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// JSON is a valid YAML 1.2 subset — parse both ways.
	var handoff SprintHandoffV1
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if handoff.Sprint != 2 || !strings.HasSuffix(handoff.Task, "Task-904-string-reverse.md") {
		t.Fatalf("handoff = %+v", handoff)
	}
	if len(handoff.Decisions) != 2 || handoff.Decisions[0].Why != "verified by signature test" {
		t.Fatalf("decisions = %+v", handoff.Decisions)
	}
	// fail rows surface as risks (open findings), never as silent done.
	if len(handoff.Risks) != 1 || !strings.Contains(handoff.Risks[0], "AC-3") {
		t.Fatalf("risks = %+v", handoff.Risks)
	}
	if filepath.Base(path) != "handoff-sprint-2.yaml" {
		t.Fatalf("path = %q", path)
	}
}

// Scenario: Sprint kế tiếp tự động nạp handoff của sprint trước như nguồn ưu tiên cao
func TestSprintHandoff_NextSprintConsumesAsHighPriority(t *testing.T) {
	svc, rs, cwd := handoffRun(t)
	if path := svc.emitSprintHandoff(rs); path == "" {
		t.Fatalf("handoff not written")
	}
	ctx := previousSprintHandoffContext(cwd, 3) // sprint 3 reads sprint 2's handoff
	if !strings.Contains(ctx, "Sprint Handoff") || !strings.Contains(ctx, "AC-3") {
		t.Fatalf("next sprint prompt must carry the handoff: %q", ctx)
	}
}

// Scenario: Thiếu file handoff từ sprint trước -> kích hoạt fallback an toàn mà không dừng phiên
func TestSprintHandoff_MissingHandoff_GracefulFallback(t *testing.T) {
	cwd := t.TempDir()
	if got := previousSprintHandoffContext(cwd, 1); got != "" {
		t.Fatalf("sprint 1 has no predecessor: %q", got)
	}
	if got := previousSprintHandoffContext(cwd, 5); got != "" {
		t.Fatalf("missing file must degrade to empty: %q", got)
	}
}

// Scenario: Decisions chỉ được trích từ bằng chứng kiểm chứng (anti-hallucination) —
// không verdict rows thì không có decisions/risks nào được bịa ra.
func TestSprintHandoff_WhyFieldDerivedFromEvidenceOnly(t *testing.T) {
	svc, rs, cwd := handoffRun(t)
	svc.mu.Lock()
	rs.lastFlowVerdicts = nil
	svc.mu.Unlock()
	if path := svc.emitSprintHandoff(rs); path == "" {
		t.Fatalf("handoff not written")
	}
	data, err := os.ReadFile(sprintHandoffPath(cwd, 2))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var handoff SprintHandoffV1
	if err := json.Unmarshal(data, &handoff); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(handoff.Decisions) != 0 || len(handoff.Risks) != 0 {
		t.Fatalf("no evidence → no invented decisions/risks: %+v", handoff)
	}
}

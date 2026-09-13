package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-344 (CP-62 P-2 follow-up): wire per-AC verdict coverage into the
// submit path. Task-338 shipped the pure functions; these tests pin the
// deterministic resolution of the governing task artifact and the bridge
// enforcement — provider-neutral by construction (one funnel, three faces).

func coverageWriteDoc(t *testing.T, ws, rel, content string, mod time.Time) {
	t.Helper()
	path := filepath.Join(ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func coverageRun(t *testing.T) (*InteractiveService, *interactiveRun) {
	t.Helper()
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Dev, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.runs[handle.RunID].workspaceCwd = t.TempDir()
	return svc, svc.runs[handle.RunID]
}

func coverageChild(t *testing.T, svc *InteractiveService, parent *interactiveRun, stepID string) *interactiveRun {
	t.Helper()
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: parent.workingMode, Client: "tui",
	})
	if err != nil {
		t.Fatalf("child createRun: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	child := svc.runs[handle.RunID]
	child.parentRunID = parent.id
	child.stepID = stepID
	return child
}

// Scenario: Reviewer vibe nộp thiếu AC -> bị từ chối với error nêu đích danh AC thiếu
func TestReviewACCoverage_VibeTaskDoc_MissingACRejected(t *testing.T) {
	svc, rs := coverageRun(t)
	ws := rs.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md",
		"# Task\n- [ ] AC-1 login\n- [ ] AC-2 refresh\n- [ ] AC-3 logout\n", time.Now())
	svc.mu.Lock()
	rs.vibeTaskName = "Task-1-auth.md" // runtime value là shortTaskName
	svc.mu.Unlock()

	in, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "changes_requested",
		Feedback: "AC-2 missing",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}},
	})
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	got := svc.validateReviewACCoverage(rs, in)
	if got == nil || !strings.Contains(got.Error(), "AC-2") {
		t.Fatalf("want missing AC-2 error, got %v", got)
	}
	in2, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status: "approved",
		Verdicts: []VerdictRow{
			{ACID: "AC-1", Verdict: "pass"}, {ACID: "AC-2", Verdict: "pass"}, {ACID: "AC-3", Verdict: "pass"},
		},
	})
	if err := svc.validateReviewACCoverage(rs, in2); err != nil {
		t.Fatalf("full coverage must pass: %v", err)
	}
}

// Scenario: plan_reviewer qua INPUT pathTemplate binding -> glob file MỚI NHẤT khớp pattern
func TestReviewACCoverage_TemplateInputBinding_NewestFileWins(t *testing.T) {
	svc, parent := coverageRun(t)
	ws := parent.workspaceCwd
	old := time.Now().Add(-time.Hour)
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-old.md",
		"# old\n- [ ] AC-1\n- [ ] AC-2\n", old)
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-2-new.md",
		"# new\n- [ ] AC-1\n- [ ] AC-2\n- [ ] AC-3\n", time.Now())
	svc.mu.Lock()
	parent.activeFlowNodes = []agentpack.FlowNode{{
		ID: "plan_reviewer", Posture: PostureReadOnly,
		ArtifactBindings: []agentpack.FlowArtifactBinding{{
			Direction:      "input",
			ArtifactTypeID: ArtifactTypeFile,
			ConfigJSON:     map[string]any{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
		}},
	}}
	svc.mu.Unlock()
	child := coverageChild(t, svc, parent, "plan_reviewer")

	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "approved",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}, {ACID: "AC-2", Verdict: "pass"}},
	})
	got := svc.validateReviewACCoverage(child, in)
	if got == nil || !strings.Contains(got.Error(), "AC-3") {
		t.Fatalf("newest file (Task-2) must govern: want missing AC-3, got %v", got)
	}
}

// Scenario: reviewer không có binding riêng -> dùng OUTPUT pathTemplate của node writer anh em
func TestReviewACCoverage_SiblingOutputTemplate_ForReviewerWithoutBinding(t *testing.T) {
	svc, parent := coverageRun(t)
	ws := parent.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-7-reverse.md",
		"# task\n- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	parent.activeFlowNodes = []agentpack.FlowNode{
		{
			ID: "plan_writer",
			ArtifactBindings: []agentpack.FlowArtifactBinding{{
				Direction:      "output",
				Required:       true,
				ArtifactTypeID: ArtifactTypeFile,
				ConfigJSON:     map[string]any{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
			}},
		},
		{ID: "reviewer", Posture: PostureReadOnly},
	}
	svc.mu.Unlock()
	child := coverageChild(t, svc, parent, "reviewer")

	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "approved",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}},
	})
	got := svc.validateReviewACCoverage(child, in)
	if got == nil || !strings.Contains(got.Error(), "AC-2") {
		t.Fatalf("sibling output template must govern: want missing AC-2, got %v", got)
	}
}

// Scenario: Owner debate (verdict_only) KHÔNG BAO GIỜ bị enforce AC coverage
func TestReviewACCoverage_OwnerVerdictOnly_NeverEnforced(t *testing.T) {
	svc, parent := coverageRun(t)
	ws := parent.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md",
		"- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	parent.activeFlowNodes = []agentpack.FlowNode{{
		ID: "owner_1", Posture: PostureVerdictOnly,
		ArtifactBindings: []agentpack.FlowArtifactBinding{{
			Direction:      "input",
			ArtifactTypeID: ArtifactTypeFile,
			ConfigJSON:     map[string]any{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
		}},
	}}
	svc.mu.Unlock()
	child := coverageChild(t, svc, parent, "owner_1")

	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
	if err := svc.validateReviewACCoverage(child, in); err != nil {
		t.Fatalf("verdict_only must never be AC-covered: %v", err)
	}
}

// Scenario: Không resolve được doc (flow tự do / legacy) -> passthrough byte-identical
func TestReviewACCoverage_NoDoc_Passthrough(t *testing.T) {
	svc, rs := coverageRun(t)
	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
	if err := svc.validateReviewACCoverage(rs, in); err != nil {
		t.Fatalf("no doc -> passthrough, got %v", err)
	}
}

// Scenario: Raw flow_control (không qua submit_review_outcome) -> bỏ qua
func TestReviewACCoverage_RawFlowControl_Skipped(t *testing.T) {
	svc, rs := coverageRun(t)
	ws := rs.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md", "- [ ] AC-1\n", time.Now())
	svc.mu.Lock()
	rs.vibeTaskName = "Task-1-auth.md"
	svc.mu.Unlock()
	if err := svc.validateReviewACCoverage(rs, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("raw flow control must be skipped: %v", err)
	}
}

// Scenario: Vibe hub (root run, không posture) — nộp verdicts THIẾU bị chặn, nộp rỗng passthrough
func TestReviewACCoverage_VibeRootHub_PartialVerdictsRejected(t *testing.T) {
	svc, rs := coverageRun(t)
	ws := rs.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md",
		"- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-1-auth.md"}
	rs.vibeSprintIndex = 1
	svc.mu.Unlock()

	full, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"}) // no verdicts
	if err := svc.validateReviewACCoverage(rs, full); err != nil {
		t.Fatalf("hub submit without verdict rows stays passthrough: %v", err)
	}
	partial, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "approved",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}},
	})
	if got := svc.validateReviewACCoverage(rs, partial); got == nil || !strings.Contains(got.Error(), "AC-2") {
		t.Fatalf("hub partial verdicts must be rejected: got %v", got)
	}
}

// Scenario: Bridge-level — thiếu AC bị từ chối TRƯỚC khi flow control xử lý
func TestReviewACCoverage_BridgeSubmit_RejectsBeforeProcessing(t *testing.T) {
	svc, parent := coverageRun(t)
	ws := parent.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md",
		"- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	parent.activeFlowNodes = []agentpack.FlowNode{
		{
			ID: "plan_writer",
			ArtifactBindings: []agentpack.FlowArtifactBinding{{
				Direction:      "output",
				Required:       true,
				ArtifactTypeID: ArtifactTypeFile,
				ConfigJSON:     map[string]any{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
			}},
		},
		{ID: "reviewer", Posture: PostureReadOnly},
	}
	svc.mu.Unlock()
	child := coverageChild(t, svc, parent, "reviewer")
	child.workspaceCwd = ws

	bridge := &turnBridge{svc: svc, rs: child}
	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "approved",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}},
	})
	if _, err := bridge.SubmitFlowControl(in); err == nil || !strings.Contains(err.Error(), "AC-2") {
		t.Fatalf("bridge must reject missing AC-2, got %v", err)
	}
	full, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status: "approved",
		Verdicts: []VerdictRow{
			{ACID: "AC-1", Verdict: "pass"}, {ACID: "AC-2", Verdict: "pass"},
		},
	})
	if _, err := bridge.SubmitFlowControl(full); err != nil {
		t.Fatalf("full coverage submit must proceed: %v", err)
	}
}

// Scenario: Cache — doc resolve 1 lần/child run, xóa file sau đó vẫn còn enforce
func TestReviewACCoverage_CachedPerChildRun(t *testing.T) {
	svc, rs := coverageRun(t)
	ws := rs.workspaceCwd
	doc := "requirements/08-Task/todo/Task-1-auth.md"
	coverageWriteDoc(t, ws, doc, "- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	rs.vibeTaskName = "Task-1-auth.md"
	svc.mu.Unlock()

	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "approved",
		Verdicts: []VerdictRow{{ACID: "AC-1", Verdict: "pass"}},
	})
	if got := svc.validateReviewACCoverage(rs, in); got == nil {
		t.Fatalf("first submit must enforce")
	}
	if err := os.Remove(filepath.Join(ws, filepath.FromSlash(doc))); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := svc.validateReviewACCoverage(rs, in); got == nil || !strings.Contains(got.Error(), "AC-2") {
		t.Fatalf("cached set must still enforce after doc removal: got %v", got)
	}
}

// Task-352 (review finding): read_only face + resolvable doc + ZERO verdict
// rows → reject-all. Đây là cell "rubber-stamp guard" — mục đích tuyên bố của
// task — nhưng trước đây chỉ được pin qua các test partial rows.
func TestReviewACCoverage_ReadOnlyZeroRows_Rejected(t *testing.T) {
	svc, parent := coverageRun(t)
	ws := parent.workspaceCwd
	coverageWriteDoc(t, ws, "requirements/08-Task/todo/Task-1-auth.md",
		"- [ ] AC-1\n- [ ] AC-2\n", time.Now())
	svc.mu.Lock()
	parent.activeFlowNodes = []agentpack.FlowNode{
		{
			ID: "plan_writer",
			ArtifactBindings: []agentpack.FlowArtifactBinding{{
				Direction:      "output",
				Required:       true,
				ArtifactTypeID: ArtifactTypeFile,
				ConfigJSON:     map[string]any{"pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md"},
			}},
		},
		{ID: "reviewer", Posture: PostureReadOnly},
	}
	svc.mu.Unlock()
	child := coverageChild(t, svc, parent, "reviewer")

	in, _ := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
	got := svc.validateReviewACCoverage(child, in)
	if got == nil || !strings.Contains(got.Error(), "AC-1") {
		t.Fatalf("read_only zero rows must be rejected with named ACs, got %v", got)
	}
}

// Task-352: malformed `verdicts` arg (present nhưng không phải array) phải bị
// từ chối ở parse layer — không được rơi im lặng vào empty-passthrough.
func TestReviewACCoverage_MalformedVerdictsArgRejected(t *testing.T) {
	_, err := parseReviewOutcomeInput(map[string]any{
		"status":   "approved",
		"verdicts": map[string]any{"ac_id": "AC-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "verdicts must be an array") {
		t.Fatalf("malformed verdicts arg must be rejected, got %v", err)
	}
}

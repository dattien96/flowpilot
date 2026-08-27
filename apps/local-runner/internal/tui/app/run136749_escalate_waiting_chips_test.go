package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// run-136749: Rag Harness implement WAITING_USER_APPROVAL + blocked escalate
// showed no [Continue]/[Stop] and kept Thinking on. WAITING_USER_APPROVAL is a
// park stamp, not live work — chips must appear, Thinking off.

func escalateBlockedModel() *AppModel {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "rag-harness"}
	m.runHandle = &client.RunHandle{RunID: "run-136749", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "escalate"
	// Rag-harness: context DONE, implement WAITING, validate/audit pending.
	m.flowSteps = []client.WorkflowStepRuntime{
		{NodeID: "preflight_contract_plan", Status: "DONE"},
		{NodeID: "preflight_contract_freeze", Status: "DONE"},
		{NodeID: "context", Status: "DONE"},
		{NodeID: "implement", Status: "WAITING_USER_APPROVAL"},
		{NodeID: "validate", Status: "PENDING"},
		{NodeID: "audit", Status: "PENDING"},
	}
	m.flowStepsActive = "implement"
	// Agent runs: main completed + implement stamped waiting_user_approval (park)
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-136749", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-136750", AgentName: "coder", Label: "implement", Status: "waiting_user_approval"},
	}
	return m
}

func TestRun136749_EscalateWAITINGHasContinueStopNoApprove(t *testing.T) {
	m := escalateBlockedModel()
	view := stripANSI(m.View())
	if !strings.Contains(view, "[Retry]") || !strings.Contains(view, "[Stop]") {
		t.Fatalf("escalate WAITING must render [Continue]/[Stop], got:\n%s", view)
	}
	if strings.Contains(view, "Approve") || strings.Contains(view, "Deny") {
		t.Fatalf("escalate park must not show Approve/Deny, got:\n%s", view)
	}
	if !m.flowLoopBlocked() {
		t.Fatal("flowLoopBlocked must be true for escalate + WAITING park")
	}
	if m.workIsLive() {
		t.Fatal("workIsLive must be false when blocked on WAITING park (Thinking off)")
	}
	if m.turnIsActive() {
		t.Fatal("turnIsActive must be false when blocked on WAITING park ([stop] off)")
	}
}

func TestRun136749_YOLOApprovalStillWinsOverPark(t *testing.T) {
	m := escalateBlockedModel()
	m.approval = &ApprovalState{ID: "ap-1", Kind: "exec", Command: "go test ./..."}
	view := stripANSI(m.View())
	if strings.Contains(view, "[Retry]") {
		t.Fatalf("YOLO approval present must not show [Continue], got:\n%s", view)
	}
	if m.flowLoopBlocked() {
		t.Fatal("flowLoopBlocked must be false when YOLO approval is pending")
	}
	// approval bar must still be present (separate path)
	if !strings.Contains(view, "Approve") {
		t.Fatalf("YOLO approval bar must show Approve, got:\n%s", view)
	}
}

func TestRun136749_QuestionWinsOverPark(t *testing.T) {
	m := escalateBlockedModel()
	m.question = &QuestionState{ID: "q-1", Prompt: "Which model?"}
	if m.flowLoopBlocked() {
		t.Fatal("flowLoopBlocked must be false when question is pending")
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "[Retry]") {
		t.Fatalf("question present must not show [Continue], got:\n%s", view)
	}
}

func TestRun136749_RunningChildStillHidesChips(t *testing.T) {
	m := escalateBlockedModel()
	// Replace WAITING with truly running coder
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-136749", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-136750", AgentName: "coder", Label: "implement", Status: "running"},
	}
	if m.flowLoopBlocked() {
		t.Fatal("blocked with running child must not read as parked")
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "[Retry]") {
		t.Fatalf("running child must hide [Continue], got:\n%s", view)
	}
}

func TestRun136749_CapAndDelegateFailedAlsoPark(t *testing.T) {
	for _, reason := range []string{"cap", "delegate_failed"} {
		m := escalateBlockedModel()
		m.flowBlockReason = reason
		if !m.flowLoopBlocked() {
			t.Fatalf("%s + WAITING park must be blocked", reason)
		}
		view := stripANSI(m.View())
		if !strings.Contains(view, "[Retry]") {
			t.Fatalf("%s must render [Continue], got:\n%s", reason, view)
		}
	}
}

func TestRun136749_BannerIncludesGateReason(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: "Reviewer requested escalate: missing clamp"},
		Runs: []client.AgentRunSummary{{RunID: "run-136749", AgentName: "main", Status: "completed"}},
	})
	var banner string
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "blocked: escalate") {
			banner = msg.Content
		}
	}
	if banner == "" {
		t.Fatalf("escalate banner not found, messages=%v", m.messages)
	}
	if !strings.Contains(banner, "missing clamp") {
		t.Fatalf("escalate banner must include GateReason, got %q", banner)
	}
	// Unicode truncation
	m2 := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	gate := strings.Repeat("á", 130) + " escalate reason"
	m2.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: gate},
		Runs: []client.AgentRunSummary{{RunID: "run-2", Status: "completed"}},
	})
	var banner2 string
	for _, msg := range m2.messages {
		if strings.Contains(msg.Content, "blocked: escalate") {
			banner2 = msg.Content
		}
	}
	if strings.Contains(banner2, "\ufffd") {
		t.Fatalf("banner has broken rune, got %q", banner2)
	}
	if !strings.Contains(banner2, "…") {
		t.Fatalf("long gate must be truncated with …, got %q", banner2)
	}
}

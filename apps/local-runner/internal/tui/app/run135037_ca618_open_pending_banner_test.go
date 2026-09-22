package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-618 I3: /open (prev empty) with many FAILED history must emit at most 1, the last FAILED+note.
func TestCA618_FirstPollManyFailedHistoryCapsToOne(t *testing.T) {
	prev := []client.WorkflowStepRuntime{}
	next := []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "old note 1"},
		{StepID: "s2", NodeID: "validate", Status: "FAILED", RejectionNote: "old note 2"},
		{StepID: "s3", NodeID: "audit", Status: "FAILED", RejectionNote: "last note gpt-5.4"},
	}
	got := formatStepChatNotices(prev, next, "", "", "")
	if len(got) != 1 {
		t.Fatalf("first-poll many FAILED want 1 line, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "gpt-5.4") {
		t.Fatalf("want last note gpt-5.4, got %q", got[0])
	}
}

// CA-618 I3: PENDING → FAILED+note when poll skipped RUNNING.
func TestCA618_PendingToFailedEmitsNote(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "PENDING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "The 'gpt-5.4' model is not supported"},
	}
	got := formatStepChatNotices(prev, next, "", "", "")
	if len(got) != 1 || !strings.Contains(got[0], "gpt-5.4") {
		t.Fatalf("PENDING→FAILED must surface note, got %v", got)
	}
}

// CA-618 I3: first-poll with later RUNNING must not dump old FAILED.
func TestCA618_FirstPollWithLaterRunningEmitsNothing(t *testing.T) {
	prev := []client.WorkflowStepRuntime{}
	next := []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "old fail"},
		{StepID: "s2", NodeID: "grok-coder", Status: "RUNNING"},
	}
	got := formatStepChatNotices(prev, next, "", "grok-coder", "")
	// First-poll FAILED behind an active RUNNING should not spam chat.
	// Only the RUNNING notice (s2) is expected, not the old FAILED s1.
	for _, line := range got {
		if strings.Contains(line, "old fail") {
			t.Fatalf("first-poll with later RUNNING must not emit old FAILED, got %v", got)
		}
	}
}

// CA-618 M2: truncateRunes used so multi-byte gate not broken.
func TestCA618_BannerTruncatesRunesNotBytes(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	// gate with Vietnamese runes, >120 runes
	gate := strings.Repeat("á", 130) + " gpt-5.4"
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "delegate_failed", GateReason: gate},
		Runs:      []client.AgentRunSummary{{RunID: "run-1", Status: "completed"}},
	})
	// find banner
	var banner string
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "blocked: delegate_failed") {
			banner = msg.Content
		}
	}
	if banner == "" {
		t.Fatalf("banner not found, messages=%v", m.messages)
	}
	if strings.Contains(banner, "\ufffd") {
		t.Fatalf("banner has broken rune, got %q", banner)
	}
	if !strings.Contains(banner, "…") {
		t.Fatalf("banner must be truncated with …, got %q", banner)
	}
}

// CA-618 M3: reason empty but gate present — banner has "— gate", not "end. —".
func TestCA618_BannerReasonEmptyGatePresent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "", GateReason: "gpt-5.4 not supported"},
		Runs:      []client.AgentRunSummary{{RunID: "run-1", Status: "completed"}},
	})
	var banner string
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "blocked") {
			banner = msg.Content
		}
	}
	if banner == "" {
		t.Fatalf("banner not found")
	}
	if !strings.Contains(banner, "gpt-5.4") {
		t.Fatalf("banner must contain gate, got %q", banner)
	}
	if strings.Contains(banner, "end. —") {
		t.Fatalf("banner must not contain 'end. —', got %q", banner)
	}
}

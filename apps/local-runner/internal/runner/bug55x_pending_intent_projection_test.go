package runner

import (
	"context"
	"testing"
)

// Reproduce: a durable child row carrying a pending resume/reprompt intent is
// projected as "cancelled" by listAgentRunSummaries' disk fallback
// (pending-blind normalizeResumedStatus) while reconstructRun reads the same
// row as Running via normalizeResumedFlowStatus. Same durable record, two
// different statuses — the read surface must agree with the reconstruct
// semantics (live-observed 2026-09-29: seeded pending_resume row listed
// `cancelled` in the Agents view while the intent was still armed).
func TestDiskFallbackKeepsPendingIntentChildRunning(t *testing.T) {
	svc, _ := newTestServer(t)
	parent := "parent-proj"
	child := ProviderSessionState{
		RunID:               "child-parked",
		ParentRunID:         parent,
		ProjectID:           "proj-web",
		ProviderKey:         ProviderKeyCodex,
		Status:              RunStatusRunning,
		AgentStatus:         string(RunStatusRunning),
		AgentName:           "vibe-intake",
		PendingResumePrompt: "dispatch me once unblocked",
		PendingResumeStepID: "chat-child-parked",
		PendingResumeGen:    7,
	}
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	summaries, err := svc.listAgentRunSummaries(parent)
	if err != nil {
		t.Fatalf("listAgentRunSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	if summaries[0].Status != RunStatusRunning {
		t.Fatalf("Status = %q, want %q (durable pending_resume intent must keep the run non-terminal)", summaries[0].Status, RunStatusRunning)
	}
	if summaries[0].AgentStatus == string(RunStatusCancelled) {
		t.Fatalf("AgentStatus = %q, want non-terminal while a resume intent is armed", summaries[0].AgentStatus)
	}
}

// Same arm for the gate-reprompt intent kind — flushDurableTurnIntents
// prefers reprompt over resume; both keep the run non-terminal.
func TestDiskFallbackKeepsPendingRepromptChildRunning(t *testing.T) {
	svc, _ := newTestServer(t)
	child := ProviderSessionState{
		RunID:                     "child-reprompt",
		ParentRunID:               "parent-proj",
		ProjectID:                 "proj-web",
		ProviderKey:               ProviderKeyCodex,
		Status:                    RunStatusRunning,
		AgentStatus:               string(RunStatusRunning),
		PendingGateRepromptPrompt: "fix the flagged paths",
		PendingGateRepromptStepID: "chat-child-reprompt",
		PendingGateRepromptGen:    3,
	}
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	summaries, err := svc.listAgentRunSummaries("parent-proj")
	if err != nil {
		t.Fatalf("listAgentRunSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Status != RunStatusRunning {
		t.Fatalf("summaries = %+v, want 1 row Status=running", summaries)
	}
}

// Guard the guardrail: a plain interrupted child (no durable intent) must
// still normalize to cancelled — the BUG-251 stale-spinner fix.
func TestDiskFallbackStillCancelsPlainInterruptedChild(t *testing.T) {
	svc, _ := newTestServer(t)
	child := ProviderSessionState{
		RunID:       "child-interrupted",
		ParentRunID: "parent-proj",
		ProjectID:   "proj-web",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusRunning,
		AgentStatus: string(RunStatusRunning),
	}
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	summaries, err := svc.listAgentRunSummaries("parent-proj")
	if err != nil {
		t.Fatalf("listAgentRunSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].Status != RunStatusCancelled {
		t.Fatalf("summaries = %+v, want 1 row Status=cancelled (no intent → crash residue)", summaries)
	}
}

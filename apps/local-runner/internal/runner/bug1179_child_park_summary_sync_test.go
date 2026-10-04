package runner

import (
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// run-204891 (live CP-03 sprint, Task-039): a scaffold child finished its turn
// and parked at the requirement gate (waiting_user_approval on the run record)
// while /agents kept reporting status=running — the gate park paths wrote
// rs.status/rs.agentStatus but never upserted the orchestrator summary. The
// desktop badge counted the parked child as executing (CA-1178 shows the
// symptom downstream), and any liveness check reading summaries saw a phantom
// live member. Both child-capable park sites must sync the summary:
// applyVibeGateResolver (requirement route) and handleSpawnedChildTurnFailure
// (flow_awaiting_user park).

func TestBug1179_RequirementParkOnChildSyncsSummary(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-gate", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:       rs.id,
		ParentRunID: parentID,
		AgentName:   "scaffold-architect",
		Role:        "scaffold",
		Status:      RunStatusRunning,
		AgentStatus: string(RunStatusRunning),
	})
	svc.mu.Lock()
	svc.runs[rs.id].workingMode = "vibe"
	child := svc.runs[rs.id]
	svc.mu.Unlock()

	handled := svc.applyVibeGateResolver(rs.id, parentID, "turn-1", child, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID, Trigger: "requirement_signature_drift", Action: "block"},
		}},
	})
	if !handled {
		t.Fatal("requirement violation was not classified as a requirement park")
	}
	svc.mu.Lock()
	if svc.runs[rs.id].status != RunStatusWaitingUserApr {
		t.Fatalf("child status = %q, want waiting_user_approval", svc.runs[rs.id].status)
	}
	svc.mu.Unlock()

	sum, ok := svc.agentOrchestrator.currentSummary(parentID, rs.id)
	if !ok {
		t.Fatal("child summary missing")
	}
	if sum.Status != RunStatusWaitingUserApr {
		t.Fatalf("summary Status = %q, want waiting_user_approval — /agents still reports the parked child as running", sum.Status)
	}
	if sum.AgentStatus != "waiting_user_approval" {
		t.Fatalf("summary AgentStatus = %q, want waiting_user_approval", sum.AgentStatus)
	}
}

func TestBug1179_SpawnedChildAwaitingUserParkSyncsSummary(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-spawn", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:       rs.id,
		ParentRunID: parentID,
		AgentName:   "coder",
		Role:        "coder",
		Status:      RunStatusRunning,
		AgentStatus: string(RunStatusRunning),
	})

	svc.handleSpawnedChildTurnFailure(rs.id, parentID, "prompt", "step-1",
		&apiErr{status: 409, code: "flow_awaiting_user", msg: "awaiting user"})

	sum, ok := svc.agentOrchestrator.currentSummary(parentID, rs.id)
	if !ok {
		t.Fatal("child summary missing")
	}
	if sum.Status != RunStatusWaitingUserApr {
		t.Fatalf("summary Status = %q, want waiting_user_approval", sum.Status)
	}
	svc.mu.Lock()
	if svc.runs[rs.id].pendingResumePrompt == "" {
		t.Fatal("durable resume intent lost — the park must keep pendingResumePrompt armed")
	}
	svc.mu.Unlock()
}

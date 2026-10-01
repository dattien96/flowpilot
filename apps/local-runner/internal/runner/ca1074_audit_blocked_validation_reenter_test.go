package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1074 (live TestVibeTasksLive — 6/6 lanes starved at 1/N sprints): a
// sprint audit escalate "Audit blocked: validation was not positively
// verified (blocked_validation_failed)" parked the run WITHOUT stamping
// lastEscalatedInlineNodeID, so every operator Continue fell through to
// the generic hub reinvoke — looping synthesis → audit on the same stale
// validation state (~10min of LLM work per round) forever, while the
// command.validate node never re-ran and flowValidationRetryState stayed
// empty. Sprint-1 could never terminate, so the next sprint never spawned.
//
// The fix mirrors run-202550's missing-CA seam: the escalate must park the
// AUDIT node itself (not the first-hub fallback), and Continue on a
// validation-blocked audit park must re-enter the nearest upstream
// command.validate node — which records a fresh flowValidationRetryState —
// so the flow advances validate → synthesis → audit on its own edges.
//
// New file; no pre-existing test is modified. The engine path takes no
// providerKey, so the matrix guards future provider drift.

// ca1074SprintTail returns the vibe-sprint tail: coder writer ->
// validate -> synthesis hub -> audit — the exact segment the churn loop
// bypassed.
func ca1074SprintTail() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
		{From: "validate", To: "ask_user", When: "escalate", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate", Lifecycle: "once"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

// TestCA1074AuditValidationBlockedStampsAuditNode locks the reported repro
// half: a blocked_validation_failed escalate must park the AUDIT node
// WAITING and stamp it as the escalated node — the synthesis hub must NOT
// be the stamped node (that is what mis-routed every Continue into the
// generic hub reinvoke).
func TestCA1074AuditValidationBlockedStampsAuditNode(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 4)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			runID := parent.RunID
			workspace := t.TempDir()
			initGitRepoForAuditFixture(t, workspace)
			edges, nodes := ca1074SprintTail()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.workspaceCwd = workspace
			rs.flowStartGitHead = headOfRepo(t, workspace)
			// The wedge condition: validate never ran, so no retry state —
			// BuildAuditDraft blocks on ValidationResult == "".
			rs.flowValidationRetryState = nil
			svc.mu.Unlock()

			auditNode, ok := findFlowNode(nodes, "audit")
			if !ok {
				t.Fatal("fixture must declare an audit node")
			}
			if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "implemented the marker") {
				t.Fatalf("%s: runAuditNode must handle (escalate) blocked_validation_failed", pk)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" || st.BlockReason != "escalate" {
				t.Fatalf("%s: loop = %q/%q, want blocked/escalate", pk, st.Status, st.BlockReason)
			}
			if !strings.Contains(st.GateReason, "validation was not positively verified") {
				t.Fatalf("%s: gateReason = %q, want validation-not-verified", pk, st.GateReason)
			}
			svc.mu.Lock()
			stamped := svc.runs[runID].lastEscalatedInlineNodeID
			svc.mu.Unlock()
			if stamped != "audit" {
				t.Fatalf("%s: lastEscalatedInlineNodeID = %q, want audit", pk, stamped)
			}
			if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: audit = %v, want WAITING_USER_APPROVAL", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "synthesis"); got == StepStatusWaitingUserApr {
				t.Fatalf("%s: synthesis hub must NOT be stamped WAITING on an audit block", pk)
			}
		})
	}
}

// TestCA1074ValidationBlockedContinueReentersValidate locks the remediation
// routing: Continue on a validation-blocked audit park must re-enter the
// upstream command.validate node — recording a fresh
// flowValidationRetryState — never the audit re-check or the synthesis hub
// (both can only re-read the same stale state).
func TestCA1074ValidationBlockedContinueReentersValidate(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 4)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			runID := parent.RunID
			workspace := t.TempDir()
			initGitRepoForAuditFixture(t, workspace)
			// A trivially-green suite: the re-entered validate records a real
			// "passed" verdict, the same artifact the flow produces when
			// coder→validate runs for real.
			if err := os.MkdirAll(filepath.Join(workspace, ".flowpilot", "guard"), 0o755); err != nil {
				t.Fatalf("mkdir guard: %v", err)
			}
			if err := os.WriteFile(filepath.Join(workspace, ".flowpilot", "guard", "test_baseline.json"),
				[]byte(`{"test_command":"true"}`), 0o644); err != nil {
				t.Fatalf("seed baseline: %v", err)
			}
			edges, nodes := ca1074SprintTail()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit", RoundCap: 3})

			// Park exactly like the audit blocked_validation_failed
			// escalate: audit stamped escalated + WAITING, loop blocked.
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.workspaceCwd = workspace
			rs.flowStartGitHead = headOfRepo(t, workspace)
			rs.flowValidationRetryState = nil
			rs.lastEscalatedInlineNodeID = "audit"
			svc.mu.Unlock()
			svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusWaitingUserApr)
			svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Status = "blocked"
				st.BlockReason = "escalate"
				st.GateReason = "Audit blocked: validation was not positively verified (status=blocked_validation_failed, validation=)."
				return st
			})

			if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}
			// The continue must dispatch command.validate for real (async —
			// tryAdvanceFlowThroughInline). Poll the retry state it records.
			deadline := time.Now().Add(15 * time.Second)
			for {
				svc.mu.Lock()
				var st FlowValidationRetryState
				if rs := svc.runs[runID]; rs != nil && rs.flowValidationRetryState != nil {
					st = *rs.flowValidationRetryState
				}
				svc.mu.Unlock()
				if st.Status != "" {
					if st.Status != "passed" {
						t.Fatalf("%s: re-entered validate status = %q, want passed (test_command=true)", pk, st.Status)
					}
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("%s: Continue never re-entered command.validate — flowValidationRetryState stayed empty (synthesis/audit churn)", pk)
				}
				time.Sleep(50 * time.Millisecond)
			}
		})
	}
}

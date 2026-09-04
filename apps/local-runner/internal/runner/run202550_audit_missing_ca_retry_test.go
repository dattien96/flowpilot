package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-202550 (CP-58 S6 / task-harness): the flow reached the audit node with
// code changed but no change-audit note. The audit tier-3 escalate parked
// plan_synthesis WAITING_USER_APPROVAL (setFlowStepAwaitingUser's first-hub
// fallback) instead of the audit node, so Retry reinvoked the plan hub —
// re-running the plan instead of writing the missing CA note — and the audit
// never re-ran. Retry on a missing-CA park must re-enter the nearest upstream
// agent.code writer with an explicit "write the missing CA note" prompt.
//
// New file; no pre-existing test is modified. The engine path takes no
// providerKey, so the matrix guards future provider drift.

// run202550DualHubChain returns the task-harness tail: plan hub + code writer
// + validate + reviewer + synthesis hub + audit, forward-chained.
func run202550DualHubChain() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "implement", When: "done", Kind: "forward"},
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate", Lifecycle: "once"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

// run202550AuditFixture builds a git workspace with a code change and NO
// change-audit note, so the audit tier-3 doc-rule pass fires r-ca.
func run202550AuditFixture(t *testing.T, pk ProviderKey) (*InteractiveService, string, []agentpack.FlowEdge, []agentpack.FlowNode, agentpack.FlowNode) {
	t.Helper()
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
	// Code change, deliberately no change-audit note.
	p4WriteFile(t, workspace, "calc.go", "package calc\n")
	edges, nodes := run202550DualHubChain()
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("fixture must declare an audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "synthesis"
	rs.workspaceCwd = workspace
	rs.flowStartGitHead = headOfRepo(t, workspace)
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID, edges, nodes, auditNode
}

// TestRun202550AuditMissingCAStampsAuditNotHub locks the reported repro: an
// audit tier-3 missing-CA escalate must park the AUDIT node WAITING (and stamp
// it as the escalated node) — plan_synthesis must NOT be stamped WAITING.
func TestRun202550AuditMissingCAStampsAuditNotHub(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, edges, nodes, auditNode := run202550AuditFixture(t, pk)

			if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "implemented GCD") {
				t.Fatalf("%s: runAuditNode must handle (escalate) the missing-CA tier-3 block", pk)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" || st.BlockReason != "escalate" {
				t.Fatalf("%s: loop = %q/%q, want blocked/escalate", pk, st.Status, st.BlockReason)
			}
			if !isMissingChangeAuditNoteReason(st.GateReason) {
				t.Fatalf("%s: gateReason = %q, want the missing-CA reason", pk, st.GateReason)
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
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got == StepStatusWaitingUserApr {
				t.Fatalf("%s: plan_synthesis must NOT be stamped WAITING on an audit block", pk)
			}
		})
	}
}

// seedRun202550ImplementChild registers a completed implement child so Retry
// has a writer run to re-enter. The child is a real run object (BUG-327
// pattern), not a hand-built struct, so the reinvoked turn can start.
func seedRun202550ImplementChild(t *testing.T, svc *InteractiveService, runID string, pk ProviderKey) string {
	t.Helper()
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = runID
	crs.agentName = "coder"
	crs.label = "implement"
	crs.role = "coder"
	crs.status = RunStatusCompleted
	crs.stepID = "step-implement"
	svc.agentOrchestrator.registerChild(runID, child.RunID)
	return child.RunID
}

// TestRun202550MissingCARetryReentersWriter locks the Retry routing: on a
// missing-CA audit park, resumeFlowWithFeedback must reinvoke the implement
// child with a write-the-CA prompt — never the plan hub.
func TestRun202550MissingCARetryReentersWriter(t *testing.T) {
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
			edges, nodes := run202550DualHubChain()
			setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
			childID := seedRun202550ImplementChild(t, svc, runID, pk)

			// Park exactly like the audit tier-3 missing-CA escalate does.
			svc.mu.Lock()
			svc.runs[runID].lastEscalatedInlineNodeID = "audit"
			svc.mu.Unlock()
			svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusWaitingUserApr)
			svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Status = "blocked"
				st.BlockReason = "escalate"
				st.GateReason = "Audit gate (aggregate): Flow gate: code changed but no change-audit note found."
				return st
			})

			if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}
			// Synchronous proof of writer re-entry (BUG-327 pattern):
			// reinvokeMatchingFlowChild bumps activationSeq before the async
			// turn starts.
			svc.mu.Lock()
			activationSeq := svc.runs[childID].activationSeq
			svc.mu.Unlock()
			if activationSeq != 1 {
				t.Fatalf("%s: implement child activationSeq = %d, want 1 (Retry must re-enter the writer)", pk, activationSeq)
			}
			select {
			case req := <-ch:
				if !strings.Contains(strings.ToLower(req.Prompt), "change-audit note") {
					t.Fatalf("%s: writer re-entry prompt must instruct the missing CA note; got %.200q", pk, req.Prompt)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("%s: implement writer turn was never scheduled after Retry", pk)
			}
			if got := flowStepStatus(t, svc, runID, "implement"); got != StepStatusRunning {
				t.Fatalf("%s: implement = %v, want RUNNING after Retry", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got == StepStatusRunning {
				t.Fatalf("%s: plan_synthesis must NOT be reinvoked on a missing-CA Retry", pk)
			}
		})
	}
}

// TestRun202550NonMissingCAAuditParkKeepsAuditRedispatch is the near-miss:
// an audit park for any OTHER reason (e.g. scope drift text) must NOT take
// the writer path — it falls through to the existing audit re-dispatch.
func TestRun202550NonMissingCAAuditParkKeepsAuditRedispatch(t *testing.T) {
	svc, runID, ch := run198699HarnessService(t, ProviderKeyCodex)
	edges, nodes := run202550DualHubChain()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	seedRun202550ImplementChild(t, svc, runID, ProviderKeyCodex)

	svc.mu.Lock()
	svc.runs[runID].lastEscalatedInlineNodeID = "audit"
	svc.mu.Unlock()
	svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusWaitingUserApr)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = "Audit gate (aggregate): some other audit finding."
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	select {
	case req := <-ch:
		if strings.Contains(strings.ToLower(req.Prompt), "change-audit note") {
			t.Fatalf("non-missing-CA park must not take the writer-CA path; got %.200q", req.Prompt)
		}
	case <-time.After(5 * time.Second):
		// No writer turn scheduled — acceptable: the park fell through to the
		// audit re-dispatch path, which needs no writer turn.
	}
	if got := countChildrenWithLabel(svc, runID, "implement"); got != 1 {
		t.Fatalf("implement child count = %d, want 1 (no extra writer spawn)", got)
	}
}

// TestRun202550UpstreamCodeWriterResolution locks the helper: nearest upstream
// agent.code from audit is implement (not test_signatures-style farther
// writers); unknown nodes and writer-less chains resolve empty.
func TestRun202550UpstreamCodeWriterResolution(t *testing.T) {
	edges, nodes := run202550DualHubChain()
	if got := upstreamCodeWriterForNode(edges, nodes, "audit"); got != "implement" {
		t.Fatalf("upstream writer of audit = %q, want implement", got)
	}
	if got := upstreamCodeWriterForNode(edges, nodes, "validate"); got != "implement" {
		t.Fatalf("upstream writer of validate = %q, want implement", got)
	}
	if got := upstreamCodeWriterForNode(edges, nodes, "no-such-node"); got != "" {
		t.Fatalf("unknown node must resolve empty, got %q", got)
	}
	if got := upstreamCodeWriterForNode(edges, nodes, ""); got != "" {
		t.Fatalf("empty node must resolve empty, got %q", got)
	}
	hubOnly := []agentpack.FlowEdge{{From: "plan_synthesis", To: "synthesis", When: "done", Kind: "forward"}}
	hubNodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	if got := upstreamCodeWriterForNode(hubOnly, hubNodes, "synthesis"); got != "" {
		t.Fatalf("writer-less chain must resolve empty, got %q", got)
	}
}

// TestRun202550MissingCAReasonMatcher locks the reason predicate: the live
// audit message matches; unrelated parks do not.
func TestRun202550MissingCAReasonMatcher(t *testing.T) {
	for _, yes := range []string{
		"Audit gate (aggregate): Flow gate: code changed but no change-audit note found.",
		"flow gate: code changed but no change-audit note found",
		"Code changed but no change-audit note found.",
	} {
		if !isMissingChangeAuditNoteReason(yes) {
			t.Errorf("reason %q must match as missing-CA", yes)
		}
	}
	for _, no := range []string{
		"",
		"flow scope drift: wrote outside the frozen contract's declared paths: a.go",
		"cap 3 reached with 2 open issue(s)",
		"hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)",
		"Audit gate (aggregate): something else failed.",
	} {
		if isMissingChangeAuditNoteReason(no) {
			t.Errorf("reason %q must NOT match as missing-CA", no)
		}
	}
}

package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Sprint-boundary Continue gate: a vibe-sprint audit that completes with plan
// tasks left must park an ok/cancel form for the next sprint in the same
// session — not settle done silently (live run-223416 / CA-817 run-635006 ran
// 1 sprint then done and the remaining tasks never started).
//
// New file in e521075; extended afterwards. The TestVibeSprintBoundary_
// ReopenReparks rework below is an intentional spec inversion (sealed-done
// with tasks left changed from "never re-park" to "one Continue offer unless
// declined"), called out in CA-819 and re-reviewed — not a silent weakening.
//
// The park/decision paths take no providerKey (Agnostic Case 1, same
// precedent as CA-814/817/818); the ok-path is additionally matrixed over
// Claude/Codex/Grok run keys.

var boundaryTestPlan = []string{
	"requirements/08-Task/todo/Task-904-sprint1-grid-snake-food-collision.md",
	"requirements/08-Task/todo/Task-905-sprint2-tick-loop-wasd-input.md",
	"requirements/08-Task/todo/Task-906-sprint3-score-gameover-run.md",
}

// boundaryTestPlan10 isolates the budget gate from the last-sprint gate:
// index 8 of 10 with budget 8 is budget-hit but not plan-empty.
var boundaryTestPlan10 = []string{
	"requirements/08-Task/todo/Task-901.md",
	"requirements/08-Task/todo/Task-902.md",
	"requirements/08-Task/todo/Task-903.md",
	"requirements/08-Task/todo/Task-904.md",
	"requirements/08-Task/todo/Task-905.md",
	"requirements/08-Task/todo/Task-906.md",
	"requirements/08-Task/todo/Task-907.md",
	"requirements/08-Task/todo/Task-908.md",
	"requirements/08-Task/todo/Task-909.md",
	"requirements/08-Task/todo/Task-910.md",
}

func armBoundaryRun(t *testing.T, svc *InteractiveService, pk ProviderKey, mode string, plan []string, index int) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk,
		WorkingMode: mode, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := parent.RunID
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.vibeTaskPlan = append([]string(nil), plan...)
	rs.vibeSprintIndex = index
	rs.vibeSprintBudget = defaultVibeSprintBudget
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusDone)
	return runID
}

func mustBoundaryPending(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil || !rs.vibeSprintBoundaryPending {
		t.Fatalf("want boundary pending, run=%+v", rs)
	}
	if got := rs.vibeSprintBoundaryTask; !strings.Contains(got, "Task-905") {
		t.Fatalf("boundary task=%q want Task-905", got)
	}
}

func TestVibeSprintBoundary_ParksWhenTasksRemain(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)

	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("sprint 1/3 done must park the boundary gate")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q want blocked/%s", loop.Status, loop.BlockReason, vibeSprintBoundaryReason)
	}
	if !strings.Contains(loop.GateReason, "Sprint 1/3 done") || !strings.Contains(loop.GateReason, "Task-905") {
		t.Fatalf("gateReason=%q must name finished + next sprint", loop.GateReason)
	}
	mustBoundaryPending(t, svc, runID)
	svc.mu.Lock()
	idx := svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if idx != 1 {
		t.Fatalf("index=%d want 1 (peek must not consume)", idx)
	}
	if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusDone {
		t.Fatalf("audit=%v want DONE (CA-818: stamp before park)", got)
	}
	view, apiErr := svc.runSnapshot(runID)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %v", apiErr)
	}
	if view.PendingGate == nil || len(view.PendingGate.GateOptions) != 2 {
		t.Fatalf("PendingGate=%+v want ok/cancel", view.PendingGate)
	}
	if !strings.Contains(view.PendingGate.ResumeFrom, "sprint 2/3") || !strings.Contains(view.PendingGate.ResumeFrom, "Task-905") {
		t.Fatalf("ResumeFrom=%q want next-sprint label", view.PendingGate.ResumeFrom)
	}
	// Idempotent: a second park is a no-op, never a second gate.
	if svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("second park must be a no-op while pending")
	}
}

func TestVibeSprintBoundary_NoParkShapes(t *testing.T) {
	cases := []struct {
		name  string
		mode  string
		plan  []string
		index int
	}{
		{"last sprint settles done", workingmode.Vibe, boundaryTestPlan, 3},
		{"non-vibe never parks", workingmode.Dev, boundaryTestPlan, 1},
		{"budget hit settles done (not last sprint)", workingmode.Vibe, boundaryTestPlan10, 8},
		{"empty plan settles done", workingmode.Vibe, nil, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := armBoundaryRun(t, svc, ProviderKeyCodex, tc.mode, tc.plan, tc.index)
			if svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
				t.Fatal("must not park; existing settle behavior stands")
			}
			svc.mu.Lock()
			pending := svc.runs[runID].vibeSprintBoundaryPending
			svc.mu.Unlock()
			if pending {
				t.Fatal("boundary flag must stay clear")
			}
		})
	}
}

func TestVibeSprintBoundary_OkStartsNextSprint(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	pending, idx := rs.vibeSprintBoundaryPending, rs.vibeSprintIndex
	ref := rs.chatFlowRef
	svc.mu.Unlock()
	if pending {
		t.Fatal("ok must consume the park")
	}
	if idx != 2 {
		t.Fatalf("index=%d want 2 (next sprint taken)", idx)
	}
	if workingmode.BareFlowID(ref) != vibeSprintFlowID {
		t.Fatalf("chatFlowRef=%q want vibe-sprint", ref)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "running" {
		t.Fatalf("loop=%q want running", loop.Status)
	}
	// Sprint 2 really started: vibe-sprint topology is live and its entry
	// child was spawned (index+ref alone would also pass on a failed resolve).
	svc.mu.Lock()
	rsTopo := svc.runs[runID]
	hasTdd := runHasFlowNode(rsTopo, "tdd")
	hasCoder := runHasFlowNode(rsTopo, "coder")
	svc.mu.Unlock()
	if !hasTdd || !hasCoder {
		t.Fatal("active topology must be vibe-sprint (tdd+coder)")
	}
	if got := countChildrenWithLabel(svc, runID, "preflight_contract_plan"); got < 1 {
		t.Fatalf("preflight children=%d want >=1 (entry actually spawned)", got)
	}
}

// TestVibeSprintBoundary_OkMatrixProviderKeys runs the same ok-path with a
// run stamped per provider key. The boundary park/decision/start chain takes
// no providerKey (verified by grep: zero providerKey references in
// vibe_sprint_boundary.go), so all three keys must behave identically. Runs
// are injected directly: createRun gates claude/grok on controlled runtime,
// which the decision layer never touches.
func TestVibeSprintBoundary_OkMatrixProviderKeys(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := "run-boundary-" + string(pk)
			nodes := []agentpack.FlowNode{
				{ID: "validate", Behavior: "command.validate"},
				{ID: "audit", Behavior: "artifact.audit_draft"},
			}
			svc.mu.Lock()
			svc.runs[runID] = &interactiveRun{
				id:               runID,
				projectID:        "proj",
				providerKey:      pk,
				workingMode:      workingmode.Vibe,
				flowEngineDriven: true,
				autoOrchestrate:  true,
				vibeTaskPlan:     append([]string(nil), boundaryTestPlan...),
				vibeSprintIndex:  1,
				vibeSprintBudget: defaultVibeSprintBudget,
			}
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
			svc.reseedFlowStepRuntime(runID, nodes)
			svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusDone)
			if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
				t.Fatal("must park first")
			}
			if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
				t.Fatalf("ok: %v", apiErr)
			}
			svc.mu.Lock()
			rs := svc.runs[runID]
			pending, idx := rs.vibeSprintBoundaryPending, rs.vibeSprintIndex
			ref := rs.chatFlowRef
			svc.mu.Unlock()
			if pending {
				t.Fatal("ok must consume the park")
			}
			if idx != 2 {
				t.Fatalf("index=%d want 2", idx)
			}
			if workingmode.BareFlowID(ref) != vibeSprintFlowID {
				t.Fatalf("chatFlowRef=%q want vibe-sprint", ref)
			}
		})
	}
}

func TestVibeSprintBoundary_CancelSettlesDone(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "cancel", ""); apiErr != nil {
		t.Fatalf("cancel: %v", apiErr)
	}
	svc.mu.Lock()
	pending, idx := svc.runs[runID].vibeSprintBoundaryPending, svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if pending {
		t.Fatal("cancel must consume the park")
	}
	if idx != 1 {
		t.Fatalf("index=%d want 1 (no sprint started)", idx)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "done" {
		t.Fatalf("loop=%q want done (finished sprints stand)", loop.Status)
	}
}

func TestVibeSprintBoundary_EmptyContinueStartsNextSprint(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Blocked-chip [Retry] posts feedback "continue" (cmdContinueFlow).
	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	svc.mu.Lock()
	pending, idx := svc.runs[runID].vibeSprintBoundaryPending, svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if pending {
		t.Fatal("continue must consume the park")
	}
	if idx != 2 {
		t.Fatalf("index=%d want 2", idx)
	}
}

func TestVibeSprintBoundary_AuditAutoFinalizeParks(t *testing.T) {
	// Reported shape: sprint audit completes (here via the vibe missing-key
	// auto-finalize) with tasks left → boundary form, never silent done.
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	workspace := t.TempDir()
	initGitRepoForAuditFixture(t, workspace)
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	edges := []agentpack.FlowEdge{{From: "audit", To: "done", When: "done", Kind: "forward"}}
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = workspace
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.vibeTaskPlan = append([]string(nil), boundaryTestPlan...)
	rs.vibeSprintIndex = 1
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "", FeatureConfidence: ConfidenceUnresolved}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	if !svc.runAuditNode(context.Background(), parent.RunID, edges, nodes, auditNode, "snake tests green") {
		t.Fatal("runAuditNode must handle the audit completion")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status == "done" {
		t.Fatal("must NOT settle done silently with 2 sprints left")
	}
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q want blocked/%s", loop.Status, loop.BlockReason, vibeSprintBoundaryReason)
	}
	mustBoundaryPending(t, svc, parent.RunID)
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got != StepStatusDone {
		t.Fatalf("audit=%v want DONE", got)
	}
}

func TestVibeSprintBoundary_ReopenReparks(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Same-process reopen with the park still pending: re-derive is a no-op,
	// never a second gate.
	svc.maybeReparkVibeSprintBoundary(runID)
	mustBoundaryPending(t, svc, runID)
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q must stay on the single gate", loop.Status, loop.BlockReason)
	}

	// Sealed loop is covered by ReopenOffers/ReopenSkips tests; a finished
	// plan never re-parks even when sealed done.
	svc2, _ := newTestServer(t)
	doneID := armBoundaryRun(t, svc2, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 3)
	svc2.agentOrchestrator.setLoop(doneID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc2.maybeReparkVibeSprintBoundary(doneID)
	svc2.mu.Lock()
	pending := svc2.runs[doneID].vibeSprintBoundaryPending
	svc2.mu.Unlock()
	if pending {
		t.Fatal("finished plan must not re-park")
	}
}

func TestVibeSprintBoundary_PromptNoteHelpers(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"continue", ""},
		{"Continue", ""},
		{"  continue  ", ""},
		{"", ""},
		{"focus on WASD", "focus on WASD"},
		{"  focus on WASD  ", "focus on WASD"},
	} {
		if got := normalizeBoundaryNote(tc.in); got != tc.want {
			t.Errorf("normalizeBoundaryNote(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if got := vibeSprintPromptWithNote("Task-905", ""); got != "Task-905" {
		t.Errorf("empty note must leave prompt untouched, got %q", got)
	}
	if got := vibeSprintPromptWithNote("Task-905", "focus on WASD"); got != "Task-905\n\nOperator note:\nfocus on WASD" {
		t.Errorf("note not appended, got %q", got)
	}
}

func boundaryChildPrompt(t *testing.T, svc *InteractiveService, runID, label string) string {
	t.Helper()
	for _, cid := range svc.agentOrchestrator.listChildren(runID) {
		svc.mu.Lock()
		ch := svc.runs[cid]
		var prompt string
		if ch != nil && ch.label == label {
			prompt = ch.lastFullPrompt
			if prompt == "" {
				prompt = ch.pendingTurnPrompt
			}
		}
		svc.mu.Unlock()
		if prompt != "" {
			return prompt
		}
	}
	return ""
}

func TestVibeSprintBoundary_NoteReachesSprintPrompt(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if _, err := svc.resumeFlowWithFeedback(runID, "focus on WASD"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if got := boundaryChildPrompt(t, svc, runID, "preflight_contract_plan"); strings.Contains(got, "Operator note:\nfocus on WASD") {
			return
		}
		if time.Now().After(deadline) {
			kids := len(svc.agentOrchestrator.listChildren(runID))
			loop := svc.agentOrchestrator.loopStateFor(runID)
			t.Fatalf("entry child prompt never carried the operator note (children=%d loop=%q/%q)", kids, loop.Status, loop.BlockReason)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVibeSprintBoundary_ContinueCarriesNoNoteForRetry(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Blocked-chip [Retry] posts the literal "continue": it must start the
	// sprint WITHOUT an "Operator note:" suffix.
	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("continue: %v", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if got := boundaryChildPrompt(t, svc, runID, "preflight_contract_plan"); got != "" {
			if strings.Contains(got, "Operator note:") {
				t.Fatalf("Retry payload leaked into prompt: %.200q", got)
			}
			if !strings.Contains(got, "Task-905") {
				t.Fatalf("prompt %.200q must carry the Task-905 sprint", got)
			}
			return
		}
		if time.Now().After(deadline) {
			kids := len(svc.agentOrchestrator.listChildren(runID))
			loop := svc.agentOrchestrator.loopStateFor(runID)
			t.Fatalf("entry child never started (children=%d loop=%q/%q)", kids, loop.Status, loop.BlockReason)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVibeSprintBoundary_DoubleContinueStartsOnce(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	kidsBefore := countChildrenWithLabel(svc, runID, "preflight_contract_plan")
	if kidsBefore < 1 {
		t.Fatalf("preflight children=%d want >=1 (first ok must spawn)", kidsBefore)
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "running" {
		t.Fatalf("loop=%q want running after first ok", loop.Status)
	}
	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("second continue: %v", err)
	}
	svc.mu.Lock()
	idx := svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if idx != 2 {
		t.Fatalf("index=%d want 2 (no second take)", idx)
	}
	if got := countChildrenWithLabel(svc, runID, "preflight_contract_plan"); got != kidsBefore {
		t.Fatalf("preflight children=%d want %d (no second spawn)", got, kidsBefore)
	}
}

func TestVibeSprintBoundary_StopClearsPark(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if _, apiErr := svc.stopAgentLoop(runID); apiErr != nil {
		t.Fatalf("stop: %v", apiErr)
	}
	svc.mu.Lock()
	pending, idx := svc.runs[runID].vibeSprintBoundaryPending, svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if pending {
		t.Fatal("Stop must clear the boundary park")
	}
	// A late ok after Stop must not resurrect or start a sprint.
	_ = svc.SubmitGateDecision(runID, "ok", "")
	svc.mu.Lock()
	idxAfter := svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if idxAfter != idx || idxAfter != 1 {
		t.Fatalf("index=%d want 1 (no start after Stop)", idxAfter)
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "stopped" {
		t.Fatalf("loop=%q want stopped (Stop wins)", loop.Status)
	}
	view, apiErr := svc.runSnapshot(runID)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %v", apiErr)
	}
	if view.PendingGate != nil {
		t.Fatalf("PendingGate=%+v must be nil after Stop", view.PendingGate)
	}
}

func TestVibeSprintBoundary_InvalidOptionKeepsPark(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "bogus", ""); apiErr == nil {
		t.Fatal("bogus option must be rejected")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q must stay parked", loop.Status, loop.BlockReason)
	}
	mustBoundaryPending(t, svc, runID)
}

func TestVibeSprintBoundary_ReopenOffersOnSilentDone(t *testing.T) {
	// Live run-223416 shape: audit DONE, 1/3 sprints started, loop done via
	// the old silent settle (no boundary park ever fired, never declined).
	// Reopening must offer the Continue form in the same session.
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.maybeReparkVibeSprintBoundary(runID)
	mustBoundaryPending(t, svc, runID)
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q want blocked/%s", loop.Status, loop.BlockReason, vibeSprintBoundaryReason)
	}
	if !strings.Contains(loop.GateReason, "Task-905") {
		t.Fatalf("gateReason=%q must name the next sprint", loop.GateReason)
	}
}

func TestVibeSprintBoundary_ReopenSkipsDeclined(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.mu.Lock()
	svc.runs[runID].vibeSprintBoundaryDeclined = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("explicitly declined runs must never be re-offered")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "done" {
		t.Fatalf("loop=%q want done (untouched)", loop.Status)
	}
}

func TestVibeSprintBoundary_ReopenSkipsStopped(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "stopped", Cap: 3, RoundCap: 3})
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("Stop wins: stopped runs must not re-park")
	}
}

func TestVibeSprintBoundary_DeclineSetsMarker(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "cancel", ""); apiErr != nil {
		t.Fatalf("cancel: %v", apiErr)
	}
	svc.mu.Lock()
	declined := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if !declined {
		t.Fatal("cancel must record the durable decline marker")
	}
}

func TestVibeSprintBoundary_ContinueClearsMarker(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	svc.mu.Lock()
	svc.runs[runID].vibeSprintBoundaryDeclined = true
	svc.mu.Unlock()
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	declined, idx := rs.vibeSprintBoundaryDeclined, rs.vibeSprintIndex
	svc.mu.Unlock()
	if declined {
		t.Fatal("continue must clear a stale decline marker")
	}
	if idx != 2 {
		t.Fatalf("index=%d want 2", idx)
	}
}

func TestVibeSprintBoundary_DeclinedPersistsJSON(t *testing.T) { // The decline marker must survive the sessions.ndjson round trip or the
	// reopen offer cannot tell declined runs from silently-settled ones.
	rec := ndjsonSessionRecord{
		RunID:                      "run-x",
		VibeTaskPlan:               append([]string(nil), boundaryTestPlan...),
		VibeSprintIndex:            1,
		VibeSprintBoundaryDeclined: true,
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), "vibe_sprint_boundary_declined") {
		t.Fatalf("key missing in %s", raw)
	}
	var back ndjsonSessionRecord
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.VibeSprintBoundaryDeclined || back.VibeSprintIndex != 1 || len(back.VibeTaskPlan) != 3 {
		t.Fatalf("round trip lost fields: %+v", back)
	}
}

func TestVibeSprintBoundary_DeclinedSurvivesFullChain(t *testing.T) {
	// Thread the marker through every production copy on the persist path:
	// run -> ProviderSessionState -> record -> JSON -> state. (The final
	// reconstruct assignment is a single mirrored line, reviewed.)
	rs := &interactiveRun{
		id:                         "run-chain",
		workingMode:                workingmode.Vibe,
		vibeTaskPlan:               append([]string(nil), boundaryTestPlan...),
		vibeSprintIndex:            1,
		vibeSprintBudget:           defaultVibeSprintBudget,
		vibeSprintBoundaryDeclined: true,
	}
	st := sessionStateOf(rs)
	if !st.VibeSprintBoundaryDeclined || st.VibeSprintIndex != 1 || len(st.VibeTaskPlan) != 3 {
		t.Fatalf("sessionStateOf dropped fields: %+v", st)
	}
	rec := sessionRecordFrom(st)
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var recBack ndjsonSessionRecord
	if err := json.Unmarshal(raw, &recBack); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	back := sessionStateFromRecord(recBack)
	if !back.VibeSprintBoundaryDeclined || back.VibeSprintIndex != 1 || len(back.VibeTaskPlan) != 3 {
		t.Fatalf("chain lost fields: %+v", back)
	}
}

func TestVibeSprintBoundary_ReopenSkipsBlockedOtherReason(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", GateReason: "cap 3 reached with 0 open issue(s)",
		Cap: 3, RoundCap: 3,
	})
	svc.maybeReparkVibeSprintBoundary(runID)
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.BlockReason != "cap" {
		t.Fatalf("BlockReason=%q must stay cap (no card stealing)", loop.BlockReason)
	}
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("must not park over another gate's card")
	}
}

func TestVibeSprintBoundary_ReopenSkipsFinishedPlanAfterMutation(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	// Tasks deleted under a silent-done run: nothing left to offer.
	svc.mu.Lock()
	svc.runs[runID].vibeTaskPlan = append([]string(nil), boundaryTestPlan[:1]...)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("emptied plan must not re-offer")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "done" {
		t.Fatalf("loop=%q want done (untouched)", loop.Status)
	}
}

func TestVibeSprintBoundary_DeclineStopReopenKeepsDeclined(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "cancel", ""); apiErr != nil {
		t.Fatalf("cancel: %v", apiErr)
	}
	if _, apiErr := svc.stopAgentLoop(runID); apiErr != nil {
		t.Fatalf("stop: %v", apiErr)
	}
	svc.mu.Lock()
	declined := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if !declined {
		t.Fatal("Stop must preserve the decline marker")
	}
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("declined+stopped run must never re-offer")
	}
}

func TestVibeSprintBoundary_DualParkServesBoundary(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// A stale resume-confirm stacked underneath must not win the card or
	// the router: boundary-first everywhere.
	svc.mu.Lock()
	svc.runs[runID].vibeResumeConfirm = true
	svc.runs[runID].vibeResumeFromNode = "validate"
	svc.mu.Unlock()
	view, apiErr := svc.runSnapshot(runID)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %v", apiErr)
	}
	if view.PendingGate == nil || !strings.Contains(view.PendingGate.ResumeFrom, "sprint 2/3") {
		t.Fatalf("PendingGate=%+v must serve the boundary", view.PendingGate)
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	svc.mu.Lock()
	idx := svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if idx != 2 {
		t.Fatalf("index=%d want 2 (ok routed to boundary, not resume)", idx)
	}
}

func TestVibeSprintBoundary_SkipsShowNoGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		decline bool
		loop    AgentLoopState
	}{
		{"declined done shows no gate", true, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3}},
		{"stopped shows no gate", false, AgentLoopState{Status: "stopped", Cap: 3, RoundCap: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
			svc.mu.Lock()
			svc.runs[runID].vibeSprintBoundaryDeclined = tc.decline
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(runID, tc.loop)
			svc.maybeReparkVibeSprintBoundary(runID)
			view, apiErr := svc.runSnapshot(runID)
			if apiErr != nil {
				t.Fatalf("runSnapshot: %v", apiErr)
			}
			if view.PendingGate != nil {
				t.Fatalf("PendingGate=%+v must be nil (no Continue form)", view.PendingGate)
			}
		})
	}
}
func armResumeNodes(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
	}
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
}

func TestVibeSprintBoundary_ResumeConfirmSkipsDeclined(t *testing.T) {
	newDeclinedRun := func(t *testing.T, declined bool) (*InteractiveService, string) {
		t.Helper()
		svc, _ := newTestServer(t)
		parent, err := svc.createRun(StartRunInput{
			ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
			WorkingMode: "vibe", Client: "tui",
		})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		svc.mu.Lock()
		rs := svc.runs[parent.RunID]
		rs.flowEngineDriven = true
		rs.vibeSprintBoundaryDeclined = declined
		svc.mu.Unlock()
		svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
		armResumeNodes(t, svc, parent.RunID)
		return svc, parent.RunID
	}
	t.Run("declined suppresses resume park", func(t *testing.T) {
		svc, runID := newDeclinedRun(t, true)
		svc.maybeParkVibeResumeConfirm(runID)
		svc.mu.Lock()
		confirm := svc.runs[runID].vibeResumeConfirm
		svc.mu.Unlock()
		if confirm {
			t.Fatal("declined runs must not park resume-confirm")
		}
	})
	t.Run("control parks without decline", func(t *testing.T) {
		svc, runID := newDeclinedRun(t, false)
		svc.maybeParkVibeResumeConfirm(runID)
		svc.mu.Lock()
		confirm, from := svc.runs[runID].vibeResumeConfirm, svc.runs[runID].vibeResumeFromNode
		svc.mu.Unlock()
		if !confirm || from != "tdd" {
			t.Fatalf("confirm=%v from=%q want true/tdd", confirm, from)
		}
	})
}

func TestVibeSprintBoundary_ContinueBudgetReparks(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Budget exhausts under the park: continue must re-park budget, never
	// start a sprint or strand running-with-nothing.
	svc.mu.Lock()
	svc.runs[runID].vibeSprintBudget = 1
	svc.mu.Unlock()
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	svc.mu.Lock()
	pending, idx := svc.runs[runID].vibeSprintBoundaryPending, svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if pending {
		t.Fatal("budget path must consume the boundary park")
	}
	if idx != 1 {
		t.Fatalf("index=%d want 1 (no take on budget)", idx)
	}
	svc.mu.Lock()
	declinedBudget := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if declinedBudget {
		t.Fatal("budget path must preserve (not set) the decline marker")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "budget" {
		t.Fatalf("loop=%q/%q want blocked/budget", loop.Status, loop.BlockReason)
	}
	if got := countChildrenWithLabel(svc, runID, "preflight_contract_plan"); got != 0 {
		t.Fatalf("preflight children=%d want 0 (no sprint started)", got)
	}
}

func TestVibeSprintBoundary_ContinueLockedStaysParked(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// A lock re-arms under the park: continue must keep the gate parked so
	// the operator can Stop; the sprint index must not move.
	svc.mu.Lock()
	svc.runs[runID].vibeAwaitingLock = true
	svc.mu.Unlock()
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	mustBoundaryPending(t, svc, runID)
	svc.mu.Lock()
	idx := svc.runs[runID].vibeSprintIndex
	declinedLocked := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if idx != 1 {
		t.Fatalf("index=%d want 1 (no take while locked)", idx)
	}
	if declinedLocked {
		t.Fatal("locked path must preserve (not set) the decline marker")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q must stay on the boundary gate", loop.Status, loop.BlockReason)
	}
}

func TestVibeSprintBoundary_ContinueEmptiedPlanSettlesWithoutMarker(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Plan emptied under the park: the operator said go, so settle done
	// WITHOUT recording a decline — a restored plan must stay offerable.
	svc.mu.Lock()
	svc.runs[runID].vibeTaskPlan = append([]string(nil), boundaryTestPlan[:1]...)
	svc.mu.Unlock()
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	pending, declined := rs.vibeSprintBoundaryPending, rs.vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if pending {
		t.Fatal("emptied plan must consume the park")
	}
	if declined {
		t.Fatal("emptied plan must NOT record a decline (operator said go)")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "done" {
		t.Fatalf("loop=%q want done", loop.Status)
	}
}

func TestVibeSprintBoundary_CancelDeferredOnOpenCohort(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Open the barrier: cancel must keep the gate parked and report 409,
	// never half-settle with declined=true.
	svc.agentOrchestrator.registerCohortMember(runID, "cohort-1")
	apiErr := svc.SubmitGateDecision(runID, "cancel", "")
	if apiErr == nil {
		t.Fatal("cohort-deferred cancel must return 409")
	}
	if apiErr.code != "boundary_settle_deferred" {
		t.Fatalf("code=%q want boundary_settle_deferred", apiErr.code)
	}
	mustBoundaryPending(t, svc, runID)
	svc.mu.Lock()
	declined := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if declined {
		t.Fatal("deferred cancel must not record a decline")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q must stay on the boundary gate", loop.Status, loop.BlockReason)
	}
}

func TestVibeSprintBoundary_ReopenSkipsPaused(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "paused", Cap: 3, RoundCap: 3})
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("paused loops must defer the offer until unpause")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "paused" {
		t.Fatalf("loop=%q want paused (untouched)", loop.Status)
	}
}

func TestVibeSprintBoundary_DeclineRestoresOnRefusedSettle(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Burn this turn's one-decision slot so applyFlowControl refuses the
	// decline settle: the gate must come back, never strand.
	svc.mu.Lock()
	svc.runs[runID].currentTurnID = "turn-1"
	svc.runs[runID].lastFlowControlTurnID = "turn-1"
	svc.mu.Unlock()
	if svc.declineVibeSprintBoundary(runID) {
		t.Fatal("refused settle must return false")
	}
	mustBoundaryPending(t, svc, runID)
	svc.mu.Lock()
	declined := svc.runs[runID].vibeSprintBoundaryDeclined
	svc.mu.Unlock()
	if declined {
		t.Fatal("restored gate must not carry a decline marker")
	}
}

func TestVibeSprintBoundary_OkCustomTextReachesPrompt(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	// Gate ok with custom text must deliver it as the sprint note (same as
	// Revise/Continue-with-feedback).
	if apiErr := svc.SubmitGateDecision(runID, "ok", "focus on tick loop"); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		if got := boundaryChildPrompt(t, svc, runID, "preflight_contract_plan"); strings.Contains(got, "Operator note:\nfocus on tick loop") {
			return
		}
		if time.Now().After(deadline) {
			kids := len(svc.agentOrchestrator.listChildren(runID))
			loop := svc.agentOrchestrator.loopStateFor(runID)
			t.Fatalf("gate note never reached prompt (children=%d loop=%q/%q)", kids, loop.Status, loop.BlockReason)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVibeSprintBoundary_RunningReopenReDerives(t *testing.T) {
	// Restart-loss shape for a non-sealed loop: audit DONE durable, plan and
	// index durable, memory-only flag lost, loop running. Reopen must
	// re-derive the identical gate (the case dropped in the first rework).
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	svc.maybeReparkVibeSprintBoundary(runID)
	mustBoundaryPending(t, svc, runID)
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("loop=%q/%q want blocked/%s", loop.Status, loop.BlockReason, vibeSprintBoundaryReason)
	}
	if !strings.Contains(loop.GateReason, "Task-905") {
		t.Fatalf("gateReason=%q must name the next sprint", loop.GateReason)
	}
}

func TestVibeSprintBoundary_ActiveNodeLifecycle(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if got := svc.agentOrchestrator.loopStateFor(runID).ActiveNode; got != "audit" {
		t.Fatalf("ActiveNode=%q want audit while parked", got)
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	if got := svc.agentOrchestrator.loopStateFor(runID).ActiveNode; got != "" {
		t.Fatalf("ActiveNode=%q want empty after continue", got)
	}
}

func TestVibeSprintBoundary_PauseResumeKeepGate(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	svc.pauseAgentLoop(runID, "operator pause")
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("pause must not steal the boundary card: %q/%q", loop.Status, loop.BlockReason)
	}
	svc.resumeAgentLoop(runID)
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "blocked" || loop.BlockReason != vibeSprintBoundaryReason {
		t.Fatalf("resume must not dissolve the boundary card: %q/%q", loop.Status, loop.BlockReason)
	}
	mustBoundaryPending(t, svc, runID)
}

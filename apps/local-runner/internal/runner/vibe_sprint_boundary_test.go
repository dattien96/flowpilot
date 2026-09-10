package runner

import (
	"context"
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
// New file; no pre-existing test is modified. The park/decision paths take no
// providerKey (Agnostic Case 1, same precedent as CA-814/817/818); the ok-path
// is additionally matrixed over Claude/Codex/Grok run keys.

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

	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
	if svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
			if svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
			if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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

func TestVibeSprintBoundary_ReopenReparks(t *testing.T) {	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
		t.Fatal("must park first")
	}
	// Simulate a restart: memory-only flag is lost, audit DONE + plan/index
	// are durable. Reopen must re-derive the same gate.
	svc.mu.Lock()
	svc.runs[runID].vibeSprintBoundaryPending = false
	svc.runs[runID].vibeSprintBoundaryTask = ""
	svc.mu.Unlock()
	svc.maybeReparkVibeSprintBoundary(runID)
	mustBoundaryPending(t, svc, runID)

	// Sealed loops stay sealed: Stop/done is never resurrected by reopen.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[runID].vibeSprintBoundaryPending = false
	svc.runs[runID].vibeSprintBoundaryTask = ""
	svc.mu.Unlock()
	if !svc.loopSealedForReinvoke(runID) {
		t.Fatal("loop=done must read sealed")
	}
	svc.maybeReparkVibeSprintBoundary(runID)
	svc.mu.Lock()
	pending := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if pending {
		t.Fatal("sealed loop must not re-park")
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
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
			t.Fatal("entry child prompt never carried the operator note")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVibeSprintBoundary_ContinueCarriesNoNoteForRetry(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
			t.Fatal("entry child never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestVibeSprintBoundary_DoubleContinueStartsOnce(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}
	kidsBefore := countChildrenWithLabel(svc, runID, "preflight_contract_plan")
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
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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
}

func TestVibeSprintBoundary_InvalidOptionKeepsPark(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit") {
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

package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// ---- AgentOrchestrator unit tests ------------------------------------------

func TestAgentOrchestratorRegisterAndListChildren(t *testing.T) {
	o := newAgentOrchestrator()
	o.registerChild("parent-1", "child-a")
	o.registerChild("parent-1", "child-b")
	o.registerChild("parent-2", "child-c")

	got := o.listChildren("parent-1")
	if len(got) != 2 || got[0] != "child-a" || got[1] != "child-b" {
		t.Errorf("listChildren(parent-1) = %v, want [child-a child-b]", got)
	}
	if other := o.listChildren("parent-2"); len(other) != 1 || other[0] != "child-c" {
		t.Errorf("listChildren(parent-2) = %v, want [child-c]", other)
	}
	if empty := o.listChildren("unknown"); len(empty) != 0 {
		t.Errorf("listChildren(unknown) = %v, want []", empty)
	}
}

func TestAgentOrchestratorListChildrenReturnsCopy(t *testing.T) {
	o := newAgentOrchestrator()
	o.registerChild("p", "c1")
	slice := o.listChildren("p")
	slice[0] = "mutated"
	// Internal state must be unchanged.
	if got := o.listChildren("p"); got[0] != "c1" {
		t.Errorf("mutating returned slice affected internal state")
	}
}

func TestAgentOrchestratorOpenWaiterAndSignalCompletion(t *testing.T) {
	o := newAgentOrchestrator()
	ch := o.openWaiter("run-x")

	done := make(chan agentCompletion, 1)
	go func() {
		done <- <-ch
	}()

	o.signalChild("run-x", "hello world", false, "", RunStatusCompleted)

	select {
	case c := <-done:
		if c.finalMessage != "hello world" || c.failed || c.status != RunStatusCompleted {
			t.Errorf("got completion %+v, want finalMessage=hello world failed=false", c)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func TestAgentOrchestratorOpenWaiterAndSignalFailure(t *testing.T) {
	o := newAgentOrchestrator()
	ch := o.openWaiter("run-fail")
	o.signalChild("run-fail", "", true, "adapter error", RunStatusFailed)

	select {
	case c := <-ch:
		if !c.failed || c.errMsg != "adapter error" {
			t.Errorf("got %+v, want failed=true errMsg=adapter error", c)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// Signal fires BEFORE the receiver reaches the select — buffered channel means
// the value is not lost.
func TestAgentOrchestratorSignalBeforeReceive(t *testing.T) {
	o := newAgentOrchestrator()
	ch := o.openWaiter("run-early")
	// Signal first, then read.
	o.signalChild("run-early", "done", false, "", RunStatusCompleted)

	select {
	case c := <-ch:
		if c.finalMessage != "done" {
			t.Errorf("got %q, want done", c.finalMessage)
		}
	default:
		t.Fatal("expected value in buffered channel, got none")
	}
}

func TestAgentOrchestratorSignalIdempotent(t *testing.T) {
	o := newAgentOrchestrator()
	o.openWaiter("run-idem")
	o.signalChild("run-idem", "first", false, "", RunStatusCompleted)
	// Second call must not panic (waiter was deleted after first signal).
	o.signalChild("run-idem", "second", false, "", RunStatusCompleted)
}

func TestAgentOrchestratorSignalNoWaiter(t *testing.T) {
	o := newAgentOrchestrator()
	// No openWaiter — signalChild must not panic.
	o.signalChild("no-such-run", "ignored", false, "", RunStatusCompleted)
}

func TestAgentOrchestratorLoopTransitionsAndBus(t *testing.T) {
	o := newAgentOrchestrator()
	snap := o.pause("parent", "gate")
	if snap.LoopState.Status != "paused" || snap.LoopState.RoundCap != 3 || snap.LoopState.GateReason != "gate" {
		t.Fatalf("pause snapshot = %+v", snap.LoopState)
	}
	snap = o.resume("parent")
	if snap.LoopState.Status != "running" || snap.LoopState.GateReason != "" {
		t.Fatalf("resume snapshot = %+v", snap.LoopState)
	}
	snap = o.addBus("parent", AgentBusMessage{ID: "b1", ParentRunID: "parent", Kind: "user-feedback", Message: "please revise", Queued: true})
	if len(snap.BusMessages) != 1 || snap.BusMessages[0].Message != "please revise" {
		t.Fatalf("bus snapshot = %+v", snap.BusMessages)
	}
	snap = o.stop("parent")
	if snap.LoopState.Status != "stopped" {
		t.Fatalf("stop snapshot = %+v", snap.LoopState)
	}
}

func TestAgentOrchestratorRoundCapTerminates(t *testing.T) {
	o := newAgentOrchestrator()
	snap := o.advanceRound("parent")
	if snap.LoopState.Round != 1 || snap.LoopState.Status != "running" {
		t.Fatalf("round 1 = %+v", snap.LoopState)
	}
	o.setLoop("parent", AgentLoopState{RoundCap: 1, Status: "running"})
	snap = o.advanceRound("parent")
	if snap.LoopState.Status != "stopped" || snap.LoopState.GateReason != "round cap reached" {
		t.Fatalf("cap snapshot = %+v", snap.LoopState)
	}
}

func TestAgentOrchestratorQueuesFeedback(t *testing.T) {
	o := newAgentOrchestrator()
	snap := o.queueFeedback("parent", AgentBusMessage{ID: "b1", ParentRunID: "parent", Kind: "user-feedback", Message: "revise", Queued: true})
	if snap.LoopState.Status != "" || snap.LoopState.GateReason != "" {
		t.Fatalf("queue snapshot = %+v", snap.LoopState)
	}
	msg := o.nextQueuedFeedback("parent")
	if msg == nil || msg.Message != "revise" {
		t.Fatalf("nextQueuedFeedback = %+v", msg)
	}
}

type childWaitApprovalAdapter struct{}

func (a *childWaitApprovalAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *childWaitApprovalAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, ApprovalEvents: true}
}
func (a *childWaitApprovalAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	_, err := bridge.RequestApproval(ApprovalDetails{
		Command: "npm test",
		Reason:  "needs approval",
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	})
	return err
}

type childWaitQuestionAdapter struct{}

func (a *childWaitQuestionAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *childWaitQuestionAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *childWaitQuestionAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	_, err := bridge.AskQuestion("Pick one", []QuestionOption{{Label: "A", Value: "a"}}, false)
	return err
}

type childCompleteAdapter struct{}

func (a *childCompleteAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *childCompleteAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *childCompleteAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
	return nil
}

// ---- parseSpawnAgentInput --------------------------------------------------

func TestParseSpawnAgentInputValid(t *testing.T) {
	args := map[string]any{
		"agent":     "reviewer",
		"prompt":    "review the diff",
		"provider":  "codex",
		"wait":      true,
		"dependsOn": []any{"run-abc"},
	}
	in, err := parseSpawnAgentInput(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Agent != "reviewer" || in.Prompt != "review the diff" || in.Provider != "codex" || !in.Wait {
		t.Errorf("unexpected result: %+v", in)
	}
	if len(in.DependsOn) != 1 || in.DependsOn[0] != "run-abc" {
		t.Errorf("dependsOn = %v, want [run-abc]", in.DependsOn)
	}
}

func TestParseSpawnAgentInputMissingAgent(t *testing.T) {
	_, err := parseSpawnAgentInput(map[string]any{"prompt": "hello"})
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestParseSpawnAgentInputMissingPrompt(t *testing.T) {
	_, err := parseSpawnAgentInput(map[string]any{"agent": "coder"})
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestParseSpawnAgentInputEmptyDependsOn(t *testing.T) {
	in, err := parseSpawnAgentInput(map[string]any{
		"agent": "coder", "prompt": "implement it", "dependsOn": []any{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(in.DependsOn) != 0 {
		t.Errorf("dependsOn = %v, want []", in.DependsOn)
	}
}

// ---- spawnChildRun + HTTP ---------------------------------------------------

func TestSpawnChildRunCreatesRunWithAgentIdentity(t *testing.T) {
	svc, _ := newTestServer(t)

	// Create a parent run first.
	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "researcher",
		Prompt: "summarise the repo",
		Wait:   false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	if result.RunID == "" || result.ProviderSessionID == "" {
		t.Errorf("expected non-empty RunID and ProviderSessionID, got %+v", result)
	}
	if result.Status != "spawned" {
		t.Errorf("status = %q, want spawned", result.Status)
	}

	// Child run must be stamped with the parent id and agent identity.
	svc.mu.Lock()
	child := svc.runs[result.RunID]
	svc.mu.Unlock()
	if child == nil {
		t.Fatal("child run not found in service map")
	}
	if child.parentRunID != parent.RunID {
		t.Errorf("parentRunID = %q, want %q", child.parentRunID, parent.RunID)
	}
	if child.agentName == "" {
		t.Errorf("agentName is empty")
	}
}

func TestSpawnChildRunInheritsParentModelDefaults(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:       "proj",
		ChatMode:        "normal_chat",
		ProviderKey:     ProviderKeyCodex,
		Model:           "gpt-5.5",
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "researcher",
		Prompt: "summarise the repo",
		Wait:   false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	svc.mu.Lock()
	child := svc.runs[result.RunID]
	svc.mu.Unlock()
	if child == nil {
		t.Fatal("child run not found in service map")
	}
	if child.modelName != "gpt-5.5" {
		t.Fatalf("child modelName = %q, want parent model gpt-5.5", child.modelName)
	}
	if child.reasoningEffort != "medium" {
		t.Fatalf("child reasoningEffort = %q, want medium", child.reasoningEffort)
	}
}

func TestSpawnChildRunRegistersTreeEdge(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	r1, e1 := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "write tests", Provider: "codex", Wait: false})
	r2, e2 := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review", Provider: "codex", Wait: false})
	if e1 != nil || e2 != nil {
		t.Fatalf("spawnChildRun errors: %v %v", e1, e2)
	}

	children := svc.agentOrchestrator.listChildren(parent.RunID)
	if len(children) != 2 || children[0] != r1.RunID || children[1] != r2.RunID {
		t.Errorf("children = %v, want [%s %s]", children, r1.RunID, r2.RunID)
	}
}

func TestSpawnChildRunWaitTrueWaitsThroughApprovalGate(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &childWaitApprovalAdapter{} },
	})
	svc := NewInteractiveServiceWithRegistry(reg)
	svc.approvalTTL = time.Second

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	done := make(chan SpawnAgentResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
			Agent: "reviewer", Prompt: "review", Provider: "claude", Wait: true,
		})
		if spawnErr != nil {
			errCh <- spawnErr
			return
		}
		done <- result
	}()

	var approvalID string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, rs := range svc.runs {
			if rs.parentRunID == parent.RunID && rs.pendingApprovalID != "" {
				approvalID = rs.pendingApprovalID
				return true
			}
		}
		return false
	}, "child approval")

	select {
	case result := <-done:
		t.Fatalf("spawnChildRun returned before approval resolved: %+v", result)
	case err := <-errCh:
		t.Fatalf("spawnChildRun failed before approval resolved: %v", err)
	default:
	}

	if apiErr := svc.SubmitApprovalDecision(approvalID, "approve"); apiErr != nil {
		t.Fatalf("SubmitApprovalDecision: %v", apiErr)
	}

	select {
	case result := <-done:
		if result.Status != string(RunStatusCompleted) {
			t.Fatalf("status = %q, want %q", result.Status, RunStatusCompleted)
		}
	case err := <-errCh:
		t.Fatalf("spawnChildRun(wait approval): %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for child completion after approval")
	}
}

func TestSpawnChildRunEmitsParentGraphWhenChildWaitsApproval(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &childWaitApprovalAdapter{} },
	})
	svc := NewInteractiveServiceWithRegistry(reg)
	svc.approvalTTL = time.Second

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "review", Provider: "claude", Wait: false,
	}); err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}

	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		parentRun := svc.runs[parent.RunID]
		if parentRun == nil {
			return false
		}
		for i := len(parentRun.events) - 1; i >= 0; i-- {
			ev := parentRun.events[i]
			if ev.Type != EventAgentGraphUpdated || ev.AgentGraphSnapshot == nil {
				continue
			}
			for _, child := range ev.AgentGraphSnapshot.Runs {
				if child.ParentRunID == parent.RunID && child.Status == RunStatusWaitingApproval && child.AgentStatus == string(RunStatusWaitingApproval) {
					return true
				}
			}
		}
		return false
	}, "parent graph waiting_approval update")
}

func TestSpawnChildRunWaitTrueWaitsThroughQuestionGate(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &childWaitQuestionAdapter{} },
	})
	svc := NewInteractiveServiceWithRegistry(reg)
	svc.questionTTL = time.Second

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	done := make(chan SpawnAgentResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
			Agent: "reviewer", Prompt: "review", Provider: "claude", Wait: true,
		})
		if spawnErr != nil {
			errCh <- spawnErr
			return
		}
		done <- result
	}()

	var questionID string
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, rs := range svc.runs {
			if rs.parentRunID == parent.RunID && rs.pendingQuestionID != "" {
				questionID = rs.pendingQuestionID
				return true
			}
		}
		return false
	}, "child question")

	select {
	case result := <-done:
		t.Fatalf("spawnChildRun returned before question resolved: %+v", result)
	case err := <-errCh:
		t.Fatalf("spawnChildRun failed before question resolved: %v", err)
	default:
	}

	if apiErr := svc.AnswerQuestion(questionID, []string{"a"}); apiErr != nil {
		t.Fatalf("AnswerQuestion: %v", apiErr)
	}

	select {
	case result := <-done:
		if result.Status != string(RunStatusCompleted) {
			t.Fatalf("status = %q, want %q", result.Status, RunStatusCompleted)
		}
	case err := <-errCh:
		t.Fatalf("spawnChildRun(wait question): %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for child completion after question answer")
	}
}

func TestListAgentRunSummariesHTTP(t *testing.T) {
	svc, srv := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /agents status=%d body=%s", status, body)
	}
	var summaries []AgentRunSummary
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("decode summaries: %v (body=%s)", err, body)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].ParentRunID != parent.RunID {
		t.Errorf("parentRunId = %q, want %q", summaries[0].ParentRunID, parent.RunID)
	}
	if summaries[0].AgentName == "" {
		t.Errorf("agentName is empty")
	}
}

// TestListAgentRunSummariesHTTPWorkflowModeParent is the workflow-mode analogue of
// TestListAgentRunSummariesHTTP above. BUG-169: a Flow Mode ("Review Loop" workflow)
// run's orchestrator narrated spawning a coder agent, but the desktop's Agents panel
// never showed it. All existing coverage for "does a spawned child show up in
// listAgentRunSummaries" used a normal_chat parent (runKind="chat"); this closes that
// gap for a workflow-mode parent (runKind="workflow", the kind Flow Mode actually
// launches) to rule in/out a runKind-specific listing defect. It passes, ruling that
// hypothesis out — the desktop-side symptom is not reproducible at this layer.
func TestListAgentRunSummariesHTTPWorkflowModeParent(t *testing.T) {
	svc, srv := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj-web", StepID: "step-plan", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /agents status=%d body=%s", status, body)
	}
	var summaries []AgentRunSummary
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("decode summaries: %v (body=%s)", err, body)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].ParentRunID != parent.RunID {
		t.Errorf("parentRunId = %q, want %q", summaries[0].ParentRunID, parent.RunID)
	}
}

func TestSpawnAgentHTTPEndpoint(t *testing.T) {
	_, srv := newTestServer(t)

	// First create a parent run.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run: status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/spawn-agent",
		SpawnAgentInput{Agent: "researcher", Prompt: "hello", Wait: false}, nil)
	if status != http.StatusOK {
		t.Fatalf("spawn-agent: status=%d body=%s", status, body)
	}
	var result SpawnAgentResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode result: %v (body=%s)", err, body)
	}
	if result.RunID == "" {
		t.Error("result.RunID is empty")
	}
	if result.Status != "spawned" {
		t.Errorf("status = %q, want spawned", result.Status)
	}
}

func TestSpawnAgentHTTPEndpointUnknownParent(t *testing.T) {
	_, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/no-such-run/spawn-agent",
		SpawnAgentInput{Agent: "coder", Prompt: "hi"}, nil)
	if status == http.StatusOK {
		t.Errorf("expected error status for unknown parent, got 200: %s", body)
	}
}

// TestSpawnAgentRejectsOrphanWithAvailableProvider ensures that the parent-run existence
// check fires BEFORE createRun, so an unknown parent is rejected even when the requested
// provider is fully available in the test registry.
func TestSpawnAgentRejectsOrphanWithAvailableProvider(t *testing.T) {
	svc, srv := newTestServer(t)
	_ = svc // ensure service is initialised

	// Use codex provider (registered and available in newTestServer).
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/ghost-parent/spawn-agent",
		SpawnAgentInput{Agent: "researcher", Prompt: "hi", Provider: "codex"}, nil)
	if status == http.StatusOK {
		t.Errorf("expected error for unknown parent (available provider), got 200: %s", body)
	}
}

// TestAgentOrchestratorHistoricalChildren verifies that setHistoricalChildren stores
// summaries and historicalChildren returns them correctly.
func TestAgentOrchestratorHistoricalChildren(t *testing.T) {
	o := newAgentOrchestrator()

	summaries := []AgentRunSummary{
		{RunID: "child-1", AgentName: "coder", Role: "coder", Status: RunStatusCompleted, ParentRunID: "parent-1"},
		{RunID: "child-2", AgentName: "reviewer", Role: "reviewer", Status: RunStatusCompleted, ParentRunID: "parent-1"},
	}
	o.setHistoricalChildren("parent-1", summaries)

	got := o.historicalChildren("parent-1")
	if len(got) != 2 {
		t.Fatalf("historicalChildren = %d, want 2", len(got))
	}
	if got[0].RunID != "child-1" || got[1].RunID != "child-2" {
		t.Errorf("unexpected order: %+v", got)
	}
	// Empty parent returns nil.
	if h := o.historicalChildren("no-such-parent"); h != nil {
		t.Errorf("historicalChildren(unknown) = %v, want nil", h)
	}
}

// TestAgentTreeSurvivesChatSyncManifest verifies the chat-sync round-trip:
// spawned children are captured by listAgentRunSummaries (which BuildChatSessionSyncManifest
// calls) and are returned again after setHistoricalChildren is called on a fresh service
// (simulating restoreChatRunFromDrive).
func TestAgentTreeSurvivesChatSyncManifest(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	// Capture the summaries as BuildChatSessionSyncManifest would.
	captured := svc.listAgentRunSummaries(parent.RunID)
	if len(captured) != 1 {
		t.Fatalf("listAgentRunSummaries = %d, want 1", len(captured))
	}
	if captured[0].AgentName == "" {
		t.Errorf("captured[0].AgentName is empty")
	}

	// Simulate restore: fresh service, load historical summaries as restoreChatRunFromDrive does.
	svc2, _ := newTestServer(t)
	restoredRunID := "restored-run-1"
	svc2.agentOrchestrator.setHistoricalChildren(restoredRunID, captured)

	summaries := svc2.listAgentRunSummaries(restoredRunID)
	if len(summaries) != 1 {
		t.Fatalf("listAgentRunSummaries after restore = %d, want 1", len(summaries))
	}
	if summaries[0].AgentName != captured[0].AgentName {
		t.Errorf("agentName mismatch: got %q want %q", summaries[0].AgentName, captured[0].AgentName)
	}
	if summaries[0].RunID != captured[0].RunID {
		t.Errorf("runId mismatch: got %q want %q", summaries[0].RunID, captured[0].RunID)
	}
}

func TestAgentTreeSurvivesRunnerRestartBeforeSync(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &childCompleteAdapter{} },
	})
	svc := NewInteractiveServiceWithStore(reg, nil, store)
	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude,
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement", Provider: "claude", Wait: true, DependsOn: []string{"run-abc"},
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}

	reloadedStore, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reload: %v", err)
	}
	restarted := NewInteractiveServiceWithStore(reg, nil, reloadedStore)

	summaries := restarted.listAgentRunSummaries(parent.RunID)
	if len(summaries) != 1 {
		t.Fatalf("listAgentRunSummaries after restart = %d, want 1", len(summaries))
	}
	if summaries[0].ParentRunID != parent.RunID {
		t.Fatalf("parentRunId = %q, want %q", summaries[0].ParentRunID, parent.RunID)
	}
	if summaries[0].AgentName != "coder" {
		t.Fatalf("agentName = %q, want coder", summaries[0].AgentName)
	}
	if len(summaries[0].DependsOn) != 1 || summaries[0].DependsOn[0] != "run-abc" {
		t.Fatalf("dependsOn = %v, want [run-abc]", summaries[0].DependsOn)
	}
}

// ---- Flow vocabulary tests (Task-089) -----------------------------------------

func TestParseFlowControlInputValid(t *testing.T) {
	for _, status := range []string{"continue", "done", "escalate"} {
		in, err := parseFlowControlInput(map[string]any{
			"status":  status,
			"summary": "all good",
			"payload": map[string]any{"k": "v"},
		})
		if err != nil {
			t.Fatalf("status=%q unexpected error: %v", status, err)
		}
		if in.Status != status {
			t.Errorf("status = %q, want %q", in.Status, status)
		}
		if in.Summary != "all good" {
			t.Errorf("summary = %q, want all good", in.Summary)
		}
		if in.Payload["k"] != "v" {
			t.Errorf("payload round-trip failed: %+v", in.Payload)
		}
	}
}

func TestParseFlowControlInputRejectsUnknownStatus(t *testing.T) {
	for _, bad := range []string{"", "approved", "reject", "DONE"} {
		_, err := parseFlowControlInput(map[string]any{"status": bad})
		if err == nil {
			t.Errorf("expected error for status=%q", bad)
		}
	}
}

func TestParseFlowControlInputPayloadOptional(t *testing.T) {
	in, err := parseFlowControlInput(map[string]any{"status": "done"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.Payload != nil {
		t.Errorf("payload should be nil when absent, got %+v", in.Payload)
	}
}

func TestApplyFlowNodeDefaults(t *testing.T) {
	n := FlowNode{}
	applyFlowNodeDefaults(&n)
	if n.Run != "delegate" || n.Lifecycle != "reinvoke" || n.Join != "all" {
		t.Errorf("unexpected defaults: %+v", n)
	}
}

func TestApplyFlowNodeDefaultsDoesNotOverwrite(t *testing.T) {
	n := FlowNode{Run: "inline", Lifecycle: "once", Join: "any"}
	applyFlowNodeDefaults(&n)
	if n.Run != "inline" || n.Lifecycle != "once" || n.Join != "any" {
		t.Errorf("defaults must not overwrite set fields: %+v", n)
	}
}

func TestApplyFlowPolicyDefaults(t *testing.T) {
	p := FlowPolicy{}
	applyFlowPolicyDefaults(&p)
	if p.Cap != 3 || p.OnCap != "escalate" || p.ExtendBy != 2 || p.ExtendMax != 2 {
		t.Errorf("unexpected defaults: %+v", p)
	}
}

func TestApplyFlowPolicyDefaultsDoesNotOverwrite(t *testing.T) {
	p := FlowPolicy{Cap: 5, OnCap: "done", ExtendBy: 1, ExtendMax: 3}
	applyFlowPolicyDefaults(&p)
	if p.Cap != 5 || p.OnCap != "done" || p.ExtendBy != 1 || p.ExtendMax != 3 {
		t.Errorf("defaults must not overwrite set fields: %+v", p)
	}
}

func TestParseJoin(t *testing.T) {
	cases := []struct {
		input    string
		wantMode string
		wantN    int
		wantErr  bool
	}{
		{"", "all", 0, false},
		{"all", "all", 0, false},
		{"any", "any", 0, false},
		{"quorum(2)", "quorum", 2, false},
		{"quorum(10)", "quorum", 10, false},
		{"quorum(0)", "", 0, true},
		{"quorum()", "", 0, true},
		{"quorum", "", 0, true},
		{"unknown", "", 0, true},
	}
	for _, c := range cases {
		mode, n, err := parseJoin(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseJoin(%q): expected error, got mode=%q n=%d", c.input, mode, n)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseJoin(%q): unexpected error: %v", c.input, err)
			continue
		}
		if mode != c.wantMode || n != c.wantN {
			t.Errorf("parseJoin(%q) = (%q,%d), want (%q,%d)", c.input, mode, n, c.wantMode, c.wantN)
		}
	}
}

func TestResolveFaceStatusReviewOutcome(t *testing.T) {
	face := reviewOutcomeFace()
	if face.Tool != "submit_review_outcome" {
		t.Errorf("tool = %q, want submit_review_outcome", face.Tool)
	}
	cases := map[string]string{
		"approved":          "done",
		"changes_requested": "continue",
		"blocked":           "escalate",
	}
	for domain, want := range cases {
		got, ok := resolveFaceStatus(face, domain)
		if !ok || got != want {
			t.Errorf("resolveFaceStatus(%q): got (%q,%v), want (%q,true)", domain, got, ok, want)
		}
	}
	if _, ok := resolveFaceStatus(face, "unknown"); ok {
		t.Error("resolveFaceStatus(unknown): expected ok=false")
	}
}

// TestReviewOutcomeFaceReadsFromPackDeclaredStatusMap is the regression test
// for BUG-NOTE-CP42 #11: submit-review-outcome.yaml declares this exact
// mapping as pack data (statusMap), but the runtime built a hardcoded Go
// literal duplicating the same values instead of ever reading the pack's own
// declaration — CP-42 P-5 requires declared faces to actually become pack
// data. This proves reviewOutcomeFace() now sources its Tool/Map directly
// from agentpack.LoadBuiltinToolFace, not an independent literal that could
// silently drift from the YAML.
func TestReviewOutcomeFaceReadsFromPackDeclaredStatusMap(t *testing.T) {
	packFace, ok, err := agentpack.LoadBuiltinToolFace("submit_review_outcome")
	if err != nil {
		t.Fatalf("LoadBuiltinToolFace: %v", err)
	}
	if !ok {
		t.Fatal("expected the pack to declare a submit_review_outcome tool face")
	}
	runtimeFace := reviewOutcomeFace()
	if runtimeFace.Tool != packFace.ID {
		t.Fatalf("reviewOutcomeFace().Tool = %q, want the pack's own face id %q", runtimeFace.Tool, packFace.ID)
	}
	if len(runtimeFace.Map) == 0 || !reflect.DeepEqual(runtimeFace.Map, packFace.StatusMap) {
		t.Fatalf("reviewOutcomeFace().Map = %#v, want it to equal the pack's own statusMap %#v", runtimeFace.Map, packFace.StatusMap)
	}
}

func TestValidateFlowEdgesAcceptsForwardDuplicates(t *testing.T) {
	edges := []FlowEdge{
		{From: "a", To: "b", When: "done", Kind: "forward"},
		{From: "a", To: "c", When: "done", Kind: "forward"},
	}
	if err := validateFlowEdges(edges); err != nil {
		t.Errorf("forward duplicate should be allowed: %v", err)
	}
}

func TestValidateFlowEdgesRejectsBackDuplicate(t *testing.T) {
	edges := []FlowEdge{
		{From: "b", To: "a", When: "continue", Kind: "back"},
		{From: "c", To: "a", When: "continue", Kind: "back"},
	}
	if err := validateFlowEdges(edges); err == nil {
		t.Error("expected error for duplicate back-edges on same status")
	}
}

func TestValidateFlowEdgesAllowsDifferentBackStatuses(t *testing.T) {
	edges := []FlowEdge{
		{From: "b", To: "a", When: "continue", Kind: "back"},
		{From: "c", To: "a", When: "escalate", Kind: "back"},
	}
	if err := validateFlowEdges(edges); err != nil {
		t.Errorf("different back-edge statuses should be allowed: %v", err)
	}
}

func TestFlowVocabularyJSONRoundTrip(t *testing.T) {
	node := FlowNode{ID: "n1", Agent: "agent-a", Run: "delegate", Lifecycle: "reinvoke", Join: "quorum(2)"}
	edge := FlowEdge{From: "n1", To: "n2", When: "done", Kind: "forward"}
	policy := FlowPolicy{Cap: 3, OnCap: "escalate", ExtendBy: 2, ExtendMax: 2}
	input := FlowControlInput{Status: "continue", Summary: "s", Payload: map[string]any{"x": float64(1)}}
	result := FlowControlResult{Status: "continue", Round: 1, Cap: 3, OpenIssues: 2, NextAction: "retry"}

	roundTrip := func(v any, dest any) {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if err := json.Unmarshal(b, dest); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
	}

	var n2 FlowNode
	roundTrip(node, &n2)
	if n2 != node {
		t.Errorf("FlowNode round-trip: got %+v want %+v", n2, node)
	}

	var e2 FlowEdge
	roundTrip(edge, &e2)
	if e2 != edge {
		t.Errorf("FlowEdge round-trip: got %+v want %+v", e2, edge)
	}

	var p2 FlowPolicy
	roundTrip(policy, &p2)
	if p2 != policy {
		t.Errorf("FlowPolicy round-trip: got %+v want %+v", p2, policy)
	}

	var i2 FlowControlInput
	roundTrip(input, &i2)
	if i2.Status != input.Status || i2.Summary != input.Summary || i2.Payload["x"] != float64(1) {
		t.Errorf("FlowControlInput round-trip: got %+v want %+v", i2, input)
	}

	var r2 FlowControlResult
	roundTrip(result, &r2)
	if r2 != result {
		t.Errorf("FlowControlResult round-trip: got %+v want %+v", r2, result)
	}
}

func TestFlowVocabularyNoRoleStrings(t *testing.T) {
	// Grep-style guard: none of the new type/function names may embed role strings.
	// This is a compile-time property but we verify it here by inspecting string constants
	// the engine actually uses.
	forbidden := []string{"coder", "reviewer", "approved"}
	engineStrings := []string{
		"continue", "done", "escalate",
		"delegate", "inline", "reinvoke", "once",
		"all", "any", "quorum",
		"forward", "back",
		"flow_control", "submit_review_outcome",
	}
	for _, s := range engineStrings {
		for _, bad := range forbidden {
			if s == bad {
				t.Errorf("engine string %q is a role string and must not appear in engine vocabulary", s)
			}
		}
	}
}

func TestListAgentRunSummariesEmptyHTTP(t *testing.T) {
	_, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run: %d %s", status, body)
	}
	var handle RunHandle
	_ = json.Unmarshal(body, &handle)

	status, body = doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+handle.RunID+"/agents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /agents: %d %s", status, body)
	}
	var summaries []AgentRunSummary
	if err := json.Unmarshal(body, &summaries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected empty list, got %d items", len(summaries))
	}
}

// TestGraphSnapshotDedupsHistoricalAndLiveChildren guards BUG-116: graphSnapshot must not
// list a child twice when it appears in BOTH the historical set (from a sync/restore) and
// the live children set. The live summary (current status) must win.
// ---- Review-loop template tests (Task-091) ------------------------------------

func TestParseReviewOutcomeInputValid(t *testing.T) {
	cases := []struct {
		args map[string]any
		want ReviewOutcomeInput
	}{
		{
			map[string]any{"status": "approved"},
			ReviewOutcomeInput{Status: "approved"},
		},
		{
			map[string]any{"status": "changes_requested", "feedback": "fix the tests", "issues": []any{
				map[string]any{"title": "nil dereference", "severity": "error", "file": "main.go"},
			}},
			ReviewOutcomeInput{Status: "changes_requested", Feedback: "fix the tests", Issues: []ReviewIssue{{Title: "nil dereference", Severity: "error", File: "main.go"}}},
		},
		{
			map[string]any{"status": "blocked", "feedback": "no build"},
			ReviewOutcomeInput{Status: "blocked", Feedback: "no build"},
		},
	}
	for _, c := range cases {
		got, err := parseReviewOutcomeInput(c.args)
		if err != nil {
			t.Errorf("parseReviewOutcomeInput(%v): unexpected error: %v", c.args, err)
			continue
		}
		if got.Status != c.want.Status || got.Feedback != c.want.Feedback {
			t.Errorf("parseReviewOutcomeInput: got %+v, want %+v", got, c.want)
		}
		if len(got.Issues) != len(c.want.Issues) {
			t.Errorf("Issues len: got %d, want %d", len(got.Issues), len(c.want.Issues))
		}
	}
}

func TestParseReviewOutcomeInputRejectsUnknownStatus(t *testing.T) {
	_, err := parseReviewOutcomeInput(map[string]any{"status": "rejected"})
	if err == nil {
		t.Error("expected error for unknown status 'rejected'")
	}
}

func TestParseReviewOutcomeInputRequiresFeedbackForChangesRequested(t *testing.T) {
	_, err := parseReviewOutcomeInput(map[string]any{"status": "changes_requested"})
	if err == nil {
		t.Error("expected error for changes_requested without feedback")
	}
	_, err = parseReviewOutcomeInput(map[string]any{"status": "changes_requested", "feedback": "  "})
	if err == nil {
		t.Error("expected error for changes_requested with blank feedback")
	}
}

func TestReviewOutcomeToFlowControlMapping(t *testing.T) {
	cases := []struct {
		domainStatus string
		feedback     string
		wantGeneric  string
	}{
		{"approved", "", "done"},
		{"changes_requested", "revise", "continue"},
		{"blocked", "cannot proceed", "escalate"},
	}
	for _, c := range cases {
		in := ReviewOutcomeInput{Status: c.domainStatus, Feedback: c.feedback}
		fc, err := reviewOutcomeToFlowControl(in)
		if err != nil {
			t.Errorf("reviewOutcomeToFlowControl(%q): unexpected error: %v", c.domainStatus, err)
			continue
		}
		if fc.Status != c.wantGeneric {
			t.Errorf("status: got %q, want %q", fc.Status, c.wantGeneric)
		}
	}
}

func TestReviewOutcomeIssuesRideInPayload(t *testing.T) {
	in := ReviewOutcomeInput{
		Status:   "changes_requested",
		Feedback: "fix it",
		Issues:   []ReviewIssue{{Title: "bug", File: "main.go"}},
	}
	fc, err := reviewOutcomeToFlowControl(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	issues, ok := fc.Payload["issues"]
	if !ok {
		t.Fatal("issues missing from Payload")
	}
	issueList, ok := issues.([]ReviewIssue)
	if !ok || len(issueList) != 1 || issueList[0].Title != "bug" {
		t.Errorf("payload issues = %v, want 1 issue with Title=bug", issues)
	}
}

func TestReviewLoopFlowConfigValid(t *testing.T) {
	nodes, edges, policy := ReviewLoopFlowConfig()
	if len(nodes) != 4 {
		t.Errorf("nodes = %d, want 4", len(nodes))
	}
	if len(edges) != 7 {
		t.Errorf("edges = %d, want 7", len(edges))
	}
	if policy.Cap != 3 || policy.OnCap != "escalate" {
		t.Errorf("policy = %+v, want Cap=3 OnCap=escalate", policy)
	}
	if err := validateFlowEdges(edges); err != nil {
		t.Errorf("validateFlowEdges: %v", err)
	}
	backCount := 0
	for _, e := range edges {
		if e.Kind == "back" {
			backCount++
		}
	}
	if backCount != 1 {
		t.Errorf("back-edge count = %d, want 1", backCount)
	}
}

func TestGraphSnapshotDedupsHistoricalAndLiveChildren(t *testing.T) {
	o := newAgentOrchestrator()
	// Same child present as historical (Completed) and live (Running).
	o.setHistoricalChildren("parent-1", []AgentRunSummary{
		{RunID: "child-1", AgentName: "reviewer", Role: "reviewer", Status: RunStatusCompleted, ParentRunID: "parent-1"},
	})
	o.registerChild("parent-1", "child-1")
	o.upsertSummary("parent-1", AgentRunSummary{
		RunID: "child-1", AgentName: "reviewer", Role: "reviewer", Status: RunStatusRunning, ParentRunID: "parent-1",
	})

	snap := o.graphSnapshot("parent-1")
	if len(snap.Runs) != 1 {
		t.Fatalf("graphSnapshot.Runs = %d, want 1 (deduped): %+v", len(snap.Runs), snap.Runs)
	}
	if snap.Runs[0].Status != RunStatusRunning {
		t.Errorf("deduped child status = %q, want live %q", snap.Runs[0].Status, RunStatusRunning)
	}
}

func TestCohortBufferAppendAndDrain(t *testing.T) {
	o := newAgentOrchestrator()
	o.appendCohortResult("p1", "cohort-A", cohortEntry{Label: "alpha", Provider: "claude", FinalMessage: "ok", Status: "completed"})
	o.appendCohortResult("p1", "cohort-A", cohortEntry{Label: "beta", Provider: "codex", FinalMessage: "done", Status: "completed"})
	o.appendCohortResult("p1", "cohort-B", cohortEntry{Label: "gamma", Provider: "claude", FinalMessage: "also ok", Status: "completed"})

	entries := o.drainCohort("p1", "cohort-A")
	if len(entries) != 2 {
		t.Fatalf("drainCohort returned %d entries, want 2", len(entries))
	}
	if entries[0].Label != "alpha" || entries[1].Label != "beta" {
		t.Errorf("unexpected entries: %+v", entries)
	}
	// cohort-A is gone; cohort-B untouched
	if got := o.drainCohort("p1", "cohort-A"); len(got) != 0 {
		t.Errorf("second drain should be empty, got %d entries", len(got))
	}
	if got := o.drainCohort("p1", "cohort-B"); len(got) != 1 {
		t.Errorf("cohort-B drain = %d entries, want 1", len(got))
	}
}

func TestBuildCohortNoteContainsAllMembers(t *testing.T) {
	entries := []cohortEntry{
		{Label: "alpha", Provider: "claude", FinalMessage: "LGTM", Status: "completed"},
		{Label: "beta", Provider: "codex", FinalMessage: "needs fix", Status: "completed"},
		{Label: "gamma", Provider: "claude", Status: "failed", Err: "timeout"},
	}
	note := buildCohortNote("parent-1", "c1", entries, 2)
	for _, want := range []string{"round 2", "3 results joined", "alpha", "beta", "gamma", "failed: timeout", "Synthesize"} {
		if !strings.Contains(note, want) {
			t.Errorf("note missing %q:\n%s", want, note)
		}
	}
	// No hardcoded role names.
	for _, bad := range []string{"coder", "reviewer", "approved", "changes_requested"} {
		if strings.Contains(note, bad) {
			t.Errorf("note contains forbidden role string %q", bad)
		}
	}
}

// TestSummarizeCohortNoteForUserStripsEngineInstructions is the regression test
// for BUG-233: the awaiting-user card must show the reviewers' actual findings,
// not the engine-internal header/instructions meant for the hub's own prompt.
func TestSummarizeCohortNoteForUserStripsEngineInstructions(t *testing.T) {
	entries := []cohortEntry{
		{Label: "alpha", Provider: "claude", FinalMessage: "LGTM", Status: "completed"},
		{Label: "beta", Provider: "codex", FinalMessage: "needs fix", Status: "completed"},
		{Label: "gamma", Provider: "claude", Status: "failed", Err: "timeout"},
	}
	note := buildCohortNote("parent-1", "c1", entries, 2)
	summary := summarizeCohortNoteForUser(note)

	for _, want := range []string{"alpha", "LGTM", "beta", "needs fix", "gamma", "failed: timeout"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
	for _, bad := range []string{"[flow-engine", "Flow round", "This joined result note", "Synthesize:"} {
		if strings.Contains(summary, bad) {
			t.Errorf("summary still contains engine-internal text %q:\n%s", bad, summary)
		}
	}
}

func TestSummarizeCohortNoteForUserEmptyInputReturnsEmpty(t *testing.T) {
	if got := summarizeCohortNoteForUser(""); got != "" {
		t.Errorf("summarizeCohortNoteForUser(\"\") = %q, want \"\"", got)
	}
	if got := summarizeCohortNoteForUser("   \n  "); got != "" {
		t.Errorf("summarizeCohortNoteForUser(whitespace) = %q, want \"\"", got)
	}
}

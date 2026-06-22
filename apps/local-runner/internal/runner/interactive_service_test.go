package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func init() { fakeAdapterDelay = 0 }

func newTestServer(t *testing.T) (*InteractiveService, *httptest.Server) {
	t.Helper()
	svc := NewInteractiveService()
	svc.approvalTTL = 50 * time.Millisecond
	svc.questionTTL = 50 * time.Millisecond
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

func doJSON(t *testing.T, method, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.Bytes()
}

func startRun(t *testing.T, base string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: "proj-web", WorkflowID: "wf-feature", StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if h.RunID == "" || h.ProviderSessionID == "" {
		t.Fatalf("empty handle: %+v", h)
	}
	return h.RunID
}

func startProjectRun(t *testing.T, base, projectID, workflowID string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs", StartRunInput{ProjectID: projectID, WorkflowID: workflowID, StepID: "step-plan"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start project run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	return h.RunID
}

func sendTurn(t *testing.T, base, runID, scenario string, headers map[string]string) (int, string) {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs/"+runID+"/turns",
		map[string]any{"stepId": "step-plan", "prompt": "hi", "scenario": scenario}, headers)
	var out struct {
		TurnID string `json:"turnId"`
	}
	_ = json.Unmarshal(body, &out)
	return status, out.TurnID
}

func getSnapshot(t *testing.T, base, runID string) runSnapshotView {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/client/workflow-runs/"+runID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot status=%d body=%s", status, body)
	}
	var v runSnapshotView
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return v
}

type captureTurnAdapter struct {
	ch chan TurnRequest
}

func (a *captureTurnAdapter) Key() ProviderKey { return ProviderKeyClaude }
func (a *captureTurnAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true, SkillSelection: true}
}
func (a *captureTurnAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.ch <- req
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}

func TestChatModeSelectedControlsReachProviderTurnRequest(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:         ProviderKeyClaude,
		DisplayName: "Claude",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, SkillSelection: true, ApprovalEvents: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:       "proj-web",
		ProviderKey:     ProviderKeyClaude,
		Model:           "claude-sonnet-4-5",
		ReasoningEffort: "high",
		YoloMode:        true,
		ChatMode:        "normal_chat",
		Cwd:             "/Users/dev/project",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	if handle.StepID == "" {
		t.Fatalf("chat run must return synthetic step id: %+v", handle)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"prompt": "hello",
		"selectedSkills": []map[string]any{
			{"name": "planner", "source": "slash_picker"},
			{"name": "code-review", "source": "slash_picker"},
		},
		"reasoningEffort": "high",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	select {
	case req := <-capture.ch:
		if req.ModelName != "claude-sonnet-4-5" {
			t.Fatalf("ModelName = %q, want claude-sonnet-4-5", req.ModelName)
		}
		if req.ReasoningEffort != "high" {
			t.Fatalf("ReasoningEffort = %q, want high", req.ReasoningEffort)
		}
		if !req.YoloMode {
			t.Fatal("YoloMode = false, want true")
		}
		if req.Cwd != "/Users/dev/project" {
			t.Fatalf("Cwd = %q, want /Users/dev/project", req.Cwd)
		}
		if len(req.SelectedSkills) != 2 || req.SelectedSkills[0].Name != "planner" || req.SelectedSkills[1].Name != "code-review" {
			t.Fatalf("SelectedSkills = %+v, want planner + code-review", req.SelectedSkills)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for captured provider turn request")
	}
}

// TestChatModeTurnLevelControlsOverrideRunDefaults locks BUG-063: model + YOLO are
// per-turn in chat mode, so changing them between prompts reaches the provider turn
// request — they are no longer frozen at the run-level values captured at startRun.
func TestChatModeTurnLevelControlsOverrideRunDefaults(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 1)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-web",
		ProviderKey: ProviderKeyClaude,
		Model:       "model-a",
		YoloMode:    true,
		ChatMode:    "normal_chat",
		Cwd:         "/w",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// Turn 1 omits model/yolo → the run-level defaults (model-a, yolo=true) are used.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "one",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 1 status=%d body=%s", status, body)
	}
	req1 := <-capture.ch
	if req1.ModelName != "model-a" || !req1.YoloMode {
		t.Fatalf("turn 1 req = {model:%q yolo:%v}, want {model-a true}", req1.ModelName, req1.YoloMode)
	}
	waitFor(t, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[handle.RunID].turnInFlight
	}, "turn 1 to finish")

	// Turn 2 supplies new per-turn values → they override the run-level defaults.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID, "prompt": "two", "model": "model-b", "yoloMode": false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("turn 2 status=%d body=%s", status, body)
	}
	req2 := <-capture.ch
	if req2.ModelName != "model-b" || req2.YoloMode {
		t.Fatalf("turn 2 req = {model:%q yolo:%v}, want {model-b false}", req2.ModelName, req2.YoloMode)
	}
}

func TestAgentGraphRoutesExposeSnapshotAndControls(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-graph", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("graph status=%d body=%s", status, body)
	}
	var graph AgentGraphSnapshot
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if graph.LoopState.RoundCap != 3 {
		t.Fatalf("round cap = %d, want 3", graph.LoopState.RoundCap)
	}
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/pause", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", status, body)
	}
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("decode paused graph: %v", err)
	}
	if len(graph.Runs) == 0 || graph.Runs[0].RunID != spawned.RunID || graph.Runs[0].AgentName == "" || graph.Runs[0].Status == "" {
		t.Fatalf("pause graph lost run metadata: %+v", graph.Runs)
	}
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-loop/feedback", map[string]any{"message": "revise", "toRunId": "child-1"}, nil)
	if status != http.StatusOK {
		t.Fatalf("feedback status=%d body=%s", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent.RunID+"/agent-bus", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("bus status=%d body=%s", status, body)
	}
	var bus []AgentBusMessage
	if err := json.Unmarshal(body, &bus); err != nil {
		t.Fatalf("decode bus: %v", err)
	}
	foundFeedback := false
	for _, msg := range bus {
		if msg.Kind == "user-feedback" && msg.Message == "revise" {
			foundFeedback = true
			break
		}
	}
	if !foundFeedback {
		t.Fatalf("bus = %+v", bus)
	}
}

func TestSpawnChildEmitsGraphAndBusEvents(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false}); spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	status, body := doJSON(t, "GET", srv.URL+"/admin/workflow-runs/"+parent.RunID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin events status=%d body=%s", status, body)
	}
	var evs []ProviderEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	hasGraph := false
	hasBus := false
	for _, ev := range evs {
		if ev.Type == EventAgentGraphUpdated {
			hasGraph = true
		}
		if ev.Type == EventAgentBusMessage {
			hasBus = true
		}
	}
	if !hasGraph || !hasBus {
		t.Fatalf("events missing graph/bus updates: %+v", evs)
	}
}

type loopRestartAdapter struct {
	turns chan string
}

func (a *loopRestartAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *loopRestartAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *loopRestartAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	switch {
	case strings.Contains(strings.ToLower(req.Prompt), "review"):
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "changes requested"})
	default:
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coder complete"})
	}
	return nil
}

type gatedLoopAdapter struct {
	turns        chan string
	releaseCoder chan struct{}
}

func (a *gatedLoopAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *gatedLoopAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *gatedLoopAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	switch {
	case strings.Contains(strings.ToLower(req.Prompt), "implement"):
		<-a.releaseCoder
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coder complete"})
	case strings.Contains(strings.ToLower(req.Prompt), "review"):
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "changes requested"})
	default:
		bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	}
	return nil
}

func TestChangesRequestedRestartsCoderTurn(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &loopRestartAdapter{turns: make(chan string, 4)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	coder, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn coder: %v", spawnErr)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review", Provider: "codex", DependsOn: []string{coder.RunID}, Wait: false}); spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	seen := []string{}
	deadline := time.After(3 * time.Second)
	for len(seen) < 3 {
		select {
		case prompt := <-adapter.turns:
			seen = append(seen, prompt)
		case <-deadline:
			t.Fatalf("timed out waiting for turns, saw %v", seen)
		}
	}
	if !strings.Contains(strings.ToLower(seen[2]), "changes requested") {
		t.Fatalf("third turn prompt = %q, want restarted coder with review feedback", seen[2])
	}
	graph := svc.agentGraphSnapshot(parent.RunID)
	if graph.LoopState.Round != 1 {
		t.Fatalf("loop round = %d, want 1 after changes-requested retry", graph.LoopState.Round)
	}
}

func TestReviewerWaitsForDependencyAndResumeReleasesPendingTurn(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &gatedLoopAdapter{turns: make(chan string, 4), releaseCoder: make(chan struct{})}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.pauseAgentLoop(parent.RunID, "paused for test")
	coder, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "coder", Prompt: "implement", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn coder: %v", spawnErr)
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review", Provider: "codex", DependsOn: []string{coder.RunID}, Wait: false}); spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "implement") {
			t.Fatalf("first prompt = %q, want coder turn", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for coder turn")
	}
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected reviewer turn while paused: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	close(adapter.releaseCoder)
	time.Sleep(50 * time.Millisecond)
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected reviewer turn after coder completion while paused: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	graph := svc.agentGraphSnapshot(parent.RunID)
	if got := graph.LoopState.Status; got != "paused" {
		t.Fatalf("loop status = %q, want paused", got)
	}
	svc.resumeAgentLoop(parent.RunID)
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "review") || !strings.Contains(strings.ToLower(prompt), "coder complete") {
			t.Fatalf("reviewer prompt = %q, want resumed handoff", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reviewer turn after resume")
	}
}

type dependencyReleaseAdapter struct {
	turns           chan string
	releaseReviewer chan struct{}
}

func (a *dependencyReleaseAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *dependencyReleaseAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *dependencyReleaseAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	a.turns <- req.Prompt
	if strings.Contains(strings.ToLower(req.Prompt), "review this change") {
		<-a.releaseReviewer
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: req.Prompt + " complete"})
	return nil
}

func TestCompletedReviewerReleasesDependentTester(t *testing.T) {
	svc := NewInteractiveService()
	adapter := &dependencyReleaseAdapter{turns: make(chan string, 4), releaseReviewer: make(chan struct{})}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return adapter },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	reviewer, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{Agent: "reviewer", Prompt: "review this change", Provider: "codex", Wait: false})
	if spawnErr != nil {
		t.Fatalf("spawn reviewer: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "review this change") {
			t.Fatalf("first prompt = %q, want reviewer turn", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for reviewer turn")
	}
	if _, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:     "tester",
		Prompt:    "run the regression checks",
		Provider:  "codex",
		DependsOn: []string{reviewer.RunID},
		Wait:      false,
	}); spawnErr != nil {
		t.Fatalf("spawn tester: %v", spawnErr)
	}
	select {
	case prompt := <-adapter.turns:
		t.Fatalf("unexpected tester turn before reviewer completion: %q", prompt)
	case <-time.After(200 * time.Millisecond):
	}
	close(adapter.releaseReviewer)
	select {
	case prompt := <-adapter.turns:
		if !strings.Contains(strings.ToLower(prompt), "run the regression checks") || !strings.Contains(strings.ToLower(prompt), "review this change complete") {
			t.Fatalf("second prompt = %q, want dependent tester turn with reviewer handoff", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for dependent tester turn")
	}
}

// TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted guards BUG-106: when a child
// provider's turn/completed notification carries an empty FinalMessage but the actual text
// arrived via EventMessageCompleted streaming events, spawnChildRun(wait=true) must still
// return the child's text in SpawnAgentResult.FinalMessage (not an empty string).
func TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted(t *testing.T) {
	svc := NewInteractiveService()
	// Adapter emits the child text ONLY via EventMessageCompleted (no FinalMessage on
	// EventTurnCompleted) — reproducing Codex new-protocol turn/completed behaviour.
	reg := newProviderRegistry()
	msgOnlyAdapter := &msgOnlyTurnAdapter{reply: "CHILD_AGENT_DONE"}
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return msgOnlyAdapter },
	})
	svc.registry = reg

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:    "reviewer",
		Prompt:   "check it",
		Provider: "codex",
		Wait:     true,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	if result.FinalMessage != "CHILD_AGENT_DONE" {
		t.Fatalf("FinalMessage = %q, want %q", result.FinalMessage, "CHILD_AGENT_DONE")
	}
}

// msgOnlyTurnAdapter emits the child reply via EventMessageCompleted with an empty
// FinalMessage on EventTurnCompleted (reproduces Codex new-protocol turn/completed).
type msgOnlyTurnAdapter struct{ reply string }

func (a *msgOnlyTurnAdapter) Key() ProviderKey { return ProviderKeyCodex }
func (a *msgOnlyTurnAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *msgOnlyTurnAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventMessageCompleted, Text: a.reply})
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: ""}) // empty — the bug case
	return nil
}

// readTurnReqFor drains the shared capture channel until it sees the turn request for runID,
// skipping unrelated runs' turns (e.g. the spawned child's own first turn).
func readTurnReqFor(t *testing.T, ch chan TurnRequest, runID string) TurnRequest {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case req := <-ch:
			if req.RunID == runID {
				return req
			}
		case <-deadline:
			t.Fatalf("timed out waiting for turn request for run %s", runID)
		}
	}
}

// TestUISpawnInjectsContextIntoParentProviderTurn guards BUG-122: a child agent spawned from
// the desktop UI is invisible to the parent's provider conversation. The next parent turn's
// provider prompt must carry a system note naming the child so the parent agent can answer
// "which sub-agents did we start?", while the displayed prompt stays clean.
func TestUISpawnInjectsContextIntoParentProviderTurn(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Spawn from the desktop UI (the HTTP handler sets UIInitiated=true). wait=false so the
	// child runs in the background; its first turn flows through the same capture adapter.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/spawn-agent", map[string]any{
		"agent": "coder", "prompt": "do work", "provider": "claude", "wait": false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("spawn status=%d body=%s", status, body)
	}
	var spawn SpawnAgentResult
	if err := json.Unmarshal(body, &spawn); err != nil {
		t.Fatalf("decode spawn: %v", err)
	}
	// Let the child turn complete so both the spawn note and the result note are queued.
	waitTerminal(t, srv.URL, spawn.RunID)

	// Parent turn: its provider prompt must carry the injected sub-agent context.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "which sub-agents did we start?",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("parent turn status=%d body=%s", status, body)
	}

	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if !strings.Contains(parentReq.Prompt, "FlowPilot system note") || !strings.Contains(parentReq.Prompt, "coder") {
		t.Fatalf("parent provider prompt missing UI-spawn context: %q", parentReq.Prompt)
	}
	if !strings.HasSuffix(strings.TrimSpace(parentReq.Prompt), "which sub-agents did we start?") {
		t.Fatalf("user prompt should be appended after the context block: %q", parentReq.Prompt)
	}

	// The displayed prompt (turn_started event) stays clean — the note is provider-only.
	var displayed string
	for _, e := range adminEvents(t, srv.URL, parent.RunID) {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			displayed = e.Prompt
		}
	}
	if displayed != "which sub-agents did we start?" {
		t.Fatalf("displayed prompt should be clean, got %q", displayed)
	}

	// A second parent turn must NOT re-inject (the buffer was cleared after consumption).
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "thanks",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("second parent turn status=%d body=%s", status, body)
	}
	secondReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if strings.Contains(secondReq.Prompt, "FlowPilot system note") {
		t.Fatalf("context must be injected once, not re-sent: %q", secondReq.Prompt)
	}
}

// TestToolSpawnDoesNotInjectParentContext guards BUG-122: the AI spawn_agent tool path
// (UIInitiated=false) must NOT inject a context note — those spawns are already part of the
// parent's provider conversation as a tool call/result.
func TestToolSpawnDoesNotInjectParentContext(t *testing.T) {
	svc := NewInteractiveService()
	capture := &captureTurnAdapter{ch: make(chan TurnRequest, 8)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	// Direct service call mirrors the AI tool path: UIInitiated stays false.
	spawn, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "do work", Provider: "claude", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	waitTerminal(t, srv.URL, spawn.RunID)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent.RunID+"/turns", map[string]any{
		"stepId": "chat-" + parent.RunID, "prompt": "hello",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("parent turn status=%d body=%s", status, body)
	}
	parentReq := readTurnReqFor(t, capture.ch, parent.RunID)
	if strings.Contains(parentReq.Prompt, "FlowPilot system note") {
		t.Fatalf("tool-path spawn must not inject context, got prompt: %q", parentReq.Prompt)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func adminEvents(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/admin/workflow-runs/"+runID+"/events", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin events status=%d", status)
	}
	var evs []ProviderEvent
	if err := json.Unmarshal(body, &evs); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	return evs
}

func waitTerminal(t *testing.T, base, runID string) []ProviderEvent {
	t.Helper()
	var evs []ProviderEvent
	waitFor(t, func() bool {
		evs = adminEvents(t, base, runID)
		if len(evs) == 0 {
			return false
		}
		last := evs[len(evs)-1].Type
		return last == EventTurnCompleted || last == EventTurnFailed
	}, "terminal event")
	return evs
}

func assertSeqContiguous(t *testing.T, evs []ProviderEvent) {
	t.Helper()
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq not contiguous at %d: got %d (%s)", i, e.Seq, e.Type)
		}
	}
}

// ---- tests -----------------------------------------------------------------

func TestCatalogAndRegistry(t *testing.T) {
	svc, srv := newTestServer(t)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "Acme Web App") {
		t.Fatalf("projects status=%d body=%s", status, body)
	}
	status, body = doJSON(t, "GET", srv.URL+"/client/workflows/wf-feature/steps", nil, nil)
	if status != http.StatusOK || !strings.Contains(string(body), "step-plan") {
		t.Fatalf("steps status=%d body=%s", status, body)
	}

	// registry: codex available, claude/gemini placeholders
	status, body = doJSON(t, "GET", srv.URL+"/admin/providers", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("providers status=%d", status)
	}
	for _, want := range []string{`"codex"`, `"claude"`, `"gemini"`, `"available"`, `"placeholder"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("providers missing %s in %s", want, body)
		}
	}
	// disabled/placeholder adapters surface the typed error
	if _, err := svc.registry.Adapter(ProviderKeyClaude); err == nil {
		t.Fatal("expected UnsupportedProviderRuntimeError for claude")
	} else if _, ok := err.(*UnsupportedProviderRuntimeError); !ok {
		t.Fatalf("expected UnsupportedProviderRuntimeError, got %T", err)
	}
}

func TestNormalTurnPersistsWithSeq(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	if status, turnID := sendTurn(t, srv.URL, runID, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	evs := waitTerminal(t, srv.URL, runID)

	assertSeqContiguous(t, evs)
	if evs[0].Type != EventTurnStarted {
		t.Fatalf("first event = %s, want turn_started", evs[0].Type)
	}
	if evs[0].Prompt != "hi" {
		t.Fatalf("turn_started prompt = %q, want hi", evs[0].Prompt)
	}
	if last := evs[len(evs)-1]; last.Type != EventTurnCompleted {
		t.Fatalf("last event = %s, want turn_completed", last.Type)
	}
	if got := getSnapshot(t, srv.URL, runID).Status; got != RunStatusCompleted {
		t.Fatalf("status = %s, want completed", got)
	}

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	steps := store.steps[runID]
	if len(steps) != 1 || steps[0].Status != StepStatusDone {
		t.Fatalf("workflow steps = %+v, want one DONE step", steps)
	}
	if store.runStatus[runID] != RunStatusEngineDone {
		t.Fatalf("run status = %s, want DONE", store.runStatus[runID])
	}
	if store.applies < 2 {
		t.Fatalf("expected running + orchestrator transitions, got applies=%d", store.applies)
	}
	events := store.events[runID]
	if len(events) != 3 {
		t.Fatalf("persisted events = %d, want 3 non-delta events", len(events))
	}
	if events[0].Type != EventTurnStarted || events[1].Type != EventMessageCompleted || events[2].Type != EventTurnCompleted {
		t.Fatalf("persisted events = %+v, want turn_started/message_completed/turn_completed", events)
	}
	if events[0].Prompt != "hi" {
		t.Fatalf("persisted turn_started prompt = %q, want hi", events[0].Prompt)
	}
	for _, ev := range events {
		if ev.Type == EventMessageDelta {
			t.Fatalf("delta event should not be persisted: %+v", ev)
		}
	}
	replayed := streamEvents(t, srv.URL, runID, 0, 1)
	if len(replayed) != 1 || replayed[0].Type != EventTurnStarted || replayed[0].Prompt != "hi" {
		t.Fatalf("replayed first event = %+v, want turn_started with prompt", replayed)
	}
}

func TestProjectRunHistoryFiltersRunsByProject(t *testing.T) {
	_, srv := newTestServer(t)
	first := startProjectRun(t, srv.URL, "proj-web", "wf-feature")
	other := startProjectRun(t, srv.URL, "proj-mobile", "wf-feature")
	second := startProjectRun(t, srv.URL, "proj-web", "wf-feature")

	if status, turnID := sendTurn(t, srv.URL, first, "normal", nil); status != http.StatusOK || turnID == "" {
		t.Fatalf("send turn status=%d turnId=%q", status, turnID)
	}
	waitTerminal(t, srv.URL, first)

	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history len=%d want 2: %+v", len(history), history)
	}
	for _, item := range history {
		if item.ProjectID != "proj-web" {
			t.Fatalf("history included other project: %+v", item)
		}
		if item.RunID == other {
			t.Fatalf("history included excluded run %s", other)
		}
	}
	if history[0].RunID != first {
		t.Fatalf("first history run=%s want updated run %s (second created run was %s)", history[0].RunID, first, second)
	}
	if history[0].LastPrompt != "hi" || history[0].LastMessage == "" || history[0].Status != RunStatusCompleted {
		t.Fatalf("updated history item missing summary: %+v", history[0])
	}
}

func TestProjectRunHistoryExcludesLiveChildAgentRuns(t *testing.T) {
	_, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj-web", WorkflowID: "wf-feature", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	parent := handle.RunID

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+parent+"/spawn-agent", SpawnAgentInput{
		Agent:    "coder",
		Prompt:   "implement the small change",
		Provider: "codex",
		Wait:     false,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("spawn-agent status=%d body=%s", status, body)
	}
	var spawned SpawnAgentResult
	if err := json.Unmarshal(body, &spawned); err != nil {
		t.Fatalf("decode spawn result: %v", err)
	}
	if spawned.RunID == "" {
		t.Fatalf("spawn result missing run id: %+v", spawned)
	}

	status, body = doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != parent {
		t.Fatalf("history = %+v, want only parent %s; child %s must stay out of main history", history, parent, spawned.RunID)
	}

	status, body = doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+parent+"/agents", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("agents status=%d body=%s", status, body)
	}
	var agents []AgentRunSummary
	if err := json.Unmarshal(body, &agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 1 || agents[0].RunID != spawned.RunID || agents[0].ParentRunID != parent {
		t.Fatalf("agents = %+v, want child %s under parent %s", agents, spawned.RunID, parent)
	}
}

func TestProjectRunHistoryExcludesLiveOrphanAgentMetadataRuns(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["parent-run"] = &interactiveRun{
		id:                "parent-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-parent",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "Use spawn_agent exactly once with agent=\"reviewer\".",
		runKind:           "chat",
	}
	svc.runs["orphan-agent-run"] = &interactiveRun{
		id:                "orphan-agent-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-child",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "You are the reviewer sub-agent. Review the coder's diff.\n\nDo not use tools. Reply exactly: CHILD_AGENT_DONE.",
		lastMessage:       "CHILD_AGENT_DONE",
		runKind:           "chat",
		agentName:         "reviewer",
		role:              "reviewer",
		agentStatus:       string(RunStatusCompleted),
	}
	svc.mu.Unlock()

	history := svc.projectRunHistory("proj-web")

	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only parent; live orphan agent metadata run must stay out of main history", history)
	}
}

func TestProjectRunHistoryKeepsLiveRootRowsWithAgentMetadata(t *testing.T) {
	svc, _ := newTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	svc.mu.Lock()
	svc.runs["main-run"] = &interactiveRun{
		id:                "main-run",
		projectID:         "proj-web",
		workflowID:        "wf-feature",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "session-main",
		status:            RunStatusCompleted,
		createdAt:         now,
		updatedAt:         now,
		lastPrompt:        "Main agent prompt",
		lastMessage:       "Main agent response",
		runKind:           "chat",
		agentName:         "main",
		agentStatus:       string(RunStatusCompleted),
	}
	svc.mu.Unlock()

	history := svc.projectRunHistory("proj-web")

	if len(history) != 1 || history[0].RunID != "main-run" {
		t.Fatalf("history = %+v, want main root row even when live metadata contains agent-like fields", history)
	}
}

func TestProjectRunHistoryKeepsPersistedRootRowsWithAgentMetadataAfterRestart(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "main-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-main",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "Main agent prompt",
		LastMessage:       "Main agent response",
		RunKind:           "chat",
		AgentName:         "main",
		AgentStatus:       string(RunStatusCompleted),
	}); err != nil {
		t.Fatalf("seed main session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "main-run" {
		t.Fatalf("history = %+v, want persisted main root row after restart", history)
	}
}

func TestProjectRunHistoryExcludesPersistedChildAgentRuns(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "parent-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-parent",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "parent prompt",
	}); err != nil {
		t.Fatalf("seed parent session: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "child-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-child",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "child prompt",
		ParentRunID:       "parent-run",
		AgentName:         "coder",
		Role:              "coder",
	}); err != nil {
		t.Fatalf("seed child session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only persisted parent; child-run must stay out of main history", history)
	}
}

func TestProjectRunHistoryExcludesLegacyOrphanAgentPromptRuns(t *testing.T) {
	registry := DefaultProviderRegistry()
	catalog := newInteractiveCatalog()
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "parent-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-parent",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "Use spawn_agent exactly once with agent=\"reviewer\".",
	}); err != nil {
		t.Fatalf("seed parent session: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "orphan-child-run",
		ProjectID:         "proj-web",
		WorkflowID:        "wf-feature",
		ProviderSessionID: "session-child",
		ProviderKey:       ProviderKeyCodex,
		Status:            RunStatusCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
		LastPrompt:        "You are the reviewer sub-agent. Review the coder's diff adversarially for correctness, regressions, and missed edge cases. Return either APPROVED or CHANGES-REQUESTED with specific, actionable feedback.\n\nDo not use tools. Reply exactly: CHILD_AGENT_DONE.",
		LastMessage:       "CHILD_AGENT_DONE",
		RunKind:           "chat",
	}); err != nil {
		t.Fatalf("seed orphan child session: %v", err)
	}

	_, srv := newTestServerWith(t, registry, catalog, store)
	status, body := doJSON(t, "GET", srv.URL+"/client/projects/proj-web/workflow-runs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("history status=%d body=%s", status, body)
	}
	var history []runHistoryItem
	if err := json.Unmarshal(body, &history); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(history) != 1 || history[0].RunID != "parent-run" {
		t.Fatalf("history = %+v, want only parent; legacy orphan child must stay out of main history", history)
	}
}

func TestEventStreamReplayNoGapsOrDupes(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "tool-heavy", nil)
	all := waitTerminal(t, srv.URL, runID)
	maxSeq := all[len(all)-1].Seq

	// reconnect from the middle: replay only seq > after
	after := int64(3)
	want := int(maxSeq - after)
	got := streamEvents(t, srv.URL, runID, after, want)
	if len(got) != want {
		t.Fatalf("replay got %d events, want %d", len(got), want)
	}
	for i, e := range got {
		if e.Seq != after+int64(i)+1 {
			t.Fatalf("replay gap/dupe at %d: seq=%d", i, e.Seq)
		}
	}
}

func TestApprovalDenyThenApprove(t *testing.T) {
	_, srv := newTestServer(t)

	// deny
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	var approvalID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingApproval != nil {
			approvalID = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval")

	// invalid decision → 400
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "bogus"}, nil); st != http.StatusBadRequest {
		t.Fatalf("invalid decision status=%d, want 400", st)
	}
	// deny
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "deny"}, nil); st != http.StatusOK {
		t.Fatalf("deny status=%d", st)
	}
	// idempotent: second submit still 200 (first-write-wins)
	if st, _ := doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID+"/decision", map[string]string{"decision": "approve"}, nil); st != http.StatusOK {
		t.Fatalf("idempotent resubmit status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if !hasToolStatus(evs, "shell", "cancelled") {
		t.Fatal("deny: expected shell tool_completed cancelled")
	}

	// approve (fresh run)
	runID2 := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID2, "approval-required", nil)
	var approvalID2 string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID2)
		if s.PendingApproval != nil {
			approvalID2 = s.PendingApproval.ApprovalID
			return true
		}
		return false
	}, "pending approval 2")
	doJSON(t, "POST", srv.URL+"/client/approvals/"+approvalID2+"/decision", map[string]string{"decision": "approve"}, nil)
	evs2 := waitTerminal(t, srv.URL, runID2)
	if !hasToolStatus(evs2, "shell", "success") {
		t.Fatal("approve: expected shell tool_completed success")
	}
}

func TestQuestionAnswer(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	var qID string
	waitFor(t, func() bool {
		s := getSnapshot(t, srv.URL, runID)
		if s.PendingQuestion != nil {
			qID = s.PendingQuestion.QuestionID
			return true
		}
		return false
	}, "pending question")
	if st, _ := doJSON(t, "POST", srv.URL+"/client/questions/"+qID+"/answer", map[string]any{"choice": "css-modules"}, nil); st != http.StatusOK {
		t.Fatalf("answer status=%d", st)
	}
	evs := waitTerminal(t, srv.URL, runID)
	if evs[len(evs)-1].Type != EventTurnCompleted {
		t.Fatal("question: expected turn_completed after answer")
	}
}

func TestInterruptCancelsTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "waiting_question")

	if st, _ := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/interrupt", nil, nil); st != http.StatusOK {
		t.Fatalf("interrupt status=%d", st)
	}
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusCancelled }, "cancelled")
}

func TestConcurrentTurnConflict(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil) // blocks in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("concurrent turn status=%d, want 409", st)
	}
}

func TestIdempotentTurn(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	h := map[string]string{"Idempotency-Key": "k1"}
	_, t1 := sendTurn(t, srv.URL, runID, "question-required", h) // in-flight
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")
	st, t2 := sendTurn(t, srv.URL, runID, "question-required", h)
	if st != http.StatusOK || t2 != t1 {
		t.Fatalf("idempotent turn status=%d t1=%s t2=%s", st, t1, t2)
	}
}

func TestApprovalExpiry(t *testing.T) {
	_, srv := newTestServer(t) // TTL = 50ms
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)
	last := evs[len(evs)-1]
	if last.Type != EventTurnFailed || !last.Recoverable {
		t.Fatalf("expiry: last event=%s recoverable=%v, want turn_failed recoverable", last.Type, last.Recoverable)
	}
}

func TestAccountMismatch(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	svc.activeAccountID = "switched"

	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("turn after account switch status=%d, want 409", st)
	}
	if st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+runID+"/resume", nil, nil); st != http.StatusOK || !strings.Contains(string(body), "\"runId\":\"run-1\"") {
		t.Fatalf("resume after switch status=%d body=%s", st, body)
	}
}

// ---- helpers ---------------------------------------------------------------

func hasToolStatus(evs []ProviderEvent, tool, status string) bool {
	for _, e := range evs {
		if e.Type == EventToolCompleted && e.ToolName == tool && e.Status == status {
			return true
		}
	}
	return false
}

// streamEvents connects to the SSE endpoint and returns up to wantCount events,
// then cancels.
func streamEvents(t *testing.T, base, runID string, afterSeq int64, wantCount int) []ProviderEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	url := base + "/client/workflow-runs/" + runID + "/events/stream?afterSeq=" +
		strconv.FormatInt(afterSeq, 10)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream connect: %v", err)
	}
	defer resp.Body.Close()

	var out []ProviderEvent
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev ProviderEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		out = append(out, ev)
		if len(out) >= wantCount {
			cancel()
			return out
		}
	}
	return out
}

func TestSeedIDCounterAvoidsRunIDReuseAfterRestart(t *testing.T) {
	store := newFakeWorkflowStore()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Persisted runs from a previous runner session; run-17 is a child of run-9.
	for _, sess := range []ProviderSessionState{
		{RunID: "run-9", ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now, RunKind: "chat"},
		{RunID: "run-17", ParentRunID: "run-9", ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now, RunKind: "chat", AgentName: "reviewer"},
	} {
		if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
			t.Fatalf("seed session: %v", err)
		}
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	// The next minted run id must be numerically above every persisted run id so a new
	// chat cannot collide with a previous run and inherit its child agents. (BUG-117)
	id := svc.nextID("run")
	if n := numericIDSuffix(id); n <= 17 {
		t.Fatalf("nextID after seeding = %q (suffix %d), want suffix > 17", id, n)
	}
}

func TestNumericIDSuffix(t *testing.T) {
	cases := map[string]int64{"run-14": 14, "evt-1": 1, "run-": 0, "main-run": 0, "": 0, "bus-007": 7}
	for in, want := range cases {
		if got := numericIDSuffix(in); got != want {
			t.Errorf("numericIDSuffix(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestReconstructRunPreservesUpdatedAt guards BUG-118: opening (reconstructing) a persisted
// chat must keep its stored updatedAt — the in-memory history list reports rs.updatedAt, so
// seeding it with `now` made every opened chat jump to the top with the current date.
func TestReconstructRunPreservesUpdatedAt(t *testing.T) {
	store := newFakeWorkflowStore()
	persisted := "2026-06-19T10:00:00.000000000Z"
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-1",
		ProviderKey: ProviderKeyClaude,
		Status:      RunStatusCompleted,
		RunKind:     "chat",
		StartedAt:   "2026-06-19T09:00:00.000000000Z",
		UpdatedAt:   persisted,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	rs, apiErr := svc.loadPersistedRun("run-1")
	if apiErr != nil {
		t.Fatalf("loadPersistedRun: %v", apiErr)
	}
	if rs.updatedAt != persisted {
		t.Fatalf("reconstructRun updatedAt = %q, want preserved %q", rs.updatedAt, persisted)
	}
}

// TestAgentGraphUpdateDoesNotResetParentRunStatus guards BUG-120: agent_graph_updated and
// agent_bus_message are orchestration relays emitted on the PARENT run to refresh the Agents
// panel. They must NOT flip the parent's run status to running — otherwise an idle/completed
// parent shows a perpetual "running" spinner whenever a child agent emits graph activity.
func TestAgentGraphUpdateDoesNotResetParentRunStatus(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].status = RunStatusCompleted
	svc.mu.Unlock()

	// Child activity relays these on the parent run.
	svc.emitAgentGraph(parent.RunID, svc.agentGraphSnapshot(parent.RunID))
	svc.emitAgentBus(parent.RunID, AgentBusMessage{ID: "bus-1", ParentRunID: parent.RunID, Kind: "handoff", Message: "x"})

	svc.mu.Lock()
	got := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if got != RunStatusCompleted {
		t.Fatalf("parent status after orchestration relays = %q, want %q (must not flip to running)", got, RunStatusCompleted)
	}
}

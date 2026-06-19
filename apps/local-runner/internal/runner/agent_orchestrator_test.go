package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
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

	o.signalChild("run-x", "hello world", false, "")

	select {
	case c := <-done:
		if c.finalMessage != "hello world" || c.failed {
			t.Errorf("got completion %+v, want finalMessage=hello world failed=false", c)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for signal")
	}
}

func TestAgentOrchestratorOpenWaiterAndSignalFailure(t *testing.T) {
	o := newAgentOrchestrator()
	ch := o.openWaiter("run-fail")
	o.signalChild("run-fail", "", true, "adapter error")

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
	o.signalChild("run-early", "done", false, "")

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
	o.signalChild("run-idem", "first", false, "")
	// Second call must not panic (waiter was deleted after first signal).
	o.signalChild("run-idem", "second", false, "")
}

func TestAgentOrchestratorSignalNoWaiter(t *testing.T) {
	o := newAgentOrchestrator()
	// No openWaiter — signalChild must not panic.
	o.signalChild("no-such-run", "ignored", false, "")
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

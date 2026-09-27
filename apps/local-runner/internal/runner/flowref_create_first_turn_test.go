package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// FlowRef at run create (review finding): POST /client/workflow-runs with a
// "flowRef" stamps chatFlowRef on the run (createRun), but handleStartTurn's
// resolveWorkflowFlowRef only consulted rs.workflowID — empty for a
// flowRef-launched run — so the first turn fell through to a plain chat turn
// and the mounted flow never spawned a single node. The creation-time mount
// must engage the flow engine exactly like a turn-supplied flowRef.
func TestFlowRefAtRunCreateStartsFlowOnFirstTurn(t *testing.T) {
	const flowRef = "77777777-7777-7777-7777-777777777777"

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := newInteractiveCatalog()
	catalog.steps[flowRef] = []Step{
		{ID: "drafting", WorkflowID: flowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
	}
	svc, srv := newTestServerWith(t, reg, catalog, newFakeWorkflowStore())

	store := newFakeFlowDefinitionStore()
	store.byRef[flowRef] = FlowDefinitionRecord{
		FlowRef:   flowRef,
		Name:      "Create-Mount Flow",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID: "create-mount-two-step",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
				{ID: "final_check", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
			},
			Edges: []agentpack.FlowEdge{
				{From: "drafting", To: "final_check", When: "done", Kind: "forward"},
			},
		},
	}
	svc.SetFlowDefinitionStore(store)

	// Create the run with flowRef at the HTTP entry — no workflowId.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", map[string]any{
		"projectId":   "proj",
		"providerKey": string(ProviderKeyCodex),
		"flowRef":     flowRef,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("create run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// First turn carries no flowRef — the creation-time mount must resolve it.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+h.RunID+"/turns",
		map[string]any{"stepId": "turn-1", "prompt": "draft the feature"}, nil)
	if status != http.StatusOK && status != http.StatusAccepted {
		t.Fatalf("turn status=%d body=%s", status, body)
	}

	waitLoop(t, "flow entry child spawned from creation-time flowRef", 5*time.Second, func() bool {
		return countChildrenWithLabel(svc, h.RunID, "drafting") == 1
	})

	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	driven := rs != nil && rs.flowEngineDriven
	svc.mu.Unlock()
	if !driven {
		t.Fatal("run must be marked flow-engine-driven once the creation-time flowRef resolves")
	}
}

// A run created with flowRef=vibe-cp-ingest must still face BUG-399's intake
// fence on the first turn: resolveWorkflowFlowRef copies chatFlowRef into the
// turn's FlowRef BEFORE startTurn's validateVibeCpIngestSource runs, so a
// turn that names no CP-shaped source must reject 422 — and no provider turn
// may dispatch behind the fence.
func TestFlowRefAtRunCreateCpIngestRejectsMissingSource(t *testing.T) {
	var sends int64
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				atomic.AddInt64(&sends, 1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	_, srv := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", map[string]any{
		"projectId":   "proj",
		"providerKey": string(ProviderKeyCodex),
		"flowRef":     "vibe-cp-ingest",
		"workingMode": "vibe",
	}, map[string]string{"X-Client": "tui"})
	if status != http.StatusOK {
		t.Fatalf("create run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// Turn carries no flowRef and no CP source — the creation-time mount must
	// resolve into FlowRef and hit the invalid_cp_source fence.
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+h.RunID+"/turns",
		map[string]any{"stepId": "turn-1", "prompt": "ingest my plan"}, nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "invalid_cp_source") {
		t.Fatalf("want 422 invalid_cp_source for source-less cp-ingest turn, got status=%d body=%s", status, body)
	}
	if n := atomic.LoadInt64(&sends); n != 0 {
		t.Fatalf("rejected cp-ingest turn must never reach the provider, sends=%d", n)
	}
}

package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// bug430Service builds a service whose codex provider is an immediately-
// completing fake adapter so startTurn admission/dispatch is deterministic.
func bug430Service(t *testing.T) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "continued"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func bug430ParkedChat(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	handle, aerr := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
		Model:       "gpt-5.4-mini",
	})
	if aerr != nil {
		t.Fatalf("createRun: %v", aerr)
	}
	// Drift park: the loop blocks with the drift reason (drift_pause.go).
	svc.agentOrchestrator.mutateLoop(handle.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = DriftPauseBlockReason
		st.GateReason = "drift score 90 (signals: churn) at step \"s\" — confirm to continue"
		return st
	})
	return handle.RunID
}

// BUG-430(a) (live BUG-LIVE-UI-1, run-9): /continue on a drift-parked *chat*
// run flipped loopState to running but dispatched nothing —
// maybeAutoReinvokeHubWithNote no-ops on autoOrchestrate=false (plain chat
// runs), so the TUI sat at "in progress — Enter disabled" with a dead loop.
// A drift park means "confirm to continue": Continue must re-drive a real
// turn on the run (the feedback rides as the prompt).
func TestBug430_DriftParkedChatContinueDispatchesTurn(t *testing.T) {
	svc := bug430Service(t)
	runID := bug430ParkedChat(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	inflight := svc.runs[runID].turnInFlight
	svc.mu.Unlock()
	st := svc.agentOrchestrator.loopStateFor(runID)
	if !inflight {
		t.Fatalf("/continue on a drift-parked chat run dispatched no turn: turnInFlight=false loop=%q block=%q — composer soft-locks", st.Status, st.BlockReason)
	}
}

// BUG-430(a) failure leg: when the resume cannot dispatch (a pending approval
// still owns the run → startTurn 409s), the loop must re-park with the reason
// — never be left "running" with nothing in flight (that is the soft-lock).
func TestBug430_DriftParkedChatContinueReparksOnDispatchFailure(t *testing.T) {
	svc := bug430Service(t)
	runID := bug430ParkedChat(t, svc)

	svc.mu.Lock()
	svc.runs[runID].pendingApprovalID = "appr-stuck" // startTurn → 409 awaiting_user
	svc.mu.Unlock()

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" {
		t.Fatalf("failed dispatch must re-park the loop, got status=%q (dead running = soft-lock)", st.Status)
	}
	if strings.TrimSpace(st.GateReason) == "" {
		t.Fatal("re-park must carry the dispatch failure reason")
	}
}

// BUG-430(a) route e2e: POST /client/workflow-runs/{id}/agent-loop/continue —
// the TUI's /continue path — must leave the drift-parked chat run with a turn
// in flight, not a dead running loop.
func TestBug430_ContinueRouteDispatchesDriftParkedChatTurn(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "continued"})
				return nil
			})
		},
	})
	svc, srv := newTestServerWith(t, reg, newInteractiveCatalog(), newFakeWorkflowStore())
	runID := bug430ParkedChat(t, svc)

	resp, err := http.Post(srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/continue", "application/json", strings.NewReader(`{"feedback":"continue"}`))
	if err != nil {
		t.Fatalf("continue POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("continue POST status=%d", resp.StatusCode)
	}
	svc.mu.Lock()
	inflight := svc.runs[runID].turnInFlight
	svc.mu.Unlock()
	if !inflight {
		t.Fatal("/continue route on a drift-parked chat run dispatched no turn — composer soft-locks")
	}
}

// BUG-429 server side: the flow-picker endpoint serves the user-startable dev
// harnesses — the catalog /flow must read (the chat-orchestration endpoint
// answers a different question and returns [] for flow picking).
func TestBug429_FlowPickerEndpointServesDevHarnesses(t *testing.T) {
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	_ = svc

	resp, err := http.Get(srv.URL + "/client/flow-picker-options?workingMode=dev")
	if err != nil {
		t.Fatalf("flow-picker GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("flow-picker GET status=%d", resp.StatusCode)
	}
	var opts []BuiltinFlowOption
	if err := json.NewDecoder(resp.Body).Decode(&opts); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, o := range opts {
		if o.FlowRef == "task-harness" {
			found = true
		}
	}
	if !found {
		t.Fatalf("flow-picker-options missing task-harness: %+v", opts)
	}
}

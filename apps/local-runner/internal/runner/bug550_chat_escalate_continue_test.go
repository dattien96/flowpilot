package runner

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// bug550ParkedChat builds a plain chat run parked the way a gate-reprompt-
// exhausted escalate leaves it: loop blocked, blockReason=escalate, the
// violation text in GateReason, no hub, no flow nodes, no children.
func bug550ParkedChat(t *testing.T, svc *InteractiveService) string {
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
	svc.agentOrchestrator.mutateLoop(handle.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = "Gate reprompt exhausted: Flow gate: Task/BUG document(s) without a Definition of Done checklist: requirements/08-Task/Task-001.md"
		return st
	})
	return handle.RunID
}

// BUG-550 (live run-2012663): a post-turn flow gate that exhausts its reprompt
// budget escalates via applyFlowControl, which parks even a PLAIN chat run
// (blocked/escalate + awaiting-user card). Pressing Retry on that card went
// through resumeFlowWithFeedback, flipped the loop to running and emitted
// agent_graph_updated — then dispatched nothing: the drift-park branch only
// matched DriftPauseBlockReason, the maybeAutoReinvokeHubWithNote tail no-ops
// on autoOrchestrate=false, and the hub-stall watchdog refuses non-flow runs
// (!rs.flowEngineDriven). Result: a dead "running" loop and a soft-locked
// composer. Retry on an escalate-parked chat run must re-drive a real turn.
func TestBug550_GateEscalatedChatContinueDispatchesTurn(t *testing.T) {
	svc := bug430Service(t)
	runID := bug550ParkedChat(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	inflight := svc.runs[runID].turnInFlight
	svc.mu.Unlock()
	st := svc.agentOrchestrator.loopStateFor(runID)
	if !inflight {
		t.Fatalf("Retry on an escalate-parked chat run dispatched no turn: turnInFlight=false loop=%q block=%q — composer soft-locks", st.Status, st.BlockReason)
	}
}

// The re-driven turn must carry the outstanding gate violation so a bare
// Retry does not re-violate blind and burn straight back into escalate.
func TestBug550_EscalateResumePromptCarriesGateViolation(t *testing.T) {
	promptCh := make(chan string, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req.Prompt:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "continued"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	runID := bug550ParkedChat(t, svc)

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	select {
	case prompt := <-promptCh:
		if !strings.Contains(prompt, "Definition of Done") {
			t.Fatalf("resumed turn prompt must carry the outstanding gate violation, got: %q", prompt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no turn dispatched after escalate-park retry")
	}
}

// Failure leg: when the resume cannot dispatch (a pending approval still owns
// the run → startTurn 409s), the loop must re-park carrying the reason — and
// must keep the escalate blockReason, not get relabelled drift.
func TestBug550_EscalateContinueReparksOnDispatchFailure(t *testing.T) {
	svc := bug430Service(t)
	runID := bug550ParkedChat(t, svc)

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
	if st.BlockReason != "escalate" {
		t.Fatalf("re-park must preserve blockReason=escalate, got %q", st.BlockReason)
	}
	if strings.TrimSpace(st.GateReason) == "" {
		t.Fatal("re-park must carry the dispatch failure reason")
	}
}

// Route e2e: POST /client/workflow-runs/{id}/agent-loop/continue — the desktop
// Retry chip's path — must leave the escalate-parked chat run with a turn in
// flight, not a dead running loop.
func TestBug550_ContinueRouteDispatchesEscalatedChatTurn(t *testing.T) {
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
	runID := bug550ParkedChat(t, svc)

	resp, err := http.Post(srv.URL+"/client/workflow-runs/"+runID+"/agent-loop/continue", "application/json", strings.NewReader(`{"feedback":""}`))
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
		t.Fatal("/continue route on an escalate-parked chat run dispatched no turn — composer soft-locks")
	}
}

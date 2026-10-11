package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-665: a member leg left `running` with no in-flight turn, no queued
// intent, and no live gate is a dead dispatch — the run-675483 live shape
// (2h20m silence, all recovery levers no-op). The dead-dispatch sweep and
// live-work checks must not count it as live work, and the loop's blocked
// truth must surface on the run snapshot (the turn fence knew before the
// read API did).

func bug665Parent(t *testing.T, s *InteractiveService) *interactiveRun {
	t.Helper()
	parent := &interactiveRun{
		id: "run-parent", providerKey: ProviderKeyCodex, modelName: "gpt-5.4",
		providerAccountID: "cx-0", chatID: "chat-1", status: RunStatusRunning,
		runKind: "flow", legState: LegStateActive, flowEngineDriven: true,
	}
	s.mu.Lock()
	s.runs[parent.id] = parent
	s.mu.Unlock()
	return parent
}

func bug665Leg(s *InteractiveService, parentID, label string, status RunStatus) *interactiveRun {
	child := &interactiveRun{
		id: "run-leg", providerKey: ProviderKeyCodex, modelName: "gpt-5.4",
		providerAccountID: "cx-0", chatID: "chat-1", parentRunID: parentID,
		label: label, agentName: label, stepID: label,
		status:              status,
		runKind:             "flow",
		legState:            LegStateActive,
		lastProviderEventAt: time.Now().Add(-3 * time.Hour).UTC(), // live: 2h20m quiet
	}
	s.mu.Lock()
	s.runs[child.id] = child
	s.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID: child.id, AgentName: child.agentName, Label: child.label,
		Status: child.status, ParentRunID: parentID,
	})
	s.mu.Unlock()
	return child
}

// The live wedge: leg running, turn over, nothing queued — must NOT count
// as live dispatch so the sweep can re-drive it. vibeNodeHasLiveWork keeps
// its CLAIM semantic (a ghost leg still owns the step — BUG-1197): the two
// questions differ deliberately.
func TestBug665_DeadLegNotCountedLive(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	bug665Leg(s, parent.id, "coder", RunStatusRunning)

	if s.deadDispatchNonTerminalChildExists(parent.id, "coder") {
		t.Fatal("dead-dispatched quiet leg counted as live — suppresses its own re-drive")
	}
	if !s.vibeNodeHasLiveWork(parent.id, "coder") {
		t.Fatal("ghost leg still claims its node — BUG-1197 claim semantic must hold")
	}
}

// Near-miss shapes that MUST still count live — each is genuinely busy or a
// user-visible park, and suppressing the re-drive on them would be a
// regression (double dispatch / racing a real decision).
func TestBug665_LiveShapesStillCounted(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(c *interactiveRun)
		state RunStatus
	}{
		{"in-flight turn", func(c *interactiveRun) { c.turnInFlight = true }, RunStatusRunning},
		{"armed gate reprompt", func(c *interactiveRun) { c.pendingGateRepromptPrompt = "p"; c.pendingGateRepromptStepID = "s" }, RunStatusRunning},
		{"armed resume intent", func(c *interactiveRun) { c.pendingResumePrompt = "p" }, RunStatusRunning},
		{"pending approval", func(c *interactiveRun) { c.pendingApprovalID = "a-1" }, RunStatusRunning},
		{"queued turn prompt", func(c *interactiveRun) { c.pendingTurnPrompt = "p" }, RunStatusRunning},
		{"waiting user approval park", nil, RunStatusWaitingUserApr},
		{"waiting question park", nil, RunStatusWaitingQuestion},
		{"waiting approval park", nil, RunStatusWaitingApproval},
		{"starting", nil, RunStatusStarting},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := task448Service(t, ProviderKeyCodex)
			parent := bug665Parent(t, s)
			leg := bug665Leg(s, parent.id, "coder", tc.state)
			if tc.mut != nil {
				s.mu.Lock()
				tc.mut(leg)
				s.mu.Unlock()
			}
			if !s.deadDispatchNonTerminalChildExists(parent.id, "coder") {
				t.Fatalf("%s leg must count live — suppressing re-drive risks double dispatch", tc.name)
			}
		})
	}
}

// A leg carrying an armed pendingFlowGateSettle is parked-by-design, not
// dead dispatch — the settle machinery owns it (deferred while the parent
// is blocked per BUG-1177, re-driven by resumePendingFlowGate otherwise).
// Re-driving it would hit the awaiting_user fence and arm a bogus
// gate-reprompt, breaking the deferred-settle invariant.
func TestBug665_DeferredSettleLegCountsLive(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	leg := bug665Leg(s, parent.id, "coder", RunStatusRunning)
	s.mu.Lock()
	leg.pendingFlowGateSettle = true
	leg.pendingFlowGateTurnID = "turn-owed"
	leg.pendingFlowGateFinalMsg = "verdict emitted"
	s.mu.Unlock()
	if !s.deadDispatchNonTerminalChildExists(parent.id, "coder") {
		t.Fatal("settle-armed leg must count live — its disposition is owed, not lost")
	}
}

// Terminal legs stay invisible to the live check (unchanged contract).
func TestBug665_TerminalLegsNotCounted(t *testing.T) {
	for _, st := range []RunStatus{RunStatusCompleted, RunStatusFailed, RunStatusCancelled} {
		s := task448Service(t, ProviderKeyCodex)
		parent := bug665Parent(t, s)
		bug665Leg(s, parent.id, "coder", st)
		if s.deadDispatchNonTerminalChildExists(parent.id, "coder") {
			t.Fatalf("%s leg must not count as live", st)
		}
	}
}

// A leg quiet only because its turn just ended is still a ghost by the
// BUG-354 contract — the parent owns its next dispatch; freshness of the
// last event is not what makes it live (the live leg had events too — they
// were just 2h20m old).
func TestBug665_GhostDefinedByDispatchNotEventAge(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	leg := bug665Leg(s, parent.id, "coder", RunStatusRunning)
	s.mu.Lock()
	leg.lastProviderEventAt = time.Now().UTC()
	s.mu.Unlock()
	if s.deadDispatchNonTerminalChildExists(parent.id, "coder") {
		t.Fatal("turn-over leg with fresh last-event is still a dispatch ghost")
	}
}

// Hidden-gate surface: a blocked flow loop must report blocked on the run
// snapshot — the live run reported running while the turn fence already saw
// flow_awaiting_user.
func TestBug665_BlockedLoopSurfacesBlocked(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	s.agentOrchestrator.mutateLoop(parent.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "flow_awaiting_user"
		return st
	})
	view, apiErr := s.runSnapshot(parent.id)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %+v", apiErr)
	}
	if view.Status != RunStatus("blocked") {
		t.Fatalf("blocked loop must surface status=blocked, got %q", view.Status)
	}
}

// And the inverse: a running loop must not be masked by a stale status —
// the gate surface only overrides live-looking statuses.
func TestBug665_RunningLoopNotMaskedBlocked(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	s.agentOrchestrator.mutateLoop(parent.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	view, apiErr := s.runSnapshot(parent.id)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %+v", apiErr)
	}
	if view.Status != RunStatusRunning {
		t.Fatalf("running loop must surface status=running, got %q", view.Status)
	}
}

func bug665FlowServer(t *testing.T) (*InteractiveService, *httptest.Server, string) {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, srv := newTestServerWith(t, reg, nil, nil)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.autoOrchestrate = true
	prs.status = RunStatusRunning
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()
	return svc, srv, parent.RunID
}

// The live lever that no-oped: POST agent-loop/continue on a ghost member
// leg must redrive the leg through the parent's graph path — not return
// 200 while doing nothing (run-675483 was wedged 2h20m on this surface).
func TestBug665_ContinueOnGhostLegRedrives(t *testing.T) {
	svc, srv, parentID := bug665FlowServer(t)
	leg := bug665Leg(svc, parentID, "coder", RunStatusRunning)
	svc.agentOrchestrator.registerChild(parentID, leg.id)

	svc.mu.Lock()
	seq0 := leg.activationSeq
	svc.mu.Unlock()

	resp, herr := http.Post(srv.URL+"/client/workflow-runs/"+leg.id+"/agent-loop/continue",
		"application/json", strings.NewReader(`{"feedback":"wake"}`))
	if herr != nil {
		t.Fatalf("continue POST: %v", herr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("continue POST status=%d", resp.StatusCode)
	}
	svc.mu.Lock()
	seq1 := leg.activationSeq
	svc.mu.Unlock()
	if seq1 == seq0 {
		t.Fatal("continue on a ghost leg returned 200 but redrove nothing — the silent no-op BUG-665 captured")
	}
}

// Near-miss: a settle-armed leg is parked-by-design (BUG-1177) — continue
// must NOT redrive it through the ghost path.
func TestBug665_ContinueOnSettleArmedLegDoesNotRedrive(t *testing.T) {
	svc, srv, parentID := bug665FlowServer(t)
	leg := bug665Leg(svc, parentID, "coder", RunStatusRunning)
	svc.agentOrchestrator.registerChild(parentID, leg.id)
	svc.mu.Lock()
	leg.pendingFlowGateSettle = true
	leg.pendingFlowGateTurnID = "turn-owed"
	seq0 := leg.activationSeq
	svc.mu.Unlock()

	resp, herr := http.Post(srv.URL+"/client/workflow-runs/"+leg.id+"/agent-loop/continue",
		"application/json", strings.NewReader(`{"feedback":"wake"}`))
	if herr != nil {
		t.Fatalf("continue POST: %v", herr)
	}
	defer resp.Body.Close()
	svc.mu.Lock()
	seq1 := leg.activationSeq
	svc.mu.Unlock()
	if seq1 != seq0 {
		t.Fatal("settle-armed leg was redriven — its owed disposition belongs to the settle machinery, not a nudge")
	}
}

// Terminal status wins over a stale blocked loop stamp — never mask a
// truthful terminal surface.
func TestBug665_TerminalStatusNotMaskedBlocked(t *testing.T) {
	s := task448Service(t, ProviderKeyCodex)
	parent := bug665Parent(t, s)
	s.mu.Lock()
	parent.status = RunStatusCompleted
	s.mu.Unlock()
	s.agentOrchestrator.mutateLoop(parent.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		return st
	})
	view, apiErr := s.runSnapshot(parent.id)
	if apiErr != nil {
		t.Fatalf("runSnapshot: %+v", apiErr)
	}
	if view.Status != RunStatusCompleted {
		t.Fatalf("terminal status must win over blocked loop, got %q", view.Status)
	}
}

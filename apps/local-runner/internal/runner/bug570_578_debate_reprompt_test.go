package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// BUG-570 (live run-100368): a reprompt-class gate violation on a vibe child
// routes to the owner-debate resolver BEFORE the reprompt budget accounting —
// applyVibeGateResolver returns early and repromptAttempts is never
// incremented. The debate resolves, the gated child gets its reprompt, the
// next turn violates identically, and a fresh debate mounts again forever
// (the live tdd leg churned owner-debate rounds 4/5+ on zero-delta turns).
//
// The dev/child reprompt paths bound this at maxFlowGateReprompts and then
// escalate to the operator. The debate-routed reprompt must consume the same
// budget: a remediation reprompt IS the retry.

func bug570RepromptResult() flowgate.EnforceResult {
	return flowgate.EnforceResult{
		Action:  "reprompt",
		Message: "r-scope: out-of-scope writes",
		Violations: []flowgate.Violation{{
			Rule:   flowgate.Rule{ID: "r-scope", Action: "reprompt"},
			Detail: "write outside declared scope",
		}},
	}
}

func TestBUG570DebateRoutedRepromptConsumesBudget(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = bug567SprintGraph()
	prs.flowEngineDriven = true
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "tdd"
	crs.workingMode = workingmode.Vibe
	svc.mu.Unlock()

	// First reprompt-class violation: debate mounts, budget consumed once.
	if !svc.applyVibeGateResolver(child.RunID, parent.RunID, "turn-1", crs, bug570RepromptResult()) {
		t.Fatal("first reprompt-class violation should route to owner debate")
	}
	svc.mu.Lock()
	mounted := len(prs.vibeParkedNodes) > 0
	attempts1 := crs.repromptAttempts
	prs.vibeParkedNodes = nil // simulate the debate resolving + restore
	prs.vibeParkedEdges = nil
	svc.mu.Unlock()
	if !mounted {
		t.Fatal("first violation should have mounted the debate (parked sprint topology)")
	}
	if attempts1 != 1 {
		t.Fatalf("debate-routed reprompt must consume the reprompt budget: attempts=%d want 1", attempts1)
	}

	// Second violation: debate mounts again, budget consumed again.
	if !svc.applyVibeGateResolver(child.RunID, parent.RunID, "turn-2", crs, bug570RepromptResult()) {
		t.Fatal("second reprompt-class violation should route to owner debate")
	}
	svc.mu.Lock()
	mounted = len(prs.vibeParkedNodes) > 0
	prs.vibeParkedNodes = nil
	prs.vibeParkedEdges = nil
	svc.mu.Unlock()
	if !mounted {
		t.Fatal("second violation should have mounted the debate")
	}

	// Third identical violation: budget exhausted — escalate, do NOT mount
	// another debate (this is the live unbounded loop).
	if !svc.applyVibeGateResolver(child.RunID, parent.RunID, "turn-3", crs, bug570RepromptResult()) {
		t.Fatal("exhausted reprompt budget should still claim the turn (escalate)")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(prs.vibeParkedNodes) > 0 {
		t.Fatal("third reprompt-class violation mounted yet another debate — the loop is unbounded")
	}
	if prs.lastEscalatedInlineNodeID == "" {
		t.Fatal("exhausted debate-routed reprompt must escalate the node on the parent")
	}
}

// BUG-578 (live run-100368): a debate_synthesis escalate retried through the
// generic inline dispatch re-drives the hub turn with a bare "reached step"
// prompt — the owners' joined result note was consumed by the first
// synthesis turn, so the hub has nothing to synthesize and re-escalates
// identically forever. The retry must re-attach the last joined cohort note.
func TestBUG578DebateSynthesisRetryCarriesJoinedNote(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: "escalate", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.activeFlowNodes = bug567DebateGraph()
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "debate_synthesis", To: "done", When: "done", Kind: "forward"}}
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.lastEscalatedInlineNodeID = vibeDebateSynthesisNodeID
	rs.lastCohortNote = "[flow-engine joined result note]\nFlow round 0 — 2 results joined.\n\"owner_1\" (codex): reprompt needed\n  verdict=reprompt\n---\nThis joined result note is part of your current prompt context."
	svc.mu.Unlock()

	if _, err := svc.resumeFlowWithFeedback(parent.RunID, "retry"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	waitLoop(t, "debate_synthesis retry carries the owners' joined result note", 5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "[flow-engine joined result note]") && strings.Contains(p, "owner_1") {
				return true
			}
		}
		return false
	})
}

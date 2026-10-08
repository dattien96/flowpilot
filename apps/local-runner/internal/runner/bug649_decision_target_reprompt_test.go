package runner

import (
	"context"
	"testing"
)

// BUG-649 (live run-523131, Task-113 adjudication): the decisionTarget branch
// of SubmitGateDecision consumed pendingGateBlock but never armed the reprompt
// markers (proposalTurnPending / gateFixCodeActive) — only the direct-block
// branch did. The re-driven turn then re-evaluated the SAME stale gate block:
// three identical adjudication cards for one operator decision.

// blockingAdapter keeps the resumed turn in flight so the armed markers are
// still observable when the assertion runs (the flag is consumed inside
// runFlowGate at turn finalize).
func bug649BlockedService(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, _ TurnRequest, _ TurnBridge) error {
				<-ctx.Done()
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun: %d %q", cerr.status, cerr.code)
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.pendingGateBlock = &gateBlockInfo{stepID: "step-1", regressedTests: []string{"TestX"}}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "blocked", BlockReason: "gate", ActiveNode: "hub"})
	return svc, run.RunID
}

// The decision is accepted, the block consumed, and the proposal marker must
// be armed on the decision target — otherwise the resumed turn re-enters the
// gate as an ordinary turn and hits the stale block again.
func TestBug649_DecisionTargetArmsProposalTurn(t *testing.T) {
	svc, runID := bug649BlockedService(t)

	if e := svc.SubmitGateDecision(runID, "suggest-requirement-change", ""); e != nil {
		t.Fatalf("SubmitGateDecision: %d %q %q", e.status, e.code, e.msg)
	}

	svc.mu.Lock()
	rs := svc.runs[runID]
	armed := rs.proposalTurnPending
	block := rs.pendingGateBlock
	fixActive := rs.gateFixCodeActive
	svc.mu.Unlock()
	if block != nil {
		t.Fatal("pendingGateBlock must be consumed once the decision routes to resume")
	}
	if !armed {
		t.Fatal("proposalTurnPending not armed — resumed turn re-evaluates the stale block (the live wedge)")
	}
	if fixActive {
		t.Fatal("suggest-requirement-change must clear gateFixCodeActive (symmetric with direct-block branch)")
	}
}

// Both branches must leave the same gate-intent shape on the target run —
// the BUG-649 wedge was a half-transition (consumed block, no arm).
func TestBug649_BothBranchesSymmetric(t *testing.T) {
	cases := []struct {
		option      string
		custom      string
		wantPropsal bool
		wantFixCode bool
	}{
		{"keep-test-fix-code", "", false, true},
		{"suggest-requirement-change", "", true, false},
		{"custom", "amend the scope and retry", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			svc, runID := bug649BlockedService(t)
			if e := svc.SubmitGateDecision(runID, tc.option, tc.custom); e != nil {
				t.Fatalf("SubmitGateDecision: %d %q %q", e.status, e.code, e.msg)
			}
			svc.mu.Lock()
			rs := svc.runs[runID]
			proposal := rs.proposalTurnPending
			fixCode := rs.gateFixCodeActive
			svc.mu.Unlock()
			if proposal != tc.wantPropsal || fixCode != tc.wantFixCode {
				t.Fatalf("option %q → proposalTurnPending=%v gateFixCodeActive=%v, want %v/%v",
					tc.option, proposal, fixCode, tc.wantPropsal, tc.wantFixCode)
			}
		})
	}
}

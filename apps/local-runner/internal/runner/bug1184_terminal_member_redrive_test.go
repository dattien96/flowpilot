package runner

import (
	"strings"
	"testing"
	"time"
)

// BUG-1184 — a review-cohort member whose leg dies terminal-failed (provider
// error, turn_failed) never records its machine verdict. The done-edge
// verdict gate saw "missing verdict + no live member" and escalated straight
// to a human park — even though resumeVerdictDeficientMembers (CA-1098)
// already knows how to resurrect a non-live member or spawn a replacement.
// The operator had to click Continue for a defect the engine can remediate.
// Fix: the gate auto re-drives the deficient member ONCE (bounded per
// episode, reset when a fresh member verdict lands) before escalating.

// bug1184DeadReviewerWedge seeds the ca1098 topology with the reviewer
// member TERMINAL-FAILED and no recorded verdict — the live shape where the
// done-edge escalates with nobody home.
func bug1184DeadReviewerWedge(t *testing.T, svc *InteractiveService, runID string, pk ProviderKey) string {
	t.Helper()
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")

	childID := runID + "-rev"
	svc.mu.Lock()
	accountID := svc.runs[runID].providerAccountID
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       runID,
		label:             "reviewer",
		role:              "reviewer",
		agentName:         "reviewer",
		status:            RunStatusFailed, // terminal — the BUG-1184 shape
		agentStatus:       string(RunStatusFailed),
		providerKey:       pk,
		providerAccountID: accountID,
		flowCohortId:      "flow-auto-validate-round-0",
		stepID:            "reviewer",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, childID)
	return childID
}

// (a) The done-edge must auto-redrive the terminal member instead of
// escalating to a human park.
func TestBug1184_DoneEdgeAutoRedrivesTerminalMember(t *testing.T) {
	pk := ProviderKeyClaude
	ch := make(chan TurnRequest, 8)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, pk, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	childID := bug1184DeadReviewerWedge(t, svc, parent.RunID, pk)

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must take over synthesis->audit")
	}
	if res.NextAction == "escalate" || res.NextAction == "" {
		t.Fatalf("terminal member + missing verdict escalated instead of auto-redriving: %+v", res)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case req := <-ch:
			if req.RunID == childID && strings.Contains(req.Prompt, "machine verdict") {
				return
			}
		case <-time.After(50 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("no re-drive turn reached the terminal-failed reviewer member")
			}
		}
	}
}

// (b) The auto-redrive is bounded: with the episode budget spent, the same
// wedge escalates like before — no infinite respawn loop.
func TestBug1184_AutoRedriveBoundedThenEscalates(t *testing.T) {
	pk := ProviderKeyClaude
	ch := make(chan TurnRequest, 8)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, pk, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	childID := bug1184DeadReviewerWedge(t, svc, parent.RunID, pk)

	// Episode budget already spent — the done-edge must escalate, not redrive.
	svc.mu.Lock()
	svc.runs[parent.RunID].verdictAutoRedrives = maxVerdictAutoRedrives
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "synthesized"})
	if !handled {
		t.Fatal("advanceHubDoneThroughEdge must still take over")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" && loop.Status != LoopStatusTournamentEscalation {
		t.Fatalf("exhausted auto-redrive budget must escalate, loop=%+v res=%+v", loop, res)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[childID].status == RunStatusRunning {
		t.Fatal("exhausted budget must not resurrect the terminal member")
	}
}

// (c) A landing verdict resets the episode budget — the next missing-verdict
// episode gets a fresh bound rather than inheriting the last one's spend.
func TestBug1184_VerdictProgressResetsBudget(t *testing.T) {
	pk := ProviderKeyClaude
	reg := newProviderRegistry()
	registerKeyedCapture(reg, pk, make(chan TurnRequest, 8))
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	bug1184DeadReviewerWedge(t, svc, parent.RunID, pk)

	svc.mu.Lock()
	svc.runs[parent.RunID].verdictAutoRedrives = 1
	svc.mu.Unlock()
	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{{Label: "reviewer", MachineVerdict: "approved"}})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[parent.RunID].verdictAutoRedrives != 0 {
		t.Fatalf("landing verdict must reset the auto-redrive budget, got %d", svc.runs[parent.RunID].verdictAutoRedrives)
	}
}

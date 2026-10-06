package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-641 (live run-306526): a leg's waiting_* status is mirrored onto the
// parent flow step (settleFlowChildStepAwaitingUserLocked stamps the label's
// step WAITING_USER_APPROVAL). Both heal paths — the wedge sweep's
// healOrphanedWait (BUG-581/579) and healMirroredQuestionWaitLocked
// (CA-642/CA-1207) — cleared rs.status but never un-stamped the mirrored
// step, so the node stayed "Waiting: question" forever: desktop painted a
// phantom card and the hub kept deferring reinvokes on "children waiting".
// The step must heal with the leg.

func bug641FlowFixture(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.status = RunStatusRunning
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Unlock()
	return svc, parent.RunID
}

func bug641WaitingLeg(t *testing.T, svc *InteractiveService, id, parentID, label string) {
	t.Helper()
	svc.mu.Lock()
	svc.runs[id] = &interactiveRun{
		id:                  id,
		parentRunID:         parentID,
		label:               label,
		stepID:              label,
		status:              RunStatusWaitingQuestion,
		agentStatus:         string(RunStatusWaitingQuestion),
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
}

func TestBug641_OrphanedWaitHealClearsMirroredStepStamp(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	bug641WaitingLeg(t, svc, "leg-coder", parentID, "coder")
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	legStatus := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if legStatus != RunStatusRunning {
		t.Fatalf("orphaned leg wait not healed: %s", legStatus)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusRunning {
		t.Fatalf("mirrored step stamp must heal with the leg: status=%s, want RUNNING", st)
	}
}

// Reviewer I-2: the step stamp keys on the node id (= label) — with
// duplicate labels (BUG-639 class) a DIFFERENT sibling leg may own the live
// park. Healing the orphaned leg must not un-stamp a wait its sibling holds.
func TestBug641_HealKeepsStepStampOwnedBySiblingLeg(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	// Older leg: orphaned wait, no card — the heal target.
	bug641WaitingLeg(t, svc, "leg-coder-old", parentID, "coder")
	svc.agentOrchestrator.registerChild(parentID, "leg-coder-old")
	// Newer same-label sibling: holds the REAL park (carded).
	svc.mu.Lock()
	svc.runs["leg-coder-new"] = &interactiveRun{
		id:                "leg-coder-new",
		parentRunID:       parentID,
		label:             "coder",
		stepID:            "coder",
		status:            RunStatusWaitingUserApr,
		pendingApprovalID: "appr-live",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, "leg-coder-new")
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	legStatus := svc.runs["leg-coder-old"].status
	svc.mu.Unlock()
	if legStatus != RunStatusRunning {
		t.Fatalf("orphaned sibling leg not healed: %s", legStatus)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusWaitingUserApr {
		t.Fatalf("sibling-owned step stamp clobbered: status=%s, want WAITING_USER_APPROVAL", st)
	}
}

// A card-backed wait is real work — neither the leg nor its step stamp may
// be healed.
func TestBug641_SweepKeepsStepWaitBackedByCard(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.runs["leg-coder"] = &interactiveRun{
		id:                  "leg-coder",
		parentRunID:         parentID,
		label:               "coder",
		stepID:              "coder",
		status:              RunStatusWaitingQuestion,
		pendingQuestionID:   "q-live",
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	legStatus := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if legStatus != RunStatusWaitingQuestion {
		t.Fatalf("card-backed wait healed: %s", legStatus)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusWaitingUserApr {
		t.Fatalf("card-backed step stamp clobbered: status=%s", st)
	}
}

// A waiting_user_approval leg whose parent loop is blocked is a designed
// park — the parent owns the decision surface. Healing it would be a
// silent auto-allow; leg and step must both stay.
func TestBug641_SweepKeepsOwnerParkedWaitAndStep(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.runs["leg-coder"] = &interactiveRun{
		id:                  "leg-coder",
		parentRunID:         parentID,
		label:               "coder",
		stepID:              "coder",
		status:              RunStatusWaitingUserApr,
		agentStatus:         string(RunStatusWaitingUserApr),
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked"})
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()
	time.Sleep(50 * time.Millisecond)

	svc.mu.Lock()
	legStatus := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if legStatus != RunStatusWaitingUserApr {
		t.Fatalf("owner-parked leg wait healed: %s", legStatus)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusWaitingUserApr {
		t.Fatalf("owner-parked step stamp clobbered: %s", st)
	}
}

// healMirroredQuestionWaitLocked heals sibling-leg mirrors of a resolved
// question — the mirrored leg's step stamp must clear the same way.
func TestBug641_MirroredWaitHealClearsStepStamp(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})
	svc.mu.Lock()
	svc.runs["leg-owner"] = &interactiveRun{
		id:          "leg-owner",
		parentRunID: parentID,
		label:       "owner_1",
		runKind:     "chat",
		chatID:      "chat-flow",
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs["leg-mirror"] = &interactiveRun{
		id:          "leg-mirror",
		parentRunID: parentID,
		label:       "coder",
		runKind:     "chat",
		chatID:      "chat-flow", // sibling leg of the same chat — mirror target
		status:      RunStatusWaitingQuestion,
		agentStatus: string(RunStatusWaitingQuestion),
		subs:        map[int64]chan ProviderEvent{},
	}
	owner := svc.runs["leg-owner"]
	svc.healMirroredQuestionWaitLocked(owner, "q-resolved")
	svc.mu.Unlock()

	svc.mu.Lock()
	mirrorStatus := svc.runs["leg-mirror"].status
	svc.mu.Unlock()
	if mirrorStatus != RunStatusRunning {
		t.Fatalf("mirrored wait not healed: %s", mirrorStatus)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusRunning {
		t.Fatalf("mirrored leg's step stamp must heal with the wait: status=%s", st)
	}
}

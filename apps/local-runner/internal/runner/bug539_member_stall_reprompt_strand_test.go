package runner

import (
	"net/http"
	"testing"
	"time"
)

// BUG-539 (live run-2830, run-1663): member_stalled counted a member with an
// ARMED gate-reprompt intent as silent — the intent could not dispatch because
// the loop was blocked (hub_stalled). The stall → park wipe is the deliberate
// BUG-354 freeze contract (pendingGateCodePaths survives as the re-check seed),
// but the orphan-cure Resume mint then reset repromptAttempts — making
// maxFlowGateReprompts unreachable and the gate loop unbounded. The fix:
// armed remediation is queued work, not silence (shield), armed intents flush
// through the reprompt channel (cap preserved), and the counter only resets
// once the code-path debt is cleared.
func TestBug539_MemberStallShieldsArmedReprompt(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-539p", "run-539c"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		subs:             map[int64]chan ProviderEvent{},
	}
	// Live shape: reprompt armed by the gate, undispatched behind the blocked
	// loop; no user-visible card.
	svc.runs[childID] = &interactiveRun{
		id:                        childID,
		parentRunID:               parentID,
		label:                     "candidate-a",
		status:                    RunStatusWaitingUserApr,
		agentStatus:               "waiting_user_approval",
		flowCohortId:              "flow-auto-parallel_rollout-round-0",
		pendingGateRepromptPrompt: "[flow-gate] re-check the flagged code paths",
		pendingGateRepromptStepID: "step-x",
		pendingGateRepromptGen:    1,
		lastProviderEventAt:       time.Now().UTC().Add(-10 * time.Minute),
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("member_stalled must not fire while a member holds an armed gate-reprompt intent")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status == "blocked" && loop.BlockReason == "member_stalled" {
		t.Fatalf("member with queued remediation counted as stalled: %+v", loop)
	}
}

// A genuinely silent member (no gate, no card, no armed intent) must still
// stall — the shield must not swallow real stalls.
func TestBug539_SilentMemberStillStalls(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-539sp", "run-539sc"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                  childID,
		parentRunID:         parentID,
		label:               "candidate-a",
		status:              RunStatusRunning,
		flowCohortId:        "flow-auto-parallel_rollout-round-0",
		lastProviderEventAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if !svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("silent member with no armed intent must still fire member_stalled")
	}
}

// The orphan-cure in resumeFlowWithFeedback must NOT mint a fresh Resume turn
// for a child whose reprompt is armed — the durable flush owns it (reprompt
// channel keeps repromptAttempts so the cap stays reachable). Pre-fix the
// Resume mint reset repromptAttempts=0 every cycle → unbounded gate loop.
func TestBug539_ArmedRepromptChildNotOrphanCured(t *testing.T) {
	svc, parentID := clusterFService(t)

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = parentID
	child.label = "candidate-a"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = "waiting_user_approval"
	child.stepID = handle.StepID
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	child.turnCount = 3
	child.pendingGateCodePaths = []string{"main.go"}
	child.pendingGateRepromptPrompt = "[flow-gate] re-check the flagged code paths"
	child.pendingGateRepromptStepID = handle.StepID
	child.pendingGateRepromptGen = 1
	child.repromptAttempts = 2 // at cap — must survive the re-drive
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, handle.RunID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "hub_stalled"
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(parentID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := child.turnCount > 3
		svc.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.turnCount <= 3 {
		t.Fatal("armed reprompt never re-drove the child on unblock")
	}
	if child.turnCount > 4 {
		t.Fatalf("child re-drove %d times — armed reprompt double-driven with orphan Resume mint", child.turnCount-3)
	}
	if child.repromptAttempts != 2 {
		t.Fatalf("repromptAttempts=%d, want 2 — the re-drive must ride the reprompt channel so the cap stays reachable", child.repromptAttempts)
	}
	if child.pendingGateRepromptPrompt != "" {
		t.Fatal("reprompt intent still armed after its turn dispatched — would double-drive on next flush")
	}
}

// Skip on a stalled member must kill its armed intents and close its leg —
// live run-2830 kept leg_state=active and its armed reprompt after skip
// (orphaned approval + zombie claim).
func TestBug539_SkipClearsIntentsAndClosesLeg(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "reviewer_security"
	c.status = RunStatusRunning
	c.legState = LegStateActive
	c.pendingGateRepromptPrompt = "[flow-gate] re-check"
	c.pendingGateRepromptStepID = "step-x"
	c.pendingGateRepromptGen = 1
	c.pendingResumePrompt = "resume me"
	c.pendingResumeStepID = "step-x"
	c.pendingResumeGen = 1
	c.pendingTurnPrompt = "queued"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "member_stalled", ActiveNode: "reviewer_security", Cap: 3, Mode: "explicit",
	})

	_, handled, aerr := svc.handleMemberAction(parent.RunID, MemberAction{Action: "skip", Node: "reviewer_security"})
	if aerr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, aerr)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if c.status != RunStatusFailed {
		t.Fatalf("skipped member status=%s, want failed", c.status)
	}
	if c.pendingGateRepromptPrompt != "" || c.pendingResumePrompt != "" || c.pendingTurnPrompt != "" {
		t.Fatalf("skipped member kept armed intents (reprompt=%q resume=%q turn=%q) — dead member would re-drive",
			c.pendingGateRepromptPrompt, c.pendingResumePrompt, c.pendingTurnPrompt)
	}
	if c.legState == LegStateActive {
		t.Fatal("skipped member leaked an active leg/worktree claim")
	}
}

// Retry on a stalled member must clear any armed reprompt/resume before its
// fresh turn — otherwise the unblock flush double-drives remediation on top of
// the retry turn.
func TestBug539_RetryClearsArmedIntents(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "reviewer_security"
	c.status = RunStatusRunning
	c.pendingGateRepromptPrompt = "[flow-gate] re-check"
	c.pendingGateRepromptStepID = "step-x"
	c.pendingGateRepromptGen = 1
	c.pendingResumePrompt = "resume me"
	c.pendingResumeStepID = "step-x"
	c.pendingResumeGen = 1
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "member_stalled", ActiveNode: "reviewer_security", Cap: 3, Mode: "explicit",
	})

	_, handled, aerr := svc.handleMemberAction(parent.RunID, MemberAction{Action: "retry", Node: "reviewer_security"})
	if aerr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, aerr)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if c.pendingGateRepromptPrompt != "" || c.pendingResumePrompt != "" {
		t.Fatalf("retried member kept armed intents (reprompt=%q resume=%q) — flush would double-drive over the retry turn",
			c.pendingGateRepromptPrompt, c.pendingResumePrompt)
	}
}

// Fresh user/engine turns on a run that still owes gate re-check must not
// reset repromptAttempts — the cap bounds the remediation LOOP regardless of
// which channel re-drives it (orphan-cure Resume mint included).
func TestBug539_RemediationTurnKeepsRepromptCounter(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.repromptAttempts = 2
	rs.pendingGateCodePaths = []string{"main.go"}
	svc.mu.Unlock()

	_, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-x", Prompt: "resume remediation"}, "", "")
	if apiErr != nil && apiErr.status != http.StatusConflict {
		// Some early refusals are fine — only the counter matters here.
		t.Logf("startTurn refused early: %v", apiErr)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.repromptAttempts != 2 {
		t.Fatalf("repromptAttempts=%d — reset while gate code-path debt still owed makes the cap unreachable", rs.repromptAttempts)
	}
}

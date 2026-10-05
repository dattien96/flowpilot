package runner

import (
	"testing"
	"time"
)

// BUG-1188 (live run-204891): a hub reinvoke racing the blocked→running
// resume transition gets a flow_awaiting_user admission reject — the loop
// flips while the goroutine is mid-flight. That shape is transient like
// hub_parked, NOT a real failure: counting it against
// hubReinvokeStartFailCount poisons the channel — once the count passes 3
// the armed pendingHubReinvoke has no drain path left (scheduleChildTurn
// sets shouldDrain=false AND the hub_stall tick gate requires count<=3),
// so the loop cycles running↔blocked hub_stalled forever with no turn
// ever starting. Live evidence: 6 consecutive hub_reinvoke_start_failed
// entries on run-204891 (06:23→06:59), all flow_awaiting_user while the
// run's own loop read "running" in the same diag record.
func TestBug1188_FlowAwaitingUserReinvokeDoesNotBurnFailBudget(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-1188-transient"
	rs := &interactiveRun{
		id:               runID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		reinvokeInFlight: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	// Loop blocked: admission rejects with flow_awaiting_user (the park the
	// resume's unblock is about to clear on a parallel goroutine).
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.Cap = 10
		return st
	})
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.scheduleChildTurn(runID, "step-hub", "synthesize the cohort note")

	// Wait for the reject path to settle (flag cleared).
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := !rs.reinvokeInFlight
		svc.mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.reinvokeInFlight {
		t.Fatal("reinvokeInFlight still true after startTurn reject — flag must clear on the error path")
	}
	if rs.hubReinvokeStartFailCount != 0 {
		t.Fatalf("flow_awaiting_user burned the consecutive-fail budget: count=%d (want 0 — transient like hub_parked, drained by the next stall tick or resume)", rs.hubReinvokeStartFailCount)
	}
	if !rs.pendingHubReinvoke {
		t.Fatal("pending reinvoke dropped after transient reject — must stay armed for the post-unblock drain")
	}
}

// BUG-1188 (live run-204891) — relayed success wedges the source flag.
// startTurn on a closed leg relays to the chat's active successor leg
// (interactive_service.go:11400). The success path clears
// reinvokeInFlight on the run the turn actually starts on — the LEG —
// leaving the SOURCE run's flag true forever. Every later
// maybeAutoReinvokeHub* guard (!reinvokeInFlight) then defers, the armed
// pendingHubReinvoke drains into the same defer, and the loop cycles
// running↔blocked hub_stalled with the flag never clearing. The caller —
// scheduleChildTurn — owns the flag on runID and must clear it after ANY
// successful startTurn return, relayed or not.
func TestBug1188_RelayedSuccessClearsSourceReinvokeInFlight(t *testing.T) {
	svc := bug289Service(t)
	chatID := "chat-1188"

	// Successor leg: a real run so its own turn admission has the plumbing
	// createRun provisions (session, step, account wiring).
	leg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun leg: %v", err)
	}
	svc.mu.Lock()
	lr := svc.runs[leg.RunID]
	lr.chatID = chatID
	lr.legSeq = 1
	lr.providerAccountID = svc.activeAccountID
	svc.mu.Unlock()

	// Source: the closed sprint hub leg whose reinvoke is scheduled.
	src := &interactiveRun{
		id:                "run-src-1188",
		chatID:            chatID,
		legSeq:            0,
		legState:          LegStateClosed,
		legClosedReason:   LegClosedReasonContextReset,
		autoOrchestrate:   true,
		flowEngineDriven:  true,
		reinvokeInFlight:  true,
		status:            RunStatusRunning,
		providerAccountID: svc.activeAccountID,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[src.id] = src
	svc.mu.Unlock()

	svc.scheduleChildTurn(src.id, "step-hub", "synthesize the cohort note")

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		srcFlag := src.reinvokeInFlight
		legTurn := lr.turnInFlight || lr.status == RunStatusRunning && lr.currentTurnID != ""
		svc.mu.Unlock()
		if legTurn || !srcFlag {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if lr.currentTurnID == "" && !lr.turnInFlight && lr.status != RunStatusRunning {
		t.Skipf("relayed turn did not start on the leg in test harness (status=%s) — flag-clear assertion needs a real dispatch", lr.status)
	}
	if src.reinvokeInFlight {
		t.Fatal("source reinvokeInFlight still true after relayed startTurn succeeded — the success must clear the flag on the scheduling run, not only the run the turn lands on")
	}
}

// BUG-1188 — a child reprompt refused by a still-open parent decision card
// (flow_awaiting_user at interactive_service.go:11651 — the PARENT loop is
// blocked) must not be retried in-process (a human-scale park) nor dropped
// (the BUG-1185 stranded-member shape): it parks on the DURABLE
// gate-reprompt intent, which resumeFlowWithFeedback's child sweep flushes
// after unblock — and which survives restart. The parked status mirrors
// handleSpawnedChildTurnFailure so summaries agree (BUG-1179).
func TestBug1188_FlowAwaitingUserChildRepromptParksOnDurableIntent(t *testing.T) {
	svc := bug289Service(t)
	child := &interactiveRun{
		id:               "child-1188",
		parentRunID:      "parent-1188",
		reinvokeInFlight: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	// Parent's decision card is open → child admission is refused with
	// flow_awaiting_user before any provider plumbing is reached.
	svc.agentOrchestrator.mutateLoop("parent-1188", func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.Cap = 10
		return st
	})
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.mu.Unlock()

	svc.scheduleChildTurn(child.id, "step-1", "reprompt after verdict")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		inFlight, parked := child.reinvokeInFlight, child.pendingGateRepromptStepID
		svc.mu.Unlock()
		if !inFlight && parked != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.reinvokeInFlight {
		t.Fatal("reinvokeInFlight still true after flow_awaiting_user park")
	}
	if child.pendingGateRepromptStepID != "step-1" || child.pendingGateRepromptPrompt != "reprompt after verdict" {
		t.Fatalf("durable reprompt intent not armed: step=%q prompt=%q",
			child.pendingGateRepromptStepID, child.pendingGateRepromptPrompt)
	}
	if child.pendingGateRepromptGen != 1 {
		t.Fatalf("reprompt gen = %d, want 1", child.pendingGateRepromptGen)
	}
	if child.status != RunStatusWaitingUserApr {
		t.Fatalf("child status = %q, want waiting_user_approval park", child.status)
	}
	if child.hubReinvokeStartFailCount != 0 {
		t.Fatalf("flow_awaiting_user burned the fail budget on a child: count=%d",
			child.hubReinvokeStartFailCount)
	}
	if child.pendingHubReinvoke {
		t.Fatal("child armed the parent-only pendingHubReinvoke path")
	}
}

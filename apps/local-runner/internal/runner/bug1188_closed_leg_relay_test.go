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

	// The flag clear (scheduleChildTurn success path) runs AFTER startTurn
	// returns — which is AFTER the relayed leg's turnInFlight/currentTurnID
	// are already set. Breaking the poll on legTurn therefore races the
	// outer goroutine's scheduling gap; poll for the flag itself and only
	// use leg evidence to decide skip-vs-assert.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := !src.reinvokeInFlight
		svc.mu.Unlock()
		if done {
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

// BUG-1188 follow-up: the durable reprompt armed on a parked child must
// survive a SUBSEQUENT parkFlowForAwaitingUser on the parent (a second
// escalate/cap while the loop is still blocked). parkFlowForAwaitingUser's
// child sweep wiped pendingGateReprompt* on every child unconditionally —
// the BUG-354 "no live auto-intents" contract — but an intent armed on an
// already-parked child is not a live auto-intent: it is the owed-delivery
// handoff the resume sweep flushes after unblock. Wiping it re-creates the
// BUG-1185 strand: child stays waiting_user_approval, no pending record, no
// driver (live run-215331 sat parked ~95min through several parent blocks).
// Only children this park is actually freezing (still running/starting)
// lose their intents.
func TestBug1188_ReParkPreservesParkedChildRepromptIntent(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-1188rp", "run-1188rc"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	// Shape left by the flow_awaiting_user park above: already
	// waiting_user_approval with the durable reprompt armed.
	svc.runs[childID] = &interactiveRun{
		id:                        childID,
		parentRunID:               parentID,
		label:                     "coder",
		status:                    RunStatusWaitingUserApr,
		agentStatus:               "waiting_user_approval",
		turnCount:                 1,
		pendingGateRepromptPrompt: "reprompt after verdict",
		pendingGateRepromptStepID: "step-1",
		pendingGateRepromptGen:    1,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)

	svc.parkFlowForAwaitingUser(parentID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	child := svc.runs[childID]
	if child.pendingGateRepromptPrompt != "reprompt after verdict" ||
		child.pendingGateRepromptStepID != "step-1" {
		t.Fatalf("re-park wiped the parked child's armed reprompt (prompt=%q step=%q) — owed reprompt dropped, member re-stranded",
			child.pendingGateRepromptPrompt, child.pendingGateRepromptStepID)
	}
	if child.pendingGateRepromptGen != 1 {
		t.Fatalf("reprompt gen = %d after re-park, want 1 (high-water kept for claim idempotency)", child.pendingGateRepromptGen)
	}
}

// A RUNNING child is what the BUG-354 wipe contract targets — keep wiping,
// but through the canonical clearIntentFieldsLocked: the old 3-field wipe
// zeroed the gen while leaving deliveredGen/acceptedTurn/failCount/failGen
// behind. A re-armed intent (gen back to 1) could then hit the flush's
// "delivered == gen && acceptedTurn != ''" consumed check and be cleared
// as already-delivered — a silent second-class drop on top of the wipe.
func TestBug1188_ParkWipeUsesConsistentIntentClear(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-1188wp", "run-1188wc"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                             childID,
		parentRunID:                    parentID,
		label:                          "coder",
		status:                         RunStatusRunning,
		agentStatus:                    string(RunStatusRunning),
		turnCount:                      2,
		pendingGateRepromptPrompt:      "stale reprompt",
		pendingGateRepromptStepID:      "step-x",
		pendingGateRepromptGen:         3,
		pendingGateRepromptDeliveredGen: 2,
		pendingGateRepromptAcceptedTurn: "turn-old",
		pendingGateRepromptFailCount:   2,
		pendingGateRepromptFailGen:     2,
		subs:                           map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)

	svc.parkFlowForAwaitingUser(parentID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	child := svc.runs[childID]
	if child.pendingGateRepromptPrompt != "" || child.pendingGateRepromptStepID != "" {
		t.Fatalf("running child reprompt must still wipe (BUG-354): prompt=%q step=%q",
			child.pendingGateRepromptPrompt, child.pendingGateRepromptStepID)
	}
	if child.pendingGateRepromptDeliveredGen != 0 || child.pendingGateRepromptAcceptedTurn != "" {
		t.Fatalf("stale delivered/accepted markers survived the wipe: deliveredGen=%d acceptedTurn=%q — a re-armed intent can false-positive the consumed check",
			child.pendingGateRepromptDeliveredGen, child.pendingGateRepromptAcceptedTurn)
	}
	if child.pendingGateRepromptFailCount != 0 || child.pendingGateRepromptFailGen != 0 {
		t.Fatalf("stale fail budget survived the wipe: count=%d gen=%d — a fresh re-arm inherits a spent budget",
			child.pendingGateRepromptFailCount, child.pendingGateRepromptFailGen)
	}
}

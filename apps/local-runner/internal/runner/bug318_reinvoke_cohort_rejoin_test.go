package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestReinvokeLifecycleCohortReviewerRejoinsFreshCohortEachRound is the BUG-318
// regression. A review-loop whose reviewer node is reinvoke-lifecycle (reuses one
// child run across rounds — e.g. a single "my-reviewer-claude") hangs after round
// 2's reviewer completes: the hub never synthesizes and no watchdog trips.
//
// Round 0 spawns the reviewer into cohort "flow-auto-coder-round-0"; when it
// completes, cohort-join drains that cohort (deleting its cohortExpected entry)
// and reinvokes the hub. Round 1 advances via tryAdvanceFlowFromNode's reuse-child
// branch, which reinvokes the SAME child. Before the fix that branch never
// refreshed the child's flowCohortId, so the reused reviewer kept the drained
// round-0 cohort id; its completion appended to a dead cohort (cohortExpected==0),
// cohortComplete stayed false, and maybeAutoReinvokeHubWithNote was never
// scheduled -> the flow hung. The fix re-registers the fresh round's cohort and
// re-tags the reused child so every round joins a live cohort.
//
// Provider-agnostic: the reuse/cohort machinery is shared, so the matrix runs all
// three V2 providers (Claude/Codex/Grok) even though only the reused-reviewer
// SHAPE — not the provider — triggers it.
func TestReinvokeLifecycleCohortReviewerRejoinsFreshCohortEachRound(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			var mu sync.Mutex
			var prompts []string
			blockRound1 := make(chan struct{})
			var released bool
			releaseOnce := func() {
				mu.Lock()
				if !released {
					released = true
					close(blockRound1)
				}
				mu.Unlock()
			}
			t.Cleanup(releaseOnce)

			reg := newProviderRegistry()
			reg.register(ProviderRegistration{
				Key: pk, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
						mu.Lock()
						prompts = append(prompts, req.Prompt)
						mu.Unlock()
						// Keep round-1's reviewer turn in flight so the test can inspect
						// the cohort BEFORE completion drains it (no race with drainCohort).
						if strings.Contains(req.Prompt, "round 1 result") {
							<-blockRound1
						}
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
						return nil
					})
				},
			})

			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
			svc.mu.Lock()
			svc.runs[parent.RunID].flowEngineDriven = true
			svc.runs[parent.RunID].autoOrchestrate = true
			svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
			svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke"}}
			svc.mu.Unlock()

			// Round 0: coder completion spawns the reviewer INTO cohort
			// "flow-auto-coder-round-0" (round defaults to 0).
			if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "round 0 result") {
				t.Fatal("round 0 tryAdvanceFlowFromNode returned false")
			}
			waitLoop(t, "round-0 reviewer completed (drains round-0 cohort)", 3*time.Second, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				for _, run := range svc.runs {
					if run.parentRunID == parent.RunID && run.label == "reviewer" && run.status == RunStatusCompleted {
						return true
					}
				}
				return false
			})

			// Round 1: same reused child is reinvoked.
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 1})
			if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "round 1 result") {
				t.Fatal("round 1 tryAdvanceFlowFromNode returned false")
			}
			waitLoop(t, "reviewer reinvoked for round 1", 3*time.Second, func() bool {
				mu.Lock()
				defer mu.Unlock()
				for _, p := range prompts {
					if strings.Contains(p, "round 1 result") {
						return true
					}
				}
				return false
			})

			// The reused reviewer must now carry THIS round's fresh cohort id.
			const freshCohort = "flow-auto-coder-round-1"
			svc.mu.Lock()
			var got string
			for _, run := range svc.runs {
				if run.parentRunID == parent.RunID && run.label == "reviewer" {
					got = run.flowCohortId
				}
			}
			svc.mu.Unlock()
			if got != freshCohort {
				t.Fatalf("reinvoked reviewer flowCohortId = %q, want %q (stale/lost cohort => hub never synthesizes round 2 => flow hangs)", got, freshCohort)
			}

			// The fresh cohort must be REGISTERED (expected>0) so the member's join
			// actually completes the barrier. Inspected while round-1 is still in
			// flight, so drainCohort has not yet cleared it.
			svc.agentOrchestrator.appendCohortResult(parent.RunID, freshCohort, cohortEntry{Label: "reviewer", Status: "completed"})
			if !svc.agentOrchestrator.cohortComplete(parent.RunID, freshCohort) {
				t.Fatalf("round-1 cohort %q not complete after its only member joined: cohortExpected was never registered => cohortComplete stays false => no synthesis reinvoke => hang", freshCohort)
			}
		})
	}
}

// TestReinvokeLifecycleCohortReviewerRejoinsEachRoundThroughRound2 proves the
// re-join is per-round-generic, not a one-shot round-1 fix: the reused reviewer
// carries the fresh cohort id for round 1 AND round 2 (regression axis: multi-round).
func TestReinvokeLifecycleCohortReviewerRejoinsEachRoundThroughRound2(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				// Keep every reviewer turn in flight so no completion can mutate the
				// loop between the per-round advances — this probes the synchronous
				// re-tag that tryAdvanceFlowFromNode performs, deterministically.
				<-block
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke"}}
	svc.mu.Unlock()

	reviewerCohort := func() string {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label == "reviewer" {
				return run.flowCohortId
			}
		}
		return "<no reviewer>"
	}

	// Round 0 spawns the reviewer; rounds 1 and 2 reinvoke the same child and must
	// each re-tag it with THAT round's fresh cohort id (set synchronously by the
	// advance, so a completion is not required to observe it).
	for round := 0; round <= 2; round++ {
		svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5, Round: round})
		if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "round result") {
			t.Fatalf("round %d tryAdvanceFlowFromNode returned false", round)
		}
		wantCohort := "flow-auto-coder-round-" + itoaBug318(round)
		if got := reviewerCohort(); got != wantCohort {
			t.Fatalf("round %d reviewer flowCohortId = %q, want %q", round, got, wantCohort)
		}
	}
}

// TestReinvokeLifecycleMultiMemberReuseCohortBothMembersRejoin covers a cohort with
// TWO reinvoke-lifecycle reviewers (exercises preRegisterCohort with size 2): on
// reinvoke, BOTH members must re-join the same fresh round cohort and the barrier
// must register expected==2, so the join completes only when both report.
func TestReinvokeLifecycleMultiMemberReuseCohortBothMembersRejoin(t *testing.T) {
	block := make(chan struct{})
	var mu sync.Mutex
	var released bool
	release := func() {
		mu.Lock()
		if !released {
			released = true
			close(block)
		}
		mu.Unlock()
	}
	t.Cleanup(release)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if strings.Contains(req.Prompt, "round 1") {
					<-block // keep round-1 members in flight to inspect the live cohort
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].autoOrchestrate = true
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "rev_a", When: "done", Kind: "forward"},
		{From: "coder", To: "rev_b", When: "done", Kind: "forward"},
	}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "rev_a", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke"},
		{ID: "rev_b", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke"},
	}
	svc.mu.Unlock()

	// Round 0 spawns both reviewers into the round-0 cohort; wait for both to finish.
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "round 0") {
		t.Fatal("round 0 advance returned false")
	}
	waitLoop(t, "both round-0 reviewers completed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		done := 0
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && (run.label == "rev_a" || run.label == "rev_b") && run.status == RunStatusCompleted {
				done++
			}
		}
		return done == 2
	})

	// Round 1 reinvokes both; they stay in flight (blocked) so the cohort is
	// inspectable before any completion drains it.
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 1})
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "round 1") {
		t.Fatal("round 1 advance returned false")
	}
	const freshCohort = "flow-auto-coder-round-1"
	waitLoop(t, "both reviewers re-joined round-1 cohort", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		joined := 0
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && (run.label == "rev_a" || run.label == "rev_b") && run.flowCohortId == freshCohort {
				joined++
			}
		}
		return joined == 2
	})
	// Barrier expected==2: one member completing must NOT complete the cohort.
	svc.agentOrchestrator.appendCohortResult(parent.RunID, freshCohort, cohortEntry{Label: "rev_a", Status: "completed"})
	if svc.agentOrchestrator.cohortComplete(parent.RunID, freshCohort) {
		t.Fatal("cohort completed with only 1/2 members: expected size was not re-registered to 2")
	}
	svc.agentOrchestrator.appendCohortResult(parent.RunID, freshCohort, cohortEntry{Label: "rev_b", Status: "completed"})
	if !svc.agentOrchestrator.cohortComplete(parent.RunID, freshCohort) {
		t.Fatal("cohort not complete after both members joined: barrier expected count wrong => hub never synthesizes")
	}
}

// itoaBug318 avoids an fmt import churn for the single small-int conversion above.
func itoaBug318(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// TestFlowChildSettleArmsHubStallWatchdogWithoutReinvoke is the BUG-318 P2
// (watchdog hardening) regression. Every maybeScheduleHubStallCheck call site is
// coupled to a hub reinvoke/notify actually being scheduled, so when a child
// completes into a state that schedules NOTHING (here: an incomplete cohort —
// the same failure shape as the primary bug), the F-0 hub watchdog was never
// armed and the hub hung silently forever. After the fix, settleFlowChildTurnCompletedLocked
// arms the watchdog on every flow child settle, so the hub blocks with an
// actionable hub_stalled card after the stall timeout regardless of the cause.
func TestFlowChildSettleArmsHubStallWatchdogWithoutReinvoke(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// A cohort whose expected size (2) is never met — only ONE member settles, so
	// cohortComplete stays false and NO hub reinvoke is scheduled: exactly the
	// dead-end the watchdog must catch.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "stuck-cohort", 2)
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].autoOrchestrate = true
	// Short stall window so the watchdog fires within the test instead of 2 min.
	svc.runs[parent.RunID].stallTimeout = 200 * time.Millisecond
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "reviewer_a"
	rs.flowCohortId = "stuck-cohort"
	rs.status = RunStatusCompleted
	rs.pendingFlowGateSettle = false
	// Settle the lone member directly (as runTurn does after the child gate passes).
	// Before the fix this schedules no reinvoke AND arms no watchdog; after the fix
	// it arms the hub stall watchdog regardless.
	svc.settleFlowChildTurnCompletedLocked(rs, "reviewed", ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
	svc.mu.Unlock()

	waitLoop(t, "hub blocks with hub_stalled after a settle that scheduled no reinvoke", 6*time.Second, func() bool {
		st := svc.agentOrchestrator.loopStateFor(parent.RunID)
		return st.Status == "blocked" && st.BlockReason == "hub_stalled"
	})
}

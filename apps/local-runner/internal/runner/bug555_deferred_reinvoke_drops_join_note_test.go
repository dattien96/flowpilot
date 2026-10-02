package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// BUG-555 live repro (run-38799, PrivateVault CP-02, 2026-10-02): a cohort
// join that landed while the hub had turnInFlight armed pendingHubReinvoke
// with the note parked ONLY in pendingAgentContext. Every drain site then
// fired plain maybeAutoReinvokeHub("") — the joined note reached the hub
// solely through the "[FlowPilot system note]" context block
// (composeAgentContextBlock), which resumed-thread providers do not surface
// as a visible user message — the exact reason WithNote embeds the note
// inline (BUG-synthesis-hang). The hub synthesized with nothing to
// synthesize → escalate → park; Continue+feedback still did not carry the
// join note → escalate loop until hub_reinvoke_cap_blocked.
//
// Fix: the schedule path in maybeAutoReinvokeHubWithNote drains owed
// pendingAgentContext into the inline prompt, so a deferred/plain reinvoke
// still carries the note as first-class prompt text (not wrapped in the
// system-note block). Provider-agnostic (runner-side scheduling).
//
// Assertion contract: the marker must be present AND no
// agentContextBlockOpen wrapper may appear — the wrapper is the delivery
// channel that provably fails on resumed provider threads.

func bug555Service(t *testing.T, captured *string) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				*captured = req.Prompt
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func bug555ParentRun(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	handle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(handle.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 10
		return st
	})
	return handle.RunID
}

func waitForCapturedPrompt(t *testing.T, captured *string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if *captured != "" {
			return *captured
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no hub turn prompt was ever dispatched")
	return ""
}

func assertInlineNote(t *testing.T, prompt, marker string) {
	t.Helper()
	if !strings.Contains(prompt, marker) {
		t.Fatalf("reinvoke prompt dropped the joined note entirely:\n%s", prompt)
	}
	if strings.Contains(prompt, agentContextBlockOpen) {
		t.Fatalf("joined note delivered only inside the invisible [FlowPilot system note] "+
			"context block — resumed-thread providers never surface it (BUG-555):\n%s", prompt)
	}
}

// D1 (live shape): join lands while a hub turn is in flight → deferred → the
// idle drain must schedule a reinvoke whose prompt embeds the joined note
// inline, not wrapped in the context block.
func TestBug555_DeferredReinvokeCarriesJoinedNoteInline(t *testing.T) {
	captured := ""
	svc := bug555Service(t, &captured)
	runID := bug555ParentRun(t, svc)

	svc.mu.Lock()
	svc.runs[runID].turnInFlight = true // in-flight hub turn, as in the live wedge
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHubWithNote(runID, "JOINED-NOTE-MARKER: coder done, suite green")

	svc.mu.Lock()
	armed := svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if !armed {
		t.Fatal("note-bearing join during an in-flight turn must arm pendingHubReinvoke")
	}

	// In-flight turn ends → notifyTurnIdle drains the armed reinvoke.
	svc.mu.Lock()
	svc.runs[runID].turnInFlight = false
	svc.mu.Unlock()
	svc.notifyTurnIdle(runID)

	prompt := waitForCapturedPrompt(t, &captured)
	assertInlineNote(t, prompt, "JOINED-NOTE-MARKER")
}

// D2: a plain reinvoke with owed pendingAgentContext (cap-blocked/resume path)
// must embed the owed note inline rather than firing the bare generic tail.
func TestBug555_PlainReinvokePullsOwedContext(t *testing.T) {
	captured := ""
	svc := bug555Service(t, &captured)
	runID := bug555ParentRun(t, svc)

	svc.mu.Lock()
	svc.appendPendingAgentContextLocked(runID, "JOINED-NOTE-MARKER: reviewer approved")
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHub(runID)

	prompt := waitForCapturedPrompt(t, &captured)
	assertInlineNote(t, prompt, "JOINED-NOTE-MARKER")

	// Owed context moved into the prompt — not left to double-render as a
	// context block on the same turn.
	svc.mu.Lock()
	left := len(svc.runs[runID].pendingAgentContext)
	svc.mu.Unlock()
	if left != 0 {
		t.Fatalf("drained pendingAgentContext should be empty, still holds %d entries", left)
	}
}

// D3: BUG-275 dedupe survives — a join that already appended its note must not
// embed it twice when the note also arrives as cohortNote.
func TestBug555_NoteBearingReinvokeDoesNotDuplicateJoinNote(t *testing.T) {
	captured := ""
	svc := bug555Service(t, &captured)
	runID := bug555ParentRun(t, svc)

	note := "JOINED-NOTE-MARKER: review verdicts in"
	svc.mu.Lock()
	svc.appendPendingAgentContextLocked(runID, note)
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHubWithNote(runID, note)

	prompt := waitForCapturedPrompt(t, &captured)
	assertInlineNote(t, prompt, "JOINED-NOTE-MARKER")
	if strings.Count(prompt, "JOINED-NOTE-MARKER") != 1 {
		t.Fatalf("joined note must appear exactly once in the prompt, got:\n%s", prompt)
	}
}

// D4: nothing owed → prompt stays the bare generic tail (unchanged behavior).
func TestBug555_NoOwedContextKeepsBarePrompt(t *testing.T) {
	captured := ""
	svc := bug555Service(t, &captured)
	runID := bug555ParentRun(t, svc)

	svc.maybeAutoReinvokeHub(runID)

	prompt := waitForCapturedPrompt(t, &captured)
	if strings.Contains(prompt, "JOINED-NOTE-MARKER") || !strings.Contains(prompt, "Agent results ready") {
		t.Fatalf("bare reinvoke must keep the generic tail, got:\n%s", prompt)
	}
}

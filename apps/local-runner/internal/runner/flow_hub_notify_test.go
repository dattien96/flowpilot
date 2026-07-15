package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestComposeHubNotifyPromptWithTelegramBinding proves the reinvoke prompt
// reuses appendTelegramOutputPrompt's write-contract (Message Template +
// send_message instruction) and appends the generic "call the flow control
// tool" instruction hub.notify needs (there is no cohort/join note to
// synthesize here, unlike the review-loop synthesis prompt).
func TestComposeHubNotifyPromptWithTelegramBinding(t *testing.T) {
	node := agentpack.FlowNode{
		ID: "notify",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{
				"chatId": "-100123456", "messageTemplate": "Status: {{status}}",
			}},
		},
	}
	got := composeHubNotifyPrompt(node)
	if !strings.Contains(got, "notify") {
		t.Fatalf("expected the node id referenced in the prompt, got %q", got)
	}
	if !strings.Contains(got, "sending a Telegram notification") {
		t.Fatalf("expected the (softened) Telegram write-contract section, got %q", got)
	}
	if !strings.Contains(got, "Status: {{status}}") {
		t.Fatalf("expected the artifact's Message Template verbatim, got %q", got)
	}
	if !strings.Contains(got, "send_message") {
		t.Fatalf("expected send_message tool mentioned, got %q", got)
	}
	// BUG-287: the instruction must name the ACTUAL callable tool/enum value
	// (submit_review_outcome / approved) — the generic "status=done" the model
	// has no way to literally invoke, per submit-review-outcome.yaml's schema.
	if !strings.Contains(got, "submit_review_outcome") || !strings.Contains(got, `status="approved"`) {
		t.Fatalf("expected an instruction to call submit_review_outcome with status=approved, got %q", got)
	}
}

// TestComposeHubNotifyPromptWithoutBinding proves the prompt degrades safely
// (no Telegram section, still tells the hub to call the control tool) for a
// hub.notify node with no telegram OUTPUT binding at all.
func TestComposeHubNotifyPromptWithoutBinding(t *testing.T) {
	got := composeHubNotifyPrompt(agentpack.FlowNode{ID: "notify"})
	if strings.Contains(got, "send_message") {
		t.Fatalf("expected no Telegram section without a binding, got %q", got)
	}
	if !strings.Contains(got, "submit_review_outcome") || !strings.Contains(got, `status="approved"`) {
		t.Fatalf("expected the control-tool instruction regardless, got %q", got)
	}
}

// hubNotifyFlowFixture wires "synthesis(hub.inline) --done--> notify(hub.notify)
// --done--> done", the shape context-coding-review-synthesis.yaml gains when a
// user chains a hub.notify node after synthesis.
func hubNotifyFlowFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "synthesis", To: "notify", When: "done", Kind: "forward"},
		{From: "notify", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "notify", Behavior: "hub.notify"},
	}
	return edges, nodes
}

// TestAdvanceHubDoneThroughEdgeDispatchesHubNotify proves synthesis's own
// "done" call, when its forward edge targets a hub.notify node, reinvokes the
// SAME hub session (no child spawn) instead of settling the flow — and tracks
// "notify" as the run's new active hub node so a LATER "done" resolves against
// notify's own edge.
func TestAdvanceHubDoneThroughEdgeDispatchesHubNotify(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := hubNotifyFlowFixture()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "review approved"})
	if !handled {
		t.Fatal("expected advanceHubDoneThroughEdge to take over for synthesis->notify(hub.notify)")
	}
	// BUG-284 follow-up: reports Status:"done" (the caller's own verdict WAS
	// accepted) with NextAction:"advancing", not "continue"/"looping" — telling
	// the model its done() call was rejected is what caused it to also call
	// escalate() in the same turn live, contributing to the notify send being
	// dropped (see the loop-status guard test below for that failure mode).
	if res.NextAction != "advancing" || res.Status != "done" {
		t.Fatalf("expected Status=done/NextAction=advancing (flow keeps running internally, but the caller's verdict is confirmed), got %+v", res)
	}

	svc.mu.Lock()
	activeHubNodeID := svc.runs[parent.RunID].activeHubNodeID
	reinvokeInFlight := svc.runs[parent.RunID].reinvokeInFlight
	svc.mu.Unlock()
	if activeHubNodeID != "notify" {
		t.Fatalf("activeHubNodeID = %q, want %q so a later done resolves against notify's own edge", activeHubNodeID, "notify")
	}
	if !reinvokeInFlight {
		t.Fatal("expected the hub reinvoke to have been scheduled (reinvokeInFlight=true) synchronously")
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; st == "done" {
		t.Fatalf("flow must NOT settle done when handing off to hub.notify, got status=%q", st)
	}
}

// TestAdvanceHubDoneThroughEdgeResolvesSecondHubNodeAgainstOwnEdge is the
// regression proof for the bug this design must avoid: once activeHubNodeID
// has moved on to "notify", a later "done" call must resolve against notify's
// OWN forward edge (here, the terminal) — not re-find "synthesis" via
// hubInlineNodeID and loop forever re-dispatching hub.notify.
func TestAdvanceHubDoneThroughEdgeResolvesSecondHubNodeAgainstOwnEdge(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := hubNotifyFlowFixture()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "notify" // simulates hub.notify's own turn now calling flow_control(done)
	svc.mu.Unlock()

	_, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "sent"})
	if handled {
		t.Fatal("notify--done-->done (terminal) must NOT be handled here — applyFlowControl settles it, exactly like the synthesis-terminal case")
	}
	svc.mu.Lock()
	cleared := svc.runs[parent.RunID].activeHubNodeID
	svc.mu.Unlock()
	if cleared != "" {
		t.Fatalf("activeHubNodeID = %q, want cleared once the terminal edge is reached", cleared)
	}
}

// TestTryAdvanceFlowThroughInlineDispatchesHubNotify proves a hub.notify node
// reached directly (e.g. a completed agent.delegate child's single forward
// target, not via synthesis) takes the same dispatch path.
func TestTryAdvanceFlowThroughInlineDispatchesHubNotify(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "notify", When: "done", Kind: "forward"},
		{From: "notify", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "notify", Behavior: "hub.notify"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "coder finished") {
		t.Fatal("expected tryAdvanceFlowFromNode to dispatch the hub.notify target and return true")
	}
	svc.mu.Lock()
	activeHubNodeID := svc.runs[parent.RunID].activeHubNodeID
	reinvokeInFlight := svc.runs[parent.RunID].reinvokeInFlight
	svc.mu.Unlock()
	if activeHubNodeID != "notify" {
		t.Fatalf("activeHubNodeID = %q, want %q", activeHubNodeID, "notify")
	}
	if !reinvokeInFlight {
		t.Fatal("expected the hub reinvoke to have been scheduled")
	}
}

// TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne is the
// direct regression proof for BUG-284: a hub.notify "done" is observed from
// INSIDE the very hub turn that is finishing (SubmitFlowControl is called by
// the AI's own tool call mid-turn, so turnInFlight is still true), so the
// reinvoke ALWAYS defers on the first attempt — this is the common case, not
// a rare race. The turn-completion retry must fire the STASHED hub.notify
// prompt (pendingHubReinvokePrompt), not the generic synthesis
// autoReinvokePromptText() the pre-fix retry always used — which silently
// dropped the notify dispatch entirely (observed live as run-8491's
// "tele-step" node ending SKIPPED with no Telegram ever sent).
func TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne(t *testing.T) {
	turnNum := 0
	secondTurnPrompt := make(chan string, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				turnNum++
				if turnNum == 1 {
					// Simulate the synthesizer calling flow_control("done") mid-turn —
					// the exact moment this run's OWN turnInFlight is still true.
					if _, err := b.SubmitFlowControl(FlowControlInput{Status: "done", Summary: "synthesis approved"}); err != nil {
						t.Errorf("first-turn SubmitFlowControl: %v", err)
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "synthesis turn done"})
					return nil
				}
				// Second turn = the retried hub.notify reinvoke.
				secondTurnPrompt <- req.Prompt
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "notify turn done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := parentHandle.RunID

	edges, nodes := hubNotifyFlowFixture()
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "s1", Prompt: "kick off synthesis"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}

	var capturedSecondPrompt string
	select {
	case capturedSecondPrompt = <-secondTurnPrompt:
	case <-time.After(2 * time.Second):
		t.Fatal("the deferred hub.notify reinvoke never retried — a second hub turn was never scheduled (BUG-284 regressed)")
	}

	if !strings.Contains(capturedSecondPrompt, "notify") {
		t.Fatalf("expected the hub.notify write-contract prompt, got %q", capturedSecondPrompt)
	}
	if strings.Contains(capturedSecondPrompt, "Synthesize the join note above") {
		t.Fatal("retried with the GENERIC synthesis reinvoke prompt instead of the stashed hub.notify prompt — BUG-284 regressed")
	}

	svc.mu.Lock()
	activeHubNodeID := svc.runs[runID].activeHubNodeID
	svc.mu.Unlock()
	if activeHubNodeID != "notify" {
		t.Fatalf("activeHubNodeID = %q, want %q", activeHubNodeID, "notify")
	}
}

// TestDispatchHubNotifyNodeMarksStepRunning is the direct regression proof for
// the other half of BUG-284: without stamping the node RUNNING at dispatch
// time, a deferred-and-lost reinvoke left it PENDING, and markFlowRunComplete
// (flow_step_runtime.go) sweeps any still-PENDING step to SKIPPED — exactly
// the "tele-step: skipped" symptom observed live in run-8491.
func TestDispatchHubNotifyNodeMarksStepRunning(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	node := agentpack.FlowNode{ID: "notify", Behavior: "hub.notify"}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()
	seeder, ok := svc.workflowStore.(workflowRunSeeder)
	if !ok {
		t.Fatal("fakeWorkflowStore must implement workflowRunSeeder for this test")
	}
	seeder.seed(parent.RunID, svc.flowStepRowsFromNodes(context.Background(), parent.RunID, []agentpack.FlowNode{node}, StepStatusPending, ""))

	svc.dispatchHubNotifyNode(parent.RunID, node)

	steps, loadErr := store.LoadRunSteps(context.Background(), parent.RunID)
	if loadErr != nil {
		t.Fatalf("LoadRunSteps: %v", loadErr)
	}
	var found bool
	for _, st := range steps {
		if st.ID == "notify" {
			found = true
			if st.Status != StepStatusRunning {
				t.Fatalf("notify step status = %q, want %q", st.Status, StepStatusRunning)
			}
		}
	}
	if !found {
		t.Fatal("expected a seeded step for node 'notify'")
	}
}

// TestMaybeAutoReinvokeHubWithPromptReArmsOnBlockedStatus is the direct
// regression proof for the second half of BUG-284 (run-8774): the SAME turn
// that dispatched hub.notify can also escalate/pause before the deferred
// retry actually fires (observed live — a synthesizer called done(), got a
// "continue" tool result back, then called escalate() in the same turn).
// Without re-arming here, the reinvoke was silently dropped forever: resuming
// later fired the GENERIC synthesis reinvoke instead, and because
// activeHubNodeID stayed pointed at the notify node, the next unrelated
// "done" got misattributed as that node's own completion — settling the flow
// and marking it DONE without the notification ever actually being sent.
func TestMaybeAutoReinvokeHubWithPromptReArmsOnBlockedStatus(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		return st
	})

	svc.maybeAutoReinvokeHubWithPrompt(runID, "MY HUB NOTIFY PROMPT")

	svc.mu.Lock()
	pending := svc.runs[runID].pendingHubReinvoke
	stashedPrompt := svc.runs[runID].pendingHubReinvokePrompt
	svc.mu.Unlock()
	if !pending {
		t.Fatal("pendingHubReinvoke must be re-armed when the loop is blocked, not silently dropped")
	}
	if stashedPrompt != "MY HUB NOTIFY PROMPT" {
		t.Fatalf("pendingHubReinvokePrompt = %q, want the original prompt preserved", stashedPrompt)
	}
}

// TestResumeFlowWithFeedbackRetriesPendingHubNotifyPromptInsteadOfGeneric
// proves resumeFlowWithFeedback consumes a re-armed hub.notify prompt instead
// of firing the generic synthesis reinvoke — the other half of the BUG-284
// (run-8774) fix. Without this, "Continue" after an escalate mid-hub.notify
// would restart the review turn generically, and a later unrelated "done"
// would be misattributed to the still-stale activeHubNodeID.
func TestResumeFlowWithFeedbackRetriesPendingHubNotifyPromptInsteadOfGeneric(t *testing.T) {
	capturedPrompt := make(chan string, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				capturedPrompt <- req.Prompt
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "notify retry turn"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parentHandle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := parentHandle.RunID

	edges, nodes := hubNotifyFlowFixture()
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "notify"
	rs.pendingHubReinvoke = true
	rs.pendingHubReinvokePrompt = "STASHED HUB NOTIFY PROMPT"
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	select {
	case p := <-capturedPrompt:
		if p != "STASHED HUB NOTIFY PROMPT" {
			t.Fatalf("resume retried with prompt %q, want the stashed hub.notify prompt (not the generic resume note)", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resumeFlowWithFeedback never retried the pending hub.notify prompt")
	}

	svc.mu.Lock()
	pendingLeft := svc.runs[runID].pendingHubReinvoke
	promptLeft := svc.runs[runID].pendingHubReinvokePrompt
	svc.mu.Unlock()
	if pendingLeft || promptLeft != "" {
		t.Fatalf("expected pendingHubReinvoke/pendingHubReinvokePrompt cleared after consumption, got pending=%v prompt=%q", pendingLeft, promptLeft)
	}
}

// TestComposeHubNotifyPromptInstructsFillingTemplateFromRealResults is the
// BUG-286 (Fix B) regression: the write-contract used to show the Message
// Template verbatim with no instruction on what to DO with it, so a receiving
// model could (and was observed live to) send the template's own placeholder
// text back unfilled instead of composing a real summary from this turn's
// actual results.
func TestComposeHubNotifyPromptInstructsFillingTemplateFromRealResults(t *testing.T) {
	node := agentpack.FlowNode{
		ID: "tele-step",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{
				"chatId": "-100123456", "messageTemplate": "Status: {{status}}\nSummary: (agent fills)",
			}},
		},
	}
	got := composeHubNotifyPrompt(node)
	if !strings.Contains(got, "ACTUAL outcome") {
		t.Fatalf("expected an instruction to fill the template from the real outcome, got %q", got)
	}
	if !strings.Contains(got, "Do not send the template's placeholder text unfilled") {
		t.Fatalf("expected an explicit warning against echoing the template verbatim, got %q", got)
	}
}

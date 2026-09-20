package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// CP-67 P-1/P-5 (Task-378/381/382): transport + routing coverage for the
// coder→negotiation-hub path that the earlier slices left unwired —
// renegotiate_signatures parsing, batch payload passthrough, record-only
// bridge behavior, HTTP coder-face mapping, back-edge shadowing, and the
// negotiation-hub dispatch that consumes the buffered batch.

func cp67BatchArgs() map[string]any {
	return map[string]any{
		"status": "renegotiate_signatures",
		"batch_signature_requests": []any{
			map[string]any{
				"symbol":             "Add",
				"file":               "calc/calc.go",
				"current_signature":  "func Add(a, b int) error",
				"proposed_signature": "func Add(a, b int) (int, error)",
				"rationale":          "spec needs the sum returned, not only an error",
			},
		},
	}
}

func TestReviewOutcomeAcceptsRenegotiateSignaturesAndPreservesBatch(t *testing.T) {
	roi, err := parseReviewOutcomeInput(cp67BatchArgs())
	if err != nil {
		t.Fatalf("parseReviewOutcomeInput: %v", err)
	}
	fc, err := reviewOutcomeToFlowControl(roi)
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if fc.Status != "continue" {
		t.Fatalf("renegotiate_signatures must map to continue, got %q", fc.Status)
	}
	reqs, err := parseCoderBatchSignatureRequests(fc.Payload)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("batch must survive the payload mapping: reqs=%v err=%v", reqs, err)
	}
	if reqs[0].Symbol != "Add" || reqs[0].Rationale == "" {
		t.Fatalf("batch row lost fields: %+v", reqs[0])
	}
}

func TestReviewOutcomeRejectsRenegotiateWithoutBatch(t *testing.T) {
	_, err := parseReviewOutcomeInput(map[string]any{"status": "renegotiate_signatures"})
	if err == nil || !strings.Contains(err.Error(), "batch_signature_requests") {
		t.Fatalf("renegotiate_signatures without a batch must be rejected, got %v", err)
	}
}

func TestCoderOutcomeFaceMapsDomainStatuses(t *testing.T) {
	face := coderOutcomeFace()
	for domain, want := range map[string]string{
		"completed":              "done",
		"renegotiate_signatures": "continue",
		"blocked":                "escalate",
	} {
		got, ok := resolveFaceStatus(face, domain)
		if !ok || got != want {
			t.Fatalf("coderOutcomeFace maps %q -> %q (ok=%v), want %q", domain, got, ok, want)
		}
	}
	if !looksLikeCoderOutcome("renegotiate_signatures") || !looksLikeCoderOutcome("completed") {
		t.Fatal("coder-domain statuses must route to the coder face")
	}
	if looksLikeCoderOutcome("approved") {
		t.Fatal("review statuses must NOT route to the coder face")
	}
}

func TestSubmitFlowControlCoderBatchIsRecordOnly(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	child := newReproduceChildRun(svc, "child-coder", parent.RunID, t.TempDir(), "", "implement")

	fc, mapErr := reviewOutcomeToFlowControl(mustParseReviewOutcome(t, cp67BatchArgs()))
	if mapErr != nil {
		t.Fatalf("map: %v", mapErr)
	}
	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-1"}
	res, submitErr := bridge.SubmitFlowControl(fc)
	if submitErr != nil {
		t.Fatalf("SubmitFlowControl: %v", submitErr)
	}
	if res.NextAction != "renegotiation_recorded" {
		t.Fatalf("NextAction = %q, want renegotiation_recorded (record-only)", res.NextAction)
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Round != 0 || st.Status != "running" {
		t.Fatalf("record-only continue must not advance the loop: %+v", st)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 1 {
		t.Fatalf("batch must be buffered for the negotiation hub, got %d rows", len(got))
	}
}

func TestSubmitFlowControlDelegateCannotSettleOrLoop(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	child := newReproduceChildRun(svc, "child-coder", parent.RunID, t.TempDir(), "", "implement")
	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-1"}

	for _, st := range []string{"done", "continue"} {
		if _, err := bridge.SubmitFlowControl(FlowControlInput{Status: st, viaReviewOutcome: true}); err == nil {
			t.Fatalf("a delegate child must not %s the flow", st)
		}
	}
	if lst := svc.agentOrchestrator.loopStateFor(parent.RunID); lst.Status != "running" {
		t.Fatalf("rejected submissions must leave the loop running, got %q", lst.Status)
	}
}

func mustParseReviewOutcome(t *testing.T, args map[string]any) ReviewOutcomeInput {
	t.Helper()
	roi, err := parseReviewOutcomeInput(args)
	if err != nil {
		t.Fatalf("parseReviewOutcomeInput: %v", err)
	}
	return roi
}

func TestHandleSubmitFlowControlCoderOutcomeHTTP(t *testing.T) {
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	body, _ := json.Marshal(cp67BatchArgs())
	resp, postErr := http.Post(srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control", "application/json", bytes.NewReader(body))
	if postErr != nil {
		t.Fatalf("post: %v", postErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("coder outcome over HTTP must be accepted, got %d", resp.StatusCode)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 1 {
		t.Fatalf("HTTP coder batch must buffer for the negotiation hub, got %d rows", len(got))
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Round != 0 {
		t.Fatalf("record-only HTTP continue must not advance the loop, Round=%d", st.Round)
	}
}

func TestHandleSubmitFlowControlRejectsUnknownRun(t *testing.T) {
	_, srv := newTestServer(t)
	body, _ := json.Marshal(cp67BatchArgs())
	resp, err := http.Post(srv.URL+"/client/workflow-runs/run-missing/flow-control", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown run must 404, got %d", resp.StatusCode)
	}
}

// cp67NegotiationEdges mirrors the task-harness negotiation slice:
// synthesis --continue/forward--> synthesis_negotiation --continue/back-->
// test_signatures, synthesis_negotiation --done/forward--> synthesis.
func cp67NegotiationTopology() ([]agentpack.FlowNode, []agentpack.FlowEdge) {
	nodes := []agentpack.FlowNode{
		{ID: "test_signatures", Behavior: "agent.scaffold"},
		{ID: "implement", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "synthesis_negotiation", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "test_signatures", To: "implement", When: "done", Kind: "forward"},
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "synthesis_negotiation", When: "continue", Kind: "forward"},
		{From: "synthesis_negotiation", To: "test_signatures", When: "continue", Kind: "back"},
		{From: "synthesis_negotiation", To: "synthesis", When: "done", Kind: "forward"},
	}
	return nodes, edges
}

func TestContinueBackEdgePhaseHubDoesNotShadowWriterEdge(t *testing.T) {
	_, edges := cp67NegotiationTopology()
	// Regression pin: the negotiation phase's own back-edge used to win the
	// hub-aware fallback (dist 1 via synthesis_negotiation->synthesis beats
	// validate->reviewer->synthesis at dist 2) — a normal review continue
	// from synthesis re-entered test_signatures instead of implement.
	got, ok := resolveContinueBackEdgeTarget(edges, "synthesis")
	if !ok || got != "implement" {
		t.Fatalf("synthesis continue must re-enter implement, got %q ok=%v", got, ok)
	}
	got, ok = resolveContinueBackEdgeTarget(edges, "synthesis_negotiation")
	if !ok || got != "test_signatures" {
		t.Fatalf("negotiation hub continue must re-enter test_signatures, got %q ok=%v", got, ok)
	}
}

func TestNegotiationHubNodeForFindsPhaseHub(t *testing.T) {
	nodes, edges := cp67NegotiationTopology()
	node, ok := negotiationHubNodeFor(edges, nodes)
	if !ok || node.ID != "synthesis_negotiation" {
		t.Fatalf("negotiationHubNodeFor = %q ok=%v, want synthesis_negotiation", node.ID, ok)
	}
	// A flow with no negotiation topology has no phase hub.
	plainNodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	plainEdges := []agentpack.FlowEdge{
		{From: "implement", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "implement", When: "continue", Kind: "back"},
	}
	if _, ok := negotiationHubNodeFor(plainEdges, plainNodes); ok {
		t.Fatal("a plain review loop must not resolve a negotiation hub")
	}
}

func TestNegotiationPhaseActiveViaActiveHubNodeID(t *testing.T) {
	svc, runID := negotiationTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, NegotiationCap: 5})
	// A live hub reinvoke turn carries a synthetic step id — the phase is
	// identified by activeHubNodeID, which dispatchHubNotifyNode stamps.
	svc.mu.Lock()
	svc.runs[runID].stepID = "step-9"
	svc.runs[runID].activeHubNodeID = "synthesis_negotiation"
	svc.mu.Unlock()

	if _, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue"}); err != nil {
		t.Fatalf("continue: %v", err)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.NegotiationRound != 1 || st.Round != 0 {
		t.Fatalf("phase must be detected via activeHubNodeID: NegotiationRound=%d Round=%d", st.NegotiationRound, st.Round)
	}
}

func TestRenderNegotiationBatchPromptCarriesRowsAndRouting(t *testing.T) {
	prompt := renderNegotiationBatchPrompt([]CoderBatchSignatureRequest{
		{Symbol: "Add", File: "calc/calc.go", CurrentSignature: "func Add(a,b int) error", ProposedSignature: "func Add(a,b int) (int,error)", Rationale: "need the sum"},
		{Symbol: "Div", File: "calc/calc.go", CurrentSignature: "func Div(a,b int) int", ProposedSignature: "func Div(a,b int) (int,error)", Rationale: "divide-by-zero"},
	})
	for _, want := range []string{"Add", "Div", "need the sum", "divide-by-zero", "submit_review_outcome", "changes_requested", "approved"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("negotiation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCoderCompletionDispatchesNegotiationHubWithBatch(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	prs.stepID = "step-4"
	// turnInFlight forces the hub reinvoke to defer into pendingHubReinvoke —
	// the deterministic assertion point for the batch-injected prompt.
	prs.turnInFlight = true
	svc.mu.Unlock()

	svc.bufferCoderBatchSignatures(parent.RunID, []CoderBatchSignatureRequest{
		{Symbol: "Add", File: "calc/calc.go", CurrentSignature: "func Add(a,b int) error", ProposedSignature: "func Add(a,b int) (int,error)", Rationale: "need the sum"},
	})

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "done") {
		t.Fatal("coder completion with a pending batch must dispatch the negotiation hub")
	}
	svc.mu.Lock()
	prs = svc.runs[parent.RunID]
	hubID := prs.activeHubNodeID
	reinvoke := prs.pendingHubReinvoke
	reinvokePrompt := prs.pendingHubReinvokePrompt
	svc.mu.Unlock()
	if hubID != "synthesis_negotiation" {
		t.Fatalf("activeHubNodeID = %q, want synthesis_negotiation", hubID)
	}
	if !reinvoke || !strings.Contains(reinvokePrompt, "Add") || !strings.Contains(reinvokePrompt, "need the sum") {
		t.Fatalf("the consumed batch must be injected into the hub prompt (reinvoke=%v)", reinvoke)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 0 {
		t.Fatalf("the batch must be consumed once dispatched, got %d rows", len(got))
	}
}

func TestCoderCompletionWithoutBatchAdvancesNormally(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "done") {
		t.Fatal("coder completion without a batch must follow the normal done edge")
	}
	svc.mu.Lock()
	hubID := svc.runs[parent.RunID].activeHubNodeID
	svc.mu.Unlock()
	if hubID == "synthesis_negotiation" {
		t.Fatal("no batch -> no negotiation dispatch")
	}
}

func TestCoderBatchAccumulatesAcrossSubmissions(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	child := newReproduceChildRun(svc, "child-coder", parent.RunID, t.TempDir(), "", "implement")
	bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-1"}

	for i := 0; i < 2; i++ {
		fc, mapErr := reviewOutcomeToFlowControl(mustParseReviewOutcome(t, cp67BatchArgs()))
		if mapErr != nil {
			t.Fatalf("map %d: %v", i, mapErr)
		}
		res, submitErr := bridge.SubmitFlowControl(fc)
		if submitErr != nil {
			t.Fatalf("submit %d: %v", i, submitErr)
		}
		if res.NextAction != "renegotiation_recorded" {
			t.Fatalf("submit %d: NextAction = %q, want renegotiation_recorded", i, res.NextAction)
		}
	}
	// Batches submitted across separate coder turns accumulate under the
	// parent's stepID until the negotiation hub consumes them — a second
	// request must not silently overwrite the first.
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 2 {
		t.Fatalf("accumulated batch = %d rows, want 2", len(got))
	}
	// The buffer is scoped to this run — no leakage into sibling runs.
	other, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun 2: %v", err)
	}
	if got := svc.snapshotCoderBatchSignatures(other.RunID); len(got) != 0 {
		t.Fatalf("batch buffer must be per-run, sibling got %d rows", len(got))
	}
}

func TestNegotiationMultiRoundRedispatchesHub(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5, NegotiationCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	prs.stepID = "step-4"
	prs.turnInFlight = true // hub reinvoke defers into pendingHubReinvoke
	svc.mu.Unlock()

	dispatchRound := func(symbol, rationale string) {
		svc.bufferCoderBatchSignatures(parent.RunID, []CoderBatchSignatureRequest{
			{Symbol: symbol, File: "calc/calc.go", CurrentSignature: "func " + symbol + "(a,b int) int", ProposedSignature: "func " + symbol + "(a,b int) (int,error)", Rationale: rationale},
		})
		if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "done") {
			t.Fatalf("coder completion with pending batch (%s) must dispatch the hub", symbol)
		}
		svc.mu.Lock()
		prs = svc.runs[parent.RunID]
		prompt := prs.pendingHubReinvokePrompt
		hubID := prs.activeHubNodeID
		svc.mu.Unlock()
		if hubID != "synthesis_negotiation" {
			t.Fatalf("round for %s: activeHubNodeID = %q", symbol, hubID)
		}
		if !strings.Contains(prompt, symbol) || !strings.Contains(prompt, rationale) {
			t.Fatalf("round for %s: batch not injected into hub prompt", symbol)
		}
		if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 0 {
			t.Fatalf("round for %s: batch must be consumed, got %d rows", symbol, len(got))
		}
	}
	hubContinue := func(wantRound int) {
		res, err := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "continue"})
		if err != nil {
			t.Fatalf("hub continue round %d: %v", wantRound, err)
		}
		if res.Status != "continue" {
			t.Fatalf("hub continue round %d: status = %q", wantRound, res.Status)
		}
		if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.NegotiationRound != wantRound {
			t.Fatalf("NegotiationRound = %d, want %d", st.NegotiationRound, wantRound)
		}
	}

	// Round 1: coder batch -> hub dispatch -> hub continue.
	dispatchRound("Add", "need the sum")
	hubContinue(1)
	// Round 2: a fresh batch re-buffers and re-dispatches the hub; the
	// counter keeps climbing toward the phase cap of 5.
	dispatchRound("Div", "divide-by-zero")
	hubContinue(2)
}

func TestCoderRenegotiationHTTPDispatchWritesDiagLog(t *testing.T) {
	// Live-shape check: the batch arrives over the real HTTP surface
	// (POST /client/workflow-runs/{id}/flow-control), the node-completion
	// event dispatches the negotiation hub, and the decision is observable in
	// the per-run flow-diag ndjson log — the same artifact an operator greps
	// on a live run.
	diagDir := t.TempDir()
	t.Setenv("FLOWPILOT_FLOW_DIAG_DIR", diagDir)
	svc, srv := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5, NegotiationCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	prs.stepID = "step-4"
	prs.turnInFlight = true
	svc.mu.Unlock()

	body, _ := json.Marshal(cp67BatchArgs())
	resp, postErr := http.Post(srv.URL+"/client/workflow-runs/"+parent.RunID+"/flow-control", "application/json", bytes.NewReader(body))
	if postErr != nil {
		t.Fatalf("post: %v", postErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("coder batch over HTTP must be accepted, got %d", resp.StatusCode)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 1 {
		t.Fatalf("HTTP batch must be buffered record-only, got %d rows", len(got))
	}

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "done") {
		t.Fatal("coder completion with a pending batch must dispatch the negotiation hub")
	}
	logBytes, readErr := os.ReadFile(filepath.Join(diagDir, parent.RunID+".ndjson"))
	if readErr != nil {
		t.Fatalf("flow-diag log for the run must exist: %v", readErr)
	}
	logText := string(logBytes)
	for _, want := range []string{"flow_advance_negotiation_hub", "batch_rows", "implement", "synthesis_negotiation"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("diag log missing %q:\n%s", want, logText)
		}
	}
}

func TestCoderOutcomeTransportProviderParity(t *testing.T) {
	// CP-67 safe-fix-contract R2: submit_review_outcome is wired through the
	// shared turnBridge.SubmitFlowControl — no provider-specific adapter path.
	// The same coder batch must produce the same record-only outcome for
	// claude, codex, and grok parent runs.
	for _, provider := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		svc := newFreezeTestServiceForProvider(t, provider)
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: provider})
		if err != nil {
			t.Fatalf("%s createRun: %v", provider, err)
		}
		svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
		child := newReproduceChildRun(svc, "child-coder", parent.RunID, t.TempDir(), "", "implement")
		fc, mapErr := reviewOutcomeToFlowControl(mustParseReviewOutcome(t, cp67BatchArgs()))
		if mapErr != nil {
			t.Fatalf("%s map: %v", provider, mapErr)
		}
		bridge := &turnBridge{svc: svc, rs: child, ctx: context.Background(), turnID: "turn-1"}
		res, submitErr := bridge.SubmitFlowControl(fc)
		if submitErr != nil {
			t.Fatalf("%s SubmitFlowControl: %v", provider, submitErr)
		}
		if res.NextAction != "renegotiation_recorded" {
			t.Fatalf("%s: NextAction = %q, want renegotiation_recorded", provider, res.NextAction)
		}
		got := svc.snapshotCoderBatchSignatures(parent.RunID)
		if len(got) != 1 || got[0].Symbol != "Add" {
			t.Fatalf("%s: buffered batch = %+v, want the Add row", provider, got)
		}
	}
}

func TestIsSignatureLockedCoderChild(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})
	svc.mu.Lock()
	svc.runs[parentID].flowEngineDriven = true
	svc.mu.Unlock()

	coder := newReproduceChildRun(svc, "child-coder", parentID, dir, head, "implement")
	if svc.isSignatureLockedCoderChild(coder) {
		t.Fatal("no SignatureHash pinned yet — the child must not be treated as locked")
	}

	writeRepoFile(t, dir, "calc/calc.go", "package calc\n\nimport \"errors\"\n\nfunc Add(a, b int) error {\n\treturn errors.New(\"not implemented\")\n}\n")
	writeRepoFile(t, dir, "calc/scaffold_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestRed(t *testing.T) { if err := Add(1,2); err == nil { t.Fatal() } }\n")
	svc.recordScaffoldArtifactsLock(dir, parentID, []string{"calc/calc.go", "calc/scaffold_test.go"})

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok || rec.SignatureHash == "" {
		t.Fatalf("fixture must pin a SignatureHash: ok=%v err=%v hash=%q", ok, err, rec.SignatureHash)
	}
	if !svc.isSignatureLockedCoderChild(coder) {
		t.Fatal("agent.code child on a signature-pinned contract must be detected")
	}
	// A non-writer child (different label -> different node) is not.
	other := newReproduceChildRun(svc, "child-other", parentID, dir, head, "reproduce_test")
	if svc.isSignatureLockedCoderChild(other) {
		t.Fatal("a non-agent.code node must not get the coder outcome tool")
	}
}

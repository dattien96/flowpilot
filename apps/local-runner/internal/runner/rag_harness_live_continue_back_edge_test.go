package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-293 follow-up (CA-628 kill-review I-2/I-1): lock the LIVE rag-harness
// continue topology and reviewer prompt contract. The earlier synthesis edge
// test used a synthetic synthesis→implement back-edge the pack validator
// forbids on the real flow; these tests pin the production shape — exactly one
// continue back-edge (validate → implement) shared by validate retries AND the
// synthesis hub's changes_requested — plus the CP-53 machine-verdict prompt on
// the reviewer node. New file — no pre-existing test is modified.

func liveRAGHarnessDefinition(t *testing.T) agentpack.FlowDefinition {
	t.Helper()
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == "rag-harness" {
			return def
		}
	}
	t.Fatal("rag-harness flow missing from builtin pack")
	return agentpack.FlowDefinition{}
}

// liveRAGHarnessPostImplementFixture carries the LIVE continue back-edge
// source (validate -> implement — the pack validator rejects a second continue
// back-edge, so this is unambiguous) plus the reviewer -> synthesis -> audit
// forward chain. The implement -> validate forward edge is intentionally
// omitted: its presence would auto-advance the completed implement child into
// runValidateNode, which (with no workspace/command) escalates and races the
// hub's continue below. The full forward topology is separately pinned by
// TestRAGHarnessLiveEdgesHaveExactlyOneContinueBackEdge against the real YAML;
// this fixture isolates the continue re-entry mechanism on the live back-edge.
func liveRAGHarnessPostImplementFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

func TestRAGHarnessLiveEdgesHaveExactlyOneContinueBackEdge(t *testing.T) {
	def := liveRAGHarnessDefinition(t)

	var continueBack []agentpack.FlowEdge
	for _, e := range def.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), "back") &&
			strings.EqualFold(strings.TrimSpace(e.When), "continue") {
			continueBack = append(continueBack, e)
		}
	}
	if len(continueBack) != 1 {
		t.Fatalf("rag-harness must declare exactly one continue back-edge, got %d: %+v", len(continueBack), continueBack)
	}
	e := continueBack[0]
	if e.From != "validate" || e.To != "implement" {
		t.Fatalf("continue back-edge = %q -> %q, want validate -> implement", e.From, e.To)
	}
	// The synthesis hub's changes_requested (continue) resolves the SAME edge —
	// the pack validator forbids a second continue back-edge, so this lookup is
	// unambiguous for the live flow.
	if target, ok := resolveContinueBackEdgeTarget(def.Edges); !ok || target != "implement" {
		t.Fatalf("resolveContinueBackEdgeTarget = %q (ok=%v), want implement", target, ok)
	}
}

// TestRAGHarnessSynthesisContinueOnLiveEdgesReinvokesImplement drives a hub
// synthesis "continue" (changes_requested) through the LIVE edge shape — only
// validate→implement exists — proving review findings still re-enter the coder
// and that validate retries and review loops share one back-edge (session
// reuse across rounds).
func TestRAGHarnessSynthesisContinueOnLiveEdgesReinvokesImplement(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	edges, nodes := liveRAGHarnessPostImplementFixture()

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "implement first round", Label: "implement", Wait: false,
	}); err != nil {
		t.Fatalf("spawn implement: %v", err)
	}
	waitLoop(t, "implement first turn completed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 1
	})

	// Hub synthesis decides changes_requested -> continue back-edge. With the
	// live topology the re-entry target is implement (validate -> implement).
	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "Review findings: fix the nil deref and cover the restart case",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	// The hub's own auto-reinvoke turn (agent-results-ready) also flows through
	// this adapter, so wait for the implement RE-ENTRY prompt specifically.
	waitLoop(t, "implement reinvoked with review findings via live validate->implement edge", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "Feedback received") && strings.Contains(p, "fix the nil deref") {
				return true
			}
		}
		return false
	})
	// Reuse, not a fresh child: the reinvoked implement keeps its session.
	svc.mu.Lock()
	children := 0
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label == "implement" {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 1 {
		t.Fatalf("implement child count = %d, want 1 (session reuse across review rounds)", children)
	}
}

// TestRAGHarnessReviewerPromptRecordsMachineVerdictOnly locks the CP-53
// reviewer prompt contract for rag-harness: the reviewer must be told to
// record a machine verdict via submit_review_outcome (record-only), and must
// NEVER be told to drive flow routing (flow_control), matching the review-loop
// prompt hygiene rule (BUG-NOTE-CP42 #13 / CA-167).
func TestRAGHarnessReviewerPromptRecordsMachineVerdictOnly(t *testing.T) {
	tmpl, ok, err := agentpack.LoadBuiltinPrompt("prompts/review-safe-fix-contract.md")
	if err != nil || !ok {
		t.Fatalf("review-safe-fix-contract.md not loadable: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(tmpl.Contents, "submit_review_outcome") {
		t.Fatalf("rag-harness reviewer prompt must instruct submit_review_outcome (CP-53 machine verdict)")
	}
	if strings.Contains(tmpl.Contents, "flow_control") {
		t.Fatalf("rag-harness reviewer prompt must not name flow_control (reviewers never drive routing)")
	}
	// Also cover the composed prompt path: a rag-harness reviewer auto-spawn
	// (tryAdvanceFlowFromNode -> composeFlowNodeAgentPrompt) includes the static
	// review template without flow_control language.
	composed := composeFlowNodeAgentPrompt(t.TempDir(), "review base", agentpack.FlowNode{
		ID:             "reviewer",
		PromptTemplate: "prompts/review-safe-fix-contract.md",
	})
	if !strings.Contains(composed, "submit_review_outcome") {
		t.Fatalf("composed rag-harness reviewer prompt missing submit_review_outcome")
	}
	if strings.Contains(composed, "flow_control") {
		t.Fatalf("composed rag-harness reviewer prompt must not contain flow_control")
	}
}
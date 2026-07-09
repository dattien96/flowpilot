package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func TestEntryDelegateNodesReturnsOnlyNoDependencyDelegateNodes(t *testing.T) {
	def := agentpack.FlowDefinition{
		Nodes: []agentpack.FlowNode{
			{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
			{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
			{ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
			{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		},
	}
	entries := entryDelegateNodes(def)
	if len(entries) != 1 || entries[0].ID != "coder" {
		t.Fatalf("entries = %#v, want exactly [coder]", entries)
	}
}

func TestEntryDelegateNodesEmptyWhenNoMatch(t *testing.T) {
	def := agentpack.FlowDefinition{
		Nodes: []agentpack.FlowNode{
			{ID: "hub", Behavior: "hub.inline"},
			{ID: "downstream", Behavior: "agent.delegate", Agent: "agents/coder.md", DependsOn: []string{"hub"}},
		},
	}
	if entries := entryDelegateNodes(def); len(entries) != 0 {
		t.Fatalf("entries = %#v, want none", entries)
	}
}

// TestEntryDelegateNodesExcludesEdgeOnlyDependencyTarget reproduces a real
// live-test bug (found 2026-07-09 diagnosing rag-harness via
// .flowpilot/logs/features/agent-flow-engine): rag-harness.yaml's "implement"
// node declares its dependency on "context" purely through the edges list
// (`context -> implement, when: done, kind: forward`), never a dependsOn
// field. entryDelegateNodes used to check dependsOn only, so it wrongly
// classified "implement" as a zero-dependency entry delegate node —
// startResolvedFlow then spawned the coder directly and skipped
// context.produce (and startInlineEntryChain's step-status wiring)
// entirely. The fix: a node with an incoming FORWARD edge is never an entry,
// regardless of its own dependsOn list.
func TestEntryDelegateNodesExcludesEdgeOnlyDependencyTarget(t *testing.T) {
	def := agentpack.FlowDefinition{
		Nodes: []agentpack.FlowNode{
			{ID: "context", Behavior: "context.produce"},
			{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"}, // no DependsOn — matches rag-harness.yaml verbatim
		},
		Edges: []agentpack.FlowEdge{
			{From: "context", To: "implement", When: "done", Kind: "forward"},
		},
	}
	if entries := entryDelegateNodes(def); len(entries) != 0 {
		t.Fatalf("entryDelegateNodes = %#v, want none (implement has an incoming forward edge from context)", entries)
	}
	entries := entryNodesNoDeps(def)
	if len(entries) != 1 || entries[0].ID != "context" {
		t.Fatalf("entryNodesNoDeps = %#v, want exactly [context]", entries)
	}
}

// TestEntryDelegateNodesBackEdgeDoesNotDisqualifyEntry verifies the fix does
// NOT break review-loop's own shape: "coder" has an incoming BACK edge from
// "synthesis" (the continue-loop), which must never disqualify it as the
// entry node — only a FORWARD edge does.
func TestEntryDelegateNodesBackEdgeDoesNotDisqualifyEntry(t *testing.T) {
	def := agentpack.FlowDefinition{
		Nodes: []agentpack.FlowNode{
			{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
			{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		},
		Edges: []agentpack.FlowEdge{
			{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
			{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
		},
	}
	entries := entryDelegateNodes(def)
	if len(entries) != 1 || entries[0].ID != "coder" {
		t.Fatalf("entries = %#v, want exactly [coder] (a back edge must not disqualify it)", entries)
	}
}

func TestForwardDoneTargetsFindsFanOutTargets(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
		{From: "coder", To: "reviewer_correctness", When: "escalate", Kind: "forward"}, // wrong When, ignored
	}
	targets := forwardDoneTargets(edges, "coder")
	if len(targets) != 2 || targets[0] != "reviewer_correctness" || targets[1] != "reviewer_security" {
		t.Fatalf("targets = %#v, want [reviewer_correctness reviewer_security]", targets)
	}
}

func TestForwardDoneTargetsNoMatchReturnsEmpty(t *testing.T) {
	edges := []agentpack.FlowEdge{{From: "synthesis", To: "coder", When: "continue", Kind: "back"}}
	if targets := forwardDoneTargets(edges, "coder"); len(targets) != 0 {
		t.Fatalf("targets = %#v, want none", targets)
	}
}

func TestFindFlowNodeReturnsMatchByID(t *testing.T) {
	nodes := []agentpack.FlowNode{{ID: "coder", Agent: "agents/coder.md"}, {ID: "synthesis", Behavior: "hub.inline"}}
	node, ok := findFlowNode(nodes, "synthesis")
	if !ok || node.Behavior != "hub.inline" {
		t.Fatalf("findFlowNode(synthesis) = %#v, ok=%v", node, ok)
	}
	if _, ok := findFlowNode(nodes, "does-not-exist"); ok {
		t.Fatal("expected no match for unknown node id")
	}
}

// TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget proves the safety valve:
// if a forward target isn't a spawnable agent.delegate node (e.g. review-loop's
// "synthesis", which is the hub's own inline turn), tryAdvanceFlowFromNode
// returns false rather than guessing — callers fall back to the pre-existing
// note+reinvoke-hub behavior for that case.
func TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{
		{From: "reviewer_correctness", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	svc.mu.Unlock()

	if svc.tryAdvanceFlowFromNode(parent.RunID, "reviewer_correctness", "looks good") {
		t.Fatal("expected tryAdvanceFlowFromNode to bail (return false) when the target is hub.inline, not agent.delegate")
	}
}

func TestTryAdvanceFlowFromNodeLifecycleReinvokeReusesExistingTarget(t *testing.T) {
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
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke"}}
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "initial review", Label: "reviewer", Wait: false,
	}); err != nil {
		t.Fatalf("spawnChildRun(reviewer): %v", err)
	}
	waitLoop(t, "initial reviewer completed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label == "reviewer" && run.status == RunStatusCompleted {
				return true
			}
		}
		return false
	})

	before := countChildrenWithLabel(svc, parent.RunID, "reviewer")
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "new coder result") {
		t.Fatal("tryAdvanceFlowFromNode returned false; expected reinvoke target to advance")
	}
	waitLoop(t, "reviewer reinvoked", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 2 && strings.Contains(prompts[len(prompts)-1], "new coder result")
	})
	if after := countChildrenWithLabel(svc, parent.RunID, "reviewer"); after != before {
		t.Fatalf("reviewer child count = %d, want unchanged %d for lifecycle=reinvoke", after, before)
	}
}

func TestTryAdvanceFlowFromNodeLifecycleSpawnCreatesFreshTarget(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "reviewer", When: "done", Kind: "forward"}}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn"}}
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "initial review", Label: "reviewer", Wait: false,
	}); err != nil {
		t.Fatalf("spawnChildRun(reviewer): %v", err)
	}
	waitLoop(t, "initial reviewer exists", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, parent.RunID, "reviewer") == 1
	})

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "new coder result") {
		t.Fatal("tryAdvanceFlowFromNode returned false; expected spawn target to advance")
	}
	waitLoop(t, "fresh reviewer spawned", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, parent.RunID, "reviewer") == 2
	})
}

func countChildrenWithLabel(svc *InteractiveService, parentRunID, label string) int {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	count := 0
	for _, run := range svc.runs {
		if run.parentRunID == parentRunID && run.label == label {
			count++
		}
	}
	return count
}

// TestCoderCompletionAutoSpawnsReviewerCohort is the real end-to-end proof
// this whole task was about: after investigating a user question about how
// the AI hub would know to spawn a reviewer cohort, it turned out nothing
// told it to. This proves Go now spawns the cohort itself, deterministically,
// from the flow's own edge data — with no AI decision involved at all.
func TestCoderCompletionAutoSpawnsReviewerCohort(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				finalMsg := "ok"
				if strings.Contains(req.Prompt, "fix the crash") {
					// This is the coder's own first turn — simulate it finishing
					// with a result the reviewers should see.
					finalMsg = "Fixed the null pointer at handler.go:42."
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMsg})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	// The coder's own turn completes asynchronously; wait for the reviewer
	// cohort Go should auto-spawn in response, with no AI/test code deciding
	// to spawn them manually.
	var reviewerLabels []string
	waitLoop(t, "reviewer cohort auto-spawned", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		reviewerLabels = nil
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label != "coder" {
				reviewerLabels = append(reviewerLabels, run.label)
			}
		}
		return len(reviewerLabels) == 2
	})

	sort.Strings(reviewerLabels)
	want := []string{"reviewer_correctness", "reviewer_security"}
	if !reflect.DeepEqual(reviewerLabels, want) {
		t.Fatalf("reviewer labels = %v, want %v", reviewerLabels, want)
	}

	// Both reviewers must be part of the same cohort so the existing join
	// logic reinvokes the hub exactly once after both complete.
	svc.mu.Lock()
	cohortIDs := map[string]struct{}{}
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label != "coder" {
			cohortIDs[run.flowCohortId] = struct{}{}
		}
	}
	svc.mu.Unlock()
	if len(cohortIDs) != 1 {
		t.Fatalf("expected both reviewers in exactly one shared cohort, got %v", cohortIDs)
	}
	for id := range cohortIDs {
		if id == "" {
			t.Fatal("expected a non-empty shared FlowCohortID")
		}
	}
}

// TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel is the regression
// test for BUG-228: an agent.delegate flow node whose role has its own
// purpose-named step_definitions row ("flow-agent-delegate-reviewer") must
// run on that row's own configured model/provider, independent of the flow's
// own resolved model — matching the same "Flow: Reviewer" catalog entry the
// desktop Settings > Workflows > Steps editor already exposes. The coder node
// has no such row and must keep inheriting the flow's own resolved model
// (Codex here), proving this is a per-role override, not a blanket switch.
func TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel(t *testing.T) {
	reg := registryWithClaudeAvailable()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				finalMsg := "ok"
				if strings.Contains(req.Prompt, "fix the crash") {
					finalMsg = "Fixed the null pointer at handler.go:42."
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMsg})
				return nil
			})
		},
	})

	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"], Step{
		ID: "flow-agent-delegate-reviewer", Name: "Flow: Reviewer", Model: "claude-sonnet",
	})

	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	waitLoop(t, "reviewer cohort auto-spawned", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		count := 0
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label != "coder" {
				count++
			}
		}
		return count == 2
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, run := range svc.runs {
		if run.parentRunID != parent.RunID {
			continue
		}
		switch run.label {
		case "coder":
			if run.modelName != "gpt-5.4-mini" || run.providerKey != ProviderKeyCodex {
				t.Errorf("coder run = provider=%q model=%q, want codex/gpt-5.4-mini (flow's own model, no step_definitions row for coder)", run.providerKey, run.modelName)
			}
		case "reviewer_correctness", "reviewer_security":
			if run.modelName != "claude-sonnet" || run.providerKey != ProviderKeyClaude {
				t.Errorf("%s run = provider=%q model=%q, want claude/claude-sonnet (flow-agent-delegate-reviewer's own model)", run.label, run.providerKey, run.modelName)
			}
		}
	}
}

func TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"],
		Step{
			ID:     "flow-agent-delegate-reviewer",
			Name:   "Flow: Reviewer",
			Model:  "gpt-5.4-mini",
			NodeID: "",
		},
		Step{
			ID:       "flowpilot_core_flow_pack__review_loop__reviewer_correctness",
			Name:     "Review Loop: Reviewer Correctness",
			NodeID:   "reviewer_correctness",
			AgentRef: "agents/reviewer.md",
			Model:    "gpt-5.4",
		},
	)
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())

	got := svc.resolveFlowNodeModel(context.Background(), agentpack.FlowNode{
		ID:       "reviewer_correctness",
		Behavior: "agent.delegate",
		Agent:    "agents/reviewer.md",
	})
	if got != "gpt-5.4" {
		t.Fatalf("resolveFlowNodeModel = %q, want node-specific step_definitions.model gpt-5.4", got)
	}
}

// TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow is the BUG-241
// regression: an early BUG-236 draft migration copied node_id onto the generic
// dispatch-category rows (e.g. flow-agent-delegate-reviewer.node_id =
// "reviewer_correctness") and the BUG-239 repair never cleared it. Because that
// contaminated generic row also matched the node's node_id, it could win the
// node_id lookup over the node's OWN per-node row — so two graph nodes sharing
// an agent file (reviewer_correctness / reviewer_security) resolved to
// DIFFERENT models: one aliased onto the single contaminated generic row, the
// other fell through to its own per-node row. The guard must skip generic
// dispatch rows in the node_id match so BOTH reviewers resolve their own
// per-node step definition regardless of any residual contamination.
func TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"],
		// Contaminated generic dispatch row: a node_id it should never carry.
		Step{
			ID:       "flow-agent-delegate-reviewer",
			Name:     "Flow: Reviewer",
			Model:    "claude-sonnet",
			NodeID:   "reviewer_correctness",
			AgentRef: "agents/reviewer.md",
		},
		// The two real per-node rows the workflow relation actually points to.
		Step{
			ID:       "flowpilot_core_flow_pack__review_loop__reviewer_correctness",
			Name:     "Review Loop: Reviewer Correctness",
			NodeID:   "reviewer_correctness",
			AgentRef: "agents/reviewer.md",
			Model:    "gpt-5.4",
		},
		Step{
			ID:       "flowpilot_core_flow_pack__review_loop__reviewer_security",
			Name:     "Review Loop: Reviewer Security",
			NodeID:   "reviewer_security",
			AgentRef: "agents/reviewer.md",
			Model:    "claude-haiku",
		},
	)
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())

	if got := svc.resolveFlowNodeModel(context.Background(), agentpack.FlowNode{
		ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md",
	}); got != "gpt-5.4" {
		t.Fatalf("reviewer_correctness resolved = %q, want its own per-node gpt-5.4 (not the contaminated generic row's claude-sonnet)", got)
	}
	if got := svc.resolveFlowNodeModel(context.Background(), agentpack.FlowNode{
		ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md",
	}); got != "claude-haiku" {
		t.Fatalf("reviewer_security resolved = %q, want its own per-node claude-haiku", got)
	}
}

func TestResolveFlowNodeModelIgnoresHubInlineAgentRef(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"],
		Step{
			ID:         "flowpilot_core_flow_pack__review_loop__synthesis",
			Name:       "Review Loop: Synthesis",
			NodeID:     "synthesis",
			BehaviorID: "hub.inline",
			AgentRef:   "agents/synthesizer.md",
			Model:      "haiku",
		},
	)
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())

	got := svc.resolveFlowNodeModel(context.Background(), agentpack.FlowNode{
		ID:       "synthesis",
		Behavior: "hub.inline",
		Agent:    "agents/synthesizer.md",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel = %q, want empty so hub.inline inherits the parent run model", got)
	}
}

// TestCoderCompletionAutoSpawnedReviewerPromptDoesNotInstructFlowControlCall is
// the regression test for BUG-NOTE-CP42 #13: tryAdvanceFlowFromNode's
// auto-spawn prompt used to tell the reviewer to "report your findings via
// the flow's declared control tool" — i.e. call submit_review_outcome/
// flow_control itself. turnBridge.SubmitFlowControl routes a child's call
// straight to the parent's applyFlowControl with no cohort-join gate, so a
// reviewer that followed that instruction could advance/complete/block the
// whole round before the other cohort member finished, bypassing the join
// barrier entirely. This asserts the auto-spawned reviewer's prompt now
// matches every other reviewer-spawn path in this codebase: asked to review
// and report findings as its own message, never told to call any tool.
func TestCoderCompletionAutoSpawnedReviewerPromptDoesNotInstructFlowControlCall(t *testing.T) {
	var reviewerPrompt string
	var mu sync.Mutex

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if strings.Contains(req.Prompt, "Review this result from node") {
					mu.Lock()
					reviewerPrompt = req.Prompt
					mu.Unlock()
				}
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	waitLoop(t, "reviewer prompt observed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reviewerPrompt != ""
	})

	mu.Lock()
	got := reviewerPrompt
	mu.Unlock()
	for _, forbidden := range []string{"control tool", "flow_control", "submit_review_outcome"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("auto-spawned reviewer prompt = %.300q, must not instruct it to call any control tool (found %q) — "+
				"only the hub's own synthesis turn after cohort join may do that", got, forbidden)
		}
	}
}

func TestCoderCompletionAutoAdvanceDoesNotScheduleEmptyHubReinvoke(t *testing.T) {
	var mu sync.Mutex
	reviewerPrompts := 0
	synthesisPrompts := 0
	releaseReviewers := make(chan struct{})
	var closeRelease sync.Once
	t.Cleanup(func() { closeRelease.Do(func() { close(releaseReviewers) }) })

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, req TurnRequest, b TurnBridge) error {
				switch {
				case strings.Contains(req.Prompt, "Review this result from node"):
					mu.Lock()
					reviewerPrompts++
					mu.Unlock()
					select {
					case <-releaseReviewers:
					case <-ctx.Done():
						return ctx.Err()
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "approved"})
				case strings.Contains(req.Prompt, "[flow-engine] Agent results ready"):
					mu.Lock()
					synthesisPrompts++
					mu.Unlock()
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "synthesized"})
				default:
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "implemented"})
				}
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	waitLoop(t, "reviewer cohort started", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return reviewerPrompts == 2
	})

	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	got := synthesisPrompts
	mu.Unlock()
	if got != 0 {
		t.Fatalf("hub was reinvoked before reviewer cohort joined; synthesis prompts before reviewer release = %d", got)
	}

	closeRelease.Do(func() { close(releaseReviewers) })
}

func TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote(t *testing.T) {
	var synthesisPrompt string
	var mu sync.Mutex

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				switch {
				case strings.Contains(req.Prompt, "[flow-engine] Agent results ready") &&
					strings.Contains(req.Prompt, "[flow-engine joined result note]"):
					mu.Lock()
					synthesisPrompt = req.Prompt
					mu.Unlock()
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "synthesized"})
				case strings.Contains(req.Prompt, "Review this result from node"):
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "approved"})
				default:
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "implemented"})
				}
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	waitLoop(t, "synthesis prompt captured", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return synthesisPrompt != ""
	})

	mu.Lock()
	got := synthesisPrompt
	mu.Unlock()
	for _, want := range []string{
		"[flow-engine joined result note]",
		`"reviewer_correctness"`,
		`"reviewer_security"`,
		"[flow-engine] Agent results ready.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("synthesis prompt missing %q:\n%s", want, got)
		}
	}
}

// TestForwardAutoAdvanceFiresForNonCoderNamedEntryNode is the regression
// test for BUG-NOTE-CP42 #17: the EventTurnCompleted handler only tried
// tryAdvanceFlowFromNode (via advanceOrNotifyHub) when the completing child
// was isCoderRun(rs) — a role/agent-name match on "coder". A custom,
// user-authored flow whose entry delegate node uses any other agent/role
// name (e.g. "planner") would never reach the generic auto-advance path at
// all: its completion fell through to a passive "Sub-agent completed" note
// that never reinvokes the hub, silently stalling the flow. This proves a
// tracked flow now auto-advances regardless of the entry node's role name.
func TestForwardAutoAdvanceFiresForNonCoderNamedEntryNode(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "plan ready"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// A custom two-node flow whose entry node is "planner" (not "coder"),
	// tracked directly on the parent the way startResolvedFlow would.
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{
		{From: "planner", To: "reviewer", When: "done", Kind: "forward"},
	}
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "planner", Behavior: "agent.delegate", Agent: "agents/planner.md"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
	}
	svc.mu.Unlock()

	// "planner" is not in the catalog, so its role/agentName never contain
	// "coder" — isCoderRun would be false for this child.
	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "planner", Prompt: "draft a plan", Wait: false, Label: "planner", AutoOrchestrate: true,
	}); err != nil {
		t.Fatalf("spawnChildRun(planner): %v", err)
	}

	waitLoop(t, "reviewer auto-spawned from a non-coder-named entry node", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label == "reviewer" {
				return true
			}
		}
		return false
	})
}

func TestResolveContinueBackEdgeTargetFindsBackContinueEdge(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
		{From: "reviewer_correctness", To: "synthesis", When: "done", Kind: "forward"},
		{From: "reviewer_security", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
		{From: "synthesis", To: "done", When: "done", Kind: "forward"},
		{From: "synthesis", To: "ask_user", When: "escalate", Kind: "forward"},
	}
	target, ok := resolveContinueBackEdgeTarget(edges)
	if !ok || target != "coder" {
		t.Fatalf("target = %q, ok = %v; want (coder, true)", target, ok)
	}
}

func TestResolveContinueBackEdgeTargetNoMatchReturnsFalse(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "reviewer", When: "done", Kind: "forward"},
	}
	if _, ok := resolveContinueBackEdgeTarget(edges); ok {
		t.Fatal("expected no match for edges with no back/continue edge")
	}
	if _, ok := resolveContinueBackEdgeTarget(nil); ok {
		t.Fatal("expected no match for nil edges")
	}
}

func TestFlowNodeAgentNameDerivesFromFilePath(t *testing.T) {
	cases := map[string]string{
		"agents/coder.md":    "coder",
		"agents/reviewer.md": "reviewer",
		"coder.md":           "coder",
		"":                   "",
	}
	for agent, want := range cases {
		got := flowNodeAgentName(agentpack.FlowNode{Agent: agent})
		if got != want {
			t.Errorf("flowNodeAgentName(%q) = %q, want %q", agent, got, want)
		}
	}
}

// TestStartResolvedFlowSpawnsOnlyTheEntryNode is the integration-level proof
// that a resolved flowRef actually spawns something: review-loop's entry
// node ("coder") should be spawned as a child, while the downstream
// reviewer/synthesis nodes (reached only via edges/dependsOn, not spawned
// directly) must not be.
func TestStartResolvedFlowSpawnsOnlyTheEntryNode(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the null pointer bug")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var coderChildren, otherChildren int
	for _, run := range svc.runs {
		if run.parentRunID != parent.RunID {
			continue
		}
		if run.role == "coder" || run.agentName == "coder" {
			coderChildren++
		} else {
			otherChildren++
		}
	}
	if coderChildren != 1 {
		t.Fatalf("coder children = %d, want 1 (svc.runs snapshot: %d total children)", coderChildren, coderChildren+otherChildren)
	}
	if otherChildren != 0 {
		t.Fatalf("expected no non-coder children spawned directly, got %d", otherChildren)
	}

	// The spawn must have set explicit mode + autoOrchestrate, mirroring what
	// an AI-driven autoOrchestrate=true spawn_agent call already does.
	if svc.agentOrchestrator.loopMode(parent.RunID) != "explicit" {
		t.Errorf("loopMode = %q, want explicit", svc.agentOrchestrator.loopMode(parent.RunID))
	}
}

// TestStartResolvedFlowNotifiesHubToWait proves the hub (parent run) gets a
// pending note telling it an agent already started, so it doesn't
// redundantly try to also do the coding itself (per user direction: prompt
// the main agent that work is already underway and it should wait).
func TestStartResolvedFlowNotifiesHubToWait(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the null pointer bug")

	svc.mu.Lock()
	notes := append([]string(nil), svc.runs[parent.RunID].pendingAgentContext...)
	svc.mu.Unlock()

	found := false
	for _, n := range notes {
		if strings.Contains(n, "already been spawned") && strings.Contains(n, "duplicate that work") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a wait notice in hub's pending context, got %#v", notes)
	}
}

// TestStartResolvedFlowNoNotifyWhenNothingSpawned proves the hub is not
// notified when the flow's entry node could not be spawned — a notice
// telling the hub to "wait" would be actively wrong if nothing is running.
func TestStartResolvedFlowNoNotifyWhenNothingSpawned(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/does-not-exist", "fix the bug")

	svc.mu.Lock()
	notes := append([]string(nil), svc.runs[parent.RunID].pendingAgentContext...)
	svc.mu.Unlock()
	if len(notes) != 0 {
		t.Fatalf("expected no pending notes when nothing was spawned, got %#v", notes)
	}
}

// TestStartTurnWithFlowRefSpawnsEntryNodeAsynchronously proves the actual
// wiring path (startTurn -> go startResolvedFlow), not just the underlying
// function in isolation: a first turn carrying SubMode/FlowRef must result
// in a coder child appearing, while the hub stays quiet until reinvoked.
func TestStartTurnWithFlowRefSpawnsEntryNodeAsynchronously(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
		StepID:  "chat-" + parent.RunID,
		Prompt:  "fix the crash on startup",
		SubMode: "bug",
		FlowRef: "flowpilot-core-flow-pack/review-loop",
	}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		found := false
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && (run.role == "coder" || run.agentName == "coder") {
				found = true
				break
			}
		}
		svc.mu.Unlock()
		if found {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no coder child appeared within 2s of starting the turn with flowRef set")
}

// TestChatModeHandleStartTurnMarksRunFlowEngineDriven is the regression test
// for the Chat Mode Review Loop gap: every other startResolvedFlow/startTurn
// test in this file calls svc.startTurn(...) directly, bypassing
// handleStartTurn (the actual HTTP handler a real desktop "bug" sub-mode
// chat turn hits, per apps/desktop-flowpilot/src/state/store.ts's
// subMode/flowRef payload). That let a real defect through unnoticed:
// handleStartTurn only called markFlowEngineDriven on its own
// resolveWorkflowFlowRef branch (the Flow-Mode workflow-picker path), never
// on the explicit chat flowRef path — so a Chat Mode review-loop run spawned
// its entry node correctly but silently never got flagged flow-engine-driven,
// disabling every isFlowEngineDriven-gated behavior (BUG-174's legacy-planner
// suppression, BUG-226's no-tool-call escalation, BUG-234's step settlement)
// for Chat Mode only. Fixed by setting flowEngineDriven inline in startTurn
// itself, covering both paths by construction. This test drives the real
// HTTP route so a future regression in either handleStartTurn or startTurn
// is caught here, not just in a lower-level unit test.
func TestChatModeHandleStartTurnMarksRunFlowEngineDriven(t *testing.T) {
	svc, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	// Mirrors exactly what the desktop's first chat turn sends when the user
	// picked "Review Loop" from the "Bug" sub-mode's built-in orchestration
	// picker (store.ts's sendPrompt: subMode="bug", flowRef=<picked option>).
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId":  handle.StepID,
		"prompt":  "fix the crash on startup",
		"subMode": "bug",
		"flowRef": "flowpilot-core-flow-pack/review-loop",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	waitLoop(t, "coder entry node spawned via the real HTTP handleStartTurn route", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, handle.RunID, "coder") == 1
	})

	if !svc.isFlowEngineDriven(handle.RunID) {
		t.Fatal("run started via Chat Mode's explicit flowRef (the \"bug\" sub-mode picker) must be flagged " +
			"flow-engine-driven, exactly like a Flow-Mode workflow-picker launch — otherwise the legacy bulk " +
			"step planner, the BUG-226 escalation safety net, and BUG-234's step settlement are all silently " +
			"disabled for Chat Mode")
	}
}

// TestChatModeRunHistoryExposesOrchestrationPickerSelection is the
// regression test for BUG-263: a run started via Chat Mode's explicit
// flowRef picker must expose its subMode/flowRef in the run-history API
// response, so the desktop can restore the Chat Intent panel's Bug tab /
// Built-in orchestration selection after reopening the run (e.g. following a
// runner restart) instead of silently falling back to "Normal".
func TestChatModeRunHistoryExposesOrchestrationPickerSelection(t *testing.T) {
	svc, srv := newTestServer(t)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId":  handle.StepID,
		"prompt":  "fix the crash on startup",
		"subMode": "bug",
		"flowRef": "flowpilot-core-flow-pack/review-loop",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	waitLoop(t, "coder entry node spawned", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, handle.RunID, "coder") == 1
	})

	history := svc.projectRunHistory("proj")
	var item *runHistoryItem
	for i := range history {
		if history[i].RunID == handle.RunID {
			item = &history[i]
			break
		}
	}
	if item == nil {
		t.Fatalf("run %q not found in project run history", handle.RunID)
	}
	if item.SubMode != "bug" {
		t.Fatalf("runHistoryItem.SubMode = %q, want %q", item.SubMode, "bug")
	}
	if item.FlowRef != "flowpilot-core-flow-pack/review-loop" {
		t.Fatalf("runHistoryItem.FlowRef = %q, want %q", item.FlowRef, "flowpilot-core-flow-pack/review-loop")
	}
}

// TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn is the
// regression test for BUG-261. It reproduces the exact live failure mode: an
// explicit chat flowRef (Bug sub-mode picker) whose stored mirror definition
// is corrupted/invalid (a node's dependsOn references a node id that doesn't
// exist — the same shape as the live "claude-review-fake-model" corruption)
// still passes validateChatOrchestrationSelection's option check (it only
// checks the flowRef string is a known option, not that the stored
// definition resolves), so it used to reach startTurn, which unconditionally
// suppressed the hub's own turn (flowStartOnly=true) before
// startResolvedFlow's async resolve ever ran and failed. With nothing ever
// spawned to reinvoke the hub, the run got stuck forever at "completed" with
// no assistant reply. The fix resolves the flowRef synchronously in
// handleStartTurn and clears it on failure, so the turn falls through to a
// normal provider call instead.
func TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn(t *testing.T) {
	store := newFakeFlowDefinitionStore()
	if _, err := NewFlowMirrorSyncService(store).SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}
	rec, ok, err := store.GetByPackFlow(context.Background(), "flowpilot-core-flow-pack", "review-loop")
	if err != nil || !ok {
		t.Fatalf("expected mirrored review-loop row, ok=%v err=%v", ok, err)
	}
	// Corrupt the stored mirror exactly like the live incident: a node's
	// dependsOn references a node id that does not exist anywhere in the flow.
	for i, node := range rec.Definition.Nodes {
		if node.ID == "synthesis" {
			rec.Definition.Nodes[i].DependsOn = append(node.DependsOn, "claude-review-fake-model")
		}
	}
	if _, err := store.Upsert(context.Background(), rec); err != nil {
		t.Fatalf("seed corrupted mirror row: %v", err)
	}
	// Sanity-check the corruption actually breaks resolution, so this test
	// would fail loudly (not silently pass for an unrelated reason) if the
	// fixture setup above ever stops matching agentpack.ValidateFlowDefinition's
	// checks.
	if _, err := NewFlowDefinitionResolver(store).ResolveFlowRef(context.Background(), "flowpilot-core-flow-pack/review-loop"); err == nil {
		t.Fatal("fixture setup did not actually corrupt the mirror row; resolve unexpectedly succeeded")
	}

	svc, srv := newTestServer(t)
	svc.SetFlowDefinitionStore(store)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start chat run status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId":  handle.StepID,
		"prompt":  "fix bug 1+1 != 2",
		"subMode": "bug",
		"flowRef": "flowpilot-core-flow-pack/review-loop",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("send chat turn status=%d body=%s", status, body)
	}

	waitLoop(t, "turn settles via a real provider call, not the flow-handoff short-circuit", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[handle.RunID]
		return rs != nil && !rs.turnInFlight && rs.lastEventType == EventTurnCompleted
	})

	if countChildrenWithLabel(svc, handle.RunID, "coder") != 0 {
		t.Fatal("no child should ever spawn when the flowRef fails to resolve")
	}
	if svc.isFlowEngineDriven(handle.RunID) {
		t.Fatal("a run whose flowRef failed to resolve must not be flagged flow-engine-driven — nothing ever started")
	}

	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var gotReply bool
	for _, ev := range events {
		if ev.Type == EventMessageCompleted && strings.TrimSpace(ev.Text) != "" {
			gotReply = true
		}
	}
	if !gotReply {
		t.Fatalf("expected a real assistant reply (EventMessageCompleted with text) once the turn fell back to normal chat; events=%#v", events)
	}
	last := events[len(events)-1]
	if last.Type != EventTurnCompleted || strings.TrimSpace(last.FinalMessage) == "" {
		t.Fatalf("expected a non-empty terminal EventTurnCompleted (the pre-fix bug produced an empty synthetic one), got %#v", last)
	}
}

// TestContinueReinvokeUsesEdgeResolvedTargetForFlowStartedRun proves the
// Task-180 edge-driven path end to end: a run started via startResolvedFlow
// tracks the flow's edges, and applyFlowControl("continue") reinvokes the
// child by resolving the synthesis->coder back-edge — not by scanning for a
// role literally named "coder" (isCoderRun is never consulted when
// activeFlowEdges is set).
func TestContinueReinvokeUsesEdgeResolvedTargetForFlowStartedRun(t *testing.T) {
	reentryPrompts := make(chan string, 5)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if strings.Contains(req.Prompt, "the fix wasn't complete") {
					reentryPrompts <- req.Prompt
				}
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// startResolvedFlow is called synchronously here (not via the async `go`
	// wrapper startTurn uses), so activeFlowEdges and the entry-node child are
	// both set by the time this call returns.
	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	svc.mu.Lock()
	edgeCount := len(svc.runs[parent.RunID].activeFlowEdges)
	svc.mu.Unlock()
	if edgeCount == 0 {
		t.Fatal("expected activeFlowEdges to be set after startResolvedFlow")
	}

	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status:  "continue",
		Summary: "the fix wasn't complete, please address the remaining issue",
	}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	select {
	case prompt := <-reentryPrompts:
		if !strings.Contains(prompt, "the fix wasn't complete") {
			t.Errorf("re-entry prompt = %.200q, want feedback text", prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("coder re-entry did not fire within 3s after applyFlowControl(continue)")
	}
}

// TestStartTurnWithFlowRefSkipsHubsOwnFirstProviderTurn guards the Flow Mode
// handoff path: the first user turn should start the flow entry child, but the
// parent hub must not also send a provider turn that can show a main-agent
// response before the child result is ready.
func TestStartTurnWithFlowRefSkipsHubsOwnFirstProviderTurn(t *testing.T) {
	var seenParent bool
	var seenChild bool
	parentRunID := ""
	var mu sync.Mutex

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				if req.RunID == parentRunID {
					seenParent = true
				} else {
					seenChild = true
				}
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
	parentRunID = parent.RunID

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
		StepID:  "chat-" + parent.RunID,
		Prompt:  "fix the crash on startup",
		SubMode: "bug",
		FlowRef: "flowpilot-core-flow-pack/review-loop",
	}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	waitLoop(t, "child provider turn observed by fake adapter", 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return seenChild
	})

	mu.Lock()
	gotParent := seenParent
	mu.Unlock()
	if gotParent {
		t.Fatal("hub parent provider was called on the first flowRef turn; want silent handoff to child")
	}
}

func TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
		StepID:  "chat-" + parent.RunID,
		Prompt:  "fix the crash on startup",
		SubMode: "bug",
		FlowRef: "flowpilot-core-flow-pack/review-loop",
	}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	waitLoop(t, "synthetic hub turn settled", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[parent.RunID]
		return rs != nil && !rs.turnInFlight && rs.lastEventType == EventTurnCompleted
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if rs == nil {
		t.Fatal("parent run missing")
	}
	if rs.lastEventType != EventTurnCompleted {
		t.Fatalf("last event type = %s, want %s", rs.lastEventType, EventTurnCompleted)
	}
	if len(rs.events) < 2 {
		t.Fatalf("events = %d, want at least turn_started + turn_completed", len(rs.events))
	}
	if rs.events[0].Type != EventTurnStarted {
		t.Fatalf("first event = %s, want %s", rs.events[0].Type, EventTurnStarted)
	}
	if rs.events[len(rs.events)-1].Type != EventTurnCompleted {
		t.Fatalf("last event = %s, want %s", rs.events[len(rs.events)-1].Type, EventTurnCompleted)
	}
}

func TestStartResolvedFlowChildInheritsWorkflowYoloDefault(t *testing.T) {
	childYolo := make(chan bool, 1)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case childYolo <- req.YoloMode:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := baseTestCatalog()
	catalog.workflows["proj-1"][0].YoloMode = true
	catalog.steps["wf-1"][0].Model = "gpt-5.4-mini"
	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	parentYolo := svc.runs[parent.RunID].yolo
	svc.mu.Unlock()
	if !parentYolo {
		t.Fatal("parent run yolo = false, want true from workflow yolo_mode")
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash on startup")

	select {
	case got := <-childYolo:
		if !got {
			t.Fatal("flow child turn YoloMode = false, want true inherited from parent workflow default")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("flow child turn did not start")
	}
}

// TestStartResolvedFlowStartsInlineEntryFlow is the regression test for
// BUG-NOTE-CP42 #9: rag-harness's entry node ("context") is an inline
// context.produce node, not an agent.delegate node, so entryDelegateNodes
// alone would find nothing to spawn and the flow would silently do nothing.
// This proves startResolvedFlow now runs that inline node synchronously and
// spawns the next agent.delegate node ("implement") it forward-edges to.
func TestStartResolvedFlowStartsInlineEntryFlow(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/rag-harness", "add retrieval for the docs feature")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var implementChildren, otherChildren int
	for _, run := range svc.runs {
		if run.parentRunID != parent.RunID {
			continue
		}
		if run.label == "implement" {
			implementChildren++
		} else {
			otherChildren++
		}
	}
	if implementChildren != 1 {
		t.Fatalf("implement children = %d, want 1 (other children: %d)", implementChildren, otherChildren)
	}
	if len(svc.runs[parent.RunID].activeFlowEdges) == 0 {
		t.Fatal("expected activeFlowEdges to be tracked after an inline-entry flow start")
	}
	// BUG (found 2026-07-09 live-testing rag-harness): the assertions above
	// (a lone "implement" child + non-empty activeFlowEdges) are satisfied
	// EQUALLY by the correct path (context.produce runs, then implement is
	// spawned) and by the bug (entryDelegateNodes wrongly treats "implement"
	// itself as the entry, skipping context.produce entirely) — this test
	// passed even while that bug was live. planContextPackage is only ever
	// set by startInlineEntryChain after a successful context.produce
	// dispatch, so asserting it here closes that exact blind spot.
	if svc.runs[parent.RunID].planContextPackage == nil {
		t.Fatal("expected planContextPackage to be set — context.produce must run before implement is spawned, not be bypassed by it")
	}
}

// TestStartResolvedFlowSpawnsPackAgentEvenWhenProjectShadowsItsName is the
// regression test for BUG-NOTE-CP42 #23: flowNodeAgentName reduces a node's
// pack file path ("agents/coder.md") to the bare catalog name "coder", and
// spawnChildRun used to resolve that name through AgentCatalog.listAgents,
// whose precedence is project-local > provider-home > built-in. A repo that
// happens to define its own .claude/agents/coder.md (for any unrelated
// purpose) would silently replace the built-in Review Loop's coder agent —
// breaking the "pack-driven, not hardcoded" guarantee. This proves the fix:
// the spawned coder still carries the pack's own role/system prompt even
// with a same-named project-local agent present.
func TestStartResolvedFlowSpawnsPackAgentEvenWhenProjectShadowsItsName(t *testing.T) {
	svc, _ := newTestServer(t)

	workspace := t.TempDir()
	agentsDir := filepath.Join(workspace, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir agents dir: %v", err)
	}
	shadow := "---\nname: coder\ndescription: unrelated project agent that happens to share the pack's agent name\nrole: hijacked\n---\n\nThis is not the FlowPilot pack's coder agent.\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "coder.md"), []byte(shadow), 0o644); err != nil {
		t.Fatalf("write shadow agent: %v", err)
	}

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = workspace
	svc.mu.Unlock()

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the null pointer bug")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var coderChild *interactiveRun
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label == "coder" {
			coderChild = run
		}
	}
	if coderChild == nil {
		t.Fatal("expected a coder child to be spawned")
	}
	if coderChild.role == "hijacked" {
		t.Fatalf("coder child role = %q; the project-local .claude/agents/coder.md shadowed the pack's own coder agent", coderChild.role)
	}
	if coderChild.role != "coder" {
		t.Fatalf("coder child role = %q, want the pack's own role %q", coderChild.role, "coder")
	}
}

func TestStartResolvedFlowUnknownFlowRefSpawnsNothing(t *testing.T) {
	svc, _ := newTestServer(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/does-not-exist", "do something")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID {
			t.Fatalf("expected no children spawned for an unknown flowRef, found %+v", run)
		}
	}
}

// TestCustomUserOwnedFlowResolvesSpawnsEntryAndAdvancesEdge is Task-189's own
// DoD item: "a runner test that a from-scratch custom workflow (step-
// definitions with behavior/agent + edges) resolves via
// resolveWorkflowFlowRef and spawns its entry node + advances an edge."
//
// Unlike every other startResolvedFlow test in this file, the flowRef here is
// NOT one of the two builtin pack flows (review-loop/rag-harness) — it is a
// bare UUID-style ref registered directly on a fakeFlowDefinitionStore with
// Source: "supabase_user_definition", exactly the shape a brand-new workflow
// authored from scratch in Settings (Task-189 slices 2-4: behavior picker +
// edges form-list/canvas) would produce once saved. This proves the "no
// schema/runtime change needed" claim in Task-189's own AI Quick View: a
// custom flow that only ever exists as step_definitions + workflow edges
// resolves and runs through the exact same path a built-in does.
func TestCustomUserOwnedFlowResolvesSpawnsEntryAndAdvancesEdge(t *testing.T) {
	const customFlowRef = "33333333-3333-3333-3333-333333333333"

	// spawnChildRun propagates the parent's workflowID onto every child's own
	// createRun call, which — for ANY workflowID, not just a flow's own ref —
	// looks up catalog step rows via ListWorkflowSteps to seed that child's
	// step timeline (createRun, interactive_handlers.go). A real custom flow
	// authored via Task-189 always has real workflow_steps rows alongside its
	// step_definitions, so this seeds the same thing here: one catalog step
	// row for customFlowRef, just enough for createRun's "has enabled steps"
	// check to pass for the entry node's own child run.
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := newInteractiveCatalog()
	catalog.steps[customFlowRef] = []Step{
		{ID: "drafting", WorkflowID: customFlowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
	}
	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())

	store := newFakeFlowDefinitionStore()
	store.byRef[customFlowRef] = FlowDefinitionRecord{
		FlowRef:   customFlowRef,
		Name:      "My Custom Flow",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID: "custom-two-step",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
				{ID: "final_check", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
			},
			Edges: []agentpack.FlowEdge{
				{From: "drafting", To: "final_check", When: "done", Kind: "forward"},
			},
		},
	}
	svc.SetFlowDefinitionStore(store)

	// A Flow Mode workflow-picker launch: workflowID carries the user-owned
	// workflow row's UUID, not a flowRef (mirrors what resolveWorkflowFlowRef's
	// own doc comment describes as its trigger case).
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parent.RunID].workflowID = customFlowRef
	svc.mu.Unlock()

	ref, ok := svc.resolveWorkflowFlowRef(context.Background(), parent.RunID)
	if !ok {
		t.Fatal("resolveWorkflowFlowRef returned false; want it to resolve the custom user-owned flow")
	}
	if ref != customFlowRef {
		t.Fatalf("resolved flowRef = %q, want %q", ref, customFlowRef)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, ref, "draft the feature")

	if countChildrenWithLabel(svc, parent.RunID, "drafting") != 1 {
		t.Fatal("expected the custom flow's entry node (drafting) to be spawned as a child")
	}
	if countChildrenWithLabel(svc, parent.RunID, "final_check") != 0 {
		t.Fatal("final_check depends on drafting and must not be spawned until the edge advances")
	}

	// The entry node's turn completes asynchronously (the fake adapter emits
	// EventTurnCompleted in its own goroutine); production wiring
	// (parentHasTrackedFlow's branch in the EventTurnCompleted handler, see
	// interactive_service.go's advanceOrNotifyHub call) then auto-advances via
	// tryAdvanceFlowFromNode on its own — driven purely by the custom flow's
	// own edge data set by startResolvedFlow above, with no test code deciding
	// to spawn final_check itself. Calling tryAdvanceFlowFromNode directly
	// here as well would double-spawn (BUG-NOTE: it doesn't dedupe against a
	// prior identical advance), so this only waits for it.
	waitLoop(t, "final_check auto-spawned once drafting's forward done edge advanced", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, parent.RunID, "final_check") == 1
	})
}

// TestCustomFlowWithThreeReviewersJoinsAfterAllComplete directly answers a
// concrete question raised while reviewing CP-36's E2E guide's Scenario 9
// (N=3 parallel reviewers): the built-in Review Loop's own review-loop.yaml
// hardcodes exactly 2 reviewer nodes, so that scenario can no longer be
// driven by prompt text (the retired agent-review-loop skill used to let the
// prompt say "spawn 3 reviewers"). The question was whether a from-scratch
// CUSTOM flow authored via Task-189's UI (behavior picker + edges canvas) —
// not a built-in, and not editing one — could declare 3 reviewer-equivalent
// nodes fanning out from one entry node and have the cohort/join mechanism
// correctly wait for all 3 (not 2) before advancing, proving the underlying
// engine has no hardcoded reviewer-count assumption anywhere.
//
// This is a genuinely different flow topology from every other test in this
// file (3-way fan-out, not 2), registered as a bare-UUID
// supabase_user_definition record exactly like
// TestCustomUserOwnedFlowResolvesSpawnsEntryAndAdvancesEdge — the same shape
// Task-189's editor would produce for "coder + 3 reviewer nodes, all
// dependsOn:[drafting], same cohort, forward-edged from drafting".
func TestCustomFlowWithThreeReviewersJoinsAfterAllComplete(t *testing.T) {
	const customFlowRef = "44444444-4444-4444-4444-444444444444"

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := newInteractiveCatalog()
	catalog.steps[customFlowRef] = []Step{
		{ID: "drafting", WorkflowID: customFlowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
	}
	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())

	store := newFakeFlowDefinitionStore()
	store.byRef[customFlowRef] = FlowDefinitionRecord{
		FlowRef:   customFlowRef,
		Name:      "My Custom 3-Reviewer Flow",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID: "custom-three-reviewer",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
				{ID: "review_a", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
				{ID: "review_b", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
				{ID: "review_c", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
			},
			Edges: []agentpack.FlowEdge{
				{From: "drafting", To: "review_a", When: "done", Kind: "forward"},
				{From: "drafting", To: "review_b", When: "done", Kind: "forward"},
				{From: "drafting", To: "review_c", When: "done", Kind: "forward"},
			},
		},
	}
	svc.SetFlowDefinitionStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parent.RunID].workflowID = customFlowRef
	svc.mu.Unlock()

	ref, ok := svc.resolveWorkflowFlowRef(context.Background(), parent.RunID)
	if !ok {
		t.Fatal("resolveWorkflowFlowRef returned false; want it to resolve the custom 3-reviewer flow")
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, ref, "draft the feature")

	// The entry node's completion auto-advances all 3 forward "done" edges at
	// once (forwardDoneTargets returns every matching edge, not just the
	// first) — this is the fan-out half of the question.
	waitLoop(t, "all 3 reviewer nodes auto-spawned from drafting's 3 forward edges", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, parent.RunID, "review_a") == 1 &&
			countChildrenWithLabel(svc, parent.RunID, "review_b") == 1 &&
			countChildrenWithLabel(svc, parent.RunID, "review_c") == 1
	})

	// All 3 must share one cohort (this is what makes the join wait for
	// exactly 3, not 2) — the fan-out loop in tryAdvanceFlowFromNode sets
	// CohortSize: len(targetNodes), which is 3 here, not a hardcoded 2. The
	// actual registered expected-count lives in the orchestrator's internal
	// cohortExpected map (unexported), so it's proven behaviorally below
	// instead: the hub reinvoke must wait for all 3, not fire after 2.
	svc.mu.Lock()
	cohortIDs := map[string]struct{}{}
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label != "drafting" {
			cohortIDs[run.flowCohortId] = struct{}{}
		}
	}
	svc.mu.Unlock()
	if len(cohortIDs) != 1 {
		t.Fatalf("expected all 3 reviewers in exactly one shared cohort, got %v", cohortIDs)
	}
	for id := range cohortIDs {
		if id == "" {
			t.Fatal("expected a non-empty shared FlowCohortID")
		}
	}

	// The fan-in half of the question: the hub (this test's parent run, which
	// never took its own first turn — startResolvedFlow was called directly,
	// not through startTurn) must be reinvoked exactly once, driven by the
	// cohort join completing with all 3 members — proving the join genuinely
	// counts to 3, not silently proceeding after 2 the way a hardcoded-2
	// assumption would. All 3 reviewers' turns already completed (the fake
	// adapter is synchronous per spawn), so by the time the join buffer holds
	// all 3 results the cohort-complete reinvoke should fire exactly once,
	// taking turnCount from 0 to 1.
	waitLoop(t, "hub reinvoked after all 3 reviewers joined", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.runs[parent.RunID].turnCount >= 1
	})
	svc.mu.Lock()
	finalTurnCount := svc.runs[parent.RunID].turnCount
	svc.mu.Unlock()
	if finalTurnCount != 1 {
		t.Fatalf("hub turnCount = %d, want exactly 1 (one reinvoke after the 3-member join) — "+
			"a value >1 would mean the hub was reinvoked more than once for one join", finalTurnCount)
	}
}

// TestStartResolvedFlowAppliesConfiguredCapAndExtendBy is the regression test
// for the gap found while reviewing CP-36's Scenario 3 (cap override): no
// code path ever applied a flow's own Definition.Policy.Cap/ExtendBy to a
// fresh run's loop state — effectiveCap()'s hardcoded fallback of 3, and
// extendCap/resumeFlowWithFeedback's hardcoded defaultExtendBy=2, silently
// governed EVERY flow regardless of its own configured policy_cap/
// policy_extend_by. The built-in Review Loop's own YAML happens to declare
// cap=3/extendBy=2, which is exactly why this went unnoticed — it matched
// the hardcoded fallback by coincidence.
//
// Registers a custom flow with Policy.Cap=2, Policy.ExtendBy=5 (deliberately
// different from the old hardcoded 3/2) and proves both take effect: the
// loop's Cap is 2 immediately after start (not 3), and extendCap raises it
// by 5 (not 2).
func TestStartResolvedFlowAppliesConfiguredCapAndExtendBy(t *testing.T) {
	const customFlowRef = "55555555-5555-5555-5555-555555555555"

	svc, _ := newTestServer(t)
	catalog := newInteractiveCatalog()
	catalog.steps[customFlowRef] = []Step{
		{ID: "drafting", WorkflowID: customFlowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
	}
	svc.catalog = catalog

	store := newFakeFlowDefinitionStore()
	store.byRef[customFlowRef] = FlowDefinitionRecord{
		FlowRef:   customFlowRef,
		Name:      "My Custom Capped Flow",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID: "custom-capped",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
			},
			Policy: agentpack.FlowPolicy{Cap: 2, ExtendBy: 5},
		},
	}
	svc.SetFlowDefinitionStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.startResolvedFlow(context.Background(), parent.RunID, customFlowRef, "draft the feature")

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Cap != 2 {
		t.Fatalf("loop.Cap = %d, want 2 (the custom flow's own Policy.Cap) — the old hardcoded fallback of 3 must not apply", st.Cap)
	}
	if st.ExtendBy != 5 {
		t.Fatalf("loop.ExtendBy = %d, want 5 (the custom flow's own Policy.ExtendBy)", st.ExtendBy)
	}

	// Drive the loop to its (now genuinely 2, not 3) cap and confirm it
	// blocks at exactly round 2, not 3.
	svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Round = 2
		return st
	})
	result, flowErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "continue", Summary: "still open issues"})
	if flowErr != nil {
		t.Fatalf("applyFlowControl(continue): %v", flowErr)
	}
	if result.NextAction != "awaiting_user" {
		t.Fatalf("NextAction = %q, want awaiting_user — the loop should block at round 2 since Cap=2, not silently allow a 3rd round", result.NextAction)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status != "blocked" {
		t.Fatalf("loop.Status = %q, want blocked", st.Status)
	}

	// extendCap must raise Cap by the flow's own ExtendBy (5), not the old
	// hardcoded 2 — new cap should be 2+5=7, not 2+2=4.
	extendResult, extendErr := svc.extendCap(parent.RunID)
	if extendErr != nil {
		t.Fatalf("extendCap: %v", extendErr)
	}
	if extendResult.Cap != 7 {
		t.Fatalf("extendCap raised Cap to %d, want 7 (2 + the flow's own ExtendBy=5, not the old hardcoded +2=4)", extendResult.Cap)
	}
}

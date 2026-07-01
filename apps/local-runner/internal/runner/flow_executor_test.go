package runner

import (
	"context"
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
// in a coder child appearing, without blocking the turn's own response.
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

// TestStartTurnWithFlowRefPrependsWaitNoticeToHubsOwnFirstTurn is the
// regression test for BUG#1 (BUG-NOTE-CP42): notifyHubFlowStarted used to
// append its wait notice to pendingAgentContext from startResolvedFlow's own
// goroutine, which races startTurn's synchronous drain of that same field a
// few lines later — the note reliably lost the race and never reached the
// hub's own first-turn prompt. This proves the fix (prepending the notice
// synchronously to the turn's prompt before the goroutine is even dispatched)
// by asserting the wait notice is present in the PARENT run's own first-turn
// prompt, not just in some later pending-context note.
func TestStartTurnWithFlowRefPrependsWaitNoticeToHubsOwnFirstTurn(t *testing.T) {
	var parentPrompt string
	var mu sync.Mutex

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if strings.Contains(req.Prompt, "fix the crash on startup") {
					mu.Lock()
					parentPrompt = req.Prompt
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

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{
		StepID:  "chat-" + parent.RunID,
		Prompt:  "fix the crash on startup",
		SubMode: "bug",
		FlowRef: "flowpilot-core-flow-pack/review-loop",
	}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	waitLoop(t, "hub's own first turn observed by fake adapter", 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return parentPrompt != ""
	})

	mu.Lock()
	got := parentPrompt
	mu.Unlock()
	if !strings.Contains(got, "already been spawned") || !strings.Contains(got, "duplicate that work") {
		t.Fatalf("hub's own first-turn prompt = %.300q, want it to contain the flow-start wait notice", got)
	}
	if !strings.Contains(got, "fix the crash on startup") {
		t.Fatalf("hub's own first-turn prompt = %.300q, want it to still contain the original user prompt", got)
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

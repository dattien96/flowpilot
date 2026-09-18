package runner

import (
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// CP-64 P-3 (Task-366): bug flows wire the reproduce-first node and lock the
// reproduction test file read-only for the coder step. New file — no
// pre-existing test is modified.

const reproduceGateFlag = "FLOWPILOT_ENABLE_REPRODUCE_GATE"

// assertBugFlowReproduceGate pins the CP-64 topology contract on one bug flow:
// the legacy empty-signature TDD node is REPLACED (not accompanied) by
// reproduce_test, wired prev -> reproduce_test -> implement, with the coder's
// continue back-edge and the safety topology intact.
func assertBugFlowReproduceGate(t *testing.T, flowID, prevNodeID string) {
	t.Helper()
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load builtin pack: %v", err)
	}
	var flow *agentpack.FlowDefinition
	for i := range pack.Flows {
		if pack.Flows[i].ID == flowID {
			flow = &pack.Flows[i]
			break
		}
	}
	if flow == nil {
		t.Fatalf("flow %q missing from builtin pack", flowID)
	}
	var repro, implement *agentpack.FlowNode
	for i := range flow.Nodes {
		switch flow.Nodes[i].ID {
		case "reproduce_test":
			repro = &flow.Nodes[i]
		case "implement":
			implement = &flow.Nodes[i]
		case "test_signatures":
			t.Fatalf("flow %q still declares the legacy test_signatures node — CP-64 replaces it, bug fix has no empty-signature mode", flowID)
		}
	}
	if repro == nil || implement == nil {
		t.Fatalf("flow %q missing reproduce_test/implement node", flowID)
	}
	if !IsReproduceBehavior(repro.Behavior) {
		t.Fatalf("reproduce_test behavior = %q, want agent.reproduce", repro.Behavior)
	}
	if repro.Agent != "agents/reproducer.md" || repro.PromptTemplate != "prompts/reproduce-failing-test.md" {
		t.Fatalf("reproduce_test assets drifted: agent=%q prompt=%q", repro.Agent, repro.PromptTemplate)
	}
	if implement.Behavior != "agent.code" {
		t.Fatalf("implement behavior = %q, want agent.code (the coder stays the frozen writer)", implement.Behavior)
	}
	hasEdge := func(from, to, when, kind string) bool {
		for _, e := range flow.Edges {
			if e.From == from && e.To == to && e.When == when && e.Kind == kind {
				return true
			}
		}
		return false
	}
	if !hasEdge(prevNodeID, "reproduce_test", "done", "forward") {
		t.Fatalf("flow %q missing edge %s -> reproduce_test (done/forward)", flowID, prevNodeID)
	}
	if !hasEdge("reproduce_test", "implement", "done", "forward") {
		t.Fatalf("flow %q missing edge reproduce_test -> implement (done/forward)", flowID)
	}
	// The coder's continue back-edge still re-enters at implement (Task-366 T-3);
	// the plan phase's own back-edge (bug-plan-harness) is untouched by CP-64.
	if !hasEdge("validate", "implement", "continue", "back") {
		t.Fatalf("flow %q missing the validate -> implement continue back-edge", flowID)
	}
	if err := agentpack.ValidateFlowSafetyTopology(*flow); err != nil {
		t.Fatalf("flow %q safety topology: %v", flowID, err)
	}
}

// Scenario: bug-harness mang node reproduce_test voi behavior agent.reproduce.
func TestBugHarnessTopologyContainsReproduceGate(t *testing.T) {
	assertBugFlowReproduceGate(t, "bug-harness", "context")
}

// Scenario: bug-plan-harness tuong tu, chuoi freeze -> reproduce_test -> implement.
func TestBugPlanHarnessTopologyContainsReproduceGate(t *testing.T) {
	assertBugFlowReproduceGate(t, "bug-plan-harness", "preflight_contract_freeze")
	// CP-64 constraint: new-feature flows keep the legacy empty-signature node.
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	for _, flow := range pack.Flows {
		switch flow.ID {
		case "task-harness", "rag-harness":
			found := false
			for _, n := range flow.Nodes {
				if n.ID == "test_signatures" {
					found = true
				}
				if IsReproduceBehavior(n.Behavior) {
					t.Fatalf("flow %q must not declare a reproduce node (CP-64 scope is bug flows only)", flow.ID)
				}
			}
			if !found {
				t.Fatalf("flow %q lost its legacy test_signatures node", flow.ID)
			}
		case "vibe-sprint":
			// vibe-sprint's TDD hop is the SD-24 "tdd" node — also untouched.
			found := false
			for _, n := range flow.Nodes {
				if n.ID == "tdd" {
					found = true
				}
				if IsReproduceBehavior(n.Behavior) {
					t.Fatalf("flow %q must not declare a reproduce node (CP-64 scope is bug flows only)", flow.ID)
				}
			}
			if !found {
				t.Fatalf("vibe-sprint lost its legacy tdd node")
			}
		}
	}

}

// newReproduceFixture builds a parent run whose topology is the CP-64 bug chain
// (reproduce_test -> implement) inside a real git workspace.
func newReproduceFixture(t *testing.T, workspace string) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reproduce_test", Behavior: "agent.reproduce", Agent: "agents/reproducer.md", PromptTemplate: "prompts/reproduce-failing-test.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", PromptTemplate: "prompts/implement-complete-tests.md"},
	}
	prs.workspaceCwd = workspace
	svc.mu.Unlock()
	return svc, parent.RunID
}

func newReproduceChildRun(svc *InteractiveService, childID, parentID, workspace, turnStartHead, label string) *interactiveRun {
	rs := &interactiveRun{
		id:               childID,
		parentRunID:      parentID,
		label:            label,
		stepID:           label,
		workspaceCwd:     workspace,
		turnStartGitHead: turnStartHead,
		status:           RunStatusRunning,
		providerKey:      ProviderKeyCodex,
		subs:             map[int64]chan ProviderEvent{},
		idempotency:      map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[childID] = rs
	svc.mu.Unlock()
	return rs
}

// Scenario: sau khi reproduce pass, file test duoc ghi vao frozen record cua
// step implement dang read-only; Coder write/edit trung path do bi silent-deny
// o shared bridge, con doc thi duoc phep.
func TestCoderNodeHasTestFileAsReadOnly(t *testing.T) {
	t.Setenv(reproduceGateFlag, "1")
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	// The reproduce turn wrote its test (and an unrelated doc that must NOT be
	// locked — only test files are evidence).
	svc.recordReproduceTestLock(dir, parentID, []string{"calc/reproduce_test.go", "docs/notes.md"})

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok {
		t.Fatalf("frozen contract for implement missing: ok=%v err=%v", ok, err)
	}
	if got := changecontract.ReadOnlyLockedPaths(rec); len(got) != 1 || got[0] != "calc/reproduce_test.go" {
		t.Fatalf("ReadOnlyPaths = %v, want [calc/reproduce_test.go] (docs must not be locked)", got)
	}
	// AC-4: the coder's DeclaredPaths stay production-only.
	for _, p := range rec.DeclaredPaths {
		if p == "calc/reproduce_test.go" {
			t.Fatal("reproduce test leaked into the coder's DeclaredPaths")
		}
	}

	coder := newReproduceChildRun(svc, "child-coder", parentID, dir, head, "implement")
	cases := []struct {
		name    string
		details ApprovalDetails
		want    string // "", "deny"
	}{
		{"absolute write", ApprovalDetails{Kind: "file", Command: filepath.Join(dir, "calc", "reproduce_test.go"), Reason: "Write"}, "deny"},
		{"relative edit", ApprovalDetails{Kind: "file", Command: "calc/reproduce_test.go", Reason: "Edit"}, "deny"},
		{"read is allowed", ApprovalDetails{Kind: "file", Command: "calc/reproduce_test.go", Reason: "Read"}, ""},
		{"production write allowed", ApprovalDetails{Kind: "file", Command: "calc/calc.go", Reason: "Write"}, ""},
		{"rm the test", ApprovalDetails{Kind: "exec", Command: "rm -f calc/reproduce_test.go", Reason: "Bash"}, "deny"},
		{"sed -i the test", ApprovalDetails{Kind: "exec", Command: "sed -i 's/5/6/' calc/reproduce_test.go", Reason: "Bash"}, "deny"},
		{"read the test", ApprovalDetails{Kind: "exec", Command: "cat calc/reproduce_test.go", Reason: "Bash"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, reason, handled := svc.decideReproduceTestLock(coder, tc.details)
			if tc.want == "deny" {
				if !handled || decision != "deny" || reason != "reproduce_test_locked" {
					t.Fatalf("got decision=%q reason=%q handled=%v, want deny/reproduce_test_locked/true", decision, reason, handled)
				}
				return
			}
			if handled {
				t.Fatalf("unexpected bridge handling: decision=%q reason=%q", decision, reason)
			}
		})
	}

	// A non-coder child is never lock-enforced (the lock binds to the writer).
	reviewer := newReproduceChildRun(svc, "child-reviewer", parentID, dir, head, "reviewer")
	if _, _, handled := svc.decideReproduceTestLock(reviewer, ApprovalDetails{Kind: "file", Command: "calc/reproduce_test.go", Reason: "Write"}); handled {
		t.Fatal("non-writer child must not be lock-enforced")
	}
}

// Scenario: flag tat -> khong lock, khong enforce (CP-64 §8 fallback).
func TestCoderTestFileLockDisabledWithFlagOff(t *testing.T) {
	t.Setenv(reproduceGateFlag, "")
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	svc.recordReproduceTestLock(dir, parentID, []string{"calc/reproduce_test.go"})
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok {
		t.Fatalf("frozen contract missing: ok=%v err=%v", ok, err)
	}
	if len(rec.ReadOnlyPaths) != 0 {
		t.Fatalf("flag off must never write the lock, got %v", rec.ReadOnlyPaths)
	}
	coder := newReproduceChildRun(svc, "child-coder", parentID, dir, head, "implement")
	if _, _, handled := svc.decideReproduceTestLock(coder, ApprovalDetails{Kind: "file", Command: "calc/reproduce_test.go", Reason: "Write"}); handled {
		t.Fatal("flag off must not enforce the lock at the bridge")
	}
}

// Scenario (P-2 T-2, pinned here because agentpack tests cannot import runner):
// agent.reproduce resolves at the runtime registry to the SAME delegate-scoped
// handler as agent.delegate — no new spawn path — and the flag-off degradation
// resolves the legacy prompt/agent pair.
func TestReproduceBehaviorRuntimeBinding(t *testing.T) {
	spec, err := DefaultBehaviorRegistry().Resolve("agent.reproduce")
	if err != nil {
		t.Fatalf("resolve agent.reproduce: %v", err)
	}
	if spec.Scope != BehaviorScopeDelegate {
		t.Fatalf("agent.reproduce scope = %q, want delegate", spec.Scope)
	}
	delegate, err := DefaultBehaviorRegistry().Resolve("agent.delegate")
	if err != nil {
		t.Fatalf("resolve agent.delegate: %v", err)
	}
	if spec.Handler == nil || delegate.Handler == nil {
		t.Fatal("agent.reproduce/agent.delegate handler missing")
	}
	// And it is deliberately NOT a code-writing (frozen writer) behavior.
	if IsCodeWritingBehavior(BehaviorAgentReproduce) {
		t.Fatal("agent.reproduce must not be classified as a code-writing behavior")
	}
	node := agentpack.FlowNode{
		Behavior:       "agent.reproduce",
		Agent:          "agents/reproducer.md",
		PromptTemplate: "prompts/reproduce-failing-test.md",
	}
	if got := resolveReproducePrompt(false, node); got != "prompts/test-signatures.md" {
		t.Fatalf("flag-off prompt = %q, want the legacy empty-signature prompt", got)
	}
	if got := resolveReproduceAgent(false, node); got != "agents/tester.md" {
		t.Fatalf("flag-off agent = %q, want the legacy tester", got)
	}
	if got := resolveReproducePrompt(true, node); got != "prompts/reproduce-failing-test.md" {
		t.Fatalf("flag-on prompt = %q", got)
	}
	if got := resolveReproduceAgent(true, node); got != "agents/reproducer.md" {
		t.Fatalf("flag-on agent = %q", got)
	}
	if !reproduceNodeAsFrozenWriter(node.Behavior) {
		t.Fatal("flag off must bind the reproduce node to the frozen draft (legacy signature mode)")
	}
	t.Setenv(reproduceGateFlag, "1")
	if reproduceNodeAsFrozenWriter(node.Behavior) {
		t.Fatal("flag on must NOT bind the reproduce node to the frozen draft")
	}
}

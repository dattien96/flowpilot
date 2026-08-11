package agentpack

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// CP-55 P-1 (Task-263): coverage for ValidateFlowSafetyTopology,
// ForwardDominates and EveryDonePathIncludesAcceptance. New file — no
// pre-existing test is modified.

func TestValidateFlowSafetyTopologyRejectsWriterAsEntry(t *testing.T) {
	def := FlowDefinition{
		ID: "writer-as-entry",
		Nodes: []FlowNode{
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "entry") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want an entry-node rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsWriterWithoutFreeze(t *testing.T) {
	def := FlowDefinition{
		ID: "writer-without-freeze",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "context.produce"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "freeze") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a missing-freeze rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsPathThatBypassesFreeze(t *testing.T) {
	def := FlowDefinition{
		ID: "bypass-freeze",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "hub.inline"},
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "freeze", Kind: "forward"},
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "entry", To: "writer", Kind: "forward"}, // bypass branch: skips freeze entirely
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "freeze") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a freeze-bypass rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsDonePathWithoutAcceptance(t *testing.T) {
	def := FlowDefinition{
		ID: "no-acceptance",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "writer", Kind: "forward"},
			{From: "writer", To: "done", Kind: "forward"}, // no acceptance step in between
		},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "acceptance") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want an acceptance-boundary rejection", err)
	}
}

func TestValidateFlowSafetyTopologyAcceptsFreezeContextWriterValidation(t *testing.T) {
	def := FlowDefinition{
		ID: "valid-writer-topology",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "context", Behavior: "context.produce"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "context", Kind: "forward"},
			{From: "context", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"validate"},
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want nil for a valid freeze->context->writer->validate topology with validate declared as the acceptance node", err)
	}
}

func TestForwardDominatesHandlesBranches(t *testing.T) {
	// entry -> dominator -> {branchA, branchB} -> target: dominator sits
	// strictly before the branch, so it dominates; a node on only one branch
	// does not, since the other branch reaches target without it.
	def := FlowDefinition{
		ID: "branches",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "hub.inline"},
			{ID: "dominator", Behavior: "contract.freeze"},
			{ID: "branchA", Behavior: "command.validate"},
			{ID: "branchB", Behavior: "command.validate"},
			{ID: "target", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "dominator", Kind: "forward"},
			{From: "dominator", To: "branchA", Kind: "forward"},
			{From: "dominator", To: "branchB", Kind: "forward"},
			{From: "branchA", To: "target", Kind: "forward"},
			{From: "branchB", To: "target", Kind: "forward"},
		},
	}
	if !ForwardDominates(def, "dominator", "target") {
		t.Fatal("expected dominator to dominate target across both branches")
	}
	if ForwardDominates(def, "branchA", "target") {
		t.Fatal("expected branchA to NOT dominate target (branchB bypasses it)")
	}
}

func TestForwardDominatesIgnoresRetryBackEdgesForEntryDominance(t *testing.T) {
	// "entry" has a retry back-edge pointing at it (validate -> entry,
	// kind=back); that must not disqualify "entry" from being the flow's
	// entry node, nor confuse its dominance over "writer".
	def := FlowDefinition{
		ID: "retry-loop",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "entry", When: "continue", Kind: "back"},
			{From: "validate", To: "done", Kind: "forward"},
		},
	}
	if !ForwardDominates(def, "entry", "writer") {
		t.Fatal("expected entry to dominate writer; the validate->entry back edge must not defeat entry-node dominance")
	}
}

// TestEveryDonePathIncludesAcceptanceHandlesReviewLoop exercises the real
// review-loop.yaml topology (preflight_contract_plan -> preflight_contract_freeze
// -> coder -> reviewer_correctness/reviewer_security -> synthesis ->
// done/ask_user, plus a synthesis->coder retry back edge, post CP-55 P-8
// migration). Sets AcceptanceNodes explicitly on the local copy anyway
// (redundant with the real YAML's own acceptance_nodes: [synthesis] since
// P-8, but keeps this test's intent self-contained and independent of the
// YAML's current declared value): both reviewer branches converge on
// synthesis before either can reach done.
func TestEveryDonePathIncludesAcceptanceHandlesReviewLoop(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "review-loop")
	if !ok {
		t.Fatal("review-loop flow missing")
	}
	def.AcceptanceNodes = []string{"synthesis"}
	if !EveryDonePathIncludesAcceptance(def, "coder") {
		t.Fatal("expected review-loop's coder node to satisfy the acceptance-boundary check once synthesis is declared as the acceptance node (both reviewer branches converge on synthesis before done)")
	}
}

// TestEveryDonePathIncludesAcceptanceRejectsReviewLoopWithoutDeclaredAcceptance
// proves the fix for the review finding that the old check ("writer's own
// edges never target done directly") was satisfied by review-loop's real
// topology even though no acceptance boundary is declared anywhere -- coder
// never edges straight to done, but that is not the same claim as "every
// path crosses acceptance". With the real declared-acceptance semantics and
// acceptance_nodes cleared on a local copy (CP-55 P-8 migrated review-loop.yaml
// to declare acceptance_nodes: [synthesis] for real, so this test clears it
// explicitly to isolate the scenario it exists to prove, rather than relying
// on the real YAML happening to declare none), the
// coder->reviewer->synthesis->done path must still correctly fail without a
// declared acceptance node.
func TestEveryDonePathIncludesAcceptanceRejectsReviewLoopWithoutDeclaredAcceptance(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "review-loop")
	if !ok {
		t.Fatal("review-loop flow missing")
	}
	def.AcceptanceNodes = nil
	if EveryDonePathIncludesAcceptance(def, "coder") {
		t.Fatal("expected review-loop's coder node to fail the acceptance-boundary check when no acceptance_nodes are declared")
	}
}

// TestFlowValidationTerminatesOnCycle guards against an infinite loop when a
// flow declares a genuine forward-kind cycle (as opposed to a kind=back retry
// edge, which is already excluded from traversal).
func TestFlowValidationTerminatesOnCycle(t *testing.T) {
	def := FlowDefinition{
		ID: "forward-cycle",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "contract.freeze"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "loopA", Kind: "forward"},
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"}, // forward-declared cycle, not kind=back
			{From: "loopB", To: "writer", Kind: "forward"},
			{From: "writer", To: "done", Kind: "forward"},
		},
	}
	result := make(chan error, 1)
	go func() { result <- ValidateFlowSafetyTopology(def) }()
	select {
	case <-result:
		// Terminated; this test only asserts termination, not the outcome.
	case <-time.After(2 * time.Second):
		t.Fatal("ValidateFlowSafetyTopology did not terminate on a flow containing a forward-declared cycle")
	}
}

// TestValidateFlowSafetyTopologyAcceptsAllBuiltinFlowsUnchanged is the
// compatibility guard for the whole pack: every built-in flow — including
// the three CP-55 P-8 migrated to agent.code/contract.freeze
// (review-loop, rag-harness, context-coding-review-synthesis) — must pass
// ValidateFlowSafetyTopology. Named "Unchanged" from when this test predated
// P-8 (no flow declared agent.code yet); kept as-is since the guarantee it
// proves — every built-in flow in the pack is topologically safe — is the
// same guarantee, now exercised against real writer/acceptance topology
// instead of vacuously (a flow with zero agent.code nodes always passes).
func TestValidateFlowSafetyTopologyAcceptsAllBuiltinFlowsUnchanged(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	if len(pack.Flows) == 0 {
		t.Fatal("expected at least one built-in flow to check")
	}
	for _, def := range pack.Flows {
		if err := ValidateFlowSafetyTopology(def); err != nil {
			t.Errorf("flow %q: ValidateFlowSafetyTopology error = %v, want nil (no built-in flow declares agent.code/contract.freeze yet)", def.ID, err)
		}
	}
}

// TestValidateFlowDefinitionStillAcceptsLegacyAgentDelegateFlow proves the
// new ValidateFlowSafetyTopology call wired into ValidateFlowDefinition is a
// no-op for a flow that only ever uses the pre-existing "coding" alias.
func TestValidateFlowDefinitionStillAcceptsLegacyAgentDelegateFlow(t *testing.T) {
	def := FlowDefinition{
		ID: "legacy-delegate-only",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "coding", Agent: "agents/coder.md"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "done", Kind: "forward"},
		},
	}
	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition error = %v, want nil for a legacy agent.delegate-only flow", err)
	}
}

// --- CP-55 P-1 review fix: forwardDominatesFrom must fail closed ---
//
// The original implementation returned true whenever its wall-BFS simply
// never happened to visit the target -- which is exactly what happens when
// the target is unreachable from any real entry (an absent id, a flow with
// no entry at all, or a writer/freeze pair stranded in a forward cycle
// disconnected from every real entry). These tests pin the fail-closed
// behavior the review required: dominance is never true unless reachability
// from a real entry is proved first.

func TestForwardDominatesRejectsAbsentDominatorID(t *testing.T) {
	def := FlowDefinition{
		ID: "absent-dominator",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "writer", Kind: "forward"},
		},
	}
	if ForwardDominates(def, "does-not-exist", "writer") {
		t.Fatal("expected ForwardDominates to fail closed when the dominator id is not a declared node")
	}
}

func TestForwardDominatesRejectsAbsentTargetID(t *testing.T) {
	def := FlowDefinition{
		ID: "absent-target",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "writer", Kind: "forward"},
		},
	}
	if ForwardDominates(def, "entry", "does-not-exist") {
		t.Fatal("expected ForwardDominates to fail closed when the target id is not a declared node")
	}
}

func TestForwardDominatesRejectsUnreachableWriter(t *testing.T) {
	// A real entry chain (entry -> validate -> done) coexists with a
	// disconnected freeze2/writer2 forward cycle that no edge from the real
	// entry ever reaches. writer2 is a declared node, but unreachable.
	def := FlowDefinition{
		ID: "unreachable-writer",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "hub.inline"},
			{ID: "validate", Behavior: "command.validate"},
			{ID: "freeze2", Behavior: "contract.freeze"},
			{ID: "writer2", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "validate", Kind: "forward"},
			{From: "freeze2", To: "writer2", Kind: "forward"},
			{From: "writer2", To: "freeze2", Kind: "forward"},
		},
	}
	if ForwardDominates(def, "freeze2", "writer2") {
		t.Fatal("expected ForwardDominates to fail closed for a writer unreachable from any real entry")
	}
}

func TestForwardDominatesRejectsNoEntryForwardCycle(t *testing.T) {
	// The entire graph is a forward-declared cycle; every node has an
	// incoming forward edge, so this flow has no entry at all.
	def := FlowDefinition{
		ID: "no-entry-cycle",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "freeze", Kind: "forward"},
		},
	}
	if ForwardDominates(def, "freeze", "writer") {
		t.Fatal("expected ForwardDominates to fail closed when the flow has no entry node at all")
	}
}

func TestForwardDominatesHandlesMultipleEntriesAllFunnelingThroughDominator(t *testing.T) {
	def := FlowDefinition{
		ID: "multi-entry-funnel",
		Nodes: []FlowNode{
			{ID: "e1", Behavior: "hub.inline"},
			{ID: "e2", Behavior: "hub.inline"},
			{ID: "dominator", Behavior: "contract.freeze"},
			{ID: "target", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "e1", To: "dominator", Kind: "forward"},
			{From: "e2", To: "dominator", Kind: "forward"},
			{From: "dominator", To: "target", Kind: "forward"},
		},
	}
	if !ForwardDominates(def, "dominator", "target") {
		t.Fatal("expected dominator to dominate target across multiple entries that all funnel through it")
	}
}

func TestForwardDominatesHandlesMultipleEntriesOneBypassingDominator(t *testing.T) {
	def := FlowDefinition{
		ID: "multi-entry-bypass",
		Nodes: []FlowNode{
			{ID: "e1", Behavior: "hub.inline"},
			{ID: "e2", Behavior: "hub.inline"},
			{ID: "dominator", Behavior: "contract.freeze"},
			{ID: "target", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "e1", To: "dominator", Kind: "forward"},
			{From: "dominator", To: "target", Kind: "forward"},
			{From: "e2", To: "target", Kind: "forward"}, // bypasses dominator entirely
		},
	}
	if ForwardDominates(def, "dominator", "target") {
		t.Fatal("expected dominator to NOT dominate target when a second entry (e2) reaches it directly")
	}
}

// TestValidateFlowSafetyTopologyRejectsNoEntryForwardCycleContainingWriter and
// TestValidateFlowSafetyTopologyRejectsWriterFreezeCycleDisconnectedFromRealEntries
// are direct reproductions of the CRITICAL review finding: a writer/freeze
// forward cycle disconnected from every entry used to pass
// ValidateFlowSafetyTopology outright, because the old dominance walk read
// "never visited the target" as "dominated" regardless of why it was never
// visited.

func TestValidateFlowSafetyTopologyRejectsNoEntryForwardCycleContainingWriter(t *testing.T) {
	def := FlowDefinition{
		ID: "no-entry-cycle-full",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "freeze", Kind: "forward"},
		},
	}
	if err := ValidateFlowSafetyTopology(def); err == nil {
		t.Fatal("expected ValidateFlowSafetyTopology to reject a writer/freeze cycle with no entry anywhere in the flow")
	}
}

func TestValidateFlowSafetyTopologyRejectsWriterFreezeCycleDisconnectedFromRealEntries(t *testing.T) {
	def := FlowDefinition{
		ID: "disconnected-cycle",
		Nodes: []FlowNode{
			{ID: "entry", Behavior: "hub.inline"},
			{ID: "validate", Behavior: "command.validate"},
			{ID: "freeze2", Behavior: "contract.freeze"},
			{ID: "writer2", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "entry", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
			{From: "freeze2", To: "writer2", Kind: "forward"},
			{From: "writer2", To: "freeze2", Kind: "forward"},
		},
	}
	if err := ValidateFlowSafetyTopology(def); err == nil {
		t.Fatal("expected ValidateFlowSafetyTopology to reject a writer/freeze cycle disconnected from every real entry")
	}
}

// --- CP-55 P-1 review fix: declared acceptance_nodes boundary ---
//
// The original everyDonePathIncludesAcceptance only checked the writer's own
// direct edges, so writer -> non-acceptance-intermediate -> done incorrectly
// passed. These tests pin the real, Flow-declared acceptance_nodes
// traversal: at least one must be declared, every id must name a real
// non-writer node, and every forward path from the writer to "done" must
// cross a declared acceptance node -- with "before the writer" not counting
// and mixed branches failing closed.

func TestValidateFlowSafetyTopologyRequiresAcceptanceNodeDeclaration(t *testing.T) {
	def := FlowDefinition{
		ID: "missing-acceptance-declaration",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		// AcceptanceNodes intentionally left empty.
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "acceptance_nodes") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a missing acceptance_nodes declaration rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsUnknownAcceptanceNodeID(t *testing.T) {
	def := FlowDefinition{
		ID: "unknown-acceptance-id",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"nonexistent-node"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "acceptance_nodes entry") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want an unknown acceptance_nodes id rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsAcceptanceNodeThatIsWriter(t *testing.T) {
	def := FlowDefinition{
		ID: "acceptance-is-writer",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"writer"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "writer node") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a writer-as-acceptance rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsPathThatBypassesDeclaredAcceptanceViaIntermediateNode(t *testing.T) {
	// IMPORTANT repro: the old check only looked at the writer's own direct
	// edges, so writer -> intermediate -> done incorrectly passed because
	// "intermediate" isn't literally "done". "audit" is declared as a valid
	// acceptance node, but the writer's actual forward path never reaches
	// it -- this must now fail.
	def := FlowDefinition{
		ID: "bypasses-declared-acceptance",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "intermediate", Behavior: "command.validate"},
			{ID: "audit", Behavior: "artifact.audit_draft"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "intermediate", Kind: "forward"},
			{From: "intermediate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"audit"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a bypassed-acceptance rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsMixedBranchesWhenOneBypassesAcceptance(t *testing.T) {
	def := FlowDefinition{
		ID: "mixed-branch-bypass",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "audit", Behavior: "artifact.audit_draft"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "audit", Kind: "forward"}, // branch 1: crosses acceptance
			{From: "audit", To: "done", Kind: "forward"},
			{From: "writer", To: "done", Kind: "forward"}, // branch 2: bypasses acceptance entirely
		},
		AcceptanceNodes: []string{"audit"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a mixed-branch rejection", err)
	}
}

func TestValidateFlowSafetyTopologyAcceptsWhenEveryBranchCrossesAcceptance(t *testing.T) {
	def := FlowDefinition{
		ID: "every-branch-crosses-acceptance",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "reviewer_a", Behavior: "command.validate"},
			{ID: "reviewer_b", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "reviewer_a", Kind: "forward"},
			{From: "writer", To: "reviewer_b", Kind: "forward"},
			{From: "reviewer_a", To: "done", Kind: "forward"},
			{From: "reviewer_b", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"reviewer_a", "reviewer_b"},
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want nil when every branch crosses a declared acceptance node", err)
	}
}

func TestValidateFlowSafetyTopologyIgnoresAcceptanceNodeCrossedBeforeWriter(t *testing.T) {
	// "acceptance before the writer does not count": review_pre sits between
	// freeze and writer, but the writer's own forward path goes straight to
	// done afterward. review_pre being declared as the acceptance node must
	// not satisfy the writer's downstream check.
	def := FlowDefinition{
		ID: "acceptance-before-writer",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "review_pre", Behavior: "command.validate"},
			{ID: "writer", Behavior: "agent.code"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "review_pre", Kind: "forward"},
			{From: "review_pre", To: "writer", Kind: "forward"},
			{From: "writer", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"review_pre"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want rejection since acceptance was only crossed before the writer ran", err)
	}
}

func TestValidateFlowSafetyTopologyAcceptanceTraversalTerminatesOnForwardCycleAfterWriter(t *testing.T) {
	def := FlowDefinition{
		ID: "acceptance-cycle-after-writer",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "loopA", Kind: "forward"},
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"}, // forward-declared cycle downstream of the writer, never reaching done
		},
		AcceptanceNodes: []string{"loopA"},
	}
	result := make(chan error, 1)
	go func() { result <- ValidateFlowSafetyTopology(def) }()
	select {
	case <-result:
		// Terminated; this test only asserts termination, not the outcome.
	case <-time.After(2 * time.Second):
		t.Fatal("ValidateFlowSafetyTopology did not terminate on a forward-declared cycle downstream of a writer")
	}
}

// --- CP-55 P-1 review pass 2 fix: a writer must have a reachable "done" ---
//
// everyForwardPathFromWriterCrossesAcceptance used to default to "true" once
// its BFS ran out of states to explore, which is also exactly what happens
// when the writer has no successor at all, or when every reachable path
// loops forever without ever reaching "done". These tests pin the pass-2
// requirement: at least one reachable "done" is required per writer, not
// just "no path reaches done without crossing acceptance".

func TestValidateFlowSafetyTopologyRejectsWriterWithNoSuccessor(t *testing.T) {
	def := FlowDefinition{
		ID: "writer-no-successor",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			// writer has no outgoing edges at all.
		},
		AcceptanceNodes: []string{"validate"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want rejection since the writer has no reachable terminal done at all", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsAcceptedPathIntoCycleWithNoDone(t *testing.T) {
	def := FlowDefinition{
		ID: "accepted-cycle-no-done",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "accept", Behavior: "command.validate"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "accept", Kind: "forward"},
			{From: "accept", To: "loopA", Kind: "forward"},
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"}, // forward cycle, never reaches done
		},
		AcceptanceNodes: []string{"accept"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want rejection since no path from the writer ever reaches done, even though the one path that exists crosses acceptance", err)
	}
}

func TestValidateFlowSafetyTopologyAcceptsCycleBranchWithOneAcceptedPathToDone(t *testing.T) {
	def := FlowDefinition{
		ID: "cycle-branch-plus-accepted-done",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
			{ID: "accept", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "loopA", Kind: "forward"}, // branch 1: cycles forever, never reaches done
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"},
			{From: "writer", To: "accept", Kind: "forward"}, // branch 2: crosses acceptance, reaches done
			{From: "accept", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"accept"},
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want nil: one branch cycles forever but a second branch reaches done through the declared acceptance node", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsMixedCycleBranchReachingDoneWithoutAcceptance(t *testing.T) {
	def := FlowDefinition{
		ID: "cycle-branch-plus-unaccepted-done",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
			{ID: "accept", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "loopA", Kind: "forward"}, // branch 1: cycles forever, never reaches done
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"},
			{From: "writer", To: "done", Kind: "forward"}, // branch 2: bypasses acceptance entirely
		},
		AcceptanceNodes: []string{"accept"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "never crosses") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want rejection: one branch reaches done without ever crossing the declared acceptance node", err)
	}
}

// --- CP-55 P-1 review pass 2 fix: blank/duplicate acceptance_nodes ids ---

func TestValidateFlowSafetyTopologyRejectsBlankAcceptanceNodeID(t *testing.T) {
	def := FlowDefinition{
		ID: "blank-acceptance-id",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"   "},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "blank") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a blank acceptance_nodes entry rejection", err)
	}
}

func TestValidateFlowSafetyTopologyRejectsDuplicateAcceptanceNodeID(t *testing.T) {
	def := FlowDefinition{
		ID: "duplicate-acceptance-id",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "validate", Kind: "forward"},
			{From: "validate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"validate", "validate"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a duplicate acceptance_nodes entry rejection", err)
	}
}

// --- CP-55 P-1 review pass 2 fix: root acceptance_nodes YAML key ---
//
// Every prior acceptance_nodes test builds a FlowDefinition Go struct
// directly; none of them prove the root `acceptance_nodes` YAML key actually
// parses through the real LoadFlowFS path. These three do, using an
// in-memory fs.FS (fstest.MapFS) exactly like pack_test.go's own
// LoadFlowFS-level tests.

func TestLoadFlowFSParsesRootAcceptanceNodesKey(t *testing.T) {
	fsys := fstest.MapFS{
		"flows/writer-flow.yaml": &fstest.MapFile{Data: []byte(`
id: writer-flow
acceptance_nodes:
  - validate
nodes:
  - id: freeze
    behavior: contract.freeze
  - id: writer
    behavior: agent.code
  - id: validate
    behavior: command.validate
edges:
  - from: freeze
    to: writer
    kind: forward
  - from: writer
    to: validate
    kind: forward
  - from: validate
    to: done
    kind: forward
`)},
	}
	def, err := LoadFlowFS(fsys, "flows/writer-flow.yaml")
	if err != nil {
		t.Fatalf("LoadFlowFS: %v, want the root acceptance_nodes key to parse and pass validation", err)
	}
	if len(def.AcceptanceNodes) != 1 || def.AcceptanceNodes[0] != "validate" {
		t.Fatalf("AcceptanceNodes = %#v, want [\"validate\"] parsed from the root acceptance_nodes YAML key", def.AcceptanceNodes)
	}
}

func TestLoadFlowFSRejectsAgentCodeWriterWithoutAcceptanceNodesKey(t *testing.T) {
	fsys := fstest.MapFS{
		"flows/writer-no-acceptance.yaml": &fstest.MapFile{Data: []byte(`
id: writer-no-acceptance
nodes:
  - id: freeze
    behavior: contract.freeze
  - id: writer
    behavior: agent.code
  - id: validate
    behavior: command.validate
edges:
  - from: freeze
    to: writer
    kind: forward
  - from: writer
    to: validate
    kind: forward
  - from: validate
    to: done
    kind: forward
`)},
	}
	_, err := LoadFlowFS(fsys, "flows/writer-no-acceptance.yaml")
	if err == nil || !strings.Contains(err.Error(), "acceptance_nodes") {
		t.Fatalf("LoadFlowFS error = %v, want a real-YAML-parsed rejection for an agent.code writer with no root acceptance_nodes key", err)
	}
}

func TestLoadFlowFSLegacyFlowWithoutAcceptanceNodesKeyStillLoads(t *testing.T) {
	fsys := fstest.MapFS{
		"flows/legacy-flow.yaml": &fstest.MapFile{Data: []byte(`
id: legacy-flow
nodes:
  - id: entry
    behavior: coding
    agent: agents/coder.md
edges:
  - from: entry
    to: done
    kind: forward
`)},
	}
	def, err := LoadFlowFS(fsys, "flows/legacy-flow.yaml")
	if err != nil {
		t.Fatalf("LoadFlowFS error = %v, want nil for a legacy flow with no acceptance_nodes key and no agent.code node", err)
	}
	if len(def.AcceptanceNodes) != 0 {
		t.Fatalf("AcceptanceNodes = %#v, want empty when the root acceptance_nodes key is omitted", def.AcceptanceNodes)
	}
}

func findFlowByID(flows []FlowDefinition, id string) (FlowDefinition, bool) {
	for _, f := range flows {
		if f.ID == id {
			return f, true
		}
	}
	return FlowDefinition{}, false
}

// --- CP-55 P-1 pass 4 fix: the no-reachable-done diagnostic must not claim a
// bypassing path exists ---
//
// Before this fix, ValidateFlowSafetyTopology raised the exact same "has a
// forward path to the terminal done state that never crosses..." error for
// both a writer with no reachable "done" at all and a writer whose reachable
// "done" path bypasses acceptance -- falsely implying a bypassing path
// existed even when no path to "done" existed at all. These tests pin the
// corrected, per-case wording: the two existing tests above that already
// cover the no-reachable-done shapes only ever asserted the generic "never
// crosses" substring, which both the old (misleading) and new (truthful)
// wording satisfy, so they never actually pinned which claim the message
// made.

func TestValidateFlowSafetyTopologyNoReachableDoneDiagnosticDoesNotClaimBypassExists(t *testing.T) {
	def := FlowDefinition{
		ID: "writer-no-successor-diagnostic",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "validate", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			// writer has no outgoing edges at all: no reachable "done".
		},
		AcceptanceNodes: []string{"validate"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil {
		t.Fatal("expected an error for a writer with no reachable terminal done at all")
	}
	if strings.Contains(err.Error(), "has a forward path to the terminal") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a no-reachable-done diagnostic that does not falsely claim a forward path to done exists", err)
	}
	if !strings.Contains(err.Error(), "has no forward path that reaches") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want the diagnostic to truthfully state no forward path reaches done at all", err)
	}
}

func TestValidateFlowSafetyTopologyNoReachableDoneViaCycleDiagnosticDoesNotClaimBypassExists(t *testing.T) {
	def := FlowDefinition{
		ID: "accepted-cycle-no-done-diagnostic",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "accept", Behavior: "command.validate"},
			{ID: "loopA", Behavior: "command.validate"},
			{ID: "loopB", Behavior: "command.validate"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "accept", Kind: "forward"},
			{From: "accept", To: "loopA", Kind: "forward"},
			{From: "loopA", To: "loopB", Kind: "forward"},
			{From: "loopB", To: "loopA", Kind: "forward"}, // forward cycle, never reaches done
		},
		AcceptanceNodes: []string{"accept"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil {
		t.Fatal("expected an error for a writer whose only path crosses acceptance but then cycles forever without ever reaching done")
	}
	if strings.Contains(err.Error(), "has a forward path to the terminal") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want a no-reachable-done diagnostic that does not falsely claim a bypassing forward path to done exists", err)
	}
	if !strings.Contains(err.Error(), "has no forward path that reaches") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want the diagnostic to truthfully state no forward path reaches done at all", err)
	}
}

func TestValidateFlowSafetyTopologyBypassDiagnosticStillClaimsPathExists(t *testing.T) {
	// Contrast case for the two tests above: a writer that DOES have a
	// reachable "done" path, but it bypasses the declared acceptance node,
	// must keep the "has a forward path" wording -- that claim is true here.
	def := FlowDefinition{
		ID: "bypass-diagnostic",
		Nodes: []FlowNode{
			{ID: "freeze", Behavior: "contract.freeze"},
			{ID: "writer", Behavior: "agent.code"},
			{ID: "intermediate", Behavior: "command.validate"},
			{ID: "audit", Behavior: "artifact.audit_draft"},
		},
		Edges: []FlowEdge{
			{From: "freeze", To: "writer", Kind: "forward"},
			{From: "writer", To: "intermediate", Kind: "forward"},
			{From: "intermediate", To: "done", Kind: "forward"},
		},
		AcceptanceNodes: []string{"audit"},
	}
	err := ValidateFlowSafetyTopology(def)
	if err == nil {
		t.Fatal("expected an error for a writer whose only reachable done path bypasses the declared acceptance node")
	}
	if !strings.Contains(err.Error(), "has a forward path to the terminal") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want the bypass diagnostic to truthfully state a forward path to done exists", err)
	}
	if strings.Contains(err.Error(), "has no forward path") {
		t.Fatalf("ValidateFlowSafetyTopology error = %v, want the bypass diagnostic not to also claim no path exists", err)
	}
}

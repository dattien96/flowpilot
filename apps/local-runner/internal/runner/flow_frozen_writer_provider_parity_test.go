package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// newFreezeTestServiceForProvider mirrors newFreezeTestService but registers
// the fake adapter under an arbitrary provider key, so the CP-55 preflight
// contract mechanism can be driven identically across Claude/Codex/Grok —
// proving there is no per-provider branch in runContractFreezeNode /
// changecontract.ParsePreflightDraft, per the spec's own "the contract is
// provider-agnostic... parser, validation, freeze, gate and scoring are
// Go-owned and shared" requirement.
func newFreezeTestServiceForProvider(t *testing.T, key ProviderKey) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: key, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coded"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func newFreezeTestRunForProvider(t *testing.T, svc *InteractiveService, key ProviderKey, workspace string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, headSHA string) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: key})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = workspace
	rs.flowStartGitHead = headSHA
	rs.flowStartWorktreeFingerprint = baselineWorktreeFingerprint(workspace)
	rs.providerKey = key
	svc.mu.Unlock()
	return parent.RunID
}

// assertPreflightContractSharedAcrossProviders drives the identical planner
// draft through runContractFreezeNode for the given provider key and returns
// the resulting FrozenContractRecord for cross-provider comparison.
func assertPreflightContractSharedAcrossProviders(t *testing.T, key ProviderKey) changecontract.FrozenContractRecord {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, key)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRunForProvider(t, svc, key, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatalf("provider %q: expected runContractFreezeNode to return true (handled)", key)
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(runID, "coder")
	if err != nil || !ok {
		t.Fatalf("provider %q: expected a frozen record, ok=%v err=%v", key, ok, err)
	}
	waitLoop(t, "coder spawned", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
	return rec
}

// TestClaudePreflightContractUsesSharedParserAndGate pins that Claude drives
// the identical shared preflight-contract mechanism as every other provider —
// grep across the Claude adapter/mapper files finds zero preflight/contract-
// freeze branching (CP-55 P-8 research), so this is a pinning test proving
// the lack of a per-provider path, not a comparison between two branches
// (there is only one).
func TestClaudePreflightContractUsesSharedParserAndGate(t *testing.T) {
	rec := assertPreflightContractSharedAcrossProviders(t, ProviderKeyClaude)
	if rec.FeatureKey != "calc-core" || rec.Intent != "fix rounding" {
		t.Fatalf("unexpected frozen record for Claude: %+v", rec)
	}
}

// TestCodexPreflightContractUsesSharedParserAndGate is the Codex counterpart.
func TestCodexPreflightContractUsesSharedParserAndGate(t *testing.T) {
	rec := assertPreflightContractSharedAcrossProviders(t, ProviderKeyCodex)
	if rec.FeatureKey != "calc-core" || rec.Intent != "fix rounding" {
		t.Fatalf("unexpected frozen record for Codex: %+v", rec)
	}
}

// TestGrokPreflightContractUsesSharedParserAndGate is the Grok counterpart.
func TestGrokPreflightContractUsesSharedParserAndGate(t *testing.T) {
	rec := assertPreflightContractSharedAcrossProviders(t, ProviderKeyGrok)
	if rec.FeatureKey != "calc-core" || rec.Intent != "fix rounding" {
		t.Fatalf("unexpected frozen record for Grok: %+v", rec)
	}
}

// TestProviderAdaptersReceiveEquivalentFrozenContractPayload proves the three
// providers' frozen records are byte-equivalent on every field that
// shouldn't vary by provider (everything except RunID, which is naturally
// distinct per test service instance) — the strongest form of "no
// provider-specific branch may weaken preflight or scope enforcement" this
// suite can assert without a live multi-provider integration harness.
func TestProviderAdaptersReceiveEquivalentFrozenContractPayload(t *testing.T) {
	claude := assertPreflightContractSharedAcrossProviders(t, ProviderKeyClaude)
	codex := assertPreflightContractSharedAcrossProviders(t, ProviderKeyCodex)
	grok := assertPreflightContractSharedAcrossProviders(t, ProviderKeyGrok)

	for _, pair := range [][2]changecontract.FrozenContractRecord{{claude, codex}, {codex, grok}, {claude, grok}} {
		a, b := pair[0], pair[1]
		if a.FeatureKey != b.FeatureKey || a.Intent != b.Intent || len(a.DeclaredPaths) != len(b.DeclaredPaths) {
			t.Fatalf("provider-dependent frozen contract payload: %+v vs %+v", a, b)
		}
		for i := range a.DeclaredPaths {
			if a.DeclaredPaths[i] != b.DeclaredPaths[i] {
				t.Fatalf("DeclaredPaths differ by provider: %v vs %v", a.DeclaredPaths, b.DeclaredPaths)
			}
		}
	}
}

// TestNormalChatRemainsUsableWithoutFlowGuarantees pins that a plain Normal
// chat coding request — no Flow topology, no frozen contract, no
// acceptance_nodes — is completely unaffected by every CP-55 mechanism this
// phase wires into the three built-in Flows: it is neither blocked for
// lacking a frozen contract nor required to stage a pending Canonical Head
// update. This is CP-55's own repeatedly-stated backward-compatibility
// invariant (P-1 through P-7 each pin it separately for their own
// mechanism); P-8 pins it once more against the now-migrated pack to prove
// migrating the built-in Flows did not tighten Normal chat's own contract.
func TestNormalChatRemainsUsableWithoutFlowGuarantees(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	relaxGateRulesForPendingCanonicalTests(t, dir)
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	rs := newP4ChildRun(svc, "root-turn", "", dir, head)
	rs.id = parent.RunID
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	rs.workspaceCwd = dir
	// Deliberately no activeFlowNodes/flowEngineDriven — a plain Normal chat
	// turn with no Flow topology at all, exactly like every pre-CP-55 turn.
	svc.mu.Unlock()

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	msg := "[Change Contract]\nfeature: calc-core\nintent: fix rounding\nfiles: src/calc.go\n\ndone"
	if svc.runFlowGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: msg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("Normal chat must not be blocked for lacking a frozen contract")
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || !found {
		t.Fatalf("Normal chat must still write Canonical Head immediately, found=%v err=%v", found, err)
	}
	pstore, pstoreErr := changecontract.NewPendingCanonicalStore(dir)
	if pstoreErr != nil {
		t.Fatal(pstoreErr)
	}
	if _, ok, _ := pstore.GetPending(parent.RunID, "calc-core"); ok {
		t.Fatal("Normal chat must never stage a pending canonical update")
	}
}

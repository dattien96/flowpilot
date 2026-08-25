package runner

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// run-147126: rag-harness audit tier-3 fired r-contract ("code changed without
// a declared Change Contract (used an inferred one)") even though the preflight
// contract was frozen in the FrozenStore. The coder is skipSave (CP-55 P-8), so
// the legacy Store is empty and audit read ContractDeclared=false. Audit must
// honor an active FrozenStore record as a declared contract. New file — no
// pre-existing test modified.

func newAuditFrozenFixture(t *testing.T, declareFrozen bool) (svc *InteractiveService, parentID string, workspace string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ = newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID = parent.RunID
	workspace = t.TempDir()
	initGitRepoForAuditFixture(t, workspace)

	// CA note must exist so r-ca does NOT fire (only r-contract is under test).
	p4WriteFile(t, workspace, "change-audit/CA-635-run147126-audit-honors-frozen-contract.md", "# CA-635\n")
	// Feature registry so the audit draft is "ready" (feature key verified).
	writeFeatureKeys(t, workspace, []string{"calc-format"})
	// Code change (declared path).
	p4WriteFile(t, workspace, "format.go", "package gatesandbox\n")

	svc.mu.Lock()
	rs := svc.runs[parentID]
	edges, nodes := flowFixtureEdgesNodes()
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = workspace
	rs.flowStartGitHead = headOfRepo(t, workspace)
	// Audit requires positive validation verification (status=passed).
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	// Plan context carries the feature key (frozen contract overrides below).
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "calc-format", FeatureConfidence: ConfidenceVerified}
	svc.mu.Unlock()

	if declareFrozen {
		frozenStore, err := changecontract.NewFrozenStore(workspace)
		if err != nil {
			t.Fatalf("NewFrozenStore: %v", err)
		}
		draft := changecontract.PreflightContractDraft{FeatureKey: "calc-format", Intent: "Add ClampChecked", DeclaredPaths: []string{"format.go", "format_test.go"}}
		baseline := baselineWorktreeFingerprint(workspace)
		rec, err := changecontract.FreezeContract(workspace, parentID, "preflight_contract_plan", "implement", draft, rs.flowStartGitHead, baseline, "", 1, time.Now().UTC())
		if err != nil {
			t.Fatalf("FreezeContract: %v", err)
		}
		if err := frozenStore.SaveFrozen(rec); err != nil {
			t.Fatalf("SaveFrozen: %v", err)
		}
	}
	return svc, parentID, workspace
}

func headOfRepo(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRun147126_AuditHonorsFrozenContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
		model    string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4-mini"},
		{"claude", ProviderKeyClaude, "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, parentID, workspace := newAuditFrozenFixture(t, true)

			// Legacy Store must be EMPTY (skipSave behavior — frozen writer
			// never writes contracts.ndjson).
			if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
				if c, ok := store.GetLatestForRun(parentID); ok {
					t.Fatalf("legacy store must be empty for a frozen writer, got %+v", c)
				}
			}

			_, nodes := flowFixtureEdgesNodes()
			auditNode, ok := findFlowNode(nodes, "audit")
			if !ok {
				t.Fatal("fixture must declare an audit node")
			}
			if !svc.frozenContractDeclaredForRun(workspace, parentID) {
				t.Fatal("frozen contract must be detected as declared for the run")
			}
			handled := svc.runAuditNode(context.Background(), parentID, nil, nodes, auditNode, "implemented ClampChecked")
			if !handled {
				t.Fatal("runAuditNode must handle (not escalate) when an active frozen contract exists")
			}
			snap := svc.agentGraphSnapshot(parentID)
			if got := snap.LoopState.Status; got != "done" && got != "running" {
				t.Fatalf("loop status = %q, want done (or running after settle); GateReason=%q", got, snap.LoopState.GateReason)
			}
		})
	}
}

func TestRun147126_AuditStillBlocksWithoutAnyContract(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{"grok"}, {"codex"}, {"claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, parentID, _ := newAuditFrozenFixture(t, false) // no frozen, no legacy
			_, nodes := flowFixtureEdgesNodes()
			auditNode, _ := findFlowNode(nodes, "audit")
			handled := svc.runAuditNode(context.Background(), parentID, nil, nodes, auditNode, "implemented ClampChecked")
			if !handled {
				t.Fatal("runAuditNode must handle (escalate) when NO contract is declared")
			}
			snap := svc.agentGraphSnapshot(parentID)
			if snap.LoopState.Status != "blocked" || snap.LoopState.BlockReason != "escalate" {
				t.Fatalf("expected loop blocked (escalate), got %q/%q", snap.LoopState.Status, snap.LoopState.BlockReason)
			}
			if !strings.Contains(snap.LoopState.GateReason, "declared Change Contract") {
				t.Fatalf("expected gate reason to mention declared contract, got %q", snap.LoopState.GateReason)
			}
		})
	}
}

func TestRun147126_AuditStillHonorsLegacyDeclaredContract(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{"grok"}, {"codex"}, {"claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// BUG-288 #2 legacy path must not regress: legacy Store with
			// ConfidenceDeclared still passes without a frozen record.
			svc, parentID, workspace := newAuditFrozenFixture(t, false)
			store, err := changecontract.NewStore(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(changecontract.Contract{RunID: parentID, StepID: "implement", FeatureKey: "calc-format", Confidence: changecontract.ConfidenceDeclared}); err != nil {
				t.Fatal(err)
			}
			_, nodes := flowFixtureEdgesNodes()
			auditNode, _ := findFlowNode(nodes, "audit")
			handled := svc.runAuditNode(context.Background(), parentID, nil, nodes, auditNode, "implemented ClampChecked")
			if !handled {
				t.Fatal("legacy declared contract must keep audit from escalating (BUG-288 #2)")
			}
			if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "done" && got != "running" {
				t.Fatalf("loop status = %q, want done (or running)", got)
			}
		})
	}
}

func TestRun147126_SupersededFrozenStillBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{"grok"}, {"codex"}, {"claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, parentID, workspace := newAuditFrozenFixture(t, true)
			// Supersede the frozen contract — it must no longer count as declared.
			frozenStore, err := changecontract.NewFrozenStore(workspace)
			if err != nil {
				t.Fatal(err)
			}
			recs, err := frozenStore.ListForRun(parentID)
			if err != nil || len(recs) != 1 {
				t.Fatalf("expected 1 frozen record, got %d (err=%v)", len(recs), err)
			}
			if err := frozenStore.AppendStatus(recs[0].ContractID, changecontract.ContractStatusSuperseded, "test", time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			_, nodes := flowFixtureEdgesNodes()
			auditNode, _ := findFlowNode(nodes, "audit")
			handled := svc.runAuditNode(context.Background(), parentID, nil, nodes, auditNode, "implemented ClampChecked")
			if !handled {
				t.Fatal("superseded frozen contract must NOT count as declared (still escalate)")
			}
			if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "blocked" {
				t.Fatalf("loop status = %q, want blocked", got)
			}
		})
	}
}

func TestRun147126_ContinueAfterAuditParkDoesNotFlapSynthesis(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
		model    string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4-mini"},
		{"claude", ProviderKeyClaude, "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, parentID, _ := newAuditFrozenFixture(t, true)
			_, nodes := flowFixtureEdgesNodes()
			// Park on audit with the frozen contract present.
			svc.mu.Lock()
			p := svc.runs[parentID]
			p.lastEscalatedInlineNodeID = "audit"
			p.activeFlowAcceptanceNodes = []string{"audit"}
			p.autoOrchestrate = true
			p.modelName = tc.model
			p.providerKey = tc.provider
			svc.mu.Unlock()
			svc.reseedFlowStepRuntime(parentID, nodes)
			svc.setFlowStepStatus(context.Background(), parentID, "audit", StepStatusWaitingUserApr)

			if _, err := svc.resumeFlowWithFeedback(parentID, "continue"); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", tc.name, err)
			}
			// The inline dispatch runs in a goroutine; wait for it to settle
			// (either re-advance to done, or re-park — the loop must NOT stay
			// blocked with the SAME reason after the frozen contract is honored).
			waitLoop(t, "audit re-entry settles", 5*time.Second, func() bool {
				st := svc.agentOrchestrator.loopStateFor(parentID)
				return st.Status != "blocked"
			})
		})
	}
}

var _ = exec.Command

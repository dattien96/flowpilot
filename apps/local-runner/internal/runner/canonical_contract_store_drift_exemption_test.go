package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// Live task-harness (test_signatures turn): the frozen-scope drift gate parked
// the flow with "flow scope drift: wrote outside the frozen contract's declared
// paths: .flowpilot/canonical/calc-core.json, .flowpilot/contracts/contracts.ndjson".
// Both are runner-owned stores the gate's own diff observes — SaveHead writes the
// Canonical Head on gate passes, commitChangeContract writes the legacy Store
// for non-frozen writers — never the current frozen writer's drift. They need
// the same exact-path exemption FrozenStore's own two files already have
// (CA-427 Finding 2 discipline: no .flowpilot/**-wide exemption —
// flow-rules.json and forged siblings must still drift).
//
// New file; no pre-existing test is modified. Provider-agnostic: the drift
// comparison and the gate filter take no providerKey, so the matrix guards
// future provider drift.

// newCanonicalDriftFixture mirrors newP4CodeWriterFixture but plumbs the
// provider key through, so the repro runs identically on Claude/Codex/Grok.
func newCanonicalDriftFixture(t *testing.T, pk ProviderKey, workspace string) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestServiceForProvider(t, pk)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"}}
	prs.workspaceCwd = workspace
	svc.mu.Unlock()
	return svc, parent.RunID
}

func TestCanonicalAndContractsStoreWritesDoNotTriggerScopeDrift(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newCanonicalDriftFixture(t, pk, dir)
			freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
			rs := newP4ChildRun(svc, "child-canonical-"+string(pk), parentID, dir, head)
			rs.providerKey = pk

			// Writer changed its declared file; the runner wrote its own two
			// store files into the same workspace the gate then diffs.
			p4WriteFile(t, dir, "src/calc.go", "package calc\n")
			p4WriteFile(t, dir, ".flowpilot/canonical/calc-core.json", `{"feature_key":"calc-core"}`+"\n")
			p4WriteFile(t, dir, ".flowpilot/contracts/contracts.ndjson", `{"run_id":"x"}`+"\n")

			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{
					"src/calc.go",
					".flowpilot/canonical/calc-core.json",
					".flowpilot/contracts/contracts.ndjson",
				},
			}, 0)

			if blocked {
				t.Fatalf("%s: canonical head + legacy contracts store writes must not cause scope drift block", pk)
			}
		})
	}
}

func TestCanonicalStoreWritesPlusTrueDriftStillBlocks(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newCanonicalDriftFixture(t, pk, dir)
			freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
			rs := newP4ChildRun(svc, "child-canonical-drift-"+string(pk), parentID, dir, head)
			rs.providerKey = pk

			p4WriteFile(t, dir, "src/calc.go", "package calc\n")
			p4WriteFile(t, dir, ".flowpilot/canonical/calc-core.json", `{"feature_key":"calc-core"}`+"\n")
			p4WriteFile(t, dir, "src/extra.go", "package calc\n")

			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{
					"src/calc.go",
					".flowpilot/canonical/calc-core.json",
					"src/extra.go",
				},
			}, 0)

			if !blocked {
				t.Fatalf("%s: expected scope drift block when unexpected extra.go is written", pk)
			}
			snap := svc.agentGraphSnapshot(parentID)
			if !strings.Contains(snap.LoopState.GateReason, "src/extra.go") {
				t.Fatalf("%s: expected gate reason to mention src/extra.go, got %q", pk, snap.LoopState.GateReason)
			}
		})
	}
}

func TestFlowRulesRewriteStillBlocksDespiteStoreExemption(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newCanonicalDriftFixture(t, ProviderKeyCodex, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-rules", parentID, dir, head)

	// The new exemption must not become a .flowpilot/**-wide hole (CA-427):
	// rewriting the gate's own rules still drifts.
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[{"id":"r-contract","enabled":false}]`)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{".flowpilot/settings/flow-rules.json"},
	}, 0) {
		t.Fatal("a writer rewriting its own gate rules file must still count as scope drift")
	}
}

func TestIsCanonicalHeadStorePath(t *testing.T) {
	for _, p := range []string{
		".flowpilot/canonical/calc-core.json",
		".flowpilot/canonical/other-feature.json",
		`.flowpilot\canonical\calc-core.json`,
		"./.flowpilot/canonical/calc-core.json",
	} {
		if !changecontract.IsCanonicalHeadStorePath(p) {
			t.Errorf("%q should be recognized as canonical-head bookkeeping", p)
		}
	}
	for _, p := range []string{
		"",
		".flowpilot/canonical/",
		".flowpilot/canonical/nested/calc-core.json",
		".flowpilot/canonical/notes.md",
		".flowpilot/canonical",
		".flowpilot/settings/flow-rules.json",
		".flowpilot/contracts/contracts.ndjson",
		".flowpilot/canonical-pending/pending_canonical.ndjson",
		"src/calc.go",
	} {
		if changecontract.IsCanonicalHeadStorePath(p) {
			t.Errorf("%q must NOT be exempt as canonical-head bookkeeping", p)
		}
	}
}

func TestIsLegacyContractsStorePath(t *testing.T) {
	for _, p := range []string{
		".flowpilot/contracts/contracts.ndjson",
		`.flowpilot\contracts\contracts.ndjson`,
		"./.flowpilot/contracts/contracts.ndjson",
	} {
		if !changecontract.IsLegacyContractsStorePath(p) {
			t.Errorf("%q should be recognized as legacy contracts-store bookkeeping", p)
		}
	}
	for _, p := range []string{
		"",
		".flowpilot/contracts/frozen_contracts.ndjson",
		".flowpilot/contracts/forged.ndjson",
		".flowpilot/contracts/",
		".flowpilot/settings/flow-rules.json",
		".flowpilot/canonical/calc-core.json",
		"src/calc.go",
	} {
		if changecontract.IsLegacyContractsStorePath(p) {
			t.Errorf("%q must NOT be exempt as legacy contracts-store bookkeeping", p)
		}
	}
}

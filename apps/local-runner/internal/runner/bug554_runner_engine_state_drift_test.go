package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// BUG-554 (run-38799, PrivateVault CP-02): the coder's FrozenContractScopeDrift
// gate parked the flow on files the RUNNER itself writes into the workspace
// every turn — engine-init.json (runEngineInit), tooling.json
// (tooling.CheckAll), ledger-needs-update (changeledger sentinel),
// catalog/features.ndjson (contextsync catalog) and ledger/.cursor
// (changeledger scan cursor). None were in the exemption lists, and
// /agent-loop/amend rejects .flowpilot/* as "not a concrete code target" — so
// the park had no product unblock path. Same self-park class as CA-649 and
// CA-1089 for engine files added after those lists were written. Additive
// only — CA-427 Finding 2 stays closed (.flowpilot/settings/flow-rules.json
// still drifts).

func TestGateDriftIgnoresRunnerEngineStateFiles(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	bookkeeping := []string{
		".flowpilot/engine-init.json",
		".flowpilot/tooling.json",
		".flowpilot/ledger-needs-update",
		".flowpilot/catalog/features.ndjson",
		".flowpilot/ledger/.cursor",
	}
	for _, p := range bookkeeping {
		p4WriteFile(t, dir, p, "{}\n")
	}

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: bookkeeping}, 0) {
		t.Fatal("runner-written engine-state files must NOT block the coder gate")
	}
}

func TestGateDriftStillBlocksCodeAlongsideEngineState(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, ".flowpilot/engine-init.json", "{}\n")
	p4WriteFile(t, dir, ".flowpilot/tooling.json", "{}\n")
	p4WriteFile(t, dir, "src/surprise.go", "package x\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/engine-init.json", ".flowpilot/tooling.json", "src/surprise.go"}}, 0) {
		t.Fatal("a real out-of-scope code file must still block alongside engine-state churn")
	}
}

// TestBUG554EngineStateListsCoverLiveFlaggedPaths pins the exact paths
// observed drifting in run-38799 to the lists that now own them.
func TestBUG554EngineStateListsCoverLiveFlaggedPaths(t *testing.T) {
	ledger := map[string]bool{}
	for _, p := range changecontract.RunnerLedgerBookkeepingPaths() {
		ledger[p] = true
	}
	config := map[string]bool{}
	for _, p := range changecontract.RunnerOwnedConfigPaths() {
		config[p] = true
	}
	for _, p := range []string{
		".flowpilot/ledger/.cursor",
		".flowpilot/ledger-needs-update",
		".flowpilot/catalog/features.ndjson",
	} {
		if !ledger[p] {
			t.Fatalf("RunnerLedgerBookkeepingPaths missing %q", p)
		}
	}
	for _, p := range []string{
		".flowpilot/engine-init.json",
		".flowpilot/tooling.json",
	} {
		if !config[p] {
			t.Fatalf("RunnerOwnedConfigPaths missing %q", p)
		}
	}
	// CA-427 Finding 2 must stay closed.
	if changecontract.IsRunnerLedgerBookkeepingPath(".flowpilot/settings/flow-rules.json") ||
		changecontract.IsRunnerOwnedConfigPath(".flowpilot/settings/flow-rules.json") {
		t.Fatal("flow-rules.json must remain subject to drift enforcement")
	}
}

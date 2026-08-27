package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// CA-649 (run-161570): the coder's FrozenContractScopeDrift gate parked itself
// on `.flowpilot/gate-metrics.ndjson` — the runner's OWN gate observability
// file, appended by recordGate*Metric on EVERY gate pass. It is therefore
// always dirty at the exact moment the coder's gate diff is computed and, not
// being in the frozen contract's declared paths, always drifted. The exemption
// list (RunnerLedgerBookkeepingPaths) gains exactly this one file; CA-427's
// security hole stays closed (.flowpilot/settings/flow-rules.json still
// drifts). Additive only — legacy suites untouched.

func TestGateDriftIgnoresGateMetricsNdjson(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	// The gate itself appended metrics during this turn (run-161570 repro).
	p4WriteFile(t, dir, ".flowpilot/gate-metrics.ndjson",
		`{"ts":"2026-08-27T01:00:00Z","run_id":"run-x","action":"block"}`+"\n")

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/gate-metrics.ndjson"}}, 0) {
		t.Fatal("writing only the runner's own gate-metrics file must NOT block the gate")
	}
}

func TestGateDriftStillBlocksCodeAlongsideGateMetrics(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, ".flowpilot/gate-metrics.ndjson", "x\n")
	p4WriteFile(t, dir, "src/surprise.go", "package x\n") // real out-of-scope code

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/surprise.go"}}, 0) {
		t.Fatal("a real out-of-scope code file must still block even with gate-metrics churn")
	}
}

// TestCA649GateRulesFileStillDrifts guards CA-427 Finding 2:
// exempting one exact metrics file must not open the .flowpilot/** hole.
func TestCA649GateRulesFileStillDrifts(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-3", parentID, dir, head)

	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[{"id":"r-contract","enabled":false}]`)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/settings/flow-rules.json"}}, 0) {
		t.Fatal("a writer rewriting its own gate rules file must still count as scope drift")
	}
}

func TestRunnerLedgerBookkeepingPathsIncludesGateMetrics(t *testing.T) {
	for _, p := range []string{
		".flowpilot/gate-metrics.ndjson",
		".flowpilot/manifest.json",
		".flowpilot/ledger/chat_summary.ndjson",
		".flowpilot/ledger/feature_history.ndjson",
	} {
		if !changecontract.IsRunnerLedgerBookkeepingPath(p) {
			t.Errorf("IsRunnerLedgerBookkeepingPath(%q) = false, want true", p)
		}
	}
	// And it must NOT widen to gate config (CA-427 hole).
	if changecontract.IsRunnerLedgerBookkeepingPath(".flowpilot/settings/flow-rules.json") {
		t.Fatal("flow-rules.json must never be runner-ledger bookkeeping")
	}
}
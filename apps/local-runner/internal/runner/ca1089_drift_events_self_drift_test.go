package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// CA-1089 (live run-3362): the frozen-contract scope gate parked sprint
// writers on `.flowpilot/workflow_drift_events.json` — the runner's OWN drift
// event log, appended by appendDriftEvent (gate_hook.go driftEventsFileName)
// on every evaluated turn. It is always dirty inside the writer's diff window
// and never in the frozen contract's declared paths, so every writer
// false-positive blocked and escalated — in the vibe lane the escalate routed
// to the owner-debate mount which wedged the run (CA-1088). Same self-park
// class as CA-649's gate-metrics.ndjson exemption: exact-path exemption in
// RunnerOwnedConfigPaths, CA-427's .flowpilot/** hole stays closed.

func TestCA1089_DriftEventsFileDoesNotBlockScopeGate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	// The gate's own drift recorder appended during this turn (run-3362 repro:
	// "flow scope drift: wrote outside the frozen contract's declared paths:
	// .flowpilot/workflow_drift_events.json").
	p4WriteFile(t, dir, ".flowpilot/workflow_drift_events.json",
		`{"run_id":"run-3362","step_id":"tdd","drift_score":20}`+"\n")

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/workflow_drift_events.json"}}, 0) {
		t.Fatal("the runner's own drift-events file must NOT block the frozen-scope gate")
	}
}

// Same file plus a real out-of-scope write — the exemption must not launder
// real drift.
func TestCA1089_RealDriftStillBlocksAlongsideDriftEvents(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, ".flowpilot/workflow_drift_events.json", "{}\n")
	p4WriteFile(t, dir, "src/surprise.go", "package x\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1",
		finalizeInput{FinalMessage: "done", ChangedFiles: []string{
			".flowpilot/workflow_drift_events.json", "src/surprise.go"}}, 0) {
		t.Fatal("a real out-of-scope file must still block even with drift-events churn")
	}
}

// Exact-path contract: the exemption covers the drift-events file and nothing
// else under .flowpilot/.
func TestCA1089_DriftEventsIsRunnerOwnedConfigPath(t *testing.T) {
	if !changecontract.IsRunnerOwnedConfigPath(".flowpilot/workflow_drift_events.json") {
		t.Fatal("workflow_drift_events.json must be a runner-owned config path")
	}
	for _, p := range []string{
		".flowpilot/settings/flow-rules.json",
		".flowpilot/workflow_drift_events_2.json",
		"sub/.flowpilot/workflow_drift_events.json",
	} {
		if changecontract.IsRunnerOwnedConfigPath(p) {
			t.Fatalf("IsRunnerOwnedConfigPath(%q) = true, want false", p)
		}
	}
}

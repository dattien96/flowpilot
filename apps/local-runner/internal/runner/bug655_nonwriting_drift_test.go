package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/flowgate"
)

// BUG-655 (live run-523131): the drift detector scored zero_delta_progress
// on synthesis/debate/review hub turns that write no files BY DESIGN —
// routing nodes whose contractual output is a verdict/decision, not a file
// delta. 419 spurious events in one run flooded pause_for_human and spawned
// owner-debate traffic for "stalls" that were just verdict turns.
//
// Fix: node-class awareness — a declared non-coding workloadClass
// (scan|high_reasoning) marks the summary NonWritingNode and exempts the
// zero_delta_progress signal; coding-class legs still fire it.

func bug655Run(label string) (*InteractiveService, *interactiveRun) {
	s := &InteractiveService{}
	parent := &interactiveRun{id: "run-655-hub", workspaceCwd: t655Dir}
	parent.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", WorkloadClass: agentpack.WorkloadCoding},
		{ID: "coder", Behavior: "agent.code", WorkloadClass: agentpack.WorkloadCoding},
		{ID: "reviewer", Behavior: "agent.delegate", WorkloadClass: agentpack.WorkloadHighReasoning},
		{ID: "scan_leg", Behavior: "agent.delegate", WorkloadClass: agentpack.WorkloadScan},
		{ID: "unclassified", Behavior: "agent.delegate"},
	}
	leg := &interactiveRun{
		id:           "run-655-" + label,
		parentRunID:  parent.id,
		label:        label,
		workspaceCwd: t655Dir,
	}
	leg.events = append(leg.events, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last: &TokenUsageBreakdown{TotalTokens: 50000},
		},
	})
	if s.runs == nil {
		s.runs = map[string]*interactiveRun{}
	}
	s.runs[parent.id] = parent
	s.runs[leg.id] = leg
	return s, leg
}

const t655Dir = "/tmp/bug655"

func TestBug655_SynthesisTurnNoDrift(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	// reviewer leg: agent.delegate high_reasoning — emits a verdict, writes
	// nothing. Zero file delta is the contractual shape, not a stall.
	s, leg := bug655Run("reviewer")
	tr := &flowgate.TurnResult{
		FinalMessage: "Verdict: changes_requested — ac-2 lacks null-input coverage.",
	}
	s.recordDriftTelemetry(leg, "turn-1", tr)

	st := driftStateFor(s, leg.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 0 {
		t.Fatalf("non-writing hub turn must score 0 drift, got score=%d signals in history=%+v", st.lastScore, st.history)
	}
	if len(st.history) != 1 || !st.history[0].NonWritingNode {
		t.Fatalf("summary must carry NonWritingNode for the reviewer leg, got %+v", st.history)
	}
}

func TestBug655_ScanTurnNoDrift(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	s, leg := bug655Run("scan_leg")
	s.recordDriftTelemetry(leg, "turn-1", &flowgate.TurnResult{FinalMessage: "Scan complete: 3 hotspots."})

	st := driftStateFor(s, leg.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 0 {
		t.Fatalf("scan-class turn must score 0 drift, got %d", st.lastScore)
	}
}

func TestBug655_WriterTurnZeroDeltaStillDetected(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	s, leg := bug655Run("coder")
	s.recordDriftTelemetry(leg, "turn-1", &flowgate.TurnResult{
		FinalMessage: "I explored the repo and will implement next.",
	})

	st := driftStateFor(s, leg.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 20 {
		t.Fatalf("coding-class writer with zero delta must still fire zero_delta_progress (+20), got score=%d", st.lastScore)
	}
	if st.history[0].NonWritingNode {
		t.Fatal("coding-class leg must not be marked NonWritingNode")
	}
}

func TestBug655_UndeclaredClassStillDetected(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	// A user-authored delegate node with no workloadClass keeps the guard —
	// unknown class must not silently disable detection (fail-closed).
	s, leg := bug655Run("unclassified")
	s.recordDriftTelemetry(leg, "turn-1", &flowgate.TurnResult{FinalMessage: "thinking out loud"})

	st := driftStateFor(s, leg.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 20 {
		t.Fatalf("undeclared workload class must keep zero_delta detection, got score=%d", st.lastScore)
	}
}

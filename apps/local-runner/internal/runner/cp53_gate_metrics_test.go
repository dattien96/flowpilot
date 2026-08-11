package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

func TestAppendGateMetricWritesNDJSON(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	ev := gateMetricEvent{
		Action: "block",
		RunID:  "run-1",
		StepID: "step-a",
		RuleIDs: []string{"r-reg"},
	}
	if err := appendGateMetric(dot, ev); err != nil {
		t.Fatalf("appendGateMetric: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dot, gateMetricsFileName))
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	var got gateMetricEvent
	if err := json.Unmarshal(bytesTrimLine(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v raw=%q", err, raw)
	}
	if got.Action != "block" || got.RunID != "run-1" || len(got.RuleIDs) != 1 || got.RuleIDs[0] != "r-reg" {
		t.Fatalf("unexpected event: %#v", got)
	}
	if got.TS == "" {
		t.Fatal("expected timestamp to be set")
	}
}

func TestGateMetricRuleIDsDedupes(t *testing.T) {
	violations := []flowgate.Violation{
		{Rule: flowgate.Rule{ID: "r-reg"}},
		{Rule: flowgate.Rule{ID: "r-tests"}},
		{Rule: flowgate.Rule{ID: "r-reg"}},
	}
	ids := gateMetricRuleIDs(violations)
	if len(ids) != 2 {
		t.Fatalf("len(ids)=%d want 2: %v", len(ids), ids)
	}
}

func TestRecordGateAcceptedMetricTurnsToAccept(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	rs := &interactiveRun{
		id:               "run-acc",
		lastTurnStepID:   "step-1",
		repromptAttempts: 2,
	}
	svc := &InteractiveService{}
	svc.recordGateAcceptedMetric(dot, rs, "turn-9", "enforce")
	raw, err := os.ReadFile(filepath.Join(dot, gateMetricsFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got gateMetricEvent
	if err := json.Unmarshal(bytesTrimLine(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != "accepted" {
		t.Fatalf("action=%q want accepted", got.Action)
	}
	if got.TurnsToAccept != 3 {
		t.Fatalf("turns_to_accept=%d want 3 (repromptAttempts+1)", got.TurnsToAccept)
	}
}

func TestRecordGateOverrideMetric(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	rs := &interactiveRun{id: "run-ov", lastTurnStepID: "step-2"}
	svc := &InteractiveService{}
	svc.recordGateOverrideMetric(dot, rs, []string{"TestFoo", "TestBar"})
	raw, err := os.ReadFile(filepath.Join(dot, gateMetricsFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got gateMetricEvent
	if err := json.Unmarshal(bytesTrimLine(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Action != "override" || len(got.OverrideTests) != 2 {
		t.Fatalf("unexpected override event: %#v", got)
	}
}

func bytesTrimLine(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

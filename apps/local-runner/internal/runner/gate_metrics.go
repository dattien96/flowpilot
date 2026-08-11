package runner

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"flowpilot-runner/internal/flowgate"
)

const gateMetricsFileName = "gate-metrics.ndjson"

// gateMetricEvent is one append-only observability record (Task-272 / CP-53 P-6).
// Emitted for inspection only — does not affect gate decisions.
type gateMetricEvent struct {
	TS              string   `json:"ts"`
	RunID           string   `json:"run_id,omitempty"`
	StepID          string   `json:"step_id,omitempty"`
	TurnID          string   `json:"turn_id,omitempty"`
	GateMode        string   `json:"gate_mode,omitempty"`
	Action          string   `json:"action"`
	RuleIDs         []string `json:"rule_ids,omitempty"`
	RepromptAttempt int      `json:"reprompt_attempt,omitempty"`
	TurnsToAccept   int      `json:"turns_to_accept,omitempty"`
	OverrideTests   []string `json:"override_tests,omitempty"`
	TokenIn         int64    `json:"token_in,omitempty"`
	TokenOut        int64    `json:"token_out,omitempty"`
}

func gateMetricRuleIDs(violations []flowgate.Violation) []string {
	if len(violations) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(violations))
	out := make([]string, 0, len(violations))
	for _, v := range violations {
		id := v.Rule.ID
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func appendGateMetric(dotFP string, ev gateMetricEvent) error {
	if dotFP == "" {
		return nil
	}
	if ev.TS == "" {
		ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dotFP, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dotFP, gateMetricsFileName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return err
	}
	log.Printf("[gate-metric] action=%s run=%q step=%q mode=%q rules=%v reprompt=%d turns_to_accept=%d override_tests=%d",
		ev.Action, ev.RunID, ev.StepID, ev.GateMode, ev.RuleIDs, ev.RepromptAttempt, ev.TurnsToAccept, len(ev.OverrideTests))
	return nil
}

func gateMetricTokenUsage(_ *interactiveRun) (in, out int64) {
	// v1: token fields optional; cost-per-accepted-change falls back to turns_to_accept.
	return 0, 0
}

func (s *InteractiveService) recordGateEnforceMetric(dotFP string, rs *interactiveRun, turnID, gateMode string, result flowgate.EnforceResult) {
	if rs == nil {
		return
	}
	action := result.Action
	if action == "" {
		action = "pass"
	}
	ev := gateMetricEvent{
		RunID:    rs.id,
		StepID:   rs.lastTurnStepID,
		TurnID:   turnID,
		GateMode: gateMode,
		Action:   action,
		RuleIDs:  gateMetricRuleIDs(result.Violations),
	}
	if err := appendGateMetric(dotFP, ev); err != nil {
		log.Printf("[gate-metric] append failed run=%q: %v", rs.id, err)
	}
}

func (s *InteractiveService) recordGateEscalateMetric(dotFP string, rs *interactiveRun, turnID, gateMode, reason string) {
	if rs == nil {
		return
	}
	ev := gateMetricEvent{
		RunID:    rs.id,
		StepID:   rs.lastTurnStepID,
		TurnID:   turnID,
		GateMode: gateMode,
		Action:   "escalate",
		RuleIDs:  []string{reason},
	}
	if err := appendGateMetric(dotFP, ev); err != nil {
		log.Printf("[gate-metric] append escalate failed run=%q: %v", rs.id, err)
	}
}

func (s *InteractiveService) recordGateOverrideMetric(dotFP string, rs *interactiveRun, testNames []string) {
	if rs == nil {
		return
	}
	ev := gateMetricEvent{
		RunID:         rs.id,
		StepID:        rs.lastTurnStepID,
		Action:        "override",
		OverrideTests: append([]string(nil), testNames...),
	}
	if err := appendGateMetric(dotFP, ev); err != nil {
		log.Printf("[gate-metric] append override failed run=%q: %v", rs.id, err)
	}
}

func (s *InteractiveService) recordGateAcceptedMetric(dotFP string, rs *interactiveRun, turnID, gateMode string) {
	if rs == nil {
		return
	}
	in, out := gateMetricTokenUsage(rs)
	turns := rs.repromptAttempts + 1
	if turns < 1 {
		turns = 1
	}
	ev := gateMetricEvent{
		RunID:         rs.id,
		StepID:        rs.lastTurnStepID,
		TurnID:        turnID,
		GateMode:      gateMode,
		Action:        "accepted",
		TurnsToAccept: turns,
		TokenIn:       in,
		TokenOut:      out,
	}
	if err := appendGateMetric(dotFP, ev); err != nil {
		log.Printf("[gate-metric] append accepted failed run=%q: %v", rs.id, err)
	}
}

package runner

import (
	"flowpilot-runner/internal/flowgate"
)

// gateBlindBlocksTurn implements CP-53 P-1: when the oracle is blind and the turn
// touched production code, fail closed in enforce (block) or surface warn otherwise.
// Returns true when the turn must not proceed (block path).
func (s *InteractiveService) gateBlindBlocksTurn(
	runID, turnID string,
	epoch int64,
	rs *interactiveRun,
	dotFP string,
	baseline *flowgate.Baseline,
	oracle flowgate.OracleResult,
	diff []flowgate.ChangedFile,
) bool {
	reason := flowgate.ClassifyGateBlind(baseline, oracle, dotFP)
	if reason == "" {
		return false
	}
	if !flowgate.HasCodeChanges(diff) {
		return false
	}
	gateMode := loadGateMode(dotFP)
	msg := flowgate.GateBlindMessage(reason)
	status := "warn"
	if gateMode == "enforce" {
		status = "block"
	}
	runIDForMetric := runID
	stepID := ""
	if rs != nil {
		runIDForMetric = rs.id
		stepID = rs.lastTurnStepID
	}
	if !s.gateEpochStillValid(runID, epoch) {
		return gateMode == "enforce"
	}
	s.mu.Lock()
	if rs2 := s.runs[runID]; rs2 != nil && rs2.gateEpoch == epoch {
		s.emitLocked(rs2, ProviderEvent{
			Type:           EventFlowGateViolation,
			ProviderTurnID: turnID,
			Error:          msg,
			Status:         status,
		})
	}
	s.mu.Unlock()
	_ = appendGateMetric(dotFP, gateMetricEvent{
		RunID:    runIDForMetric,
		StepID:   stepID,
		TurnID:   turnID,
		GateMode: gateMode,
		Action:   "gate_blind",
		RuleIDs:  []string{string(reason)},
	})
	if gateMode == "enforce" {
		return true
	}
	return false
}

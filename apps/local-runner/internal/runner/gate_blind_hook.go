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
	// CP-67 contract-first (live run-13173): a red baseline is the DESIGNED
	// intermediate of signature-locked flows — the scaffold turn produces the
	// RED suite by contract, and the tree stays dirty until the coder lands
	// the implementation, so a red/stale baseline can never refresh green and
	// every scaffold+coder turn would block on red_at_capture forever.
	// Downgrade to warn: r-scaffold-red / r-signature-lock own correctness for
	// this pipeline, and validate still runs the suite directly.
	contractFirstRed := reason == flowgate.GateBlindRedAtCapture && rs != nil
	if contractFirstRed {
		if rec, ok := s.frozenContractForRun(rs.workspaceCwd, rs.parentRunID); !ok || len(rec.DeclaredPaths) == 0 {
			contractFirstRed = false
		}
	}
	gateMode := loadGateMode(dotFP)
	msg := flowgate.GateBlindMessage(reason)
	status := "warn"
	if gateMode == "enforce" && !contractFirstRed {
		status = "block"
	}
	runIDForMetric := runID
	stepID := ""
	if rs != nil {
		runIDForMetric = rs.id
		stepID = rs.lastTurnStepID
	}
	if !s.gateEpochStillValid(runID, epoch) {
		return gateMode == "enforce" && !contractFirstRed
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
	if gateMode == "enforce" && !contractFirstRed {
		return true
	}
	return false
}

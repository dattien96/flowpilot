package runner

import (
	"context"
	"log"

	"flowpilot-runner/internal/flowgate"
)

// supersedeRedBaselineOnOracleGreen replaces a red-at-capture baseline with a
// verified-green one when the oracle's own suite run this turn passed
// (BUG-632, live run-174243). A sprint's working tree is dirty by
// construction from scaffold until the audit commit, so
// RefreshBaselineIfStale's "dirty ⇒ keep prior truth" rule lets a TDD-stub
// red baseline survive the whole sprint and warn on every code turn.
//
// Recapture via CaptureBaselineContext (the FULL unscoped command): the
// oracle run may have been package-scoped by scopeTestCommand, so its Passed
// list can be a subset and is not safe to write back. Fail-closed: env
// errors, suite failures, ctx cancellation, or a still-red recapture leave
// the prior baseline untouched — red_at_capture stays honest.
func (s *InteractiveService) supersedeRedBaselineOnOracleGreen(ctx context.Context, repoDir, dotFP string, baseline *flowgate.Baseline, oracle flowgate.OracleResult) *flowgate.Baseline {
	if baseline == nil || baseline.SuitePassed || oracle.EnvError != "" || !oracle.SuitePassed {
		return baseline
	}
	fresh, err := flowgate.CaptureBaselineContext(ctx, repoDir, dotFP)
	if err != nil || fresh == nil || !fresh.SuitePassed {
		return baseline
	}
	log.Printf("[gate] baseline superseded: red-at-capture replaced by verified green suite (head=%s)", fresh.HeadSHA)
	return fresh
}

// gateBlindBlocksTurn implements CP-53 P-1: when the oracle is blind and the turn
// touched production code, fail closed in enforce (block) or surface warn otherwise.
// Returns true when the turn must not proceed (block path).
func (s *InteractiveService) gateBlindBlocksTurn(
	ctx context.Context,
	runID, turnID string,
	epoch int64,
	rs *interactiveRun,
	repoDir string,
	dotFP string,
	baseline *flowgate.Baseline,
	oracle flowgate.OracleResult,
	diff []flowgate.ChangedFile,
) bool {
	baseline = s.supersedeRedBaselineOnOracleGreen(ctx, repoDir, dotFP, baseline, oracle)
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
	// Task-455: chat-surface runs gate as warn regardless of the persisted
	// gate_mode — a blind block never interrupts a plain chat turn.
	gateMode := effectiveGateMode(dotFP, rs)
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

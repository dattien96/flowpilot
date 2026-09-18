package runner

import (
	"context"
	"strings"

	"flowpilot-runner/internal/lsp"
)

// lspChecker abstracts live compiler diagnostics for the post-turn gate
// path. *lsp.ServerSet implements it; tests inject fakes.
type lspChecker interface {
	CheckFiles(ctx context.Context, workspaceRoot string, relPaths []string) string
}

// lspDiagnosticsForTurn returns formatted compiler errors for the files
// written this turn, or "" when clean, unsupported or unavailable. A nil
// checker (no LSP configured) yields "" with zero behavior change, so
// every pre-CP-63 test path is untouched.
func (s *InteractiveService) lspDiagnosticsForTurn(ctx context.Context, workspaceCwd string, changedFiles []string) string {
	if s == nil || strings.TrimSpace(workspaceCwd) == "" || len(changedFiles) == 0 {
		return ""
	}
	checker := s.lspCheckerOrDefault()
	if checker == nil {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return checker.CheckFiles(ctx, workspaceCwd, changedFiles)
}

func (s *InteractiveService) lspCheckerOrDefault() lspChecker {
	if s != nil && s.lspChecker != nil {
		return s.lspChecker
	}
	return lsp.DefaultSet()
}

// blockTurnForLSPDiagnostics parks the turn on compiler errors: it arms the
// standard gate-reprompt fields so the settle machinery opens a new turn
// carrying the diagnostic message, then reports blocked. Epoch handling
// mirrors the gate's own fail-closed contract.
func (s *InteractiveService) blockTurnForLSPDiagnostics(runID string, epoch int64, turnID, diagMsg string) bool {
	if !s.gateEpochStillValid(runID, epoch) {
		return true
	}
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || rs.gateEpoch != epoch {
		s.mu.Unlock()
		return true
	}
	// The settle chain requires a non-empty step to start the reprompt turn;
	// fall back to the run's own step when the last turn left none.
	stepID := strings.TrimSpace(rs.lastTurnStepID)
	if stepID == "" {
		stepID = strings.TrimSpace(rs.stepID)
	}
	rs.pendingGateRepromptPrompt = diagMsg
	rs.pendingGateRepromptStepID = stepID
	rs.pendingGateRepromptGen++
	s.emitLocked(rs, ProviderEvent{
		Type:           EventFlowGateViolation,
		ProviderTurnID: turnID,
		Error:          diagMsg,
		Status:         "reprompt",
	})
	s.mu.Unlock()
	return true
}

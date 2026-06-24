package runner

import (
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/flowgate"
)

const maxFlowGateReprompts = 2

// runFlowGate is the post-turn enforcement hook (CP-35 P-4/P-5). It is called
// after turnInFlight=false and before finalizer.Finalize on a clean completion.
//
// It observes the git diff and test outcomes, evaluates the flow rules, and
// either allows the turn to finalise, reprompts the AI with the missing
// requirement, or blocks the step from completing.
//
// All errors inside this function are non-fatal: if the gate cannot observe or
// evaluate it returns false (degraded = safe, turn completes normally).
func (s *InteractiveService) runFlowGate(
	_ context.Context, rs *interactiveRun, turnID string, fin finalizeInput,
) (block bool) {
	cwd := rs.workspaceCwd
	if cwd == "" {
		return false
	}
	dotFP := filepath.Join(cwd, ".flowpilot")

	// 1. Observe git diff — include commits made during the turn (CP-35).
	// ObserveGitDiffSince merges committed changes since turn-start with any
	// remaining uncommitted changes, so files the AI committed are not missed.
	s.mu.Lock()
	baseSHA := rs.turnStartGitHead
	s.mu.Unlock()
	diff, _ := flowgate.ObserveGitDiffSince(cwd, baseSHA)
	log.Printf("[gate] cwd=%q baseSHA=%q diffLen=%d diff=%+v", cwd, baseSHA, len(diff), diff)

	// 2. Load test baseline — non-fatal. Baseline must already exist (captured at
	// turn-start by ensureBaseline before the AI ran). If still nil the oracle
	// is skipped for this turn rather than capturing a broken-state snapshot.
	baseline, _ := flowgate.LoadBaseline(dotFP)

	// 3. Run regression oracle against the diff.
	oracle := flowgate.RunOracle(cwd, baseline, diff)

	// 4. Build TurnResult for the evaluator.
	var failedTests []string
	if oracle.HasRegression {
		failedTests = oracle.Regressed
	}
	tr := flowgate.TurnResult{
		RunID:        rs.id,
		StepID:       rs.stepID,
		FinalMessage: fin.FinalMessage,
		GitDiff:      diff,
		Tests: flowgate.TestOutcome{
			Ran:    baseline != nil,
			Failed: failedTests,
		},
	}

	// 5. Load rules; fall back to defaults when flow-rules.json is absent.
	rules := flowgate.DefaultRules()
	if loaded, err := flowgate.LoadRules(filepath.Join(dotFP, "settings")); err == nil {
		rules = loaded
	}

	// 6. Evaluate rule set.
	violations := flowgate.Evaluate(tr, rules)
	hasCA := flowgate.HasChangeAuditNote(diff)
	hasCode := flowgate.HasCodeChanges(diff)
	log.Printf("[gate] violations=%d gateMode=%q hasCode=%v hasCA=%v", len(violations), loadGateMode(dotFP), hasCode, hasCA)

	// 7. Surface oracle-detected tampering as an additional warn violation so the
	// desktop can display it even when no rule explicitly covers it.
	if oracle.HasTampering {
		violations = append(violations, flowgate.Violation{
			Rule: flowgate.Rule{
				ID:      "r-tamper",
				Scope:   "step",
				Trigger: "oracle_tamper",
				Action:  "warn",
				Enabled: true,
			},
			Detail: "pre-existing test file modified: " + strings.Join(oracle.Tampered, ", "),
		})
	}

	if len(violations) == 0 {
		return false
	}

	// 8. Enforce — read gate_mode from .flowpilot/settings/gate-config.json; default warn.
	result := flowgate.Enforce(violations, loadGateMode(dotFP))

	// 9. Emit the violation event so the desktop can surface it inline. Status carries
	// the resolved action ("block" | "reprompt" | "warn") so the desktop can render a
	// blocking gate (e.g. failed tests) as a modal rather than only an inline card.
	s.mu.Lock()
	s.emitLocked(rs, ProviderEvent{
		Type:           EventFlowGateViolation,
		ProviderTurnID: turnID,
		Error:          result.Message,
		Status:         result.Action,
	})
	s.mu.Unlock()

	switch result.Action {
	case "block":
		return true

	case "reprompt":
		s.mu.Lock()
		attempts := rs.repromptAttempts
		rs.repromptAttempts++
		stepID := rs.lastTurnStepID
		s.mu.Unlock()
		log.Printf("[gate] reprompt attempt=%d stepID=%q", attempts, stepID)
		if attempts < maxFlowGateReprompts {
			go func(runID, stepID, prompt string) {
				_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
			}(rs.id, stepID, result.Message)
		}
		// Whether reprompting or max reached, suppress the current completion.
		return true
	}

	// "warn" or "approve": log only, let the turn complete normally.
	return false
}

// loadGateMode delegates to the shared readGateMode helper (engine_gate_config.go).
func loadGateMode(dotFP string) string {
	return readGateMode(dotFP)
}

// ensureBaseline captures a test baseline for cwd if one does not already exist.
// It must be called BEFORE the AI turn starts so the snapshot reflects a known-good
// state. If baseline capture fails it is silently ignored (non-fatal). (CP-35)
func (s *InteractiveService) ensureBaseline(cwd string) {
	if cwd == "" {
		return
	}
	dotFP := filepath.Join(cwd, ".flowpilot")
	if bl, _ := flowgate.LoadBaseline(dotFP); bl != nil {
		return // already exists
	}
	_, _ = flowgate.CaptureBaseline(cwd, dotFP)
}

// captureGitHead returns the current HEAD SHA in repoDir, trimmed of whitespace.
func captureGitHead(repoDir string) (string, error) {
	out, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

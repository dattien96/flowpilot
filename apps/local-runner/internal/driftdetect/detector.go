// Package driftdetect implements CP-23 Phase 2 (Task-335): the Drift Detector
// ("Bộ phát hiện lệch hướng") — a deterministic, 0-token telemetry aggregator
// that scores every completed AI turn for wrong-way behaviour and resolves the
// CP-23 Correction Ladder.
//
// Design contracts (CP-23 D-2/D-3, Task-335 T-1..T-4):
//   - Telemetry aggregator only: it never re-inspects diffs or re-runs tests.
//     The caller feeds in the signals the flow gates already computed
//     (r-scope's ScopeOutOfScopePaths, the test oracle's failed test names) —
//     100% reuse of existing gate signals, zero duplicated diff logic.
//   - 0 LLM / 0 token heuristics (CP-23 D-3 tier 1): pure deterministic Go.
//   - Soft ladder only: inject system note (30-59) → narrow context (60-79) →
//     pause for human (80+). Never auto-rollback (CP-23 hard constraint).
//
// Signal weights (CP-23 T-2):
//
//	out_of_scope_edit       +35   (reused r-scope signal, ScopeOutOfScopePaths)
//	repeated_test_failure   +30   (same test name failed in >=2 consecutive turns)
//	apology_loop            +25   (apology/filler phrases in >=2 consecutive turns)
//	zero_delta_progress     +20   (>2000 tokens with zero file delta, CP-23 R-2 guard)
//
// Decay model (Task-335 §9 DOD item 8 — deterministic, documented here):
// the score is a CARRIED RUNNING TOTAL recovered by replaying the history
// oldest→newest (carriedDriftScore). Each past turn's signal score is added to
// the carried total (capped at 100); a past turn that raises NO negative
// signal and produced a concrete file delta counts as a successful correction
// and HALVES the carried score (integer floor — a "half-life"). The current
// turn follows the same rule: negative signals add on top of the carried
// total; a clean turn with a file delta halves it; a clean turn without a
// delta leaves it unchanged (no correction evidence). The score therefore
// decays progressively as the AI corrects instead of accumulating forever.
//
// Provider-agnostic by construction: pure Go, deterministic, 0 LLM —
// identical behaviour for Claude / Codex / Grok / any provider.
package driftdetect

import "strings"

// CorrectionAction is one step of the CP-23 Correction Ladder
// (Task-335 §11 Code Guide).
type CorrectionAction string

const (
	// ActionNone: drift score 0-29 — healthy turn, no correction needed.
	ActionNone CorrectionAction = "none"
	// ActionInjectSystemNote: 30-59 — soft warning note injected into the
	// next prompt asking the AI to change strategy.
	ActionInjectSystemNote CorrectionAction = "inject_system_note"
	// ActionNarrowContext: 60-79 — the next turn runs with a tightened
	// (halved-cap) Budget Packer context.
	ActionNarrowContext CorrectionAction = "narrow_context"
	// ActionPauseForHuman: 80+ — pause and wait for human intervention.
	// NEVER auto-rollback (CP-23 hard constraint).
	ActionPauseForHuman CorrectionAction = "pause_for_human"
)

// Triggered signal names (Task-335 §11 / CP-23 T-2).
const (
	SignalOutOfScopeEdit      = "out_of_scope_edit"
	SignalRepeatedTestFailure = "repeated_test_failure"
	SignalApologyLoop         = "apology_loop"
	SignalZeroDeltaProgress   = "zero_delta_progress"
)

const (
	// maxDriftScore caps the drift score (Task-335 §11: "0 - 100").
	maxDriftScore = 100

	// weightOutOfScopeEdit: r-scope contract violation (CP-23 T-2).
	weightOutOfScopeEdit = 35
	// weightRepeatedTestFailure: same test failed >=2 consecutive turns.
	weightRepeatedTestFailure = 30
	// weightApologyLoop: apology/filler phrase loop across >=2 turns.
	weightApologyLoop = 25
	// weightZeroDeltaProgress: heavy token burn with zero file delta.
	weightZeroDeltaProgress = 20
)

// DriftEvent is one drift evaluation of a completed turn (Task-335 §11 Code
// Guide); persisted as JSON for the Task-336 (Phase 3) handoff. StepID is
// required by the §9 DOD but absent from the §11 sketch — TurnSummary carries
// no step identity, so the caller fills it from its own TurnResult.
type DriftEvent struct {
	RunID            string           `json:"run_id"`
	StepID           string           `json:"step_id,omitempty"`
	TurnID           string           `json:"turn_id"`
	DriftScore       int              `json:"drift_score"` // 0 - 100
	TriggeredSignals []string         `json:"triggered_signals"`
	CorrectionAction CorrectionAction `json:"correction_action"`
	SystemNotePrompt string           `json:"system_note_prompt,omitempty"`
}

// TurnSummary contains the per-turn summary the caller derives from its own
// gate observations (Task-335 §11 Code Guide). TestResults holds FAILING test
// names only; ScopeOutOfScopePaths is the r-scope signal reused verbatim.
type TurnSummary struct {
	TurnID               string   `json:"turn_id"`
	FinalMessage         string   `json:"final_message"`
	TokensConsumed       int      `json:"tokens_consumed"`
	FilesChanged         []string `json:"files_changed"`
	TestResults          []string `json:"test_results"` // Danh sách tên test fail
	ScopeOutOfScopePaths []string `json:"scope_out_of_scope_paths"`
}

// EvaluateTurnDrift evaluates the drift level after a completed turn
// (Task-335 §11 Code Guide). history holds the PREVIOUS turns of the same run
// (oldest first, excluding current); current is the turn just completed.
//
// The returned DriftEvent carries the current TurnID; the caller (which knows
// the run context) populates RunID. When the resolved action is
// ActionInjectSystemNote, SystemNotePrompt is pre-filled with
// GenerateSystemNote(event) so the persisted event is self-contained for the
// Phase-3 handoff.
func EvaluateTurnDrift(history []TurnSummary, current TurnSummary) DriftEvent {
	carried := carriedDriftScore(history)
	turnScore, signals := signalScore(history, current)

	score := carried
	switch {
	case turnScore > 0:
		score = clampScore(carried + turnScore)
	case len(current.FilesChanged) > 0:
		// Decay (DOD item 8): the AI corrected successfully this turn — no
		// negative signal AND a concrete file delta — so the carried score
		// halves instead of sticking forever.
		score = carried / 2
	}
	// A clean turn with no file delta leaves the carried score unchanged:
	// chat-only turns are not correction evidence.

	action := resolveCorrectionAction(score)
	event := DriftEvent{
		TurnID:           current.TurnID,
		DriftScore:       score,
		TriggeredSignals: signals,
		CorrectionAction: action,
	}
	if action == ActionInjectSystemNote {
		event.SystemNotePrompt = GenerateSystemNote(event)
	}
	return event
}

// signalScore computes the current turn's own drift contribution from the
// four Task-335 heuristics, given the preceding turns of the same run.
func signalScore(history []TurnSummary, current TurnSummary) (int, []string) {
	score := 0
	signals := make([]string, 0, 4)

	// 1. Reused r-scope signal (CP-23 D-2 — no new diff checking here).
	if len(current.ScopeOutOfScopePaths) > 0 {
		score += weightOutOfScopeEdit
		signals = append(signals, SignalOutOfScopeEdit)
	}

	// 2. Same test failing repeatedly across consecutive turns.
	if checkRepeatedTestFailures(history, current) {
		score += weightRepeatedTestFailure
		signals = append(signals, SignalRepeatedTestFailure)
	}

	// 3. Apology / filler loop — requires >=2 consecutive turns (Task-335
	// Open Question: a single apology is normal conversation).
	if checkApologyPatterns(history, current) {
		score += weightApologyLoop
		signals = append(signals, SignalApologyLoop)
	}

	// 4. Heavy token burn with zero file delta (CP-23 R-2 guard).
	if checkZeroDeltaProgress(current) {
		score += weightZeroDeltaProgress
		signals = append(signals, SignalZeroDeltaProgress)
	}
	return score, signals
}

// carriedDriftScore replays the history deterministically to recover the
// running drift score entering the current turn (see the package decay-model
// comment). Empty history (first turn) carries 0.
func carriedDriftScore(history []TurnSummary) int {
	score := 0
	for i := range history {
		turnScore, _ := signalScore(history[:i], history[i])
		switch {
		case turnScore > 0:
			score = clampScore(score + turnScore)
		case len(history[i].FilesChanged) > 0:
			// Successful correction mid-history: half-life decay.
			score /= 2
		}
	}
	return score
}

// clampScore caps the drift score at maxDriftScore (Task-335: 0-100).
func clampScore(score int) int {
	if score > maxDriftScore {
		return maxDriftScore
	}
	if score < 0 {
		return 0
	}
	return score
}

// hasSignal reports whether the signal name is present in the list.
func hasSignal(signals []string, name string) bool {
	for _, s := range signals {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

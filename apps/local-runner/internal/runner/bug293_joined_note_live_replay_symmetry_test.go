package runner

// BUG-293 / CP-51 A1: a review-loop hub reinvoke embeds the "[flow-engine joined
// result note] ... you must call submit_review_outcome" synthesis prompt as the
// turn's prompt. Live, startTurn streamed that prompt on turn_started, so the
// desktop rendered it as a user bubble. On replay it was dropped
// (userFacingTranscriptEvents -> isInternalTranscriptEvent, keyed on
// isSystemPrompt), so after a server restart + reopen the bubble disappeared:
// exactly one message was "lost".
//
// The fix redacts the DISPLAY prompt on the live turn_started for any system
// prompt (liveTurnStartedDisplayPrompt), so live and replay hide the same set.
// These tests pin that invariant. They are additive and touch no existing test.

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// realJoinedResultNote builds a faithful hub synthesis prompt the same way
// maybeAutoReinvokeHubWithNote does: the consolidated cohort note followed by the
// auto-reinvoke instruction tail. Using the production composers keeps the test
// honest if the note wording ever changes.
func realJoinedResultNote() string {
	cohortNote := buildCohortNote("run-parent", "flow-auto-hub-round-0", []cohortEntry{
		{Label: "reviewer_correctness", Provider: "claude", Status: "completed", FinalMessage: "Verified independently: go test passes. Verdict: APPROVE."},
		{Label: "reviewer_security", Provider: "claude", Status: "completed", FinalMessage: "Review verdict: APPROVED. Bug is resolved."},
	}, 0)
	return cohortNote + "\n\n---\n\n" + autoReinvokePromptText()
}

// TestBug293LiveTurnStartedHidesJoinedResultNote is the direct regression: the
// live display prompt for the joined result note is empty, so no bubble renders.
func TestBug293LiveTurnStartedHidesJoinedResultNote(t *testing.T) {
	note := realJoinedResultNote()
	if !strings.Contains(note, "[flow-engine joined result note]") {
		t.Fatalf("fixture is not a joined result note: %q", note)
	}
	if got := liveTurnStartedDisplayPrompt(note); got != "" {
		t.Fatalf("live display prompt for joined result note = %q, want \"\" (hidden)", got)
	}
}

// TestBug293LiveTurnStartedKeepsRealUserPrompt guards against over-redaction: a
// genuine user prompt must still render live.
func TestBug293LiveTurnStartedKeepsRealUserPrompt(t *testing.T) {
	const userPrompt = "fix bug 1+1 != 2"
	if got := liveTurnStartedDisplayPrompt(userPrompt); got != userPrompt {
		t.Fatalf("live display prompt for user prompt = %q, want %q (preserved)", got, userPrompt)
	}
}

// TestBug293LiveAndReplayHideTheSamePrompts is the core invariant: for every
// prompt kind, the LIVE path (liveTurnStartedDisplayPrompt -> "") and the REPLAY
// path (isInternalTranscriptEvent on a turn_started) must agree on whether the
// bubble is hidden. This is what keeps a "lost message" from recurring for any
// internal prompt class, not just the joined note.
func TestBug293LiveAndReplayHideTheSamePrompts(t *testing.T) {
	handoff := handoffPromptPrefix + "\nsource feature: calc-core\n\nprior conversation ..."
	gateReprompt := flowgate.GateRepromptPrefix + " audit note for step coder."

	cases := []struct {
		name       string
		prompt     string
		wantHidden bool
	}{
		{"joined_result_note", realJoinedResultNote(), true},
		{"flow_engine_results_ready", autoReinvokePromptText(), true},
		{"gate_reprompt", gateReprompt, true},
		{"handoff_envelope", handoff, true},
		{"real_user_prompt", "fix bug 1+1 != 2", false},
		{"continuation_prompt", "continue", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			liveHidden := liveTurnStartedDisplayPrompt(tc.prompt) == ""
			replayHidden := isInternalTranscriptEvent(ProviderEvent{Type: EventTurnStarted, Prompt: tc.prompt})
			if liveHidden != replayHidden {
				t.Fatalf("live/replay disagree for %s: liveHidden=%t replayHidden=%t", tc.name, liveHidden, replayHidden)
			}
			if liveHidden != tc.wantHidden {
				t.Fatalf("%s: hidden=%t, want %t", tc.name, liveHidden, tc.wantHidden)
			}
		})
	}
}

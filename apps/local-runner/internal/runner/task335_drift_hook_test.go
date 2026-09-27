package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/driftdetect"
	"flowpilot-runner/internal/flowgate"
)

// driftEventsPath is the CP-23 §5 workspace artifact location the Task-335
// hook persists to (under the TARGET project workspace .flowpilot dir).
func driftEventsPath(workspaceCwd string) string {
	return filepath.Join(workspaceCwd, ".flowpilot", driftEventsFileName)
}

// readDriftEvents parses the persisted JSONL drift events.
func readDriftEvents(t *testing.T, path string) []driftdetect.DriftEvent {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open drift events: %v", err)
	}
	defer f.Close()
	var events []driftdetect.DriftEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev driftdetect.DriftEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("parse drift event line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan drift events: %v", err)
	}
	return events
}

// MVP posture: the drift detector is always ON — the env flag is ignored.
// The under-budget prompt seam still passes through byte-identical, and a
// blatantly drifting pair of turns persists drift events even with the
// legacy env explicitly cleared.
func TestTask335_DriftHookDisabled_NoEventWritten_PromptByteIdentical(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "0") // env no longer gates — must be ignored
	prompt := "---\n\nDo the task in scope."

	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-335-off", workspaceCwd: cwd}

	// Under-budget prompts stay byte-identical (packer passthrough).
	if got := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-0"); got != prompt {
		t.Fatalf("under-budget prompt must pass through byte-identical (got %d bytes, want %d)", len(got), len(prompt))
	}

	// Two blatantly drifting turns through the REAL hook: a repeated apology
	// (≥2 consecutive turns) plus an out-of-scope edit and a repeated test
	// failure. The detector is always on now — the event file IS written.
	apology := "Tôi rất xin lỗi, tôi sẽ thử lại cách khác."
	tr1 := &flowgate.TurnResult{
		FinalMessage:         apology,
		ChangedPaths:         []string{"config/secret.go"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
		Tests:                flowgate.TestOutcome{Failed: []string{"TestCalc_Add_Fails"}},
		WorkspaceCwd:         cwd,
	}
	tr2 := &flowgate.TurnResult{
		FinalMessage:         apology,
		ChangedPaths:         []string{"config/secret.go"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
		Tests:                flowgate.TestOutcome{Failed: []string{"TestCalc_Add_Fails"}},
		WorkspaceCwd:         cwd,
	}
	s.recordDriftTelemetry(rs, "turn-1", tr1)
	s.recordDriftTelemetry(rs, "turn-2", tr2)

	if _, err := os.Stat(driftEventsPath(cwd)); os.IsNotExist(err) {
		t.Fatal("drift detector is always on — the event file must be written")
	}
}

// Task-335 acceptance (§6): when the AI apologizes 2 turns in a row the drift
// score rises, a drift event is persisted with correction_action=
// inject_system_note, and the next prompt assembly carries the injected
// system note (one-shot — the assembly after that is clean again).
func TestTask335_DriftHookEnabled_ApologyLoop_WritesEventAndInjectsNote(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-335-apology", workspaceCwd: cwd}

	apology := "Tôi rất xin lỗi, tôi sẽ thử lại cách khác."
	// turn-1: a single cheap apology — normal conversation, no drift event
	// (resolved Open Question: the loop needs ≥2 consecutive turns).
	s.recordDriftTelemetry(rs, "turn-1", &flowgate.TurnResult{FinalMessage: apology, WorkspaceCwd: cwd})
	if _, err := os.Stat(driftEventsPath(cwd)); !os.IsNotExist(err) {
		t.Fatalf("single apology must not persist a drift event, stat err: %v", err)
	}

	// turn-2: apology again + heavy token burn with zero file delta — the
	// canonical Task-335 §3 wrong-way signature (apology_loop + zero delta).
	// The per-turn token usage arrives as the provider-agnostic
	// EventTokenUsageUpdated event, exactly like the live adapters emit it.
	rs.events = append(rs.events, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last: &TokenUsageBreakdown{TotalTokens: 3000},
		},
	})
	s.recordDriftTelemetry(rs, "turn-2", &flowgate.TurnResult{FinalMessage: apology, WorkspaceCwd: cwd})

	events := readDriftEvents(t, driftEventsPath(cwd))
	if len(events) != 1 {
		t.Fatalf("exactly one drift event must be persisted, got %d (%+v)", len(events), events)
	}
	ev := events[0]
	if ev.RunID != "run-335-apology" {
		t.Fatalf("event must carry the run id, got %q", ev.RunID)
	}
	if ev.TurnID != "turn-2" {
		t.Fatalf("event must carry the turn id, got %q", ev.TurnID)
	}
	if ev.DriftScore < 30 || ev.DriftScore > 100 {
		t.Fatalf("apology loop drift score must be in [30,100], got %d", ev.DriftScore)
	}
	if ev.CorrectionAction != driftdetect.ActionInjectSystemNote {
		t.Fatalf("apology loop must resolve inject_system_note, got %q", ev.CorrectionAction)
	}
	if !hasDriftSignal(ev.TriggeredSignals, driftdetect.SignalApologyLoop) {
		t.Fatalf("event must cite apology_loop, got %v", ev.TriggeredSignals)
	}
	if ev.SystemNotePrompt == "" {
		t.Fatalf("persisted event must carry the generated system note for the Task-336 handoff")
	}

	// The NEXT prompt assembly consumes the pending action: the system note
	// is appended to the (otherwise unchanged) prompt.
	prompt := "---\n\nFix the pagination bug."
	got := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-3")
	if !strings.HasPrefix(got, prompt) {
		t.Fatalf("the original prompt must be preserved verbatim before the note, got %q", got)
	}
	note := strings.TrimPrefix(got, prompt+"\n\n")
	if !strings.Contains(note, driftdetect.SignalApologyLoop) || !strings.Contains(note, "FlowPilot Drift Detector") {
		t.Fatalf("next prompt must carry the drift system note citing the signals, got %q", note)
	}
	if strings.Contains(note, "\n") && strings.Count(note, "drift_score=") != 1 {
		t.Fatalf("exactly one note must be injected, got %q", note)
	}

	// One-shot: the following assembly is clean again (byte-identical).
	if got2 := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-4"); got2 != prompt {
		t.Fatalf("ladder action must be consumed one-shot, got %q", got2)
	}
}

// Task-335 acceptance (§6): r-scope violation combined with a repeated test
// failure scores ≥ 60 → the narrow_context rung — the next prompt is packed
// under the halved (tightened) Budget Packer budget via the Task-334 seam,
// even with the Budget Packer rollout flag itself left OFF.
func TestTask335_DriftHookEnabled_ScopeAndTestLoop_NarrowsNextPrompt(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-335-narrow", workspaceCwd: cwd}

	// turn-1: the test fails (no prior turn — no loop yet).
	s.recordDriftTelemetry(rs, "turn-1", &flowgate.TurnResult{
		FinalMessage: "Đang sửa lỗi test.",
		ChangedPaths: []string{"internal/calc/calc.go"},
		Tests:        flowgate.TestOutcome{Failed: []string{"TestCalc_Add_Fails"}},
	})
	// turn-2: same test fails again AND the turn edited out of scope.
	s.recordDriftTelemetry(rs, "turn-2", &flowgate.TurnResult{
		FinalMessage:         "Đã thử sửa lại nhưng test vẫn fail.",
		ChangedPaths:         []string{"config/secret.go"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
		Tests:                flowgate.TestOutcome{Failed: []string{"TestCalc_Add_Fails"}},
	})

	events := readDriftEvents(t, driftEventsPath(cwd))
	if len(events) != 1 {
		t.Fatalf("only the loop turn must be persisted, got %d events", len(events))
	}
	if events[0].CorrectionAction != driftdetect.ActionNarrowContext {
		t.Fatalf("scope violation + repeated test failure must resolve narrow_context, got %q (score %d)",
			events[0].CorrectionAction, events[0].DriftScore)
	}
	if events[0].DriftScore < 60 || events[0].DriftScore > 79 {
		t.Fatalf("narrow_context rung requires score 60-79, got %d", events[0].DriftScore)
	}

	// The next prompt assembly packs under the HALVED budget (Task-334 seam,
	// tightened) — the oversized raw excerpt is pruned hard, the current task
	// is retained whole (CP-23 R-1).
	prompt := task334ComposedPrompt(6000) // ~264KB ≈ 66k tokens of raw excerpt
	packed := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-3")
	if len(packed) >= len(prompt) {
		t.Fatalf("narrow_context must tighten the packed prompt: packed=%d >= original=%d", len(packed), len(prompt))
	}
	if len(packed) > 4000*4+1024 { // halved total budget (4000 tokens, ~4 chars/token) + header margin
		t.Fatalf("packed prompt must stay within the halved 4000-token budget, got %d bytes", len(packed))
	}
	if !strings.Contains(packed, "Implement the add function per the contract.") {
		t.Fatalf("current task must be retained under the narrowed budget")
	}
}

// hasDriftSignal is the integration-side signal membership helper.
func hasDriftSignal(signals []string, name string) bool {
	for _, s := range signals {
		if strings.EqualFold(strings.TrimSpace(s), name) {
			return true
		}
	}
	return false
}

// Review-hardening regression (Task-335 §8 follow-up): gate re-evaluation of
// the SAME completed turn (resume path) must not compare the turn against
// itself — the stored history entry with the same TurnID is dropped before
// evaluating, so re-running the gate cannot inflate the score (a lone
// scope-violation turn stays at 35 instead of self-firing
// repeated_test_failure and jumping to the narrow_context rung).
func TestTask335_DriftHookResumeSameTurn_DoesNotSelfCompare(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-335-resume", workspaceCwd: cwd}

	tr := &flowgate.TurnResult{
		FinalMessage:         "Đang sửa lỗi test.",
		ChangedPaths:         []string{"internal/calc/calc.go"},
		ScopeOutOfScopePaths: []string{"config/secret.go"},
		Tests:                flowgate.TestOutcome{Failed: []string{"TestCalc_Add_Fails"}},
	}
	s.recordDriftTelemetry(rs, "turn-1", tr)
	events := readDriftEvents(t, driftEventsPath(cwd))
	if len(events) != 1 || events[0].DriftScore != 35 {
		t.Fatalf("first evaluation must score exactly 35, got %+v", events)
	}

	// Resume path: the gate re-runs for the SAME turn (JSONL keeps one line
	// per evaluation — documented; Phase 3 dedupes by run_id+turn_id).
	s.recordDriftTelemetry(rs, "turn-1", tr)
	events = readDriftEvents(t, driftEventsPath(cwd))
	if len(events) != 2 {
		t.Fatalf("resume persists exactly one more event line, got %d", len(events))
	}
	if events[1].DriftScore != 35 {
		t.Fatalf("same-turn re-evaluation must not self-compare (no score inflation), got %d", events[1].DriftScore)
	}
	if events[1].CorrectionAction != driftdetect.ActionInjectSystemNote {
		t.Fatalf("re-evaluation must stay on the inject_system_note rung, got %q", events[1].CorrectionAction)
	}
}

// Review-hardening regression: the process-wide drift state map is bounded —
// entries for finished runs are evicted instead of accumulating forever.
func TestTask335_DriftStateMap_BoundedEviction(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	s := &InteractiveService{}
	for i := 0; i < driftStateMaxEntries+10; i++ {
		driftStateFor(s, fmt.Sprintf("run-335-evict-%d", i))
	}
	driftStates.Lock()
	defer driftStates.Unlock()
	if len(driftStates.m) > driftStateMaxEntries {
		t.Fatalf("drift state map must stay bounded, got %d entries (cap %d)", len(driftStates.m), driftStateMaxEntries)
	}
}

// Review round-2 hardening: an assembly with an EMPTY prompt consumes the
// pulled ladder actions but must re-stash them — the pending drift note still
// reaches the next NON-empty prompt instead of being silently dropped.
func TestTask335_LadderNoteRestashedOnEmptyPrompt(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-335-restash", workspaceCwd: cwd}

	// turn-1: first apology with a file delta and no token event yet (score 0,
	// carried 0) so turn-2's apology loop + zero-delta lands on the 30-59
	// inject_system_note rung instead of narrow_context.
	apology := "Tôi rất xin lỗi, tôi sẽ thử lại cách khác."
	s.recordDriftTelemetry(rs, "turn-1", &flowgate.TurnResult{
		FinalMessage: apology,
		ChangedPaths: []string{"internal/calc/calc.go"},
		WorkspaceCwd: cwd,
	})
	rs.events = append(rs.events, ProviderEvent{
		Type:       EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{Last: &TokenUsageBreakdown{TotalTokens: 3000}},
	})
	s.recordDriftTelemetry(rs, "turn-2", &flowgate.TurnResult{FinalMessage: apology, WorkspaceCwd: cwd})

	if got := s.applyBudgetPackerIfEnabled(rs, "", "turn-3"); got != "" {
		t.Fatalf("empty prompt must stay empty, got %q", got)
	}
	prompt := "---\n\nFix the pagination bug."
	got := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-4")
	if !strings.Contains(got, "FlowPilot Drift Detector") {
		t.Fatalf("re-stashed note must reach the next non-empty prompt, got %q", got)
	}
	if got2 := s.applyBudgetPackerIfEnabled(rs, prompt, "turn-5"); got2 != prompt {
		t.Fatalf("re-stashed note must still be one-shot, got %q", got2)
	}
}

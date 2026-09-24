package runner

import (
	"testing"

	"flowpilot-runner/internal/driftdetect"
	"flowpilot-runner/internal/flowgate"
)

// BUG-467: the drift detector's FilesChanged is fed from the turn-scoped
// workspace diff verbatim. On a workspace where .flowpilot/** is git-tracked,
// the runner's OWN state writes (dispatch.ndjson, sessions.ndjson, per-run
// turns.ndjson — appended during every turn) land in every turn diff, so
// FilesChanged is never empty and the zero_delta_progress signal can never
// fire — the whole correction ladder is unreachable on such workspaces.
//
// BUG-288 #16/F-25 already classifies .flowpilot/** as "runtime metadata,
// not product code" (flowgate.IsDocOrAuditFile); the drift summary must apply
// the same exclusion so only real workspace deltas count as progress.
func TestBug467_DriftSummaryExcludesRuntimeMetadataFromFilesChanged(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-467", workspaceCwd: cwd}

	// Provider reported >2000 tokens; the ONLY "changed paths" the turn-scoped
	// diff observed are the runner's own .flowpilot/** bookkeeping writes that
	// happen during every turn — no agent-produced workspace delta.
	rs.events = append(rs.events, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last: &TokenUsageBreakdown{TotalTokens: 99226},
		},
	})
	tr := &flowgate.TurnResult{
		FinalMessage: "Here is a detailed explanation of the GC algorithm.",
		ChangedPaths: []string{
			".flowpilot/chats/957928cc-1f80-43ce-a7e8-2cf2ebb36595/dispatch.ndjson",
			".flowpilot/chats/run-467-turns.ndjson",
			".flowpilot/chats/sessions.ndjson",
		},
	}
	s.recordDriftTelemetry(rs, "turn-1", tr)

	st := driftStateFor(s, rs.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 20 {
		t.Fatalf("runtime-metadata-only delta must score zero_delta_progress (+20), got score=%d history=%+v", st.lastScore, st.history)
	}
	if got := st.history[0].FilesChanged; len(got) != 0 {
		t.Fatalf(".flowpilot/** paths must be excluded from drift FilesChanged, got %v", got)
	}
}

// The same exclusion must NOT swallow real agent deltas mixed into the same
// diff — a turn that wrote product code alongside the runner bookkeeping is
// a genuine progress turn (no zero_delta signal).
func TestBug467_DriftSummaryKeepsRealDeltasMixedWithMetadata(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-467b", workspaceCwd: cwd}

	rs.events = append(rs.events, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last: &TokenUsageBreakdown{TotalTokens: 50000},
		},
	})
	tr := &flowgate.TurnResult{
		FinalMessage: "Implemented the parser.",
		ChangedPaths: []string{
			".flowpilot/chats/sessions.ndjson",
			"snake/parser.go",
		},
	}
	s.recordDriftTelemetry(rs, "turn-1", tr)

	st := driftStateFor(s, rs.id)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.lastScore != 0 {
		t.Fatalf("real file delta mixed with metadata must not score zero_delta, got score=%d", st.lastScore)
	}
	if got := st.history[0].FilesChanged; len(got) != 1 || got[0] != "snake/parser.go" {
		t.Fatalf("only .flowpilot/** must be filtered, got %v", got)
	}
}

// Four consecutive runtime-metadata-only turns must climb the ladder to
// pause_for_human (4 x +20 = 80) — the live bed scenario that BUG-467 made
// unreachable.
func TestBug467_DriftLadderReachesPauseOnMetadataOnlyDeltas(t *testing.T) {
	t.Setenv(driftDetectorEnvFlag, "1")
	cwd := t.TempDir()
	s := &InteractiveService{}
	rs := &interactiveRun{id: "run-467c", workspaceCwd: cwd}

	rs.events = append(rs.events, ProviderEvent{
		Type: EventTokenUsageUpdated,
		TokenUsage: &TokenUsageSnapshot{
			Last: &TokenUsageBreakdown{TotalTokens: 60000},
		},
	})
	metaOnly := []string{".flowpilot/chats/sessions.ndjson"}
	var last driftdetect.DriftEvent
	for i, turnID := range []string{"t1", "t2", "t3", "t4"} {
		s.recordDriftTelemetry(rs, turnID, &flowgate.TurnResult{
			FinalMessage: "Still explaining.",
			ChangedPaths: metaOnly,
		})
		_ = i
	}
	_ = last
	events := readDriftEvents(t, driftEventsPath(cwd))
	if len(events) != 4 {
		t.Fatalf("4 zero-delta turns must persist 4 drift events, got %d", len(events))
	}
	if events[3].DriftScore != 80 || events[3].CorrectionAction != driftdetect.ActionPauseForHuman {
		t.Fatalf("4th zero-delta turn must reach pause_for_human at score 80, got %+v", events[3])
	}
	if events[1].CorrectionAction != driftdetect.ActionInjectSystemNote {
		t.Fatalf("2nd turn (score 40) must resolve inject_system_note, got %q", events[1].CorrectionAction)
	}
	if events[2].CorrectionAction != driftdetect.ActionNarrowContext {
		t.Fatalf("3rd turn (score 60) must resolve narrow_context, got %q", events[2].CorrectionAction)
	}
}

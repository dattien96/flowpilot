package runner

// Live-found (CP-84 live matrix, run-1 on devin/swe-2): a chat run whose turn
// settled Completed was stamped Cancelled ~200ms later — sessions.ndjson
// recorded completed then cancelled for the same turn. The turn's context is
// cancelled during normal post-settle teardown; if that cancellation reaches
// finishTurn as context.Canceled, the generic "interrupted by user" branch
// overwrites the terminal status unconditionally (sibling branches all guard:
// stalledSkipCause preserves Failed, line ~7787 requires status==Running).
// Terminal states must be sticky: a completed turn can never become cancelled.

import (
	"context"
	"testing"
)

func TestFinishTurnLateCancelPreservesCompleted(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.status = RunStatusCompleted
	rs.agentStatus = string(RunStatusCompleted)
	rs.turnInFlight = true
	svc.mu.Unlock()

	// The turn already settled; this is the teardown cancel arriving late.
	svc.finishTurn(rs, "turn-1", context.Canceled)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.status != RunStatusCompleted {
		t.Fatalf("status = %q, want %q — late cancel must not overwrite a settled turn", rs.status, RunStatusCompleted)
	}
}

package runner

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ---- T-43: account switch recreates app-server, interrupts in-flight ----------

// Switching the active account interrupts the in-flight turn (marked recoverable /
// cancelled) and, afterwards, the run bound to the previous account can no longer
// start a turn — its threads are hidden until that account is active again. A new
// run created under the new account runs normally (one account at a time).
func TestAccountSwitchInterruptsAndScopes(t *testing.T) {
	_, srv := newTestServer(t)

	// run on the default account, blocked mid-turn on a question
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "question-required", nil)
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusWaitingQuestion }, "in-flight")

	// switch active account → interrupts the in-flight turn
	st, body := doJSON(t, "POST", srv.URL+"/client/active-account", map[string]string{"accountId": "acct-2"}, nil)
	if st != http.StatusOK {
		t.Fatalf("set active account status=%d body=%s", st, body)
	}
	var res struct {
		ActiveAccountID  string `json:"activeAccountId"`
		InterruptedTurns int    `json:"interruptedTurns"`
	}
	_ = json.Unmarshal(body, &res)
	if res.ActiveAccountID != "acct-2" || res.InterruptedTurns != 1 {
		t.Fatalf("switch result = %+v, want acct-2 + 1 interrupted", res)
	}

	// the interrupted turn is cancelled (recoverable, re-sendable)
	waitFor(t, func() bool { return getSnapshot(t, srv.URL, runID).Status == RunStatusCancelled }, "cancelled")
	evs := waitTerminal(t, srv.URL, runID)
	last := evs[len(evs)-1]
	if last.Type != EventTurnFailed || !last.Recoverable {
		t.Fatalf("interrupted turn last event = %s recoverable=%v, want recoverable turn_failed", last.Type, last.Recoverable)
	}

	// the old run's account is no longer active → turn is scoped out (409)
	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("turn on previous-account run status=%d, want 409 (threads hidden)", st)
	}

	// a fresh run under the new active account runs normally
	runID2 := startRun(t, srv.URL)
	if st, turnID := sendTurn(t, srv.URL, runID2, "normal", nil); st != http.StatusOK || turnID == "" {
		t.Fatalf("turn on new-account run status=%d turnId=%q", st, turnID)
	}
	waitTerminal(t, srv.URL, runID2)
}

// switching back to the original account re-enables its runs (threads visible again).
func TestAccountSwitchBackReenablesRun(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL) // bound to "default"

	svc.SetActiveAccount("acct-2")
	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusConflict {
		t.Fatalf("expected 409 while account switched away, got %d", st)
	}

	svc.SetActiveAccount("default")
	if st, _ := sendTurn(t, srv.URL, runID, "normal", nil); st != http.StatusOK {
		t.Fatalf("expected 200 after switching back, got %d", st)
	}
	waitTerminal(t, srv.URL, runID)
}

// ---- T-02: multi-workspace (per-run cwd) -----------------------------------

// Two runs with different cwd run independently; each run's working directory is
// carried and surfaced (per-thread cwd is authoritative, not Runner.workspace).
func TestMultiWorkspaceRunsIndependent(t *testing.T) {
	_, srv := newTestServer(t)

	startWithCwd := func(cwd string) string {
		st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs",
			StartRunInput{ProjectID: "proj-web", WorkflowID: "wf-feature", StepID: "step-plan", Cwd: cwd}, nil)
		if st != http.StatusOK {
			t.Fatalf("start run cwd=%s status=%d body=%s", cwd, st, body)
		}
		var h RunHandle
		_ = json.Unmarshal(body, &h)
		return h.RunID
	}

	runA := startWithCwd("/work/project-a")
	runB := startWithCwd("/work/project-b")

	// both runs complete independently
	sendTurn(t, srv.URL, runA, "normal", nil)
	sendTurn(t, srv.URL, runB, "file-changes", nil)
	evsA := waitTerminal(t, srv.URL, runA)
	evsB := waitTerminal(t, srv.URL, runB)
	if evsA[len(evsA)-1].Type != EventTurnCompleted || evsB[len(evsB)-1].Type != EventTurnCompleted {
		t.Fatal("both multi-workspace runs should complete")
	}

	// each run's cwd is surfaced on the admin session view
	assertCwd := func(runID, want string) {
		st, body := doJSON(t, "GET", srv.URL+"/admin/workflow-runs/"+runID+"/provider-sessions", nil, nil)
		if st != http.StatusOK {
			t.Fatalf("sessions status=%d", st)
		}
		var sessions []map[string]any
		_ = json.Unmarshal(body, &sessions)
		if len(sessions) != 1 || sessions[0]["workingDirectory"] != want {
			t.Fatalf("run %s workingDirectory = %v, want %s", runID, sessions, want)
		}
	}
	assertCwd(runA, "/work/project-a")
	assertCwd(runB, "/work/project-b")
}

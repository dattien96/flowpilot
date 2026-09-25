package runner

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// BUG-392 (live CP-61/CP-62): the HTTP flow-control endpoint applied the raw
// status without the per-AC verdict coverage check the provider bridge path
// runs — a partial review verdict posted to the hub run advanced the flow even
// though the same submission via submit_review_outcome is rejected in-turn.
// The bridge ordering also buried the AC error under the delegate-child
// routing rejection; the coverage verdict must win on both surfaces.
func TestBug392_HTTPFlowControlEnforcesACCoverage(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.workspaceCwd = t.TempDir()
	rs.vibeTaskName = "Task-1-auth.md"
	svc.mu.Unlock()
	coverageWriteDoc(t, rs.workspaceCwd, "requirements/08-Task/todo/Task-1-auth.md",
		"# Task\n- [ ] AC-1 login\n- [ ] AC-2 refresh\n", time.Now())

	// Partial verdict set (AC-1 only) must be rejected with the missing-AC
	// error — the same verdict the bridge path returns.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/flow-control", map[string]any{
		"status":   "approved",
		"feedback": "looks fine",
		"verdicts": []map[string]any{{"ac_id": "AC-1", "verdict": "pass"}},
	}, nil)
	if status == http.StatusOK {
		t.Fatalf("partial verdict must not be accepted; body=%s", body)
	}
	if !strings.Contains(string(body), "AC-2") {
		t.Fatalf("rejection must name missing AC-2, got status=%d body=%s", status, body)
	}
}

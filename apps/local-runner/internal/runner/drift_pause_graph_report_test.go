package runner

import (
	"encoding/json"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// CP-23 / CP-62: the client-facing graph report must preserve the drift park,
// independently of the top-level provider run status.
func TestDriftPauseGraphReportPreservesBlockedReason(t *testing.T) {
	for _, mode := range []string{workingmode.Dev, workingmode.Vibe} {
		t.Run(mode, func(t *testing.T) {
			svc, rs := driftPauseRun(t, mode)
			svc.armDriftPause(rs, driftPauseEvent())
			body, err := json.Marshal(svc.agentOrchestrator.graphSnapshot(rs.id))
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				LoopState struct {
					Status      string `json:"status"`
					BlockReason string `json:"blockReason"`
					GateReason  string `json:"gateReason"`
				} `json:"loopState"`
			}
			if err := json.Unmarshal(body, &report); err != nil {
				t.Fatal(err)
			}
			if mode == workingmode.Dev {
				if report.LoopState.Status != "blocked" || report.LoopState.BlockReason != "drift" || !strings.Contains(report.LoopState.GateReason, "86") {
					t.Fatalf("report lost drift park: %s", body)
				}
			} else if report.LoopState.Status == "blocked" || report.LoopState.BlockReason == "drift" {
				t.Fatalf("vibe report incorrectly carries drift park: %s", body)
			}
		})
	}
}

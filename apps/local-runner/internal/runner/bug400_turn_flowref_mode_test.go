package runner

import (
	"net/http"
	"strings"
	"testing"
)

// BUG-400 (live CP-41 runs 11597/13679/15641): handleStartTurn skipped every
// working-mode gate for vibe/harness family flowRefs — a dev-mode run launched
// AND completed a full vibe-sprint via POST /turns {"flowRef":"vibe-sprint"}.
// The turn-level flowRef must pass FlowAllowedForWorkingMode exactly like the
// run-create path does.
func TestBug400_TurnFlowRefRespectsWorkingMode(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId":  "chat",
		"prompt":  "start the sprint",
		"flowRef": "vibe-sprint",
	}, nil)
	if status != http.StatusBadRequest && status != http.StatusForbidden {
		t.Fatalf("vibe-sprint on a dev run must be rejected, got status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), "working_mode_flow_forbidden") {
		t.Fatalf("want working_mode_flow_forbidden, got status=%d body=%s", status, body)
	}
}

// Control: a harness flow allowed under dev is NOT rejected by the mode gate
// (it may still fail downstream in the fake environment, but never with
// working_mode_flow_forbidden).
func TestBug400_TurnFlowRefHarnessAllowedUnderDev(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId":  "chat",
		"prompt":  "run the harness",
		"flowRef": "task-harness",
	}, nil)
	if status != http.StatusOK && strings.Contains(string(body), "working_mode_flow_forbidden") {
		t.Fatalf("task-harness is dev-allowed and must not hit the mode gate, got status=%d body=%s", status, body)
	}
}

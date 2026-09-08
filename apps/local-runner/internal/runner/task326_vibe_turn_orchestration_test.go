package runner

import (
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func TestStartTurn_VibeIngestNotBlockedByBugSubMode(t *testing.T) {
	_, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("start status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	st, turnBody := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+h.RunID+"/turns", map[string]any{
		"prompt":  "Build a snake game",
		"subMode": "bug",
		"flowRef": "vibe-ingest",
	}, xClient("tui"))
	if st == http.StatusBadRequest && strings.Contains(string(turnBody), "invalid_flow_ref") {
		t.Fatalf("vibe-ingest turn must not fail chat orchestration: %s", turnBody)
	}
	if st >= 500 {
		t.Fatalf("turn status=%d body=%s", st, turnBody)
	}
}

func TestSkipChatOrchestrationCheck_VibeAndHarness(t *testing.T) {
	if !workingmode.SkipChatOrchestrationCheck("vibe-ingest") {
		t.Fatal("vibe-ingest")
	}
	if !workingmode.SkipChatOrchestrationCheck("flowpilot-core-flow-pack/vibe-cp-ingest") {
		t.Fatal("pack vibe-cp-ingest")
	}
	if !workingmode.SkipChatOrchestrationCheck("task-harness") {
		t.Fatal("task-harness")
	}
	if workingmode.SkipChatOrchestrationCheck("flowpilot-core-flow-pack/review-loop") {
		t.Fatal("review-loop stays under chat orchestration")
	}
}

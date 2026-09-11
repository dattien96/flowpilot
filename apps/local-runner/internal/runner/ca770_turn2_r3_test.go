package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func TestClaudeMCPVibeRequirementRejectedWhenOnlyReviewOffered(t *testing.T) {
	srv := newClaudeMCPServer()
	bridge := &fakeGrokBridge{}
	tok := srv.register(bridge, true)
	defer srv.unregister(tok)

	_, resp := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "vibe-requirement-outcome",
			"arguments": map[string]any{"verdict": "aligned"},
		},
	})
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("expected error rejecting vibe-requirement-outcome when not offered, got %+v", resp)
	}
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "vibe-requirement-outcome is not available") {
		t.Fatalf("error message=%q", msg)
	}
	if bridge.flowControlIn.Status != "" {
		t.Fatalf("SubmitFlowControl must not run, got %+v", bridge.flowControlIn)
	}
}

func TestClaudeMCPVibeRequirementRoundTripWhenAllowed(t *testing.T) {
	srv := newClaudeMCPServer()
	bridge := &fakeGrokBridge{
		flowControlResult: FlowControlResult{Status: "done"},
	}
	tok := srv.register(bridge, true)
	defer srv.unregister(tok)
	srv.setAllowVibeRequirement(tok, true)

	raw, errObj := srv.dispatch("tools/call", map[string]any{
		"params": map[string]any{
			"name":      "vibe-requirement-outcome",
			"arguments": map[string]any{"verdict": "aligned", "summary": "AC-1 covered"},
		},
	}, tok)
	if errObj != nil {
		t.Fatalf("tools/call vibe-requirement-outcome error: %+v", errObj)
	}
	if raw == nil {
		t.Fatal("empty result")
	}
	if bridge.flowControlIn.Status != "done" {
		t.Fatalf("SubmitFlowControl status=%q want done (aligned)", bridge.flowControlIn.Status)
	}
}

func TestClaudeMCPVibeRequirementNotOnToolsListUntilAllowed(t *testing.T) {
	srv := newClaudeMCPServer()
	tok := srv.register(&fakeGrokBridge{}, true)
	defer srv.unregister(tok)

	_, listed := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/list",
	})
	res, _ := listed["result"].(map[string]any)
	tools, _ := res["tools"].([]any)
	if toolNamed(tools, "vibe-requirement-outcome") {
		t.Fatal("must not advertise vibe face when only review is offered")
	}

	srv.setAllowVibeRequirement(tok, true)
	_, listed2 := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/list",
	})
	res2, _ := listed2["result"].(map[string]any)
	tools2, _ := res2["tools"].([]any)
	if !toolNamed(tools2, "vibe-requirement-outcome") {
		t.Fatal("must advertise vibe face after setAllowVibeRequirement")
	}
}

func TestVibeSession_ReconstructAwaitingLockAndIdempotent(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeAwaitingLock = true
	rs.vibeLockNodeID = vibeSSLockNodeID
	rs.vibeLockPath = "requirements/05-System-Specs/SS-18-Vibe-Working-Mode.md"
	rs.vibeLockedSS = rs.vibeLockPath
	rs.vibeTaskPlan = []string{"sprint-0"}
	st := sessionStateOf(rs)
	svc.mu.Unlock()

	svc2 := NewInteractiveService()
	got, recErr := svc2.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if !got.vibeAwaitingLock || got.vibeLockNodeID != vibeSSLockNodeID || got.vibeLockedSS == "" {
		t.Fatalf("lock park lost: awaiting=%v node=%q ss=%q", got.vibeAwaitingLock, got.vibeLockNodeID, got.vibeLockedSS)
	}
	if got.workingMode != workingmode.Vibe {
		t.Fatalf("mode=%q", got.workingMode)
	}

	st2 := sessionStateOf(got)
	got2, recErr2 := NewInteractiveService().reconstructRun(st2)
	if recErr2 != nil {
		t.Fatalf("second reconstruct: %v", recErr2)
	}
	if !got2.vibeAwaitingLock || got2.vibeLockPath != got.vibeLockPath || len(got2.vibeTaskPlan) != 1 {
		t.Fatalf("idempotent reconstruct lost state: %+v", got2)
	}
}

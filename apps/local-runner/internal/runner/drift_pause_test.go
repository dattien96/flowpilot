package runner

import (
	"testing"

	"flowpilot-runner/internal/driftdetect"
	"flowpilot-runner/internal/workingmode"
)

// Task-348 (CP-23 ladder completion): the drift ≥80 pause leg parks the run
// and asks the human in dev mode; vibe mode never asks (P-1 owner debate).

func driftPauseEvent() driftdetect.DriftEvent {
	return driftdetect.DriftEvent{
		RunID:            "run-1",
		StepID:           "coder",
		DriftScore:       86,
		TriggeredSignals: []string{"scope_expansion"},
		CorrectionAction: driftdetect.ActionPauseForHuman,
	}
}

func driftPauseRun(t *testing.T, mode string) (*InteractiveService, *interactiveRun) {
	t.Helper()
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: mode, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.runs[handle.RunID].workspaceCwd = t.TempDir()
	return svc, svc.runs[handle.RunID]
}

// Scenario: Dev mode drift 80+ -> run bị park (block reason "drift") + event drift_pause_required
func TestDriftPause_DevModeParksRunAndEmitsEvent(t *testing.T) {
	svc, rs := driftPauseRun(t, workingmode.Dev)
	if !svc.armDriftPause(rs, driftPauseEvent()) {
		t.Fatalf("dev drift pause must land")
	}
	st := svc.agentOrchestrator.loopStateFor(rs.id)
	if st.Status != "blocked" || st.BlockReason != DriftPauseBlockReason {
		t.Fatalf("loop state = %+v, want blocked/drift", st)
	}
	if st.GateReason == "" {
		t.Fatalf("gate reason must explain the drift ask")
	}
	svc.mu.Lock()
	var found *ProviderEvent
	for i := len(rs.events) - 1; i >= 0; i-- {
		if rs.events[i].Type == EventDriftPauseRequired {
			found = &rs.events[i]
			break
		}
	}
	svc.mu.Unlock()
	if found == nil {
		t.Fatalf("drift_pause_required event not emitted")
	}
	payload, ok := found.Input.(map[string]any)
	if !ok || payload["score"] != 86 {
		t.Fatalf("event payload = %+v", found.Input)
	}
}

// Scenario: Park 2 lần -> idempotent (không duplicate event)
func TestDriftPause_IdempotentWhileParked(t *testing.T) {
	svc, rs := driftPauseRun(t, workingmode.Dev)
	if !svc.armDriftPause(rs, driftPauseEvent()) {
		t.Fatalf("first pause must land")
	}
	if svc.armDriftPause(rs, driftPauseEvent()) {
		t.Fatalf("second pause must be a no-op while parked")
	}
	svc.mu.Lock()
	count := 0
	for _, ev := range rs.events {
		if ev.Type == EventDriftPauseRequired {
			count++
		}
	}
	svc.mu.Unlock()
	if count != 1 {
		t.Fatalf("drift_pause_required emitted %d times, want 1", count)
	}
}

// Scenario: Vibe mode -> KHÔNG BAO GIỜ park cho drift (P-1 owner debate owns it)
func TestDriftPause_VibeModeNeverAsksUser(t *testing.T) {
	svc, rs := driftPauseRun(t, workingmode.Vibe)
	if svc.armDriftPause(rs, driftPauseEvent()) {
		t.Fatalf("vibe drift must not park the run")
	}
	st := svc.agentOrchestrator.loopStateFor(rs.id)
	if st.Status == "blocked" {
		t.Fatalf("vibe run must not be blocked by drift: %+v", st)
	}
	svc.mu.Lock()
	count := 0
	for _, ev := range rs.events {
		if ev.Type == EventDriftPauseRequired {
			count++
		}
	}
	svc.mu.Unlock()
	if count != 0 {
		t.Fatalf("vibe must not emit drift_pause_required, got %d", count)
	}
}

// Scenario: Flow child drift -> park PARENT (hub drives the loop)
func TestDriftPause_FlowChildParksParent(t *testing.T) {
	svc, parent := driftPauseRun(t, workingmode.Dev)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Dev, Client: "tui",
	})
	if err != nil {
		t.Fatalf("child createRun: %v", err)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = parent.id
	svc.mu.Unlock()

	if !svc.armDriftPause(child, driftPauseEvent()) {
		t.Fatalf("child drift pause must land on the parent")
	}
	pst := svc.agentOrchestrator.loopStateFor(parent.id)
	if pst.Status != "blocked" || pst.BlockReason != DriftPauseBlockReason {
		t.Fatalf("parent loop state = %+v, want blocked/drift", pst)
	}
}

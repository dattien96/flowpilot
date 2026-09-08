package runner

import (
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func TestVibeSessionSnapshot_IncludesLockAndPlan(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-321-Vibe-Cp-Driven-Entry.md"}
	rs.vibeLockedCP = "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"
	rs.vibeAwaitingLock = false
	st := sessionStateOf(rs)
	svc.mu.Unlock()
	if len(st.VibeTaskPlan) != 1 {
		t.Fatalf("plan=%v", st.VibeTaskPlan)
	}
	if st.VibeLockedCP == "" || st.WorkingMode != workingmode.Vibe {
		t.Fatalf("locked=%q mode=%q", st.VibeLockedCP, st.WorkingMode)
	}
}

func TestVibeSlicer_RecordsReadOnlyPlanArtifacts(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.onVibeCpNodeDone(parent.RunID, vibeSSLockNodeID)
	svc.onVibeCpNodeDone(parent.RunID, vibeSprintSlicerNodeID)
	arts, ok := svc.runArtifacts.forRun(parent.RunID)
	if !ok || len(arts) == 0 {
		t.Fatal("sliced plan must be recorded as artifacts")
	}
}

func TestClaudeMCPToolDefsWithVibe_OnlyWhenAllowed(t *testing.T) {
	plain := claudeMCPToolDefsWithVibe(true, false)
	if toolNamed(plain, "vibe-requirement-outcome") {
		t.Fatal("must not advertise vibe face on non-vibe hub")
	}
	vibe := claudeMCPToolDefsWithVibe(true, true)
	if !toolNamed(vibe, "vibe-requirement-outcome") {
		t.Fatal("vibe hub must advertise vibe-requirement-outcome")
	}
	if !toolNamed(vibe, "submit_review_outcome") {
		t.Fatal("review face still advertised")
	}
}

func toolNamed(defs []any, name string) bool {
	for _, d := range defs {
		m, ok := d.(map[string]any)
		if ok && m["name"] == name {
			return true
		}
	}
	return false
}

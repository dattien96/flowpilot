package runner

import "testing"

func TestVibeIngest_AwaitsSSLock(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !svc.vibeSprintStartBlocked(parent.RunID) {
		t.Fatal("vibe-ingest must await ss_lock")
	}
	svc.onVibeCpNodeDone(parent.RunID, vibeSSLockNodeID)
	if svc.vibeSprintStartBlocked(parent.RunID) {
		t.Fatal("ss_lock done must clear awaiting")
	}
}

func TestVibeIngest_SprintSlicerSeedsPlan(t *testing.T) {
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
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if rs == nil || len(rs.vibeTaskPlan) == 0 || rs.vibeTaskPlan[0] != "sprint-0" {
		t.Fatalf("plan=%v", rs.vibeTaskPlan)
	}
	if rs.vibeSprintIndex < 1 {
		t.Fatalf("index=%d, want advanced after auto-start", rs.vibeSprintIndex)
	}
}

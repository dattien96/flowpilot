package runner

import (
	"context"
	"strings"
	"testing"
)

// BUG-547 (live run-39478 on :4322): a vibe-mode flow started via
// POST /client/workflow-runs with workflowId=vibe-cp-ingest dead-ends at
// its first child spawn — the child inherits the parent's workflowID
// ("vibe-cp-ingest", a vibe-family id) but NOT its workingMode, so
// createRun's user-mount gate FlowAllowedForWorkingMode(dev, vibe, user)
// rejects the spawn: flow_start_all_entries_failed, nothing ever runs.
//
// A child spawn is engine-internal — the flow mount was already validated
// when the user started the parent. The child must carry the parent's
// working mode (children of a vibe run are vibe runs) and must not be
// re-validated as a user mount.
func TestBug547_VibeParentChildSpawnNotFlowGated(t *testing.T) {
	svc, _ := clusterFService(t)

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkflowID: "vibe-cp-ingest", WorkingMode: "vibe", Client: "tui",
		Model: "gpt-5.4-mini",
	})
	if apiErr != nil {
		t.Fatalf("createRun vibe parent: %v", apiErr)
	}
	svc.mu.Lock()
	if got := svc.runs[parent.RunID].workingMode; got != "vibe" {
		svc.mu.Unlock()
		t.Fatalf("parent workingMode = %q, want vibe", got)
	}
	svc.mu.Unlock()

	result, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "vibe-intake",
		Prompt: "read the CP",
		Wait:   false,
	})
	if spawnErr != nil {
		t.Fatalf("vibe child spawn must not be flow-gated: %v", spawnErr)
	}
	svc.mu.Lock()
	child := svc.runs[result.RunID]
	got := child.workingMode
	svc.mu.Unlock()
	if got != "vibe" {
		t.Fatalf("child must inherit parent workingMode vibe, got %q", got)
	}
}

// The gate must still reject a USER-mounted vibe family flow under dev mode —
// the bypass applies only to engine-internal child spawns.
func TestBug547_DevUserVibeFlowStillForbidden(t *testing.T) {
	svc, _ := clusterFService(t)
	_, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkflowID: "vibe-cp-ingest", WorkingMode: "dev",
	})
	if apiErr == nil || !strings.Contains(apiErr.code, "working_mode") {
		t.Fatalf("dev user mounting vibe-cp-ingest must stay forbidden, got %v", apiErr)
	}
}

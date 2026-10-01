package runner

// CA-1078: a Flow-mode launch sends the workflow row's catalog UUID as
// WorkflowID — not the pack flow identity. enforceWorkingModeStart gated
// that UUID verbatim, so a Vibe user picking the builtin "Vibe Tasks"
// mirror row got working_mode_flow_forbidden (the UUID is not a bare
// vibe-user id). The gate must resolve the catalog identity first —
// fail-closed for unresolvable/system ids either way.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
}

const (
	ca1078VibeTasksUUID  = "916056ab-56db-4780-b46e-5147b0d9a635"
	ca1078VibeSprintUUID = "11111111-2222-3333-4444-555555555555"
	ca1078UnknownUUID    = "99999999-8888-7777-6666-555555555555"
)

// seedCA1078Store syncs the builtin pack into the fake store and aliases
// catalog-UUID keys to the mirror records, mirroring the real
// SupabaseWorkflowFlowStore.GetByRef contract (id=eq.<uuid> → record whose
// FlowRef is the canonical packId/flowId).
func seedCA1078Store(t *testing.T) *fakeFlowDefinitionStore {
	t.Helper()
	store := newFakeFlowDefinitionStore()
	if _, err := NewFlowMirrorSyncService(store).SyncBuiltins(context.Background()); err != nil {
		t.Fatalf("SyncBuiltins: %v", err)
	}
	for packFlowID, uuid := range map[string]string{
		"vibe-tasks":  ca1078VibeTasksUUID,
		"vibe-sprint": ca1078VibeSprintUUID,
	} {
		rec, ok, err := store.GetByPackFlow(context.Background(),
			"flowpilot-core-flow-pack", packFlowID)
		if err != nil || !ok {
			t.Fatalf("mirror for %q not synced: ok=%v err=%v", packFlowID, ok, err)
		}
		store.byRef[uuid] = rec
	}
	return store
}

// Scenario: vibe user launches the builtin vibe-tasks mirror row via its
// catalog UUID (Flow-mode picker). The gate must see the resolved pack
// identity, not the raw UUID.
// Expect: nil apiErr.
func TestEnforceWorkingModeStart_VibeWorkflowUUIDResolvesToVibeTasks(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	in := StartRunInput{
		WorkingMode: "vibe",
		Client:      "desktop",
		WorkflowID:  ca1078VibeTasksUUID,
	}
	if e := svc.enforceWorkingModeStart(&in); e != nil {
		t.Fatalf("want nil, got %d %s: %s", e.status, e.code, e.msg)
	}
}

// Scenario: vibe user cannot launder a system flow through its catalog
// UUID — the resolved identity is still gated.
// Expect: 400 working_mode_flow_forbidden.
func TestEnforceWorkingModeStart_VibeWorkflowUUIDSystemFlowForbidden(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	in := StartRunInput{
		WorkingMode: "vibe",
		Client:      "desktop",
		WorkflowID:  ca1078VibeSprintUUID,
	}
	e := svc.enforceWorkingModeStart(&in)
	if e == nil || e.code != workingmode.CodeFlowForbidden {
		t.Fatalf("want %s, got %v", workingmode.CodeFlowForbidden, e)
	}
}

// Scenario: an unresolvable UUID stays fail-closed in vibe mode.
// Expect: 400 working_mode_flow_forbidden.
func TestEnforceWorkingModeStart_VibeWorkflowUUIDUnknownForbidden(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	in := StartRunInput{
		WorkingMode: "vibe",
		Client:      "desktop",
		WorkflowID:  ca1078UnknownUUID,
	}
	e := svc.enforceWorkingModeStart(&in)
	if e == nil || e.code != workingmode.CodeFlowForbidden {
		t.Fatalf("want %s, got %v", workingmode.CodeFlowForbidden, e)
	}
}

// Scenario: dev user cannot start a vibe-family flow via its catalog UUID
// either — the immediate first-turn gate already rejected the resolved
// ref; the create gate must not pass it through earlier.
// Expect: 400 working_mode_flow_forbidden.
func TestEnforceWorkingModeStart_DevWorkflowUUIDVibeFlowForbidden(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	in := StartRunInput{
		WorkingMode: "dev",
		WorkflowID:  ca1078VibeTasksUUID,
	}
	e := svc.enforceWorkingModeStart(&in)
	if e == nil || e.code != workingmode.CodeFlowForbidden {
		t.Fatalf("want %s, got %v", workingmode.CodeFlowForbidden, e)
	}
}

// Scenario: a pending-armed vibe run pinned by workflowID forwards to the
// CANONICAL pack ref — the committed chatFlowRef feeds downstream
// BareFlowID consumers (vibeAwaitingLock, isVibeCpSourcedFlowID) that a
// raw UUID would silently miss. The run carries a real CP-shaped source +
// parented task file so the CP-source fence (BUG-399) admits it.
// Expect: flowpilot-core-flow-pack/vibe-tasks, nil apiErr.
func TestForwardPinnedFlow_WorkflowUUIDPinResolvesCanonical(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	cwd := t.TempDir()
	writeTestFile(t, cwd, "requirements/07-Coding-Plan/CP-90.md",
		"Document ID: CP-90\n")
	writeTestFile(t, cwd, "requirements/08-Task/todo/Task-01-thing.md",
		"# Task-01\n\nParent Documents: CP-90\n")
	rs := &interactiveRun{
		id:           "run-ca1078",
		workflowID:   ca1078VibeTasksUUID,
		workingMode:  "vibe",
		flowArm:      FlowArmPending,
		workspaceCwd: cwd,
		sourceDocID:  "requirements/07-Coding-Plan/CP-90.md",
	}
	flowRef, e := svc.forwardPinnedFlow(context.Background(), rs, TurnInput{})
	if e != nil {
		t.Fatalf("want nil, got %d %s: %s", e.status, e.code, e.msg)
	}
	if want := workingmode.PackPrefix + "vibe-tasks"; flowRef != want {
		t.Fatalf("want canonical ref %q, got %q", want, flowRef)
	}
}

// Scenario: the same pin on a vibe-sprint mirror row stays forbidden on a
// forward — resolved identity is gated, not the opaque UUID.
// Expect: 400 working_mode_flow_forbidden.
func TestForwardPinnedFlow_WorkflowUUIDPinSystemFlowForbidden(t *testing.T) {
	svc := NewInteractiveService()
	svc.SetFlowDefinitionStore(seedCA1078Store(t))
	rs := &interactiveRun{
		id:          "run-ca1078-sprint",
		workflowID:  ca1078VibeSprintUUID,
		workingMode: "vibe",
		flowArm:     FlowArmPending,
	}
	if _, e := svc.forwardPinnedFlow(context.Background(), rs, TurnInput{}); e == nil ||
		e.code != workingmode.CodeFlowForbidden {
		t.Fatalf("want %s, got %v", workingmode.CodeFlowForbidden, e)
	}
}

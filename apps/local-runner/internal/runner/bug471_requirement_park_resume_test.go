package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/flowgate"
)

// BUG-471: a vibe "requirement" park (parkVibeRequirement — e.g. task_slicer
// produced zero Task files parented to the run's CP) has no dedicated resume
// branch. Generic Continue falls through to maybeAutoReinvokeHubWithNote,
// which vibeHubSealed skips on post-lock vibe runs (cp_lock already sealed),
// so the unblock leaves a running loop with no work — the run only resurfaces
// via the hub_stall watchdog (live run-102429: validator step flapped
// RUNNING/WAITING with no delegate; live run-103685: unblock → sealed-hub
// no-op → hub_stalled). Resume must re-invoke the same advance that parked:
// re-evaluate the parked condition and either proceed or re-park.
// Additive — no existing tests touched.

func bug471Run(t *testing.T, svc *InteractiveService, dir string) string {
	t.Helper()
	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parentH.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workingMode = "vibe"
	rs.workspaceCwd = dir
	rs.vibeCpDocID = "CP-01"
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: vibeTaskSlicerNodeID}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: vibeTaskSlicerNodeID, To: "done", When: "done"}}
	svc.mu.Unlock()
	return parentH.RunID
}

func bug471Loop(t *testing.T, svc *InteractiveService, runID string) (status, reason string) {
	t.Helper()
	st := svc.agentOrchestrator.loopStateFor(runID)
	return st.Status, st.BlockReason
}

// bug471SeedForeignTasks writes only non-CP-01 tasks so the CP-scoped
// collector returns zero for a CP-01 run (the parked condition).
func bug471SeedForeignTasks(t *testing.T, dir string) {
	t.Helper()
	taskDir := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Task-1-calc-gcd-lcm.md":      bug468TaskNoParent,
		"Task-12-snake-core-model.md": bug468TaskCP02,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Still zero CP-01 tasks at Continue → the same advance must re-run and
// re-park (blocked/requirement), not strand the loop running with no work.
func TestBUG471_RequirementParkContinueReevaluatesAndReparks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	bug471SeedForeignTasks(t, dir) // stale no-parent Task-1 + CP-02 Task-12 only — zero CP-01
	runID := bug471Run(t, svc, dir)

	svc.parkVibeRequirementFrom(runID, "task_slicer produced no Task files parented to CP-01", vibeTaskSlicerNodeID)
	status, reason := bug471Loop(t, svc, runID)
	if status != "blocked" || reason != "requirement" {
		t.Fatalf("precondition: loop = %s/%s, want blocked/requirement", status, reason)
	}

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	status, reason = bug471Loop(t, svc, runID)
	if status != "blocked" || reason != "requirement" {
		t.Fatalf("after continue with still-zero tasks: loop = %s/%s, want blocked/requirement re-park (pre-fix leaves running zombie via sealed hub no-op)", status, reason)
	}
}

// Operator adds the missing CP-01 tasks while parked → Continue must re-run
// the slicer-done advance: CP-scoped plan rebuilt, not a sealed-hub no-op.
func TestBUG471_RequirementParkContinueAdvancesWhenTasksAppear(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	bug468SeedTasks(t, dir)
	runID := bug471Run(t, svc, dir)

	svc.parkVibeRequirementFrom(runID, "task_slicer produced no Task files parented to CP-01", vibeTaskSlicerNodeID)

	// Operator fixes the bed while parked: a CP-01-parented task lands.
	extra := "# Task\n\n## Metadata\n\n- Document ID: `Task-21`\n- Parent Documents: `CP-01` (work item `P-2`), `SS-02`\n"
	if err := os.WriteFile(filepath.Join(dir, "requirements", "08-Task", "todo", "Task-21-snake-loop.md"), []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	plan := append([]string(nil), rs.vibeTaskPlan...)
	fromNode := rs.vibeRequirementFromNode
	svc.mu.Unlock()
	if len(plan) != 2 {
		t.Fatalf("vibeTaskPlan = %v, want the 2 CP-01-parented tasks (Task-9 + Task-21) — resume must rebuild the CP-scoped plan", plan)
	}
	if fromNode != "" {
		t.Fatalf("vibeRequirementFromNode = %q, want consumed", fromNode)
	}
}

// The saved from-node is durable state: a restart mid-park must restore it so
// the post-restart Continue still re-drives the same advance.
func TestBUG471_RequirementFromNodeSurvivesDurableRoundTrip(t *testing.T) {
	svc, _ := newTestServer(t)
	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parentH.RunID]
	rs.workingMode = "vibe"
	rs.vibeRequirementFromNode = vibeTaskSlicerNodeID
	st := sessionStateOf(rs)
	svc.mu.Unlock()
	if st.VibeRequirementFromNode != vibeTaskSlicerNodeID {
		t.Fatalf("sessionStateOf dropped the field: %q", st.VibeRequirementFromNode)
	}
	back, apiErr := svc.reconstructRunInternal(st, true)
	if apiErr != nil {
		t.Fatalf("reconstructRunInternal: %v", apiErr)
	}
	if back.vibeRequirementFromNode != vibeTaskSlicerNodeID {
		t.Fatalf("rehydrated vibeRequirementFromNode = %q, want %q", back.vibeRequirementFromNode, vibeTaskSlicerNodeID)
	}
}

// The coder-resume parks ("tdd artifact missing before coder" / "sprint graph
// missing") carry the resume:coder marker — Continue must re-run
// maybeResumeVibeCoderAfterTdd, which re-parks while tdd evidence is absent.
func TestBUG471_RequirementParkCoderResumeMarkerReparks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parentH.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = "vibe"
	rs.workspaceCwd = dir
	rs.vibeCheckpointNode = "tdd" // tddDone without needing step-status rows
	svc.mu.Unlock()
	runID := parentH.RunID

	svc.parkVibeRequirementFrom(runID, "tdd artifact missing before coder (no bypass)", vibeRequirementResumeCoder)
	if status, reason := bug471Loop(t, svc, runID); status != "blocked" || reason != "requirement" {
		t.Fatalf("precondition: loop = %s/%s, want blocked/requirement", status, reason)
	}

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	// Still no tdd evidence → the re-run resume path must re-park, not strand.
	if status, reason := bug471Loop(t, svc, runID); status != "blocked" || reason != "requirement" {
		t.Fatalf("after continue with no tdd evidence: loop = %s/%s, want blocked/requirement re-park", status, reason)
	}
}

// A turn-level gate requirement park (applyVibeGateResolver) records no advance
// context — it must clear any stale from-node so Continue keeps the generic
// path instead of replaying an earlier advance-path park.
func TestBUG471_GateRequirementParkClearsStaleFromNode(t *testing.T) {
	svc, _ := newTestServer(t)
	parentH, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parentH.RunID]
	rs.workingMode = "vibe"
	rs.vibeRequirementFromNode = vibeTaskSlicerNodeID // stale residue
	svc.mu.Unlock()

	handled := svc.applyVibeGateResolver(parentH.RunID, "", "turn-1", rs, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID, Trigger: "requirement_signature_drift", Action: "block"},
		}},
	})
	if !handled {
		t.Fatal("requirement violation was not classified as a requirement park")
	}
	svc.mu.Lock()
	got := svc.runs[parentH.RunID].vibeRequirementFromNode
	svc.mu.Unlock()
	if got != "" {
		t.Fatalf("stale vibeRequirementFromNode survived the gate park: %q", got)
	}
	if status, reason := bug471Loop(t, svc, parentH.RunID); status != "blocked" || reason != "requirement" {
		t.Fatalf("gate park: loop = %s/%s, want blocked/requirement", status, reason)
	}
}

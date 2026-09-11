package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func task328Write(t *testing.T, cwd, rel, body string) {
	t.Helper()
	p := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task328ArmAfterCpWriter(t *testing.T, svc *InteractiveService, cwd string) string {
	t.Helper()
	runID := "run-task328-cp"
	nodes := []agentpack.FlowNode{
		{ID: "ingest_reader", Behavior: "agent.delegate"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
		{ID: vibeCpWriterNodeID, Behavior: "agent.delegate"},
	}
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		flowEngineDriven:   true,
		workspaceCwd:       cwd,
		vibeSSSealed:       true,
		vibeLockedSS:       "requirements/05-System-Specs/SS-01-snake.md",
		chatFlowRef:        workingmode.PackPrefix + vibeIngestFlowID,
		lastPrompt:         "Build terminal snake MVP",
		vibeCheckpointNode: vibeCpWriterNodeID,
		activeFlowNodes:    nodes,
		activeFlowEdges: []agentpack.FlowEdge{
			{From: vibeSSLockNodeID, To: vibeCpWriterNodeID, When: "done", Kind: "forward"},
			{From: vibeCpWriterNodeID, To: "done", When: "done", Kind: "forward"},
		},
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(t.Context(), runID, "ingest_reader", StepStatusDone)
	svc.setFlowStepStatus(t.Context(), runID, vibeSSLockNodeID, StepStatusDone)
	svc.setFlowStepStatus(t.Context(), runID, vibeCpWriterNodeID, StepStatusDone)
	return runID
}

// R-CP-K: CP on disk, no Tasks → park Resume join (cp_writer→done has no forward edge).
func TestTask328_CpPresentParksJoinResume(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-60-snake.md", "# CP\n")
	runID := task328ArmAfterCpWriter(t, svc, cwd)

	if !svc.maybeParkVibeCpJoinResume(runID) {
		t.Fatal("want CP-join resume park when CP present and no Tasks")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if !rs.vibeResumeConfirm || rs.vibeResumeFromNode != vibeCpWriterNodeID {
		t.Fatalf("confirm=%v from=%q", rs.vibeResumeConfirm, rs.vibeResumeFromNode)
	}
	if rs.vibeAwaitingLock {
		t.Fatal("must not park ss_lock when CP present")
	}
}

// R-CP-D1: CP deleted, SS remains → restart cp_writer (not ss_lock / not ingest_reader).
func TestTask328_CpMissingRestartsCpWriter(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	runID := task328ArmAfterCpWriter(t, svc, cwd)

	if !svc.restartVibeCpWriterForMissingCP(runID) {
		t.Fatal("expected cp_writer rewrite when CP missing and SS sealed")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeAwaitingLock || rs.vibeLockNodeID == vibeSSLockNodeID {
		t.Fatalf("must not re-park ss_lock: awaiting=%v node=%q", rs.vibeAwaitingLock, rs.vibeLockNodeID)
	}
	if rs.vibeResumeConfirm {
		t.Fatal("missing CP must rewrite, not park join resume")
	}
}

func TestTask328_ResumeOKJoinsTaskSlicerWhenCpPresent(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	cpRel := "requirements/07-Coding-Plan/todo/CP-60-snake.md"
	task328Write(t, cwd, cpRel, "# CP\n")
	runID := task328ArmAfterCpWriter(t, svc, cwd)
	svc.mu.Lock()
	svc.runs[runID].vibeResumeConfirm = true
	svc.runs[runID].vibeResumeFromNode = vibeCpWriterNodeID
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})

	if e := svc.SubmitGateDecision(runID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeResumeConfirm {
		t.Fatal("confirm must clear")
	}
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		t.Fatalf("OK must join vibe-cp-ingest, got %q", rs.chatFlowRef)
	}
}

func TestTask328_ProvidersAgnostic(t *testing.T) {
	// Case 1: helpers take no providerKey. Assert decision flags without
	// requiring Claude/Grok createRun adapters (spawn may be unavailable).
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			cwd := t.TempDir()
			task328Write(t, cwd, "requirements/05-System-Specs/SS-01.md", "# SS\n")
			if vibeCPArtifactsPresent(cwd) {
				t.Fatal("CP must be missing")
			}
			rs := &interactiveRun{
				id:           "run-cp-" + string(pk),
				providerKey:  pk,
				workingMode:  workingmode.Vibe,
				workspaceCwd: cwd,
				vibeSSSealed: true,
				vibeLockedSS: "requirements/05-System-Specs/SS-01.md",
			}
			if !vibeSSLockArtifactsPresent(cwd, rs) {
				t.Fatal("SS must be present")
			}
			_ = pk
		})
	}
}

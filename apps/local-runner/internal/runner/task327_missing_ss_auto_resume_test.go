package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func task327WriteSS(t *testing.T, cwd, rel, body string) {
	t.Helper()
	p := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task327ArmSSLockPark(t *testing.T, svc *InteractiveService, cwd, ssRel string) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui", Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workspaceCwd = cwd
	rs.vibeAwaitingLock = true
	rs.vibeLockNodeID = vibeSSLockNodeID
	rs.vibeLockPath = ssRel
	rs.vibeLockedSS = ssRel
	rs.lastPrompt = "Build terminal snake MVP"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "ingest_reader", Behavior: "agent.delegate"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
		{ID: vibeCpWriterNodeID, Behavior: "agent.delegate"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "ingest_reader", To: vibeSSLockNodeID, When: "done", Kind: "forward"},
		{From: vibeSSLockNodeID, To: vibeCpWriterNodeID, When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeLockBlockReason
		st.ActiveNode = vibeSSLockNodeID
		st.GateReason = "SS Preview & Lock"
		return st
	})
	return parent.RunID
}

// Task-327 / R-SS-D: delete SS while parked → reconstruct clears lock park and
// restarts vibe-ingest from ingest_reader (O-6 close).
func TestTask327_ReconstructMissingSSClearsParkAndRestartsIngest(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	ssRel := "requirements/05-System-Specs/SS-01-snake.md"
	task327WriteSS(t, cwd, ssRel, "# SS\n")
	runID := task327ArmSSLockPark(t, svc, cwd, ssRel)

	svc.mu.Lock()
	st := sessionStateOf(svc.runs[runID])
	st.LoopState = svc.agentOrchestrator.loopStateFor(runID)
	svc.mu.Unlock()

	if err := os.Remove(filepath.Join(cwd, filepath.FromSlash(ssRel))); err != nil {
		t.Fatal(err)
	}

	svc2 := NewInteractiveService()
	got, recErr := svc2.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if got.vibeAwaitingLock {
		t.Fatal("Task-327: missing SS must clear vibe_awaiting_lock on reconstruct")
	}
	if got.vibeLockNodeID != "" || got.vibeLockedSS != "" {
		t.Fatalf("lock stamps must clear: node=%q ss=%q", got.vibeLockNodeID, got.vibeLockedSS)
	}
	loop := svc2.agentOrchestrator.loopStateFor(got.id)
	if loop.BlockReason == vibeLockBlockReason || loop.ActiveNode == vibeSSLockNodeID {
		t.Fatalf("zombie ss_lock park remained: %+v", loop)
	}
}

// Task-327 / R-SS-K: SS still on disk → reconstruct keeps awaiting lock (CA-770 contract with cwd).
func TestTask327_ReconstructWithSSKeepsLockPark(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	ssRel := "requirements/05-System-Specs/SS-01-snake.md"
	task327WriteSS(t, cwd, ssRel, "# SS\n")
	runID := task327ArmSSLockPark(t, svc, cwd, ssRel)

	svc.mu.Lock()
	st := sessionStateOf(svc.runs[runID])
	st.LoopState = svc.agentOrchestrator.loopStateFor(runID)
	svc.mu.Unlock()

	svc2 := NewInteractiveService()
	got, recErr := svc2.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if !got.vibeAwaitingLock || got.vibeLockNodeID != vibeSSLockNodeID || got.vibeLockedSS != ssRel {
		t.Fatalf("SS present must keep park: awaiting=%v node=%q ss=%q", got.vibeAwaitingLock, got.vibeLockNodeID, got.vibeLockedSS)
	}
}

// Task-327: Continue with missing SS must not stamp/advance to cp_writer — recover ingest instead.
func TestTask327_ContinueMissingSSRecoversIngestNotCpWriter(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	ssRel := "requirements/05-System-Specs/SS-01-snake.md"
	runID := task327ArmSSLockPark(t, svc, cwd, ssRel) // no file written → missing

	if _, ok := svc.resumeVibeLock(runID, "continue", AgentGraphSnapshot{}); !ok {
		t.Fatal("resumeVibeLock should handle missing-SS recover")
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeAwaitingLock {
		t.Fatal("Continue+missing SS must clear awaiting lock")
	}
	if rs.vibeSSSealed {
		t.Fatal("must not seal SS lock when recovering")
	}
	if rs.vibeLockedSS != "" {
		t.Fatalf("must not stamp locked SS path=%q", rs.vibeLockedSS)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.ActiveNode == vibeCpWriterNodeID {
		t.Fatal("must not advance to cp_writer when SS missing")
	}
}

// Task-327 T-1: empty cwd is unknown — do not treat as missing (keeps CA-770 shape).
func TestTask327_EmptyCwdDoesNotTreatSSAsMissing(t *testing.T) {
	rs := &interactiveRun{
		workingMode: workingmode.Vibe,
		vibeLockedSS: "requirements/05-System-Specs/SS-18-x.md",
	}
	if !vibeSSLockArtifactsPresent("", rs) {
		t.Fatal("empty cwd must report present/unknown")
	}
}

func TestTask327_ProvidersAgnosticHelper(t *testing.T) {
	// Case 1 (cross-provider-parity): maybeRecoverMissingVibeSSLock /
	// vibeSSLockArtifactsPresent take no providerKey and never branch on one.
	// Seed runs per provider without createRun (Claude controlled runtime NI).
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			cwd := t.TempDir()
			runID := "run-" + string(pk)
			svc.mu.Lock()
			svc.runs[runID] = &interactiveRun{
				id:              runID,
				providerKey:     pk,
				workingMode:     workingmode.Vibe,
				workspaceCwd:    cwd,
				vibeAwaitingLock: true,
				vibeLockNodeID:  vibeSSLockNodeID,
				vibeLockedSS:    "requirements/05-System-Specs/SS-missing.md",
				lastPrompt:      "idea",
				subs:            map[int64]chan ProviderEvent{},
			}
			svc.mu.Unlock()
			if !svc.maybeRecoverMissingVibeSSLock(runID) {
				t.Fatalf("%s: expected recover when SS missing", pk)
			}
			svc.mu.Lock()
			defer svc.mu.Unlock()
			if svc.runs[runID].vibeAwaitingLock {
				t.Fatalf("%s: awaiting lock not cleared", pk)
			}
		})
	}
}

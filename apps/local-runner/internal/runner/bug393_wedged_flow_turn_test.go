package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BUG-393 (live CP-49 run-2290/turn-8012): after a vibe sprint wedged, a plain
// POST /turns {stepId:"tdd"} ran an ungated chat turn that wrote production
// code with ZERO gate evaluation — a turn admitted onto a sealed loop
// (turnStartedAfterLoopDone) completed via a branch that never armed settle
// AND runTurn skipped the gate outright ("there is no active flow decision
// left to protect"). The fix keeps BUG-302/305/308's admission + immediate
// broadcast semantics (pendingFlowGateSettle stays unarmed) but evaluates the
// post-turn gate on the diff like every other turn — a violating write
// reprompts instead of silently landing.

// The already-sealed "blocked" loop stays refused by the existing
// flow_awaiting_user fence (run-1675) — pin that it still fires.
func TestBug393_BlockedLoopStillRefused(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "blocked", BlockReason: "hub_stalled"})

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": "tdd",
		"prompt": "continue",
	}, nil)
	if status == http.StatusOK {
		t.Fatalf("turn on a blocked flow loop must be refused, got 200: %s", body)
	}
	if !strings.Contains(string(body), "flow_awaiting_user") {
		t.Fatalf("want the existing flow_awaiting_user fence, got status=%d body=%s", status, body)
	}
}

// BUG-305 pin intact: a post-seal follow-up completes immediately with no
// pending-settle arm (deferral would strand the live broadcast) — the gate
// runs post-completion instead.
func TestBug393_PostSealTurnDoesNotArmSettle(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "done"})
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	rs.turnStartedAfterLoopDone = true
	rs.currentTurnID = "turn-x"
	rs.events = append(rs.events, ProviderEvent{Type: EventFileChanged, Path: "stringutil/reverse.go"})
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done", ProviderTurnID: "turn-x"})
	armed := rs.pendingFlowGateSettle
	st := rs.status
	svc.mu.Unlock()
	if armed {
		t.Fatal("post-seal turn must NOT arm pendingFlowGateSettle (BUG-305: deferral strands the live TurnCompleted)")
	}
	if st != RunStatusCompleted {
		t.Fatalf("post-seal turn must still complete immediately, status=%q", st)
	}
}

// The sealed loop must not fail-close the gate's epoch validity for the
// admitted post-seal turn — and must still invalidate for ordinary turns.
func TestBug393_GateEpochValidForPostSealTurn(t *testing.T) {
	svc, _ := newTestServer(t)
	handle, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "done"})
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.flowEngineDriven = true
	rs.gateEpoch = 7
	rs.turnStartedAfterLoopDone = true
	svc.mu.Unlock()
	if !svc.gateEpochStillValid(handle.RunID, 7) {
		t.Fatal("sealed loop must not invalidate the post-seal turn's own gate epoch")
	}
	svc.mu.Lock()
	rs.turnStartedAfterLoopDone = false
	svc.mu.Unlock()
	if svc.gateEpochStillValid(handle.RunID, 7) {
		t.Fatal("ordinary turn on a sealed loop must stay invalid (Stop semantics)")
	}
}

// End-to-end at the seam: a post-seal turn whose turn produces a real code
// diff must get gate evaluation — the r-ca rule (code change without a
// change-audit note) fires a flow_gate_violation reprompt. Pre-fix the gate
// never ran for these turns so no violation could ever appear.
func TestBug393_PostSealTurnStillGateEvaluated(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if physical, err := filepath.EvalSymlinks(dir); err == nil {
		dir = physical
	}
	gitRun := func(args ...string) {
		t.Helper()
		var out []byte
		var err error
		for attempt := 0; attempt < 20; attempt++ {
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
				"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
				"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
				"HOME="+dir,
			)
			out, err = cmd.CombinedOutput()
			if err == nil {
				return
			}
			if !strings.Contains(string(out), "index.lock") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	gitRun("init")
	gitRun("config", "user.email", "t@e")
	gitRun("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "calc.go"), []byte("package calcapp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "-A")
	gitRun("commit", "-m", "seed")

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				// Write a real source file (no change-audit note → r-ca fires
				// under the post-turn gate) then complete.
				werr := os.WriteFile(filepath.Join(dir, "reverse.go"), []byte("package calcapp\n\nfunc Reverse(s string) string { return s }\n"), 0o644)
				if werr != nil {
					return werr
				}
				b.Emit(ProviderEvent{Type: EventFileChanged, Path: "reverse.go", ChangeType: "created"})
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "implemented Reverse"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", Client: "tui", Cwd: dir,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("createRun status=%d body=%s", status, body)
	}
	var handle RunHandle
	if err := json.Unmarshal(body, &handle); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "done"})
	svc.mu.Lock()
	rs := svc.runs[handle.RunID]
	rs.flowEngineDriven = true
	rs.turnCount = 1 // follow-up, not the flow-launching first turn
	svc.mu.Unlock()

	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": "tdd",
		"prompt": "continue",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("post-seal follow-up turn should be admitted (BUG-302/308), got %d %s", status, body)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[handle.RunID]
		sawViolation := false
		var types []string
		if rs != nil {
			for _, ev := range rs.events {
				types = append(types, string(ev.Type))
				if ev.Type == EventFlowGateViolation {
					sawViolation = true
				}
			}
		}
		svc.mu.Unlock()
		if sawViolation {
			return // gate evaluated the post-seal turn — fix verified
		}
		select {
		case <-time.After(20 * time.Millisecond):
		}
		_ = types
	}
	svc.mu.Lock()
	rs = svc.runs[handle.RunID]
	var types []string
	st := ""
	if rs != nil {
		st = string(rs.status)
		for _, ev := range rs.events {
			types = append(types, string(ev.Type))
		}
	}
	svc.mu.Unlock()
	t.Fatalf("post-seal turn ran with NO gate evaluation — status=%s events=%v", st, types)
}

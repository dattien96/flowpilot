package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Task-452 (CP-89): the forwardFlow turn seam — an explicit, deterministic
// signal that flips a pending flowArm latch to started and launches the
// pinned flow. No model inference, no prompt-length heuristics; the latch is
// the contract.

func waitTurnIdle(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		idle := !svc.runs[runID].turnInFlight
		svc.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("turn on %s never went idle", runID)
}

func TestTask452_BareForwardPassesAdmission(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// A forward body with no prompt must still pass BUG-509's empty-body
	// admission (the forward is the actionable content). stepId is required
	// for ANY turn — the bare-forward case means "no prompt".
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/"+h.RunID+"/turns", strings.NewReader(`{"forwardFlow":true,"stepId":"chat-1"}`))
	r.SetPathValue("runId", h.RunID)
	svc.handleStartTurn(w, r)
	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "prompt is required") {
		t.Fatalf("bare forwardFlow must pass admission, got %d %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("forward turn must succeed, got %d %s", w.Code, w.Body.String())
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("forward must flip pending→started, arm=%q", arm)
	}
}

func TestTask452_ForwardStartsFlowFromPin(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// A couple of plain chat turns first — the pending run stays chat. Wait
	// for each turn's runTurn goroutine to release turnInFlight naturally —
	// force-clearing while the goroutine is alive races its unlocked reads.
	for i := 0; i < 2; i++ {
		if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", Prompt: "hello"}, "", ""); e != nil {
			t.Fatalf("chat turn %d: %v", i, e)
		}
		waitTurnIdle(t, svc, h.RunID)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "chốt"}, "", ""); e != nil {
		t.Fatalf("forward turn: %v", e)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	arm, driven := rs.flowArm, rs.flowEngineDriven
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("arm = %q, want started", arm)
	}
	if !driven {
		t.Fatal("forward must arm flowEngineDriven via the existing start path")
	}
}

func TestTask452_ForwardWithoutPinIs422(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "forward_requires_flow_pin" {
		t.Fatalf("forward with no pin must 422 forward_requires_flow_pin, got %v", e)
	}
}

func TestTask452_ForwardOnStartedIs422(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("first forward: %v", e)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "flow_already_started" {
		t.Fatalf("second forward must 422 flow_already_started, got %v", e)
	}
}

func TestTask452_ForwardOnImmediateRunIs422(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", // immediate default — flow starts on turn 1 anyway
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "flow_already_started" {
		t.Fatalf("forward on immediate arm must 422 flow_already_started, got %v", e)
	}
}

// Corrupt durable row: flow_arm=pending on a CHILD run. Clients cannot mint
// this (StartRunInput has no ParentRunID), but a bad row that survives
// reconstruct must not let forwardFlow launch a nested flow from a child.
func TestTask452_ForwardOnPendingChildRunIs422(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[h.RunID].parentRunID = "run-parent"
	svc.mu.Unlock()
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "forward_requires_root_run" {
		t.Fatalf("forward on pending child must 422 forward_requires_root_run, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("rejected forward must leave latch pending, got %q", arm)
	}
}

// A failed fence never consumes the pending latch — fix the issue, retry.
func TestTask452_FailedFenceKeepsPending(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// vibe-cp-ingest without a CP-shaped source fails the forward fence.
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "go"}, "", "")
	if e == nil || e.code != "invalid_cp_source" {
		t.Fatalf("missing CP source must fail invalid_cp_source, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("failed fence must keep pending, arm=%q", arm)
	}
}

// A pin whose stored definition is corrupt fails closed — forward must not
// fall back to a chat turn, and the latch stays pending.
func TestTask452_CorruptDefinitionFailsClosed(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "definitely-not-a-real-flow", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	_, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", "")
	if e == nil || e.status != http.StatusUnprocessableEntity || e.code != "invalid_flow_definition" {
		t.Fatalf("corrupt pin must 422 invalid_flow_definition, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("failed fence must keep pending, arm=%q", arm)
	}
}

// A pending run whose pin came from workflowID (not chatFlowRef) resolves at
// forward time — the workflowID mount is the second resolution source.
func TestTask452_WorkflowIDMountedPinResolves(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkflowID:  "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		Model:   "gpt-5.4-mini",
		FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	arm, ref := rs.flowArm, rs.chatFlowRef
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("workflowID-mounted pending create, arm=%q", arm)
	}
	if ref != "" {
		// workflowID pin lives on workflowID, not chatFlowRef — the resolver
		// must find it without a stamped ref.
		t.Fatalf("workflowID pin unexpectedly stamped chatFlowRef=%q", ref)
	}
	resolved, e := resolvePinnedFlowRefForForward(rs)
	if e != nil {
		t.Fatalf("workflowID pin must resolve, got %v", e)
	}
	if resolved == "" {
		t.Fatal("resolved pin must be non-empty")
	}
}

// The turnCount!=0 guard on resolveWorkflowFlowRef is untouched: a pending
// run mid-chat still resolves false (BUG-315 class protection).
func TestTask452_TurnCountGuardUnchanged(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", Prompt: "turn one"}, "", ""); e != nil {
		t.Fatalf("chat turn: %v", e)
	}
	if _, ok := svc.resolveWorkflowFlowRef(context.Background(), h.RunID); ok {
		t.Fatal("resolveWorkflowFlowRef must stay false once turnCount>0")
	}
}

// The create-pinned SourceDocID counts at forward time — no repaste needed.
func TestTask452_PinnedSourceDocIDSatisfiesCpIngest(t *testing.T) {
	ws := t.TempDir()
	cpDir := filepath.Join(ws, "requirements", "07-Coding-Plan")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cpPath := filepath.Join(cpDir, "CP-89-test.md")
	if err := os.WriteFile(cpPath, []byte("# CP\nDocument ID: CP-89\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "pending", Cwd: ws,
		SourceDocID: "requirements/07-Coding-Plan/CP-89-test.md",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Forward carries no SourceDocID — the create pin must satisfy the fence.
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("forward with pinned source must pass, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("arm = %q, want started", arm)
	}
}

// A pending vibe-cp-ingest run's first CHAT turn is a plain chat turn — the
// BUG-399 ingest fence only applies when the flow actually starts (immediate
// first turn or explicit forward). Without this, pending chat was dead on
// turn 1 until a CP source was pasted — exactly what the latch exists to defer.
func TestTask452_PendingVibeChatTurnNotIngestFenced(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// turnCount==0 + adopted flowRef = the BUG-399 fence's trigger shape, but
	// flowArm=pending means this turn is chat, not a flow start — no source
	// required.
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", Prompt: "plain chat"}, "", ""); e != nil {
		t.Fatalf("pending vibe-cp-ingest chat turn must not be ingest-fenced: %v", e)
	}
	waitTurnIdle(t, svc, h.RunID)
	svc.mu.Lock()
	arm := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("arm = %q, want still pending", arm)
	}
	// The bare forward is still fenced — only the CHAT leg is exempt.
	if _, e := svc.startTurn(h.RunID, TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e == nil || e.code != "invalid_cp_source" {
		t.Fatalf("forward without source must still fail invalid_cp_source, got %v", e)
	}
}

// A turn-level SourceDocID on the forward IS this run's launch metadata — it
// must be adopted onto rs.sourceDocID (like the immediate first-turn block
// does) so vibeLockedCP stamps the validated value, not the stale pin.
func TestTask452_ForwardTurnSourceDocIDAdopted(t *testing.T) {
	ws := t.TempDir()
	cpDir := filepath.Join(ws, "requirements", "07-Coding-Plan")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cpPath := filepath.Join(cpDir, "CP-89-turn.md")
	if err := os.WriteFile(cpPath, []byte("# CP\nDocument ID: CP-89\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "pending", Cwd: ws,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID, TurnInput{
		StepID: "chat", ForwardFlow: true,
		ChangeType: "coding-plan", SourceDocID: "requirements/07-Coding-Plan/CP-89-turn.md",
	}, "", ""); e != nil {
		t.Fatalf("forward with turn-level source must pass, got %v", e)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	src, lockedCP := rs.sourceDocID, rs.vibeLockedCP
	svc.mu.Unlock()
	if src != "requirements/07-Coding-Plan/CP-89-turn.md" {
		t.Fatalf("sourceDocID not adopted, got %q", src)
	}
	if lockedCP != src {
		t.Fatalf("vibeLockedCP = %q, want adopted source %q", lockedCP, src)
	}
}

// ---- prepared-relaunch idempotency (review residual fix) --------------------
// A durably-prepared forward turn relaunched after a crash must not lose the
// forward intent: the flowArm latch is the idempotency key.

func TestTask452_PreparedRelaunchForwardOnPendingStillLaunches(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Simulate the crash window: the durable key saw prep but never the
	// launch ack — the arm is still pending, so the relaunch must run the
	// full fence+flip+launch path rather than replaying a chat turn.
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	rs.idempotency["durable-fwd-restart-1"] = durableIdemPreparedPrefix + "turn-9"
	svc.mu.Unlock()

	tid, apiErr := svc.startTurn(h.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true}, "", "durable-fwd-restart-1")
	if apiErr != nil {
		t.Fatalf("relaunched forward must not error: %v", apiErr)
	}
	if tid != "turn-9" {
		t.Fatalf("relaunch must reuse prepared turnID, got %q", tid)
	}
	svc.mu.Lock()
	rs = svc.runs[h.RunID]
	arm := rs.flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("relaunched forward must still flip pending→started, arm=%q", arm)
	}
}

func TestTask452_PreparedRelaunchForwardOnStartedCompletesNoRespawn(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Crash AFTER the flip+topology commit but before the launch ack: the
	// durable row says started. The relaunch completes the turn synthetically
	// (flowStartOnly) — it must NOT call forwardPinnedFlow again (which would
	// 422 flow_already_started) and must NOT re-spawn children.
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	rs.flowArm = FlowArmStarted
	rs.idempotency["durable-fwd-restart-2"] = durableIdemPreparedPrefix + "turn-9"
	svc.mu.Unlock()

	tid, apiErr := svc.startTurn(h.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true}, "", "durable-fwd-restart-2")
	if apiErr != nil {
		t.Fatalf("relaunched forward on committed arm must complete, not error: %v", apiErr)
	}
	if tid != "turn-9" {
		t.Fatalf("relaunch must reuse prepared turnID, got %q", tid)
	}
	// No child spawn may happen on this replay — count children after settle.
	waitTurnIdle(t, svc, h.RunID)
	svc.mu.Lock()
	children := 0
	for _, c := range svc.runs {
		if c.parentRunID == h.RunID {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 0 {
		t.Fatalf("relaunched forward must not spawn children, got %d", children)
	}
}

// Durable-first ordering: the forward path must commit arm=started (with
// flow topology) to the durable row BEFORE the entry spawn goroutine can
// create children — a crash can never orphan a launch the row forgot.
func TestTask452_ForwardPersistsStartedBeforeSpawn(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, e := svc.startTurn(h.RunID,
		TurnInput{StepID: "chat", ForwardFlow: true}, "", ""); e != nil {
		t.Fatalf("forward: %v", e)
	}
	// startTurn returns after the durable commit — the session snapshot must
	// already carry arm=started + topology, independent of goroutine timing.
	svc.mu.Lock()
	snap := sessionStateOf(svc.runs[h.RunID])
	svc.mu.Unlock()
	if snap.FlowArm != "started" {
		t.Fatalf("snapshot at startTurn return must carry started, got %q", snap.FlowArm)
	}
	if len(snap.ActiveFlowNodes) == 0 {
		t.Fatal("snapshot must carry flow topology — started without nodes is the never-launched crash state")
	}
}

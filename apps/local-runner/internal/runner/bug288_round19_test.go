package runner

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---- R19-1: Stop during durable pre-persist must not resurrect turn --------

func TestDurableStartAbortWhenStoppedDuringPersist(t *testing.T) {
	var svc *InteractiveService
	var runID string
	var cancelledDuringPersist atomic.Bool
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		// Only intercept durable pre-persist (prep: value).
		for k, v := range session.IdempotencyKeys {
			if strings.HasPrefix(k, "durable-") && strings.HasPrefix(v, durableIdemPreparedPrefix) {
				if !cancelledDuringPersist.Swap(true) {
					// Simulate Stop while s.mu is unlocked for persist.
					svc.mu.Lock()
					if rs := svc.runs[runID]; rs != nil {
						rs.status = RunStatusCancelled
						rs.agentStatus = string(RunStatusCancelled)
						rs.turnInFlight = false
						rs.currentTurnID = ""
						rs.turnCancel = nil
						clearDurableRecoveryStateLocked(rs)
					}
					svc.mu.Unlock()
				}
			}
		}
		return nil
	}
	svc, _ = newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	runID = run.RunID

	idem := "durable-" + runID + "-resume-00000000000000000001"
	tid, apiErr := svc.startTurn(runID, TurnInput{StepID: "s1", Prompt: "continue"}, "", idem)
	if apiErr == nil {
		t.Fatalf("expected abort after Stop mid-persist, got turnID=%q", tid)
	}
	if apiErr.code != "flow_stopped" && apiErr.code != "turn_aborted" {
		t.Fatalf("apiErr code=%q msg=%q, want flow_stopped or turn_aborted", apiErr.code, apiErr.msg)
	}
	if !cancelledDuringPersist.Load() {
		t.Fatal("hook never saw durable prep persist")
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	// Must not leave a live turn after abort.
	if rs.turnInFlight || rs.currentTurnID != "" {
		t.Fatalf("turn still in flight after abort: inFlight=%v current=%q", rs.turnInFlight, rs.currentTurnID)
	}
	// TurnStarted must not have been emitted for a resurrected turn.
	for _, ev := range rs.events {
		if ev.Type == EventTurnStarted {
			t.Fatal("EventTurnStarted must not emit after Stop mid-durable-persist")
		}
	}
	svc.mu.Unlock()
}

// ---- R19-2: prepared-only durable key must relaunch, not short-circuit ----

func TestDurablePreparedIdempotencyRelaunchesNotShortCircuit(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate crash after durable pre-persist: prep:turnID on disk/RAM, no
	// provider launch-ack, turn not in flight.
	const ghost = "turn-ghost"
	idem := "durable-" + run.RunID + "-restart-00000000000000000002"
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	if rs.idempotency == nil {
		rs.idempotency = map[string]string{}
	}
	rs.idempotency[idem] = durableIdemPreparedValue(ghost)
	rs.turnCount = 1
	rs.stepID = "s1"
	rs.turnInFlight = false
	rs.currentTurnID = ""
	svc.mu.Unlock()

	// First startTurn with same key must NOT short-circuit as accept — it must
	// relaunch and promote to launched form.
	tid, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "retry"}, "", idem)
	if apiErr != nil {
		t.Fatalf("startTurn relaunch: %v", apiErr)
	}
	if tid != ghost {
		t.Fatalf("turnID=%q, want reused %s", tid, ghost)
	}
	svc.mu.Lock()
	raw := svc.runs[run.RunID].idempotency[idem]
	svc.mu.Unlock()
	gotTurn, launched := parseDurableIdemValue(raw)
	if !launched || gotTurn != ghost {
		t.Fatalf("after relaunch raw=%q, want launched %s", raw, ghost)
	}

	// Second call with launched key short-circuits (turn still in flight would
	// 409 without idempotency; launched key must win first).
	tid2, apiErr2 := svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "retry"}, "", idem)
	if apiErr2 != nil {
		t.Fatalf("replay: %v", apiErr2)
	}
	if tid2 != ghost {
		t.Fatalf("replay turnID=%q", tid2)
	}
}

func TestParseDurableIdemValuePhases(t *testing.T) {
	tid, launched := parseDurableIdemValue("prep:turn-1")
	if tid != "turn-1" || launched {
		t.Fatalf("prep: got %q launched=%v", tid, launched)
	}
	tid, launched = parseDurableIdemValue("turn-1")
	if tid != "turn-1" || !launched {
		t.Fatalf("bare: got %q launched=%v", tid, launched)
	}
}

// ---- R19-3: warn/approve path uses withGateEpochDurable fail-closed --------

func TestWithGateEpochDurableBlocksOnFnError(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	epoch := svc.runs[run.RunID].gateEpoch
	svc.mu.Unlock()
	ok := svc.withGateEpochDurable(run.RunID, epoch, func() error {
		return errors.New("contract io failed")
	})
	if ok {
		t.Fatal("withGateEpochDurable must return false on fn error (fail-closed)")
	}
}

func TestWithGateEpochDurableBlocksOnEpochBump(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	epoch := svc.runs[run.RunID].gateEpoch
	svc.runs[run.RunID].gateEpoch++ // Stop bump
	svc.mu.Unlock()
	ok := svc.withGateEpochDurable(run.RunID, epoch, func() error {
		t.Fatal("fn must not run when epoch stale")
		return nil
	})
	if ok {
		t.Fatal("stale epoch must block")
	}
}

// ---- R19-4: markerSecret threaded through behavior + retry composer --------

func TestBehaviorContextRenderUsesPayloadMarkerSecret(t *testing.T) {
	secA := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secB := []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	pkg := FlowContextPackage{WorkflowRunID: "run-m", PackageID: "pkg-m"}
	out, err := behaviorContextRender(context.Background(), BehaviorInput{
		NodeID: "coding",
		Prompt: "implement",
		Payload: map[string]any{
			"package":      pkg,
			"markerSecret": secA,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.NextPromptFragments) == 0 {
		t.Fatal("empty render")
	}
	rendered := out.NextPromptFragments[0]
	macA := runMarkerMACWith(secA, "fcp", "run-m")
	macB := runMarkerMACWith(secB, "fcp", "run-m")
	if !strings.Contains(rendered, macA) {
		t.Fatalf("render must mint with secA (%s), got:\n%s", macA, rendered)
	}
	if strings.Contains(rendered, macB) {
		t.Fatal("render must not mint with secB")
	}
	// Service-scoped verify: only secA trusts the marker.
	if !isFlowContextHandoffWithSecret(secA, rendered, "run-m") {
		t.Fatal("secA must verify its own marker")
	}
	if isFlowContextHandoffWithSecret(secB, rendered, "run-m") {
		t.Fatal("secB must not verify secA marker")
	}
}

func TestComposeRetryPromptWithSecretMintsServiceMarker(t *testing.T) {
	sec := []byte("cccccccccccccccccccccccccccccccc")
	pkg := FlowContextPackage{WorkflowRunID: "run-retry", PackageID: "pkg-r"}
	out := ComposeRetryPromptWithSecret(pkg, FlowValidationRetryState{
		RetryAttempt: 1,
		MaxRetries:   3,
	}, sec)
	mac := runMarkerMACWith(sec, "fcp", "run-retry")
	if !strings.Contains(out, mac) {
		t.Fatalf("retry prompt missing service MAC %s\n%s", mac, out)
	}
	if !isFlowContextHandoffWithSecret(sec, out, "run-retry") {
		t.Fatal("service secret must verify retry marker")
	}
}

// ---- R19-5: third checkpoint success clears gate_settle_checkpoint marker --

func TestGateSettleCheckpointClearsBlockedAfterThirdSuccess(t *testing.T) {
	// Only fail settle-checkpoint upserts (PendingFlowGateSettle true), not createRun.
	var settleFailsLeft atomic.Int32
	settleFailsLeft.Store(2)
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		if session.PendingFlowGateSettle {
			if settleFailsLeft.Add(-1) >= 0 {
				return errors.New("simulated persist fail")
			}
		}
		return nil
	}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.currentTurnID = "turn-settle"
	ok := svc.markPendingFlowGateSettleLocked(rs, "done", time.Now().UTC().Format(time.RFC3339Nano))
	if !ok {
		t.Fatal("expected true when third persist succeeds")
	}
	if rs.gateCheckpointNotDurable {
		t.Fatal("gateCheckpointNotDurable must be false after success")
	}
	if rs.intentBlockedKind != "" {
		t.Fatalf("intentBlockedKind=%q, want cleared after third success", rs.intentBlockedKind)
	}
	if !rs.pendingFlowGateSettle {
		t.Fatal("pendingFlowGateSettle must remain set")
	}
	svc.mu.Unlock()
}

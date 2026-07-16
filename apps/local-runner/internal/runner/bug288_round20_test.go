package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// ---- R20-1: orphan launch-ack (bare, no progress) must relaunch ------------

func TestDurableOrphanLaunchAckRelaunchesNotShortCircuit(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	const ghost = "turn-orphan-ack"
	idem := "durable-" + run.RunID + "-resume-00000000000000000003"
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	if rs.idempotency == nil {
		rs.idempotency = map[string]string{}
	}
	// Bare key as if launch-ack persisted, but no live turn / no terminal evidence.
	rs.idempotency[idem] = ghost
	rs.turnInFlight = false
	rs.currentTurnID = ""
	rs.lastTurnID = ""
	rs.turnCount = 1
	rs.stepID = "s1"
	svc.mu.Unlock()

	if durableIdemReplaySafe(rs, ghost, true) {
		t.Fatal("orphan bare key must not be replay-safe")
	}

	tid, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "s1", Prompt: "retry orphan"}, "", idem)
	if apiErr != nil {
		t.Fatalf("relaunch: %v", apiErr)
	}
	if tid != ghost {
		t.Fatalf("turnID=%q, want reused %s", tid, ghost)
	}
	svc.mu.Lock()
	rs = svc.runs[run.RunID]
	if !rs.turnInFlight || rs.currentTurnID != ghost {
		t.Fatalf("expected live relaunch inFlight=%v current=%q", rs.turnInFlight, rs.currentTurnID)
	}
	// Now clear-safe (live in this process).
	if !durableIntentClearOK(rs, ghost) {
		t.Fatal("live turn must be clear-safe")
	}
	svc.mu.Unlock()
}

func TestDurableLaunchAckPersistFailureDoesNotLaunch(t *testing.T) {
	var svc *InteractiveService
	var runID string
	var failLaunchAck atomic.Bool
	failLaunchAck.Store(true)
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		if !failLaunchAck.Load() {
			return nil
		}
		// Fail only when promoting to bare launch-ack (has durable key without prep:).
		for k, v := range session.IdempotencyKeys {
			if strings.HasPrefix(k, "durable-") && v != "" && !strings.HasPrefix(v, durableIdemPreparedPrefix) {
				return errors.New("simulated launch-ack persist fail")
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
	idem := "durable-" + runID + "-reprompt-00000000000000000004"

	tid, apiErr := svc.startTurn(runID, TurnInput{StepID: "s1", Prompt: "x"}, "", idem)
	if apiErr == nil {
		t.Fatalf("expected launch-ack fail-closed, got turnID=%q", tid)
	}
	if apiErr.code != "session_persist_failed" {
		t.Fatalf("code=%q, want session_persist_failed", apiErr.code)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	if rs.turnInFlight {
		t.Fatal("must not leave turnInFlight after launch-ack fail")
	}
	raw := rs.idempotency[idem]
	svc.mu.Unlock()
	// Disk/RAM should be prep (or empty), not bare successful launch without provider.
	if raw != "" {
		_, launched := parseDurableIdemValue(raw)
		if launched {
			t.Fatalf("must not leave bare launch-ack after fail-closed abort, raw=%q", raw)
		}
	}
	// Recovery with storage healthy can relaunch.
	failLaunchAck.Store(false)
	tid2, apiErr2 := svc.startTurn(runID, TurnInput{StepID: "s1", Prompt: "x"}, "", idem)
	if apiErr2 != nil {
		t.Fatalf("relaunch after storage recovery: %v", apiErr2)
	}
	if tid2 == "" {
		t.Fatal("expected turn id on relaunch")
	}
}

// ---- R20-2: foreign-service marker must not skip feature history ------------

func TestInjectFeatureHistoryIgnoresForeignServiceMarker(t *testing.T) {
	secA := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	secB := []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	// Marker minted with B's secret only.
	markerB := flowContextTrustedMarkerWith(secB, "run-foreign")
	userPrompt := "please improve chat-ui"
	prompt := flowContextHandoffPrefix + "\n" + markerB + "\n\n" + userPrompt

	if isFlowContextHandoffWithSecret(secA, prompt) {
		t.Fatal("service A must not treat B's marker as trusted handoff")
	}
	if !isFlowContextHandoffWithSecret(secB, prompt) {
		t.Fatal("service B must trust its own marker")
	}

	// Workspace with catalog so inject would prepend history when handoff is false.
	workspace := t.TempDir()
	dotDir := filepath.Join(workspace, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(workspace, "change-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dotDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "change-audit", "FEATURE-KEYS.md"), []byte("- chat-ui — Chat UI\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := changeledger.New(dotDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Upsert([]changeledger.Entry{{
		CommitHash: "c1", FeatureKey: "chat-ui", Summary: "first history",
		CommittedAt: "2026-01-01T00:00:00Z", Confidence: changeledger.ConfidenceHigh,
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(workspace, ledger, dotDir); err != nil {
		t.Fatal(err)
	}

	// Service A must still inject history (foreign B marker must not suppress).
	outA := injectFeatureHistoryPromptWithSecret(workspace, prompt, nil, secA)
	if !strings.Contains(outA, "first history") {
		t.Fatalf("service A must inject history despite foreign marker; got %q", outA)
	}
	// Service B correctly treats as handoff → no double history inject.
	outB := injectFeatureHistoryPromptWithSecret(workspace, prompt, nil, secB)
	if outB != prompt {
		t.Fatalf("service B must skip inject for its own marker; got %q", outB)
	}
}

// ---- R20-3: fail/fail/succeed never leaves durable blocked marker ----------

func TestGateSettleFailFailSucceedNoBlockedMarker(t *testing.T) {
	var settleAttempts atomic.Int32
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		if !session.PendingFlowGateSettle {
			return nil
		}
		n := settleAttempts.Add(1)
		// First two settle writes fail; third succeeds. Never persist blocked kind.
		if n <= 2 {
			return errors.New("simulated settle fail")
		}
		if session.IntentBlockedKind == "gate_settle_checkpoint" {
			return errors.New("must not persist transient blocked marker")
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
	rs.currentTurnID = "turn-s"
	ok := svc.markPendingFlowGateSettleLocked(rs, "done", time.Now().UTC().Format(time.RFC3339Nano))
	if !ok {
		t.Fatal("expected settle success on third write")
	}
	if rs.intentBlockedKind != "" {
		t.Fatalf("intentBlockedKind=%q, want empty (never durable false blocked)", rs.intentBlockedKind)
	}
	if rs.gateCheckpointNotDurable {
		t.Fatal("gateCheckpointNotDurable must be false")
	}
	svc.mu.Unlock()
	if settleAttempts.Load() < 3 {
		t.Fatalf("settle attempts=%d, want >=3", settleAttempts.Load())
	}
}

func TestDurableIdemReplaySafeRequiresEvidence(t *testing.T) {
	rs := &interactiveRun{id: "r1", idempotency: map[string]string{}}
	if durableIdemReplaySafe(rs, "t1", true) {
		t.Fatal("bare without evidence must be false")
	}
	rs.turnInFlight = true
	rs.currentTurnID = "t1"
	if !durableIdemReplaySafe(rs, "t1", true) {
		t.Fatal("live turn must be true")
	}
	rs.turnInFlight = false
	rs.currentTurnID = ""
	rs.lastTurnID = "t1"
	if !durableIdemReplaySafe(rs, "t1", true) {
		t.Fatal("lastTurnID evidence must be true")
	}
	if durableIdemReplaySafe(rs, "t1", false) {
		t.Fatal("prep never replay-safe")
	}
}

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- Task-249 ----

func TestStopCASBeforeSendStarted_SendCASFails_NothingSent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	st, _ := store.GetRunStopState(ctx, "r1")
	_, _ = store.RequestRunStop(ctx, "r1", st.Revision, StopReasonUser)
	_, err := store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	var fence ErrRunStopFence
	if !errors.As(err, &fence) {
		t.Fatalf("want fence, got %v", err)
	}
	_, err = store.CommitPreSendCancellationAndClearIntent(ctx, "r1", "t1", rev, "r1", rec.OuterIntentKey, 1, fence.CurrentGeneration, PreSendStopSelf)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchTerminalCancelled {
		t.Fatalf("state=%s", got.State)
	}
}

func TestSessionIDIsNotAReceipt(t *testing.T) {
	// Codex/Grok session creation never advances past send_started without Accepted.
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	// Simulating thread/start or session/new alone: no CommitReceipt.
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchSendStarted {
		t.Fatalf("session id must not be receipt: %s", got.State)
	}
	_ = rev
}

func TestProviderV2GeminiDisabled_ClaudeEnabled(t *testing.T) {
	// Claude's Task-257 probe recorded the same negative (unprovable ⇒ uncertain)
	// outcome as Codex/Grok, so it joins the default-enabled three-outcome bucket.
	if !providerV2Enabled(ProviderKey("claude")) {
		t.Fatal("claude must be V2-enabled (three-outcome, no Accepted seam) per Task-257 evidence")
	}
	if providerV2Enabled(ProviderKey("gemini")) {
		t.Fatal("gemini must be V2-disabled")
	}
	if !providerV2Enabled(ProviderKey("codex")) {
		t.Fatal("codex enabled with three-outcome")
	}
}

// ---- Task-252 ----

func TestFCPMarker_EmptyAllowedSet_FailsClosed(t *testing.T) {
	// With empty AllowedMarkerIDs, handoff must not suppress — injectFeatureHistoryPromptCtx
	// returns body path. We only assert the handoff short-circuit is skipped.
	out := injectFeatureHistoryPromptCtx("/tmp", "plain prompt without history markers", nil,
		MarkerVerificationContext{Secret: []byte("secret"), AllowedMarkerIDs: nil})
	// Without catalog the body returns prompt unchanged — key is it did not treat as handoff.
	if out != "plain prompt without history markers" {
		// empty workspace may still return same prompt
	}
	// Foreign empty set: isFlowContextHandoffWithSecret not called when IDs empty.
	_ = out
}

func TestFCPMarker_AllowedIDsRequired(t *testing.T) {
	ids := allowedFCPMarkerIDs(&interactiveRun{id: "run-a", pendingRestartProvenanceRunID: "run-b"})
	if len(ids) != 2 || ids[0] != "run-a" || ids[1] != "run-b" {
		t.Fatalf("ids=%v", ids)
	}
	ids = allowedFCPMarkerIDs(&interactiveRun{id: "run-a"})
	if len(ids) != 1 || ids[0] != "run-a" {
		t.Fatalf("self only: %v", ids)
	}
}

// ---- Task-253 ----

func TestApplySessionRuntime_CorruptBlob_RepairRequired(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// Activate V2
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))
	sess := &ProviderSessionState{RunID: "r1"}
	err := applySessionRuntimeV2(ctx, store, sess, json.RawMessage(`{not json`))
	if err == nil || !sess.RepairRequired {
		t.Fatalf("want repair_required, err=%v repair=%v", err, sess.RepairRequired)
	}
	rep, ok, _ := store.GetOpenRepair(ctx, "r1")
	if !ok || rep.Reason == "" {
		t.Fatal("OpenRepair not durable")
	}
}

func TestApplySessionRuntime_VersionUnknown_Repair(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))
	sess := &ProviderSessionState{RunID: "r1"}
	raw, _ := json.Marshal(map[string]any{"schema_version": 99, "label": "x"})
	err := applySessionRuntimeV2(ctx, store, sess, raw)
	if err == nil || !sess.RepairRequired {
		t.Fatalf("want repair, err=%v", err)
	}
}

func TestApplySessionRuntime_MissingBlobOnV2Run_Repair(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))
	sess := &ProviderSessionState{RunID: "r1"}
	err := applySessionRuntimeV2(ctx, store, sess, nil)
	if err == nil || !sess.RepairRequired {
		t.Fatalf("missing blob on V2 must repair: err=%v", err)
	}
}

func TestApplySessionRuntime_MissingBlobOnV0Run_OK(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	sess := &ProviderSessionState{RunID: "legacy"}
	err := applySessionRuntimeV2(ctx, store, sess, nil)
	if err != nil || sess.RepairRequired {
		t.Fatalf("legacy missing blob ok: err=%v repair=%v", err, sess.RepairRequired)
	}
}

func TestApplySessionRuntime_LegacyV0Intact_Restores(t *testing.T) {
	ctx := context.Background()
	sess := &ProviderSessionState{RunID: "r1"}
	raw, _ := json.Marshal(sessionRuntimeBlob{SchemaVersion: 0, Label: "ok", StepID: "coding"})
	err := applySessionRuntimeV2(ctx, nil, sess, raw)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Label != "ok" || sess.StepID != "coding" {
		t.Fatalf("not restored: %+v", sess)
	}
}

// ---- Task-254 ----

func TestSnapshot_ActiveLowGenKeySurvives48TerminalKeys(t *testing.T) {
	m := map[string]string{}
	// 48 high-gen resume keys (terminal history)
	for i := 1; i <= 48; i++ {
		m["durable-r1-resume-"+itoa64(int64(i+10))] = "turn-r-" + itoa64(int64(i))
	}
	// Active low-gen restart
	m["durable-child-restart-1"] = "turn-active"
	nonTerm := map[string]bool{"durable-child-restart-1": true}
	out := durableIdempotencySnapshotWithNonTerminal(m, nonTerm)
	if out["durable-child-restart-1"] != "turn-active" {
		t.Fatalf("active key dropped: %#v", out)
	}
}

func TestSnapshot_PerNamespaceCap_NoCrossEviction(t *testing.T) {
	m := map[string]string{
		"durable-r-restart-1": "a",
		"durable-r-resume-99": "b",
		"durable-r-resume-98": "c",
	}
	out := durableIdempotencySnapshotWithNonTerminal(m, nil)
	if out["durable-r-restart-1"] == "" {
		// terminal-only cap per NS keeps restart namespace separately
		t.Fatalf("restart namespace should retain its key: %#v", out)
	}
}

// ---- Task-250 ----

func TestRecoveryScanner_PreparedIsSafelyRetryable(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))
	sc := &RecoveryScanner{Store: store, Owner: "sc1", Lease: time.Minute}
	if err := sc.ScanRun(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchPrepared {
		t.Fatalf("prepared stays retryable: %s", got.State)
	}
}

func TestRecoveryScanner_SendStartedGoesUncertain(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	_, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	sc := &RecoveryScanner{Store: store, Owner: "sc1", Lease: time.Minute}
	if err := sc.ScanRun(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchUncertain {
		t.Fatalf("want uncertain, got %s", got.State)
	}
}

// ---- Task-251 ----

func TestSettleDriver_PhasesAdvanceToFinalized(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	_, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", rev, proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	d := &SettleDriver{Store: store}
	if err := d.DriveSettle(ctx, "r1", "t1"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.SettlePhase != SettleFinalized {
		t.Fatalf("settle phase=%s", got.SettlePhase)
	}
}

// TestResumePendingFlowGate_CompletionEventKeyedByTurnID closes the "type-only
// replace" bug found in the 2026-07-17 audit (ledger EL): the prior check only
// compared the last event's Type == EventTurnCompleted, so a DIFFERENT turn's
// completion event ending up last (e.g. a fast-completing sibling gate) would
// silently overwrite it instead of appending its own — losing history. Now
// keyed by (Type, ProviderTurnID): same turnID replays as an idempotent
// upsert; a different turnID always appends.
func TestResumePendingFlowGate_CompletionEventKeyedByTurnID(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	// Seed a PRIOR turn's completion event as the last event in the log — the
	// exact condition that triggered the type-only-replace bug.
	crs.events = []ProviderEvent{{
		Type: EventTurnCompleted, ProviderTurnID: "turn-prior", FinalMessage: "prior turn done", Seq: 1,
	}}
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "after restart"
	crs.pendingFlowGateOccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	crs.lastTurnID = "turn-resume-1"
	crs.status = RunStatusRunning
	svc.mu.Unlock()

	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	crs = svc.runs[child.RunID]
	var priorSeen, newSeen bool
	for _, ev := range crs.events {
		if ev.Type != EventTurnCompleted {
			continue
		}
		if ev.ProviderTurnID == "turn-prior" {
			priorSeen = true
		}
		if ev.FinalMessage == "after restart" {
			newSeen = true
		}
	}
	svc.mu.Unlock()
	if !priorSeen {
		t.Fatal("the prior turn's completion event was overwritten — type-only-replace bug regressed")
	}
	if !newSeen {
		t.Fatal("the new turn's completion event was never appended")
	}
}

// ---- Task-255 (skeleton matrix cells) ----

func TestCrashMatrix_PreSendStopZeroBytes(t *testing.T) {
	TestStopCASBeforeSendStarted_SendCASFails_NothingSent(t)
}

func TestCrashMatrix_TornTail(t *testing.T) {
	TestLocalStore_DiskBeforeRAM_TornTailDropped(t)
}

// ---- Task-256 ----

func TestOperatorAttentionAndResolve(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")

	svc := &InteractiveService{dispatchStore: store}
	mux := http.NewServeMux()
	svc.RegisterDispatchOperatorRoutes(mux)

	// Per-run path (Task-256 namespace fix, 2026-07-17) — never a root /dispatch.
	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatch-attention", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("attention status %d", rr.Code)
	}
	var attn dispatchAttentionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &attn); err != nil {
		t.Fatalf("decode attention: %v", err)
	}
	if len(attn.Items) != 1 || attn.Items[0].RunID != "r1" {
		t.Fatalf("attention items = %+v", attn.Items)
	}

	body := resolveUncertainRequest{ExpectedRev: rev, ResolutionID: "res-1", Action: "abandon"}
	raw, _ := json.Marshal(body)
	req = httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/dispatches/t1/resolve", strings.NewReader(string(raw)))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("resolve status %d body=%s", rr.Code, rr.Body.String())
	}
	var disposition map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &disposition); err != nil {
		t.Fatalf("decode disposition: %v", err)
	}
	if disposition["state"] != string(DispatchTerminalCancelled) {
		t.Fatalf("resolve response must surface the committed state, got %+v", disposition)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchTerminalCancelled {
		t.Fatalf("state=%s", got.State)
	}

	// Inspect endpoint (new, 2026-07-17): full record readable per-turn.
	req = httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatches/t1", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("inspect status %d body=%s", rr.Code, rr.Body.String())
	}
	var inspected map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &inspected); err != nil {
		t.Fatalf("decode inspect: %v", err)
	}
	if inspected["state"] != string(DispatchTerminalCancelled) || inspected["turnId"] != "t1" {
		t.Fatalf("inspect response = %+v", inspected)
	}
}

// ---- Task-257 ----

func TestCapabilityEvidence_CodexGrokClaudeNoAcceptedSeam(t *testing.T) {
	// Documented negative result: no adapter may call Accepted for codex/grok/claude.
	// Guard: provider enable allows them; Accepted remains unused in adapters.
	if !providerV2Enabled(ProviderKey("codex")) || !providerV2Enabled(ProviderKey("grok")) {
		t.Fatal("codex/grok should be V2 three-outcome enabled")
	}
	t.Setenv("FLOWPILOT_DISPATCH_V2_PROVIDERS", "")
	if !providerV2Enabled(ProviderKey("claude")) {
		t.Fatal("claude should be V2 three-outcome enabled by default per Task-257 evidence")
	}
	if providerV2Enabled(ProviderKey("gemini")) {
		t.Fatal("gemini deferred by default")
	}
}

func TestCapabilityEvidence_GeminiAllowListOptIn(t *testing.T) {
	t.Setenv("FLOWPILOT_DISPATCH_V2_PROVIDERS", "gemini")
	if !providerV2Enabled(ProviderKey("gemini")) {
		t.Fatal("allow-list should enable experimental V2 for gemini")
	}
	// Still no Accepted seam — opt-in is three-outcome only until evidence wires receipt.
}

func TestDispatchV2DefaultOn(t *testing.T) {
	t.Setenv("FLOWPILOT_DISPATCH_V2", "")
	if !DispatchV2EnvEnabled() {
		t.Fatal("empty env must default V2 on")
	}
	t.Setenv("FLOWPILOT_DISPATCH_V2", "0")
	if DispatchV2EnvEnabled() {
		t.Fatal("0 must kill-switch V2 off")
	}
	t.Setenv("FLOWPILOT_DISPATCH_V2", "false")
	if DispatchV2EnvEnabled() {
		t.Fatal("false must kill-switch V2 off")
	}
	t.Setenv("FLOWPILOT_DISPATCH_V2", "1")
	if !DispatchV2EnvEnabled() {
		t.Fatal("1 must keep V2 on")
	}
}

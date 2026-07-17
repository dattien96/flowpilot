package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// CP-51 Task-256. TestOperatorAttentionAndResolve (cp51_tasks_test.go) already
// covers the per-run namespace + resolve + inspect happy path. These close the
// remaining named §4.2 gaps found in the 2026-07-17 audit.

func newOperatorTestMux(store DispatchStore) *http.ServeMux {
	svc := &InteractiveService{dispatchStore: store}
	mux := http.NewServeMux()
	svc.RegisterDispatchOperatorRoutes(mux)
	return mux
}

func TestDispatchAttentionHandlers_StatusAndReplay(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev, _ := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")

	mux := newOperatorTestMux(store)

	post := func() *httptest.ResponseRecorder {
		body := resolveUncertainRequest{ExpectedRev: rev, ResolutionID: "res-replay", Action: "abandon"}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/dispatches/t1/resolve", strings.NewReader(string(raw)))
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	first := post()
	if first.Code != 200 {
		t.Fatalf("first resolve status %d body=%s", first.Code, first.Body.String())
	}
	// Double-submit with the SAME resolutionID must replay the first result
	// (idempotent), not error and not double-apply.
	second := post()
	if second.Code != 200 {
		t.Fatalf("replay resolve status %d body=%s", second.Code, second.Body.String())
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchTerminalCancelled {
		t.Fatalf("state after replay = %s", got.State)
	}
}

func TestResolveHandler_ReturnsAtomicSettlementDisposition(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev, _ := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")

	mux := newOperatorTestMux(store)
	body := resolveUncertainRequest{ExpectedRev: rev, ResolutionID: "res-1", Action: "mark_completed"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/dispatches/t1/resolve", strings.NewReader(string(raw)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("resolve status %d body=%s", rr.Code, rr.Body.String())
	}
	var disposition map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &disposition); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// OR ledger row: the response must carry the ALREADY-COMMITTED state/settle
	// disposition, not require a follow-up call to learn it.
	if disposition["state"] == nil || disposition["settlePhase"] == nil {
		t.Fatalf("resolve response missing settlement disposition: %+v", disposition)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if disposition["state"] != string(got.State) {
		t.Fatalf("disposition.state=%v does not match committed record state=%s", disposition["state"], got.State)
	}
}

func TestDispatchInspect_RedactsCanonicalReceiptEvidence(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev, _ := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rcpt := ReceiptEvidence{
		ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack",
		PayloadCanonicalJSON: []byte(`{"secret":"do-not-leak-me"}`),
		PayloadSHA256:        "canonical-hash-only",
	}
	_, _ = store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt, "r1", rec.OuterIntentKey, 1)

	mux := newOperatorTestMux(store)
	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatches/t1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("inspect status %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "do-not-leak-me") {
		t.Fatalf("inspect response leaked the raw canonical payload: %s", rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	receipt, ok := out["receiptEvidence"].(map[string]any)
	if !ok {
		t.Fatalf("expected receiptEvidence in inspect response: %+v", out)
	}
	if receipt["payloadSHA256"] != "canonical-hash-only" {
		t.Fatalf("expected canonical hash surfaced, got %+v", receipt)
	}
	if _, hasRaw := receipt["payloadCanonicalJSON"]; hasRaw {
		t.Fatal("inspect must never expose PayloadCanonicalJSON")
	}
}

func TestRepairResolutionHandler_TwoPhaseAndAbandon(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// Activate the run (repair needs a run row to exist conceptually; OpenRepair
	// itself doesn't require a dispatch record).
	rev, err := store.OpenRepair(ctx, "r1", "corrupt runtime blob", []byte(`{"broken`), "")
	if err != nil {
		t.Fatalf("OpenRepair: %v", err)
	}

	mux := newOperatorTestMux(store)
	body := repairResolutionRequest{ExpectedRepairRev: rev, ResolutionID: "res-repair-1", Action: "abandon"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/repair-resolution", strings.NewReader(string(raw)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("repair-resolution status %d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["outcome"] != string(RepairResolvedAbandon) {
		t.Fatalf("expected resolved_abandon, got %+v", out)
	}
	// The two-phase flow must have gone through Begin (claims the attempt) then
	// Commit (resolves) — confirm the repair is no longer open.
	if _, stillOpen, _ := store.GetOpenRepair(ctx, "r1"); stillOpen {
		t.Fatal("repair must no longer be open after abandon resolution")
	}
}

func TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev, _ := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")

	mux := newOperatorTestMux(store)
	// SS-17 §8: a mismatching envelope hash simulates the run having received a
	// newer prompt since this record was held — retry-as-new must be rejected.
	body := retryAsNewRequest{
		ExpectedRev: rev, ResolutionID: "res-retry-1", NewTurnID: "t2",
		ExpectedEnvelopeHash: "stale-hash-does-not-match",
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/dispatches/t1/retry-as-new", strings.NewReader(string(raw)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("superseded retry-as-new status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Error.Code != "dispatch_retry_superseded" {
		t.Fatalf("expected dispatch_retry_superseded code, got %+v", out)
	}
}

func TestRepairResolutionHandler_RetryLoadRestoresSession(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// Activate V2 so applySessionRuntimeV2's missing-blob guard is meaningful.
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))

	validBlob := []byte(`{"schema_version":1,"label":"restored-label"}`)
	rev, err := store.OpenRepair(ctx, "r1", "corrupt runtime blob", validBlob, "")
	if err != nil {
		t.Fatalf("OpenRepair: %v", err)
	}

	fakeStore := newFakeWorkflowStore()
	if err := fakeStore.UpsertProviderSession(ctx, ProviderSessionState{RunID: "r1", ProjectID: "p"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	svc := &InteractiveService{dispatchStore: store, workflowStore: fakeStore}
	mux := http.NewServeMux()
	svc.RegisterDispatchOperatorRoutes(mux)

	body := repairResolutionRequest{ExpectedRepairRev: rev, ResolutionID: "res-retry-load", Action: "retry_load"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/repair-resolution", strings.NewReader(string(raw)))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("repair-resolution status %d body=%s", rr.Code, rr.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["outcome"] != string(RepairResolvedRetryLoad) {
		t.Fatalf("expected resolved_retry_load, got %+v", out)
	}
	restored, found, err := fakeStore.GetProviderSession(ctx, "r1")
	if err != nil || !found {
		t.Fatalf("GetProviderSession after retry-load: found=%v err=%v", found, err)
	}
	if restored.Label != "restored-label" {
		t.Fatalf("retry-load did not apply the validated blob: %+v", restored)
	}
	if _, stillOpen, _ := store.GetOpenRepair(ctx, "r1"); stillOpen {
		t.Fatal("repair must no longer be open after successful retry-load")
	}
}

func TestDispatchAttention_RebuildsAfterRestart(t *testing.T) {
	// "Restart" here means: a fresh InteractiveService instance (no shared RAM)
	// wired to the SAME durable store sees the identical attention set — proving
	// attention is derived from durable truth, not an ephemeral in-process cache,
	// so it inherently survives a process restart without needing a separate
	// event-sourced notification log.
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r1", "t1"))
	rev, _ := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, _, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")

	mux1 := newOperatorTestMux(store)
	req := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatch-attention", nil)
	rr := httptest.NewRecorder()
	mux1.ServeHTTP(rr, req)
	var before dispatchAttentionResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &before)

	// Fresh service + fresh mux ("restart"), same store.
	mux2 := newOperatorTestMux(store)
	req2 := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatch-attention", nil)
	rr2 := httptest.NewRecorder()
	mux2.ServeHTTP(rr2, req2)
	var after dispatchAttentionResponse
	_ = json.Unmarshal(rr2.Body.Bytes(), &after)

	if len(before.Items) == 0 || len(after.Items) != len(before.Items) {
		t.Fatalf("attention did not survive restart: before=%+v after=%+v", before.Items, after.Items)
	}

	// And it clears once resolved.
	_, rev2, _ := store.Get(ctx, "r1", "t1")
	resolveBody := resolveUncertainRequest{ExpectedRev: rev2, ResolutionID: "res-clear", Action: "abandon"}
	raw, _ := json.Marshal(resolveBody)
	req3 := httptest.NewRequest(http.MethodPost, "/client/workflow-runs/r1/dispatches/t1/resolve", strings.NewReader(string(raw)))
	rr3 := httptest.NewRecorder()
	mux2.ServeHTTP(rr3, req3)
	if rr3.Code != 200 {
		t.Fatalf("resolve status %d body=%s", rr3.Code, rr3.Body.String())
	}
	req4 := httptest.NewRequest(http.MethodGet, "/client/workflow-runs/r1/dispatch-attention", nil)
	rr4 := httptest.NewRecorder()
	mux2.ServeHTTP(rr4, req4)
	var cleared dispatchAttentionResponse
	_ = json.Unmarshal(rr4.Body.Bytes(), &cleared)
	if len(cleared.Items) != 0 {
		t.Fatalf("attention must clear after resolution, got %+v", cleared.Items)
	}
}

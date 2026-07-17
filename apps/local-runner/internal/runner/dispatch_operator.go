package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Operator resolution surface (SS-17 / Task-256).
//
// Routes are per-run, mounted alongside the existing /client/workflow-runs/{runId}
// handlers (CP-51 Task-256 §4.1 Step 1 — a root /dispatch namespace is explicitly
// rejected; fixed 2026-07-17 after the 2026-07-17 audit found the original
// implementation used exactly that forbidden root namespace).

type dispatchAttentionResponse struct {
	Items []AttentionItem `json:"items"`
}

type resolveUncertainRequest struct {
	ExpectedRev  int64  `json:"expectedRev"`
	ResolutionID string `json:"resolutionId"`
	Action       string `json:"action"` // mark_completed|mark_failed|confirm_cancelled|abandon
	Detail       string `json:"detail,omitempty"`
}

type retryAsNewRequest struct {
	ExpectedRev          int64  `json:"expectedRev"`
	ResolutionID         string `json:"resolutionId"`
	NewTurnID            string `json:"newTurnId"`
	ExpectedIntentGen    int64  `json:"expectedIntentGen"`
	ExpectedEnvelopeHash string `json:"expectedEnvelopeHash"`
}

type repairResolutionRequest struct {
	ExpectedRepairRev int64  `json:"expectedRepairRev"`
	ResolutionID      string `json:"resolutionId"`
	Action            string `json:"action"` // retry_load|abandon
}

// RegisterDispatchOperatorRoutes mounts SS-17 operator endpoints on mux, one
// level under each run — never a root /dispatch namespace.
func (s *InteractiveService) RegisterDispatchOperatorRoutes(mux *http.ServeMux) {
	if s == nil || mux == nil {
		return
	}
	mux.HandleFunc("GET /client/workflow-runs/{runId}/dispatch-attention", s.handleDispatchAttention)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/dispatches/{turnId}", s.handleDispatchInspect)
	mux.HandleFunc("GET /client/workflow-runs/{runId}/dispatches/{turnId}/audit", s.handleDispatchAudit)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/dispatches/{turnId}/resolve", s.handleDispatchResolve)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/dispatches/{turnId}/retry-as-new", s.handleDispatchRetryAsNew)
	mux.HandleFunc("POST /client/workflow-runs/{runId}/repair-resolution", s.handleDispatchRepairResolution)
}

// dispatchSettlementDisposition reads back the record after a mutation so the
// response surfaces the ALREADY-COMMITTED settlement outcome (OR ledger row:
// callers must never schedule a separate mutation to learn it).
func dispatchSettlementDisposition(rec DispatchRecord) map[string]any {
	return map[string]any{
		"state":       string(rec.State),
		"settlePhase": string(rec.SettlePhase),
		"stopOutcome": rec.StopOutcome,
	}
}

func (s *InteractiveService) handleDispatchAttention(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveJSON(w, http.StatusOK, dispatchAttentionResponse{Items: []AttentionItem{}})
		return
	}
	runID := r.PathValue("runId")
	all, err := s.dispatchStore.ListAttention(r.Context())
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "attention_list_failed", err.Error()))
		return
	}
	items := make([]AttentionItem, 0, len(all))
	for _, it := range all {
		if it.RunID == runID {
			items = append(items, it)
		}
	}
	writeInteractiveJSON(w, http.StatusOK, dispatchAttentionResponse{Items: items})
}

// handleDispatchInspect returns the full record (states/revision/envelope
// summary/audit-visible evidence) for one turn, plus quarantined-repair blob
// metadata when the run has an open repair. RE: only canonical identity/hash
// is exposed for receipt/terminal evidence — never a raw secret-bearing payload.
func (s *InteractiveService) handleDispatchInspect(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveError(w, newAPIErr(http.StatusServiceUnavailable, "dispatch_store_unavailable", "dispatch store unavailable"))
		return
	}
	runID, turnID := r.PathValue("runId"), r.PathValue("turnId")
	rec, rev, err := s.dispatchStore.Get(r.Context(), runID, turnID)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "dispatch_not_found", err.Error()))
		return
	}
	out := map[string]any{
		"runId":            rec.RunID,
		"turnId":           rec.TurnID,
		"state":            string(rec.State),
		"revision":         rev,
		"settlePhase":      string(rec.SettlePhase),
		"stopOutcome":      rec.StopOutcome,
		"cancelRequested":  rec.CancelRequested,
		"envelopeHash":     rec.EnvelopeHash,
		"outerIntentKey":   rec.OuterIntentKey,
		"outerIntentGen":   rec.OuterIntentGen,
		"intentOwnerRunID": rec.IntentOwnerRunID,
	}
	if rec.ReceiptEvidence != nil {
		out["receiptEvidence"] = map[string]any{
			"providerKey":   rec.ReceiptEvidence.ProviderKey,
			"receiptId":     rec.ReceiptEvidence.ReceiptID,
			"evidenceKind":  rec.ReceiptEvidence.EvidenceKind,
			"payloadSHA256": rec.ReceiptEvidence.PayloadSHA256, // canonical hash only — never PayloadCanonicalJSON
		}
	}
	if rec.TerminalEvidence != nil {
		out["terminalEvidence"] = map[string]any{
			"providerKey":   rec.TerminalEvidence.ProviderKey,
			"evidenceKind":  rec.TerminalEvidence.EvidenceKind,
			"outcome":       rec.TerminalEvidence.Outcome,
			"payloadSHA256": rec.TerminalEvidence.PayloadSHA256,
		}
	}
	if repair, ok, _ := s.dispatchStore.GetOpenRepair(r.Context(), runID); ok {
		out["openRepair"] = map[string]any{
			"repairRevision": repair.RepairRevision,
			"reason":         repair.Reason,
			"quarantineHash": repair.QuarantineHash,
			"state":          repair.State,
			"createdAt":      repair.CreatedAt,
			// QuarantineBlob (raw) intentionally omitted — inspect surfaces metadata,
			// not the raw quarantined payload, which may carry sensitive content.
		}
	}
	writeInteractiveJSON(w, http.StatusOK, out)
}

func (s *InteractiveService) handleDispatchAudit(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveJSON(w, http.StatusOK, map[string]any{"entries": []AuditEntry{}})
		return
	}
	runID, turnID := r.PathValue("runId"), r.PathValue("turnId")
	entries, err := s.dispatchStore.ListAudit(r.Context(), runID, turnID)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "audit_list_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *InteractiveService) handleDispatchResolve(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveError(w, newAPIErr(http.StatusServiceUnavailable, "dispatch_store_unavailable", "dispatch store unavailable"))
		return
	}
	runID, turnID := r.PathValue("runId"), r.PathValue("turnId")
	var req resolveUncertainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_body", err.Error()))
		return
	}
	action := ResolveAction(strings.TrimSpace(req.Action))
	_, err := s.dispatchStore.ResolveUncertain(r.Context(), runID, turnID, req.ExpectedRev,
		req.ResolutionID, action, OperatorEvidence{
			Actor: "local-operator", Detail: req.Detail,
			CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	if err != nil {
		writeInteractiveError(w, dispatchOperatorErr(err))
		return
	}
	rec, _, gerr := s.dispatchStore.Get(r.Context(), runID, turnID)
	if gerr != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "post_resolve_read_failed", gerr.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, dispatchSettlementDisposition(rec))
}

func (s *InteractiveService) handleDispatchRetryAsNew(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveError(w, newAPIErr(http.StatusServiceUnavailable, "dispatch_store_unavailable", "dispatch store unavailable"))
		return
	}
	runID, oldTurnID := r.PathValue("runId"), r.PathValue("turnId")
	var req retryAsNewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_body", err.Error()))
		return
	}
	if req.NewTurnID == "" {
		req.NewTurnID = fmt.Sprintf("turn-retry-%d", time.Now().UnixNano())
	}
	_, err := s.dispatchStore.RetryAsNew(r.Context(), runID, oldTurnID, req.ExpectedRev,
		req.ResolutionID, req.NewTurnID, req.ExpectedIntentGen, req.ExpectedEnvelopeHash)
	if err != nil {
		if errors.Is(err, ErrSuperseded) {
			// RS ledger row: the card must offer abandon, not another retry —
			// surface the specific reason instead of a generic 409.
			writeInteractiveError(w, newAPIErr(http.StatusConflict, "dispatch_retry_superseded",
				"run received a newer prompt since this record was held; retry-as-new is no longer valid — abandon instead"))
			return
		}
		writeInteractiveError(w, dispatchOperatorErr(err))
		return
	}
	oldRec, _, gerr := s.dispatchStore.Get(r.Context(), runID, oldTurnID)
	if gerr != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadGateway, "post_retry_read_failed", gerr.Error()))
		return
	}
	out := dispatchSettlementDisposition(oldRec)
	out["newTurnId"] = req.NewTurnID
	writeInteractiveJSON(w, http.StatusOK, out)
}

// handleDispatchRepairResolution is the single defined two-phase store flow
// (SD-24 §6.7): BeginRepairResolution (CAS-claim + audit + quarantined raw) →
// for retry_load, apply the raw against this run's persisted session
// (server-side, never in the store transaction) → CommitRepairResolution.
func (s *InteractiveService) handleDispatchRepairResolution(w http.ResponseWriter, r *http.Request) {
	if s.dispatchStore == nil {
		writeInteractiveError(w, newAPIErr(http.StatusServiceUnavailable, "dispatch_store_unavailable", "dispatch store unavailable"))
		return
	}
	runID := r.PathValue("runId")
	var req repairResolutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_body", err.Error()))
		return
	}
	action := RepairActionAbandon
	if strings.TrimSpace(req.Action) == string(RepairActionRetryLoad) {
		action = RepairActionRetryLoad
	}
	attemptRev, raw, err := s.dispatchStore.BeginRepairResolution(r.Context(), runID, req.ExpectedRepairRev, req.ResolutionID, action)
	if err != nil {
		writeInteractiveError(w, dispatchOperatorErr(err))
		return
	}
	outcome := RepairResolvedAbandon
	detail := "operator abandon"
	if action == RepairActionRetryLoad {
		sess, found, serr := s.getPersistedSessionForRepair(r.Context(), runID)
		if serr != nil || !found {
			outcome, detail = RepairFailedStillOpen, "session not found for retry-load"
		} else if verr := applySessionRuntimeV2(r.Context(), s.dispatchStore, &sess, raw); verr != nil {
			outcome, detail = RepairFailedStillOpen, "retry-load re-validation failed: "+verr.Error()
		} else {
			// applySessionRuntimeV2 succeeded against the quarantined raw — persist
			// the recovered session so the run is genuinely usable again.
			if perr := s.persistProviderSession(sess); perr != nil {
				outcome, detail = RepairFailedStillOpen, "recovered session persist failed: "+perr.Error()
			} else {
				outcome, detail = RepairResolvedRetryLoad, "operator retry-load"
			}
		}
	}
	rev, cerr := s.dispatchStore.CommitRepairResolution(r.Context(), runID, attemptRev, req.ResolutionID, outcome, detail)
	if cerr != nil {
		writeInteractiveError(w, dispatchOperatorErr(cerr))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, map[string]any{"revision": rev, "outcome": string(outcome), "detail": detail})
}

// getPersistedSessionForRepair loads runID's session from whichever store
// backs this service, for the retry-load re-validation step.
func (s *InteractiveService) getPersistedSessionForRepair(ctx context.Context, runID string) (ProviderSessionState, bool, error) {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return ProviderSessionState{}, false, nil
	}
	return reader.GetProviderSession(ctx, runID)
}

// dispatchOperatorErr maps a store error to an HTTP status without leaking
// internal error strings as the sole signal — 404 for not-found, 409 for any
// CAS/state conflict (stale revision, illegal transition, wrong repair state),
// 502 for anything else.
func dispatchOperatorErr(err error) *apiErr {
	switch {
	case errors.Is(err, ErrNotFound):
		return newAPIErr(http.StatusNotFound, "dispatch_not_found", err.Error())
	case errors.Is(err, ErrStaleDispatch), errors.Is(err, ErrIllegalTransition), errors.Is(err, ErrReceiptConflict), errors.Is(err, ErrEffectConflict):
		return newAPIErr(http.StatusConflict, "dispatch_conflict", err.Error())
	default:
		return newAPIErr(http.StatusBadGateway, "dispatch_operator_error", err.Error())
	}
}

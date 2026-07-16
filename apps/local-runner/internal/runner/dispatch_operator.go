package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Operator resolution surface (SS-17 / Task-256).
// Routes are registered on InteractiveService under /client/dispatch/...

type dispatchAttentionResponse struct {
	Items []AttentionItem `json:"items"`
}

type resolveUncertainRequest struct {
	RunID        string `json:"runId"`
	TurnID       string `json:"turnId"`
	ExpectedRev  int64  `json:"expectedRev"`
	ResolutionID string `json:"resolutionId"`
	Action       string `json:"action"` // mark_completed|mark_failed|confirm_cancelled|abandon
	Detail       string `json:"detail,omitempty"`
}

type retryAsNewRequest struct {
	RunID                 string `json:"runId"`
	OldTurnID             string `json:"oldTurnId"`
	ExpectedRev           int64  `json:"expectedRev"`
	ResolutionID          string `json:"resolutionId"`
	NewTurnID             string `json:"newTurnId"`
	ExpectedIntentGen     int64  `json:"expectedIntentGen"`
	ExpectedEnvelopeHash  string `json:"expectedEnvelopeHash"`
}

type repairResolveRequest struct {
	RunID            string `json:"runId"`
	ExpectedRepairRev int64 `json:"expectedRepairRev"`
	ResolutionID     string `json:"resolutionId"`
	Action           string `json:"action"` // retry_load|abandon
	// For abandon: Begin then Commit resolved_abandon without loader.
}

// RegisterDispatchOperatorRoutes mounts SS-17 operator endpoints on mux.
func (s *InteractiveService) RegisterDispatchOperatorRoutes(mux *http.ServeMux) {
	if s == nil || mux == nil {
		return
	}
	mux.HandleFunc("GET /client/dispatch/attention", s.handleDispatchAttention)
	mux.HandleFunc("POST /client/dispatch/resolve", s.handleDispatchResolve)
	mux.HandleFunc("POST /client/dispatch/retry-as-new", s.handleDispatchRetryAsNew)
	mux.HandleFunc("POST /client/dispatch/repair/begin", s.handleDispatchRepairBegin)
	mux.HandleFunc("POST /client/dispatch/repair/commit", s.handleDispatchRepairCommit)
	mux.HandleFunc("GET /client/dispatch/audit", s.handleDispatchAudit)
}

func (s *InteractiveService) handleDispatchAttention(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	items, err := s.dispatchStore.ListAttention(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// Serialize with both snake_case and camelCase-friendly fields for desktop.
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"kind": it.Kind, "run_id": it.RunID, "runId": it.RunID,
			"turn_id": it.TurnID, "turnId": it.TurnID,
			"reason": it.Reason, "updated_at": it.UpdatedAt, "updatedAt": it.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *InteractiveService) handleDispatchResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		http.Error(w, "dispatch store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req resolveUncertainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := ResolveAction(strings.TrimSpace(req.Action))
	rev, err := s.dispatchStore.ResolveUncertain(r.Context(), req.RunID, req.TurnID, req.ExpectedRev,
		req.ResolutionID, action, OperatorEvidence{
			Actor: "local-operator", Detail: req.Detail,
			CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": rev})
}

func (s *InteractiveService) handleDispatchRetryAsNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		http.Error(w, "dispatch store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req retryAsNewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.NewTurnID == "" {
		req.NewTurnID = fmt.Sprintf("turn-retry-%d", time.Now().UnixNano())
	}
	rev, err := s.dispatchStore.RetryAsNew(r.Context(), req.RunID, req.OldTurnID, req.ExpectedRev,
		req.ResolutionID, req.NewTurnID, req.ExpectedIntentGen, req.ExpectedEnvelopeHash)
	if err != nil {
		status := http.StatusConflict
		if err == ErrSuperseded {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": rev, "newTurnId": req.NewTurnID})
}

func (s *InteractiveService) handleDispatchRepairBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		http.Error(w, "dispatch store unavailable", http.StatusServiceUnavailable)
		return
	}
	var req repairResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action := RepairActionRetryLoad
	if req.Action == string(RepairActionAbandon) {
		action = RepairActionAbandon
	}
	rev, raw, err := s.dispatchStore.BeginRepairResolution(r.Context(), req.RunID, req.ExpectedRepairRev, req.ResolutionID, action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attemptRev": rev,
		"raw":        string(raw),
	})
}

func (s *InteractiveService) handleDispatchRepairCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		http.Error(w, "dispatch store unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		RunID        string `json:"runId"`
		AttemptRev   int64  `json:"attemptRev"`
		ResolutionID string `json:"resolutionId"`
		Outcome      string `json:"outcome"`
		Detail       string `json:"detail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rev, err := s.dispatchStore.CommitRepairResolution(r.Context(), body.RunID, body.AttemptRev,
		body.ResolutionID, RepairOutcome(body.Outcome), body.Detail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": rev})
}

func (s *InteractiveService) handleDispatchAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dispatchStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"entries": []AuditEntry{}})
		return
	}
	runID := r.URL.Query().Get("runId")
	turnID := r.URL.Query().Get("turnId")
	entries, err := s.dispatchStore.ListAudit(r.Context(), runID, turnID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

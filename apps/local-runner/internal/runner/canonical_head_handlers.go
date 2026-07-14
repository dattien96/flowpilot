package runner

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// toCanonicalHeadResponse maps a loaded/updated CanonicalHead to the wire shape.
func toCanonicalHeadResponse(featureKey string, head changecontract.CanonicalHead, found bool) canonicalHeadResponse {
	return canonicalHeadResponse{
		FeatureKey:        featureKey,
		BehaviorStatement: head.BehaviorStatement,
		IntentSignature:   head.IntentSignature,
		Status:            head.Status,
		SpecConfidence:    head.SpecConfidence,
		Decisions:         head.Decisions,
		Found:             found,
	}
}

// canonicalHeadResponse is the wire shape for the admin Canonical Head panel
// (Task-188, CP-43 P-5): behavior, signature-status, and the rejected/
// reverted decisions list ("do NOT re-attempt").
type canonicalHeadResponse struct {
	FeatureKey        string                    `json:"feature_key"`
	BehaviorStatement string                    `json:"behavior_statement"`
	IntentSignature   string                    `json:"intent_signature"`
	Status            string                    `json:"status"`
	SpecConfidence    string                    `json:"spec_confidence"`
	Decisions         []changecontract.Decision `json:"decisions"`
	Found             bool                      `json:"found"`
}

// handleGetCanonicalHead handles
// GET /client/projects/{projectId}/features/{featureKey}/canonical-head?workingDirectory=...
// Mirrors handleGetEngineGateConfig's workingDirectory-query-param resolution
// (the {projectId} path segment is kept for REST-route consistency with the
// other /client/projects/{projectId}/engine/* endpoints but is not itself
// used to resolve the workspace).
func (s *InteractiveService) handleGetCanonicalHead(w http.ResponseWriter, r *http.Request) {
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(r.URL.Query().Get("workingDirectory"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	featureKey := r.PathValue("featureKey")
	if featureKey == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "feature_key_required", "featureKey is required"))
		return
	}

	head, found, err := changecontract.LoadHead(workingDirectory, featureKey)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "canonical_head_load_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, canonicalHeadResponse{
		FeatureKey:        featureKey,
		BehaviorStatement: head.BehaviorStatement,
		IntentSignature:   head.IntentSignature,
		Status:            head.Status,
		SpecConfidence:    head.SpecConfidence,
		Decisions:         head.Decisions,
		Found:             found,
	})
}

// contractResponse is the wire shape for a single step's Change Contract
// (Task-184), used by the admin panel to render in/out-of-scope highlighting
// alongside the Canonical Head (Task-188 T-5).
type contractResponse struct {
	RunID           string   `json:"run_id"`
	StepID          string   `json:"step_id"`
	FeatureKey      string   `json:"feature_key"`
	Intent          string   `json:"intent"`
	DeclaredPaths   []string `json:"declared_paths"`
	DeclaredSymbols []string `json:"declared_symbols"`
	Confidence      string   `json:"confidence"`
	Found           bool     `json:"found"`
}

// handleGetStepContract handles
// GET /client/workflow-runs/{runId}/steps/{stepId}/contract
func (s *InteractiveService) handleGetStepContract(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runId")
	stepID := r.PathValue("stepId")

	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found"))
		return
	}
	if rs.workspaceCwd == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "workspace_unavailable", "run has no working directory"))
		return
	}

	store, err := changecontract.NewStore(rs.workspaceCwd)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "contract_store_unavailable", err.Error()))
		return
	}
	c, found := store.Get(runID, stepID)
	writeInteractiveJSON(w, http.StatusOK, contractResponse{
		RunID:           runID,
		StepID:          stepID,
		FeatureKey:      c.FeatureKey,
		Intent:          c.Intent,
		DeclaredPaths:   c.DeclaredPaths,
		DeclaredSymbols: c.DeclaredSymbols,
		Confidence:      c.Confidence,
		Found:           found,
	})
}

type rebaselineCanonicalHeadRequest struct {
	WorkingDirectory string `json:"workingDirectory"`
}

// handleRebaselineCanonicalHead handles
// POST /client/projects/{projectId}/features/{featureKey}/canonical-head/rebaseline
// It re-baselines a spec_less Head to `current` using the feature's governing
// docs from the catalog (Task-186 r-attach-spec: this is the human-confirmation
// caller for RebaselineWithSpec, invoked when a user explicitly confirms the
// newly attached spec matches current behavior — never automatic, BR-2).
func (s *InteractiveService) handleRebaselineCanonicalHead(w http.ResponseWriter, r *http.Request) {
	var req rebaselineCanonicalHeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(req.WorkingDirectory)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	featureKey := r.PathValue("featureKey")
	if featureKey == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "feature_key_required", "featureKey is required"))
		return
	}

	head, found, err := changecontract.LoadHead(workingDirectory, featureKey)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "canonical_head_load_failed", err.Error()))
		return
	}
	if !found {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "canonical_head_not_found", "no Canonical Head to rebaseline for this feature"))
		return
	}

	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	catalog, _ := featurecatalog.LoadCatalog(dotFP)
	var docIDs []string
	if catalog != nil {
		if feat, ok := catalog.Get(featureKey); ok {
			docIDs = feat.DocRefs
		}
	}
	if len(docIDs) == 0 {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "no_governing_docs", "no governing docs are registered for this feature — nothing to attach"))
		return
	}

	updated := changecontract.RebaselineWithSpec(workingDirectory, head, docIDs)
	if err := changecontract.SaveHead(workingDirectory, updated); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "canonical_head_save_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, toCanonicalHeadResponse(featureKey, updated, true))
}

type retireCanonicalHeadRequest struct {
	WorkingDirectory string   `json:"workingDirectory"`
	Action           string   `json:"action"`
	Targets          []string `json:"targets"`
}

// handleRetireCanonicalHead handles
// POST /client/projects/{projectId}/features/{featureKey}/canonical-head/retire
// It retires a feature's Head (renamed/merged/deprecated). For renamed/merged
// it copies the retiring Head's Decisions into each target Head (minting a
// target Head via BuildHead when one does not exist yet) so negative knowledge
// is preserved (Task-187). The human confirms by taking this explicit action —
// the retire never happens automatically (BR-2). Retired Heads are kept on
// disk for provenance, not deleted.
func (s *InteractiveService) handleRetireCanonicalHead(w http.ResponseWriter, r *http.Request) {
	var req retireCanonicalHeadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(req.WorkingDirectory)
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	featureKey := r.PathValue("featureKey")
	if featureKey == "" {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "feature_key_required", "featureKey is required"))
		return
	}
	switch req.Action {
	case changecontract.RetireActionRenamed, changecontract.RetireActionMerged, changecontract.RetireActionDeprecated:
	default:
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_retire_action", "action must be one of renamed, merged, deprecated"))
		return
	}
	if (req.Action == changecontract.RetireActionRenamed || req.Action == changecontract.RetireActionMerged) && len(req.Targets) == 0 {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "targets_required", "rename/merge requires at least one target feature key"))
		return
	}

	head, found, err := changecontract.LoadHead(workingDirectory, featureKey)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "canonical_head_load_failed", err.Error()))
		return
	}
	if !found {
		writeInteractiveError(w, newAPIErr(http.StatusNotFound, "canonical_head_not_found", "no Canonical Head to retire for this feature"))
		return
	}

	dotFP := filepath.Join(workingDirectory, ".flowpilot")
	catalog, _ := featurecatalog.LoadCatalog(dotFP)
	ledger, _ := changeledger.New(dotFP)

	targetHeads := make([]changecontract.CanonicalHead, 0, len(req.Targets))
	for _, target := range req.Targets {
		if target == "" {
			continue
		}
		th, ok, loadErr := changecontract.LoadHead(workingDirectory, target)
		if loadErr != nil {
			writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "target_head_load_failed", loadErr.Error()))
			return
		}
		if !ok {
			// Mint a fresh Head for a not-yet-seen successor so the retiring
			// feature's decisions have a home (Task-187 rename/merge).
			th = changecontract.BuildHead(workingDirectory, target, ledger, catalog, nil)
		}
		targetHeads = append(targetHeads, th)
	}

	retired, updatedTargets := changecontract.RetireHead(head, req.Action, targetHeads)
	if err := changecontract.SaveHead(workingDirectory, retired); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "canonical_head_save_failed", err.Error()))
		return
	}
	for _, target := range updatedTargets {
		if err := changecontract.SaveHead(workingDirectory, target); err != nil {
			writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "target_head_save_failed", err.Error()))
			return
		}
	}
	writeInteractiveJSON(w, http.StatusOK, toCanonicalHeadResponse(featureKey, retired, true))
}

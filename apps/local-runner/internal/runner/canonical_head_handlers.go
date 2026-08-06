package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
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
	// ScopeDiff fields (Task-188 T-5): live git diff vs declared scope.
	TouchedPaths    []string `json:"touched_paths,omitempty"`
	InScopePaths    []string `json:"in_scope_paths,omitempty"`
	OutOfScopePaths []string `json:"out_of_scope_paths,omitempty"`
	ScopeDiffNote   string   `json:"scope_diff_note,omitempty"`
}

type featureListResponse struct {
	FeatureKeys []string `json:"feature_keys"`
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
	resp := contractResponse{
		RunID:           runID,
		StepID:          stepID,
		FeatureKey:      c.FeatureKey,
		Intent:          c.Intent,
		DeclaredPaths:   c.DeclaredPaths,
		DeclaredSymbols: c.DeclaredSymbols,
		Confidence:      c.Confidence,
		Found:           found,
	}
	if found && len(c.DeclaredPaths) > 0 {
		if diff, err := flowgate.ObserveGitDiff(rs.workspaceCwd); err == nil {
			touched, inScope, outScope := computeContractScopeView(c, diff)
			resp.TouchedPaths = touched
			resp.InScopePaths = inScope
			resp.OutOfScopePaths = outScope
			resp.ScopeDiffNote = "Computed from current git diff in the run workspace (live snapshot)."
		} else {
			resp.ScopeDiffNote = "Git diff unavailable: " + err.Error()
		}
	}
	writeInteractiveJSON(w, http.StatusOK, resp)
}

func computeContractScopeView(c changecontract.Contract, diff []flowgate.ChangedFile) (touched, inScope, outOfScope []string) {
	outOfScope, _ = changecontract.ScopeDiff(c, diff, nil)
	outSet := make(map[string]struct{}, len(outOfScope))
	for _, p := range outOfScope {
		outSet[p] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, f := range diff {
		p := strings.TrimSpace(f.Path)
		if p == "" || flowgate.IsDocOrAuditFile(p) {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		touched = append(touched, p)
		if _, oos := outSet[p]; oos {
			continue
		}
		inScope = append(inScope, p)
	}
	sort.Strings(touched)
	sort.Strings(inScope)
	sort.Strings(outOfScope)
	return touched, inScope, outOfScope
}

// handleListProjectFeatures handles
// GET /client/projects/{projectId}/features?workingDirectory=...
func (s *InteractiveService) handleListProjectFeatures(w http.ResponseWriter, r *http.Request) {
	workingDirectory, apiErr := s.resolveEngineWorkingDirectory(r.URL.Query().Get("workingDirectory"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	keys, err := listFeatureKeysForWorkspace(workingDirectory)
	if err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "feature_list_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, featureListResponse{FeatureKeys: keys})
}

func listFeatureKeysForWorkspace(workspace string) ([]string, error) {
	seen := make(map[string]struct{})
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		seen[k] = struct{}{}
	}
	dotFP := filepath.Join(workspace, ".flowpilot")
	if catalog, err := featurecatalog.LoadCatalog(dotFP); err == nil {
		for _, f := range catalog.All() {
			add(f.Key)
		}
	}
	if ledger, err := changeledger.New(dotFP); err == nil {
		for _, k := range ledger.ListFeatures() {
			add(k)
		}
	}
	canonicalDir := filepath.Join(dotFP, "canonical")
	entries, err := os.ReadDir(canonicalDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			add(strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
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

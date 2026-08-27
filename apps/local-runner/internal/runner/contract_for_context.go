package runner

import (
	"path/filepath"

	"flowpilot-runner/internal/changecontract"
)

// latestContractForRun returns the latest Contract for runID with a FrozenStore
// fallback for Flow mode. Chat mode writes contracts.ndjson via Store;
// Flow preflight freeze writes frozen_contracts.ndjson via FrozenStore. Context
// sources (change.contract, source.dependence) must surface a Body on Flow runs
// even though the legacy Store is empty for that run. Precedence:
//  1. contracts.ndjson (legacy/chat) — GetLatestForRun
//  2. frozen_contracts.ndjson (Flow) — highest active version across all coder steps
//  3. miss → ok=false (empty Body, omitted heading — preserves
//     TestDependenceSourceNoContractDegrades / InferredDirBucket semantics)
func latestContractForRun(workspace, runID string) (changecontract.Contract, bool) {
	if workspace == "" || runID == "" {
		return changecontract.Contract{}, false
	}
	if s, err := changecontract.OpenStoreReadOnly(workspace); err == nil && s != nil {
		if c, ok := s.GetLatestForRun(runID); ok {
			return c, true
		}
	}
	// Flow fallback: no legacy contract for this run → try frozen preflight store.
	// NewFrozenStore creates the directory if missing; a missing contracts file
	// simply yields an empty store (no error, no side-effect beyond MkdirAll).
	if fs, err := changecontract.NewFrozenStore(workspace); err == nil && fs != nil {
		// Prefer a single active version when the caller will also use a
		// coderStepID (change.contract per-step fetch could use GetFrozenForStep),
		// but for the current shared hints we only have WorkflowRunID, so we pick
		// the latest active record across all steps for the run.
		if recs, err := fs.ListForRun(runID); err == nil && len(recs) > 0 {
			// ListForRun: superseded/abandoned still present — filter to active.
			// Re-check each via GetFrozenForStep's active semantics by re-opening
			// the status map, but deduping via fs.isActive is unexported.
			// Instead: collect the highest-version active per step, then pick the
			// newest DeclaredAt across steps. The helper below re-uses the store's
			// own active check via GetFrozenForStep per distinct CoderStepID.
			seenStep := map[string]struct{}{}
			var best *changecontract.FrozenContractRecord
			for _, r := range recs {
				if _, seen := seenStep[r.CoderStepID]; seen {
					continue
				}
				seenStep[r.CoderStepID] = struct{}{}
				active, ok, _ := fs.GetFrozenForStep(runID, r.CoderStepID)
				if !ok {
					continue
				}
				if best == nil || active.DeclaredAt.After(best.DeclaredAt) || (active.DeclaredAt.Equal(best.DeclaredAt) && active.Version > best.Version) {
					cp := active
					best = &cp
				}
			}
			if best != nil {
				c := frozenToContract(*best)
				// Keep the same on-disk SourceRef callers expect for provenance
				// (contracts.ndjson) even when data came from frozen store — the
				// fetch callers set their own SourceRef (see change_contract /
				// dependence Fetch). We only return the Contract value.
				_ = filepath.Join(workspace, ".flowpilot", "contracts", "contracts.ndjson")
				return c, true
			}
		}
	}
	return changecontract.Contract{}, false
}

func frozenToContract(r changecontract.FrozenContractRecord) changecontract.Contract {
	return changecontract.Contract{
		RunID:         r.RunID,
		StepID:        r.CoderStepID,
		FeatureKey:    r.FeatureKey,
		Intent:        r.Intent,
		DeclaredPaths: append([]string(nil), r.DeclaredPaths...),
		DeclaredAt:    r.DeclaredAt,
		Confidence:    changecontract.ConfidenceDeclared,
	}
}

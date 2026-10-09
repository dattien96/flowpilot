package runner

import (
	"context"

	"flowpilot-runner/internal/workingmode"
)

// allowVibeCloneUserStart is the single additive allowance on top of the
// fail-closed working-mode family gate: a catalog-UUID flowRef whose
// cloned_from ancestry roots at a vibe user-startable built-in
// (vibe-ingest / vibe-cp-ingest / vibe-tasks / vibe-adopt) may start with
// user kind in vibe mode — a clone keeps its source's role.
//
// Everything else stays rejected by the main gate: unknown refs, broken
// chains, and chains rooted at system-only flows (vibe-sprint /
// vibe-owner-debate / vibe-adopt-sprint) all return false here, exactly
// like their canonical ids. Callers only consult this after
// FlowAllowedForWorkingMode already rejected, so it can only ever admit —
// never deny — and costs store reads solely on that reject path.
func allowVibeCloneUserStart(ctx context.Context, store FlowDefinitionStore, mode, flowID string) bool {
	norm, err := workingmode.Normalize(mode)
	if err != nil || norm != workingmode.Vibe || store == nil {
		return false
	}
	lookup := func(ref string) (string, string, bool) {
		rec, ok, err := store.GetByRef(ctx, ref)
		if err != nil || !ok {
			return "", "", false
		}
		return rec.ClonedFrom, rec.PackFlowID, true
	}
	return workingmode.CloneInheritsVibeUserStart(flowID, lookup)
}

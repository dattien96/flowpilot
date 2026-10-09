package runner

import (
	"context"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

// allowVibeCloneStart is the single additive allowance on top of the
// fail-closed working-mode family gate: a catalog-UUID flowRef whose
// cloned_from ancestry roots at a vibe built-in (user-startable or
// system-only) may start with user kind in vibe mode — a clone keeps its
// source's role. The canonical system ids stay forbidden exactly as the
// main gate pins them; only user-owned copies are admitted.
//
// Everything else stays rejected by the main gate: unknown refs and broken
// chains return false here. Callers only consult this after
// FlowAllowedForWorkingMode already rejected, so it can only ever admit —
// never deny — and costs store reads solely on that reject path.
func allowVibeCloneStart(ctx context.Context, store FlowDefinitionStore, mode, flowID string) bool {
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
	return workingmode.CloneInheritsVibeStart(flowID, lookup)
}

// vibeCloneRootPackID resolves a catalog-UUID flowRef through cloned_from
// ancestry to its root pack flow id ("" when none). It backs the
// chat-orchestration deferral below: a mirror-rooted clone is a Flow-Mode
// flow authorized by the family gate, not a chat orchestration pick.
// Mirrors themselves never inherit (CA-1078 laundering pin); the main
// family gate owns those refs.
func vibeCloneRootPackID(ctx context.Context, store FlowDefinitionStore, flowID string) string {
	if store == nil {
		return ""
	}
	lookup := func(ref string) (string, string, bool) {
		rec, ok, err := store.GetByRef(ctx, ref)
		if err != nil || !ok {
			return "", "", false
		}
		return rec.ClonedFrom, rec.PackFlowID, true
	}
	if _, packFlowID, found := lookup(strings.TrimSpace(flowID)); !found || workingmode.BareFlowID(packFlowID) != "" {
		return ""
	}
	return workingmode.VibeAncestorRoot(flowID, lookup)
}

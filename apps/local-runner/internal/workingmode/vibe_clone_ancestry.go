package workingmode

import "strings"

// FlowAncestryLookup resolves a flowRef to its clone provenance and pack
// identity. clonedFrom is the parent ref ("" when the row has none);
// packFlowID is the built-in mirror's pack flow id ("" for user-owned rows).
// found is false when the ref resolves nowhere.
type FlowAncestryLookup func(ref string) (clonedFrom, packFlowID string, found bool)

// maxCloneAncestryDepth bounds the cloned_from chain walk below. Chains are
// one hop in practice (clone → source row); the cap only guards against a
// pathological cycle, which fails closed.
const maxCloneAncestryDepth = 8

// VibeUserAncestorRoot walks cloned_from ancestry and returns the root pack
// flow id when the chain lands on a built-in mirror row, else "".
// Fail-closed: unknown refs, broken chains, and cycles all yield "".
// Canonical pack ids are NOT resolved here — they are the main family
// gate's job; the walker only serves catalog-UUID refs the gate rejects.
func VibeUserAncestorRoot(flowRef string, lookup FlowAncestryLookup) string {
	if lookup == nil {
		return ""
	}
	seen := make(map[string]bool, maxCloneAncestryDepth)
	ref := strings.TrimSpace(flowRef)
	for range maxCloneAncestryDepth {
		if ref == "" || seen[ref] {
			return ""
		}
		seen[ref] = true
		clonedFrom, packFlowID, found := lookup(ref)
		if !found {
			return ""
		}
		if id := BareFlowID(packFlowID); id != "" {
			return id
		}
		ref = strings.TrimSpace(clonedFrom)
	}
	return ""
}

// CloneInheritsVibeUserStart reports whether flowRef is a user-owned flow
// whose cloned_from ancestry roots at a vibe user-startable built-in
// (vibe-ingest / vibe-cp-ingest / vibe-tasks / vibe-adopt). Chains rooted
// at system-only flows (vibe-sprint / vibe-owner-debate / vibe-adopt-sprint)
// stay rejected, exactly like their canonical ids.
func CloneInheritsVibeUserStart(flowRef string, lookup FlowAncestryLookup) bool {
	return isVibeUser(VibeUserAncestorRoot(flowRef, lookup))
}

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

// VibeAncestorRoot walks cloned_from ancestry and returns the root pack
// flow id when the chain lands on a built-in mirror row, else "".
// Fail-closed: unknown refs, broken chains, and cycles all yield "".
// Canonical pack ids are NOT resolved here — they are the main family
// gate's job; the walker only serves catalog-UUID refs the gate rejects.
func VibeAncestorRoot(flowRef string, lookup FlowAncestryLookup) string {
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

// CloneInheritsVibeStart reports whether flowRef is a user-owned flow whose
// cloned_from ancestry roots at a vibe built-in — user-startable (vibe-ingest
// / vibe-cp-ingest / vibe-tasks / vibe-adopt) or system-only (vibe-sprint /
// vibe-owner-debate / vibe-adopt-sprint). A clone keeps its source's role:
// the runner and pickers treat it exactly as startable as the flow it was
// copied from. Canonical ids stay governed by the main family gate alone.
func CloneInheritsVibeStart(flowRef string, lookup FlowAncestryLookup) bool {
	if lookup == nil {
		return false
	}
	// Only user-owned rows (no pack identity of their own) may inherit a
	// role through ancestry. A mirror/builtin row resolves to itself —
	// admitting it here would launder a system flow through its catalog
	// UUID (CA-1078 pin); the main family gate owns those refs.
	_, packFlowID, found := lookup(strings.TrimSpace(flowRef))
	if !found || BareFlowID(packFlowID) != "" {
		return false
	}
	root := VibeAncestorRoot(flowRef, lookup)
	return isVibeUser(root) || isVibeSystem(root)
}

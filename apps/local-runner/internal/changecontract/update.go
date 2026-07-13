package changecontract

import "time"

// UpdateHead folds an in-contract, gate-passing turn into h (Task-186 T-4):
// refreshes the behavior statement from c's declared/inferred intent,
// recomputes the signature, and settles status back to "current". Callers
// (gate_hook.go) must only call this when the turn was NOT spec-drifted or
// code-drifted (see SpecDrifted/CodeDrifted) — a drifted turn is surfaced for
// human reconciliation instead (BR-2), never silently folded in.
func UpdateHead(h CanonicalHead, c Contract) CanonicalHead {
	if c.Intent != "" {
		h.BehaviorStatement = c.Intent
	}
	h.Status = HeadStatusCurrent
	h.UpdatedAt = time.Now().UTC()
	h.IntentSignature = ComputeSignature(h)
	return h
}

// RebaselineWithSpec re-baselines a spec_less Head to current once a human
// has confirmed the newly-added governing doc(s) actually describe the
// feature's current behavior (r-attach-spec). This is deliberately distinct
// from drift reconciliation: a spec being ADDED is not the same as a spec
// CHANGING (SD-21 §5's "spec_less -> current" transition, not "spec_drifted").
func RebaselineWithSpec(workspace string, h CanonicalHead, docIDs []string) CanonicalHead {
	h.GoverningDocIDs = append([]string(nil), docIDs...)
	h.GoverningDocHashes = hashGoverningDocs(workspace, h.GoverningDocIDs)
	h.SpecConfidence = SpecConfidenceSpecBacked
	h.Status = HeadStatusCurrent
	h.UpdatedAt = time.Now().UTC()
	h.IntentSignature = ComputeSignature(h)
	return h
}

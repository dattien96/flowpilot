package changecontract

// SpecDrifted reports whether any of h's governing docs has changed on disk
// since the Head last recorded its hash (SD-21 "current -> spec_drifted"). A
// doc that has gone missing counts as drifted too (its current hash resolves
// to "", which will not match a previously-recorded non-empty hash) — never
// panics on a deleted file (F-1).
func SpecDrifted(workspace string, h CanonicalHead) bool {
	if len(h.GoverningDocIDs) == 0 {
		return false
	}
	for _, id := range h.GoverningDocIDs {
		var current string
		if path := resolveDocRefPath(workspace, id); path != "" {
			if hash, err := HashDoc(path); err == nil {
				current = hash
			}
		}
		if current != h.GoverningDocHashes[id] {
			return true
		}
	}
	return false
}

// CodeDrifted reports whether this turn's out-of-contract code change (per
// changecontract.ScopeDiff / Task-185's r-scope signal) happened with no
// corresponding governing-spec change to explain it (SD-21
// "current -> code_drifted"). When the spec itself also drifted this turn,
// that is reported as spec_drifted instead — the two states are mutually
// exclusive on the same turn (spec-drift takes precedence, since a changed
// spec is the more informative signal).
func CodeDrifted(hasOutOfContractChange, specDrifted bool) bool {
	return hasOutOfContractChange && !specDrifted
}

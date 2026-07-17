package runner

// allowedFCPMarkerIDs returns self + the RECORDED handoff provenance only (SD-24 §6.6).
// Topology alone (parent/sibling) is never trusted without a mint-time stamp.
func allowedFCPMarkerIDs(rs *interactiveRun) []string {
	if rs == nil || rs.id == "" {
		return nil
	}
	ids := []string{rs.id}
	if p := rs.recordedHandoffProvenanceRunID(); p != "" && p != rs.id {
		ids = append(ids, p)
	}
	return ids
}

// recordedHandoffProvenanceRunID returns the mint-time provenance for the active
// durable prompt being delivered (restart or gate-reprompt).
func (rs *interactiveRun) recordedHandoffProvenanceRunID() string {
	if rs == nil {
		return ""
	}
	if rs.pendingRestartProvenanceRunID != "" {
		return rs.pendingRestartProvenanceRunID
	}
	if rs.pendingGateRepromptProvenanceRunID != "" {
		return rs.pendingGateRepromptProvenanceRunID
	}
	// Fallback to explicit marker provenance list (session mirror).
	for _, id := range rs.markerProvenanceRunIDs {
		if id != "" && id != rs.id {
			return id
		}
	}
	return ""
}

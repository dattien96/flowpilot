package runner

import (
	"strings"
)

// Task-340 (CP-62 P-4): per-node read-only enforcement. A node whose
// FlowDefinition declares a posture is structurally gated at the shared
// approval bridge — reads pass, writes and unclassified operations are
// silent-denied with an audit event. The oracle rule (SS-14 AC-6: never
// weaken a test) becomes structural: a read_only reviewer and a verdict_only
// owner have no write path to weaken a test through.

const (
	PostureStandard    = "standard"
	PostureReadOnly    = "read_only"
	PostureVerdictOnly = "verdict_only"
)

// EventNodeIsolationWriteDenied is the audit event emitted when a gated node
// attempts a denied operation (silent-deny: the agent receives the decision,
// the run keeps moving per the loop's own retry semantics).
const EventNodeIsolationWriteDenied ProviderEventType = "node_isolation_write_denied"

// verdictToolName is the declared face the verdict_only posture allows
// (submit_review_outcome — the only MCP surface an owner needs).
const verdictToolName = "submit_review_outcome"

// flowNodePostureFor resolves the declared posture of the flow node a child
// run is executing. Children link to their node via the parent's
// activeFlowNodes matched on stepID (preferred) or label; runs without a
// parent (hubs, normal chat) or with no matching node return "" (standard —
// behavior unchanged). Reads run-state and parent nodes under s.mu.
func (s *InteractiveService) flowNodePostureFor(rs *interactiveRun) string {
	if s == nil || rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[rs.parentRunID]
	if parent == nil || len(parent.activeFlowNodes) == 0 {
		return ""
	}
	stepID, label := strings.TrimSpace(rs.stepID), strings.TrimSpace(rs.label)
	for _, n := range parent.activeFlowNodes {
		if n.ID != stepID && (label == "" || n.ID != label) {
			continue
		}
		switch strings.TrimSpace(n.Posture) {
		case PostureReadOnly:
			return PostureReadOnly
		case PostureVerdictOnly:
			return PostureVerdictOnly
		default:
			return PostureStandard
		}
	}
	return ""
}

// evaluateFlowNodeApproval is the posture→decision matrix (Task-340 §11).
// The standard/undeclared posture returns "approve" for completeness — the
// bridge never consults it for standard postures (decideFlowNodePosture
// routes on the posture first, keeping the legacy path untouched).
func evaluateFlowNodeApproval(posture string, details ApprovalDetails) string {
	switch posture {
	case PostureReadOnly:
		// Reuse the BUG-344 read-only machinery wholesale: reads (including
		// per-command-classified read-only Bash) approve; everything else —
		// including compound commands — denies by the composition invariant.
		return readOnlyApprovalDecision(details)
	case PostureVerdictOnly:
		return evaluateVerdictOnlyApproval(details)
	default:
		return "approve"
	}
}

// decideFlowNodePosture returns the auto-decision for a gated node's approval
// request. handled=false (standard/undeclared posture) keeps the legacy
// bridge path untouched — the enforcement is purely additive.
func (s *InteractiveService) decideFlowNodePosture(rs *interactiveRun, details ApprovalDetails) (decision, reason string, handled bool) {
	posture := s.flowNodePostureFor(rs)
	switch posture {
	case PostureReadOnly:
		return evaluateFlowNodeApproval(posture, details), "flow_node_posture_read_only", true
	case PostureVerdictOnly:
		return evaluateFlowNodeApproval(posture, details), "flow_node_posture_verdict_only", true
	default:
		return "", "", false
	}
}

// evaluateVerdictOnlyApproval gates the owner debate posture: no Bash at all
// (Q-3 — the owner reads via read tools and answers via the verdict tool),
// no file writes, and MCP/other calls only for known read tools or the
// verdict face.
func evaluateVerdictOnlyApproval(details ApprovalDetails) string {
	switch details.Kind {
	case "file":
		return "deny"
	case "exec":
		// Q-3: verdict_only has no Bash — even read-only commands are denied
		// so the owner cannot execute anything.
		return "deny"
	default:
		if isReadOnlyToolName(details.Reason) || isVerdictToolCall(details) {
			return "approve"
		}
		return "deny"
	}
}

// isVerdictToolCall reports whether the approval is for the declared verdict
// face (Reason carries the tool name on the claude MCP bridge; Command may
// carry it on other bridges). Wrapped MCP names count: providers surface the
// runner-hosted MCP face as `mcp__flowpilot__submit_review_outcome` or
// `flowpilot__submit_review_outcome` (the same wrapping BUG-344 documents for
// ask_user), so the match is exact OR `__<tool>`-suffixed.
func isVerdictToolCall(details ApprovalDetails) bool {
	for _, name := range []string{strings.TrimSpace(details.Reason), strings.TrimSpace(details.Command)} {
		if name == "" {
			continue
		}
		if name == verdictToolName || strings.HasSuffix(name, "__"+verdictToolName) {
			return true
		}
	}
	return false
}

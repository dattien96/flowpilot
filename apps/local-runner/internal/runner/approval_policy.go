package runner

import "strings"

// Phase 4 (04-04): the approval policy engine (YOLO=false).
//
// A `permission_required` from the runtime does not always mean "ask the human".
// The policy engine sits between the inbound approval request and the user and
// decides per command/tool:
//
//   - auto-approve known-safe operations (allowlist) → reply immediately, record;
//   - auto-deny known-dangerous operations (denylist) → reply deny, record;
//   - ask everything else → emit `permission_required`, show the card.
//
// This is what makes "gate SOME actions" possible (Task-032) instead of
// all-or-nothing. The DEFAULT is ask (empty lists). Policy is configured in Admin
// Web (03); the proxy MCP gate keeps keying on YOLO (CP-29).
//
// Every auto-decision MUST be replied back to the inbound request by the caller
// (the third dispatcher category in 04-03) — recording alone leaves the provider
// hanging. The engine only decides; the bridge does the reply + audit.

// PolicyOutcome is the engine's verdict for one approval request.
type PolicyOutcome string

const (
	PolicyAutoApprove PolicyOutcome = "auto_approve"
	PolicyAutoDeny    PolicyOutcome = "auto_deny"
	PolicyAsk         PolicyOutcome = "ask"
)

// ApprovalPolicyEngine matches an approval's command against an allow/deny list.
// Matching is case-insensitive substring containment; denylist wins over allowlist
// (safety-first: a command that is both allow- and deny-listed is denied).
type ApprovalPolicyEngine struct {
	allow []string
	deny  []string
}

// DefaultApprovalPolicyEngine is "ask everything" — the safe default (03). Empty
// lists mean no auto-decisions, so existing behavior (always show the card) holds.
func DefaultApprovalPolicyEngine() *ApprovalPolicyEngine {
	return &ApprovalPolicyEngine{}
}

// NewApprovalPolicyEngine builds an engine from allow/deny command patterns.
func NewApprovalPolicyEngine(allow, deny []string) *ApprovalPolicyEngine {
	return &ApprovalPolicyEngine{allow: normalizePatterns(allow), deny: normalizePatterns(deny)}
}

func normalizePatterns(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Decide returns the policy verdict for an approval request. The command is the
// primary signal; if it is empty the engine falls back to the tool name (so
// tool-only approvals can still be gated). Denylist precedence is intentional.
func (e *ApprovalPolicyEngine) Decide(details ApprovalDetails) PolicyOutcome {
	if e == nil {
		return PolicyAsk
	}
	subject := strings.ToLower(strings.TrimSpace(details.Command))
	if subject == "" {
		subject = strings.ToLower(strings.TrimSpace(details.Reason))
	}
	if subject == "" {
		return PolicyAsk
	}
	for _, p := range e.deny {
		if strings.Contains(subject, p) {
			return PolicyAutoDeny
		}
	}
	for _, p := range e.allow {
		if strings.Contains(subject, p) {
			return PolicyAutoApprove
		}
	}
	return PolicyAsk
}

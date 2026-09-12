package flowgate

// Task-337 (CP-62 P-1): deterministic gate precedence contract across the
// systems that share the prompt/loop — flowgate rules, the CP-23 drift
// ladder, the vibe owner-debate resolver, and the r-dod settlement. Pure
// routing over already-computed violations, 0 LLM tokens: the runner consumes
// Route to pick the resolver and AllowContextPruning to guard owner-debate
// context (T-3). Dev mode is byte-for-byte passthrough — the Task-335 ladder
// owns dev exactly as shipped.

const (
	// PrecedenceRouteNone is the legacy passthrough: the caller keeps the
	// pre-CP-62 Enforce path unchanged.
	PrecedenceRouteNone = ""
	// PrecedenceRouteUser is the requirement-class route: user-only block
	// (SS-18 BR-4) — never owner-resolved.
	PrecedenceRouteUser = "user"
	// PrecedenceRouteOwnerDebate routes block/reprompt violations (or a
	// threshold drift score) to the vibe-owner-debate flow.
	PrecedenceRouteOwnerDebate = "owner_debate"
)

// VibeDriftDebateThreshold is the drift score at which a vibe run escalates
// through the owner-debate resolver instead of the dev ladder's (deferred)
// pause path (CP-62 P-1 rule 2).
const VibeDriftDebateThreshold = 80

// PrecedenceResult is the CP-62 P-1 routing decision for one gate evaluation.
type PrecedenceResult struct {
	// Route is one of the PrecedenceRoute* constants. "" keeps the legacy
	// passthrough behavior.
	Route string
	// Action is the routed action for non-empty routes ("block" dominates).
	Action string
	// Ordered is the settlement order (T-4): r-dod-complete violations first
	// (the checklist state settles before the requirement block lands),
	// requirement-class last as the final blocker.
	Ordered []Violation
	// AllowContextPruning is false when the route is owner_debate or the
	// caller marks the turn as an owner-debate turn (T-3: the debate needs
	// the full violation context; the Budget Packer must not shrink it).
	AllowContextPruning bool
	// DriftRouted is true when owner_debate was chosen by drift score alone
	// (clean gate) — the debate prompt must carry the drift reason instead of
	// gate violations.
	DriftRouted bool
}

// IsRequirementViolation reports whether v is requirement-class (r-requirement
// or its signature-drift trigger) — always user-only, never owner-resolved
// (SS-18 BR-4).
func IsRequirementViolation(v Violation) bool {
	return v.Rule.ID == RequirementRuleID || v.Rule.Trigger == "requirement_signature_drift"
}

// IsDODCompleteViolation reports whether v belongs to r-dod-complete
// (block-or-explained settlement of a done Task/BUG doc).
func IsDODCompleteViolation(v Violation) bool {
	return v.Rule.ID == "r-dod-complete" || v.Rule.Trigger == "marked_done_with_open_dod"
}

// ResolvePrecedence returns the CP-62 P-1 routing for one gate evaluation.
//
// Tier order:
//  1. Requirement-class (r-requirement / signature drift) — user-only block,
//     wins over everything else; dod-complete still settles first in Ordered.
//  2. Vibe drift >= VibeDriftDebateThreshold — owner debate, even on a clean
//     gate (DriftRouted). Dev never routes on drift (Task-335 ladder owns it).
//  3. Other block/reprompt violations in vibe keep today's owner-debate
//     semantics; warn-only results stay passthrough.
func ResolvePrecedence(violations []Violation, workingMode string, driftScore int, isOwnerDebateTurn bool) PrecedenceResult {
	res := PrecedenceResult{AllowContextPruning: !isOwnerDebateTurn}
	var requirement, dod, others []Violation
	hasBlock, hasReprompt := false, false
	for _, v := range violations {
		switch {
		case IsRequirementViolation(v):
			requirement = append(requirement, v)
		case IsDODCompleteViolation(v):
			dod = append(dod, v)
		default:
			others = append(others, v)
			switch v.Rule.Action {
			case "block":
				hasBlock = true
			case "reprompt":
				hasReprompt = true
			}
		}
	}
	// T-4 settlement order: r-dod-complete first, requirement-class last.
	res.Ordered = append(append(append([]Violation(nil), dod...), others...), requirement...)

	if len(requirement) > 0 {
		res.Route = PrecedenceRouteUser
		res.Action = "block"
		return res
	}
	if workingMode == "vibe" {
		// An owner-debate turn never re-routes into another debate on drift
		// alone (the debate's own cap bounds it) — passthrough keeps only the
		// T-3 context guard.
		if !isOwnerDebateTurn && driftScore >= VibeDriftDebateThreshold {
			res.Route = PrecedenceRouteOwnerDebate
			res.Action = "block"
			res.DriftRouted = true
			res.AllowContextPruning = false
			return res
		}
		if hasBlock || hasReprompt {
			res.Route = PrecedenceRouteOwnerDebate
			res.Action = "block"
			if !hasBlock {
				res.Action = "reprompt"
			}
			res.AllowContextPruning = false
			return res
		}
	}
	return res
}

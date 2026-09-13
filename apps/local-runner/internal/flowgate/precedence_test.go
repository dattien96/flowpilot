package flowgate

import "testing"

// Task-337 (CP-62 P-1): deterministic gate precedence matrix.
// Tiers: requirement-class (user-only) > vibe drift 80+ (owner debate) >
// r-dod-complete (settled first) > other block/reprompt > warns.

func vibeDriftViolation(action string) Violation {
	return Violation{Rule: Rule{ID: "r-tests", Trigger: "tests_failed", Action: action}}
}

func requirementViolation() Violation {
	return Violation{Rule: Rule{ID: RequirementRuleID, Trigger: "requirement_signature_drift", Action: "block"}}
}

func dodCompleteViolation() Violation {
	return Violation{Rule: Rule{ID: "r-dod-complete", Trigger: "marked_done_with_open_dod", Action: "block"}}
}

// Scenario: Requirement-class vi phạm thì luôn thắng mọi gate khác và chặn chờ user
func TestPrecedence_RequirementClassBeatsDriftAndDod(t *testing.T) {
	res := ResolvePrecedence([]Violation{
		vibeDriftViolation("block"),
		dodCompleteViolation(),
		requirementViolation(),
	}, "vibe", 95, false)
	if res.Route != PrecedenceRouteUser {
		t.Fatalf("route=%q want user", res.Route)
	}
	if res.Action != "block" {
		t.Fatalf("action=%q want block", res.Action)
	}
	if !res.AllowContextPruning {
		t.Fatalf("user route must not disable context pruning")
	}
}

// Scenario: Trong Vibe mode, Drift score >= 80 định tuyến sang Owner Debate thay vì xuất dev card
func TestPrecedence_VibeDrift80RoutesToVibeGate(t *testing.T) {
	res := ResolvePrecedence([]Violation{vibeDriftViolation("block")}, "vibe", 80, false)
	if res.Route != PrecedenceRouteOwnerDebate {
		t.Fatalf("route=%q want owner_debate", res.Route)
	}
	if res.AllowContextPruning {
		t.Fatalf("owner debate must disable context pruning")
	}
	if !res.DriftRouted {
		t.Fatalf("drift-routed flag expected")
	}
}

// Scenario: Trong Dev mode, Drift score >= 80 vẫn xuất hiện dev card bình thường (không bị phá vỡ)
func TestPrecedence_DevModeDrift80NonRegression(t *testing.T) {
	res := ResolvePrecedence([]Violation{vibeDriftViolation("block")}, "dev", 95, false)
	if res.Route != PrecedenceRouteNone {
		t.Fatalf("route=%q want passthrough", res.Route)
	}
}

// Scenario: Turn Owner Debate không bị thu hẹp context dù drift score cao
func TestPrecedence_OwnerDebateBypassesContextReduction(t *testing.T) {
	res := ResolvePrecedence(nil, "vibe", 85, true)
	if res.AllowContextPruning {
		t.Fatalf("owner-debate turn must never allow context pruning")
	}
	if res.Route != PrecedenceRouteNone {
		t.Fatalf("clean owner-debate turn stays passthrough, got %q", res.Route)
	}
}

// Scenario: Trên cùng một turn done, r-dod-complete được đánh giá trước r-requirement
func TestPrecedence_RDodCompleteEvaluatedBeforeRRequirement(t *testing.T) {
	res := ResolvePrecedence([]Violation{
		requirementViolation(),
		dodCompleteViolation(),
	}, "vibe", 0, false)
	dodIdx, reqIdx := -1, -1
	for i, v := range res.Ordered {
		if IsDODCompleteViolation(v) && dodIdx < 0 {
			dodIdx = i
		}
		if IsRequirementViolation(v) && reqIdx < 0 {
			reqIdx = i
		}
	}
	if dodIdx < 0 || reqIdx < 0 {
		t.Fatalf("ordered missing members: dod=%d req=%d", dodIdx, reqIdx)
	}
	if dodIdx > reqIdx {
		t.Fatalf("dod-complete (%d) must settle before requirement (%d)", dodIdx, reqIdx)
	}
	if res.Route != PrecedenceRouteUser {
		t.Fatalf("route=%q want user", res.Route)
	}
}

// Edge: drift 79 (below threshold) in vibe keeps legacy semantics.
func TestPrecedence_VibeDriftBelowThresholdPassthrough(t *testing.T) {
	res := ResolvePrecedence(nil, "vibe", 79, false)
	if res.Route != PrecedenceRouteNone {
		t.Fatalf("route=%q want passthrough", res.Route)
	}
}

// Edge: warn-only violations never spawn an owner debate.
func TestPrecedence_VibeWarnOnlyPassthrough(t *testing.T) {
	res := ResolvePrecedence([]Violation{vibeDriftViolation("warn")}, "vibe", 0, false)
	if res.Route != PrecedenceRouteNone {
		t.Fatalf("route=%q want passthrough", res.Route)
	}
}

// Edge: drift-only escalation fires even on a fully clean gate (vibe).
func TestPrecedence_VibeDriftOnlyCleanGateEscalates(t *testing.T) {
	res := ResolvePrecedence(nil, "vibe", 80, false)
	if res.Route != PrecedenceRouteOwnerDebate || !res.DriftRouted {
		t.Fatalf("route=%q driftRouted=%v", res.Route, res.DriftRouted)
	}
}

// Error: empty violations + dev + no drift = pure passthrough, no panic.
func TestPrecedence_EmptyPassthrough(t *testing.T) {
	res := ResolvePrecedence(nil, "", 0, false)
	if res.Route != PrecedenceRouteNone || res.Action != "" {
		t.Fatalf("route=%q action=%q", res.Route, res.Action)
	}
}

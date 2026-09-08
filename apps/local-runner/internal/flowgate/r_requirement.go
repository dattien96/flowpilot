package flowgate

import "strings"

const RequirementRuleID = "r-requirement"

// RequirementRule is CP-60 P-2. Not in DefaultRules() so TestDefaultRules
// stays byte-stable; EnabledRulesFor appends it for working_mode=vibe only.
func RequirementRule() Rule {
	return Rule{
		ID:             RequirementRuleID,
		Scope:          "step",
		Trigger:        "requirement_signature_drift",
		RequiredOutput: "reconcile_tests_with_ss",
		Action:         "block",
		Enabled:        true,
	}
}

func testsGreen(tr TurnResult) bool {
	return tr.Tests.Ran && len(tr.Tests.Failed) == 0 && len(tr.Tests.Regressed) == 0
}

// CoerceVibeRequirementDrift maps green+TamperedTestPaths onto RequirementDrift
// so r-additive-tests is subsumed in vibe (CP-60 C8).
func CoerceVibeRequirementDrift(tr *TurnResult) {
	if tr == nil {
		return
	}
	if testsGreen(*tr) && len(tr.TamperedTestPaths) > 0 {
		tr.RequirementDrift = true
		if strings.TrimSpace(tr.RequirementDriftDetail) == "" {
			tr.RequirementDriftDetail = "green suite with weakened/tampered tests: " + strings.Join(tr.TamperedTestPaths, ", ")
		}
	}
}

// EnabledRulesFor returns the rule set for a working_mode. vibe appends
// r-requirement; dev strips it even if a stored file listed it.
func EnabledRulesFor(workingMode string, base []Rule) []Rule {
	out := append([]Rule(nil), base...)
	if workingMode == "vibe" {
		for _, r := range out {
			if r.ID == RequirementRuleID {
				return out
			}
		}
		return append(out, RequirementRule())
	}
	filtered := make([]Rule, 0, len(out))
	for _, r := range out {
		if r.ID != RequirementRuleID {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

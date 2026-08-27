package runner

import "strings"

// Task-301 T-6: maps FlowPilot canonical reasoning effort to Opencode --variant values.
// Live-verified via `opencode run --help --variant` and initialize configOptions effort
// (minimal/low/medium/high/xhigh) plus opencode models --format json if available.
// Unknown values degrade gracefully: unmapped -> ("",false) omits flag so model default applies,
// mirroring grokReasoningEffortID fallback behavior.
func opencodeReasoningVariantID(effort string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		return "minimal", true
	case "low":
		return "low", true
	case "medium":
		return "medium", true
	case "high":
		return "high", true
	case "xhigh":
		// Opencode's highest documented is xhigh (maps to max); treat xhigh as max
		return "xhigh", true
	case "max":
		return "max", true
	case "ultra":
		return "max", true
	case "":
		return "", false
	default:
		return "", false
	}
}

// opencodeVariantToEffort is the reverse for UI display (optional).
func opencodeVariantToEffort(variant string) string {
	switch strings.ToLower(strings.TrimSpace(variant)) {
	case "minimal":
		return "minimal"
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "max":
		return "max"
	default:
		return ""
	}
}

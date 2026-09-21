package runner

import "strings"

// Task-403 (CP-70): reasoning-effort mapping for Devin.
//
// Devin has no separate effort knob — the reasoning effort is embedded in the
// model id itself as the trailing suffix ("swe-2-high", "claude-opus-5-xhigh",
// "gpt-5-6-sol-max"). FlowPilot's canonical effort values therefore map to a
// Devin MODEL id: take the selected model's family, replace its effort suffix.
// Unknown/unmappable requests return "" so the caller keeps the model's own
// default (same degrade rule as grokReasoningEffortID, Task-220).

// devinEffortSuffixes are the suffix tokens Devin model ids use for effort.
// Ordered longest-first so "xhigh" wins over "high" when splitting.
var devinEffortSuffixes = []string{"xhigh", "high", "medium", "low", "max", "fast"}

// devinCanonicalEffort normalizes a FlowPilot reasoning effort value to a
// Devin effort suffix. Mirrors the canonical set the desktop sends
// (low/medium/high/xhigh/max); "" means "model default — no switch".
func devinCanonicalEffort(effort string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low", true
	case "medium", "med":
		return "medium", true
	case "high":
		return "high", true
	case "xhigh", "x-high", "extra_high", "extra-high":
		return "xhigh", true
	case "max":
		return "max", true
	default:
		return "", false
	}
}

// devinSplitModelEffort splits a Devin catalog model id into its family and
// effort suffix. "swe-2-high" → ("swe-2", "high"); "adaptive" →
// ("adaptive", ""). A trailing "-fast" modifier on the effort is preserved as
// part of the family ("gpt-5-6-sol-max-fast" → family "gpt-5-6-sol",
// effort "max-fast" is treated as effort "max" + fast flag — kept simple:
// "fast" itself is an effort suffix in Devin's catalog).
func devinSplitModelEffort(model string) (family, effort string) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return "", ""
	}
	for _, suf := range devinEffortSuffixes {
		tail := "-" + suf
		if strings.HasSuffix(m, tail) && len(m) > len(tail) {
			return m[:len(m)-len(tail)], suf
		}
	}
	return m, ""
}

// devinModelForEffort maps (model, effort) to the sibling catalog id carrying
// the requested effort. Returns "" when the request is unmappable — the model
// already carries that effort, the effort is unknown, or the model has no
// effort suffix to replace (family-only ids like "adaptive" get the suffix
// appended only when the family is non-empty).
func devinModelForEffort(model, effort string) string {
	family, current := devinSplitModelEffort(model)
	want, ok := devinCanonicalEffort(effort)
	if !ok || family == "" {
		return ""
	}
	if current == want {
		return ""
	}
	return family + "-" + want
}

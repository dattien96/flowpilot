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
	case "minimal":
		return "minimal", true
	case "none":
		return "none", true
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

// devinEffortTokens is the canonical effort vocabulary Devin embeds in catalog
// model ids as whole "-" segments (live-verified 3000.10.31 catalog). Segments
// AFTER the effort are variant modifiers, not efforts: "-fast", "-priority",
// "-1m" (context), and fusion "sidekick-<model>" tails — e.g.
// "claude-opus-5-high-fast" = family claude-opus-5 + effort high + fast
// variant, and "gpt-5-6-sol-low-priority" = sol + low + priority variant.
var devinEffortTokens = map[string]bool{
	"minimal": true, "none": true, "low": true,
	"medium": true, "high": true, "xhigh": true, "max": true,
}

// devinEffortOrder ranks tokens for stable SupportedReasoningEfforts output.
var devinEffortOrder = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}

// devinSplitModelEffortScoped splits "<family>-<effort>[-<modifiers>...]" by
// locating the LAST effort segment — modifier tails stay attached so remap
// targets keep them ("claude-opus-5-high-fast" + low → "claude-opus-5-low-fast",
// which exists in the catalog, instead of the nonexistent "...-high-low").
// The effort segment can never be the first token ("max" alone is a family).
// Returns effort="" when no effort segment exists ("adaptive", "swe-1-6-fast").
func devinSplitModelEffortScoped(model string) (family, effort, modifiers string) {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return "", "", ""
	}
	parts := strings.Split(m, "-")
	for i := len(parts) - 1; i >= 1; i-- {
		if devinEffortTokens[parts[i]] {
			return strings.Join(parts[:i], "-"), parts[i], strings.Join(parts[i+1:], "-")
		}
	}
	return m, "", ""
}

// devinModelForEffort maps (model, effort) to the sibling catalog id carrying
// the requested effort. Returns "" when the request is unmappable — the model
// already carries that effort, the effort is unknown, or the model has no
// effort suffix to replace (family-only ids like "adaptive" get the suffix
// appended only when the family is non-empty).
func devinModelForEffort(model, effort string) string {
	family, current, modifiers := devinSplitModelEffortScoped(model)
	want, ok := devinCanonicalEffort(effort)
	if !ok || family == "" {
		return ""
	}
	if current == want {
		return ""
	}
	mapped := family + "-" + want
	if modifiers != "" {
		mapped += "-" + modifiers
	}
	return mapped
}

// devinModelEffortsByID derives each catalog id's selectable efforts from its
// sibling variants: models sharing the same (family, modifiers) scope offer
// the union of effort tokens those siblings carry — so the pickers only
// present options whose remapped ids actually exist in the catalog
// ("deepseek-v4-pro" → {high, max}; bare-only ids like "adaptive" get none).
func devinModelEffortsByID(ids []string) map[string][]string {
	type scope struct{ family, modifiers string }
	groups := map[scope]map[string]bool{}
	bind := map[string]scope{}
	for _, id := range ids {
		family, effort, modifiers := devinSplitModelEffortScoped(id)
		if family == "" || effort == "" {
			continue
		}
		key := scope{family, modifiers}
		if groups[key] == nil {
			groups[key] = map[string]bool{}
		}
		groups[key][effort] = true
		bind[id] = key
	}
	out := make(map[string][]string, len(bind))
	for id, key := range bind {
		for _, e := range devinEffortOrder {
			if groups[key][e] {
				out[id] = append(out[id], e)
			}
		}
	}
	return out
}

package runner

import "strings"

// Task-440 (CP-86 P-1): catalog fallback for TokenUsage.ModelContextWindow.
// Adapters that self-report the window (Codex, Grok, OpenCode, Devin) win
// untouched; providers that never send one (Claude CLI) get the value the
// runner's static provider catalog knows. Unknown stays nil — deterministic
// degradation, never a fabricated number.
//
// The lookup is pure in-memory (static providerSpecs) so it is safe to call
// inside emitLocked under s.mu: no I/O, no blocking, no allocation beyond the
// result pointer.

// modelContextWindowFor resolves the context window for
// (providerKey, modelID) from the static provider catalog. It handles the
// ID forms the codebase actually uses: bare ("gpt-5.5"), provider-prefixed
// ("devin/swe-2-high"), and versioned family ids ("claude-sonnet-4-5" →
// catalog entry "claude-sonnet"). Longest catalog-prefix match wins so a
// more specific entry always beats the family default. nil when unknown.
func modelContextWindowFor(providerKey ProviderKey, modelID string) *int64 {
	norm := normalizeCatalogModelID(modelID)
	if norm == "" {
		return nil
	}
	spec, ok := lookupProviderSpec(string(providerKey))
	if !ok {
		return nil
	}
	var best int64
	bestLen := -1
	for _, m := range spec.Models {
		if m.ContextWindowTokens <= 0 {
			continue
		}
		id := normalizeCatalogModelID(m.ID)
		if id == "" {
			continue
		}
		if norm == id || strings.HasPrefix(norm, id+"-") {
			if len(id) > bestLen {
				best = m.ContextWindowTokens
				bestLen = len(id)
			}
		}
	}
	if bestLen < 0 {
		return nil
	}
	return &best
}

// normalizeCatalogModelID lowercases, trims, and drops a provider prefix
// ("devin/swe-2-high" → "swe-2-high" — but catalog ids may also carry the
// prefix, so both sides normalize the same way before matching).
func normalizeCatalogModelID(modelID string) string {
	norm := strings.ToLower(strings.TrimSpace(modelID))
	if i := strings.IndexByte(norm, '/'); i >= 0 {
		norm = norm[i+1:]
	}
	return norm
}

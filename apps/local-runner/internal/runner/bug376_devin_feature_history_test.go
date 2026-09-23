package runner

import "testing"

// BUG-376: devin turns never receive feature-history/CA injection because the
// provider allowlist omits ProviderKeyDevin.
func TestShouldInjectFeatureHistoryIncludesDevin(t *testing.T) {
	s := &InteractiveService{}
	for _, key := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGemini, ProviderKeyGrok, ProviderKeyDevin} {
		if !s.shouldInjectFeatureHistory(key) {
			t.Fatalf("provider %s excluded from feature-history injection", key)
		}
	}
}

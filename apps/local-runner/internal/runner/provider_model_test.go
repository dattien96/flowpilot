package runner

import "testing"

// providerKeyFromModel is the canonical model→provider mapping used to auto-select the
// provider for a workflow/step run from its configured model.
func TestProviderKeyFromModel(t *testing.T) {
	cases := map[string]ProviderKey{
		"gpt-5.5":                 ProviderKeyCodex,
		"GPT-5.4-mini":            ProviderKeyCodex,
		"claude-opus-4-8":         ProviderKeyClaude,
		"claude-sonnet":           ProviderKeyClaude,
		"gemini-3-pro":            ProviderKeyGemini,
		"gemini-3.5-flash-medium": ProviderKeyGemini,
	}
	for model, want := range cases {
		if got, ok := providerKeyFromModel(model); !ok || got != want {
			t.Fatalf("providerKeyFromModel(%q) = %q, ok=%v; want %q", model, got, ok, want)
		}
	}
	for _, unknown := range []string{"", "mystery-model", "o3-mini"} {
		if got, ok := providerKeyFromModel(unknown); ok {
			t.Fatalf("providerKeyFromModel(%q) = %q, ok=true; want ok=false", unknown, got)
		}
	}
}

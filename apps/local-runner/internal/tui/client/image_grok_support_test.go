package client

import "testing"

// CA-483: Grok accepts attachments via runner path-fallback (not ACP vision).
func TestSupportsImages_IncludesGrokPathFallback(t *testing.T) {
	for _, key := range []string{"codex", "claude", "grok", "Codex", "GROK"} {
		if !SupportsImages(key) {
			t.Fatalf("SupportsImages(%q)=false, want true", key)
		}
	}
	if SupportsImages("gemini") {
		t.Fatal("gemini still unsupported")
	}
	if reason := ImagesUnsupportedReason("grok"); reason != "" {
		t.Fatalf("grok should be supported, reason=%q", reason)
	}
	if reason := ImagesUnsupportedReason("gemini"); reason == "" {
		t.Fatal("gemini should have unsupported reason")
	}
}

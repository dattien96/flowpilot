package workingmode

import "testing"

func TestDetectVibeEntry_CPPath(t *testing.T) {
	id, src := DetectVibeEntry("requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md")
	if id != "vibe-cp-ingest" {
		t.Fatalf("flow=%q", id)
	}
	if !IsCodingPlanCPPath(src) {
		t.Fatalf("source=%q", src)
	}
}

func TestDetectVibeEntry_RawPrompt(t *testing.T) {
	id, src := DetectVibeEntry("fix login timeout")
	if id != "vibe-ingest" || src != "fix login timeout" {
		t.Fatalf("got %q %q", id, src)
	}
}

func TestDetectVibeEntry_Empty(t *testing.T) {
	id, src := DetectVibeEntry("  ")
	if id != "vibe-ingest" || src != "" {
		t.Fatalf("got %q %q", id, src)
	}
}

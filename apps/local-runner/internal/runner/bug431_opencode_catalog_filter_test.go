package runner

import (
	"strings"
	"testing"
)

// BUG-431: the multi-family opencode catalog lists Interactions-API-only and
// other non-conversational Google models (deep-research, embedding, veo, lyria,
// tts, live-*, computer-use) as available. The picker's default landed on
// google/deep-research-max-preview-04-2026 → every turn fails "This model only
// supports Interactions API". Non-chat families must be filtered from the
// served catalog, same as the existing deepseek-v4-flash-free skip.
func TestBug431_OpencodeCatalogFiltersNonChatModels(t *testing.T) {
	plain := strings.Join([]string{
		"google/deep-research-max-preview-04-2026",
		"google/deep-research-preview-04-2026",
		"google/gemini-embedding-001",
		"google/veo-3.1-generate-preview",
		"google/lyria-3-pro-preview",
		"google/gemini-2.5-flash-preview-tts",
		"google/gemini-3.1-flash-live-preview",
		"google/gemini-2.5-computer-use-preview-10-2025",
		"google/gemini-3.5-flash",
		"opencode/big-pickle",
		"opencode/muse-spark-1.2-contributor-free",
		"xai/grok-4.6",
	}, "\n")
	models, err := parseOpencodePlainModelsOutput([]byte(plain))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range models {
		seen[m.ID] = true
		for _, bad := range []string{"deep-research", "embedding", "veo-", "lyria", "tts", "live-", "computer-use"} {
			if strings.Contains(m.ID, bad) {
				t.Fatalf("non-chat model served as available: %s", m.ID)
			}
		}
	}
	for _, want := range []string{"google/gemini-3.5-flash", "opencode/big-pickle", "opencode/muse-spark-1.2-contributor-free", "xai/grok-4.6"} {
		if !seen[want] {
			t.Fatalf("chat-capable model dropped: %s (have %v)", want, seen)
		}
	}
}

// BUG-431: the verbose parser shares the same filter.
func TestBug431_OpencodeVerboseCatalogFiltersNonChatModels(t *testing.T) {
	verbose := "google/deep-research-max-preview-04-2026\n" +
		`{"capabilities":{"input":{"image":false}}}` + "\n" +
		"opencode/big-pickle\n" +
		`{"capabilities":{"input":{"image":true}}}` + "\n"
	models, ok := parseOpencodeVerboseModelsOutput([]byte(verbose))
	if !ok {
		t.Fatal("verbose parse failed")
	}
	for _, m := range models {
		if strings.Contains(m.ID, "deep-research") {
			t.Fatalf("non-chat model served by verbose parser: %s", m.ID)
		}
	}
	var found bool
	for _, m := range models {
		if m.ID == "opencode/big-pickle" {
			found = true
		}
	}
	if !found {
		t.Fatalf("chat model dropped by verbose parser: %v", models)
	}
}

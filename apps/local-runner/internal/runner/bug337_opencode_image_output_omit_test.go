package runner

import (
	"strings"
	"testing"
)

// BUG-337 F-1: opencodeToolOutput shares the same shape as grokToolOutput.
// Opencode image-send is native ACP blocks, but a read_file on a PNG would
// also return ImageContent/base64 via rawOutput. Same stub logic as Grok.

func TestBug337OpencodeToolOutput_ImageContentOmitted(t *testing.T) {
	largeBase64 := strings.Repeat("B", 200*1024)
	update := map[string]any{
		"rawOutput": map[string]any{
			"ImageContent": map[string]any{
				"data":     largeBase64,
				"mimeType": "image/png",
			},
		},
	}
	out := opencodeToolOutput(update)
	s, _ := out.(string)
	if s == "" || !strings.Contains(s, "omitted") {
		t.Fatalf("opencode image must be stubbed, got %v", out)
	}
	if strings.Contains(s, largeBase64) {
		t.Fatalf("stub must not contain large base64")
	}
}

func TestBug337OpencodeToolOutput_TextPreserved(t *testing.T) {
	update := map[string]any{"rawOutput": "short text"}
	out := opencodeToolOutput(update)
	if s, _ := out.(string); s != "short text" {
		t.Fatalf("short text must be preserved, got %q", s)
	}
}

func TestBug337OpencodeToolOutput_LargeStringOmitted(t *testing.T) {
	large := strings.Repeat("y", 100*1024)
	update := map[string]any{"rawOutput": large}
	out := opencodeToolOutput(update)
	s, _ := out.(string)
	if !strings.Contains(s, "omitted") {
		t.Fatalf("large string must be stubbed, got len %d", len(s))
	}
}

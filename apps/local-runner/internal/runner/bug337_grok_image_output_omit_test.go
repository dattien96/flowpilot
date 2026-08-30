package runner

import (
	"strings"
	"testing"
)

// BUG-337 F-1: grokToolOutput must stub image base64 so the SSE frame stays small.
// Grok's read_file on a PNG via the path-fallback returns ImageContent with MB of
// base64. The TUI SSE scanner (64 KB before this fix, 8 MB after) would still
// choke if the full payload were sent. Text outputs must remain untouched.

func TestBug337GrokToolOutput_ImageContentOmitted(t *testing.T) {
	largeBase64 := strings.Repeat("A", 200*1024) // 200 KB fake PNG base64
	update := map[string]any{
		"rawOutput": map[string]any{
			"ImageContent": map[string]any{
				"data":     largeBase64,
				"mimeType": "image/png",
				"type":     "image",
			},
		},
	}
	out := grokToolOutput(update)
	s, _ := out.(string)
	if s == "" || !strings.Contains(s, "omitted") {
		t.Fatalf("image ImageContent must be stubbed, got %v", out)
	}
	if strings.Contains(s, largeBase64) {
		t.Fatalf("stub must not contain large base64")
	}
}

func TestBug337GrokToolOutput_TextPreserved(t *testing.T) {
	update := map[string]any{
		"content": map[string]any{"text": "hello world"},
		"rawOutput": map[string]any{
			"content": "hello world",
		},
	}
	// Simulate tool output that is not image: should pass through (or be stub only if huge)
	// grokToolOutput prefers rawOutput, which here is a small map without ImageContent
	out := grokToolOutput(update)
	if out == nil {
		t.Fatal("output must not be nil")
	}
	if s, ok := out.(string); ok && strings.Contains(s, "omitted") {
		t.Fatalf("text output must not be stubbed, got %q", s)
	}
	// Small string directly
	update2 := map[string]any{"rawOutput": "short text output"}
	out2 := grokToolOutput(update2)
	if s, _ := out2.(string); s != "short text output" {
		t.Fatalf("short string must be preserved, got %q", s)
	}
}

func TestBug337GrokToolOutput_LargeStringOmitted(t *testing.T) {
	large := strings.Repeat("x", 100*1024)
	update := map[string]any{"rawOutput": large}
	out := grokToolOutput(update)
	s, _ := out.(string)
	if !strings.Contains(s, "omitted") {
		t.Fatalf("large string must be stubbed, got len %d", len(s))
	}
}

func TestBug337GrokToolOutput_DataImageFieldOmitted(t *testing.T) {
	largeBase64 := strings.Repeat("A", 80*1024)
	update := map[string]any{
		"rawOutput": map[string]any{
			"data":     largeBase64,
			"mimeType": "image/png",
		},
	}
	out := grokToolOutput(update)
	s, _ := out.(string)
	if !strings.Contains(s, "omitted") {
		t.Fatalf("data+image mime must be stubbed, got %v", out)
	}
}

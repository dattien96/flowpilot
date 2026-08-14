package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// CA-483: SendTurn with attachments writes .tmp/images and injects paths into
// session/prompt text (Grok has no multimodal image blocks).
func TestGrokSendTurn_ImagePathFallbackInPrompt(t *testing.T) {
	sessionID := "session-img-fallback-1"
	cwd := t.TempDir()

	var (
		mu         sync.Mutex
		promptText string
		promptCount int
	)

	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, cwd)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			mu.Lock()
			promptCount++
			if params, ok := m["params"].(map[string]any); ok {
				promptText = extractGrokPromptText(params)
			}
			mu.Unlock()
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	att := sampleImageAttachment()
	att.OriginalName = "clipboard.png"
	err := a.SendTurn(ctx, TurnRequest{
		RunID:          "run-img-1",
		ProviderTurnID: "turn-img-1",
		Prompt:         "describe this screenshot",
		Cwd:            cwd,
		Attachments:    []PromptAttachment{att},
	}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	mu.Lock()
	got := promptText
	n := promptCount
	mu.Unlock()
	if n != 1 {
		t.Fatalf("prompt calls=%d", n)
	}
	if !strings.Contains(got, "describe this screenshot") {
		t.Fatalf("missing user prompt in %q", got)
	}
	if !strings.Contains(got, "path fallback") {
		t.Fatalf("missing path-fallback banner in %q", got)
	}
	if !strings.Contains(filepathToSlash(got), ".tmp/images") {
		t.Fatalf("missing .tmp/images path in %q", got)
	}
	if !strings.Contains(got, "clipboard.png") {
		t.Fatalf("missing original name in %q", got)
	}
}

func TestGrokSendTurn_NoAttachments_NoPathFallbackBanner(t *testing.T) {
	sessionID := "session-no-img-1"
	cwd := t.TempDir()
	var promptText string
	var mu sync.Mutex

	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, cwd)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			if params, ok := m["params"].(map[string]any); ok {
				mu.Lock()
				promptText = extractGrokPromptText(params)
				mu.Unlock()
			}
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{
		RunID:          "run-no-img",
		ProviderTurnID: "turn-no-img",
		Prompt:         "hello only",
		Cwd:            cwd,
	}, &fakeGrokBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	mu.Lock()
	got := promptText
	mu.Unlock()
	if strings.Contains(got, "path fallback") {
		t.Fatalf("unexpected fallback banner: %q", got)
	}
	if !strings.Contains(got, "hello only") {
		t.Fatalf("missing prompt: %q", got)
	}
}

func extractGrokPromptText(params map[string]any) string {
	blocks, ok := params["prompt"].([]any)
	if !ok || len(blocks) == 0 {
		return ""
	}
	b0, ok := blocks[0].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := b0["text"].(string)
	return s
}

func filepathToSlash(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

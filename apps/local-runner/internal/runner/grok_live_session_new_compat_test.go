package runner

import (
	"context"
	"testing"
	"time"
)

// Live check: Grok 1.0.3 still accepts the Desktop-stable session/new shape
// (cwd + mcpServers only — no authMethodId). If this fails, a runner change
// would be justified; if it passes, TUI must not patch grok_adapter.
func TestLiveGrokSessionNewWithoutAuthMethodID(t *testing.T) {
	requireLiveGrokOptIn(t)
	skipIfNoRealGrokAccount(t)

	scratch := t.TempDir()
	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	h, err := r.ensureGrokProcess(ctx, "live-session-new-compat", scratch, nil, "", "low", false)
	if err != nil {
		t.Fatalf("ensureGrokProcess: %v", err)
	}
	defer h.close()

	result, err := h.dispatcher.call(ctx, "session/new", map[string]interface{}{
		"cwd":        scratch,
		"mcpServers": []interface{}{},
	})
	if err != nil {
		t.Fatalf("session/new without authMethodId failed (this WOULD justify a runner change): %v", err)
	}
	sessionID := grokACPResponseSessionID(map[string]interface{}{"result": result})
	if sessionID == "" {
		t.Fatalf("session/new succeeded but no sessionId in result: %+v", result)
	}
	t.Logf("session/new without authMethodId OK sessionId=%s", sessionID)

	promptResult, err := h.dispatcher.call(ctx, "session/prompt", grokACPPromptParams(sessionID, "Reply with exactly: OK"))
	if err != nil {
		t.Fatalf("session/prompt after unauthenticated-shape session/new failed: %v", err)
	}
	t.Logf("session/prompt OK stopReason=%v", promptResult["stopReason"])
}

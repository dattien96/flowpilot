package runner

import (
	"encoding/json"
	"strings"
	"testing"
)

// Task-444 (CP-86 P-5) — the UI surface's data contract. The client must be
// able to show "prompt ~Nk est" (heuristic input length) next to "usage Nk"
// (provider-reported) without conflating them, so token_usage_updated events
// carry the runner's own est figure (same len(prompt)/4 heuristic the audit
// line uses) as an explicit labeled field. Additive-only.

func task444NewRun(t *testing.T, prompt string) (*InteractiveService, *interactiveRun) {
	t.Helper()
	svc, _ := newTestServer(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	rs.lastPrompt = prompt
	svc.mu.Unlock()
	return svc, rs
}

func TestTask444_UsageEventCarriesEstPromptTokens(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, rs := task444NewRun(t, strings.Repeat("x", 4000))
			svc.mu.Lock()
			rs.providerKey = pk
			svc.emitLocked(rs, task443UsageEvent(200000, 42000, 42000))
			svc.mu.Unlock()
			var usage *TokenUsageSnapshot
			for _, e := range rs.events {
				if e.Type == EventTokenUsageUpdated {
					usage = e.TokenUsage
				}
			}
			if usage == nil || usage.EstPromptTokens == nil {
				t.Fatalf("%s: token_usage_updated missing estPromptTokens", pk)
			}
			if *usage.EstPromptTokens != 1000 {
				t.Fatalf("%s: estPromptTokens=%d want 1000 (len 4000 / 4)", pk, *usage.EstPromptTokens)
			}
		})
	}
}

func TestTask444_EstPromptTokensAbsentWhenNoPrompt(t *testing.T) {
	svc, rs := task444NewRun(t, "")
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 1000, 1000))
	svc.mu.Unlock()
	for _, e := range rs.events {
		if e.Type == EventTokenUsageUpdated && e.TokenUsage != nil && e.TokenUsage.EstPromptTokens != nil {
			t.Fatalf("est must be absent (rendered —) when no prompt, got %d", *e.TokenUsage.EstPromptTokens)
		}
	}
}

func TestTask444_EstPromptTokensJSONKey(t *testing.T) {
	svc, rs := task444NewRun(t, strings.Repeat("y", 800))
	svc.mu.Lock()
	svc.emitLocked(rs, task443UsageEvent(200000, 100, 100))
	svc.mu.Unlock()
	var ev ProviderEvent
	for _, e := range rs.events {
		if e.Type == EventTokenUsageUpdated {
			ev = e
		}
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"estPromptTokens"`) {
		t.Fatalf("serialized event missing estPromptTokens key: %s", raw)
	}
}

package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-540 — /open must not leak another chat's ctx/token usage into the status
// line. lastTokens is per-run RAM: clear it on open, then seed from the opened
// run's own last token_usage_updated replay event (nil when none, so only the
// model window fallback shows). Provider-agnostic — parameterized anyway.

func ca540Model(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	return m
}

func ca540OpenMsg(pk string, usage *client.TokenUsageSnapshot) ChatOpenedMsg {
	return ChatOpenedMsg{
		Handle: client.RunHandle{RunID: "run-b", ProviderKey: pk, Status: "completed"},
		Messages: []ChatMessage{
			{Role: "user", Content: "chat B prompt"},
			{Role: "assistant", Content: "chat B answer"},
		},
		TokenUsage: usage,
	}
}

func TestChatOpenedMsg_ClearsStaleTokenUsage(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := ca540Model(pk)
			// Chat A's usage is still in session RAM.
			win := int64(200000)
			m.lastTokens = &client.TokenUsageSnapshot{ModelContextWindow: &win}
			m.modelContextWin = win
			// /open of chat B (no token usage replayed for run B).
			m2, _ := m.Update(ca540OpenMsg(pk, nil))
			am := m2.(*AppModel)
			if am.lastTokens != nil {
				t.Fatalf("%s: /open must clear chat A usage, got %+v", pk, am.lastTokens)
			}
			// Status line must only show the model-window fallback, not chat A's
			// used/remain figures (no "/" total, no "remain", no "last").
			line := formatContextLimits(am.lastTokens, am.modelContextWin)
			if strings.Contains(line, "remain") || strings.Contains(line, "last ") || strings.Contains(line, "/") {
				t.Fatalf("%s: status line leaked stale usage: %q", pk, line)
			}
		})
	}
}

func TestChatOpenedMsg_SeedsUsageFromReplay(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := ca540Model(pk)
			win := int64(200000)
			m.modelContextWin = 999999 // stale window must be replaced
			usage := &client.TokenUsageSnapshot{
				ModelContextWindow: &win,
				Total:              &client.TokenUsageBreakdown{TotalTokens: 42},
			}
			m2, _ := m.Update(ca540OpenMsg(pk, usage))
			am := m2.(*AppModel)
			if am.lastTokens == nil || am.lastTokens.Total == nil || am.lastTokens.Total.TotalTokens != 42 {
				t.Fatalf("%s: /open must seed chat B usage, got %+v", pk, am.lastTokens)
			}
			if am.modelContextWin != win {
				t.Fatalf("%s: modelContextWin=%d want %d", pk, am.modelContextWin, win)
			}
			if got := formatContextLimits(am.lastTokens, am.modelContextWin); got == "" {
				t.Fatalf("%s: status line must show chat B usage", pk)
			}
		})
	}
}

func TestLastReplayTokenUsage(t *testing.T) {
	win := int64(200000)
	cases := []struct {
		name string
		evs  []client.ProviderEvent
		want int64 // total tokens of the last usage event; -1 = expect nil
	}{
		{"noEvents", nil, -1},
		{"noUsageEvents", []client.ProviderEvent{
			{Type: "turn_started", Prompt: "p"},
			{Type: "message_completed", Text: "hi"},
		}, -1},
		{"singleUsage", []client.ProviderEvent{
			{Type: "token_usage_updated", TokenUsage: &client.TokenUsageSnapshot{
				ModelContextWindow: &win, Total: &client.TokenUsageBreakdown{TotalTokens: 10},
			}},
		}, 10},
		{"lastWins", []client.ProviderEvent{
			{Type: "token_usage_updated", TokenUsage: &client.TokenUsageSnapshot{
				ModelContextWindow: &win, Total: &client.TokenUsageBreakdown{TotalTokens: 10},
			}},
			{Type: "turn_started", Prompt: "p2"},
			{Type: "token_usage_updated", TokenUsage: &client.TokenUsageSnapshot{
				ModelContextWindow: &win, Total: &client.TokenUsageBreakdown{TotalTokens: 42},
			}},
		}, 42},
		{"nilUsageIgnored", []client.ProviderEvent{
			{Type: "token_usage_updated", TokenUsage: nil},
			{Type: "token_usage_updated", TokenUsage: &client.TokenUsageSnapshot{
				ModelContextWindow: &win, Total: &client.TokenUsageBreakdown{TotalTokens: 7},
			}},
		}, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := lastReplayTokenUsage(c.evs)
			if c.want == -1 {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil || got.Total == nil || got.Total.TotalTokens != c.want {
				t.Fatalf("got %+v, want total=%d", got, c.want)
			}
		})
	}
}

// lastWins wins, the window-only fallback is exactly "ctx <window> window";
// anything with a used-token figure contains a "/" plus a remainder count.

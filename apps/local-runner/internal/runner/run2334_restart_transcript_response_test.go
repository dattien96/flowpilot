package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRun2334RestartReplayKeepsAssistantResponsesForEveryProvider(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			root := t.TempDir()
			cwd := filepath.Join(root, "workspace")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatalf("MkdirAll workspace: %v", err)
			}

			home := filepath.Join(root, string(provider)+"-home")
			sessionID := string(provider) + "-run-2334"
			const prompt = "restore this complete chat"
			const response = "The assistant response survived restart."
			writeRun2334ProviderTranscript(t, provider, home, cwd, sessionID, prompt, response)
			writeProviderAccountsConfig083(t, root, []ProviderAccount{{
				ID:          "acct-current",
				ProviderKey: string(provider),
				HomePath:    home,
				SlotIndex:   1,
				IsActive:    true,
				AuthStatus:  "connected",
				CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			}})

			store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("NewLocalFileSessionStore: %v", err)
			}
			runID := "run-2334-" + string(provider)
			if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID:             runID,
				ProjectID:         "project-1",
				ProviderKey:       provider,
				ProviderSessionID: sessionID,
				ProviderAccountID: "acct-current",
				WorkingDirectory:  cwd,
				Status:            RunStatusCompleted,
				RunKind:           "chat",
				StartedAt:         "2026-07-18T16:49:01Z",
				UpdatedAt:         "2026-07-18T16:52:17Z",
			}); err != nil {
				t.Fatalf("UpsertProviderSession: %v", err)
			}
			if err := store.AppendTurnLog(context.Background(), runID, turnLogLine{
				Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: prompt,
			}); err != nil {
				t.Fatalf("AppendTurnLog prompt: %v", err)
			}
			if provider == ProviderKeyCodex || provider == ProviderKeyGrok {
				kind := turnLogKindCodexSession
				if provider == ProviderKeyGrok {
					kind = turnLogKindGrokSession
				}
				if err := store.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: kind, SessionID: sessionID}); err != nil {
					t.Fatalf("AppendTurnLog session: %v", err)
				}
			}

			restarted := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
			handle, apiErr := restarted.resumeRun(runID)
			if apiErr != nil {
				t.Fatalf("resumeRun: code=%s message=%s", apiErr.code, apiErr.msg)
			}
			if handle.LastEventSeq == 0 {
				t.Fatal("resumeRun returned no replay events")
			}

			restarted.mu.Lock()
			events := append([]ProviderEvent(nil), restarted.runs[runID].events...)
			restarted.mu.Unlock()
			assertRun2334ReplayedPromptAndResponse(t, events, prompt, response)
		})
	}
}

func writeRun2334ProviderTranscript(t *testing.T, provider ProviderKey, home, cwd, sessionID, prompt, response string) {
	t.Helper()
	switch provider {
	case ProviderKeyCodex:
		writeCodexRolloutLines(t, home, sessionID, cwd, time.Now().UTC(), []string{
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"provider prompt"}]}}`,
			`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"The assistant response survived restart."}]}}`,
		})
	case ProviderKeyClaude:
		path := filepath.Join(home, ".claude", "projects", "project", sessionID+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll Claude project: %v", err)
		}
		writeLinesToPath083(t, path, []string{
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"provider prompt"}]}}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The assistant response survived restart."}]}}`,
		})
	case ProviderKeyGrok:
		writeGrokChatHistoryFixture(t, home, cwd, sessionID, []string{
			`{"type":"user","content":[{"type":"text","text":"<user_query>\nrestore this complete chat\n</user_query>"}]}`,
			`{"type":"assistant","content":"The assistant response survived restart."}`,
		})
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
}

func assertRun2334ReplayedPromptAndResponse(t *testing.T, events []ProviderEvent, prompt, response string) {
	t.Helper()
	var sawPrompt, sawResponse bool
	for _, event := range events {
		if event.Type == EventTurnStarted && event.Prompt == prompt {
			sawPrompt = true
		}
		if event.Type == EventMessageCompleted && event.Text == response {
			sawResponse = true
		}
	}
	if !sawPrompt || !sawResponse {
		t.Fatalf("replayed prompt=%t response=%t events=%+v", sawPrompt, sawResponse, events)
	}
}

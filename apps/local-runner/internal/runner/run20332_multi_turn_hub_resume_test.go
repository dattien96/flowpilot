package runner

// run-20332 class: multi-turn review loop must resume hub prose from turn-log
// (transcript_turn) without loading Grok chat_history — which produced
// word-doubled garbled text after restart.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun20332MultiTurnHubResumesFromTurnLogNotGrokFile(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parentID = "run-20332"
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "019f8017-hub",
		RunKind: "workflow", Status: RunStatusRunning,
		LoopState:   AgentLoopState{Status: "done", Round: 1, Cap: 3, RoundCap: 3, Mode: "explicit"},
		LastMessage: "**Consolidated review outcome submitted: `approved`**\n\nBoth reviewers agreed.",
		StartedAt:   "2026-07-20T12:00:00Z", UpdatedAt: "2026-07-20T12:30:00Z", TurnCount: 3,
	}); err != nil {
		t.Fatalf("parent: %v", err)
	}
	for _, child := range []ProviderSessionState{
		{
			RunID: "run-coder", ParentRunID: parentID, AgentName: "coder", Label: "grok-coder",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "019f8013-coder", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-20T12:00:01Z", UpdatedAt: "2026-07-20T12:10:00Z",
			LastMessage: "I'll load the task context", TurnCount: 2,
		},
		{
			RunID: "run-r0a", ParentRunID: parentID, AgentName: "reviewer", Label: "grok-review",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sid-r0", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-20T12:05:00Z", UpdatedAt: "2026-07-20T12:08:00Z",
			LastMessage: "request changes", TurnCount: 1,
		},
		{
			RunID: "run-r0b", ParentRunID: parentID, AgentName: "reviewer", Label: "my-reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sid-r0", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-20T12:05:01Z", UpdatedAt: "2026-07-20T12:08:01Z",
			LastMessage: "request changes too", TurnCount: 1,
		},
		{
			RunID: "run-r1a", ParentRunID: parentID, AgentName: "reviewer", Label: "grok-review",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sid-r1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-20T12:20:00Z", UpdatedAt: "2026-07-20T12:22:00Z",
			LastMessage: "approve", TurnCount: 1,
		},
		{
			RunID: "run-r1b", ParentRunID: parentID, AgentName: "reviewer", Label: "my-reviewer",
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sid-r1", RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-07-20T12:20:01Z", UpdatedAt: "2026-07-20T12:22:01Z",
			LastMessage: "approve too", TurnCount: 1,
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), child); err != nil {
			t.Fatalf("child %s: %v", child.RunID, err)
		}
	}

	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1 + 1 != 2",
	})
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindTranscriptTurn, TurnID: "turn-2",
		Assistant: "Both reviewers request changes for the same phantom-fix issue. Submitting changes_requested.",
	})
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindTranscriptTurn, TurnID: "turn-3",
		Assistant: "**Consolidated review outcome submitted: `approved`**\n\nBoth reviewers agreed — no conflicting verdicts.",
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, providerSessionID: "019f8017-hub",
		runKind: "workflow", flowEngineDriven: true, status: RunStatusRunning,
		resumedFromDisk: true, lastMessage: "Both reviewers agreed — no conflicting verdicts.",
		createdAt: "2026-07-20T12:00:00Z", updatedAt: "2026-07-20T12:30:00Z",
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = rs
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "done", Round: 1, Cap: 3, RoundCap: 3})
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)
	svc.appendResumedParentAnnotations(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	status := rs.status
	svc.mu.Unlock()

	var assistants []string
	var coderSpawns int
	for _, ev := range events {
		if ev.Type == EventMessageCompleted {
			if strings.Contains(ev.Text, "BothBoth") || strings.Contains(ev.Text, "changeschanges") {
				t.Fatalf("garbled doubled prose leaked: %q", ev.Text)
			}
			if strings.HasPrefix(ev.Text, "I'll load the task context") {
				t.Fatalf("coder prose on hub: %q", ev.Text)
			}
			assistants = append(assistants, ev.Text)
		}
		if ev.Type == EventAgentSpawnedByUser && ev.ChildRunID == "run-coder" {
			coderSpawns++
		}
	}
	if len(assistants) < 2 {
		t.Fatalf("want both synthesis turns from turn-log, got %v full=%v", assistants, summarizeEventTypes(events))
	}
	if !strings.Contains(assistants[0], "request changes") || !strings.Contains(assistants[1], "approved") {
		t.Fatalf("synthesis order wrong: %v", assistants)
	}
	if coderSpawns < 2 {
		t.Fatalf("want reinvoke second coder card, got %d: %v", coderSpawns, summarizeEventTypes(events))
	}
	last := events[len(events)-1]
	if last.Type != EventTurnCompleted {
		t.Fatalf("last event = %s, want turn_completed to clear Thinking...", last.Type)
	}
	if status != RunStatusCompleted {
		t.Fatalf("rs.status = %q after terminal resume, want completed", status)
	}
}

func TestNormalizeResumedFlowStatusDoneWithoutActiveNodes(t *testing.T) {
	st := ProviderSessionState{
		Status:    RunStatusRunning,
		LoopState: AgentLoopState{Status: "done"},
	}
	if got := normalizeResumedFlowStatus(st); got != RunStatusCompleted {
		t.Fatalf("normalizeResumedFlowStatus = %q, want completed (loop done, empty nodes)", got)
	}
}

func TestHistoryStatusForLiveRunUsesLoopDone(t *testing.T) {
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{
		id: "run-hist", flowEngineDriven: true, status: RunStatusRunning,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.agentOrchestrator.setLoop(rs.id, AgentLoopState{Status: "done", Cap: 3})
	svc.mu.Unlock()
	if got := svc.historyStatusForLiveRun(rs); got != RunStatusCompleted {
		t.Fatalf("historyStatusForLiveRun = %q, want completed", got)
	}
}

// The visible main-chat transcript must be equivalent before and after a restart
// for every supported provider. In particular, a flow hub cannot swap provider
// history for a partial turn log: older runs may have no transcript_turn rows.
func TestRun20332FlowHubHistoryParityForEveryProvider(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			root := t.TempDir()
			cwd := filepath.Join(root, "workspace")
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatalf("workspace: %v", err)
			}
			home := filepath.Join(root, string(provider)+"-home")
			sessionID := string(provider) + "-flow-hub"
			prompts := []string{"fix the first issue", "verify the second issue"}
			responses := []string{"Round one requested changes.", "Round two approved the fix."}
			writeRun20332FlowHubProviderHistory(t, provider, home, cwd, sessionID, prompts, responses)
			writeProviderAccountsConfig083(t, root, []ProviderAccount{{
				ID: "acct-" + string(provider), ProviderKey: string(provider), HomePath: home,
				SlotIndex: 1, AuthStatus: "connected", IsActive: true,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}})

			store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
			if err != nil {
				t.Fatalf("store: %v", err)
			}
			for i, prompt := range prompts {
				if err := store.AppendTurnLog(context.Background(), "run-20332-"+string(provider), turnLogLine{
					Kind: turnLogKindPrompt, TurnID: "turn-" + string(rune('1'+i)), Prompt: prompt,
				}); err != nil {
					t.Fatalf("prompt log: %v", err)
				}
			}
			if provider == ProviderKeyCodex || provider == ProviderKeyGrok {
				kind := turnLogKindCodexSession
				if provider == ProviderKeyGrok {
					kind = turnLogKindGrokSession
				}
				if err := store.AppendTurnLog(context.Background(), "run-20332-"+string(provider), turnLogLine{Kind: kind, SessionID: sessionID}); err != nil {
					t.Fatalf("session log: %v", err)
				}
			}

			rs := &interactiveRun{
				id: "run-20332-" + string(provider), providerKey: provider,
				providerAccountID: "acct-" + string(provider), providerSessionID: sessionID,
				realProviderSessionID: sessionID, workspaceCwd: cwd, runKind: "workflow",
				flowEngineDriven: true, status: RunStatusCompleted, resumedFromDisk: true,
				createdAt: "2026-07-20T12:00:00Z", updatedAt: "2026-07-20T12:30:00Z",
				subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
			}
			svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
			svc.mu.Lock()
			svc.runs[rs.id] = rs
			svc.mu.Unlock()
			svc.seedTranscriptFromDisk(rs)

			var gotPrompts, gotResponses []string
			for _, event := range rs.events {
				if event.Type == EventTurnStarted && event.Prompt != "" {
					gotPrompts = append(gotPrompts, event.Prompt)
				}
				if event.Type == EventMessageCompleted {
					gotResponses = append(gotResponses, event.Text)
				}
			}
			if strings.Join(gotPrompts, "|") != strings.Join(prompts, "|") || strings.Join(gotResponses, "|") != strings.Join(responses, "|") {
				t.Fatalf("history parity provider=%s prompts=%q responses=%q", provider, gotPrompts, gotResponses)
			}
			if last := rs.events[len(rs.events)-1]; last.Type != EventTurnCompleted {
				t.Fatalf("terminal replay provider=%s last=%s, want turn_completed", provider, last.Type)
			}
		})
	}
}

func TestRun20332TurnLogFallbackPreservesDuplicateResponseOrderForEveryProvider(t *testing.T) {
	for _, provider := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(provider), func(t *testing.T) {
			const sameResponse = "Acknowledged."
			historical := []ProviderEvent{
				{Type: EventTurnStarted, Prompt: "first", ProviderTurnID: "turn-1"},
				{Type: EventTurnStarted, Prompt: "second", ProviderTurnID: "turn-2"},
				{Type: EventMessageCompleted, Text: sameResponse, ProviderTurnID: "turn-2"},
			}
			merged := mergeTurnLogAssistantsIntoTranscript(historical, []turnLogLine{
				{Kind: turnLogKindTranscriptTurn, TurnID: "turn-1", Assistant: sameResponse},
				{Kind: turnLogKindTranscriptTurn, TurnID: "turn-2", Assistant: sameResponse},
			})
			if got, want := len(merged), 4; got != want {
				t.Fatalf("provider=%s events=%v, want %d", provider, summarizeEventTypes(merged), want)
			}
			if merged[1].Type != EventMessageCompleted || merged[1].ProviderTurnID != "turn-1" || merged[1].Text != sameResponse {
				t.Fatalf("provider=%s missing turn-1 response was not inserted in place: %+v", provider, merged)
			}
			if merged[3].Type != EventMessageCompleted || merged[3].ProviderTurnID != "turn-2" || merged[3].Text != sameResponse {
				t.Fatalf("provider=%s turn-2 response was not retained: %+v", provider, merged)
			}
		})
	}
}

func writeRun20332FlowHubProviderHistory(t *testing.T, provider ProviderKey, home, cwd, sessionID string, prompts, responses []string) {
	t.Helper()
	switch provider {
	case ProviderKeyCodex:
		writeCodexRolloutLines(t, home, sessionID, cwd, time.Now().UTC(), []string{
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"provider first"}]}}`,
			`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Round one requested changes."}]}}`,
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"provider second"}]}}`,
			`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Round two approved the fix."}]}}`,
		})
	case ProviderKeyClaude:
		path := filepath.Join(home, ".claude", "projects", "project", sessionID+".jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("Claude history directory: %v", err)
		}
		writeLinesToPath083(t, path, []string{
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"provider first"}]}}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Round one requested changes."}]}}`,
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"provider second"}]}}`,
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Round two approved the fix."}]}}`,
		})
	case ProviderKeyGrok:
		writeGrokChatHistoryFixture(t, home, cwd, sessionID, []string{
			`{"type":"user","content":[{"type":"text","text":"<user_query>\nprovider first\n</user_query>"}]}`,
			`{"type":"assistant","content":"Round one requested changes."}`,
			`{"type":"user","content":[{"type":"text","text":"<user_query>\nprovider second\n</user_query>"}]}`,
			`{"type":"assistant","content":"Round two approved the fix."}`,
		})
	default:
		t.Fatalf("unsupported provider %q", provider)
	}
}

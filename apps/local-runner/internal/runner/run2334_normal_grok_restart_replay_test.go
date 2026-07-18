package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors covers a
// normal-chat restart where Grok's transcript stores provider reinforcement in
// <user_query> while approvals/questions are restored from the durable sidecar.
func TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := "/workspace/gate-sandbox"
	sessionID := "session-run-2334"
	firstPrompt := "hi grok"
	secondPrompt := "Create yolo-test.txt"
	gateReprompt := "The flow gate is asking you to add a required document before this step can complete:\n\nCreate the required record now."
	thirdPrompt := "Ask whether I prefer Python, TypeScript, or Go."
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+firstPrompt+"\n\n---\ninternal ask_user reinforcement\n</user_query>"),
		`{"type":"assistant","content":"Hello."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+secondPrompt+"\n\n---\ninternal MCP preamble\n</user_query>"),
		`{"type":"assistant","content":"Creating the file."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+gateReprompt+"\n</user_query>"),
		`{"type":"assistant","content":"The required record is ready."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+thirdPrompt+"\n\n---\ninternal ask_user reinforcement\n</user_query>"),
		`{"type":"assistant","content":"Which language do you prefer?"}`,
	})
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1,
		AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: firstPrompt},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: secondPrompt},
		{Kind: turnLogKindPrompt, TurnID: "turn-gate", Prompt: gateReprompt},
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: thirdPrompt},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
	} {
		if err := store.AppendTurnLog(context.Background(), "run-2334", line); err != nil {
			t.Fatalf("AppendTurnLog(%s): %v", line.Kind, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id: "run-2334", providerKey: ProviderKeyGrok, providerAccountID: "acct-grok",
		workspaceCwd: cwd, runKind: "chat", status: RunStatusCompleted,
		createdAt: "2026-07-18T16:49:01Z", resumedFromDisk: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
		events: []ProviderEvent{
			{Type: EventPermissionRequired, ProviderTurnID: "turn-2", ApprovalID: "approval-2", OccurredAt: "2026-07-18T16:50:06Z"},
			{Type: EventUserQuestionRequired, ProviderTurnID: "turn-3", QuestionID: "question-3", OccurredAt: "2026-07-18T16:52:11Z"},
			{Type: EventPermissionRequired, ProviderTurnID: "turn-missing", ApprovalID: "approval-missing", OccurredAt: "2026-07-18T16:53:00Z"},
		},
		sidecarPrefixCount: 3,
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts []string
	indices := map[string]int{}
	for i, event := range events {
		if event.Type == EventTurnStarted && event.Prompt != "" {
			prompts = append(prompts, event.Prompt)
			indices[event.ProviderTurnID] = i
		}
		if event.Type == EventPermissionRequired {
			if event.ApprovalID == "approval-2" {
				indices["approval"] = i
			} else if event.ApprovalID == "approval-missing" {
				indices["unmatched"] = i
			}
		}
		if event.Type == EventUserQuestionRequired {
			indices["question"] = i
		}
	}
	if len(prompts) != 3 || prompts[0] != firstPrompt || prompts[1] != secondPrompt || prompts[2] != thirdPrompt {
		t.Fatalf("replayed prompts = %q, want only raw user prompts [%q %q %q]", prompts, firstPrompt, secondPrompt, thirdPrompt)
	}
	if indices["approval"] <= indices["turn-2"] || indices["approval"] >= indices["turn-3"] {
		t.Fatalf("approval index = %d, want inside turn-2 before turn-3; indices=%v", indices["approval"], indices)
	}
	if indices["question"] <= indices["turn-3"] {
		t.Fatalf("question index = %d, want after its turn-3 prompt; indices=%v", indices["question"], indices)
	}
	if indices["unmatched"] <= indices["question"] {
		t.Fatalf("unmatched sidecar index = %d, want conservative fallback after replayed transcript; indices=%v", indices["unmatched"], indices)
	}
}

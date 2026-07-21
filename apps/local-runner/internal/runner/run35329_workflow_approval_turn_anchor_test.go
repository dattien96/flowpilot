package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-35329 residual: after a full server restart, reopening a Workflow/flow-engine
// hub (Grok Review Loop, post-done follow-up) moved resolved permission_required
// cards to the absolute bottom of the timeline — after every later follow-up —
// because reorderSidecarPrefixToEnd only turn-anchored runKind=="chat".
//
// Product contract (durable-replay-contracts): approval cards retain their
// original causal position next to the turn that requested them.
//
// cross-provider-parity: Case 1 — reorder/anchor has no providerKey branch;
// exercise Grok (live symptom) plus Claude/Codex representative seeds.
// additive-tests-only: new file only.

func TestWorkflowRestartAnchorsApprovalToMatchingTurn_Grok(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := "/workspace/gate-sandbox"
	sessionID := "session-run-35329"
	// Flow first-prompt is often suppressed on the hub; two follow-ups after done.
	follow1 := "turn cũ fix gì"
	follow2 := "file Ca đã tạo trong turn cũ là gì. append vào file đó dòng hello"
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+follow1+"\n</user_query>"),
		`{"type":"assistant","content":"Turn cũ chỉ verify docs, không patch production."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+follow2+"\n</user_query>"),
		`{"type":"assistant","content":"Đã append hello vào CA."}`,
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
		{Kind: turnLogKindPrompt, TurnID: "turn-follow-1", Prompt: follow1},
		{Kind: turnLogKindPrompt, TurnID: "turn-follow-2", Prompt: follow2},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
	} {
		if err := store.AppendTurnLog(context.Background(), "run-35329", line); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}
	// Resolved approval that fired during the first follow-up turn (shell gate).
	if err := store.UpsertApproval(context.Background(), ProviderApprovalState{
		ApprovalID: "appr-git", RunID: "run-35329", Status: "resolved", Decision: "approve",
		Command: "git status -sb && git log --oneline -5", ProviderTurnID: "turn-follow-1",
	}); err != nil {
		t.Fatalf("UpsertApproval: %v", err)
	}
	if err := store.AppendEvent(context.Background(), ProviderEvent{
		Type: EventPermissionRequired, WorkflowRunID: "run-35329", ApprovalID: "appr-git",
		ProviderTurnID: "turn-follow-1", Provider: ProviderKeyGrok,
		OccurredAt: "2026-07-22T10:00:00Z",
		Details:    &ApprovalDetails{Kind: "exec", Command: "git status -sb && git log --oneline -5"},
		Decision:   "approve",
	}); err != nil {
		t.Fatalf("AppendEvent permission: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	// Reconstruct like resumeRun: load sidecar first, then seed transcript.
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID: "run-35329", ProjectID: "p", ProviderKey: ProviderKeyGrok,
		ProviderAccountID: "acct-grok", WorkingDirectory: cwd,
		Status: RunStatusCompleted, RunKind: "workflow", // the live bug path
		ProviderSessionID: sessionID,
		// Flow hub topology so this is clearly a workflow/flow reopen.
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "coder"}, {ID: "synthesis"}},
		StartedAt:       "2026-07-22T09:00:00Z",
		UpdatedAt:       "2026-07-22T10:05:00Z",
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %s", apiErr.msg)
	}
	// reconstruct already loaded sidecar into rs.events; seed moves/anchors.
	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	idxPrompt1, idxPrompt2, idxApproval := -1, -1, -1
	for i, ev := range events {
		switch {
		case ev.Type == EventTurnStarted && ev.Prompt == follow1:
			idxPrompt1 = i
		case ev.Type == EventTurnStarted && ev.Prompt == follow2:
			idxPrompt2 = i
		case ev.Type == EventPermissionRequired && ev.ApprovalID == "appr-git":
			idxApproval = i
		}
	}
	if idxPrompt1 < 0 || idxPrompt2 < 0 || idxApproval < 0 {
		t.Fatalf("missing events: prompt1=%d prompt2=%d approval=%d (n=%d)",
			idxPrompt1, idxPrompt2, idxApproval, len(events))
	}
	// Must sit after its own prompt and BEFORE the later follow-up prompt —
	// not at the absolute end after follow-2 (the run-35329 screenshot bug).
	if idxApproval <= idxPrompt1 {
		t.Fatalf("approval idx=%d must be after follow-1 prompt idx=%d", idxApproval, idxPrompt1)
	}
	if idxApproval >= idxPrompt2 {
		t.Fatalf("approval idx=%d must be before follow-2 prompt idx=%d (was moved to end)", idxApproval, idxPrompt2)
	}
}

// Same anchor path for Claude/Codex workflow kind (provider-agnostic reorder).
func TestWorkflowRestartAnchorsApprovalToMatchingTurn_AllProviders(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			svc := NewInteractiveService()
			rs := &interactiveRun{
				id: "run-wf-" + string(pk), providerKey: pk, runKind: "workflow",
				flowEngineDriven: true, resumedFromDisk: true,
				status: RunStatusCompleted, createdAt: "2026-07-22T09:00:00Z",
				subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
				events: []ProviderEvent{
					{Type: EventPermissionRequired, ProviderTurnID: "turn-a", ApprovalID: "appr-a", Decision: "approve"},
				},
				sidecarPrefixCount: 1,
			}
			// Transcript seeded after the sidecar prefix (simulates post-seedTranscript).
			rs.events = append(rs.events,
				ProviderEvent{Type: EventTurnStarted, ProviderTurnID: "turn-a", Prompt: "first follow-up"},
				ProviderEvent{Type: EventMessageCompleted, ProviderTurnID: "turn-a", Text: "answer 1"},
				ProviderEvent{Type: EventTurnStarted, ProviderTurnID: "turn-b", Prompt: "second follow-up"},
				ProviderEvent{Type: EventMessageCompleted, ProviderTurnID: "turn-b", Text: "answer 2"},
			)
			svc.mu.Lock()
			svc.runs[rs.id] = rs
			svc.mu.Unlock()

			// Call reorder directly (seedTranscript would no-op without files).
			svc.reorderSidecarPrefixToEnd(rs)

			svc.mu.Lock()
			events := append([]ProviderEvent(nil), rs.events...)
			svc.mu.Unlock()

			var iA, iAppr, iB int = -1, -1, -1
			for i, ev := range events {
				if ev.Type == EventTurnStarted && ev.ProviderTurnID == "turn-a" {
					iA = i
				}
				if ev.Type == EventPermissionRequired && ev.ApprovalID == "appr-a" {
					iAppr = i
				}
				if ev.Type == EventTurnStarted && ev.ProviderTurnID == "turn-b" {
					iB = i
				}
			}
			if iA < 0 || iAppr < 0 || iB < 0 {
				t.Fatalf("%s: missing indices a=%d appr=%d b=%d events=%+v", pk, iA, iAppr, iB, eventTypes(events))
			}
			if !(iA < iAppr && iAppr < iB) {
				t.Fatalf("%s: want turn-a < approval < turn-b, got %d < %d < %d types=%v",
					pk, iA, iAppr, iB, eventTypes(events))
			}
		})
	}
}

func eventTypes(events []ProviderEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = string(e.Type)
	}
	return out
}

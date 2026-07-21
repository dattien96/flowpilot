package runner

// Exact run-24377 wall-clock fixture (from .flowpilot/chats on 2026-07-21).
// Verifies resume order in unit tests — no desktop screenshot required.
//
// additive-tests-only: new file only.
// cross-provider-parity Case 1: shared flow-hub turn-log seed path.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun24377LiveFixtureResumeOrderIsCorrect(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatal(err)
	}
	const (
		parentID  = "run-24377"
		sessionID = "019f8526-c53f-7e23-ba74-3045301e1e94"
		cwd       = "/Users/tiendat/Desktop/BE/gate-sandbox"
	)
	grokHome := filepath.Join(root, "grok-home")
	histDir := filepath.Join(grokHome, "sessions", percentEncodeGrokCwd(cwd), sessionID)
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Polluted shared hub+coder session (as observed on disk).
	polluted := `{"type":"user","content":[{"type":"text","text":"<user_query>\nYou are the implementation agent.\n[FlowPilot sub-agent — agent: coder]\n</user_query>"}]}
{"type":"assistant","content":"CHILD ONLY must not appear on hub"}
{"type":"user","content":[{"type":"text","text":"<user_query>\n[flow-engine joined result note]\nFlow round 0\n</user_query>"}]}
{"type":"assistant","content":"POLLUTED synth"}
{"type":"user","content":[{"type":"text","text":"<user_query>\n[FlowPilot system note — sub-agents started in this session via the UI (not by you): - Flow completed.]\n\ndone rồi hả, trả lời ok or not.\n</user_query>"}]}
{"type":"assistant","content":"POLLUTED follow-up"}
`
	if err := os.WriteFile(filepath.Join(histDir, "chat_history.jsonl"), []byte(polluted), 0o644); err != nil {
		t.Fatal(err)
	}

	type child struct {
		id, label, start, upd, msg string
		turns                      int
	}
	// Exact started/updated from live sessions.ndjson for run-24377 children.
	children := []child{
		{"run-24382", "coder", "2026-07-21T14:48:57.806588Z", "2026-07-21T14:56:01.672285Z", "coder done", 4},
		{"run-25068", "reviewer_correctness", "2026-07-21T14:50:25.712638Z", "2026-07-21T14:51:57.719109Z", "r0a", 1},
		{"run-25076", "reviewer_security", "2026-07-21T14:50:26.064062Z", "2026-07-21T14:51:58.366492Z", "r0b", 1},
		{"run-27697", "reviewer_correctness", "2026-07-21T14:53:03.019575Z", "2026-07-21T14:54:19.849105Z", "r1a", 1},
		{"run-27705", "reviewer_security", "2026-07-21T14:53:03.356182Z", "2026-07-21T14:54:51.618506Z", "r1b", 1},
		{"run-30114", "reviewer_correctness", "2026-07-21T14:56:01.977075Z", "2026-07-21T14:56:51.178926Z", "r2a", 1},
		{"run-30122", "reviewer_security", "2026-07-21T14:56:02.340286Z", "2026-07-21T14:56:52.71325Z", "r2b", 1},
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID,
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "chat",
		Status:      RunStatusCompleted,
		LoopState:   AgentLoopState{Status: "done", Cap: 3, Mode: "explicit", Round: 2, RoundCap: 3},
		ChatFlowRef: "flowpilot-core-flow-pack/review-loop",
		StartedAt:   "2026-07-21T14:48:56.194028Z", UpdatedAt: "2026-07-21T14:59:01.069956Z",
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range children {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: c.id, ParentRunID: parentID, AgentName: "agent", Label: c.label,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-" + c.id, RunKind: "chat",
			Status: RunStatusCompleted, LastMessage: c.msg, TurnCount: c.turns,
			StartedAt: c.start, UpdatedAt: c.upd,
			ProviderAccountID: "acct-g", WorkingDirectory: cwd,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Exact hub turn log order from run-24377-turns.ndjson
	for _, row := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-24379", Prompt: "fix bug 1 + 1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-27217", Prompt: "[flow-engine joined result note]\nFlow round 0 — 2 results joined."},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-27217", Assistant: "I'll consolidate both reviewers' findings. Round 0: changes requested."},
		{Kind: turnLogKindPrompt, TurnID: "turn-29824", Prompt: "[flow-engine joined result note]\nFlow round 1 — 2 results joined."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-29824", Assistant: "Both reviewers agree: request changes. Round 1."},
		{Kind: turnLogKindPrompt, TurnID: "turn-31413", Prompt: "[flow-engine joined result note]\nFlow round 2 — 2 results joined."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-31413", Assistant: "## Consolidated review outcome\n**Submitted:** approved. Round 2."},
		{Kind: turnLogKindPrompt, TurnID: "turn-31488", Prompt: "done rồi hả, trả lời ok or not."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-31488", Assistant: "**ok** — flow done; correctness + security both approved."},
	} {
		if err := store.AppendTurnLog(context.Background(), parentID, row); err != nil {
			t.Fatal(err)
		}
	}
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-g", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-g"
	rs := &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, providerSessionID: sessionID,
		realProviderSessionID: sessionID, providerAccountID: "acct-g",
		workspaceCwd: cwd, runKind: "chat", flowEngineDriven: true,
		chatFlowRef: "flowpilot-core-flow-pack/review-loop",
		status:      RunStatusCompleted, resumedFromDisk: true,
		createdAt: "2026-07-21T14:48:56.194028Z", updatedAt: "2026-07-21T14:59:01.069956Z",
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)
	svc.appendResumedParentAnnotations(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	t.Logf("restored timeline (%d events):", len(events))
	for i, ev := range events {
		t.Logf("  [%02d] %s", i, describeResumeEvent(ev))
	}

	if len(events) == 0 {
		t.Fatal("empty timeline")
	}
	if events[0].Type != EventTurnStarted || events[0].Prompt != "fix bug 1 + 1 != 2" {
		t.Fatalf("events[0] = %s, want original user prompt first", describeResumeEvent(events[0]))
	}

	var prompts []string
	var synthIdx []int
	followPromptIdx := -1
	firstAgentIdx := -1
	for i, ev := range events {
		switch ev.Type {
		case EventTurnStarted:
			if strings.TrimSpace(ev.Prompt) == "" {
				continue
			}
			if isSystemPrompt(ev.Prompt) {
				t.Fatalf("[%d] system prompt surfaced: %q", i, ev.Prompt)
			}
			if strings.Contains(ev.Prompt, "implementation agent") || strings.Contains(ev.Prompt, "CHILD") {
				t.Fatalf("[%d] child pollution: %q", i, ev.Prompt)
			}
			prompts = append(prompts, ev.Prompt)
			if ev.Prompt == "done rồi hả, trả lời ok or not." {
				followPromptIdx = i
			}
		case EventMessageCompleted:
			if strings.Contains(ev.Text, "CHILD ONLY") || strings.Contains(ev.Text, "POLLUTED") {
				t.Fatalf("[%d] polluted assistant: %q", i, ev.Text)
			}
			// Hub synthesis rounds only — not the post-flow follow-up answer.
			if strings.Contains(ev.Text, "Round 0") || strings.Contains(ev.Text, "Round 1") ||
				strings.Contains(ev.Text, "Round 2") || strings.Contains(ev.Text, "changes requested") ||
				strings.Contains(ev.Text, "Consolidated review") {
				synthIdx = append(synthIdx, i)
			}
		case EventAgentSpawnedByUser:
			if firstAgentIdx < 0 {
				firstAgentIdx = i
			}
		}
	}

	if len(prompts) < 2 || prompts[0] != "fix bug 1 + 1 != 2" ||
		prompts[len(prompts)-1] != "done rồi hả, trả lời ok or not." {
		t.Fatalf("user prompts = %q, want [fix bug …, …, done rồi hả…]", prompts)
	}
	if firstAgentIdx < 0 {
		t.Fatal("no agent cards")
	}
	if firstAgentIdx == 0 {
		t.Fatalf("agents at top before original prompt: %v", summarizeEventTypes(events))
	}
	if followPromptIdx < 0 {
		t.Fatal("missing follow-up prompt")
	}
	if len(synthIdx) == 0 {
		t.Fatal("missing synthesis messages from turn log")
	}
	if followPromptIdx < synthIdx[len(synthIdx)-1] {
		t.Fatalf("follow-up at %d before last synth at %d: %v",
			followPromptIdx, synthIdx[len(synthIdx)-1], summarizeEventTypes(events))
	}
	lastMsg := -1
	for i, ev := range events {
		if ev.Type == EventMessageCompleted {
			lastMsg = i
		}
	}
	if firstAgentIdx > lastMsg {
		t.Fatalf("all agents after last message (bottom dump): %v", summarizeEventTypes(events))
	}
	if firstAgentIdx > synthIdx[0] {
		t.Fatalf("no agent card before first synthesis (agents late): firstAgent=%d firstSynth=%d full=%v",
			firstAgentIdx, synthIdx[0], summarizeEventTypes(events))
	}

	// Image 1: never park agent cards between the post-flow follow-up prompt and
	// its answer ("done rồi hả" … "ok").
	answerAfterFollow := -1
	for i := followPromptIdx + 1; i < len(events); i++ {
		if events[i].Type == EventMessageCompleted {
			answerAfterFollow = i
			break
		}
	}
	if answerAfterFollow < 0 {
		t.Fatal("missing answer after follow-up prompt")
	}
	for i := followPromptIdx + 1; i < answerAfterFollow; i++ {
		if events[i].Type == EventAgentSpawnedByUser || events[i].Type == EventAgentResultInjected {
			t.Fatalf("Image1: agent card between follow-up and answer at [%d]=%s full=%v",
				i, describeResumeEvent(events[i]), summarizeEventTypes(events))
		}
	}

	// Image 2: never two consecutive same-child coder activations without a
	// synthesis message or a different agent between them.
	assertNoConsecutiveSameChildCoderActivations(t, events)
}

// assertNoConsecutiveSameChildCoderActivations fails when spawn+result of the
// same childRunID appear twice back-to-back (coder→coder with no reviewer /
// hub synthesis between). run-24377 Image 2 residual.
func assertNoConsecutiveSameChildCoderActivations(t *testing.T, events []ProviderEvent) {
	t.Helper()
	type act struct {
		child string
		name  string
	}
	var last *act
	for i, ev := range events {
		switch ev.Type {
		case EventMessageCompleted:
			last = nil
		case EventAgentSpawnedByUser:
			name := strings.ToLower(ev.AgentName)
			isCoder := strings.Contains(name, "coder") || name == "implementation"
			if last != nil && last.child == ev.ChildRunID && last.name == "coder" && isCoder {
				t.Fatalf("Image2: consecutive coder activations for %s at [%d] (no synth/other agent between) full=%v",
					ev.ChildRunID, i, summarizeEventTypes(events))
			}
			if isCoder {
				last = &act{child: ev.ChildRunID, name: "coder"}
			} else {
				last = &act{child: ev.ChildRunID, name: "other"}
			}
		case EventAgentResultInjected:
			// keep last spawn identity through its result
		default:
			// ignore turn_completed etc.
		}
	}
}

func describeResumeEvent(ev ProviderEvent) string {
	switch ev.Type {
	case EventTurnStarted:
		p := ev.Prompt
		if len(p) > 60 {
			p = p[:60] + "…"
		}
		return fmt.Sprintf("turn_started %q", p)
	case EventMessageCompleted:
		p := ev.Text
		if len(p) > 60 {
			p = p[:60] + "…"
		}
		return fmt.Sprintf("message_completed %q", p)
	case EventAgentSpawnedByUser:
		return fmt.Sprintf("agent_spawn %s/%s", ev.AgentName, ev.ChildRunID)
	case EventAgentResultInjected:
		return fmt.Sprintf("agent_result %s/%s", ev.AgentName, ev.ChildRunID)
	case EventTurnCompleted:
		return "turn_completed"
	default:
		return string(ev.Type)
	}
}

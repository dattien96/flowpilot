package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// Cold reconstruct using REAL on-disk turn log + sessions for run-24377 when
// present (the original author's machine); otherwise falls back to the exact
// same captured incident data embedded as literals in the sibling
// run24377_live_fixture_resume_order_test.go, so this test always executes
// instead of silently no-op'ing via t.Skip on every other machine/CI (found
// during the 2026-07-22 CP-51 Phase A coverage audit).
// No desktop. Fails if timeline order is wrong.
func TestRun24377RealDiskStoreResumeOrder(t *testing.T) {
	repo := "/Users/tiendat/Desktop/flowpilot/flowpilot"
	chats := filepath.Join(repo, ".flowpilot", "chats")
	turnsPath := filepath.Join(chats, "run-24377-turns.ndjson")
	sessPath := filepath.Join(chats, "sessions.ndjson")

	type childSnap struct {
		runID, label, start, upd, msg, parent string
		turns                                 int
		status                                string
	}
	var entries []turnLogLine
	children := map[string]childSnap{}
	hubNodes := 0

	if _, statErr := os.Stat(turnsPath); statErr == nil {
		// Original author's machine: verify against the actual live capture.
		raw, err := os.ReadFile(turnsPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var o map[string]any
			if json.Unmarshal([]byte(line), &o) != nil {
				continue
			}
			kind, _ := o["kind"].(string)
			tl := turnLogLine{Kind: turnLogKind(kind)}
			if tid, ok := o["turn_id"].(string); ok {
				tl.TurnID = tid
			}
			if p, ok := o["prompt"].(string); ok {
				tl.Prompt = p
			}
			if a, ok := o["assistant"].(string); ok {
				tl.Assistant = a
			}
			if sid, ok := o["session_id"].(string); ok {
				tl.SessionID = sid
			}
			// map kinds
			switch kind {
			case "prompt":
				tl.Kind = turnLogKindPrompt
			case "transcript_turn":
				tl.Kind = turnLogKindTranscriptTurn
			case "grok_session":
				tl.Kind = turnLogKindGrokSession
			default:
				continue
			}
			entries = append(entries, tl)
		}

		// Parse real child sessions (last write wins)
		sessRaw, _ := os.ReadFile(sessPath)
		for _, line := range strings.Split(string(sessRaw), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var o map[string]any
			if json.Unmarshal([]byte(line), &o) != nil {
				continue
			}
			rid, _ := o["run_id"].(string)
			parent, _ := o["parent_run_id"].(string)
			if rid == "run-24377" {
				if nodes, ok := o["active_flow_nodes"].([]any); ok {
					hubNodes = len(nodes)
				}
				continue
			}
			if parent != "run-24377" {
				continue
			}
			label, _ := o["label"].(string)
			if label == "" {
				label, _ = o["agent_name"].(string)
			}
			st, _ := o["status"].(string)
			start, _ := o["started_at"].(string)
			upd, _ := o["updated_at"].(string)
			msg, _ := o["last_message"].(string)
			turns := 0
			switch v := o["turn_count"].(type) {
			case float64:
				turns = int(v)
			}
			children[rid] = childSnap{runID: rid, label: label, start: start, upd: upd, msg: msg, parent: parent, turns: turns, status: st}
		}
	} else {
		// Portable fallback (every other machine / CI): the exact run-24377
		// capture, transcribed as literals — same underlying incident data
		// run24377_live_fixture_resume_order_test.go already embeds.
		entries = []turnLogLine{
			{Kind: turnLogKindPrompt, TurnID: "turn-24379", Prompt: "fix bug 1 + 1 != 2"},
			{Kind: turnLogKindPrompt, TurnID: "turn-27217", Prompt: "[flow-engine joined result note]\nFlow round 0 — 2 results joined."},
			{Kind: turnLogKindGrokSession, SessionID: "019f8526-c53f-7e23-ba74-3045301e1e94"},
			{Kind: turnLogKindTranscriptTurn, TurnID: "turn-27217", Assistant: "I'll consolidate both reviewers' findings. Round 0: changes requested."},
			{Kind: turnLogKindPrompt, TurnID: "turn-29824", Prompt: "[flow-engine joined result note]\nFlow round 1 — 2 results joined."},
			{Kind: turnLogKindTranscriptTurn, TurnID: "turn-29824", Assistant: "Both reviewers agree: request changes. Round 1."},
			{Kind: turnLogKindPrompt, TurnID: "turn-31413", Prompt: "[flow-engine joined result note]\nFlow round 2 — 2 results joined."},
			{Kind: turnLogKindTranscriptTurn, TurnID: "turn-31413", Assistant: "## Consolidated review outcome\n**Submitted:** approved. Round 2."},
			{Kind: turnLogKindPrompt, TurnID: "turn-31488", Prompt: "done rồi hả, trả lời ok or not."},
			{Kind: turnLogKindTranscriptTurn, TurnID: "turn-31488", Assistant: "**ok** — flow done; correctness + security both approved."},
		}
		for _, c := range []struct {
			id, label, start, upd, msg string
			turns                      int
		}{
			{"run-24382", "coder", "2026-07-21T14:48:57.806588Z", "2026-07-21T14:56:01.672285Z", "coder done", 4},
			{"run-25068", "reviewer_correctness", "2026-07-21T14:50:25.712638Z", "2026-07-21T14:51:57.719109Z", "r0a", 1},
			{"run-25076", "reviewer_security", "2026-07-21T14:50:26.064062Z", "2026-07-21T14:51:58.366492Z", "r0b", 1},
			{"run-27697", "reviewer_correctness", "2026-07-21T14:53:03.019575Z", "2026-07-21T14:54:19.849105Z", "r1a", 1},
			{"run-27705", "reviewer_security", "2026-07-21T14:53:03.356182Z", "2026-07-21T14:54:51.618506Z", "r1b", 1},
			{"run-30114", "reviewer_correctness", "2026-07-21T14:56:01.977075Z", "2026-07-21T14:56:51.178926Z", "r2a", 1},
			{"run-30122", "reviewer_security", "2026-07-21T14:56:02.340286Z", "2026-07-21T14:56:52.71325Z", "r2b", 1},
		} {
			children[c.id] = childSnap{runID: c.id, label: c.label, start: c.start, upd: c.upd, msg: c.msg, parent: "run-24377", turns: c.turns, status: "completed"}
		}
		hubNodes = 4 // coder, reviewer_correctness, reviewer_security, synthesis
	}

	if !turnLogHasAssistantFrames(entries) {
		t.Fatal("expected assistants in real turn log")
	}
	if len(children) == 0 {
		t.Fatal("no children for run-24377 in sessions.ndjson")
	}
	t.Logf("hubNodes=%d children=%d turnLog=%d", hubNodes, len(children), len(entries))

	// Build isolated store with only this run's data (don't mutate real store)
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, "chats"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-24377", ProviderKey: ProviderKeyGrok, ProviderSessionID: "019f8526-c53f-7e23-ba74-3045301e1e94",
		ProviderAccountID: "acct-g", WorkingDirectory: "/Users/tiendat/Desktop/BE/gate-sandbox",
		RunKind: "chat", Status: RunStatusCompleted,
		LoopState:       AgentLoopState{Status: "done", Cap: 3, Mode: "explicit", Round: 2, RoundCap: 3},
		ChatFlowRef:     "flowpilot-core-flow-pack/review-loop",
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "coder"}, {ID: "reviewer_correctness"}, {ID: "reviewer_security"}, {ID: "synthesis"}},
		StartedAt:       "2026-07-21T14:48:56.194028Z", UpdatedAt: "2026-07-21T14:59:01.069956Z",
	})
	for _, c := range children {
		st := RunStatusCompleted
		if c.status != "" && c.status != "completed" {
			// keep completed for cards
		}
		_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: c.runID, ParentRunID: "run-24377", AgentName: "agent", Label: c.label,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-" + c.runID, RunKind: "chat",
			Status: st, LastMessage: c.msg, TurnCount: c.turns,
			StartedAt: c.start, UpdatedAt: c.upd,
			ProviderAccountID: "acct-g", WorkingDirectory: "/Users/tiendat/Desktop/BE/gate-sandbox",
		})
	}
	for _, e := range entries {
		if err := store.AppendTurnLog(context.Background(), "run-24377", e); err != nil {
			t.Fatal(err)
		}
	}

	// Polluted shared session file (real shape)
	grokHome := filepath.Join(root, "grok-home")
	sid := "019f8526-c53f-7e23-ba74-3045301e1e94"
	cwd := "/Users/tiendat/Desktop/BE/gate-sandbox"
	histDir := filepath.Join(grokHome, "sessions", percentEncodeGrokCwd(cwd), sid)
	_ = os.MkdirAll(histDir, 0o755)
	_ = os.WriteFile(filepath.Join(histDir, "chat_history.jsonl"), []byte(
		`{"type":"user","content":[{"type":"text","text":"<user_query>\n[FlowPilot sub-agent — agent: coder]\nchild\n</user_query>"}]}
{"type":"assistant","content":"CHILD ONLY"}
`), 0o644)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-g", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-g"
	rs := &interactiveRun{
		id: "run-24377", providerKey: ProviderKeyGrok, providerSessionID: sid,
		realProviderSessionID: sid, providerAccountID: "acct-g",
		workspaceCwd: cwd, runKind: "chat", flowEngineDriven: true,
		chatFlowRef:     "flowpilot-core-flow-pack/review-loop",
		activeFlowNodes: []agentpack.FlowNode{{ID: "coder"}, {ID: "reviewer_correctness"}, {ID: "reviewer_security"}, {ID: "synthesis"}},
		status:          RunStatusCompleted, resumedFromDisk: true,
		createdAt: "2026-07-21T14:48:56.194028Z", updatedAt: "2026-07-21T14:59:01.069956Z",
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	// Same as resumeRun for Grok
	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	t.Logf("timeline n=%d prefer=%v", len(events), svc.preferFlowHubTurnLogTranscript(rs, entries))
	for i, ev := range events {
		t.Logf("  [%02d] %s", i, describeResumeEvent(ev))
	}

	if len(events) == 0 {
		t.Fatal("empty")
	}
	if events[0].Type != EventTurnStarted || !strings.Contains(events[0].Prompt, "fix bug") {
		t.Fatalf("events[0]=%s", describeResumeEvent(events[0]))
	}
	// Must interleave: at least one message_completed between first and last agent groups
	firstAgent, lastAgent := -1, -1
	msgsBetweenAgents := 0
	for i, ev := range events {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			if firstAgent < 0 {
				firstAgent = i
			}
			lastAgent = i
		case EventMessageCompleted:
			if firstAgent >= 0 && i > firstAgent && (lastAgent < 0 || i < lastAgent || true) {
				if i > firstAgent && i < lastAgent {
					msgsBetweenAgents++
				}
			}
		}
	}
	// recount properly
	msgsBetweenAgents = 0
	seenAgent := false
	seenMsgAfterAgent := false
	seenAgentAfterMsg := false
	for _, ev := range events {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			if seenMsgAfterAgent {
				seenAgentAfterMsg = true
			}
			seenAgent = true
		case EventMessageCompleted:
			if seenAgent {
				seenMsgAfterAgent = true
				if strings.Contains(ev.Text, "Round") || strings.Contains(ev.Text, "changes requested") || strings.Contains(ev.Text, "Consolidated") {
					msgsBetweenAgents++
				}
			}
		}
	}
	if !seenAgent {
		t.Fatal("no agents")
	}
	if !seenMsgAfterAgent {
		t.Fatalf("no synthesis after agents (agents-only then nothing?): %v", summarizeEventTypes(events))
	}
	if !seenAgentAfterMsg {
		// all agents before all messages = mega-cluster / top dump of agents then synth block
		t.Fatalf("no agent after a synthesis message — agents not interleaved with rounds: %v", summarizeEventTypes(events))
	}
	// Post-flow follow-ups must appear; agents must not sit between a follow-up
	// and its answer (Image 1). Disk log may include a second follow-up after "ok".
	var userPrompts []struct {
		idx int
		p   string
	}
	for i, ev := range events {
		if ev.Type == EventTurnStarted && strings.TrimSpace(ev.Prompt) != "" && !isSystemPrompt(ev.Prompt) {
			userPrompts = append(userPrompts, struct {
				idx int
				p   string
			}{i, ev.Prompt})
		}
	}
	if len(userPrompts) < 2 {
		t.Fatalf("want ≥2 user prompts (task + follow-up), got %d: %v", len(userPrompts), userPrompts)
	}
	if !strings.Contains(userPrompts[0].p, "fix bug") {
		t.Fatalf("first prompt = %q", userPrompts[0].p)
	}
	followIdx := -1
	for _, up := range userPrompts[1:] {
		if strings.Contains(up.p, "done rồi") {
			followIdx = up.idx
			break
		}
	}
	if followIdx < 0 {
		t.Fatalf("missing 'done rồi' follow-up among %v", userPrompts)
	}
	answerAfterFollow := -1
	for i := followIdx + 1; i < len(events); i++ {
		if events[i].Type == EventMessageCompleted {
			answerAfterFollow = i
			break
		}
		if events[i].Type == EventTurnStarted {
			break
		}
	}
	if answerAfterFollow < 0 {
		t.Fatal("missing answer after 'done rồi' follow-up")
	}
	for i := followIdx + 1; i < answerAfterFollow; i++ {
		if events[i].Type == EventAgentSpawnedByUser || events[i].Type == EventAgentResultInjected {
			t.Fatalf("Image1: agent between follow-up and answer at [%d]=%s full=%v",
				i, describeResumeEvent(events[i]), summarizeEventTypes(events))
		}
	}
	// Image 2: no consecutive same-child coder activations.
	assertNoConsecutiveSameChildCoderActivations(t, events)

	// Desktop orderHistoryReplayEvents prefers timed events over untimed ones.
	// After cluster placement we must not leave agents timed and hub prose empty.
	timed, untimed := 0, 0
	for _, ev := range events {
		if strings.TrimSpace(ev.OccurredAt) == "" {
			untimed++
		} else {
			timed++
		}
	}
	if timed > 0 && untimed > 0 {
		t.Fatalf("mixed OccurredAt (timed=%d untimed=%d) will reorder agents above hub prose on desktop", timed, untimed)
	}
}

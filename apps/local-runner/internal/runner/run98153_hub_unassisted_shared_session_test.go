package runner

// run-98153: flow hub reopened mid-flow (turn log has user prompt only — no
// synthesis assistant yet). Grok shared chat_history.jsonl with children and
// foreign chats. Fall-through to provider history (run-12613) must still load
// hub-only assistant dumps, but must not surface sibling/foreign turns.
//
// additive-tests-only. cross-provider-parity Case 1: filter is provider-agnostic;
// seed paths for Grok + Claude/Codex both call it. Matrix Grok seed + unit filter
// + Claude path via same helper.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterFlowHubUnassisted_AssistantOnlyKeepsHubProse(t *testing.T) {
	// run-12613 shape: no user frames in provider file, only hub assistants.
	historical := []ProviderEvent{
		{Type: EventMessageCompleted, Text: "Parent hub: round one requests changes."},
		{Type: EventMessageCompleted, Text: "Parent hub: round two is approved."},
	}
	got := filterFlowHubUnassistedProviderHistory(historical, []string{"fix bug 1+1 !=2"})
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 assistants: %+v", len(got), got)
	}
}

func TestFilterFlowHubUnassisted_PositionalRemapWhenCountsMatchNoText(t *testing.T) {
	// run-20332 parity: durable prompts ≠ provider user text, same count → keep
	// all pairs for overlayRawTurnPrompts (do not strip assistants).
	historical := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "provider first"},
		{Type: EventMessageCompleted, Text: "Round one requested changes."},
		{Type: EventTurnStarted, Prompt: "provider second"},
		{Type: EventMessageCompleted, Text: "Round two approved the fix."},
	}
	allowed := []string{"fix the first issue", "verify the second issue"}
	got := filterFlowHubUnassistedProviderHistory(historical, allowed)
	if len(got) != 4 {
		t.Fatalf("positional remap must keep full hub history, got %d: %+v", len(got), got)
	}
	// Pollution: more provider users than durable prompts → text mode, drop all
	// non-matching (no text match → empty keep windows).
	polluted := append(append([]ProviderEvent(nil), historical...),
		ProviderEvent{Type: EventTurnStarted, Prompt: "foreign chat"},
		ProviderEvent{Type: EventMessageCompleted, Text: "FOREIGN ASSISTANT"},
	)
	got2 := filterFlowHubUnassistedProviderHistory(polluted, allowed)
	for _, e := range got2 {
		if e.Type == EventMessageCompleted && strings.Contains(e.Text, "FOREIGN") {
			t.Fatalf("extra foreign must not use positional keep: %+v", got2)
		}
	}
	if len(got2) != 0 {
		// no durable text match and count mismatch → nothing kept
		t.Fatalf("unequal count without text match must drop all user tracks, got %+v", got2)
	}
}

func TestFilterFlowHubUnassisted_DropsForeignAndChildAfterAllowedPrompt(t *testing.T) {
	// run-98153 shape after userFacing: durable prompt prepended + pollution.
	historical := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
		{Type: EventMessageCompleted, Text: "HUB ONLY — keep if after allowed"},
		{Type: EventTurnStarted, Prompt: "chào, nói cho tôi biết 1 + 1 bằng mấy"},
		{Type: EventMessageCompleted, Text: "1 + 1 = 2."},
		{Type: EventTurnStarted, Prompt: "xxxxxxxxx//eit"},
		{Type: EventMessageCompleted, Text: "Review verdict: APPROVED\nCHILD REVIEW POLLUTION"},
	}
	got := filterFlowHubUnassistedProviderHistory(historical, []string{"fix bug 1+1 != 2"})
	var texts []string
	for _, e := range got {
		switch e.Type {
		case EventTurnStarted:
			texts = append(texts, "U:"+e.Prompt)
		case EventMessageCompleted:
			texts = append(texts, "A:"+e.Text)
		}
	}
	joined := strings.Join(texts, " | ")
	if !strings.Contains(joined, "U:fix bug 1+1 != 2") {
		t.Fatalf("missing durable prompt: %v", texts)
	}
	if strings.Contains(joined, "chào") || strings.Contains(joined, "CHILD REVIEW") || strings.Contains(joined, "1 + 1 = 2") {
		t.Fatalf("foreign/child pollution leaked: %v", texts)
	}
	// Hub assistant immediately after allowed prompt is kept.
	if !strings.Contains(joined, "HUB ONLY") {
		t.Fatalf("expected hub assistant after allowed prompt: %v", texts)
	}
}

func TestFilterFlowHubUnassisted_SystemChildPromptDoesNotOpenKeepWindow(t *testing.T) {
	// Filter must run before userFacing so system child prompts close keep.
	historical := []ProviderEvent{
		{Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2"},
		{Type: EventTurnStarted, Prompt: "[FlowPilot sub-agent — agent: reviewer]\nreview the code"},
		{Type: EventMessageCompleted, Text: "CHILD ASSISTANT MUST DROP"},
	}
	got := filterFlowHubUnassistedProviderHistory(historical, []string{"fix bug 1+1 != 2"})
	got = userFacingTranscriptEvents(got)
	for _, e := range got {
		if e.Type == EventMessageCompleted && strings.Contains(e.Text, "CHILD ASSISTANT") {
			t.Fatalf("child assistant kept: %+v", got)
		}
	}
}

func TestRun98153GrokHubSeedDropsSharedSessionPollution(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const (
		parentID  = "run-98153"
		sessionID = "019ffe5a-shared-reviewers"
		cwd       = "/repo/gate-sandbox"
	)
	grokHome := filepath.Join(root, "grok-home")
	histDir := filepath.Join(grokHome, "sessions", percentEncodeGrokCwd(cwd), sessionID)
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Shared file: durable hub prompt is NOT in the file; children + foreign chat are.
	polluted := strings.Join([]string{
		`{"type":"user","content":[{"type":"text","text":"<user_query>\nYou are the review agent.\n[FlowPilot sub-agent — agent: reviewer]\nreview\n</user_query>"}]}`,
		`{"type":"assistant","content":"Review verdict: APPROVED\nCHILD ONLY POLLUTION"}`,
		`{"type":"user","content":[{"type":"text","text":"<user_query>\nchào, nói cho tôi biết 1 + 1 bằng mấy\n</user_query>"}]}`,
		`{"type":"assistant","content":"1 + 1 = 2."}`,
		`{"type":"user","content":[{"type":"text","text":"<user_query>\nxxxxxxxxx//eit\n</user_query>"}]}`,
		`{"type":"assistant","content":"That message looks like accidental input."}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(histDir, "chat_history.jsonl"), []byte(polluted), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID,
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "workflow",
		Status: RunStatusRunning, LoopState: AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"},
		StartedAt: "2026-08-14T03:38:00Z", UpdatedAt: "2026-08-14T03:41:00Z",
	})
	for _, child := range []struct{ id, label string }{
		{"run-98477", "grok-review"},
		{"run-98485", "my-reviewer"},
	} {
		_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: child.id, ParentRunID: parentID, AgentName: "reviewer", Label: child.label,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID, // shared with hub
			ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "chat",
			Status: RunStatusCompleted, StartedAt: "2026-08-14T03:39:00Z", UpdatedAt: "2026-08-14T03:41:00Z",
		})
	}
	// Durable hub turn log: user prompt only (mid-flow hang — no synthesis yet).
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindPrompt, TurnID: "turn-98155", Prompt: "fix bug 1+1 != 2",
	})
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindGrokSession, SessionID: sessionID,
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-g", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-g"
	rs := &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, providerSessionID: sessionID,
		realProviderSessionID: sessionID, providerAccountID: "acct-g",
		workspaceCwd: cwd, runKind: "workflow", flowEngineDriven: true,
		status: RunStatusRunning, resumedFromDisk: true,
		createdAt: "2026-08-14T03:38:00Z",
		subs:      map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts, assistants []string
	for _, ev := range events {
		switch ev.Type {
		case EventTurnStarted:
			if p := strings.TrimSpace(ev.Prompt); p != "" {
				prompts = append(prompts, p)
			}
		case EventMessageCompleted:
			if tx := strings.TrimSpace(ev.Text); tx != "" {
				assistants = append(assistants, tx)
			}
		}
	}
	if len(prompts) != 1 || prompts[0] != "fix bug 1+1 != 2" {
		t.Fatalf("prompts = %v, want only durable hub prompt", prompts)
	}
	for _, a := range assistants {
		if strings.Contains(a, "CHILD") || strings.Contains(a, "1 + 1 = 2") || strings.Contains(a, "accidental") {
			t.Fatalf("pollution in assistants: %v", assistants)
		}
	}
}

func TestRun98153FilterDoesNotBreakPreferWhenAssistantsPresent(t *testing.T) {
	// Gate: with assistants, prefer path skips filter entirely.
	hub := &interactiveRun{id: "h", flowEngineDriven: true}
	entries := []turnLogLine{{Kind: turnLogKindTranscriptTurn, TurnID: "t", Assistant: "synth"}}
	if shouldFilterFlowHubProviderHistory(hub, entries) {
		t.Fatal("must not filter when prefer turn-log (has assistants)")
	}
	if !(&InteractiveService{}).preferFlowHubTurnLogTranscript(hub, entries) {
		t.Fatal("prefer must still win with assistants")
	}
}

func TestShouldFilterFlowHubProviderHistory_PromptOnly(t *testing.T) {
	hub := &interactiveRun{id: "h", flowEngineDriven: true}
	entries := []turnLogLine{{Kind: turnLogKindPrompt, TurnID: "t", Prompt: "fix bug"}}
	if !shouldFilterFlowHubProviderHistory(hub, entries) {
		t.Fatal("prompt-only flow hub must filter provider history")
	}
	child := &interactiveRun{id: "c", parentRunID: "h", flowEngineDriven: true}
	if shouldFilterFlowHubProviderHistory(child, entries) {
		t.Fatal("child must not use hub filter path")
	}
	plain := &interactiveRun{id: "p", flowEngineDriven: false}
	if shouldFilterFlowHubProviderHistory(plain, entries) {
		t.Fatal("plain chat must not use hub filter")
	}
}

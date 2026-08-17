package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRun104296WrappedGateReplayKeepsQAPairing covers run-104296: Grok embeds
// every prompt inside a "## History" preamble plus ask_user/spawn_agent
// reinforcement within <user_query>, so the flow-gate reprompt no longer starts
// with GateRepromptPrefix. The old HasPrefix gate-detector missed it, the gate
// slot became overlayable, and overlayRawTurnPrompts front-aligned Q2 onto the
// gate slot (Q2+A_gate), Q3 onto Q2 (Q3+A2), leaving a leftover composed
// bubble. With isGateReprompt switched to Contains, the wrapped gate is
// classified system and replay pairs Q1-A1/Q2-A2/Q3-A3.
func TestRun104296WrappedGateReplayKeepsQAPairing(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := "/workspace/gate-sandbox"
	sessionID := "01a00dca-8623-7520-a387-96703d828f25"
	q1 := "tạo 1 file tesst.txt với nội dung hello grok"
	gate := flowgateRepromptPrefixForTest()
	q2 := "dùng google drive mcp của flowpilot cho tôi biết tên gmail account là gì"
	q3 := "dùng ask_user tool để hỏi tôi về ngôn ngữ lập trình yêu thích với các option là : python-java-kotlin-go. Sau đó hỏi tiếp câu 2 là bạn có bao nhiêu năm kinh nghiệm về nó. option là 1 năm 3 năm 7 năm"

	wrap := func(p string) string {
		return "<user_query>\n## History \"grok\" (newest = truth)\n- [937f903b 2026-08] skill ai   ← truth\n\n---\n\n" + p +
			"\n\n---\nComplete the clear, unambiguous parts of the task directly — your normal tools and approval gates still apply. Only when a required decision genuinely blocks you and you cannot reasonably infer the answer, call the FlowPilot MCP tool `ask_user`. Do NOT use the native `ask_user_question` tool.\n\n---\nDo NOT use the native spawn_subagent tool."
	}
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, wrap(q1)),
		`{"type":"assistant","content":"Creating the file and gate artifacts."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, wrap(gate)),
		`{"type":"assistant","content":"Declaring the Change Contract for the gate."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, wrap(q2)),
		`{"type":"assistant","content":"Checking FlowPilot Google Drive auth tools."}`,
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, wrap(q3)),
		`{"type":"assistant","content":"Asked both ask_user questions."}`,
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
	// Faithful to run-104296: the durable turn log records the gate reprompt too
	// (bare, no tag in the historical run). isSystemPrompt must exclude it from
	// rawPrompts via the Contains-based gate detector even though it is a real
	// entry in the file.
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-104298", Prompt: q1},
		{Kind: turnLogKindPrompt, TurnID: "turn-104517", Prompt: gate},
		{Kind: turnLogKindPrompt, TurnID: "turn-104728", Prompt: q2},
		{Kind: turnLogKindPrompt, TurnID: "turn-104842", Prompt: q3},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
	} {
		if err := store.AppendTurnLog(context.Background(), "run-104296", line); err != nil {
			t.Fatalf("AppendTurnLog(%s): %v", line.Kind, err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id: "run-104296", providerKey: ProviderKeyGrok, providerAccountID: "acct-grok",
		workspaceCwd: cwd, runKind: "chat", status: RunStatusCompleted,
		createdAt: "2026-08-17T10:00:00Z", resumedFromDisk: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts []string
	for _, event := range events {
		if event.Type == EventTurnStarted && event.Prompt != "" {
			prompts = append(prompts, event.Prompt)
		}
	}
	if len(prompts) != 3 {
		t.Fatalf("replayed user prompts = %d, want 3 (gate hidden); got %q", len(prompts), prompts)
	}
	if prompts[0] != q1 || prompts[1] != q2 || prompts[2] != q3 {
		t.Fatalf("replayed prompts = %q, want Q1-A1/Q2-A2/Q3-A3 pairing [%q %q %q] (no gate, no composed leftover, no Qn+A(n-1) shift)", prompts, q1, q2, q3)
	}
	for _, p := range prompts {
		if strings.Contains(p, "Complete the clear, unambiguous parts") || strings.Contains(p, "Do NOT use the native spawn_subagent") {
			t.Fatalf("replayed prompt leaked reinforcement/system text: %q", p)
		}
	}
}

// TestSystemPromptTagStampedPromptsAreSystem proves the durable [SYSTEM_PROMPT]
// tag alone classifies a prompt as system even when Grok wraps it inside the
// "## History" preamble — the whole point of the SSOT marker. User prompts with
// ask_user/spawn_agent reinforcement must NOT be classified system.
func TestSystemPromptTagStampedPromptsAreSystem(t *testing.T) {
	gate := flowgateRepromptPrefixForTest()
	userPrompt := "dùng ask_user tool để hỏi tôi về ngôn ngữ lập trình"
	reinforced := userPrompt + "\n\n---\nComplete the clear, unambiguous parts of the task directly — call ask_user. Do NOT use the native spawn_subagent tool."

	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"stamped bare gate", systemPromptTag + "\n" + gate, true},
		{"stamped gate wrapped in history", systemPromptTag + "\n## History \"grok\" (newest = truth)\n\n---\n\n" + gate, true},
		{"stamped user prompt", systemPromptTag + "\n" + userPrompt, true},
		{"wrapped gate WITHOUT tag (legacy detector)", "## History \"grok\" (newest = truth)\n\n---\n\n" + gate, true},
		{"bare user prompt", userPrompt, false},
		{"user prompt + reinforcement", reinforced, false},
		{"history-wrapped user prompt + reinforcement", "## History \"grok\" (newest = truth)\n\n---\n\n" + reinforced, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSystemPrompt(tc.p); got != tc.want {
				t.Fatalf("isSystemPrompt(%q) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

// TestLegacySystemPromptDetectorsProviderParity keeps the pre-tag heuristics
// working for historical data across all providers (Case 1: isSystemPrompt is
// provider-agnostic — no providerKey branch). The bare markers must still
// classify regardless of which provider wrote the transcript.
func TestLegacySystemPromptDetectorsProviderParity(t *testing.T) {
	gate := flowgateRepromptPrefixForTest()
	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"bare gate reprompt", gate, true},
		{"gate reprompt with detail lines", gate + "\n\n• Missing Change Contract. Create the file(s) above now.", true},
		{"handoff envelope", handoffPromptPrefix + "\n\nSource provider: grok\nSource run: run-x", true},
		{"flow-engine agent results", "[flow-engine] Agent results ready. You must call submit_review_outcome.", true},
		{"flow-engine review brief", "[flow-engine] Review this result from node \"coder\" and report your findings.", true},
		{"flow-engine retry", "[flow-engine] Retry: member \"coder\" was stalled; continue your work.", true},
		{"flowpilot sub-agent spawn", "[FlowPilot sub-agent — coder] write the code for this step", true},
		{"agent context joined note", "[flow-engine joined result note]\nApprove\n\n---\n\n[flow-engine] Agent results ready.", true},
		{"plain chat followup", "fix bug 1+1 != 2", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSystemPrompt(tc.p); got != tc.want {
				t.Fatalf("isSystemPrompt(%q) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
}

// TestOverlayRawTurnPromptsWrappedGateSlotsDirect pins the overlay math at the
// function level: with a wrapped gate slot in the middle, overlayRawTurnPrompts
// must pair the 3 durable raw prompts to the 3 genuine user slots (Q1, Q2, Q3)
// and leave the gate slot untouched (still system text), instead of
// front-aligning Q2 onto the gate slot.
func TestOverlayRawTurnPromptsWrappedGateSlotsDirect(t *testing.T) {
	gate := flowgateRepromptPrefixForTest()
	preamble := "## History \"grok\" (newest = truth)\n\n---\n\n"
	reinforcement := "\n\n---\nComplete the clear, unambiguous parts of the task directly. Do NOT use the native spawn_subagent tool."
	q1 := "tạo 1 file tesst.txt với nội dung hello grok"
	q2 := "dùng google drive mcp của flowpilot cho tôi biết tên gmail account là gì"
	q3 := "dùng ask_user tool để hỏi tôi về ngôn ngữ lập trình yêu thích"
	// run-104296 shape: every <user_query> carries the History preamble +
	// reinforcement; ONLY the gate slot contains the gate reprompt text.
	wrappedQ1 := preamble + q1 + reinforcement
	wrappedGate := preamble + gate + reinforcement
	wrappedQ2 := preamble + q2 + reinforcement
	wrappedQ3 := preamble + q3 + reinforcement

	historical := []ProviderEvent{
		{Type: EventTurnStarted, ProviderTurnID: "turn-104298", Prompt: wrappedQ1},
		{Type: EventTurnStarted, ProviderTurnID: "turn-104517", Prompt: wrappedGate},
		{Type: EventTurnStarted, ProviderTurnID: "turn-104728", Prompt: wrappedQ2},
		{Type: EventTurnStarted, ProviderTurnID: "turn-104842", Prompt: wrappedQ3},
	}
	rawPrompts := []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-104298", Prompt: q1},
		{Kind: turnLogKindPrompt, TurnID: "turn-104728", Prompt: q2},
		{Kind: turnLogKindPrompt, TurnID: "turn-104842", Prompt: q3},
	}

	got := overlayRawGrokTurnPrompts(historical, rawPrompts)

	var prompts []string
	var ids []string
	for _, e := range got {
		if e.Type == EventTurnStarted {
			prompts = append(prompts, e.Prompt)
			ids = append(ids, e.ProviderTurnID)
		}
	}
	want := []string{q1, wrappedGate, q2, q3}
	if len(prompts) != len(want) {
		t.Fatalf("overlayed slots = %d, want %d; prompts=%q", len(prompts), len(want), prompts)
	}
	for i := range want {
		if prompts[i] != want[i] {
			t.Fatalf("slot %d = %q, want %q; full=%q", i, prompts[i], want[i], prompts)
		}
	}
	// The gate slot keeps its own identity, never relabeled with a durable prompt.
	if ids[1] != "turn-104517" {
		t.Fatalf("gate slot turn id = %q, want turn-104517 (must not be overwritten)", ids[1])
	}
}

// flowgateRepromptPrefixForTest returns the gate reprompt prefix with the
// leading sentence the runner keys on, so the test does not depend on the exact
// wording of the flowgate constant beyond what is already shared.
func flowgateRepromptPrefixForTest() string {
	return "The flow gate is asking you to add a required document before this step can complete:\n\n• Missing Change Contract. Create the required file now."
}

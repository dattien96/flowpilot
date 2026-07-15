package runner

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestResolveTelegramMessage(t *testing.T) {
	// Template set → used verbatim.
	if got := resolveTelegramMessage(telegramOutputTarget{chatID: "c", messageTemplate: "Run: {{status}}"}, "ignored result"); got != "Run: {{status}}" {
		t.Fatalf("template case = %q, want verbatim template", got)
	}
	// No template, empty result → bare default (no dangling separator).
	if got := resolveTelegramMessage(telegramOutputTarget{chatID: "c"}, "   "); got != "[FlowPilot] Run finished." {
		t.Fatalf("empty-result case = %q, want bare default", got)
	}
	// No template, with result → default header + trimmed result.
	got := resolveTelegramMessage(telegramOutputTarget{chatID: "c"}, "  did the thing  ")
	if got != "[FlowPilot] Run finished.\n\ndid the thing" {
		t.Fatalf("result case = %q", got)
	}
}

// telegramNotifyFlowFixture wires a minimal "implement -> notify -> done" flow:
// a completed agent.delegate node forwards to a telegram.notify node bound to a
// telegram.v1 OUTPUT with an explicit chatId, whose done edge settles the flow.
func telegramNotifyFlowFixture(chatID string) ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "implement", To: "notify", When: "done", Kind: "forward"},
		{From: "notify", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "notify", Behavior: "telegram.notify", ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": chatID}},
		}},
	}
	return edges, nodes
}

// TestRunTelegramNotifyNodeSendsWithBoundChatIDAndSettlesDone proves the inline
// telegram.notify path: reached via tryAdvanceFlowFromNode, it sends through the
// runner (honoring the node's own bound chatId, NOT the connected default
// channel), verifies a message_id, then follows its forward "done" edge.
func TestRunTelegramNotifyNodeSendsWithBoundChatIDAndSettlesDone(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.runner = telegramConnectedRunner(t) // connected default channel is -100123456, auto-approve ON

	const boundChat = "-100999999" // deliberately different from the connected default
	sendCount := 0
	var gotChatID, gotText string
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		sendCount++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotChatID, gotText = body["chat_id"], body["text"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":321}}`))
	})

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := telegramNotifyFlowFixture(boundChat)
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "did the work") {
		t.Fatal("expected tryAdvanceFlowFromNode to run notify->done and return true")
	}
	if sendCount != 1 {
		t.Fatalf("expected exactly one Telegram send, got %d", sendCount)
	}
	if gotChatID != boundChat {
		t.Fatalf("chat_id = %q, want the node-bound %q (per-node override must beat connected default)", gotChatID, boundChat)
	}
	if gotText == "" {
		t.Fatal("expected a non-empty message text")
	}
}

// TestRunTelegramNotifyNodeNoBindingPassesThrough verifies a telegram.notify
// node with no bound OUTPUT target sends nothing and simply follows its forward
// "done" edge — the same degrade shape as a validate node with no command.
func TestRunTelegramNotifyNodeNoBindingPassesThrough(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	// No svc.runner set on purpose: the no-binding path must not touch it.

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges := []agentpack.FlowEdge{
		{From: "implement", To: "notify", When: "done", Kind: "forward"},
		{From: "notify", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "notify", Behavior: "telegram.notify"}, // no ArtifactBindings
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "done work") {
		t.Fatal("expected pass-through notify->done to return true")
	}
}

// TestRunTelegramNotifyNodeEscalatesWhenAutoApproveOff proves the inline path
// honors the same auto-approve gate as the MCP tool: with auto-approve off no
// message is sent, and the node escalates instead of settling the flow done.
func TestRunTelegramNotifyNodeEscalatesWhenAutoApproveOff(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	instance.SetMCPBaseURL("http://127.0.0.1:4317")
	connectTestTelegramBackend(t, instance) // auto-approve defaults to false
	svc.runner = instance

	sendCount := 0
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		sendCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := telegramNotifyFlowFixture("-100123456")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "did the work") {
		t.Fatal("expected escalate path to be handled and return true")
	}
	if sendCount != 0 {
		t.Fatalf("must not send when auto-approve is off, sendCount=%d", sendCount)
	}
	if status := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; status == "done" {
		t.Fatalf("flow must not settle done when the send was blocked; loop status=%q", status)
	}
}

// TestAdvanceHubDoneThroughEdgeChainsSynthesisToNotify proves the general
// post-synthesis chaining: a hub.inline (synthesis) node whose forward "done"
// edge targets a real successor (telegram.notify) dispatches that successor —
// sending the synthesizer's own summary — instead of settling the flow at
// synthesis. This is what lets telegram.notify sit after a coding-review-
// synthesis flow and notify with the flow's summary.
func TestAdvanceHubDoneThroughEdgeChainsSynthesisToNotify(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.runner = telegramConnectedRunner(t)

	sendCount := 0
	var gotText string
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		sendCount++
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotText = body["text"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":900}}`))
	})

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges := []agentpack.FlowEdge{
		{From: "synthesis", To: "notify", When: "done", Kind: "forward"},
		{From: "notify", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "notify", Behavior: "telegram.notify", ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": "-100123456"}},
		}},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "FLOW SUMMARY LINE"})
	if !handled {
		t.Fatal("expected advanceHubDoneThroughEdge to handle synthesis->notify chaining")
	}
	if sendCount != 1 {
		t.Fatalf("expected one Telegram send, got %d", sendCount)
	}
	if !strings.Contains(gotText, "FLOW SUMMARY LINE") {
		t.Fatalf("notify text should carry the synthesizer summary, got %q", gotText)
	}
	if res.Status != "done" || svc.agentOrchestrator.loopStateFor(parent.RunID).Status != "done" {
		t.Fatalf("flow should settle done after notify->done terminal; res=%q loop=%q", res.Status, svc.agentOrchestrator.loopStateFor(parent.RunID).Status)
	}
}

// TestAdvanceHubDoneThroughEdgeNoOpForTerminalSynthesis proves the built-in
// shape (synthesis --done--> done terminal) is untouched: the helper declines
// (handled=false) so SubmitFlowControl settles via applyFlowControl exactly as
// before — no send, no behavior change.
func TestAdvanceHubDoneThroughEdgeNoOpForTerminalSynthesis(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "synthesis", To: "done", When: "done", Kind: "forward"}}
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"}}
	rs.flowEngineDriven = true
	svc.mu.Unlock()

	if _, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "x"}); handled {
		t.Fatal("synthesis pointing at the terminal must NOT be handled here — applyFlowControl settles it unchanged")
	}
}

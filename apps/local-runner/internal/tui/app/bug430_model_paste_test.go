package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-430(b) (live BUG-LIVE-UI-4): /provider devin on a live chat left
// /status + the footer showing "devin · opencode/muse-spark-1.2-…" — the
// switch response echoed the empty request model and the guard
// `if msg.Resp.Model != ""` kept the previous provider's model id. The TUI
// must fall back to the new provider's catalog default (the same defaulting
// the pre-run /provider path uses).
func TestBug430_SwitchEmptyModelFallsBackToCatalogDefault(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode/muse-spark-1.2-contributor-free"
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode/muse-spark-1.2-contributor-free"}}},
		{Key: "devin", Models: []client.ProviderModel{{ID: "devin/swe-2-high"}, {ID: "devin/swe-2"}}},
	}

	m.applyChatSwitched(ChatSwitchedMsg{Resp: &client.ChatSwitchResponse{
		Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "devin"},
		Model:  "", // runner echoed the empty request model — live evidence
	}})

	if strings.Contains(m.model, "opencode") {
		t.Fatalf("stale foreign model survived the provider switch: %q", m.model)
	}
	if m.model != "devin/swe-2-high" {
		t.Fatalf("empty response model must fall back to the provider catalog default, got %q", m.model)
	}
}

// BUG-430(b) companion: when the response carries a real model it still wins —
// the fallback only covers the empty-model leg.
func TestBug430_SwitchExplicitModelStillWins(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode/muse-spark-1.2-contributor-free"
	m.providers = []client.Provider{
		{Key: "devin", Models: []client.ProviderModel{{ID: "devin/swe-2-high"}, {ID: "devin/swe-2"}}},
	}

	m.applyChatSwitched(ChatSwitchedMsg{Resp: &client.ChatSwitchResponse{
		Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "devin"},
		Model:  "devin/swe-2",
	}})

	if m.model != "devin/swe-2" {
		t.Fatalf("explicit response model must apply, got %q", m.model)
	}
}

// BUG-430(c) (live BUG-LIVE-UI-3): with a collapsed [Pasted N chars] token in
// the draft, a typed "/status" was appended to the draft — expansion made the
// input stop starting with "/" so it was sent as prompt text. The slash
// command must execute and the paste draft must survive.
func TestBug430_SlashCommandAfterPasteTokenExecutes(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.insertPasteSummary(strings.Repeat("lorem ipsum ", 400))
	m.insertInputAtCursor("/status")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)

	for _, msg := range am.messages {
		if msg.Role == "user" {
			t.Fatalf("/status was swallowed into the prompt draft and sent as a message: %+v", msg)
		}
	}
	if am.pendingPrompt != "" || am.turnSendPending {
		t.Fatal("slash command must not dispatch a turn")
	}
	if !strings.Contains(strings.TrimSpace(am.inputValue), "[Pasted") {
		t.Fatalf("the paste draft must survive the command, inputValue=%q", am.inputValue)
	}
	if len(am.pasteSegments) == 0 {
		t.Fatal("paste segments must survive the command")
	}
}

// BUG-430(c) companion: a draft that is paste token + ordinary text is still a
// prompt — the command intercept must only fire when the non-token remainder
// is itself a slash line.
func TestBug430_PasteTokenPlusTextStillSendsPrompt(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.insertPasteSummary(strings.Repeat("lorem ipsum ", 400))
	m.insertInputAtCursor(" please summarize")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)

	// No run handle → the prompt parks as pendingPrompt for the start cmd.
	if am.pendingPrompt == "" && !am.turnSendPending {
		t.Fatal("paste + text must still submit as a prompt")
	}
}

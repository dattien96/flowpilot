package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// Observability for BUG-339 / BUG-329: same-provider /model should update
// m.model and the next turn must send that model. This is provider-agnostic
// for the TUI layer (model string is data, no provider branch).
func TestModelSendLogging_SameProviderUpdatesModel(t *testing.T) {
	m := New(config.ChatConfig{Provider: "opencode", Model: "opencode-go/muse-spark-1.2-contributor"}, "http://127.0.0.1:1")
	m2, _ := m.handleSlashCommand("/model opencode-go/longcat-2.0")
	am := m2.(*AppModel)
	if !strings.EqualFold(am.model, "opencode-go/longcat-2.0") {
		t.Fatalf("model after /model = %q, want longcat-2.0", am.model)
	}
	if !strings.Contains(am.View(), "Model set to") {
		t.Fatalf("expected Model set to message: %q", am.View())
	}
	// The next turn's SendTurn must carry this model — openTurnStream logs it.
	// We don't execute the tea.Cmd here; we verify the model is the one that will be sent.
	if am.model != "opencode-go/longcat-2.0" {
		t.Fatalf("next turn model mismatch")
	}
}

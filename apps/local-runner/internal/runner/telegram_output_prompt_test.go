package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestRequiredTelegramOutputTargetsExtractsRequiredOutputBindings mirrors
// TestRequiredFileArtifactOutputPathsExtractsRequiredOutputBindings for
// telegram.v1 OUTPUT bindings (Task-233).
func TestRequiredTelegramOutputTargetsExtractsRequiredOutputBindings(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": "-100123456", "messageTemplate": "Run finished: {{status}}"}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: false, ConfigJSON: map[string]any{"chatId": "-100999999"}},
			{Direction: "input", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": "-100888888"}},
			{Direction: "output", ArtifactTypeID: ArtifactTypeFile, Required: true, ConfigJSON: map[string]any{"paths": []any{"a.md"}}},
		},
	}
	got := requiredTelegramOutputTargets(node)
	if len(got) != 1 || got[0].chatID != "-100123456" || got[0].messageTemplate != "Run finished: {{status}}" {
		t.Fatalf("got %#v, want exactly one target for chat -100123456", got)
	}
}

// TestRequiredTelegramOutputTargetsSkipsMissingChatID verifies a binding
// without a chatId is not treated as a send target (nothing to send to).
func TestRequiredTelegramOutputTargetsSkipsMissingChatID(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{}},
		},
	}
	if got := requiredTelegramOutputTargets(node); len(got) != 0 {
		t.Fatalf("expected no targets without a chatId, got %#v", got)
	}
}

func TestAppendTelegramOutputPromptListsChatAndTemplate(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": "-100123456", "messageTemplate": "Run finished: {{status}}"}},
		},
	}
	got := appendTelegramOutputPrompt("base", node)
	if !strings.Contains(got, "sending a Telegram notification") {
		t.Fatalf("expected write-contract heading, got %q", got)
	}
	if !strings.Contains(got, "-100123456") {
		t.Fatalf("expected chat id in prompt, got %q", got)
	}
	if !strings.Contains(got, "Run finished: {{status}}") {
		t.Fatalf("expected message template in prompt, got %q", got)
	}
	if !strings.Contains(got, "send_message") || !strings.Contains(got, "`flowpilot_telegram`") {
		t.Fatalf("expected send_message tool + telegram server name mentioned, got %q", got)
	}
	if !strings.Contains(got, "message_id") {
		t.Fatalf("expected message_id evidence requirement mentioned, got %q", got)
	}
}

func TestAppendTelegramOutputPromptNoOpWithoutBinding(t *testing.T) {
	node := agentpack.FlowNode{}
	got := appendTelegramOutputPrompt("base", node)
	if got != "base" {
		t.Fatalf("expected prompt unchanged without a telegram OUTPUT binding, got %q", got)
	}
}

// TestComposeFlowNodeAgentPromptIncludesTelegramWriteContract verifies the
// seam wiring (composeFlowNodeAgentPrompt), not just the standalone helper.
func TestComposeFlowNodeAgentPromptIncludesTelegramWriteContract(t *testing.T) {
	node := agentpack.FlowNode{
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{Direction: "output", ArtifactTypeID: ArtifactTypeTelegram, Required: true, ConfigJSON: map[string]any{"chatId": "-100123456"}},
		},
	}
	got := composeFlowNodeAgentPrompt(t.TempDir(), "base prompt", node)
	if !strings.Contains(got, "sending a Telegram notification") {
		t.Fatalf("expected composeFlowNodeAgentPrompt to include the Telegram write contract, got %q", got)
	}
}

package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func connectTestTelegramBackend(t *testing.T, instance *Runner) {
	t.Helper()
	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "telegram",
		Action:       "test",
		BotToken:     "123456:ABC-fake-token",
		ChannelID:    "-100123456",
	})
	if err != nil {
		t.Fatalf("connect test telegram backend: %v", err)
	}
}

func TestEnsureTelegramMcpProviderConfigDispatchesToClaudeInProduction(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	accountHome := t.TempDir()

	resp, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{
		ProviderKey:     "claude",
		AccountHomePath: accountHome,
	})
	if err != nil {
		t.Fatalf("EnsureTelegramMcpProviderConfig: %v", err)
	}
	if resp.ServerName != telegramMcpServerName || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestEnsureTelegramMcpProviderConfigRejectsUnsupportedProvider(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	if _, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{
		ProviderKey:     "unknown-provider",
		AccountHomePath: t.TempDir(),
	}); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

// TestEnsureTelegramMcpProviderConfigDispatchesToCodexGeminiGrok verifies
// Task-234 T-2: Telegram's stdio shape round-trips into all three remaining
// provider config formats.
func TestEnsureTelegramMcpProviderConfigDispatchesToCodexGeminiGrok(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)

	codexHome := t.TempDir()
	if _, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{ProviderKey: "codex", AccountHomePath: codexHome}); err != nil {
		t.Fatalf("EnsureTelegramMcpProviderConfig(codex): %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "config.toml")); err != nil {
		t.Fatalf("expected codex config.toml written: %v", err)
	}

	geminiHome := t.TempDir()
	if _, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{ProviderKey: "gemini", AccountHomePath: geminiHome}); err != nil {
		t.Fatalf("EnsureTelegramMcpProviderConfig(gemini): %v", err)
	}
	if _, err := os.Stat(filepath.Join(geminiHome, ".gemini", "settings.json")); err != nil {
		t.Fatalf("expected gemini settings.json written: %v", err)
	}

	grokHome := t.TempDir()
	if _, err := instance.EnsureTelegramMcpProviderConfig(TelegramMcpProviderConfigRequest{ProviderKey: "grok", AccountHomePath: grokHome}); err != nil {
		t.Fatalf("EnsureTelegramMcpProviderConfig(grok): %v", err)
	}
	if _, err := os.Stat(filepath.Join(grokHome, "config.toml")); err != nil {
		t.Fatalf("expected grok config.toml written: %v", err)
	}
}

func TestValidateTelegramCredentialFieldsRejectsMissing(t *testing.T) {
	if err := validateTelegramCredentialFields("", "chat"); err == nil {
		t.Fatal("expected error for missing bot token")
	}
	if err := validateTelegramCredentialFields("token", ""); err == nil {
		t.Fatal("expected error for missing channel id")
	}
	if err := validateTelegramCredentialFields("token", "chat"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTriggerIntegrationConnectionTelegramRejectsMissingFields(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "telegram",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.IntegrationStatus != "failed" || result.RequestStatus != "rejected" {
		t.Fatalf("expected rejected/failed result, got %#v", result)
	}
}

func TestTriggerIntegrationConnectionTelegramConnectsWithValidFields(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}
	found := false
	for _, backend := range backends {
		if backend.ProviderType != "telegram" {
			continue
		}
		found = true
		if !backend.Installed || backend.State != "installed" {
			t.Fatalf("expected telegram backend installed, got %#v", backend)
		}
	}
	if !found {
		t.Fatal("expected a telegram backend in ListMcpBackends")
	}
}

func TestEnsureClaudeTelegramMcpConfigWritesStdioEntry(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	accountHome := t.TempDir()

	resp, err := instance.EnsureClaudeTelegramMcpConfig(accountHome)
	if err != nil {
		t.Fatalf("EnsureClaudeTelegramMcpConfig: %v", err)
	}
	if !resp.Changed || resp.ServerName != telegramMcpServerName {
		t.Fatalf("unexpected response: %#v", resp)
	}

	raw, err := os.ReadFile(filepath.Join(accountHome, ".claude.json"))
	if err != nil {
		t.Fatalf("read claude config: %v", err)
	}
	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("unmarshal claude config: %v", err)
	}
	server, ok := config.McpServers[telegramMcpServerName]
	if !ok {
		t.Fatalf("expected mcpServers.telegram entry, got %#v", config.McpServers)
	}
	if server.Type != "stdio" || len(server.Args) == 0 || server.Args[0] != "telegram-mcp" {
		t.Fatalf("unexpected server entry: %#v", server)
	}
}

func TestEnsureClaudeTelegramMcpConfigIsIdempotent(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	accountHome := t.TempDir()

	if _, err := instance.EnsureClaudeTelegramMcpConfig(accountHome); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	resp, err := instance.EnsureClaudeTelegramMcpConfig(accountHome)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if resp.Changed {
		t.Fatalf("expected no change on second identical ensure, got %#v", resp)
	}
}

func TestEnsureClaudeTelegramMcpConfigFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.EnsureClaudeTelegramMcpConfig(t.TempDir()); err == nil {
		t.Fatal("expected error when telegram is not connected")
	}
}

func TestPreflightTelegramMcpFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result := instance.PreflightTelegramMcp("", "")
	if result.GoogleDriveReady || strings.TrimSpace(result.ErrorMessage) == "" {
		t.Fatalf("expected not-ready with an error message, got %#v", result)
	}
}

func TestPreflightTelegramMcpReadyWhenConnectedAndProviderConfigured(t *testing.T) {
	workspace := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	accountHome := t.TempDir()
	if _, err := instance.EnsureClaudeTelegramMcpConfig(accountHome); err != nil {
		t.Fatalf("EnsureClaudeTelegramMcpConfig: %v", err)
	}

	result := instance.PreflightTelegramMcp("claude", accountHome)
	if !result.GoogleDriveReady || !result.ProviderConfigured {
		t.Fatalf("expected fully ready, got %#v", result)
	}
}

func TestResolveConnectedTelegramCredentialFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.resolveConnectedTelegramCredential(); err == nil {
		t.Fatal("expected error when telegram is not connected")
	}
}

func TestResolveConnectedTelegramCredentialReturnsStoredValues(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)
	creds, err := instance.resolveConnectedTelegramCredential()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.BotToken != "123456:ABC-fake-token" || creds.ChannelID != "-100123456" {
		t.Fatalf("unexpected credential: %#v", creds)
	}
}

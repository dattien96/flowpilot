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

// TestTriggerIntegrationConnectionTelegramRetestReusesStoredCredential guards
// against the bug where clicking "Test" on an ALREADY-CONNECTED Telegram
// integration always failed with "a bot token is required for Telegram": the
// desktop's Existing-Integrations "Test" button resends integration.configEncrypted
// verbatim, but SD-11 §6 strips botToken from that Supabase-facing config before
// it's ever read back, so the resend carries an empty botToken. The runner must
// fall back to the previously-connected credential in its own keyring instead of
// requiring the client to resend the secret (mirrors resolveJiraCredential's
// existing fallback).
func TestTriggerIntegrationConnectionTelegramRetestReusesStoredCredential(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)

	// Re-test with an empty BotToken/ChannelID, as the desktop UI does when
	// resending a stripped configEncrypted for an existing integration.
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "telegram",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("re-test telegram connection: %v", err)
	}
	if result.RequestStatus != "accepted" || result.IntegrationStatus != "connected" {
		t.Fatalf("expected re-test to succeed via stored credential, got %#v (message=%v)", result, result.Message)
	}

	creds, err := instance.loadTelegramCredential(telegramCredentialKey("integration-telegram"))
	if err != nil {
		t.Fatalf("load telegram credential after re-test: %v", err)
	}
	if creds.BotToken != "123456:ABC-fake-token" || creds.ChannelID != "-100123456" {
		t.Fatalf("expected stored credential preserved, got %+v", creds)
	}
}

// TestTriggerIntegrationConnectionTelegramRetestFailsWhenNothingStored verifies
// the fallback still rejects cleanly (not a panic/500) when there is no prior
// connection to fall back to.
func TestTriggerIntegrationConnectionTelegramRetestFailsWhenNothingStored(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram-never-connected", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "telegram",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if result.RequestStatus != "rejected" || result.IntegrationStatus != "failed" {
		t.Fatalf("expected rejected/failed result, got %#v", result)
	}
}

// TestTriggerIntegrationConnectionTelegramAutoApproveToggle verifies the
// desktop "Enable auto-approve" path: re-Test without botToken but with an
// explicit telegramAutoApprove pointer updates the keyring flag (and a plain
// re-Test without the field leaves it alone).
func TestTriggerIntegrationConnectionTelegramAutoApproveToggle(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	connectTestTelegramBackend(t, instance)

	creds, err := instance.loadTelegramCredential(telegramCredentialKey("integration-telegram"))
	if err != nil {
		t.Fatalf("load after connect: %v", err)
	}
	if creds.AutoApprove {
		t.Fatalf("default AutoApprove must be false, got %+v", creds)
	}

	enabled := true
	if _, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProviderType:        "telegram",
		Action:              "test",
		TelegramAutoApprove: &enabled,
	}); err != nil {
		t.Fatalf("enable auto-approve: %v", err)
	}
	creds, err = instance.loadTelegramCredential(telegramCredentialKey("integration-telegram"))
	if err != nil {
		t.Fatalf("load after enable: %v", err)
	}
	if !creds.AutoApprove {
		t.Fatalf("expected AutoApprove=true after explicit toggle, got %+v", creds)
	}

	// Plain re-Test (nil pointer) must not wipe the flag — desktop Test button.
	if _, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProviderType: "telegram",
		Action:       "test",
	}); err != nil {
		t.Fatalf("plain re-test: %v", err)
	}
	creds, err = instance.loadTelegramCredential(telegramCredentialKey("integration-telegram"))
	if err != nil {
		t.Fatalf("load after plain re-test: %v", err)
	}
	if !creds.AutoApprove {
		t.Fatalf("plain re-test must preserve AutoApprove=true, got %+v", creds)
	}

	disabled := false
	if _, err := instance.TriggerIntegrationConnection(context.Background(), "integration-telegram", IntegrationConnectionRequest{
		ProviderType:        "telegram",
		Action:              "test",
		TelegramAutoApprove: &disabled,
	}); err != nil {
		t.Fatalf("disable auto-approve: %v", err)
	}
	creds, err = instance.loadTelegramCredential(telegramCredentialKey("integration-telegram"))
	if err != nil {
		t.Fatalf("load after disable: %v", err)
	}
	if creds.AutoApprove {
		t.Fatalf("expected AutoApprove=false after disable, got %+v", creds)
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

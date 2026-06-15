package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestListProviderAccountsRecoversManagedCodexSlotsFromDisk(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)
	mustWriteTestFile(t, filepath.Join(homeDir, ".codexHome1", "auth.json"), `{"tokens":{"id_token":"token-2"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	accounts = filterTestAccounts(accounts, homeDir)

	codexAccounts := filterProviderAccounts(accounts, "codex")
	if len(codexAccounts) != 2 {
		t.Fatalf("expected 2 codex accounts, got %d: %#v", len(codexAccounts), codexAccounts)
	}

	if codexAccounts[0].SlotIndex != 0 || codexAccounts[0].HomePath != filepath.Join(homeDir, ".codex") {
		t.Fatalf("expected default codex account at slot 0, got %#v", codexAccounts[0])
	}
	if !codexAccounts[0].IsActive {
		t.Fatalf("expected default codex account to be active, got %#v", codexAccounts[0])
	}

	if codexAccounts[1].SlotIndex != 1 || codexAccounts[1].HomePath != filepath.Join(homeDir, ".codexHome1") {
		t.Fatalf("expected recovered codex slot 1, got %#v", codexAccounts[1])
	}
	if codexAccounts[1].AuthStatus != "connected" {
		t.Fatalf("expected recovered codex slot 1 to be connected, got %#v", codexAccounts[1])
	}
}

func TestListProviderAccountsMarksMissingManagedSlotFailed(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	missingHome := filepath.Join(homeDir, ".codexHome2")
	state := providerAccountState{
		Accounts: []ProviderAccount{
			{
				ID:          "missing-slot",
				ProviderKey: "codex",
				DisplayName: "Account 2",
				HomePath:    missingHome,
				SlotIndex:   2,
				IsActive:    true,
				AuthStatus:  "connected",
				CreatedAt:   "2026-05-30T00:00:00Z",
				ExtraEnv:    map[string]string{},
			},
		},
	}
	if err := r.saveProviderAccountState(state); err != nil {
		t.Fatalf("saveProviderAccountState() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	accounts = filterTestAccounts(accounts, homeDir)

	var failedSlot ProviderAccount
	foundFailedSlot := false
	for _, account := range accounts {
		if account.ProviderKey == "codex" && account.SlotIndex == 2 {
			failedSlot = account
			foundFailedSlot = true
			break
		}
	}
	if !foundFailedSlot {
		t.Fatalf("expected missing codex slot 2 to remain in local registry")
	}
	if failedSlot.AuthStatus != "failed" {
		t.Fatalf("expected missing codex slot 2 to be failed, got %#v", failedSlot)
	}
	if failedSlot.IsActive {
		t.Fatalf("expected missing codex slot 2 to be deactivated, got %#v", failedSlot)
	}

	defaultFoundActive := false
	for _, account := range accounts {
		if account.ProviderKey == "codex" && account.SlotIndex == 0 && account.IsActive {
			defaultFoundActive = true
		}
	}
	if !defaultFoundActive {
		t.Fatalf("expected default codex slot to become active after missing slot failure")
	}
}

func TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("USERPROFILE", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	accountHome := filepath.Join(homeDir, "real-user")
	mustWriteTestFile(t, filepath.Join(accountHome, ".claude", ".credentials.json"), `{"claudeAiOauth":{"accessToken":"token"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	state := providerAccountState{
		Accounts: []ProviderAccount{
			{
				ID:          "stored-default-claude",
				ProviderKey: "claude",
				DisplayName: "Default Account",
				HomePath:    accountHome,
				SlotIndex:   0,
				IsActive:    false,
				AuthStatus:  "failed",
				CreatedAt:   "2026-06-15T00:00:00Z",
				ExtraEnv:    map[string]string{},
			},
		},
	}
	if err := r.saveProviderAccountState(state); err != nil {
		t.Fatalf("saveProviderAccountState() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	claudeAccounts := filterProviderAccounts(accounts, "claude")
	if len(claudeAccounts) != 1 {
		t.Fatalf("expected one Claude account, got %d: %#v", len(claudeAccounts), claudeAccounts)
	}
	if claudeAccounts[0].AuthStatus != "connected" {
		t.Fatalf("expected stored Claude account to remain connected, got %#v", claudeAccounts[0])
	}
	if !claudeAccounts[0].IsActive {
		t.Fatalf("expected stored Claude account to become active, got %#v", claudeAccounts[0])
	}
}

func TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("USERPROFILE", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	accountHome := filepath.Join(homeDir, "real-user")
	mustWriteTestFile(t, filepath.Join(accountHome, ".claude", ".credentials.json"), "\xef\xbb\xbf"+`{"claudeAiOauth":{"accessToken":"token"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	state := providerAccountState{
		Accounts: []ProviderAccount{
			{
				ID:          "stored-default-claude",
				ProviderKey: "claude",
				DisplayName: "Default Account",
				HomePath:    accountHome,
				SlotIndex:   0,
				IsActive:    false,
				AuthStatus:  "failed",
				CreatedAt:   "2026-06-15T00:00:00Z",
				ExtraEnv:    map[string]string{},
			},
		},
	}
	if err := r.saveProviderAccountState(state); err != nil {
		t.Fatalf("saveProviderAccountState() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	claudeAccounts := filterProviderAccounts(accounts, "claude")
	if len(claudeAccounts) != 1 {
		t.Fatalf("expected one Claude account, got %d: %#v", len(claudeAccounts), claudeAccounts)
	}
	if claudeAccounts[0].AuthStatus != "connected" {
		t.Fatalf("expected stored Claude account to remain connected, got %#v", claudeAccounts[0])
	}
	if !claudeAccounts[0].IsActive {
		t.Fatalf("expected stored Claude account to become active, got %#v", claudeAccounts[0])
	}
}

func TestListProviderAccountsMarksClaudeMetadataOnlyAccountFailed(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("USERPROFILE", filepath.Join(homeDir, "codexHome4"))
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	accountHome := filepath.Join(homeDir, "real-user")
	mustWriteTestFile(t, filepath.Join(accountHome, ".claude.json"), `{"oauthAccount":{"emailAddress":"user@example.com"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	state := providerAccountState{
		Accounts: []ProviderAccount{
			{
				ID:          "metadata-only-claude",
				ProviderKey: "claude",
				DisplayName: "Default Account",
				HomePath:    accountHome,
				SlotIndex:   0,
				IsActive:    true,
				AuthStatus:  "connected",
				CreatedAt:   "2026-06-15T00:00:00Z",
				ExtraEnv:    map[string]string{},
			},
		},
	}
	if err := r.saveProviderAccountState(state); err != nil {
		t.Fatalf("saveProviderAccountState() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	claudeAccounts := filterProviderAccounts(accounts, "claude")
	if len(claudeAccounts) != 1 {
		t.Fatalf("expected one Claude account, got %d: %#v", len(claudeAccounts), claudeAccounts)
	}
	if claudeAccounts[0].AuthStatus != "failed" {
		t.Fatalf("expected metadata-only Claude account to fail auth, got %#v", claudeAccounts[0])
	}
	if claudeAccounts[0].IsActive {
		t.Fatalf("expected metadata-only Claude account to be inactive, got %#v", claudeAccounts[0])
	}
}

func filterProviderAccounts(accounts []ProviderAccount, providerKey string) []ProviderAccount {
	filtered := make([]ProviderAccount, 0)
	for _, account := range accounts {
		if account.ProviderKey == providerKey {
			filtered = append(filtered, account)
		}
	}
	return filtered
}

func TestDeleteProviderAccountRemovesManagedHomeDirectory(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("XDG_CONFIG_HOME", configDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)
	// Setup a managed codex account slot 1
	managedHome := filepath.Join(homeDir, ".codexHome1")
	mustWriteTestFile(t, filepath.Join(managedHome, "auth.json"), `{"tokens":{"id_token":"token-2"}}`)

	r, err := New(".")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	accounts, err := r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	accounts = filterTestAccounts(accounts, homeDir)

	codexAccounts := filterProviderAccounts(accounts, "codex")
	// Should have default (slot 0) and recovered (slot 1)
	if len(codexAccounts) != 2 {
		t.Fatalf("expected 2 codex accounts, got %d", len(codexAccounts))
	}

	var slot1Account ProviderAccount
	for _, acc := range codexAccounts {
		if acc.SlotIndex == 1 {
			slot1Account = acc
		}
	}

	if slot1Account.ID == "" {
		t.Fatalf("could not find slot 1 account")
	}

	// Delete the account
	if err := r.DeleteProviderAccount(slot1Account.ID); err != nil {
		t.Fatalf("DeleteProviderAccount() failed: %v", err)
	}

	// Verify the directory is removed
	if _, err := os.Stat(managedHome); !os.IsNotExist(err) {
		t.Fatalf("expected managed home directory to be deleted, got err=%v", err)
	}

	// Verify that listing accounts now does not recover it
	accounts, err = r.ListProviderAccounts()
	if err != nil {
		t.Fatalf("ListProviderAccounts() failed: %v", err)
	}

	accounts = filterTestAccounts(accounts, homeDir)

	codexAccounts = filterProviderAccounts(accounts, "codex")
	if len(codexAccounts) != 1 {
		t.Fatalf("expected 1 codex account after deletion, got %d", len(codexAccounts))
	}
}

func mustWriteTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
}

func filterTestAccounts(accounts []ProviderAccount, homeDir string) []ProviderAccount {
	var filtered []ProviderAccount
	cleanHome := strings.ToLower(filepath.Clean(homeDir))
	for _, acc := range accounts {
		cleanPath := strings.ToLower(filepath.Clean(acc.HomePath))
		if strings.HasPrefix(cleanPath, cleanHome) {
			filtered = append(filtered, acc)
		}
	}
	return filtered
}

func setDiscoveryTestHome(t *testing.T, homeDir string) {
	t.Helper()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("APPDATA", filepath.Join(homeDir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(homeDir, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(homeDir, ".flowpilot", "settings", "provider-accounts.json"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("GEMINI_HOME", "")
}

// TestDiscoverCodexAccountHomes_DefaultPath tests discovering Codex account at ~/.codexHome
func TestDiscoverCodexAccountHomes_DefaultPath(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a valid Codex account at ~/.codexHome with config.toml
	codexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(codexHome, "config.toml"), `[core]
version = "1.0"`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != codexHome {
		t.Fatalf("expected path %s, got %s", codexHome, paths[0])
	}
}

// TestDiscoverCodexAccountHomes_CodexDir tests discovering Codex account with .codex/ directory
func TestDiscoverCodexAccountHomes_CodexDir(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a valid Codex account at ~/.codexHome with .codex/ directory
	codexHome := filepath.Join(homeDir, ".codexHome")
	codexDir := filepath.Join(codexHome, ".codex")
	mustWriteTestFile(t, filepath.Join(codexDir, "auth.json"), `{"tokens":{"id_token":"token"}}`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != codexHome {
		t.Fatalf("expected path %s, got %s", codexHome, paths[0])
	}
}

// TestDiscoverCodexAccountHomes_LegacyCodexDir tests discovering a legacy Codex home at ~/.codex
func TestDiscoverCodexAccountHomes_LegacyCodexDir(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	legacyCodexHome := filepath.Join(homeDir, ".codex")
	mustWriteTestFile(t, filepath.Join(legacyCodexHome, "auth.json"), `{"tokens":{"id_token":"token"}}`)

	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != legacyCodexHome {
		t.Fatalf("expected legacy path %s, got %s", legacyCodexHome, paths[0])
	}
}

// TestDiscoverCodexAccountHomes_ManagedAccounts tests discovering managed Codex account slots in ~/codex-accounts/*
func TestDiscoverCodexAccountHomes_ManagedAccounts(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create managed account slots in ~/codex-accounts/
	codexAccountsDir := filepath.Join(homeDir, "codex-accounts")

	// Slot 1: with config.toml
	slot1 := filepath.Join(codexAccountsDir, "account-1")
	mustWriteTestFile(t, filepath.Join(slot1, "config.toml"), `[core]
version = "1.0"`)

	// Slot 2: with .codex/ directory
	slot2 := filepath.Join(codexAccountsDir, "account-2")
	mustWriteTestFile(t, filepath.Join(slot2, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	// Slot 3: invalid (no config)
	slot3 := filepath.Join(codexAccountsDir, "account-3")
	mustWriteTestFile(t, filepath.Join(slot3, "some-file.txt"), `no config`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 discovered paths, got %d: %v", len(paths), paths)
	}

	// Verify both valid slots are found
	found := make(map[string]bool)
	for _, path := range paths {
		found[path] = true
	}

	if !found[slot1] {
		t.Fatalf("expected to find slot 1: %s", slot1)
	}
	if !found[slot2] {
		t.Fatalf("expected to find slot 2: %s", slot2)
	}
}

// TestDiscoverCodexAccountHomes_CodexHomeEnv tests discovering Codex from CODEX_HOME environment variable
func TestDiscoverCodexAccountHomes_CodexHomeEnv(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a Codex account at custom CODEX_HOME location
	customCodexHome := filepath.Join(homeDir, "custom", "codex-home")
	mustWriteTestFile(t, filepath.Join(customCodexHome, "config.toml"), `[core]
version = "1.0"`)

	// Set CODEX_HOME env var
	t.Setenv("CODEX_HOME", customCodexHome)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != customCodexHome {
		t.Fatalf("expected path %s, got %s", customCodexHome, paths[0])
	}
}

// TestDiscoverCodexAccountHomes_NoAccounts tests discovering when no accounts exist
func TestDiscoverCodexAccountHomes_NoAccounts(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)
	// Don't set CODEX_HOME

	// Discover accounts - should find nothing
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 0 {
		t.Fatalf("expected 0 discovered paths, got %d: %v", len(paths), paths)
	}
}

// TestDiscoverGeminiAccountHomes_DefaultPath tests discovering Gemini account
func TestDiscoverGeminiAccountHomes_DefaultPath(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a valid Gemini account at ~/.gemini/settings.json
	geminiDir := filepath.Join(homeDir, ".gemini")
	mustWriteTestFile(t, filepath.Join(geminiDir, "settings.json"), `{"user":"test"}`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != homeDir {
		t.Fatalf("expected path %s, got %s", homeDir, paths[0])
	}
}

// TestDiscoverClaudeAccountHomes_DefaultPath tests discovering Claude account
func TestDiscoverClaudeAccountHomes_DefaultPath(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a valid Claude account at ~/.claude.json
	mustWriteTestFile(t, filepath.Join(homeDir, ".claude.json"), `{"version":"1"}`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("claude")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 discovered path, got %d: %v", len(paths), paths)
	}

	if paths[0] != homeDir {
		t.Fatalf("expected path %s, got %s", homeDir, paths[0])
	}
}

// TestDiscoverProviderAccountHomes_InvalidProvider tests error handling for unsupported provider
func TestDiscoverProviderAccountHomes_InvalidProvider(t *testing.T) {
	paths, err := DiscoverProviderAccountHomes("invalid")
	if err == nil {
		t.Fatalf("expected error for unsupported provider, got nil")
	}

	if len(paths) != 0 {
		t.Fatalf("expected empty paths for invalid provider, got %v", paths)
	}
}

// =============================================================================
// Validation Function Tests
// =============================================================================

// TestIsValidCodexAccountPath_WithConfigToml tests that config.toml presence validates a path
func TestIsValidCodexAccountPath_WithConfigToml(t *testing.T) {
	homeDir := t.TempDir()

	// Create config.toml
	mustWriteTestFile(t, filepath.Join(homeDir, "config.toml"), `[core]
version = "1.0"`)

	if !isValidCodexAccountPath(homeDir) {
		t.Fatalf("expected path with config.toml to be valid, got invalid")
	}
}

// TestIsValidCodexAccountPath_WithCodexDir tests that .codex/ directory validates a path
func TestIsValidCodexAccountPath_WithCodexDir(t *testing.T) {
	homeDir := t.TempDir()

	// Create .codex/ directory with some content
	codexDir := filepath.Join(homeDir, ".codex")
	mustWriteTestFile(t, filepath.Join(codexDir, "auth.json"), `{"tokens":{"id_token":"token"}}`)

	if !isValidCodexAccountPath(homeDir) {
		t.Fatalf("expected path with .codex/ directory to be valid, got invalid")
	}
}

// TestIsValidCodexAccountPath_WithBoth tests path with both config.toml and .codex/ directory
func TestIsValidCodexAccountPath_WithBoth(t *testing.T) {
	homeDir := t.TempDir()

	// Create both config.toml and .codex/ directory
	mustWriteTestFile(t, filepath.Join(homeDir, "config.toml"), `[core]
version = "1.0"`)
	mustWriteTestFile(t, filepath.Join(homeDir, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	if !isValidCodexAccountPath(homeDir) {
		t.Fatalf("expected path with both config.toml and .codex/ to be valid, got invalid")
	}
}

// TestIsValidCodexAccountPath_InvalidPathNoConfig tests that path without config is invalid
func TestIsValidCodexAccountPath_InvalidPathNoConfig(t *testing.T) {
	homeDir := t.TempDir()

	// Create directory but no config files
	if isValidCodexAccountPath(homeDir) {
		t.Fatalf("expected path without config to be invalid, got valid")
	}
}

// TestIsValidCodexAccountPath_InvalidPathNonexistent tests that nonexistent path is invalid
func TestIsValidCodexAccountPath_InvalidPathNonexistent(t *testing.T) {
	nonexistentPath := "/nonexistent/path/that/does/not/exist"

	if isValidCodexAccountPath(nonexistentPath) {
		t.Fatalf("expected nonexistent path to be invalid, got valid")
	}
}

// TestIsValidCodexAccountPath_InvalidPathIsFile tests that a file path (not directory) is invalid
func TestIsValidCodexAccountPath_InvalidPathIsFile(t *testing.T) {
	homeDir := t.TempDir()

	// Create a file, not a directory
	filePath := filepath.Join(homeDir, "notadir")
	mustWriteTestFile(t, filePath, "content")

	if isValidCodexAccountPath(filePath) {
		t.Fatalf("expected file path to be invalid, got valid")
	}
}

// TestIsValidCodexAccountPath_EmptyDirectory tests that empty directory is invalid
func TestIsValidCodexAccountPath_EmptyDirectory(t *testing.T) {
	homeDir := t.TempDir()

	if isValidCodexAccountPath(homeDir) {
		t.Fatalf("expected empty directory to be invalid, got valid")
	}
}

// TestIsValidGeminiAccountPath_WithSettingsJson tests that .gemini/settings.json validates a path
func TestIsValidGeminiAccountPath_WithSettingsJson(t *testing.T) {
	homeDir := t.TempDir()

	// Create .gemini/settings.json
	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{"user":"test"}`)

	if !isValidGeminiAccountPath(homeDir) {
		t.Fatalf("expected path with .gemini/settings.json to be valid, got invalid")
	}
}

func TestIsValidGeminiAccountPath_EmptyGeminiDirectoryInvalid(t *testing.T) {
	homeDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(homeDir, ".gemini"), 0o755); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}

	if isValidGeminiAccountPath(homeDir) {
		t.Fatalf("expected path with empty .gemini directory to be invalid, got valid")
	}
}

// TestIsValidGeminiAccountPath_WithGeminiDir tests that valid Gemini config files validate a path
func TestIsValidGeminiAccountPath_WithGeminiDir(t *testing.T) {
	homeDir := t.TempDir()

	// Create .gemini/ directory with valid oauth config
	geminiDir := filepath.Join(homeDir, ".gemini")
	mustWriteTestFile(t, filepath.Join(geminiDir, "oauth.json"), `{"tokens":{"access_token":"token"}}`)

	if !isValidGeminiAccountPath(homeDir) {
		t.Fatalf("expected path with .gemini/ directory to be valid, got invalid")
	}
}

func TestIsValidGeminiAccountPath_ArbitraryJSONInvalid(t *testing.T) {
	testCases := []struct {
		name    string
		relPath string
		content string
	}{
		{name: "settings arbitrary object", relPath: filepath.Join(".gemini", "settings.json"), content: `{"foo":"bar"}`},
		{name: "settings array payload", relPath: filepath.Join(".gemini", "settings.json"), content: `["x"]`},
		{name: "oauth empty tokens", relPath: filepath.Join(".gemini", "oauth.json"), content: `{"tokens":{}}`},
		{name: "oauth nested client id wrapper", relPath: filepath.Join(".gemini", "oauth.json"), content: `{"foo":{"client_id":"x"}}`},
		{name: "oauth nested access token array wrapper", relPath: filepath.Join(".gemini", "oauth.json"), content: `{"wrapper":[{"access_token":"x"}]}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			homeDir := t.TempDir()
			mustWriteTestFile(t, filepath.Join(homeDir, tc.relPath), tc.content)

			if isValidGeminiAccountPath(homeDir) {
				t.Fatalf("expected arbitrary Gemini JSON candidate %q to be invalid", tc.content)
			}
		})
	}
}

// TestIsValidGeminiAccountPath_InvalidPathNoConfig tests that path without Gemini config is invalid
func TestIsValidGeminiAccountPath_InvalidPathNoConfig(t *testing.T) {
	homeDir := t.TempDir()

	// Create directory but no .gemini/ subdirectory
	if isValidGeminiAccountPath(homeDir) {
		t.Fatalf("expected path without .gemini config to be invalid, got valid")
	}
}

// TestIsValidGeminiAccountPath_InvalidPathNonexistent tests that nonexistent path is invalid
func TestIsValidGeminiAccountPath_InvalidPathNonexistent(t *testing.T) {
	nonexistentPath := "/nonexistent/path/that/does/not/exist"

	if isValidGeminiAccountPath(nonexistentPath) {
		t.Fatalf("expected nonexistent path to be invalid, got valid")
	}
}

// TestIsValidGeminiAccountPath_InvalidPathIsFile tests that a file path (not directory) is invalid
func TestIsValidGeminiAccountPath_InvalidPathIsFile(t *testing.T) {
	homeDir := t.TempDir()

	// Create a file, not a directory
	filePath := filepath.Join(homeDir, "notadir")
	mustWriteTestFile(t, filePath, "content")

	if isValidGeminiAccountPath(filePath) {
		t.Fatalf("expected file path to be invalid, got valid")
	}
}

// TestIsValidGeminiAccountPath_EmptyDirectory tests that empty directory is invalid
func TestIsValidGeminiAccountPath_EmptyDirectory(t *testing.T) {
	homeDir := t.TempDir()

	if isValidGeminiAccountPath(homeDir) {
		t.Fatalf("expected empty directory to be invalid, got valid")
	}
}

// TestIsValidClaudeAccountPath_WithClaudeJson tests that .claude.json validates a path
func TestIsValidClaudeAccountPath_WithClaudeJson(t *testing.T) {
	homeDir := t.TempDir()

	// Create .claude.json
	mustWriteTestFile(t, filepath.Join(homeDir, ".claude.json"), `{"version":"1"}`)

	if !isValidClaudeAccountPath(homeDir) {
		t.Fatalf("expected path with .claude.json to be valid, got invalid")
	}
}

// TestIsValidClaudeAccountPath_InvalidPathNoConfig tests that path without Claude config is invalid
func TestIsValidClaudeAccountPath_InvalidPathNoConfig(t *testing.T) {
	homeDir := t.TempDir()

	// Create directory but no .claude.json
	if isValidClaudeAccountPath(homeDir) {
		t.Fatalf("expected path without .claude.json to be invalid, got valid")
	}
}

// TestIsValidClaudeAccountPath_InvalidPathNonexistent tests that nonexistent path is invalid
func TestIsValidClaudeAccountPath_InvalidPathNonexistent(t *testing.T) {
	nonexistentPath := "/nonexistent/path/that/does/not/exist"

	if isValidClaudeAccountPath(nonexistentPath) {
		t.Fatalf("expected nonexistent path to be invalid, got valid")
	}
}

// TestIsValidClaudeAccountPath_InvalidPathIsFile tests that a file path (not directory) is invalid
func TestIsValidClaudeAccountPath_InvalidPathIsFile(t *testing.T) {
	homeDir := t.TempDir()

	// Create a file, not a directory
	filePath := filepath.Join(homeDir, "notadir")
	mustWriteTestFile(t, filePath, "content")

	if isValidClaudeAccountPath(filePath) {
		t.Fatalf("expected file path to be invalid, got valid")
	}
}

// TestIsValidClaudeAccountPath_EmptyDirectory tests that empty directory is invalid
func TestIsValidClaudeAccountPath_EmptyDirectory(t *testing.T) {
	homeDir := t.TempDir()

	if isValidClaudeAccountPath(homeDir) {
		t.Fatalf("expected empty directory to be invalid, got valid")
	}
}

// TestIsValidClaudeAccountPath_OtherJsonFilesIgnored tests that other JSON files don't validate a path
func TestIsValidClaudeAccountPath_OtherJsonFilesIgnored(t *testing.T) {
	homeDir := t.TempDir()

	// Create other JSON files but not .claude.json
	mustWriteTestFile(t, filepath.Join(homeDir, "other.json"), `{"some":"data"}`)

	if isValidClaudeAccountPath(homeDir) {
		t.Fatalf("expected path with other JSON files (not .claude.json) to be invalid, got valid")
	}
}

// =============================================================================
// Return List of Discovered Account Home Paths Tests (Codex Gemini)
// =============================================================================

// TestDiscoverCodexAccountHomes_CompleteDedupList tests that Codex discovery returns all found paths including
// CODEX_HOME, ~/.codexHome, and ~/codex-accounts/* slots with no duplicates
func TestDiscoverCodexAccountHomes_CompleteDedupList(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Setup 1: Create ~/.codexHome with config.toml (default path)
	codexDefaultHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(codexDefaultHome, "config.toml"), `[core]
version = "1.0"`)

	// Setup 2: Create ~/codex-accounts/slot1 with config.toml
	slot1 := filepath.Join(homeDir, "codex-accounts", "slot1")
	mustWriteTestFile(t, filepath.Join(slot1, "config.toml"), `[core]
version = "1.0"`)

	// Setup 3: Create ~/codex-accounts/slot2 with .codex/ directory
	slot2 := filepath.Join(homeDir, "codex-accounts", "slot2")
	mustWriteTestFile(t, filepath.Join(slot2, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	// Setup 4: Create ~/codex-accounts/invalid with no config (should not be included)
	invalidSlot := filepath.Join(homeDir, "codex-accounts", "invalid")
	mustWriteTestFile(t, filepath.Join(invalidSlot, "some-file.txt"), `no config`)

	// Setup 5: Set CODEX_HOME to custom location with config (should be included)
	customCodexHome := filepath.Join(homeDir, "custom-codex-home")
	mustWriteTestFile(t, filepath.Join(customCodexHome, "config.toml"), `[core]
version = "1.0"`)
	t.Setenv("CODEX_HOME", customCodexHome)

	// Discover all accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	// Verify we got all 4 valid paths (default home + 2 managed slots + custom CODEX_HOME)
	if len(paths) != 4 {
		t.Fatalf("expected 4 discovered paths, got %d: %v", len(paths), paths)
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, path := range paths {
		if seen[path] {
			t.Fatalf("found duplicate path: %s", path)
		}
		seen[path] = true
	}

	// Verify all expected paths are present
	expectedPaths := map[string]bool{
		codexDefaultHome: false,
		slot1:            false,
		slot2:            false,
		customCodexHome:  false,
	}

	for _, path := range paths {
		if _, exists := expectedPaths[path]; !exists {
			t.Fatalf("unexpected path discovered: %s", path)
		}
		expectedPaths[path] = true
	}

	for path, found := range expectedPaths {
		if !found {
			t.Fatalf("expected to find path: %s", path)
		}
	}

	// Verify invalid slot was not included
	for _, path := range paths {
		if path == invalidSlot {
			t.Fatalf("invalid slot should not be discovered: %s", invalidSlot)
		}
	}
}

// TestDiscoverCodexAccountHomes_OnlyValidPaths tests that only paths with proper config are included
func TestDiscoverCodexAccountHomes_OnlyValidPaths(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Setup 1: Create valid path with config.toml
	validPath1 := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(validPath1, "config.toml"), `[core]
version = "1.0"`)

	// Setup 2: Create invalid path with no config
	invalidPath1 := filepath.Join(homeDir, "codex-accounts", "no-config")
	mustWriteTestFile(t, filepath.Join(invalidPath1, "empty.txt"), ``)

	// Setup 3: Create invalid path as file, not directory
	invalidPath2 := filepath.Join(homeDir, "codex-accounts", "file-not-dir")
	mustWriteTestFile(t, invalidPath2, `not a directory`)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	// Should only find the valid path
	if len(paths) != 1 {
		t.Fatalf("expected 1 valid path, got %d: %v", len(paths), paths)
	}

	if paths[0] != validPath1 {
		t.Fatalf("expected path %s, got %s", validPath1, paths[0])
	}
}

// TestDiscoverGeminiAccountHomes_CompleteDedupList tests that Gemini discovery returns all found paths
// including GEMINI_HOME and ~/.gemini with no duplicates
func TestDiscoverGeminiAccountHomes_CompleteDedupList(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Setup 1: Create ~/.gemini/settings.json (user config)
	geminiDir := filepath.Join(homeDir, ".gemini")
	mustWriteTestFile(t, filepath.Join(geminiDir, "settings.json"), `{"user":"test"}`)

	// Setup 2: Set GEMINI_HOME to custom location with config
	customGeminiHome := filepath.Join(homeDir, "custom-gemini")
	customGeminiConfigDir := filepath.Join(customGeminiHome, ".gemini")
	mustWriteTestFile(t, filepath.Join(customGeminiConfigDir, "oauth.json"), `{"tokens":{"access_token":"token"}}`)
	t.Setenv("GEMINI_HOME", customGeminiHome)

	// Discover all accounts
	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	// Verify we got both paths
	if len(paths) != 2 {
		t.Fatalf("expected 2 discovered paths, got %d: %v", len(paths), paths)
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, path := range paths {
		if seen[path] {
			t.Fatalf("found duplicate path: %s", path)
		}
		seen[path] = true
	}

	// Verify both expected paths are present
	expectedPaths := map[string]bool{
		homeDir:          false, // User home with ~/.gemini
		customGeminiHome: false, // Custom GEMINI_HOME
	}

	for _, path := range paths {
		if _, exists := expectedPaths[path]; !exists {
			t.Fatalf("unexpected path discovered: %s", path)
		}
		expectedPaths[path] = true
	}

	for path, found := range expectedPaths {
		if !found {
			t.Fatalf("expected to find path: %s", path)
		}
	}
}

// TestDiscoverGeminiAccountHomes_OnlyValidPaths tests that only paths with proper Gemini config are included
func TestDiscoverGeminiAccountHomes_OnlyValidPaths(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Setup 1: Create valid Gemini path with .gemini/settings.json
	validGeminiHome := filepath.Join(homeDir, "valid-gemini")
	mustWriteTestFile(t, filepath.Join(validGeminiHome, ".gemini", "settings.json"), `{"user":"test"}`)
	t.Setenv("GEMINI_HOME", validGeminiHome)

	// Setup 2: Create invalid Gemini path (GEMINI_HOME without .gemini config)
	invalidGeminiHome := filepath.Join(homeDir, "invalid-gemini")
	mustWriteTestFile(t, filepath.Join(invalidGeminiHome, "some-file.txt"), `no gemini config`)

	// Override GEMINI_HOME to point to invalid path for testing
	t.Setenv("GEMINI_HOME", invalidGeminiHome)

	// Discover accounts - should only find the user home if it has valid config
	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	// Should find no paths since neither location has valid config
	if len(paths) != 0 {
		t.Fatalf("expected 0 valid paths, got %d: %v", len(paths), paths)
	}
}

// TestDiscoverCodexAndGemini_Completeness tests both Codex and Gemini discovery together
// to ensure complete path discovery with validation
func TestDiscoverCodexAndGemini_Completeness(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Setup Codex paths
	codexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(codexHome, "config.toml"), `[core]
version = "1.0"`)

	codexSlot := filepath.Join(homeDir, "codex-accounts", "account1")
	mustWriteTestFile(t, filepath.Join(codexSlot, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	// Setup Gemini paths
	geminiDir := filepath.Join(homeDir, ".gemini")
	mustWriteTestFile(t, filepath.Join(geminiDir, "settings.json"), `{"user":"test"}`)

	// Discover Codex accounts
	codexPaths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}

	if len(codexPaths) != 2 {
		t.Fatalf("expected 2 Codex paths, got %d: %v", len(codexPaths), codexPaths)
	}

	// Discover Gemini accounts
	geminiPaths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}

	if len(geminiPaths) != 1 {
		t.Fatalf("expected 1 Gemini path, got %d: %v", len(geminiPaths), geminiPaths)
	}

	// Verify Codex paths
	codexSeen := make(map[string]bool)
	for _, path := range codexPaths {
		codexSeen[path] = true
	}
	if !codexSeen[codexHome] {
		t.Fatalf("expected Codex default home in results")
	}
	if !codexSeen[codexSlot] {
		t.Fatalf("expected Codex managed slot in results")
	}

	// Verify Gemini paths
	if geminiPaths[0] != homeDir {
		t.Fatalf("expected Gemini home to be user home dir, got %s", geminiPaths[0])
	}
}

// TestDiscoverProviderAccountHomes_NoDuplicatesAcrossEnvAndDefault tests that CODEX_HOME duplicate
// of default path is not returned twice
func TestDiscoverProviderAccountHomes_NoDuplicatesAcrossEnvAndDefault(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create a Codex account at the default location
	defaultCodexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(defaultCodexHome, "config.toml"), `[core]
version = "1.0"`)

	// Set CODEX_HOME to the same path (this could happen with env vars)
	t.Setenv("CODEX_HOME", defaultCodexHome)

	// Discover accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes() failed: %v", err)
	}

	// Should only have 1 path, not 2 (no duplicates)
	if len(paths) != 1 {
		t.Fatalf("expected 1 unique path, got %d (duplicates detected): %v", len(paths), paths)
	}

	if paths[0] != defaultCodexHome {
		t.Fatalf("expected path %s, got %s", defaultCodexHome, paths[0])
	}
}

func TestDiscoverProviderAccountHomes_IncludesManagedSlotDirectoriesWithoutAuth(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	codexHome := filepath.Join(homeDir, ".codexHome1")
	geminiHome := filepath.Join(homeDir, ".geminiHome1")
	claudeHome := filepath.Join(homeDir, ".claudeHome1")

	for _, path := range []string{codexHome, geminiHome, claudeHome} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) failed: %v", path, err)
		}
	}

	codexPaths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}
	if len(codexPaths) != 1 || codexPaths[0] != codexHome {
		t.Fatalf("expected managed codex slot %q, got %v", codexHome, codexPaths)
	}

	geminiPaths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}
	if len(geminiPaths) != 1 || geminiPaths[0] != geminiHome {
		t.Fatalf("expected managed gemini slot %q, got %v", geminiHome, geminiPaths)
	}

	claudePaths, err := DiscoverProviderAccountHomes("claude")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('claude') failed: %v", err)
	}
	if len(claudePaths) != 1 || claudePaths[0] != claudeHome {
		t.Fatalf("expected managed claude slot %q, got %v", claudeHome, claudePaths)
	}
}

func TestDiscoverGeminiAccountHomes_IgnoresMetadataOnlyAndInvalidFiles(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "google_accounts.json"), `{"accounts":[{"email":"user@example.com"}]}`)

	invalidGeminiHome := filepath.Join(homeDir, "custom-gemini")
	mustWriteTestFile(t, filepath.Join(invalidGeminiHome, ".gemini", "oauth.json"), `not-json`)
	t.Setenv("GEMINI_HOME", invalidGeminiHome)

	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}

	if len(paths) != 0 {
		t.Fatalf("expected metadata-only and invalid Gemini homes to be ignored, got %v", paths)
	}
}

func TestDiscoverGeminiAccountHomes_IgnoresStructurallyEmptyJSONCandidates(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{}`)

	emptyGeminiHome := filepath.Join(homeDir, "custom-gemini")
	mustWriteTestFile(t, filepath.Join(emptyGeminiHome, ".gemini", "oauth.json"), `{"tokens":{}}`)
	t.Setenv("GEMINI_HOME", emptyGeminiHome)

	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}

	if len(paths) != 0 {
		t.Fatalf("expected structurally empty Gemini config candidates to be ignored, got %v", paths)
	}
}

func TestDiscoverGeminiAccountHomes_IgnoresArbitraryJSONCandidates(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{"foo":"bar"}`)

	arbitraryGeminiHome := filepath.Join(homeDir, "custom-gemini")
	mustWriteTestFile(t, filepath.Join(arbitraryGeminiHome, ".gemini", "oauth.json"), `{"foo":{"client_id":"x"}}`)
	t.Setenv("GEMINI_HOME", arbitraryGeminiHome)

	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}

	if len(paths) != 0 {
		t.Fatalf("expected arbitrary Gemini JSON candidates to be ignored, got %v", paths)
	}
}

func TestDiscoverProviderAccountHomes_DeduplicatesWindowsCaseVariants(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path canonicalization")
	}

	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	defaultCodexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(defaultCodexHome, "config.toml"), `[core]
version = "1.0"`)
	t.Setenv("CODEX_HOME", strings.ToUpper(defaultCodexHome))

	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 unique path for case variants, got %d: %v", len(paths), paths)
	}
	if !strings.EqualFold(paths[0], defaultCodexHome) {
		t.Fatalf("expected discovered path to match %q, got %q", defaultCodexHome, paths[0])
	}
}

// =============================================================================
// Task-Specific Requirements Verification Tests
// =============================================================================

// TestCodexDiscovery_AllRequiredLocations verifies all required Codex discovery locations:
// 1. CODEX_HOME environment variable
// 2. ~/.codexHome (default account home)
// 3. ~/codex-accounts/* (managed slots)
func TestCodexDiscovery_AllRequiredLocations(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Location 1: CODEX_HOME environment variable
	customCodexHome := filepath.Join(homeDir, "custom-codex")
	mustWriteTestFile(t, filepath.Join(customCodexHome, "config.toml"), `[core]
version = "1.0"`)
	t.Setenv("CODEX_HOME", customCodexHome)

	// Location 2: ~/.codexHome (default path)
	defaultCodexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(defaultCodexHome, "config.toml"), `[core]
version = "1.0"`)

	// Location 3: ~/codex-accounts/* (managed slots)
	slot1 := filepath.Join(homeDir, "codex-accounts", "account1")
	mustWriteTestFile(t, filepath.Join(slot1, ".codex", "auth.json"), `{"tokens":{"id_token":"token"}}`)

	slot2 := filepath.Join(homeDir, "codex-accounts", "account2")
	mustWriteTestFile(t, filepath.Join(slot2, "config.toml"), `[core]
version = "1.0"`)

	// Discover all accounts
	paths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}

	// Verify we discovered all three locations
	if len(paths) != 4 {
		t.Fatalf("expected to discover all 4 locations (CODEX_HOME, default, 2 managed slots), got %d: %v",
			len(paths), paths)
	}

	// Verify deduplication (no duplicates even if paths are similar)
	pathMap := make(map[string]int)
	for _, path := range paths {
		pathMap[path]++
	}
	for path, count := range pathMap {
		if count > 1 {
			t.Fatalf("path discovered multiple times: %s (count: %d)", path, count)
		}
	}

	// Verify all required paths are present
	requiredPaths := map[string]bool{
		customCodexHome:  false,
		defaultCodexHome: false,
		slot1:            false,
		slot2:            false,
	}
	for _, path := range paths {
		if _, exists := requiredPaths[path]; exists {
			requiredPaths[path] = true
		}
	}
	for path, found := range requiredPaths {
		if !found {
			t.Fatalf("required location not discovered: %s", path)
		}
	}
}

// TestGeminiDiscovery_AllRequiredLocations verifies all required Gemini discovery locations:
// 1. GEMINI_HOME environment variable
// 2. ~/.gemini (user config with settings.json or .gemini directory)
func TestGeminiDiscovery_AllRequiredLocations(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Location 1: GEMINI_HOME environment variable
	customGeminiHome := filepath.Join(homeDir, "custom-gemini")
	mustWriteTestFile(t, filepath.Join(customGeminiHome, ".gemini", "settings.json"), `{"user":"test"}`)
	t.Setenv("GEMINI_HOME", customGeminiHome)

	// Location 2: ~/.gemini (user config in home directory)
	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "oauth.json"), `{"tokens":{"access_token":"token"}}`)

	// Discover all accounts
	paths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}

	// Verify we discovered both required locations
	if len(paths) != 2 {
		t.Fatalf("expected to discover both GEMINI_HOME and ~/.gemini, got %d: %v", len(paths), paths)
	}

	// Verify both locations are present
	pathMap := make(map[string]bool)
	for _, path := range paths {
		pathMap[path] = true
	}

	if !pathMap[customGeminiHome] {
		t.Fatalf("GEMINI_HOME location not discovered: %s", customGeminiHome)
	}
	if !pathMap[homeDir] {
		t.Fatalf("~/.gemini location not discovered (should be user home dir): %s", homeDir)
	}
}

// TestValidPathValidation_OnlyValidPathsIncluded verifies that only valid paths are included:
// - Codex: config.toml or .codex/ directory must exist
// - Gemini: .gemini/ directory or .gemini/settings.json must exist
func TestValidPathValidation_OnlyValidPathsIncluded(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Create Codex paths
	validCodex := filepath.Join(homeDir, "valid-codex")
	mustWriteTestFile(t, filepath.Join(validCodex, "config.toml"), `[core]
version = "1.0"`)
	t.Setenv("CODEX_HOME", validCodex)

	// Create invalid Codex path (no config)
	invalidCodex := filepath.Join(homeDir, "codex-accounts", "invalid")
	mustWriteTestFile(t, filepath.Join(invalidCodex, "data.txt"), `some data`)

	// Create valid Gemini path
	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{"user":"test"}`)

	// Create invalid Gemini path (no config) - set as GEMINI_HOME
	invalidGemini := filepath.Join(homeDir, "invalid-gemini")
	mustWriteTestFile(t, filepath.Join(invalidGemini, "file.txt"), `content`)
	oldGeminiHome := os.Getenv("GEMINI_HOME")
	t.Setenv("GEMINI_HOME", invalidGemini)

	// Discover Codex - should only find valid path
	codexPaths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}
	if len(codexPaths) != 1 || codexPaths[0] != validCodex {
		t.Fatalf("Codex: only valid path should be included, got %v", codexPaths)
	}

	// Discover Gemini - should find valid path but not invalid GEMINI_HOME
	geminiPaths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}
	if len(geminiPaths) != 1 || geminiPaths[0] != homeDir {
		t.Fatalf("Gemini: only valid path (user home with .gemini config) should be included, got %v", geminiPaths)
	}

	if oldGeminiHome != "" {
		t.Setenv("GEMINI_HOME", oldGeminiHome)
	}
}

// TestNoDuplicatePathsReturned_DeduplicationWorks tests that duplicate paths are not returned
// even when multiple discovery methods find the same path
func TestNoDuplicatePathsReturned_DeduplicationWorks(t *testing.T) {
	homeDir := t.TempDir()
	setDiscoveryTestHome(t, homeDir)

	// Scenario 1: CODEX_HOME points to default ~/.codexHome
	defaultCodexHome := filepath.Join(homeDir, ".codexHome")
	mustWriteTestFile(t, filepath.Join(defaultCodexHome, "config.toml"), `[core]
version = "1.0"`)
	t.Setenv("CODEX_HOME", defaultCodexHome)

	codexPaths, err := DiscoverProviderAccountHomes("codex")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('codex') failed: %v", err)
	}
	if len(codexPaths) != 1 {
		t.Fatalf("Codex: expected 1 unique path even when CODEX_HOME=default, got %d: %v", len(codexPaths), codexPaths)
	}

	// Scenario 2: GEMINI_HOME points to user home
	mustWriteTestFile(t, filepath.Join(homeDir, ".gemini", "settings.json"), `{"user":"test"}`)
	t.Setenv("GEMINI_HOME", homeDir)

	geminiPaths, err := DiscoverProviderAccountHomes("gemini")
	if err != nil {
		t.Fatalf("DiscoverProviderAccountHomes('gemini') failed: %v", err)
	}
	if len(geminiPaths) != 1 {
		t.Fatalf("Gemini: expected 1 unique path even when GEMINI_HOME=home, got %d: %v", len(geminiPaths), geminiPaths)
	}

	// Verify deduplication across all discovered paths
	for _, path := range codexPaths {
		for _, other := range codexPaths {
			if path != other && filepath.Clean(path) == filepath.Clean(other) {
				t.Fatalf("Codex: found equivalent duplicate paths: %s and %s", path, other)
			}
		}
	}
}

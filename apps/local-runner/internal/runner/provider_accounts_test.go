package runner

import (
	"os"
	"path/filepath"
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

package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListProviderAccountsRecoversManagedCodexSlotsFromDisk(t *testing.T) {
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".config")
	t.Setenv("HOME", homeDir)
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

func mustWriteTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
}

package runner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProviderAccount struct {
	ID                  string            `json:"id"`
	ProviderKey         string            `json:"provider_key"`
	DisplayName         string            `json:"display_name"`
	HomePath            string            `json:"home_path"`
	SlotIndex           int               `json:"slot_index"`
	IsActive            bool              `json:"is_active"`
	AuthStatus          string            `json:"auth_status"`
	ProxyURL            string            `json:"proxy_url,omitempty"`
	ExtraEnv            map[string]string `json:"extra_env,omitempty"`
	CreatedAt           string            `json:"created_at"`
	LastAuthenticatedAt *string           `json:"last_authenticated_at,omitempty"`
	LastUsedAt          *string           `json:"last_used_at,omitempty"`
}

type ProviderAccountList struct {
	Accounts []ProviderAccount `json:"accounts"`
}

type providerAccountState struct {
	Accounts []ProviderAccount `json:"accounts"`
}

func (r *Runner) ListProviderAccounts() ([]ProviderAccount, error) {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return nil, err
	}

	updated, changed := r.syncProviderAccounts(state.Accounts)
	if changed {
		state.Accounts = updated
		if err := r.saveProviderAccountState(state); err != nil {
			return nil, err
		}
	}

	return updated, nil
}

func (r *Runner) ConnectProviderAccount(providerKey string) (ProviderAccount, error) {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return ProviderAccount{}, err
	}

	accounts, _ := r.syncProviderAccounts(state.Accounts)
	state.Accounts = accounts

	existingPaths := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account.ProviderKey == providerKey {
			existingPaths = append(existingPaths, account.HomePath)
		}
	}

	homePath, slotIndex, err := NextAccountHomePath(providerKey, existingPaths)
	if err != nil {
		return ProviderAccount{}, err
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return ProviderAccount{}, err
	}

	account := ProviderAccount{
		ID:          newProviderAccountID(),
		ProviderKey: providerKey,
		DisplayName: fmt.Sprintf("Account %d", slotIndex),
		HomePath:    homePath,
		SlotIndex:   slotIndex,
		IsActive:    false,
		AuthStatus:  "connecting",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		ExtraEnv:    map[string]string{},
	}

	state.Accounts = append(accounts, account)
	if err := r.saveProviderAccountState(state); err != nil {
		return ProviderAccount{}, err
	}

	if err := r.StartInteractiveAuth(providerKey, homePath); err != nil {
		state.Accounts = removeProviderAccountByID(state.Accounts, account.ID)
		_ = r.saveProviderAccountState(state)
		return ProviderAccount{}, err
	}

	return account, nil
}

func (r *Runner) VerifyProviderAccount(accountID string) (ProviderAccount, bool, error) {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return ProviderAccount{}, false, err
	}

	accounts, _ := r.syncProviderAccounts(state.Accounts)
	accountIndex := indexProviderAccountByID(accounts, accountID)
	if accountIndex < 0 {
		return ProviderAccount{}, false, errors.New("account not found")
	}

	account := accounts[accountIndex]
	verified := HasLocalAuthAtPath(account.ProviderKey, account.HomePath)
	if verified {
		account.AuthStatus = "connected"
		now := time.Now().UTC().Format(time.RFC3339Nano)
		account.LastAuthenticatedAt = &now
	} else {
		account.AuthStatus = "failed"
	}

	accounts[accountIndex] = account
	state.Accounts = accounts
	if err := r.saveProviderAccountState(state); err != nil {
		return ProviderAccount{}, false, err
	}

	return account, verified, nil
}

func (r *Runner) ActivateProviderAccount(accountID string) (ProviderAccount, error) {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return ProviderAccount{}, err
	}

	accounts, _ := r.syncProviderAccounts(state.Accounts)
	accountIndex := indexProviderAccountByID(accounts, accountID)
	if accountIndex < 0 {
		return ProviderAccount{}, errors.New("account not found")
	}

	account := accounts[accountIndex]
	if account.AuthStatus != "connected" {
		return ProviderAccount{}, errors.New("only connected accounts can be activated")
	}

	for i := range accounts {
		if accounts[i].ProviderKey == account.ProviderKey {
			accounts[i].IsActive = false
		}
	}
	account.IsActive = true
	accounts[accountIndex] = account

	state.Accounts = accounts
	if err := r.saveProviderAccountState(state); err != nil {
		return ProviderAccount{}, err
	}

	return account, nil
}

func (r *Runner) DeleteProviderAccount(accountID string) error {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return err
	}

	accounts, _ := r.syncProviderAccounts(state.Accounts)
	accountIndex := indexProviderAccountByID(accounts, accountID)
	if accountIndex < 0 {
		return errors.New("account not found")
	}

	account := accounts[accountIndex]
	if account.SlotIndex == 0 {
		return errors.New("default local account cannot be deleted; log out from the provider CLI if you want it removed")
	}

	if account.SlotIndex > 0 && account.HomePath != "" {
		base := filepath.Base(account.HomePath)
		prefix, ok := managedProviderHomePrefix(account.ProviderKey)
		if ok && strings.HasPrefix(base, prefix) {
			_ = os.RemoveAll(account.HomePath)
		}
	}

	accounts = append(accounts[:accountIndex], accounts[accountIndex+1:]...)
	if account.IsActive {
		accounts, _ = ensureActiveAccountForProvider(accounts, account.ProviderKey)
	}

	state.Accounts = accounts
	return r.saveProviderAccountState(state)
}

func (r *Runner) ResolveProviderAccount(providerKey string, accountID string) (ProviderAccount, error) {
	accounts, err := r.ListProviderAccounts()
	if err != nil {
		return ProviderAccount{}, err
	}

	providerAccounts := make([]ProviderAccount, 0)
	for _, account := range accounts {
		if account.ProviderKey == providerKey {
			providerAccounts = append(providerAccounts, account)
		}
	}

	if accountID != "" {
		for _, account := range providerAccounts {
			if account.ID == accountID && account.AuthStatus == "connected" {
				return account, nil
			}
		}
	}

	for _, account := range providerAccounts {
		if account.IsActive && account.AuthStatus == "connected" {
			return account, nil
		}
	}

	for _, account := range providerAccounts {
		if account.AuthStatus == "connected" {
			return account, nil
		}
	}

	return ProviderAccount{}, fmt.Errorf("no connected local account found for provider %q", providerKey)
}

func (r *Runner) TestProviderAccount(accountID string) (ProviderAccount, error) {
	state, err := r.loadProviderAccountState()
	if err != nil {
		return ProviderAccount{}, err
	}

	accounts, _ := r.syncProviderAccounts(state.Accounts)
	accountIndex := indexProviderAccountByID(accounts, accountID)
	if accountIndex < 0 {
		return ProviderAccount{}, errors.New("account not found")
	}

	account := accounts[accountIndex]
	if account.AuthStatus != "connected" {
		return ProviderAccount{}, errors.New("only connected accounts can be tested")
	}

	if err := r.StartInteractiveTest(account.ProviderKey, account.HomePath); err != nil {
		return ProviderAccount{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	account.LastUsedAt = &now
	accounts[accountIndex] = account
	state.Accounts = accounts
	if err := r.saveProviderAccountState(state); err != nil {
		return ProviderAccount{}, err
	}

	return account, nil
}

func (r *Runner) loadProviderAccountState() (providerAccountState, error) {
	path := providerAccountsConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return providerAccountState{Accounts: []ProviderAccount{}}, nil
		}
		return providerAccountState{}, err
	}

	var state providerAccountState
	if err := json.Unmarshal(raw, &state); err != nil {
		return providerAccountState{}, err
	}
	if state.Accounts == nil {
		state.Accounts = []ProviderAccount{}
	}
	return state, nil
}

func (r *Runner) saveProviderAccountState(state providerAccountState) error {
	path := providerAccountsConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

func (r *Runner) syncProviderAccounts(accounts []ProviderAccount) ([]ProviderAccount, bool) {
	if accounts == nil {
		accounts = []ProviderAccount{}
	}

	changed := false
	synced := make([]ProviderAccount, len(accounts))
	copy(synced, accounts)

	for _, providerKey := range []string{"codex", "claude", "gemini"} {
		defaultIndex := indexDefaultProviderAccount(synced, providerKey)
		defaultPath, hasDefault := DetectDefaultAccountHomePath(providerKey)

		if hasDefault {
			if defaultIndex < 0 {
				account := ProviderAccount{
					ID:          newProviderAccountID(),
					ProviderKey: providerKey,
					DisplayName: "Default Account",
					HomePath:    defaultPath,
					SlotIndex:   0,
					IsActive:    !hasActiveProviderAccount(synced, providerKey),
					AuthStatus:  "connected",
					CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
					ExtraEnv:    map[string]string{},
				}
				now := time.Now().UTC().Format(time.RFC3339Nano)
				account.LastAuthenticatedAt = &now
				synced = append(synced, account)
				changed = true
			} else {
				account := synced[defaultIndex]
				if account.HomePath != defaultPath {
					account.HomePath = defaultPath
					changed = true
				}
				if account.DisplayName != "Default Account" {
					account.DisplayName = "Default Account"
					changed = true
				}
				if account.AuthStatus != "connected" {
					account.AuthStatus = "connected"
					changed = true
				}
				if account.ExtraEnv == nil {
					account.ExtraEnv = map[string]string{}
					changed = true
				}
				if !hasActiveProviderAccount(synced, providerKey) {
					account.IsActive = true
					changed = true
				}
				synced[defaultIndex] = account
			}
		} else if defaultIndex >= 0 {
			account := synced[defaultIndex]
			if account.AuthStatus != "failed" {
				account.AuthStatus = "failed"
				changed = true
			}
			if account.IsActive {
				account.IsActive = false
				changed = true
			}
			synced[defaultIndex] = account
		}

		var providerChanged bool
		synced, providerChanged = syncManagedProviderAccounts(synced, providerKey)
		if providerChanged {
			changed = true
		}

		var activeChanged bool
		synced, activeChanged = ensureActiveAccountForProvider(synced, providerKey)
		if activeChanged {
			changed = true
		}
	}

	sort.SliceStable(synced, func(i, j int) bool {
		if synced[i].ProviderKey != synced[j].ProviderKey {
			return synced[i].ProviderKey < synced[j].ProviderKey
		}
		if synced[i].SlotIndex != synced[j].SlotIndex {
			return synced[i].SlotIndex < synced[j].SlotIndex
		}
		return synced[i].CreatedAt < synced[j].CreatedAt
	})

	return synced, changed
}

func ensureActiveAccountForProvider(accounts []ProviderAccount, providerKey string) ([]ProviderAccount, bool) {
	changed := false
	activeIndex := -1
	for i, account := range accounts {
		if account.ProviderKey == providerKey && account.IsActive && account.AuthStatus == "connected" {
			activeIndex = i
			break
		}
	}
	if activeIndex >= 0 {
		for i := range accounts {
			if accounts[i].ProviderKey == providerKey {
				nextActive := i == activeIndex
				if accounts[i].IsActive != nextActive {
					accounts[i].IsActive = nextActive
					changed = true
				}
			}
		}
		return accounts, changed
	}

	fallbackIndex := -1
	for i, account := range accounts {
		if account.ProviderKey == providerKey && account.AuthStatus == "connected" {
			if fallbackIndex < 0 || account.SlotIndex < accounts[fallbackIndex].SlotIndex {
				fallbackIndex = i
			}
		}
	}
	if fallbackIndex >= 0 {
		for i := range accounts {
			if accounts[i].ProviderKey == providerKey {
				nextActive := i == fallbackIndex
				if accounts[i].IsActive != nextActive {
					accounts[i].IsActive = nextActive
					changed = true
				}
			}
		}
	}
	return accounts, changed
}

func syncManagedProviderAccounts(accounts []ProviderAccount, providerKey string) ([]ProviderAccount, bool) {
	changed := false
	now := time.Now().UTC().Format(time.RFC3339Nano)

	for i := range accounts {
		account := accounts[i]
		if account.ProviderKey != providerKey || account.SlotIndex <= 0 {
			continue
		}

		if account.ExtraEnv == nil {
			account.ExtraEnv = map[string]string{}
			changed = true
		}
		if strings.TrimSpace(account.CreatedAt) == "" {
			account.CreatedAt = now
			changed = true
		}

		if HasLocalAuthAtPath(providerKey, account.HomePath) {
			if account.AuthStatus != "connected" {
				account.AuthStatus = "connected"
				changed = true
			}
			if account.LastAuthenticatedAt == nil {
				account.LastAuthenticatedAt = &now
				changed = true
			}
		} else if account.AuthStatus != "connecting" && account.AuthStatus != "failed" {
			account.AuthStatus = "failed"
			changed = true
		}

		accounts[i] = account
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return accounts, changed
	}

	prefix, ok := managedProviderHomePrefix(providerKey)
	if !ok {
		return accounts, changed
	}

	for slotIndex := 1; slotIndex < 1000; slotIndex++ {
		homePath := filepath.Join(homeDir, fmt.Sprintf("%s%d", prefix, slotIndex))
		if !HasLocalAuthAtPath(providerKey, homePath) {
			continue
		}

		accountIndex := indexProviderAccountByProviderAndSlot(accounts, providerKey, slotIndex)
		if accountIndex >= 0 {
			account := accounts[accountIndex]
			if filepath.Clean(account.HomePath) != filepath.Clean(homePath) {
				account.HomePath = homePath
				changed = true
			}
			if account.DisplayName != fmt.Sprintf("Account %d", slotIndex) {
				account.DisplayName = fmt.Sprintf("Account %d", slotIndex)
				changed = true
			}
			if account.AuthStatus != "connected" {
				account.AuthStatus = "connected"
				changed = true
			}
			if account.ExtraEnv == nil {
				account.ExtraEnv = map[string]string{}
				changed = true
			}
			if strings.TrimSpace(account.CreatedAt) == "" {
				account.CreatedAt = now
				changed = true
			}
			if account.LastAuthenticatedAt == nil {
				account.LastAuthenticatedAt = &now
				changed = true
			}
			accounts[accountIndex] = account
			continue
		}

		accounts = append(accounts, ProviderAccount{
			ID:                  newProviderAccountID(),
			ProviderKey:         providerKey,
			DisplayName:         fmt.Sprintf("Account %d", slotIndex),
			HomePath:            homePath,
			SlotIndex:           slotIndex,
			IsActive:            false,
			AuthStatus:          "connected",
			ExtraEnv:            map[string]string{},
			CreatedAt:           now,
			LastAuthenticatedAt: &now,
		})
		changed = true
	}

	return accounts, changed
}

func hasActiveProviderAccount(accounts []ProviderAccount, providerKey string) bool {
	for _, account := range accounts {
		if account.ProviderKey == providerKey && account.IsActive && account.AuthStatus == "connected" {
			return true
		}
	}
	return false
}

func indexDefaultProviderAccount(accounts []ProviderAccount, providerKey string) int {
	for i, account := range accounts {
		if account.ProviderKey == providerKey && account.SlotIndex == 0 {
			return i
		}
	}
	return -1
}

func indexProviderAccountByID(accounts []ProviderAccount, accountID string) int {
	for i, account := range accounts {
		if account.ID == accountID {
			return i
		}
	}
	return -1
}

func indexProviderAccountByProviderAndSlot(accounts []ProviderAccount, providerKey string, slotIndex int) int {
	for i, account := range accounts {
		if account.ProviderKey == providerKey && account.SlotIndex == slotIndex {
			return i
		}
	}
	return -1
}

func removeProviderAccountByID(accounts []ProviderAccount, accountID string) []ProviderAccount {
	index := indexProviderAccountByID(accounts, accountID)
	if index < 0 {
		return accounts
	}
	return append(accounts[:index], accounts[index+1:]...)
}

func providerAccountsConfigPath() string {
	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "provider-accounts.json")
	}

	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		return filepath.Join(homeDir, ".flowpilot", "settings", "provider-accounts.json")
	}

	return filepath.Join(".", ".flowpilot", "settings", "provider-accounts.json")
}

func managedProviderHomePrefix(providerKey string) (string, bool) {
	switch strings.ToLower(providerKey) {
	case "codex":
		return ".codexHome", true
	case "claude":
		return ".claudeHome", true
	case "gemini":
		return ".geminiHome", true
	default:
		return "", false
	}
}

func newProviderAccountID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("acct-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

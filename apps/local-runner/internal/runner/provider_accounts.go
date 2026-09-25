package runner

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
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
	r.invalidateProvidersCache()
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
	r.invalidateProvidersCache()
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

	// Task-439: a devin account that just verified pays the ACP PKCE/initialize
	// handshake now, in the background — the first chat turn after login must
	// not stall on cold auth. Warm THIS account (it may not be active yet).
	if verified {
		r.warmDevinAccountAsync(account, "login")
	}
	return account, verified, nil
}

func (r *Runner) ActivateProviderAccount(accountID string) (ProviderAccount, error) {
	r.invalidateProvidersCache()
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

	// Task-439: switching the active devin account warms the NEW account's ACP
	// process — the scope switch inside ensureDevinProcess reclaims the old
	// account's process, so the first turn on the new account stays warm.
	if account.ProviderKey == string(ProviderKeyDevin) {
		r.warmDevinAccountAsync(account, "account-switch")
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

// resolveAdapterAccount resolves the account an adapter factory launches
// under (Task-447): "" or the legacy "default" sentinel resolve the active
// connected account — byte-identical to the pre-pin behavior. A real account
// id is a strict pin: it must still exist and be connected, else the factory
// fails closed rather than silently binding a different account.
func (r *Runner) resolveAdapterAccount(providerKey, accountID string) (ProviderAccount, error) {
	if strings.TrimSpace(accountID) == "" || accountID == "default" {
		return r.ResolveProviderAccount(providerKey, "")
	}
	account, err := r.ResolveProviderAccount(providerKey, accountID)
	if err != nil {
		return ProviderAccount{}, err
	}
	if account.ID != accountID {
		return ProviderAccount{}, fmt.Errorf("pinned account %q for provider %q is not connected", accountID, providerKey)
	}
	return account, nil
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
			log.Printf("[provider-accounts] config not found path=%q — accounts auto-discovered, stored session IDs may be stale", path)
			return providerAccountState{Accounts: []ProviderAccount{}}, nil
		}
		return providerAccountState{}, err
	}

	var state providerAccountState
	if err := json.Unmarshal(stripUTF8BOM(raw), &state); err != nil {
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

	// Skip the write (and its log line) when the on-disk content is byte-identical.
	// Callers re-save on every ListProviderAccounts / resume even when the sync produced
	// no material change, which otherwise spammed the log and the disk during chat
	// navigation. This never skips a real change — the payload differs the moment any
	// account field does. (BUG-115)
	if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, payload) {
		return nil
	}

	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return err
	}
	log.Printf("[provider-accounts] config saved path=%q accounts=%d", path, len(state.Accounts))
	return nil
}

func (r *Runner) syncProviderAccounts(accounts []ProviderAccount) ([]ProviderAccount, bool) {
	if accounts == nil {
		accounts = []ProviderAccount{}
	}

	changed := false
	synced := make([]ProviderAccount, len(accounts))
	copy(synced, accounts)

	for _, providerKey := range []string{"codex", "claude", "gemini", "grok", "opencode", "devin"} {
		defaultIndex := indexDefaultProviderAccount(synced, providerKey)
		defaultPath, hasDefault := DetectDefaultAccountHomePath(providerKey)

		if hasDefault {
			if defaultIndex < 0 {
				account := ProviderAccount{
					ID:          deterministicProviderAccountID(providerKey, defaultPath),
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
			if HasLocalAuthAtPath(providerKey, account.HomePath) {
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
			} else {
				if account.AuthStatus != "failed" {
					account.AuthStatus = "failed"
					changed = true
				}
				if account.IsActive {
					account.IsActive = false
					changed = true
				}
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

	homeDir := preferredUserHomeDir()
	if homeDir == "" {
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
			if !samePath(account.HomePath, homePath) {
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
			ID:                  deterministicProviderAccountID(providerKey, homePath),
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
	if override := strings.TrimSpace(os.Getenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH")); override != "" {
		return filepath.Clean(override)
	}

	// BUG-385a: under `go test` the machine-global store must never resolve —
	// a bare-service test that reaches this path would read the developer's
	// real provider-accounts.json and a sync-triggered save would write it
	// back (a synthetic acct-* record leaked into the real store this way).
	// Tests that intentionally exercise the store set the env override above.
	if runningUnderGoTest() {
		return filepath.Join(os.TempDir(), "flowpilot-go-test", "provider-accounts.json")
	}

	if configDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(configDir) != "" {
		return filepath.Join(configDir, "FlowPilot", "provider-accounts.json")
	}

	if homeDir := preferredUserHomeDir(); homeDir != "" {
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
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-2).
		return ".grokHome", true
	case "opencode":
		// Appended last (CP-57 P-0/Task-302 T-2).
		return ".opencodeHome", true
	case "devin":
		// Appended last (CP-70 P-0/Task-402).
		return ".devinHome", true
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

// deterministicProviderAccountID returns a stable 32-hex-char account ID derived
// from the provider key and home path. Auto-discovered default and managed accounts
// use this so that regenerating provider-accounts.json (e.g. after a missing file)
// produces the same ID for the same physical installation, keeping stored session
// account references valid without requiring the BUG-092 recovery scan.
func deterministicProviderAccountID(providerKey, homePath string) string {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(homePath)))
	input := strings.ToLower(strings.TrimSpace(providerKey)) + ":" + strings.ToLower(normalized)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:16]) // 32 hex chars, same length as newProviderAccountID
}

// DiscoverProviderAccountHomes discovers provider account home paths by checking default locations.
// It returns valid provider homes plus existing FlowPilot-managed slot directories,
// so the UI can surface not-started accounts before authentication completes.
// For Codex, it checks:
//   - ~/.codexHome (default account path)
//   - ~/codex-accounts/* (managed account slots)
//
// Paths are validated by checking for config.toml or .codex/ directory presence.
func DiscoverProviderAccountHomes(providerKey string) ([]string, error) {
	switch strings.ToLower(providerKey) {
	case "codex":
		return discoverCodexAccountHomes()
	case "gemini":
		return discoverGeminiAccountHomes()
	case "claude":
		return discoverClaudeAccountHomes()
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-4).
		return discoverGrokAccountHomes()
	case "opencode":
		// Appended last (CP-57 P-0/Task-302 T-2).
		return discoverOpencodeAccountHomes()
	case "devin":
		// Appended last (CP-70 P-0/Task-402).
		return discoverDevinAccountHomes()
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerKey)
	}
}

func DiscoverProviderAccounts(providerKey string) ([]ProviderDiscoveredAccount, error) {
	paths, err := DiscoverProviderAccountHomes(providerKey)
	if err != nil {
		return nil, err
	}

	accounts := make([]ProviderDiscoveredAccount, 0, len(paths))
	for index, homePath := range paths {
		accounts = append(accounts, ProviderDiscoveredAccount{
			ID:       fmt.Sprintf("%s-%d", strings.ToLower(providerKey), index+1),
			HomePath: homePath,
			Label:    discoveredProviderAccountLabel(providerKey, homePath),
		})
	}

	return accounts, nil
}

// discoverCodexAccountHomes discovers Codex account home paths.
// Checks:
// - CODEX_HOME environment variable
// - ~/.codex (legacy authenticated home)
// - ~/.codexHome (default home path)
// - ~/.codexHomeN (FlowPilot-managed slots)
// - ~/codex-accounts/* (managed slots directory pattern)
// Returns valid paths where config.toml or local auth exists, plus existing
// FlowPilot-managed slot directories so they can be configured before auth.
func discoverCodexAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	// Check CODEX_HOME environment variable
	if codexHome := os.Getenv("CODEX_HOME"); strings.TrimSpace(codexHome) != "" {
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, codexHome, isValidCodexAccountPath)
	}

	// Check default paths in user home directory
	homeDir := preferredUserHomeDir()
	if homeDir != "" {
		// Check ~/.codex (legacy authenticated home path)
		accountPaths = appendDiscoveredAccountPath(
			accountPaths,
			discovered,
			filepath.Join(homeDir, ".codex"),
			isValidCodexAccountPath,
		)

		// Check ~/.codexHome (default account home path)
		accountPaths = appendDiscoveredAccountPath(
			accountPaths,
			discovered,
			filepath.Join(homeDir, ".codexHome"),
			isValidCodexAccountPath,
		)

		for _, path := range discoverManagedProviderHomeSlots(homeDir, ".codexHome", isValidCodexAccountPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
		}

		// Check ~/codex-accounts/* for managed account slots
		codexAccountsDir := filepath.Join(homeDir, "codex-accounts")
		managedPaths, err := discoverManagedCodexAccounts(codexAccountsDir)
		if err == nil {
			for _, path := range managedPaths {
				accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, isValidCodexAccountPath)
			}
		}
	}

	return accountPaths, nil
}

// discoverGeminiAccountHomes discovers Gemini account home paths.
// Checks:
// - ~/.gemini/settings.json (user config)
// - GEMINI_HOME environment variable if set
// - ~/.geminiHomeN (FlowPilot-managed slots)
// Returns valid paths where Gemini config/auth exists, plus existing
// FlowPilot-managed slot directories so they can be configured before auth.
func discoverGeminiAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	// Check GEMINI_HOME environment variable
	if geminiHome := os.Getenv("GEMINI_HOME"); strings.TrimSpace(geminiHome) != "" {
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, geminiHome, isValidGeminiAccountPath)
	}

	// Check default paths in user home directory
	homeDir := preferredUserHomeDir()
	if homeDir != "" {
		// Check ~/.gemini/ for settings.json
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, isValidGeminiAccountPath)

		for _, path := range discoverManagedProviderHomeSlots(homeDir, ".geminiHome", isValidGeminiAccountPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
		}
	}

	return accountPaths, nil
}

// discoverClaudeAccountHomes discovers Claude account home paths.
// Checks:
// - ~/.claude.json (user config)
// - ~/.claudeHomeN (FlowPilot-managed slots)
// Returns valid paths where Claude config/auth exists, plus existing
// FlowPilot-managed slot directories so they can be configured before auth.
func discoverClaudeAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	homeDir := preferredUserHomeDir()
	if homeDir == "" {
		return accountPaths, nil
	}

	// Check ~/.claude.json (user config in home dir)
	accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, isValidClaudeAccountPath)

	for _, path := range discoverManagedProviderHomeSlots(homeDir, ".claudeHome", isValidClaudeAccountPath) {
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
	}

	return accountPaths, nil
}

// discoverGrokAccountHomes discovers Grok Build account home paths.
// Checks:
//   - GROK_HOME environment variable
//   - ~/.grok (default account home — live-verified real layout: auth.json,
//     config.toml, sessions/, mcp_credentials.json)
//   - ~/.grokHomeN (FlowPilot-managed slots)
//
// Returns valid paths where auth.json/config.toml exists, plus existing
// FlowPilot-managed slot directories so they can be configured before auth
// (Task-210 T-4).
func discoverGrokAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	if grokHome := os.Getenv("GROK_HOME"); strings.TrimSpace(grokHome) != "" {
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, grokHome, isValidGrokAccountPath)
	}

	homeDir := preferredUserHomeDir()
	if homeDir != "" {
		accountPaths = appendDiscoveredAccountPath(
			accountPaths,
			discovered,
			filepath.Join(homeDir, ".grok"),
			isValidGrokAccountPath,
		)

		for _, path := range discoverManagedProviderHomeSlots(homeDir, ".grokHome", isValidGrokAccountPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
		}
	}

	return accountPaths, nil
}

// isValidGrokAccountPath checks if a path is a valid Grok Build account home
// path. Live-verified: config.toml and/or auth.json at the home root.
func isValidGrokAccountPath(homePath string) bool {
	info, err := os.Stat(homePath)
	if err != nil || !info.IsDir() {
		return false
	}

	if _, err := os.Stat(filepath.Join(homePath, "config.toml")); err == nil {
		return true
	}

	return HasLocalAuthAtPath("grok", homePath)
}

func discoverOpencodeAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	if cfg := os.Getenv("OPENCODE_CONFIG"); strings.TrimSpace(cfg) != "" {
		home := strings.TrimSpace(cfg)
		// CA-679: OPENCODE_CONFIG is now written as the config FILE
		// (.../opencode/opencode.json); older writers and operator shells may
		// still carry the directory form (.../opencode). Accept both.
		// Handle both POSIX and Windows separators.
		normalized := filepath.ToSlash(home)
		if strings.HasSuffix(normalized, "/opencode.json") {
			normalized = strings.TrimSuffix(normalized, "/opencode.json")
			home = filepath.FromSlash(normalized)
		}
		// Legacy directory style like /home/user/.config/opencode, derive home.
		normalized = filepath.ToSlash(home)
		if strings.HasSuffix(normalized, "/opencode") {
			normalized = strings.TrimSuffix(normalized, "/opencode")
			home = filepath.FromSlash(normalized)
		}
		normalized = filepath.ToSlash(home)
		if strings.HasSuffix(normalized, "/.config") {
			normalized = strings.TrimSuffix(normalized, "/.config")
			home = filepath.FromSlash(normalized)
		}
		home = strings.TrimSpace(home)
		if home != "" {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, home, isValidOpencodeAccountPath)
		}
	}
	if home := os.Getenv("OPENCODE_HOME"); strings.TrimSpace(home) != "" {
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, home, isValidOpencodeAccountPath)
	}
	homeDir := preferredUserHomeDir()
	if homeDir != "" {
		if authPath := resolveOpencodeAuthPathFromEnv(homeDir); authPath != "" && fileExists(authPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, nil)
		}
		if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
			if fileExists(filepath.Join(xdg, "opencode", "auth.json")) {
				accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, nil)
			}
		}
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, isValidOpencodeAccountPath)
		for _, path := range discoverManagedProviderHomeSlots(homeDir, ".opencodeHome", isValidOpencodeAccountPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
		}
	}

	return accountPaths, nil
}

func isValidOpencodeAccountPath(homePath string) bool {
	info, err := os.Stat(homePath)
	if err != nil || !info.IsDir() {
		return false
	}
	if opencodeConfigOrAuthFileExists(homePath) {
		return true
	}
	return HasLocalAuthAtPath("opencode", homePath)
}

// discoverDevinAccountHomes finds Devin account homes (CP-70 Task-402). Devin
// is XDG-based on unix (credentials.toml under XDG_DATA_HOME or
// XDG_CONFIG_HOME) and uses %APPDATA%\devin / %LOCALAPPDATA%\devin on Windows;
// managed slots (.devinHomeN) replicate the same subtree. Ambient env dirs
// force-add the real home when they carry a credentials file — same as the
// opencode discovery probes.
func discoverDevinAccountHomes() ([]string, error) {
	discovered := make(map[string]struct{})
	var accountPaths []string

	homeDir := preferredUserHomeDir()
	if homeDir != "" {
		for _, candidate := range devinAmbientAuthCandidates(homeDir) {
			if fileExists(candidate.authPath) {
				accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, nil)
				break
			}
		}
		accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, homeDir, isValidDevinAccountPath)
		for _, path := range discoverManagedProviderHomeSlots(homeDir, ".devinHome", isValidDevinAccountPath) {
			accountPaths = appendDiscoveredAccountPath(accountPaths, discovered, path, nil)
		}
	}

	return accountPaths, nil
}

// isValidDevinAccountPath reports whether a home path carries a Devin
// footprint: credentials.toml (REPL auth store) or the config/data dirs.
// Home-relative only — ambient env dirs belong to the host home probe in
// discoverDevinAccountHomes.
func isValidDevinAccountPath(homePath string) bool {
	info, err := os.Stat(homePath)
	if err != nil || !info.IsDir() {
		return false
	}
	candidates := append(devinCredentialFilePaths(homePath),
		filepath.Join(homePath, ".config", "devin", "config.json"),
		filepath.Join(homePath, ".config", "devin", "mcp_config.json"),
	)
	if runtime.GOOS == "windows" {
		// Windows Devin resolves config to %APPDATA%\devin and data to
		// %LOCALAPPDATA%\devin (live-verified: credentials.toml sits under
		// Roaming on a real host install), not the XDG dirs.
		candidates = append(candidates,
			filepath.Join(homePath, "AppData", "Roaming", "devin", "config.json"),
			filepath.Join(homePath, "AppData", "Roaming", "devin", "mcp_config.json"),
			filepath.Join(homePath, "AppData", "Local", "devin", "config.json"),
			filepath.Join(homePath, "AppData", "Local", "devin", "mcp_config.json"),
		)
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return true
		}
	}
	return HasLocalAuthAtPath("devin", homePath)
}

// discoverManagedCodexAccounts discovers managed Codex account slots from ~/codex-accounts/* directory.
// Each subdirectory is checked to see if it contains valid Codex config (config.toml or .codex/ dir).
// Returns a list of valid managed account paths.
func discoverManagedCodexAccounts(codexAccountsDir string) ([]string, error) {
	var accountPaths []string

	// Check if the codex-accounts directory exists
	dirInfo, err := os.Stat(codexAccountsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return accountPaths, nil // Directory doesn't exist, no managed accounts
		}
		return accountPaths, nil // Silently return empty on permission errors
	}

	if !dirInfo.IsDir() {
		return accountPaths, nil // Not a directory
	}

	// Read the directory entries
	entries, err := os.ReadDir(codexAccountsDir)
	if err != nil {
		return accountPaths, nil // Permission error or other issues, return empty
	}

	// Check each subdirectory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		accountPath := filepath.Join(codexAccountsDir, entry.Name())
		if isValidCodexAccountPath(accountPath) {
			accountPaths = append(accountPaths, accountPath)
		}
	}

	return accountPaths, nil
}

// isValidCodexAccountPath checks if a path is a valid Codex account home path.
// Returns true if the path contains config.toml or a .codex/ directory.
func isValidCodexAccountPath(homePath string) bool {
	// Check if path exists and is accessible
	info, err := os.Stat(homePath)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}

	// Check for config.toml (Codex config file)
	configPath := filepath.Join(homePath, "config.toml")
	if _, err := os.Stat(configPath); err == nil {
		return true
	}

	return HasLocalAuthAtPath("codex", homePath)
}

// isValidGeminiAccountPath checks if a path is a valid Gemini account home path.
// Returns true if the path contains valid Gemini config/auth state.
func isValidGeminiAccountPath(homePath string) bool {
	// Check if path exists and is accessible
	info, err := os.Stat(homePath)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}

	if HasLocalAuthAtPath("gemini", homePath) {
		return true
	}

	for _, path := range geminiConfigCandidatePaths(homePath) {
		if isValidJSONConfigFile(path) {
			return true
		}
	}

	return false
}

// isValidClaudeAccountPath checks if a path is a valid Claude account home path.
// Returns true if the path contains .claude.json or similar config.
func isValidClaudeAccountPath(homePath string) bool {
	// Check if path exists and is accessible
	info, err := os.Stat(homePath)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}

	if _, err := os.Stat(filepath.Join(homePath, ".claude.json")); err == nil {
		return true
	}

	return HasLocalAuthAtPath("claude", homePath)
}

func appendDiscoveredAccountPath(
	accountPaths []string,
	discovered map[string]struct{},
	candidate string,
	validator func(string) bool,
) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return accountPaths
	}
	if validator != nil && !validator(candidate) {
		return accountPaths
	}

	cleanCandidate := canonicalPathKey(candidate)
	if _, exists := discovered[cleanCandidate]; exists {
		return accountPaths
	}

	discovered[cleanCandidate] = struct{}{}
	return append(accountPaths, filepath.Clean(candidate))
}

func geminiConfigCandidatePaths(homePath string) []string {
	return []string{
		filepath.Join(homePath, ".gemini", "settings.json"),
		filepath.Join(homePath, ".gemini", "oauth.json"),
		filepath.Join(homePath, ".gemini", "oauth_creds.json"),
		filepath.Join(homePath, "gemini", "oauth_creds.json"),
		filepath.Join(homePath, "oauth_creds.json"),
	}
}

func discoverManagedProviderHomeSlots(
	homeDir string,
	prefix string,
	validator func(string) bool,
) []string {
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return nil
	}

	type slotPath struct {
		slot int
		path string
	}

	paths := make([]slotPath, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		slotText := strings.TrimPrefix(name, prefix)
		if slotText == "" {
			continue
		}

		slot, err := strconv.Atoi(slotText)
		if err != nil || slot <= 0 {
			continue
		}

		path := filepath.Join(homeDir, name)
		if validator(path) || isExistingDirectory(path) {
			paths = append(paths, slotPath{slot: slot, path: path})
		}
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i].slot < paths[j].slot
	})

	discovered := make([]string, 0, len(paths))
	for _, path := range paths {
		discovered = append(discovered, path.path)
	}

	return discovered
}

func discoveredProviderAccountLabel(providerKey, homePath string) string {
	cleanPath := filepath.Clean(homePath)
	base := filepath.Base(cleanPath)
	homeDir := preferredUserHomeDir()

	if homeDir != "" && samePath(homeDir, cleanPath) {
		return "Default"
	}

	if prefix, ok := managedProviderHomePrefix(providerKey); ok {
		if strings.HasPrefix(base, prefix) {
			slotText := strings.TrimPrefix(base, prefix)
			if slot, err := strconv.Atoi(slotText); err == nil && slot > 0 {
				return fmt.Sprintf("Account %d", slot)
			}
		}
	}

	if base == "" || base == "." || base == string(filepath.Separator) {
		return strings.Title(strings.ToLower(providerKey))
	}

	return base
}

func isExistingDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isValidJSONConfigFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var payload any
	if err := json.Unmarshal(stripUTF8BOM(data), &payload); err != nil {
		return false
	}

	return geminiJSONConfigPayloadLooksValid(filepath.Base(path), payload)
}

func geminiJSONConfigPayloadLooksValid(fileName string, payload any) bool {
	root, ok := payload.(map[string]any)
	if !ok {
		return false
	}

	switch strings.ToLower(fileName) {
	case "settings.json":
		if hasNonEmptyJSONString(root, "user") {
			return true
		}
		if hasNonEmptyJSONObject(root["mcpServers"]) {
			return true
		}
		return false
	case "oauth.json", "oauth_creds.json":
		return geminiOAuthPayloadLooksValid(root)
	default:
		return false
	}
}

func hasNonEmptyJSONString(root map[string]any, key string) bool {
	value, ok := root[key].(string)
	return ok && strings.TrimSpace(value) != ""
}

func hasNonEmptyJSONObject(value any) bool {
	obj, ok := value.(map[string]any)
	return ok && len(obj) > 0
}

func geminiOAuthPayloadLooksValid(root map[string]any) bool {
	if hasNonEmptyJSONString(root, "access_token") ||
		hasNonEmptyJSONString(root, "refresh_token") ||
		hasNonEmptyJSONString(root, "client_id") ||
		hasNonEmptyJSONString(root, "client_secret") {
		return true
	}

	tokens, ok := root["tokens"].(map[string]any)
	if !ok {
		return false
	}

	return hasNonEmptyJSONString(tokens, "access_token") ||
		hasNonEmptyJSONString(tokens, "refresh_token")
}

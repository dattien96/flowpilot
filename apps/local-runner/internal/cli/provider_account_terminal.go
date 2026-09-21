package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"flowpilot-runner/internal/runner"

	"github.com/spf13/cobra"
)

type accountLaunchMetadata struct {
	authStorePath          string
	accountEmail           string
	accountName            string
	usageSummary           string
	accessTokenExpiresAt   string
	refreshTokenExpiresAt  string
	refreshTokenExpiryNote string
	remaining5hPercent     *int
	remaining7dPercent     *int
	remaining5hResetAt     string
	remaining7dResetAt     string
	usageDetailLines       []usageDetailLine
}

type usageDetailLine struct {
	label            string
	remainingPercent int
	resetAt          string
}

type launchAccountOption struct {
	account  runner.ProviderAccount
	metadata accountLaunchMetadata
}

type codexAuthFile struct {
	Tokens struct {
		IDToken     string `json:"id_token"`
		AccessToken string `json:"access_token"`
	} `json:"tokens"`
}

type claudeAuthFile struct {
	OAuthAccount struct {
		EmailAddress            string `json:"emailAddress"`
		DisplayName             string `json:"displayName"`
		OrganizationBillingType string `json:"organizationBillingType"`
		BillingType             string `json:"billingType"`
		HasExtraUsageEnabled    bool   `json:"hasExtraUsageEnabled"`
		SubscriptionCreatedAt   string `json:"subscriptionCreatedAt"`
	} `json:"oauthAccount"`
	CachedExtraUsageDisabledReason string `json:"cachedExtraUsageDisabledReason"`
}

type claudeCredentialsFile struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

type claudeAuthStatus struct {
	LoggedIn         bool   `json:"loggedIn"`
	Email            string `json:"email"`
	SubscriptionType string `json:"subscriptionType"`
}

type geminiAuthFile struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
}

type geminiAccountsFile struct {
	Active string `json:"active"`
}

type geminiTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type geminiLoadResponse struct {
	CurrentTier struct {
		Name string `json:"name"`
	} `json:"currentTier"`
	CloudAICompanionProject any `json:"cloudaicompanionProject"`
}

type geminiQuotaResponse struct {
	Buckets []struct {
		ModelID           string  `json:"modelId"`
		RemainingFraction float64 `json:"remainingFraction"`
		ResetTime         string  `json:"resetTime"`
	} `json:"buckets"`
}

var geminiConfig = struct {
	ClientID          string
	ClientSecret      string
	TokenURL          string
	LoadCodeAssistURL string
	RetrieveQuotaURL  string
}{
	ClientID:          "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com",
	ClientSecret:      "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",
	TokenURL:          "https://oauth2.googleapis.com/token",
	LoadCodeAssistURL: "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist",
	RetrieveQuotaURL:  "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuota",
}

func newProviderTerminalCommand(cfg *config) *cobra.Command {
	providerKey := ""
	accountID := ""

	cmd := &cobra.Command{
		Use:   "terminal",
		Short: "Choose a connected provider account and open a terminal for it",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			instance, err := runner.New(cfg.workspace)
			if err != nil {
				return err
			}

			fmt.Fprintln(out, "Loading connected provider accounts...")
			accounts, err := instance.ListProviderAccounts()
			if err != nil {
				return err
			}

			options, err := buildLaunchAccountOptions(accounts, providerKey, out)
			if err != nil {
				return err
			}
			if len(options) == 0 {
				return errors.New("no connected provider accounts found")
			}

			if strings.TrimSpace(accountID) != "" {
				selected := selectAccountOption(options, accountID)
				if selected == nil {
					return fmt.Errorf("connected account %q was not found", accountID)
				}
				return launchSelectedProviderAccount(instance, *selected)
			}

			printLaunchAccountOptions(cmd.OutOrStdout(), options)
			selected, err := promptForAccountSelection(cmd.InOrStdin(), cmd.OutOrStdout(), options)
			if err != nil {
				return err
			}

			return launchSelectedProviderAccount(instance, selected)
		},
	}

	cmd.Flags().StringVar(&providerKey, "provider", "", "Filter selectable accounts by provider key")
	cmd.Flags().StringVar(&accountID, "account-id", "", "Launch a specific account without prompting")

	return cmd
}

func buildLaunchAccountOptions(accounts []runner.ProviderAccount, providerKey string, progress io.Writer) ([]launchAccountOption, error) {
	filtered := make([]runner.ProviderAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.AuthStatus != "connected" {
			continue
		}
		if strings.TrimSpace(providerKey) != "" && !strings.EqualFold(account.ProviderKey, providerKey) {
			continue
		}
		filtered = append(filtered, account)
	}

	if len(filtered) > 0 && progress != nil {
		fmt.Fprintf(progress, "Loading account details for %d connected account(s). This can take a few seconds...\n", len(filtered))
	}

	options := make([]launchAccountOption, 0, len(filtered))
	for index, account := range filtered {
		if progress != nil {
			fmt.Fprintf(progress, "  [%d/%d] Loading %s account metadata...\n", index+1, len(filtered), describeAccountProgress(account))
		}
		metadata, err := loadAccountLaunchMetadata(account)
		if err != nil {
			return nil, err
		}
		options = append(options, launchAccountOption{
			account:  account,
			metadata: metadata,
		})
	}

	sort.SliceStable(options, func(i, j int) bool {
		left := options[i]
		right := options[j]
		if !strings.EqualFold(left.account.ProviderKey, right.account.ProviderKey) {
			return strings.ToLower(left.account.ProviderKey) < strings.ToLower(right.account.ProviderKey)
		}
		if left.account.IsActive != right.account.IsActive {
			return left.account.IsActive
		}
		return left.account.SlotIndex < right.account.SlotIndex
	})

	if len(options) > 0 && progress != nil {
		fmt.Fprintln(progress, "Account details loaded.")
		fmt.Fprintln(progress)
	}

	return options, nil
}

func describeAccountProgress(account runner.ProviderAccount) string {
	label := strings.TrimSpace(account.DisplayName)
	if label == "" {
		label = account.ID
	}

	return fmt.Sprintf("%s (%s)", account.ProviderKey, label)
}

func selectAccountOption(options []launchAccountOption, accountID string) *launchAccountOption {
	trimmed := strings.TrimSpace(accountID)
	if trimmed == "" {
		return nil
	}

	for _, option := range options {
		if option.account.ID == trimmed {
			selected := option
			return &selected
		}
	}

	return nil
}

func launchSelectedProviderAccount(instance *runner.Runner, selected launchAccountOption) error {
	if err := printLaunchSelection(os.Stdout, selected); err != nil {
		return err
	}

	if _, err := instance.TestProviderAccount(selected.account.ID); err != nil {
		return err
	}

	_, err := fmt.Fprintf(
		os.Stdout,
		"\nOpened terminal for %s account %q in the current FlowPilot workspace.\n",
		selected.account.ProviderKey,
		displayLabel(selected),
	)
	return err
}

func printLaunchAccountOptions(w io.Writer, options []launchAccountOption) {
	fmt.Fprintln(w, "Connected provider accounts:")
	for index, option := range options {
		fmt.Fprintf(
			w,
			"\n[%d] %s\n",
			index+1,
			describeLaunchAccount(option),
		)
	}
	fmt.Fprintln(w)
}

func printLaunchSelection(w io.Writer, option launchAccountOption) error {
	_, err := fmt.Fprintf(
		w,
		"Launching %s\n",
		describeLaunchAccount(option),
	)
	return err
}

func describeLaunchAccount(option launchAccountOption) string {
	parts := []string{
		fmt.Sprintf("provider=%s", option.account.ProviderKey),
		fmt.Sprintf("account=%s", displayLabel(option)),
	}

	if option.account.IsActive {
		parts = append(parts, "active=yes")
	}
	if option.metadata.usageSummary != "" {
		parts = append(parts, fmt.Sprintf("plan=%s", option.metadata.usageSummary))
	}
	if option.metadata.remaining5hPercent != nil {
		parts = append(parts, fmt.Sprintf("remaining5h=%d%%", *option.metadata.remaining5hPercent))
	}
	if option.metadata.remaining7dPercent != nil {
		parts = append(parts, fmt.Sprintf("remaining7d=%d%%", *option.metadata.remaining7dPercent))
	}
	for _, line := range option.metadata.usageDetailLines {
		if isBuiltInQuotaLabel(line.label) {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d%%", sanitizeUsageLabel(line.label), line.remainingPercent))
	}
	parts = append(parts, fmt.Sprintf("home=%s", option.account.HomePath))

	return strings.Join(parts, " | ")
}

func displayLabel(option launchAccountOption) string {
	if option.metadata.accountEmail != "" {
		return option.metadata.accountEmail
	}
	if option.metadata.accountName != "" {
		return option.metadata.accountName
	}
	return option.account.DisplayName
}

func sanitizeUsageLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "usage"
	}
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", "-", "_")
	return strings.ToLower(replacer.Replace(trimmed))
}

func isBuiltInQuotaLabel(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	return normalized == "remaining 5h" || normalized == "remaining 7d"
}

func promptForAccountSelection(r io.Reader, w io.Writer, options []launchAccountOption) (launchAccountOption, error) {
	reader := bufio.NewReader(r)
	for {
		if _, err := fmt.Fprint(w, "Select account number: "); err != nil {
			return launchAccountOption{}, err
		}
		input, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return launchAccountOption{}, readErr
		}

		selectedIndex, parseErr := parseLaunchSelection(input, len(options))
		if parseErr == nil {
			return options[selectedIndex], nil
		}

		if _, err := fmt.Fprintf(w, "%s\n", parseErr.Error()); err != nil {
			return launchAccountOption{}, err
		}
		if errors.Is(readErr, io.EOF) {
			return launchAccountOption{}, parseErr
		}
	}
}

func parseLaunchSelection(input string, optionCount int) (int, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, errors.New("enter a number from the list")
	}

	selected, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, errors.New("enter a valid numeric selection")
	}
	if selected < 1 || selected > optionCount {
		return 0, fmt.Errorf("selection must be between 1 and %d", optionCount)
	}

	return selected - 1, nil
}

func loadAccountLaunchMetadata(account runner.ProviderAccount) (accountLaunchMetadata, error) {
	switch strings.ToLower(account.ProviderKey) {
	case "codex":
		return loadCodexAccountMetadata(account.HomePath)
	case "claude":
		return loadClaudeAccountMetadata(account.HomePath), nil
	case "gemini":
		return loadGeminiAccountMetadata(account.HomePath)
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-9): codex/claude/gemini cases above unchanged.
		return loadGrokAccountMetadata(account.HomePath)
	case "opencode":
		// Appended last (CP-57 P-0/Task-302 T-13).
		return loadOpencodeAccountMetadata(account.HomePath)
	case "devin":
		// Appended last (CP-70 P-0/Task-402): live GetUserStatus quota probe.
		return loadDevinAccountMetadata(account.HomePath)
	default:
		return accountLaunchMetadata{}, nil
	}
}

func loadOpencodeAccountMetadata(homePath string) (accountLaunchMetadata, error) {
	// Delegate to runner's shared implementation (CP-57 Task-302 DOD-7) and map to cli type.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runnerMeta, err := runner.LoadOpencodeAccountMetadata(ctx, homePath)
	if err != nil {
		// Best-effort fallback: at least return authStorePath
		return accountLaunchMetadata{authStorePath: homePath}, nil
	}
	metadata := accountLaunchMetadata{
		authStorePath: homePath,
		accountEmail:  runnerMeta.AccountEmail,
		accountName:   runnerMeta.DisplayLabel,
		usageSummary:  runnerMeta.UsageSummary,
	}
	for _, line := range runnerMeta.UsageDetailLines {
		metadata.usageDetailLines = append(metadata.usageDetailLines, usageDetailLine{label: line.Label})
	}
	metadata.remaining5hPercent = nil
	metadata.remaining7dPercent = nil
	return metadata, nil
}

// grokAuthEntry is one value in ~/.grok/auth.json, which is keyed by
// "<oidcIssuer>::<userId>" (live-verified structure, CP-46/Task-210
// authoring). CP-46 Q-5/Task-210 Q-1 originally concluded Grok exposes no
// machine-readable quota/usage endpoint (only the turn-time 402
// personal-team-blocked:spending-limit signal, isProviderUsageLimitError in
// interactive_service.go) — that was based solely on the ACP JSON-RPC
// stream. Task-216 corrects it: the `key` field below is the same cached
// OAuth bearer token Grok Build's own `/usage` TUI command uses, and
// loadGrokQuota calls the real billing endpoint behind it (live-verified).
type grokAuthEntry struct {
	Email                     string `json:"email"`
	FirstName                 string `json:"first_name"`
	LastName                  string `json:"last_name"`
	TeamID                    string `json:"team_id"`
	Key                       string `json:"key"`
	RefreshToken              string `json:"refresh_token"`
	ExpiresAt                 string `json:"expires_at"`
	OidcIssuer                string `json:"oidc_issuer"`
	OidcClientID              string `json:"oidc_client_id"`
	AuthMode                  string `json:"auth_mode"`
	CreateTime                string `json:"create_time"`
	UserID                    string `json:"user_id"`
	ProfileImageAssetID       string `json:"profile_image_asset_id"`
	PrincipalType             string `json:"principal_type"`
	PrincipalID               string `json:"principal_id"`
	CodingDataRetentionOptOut bool   `json:"coding_data_retention_opt_out"`
}

// grokCLIChatProxyBaseURL is Grok Build's own backend base URL, overridable
// via GROK_CLI_CHAT_PROXY_BASE_URL (live-verified string in the installed
// `grok` binary, Task-216). The billing/usage route lives under it.
var grokCLIChatProxyBaseURL = "https://cli-chat-proxy.grok.com/v1"

// grokCLICreditsBillingPath is the query Grok CLI 1.0.3 uses for `/usage`
// remaining (live-verified in grok.exe: `/billing?format=credits`). Plain
// `/billing` returns team-credit monthlyLimit/used and is kept as fallback.
const grokCLICreditsBillingPath = "/billing?format=credits"

type grokBillingValue struct {
	Val float64 `json:"val"`
}

type grokBillingPeriod struct {
	Type  string `json:"type"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// grokBillingConfig mirrors GET {grokCLIChatProxyBaseURL}/billing config.
// Task-216 shape: monthlyLimit/weeklyLimit + used (team credits).
// Grok CLI 1.0.3 `/billing?format=credits` shape: currentPeriod +
// creditUsagePercent (personal SuperGrok weekly remaining). Omitted
// creditUsagePercent means 0% used.
type grokBillingConfig struct {
	MonthlyLimit       *grokBillingValue  `json:"monthlyLimit"`
	WeeklyLimit        *grokBillingValue  `json:"weeklyLimit"`
	Used               *grokBillingValue  `json:"used"`
	BillingPeriodEnd   string             `json:"billingPeriodEnd"`
	CreditUsagePercent *float64           `json:"creditUsagePercent"`
	CurrentPeriod      *grokBillingPeriod `json:"currentPeriod"`
}

type grokBillingResponse struct {
	Config grokBillingConfig `json:"config"`
}

func loadGrokAccountMetadata(homePath string) (accountLaunchMetadata, error) {
	authPath := firstExistingPath(filepath.Join(homePath, "auth.json"))
	metadata := accountLaunchMetadata{authStorePath: homePath}
	if authPath == "" {
		return metadata, nil
	}

	var raw map[string]grokAuthEntry
	if err := readJSONFile(authPath, &raw); err != nil {
		return metadata, nil
	}
	for mapKey, entry := range raw {
		mutable := entry
		// Pre-emptive refresh when key is expired (Grok OIDC 6h TTL, live-verified).
		// Avoids a wasted 401 then retry. This is Grok-only; Codex/Claude/Gemini unchanged.
		if grokKeyExpired(mutable.ExpiresAt) && strings.TrimSpace(mutable.RefreshToken) != "" {
			if refreshed := tryRefreshGrokAuth(authPath, mapKey, &mutable); refreshed {
				// mutable now holds fresh key/refresh_token/expires_at
			}
		}
		metadata.accountEmail = mutable.Email
		if name := strings.TrimSpace(mutable.FirstName + " " + mutable.LastName); name != "" {
			metadata.accountName = name
		}
		if mutable.TeamID != "" {
			metadata.usageSummary = "Team " + mutable.TeamID
		} else {
			metadata.usageSummary = "Personal"
		}
		applyGrokQuotaLine(&metadata, loadGrokQuotaWithRefresh(&mutable, authPath, mapKey))
		break // a single active auth entry is expected per home
	}
	return metadata, nil
}

// loadGrokQuota calls the same billing route Grok CLI `/usage` uses:
// GET /v1/billing?format=credits with the cached auth.json bearer token
// (live-verified grok 1.0.3). Falls back to plain /billing (Task-216 team credits).
func loadGrokQuota(bearerToken string) *usageDetailLine {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return nil
	}
	if line := fetchGrokBilling(token, grokCLICreditsBillingPath); line != nil {
		return line
	}
	return fetchGrokBilling(token, "/billing")
}

func fetchGrokBilling(bearerToken, path string) *usageDetailLine {
	line, _ := fetchGrokBillingWithStatus(bearerToken, path)
	return line
}

func fetchGrokBillingWithStatus(bearerToken, path string) (*usageDetailLine, int) {
	request, err := http.NewRequest(http.MethodGet, grokCLIChatProxyBaseURL+path, nil)
	if err != nil {
		return nil, 0
	}
	request.Header.Set("Authorization", "Bearer "+bearerToken)
	request.Header.Set("Accept", "application/json")

	response, err := httpClient().Do(request)
	if err != nil {
		return nil, 0
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode
	}

	var billing grokBillingResponse
	if err := json.NewDecoder(response.Body).Decode(&billing); err != nil {
		return nil, response.StatusCode
	}

	return grokQuotaFromBilling(billing), response.StatusCode
}

// loadGrokQuotaWithRefresh is the Grok-only 401/expired -> OIDC refresh -> retry path.
// Pre-emptive expiry refresh is handled in loadGrokAccountMetadata; this handles the
// 401-after-call fallback. Scope is Grok-only; Codex/Claude/Gemini untouched.
func loadGrokQuotaWithRefresh(entry *grokAuthEntry, authPath, mapKey string) *usageDetailLine {
	if entry == nil {
		return nil
	}
	token := strings.TrimSpace(entry.Key)
	if token == "" {
		return nil
	}
	// attempt credits path
	if line, status := fetchGrokBillingWithStatus(token, grokCLICreditsBillingPath); line != nil {
		return line
	} else if status == http.StatusUnauthorized && strings.TrimSpace(entry.RefreshToken) != "" {
		if tryRefreshGrokAuth(authPath, mapKey, entry) {
			token = strings.TrimSpace(entry.Key)
			if line2, _ := fetchGrokBillingWithStatus(token, grokCLICreditsBillingPath); line2 != nil {
				return line2
			}
			// retry fallback with fresh token as well
			if line2, _ := fetchGrokBillingWithStatus(token, "/billing"); line2 != nil {
				return line2
			}
			return nil
		}
	}
	// credits miss without 401, or refresh not applicable -> try fallback with original token
	if line, status := fetchGrokBillingWithStatus(token, "/billing"); line != nil {
		return line
	} else if status == http.StatusUnauthorized && strings.TrimSpace(entry.RefreshToken) != "" {
		if tryRefreshGrokAuth(authPath, mapKey, entry) {
			token = strings.TrimSpace(entry.Key)
			if line2, _ := fetchGrokBillingWithStatus(token, "/billing"); line2 != nil {
				return line2
			}
			if line2, _ := fetchGrokBillingWithStatus(token, grokCLICreditsBillingPath); line2 != nil {
				return line2
			}
		}
	}
	return nil
}

func grokKeyExpired(expiresAt string) bool {
	raw := strings.TrimSpace(expiresAt)
	if raw == "" {
		return false
	}
	var t time.Time
	var err error
	t, err = time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return false
		}
	}
	// 5m early buffer so we refresh before the exact expiry second.
	return time.Now().After(t.Add(-5 * time.Minute))
}

func refreshGrokAccessToken(refreshToken, oidcIssuer, oidcClientID string) (string, string, time.Time, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	oidcIssuer = strings.TrimSpace(oidcIssuer)
	oidcClientID = strings.TrimSpace(oidcClientID)
	if refreshToken == "" || oidcIssuer == "" || oidcClientID == "" {
		return "", "", time.Time{}, fmt.Errorf("missing refresh material")
	}
	issuer := strings.TrimRight(oidcIssuer, "/")
	tokenURL := issuer + "/oauth2/token"
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", oidcClientID)
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return "", "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", "", time.Time{}, fmt.Errorf("grok refresh %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", time.Time{}, err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", "", time.Time{}, fmt.Errorf("empty access_token")
	}
	expiresAt := time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
	if payload.ExpiresIn <= 0 {
		expiresAt = time.Now().UTC().Add(6 * time.Hour)
	}
	newRefresh := strings.TrimSpace(payload.RefreshToken)
	if newRefresh == "" {
		newRefresh = refreshToken
	}
	return strings.TrimSpace(payload.AccessToken), newRefresh, expiresAt, nil
}

func tryRefreshGrokAuth(authPath, mapKey string, entry *grokAuthEntry) bool {
	if entry == nil || strings.TrimSpace(entry.RefreshToken) == "" {
		return false
	}
	issuer := strings.TrimSpace(entry.OidcIssuer)
	if issuer == "" {
		issuer = "https://auth.x.ai"
	}
	clientID := strings.TrimSpace(entry.OidcClientID)
	if clientID == "" {
		clientID = "b1a00492-073a-47ea-816f-4c329264a828"
	}
	newKey, newRefresh, expiresAt, err := refreshGrokAccessToken(entry.RefreshToken, issuer, clientID)
	if err != nil {
		return false
	}
	entry.Key = newKey
	entry.RefreshToken = newRefresh
	entry.ExpiresAt = expiresAt.Format(time.RFC3339Nano)
	// Persist back to auth.json preserving other fields.
	rawBytes, err := os.ReadFile(authPath)
	if err != nil {
		return true // in-memory updated, file write best-effort
	}
	var fileMap map[string]map[string]any
	if err := json.Unmarshal(rawBytes, &fileMap); err != nil {
		return true
	}
	if m, ok := fileMap[mapKey]; ok {
		m["key"] = entry.Key
		m["refresh_token"] = entry.RefreshToken
		m["expires_at"] = entry.ExpiresAt
		fileMap[mapKey] = m
		out, err := json.MarshalIndent(fileMap, "", "  ")
		if err != nil {
			return true
		}
		_ = os.WriteFile(authPath, append(out, '\n'), 0600)
	}
	return true
}

// applyGrokQuotaLine copies a Grok usage line onto account metadata. Weekly
// remaining is also stored as remaining7dPercent so TUI statusline (5h/7d)
// and Desktop meters both light up.
func applyGrokQuotaLine(metadata *accountLaunchMetadata, line *usageDetailLine) {
	if metadata == nil || line == nil {
		return
	}
	metadata.usageDetailLines = append(metadata.usageDetailLines, *line)
	if !strings.Contains(strings.ToLower(line.label), "weekly") {
		return
	}
	metadata.remaining7dPercent = intPtr(line.remainingPercent)
	metadata.remaining7dResetAt = line.resetAt
}

// grokQuotaFromBilling is the pure mapping half of loadGrokQuota, split out
// so tests can exercise it against a captured response fixture without a
// live network call (mirrors codexQuotaFromWindow's split).
//
// Credits format (`currentPeriod` + `creditUsagePercent`) is what Grok CLI
// 1.0.3 `/usage` shows as "Weekly limit". creditUsagePercent is used-percent
// (1.0 = 1% used → 99% remaining); omitted means 0% used.
// Team-credit monthlyLimit/weeklyLimit+used (Task-216) remains the fallback
// when currentPeriod is absent.
func grokQuotaFromBilling(billing grokBillingResponse) *usageDetailLine {
	if period := billing.Config.CurrentPeriod; period != nil && strings.TrimSpace(period.Type) != "" {
		usedPct := 0.0
		if billing.Config.CreditUsagePercent != nil {
			usedPct = *billing.Config.CreditUsagePercent
		}
		label := "Weekly limit"
		if strings.Contains(strings.ToUpper(period.Type), "MONTHLY") {
			label = "Monthly limit"
		}
		resetAt := grokBillingResetAt(period.End, billing.Config.BillingPeriodEnd)
		return &usageDetailLine{
			label:            label,
			remainingPercent: clampInt(int(100 - usedPct)),
			resetAt:          resetAt,
		}
	}

	limit := billing.Config.MonthlyLimit
	label := "Team Credits (Monthly)"
	if limit == nil {
		limit = billing.Config.WeeklyLimit
		label = "Team Credits (Weekly)"
	}
	if limit == nil || limit.Val <= 0 || billing.Config.Used == nil {
		return nil
	}

	remaining := clampInt(int(100 - (billing.Config.Used.Val/limit.Val)*100))
	return &usageDetailLine{
		label:            label,
		remainingPercent: remaining,
		resetAt:          grokBillingResetAt(billing.Config.BillingPeriodEnd, ""),
	}
}

func grokBillingResetAt(primary, fallback string) string {
	for _, raw := range []string{strings.TrimSpace(primary), strings.TrimSpace(fallback)} {
		if raw == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

func loadCodexAccountMetadata(homePath string) (accountLaunchMetadata, error) {
	authPath := firstExistingPath(
		filepath.Join(homePath, "auth.json"),
		filepath.Join(homePath, ".codex", "auth.json"),
		filepath.Join(homePath, "codex", "auth.json"),
	)
	if authPath == "" {
		return accountLaunchMetadata{authStorePath: homePath}, nil
	}

	var auth codexAuthFile
	if err := readJSONFile(authPath, &auth); err != nil {
		return accountLaunchMetadata{authStorePath: filepath.Dir(authPath)}, nil
	}

	payload := decodeJWTPayload(auth.Tokens.IDToken)
	metadata := accountLaunchMetadata{
		authStorePath:          filepath.Dir(authPath),
		accountEmail:           stringClaim(payload, "email"),
		accountName:            stringClaim(payload, "name"),
		accessTokenExpiresAt:   parseTokenExpiry(auth.Tokens.AccessToken),
		refreshTokenExpiryNote: "Unknown. Codex refresh token expiry is not exposed in local auth data; re-login is required only when a refresh attempt fails.",
	}

	if openAIAuth := mapClaim(payload, "https://api.openai.com/auth"); openAIAuth != nil {
		planType := stringMapClaim(openAIAuth, "chatgpt_plan_type")
		activeUntil := stringMapClaim(openAIAuth, "chatgpt_subscription_active_until")
		switch {
		case planType != "" && activeUntil != "":
			metadata.usageSummary = fmt.Sprintf("%s until %s", planType, sliceDate(activeUntil))
		case planType != "":
			metadata.usageSummary = planType
		}
	}

	if strings.TrimSpace(auth.Tokens.AccessToken) == "" {
		return metadata, nil
	}

	request, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/wham/usage", nil)
	if err != nil {
		return metadata, nil
	}
	request.Header.Set("Authorization", "Bearer "+auth.Tokens.AccessToken)
	request.Header.Set("Accept", "application/json")

	response, err := httpClient().Do(request)
	if err != nil {
		return metadata, nil
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return metadata, nil
	}

	var usage struct {
		RateLimit struct {
			PrimaryWindow   map[string]any `json:"primary_window"`
			SecondaryWindow map[string]any `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if err := json.NewDecoder(response.Body).Decode(&usage); err != nil {
		return metadata, nil
	}

	for _, window := range []map[string]any{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		quota := codexQuotaFromWindow(window)
		if quota == nil {
			continue
		}

		line := usageDetailLine{
			remainingPercent: quota.remainingPercent,
			resetAt:          quota.resetAt,
		}
		switch quota.windowSeconds {
		case 18000:
			line.label = "Remaining 5h"
			metadata.remaining5hPercent = intPtr(quota.remainingPercent)
			metadata.remaining5hResetAt = quota.resetAt
		case 604800:
			line.label = "Remaining 7d"
			metadata.remaining7dPercent = intPtr(quota.remainingPercent)
			metadata.remaining7dResetAt = quota.resetAt
		default:
			line.label = fmt.Sprintf("Window %ds", quota.windowSeconds)
		}
		metadata.usageDetailLines = append(metadata.usageDetailLines, line)
	}

	return metadata, nil
}

func loadClaudeQuota(homePath string) (remaining5h *int, reset5h string, remaining7d *int, reset7d string) {
	credPath := filepath.Join(homePath, ".claude", ".credentials.json")
	var creds claudeCredentialsFile
	if err := readJSONFile(credPath, &creds); err != nil {
		return
	}
	token := strings.TrimSpace(creds.ClaudeAiOauth.AccessToken)
	if token == "" {
		return
	}
	// Skip if token is already expired (expiresAt is milliseconds since epoch).
	if creds.ClaudeAiOauth.ExpiresAt > 0 && time.Now().UnixMilli() >= creds.ClaudeAiOauth.ExpiresAt {
		return
	}

	body := []byte(`{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"x"}]}`)
	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-client-type", "claude_code_desktop")

	resp, err := httpClient().Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // drain body

	parseUtilization := func(header string) *int {
		v := strings.TrimSpace(resp.Header.Get(header))
		if v == "" {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
		pct := clampInt(100 - int(f*100))
		return intPtr(pct)
	}
	parseReset := func(header string) string {
		v := strings.TrimSpace(resp.Header.Get(header))
		if v == "" {
			return ""
		}
		secs, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return ""
		}
		return time.Unix(secs, 0).UTC().Format(time.RFC3339)
	}

	remaining5h = parseUtilization("anthropic-ratelimit-unified-5h-utilization")
	reset5h = parseReset("anthropic-ratelimit-unified-5h-reset")
	remaining7d = parseUtilization("anthropic-ratelimit-unified-7d-utilization")
	reset7d = parseReset("anthropic-ratelimit-unified-7d-reset")
	return
}

func loadClaudeAccountMetadata(homePath string) accountLaunchMetadata {
	authPath := firstExistingPath(
		filepath.Join(homePath, ".claude.json"),
		filepath.Join(homePath, "claude", "auth.json"),
		filepath.Join(homePath, ".config", "claude", "auth.json"),
	)
	if authPath == "" {
		return accountLaunchMetadata{authStorePath: filepath.Join(homePath, ".claude")}
	}

	metadata := accountLaunchMetadata{authStorePath: filepath.Dir(authPath)}

	var auth claudeAuthFile
	if err := readJSONFile(authPath, &auth); err == nil {
		metadata.accountEmail = auth.OAuthAccount.EmailAddress
		metadata.accountName = auth.OAuthAccount.DisplayName
		metadata.usageSummary = buildClaudeUsageSummary(
			auth.OAuthAccount.OrganizationBillingType,
			auth.OAuthAccount.BillingType,
			auth.OAuthAccount.SubscriptionCreatedAt,
			auth.OAuthAccount.HasExtraUsageEnabled,
			auth.CachedExtraUsageDisabledReason,
			loadClaudeAuthStatus(homePath),
		)
	}

	status := loadClaudeAuthStatus(homePath)
	if status.Email != "" {
		metadata.accountEmail = status.Email
	}
	if metadata.usageSummary == "" {
		metadata.usageSummary = buildClaudeUsageSummary(
			auth.OAuthAccount.OrganizationBillingType,
			auth.OAuthAccount.BillingType,
			auth.OAuthAccount.SubscriptionCreatedAt,
			auth.OAuthAccount.HasExtraUsageEnabled,
			auth.CachedExtraUsageDisabledReason,
			status,
		)
	}

	r5h, reset5h, r7d, reset7d := loadClaudeQuota(homePath)
	if r5h != nil || r7d != nil {
		metadata.remaining5hPercent = r5h
		metadata.remaining5hResetAt = reset5h
		metadata.remaining7dPercent = r7d
		metadata.remaining7dResetAt = reset7d
		if r5h != nil {
			metadata.usageDetailLines = append(metadata.usageDetailLines, usageDetailLine{
				label:            "Remaining 5h",
				remainingPercent: *r5h,
				resetAt:          reset5h,
			})
		}
		if r7d != nil {
			metadata.usageDetailLines = append(metadata.usageDetailLines, usageDetailLine{
				label:            "Remaining 7d",
				remainingPercent: *r7d,
				resetAt:          reset7d,
			})
		}
	}

	return metadata
}

func loadClaudeAuthStatus(homePath string) claudeAuthStatus {
	command := "claude"
	if runtime.GOOS == "windows" {
		command = "claude.cmd"
	}

	cmd := exec.Command(command, "auth", "status", "--json")
	cmd.Dir = homePath
	cmd.Env = append(os.Environ(),
		"HOME="+homePath,
		"USERPROFILE="+homePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return claudeAuthStatus{}
	}

	var status claudeAuthStatus
	if err := json.Unmarshal(output, &status); err != nil || !status.LoggedIn {
		return claudeAuthStatus{}
	}

	return status
}

func buildClaudeUsageSummary(
	organizationBillingType string,
	billingType string,
	subscriptionCreatedAt string,
	hasExtraUsageEnabled bool,
	disabledReason string,
	status claudeAuthStatus,
) string {
	rawPlan := status.SubscriptionType
	if rawPlan == "" {
		rawPlan = organizationBillingType
	}
	if rawPlan == "" {
		rawPlan = billingType
	}

	summary := humanizeDelimitedLabel(rawPlan)
	if summary != "" && len(subscriptionCreatedAt) >= 10 {
		summary += " since " + subscriptionCreatedAt[:10]
	}

	reason := strings.ToLower(humanizeDelimitedLabel(disabledReason))
	if reason != "" {
		if summary != "" {
			return fmt.Sprintf("%s | extra usage unavailable (%s)", summary, reason)
		}
		return fmt.Sprintf("Extra usage unavailable (%s)", reason)
	}

	if hasExtraUsageEnabled {
		if summary != "" {
			return summary + " | extra usage enabled"
		}
		return "Extra usage enabled"
	}

	return summary
}

func loadGeminiAccountMetadata(homePath string) (accountLaunchMetadata, error) {
	authPath := firstExistingPath(
		filepath.Join(homePath, ".gemini", "oauth_creds.json"),
		filepath.Join(homePath, "gemini", "oauth_creds.json"),
		filepath.Join(homePath, "oauth_creds.json"),
	)
	accountsPath := firstExistingPath(
		filepath.Join(homePath, ".gemini", "google_accounts.json"),
		filepath.Join(homePath, "google_accounts.json"),
	)

	metadata := accountLaunchMetadata{
		authStorePath: filepath.Join(homePath, ".gemini"),
	}

	// Antigravity CLI no longer exposes the legacy Gemini Code Assist quota/auth
	// files that the old metadata path depended on. Showing stale buckets from that
	// deprecated API is worse than showing nothing.
	if hasGeminiAntigravityMarker(homePath) {
		return metadata, nil
	}

	if accountsPath != "" {
		var accounts geminiAccountsFile
		if err := readJSONFile(accountsPath, &accounts); err == nil {
			metadata.accountEmail = accounts.Active
		}
	}

	if authPath == "" {
		return metadata, nil
	}

	var auth geminiAuthFile
	if err := readJSONFile(authPath, &auth); err != nil {
		return metadata, nil
	}

	payload := decodeJWTPayload(auth.IDToken)
	if metadata.accountEmail == "" {
		metadata.accountEmail = stringClaim(payload, "email")
	}
	metadata.accountName = stringClaim(payload, "name")

	accessToken, err := refreshGeminiAccessToken(auth.RefreshToken)
	if err != nil || accessToken == "" {
		return metadata, nil
	}

	quota, err := loadGeminiQuota(accessToken)
	if err != nil {
		return metadata, nil
	}

	metadata.usageSummary = quota.usageSummary
	metadata.usageDetailLines = quota.usageDetailLines
	return metadata, nil
}

func hasGeminiAntigravityMarker(homePath string) bool {
	for _, candidate := range []string{
		filepath.Join(homePath, ".gemini", "antigravity-cli"),
		filepath.Join(homePath, "antigravity-cli"),
	} {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

type codexQuota struct {
	windowSeconds    int
	remainingPercent int
	resetAt          string
}

func codexQuotaFromWindow(window map[string]any) *codexQuota {
	if len(window) == 0 {
		return nil
	}

	usedPercent, ok := numberFromAny(window["used_percent"])
	if !ok {
		usedPercent, ok = numberFromAny(window["percent_used"])
	}
	windowSecondsFloat, ok := numberFromAny(window["limit_window_seconds"])
	if !ok {
		return nil
	}

	remaining := clampInt(int(100 - usedPercent))
	return &codexQuota{
		windowSeconds:    int(windowSecondsFloat),
		remainingPercent: remaining,
		resetAt:          parseResetTime(window["reset_at"], window["resets_at"]),
	}
}

type geminiQuotaMetadata struct {
	usageSummary     string
	usageDetailLines []usageDetailLine
}

func refreshGeminiAccessToken(refreshToken string) (string, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", geminiConfig.ClientID)
	form.Set("client_secret", geminiConfig.ClientSecret)

	request, err := http.NewRequest(http.MethodPost, geminiConfig.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	response, err := httpClient().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil
	}

	var payload geminiTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return "", err
	}

	return payload.AccessToken, nil
}

func loadGeminiQuota(accessToken string) (geminiQuotaMetadata, error) {
	metadata := map[string]any{
		"ideType":    9,
		"platform":   getOAuthPlatformEnum(),
		"pluginType": 2,
	}
	headers := map[string]string{
		"Authorization":     "Bearer " + accessToken,
		"Content-Type":      "application/json",
		"User-Agent":        "google-api-nodejs-client/9.15.1",
		"X-Goog-Api-Client": "google-cloud-sdk vscode_cloudshelleditor/0.1",
		"Client-Metadata":   mustMarshalJSON(metadata),
	}

	loadPayload, err := doJSONRequest(http.MethodPost, geminiConfig.LoadCodeAssistURL, headers, map[string]any{"metadata": metadata}, &geminiLoadResponse{})
	if err != nil {
		return geminiQuotaMetadata{}, err
	}
	loadResponse := loadPayload.(*geminiLoadResponse)

	projectID := resolveGeminiProjectID(loadResponse.CloudAICompanionProject)
	result := geminiQuotaMetadata{usageSummary: loadResponse.CurrentTier.Name}
	if projectID == "" {
		return result, nil
	}

	quotaPayload, err := doJSONRequest(http.MethodPost, geminiConfig.RetrieveQuotaURL, headers, map[string]any{"project": projectID}, &geminiQuotaResponse{})
	if err != nil {
		return result, err
	}
	quotaResponse := quotaPayload.(*geminiQuotaResponse)
	result.usageDetailLines = mapGeminiQuotaUsageDetailLines(quotaResponse.Buckets)
	return result, nil
}

func doJSONRequest(method string, endpoint string, headers map[string]string, body any, target any) (any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequest(method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("request to %s failed with status %d", endpoint, response.StatusCode)
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return nil, err
	}

	return target, nil
}

func mapGeminiQuotaUsageDetailLines(buckets []struct {
	ModelID           string  `json:"modelId"`
	RemainingFraction float64 `json:"remainingFraction"`
	ResetTime         string  `json:"resetTime"`
}) []usageDetailLine {
	lines := make([]usageDetailLine, 0, len(buckets))
	for _, bucket := range buckets {
		if strings.TrimSpace(bucket.ModelID) == "" {
			continue
		}
		lines = append(lines, usageDetailLine{
			label:            bucket.ModelID,
			remainingPercent: clampInt(int(bucket.RemainingFraction * 100)),
			resetAt:          parseResetTime(bucket.ResetTime),
		})
	}

	sort.Slice(lines, func(i, j int) bool {
		return lines[i].label < lines[j].label
	})
	return lines
}

func getOAuthPlatformEnum() int {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return 2
		}
		return 1
	case "linux":
		if runtime.GOARCH == "arm64" {
			return 4
		}
		return 3
	case "windows":
		return 5
	default:
		return 0
	}
}

func resolveGeminiProjectID(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if id, ok := typed["id"].(string); ok {
			return id
		}
	}
	return ""
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil
	}
	return payload
}

func parseTokenExpiry(token string) string {
	payload := decodeJWTPayload(token)
	if payload == nil {
		return ""
	}

	expiry, ok := numberFromAny(payload["exp"])
	if !ok {
		return ""
	}

	return time.Unix(int64(expiry), 0).UTC().Format(time.RFC3339)
}

func stringClaim(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func mapClaim(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	value, _ := payload[key].(map[string]any)
	return value
}

func stringMapClaim(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func firstExistingPath(paths ...string) string {
	for _, candidate := range paths {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func readJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Strip UTF-8 BOM (PowerShell Set-Content writes BOM on Windows, breaks json.Unmarshal).
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	return json.Unmarshal(raw, target)
}

func humanizeDelimitedLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		parts[index] = strings.ToUpper(lower[:1]) + lower[1:]
	}
	return strings.Join(parts, " ")
}

func numberFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	}
	return 0, false
}

func parseResetTime(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			if numeric, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				return unixMillisToISO(numeric)
			}
			if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
				return parsed.UTC().Format(time.RFC3339)
			}
			if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
				return parsed.UTC().Format(time.RFC3339)
			}
		case float64:
			return unixMillisToISO(int64(typed))
		case int64:
			return unixMillisToISO(typed)
		case int:
			return unixMillisToISO(int64(typed))
		}
	}
	return ""
}

func unixMillisToISO(value int64) string {
	if value < 1e12 {
		value *= 1000
	}
	return time.UnixMilli(value).UTC().Format(time.RFC3339)
}

func mustMarshalJSON(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func intPtr(value int) *int {
	result := value
	return &result
}

func clampInt(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func sliceDate(value string) string {
	if len(value) < 10 {
		return value
	}
	return value[:10]
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

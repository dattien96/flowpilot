package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/runner"

	"github.com/pelletier/go-toml/v2"
)

// CP-70 Devin account usage: the CLI's `/usage` slash command has no
// non-interactive equivalent, but it is backed by the Connect-RPC route
// SeatManagementService/GetUserStatus on `api_server_url` from
// credentials.toml (live-verified on devin 3000.10.31). The endpoint accepts
// unary Connect JSON (`application/json` POST) and authenticates via the
// `api_key` field inside `metadata` — the Authorization header is not
// required and no CSRF token is needed. Fields beyond api_key must be present
// but their values are not validated (verified: ide_name "flowpilot" works);
// we send the compat-tested CLI version for plausibility.
//
// Quota mapping note: Devin exposes DAILY + WEEKLY quota windows, not the
// Claude-style 5h/7d pair. We map daily->remaining5hPercent and
// weekly->remaining7dPercent because those generic fields drive the TUI
// quota chips and the desktop fallback-account ranker — the closest
// short/long window semantics available. The usageDetailLines labels keep
// the truthful "Daily quota"/"Weekly quota" names so the meter rows are not
// mislabeled.
const (
	devinUserStatusPath     = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	devinUserStatusTimeout  = 3 * time.Second
	devinUserStatusCacheTTL = 60 * time.Second
)

// devinCredentialsFile mirrors the flat credentials.toml store Devin writes
// on login (live-verified fields; extra keys ignored).
type devinCredentialsFile struct {
	WindsurfAPIKey string `toml:"windsurf_api_key"`
	APIServerURL   string `toml:"api_server_url"`
	DevinAPIURL    string `toml:"devin_api_url"`
}

// devinUserStatusResponse mirrors GetUserStatusResponse (Connect JSON:
// camelCase keys, int64 fields as strings, int32 fields as numbers —
// decoded via `any` + numberFromAny to tolerate either encoding).
type devinUserStatusResponse struct {
	UserStatus struct {
		Email      string `json:"email"`
		Name       string `json:"name"`
		PlanStatus struct {
			PlanStart              string `json:"planStart"`
			PlanEnd                string `json:"planEnd"`
			AvailablePromptCredits any    `json:"availablePromptCredits"`
			UsedPromptCredits      any    `json:"usedPromptCredits"`
			DailyQuotaRemaining    any    `json:"dailyQuotaRemainingPercent"`
			WeeklyQuotaRemaining   any    `json:"weeklyQuotaRemainingPercent"`
			DailyQuotaResetAtUnix  any    `json:"dailyQuotaResetAtUnix"`
			WeeklyQuotaResetAtUnix any    `json:"weeklyQuotaResetAtUnix"`
			PlanInfo               struct {
				PlanName  string `json:"planName"`
				DevinInfo struct {
					AccountDisplayName string `json:"accountDisplayName"`
				} `json:"devinInfo"`
			} `json:"planInfo"`
		} `json:"planStatus"`
	} `json:"userStatus"`
}

type devinUserStatusCacheEntry struct {
	status    *devinUserStatusResponse
	fetchedAt time.Time
}

var (
	devinUserStatusCacheMu sync.Mutex
	devinUserStatusCache   = map[string]devinUserStatusCacheEntry{}
	// devinUserStatusNow is a test seam for cache TTL assertions.
	devinUserStatusNow = time.Now
	// devinUserStatusHTTPDo is a test seam — httptest.Server without a real
	// Devin backend.
	devinUserStatusHTTPDo = func(request *http.Request) (*http.Response, error) {
		return httpClient().Do(request)
	}
)

// loadDevinAccountMetadata fills the shared account metadata from the Devin
// user-status API (CP-70). Best-effort: any failure degrades to a bare
// metadata entry so account listing never stalls or loses the account.
func loadDevinAccountMetadata(homePath string) (accountLaunchMetadata, error) {
	metadata := accountLaunchMetadata{authStorePath: homePath}

	credPath := firstExistingPath(runner.DevinCredentialFilePaths(homePath)...)
	if credPath == "" {
		return metadata, nil
	}
	metadata.authStorePath = credPath

	creds, err := readDevinCredentials(credPath)
	if err != nil || creds.WindsurfAPIKey == "" || creds.apiBase() == "" {
		return metadata, nil
	}

	status := loadDevinUserStatus(credPath, creds)
	if status == nil {
		return metadata, nil
	}
	applyDevinUserStatus(&metadata, status)
	return metadata, nil
}

func (c devinCredentialsFile) apiBase() string {
	if base := strings.TrimSpace(c.APIServerURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(strings.TrimSpace(c.DevinAPIURL), "/")
}

func readDevinCredentials(path string) (devinCredentialsFile, error) {
	var creds devinCredentialsFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return creds, err
	}
	if err := toml.Unmarshal(raw, &creds); err != nil {
		return creds, err
	}
	creds.WindsurfAPIKey = strings.TrimSpace(creds.WindsurfAPIKey)
	creds.APIServerURL = strings.TrimSpace(creds.APIServerURL)
	creds.DevinAPIURL = strings.TrimSpace(creds.DevinAPIURL)
	return creds, nil
}

// loadDevinUserStatus fetches GetUserStatus bounded + 60s-cached per
// credentials file (mirrors opencodeStatsDetailLines — /provider-accounts
// fans out per account on every TUI connect / panel open, and failures are
// cached too so a wedged endpoint cannot stall refresh).
func loadDevinUserStatus(credPath string, creds devinCredentialsFile) *devinUserStatusResponse {
	devinUserStatusCacheMu.Lock()
	if entry, ok := devinUserStatusCache[credPath]; ok && devinUserStatusNow().Sub(entry.fetchedAt) < devinUserStatusCacheTTL {
		status := entry.status
		devinUserStatusCacheMu.Unlock()
		return status
	}
	devinUserStatusCacheMu.Unlock()

	status := fetchDevinUserStatus(creds)

	devinUserStatusCacheMu.Lock()
	devinUserStatusCache[credPath] = devinUserStatusCacheEntry{status: status, fetchedAt: devinUserStatusNow()}
	devinUserStatusCacheMu.Unlock()
	return status
}

func fetchDevinUserStatus(creds devinCredentialsFile) *devinUserStatusResponse {
	apiKey := strings.TrimSpace(creds.WindsurfAPIKey)
	base := creds.apiBase()
	if apiKey == "" || base == "" {
		return nil
	}

	body := mustMarshalJSON(map[string]any{
		"metadata": map[string]any{
			"api_key":           apiKey,
			"ide_name":          "devin",
			"ide_version":       runner.CompatTestedDevinVersion,
			"extension_name":    "devin",
			"extension_version": runner.CompatTestedDevinVersion,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), devinUserStatusTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+devinUserStatusPath, strings.NewReader(body))
	if err != nil {
		return nil
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := devinUserStatusHTTPDo(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil
	}

	var decoded devinUserStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil
	}
	if strings.TrimSpace(decoded.UserStatus.Email) == "" && decoded.UserStatus.PlanStatus.DailyQuotaRemaining == nil {
		return nil // empty/foreign payload — treat as unavailable
	}
	return &decoded
}

// applyDevinUserStatus maps GetUserStatus onto the shared account metadata.
// Devin's quota windows are daily+weekly: daily maps onto the generic
// short-window slot (remaining5hPercent) and weekly onto the long-window slot
// (remaining7dPercent) so quota chips and the fallback ranker work; the
// detail lines carry truthful "Daily quota"/"Weekly quota" labels.
func applyDevinUserStatus(metadata *accountLaunchMetadata, status *devinUserStatusResponse) {
	if metadata == nil || status == nil {
		return
	}
	user := status.UserStatus
	plan := user.PlanStatus

	metadata.accountEmail = strings.TrimSpace(user.Email)
	if display := strings.TrimSpace(plan.PlanInfo.DevinInfo.AccountDisplayName); display != "" {
		metadata.accountName = display
	} else {
		metadata.accountName = strings.TrimSpace(user.Name)
	}

	planName := strings.TrimSpace(plan.PlanInfo.PlanName)
	if planName != "" {
		metadata.usageSummary = planName
		if end := sliceDate(plan.PlanEnd); end != "" {
			metadata.usageSummary += " until " + end
		}
	}

	if daily, ok := devinQuotaPercent(plan.DailyQuotaRemaining); ok {
		metadata.remaining5hPercent = intPtr(daily)
		metadata.remaining5hResetAt = parseResetTime(plan.DailyQuotaResetAtUnix)
		metadata.usageDetailLines = append(metadata.usageDetailLines, usageDetailLine{
			label:            "Daily quota",
			remainingPercent: daily,
			resetAt:          metadata.remaining5hResetAt,
		})
	}
	if weekly, ok := devinQuotaPercent(plan.WeeklyQuotaRemaining); ok {
		metadata.remaining7dPercent = intPtr(weekly)
		metadata.remaining7dResetAt = parseResetTime(plan.WeeklyQuotaResetAtUnix)
		metadata.usageDetailLines = append(metadata.usageDetailLines, usageDetailLine{
			label:            "Weekly quota",
			remainingPercent: weekly,
			resetAt:          metadata.remaining7dResetAt,
		})
	}
}

func devinQuotaPercent(value any) (int, bool) {
	numeric, ok := numberFromAny(value)
	if !ok {
		// Connect JSON can emit int64-as-string; try that too.
		if s, isString := value.(string); isString {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return clampInt(int(parsed)), true
			}
		}
		return 0, false
	}
	return clampInt(int(numeric)), true
}

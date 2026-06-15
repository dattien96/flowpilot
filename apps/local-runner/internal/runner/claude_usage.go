package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type claudeLocalAuthFile struct {
	CachedExtraUsageDisabledReason string `json:"cachedExtraUsageDisabledReason"`
}

func claudeUsageLimitError(homePath string) error {
	for _, path := range accountAuthPaths(string(ProviderKeyClaude), homePath) {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var auth claudeLocalAuthFile
		if err := json.Unmarshal(raw, &auth); err != nil {
			continue
		}
		reason := strings.TrimSpace(auth.CachedExtraUsageDisabledReason)
		if !isClaudeUsageLimitReason(reason) {
			continue
		}
		return fmt.Errorf("Claude usage limit reached: extra usage unavailable (%s). Switch Claude account or wait for quota reset; login is still present", humanizeReason(reason))
	}
	return nil
}

func isClaudeUsageLimitReason(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if normalized == "" {
		return false
	}
	for _, token := range []string{"out_of_credits", "credit", "quota", "limit", "rate_limit", "usage"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func humanizeReason(reason string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(reason), "_", " "))
}

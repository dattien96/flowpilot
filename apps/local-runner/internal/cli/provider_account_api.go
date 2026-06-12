package cli

import (
	"sort"
	"strings"

	"flowpilot-runner/internal/runner"
)

type providerAccountUsageLineResponse struct {
	Label            string  `json:"label"`
	RemainingPercent int     `json:"remaining_percent"`
	ResetAt          *string `json:"reset_at"`
}

type providerAccountSummaryResponse struct {
	ID                     string                             `json:"id"`
	ProviderKey            string                             `json:"provider_key"`
	DisplayName            string                             `json:"display_name"`
	DisplayLabel           string                             `json:"display_label"`
	HomePath               string                             `json:"home_path"`
	AuthStorePath          *string                            `json:"auth_store_path"`
	SlotIndex              int                                `json:"slot_index"`
	AuthStatus             string                             `json:"auth_status"`
	IsActive               bool                               `json:"is_active"`
	CreatedAt              string                             `json:"created_at"`
	LastAuthenticatedAt    *string                            `json:"last_authenticated_at"`
	AccountEmail           *string                            `json:"account_email"`
	AccountName            *string                            `json:"account_name"`
	UsageSummary           *string                            `json:"usage_summary"`
	Remaining5hPercent     *int                               `json:"remaining_5h_percent"`
	Remaining7dPercent     *int                               `json:"remaining_7d_percent"`
	Remaining5hResetAt     *string                            `json:"remaining_5h_reset_at"`
	Remaining7dResetAt     *string                            `json:"remaining_7d_reset_at"`
	UsageSource            string                             `json:"usage_source"`
	AccessTokenExpiresAt   *string                            `json:"access_token_expires_at"`
	RefreshTokenExpiresAt  *string                            `json:"refresh_token_expires_at"`
	RefreshTokenExpiryNote *string                            `json:"refresh_token_expiry_note"`
	UsageDetailLines       []providerAccountUsageLineResponse `json:"usage_detail_lines"`
}

func buildProviderAccountSummaryResponses(accounts []runner.ProviderAccount) ([]providerAccountSummaryResponse, error) {
	summaries := make([]providerAccountSummaryResponse, 0, len(accounts))
	for _, account := range accounts {
		metadata, err := loadAccountLaunchMetadata(account)
		if err != nil {
			return nil, err
		}

		usageLines := make([]providerAccountUsageLineResponse, 0, len(metadata.usageDetailLines))
		for _, line := range metadata.usageDetailLines {
			usageLines = append(usageLines, providerAccountUsageLineResponse{
				Label:            line.label,
				RemainingPercent: line.remainingPercent,
				ResetAt:          stringPtr(line.resetAt),
			})
		}

		displayLabel := strings.TrimSpace(account.DisplayName)
		if strings.TrimSpace(metadata.accountEmail) != "" {
			displayLabel = strings.TrimSpace(metadata.accountEmail)
		} else if strings.TrimSpace(metadata.accountName) != "" {
			displayLabel = strings.TrimSpace(metadata.accountName)
		}

		usageSource := "unavailable"
		if metadata.remaining5hPercent != nil || metadata.remaining7dPercent != nil || len(usageLines) > 0 {
			usageSource = "provider_api"
		}

		summaries = append(summaries, providerAccountSummaryResponse{
			ID:                     account.ID,
			ProviderKey:            account.ProviderKey,
			DisplayName:            account.DisplayName,
			DisplayLabel:           displayLabel,
			HomePath:               account.HomePath,
			AuthStorePath:          stringPtr(metadata.authStorePath),
			SlotIndex:              account.SlotIndex,
			AuthStatus:             account.AuthStatus,
			IsActive:               account.IsActive,
			CreatedAt:              account.CreatedAt,
			LastAuthenticatedAt:    account.LastAuthenticatedAt,
			AccountEmail:           stringPtr(metadata.accountEmail),
			AccountName:            stringPtr(metadata.accountName),
			UsageSummary:           stringPtr(metadata.usageSummary),
			Remaining5hPercent:     metadata.remaining5hPercent,
			Remaining7dPercent:     metadata.remaining7dPercent,
			Remaining5hResetAt:     stringPtr(metadata.remaining5hResetAt),
			Remaining7dResetAt:     stringPtr(metadata.remaining7dResetAt),
			UsageSource:            usageSource,
			AccessTokenExpiresAt:   stringPtr(metadata.accessTokenExpiresAt),
			RefreshTokenExpiresAt:  stringPtr(metadata.refreshTokenExpiresAt),
			RefreshTokenExpiryNote: stringPtr(metadata.refreshTokenExpiryNote),
			UsageDetailLines:       usageLines,
		})
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		left := summaries[i]
		right := summaries[j]
		if !strings.EqualFold(left.ProviderKey, right.ProviderKey) {
			return strings.ToLower(left.ProviderKey) < strings.ToLower(right.ProviderKey)
		}
		if left.IsActive != right.IsActive {
			return left.IsActive
		}
		return left.SlotIndex < right.SlotIndex
	})

	return summaries, nil
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

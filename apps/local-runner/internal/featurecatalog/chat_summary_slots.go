package featurecatalog

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/changeledger"
)

const recentChatSummaryCount = 3

func ChatSummarySlot(featureKey string, ledger interface {
	GetFeatureSummaries(string) ([]changeledger.ChatSummaryEntry, error)
}) string {
	entries, err := ledger.GetFeatureSummaries(featureKey)
	if err != nil || len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Discussion %q (newest last)\n", featureKey))
	start := 0
	if len(entries) > recentChatSummaryCount {
		start = len(entries) - recentChatSummaryCount
	}
	for i := start; i < len(entries); i++ {
		entry := entries[i]
		marker := ""
		if i == len(entries)-1 {
			marker = "   ← latest"
		}
		date := entry.CreatedAt
		if len(date) > 7 {
			date = date[:7]
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s%s\n", date, entry.Summary, marker))
	}
	return sb.String()
}

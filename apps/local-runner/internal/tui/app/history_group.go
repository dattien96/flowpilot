package app

import (
	"strings"

	"flowpilot-runner/internal/tui/client"
)

// chatGroup is the TUI mirror of Desktop's ChatGroup (chatHistory.ts).
// One logical chat == one group, face is the latest leg, legs keeps all.
type chatGroup struct {
	head client.RunHistoryItem
	legs []client.RunHistoryItem
}

type groupedHistoryRow struct {
	item  client.RunHistoryItem
	group *chatGroup // nil for untagged / workflow rows
}

// groupRunsByChatId collapses consecutive chat runs by chatId (Desktop parity).
// Rows without a chatId (pre-CP-59 or workflow runs) pass through 1:1. Order
// is preserved: each group takes the position of its first-seen leg. The face
// (head) is the newest leg by legSeq then updatedAt (same as Desktop's
// compareNewer), so the picker always opens the latest leg.
func groupRunsByChatId(items []client.RunHistoryItem) []groupedHistoryRow {
	rows := make([]groupedHistoryRow, 0, len(items))
	indexByChat := make(map[string]int, len(items))
	for _, it := range items {
		chatID := strings.TrimSpace(it.ChatID)
		if chatID == "" {
			rows = append(rows, groupedHistoryRow{item: it})
			continue
		}
		if idx, ok := indexByChat[chatID]; ok {
			g := rows[idx].group
			if g != nil {
				g.legs = append(g.legs, it)
				if isNewerRun(it, g.head) {
					g.head = it
					rows[idx] = groupedHistoryRow{item: it, group: g}
				}
			}
			continue
		}
		g := &chatGroup{head: it, legs: []client.RunHistoryItem{it}}
		indexByChat[chatID] = len(rows)
		rows = append(rows, groupedHistoryRow{item: it, group: g})
	}
	return rows
}

func isNewerRun(a, b client.RunHistoryItem) bool {
	if a.LegSeq != b.LegSeq {
		return a.LegSeq > b.LegSeq
	}
	return a.UpdatedAt > b.UpdatedAt
}

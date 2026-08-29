package runner

import (
	"context"
	"strings"
	"sync"
	"time"
)

// CP-57 OC-30 / Q-5 / R-5 / R-9: surface `opencode stats` cost/token usage on
// the account card. Zen is a proxy — no per-model token limit exists, so the
// lines state that explicitly instead of inventing one. The live CLI can take
// seconds (CA-659 skipped it for that reason), so the call is bounded, cached
// briefly (CA-683 — /client/provider-accounts fans out per account on every
// TUI connect and desktop panel open), and degrades to no lines rather than
// delaying account refresh.

const (
	opencodeStatsTimeout  = 3 * time.Second
	opencodeStatsCacheTTL = 60 * time.Second
)

var (
	opencodeStatsCacheMu sync.Mutex
	opencodeStatsCache   = map[string]opencodeStatsCacheEntry{}
)

type opencodeStatsCacheEntry struct {
	lines     []OpencodeAccountUsageLine
	fetchedAt time.Time
}

// opencodeStatsDetailLines runs `opencode stats` (bounded, 60s cached) and
// extracts the overview + cost/token rows for the account card. Failures are
// cached too so a wedged CLI cannot stall every account refresh.
func opencodeStatsDetailLines(ctx context.Context, homePath string) []OpencodeAccountUsageLine {
	opencodeStatsCacheMu.Lock()
	if entry, ok := opencodeStatsCache[homePath]; ok && time.Since(entry.fetchedAt) < opencodeStatsCacheTTL {
		lines := entry.lines
		opencodeStatsCacheMu.Unlock()
		return lines
	}
	opencodeStatsCacheMu.Unlock()

	lines := fetchOpencodeStatsLines(ctx, homePath)

	opencodeStatsCacheMu.Lock()
	opencodeStatsCache[homePath] = opencodeStatsCacheEntry{lines: lines, fetchedAt: time.Now()}
	opencodeStatsCacheMu.Unlock()
	return lines
}

func fetchOpencodeStatsLines(ctx context.Context, homePath string) []OpencodeAccountUsageLine {
	if ctx == nil {
		ctx = context.Background()
	}
	sctx, cancel := context.WithTimeout(ctx, opencodeStatsTimeout)
	defer cancel()
	out, err := runOpencodeCommand(sctx, homePath, "stats")
	if err != nil {
		return nil
	}
	return parseOpencodeStatsTable(string(out))
}

// parseOpencodeStatsTable reads the human-readable `opencode stats` box table
// (no JSON flag exists on 1.18.18). Each row packs key and right-aligned value
// in ONE cell separated by padding:
// "│Total Cost                                      $132.11 │".
func parseOpencodeStatsTable(out string) []OpencodeAccountUsageLine {
	values := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		cells := strings.Split(line, "│")
		if len(cells) < 2 {
			continue
		}
		fields := strings.Fields(cells[1])
		if len(fields) < 2 {
			continue
		}
		key := strings.Join(fields[:len(fields)-1], " ")
		val := fields[len(fields)-1]
		values[key] = val
	}
	if len(values) == 0 {
		return nil
	}
	var lines []OpencodeAccountUsageLine
	if sessions := values["Sessions"]; sessions != "" {
		label := "stats: " + sessions + " sessions"
		if days := values["Days"]; days != "" {
			label += " · " + days + " days"
		}
		lines = append(lines, OpencodeAccountUsageLine{Label: label})
	}
	if total := values["Total Cost"]; total != "" {
		label := "cost: " + total + " total"
		if avg := values["Avg Cost/Day"]; avg != "" {
			label += " · " + avg + "/day"
		}
		lines = append(lines, OpencodeAccountUsageLine{Label: label})
	}
	if avg := values["Avg Tokens/Session"]; avg != "" {
		label := "tokens: " + avg + " avg/session"
		if med := values["Median Tokens/Session"]; med != "" {
			label += " · " + med + " median"
		}
		lines = append(lines, OpencodeAccountUsageLine{Label: label})
	}
	lines = append(lines, OpencodeAccountUsageLine{
		Label: "token limit: N/A — zen proxy, depends on the upstream model (`opencode stats` shows usage)",
	})
	return lines
}

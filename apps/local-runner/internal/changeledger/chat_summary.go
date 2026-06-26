package changeledger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type ChatSummaryEntry struct {
	RunID      string `json:"run_id"`
	TurnID     string `json:"turn_id"`
	FeatureKey string `json:"feature_key"`
	StateKey   string `json:"state_key,omitempty"`
	Summary    string `json:"summary"`
	CreatedAt  string `json:"created_at"`
}

type ChatSummaryLedger struct {
	mu       sync.Mutex
	filePath string
	entries  []ChatSummaryEntry
}

func NewChatSummaryLedger(dotFlowpilotDir string) (*ChatSummaryLedger, error) {
	dir := filepath.Join(dotFlowpilotDir, "ledger")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &ChatSummaryLedger{
		filePath: filepath.Join(dir, "chat_summary.ndjson"),
	}
	l.loadFromDisk()
	return l, nil
}

func (l *ChatSummaryLedger) loadFromDisk() {
	f, err := os.Open(l.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry ChatSummaryEntry
		if json.Unmarshal(line, &entry) == nil && entry.FeatureKey != "" {
			l.entries = append(l.entries, entry)
		}
	}
}

func (l *ChatSummaryLedger) Append(entries []ChatSummaryEntry) error {
	if len(entries) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	fh, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()

	for _, entry := range entries {
		if entry.FeatureKey == "" || entry.Summary == "" {
			continue
		}
		line, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		if _, err := fh.Write(append(line, '\n')); err != nil {
			return err
		}
		l.entries = append(l.entries, entry)
	}
	return nil
}

// UpsertForRun records the single current summary for a (run_id, feature_key)
// pair: any existing entry for that pair is replaced and the file rewritten, so
// a rolling summary stays one line per chat-session per feature instead of
// growing per turn. Entries for other runs/features are preserved.
func (l *ChatSummaryLedger) UpsertForRun(entry ChatSummaryEntry) error {
	if entry.FeatureKey == "" || entry.Summary == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	kept := make([]ChatSummaryEntry, 0, len(l.entries)+1)
	for _, e := range l.entries {
		if e.RunID == entry.RunID && e.FeatureKey == entry.FeatureKey {
			continue
		}
		kept = append(kept, e)
	}
	kept = append(kept, entry)

	if err := l.rewrite(kept); err != nil {
		return err
	}
	l.entries = kept
	return nil
}

// rewrite atomically replaces the ledger file with entries (write temp + rename).
func (l *ChatSummaryLedger) rewrite(entries []ChatSummaryEntry) error {
	tmp := l.filePath + ".tmp"
	fh, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := fh.Write(append(line, '\n')); err != nil {
			fh.Close()
			return err
		}
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, l.filePath)
}

func (l *ChatSummaryLedger) GetFeatureSummaries(featureKey string) ([]ChatSummaryEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []ChatSummaryEntry
	for _, entry := range l.entries {
		if entry.FeatureKey == featureKey {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}

func (l *ChatSummaryLedger) GetFeatureSummariesForRun(featureKey, runID string) ([]ChatSummaryEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var out []ChatSummaryEntry
	for _, entry := range l.entries {
		if entry.FeatureKey == featureKey && entry.RunID == runID {
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt
	})
	return out, nil
}

func (l *ChatSummaryLedger) ListFeatures() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	seen := map[string]struct{}{}
	for _, entry := range l.entries {
		if entry.FeatureKey != "" {
			seen[entry.FeatureKey] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

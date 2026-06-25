package changeledger

import (
	"sort"
)

// GetFeatureHistory returns all entries for featureKey sorted ascending by CommittedAt
// (index 0 = oldest, last entry = newest/current truth). OrderIndex is reassigned
// in the returned slice to match the sort position.
func (l *Ledger) GetFeatureHistory(featureKey string) ([]Entry, error) {
	all := l.AllEntries()

	var matched []Entry
	for _, e := range all {
		if e.FeatureKey == featureKey {
			matched = append(matched, e)
		}
	}
	if len(matched) == 0 {
		return nil, nil
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CommittedAt < matched[j].CommittedAt
	})

	for i := range matched {
		matched[i].OrderIndex = i
	}
	return matched, nil
}

// ListFeatures returns the sorted list of distinct feature keys present in the ledger.
func (l *Ledger) ListFeatures() []string {
	all := l.AllEntries()

	seen := make(map[string]struct{})
	for _, e := range all {
		if e.FeatureKey != "" {
			seen[e.FeatureKey] = struct{}{}
		}
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// LatestEntry returns the most recent entry for featureKey (newest CommittedAt),
// or false if no entries exist for that key.
func (l *Ledger) LatestEntry(featureKey string) (Entry, bool) {
	history, _ := l.GetFeatureHistory(featureKey)
	if len(history) == 0 {
		return Entry{}, false
	}
	return history[len(history)-1], true
}

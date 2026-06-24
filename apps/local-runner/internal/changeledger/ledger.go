package changeledger

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const (
	ConfidenceHigh = "high"
	ConfidenceLow  = "low"
)

// Entry is one commit's contribution to the feature change ledger (Plane C, SD-17 §3.2).
// Ordered ascending by CommittedAt: index 0 = oldest, max OrderIndex = newest truth.
type Entry struct {
	CommitHash  string `json:"commit_hash"`
	FeatureKey  string `json:"feature_key"`
	Layer       string `json:"layer,omitempty"` // optional third commit tag: ui | api | domain | data | ...
	SourceDocID string `json:"source_doc_id"`   // Task-087 | BUG-130 | CP-35 | ""
	ChangeType  string `json:"change_type"`     // feature | bugfix | refactor | docs | hotfix | other
	Summary     string `json:"summary"`
	CommittedAt string `json:"committed_at"` // RFC3339
	OrderIndex  int    `json:"order_index"`  // ascending by commit time; newest = max
	Confidence  string `json:"confidence"`   // "high" | "low"
}

// Ledger is a persistent, mutex-guarded NDJSON store for feature change entries.
// It mirrors local_file_session_store.go: last-wins by CommitHash, one Entry per line.
type Ledger struct {
	mu       sync.Mutex
	filePath string
	entries  map[string]Entry // keyed by commit_hash
}

// New opens (or creates) the ledger at <dotFlowpilotDir>/ledger/feature_history.ndjson.
// Existing entries are loaded immediately.
func New(dotFlowpilotDir string) (*Ledger, error) {
	dir := filepath.Join(dotFlowpilotDir, "ledger")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Ledger{
		filePath: filepath.Join(dir, "feature_history.ndjson"),
		entries:  make(map[string]Entry),
	}
	l.loadFromDisk()
	return l, nil
}

// loadFromDisk reads all NDJSON lines and populates the in-memory map (last-wins).
// Missing file is normal on first run and is silently ignored.
func (l *Ledger) loadFromDisk() {
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
		var e Entry
		if json.Unmarshal(line, &e) == nil && e.CommitHash != "" {
			l.entries[e.CommitHash] = e
		}
	}
}

// Upsert appends entries to disk and updates the in-memory map (last-wins by CommitHash).
// Never fatal: a failed write returns an error but does not corrupt existing data.
func (l *Ledger) Upsert(entries []Entry) error {
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

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := fh.Write(append(line, '\n')); err != nil {
			return err
		}
		l.entries[e.CommitHash] = e
	}
	return nil
}

// AllEntries returns a snapshot of all in-memory entries.
func (l *Ledger) AllEntries() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, e)
	}
	return out
}

// Compact rewrites the NDJSON file with one line per CommitHash (deduplicates
// accumulated last-wins appends). Atomic: write to .tmp then rename.
func (l *Ledger) Compact() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tmpPath := l.filePath + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, e := range l.entries {
		line, err := json.Marshal(e)
		if err != nil {
			tmp.Close()
			_ = os.Remove(tmpPath)
			return err
		}
		if _, err := tmp.Write(append(line, '\n')); err != nil {
			tmp.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, l.filePath)
}

// Build runs a full incremental update: parse new git commits, enrich feature_key,
// then upsert into the ledger. Non-fatal: returns nil when the repo has no commits yet.
func Build(repoDir, dotFlowpilotDir string) error {
	l, err := New(dotFlowpilotDir)
	if err != nil {
		return err
	}

	raw, err := ParseRepo(repoDir, dotFlowpilotDir)
	if err != nil || len(raw) == 0 {
		return err
	}

	enriched := EnrichAll(raw, repoDir)
	return l.Upsert(enriched)
}

// CursorPath returns the path of the incremental cursor file used by ParseRepo.
func CursorPath(dotFlowpilotDir string) string {
	return filepath.Join(dotFlowpilotDir, "ledger", ".cursor")
}

// ledgerDir is exposed for tests to locate the storage directory.
func LedgerDir(dotFlowpilotDir string) string {
	return filepath.Join(dotFlowpilotDir, "ledger")
}

// errNoCommits is returned internally when git log produces no output.
var errNoCommits = errors.New("changeledger: no commits found")

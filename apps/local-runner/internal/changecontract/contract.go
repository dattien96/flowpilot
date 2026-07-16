// Package changecontract implements Task-184 (CP-43 P-1): capturing a
// per-turn Change Contract — what feature/intent/files the AI declared (or,
// absent a declaration, inferred from its diff) before a code-mutating step.
// Task-185 consumes the stored Contract to compute scope drift; this package
// only captures and persists it (SD-21 §4/§5/§7 step 1).
package changecontract

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Confidence values for how a Contract was obtained (SD-21 §5).
const (
	ConfidenceDeclared = "declared"
	ConfidenceInferred = "inferred"
)

// Contract is the per-turn declared-or-inferred change scope (SD-21 §5).
// It is local-only (never Drive-synced) and last-wins by (RunID, StepID).
type Contract struct {
	RunID           string    `json:"run_id"`
	StepID          string    `json:"step_id"`
	FeatureKey      string    `json:"feature_key"`
	Intent          string    `json:"intent"`
	DeclaredPaths   []string  `json:"declared_paths,omitempty"`
	DeclaredSymbols []string  `json:"declared_symbols,omitempty"`
	DeclaredAt      time.Time `json:"declared_at"`
	Confidence      string    `json:"confidence"`
}

// key identifies a Contract's last-wins slot: one per (run_id, step_id).
func (c Contract) key() string { return c.RunID + "\x00" + c.StepID }

// Store persists Contracts to <workspace>/.flowpilot/contracts/contracts.ndjson.
// Mirrors the local_file_session_store.go NDJSON idiom: mutex-guarded
// append-only file, last-wins by key loaded into an in-memory map on startup.
type Store struct {
	mu       sync.Mutex
	filePath string
	byKey    map[string]Contract
	// latestByRun tracks append-order last-wins per runID for GetLatestForRun
	// (Task-247 / Codex review: do not pick by DeclaredAt alone).
	latestByRun map[string]Contract
}

// NewStore creates a Store rooted at <workspace>/.flowpilot/contracts/. The
// directory is created if missing. Existing contracts are loaded immediately
// (last line per key wins) so Get works right after construction.
func NewStore(workspace string) (*Store, error) {
	dir := filepath.Join(workspace, ".flowpilot", "contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		filePath:    filepath.Join(dir, "contracts.ndjson"),
		byKey:       make(map[string]Contract),
		latestByRun: make(map[string]Contract),
	}
	s.loadFromDisk()
	return s, nil
}

func (s *Store) loadFromDisk() {
	f, err := os.Open(s.filePath)
	if err != nil {
		return // missing file is not an error — first run
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var c Contract
		if err := json.Unmarshal(line, &c); err != nil {
			continue // tolerate a corrupt/partial line, never fail the load
		}
		s.byKey[c.key()] = c // last-wins by (run,step)
		if rid := strings.TrimSpace(c.RunID); rid != "" {
			s.latestByRun[rid] = c // append-order last-wins per run
		}
	}
}

// Save persists c, overwriting any prior Contract for the same
// (RunID, StepID). Non-fatal by design (SS-14 AC-9): callers should treat a
// returned error as "capture degraded", never as a reason to block the turn.
func (s *Store) Save(c Contract) error {
	if c.DeclaredAt.IsZero() {
		c.DeclaredAt = time.Now().UTC()
	}
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	s.byKey[c.key()] = c
	if rid := strings.TrimSpace(c.RunID); rid != "" {
		s.latestByRun[rid] = c
	}
	return nil
}

// Get returns the most recently saved Contract for (runID, stepID).
func (s *Store) Get(runID, stepID string) (Contract, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byKey[(Contract{RunID: runID, StepID: stepID}).key()]
	return c, ok
}

// OpenStoreReadOnly loads an existing contracts.ndjson without creating the
// contracts directory (Task-247). Missing file → empty store, no error.
func OpenStoreReadOnly(workspace string) (*Store, error) {
	path := filepath.Join(workspace, ".flowpilot", "contracts", "contracts.ndjson")
	s := &Store{
		filePath:    path,
		byKey:       make(map[string]Contract),
		latestByRun: make(map[string]Contract),
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	s.loadFromDisk()
	return s, nil
}

// GetLatestForRun returns the append-order last-wins Contract for runID
// across any step (Task-247 v1). Independent of DeclaredAt so a later append
// with an older/equal timestamp still wins.
func (s *Store) GetLatestForRun(runID string) (Contract, bool) {
	if s == nil || runID == "" {
		return Contract{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.latestByRun[runID]
	return c, ok
}

// RenderContractBlock renders c as a headerless prompt body (Task-247;
// CP-50 P-4: callers own the heading). Zero-value → "".
func RenderContractBlock(c Contract) string {
	if strings.TrimSpace(c.FeatureKey) == "" && len(c.DeclaredPaths) == 0 {
		return ""
	}
	var b strings.Builder
	if fk := strings.TrimSpace(c.FeatureKey); fk != "" {
		b.WriteString("Feature: ")
		b.WriteString(fk)
		b.WriteByte('\n')
	}
	if intent := strings.TrimSpace(c.Intent); intent != "" {
		b.WriteString("Intent: ")
		b.WriteString(intent)
		b.WriteByte('\n')
	}
	b.WriteString("Declared scope (KHÔNG sửa ngoài các path này):\n")
	if len(c.DeclaredPaths) == 0 {
		b.WriteString("- (none declared)\n")
	} else {
		for _, p := range c.DeclaredPaths {
			b.WriteString("- ")
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
	conf := strings.TrimSpace(c.Confidence)
	if conf == "" {
		conf = ConfidenceDeclared
	}
	b.WriteString("Confidence: ")
	b.WriteString(conf)
	if conf == ConfidenceInferred {
		b.WriteString(" (suy ra từ diff, chưa được AI xác nhận)")
	}
	b.WriteByte('\n')
	return strings.TrimSpace(b.String())
}

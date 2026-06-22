package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const sessionStoreMaxAge = 90 * 24 * time.Hour

// localFileSessionStore implements WorkflowStore + InteractiveStateStore +
// SessionHistoryReader backed by a NDJSON file at dataDir/sessions.ndjson.
// It embeds fakeWorkflowStore for all WorkflowStore methods so the orchestration
// layer behaves identically to the default no-Supabase path. Only
// UpsertProviderSession adds a disk write-through so session metadata survives
// process restarts (BUG-080).
type localFileSessionStore struct {
	*fakeWorkflowStore
	mu       sync.Mutex
	filePath string
}

// NewLocalFileSessionStore creates a localFileSessionStore rooted at dataDir.
// The directory is created if it does not exist. Sessions from a previous
// process are loaded immediately so history is available from the first call
// to ListProviderSessionsByProject.
func NewLocalFileSessionStore(dataDir string) (*localFileSessionStore, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	s := &localFileSessionStore{
		fakeWorkflowStore: newFakeWorkflowStore(),
		filePath:          filepath.Join(dataDir, "sessions.ndjson"),
	}
	s.loadFromDisk()
	return s, nil
}

// ndjsonSessionRecord is the on-disk JSON shape for a ProviderSessionState.
type ndjsonSessionRecord struct {
	RunID             string   `json:"run_id"`
	ProjectID         string   `json:"project_id"`
	WorkflowID        string   `json:"workflow_id,omitempty"`
	ProviderKey       string   `json:"provider_key"`
	ProviderSessionID string   `json:"provider_session_id,omitempty"`
	ProviderAccountID string   `json:"provider_account_id,omitempty"`
	WorkingDirectory  string   `json:"working_directory,omitempty"`
	Status            string   `json:"status"`
	LastPrompt        string   `json:"last_prompt,omitempty"`
	LastMessage       string   `json:"last_message,omitempty"`
	StartedAt         string   `json:"started_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	RunKind           string   `json:"run_kind,omitempty"`
	SourceMachineID   string   `json:"source_machine_id,omitempty"`
	SourceRunID       string   `json:"source_run_id,omitempty"`
	RestoredFrom      string   `json:"restored_from,omitempty"`
	SyncStatus        string   `json:"sync_status,omitempty"`
	SyncUpdatedAt     string   `json:"sync_updated_at,omitempty"`
	ParentRunID       string   `json:"parent_run_id,omitempty"`
	AgentName         string   `json:"agent_name,omitempty"`
	Role              string   `json:"role,omitempty"`
	DependsOn         []string `json:"depends_on,omitempty"`
	AgentStatus       string   `json:"agent_status,omitempty"`
	ModelName         string   `json:"model_name,omitempty"`
}

// loadFromDisk reads the NDJSON file, applies last-wins dedup per run_id, and
// populates the in-memory sessions map. Entries older than sessionStoreMaxAge
// (measured by updated_at) are pruned. Malformed lines are silently skipped.
func (s *localFileSessionStore) loadFromDisk() {
	f, err := os.Open(s.filePath)
	if err != nil {
		return // missing file is normal on first run
	}
	defer f.Close()

	cutoff := time.Now().UTC().Add(-sessionStoreMaxAge)
	seen := map[string]ndjsonSessionRecord{}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ndjsonSessionRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.RunID == "" || rec.ProjectID == "" {
			continue
		}
		if rec.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, rec.UpdatedAt); err == nil && t.Before(cutoff) {
				continue
			}
		}
		seen[rec.RunID] = rec // last-wins
	}

	s.fakeWorkflowStore.mu.Lock()
	for _, rec := range seen {
		s.fakeWorkflowStore.sessions[rec.RunID] = sessionStateFromRecord(rec)
	}
	s.fakeWorkflowStore.mu.Unlock()
}

// UpsertProviderSession updates the in-memory map and appends a NDJSON line to
// the file so the state survives the next restart.
func (s *localFileSessionStore) UpsertProviderSession(_ context.Context, session ProviderSessionState) error {
	s.fakeWorkflowStore.mu.Lock()
	s.fakeWorkflowStore.sessions[session.RunID] = session
	s.fakeWorkflowStore.mu.Unlock()

	rec := sessionRecordFrom(session)
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	fh, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(line, '\n'))
	return err
}

// DeleteProviderSession removes a run from the in-memory map and rewrites the
// NDJSON file without that run_id. The rewrite is atomic (write to a temp file
// then rename) so a crash mid-write does not corrupt the store.
func (s *localFileSessionStore) DeleteProviderSession(_ context.Context, runID string) error {
	s.fakeWorkflowStore.mu.Lock()
	delete(s.fakeWorkflowStore.sessions, runID)
	s.fakeWorkflowStore.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Read existing file, keep every line whose run_id differs from runID.
	f, err := os.Open(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // nothing to rewrite
		}
		return err
	}
	var kept [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ndjsonSessionRecord
		if jsonErr := json.Unmarshal(line, &rec); jsonErr != nil || rec.RunID == runID {
			continue // drop malformed lines and the target run
		}
		kept = append(kept, append([]byte(nil), line...))
	}
	f.Close()

	// Write to a sibling temp file then rename for atomicity.
	tmpPath := s.filePath + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, line := range kept {
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
	return os.Rename(tmpPath, s.filePath)
}

// ListProviderSessionsByProject returns sessions for the given project from the
// in-memory map (populated from disk on startup and kept current by
// UpsertProviderSession). No extra file read is needed here.
func (s *localFileSessionStore) ListProviderSessionsByProject(ctx context.Context, projectID string) ([]ProviderSessionState, error) {
	return s.fakeWorkflowStore.ListProviderSessionsByProject(ctx, projectID)
}

func (s *localFileSessionStore) GetProviderSession(_ context.Context, runID string) (ProviderSessionState, bool, error) {
	s.fakeWorkflowStore.mu.Lock()
	defer s.fakeWorkflowStore.mu.Unlock()
	st, ok := s.fakeWorkflowStore.sessions[runID]
	return st, ok, nil
}

func (s *localFileSessionStore) ListAllProviderSessions(ctx context.Context) ([]ProviderSessionState, error) {
	return s.fakeWorkflowStore.ListAllProviderSessions(ctx)
}

func sessionStateFromRecord(r ndjsonSessionRecord) ProviderSessionState {
	return ProviderSessionState{
		RunID:             r.RunID,
		ProjectID:         r.ProjectID,
		WorkflowID:        r.WorkflowID,
		ProviderKey:       ProviderKey(r.ProviderKey),
		ProviderSessionID: r.ProviderSessionID,
		ProviderAccountID: r.ProviderAccountID,
		WorkingDirectory:  r.WorkingDirectory,
		Status:            RunStatus(r.Status),
		LastPrompt:        r.LastPrompt,
		LastMessage:       r.LastMessage,
		StartedAt:         r.StartedAt,
		UpdatedAt:         r.UpdatedAt,
		RunKind:           r.RunKind,
		SourceMachineID:   r.SourceMachineID,
		SourceRunID:       r.SourceRunID,
		RestoredFrom:      r.RestoredFrom,
		SyncStatus:        r.SyncStatus,
		SyncUpdatedAt:     r.SyncUpdatedAt,
		ParentRunID:       r.ParentRunID,
		AgentName:         r.AgentName,
		Role:              r.Role,
		DependsOn:         append([]string(nil), r.DependsOn...),
		AgentStatus:       r.AgentStatus,
		ModelName:         r.ModelName,
	}
}

// turnLogPath returns the path of the per-run turn-log sidecar file.
func (s *localFileSessionStore) turnLogPath(runID string) string {
	return filepath.Join(filepath.Dir(s.filePath), runID+"-turns.ndjson")
}

// AppendTurnLog appends one entry to the run's turn-log sidecar (BUG-083).
func (s *localFileSessionStore) AppendTurnLog(_ context.Context, runID string, line turnLogLine) error {
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(s.turnLogPath(runID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(data, '\n'))
	return err
}

// ReadTurnLog reads every entry from the run's turn-log sidecar (BUG-083).
// Returns nil, nil when the sidecar does not exist (old run or no turns yet).
func (s *localFileSessionStore) ReadTurnLog(_ context.Context, runID string) ([]turnLogLine, error) {
	f, err := os.Open(s.turnLogPath(runID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []turnLogLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l turnLogLine
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Kind != "" {
			lines = append(lines, l)
		}
	}
	return lines, sc.Err()
}

// DeleteTurnLog removes the run's turn-log sidecar (BUG-083).
// No-op when the file does not exist.
func (s *localFileSessionStore) DeleteTurnLog(_ context.Context, runID string) error {
	err := os.Remove(s.turnLogPath(runID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func sessionRecordFrom(s ProviderSessionState) ndjsonSessionRecord {
	return ndjsonSessionRecord{
		RunID:             s.RunID,
		ProjectID:         s.ProjectID,
		WorkflowID:        s.WorkflowID,
		ProviderKey:       string(s.ProviderKey),
		ProviderSessionID: s.ProviderSessionID,
		ProviderAccountID: s.ProviderAccountID,
		WorkingDirectory:  s.WorkingDirectory,
		Status:            string(s.Status),
		LastPrompt:        s.LastPrompt,
		LastMessage:       s.LastMessage,
		StartedAt:         s.StartedAt,
		UpdatedAt:         s.UpdatedAt,
		RunKind:           s.RunKind,
		SourceMachineID:   s.SourceMachineID,
		SourceRunID:       s.SourceRunID,
		RestoredFrom:      s.RestoredFrom,
		SyncStatus:        s.SyncStatus,
		SyncUpdatedAt:     s.SyncUpdatedAt,
		ParentRunID:       s.ParentRunID,
		AgentName:         s.AgentName,
		Role:              s.Role,
		DependsOn:         append([]string(nil), s.DependsOn...),
		AgentStatus:       s.AgentStatus,
		ModelName:         s.ModelName,
	}
}

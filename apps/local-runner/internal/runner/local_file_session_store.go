package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/agentpack"
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
	RunID               string          `json:"run_id"`
	ProjectID           string          `json:"project_id"`
	WorkflowID          string          `json:"workflow_id,omitempty"`
	ProviderKey         string          `json:"provider_key"`
	ProviderSessionID   string          `json:"provider_session_id,omitempty"`
	ProviderAccountID   string          `json:"provider_account_id,omitempty"`
	WorkingDirectory    string          `json:"working_directory,omitempty"`
	Status              string          `json:"status"`
	LastPrompt          string          `json:"last_prompt,omitempty"`
	LastMessage         string          `json:"last_message,omitempty"`
	StartedAt           string          `json:"started_at,omitempty"`
	UpdatedAt           string          `json:"updated_at,omitempty"`
	RunKind             string          `json:"run_kind,omitempty"`
	SourceMachineID     string          `json:"source_machine_id,omitempty"`
	SourceRunID         string          `json:"source_run_id,omitempty"`
	RestoredFrom        string          `json:"restored_from,omitempty"`
	SyncStatus          string          `json:"sync_status,omitempty"`
	SyncUpdatedAt       string          `json:"sync_updated_at,omitempty"`
	ParentRunID         string          `json:"parent_run_id,omitempty"`
	AgentName           string          `json:"agent_name,omitempty"`
	Label               string          `json:"label,omitempty"`
	Role                string          `json:"role,omitempty"`
	DependsOn           []string        `json:"depends_on,omitempty"`
	AgentStatus         string          `json:"agent_status,omitempty"`
	ModelName           string          `json:"model_name,omitempty"`
	PendingAgentContext []string        `json:"pending_agent_context,omitempty"`
	LoopState           *AgentLoopState `json:"loop_state,omitempty"`
	AutoOrchestrate     bool            `json:"auto_orchestrate,omitempty"`
	FlowCohortID        string          `json:"flow_cohort_id,omitempty"`
	// ActiveFlowEdges/ActiveFlowNodes persist a resolved flow's tracked
	// topology across a restart (BUG-NOTE-CP42 #16); see ProviderSessionState.
	ActiveFlowEdges []agentpack.FlowEdge `json:"active_flow_edges,omitempty"`
	ActiveFlowNodes []agentpack.FlowNode `json:"active_flow_nodes,omitempty"`
	// ChatSubMode/ChatFlowRef persist the Chat-Mode orchestration picker
	// selection a run was started with (BUG-263); see ProviderSessionState.
	ChatSubMode string `json:"chat_sub_mode,omitempty"`
	ChatFlowRef string `json:"chat_flow_ref,omitempty"`
}

// loadFromDisk reads the NDJSON file, applies last-wins dedup per run_id, and
// populates the in-memory sessions map. Entries older than sessionStoreMaxAge
// (measured by updated_at) are pruned. Malformed lines are silently skipped.
func (s *localFileSessionStore) loadFromDisk() {
	// Loaded unconditionally, before the sessions.ndjson-missing early return
	// below: a fresh install/data dir with no session ever persisted yet but a
	// question/approval already asked must still load questions.ndjson /
	// approvals.ndjson.
	s.loadQuestionsFromDisk()
	s.loadApprovalsFromDisk()

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

// loadQuestionsFromDisk populates fakeWorkflowStore.questions from
// questions.ndjson (BUG-StaleQuestion-Restart), last-write-wins per QuestionID
// — mirrors loadFromDisk's session handling above. Missing file is normal
// (no question has ever been asked yet).
func (s *localFileSessionStore) loadQuestionsFromDisk() {
	f, err := os.Open(s.questionsFilePath())
	if err != nil {
		return
	}
	defer f.Close()

	seen := map[string]ProviderQuestionState{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ProviderQuestionState
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.QuestionID == "" {
			continue
		}
		seen[rec.QuestionID] = rec // last-wins
	}

	s.fakeWorkflowStore.mu.Lock()
	for id, rec := range seen {
		s.fakeWorkflowStore.questions[id] = rec
	}
	s.fakeWorkflowStore.mu.Unlock()
}

// loadApprovalsFromDisk populates fakeWorkflowStore.approvals from
// approvals.ndjson (BUG-ApprovalReplay-Restart), last-write-wins per
// ApprovalID — the approval-side twin of loadQuestionsFromDisk above. Missing
// file is normal (no approval has ever been asked yet).
func (s *localFileSessionStore) loadApprovalsFromDisk() {
	f, err := os.Open(s.approvalsFilePath())
	if err != nil {
		return
	}
	defer f.Close()

	seen := map[string]ProviderApprovalState{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec ProviderApprovalState
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.ApprovalID == "" {
			continue
		}
		seen[rec.ApprovalID] = rec // last-wins
	}

	s.fakeWorkflowStore.mu.Lock()
	for id, rec := range seen {
		s.fakeWorkflowStore.approvals[id] = rec
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
		RunID:               r.RunID,
		ProjectID:           r.ProjectID,
		WorkflowID:          r.WorkflowID,
		ProviderKey:         ProviderKey(r.ProviderKey),
		ProviderSessionID:   r.ProviderSessionID,
		ProviderAccountID:   r.ProviderAccountID,
		WorkingDirectory:    r.WorkingDirectory,
		Status:              RunStatus(r.Status),
		LastPrompt:          r.LastPrompt,
		LastMessage:         r.LastMessage,
		StartedAt:           r.StartedAt,
		UpdatedAt:           r.UpdatedAt,
		RunKind:             r.RunKind,
		SourceMachineID:     r.SourceMachineID,
		SourceRunID:         r.SourceRunID,
		RestoredFrom:        r.RestoredFrom,
		SyncStatus:          r.SyncStatus,
		SyncUpdatedAt:       r.SyncUpdatedAt,
		ParentRunID:         r.ParentRunID,
		AgentName:           r.AgentName,
		Label:               r.Label,
		Role:                r.Role,
		DependsOn:           append([]string(nil), r.DependsOn...),
		AgentStatus:         r.AgentStatus,
		ModelName:           r.ModelName,
		PendingAgentContext: append([]string(nil), r.PendingAgentContext...),
		LoopState:           loopStateFromPtr(r.LoopState),
		AutoOrchestrate:     r.AutoOrchestrate,
		FlowCohortID:        r.FlowCohortID,
		ActiveFlowEdges:     append([]agentpack.FlowEdge(nil), r.ActiveFlowEdges...),
		ActiveFlowNodes:     append([]agentpack.FlowNode(nil), r.ActiveFlowNodes...),
		ChatSubMode:         r.ChatSubMode,
		ChatFlowRef:         r.ChatFlowRef,
	}
}

func loopStateFromPtr(p *AgentLoopState) AgentLoopState {
	if p == nil {
		return AgentLoopState{}
	}
	return *p
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

// questionsFilePath returns the path of the single questions.ndjson file that
// durably tracks ProviderQuestionState across restarts (BUG-StaleQuestion-
// Restart) — mirrors sessions.ndjson (BUG-080) but keyed by QuestionID.
func (s *localFileSessionStore) questionsFilePath() string {
	return filepath.Join(filepath.Dir(s.filePath), "questions.ndjson")
}

// UpsertQuestion updates the in-memory map (via the embedded fakeWorkflowStore)
// and appends a NDJSON line to questions.ndjson so a question's resolution
// state (resolved + Choice, or expired) survives a process restart
// (BUG-StaleQuestion-Restart) — without this, reconstructRun has no way to
// know whether a restored user_question_required event was already answered.
func (s *localFileSessionStore) UpsertQuestion(ctx context.Context, question ProviderQuestionState) error {
	if err := s.fakeWorkflowStore.UpsertQuestion(ctx, question); err != nil {
		return err
	}
	line, err := json.Marshal(question)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fh, err := os.OpenFile(s.questionsFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(line, '\n'))
	return err
}

// ListQuestionsByRun returns every persisted question state for a run
// (BUG-StaleQuestion-Restart), populated from questions.ndjson at startup and
// kept current by UpsertQuestion. No extra file read is needed here.
func (s *localFileSessionStore) ListQuestionsByRun(ctx context.Context, runID string) ([]ProviderQuestionState, error) {
	return s.fakeWorkflowStore.ListQuestionsByRun(ctx, runID)
}

// approvalsFilePath returns the path of the single approvals.ndjson file that
// durably tracks ProviderApprovalState across restarts (BUG-ApprovalReplay-
// Restart) — the approval-side twin of questionsFilePath, keyed by ApprovalID.
func (s *localFileSessionStore) approvalsFilePath() string {
	return filepath.Join(filepath.Dir(s.filePath), "approvals.ndjson")
}

// UpsertApproval updates the in-memory map (via the embedded fakeWorkflowStore)
// and appends a NDJSON line to approvals.ndjson so an approval's resolution
// (approve/deny/expired + decision) survives a process restart
// (BUG-ApprovalReplay-Restart) — without this, reconstructRun has no way to
// know whether a restored permission_required event was already resolved.
// Mirrors UpsertQuestion exactly.
func (s *localFileSessionStore) UpsertApproval(ctx context.Context, approval ProviderApprovalState) error {
	if err := s.fakeWorkflowStore.UpsertApproval(ctx, approval); err != nil {
		return err
	}
	line, err := json.Marshal(approval)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fh, err := os.OpenFile(s.approvalsFilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(line, '\n'))
	return err
}

// ListApprovalsByRun returns every persisted approval state for a run
// (BUG-ApprovalReplay-Restart), populated from approvals.ndjson at startup and
// kept current by UpsertApproval. No extra file read is needed here.
func (s *localFileSessionStore) ListApprovalsByRun(ctx context.Context, runID string) ([]ProviderApprovalState, error) {
	return s.fakeWorkflowStore.ListApprovalsByRun(ctx, runID)
}

// flowEventsPath returns the path of the per-run CP-41 flow-events sidecar.
// Returns an error when runID contains path separators that could escape the
// store directory (path-traversal guard).
func (s *localFileSessionStore) flowEventsPath(runID string) (string, error) {
	if runID == "" || filepath.Base(runID) != runID || strings.ContainsAny(runID, "/\\") {
		return "", fmt.Errorf("invalid run ID %q: must not contain path separators", runID)
	}
	return filepath.Join(filepath.Dir(s.filePath), runID+"-flow-events.ndjson"), nil
}

// isFlowSidecarEventType reports whether the event type should be persisted to
// the per-run flow-events sidecar so it survives process restarts.
//
// EventUserQuestionRequired is included so the raw "asked" event itself
// survives a restart (BUG-StaleQuestion-Restart) — raw provider transcripts
// (Claude/Codex) have no concept of it, so without this the question would not
// exist at all in the reconstructed timeline, resolved or not. Whether it was
// later answered/expired is tracked separately via ProviderQuestionState
// (UpsertQuestion) and merged onto this replayed event in reconstructRun.
//
// EventPermissionRequired is included for the exact same reason on the
// approval side (BUG-ApprovalReplay-Restart): the provider transcript has
// tool_use/tool_result but no concept of FlowPilot's own approval card, so
// without persisting the raw asked event the resolved approval card vanished
// entirely on a full server restart (only the question survived). Its
// approve/deny/expired resolution is tracked separately via
// ProviderApprovalState (UpsertApproval) and merged onto this replayed event
// in reconstructRun.
func isFlowSidecarEventType(t ProviderEventType) bool {
	switch t {
	case EventFlowContextPackage, EventFlowValidationResult,
		EventFlowValidationRetry, EventFlowAuditDraft,
		EventUserQuestionRequired, EventPermissionRequired:
		return true
	}
	return false
}

// AppendEvent writes to the in-memory store (via the embedded fakeWorkflowStore)
// and, for CP-41 event types, also appends to the per-run flow-events sidecar
// NDJSON so the events survive a process restart.
func (s *localFileSessionStore) AppendEvent(ctx context.Context, event ProviderEvent) error {
	if err := s.fakeWorkflowStore.AppendEvent(ctx, event); err != nil {
		return err
	}
	if !isFlowSidecarEventType(event.Type) || event.WorkflowRunID == "" {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	evPath, err := s.flowEventsPath(event.WorkflowRunID)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(evPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(data, '\n'))
	return err
}

// LoadFlowEvents reads all CP-41 events from the per-run flow-events sidecar.
// Returns nil, nil when the sidecar does not exist (new run or no flow events yet).
// Reads line-by-line with a 1 MiB buffer (avoids bufio.Scanner's 64 KiB limit)
// so a single malformed line only skips that line — later valid lines are kept.
// Malformed lines are logged with runID and line number to aid sidecar diagnosis.
func (s *localFileSessionStore) LoadFlowEvents(_ context.Context, runID string) ([]ProviderEvent, error) {
	evPath, err := s.flowEventsPath(runID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(evPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var evs []ProviderEvent
	br := bufio.NewReaderSize(f, 1<<20) // 1 MiB per-line buffer
	lineNum := 0
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			lineNum++
			// trim CR+LF and skip blank lines
			for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var ev ProviderEvent
				if jsonErr := json.Unmarshal(line, &ev); jsonErr == nil && ev.Type != "" {
					evs = append(evs, ev)
				} else if jsonErr != nil {
					log.Printf("LoadFlowEvents: runID=%s line=%d: malformed JSON: %v", runID, lineNum, jsonErr)
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	return evs, nil
}

// DeleteFlowEvents removes the per-run flow-events sidecar.
// No-op when the file does not exist.
func (s *localFileSessionStore) DeleteFlowEvents(_ context.Context, runID string) error {
	evPath, err := s.flowEventsPath(runID)
	if err != nil {
		return err
	}
	err = os.Remove(evPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// stepTransitionsPath returns the path of the per-run step-transition sidecar
// (Task-239 / T-10). Same traversal guard as flowEventsPath.
//
// Q-2 (Drive sync): local-only, same posture as flow-events — mid-flow cross-PC
// resume is not a required use case; both sidecars live under the same dataDir
// and are not listed in Drive chat-session sync manifests today.
func (s *localFileSessionStore) stepTransitionsPath(runID string) (string, error) {
	if runID == "" || filepath.Base(runID) != runID || strings.ContainsAny(runID, "/\\") {
		return "", fmt.Errorf("invalid run ID %q: must not contain path separators", runID)
	}
	return filepath.Join(filepath.Dir(s.filePath), runID+"-step-transitions.ndjson"), nil
}

// AppendStepTransition appends one step transition line (Task-239). Best-effort
// from the caller's perspective — errors are returned so the caller can log-warn.
func (s *localFileSessionStore) AppendStepTransition(_ context.Context, runID string, line stepTransitionLine) error {
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	path, err := s.stepTransitionsPath(runID)
	if err != nil {
		return err
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.Write(append(data, '\n'))
	return err
}

// LoadStepTransitions reads all step-transition lines for a run. Returns nil, nil
// when the sidecar does not exist. Malformed lines are skipped (same posture as
// LoadFlowEvents) so a single corrupt line cannot block resume.
func (s *localFileSessionStore) LoadStepTransitions(_ context.Context, runID string) ([]stepTransitionLine, error) {
	path, err := s.stepTransitionsPath(runID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines []stepTransitionLine
	br := bufio.NewReaderSize(f, 1<<20)
	lineNum := 0
	for {
		raw, readErr := br.ReadBytes('\n')
		if len(raw) > 0 {
			lineNum++
			for len(raw) > 0 && (raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r') {
				raw = raw[:len(raw)-1]
			}
			if len(raw) > 0 {
				var line stepTransitionLine
				if jsonErr := json.Unmarshal(raw, &line); jsonErr == nil && strings.TrimSpace(line.NodeID) != "" {
					lines = append(lines, line)
				} else if jsonErr != nil {
					log.Printf("LoadStepTransitions: runID=%s line=%d: malformed JSON: %v", runID, lineNum, jsonErr)
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
	}
	return lines, nil
}

// DeleteStepTransitions removes the per-run step-transition sidecar (Task-239 B5).
// No-op when the file does not exist.
func (s *localFileSessionStore) DeleteStepTransitions(_ context.Context, runID string) error {
	path, err := s.stepTransitionsPath(runID)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func sessionRecordFrom(s ProviderSessionState) ndjsonSessionRecord {
	return ndjsonSessionRecord{
		RunID:               s.RunID,
		ProjectID:           s.ProjectID,
		WorkflowID:          s.WorkflowID,
		ProviderKey:         string(s.ProviderKey),
		ProviderSessionID:   s.ProviderSessionID,
		ProviderAccountID:   s.ProviderAccountID,
		WorkingDirectory:    s.WorkingDirectory,
		Status:              string(s.Status),
		LastPrompt:          s.LastPrompt,
		LastMessage:         s.LastMessage,
		StartedAt:           s.StartedAt,
		UpdatedAt:           s.UpdatedAt,
		RunKind:             s.RunKind,
		SourceMachineID:     s.SourceMachineID,
		SourceRunID:         s.SourceRunID,
		RestoredFrom:        s.RestoredFrom,
		SyncStatus:          s.SyncStatus,
		SyncUpdatedAt:       s.SyncUpdatedAt,
		ParentRunID:         s.ParentRunID,
		AgentName:           s.AgentName,
		Label:               s.Label,
		Role:                s.Role,
		DependsOn:           append([]string(nil), s.DependsOn...),
		AgentStatus:         s.AgentStatus,
		ModelName:           s.ModelName,
		PendingAgentContext: append([]string(nil), s.PendingAgentContext...),
		LoopState:           loopStatePtrIfSet(s.LoopState),
		AutoOrchestrate:     s.AutoOrchestrate,
		FlowCohortID:        s.FlowCohortID,
		ActiveFlowEdges:     append([]agentpack.FlowEdge(nil), s.ActiveFlowEdges...),
		ActiveFlowNodes:     append([]agentpack.FlowNode(nil), s.ActiveFlowNodes...),
		ChatSubMode:         s.ChatSubMode,
		ChatFlowRef:         s.ChatFlowRef,
	}
}

func loopStatePtrIfSet(st AgentLoopState) *AgentLoopState {
	if st.Mode == "" && st.Round == 0 && st.Cap == 0 && st.RoundCap == 0 &&
		st.ActiveNode == "" && st.ExtendCount == 0 && st.GateReason == "" {
		return nil
	}
	cp := st
	return &cp
}

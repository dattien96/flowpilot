package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OpenDispatchStoreForServe opens the multi-project local dispatch hub rooted at
// chatsRoot (typically <runner>/.flowpilot/chats). Each project owns:
//
//	chatsRoot/<project_id>/dispatch.ndjson
//	chatsRoot/<project_id>/dispatch.lock
//
// Supabase is not used for dispatch; Drive sync carries the per-project log
// alongside chat session sync.
func OpenDispatchStoreForServe(chatsRoot string) (DispatchStore, error) {
	if strings.TrimSpace(chatsRoot) == "" {
		return NewMemoryDispatchStore(), nil
	}
	if err := os.MkdirAll(chatsRoot, 0o755); err != nil {
		return nil, err
	}
	return newMultiProjectDispatchStore(chatsRoot), nil
}

// DispatchProjectDir returns chatsRoot/<projectID> for a project's dispatch shard.
func DispatchProjectDir(chatsRoot, projectID string) string {
	id := sanitizeProjectID(projectID)
	if id == "" {
		id = "_unknown"
	}
	return filepath.Join(chatsRoot, id)
}

// DispatchLogPath is the NDJSON path for a project's dispatch commit log.
func DispatchLogPath(chatsRoot, projectID string) string {
	return filepath.Join(DispatchProjectDir(chatsRoot, projectID), "dispatch.ndjson")
}

func sanitizeProjectID(projectID string) string {
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ""
	}
	id = filepath.Base(id)
	id = strings.ReplaceAll(id, "..", "_")
	return id
}

// multiProjectDispatchStore routes DispatchStore ops to per-project local logs.
type multiProjectDispatchStore struct {
	root       string
	mu         sync.Mutex
	byProject  map[string]*localDispatchStore
	runProject map[string]string // runID -> projectID
}

func newMultiProjectDispatchStore(root string) *multiProjectDispatchStore {
	m := &multiProjectDispatchStore{
		root:       root,
		byProject:  map[string]*localDispatchStore{},
		runProject: map[string]string{},
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		logPath := filepath.Join(root, e.Name(), "dispatch.ndjson")
		if _, err := os.Stat(logPath); err != nil {
			continue
		}
		_, _ = m.openProjectLocked(e.Name())
	}
	return m
}

func (m *multiProjectDispatchStore) openProjectLocked(projectID string) (*localDispatchStore, error) {
	id := sanitizeProjectID(projectID)
	if id == "" {
		id = "_unknown"
	}
	if st, ok := m.byProject[id]; ok {
		return st, nil
	}
	st, err := NewLocalDispatchStore(DispatchProjectDir(m.root, id))
	if err != nil {
		return nil, err
	}
	m.byProject[id] = st
	for _, rec := range st.records {
		if rec == nil || rec.RunID == "" {
			continue
		}
		m.runProject[rec.RunID] = id
		if rec.ProjectID == "" {
			rec.ProjectID = id
		}
	}
	return st, nil
}

// Close releases every open project's process lock (Codex review 2026-07-17:
// multiProjectDispatchStore held per-project localDispatchStore instances with
// their own flock'd lockFile but had no Close of its own, so a caller closing
// the hub never released the underlying file locks — on Windows this left
// dispatch.lock held open and t.TempDir()'s cleanup failed with "used by
// another process" (misdiagnosed in the 2026-07-17 audit as "flock never
// closed" rather than "hub never closes its children").
func (m *multiProjectDispatchStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for id, st := range m.byProject {
		if st == nil {
			continue
		}
		if err := st.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.byProject, id)
	}
	return firstErr
}

func (m *multiProjectDispatchStore) forProject(projectID string) (*localDispatchStore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.openProjectLocked(projectID)
}

func (m *multiProjectDispatchStore) forRun(runID string) (*localDispatchStore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if pid, ok := m.runProject[runID]; ok {
		return m.openProjectLocked(pid)
	}
	for pid, st := range m.byProject {
		for _, r := range st.records {
			if r != nil && r.RunID == runID {
				m.runProject[runID] = pid
				return st, nil
			}
		}
	}
	entries, _ := os.ReadDir(m.root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		st, err := m.openProjectLocked(e.Name())
		if err != nil {
			continue
		}
		for _, r := range st.records {
			if r != nil && r.RunID == runID {
				m.runProject[runID] = sanitizeProjectID(e.Name())
				return st, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: no project shard for run %s", ErrNotFound, runID)
}

func (m *multiProjectDispatchStore) bindRun(projectID, runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runID != "" {
		m.runProject[runID] = sanitizeProjectID(projectID)
	}
}

// ExportProjectLog returns raw dispatch.ndjson for Drive upload (nil if missing).
func (m *multiProjectDispatchStore) ExportProjectLog(projectID string) ([]byte, error) {
	path := DispatchLogPath(m.root, projectID)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// ImportProjectLog writes Drive download into the project shard and reloads RAM.
func (m *multiProjectDispatchStore) ImportProjectLog(projectID string, raw []byte) error {
	id := sanitizeProjectID(projectID)
	if id == "" {
		return fmt.Errorf("project_id required")
	}
	dir := DispatchProjectDir(m.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m.mu.Lock()
	if old, ok := m.byProject[id]; ok {
		_ = old.Close()
		delete(m.byProject, id)
	}
	for run, pid := range m.runProject {
		if pid == id {
			delete(m.runProject, run)
		}
	}
	m.mu.Unlock()

	path := DispatchLogPath(m.root, id)
	if len(raw) == 0 {
		_ = os.Remove(path)
		return nil
	}
	tmp := path + ".import-tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.openProjectLocked(id)
	return err
}

func (m *multiProjectDispatchStore) ChatsRoot() string { return m.root }

// ---- DispatchStore routing ----

func (m *multiProjectDispatchStore) CreatePrepared(ctx context.Context, rec DispatchRecord, env DispatchEnvelope) error {
	pid := sanitizeProjectID(rec.ProjectID)
	if pid == "" {
		return fmt.Errorf("CreatePrepared: project_id required for per-project dispatch log")
	}
	rec.ProjectID = pid
	st, err := m.forProject(pid)
	if err != nil {
		return err
	}
	if err := st.CreatePrepared(ctx, rec, env); err != nil {
		return err
	}
	m.bindRun(pid, rec.RunID)
	return nil
}

func (m *multiProjectDispatchStore) CASAdvance(ctx context.Context, runID, turnID string, expectedRev int64,
	expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CASAdvance(ctx, runID, turnID, expectedRev, expected, next, mutate)
}

func (m *multiProjectDispatchStore) CASRecoveryAdvance(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	expected, next DispatchState, mutate func(*DispatchRecord)) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CASRecoveryAdvance(ctx, runID, turnID, expectedRev, leaseOwner, expected, next, mutate)
}

func (m *multiProjectDispatchStore) CASAdvanceSettle(ctx context.Context, runID, turnID string, expectedRev int64,
	expected, next SettlePhase) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CASAdvanceSettle(ctx, runID, turnID, expectedRev, expected, next)
}

func (m *multiProjectDispatchStore) ClaimRecovery(ctx context.Context, runID, turnID string, expectedRev int64, owner string, ttl time.Duration) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.ClaimRecovery(ctx, runID, turnID, expectedRev, owner, ttl)
}

func (m *multiProjectDispatchStore) SetCancelRequested(ctx context.Context, runID, turnID string, expectedRev, stopGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.SetCancelRequested(ctx, runID, turnID, expectedRev, stopGen)
}

func (m *multiProjectDispatchStore) GetRunStopState(ctx context.Context, runID string) (RunStopState, error) {
	st, err := m.forRun(runID)
	if err != nil {
		// No shard yet: not stopped.
		if err == ErrNotFound || strings.Contains(err.Error(), "no project shard") {
			return RunStopState{RunID: runID}, nil
		}
		return RunStopState{}, err
	}
	return st.GetRunStopState(ctx, runID)
}

func (m *multiProjectDispatchStore) RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return RunStopState{}, err
	}
	return st.RequestRunStop(ctx, runID, expectedRunStopRev, reason)
}

func (m *multiProjectDispatchStore) CommitReceiptAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	receipt ReceiptEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitReceiptAndClearIntent(ctx, runID, turnID, expectedRev, receipt, intentOwnerRunID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) CommitTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitTerminalAndSettleIntent(ctx, runID, turnID, expectedRev, proof, intentOwnerRunID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) CommitRecoveredTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	leaseOwner string, proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitRecoveredTerminalAndSettleIntent(ctx, runID, turnID, expectedRev, leaseOwner, proof, intentOwnerRunID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) CommitRecoveryUnknownOrRequireCancel(ctx context.Context, runID, turnID string, expectedRev int64,
	leaseOwner string) (RecoveryUnknownDecision, int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return "", 0, err
	}
	return st.CommitRecoveryUnknownOrRequireCancel(ctx, runID, turnID, expectedRev, leaseOwner)
}

func (m *multiProjectDispatchStore) ClaimRecoveryAttach(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string, ttl time.Duration) (RecoveryAttachToken, DispatchRecord, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return RecoveryAttachToken{}, DispatchRecord{}, err
	}
	return st.ClaimRecoveryAttach(ctx, runID, turnID, expectedRev, leaseOwner, ttl)
}

func (m *multiProjectDispatchStore) EnterRecoveryAttach(ctx context.Context, runID, turnID string, token RecoveryAttachToken) error {
	st, err := m.forRun(runID)
	if err != nil {
		return err
	}
	return st.EnterRecoveryAttach(ctx, runID, turnID, token)
}

func (m *multiProjectDispatchStore) RecordRecoveryAttachedEffect(ctx context.Context, runID, turnID string, token RecoveryAttachToken, eventID string, payload AttachedEffectPayload) (bool, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return false, err
	}
	return st.RecordRecoveryAttachedEffect(ctx, runID, turnID, token, eventID, payload)
}

func (m *multiProjectDispatchStore) CommitAttachedTerminalAndSettleIntent(ctx context.Context, runID, turnID string, expectedRev int64, token RecoveryAttachToken,
	proof TerminalEvidence, intentOwnerRunID, intentKey string, intentGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitAttachedTerminalAndSettleIntent(ctx, runID, turnID, expectedRev, token, proof, intentOwnerRunID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) CommitPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64,
	intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitPreSendCancellationAndClearIntent(ctx, runID, turnID, expectedRev, intentOwnerRunID, intentKey, intentGen, stopGen, source)
}

func (m *multiProjectDispatchStore) CommitRecoveryPreSendCancellationAndClearIntent(ctx context.Context, runID, turnID string, expectedRev int64, leaseOwner string,
	intentOwnerRunID, intentKey string, intentGen, stopGen int64, source PreSendStopSource) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitRecoveryPreSendCancellationAndClearIntent(ctx, runID, turnID, expectedRev, leaseOwner, intentOwnerRunID, intentKey, intentGen, stopGen, source)
}

func (m *multiProjectDispatchStore) ResolveUncertain(ctx context.Context, runID, turnID string, expectedRev int64,
	resolutionID string, action ResolveAction, evidence OperatorEvidence) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.ResolveUncertain(ctx, runID, turnID, expectedRev, resolutionID, action, evidence)
}

func (m *multiProjectDispatchStore) RetryAsNew(ctx context.Context, runID, oldTurnID string, expectedRev int64,
	resolutionID, newTurnID string, expectedIntentGen int64, expectedEnvelopeHash string) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.RetryAsNew(ctx, runID, oldTurnID, expectedRev, resolutionID, newTurnID, expectedIntentGen, expectedEnvelopeHash)
}

func (m *multiProjectDispatchStore) RecordEffectDone(ctx context.Context, runID, turnID, effectKind string, payload []byte, payloadHash string) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.RecordEffectDone(ctx, runID, turnID, effectKind, payload, payloadHash)
}

func (m *multiProjectDispatchStore) CreateReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, intent DurableIntent) (ReleaseManifestItem, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return ReleaseManifestItem{}, err
	}
	return st.CreateReleaseManifestItem(ctx, runID, turnID, dependentRunID, intent)
}

func (m *multiProjectDispatchStore) CommitReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev int64, child DispatchRecord, env DispatchEnvelope) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	if child.ProjectID == "" {
		if pid, ok := m.runProject[runID]; ok {
			child.ProjectID = pid
		}
	}
	return st.CommitReleaseManifestItem(ctx, runID, turnID, dependentRunID, expectedEffectRev, child, env)
}

func (m *multiProjectDispatchStore) SuppressReleaseManifestItem(ctx context.Context, runID, turnID, dependentRunID string, expectedEffectRev, stopGen int64) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.SuppressReleaseManifestItem(ctx, runID, turnID, dependentRunID, expectedEffectRev, stopGen)
}

func (m *multiProjectDispatchStore) OpenRepair(ctx context.Context, runID, reason string, rawBlob []byte, rawHash string) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		// Repair may open before any turn: require project via side channel not available —
		// fall through error.
		return 0, err
	}
	return st.OpenRepair(ctx, runID, reason, rawBlob, rawHash)
}

func (m *multiProjectDispatchStore) GetOpenRepair(ctx context.Context, runID string) (RepairRecord, bool, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return RepairRecord{}, false, nil
	}
	return st.GetOpenRepair(ctx, runID)
}

func (m *multiProjectDispatchStore) BeginRepairResolution(ctx context.Context, runID string, expectedRepairRev int64,
	resolutionID string, action RepairAction) (int64, []byte, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, nil, err
	}
	return st.BeginRepairResolution(ctx, runID, expectedRepairRev, resolutionID, action)
}

func (m *multiProjectDispatchStore) CommitRepairResolution(ctx context.Context, runID string, attemptRev int64,
	resolutionID string, outcome RepairOutcome, detail string) (int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, err
	}
	return st.CommitRepairResolution(ctx, runID, attemptRev, resolutionID, outcome, detail)
}

func (m *multiProjectDispatchStore) GetRunProtocolVersion(ctx context.Context, runID string) (int, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return 0, nil
	}
	return st.GetRunProtocolVersion(ctx, runID)
}

func (m *multiProjectDispatchStore) Get(ctx context.Context, runID, turnID string) (DispatchRecord, int64, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return DispatchRecord{}, 0, err
	}
	return st.Get(ctx, runID, turnID)
}

func (m *multiProjectDispatchStore) GetEnvelope(ctx context.Context, runID, turnID string) (DispatchEnvelope, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return DispatchEnvelope{}, err
	}
	return st.GetEnvelope(ctx, runID, turnID)
}

func (m *multiProjectDispatchStore) ListRecoverable(ctx context.Context, runID string) ([]DispatchRecord, error) {
	if runID != "" {
		st, err := m.forRun(runID)
		if err != nil {
			return nil, nil
		}
		return st.ListRecoverable(ctx, runID)
	}
	// All projects
	m.mu.Lock()
	ids := make([]string, 0, len(m.byProject))
	for id := range m.byProject {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var out []DispatchRecord
	for _, id := range ids {
		st, err := m.forProject(id)
		if err != nil {
			continue
		}
		list, err := st.ListRecoverable(ctx, "")
		if err != nil {
			continue
		}
		out = append(out, list...)
	}
	return out, nil
}

func (m *multiProjectDispatchStore) FindActiveByOuterIntent(ctx context.Context, runID, intentKey string, intentGen int64) (DispatchRecord, bool, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return DispatchRecord{}, false, nil
	}
	return st.FindActiveByOuterIntent(ctx, runID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) ListAttention(ctx context.Context) ([]AttentionItem, error) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.byProject))
	for id := range m.byProject {
		ids = append(ids, id)
	}
	// Also discover dirs not yet open.
	entries, _ := os.ReadDir(m.root)
	m.mu.Unlock()
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, e := range entries {
		if e.IsDir() && !seen[e.Name()] {
			ids = append(ids, e.Name())
		}
	}
	var out []AttentionItem
	for _, id := range ids {
		st, err := m.forProject(id)
		if err != nil {
			continue
		}
		list, err := st.ListAttention(ctx)
		if err != nil {
			continue
		}
		out = append(out, list...)
	}
	return out, nil
}

func (m *multiProjectDispatchStore) ListAudit(ctx context.Context, runID, turnID string) ([]AuditEntry, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return nil, nil
	}
	return st.ListAudit(ctx, runID, turnID)
}

func (m *multiProjectDispatchStore) ListEffects(ctx context.Context, runID, turnID string) ([]EffectDone, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return nil, nil
	}
	return st.ListEffects(ctx, runID, turnID)
}

func (m *multiProjectDispatchStore) GetResolutionResult(ctx context.Context, resolutionID string) (ResolutionResult, bool, error) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.byProject))
	for id := range m.byProject {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		st, err := m.forProject(id)
		if err != nil {
			continue
		}
		r, ok, err := st.GetResolutionResult(ctx, resolutionID)
		if err != nil {
			continue
		}
		if ok {
			return r, true, nil
		}
	}
	return ResolutionResult{}, false, nil
}

func (m *multiProjectDispatchStore) IsIntentCleared(ctx context.Context, ownerRunID, intentKey string, intentGen int64) (bool, error) {
	st, err := m.forRun(ownerRunID)
	if err != nil {
		return false, nil
	}
	return st.IsIntentCleared(ctx, ownerRunID, intentKey, intentGen)
}

func (m *multiProjectDispatchStore) HasNonTerminal(ctx context.Context, runID string) (bool, error) {
	st, err := m.forRun(runID)
	if err != nil {
		return false, nil
	}
	return st.HasNonTerminal(ctx, runID)
}

var _ DispatchStore = (*multiProjectDispatchStore)(nil)

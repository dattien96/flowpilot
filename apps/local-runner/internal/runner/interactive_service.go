package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// InteractiveService implements the Phase 2 (04-02) interactive + admin APIs: the
// catalog, an in-memory provider session/event/approval/question store, the
// per-run SSE event stream with an afterSeq reconnect cursor, the turn lifecycle
// driven through a ProviderRuntimeAdapter, and the approval/question bridges
// (idempotent + first-write-wins, with expiry). In P2 the adapter is the fake
// adapter; P3 swaps in the real Codex adapter behind the same contract.
//
// State is in-memory: it survives client reconnect (which is what P2 needs);
// runner-restart resume is Phase 3+ via Codex thread/resume.
type InteractiveService struct {
	// catalog serves projects/workflows/steps — the fake interactiveCatalog when
	// Supabase is not configured, SupabaseCatalogStore when it is (04-08 A1).
	catalog CatalogStore
	// skillsCatalog serves the local provider-skill list (not from Supabase).
	skillsCatalog *interactiveCatalog
	registry      *ProviderRegistry

	// policy decides auto-approve/auto-deny/ask for YOLO=false approvals (04-04).
	// Default is ask-everything; Admin Web (03) configures the lists.
	policy *ApprovalPolicyEngine
	// finalizer runs the post-turn hook on turn_completed (04-04), idempotent.
	finalizer *finalizer
	// workflowStore/orchestrator are the Phase 8 cut-over bridge: the interactive
	// path seeds and progresses workflow steps through the shared orchestration
	// boundary instead of remaining fully ad hoc/in-memory.
	workflowStore WorkflowStore
	orchestrator  *WorkflowOrchestrator
	runner        *Runner

	mu        sync.Mutex
	runs      map[string]*interactiveRun
	approvals map[string]*approvalRecord
	questions map[string]*questionRecord

	idCounter atomic.Int64

	// activeAccountID is the currently active provider account. resume/turns
	// validate the session's account against it (account-scope, 04-02/04-06). One
	// account in P2; tests flip it to simulate a mismatch.
	activeAccountID string

	approvalTTL time.Duration
	questionTTL time.Duration

	// maxTurnAttempts bounds send-with-retry: a turn whose adapter call fails with a
	// recoverable error (e.g. the shared app-server stream died mid-turn) is re-sent
	// up to this many times before failing the turn (04-05 send-with-retry). A user
	// interrupt or an approval/question expiry is NOT retried.
	maxTurnAttempts int
}

type interactiveRun struct {
	id                string
	projectID         string
	workflowID        string
	providerKey       ProviderKey
	providerSessionID string
	// realProviderSessionID is the provider-owned durable resume handle when it differs
	// from FlowPilot's synthetic per-run session id.
	realProviderSessionID string
	providerAccountID string
	workspaceCwd      string
	modelName         string
	yolo              bool
	// reasoningEffort is the desktop-selected effort level passed per-turn (T-4).
	reasoningEffort string
	// runKind is "chat" for normal-chat runs, "" / "workflow" for workflow runs (T-7).
	runKind string

	status        RunStatus
	createdAt     string
	updatedAt     string
	lastPrompt    string
	lastMessage   string
	sourceMachineID string
	sourceRunID     string
	restoredFrom    string
	syncStatus      string
	syncUpdatedAt   string
	seq           int64
	lastEventType ProviderEventType
	events        []ProviderEvent

	turnInFlight  bool
	currentTurnID string
	turnCancel    context.CancelFunc

	pendingApprovalID string
	pendingQuestionID string

	subs    map[int64]chan ProviderEvent
	nextSub int64

	idempotency map[string]string // Idempotency-Key -> turnId
	resumedFromDisk bool
}

type approvalRecord struct {
	id       string
	runID    string
	details  ApprovalDetails
	status   string // pending | resolved | expired
	decision string
	// policy records why an auto-decision was made (empty for human decisions):
	// "yolo_gating_disabled" | "policy_allowlist" | "policy_denylist".
	policy  string
	resolve chan string
}

type questionRecord struct {
	id          string
	runID       string
	prompt      string
	options     []QuestionOption
	multiSelect bool
	status      string // pending | resolved | expired
	choice      []string
	resolve     chan []string
}

func approvalStateFromRecord(rs *interactiveRun, rec *approvalRecord, expiresAt string) ProviderApprovalState {
	state := ProviderApprovalState{
		ApprovalID: rec.id,
		RunID:      rec.runID,
		Status:     rec.status,
		Decision:   rec.decision,
		Policy:     rec.policy,
		ExpiresAt:  expiresAt,
	}
	if rs != nil {
		state.ProviderKey = rs.providerKey
		state.ProviderTurnID = rs.currentTurnID
	}
	if rec.details.Command != "" {
		state.Command = rec.details.Command
	}
	if rec.details.Cwd != "" {
		state.Cwd = rec.details.Cwd
	}
	if rec.details.Reason != "" {
		state.Reason = rec.details.Reason
	}
	return state
}

func questionStateFromRecord(rec *questionRecord, providerTurnID, expiresAt string) ProviderQuestionState {
	return ProviderQuestionState{
		QuestionID:     rec.id,
		RunID:          rec.runID,
		ProviderTurnID: providerTurnID,
		Prompt:         rec.prompt,
		Options:        rec.options,
		MultiSelect:    rec.multiSelect,
		Status:         rec.status,
		Choice:         rec.choice,
		ExpiresAt:      expiresAt,
	}
}

// NewInteractiveService builds the Phase 2 service with the default registry
// (Codex fake-backed; Claude/Gemini disabled placeholders) and fake catalog.
func NewInteractiveService() *InteractiveService {
	return NewInteractiveServiceWithRegistry(DefaultProviderRegistry())
}

// NewInteractiveServiceWithRegistry builds the service with a caller-supplied
// provider registry — e.g. ProviderRegistryFor(runner), which backs Codex with the
// live app-server adapter when FLOWPILOT_CODEX_APPSERVER is set (04-03 registry
// swap). All other state matches NewInteractiveService.
func NewInteractiveServiceWithRegistry(registry *ProviderRegistry) *InteractiveService {
	fake := newInteractiveCatalog()
	return newInteractiveService(registry, fake, nil)
}

// NewInteractiveServiceWith builds the service with a caller-supplied registry +
// catalog store (e.g. CatalogStoreFor(runner), which returns SupabaseCatalogStore
// when Supabase is configured). The local skill list stays on the fake catalog.
func NewInteractiveServiceWith(registry *ProviderRegistry, catalog CatalogStore) *InteractiveService {
	if catalog == nil {
		catalog = newInteractiveCatalog()
	}
	return newInteractiveService(registry, catalog, nil)
}

// NewInteractiveServiceWithStore builds the service with a caller-supplied
// registry, catalog store, AND workflow store. Pass a non-nil store to override
// the default fakeWorkflowStore — e.g. localFileSessionStore for history
// persistence across restarts (BUG-080). A nil store falls back to the default.
func NewInteractiveServiceWithStore(registry *ProviderRegistry, catalog CatalogStore, store WorkflowStore) *InteractiveService {
	if catalog == nil {
		catalog = newInteractiveCatalog()
	}
	return newInteractiveService(registry, catalog, store)
}

func newInteractiveService(registry *ProviderRegistry, catalog CatalogStore, workflowStore WorkflowStore) *InteractiveService {
	if workflowStore == nil {
		workflowStore = newFakeWorkflowStore()
	}
	// Reclaim Codex image-attachment temp dirs orphaned by a prior hard crash/kill
	// (Task-052); the per-turn deferred cleanup cannot run in that case. Best-effort.
	sweepCodexImageAttachments(time.Hour, time.Now())
	return &InteractiveService{
		catalog:         catalog,
		skillsCatalog:   newInteractiveCatalog(),
		registry:        registry,
		policy:          DefaultApprovalPolicyEngine(),
		finalizer:       newFinalizer(),
		workflowStore:   workflowStore,
		orchestrator:    NewWorkflowOrchestrator(workflowStore),
		runs:            map[string]*interactiveRun{},
		approvals:       map[string]*approvalRecord{},
		questions:       map[string]*questionRecord{},
		activeAccountID: "default",
		approvalTTL:     10 * time.Minute,
		questionTTL:     10 * time.Minute,
		maxTurnAttempts: 3,
	}
}

type workflowRunSeeder interface {
	seed(runID string, steps []RuntimeWorkflowStep)
}

func (s *InteractiveService) nextID(prefix string) string {
	return prefix + "-" + strconv.FormatInt(s.idCounter.Add(1), 10)
}

func (s *InteractiveService) persistenceStore() InteractiveStateStore {
	store, _ := s.workflowStore.(InteractiveStateStore)
	return store
}

func (s *InteractiveService) AttachRunner(r *Runner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runner = r
}

func (s *InteractiveService) persistProviderSession(session ProviderSessionState) error {
	store := s.persistenceStore()
	if store == nil {
		return nil
	}
	return store.UpsertProviderSession(context.Background(), session)
}

// sessionStateOf snapshots the display fields of rs into a ProviderSessionState.
// Caller must hold s.mu or guarantee rs is not concurrently modified.
func sessionStateOf(rs *interactiveRun) ProviderSessionState {
	providerSessionID := rs.providerSessionID
	if rs.realProviderSessionID != "" {
		providerSessionID = rs.realProviderSessionID
	}
	return ProviderSessionState{
		RunID:             rs.id,
		ProjectID:         rs.projectID,
		WorkflowID:        rs.workflowID,
		ProviderSessionID: providerSessionID,
		ProviderKey:       rs.providerKey,
		ProviderAccountID: rs.providerAccountID,
		WorkingDirectory:  rs.workspaceCwd,
		Status:            rs.status,
		LastPrompt:        rs.lastPrompt,
		LastMessage:       rs.lastMessage,
		StartedAt:         rs.createdAt,
		UpdatedAt:         rs.updatedAt,
		RunKind:           rs.runKind,
		SourceMachineID:   rs.sourceMachineID,
		SourceRunID:       rs.sourceRunID,
		RestoredFrom:      rs.restoredFrom,
		SyncStatus:        rs.syncStatus,
		SyncUpdatedAt:     rs.syncUpdatedAt,
	}
}

func (s *InteractiveService) persistApproval(record ProviderApprovalState) error {
	store := s.persistenceStore()
	if store == nil {
		return nil
	}
	return store.UpsertApproval(context.Background(), record)
}

func (s *InteractiveService) persistQuestion(record ProviderQuestionState) error {
	store := s.persistenceStore()
	if store == nil {
		return nil
	}
	return store.UpsertQuestion(context.Background(), record)
}

func (s *InteractiveService) persistEvent(event ProviderEvent) error {
	store := s.persistenceStore()
	if store == nil || event.Type == EventMessageDelta {
		return nil
	}
	return store.AppendEvent(context.Background(), event)
}

// ---- errors (stable codes per 04-02 error envelope) ------------------------

var (
	errApprovalExpired = errors.New("approval expired")
	errQuestionExpired = errors.New("question expired")
)

type apiErr struct {
	status int
	code   string
	msg    string
}

func (e *apiErr) Error() string { return e.msg }

func newAPIErr(status int, code, msg string) *apiErr {
	return &apiErr{status: status, code: code, msg: msg}
}

// ============================================================================
// Event emission + SSE hub
// ============================================================================

// emitLocked stamps correlation + a monotonic per-run seq, persists, broadcasts,
// and applies the status transition. Caller holds s.mu.
func (s *InteractiveService) emitLocked(rs *interactiveRun, ev ProviderEvent) ProviderEvent {
	rs.seq++
	ev.Seq = rs.seq
	ev.ID = s.nextID("evt")
	ev.WorkflowRunID = rs.id
	ev.ProviderSessionID = rs.providerSessionID
	ev.ProviderKey = rs.providerKey
	if ev.ProviderTurnID == "" {
		ev.ProviderTurnID = rs.currentTurnID
	}
	ev.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)

	rs.events = append(rs.events, ev)
	rs.lastEventType = ev.Type
	rs.updatedAt = ev.OccurredAt
	switch ev.Type {
	case EventMessageCompleted:
		if ev.Text != "" {
			rs.lastMessage = truncateDisplayField(ev.Text, 100)
		}
	case EventTurnCompleted:
		if ev.FinalMessage != "" {
			rs.lastMessage = truncateDisplayField(ev.FinalMessage, 100)
		}
	case EventTurnFailed:
		if ev.Error != "" {
			rs.lastMessage = truncateDisplayField(ev.Error, 100)
		}
	}
	_ = s.persistEvent(ev)

	switch ev.Type {
	case EventPermissionRequired:
		rs.status = RunStatusWaitingApproval
	case EventUserQuestionRequired:
		rs.status = RunStatusWaitingQuestion
	case EventTurnCompleted:
		rs.status = RunStatusCompleted
	case EventTurnFailed:
		rs.status = RunStatusFailed
	default:
		rs.status = RunStatusRunning
	}

	for _, ch := range rs.subs {
		select {
		case ch <- ev:
		default: // subscriber slow/full — it reconnects via afterSeq, no gap
		}
	}
	return ev
}

func (s *InteractiveService) subscribe(runID string, after int64) (int64, chan ProviderEvent, []ProviderEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return 0, nil, nil, false
	}
	snapshot := make([]ProviderEvent, 0, len(rs.events))
	for _, e := range rs.events {
		if e.Seq > after {
			snapshot = append(snapshot, e)
		}
	}
	id := rs.nextSub
	rs.nextSub++
	ch := make(chan ProviderEvent, 256)
	rs.subs[id] = ch
	return id, ch, snapshot, true
}

func (s *InteractiveService) unsubscribe(runID string, id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[runID]; rs != nil {
		if ch, ok := rs.subs[id]; ok {
			delete(rs.subs, id)
			close(ch)
		}
	}
}

// ============================================================================
// Turn lifecycle + bridge
// ============================================================================

type turnBridge struct {
	svc    *InteractiveService
	rs     *interactiveRun
	ctx    context.Context
	turnID string
	// yolo is the effective YOLO posture for THIS turn (BUG-063 per-turn override). The
	// approval bridge must use it — not rs.yolo — so a YOLO change between chat prompts
	// drives the runner's auto-approve the same way it drives the adapter's sandbox/mode.
	yolo bool
}

func (b *turnBridge) Emit(ev ProviderEvent) {
	b.svc.mu.Lock()
	defer b.svc.mu.Unlock()
	if ev.ProviderTurnID == "" {
		ev.ProviderTurnID = b.turnID
	}
	b.svc.emitLocked(b.rs, ev)
}

func (b *turnBridge) RequestApproval(details ApprovalDetails) (string, error) {
	s := b.svc

	// YOLO=true (RunnerAutoApprove): the runtime runs in "never" approval mode and
	// should not ask; if a request still arrives, auto-approve and audit as
	// gating-disabled (04-04). Reply is the returned decision (adapter sends it).
	if resolveYoloPosture(b.yolo).RunnerAutoApprove {
		s.recordAutoApproval(b.rs, details, "approve", "yolo_gating_disabled")
		return "approve", nil
	}

	// YOLO=false: the policy engine may auto-decide known-safe/known-dangerous
	// operations; everything else falls through to ask-the-human. Every
	// auto-decision is recorded AND returned so the adapter replies to the inbound
	// request (the provider never hangs).
	switch s.policy.Decide(details) {
	case PolicyAutoApprove:
		s.recordAutoApproval(b.rs, details, "approve", "policy_allowlist")
		return "approve", nil
	case PolicyAutoDeny:
		s.recordAutoApproval(b.rs, details, "deny", "policy_denylist")
		return "deny", nil
	}

	s.mu.Lock()
	rec := &approvalRecord{
		id:      s.nextID("appr"),
		runID:   b.rs.id,
		details: details,
		status:  "pending",
		resolve: make(chan string, 1),
	}
	s.approvals[rec.id] = rec
	b.rs.pendingApprovalID = rec.id
	s.emitLocked(b.rs, ProviderEvent{
		Type:           EventPermissionRequired,
		ProviderTurnID: b.turnID,
		ApprovalID:     rec.id,
		Provider:       b.rs.providerKey,
		Details:        &details,
	})
	s.mu.Unlock()

	timer := time.NewTimer(s.approvalTTL)
	defer timer.Stop()
	select {
	case d := <-rec.resolve:
		return d, nil
	case <-timer.C:
		s.expireApproval(rec.id)
		return "", errApprovalExpired
	case <-b.ctx.Done():
		s.clearPendingApproval(rec.id)
		return "", b.ctx.Err()
	}
}

func (b *turnBridge) AskQuestion(prompt string, options []QuestionOption, multiSelect bool) ([]string, error) {
	s := b.svc
	s.mu.Lock()
	rec := &questionRecord{
		id:          s.nextID("q"),
		runID:       b.rs.id,
		prompt:      prompt,
		options:     options,
		multiSelect: multiSelect,
		status:      "pending",
		resolve:     make(chan []string, 1),
	}
	s.questions[rec.id] = rec
	b.rs.pendingQuestionID = rec.id
	s.emitLocked(b.rs, ProviderEvent{
		Type:           EventUserQuestionRequired,
		ProviderTurnID: b.turnID,
		QuestionID:     rec.id,
		Prompt:         prompt,
		Options:        options,
		MultiSelect:    multiSelect,
	})
	s.mu.Unlock()

	timer := time.NewTimer(s.questionTTL)
	defer timer.Stop()
	select {
	case c := <-rec.resolve:
		return c, nil
	case <-timer.C:
		s.expireQuestion(rec.id)
		return nil, errQuestionExpired
	case <-b.ctx.Done():
		s.clearPendingQuestion(rec.id)
		return nil, b.ctx.Err()
	}
}

func (s *InteractiveService) expireApproval(id string) {
	var snapshot *ProviderApprovalState
	s.mu.Lock()
	if rec := s.approvals[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		if rs := s.runs[rec.runID]; rs != nil && rs.pendingApprovalID == id {
			rs.pendingApprovalID = ""
			state := approvalStateFromRecord(rs, rec, "")
			snapshot = &state
		}
	}
	s.mu.Unlock()
	if snapshot != nil {
		_ = s.persistApproval(*snapshot)
	}
}

func (s *InteractiveService) clearPendingApproval(id string) {
	var snapshot *ProviderApprovalState
	s.mu.Lock()
	if rec := s.approvals[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		if rs := s.runs[rec.runID]; rs != nil {
			state := approvalStateFromRecord(rs, rec, "")
			snapshot = &state
		}
	}
	if rs := s.runs[s.approvalRunID(id)]; rs != nil && rs.pendingApprovalID == id {
		rs.pendingApprovalID = ""
	}
	s.mu.Unlock()
	if snapshot != nil {
		_ = s.persistApproval(*snapshot)
	}
}

// recordAutoApproval persists an auto-decision (YOLO gating-disabled or a policy
// allow/deny) for audit. It does NOT emit permission_required (no card shown) and
// does NOT block — the decision is replied to the inbound request by the caller.
func (s *InteractiveService) recordAutoApproval(rs *interactiveRun, details ApprovalDetails, decision, policy string) {
	s.mu.Lock()
	rec := &approvalRecord{
		id:       s.nextID("appr"),
		runID:    rs.id,
		details:  details,
		status:   "resolved",
		decision: decision,
		policy:   policy,
	}
	s.approvals[rec.id] = rec
	state := approvalStateFromRecord(rs, rec, "")
	s.mu.Unlock()
	_ = s.persistApproval(state)
}

func (s *InteractiveService) approvalRunID(id string) string {
	if rec := s.approvals[id]; rec != nil {
		return rec.runID
	}
	return ""
}

func (s *InteractiveService) expireQuestion(id string) {
	var snapshot *ProviderQuestionState
	s.mu.Lock()
	if rec := s.questions[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		if rs := s.runs[rec.runID]; rs != nil && rs.pendingQuestionID == id {
			rs.pendingQuestionID = ""
		}
		state := questionStateFromRecord(rec, "", "")
		snapshot = &state
	}
	s.mu.Unlock()
	if snapshot != nil {
		_ = s.persistQuestion(*snapshot)
	}
}

func (s *InteractiveService) clearPendingQuestion(id string) {
	var snapshot *ProviderQuestionState
	s.mu.Lock()
	if rec := s.questions[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		state := questionStateFromRecord(rec, "", "")
		snapshot = &state
	}
	if rec := s.questions[id]; rec != nil {
		if rs := s.runs[rec.runID]; rs != nil && rs.pendingQuestionID == id {
			rs.pendingQuestionID = ""
		}
	}
	s.mu.Unlock()
	if snapshot != nil {
		_ = s.persistQuestion(*snapshot)
	}
}

func (s *InteractiveService) runTurn(ctx context.Context, rs *interactiveRun, adapter ProviderRuntimeAdapter, in TurnInput, scenario, turnID string) {
	// Turn-level model/reasoning/YOLO override the run-level defaults when supplied
	// (BUG-063). Chat mode resends these every turn so they can change between prompts;
	// the providers re-apply them per turn (Codex thread/start per turn, Claude spawn-per-
	// turn). A nil pointer means "not supplied" and keeps the run-level default.
	effort := rs.reasoningEffort
	if in.ReasoningEffort != "" {
		effort = in.ReasoningEffort
	}
	model := rs.modelName
	if in.Model != nil {
		model = *in.Model
	}
	yolo := rs.yolo
	if in.YoloMode != nil {
		yolo = *in.YoloMode
	}
	req := TurnRequest{
		RunID:             rs.id,
		StepID:            in.StepID,
		ProviderSessionID: rs.providerSessionID,
		ProviderTurnID:    turnID,
		Prompt:            in.Prompt,
		ModelName:         model,
		SelectedSkills:    in.SelectedSkills,
		YoloMode:          yolo,
		ReasoningEffort:   effort,
		Cwd:               rs.workspaceCwd,
		Scenario:          scenario,
		Attachments:       in.Attachments,
	}
	bridge := &turnBridge{svc: s, rs: rs, ctx: ctx, turnID: turnID, yolo: yolo}
	err := s.sendTurnWithRetry(ctx, adapter, req, bridge)
	if err == nil {
		// The provider turn owns interactive UX/events; once it returns cleanly we
		// advance the shared workflow planner so the live run path no longer bypasses
		// WorkflowStore/WorkflowOrchestrator entirely. A2 will replace the fake
		// backing store and make this durable/auditable.
		_, _ = s.orchestrator.Progress(ctx, rs.id, yolo)
	}

	completed, fin := s.finishTurn(rs, turnID, err)

	// Persist settled state (status, lastMessage, updatedAt) for history survival
	// across restarts (BUG-080 F-3). Take a snapshot under lock; persist outside.
	s.mu.Lock()
	s.refreshResumeHandleLocked(rs, adapter)
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	_ = s.persistProviderSession(snap)

	// Finalizer hook runs OUTSIDE s.mu and only on a clean completion. A finalize
	// failure is recorded as retryable and must not erase the completed turn (04-04).
	if completed {
		_ = s.finalizer.Finalize(fin)
	}
}

func (s *InteractiveService) markStepRunning(ctx context.Context, runID, stepID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.workflowStore.ApplyStepTransition(ctx, runID, WorkflowStepTransition{
		StepID: stepID,
		Patch: WorkflowStepPatch{
			Status:     StepStatusRunning,
			StartedAt:  strptr(now),
			FinishedAt: strptr(""),
		},
		Logs: []WorkflowLog{{LogInfo, "Started interactive provider turn."}},
	}); err != nil {
		return err
	}
	return s.workflowStore.SetRunStatus(ctx, runID, RunStatusEngineRunning, "")
}

// sendTurnWithRetry runs the adapter turn, re-sending on a recoverable error up to
// maxTurnAttempts (04-05 send-with-retry). A user interrupt (ctx cancel) and an
// approval/question expiry are terminal — not retried. Between attempts it emits a
// note so the timeline shows the recovery.
func (s *InteractiveService) sendTurnWithRetry(ctx context.Context, adapter ProviderRuntimeAdapter, req TurnRequest, bridge TurnBridge) error {
	attempts := s.maxTurnAttempts
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		err = adapter.SendTurn(ctx, req, bridge)
		if err == nil || !isRecoverableSendError(err) || attempt == attempts {
			return err
		}
		bridge.Emit(ProviderEvent{
			Type: EventMessageDelta,
			Text: fmt.Sprintf("\n[recovering: re-sending turn after a recoverable error (attempt %d/%d)]\n", attempt+1, attempts),
		})
	}
	return err
}

// isRecoverableSendError reports whether a failed SendTurn should be re-sent. A user
// interrupt and an approval/question expiry are intentional terminal outcomes.
func isRecoverableSendError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, errApprovalExpired) || errors.Is(err, errQuestionExpired) {
		return false
	}
	if isProviderUsageLimitError(err) {
		return false
	}
	return true
}

func isProviderUsageLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "usage limit reached") ||
		strings.Contains(message, "extra usage unavailable") ||
		strings.Contains(message, "out of credits") ||
		strings.Contains(message, "out_of_credits") ||
		strings.Contains(message, "quota reset") ||
		strings.Contains(message, "rate limit")
}

// finishTurn does the locked post-turn bookkeeping: clears in-flight state, emits
// the terminal event when the adapter did not, and maps the error to a status. It
// returns whether the turn completed cleanly (so the caller runs the finalizer) and
// the finalize input gathered from the run's events.
func (s *InteractiveService) finishTurn(rs *interactiveRun, turnID string, err error) (bool, finalizeInput) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs.turnInFlight = false
	rs.turnCancel = nil
	rs.currentTurnID = ""

	switch {
	case err == nil:
		if rs.lastEventType != EventTurnCompleted && rs.lastEventType != EventTurnFailed {
			s.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: ""})
		}
		return rs.status == RunStatusCompleted, s.finalizeInputLocked(rs, turnID)
	case errors.Is(err, context.Canceled):
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: "interrupted by user", Recoverable: true})
		rs.status = RunStatusCancelled // override the failed mapping for a clean interrupt
	case errors.Is(err, errApprovalExpired) || errors.Is(err, errQuestionExpired):
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: err.Error(), Recoverable: true})
	default:
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: err.Error(), Recoverable: false})
	}
	return false, finalizeInput{}
}

// finalizeInputLocked gathers the post-turn context (final message + changed files)
// from this turn's events. Caller holds s.mu.
func (s *InteractiveService) finalizeInputLocked(rs *interactiveRun, turnID string) finalizeInput {
	in := finalizeInput{RunID: rs.id, TurnID: turnID}
	for _, e := range rs.events {
		if e.ProviderTurnID != turnID {
			continue
		}
		switch e.Type {
		case EventTurnCompleted:
			if e.FinalMessage != "" {
				in.FinalMessage = e.FinalMessage
			}
		case EventMessageCompleted:
			if in.FinalMessage == "" {
				in.FinalMessage = e.Text
			}
		case EventFileChanged:
			if e.Path != "" {
				in.ChangedFiles = append(in.ChangedFiles, e.Path)
			}
		}
	}
	return in
}

// startTurn validates, enforces one-turn-per-session, applies idempotency, emits
// turn_started, and launches the adapter. Returns the turnId.
func (s *InteractiveService) startTurn(runID string, in TurnInput, scenario, idempotencyKey string) (string, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if rs.providerAccountID != s.activeAccountID {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "provider_account_changed", "active provider account changed since the run started")
	}
	if idempotencyKey != "" {
		if tid, ok := rs.idempotency[idempotencyKey]; ok {
			s.mu.Unlock()
			return tid, nil
		}
	}
	if rs.turnInFlight {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "turn_in_progress", "a turn is already in flight for this session")
	}
	if in.StepID == "" {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusBadRequest, "invalid_request", "stepId is required")
	}

	adapter, aerr := s.registry.Adapter(rs.providerKey)
	if aerr != nil {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusBadRequest, "provider_unavailable", aerr.Error())
	}
	if rs.resumedFromDisk && rs.providerKey == ProviderKeyCodex {
		home, ok := s.resolveAccountHome(rs.providerKey, s.activeAccountID)
		if !ok {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
		}
		var promptPrep func(TurnRequest) string
		if live, ok := adapter.(*codexAdapter); ok {
			promptPrep = live.promptPrep
		}
		adapter = newCodexResumeAdapter(home, promptPrep)
	}
	if rs.resumedFromDisk && rs.providerKey == ProviderKeyClaude {
		if live, ok := adapter.(*claudeAdapter); ok && rs.realProviderSessionID != "" {
			live.pool.setRealSession(rs.providerSessionID, rs.realProviderSessionID)
			live.pool.setRealSession(rs.realProviderSessionID, rs.realProviderSessionID)
		}
	}

	turnID := s.nextID("turn")
	rs.turnInFlight = true
	rs.currentTurnID = turnID
	rs.lastPrompt = truncateDisplayField(in.Prompt, 100)
	rs.updatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	ctx, cancel := context.WithCancel(context.Background())
	rs.turnCancel = cancel
	if idempotencyKey != "" {
		rs.idempotency[idempotencyKey] = turnID
	}
	if err := s.markStepRunning(ctx, runID, in.StepID); err != nil {
		rs.turnInFlight = false
		rs.currentTurnID = ""
		rs.turnCancel = nil
		if idempotencyKey != "" {
			delete(rs.idempotency, idempotencyKey)
		}
		s.mu.Unlock()
		cancel()
		return "", newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	s.emitLocked(rs, ProviderEvent{Type: EventTurnStarted, ProviderTurnID: turnID, WorkflowStepRunID: in.StepID, Prompt: in.Prompt})
	snap := sessionStateOf(rs) // capture under lock: lastPrompt + updatedAt now set
	s.mu.Unlock()
	_ = s.persistProviderSession(snap) // BUG-080 F-3: persist outside lock, best-effort

	go s.runTurn(ctx, rs, adapter, in, scenario, turnID)
	return turnID, nil
}

func (s *InteractiveService) refreshResumeHandleLocked(rs *interactiveRun, adapter ProviderRuntimeAdapter) {
	switch rs.providerKey {
	case ProviderKeyClaude:
		live, ok := adapter.(*claudeAdapter)
		if !ok {
			return
		}
		real := live.pool.realSession(rs.providerSessionID)
		if real == "" {
			real = live.pool.realSession(rs.realProviderSessionID)
		}
		if real != "" {
			rs.realProviderSessionID = real
		}
	case ProviderKeyCodex:
		if rs.realProviderSessionID != "" {
			return
		}
		home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
		if !ok {
			return
		}
		if rolloutID, found := DiscoverCodexRolloutSessionID(home, rs.workspaceCwd); found {
			rs.realProviderSessionID = rolloutID
		}
	}
}

// SubmitApprovalDecision is idempotent + first-write-wins.
func (s *InteractiveService) SubmitApprovalDecision(approvalID, decision string) *apiErr {
	s.mu.Lock()
	rec := s.approvals[approvalID]
	if rec == nil {
		s.mu.Unlock()
		return newAPIErr(http.StatusNotFound, "run_not_found", "approval not found")
	}
	if rec.status == "resolved" {
		s.mu.Unlock()
		return nil // first-write-wins: return the resolved outcome, not an error
	}
	if rec.status == "expired" {
		s.mu.Unlock()
		return newAPIErr(http.StatusConflict, "question_expired", "approval expired")
	}
	valid := false
	for _, d := range rec.details.Decisions {
		if d.Value == decision {
			valid = true
			break
		}
	}
	if !valid {
		s.mu.Unlock()
		return newAPIErr(http.StatusBadRequest, "invalid_decision", "decision not among offered options")
	}
	rec.status = "resolved"
	rec.decision = decision
	if rs := s.runs[rec.runID]; rs != nil && rs.pendingApprovalID == approvalID {
		rs.pendingApprovalID = ""
	}
	s.mu.Unlock()
	rec.resolve <- decision
	return nil
}

// AnswerQuestion is idempotent + first-write-wins. Free-text ("Other") is allowed,
// so any non-empty choice is accepted.
func (s *InteractiveService) AnswerQuestion(questionID string, choice []string) *apiErr {
	s.mu.Lock()
	rec := s.questions[questionID]
	if rec == nil {
		s.mu.Unlock()
		return newAPIErr(http.StatusNotFound, "run_not_found", "question not found")
	}
	if rec.status == "resolved" {
		s.mu.Unlock()
		return nil
	}
	if rec.status == "expired" {
		s.mu.Unlock()
		return newAPIErr(http.StatusConflict, "question_expired", "question expired")
	}
	if len(choice) == 0 {
		s.mu.Unlock()
		return newAPIErr(http.StatusBadRequest, "invalid_decision", "a choice is required")
	}
	rec.status = "resolved"
	rec.choice = choice
	if rs := s.runs[rec.runID]; rs != nil && rs.pendingQuestionID == questionID {
		rs.pendingQuestionID = ""
	}
	s.mu.Unlock()
	rec.resolve <- choice
	return nil
}

// AskWorkflowQuestion is the deterministic, workflow-driven question path (04-04
// Path 2 / 04-05 T-30): the runner/orchestration emits `user_question_required`
// directly at a defined step — no model `ask_user` tool call involved, so the card
// always appears. It reuses the exact same question record + `AnswerQuestion`
// resume machinery as the model-driven path (idempotent, first-write-wins, expiry),
// blocking the calling step until the user answers, the question expires, or ctx is
// cancelled (interrupt). Survives client reconnect (state in the runner).
func (s *InteractiveService) AskWorkflowQuestion(ctx context.Context, runID, prompt string, options []QuestionOption, multiSelect bool) ([]string, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	rec := &questionRecord{
		id:          s.nextID("q"),
		runID:       runID,
		prompt:      prompt,
		options:     options,
		multiSelect: multiSelect,
		status:      "pending",
		resolve:     make(chan []string, 1),
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	s.emitLocked(rs, ProviderEvent{
		Type:        EventUserQuestionRequired,
		QuestionID:  rec.id,
		Prompt:      prompt,
		Options:     options,
		MultiSelect: multiSelect,
	})
	s.mu.Unlock()

	timer := time.NewTimer(s.questionTTL)
	defer timer.Stop()
	select {
	case c := <-rec.resolve:
		return c, nil
	case <-timer.C:
		s.expireQuestion(rec.id)
		return nil, newAPIErr(http.StatusConflict, "question_expired", "question expired")
	case <-ctx.Done():
		s.clearPendingQuestion(rec.id)
		return nil, newAPIErr(http.StatusConflict, "interrupted", "question interrupted")
	}
}

// Interrupt cancels the in-flight turn for a run.
func (s *InteractiveService) Interrupt(runID string) *apiErr {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if rs.turnCancel != nil {
		rs.turnCancel()
	}
	return nil
}

// SetActiveAccount switches the active provider account (04-06). Because the single
// shared app-server is bound to one account, switching:
//   - interrupts every in-flight turn and marks it recoverable (re-sendable) — not a
//     silent auto-replay, consistent with the resume model;
//   - flips the active account, after which runs bound to the previous account fail
//     turn/resume with `409 provider_account_changed` (their threads are hidden
//     until that account is active again).
//
// This enforces the "one active account at a time / workspaces on different accounts
// cannot run concurrently (serialized)" constraint. The actual app-server teardown +
// recreate (ensureCodexAppServer) is the deferred live-Codex layer (06 Part D); here
// we own the interrupt + recoverable + scoping orchestration. Returns the number of
// in-flight turns interrupted.
func (s *InteractiveService) SetActiveAccount(accountID string) int {
	s.mu.Lock()
	var cancels []context.CancelFunc
	for _, rs := range s.runs {
		if rs.turnInFlight && rs.turnCancel != nil {
			cancels = append(cancels, rs.turnCancel)
		}
	}
	s.activeAccountID = accountID
	s.mu.Unlock()

	for _, cancel := range cancels {
		cancel() // ctx cancel → finishTurn emits a recoverable turn_failed + cancelled
	}
	return len(cancels)
}

// ActiveAccount returns the currently active provider account id.
func (s *InteractiveService) ActiveAccount() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeAccountID
}

// truncateDisplayField caps a string to max runes for storage in sessions.ndjson
// and the Drive sync index. The UI (runTitle) shows at most 68 chars; 100 gives
// it room while preventing multi-KB responses from bloating the index file.
func truncateDisplayField(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

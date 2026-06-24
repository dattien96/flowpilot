package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	// agentCatalog serves the loadable sub-agent definitions (CP-19 / Task-081):
	// .claude/agents + .codex/agents + provider homes, with built-in fallbacks.
	agentCatalog *AgentCatalog
	// agentOrchestrator tracks the in-memory agent run tree and provides wait:true
	// completion signaling for spawn_agent tool calls (CP-19 / Task-082).
	agentOrchestrator *AgentOrchestrator
	registry          *ProviderRegistry

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
	// lastCodexTurnSessionID tracks the newest rollout id discovered after a
	// Codex turn so per-turn rollout ids can be logged without replacing the
	// stable durable resume handle.
	lastCodexTurnSessionID string
	providerAccountID      string
	workspaceCwd           string
	stepID                 string
	modelName              string
	yolo                   bool
	// reasoningEffort is the desktop-selected effort level passed per-turn (T-4).
	reasoningEffort string
	// runKind is "chat" for normal-chat runs, "" / "workflow" for workflow runs (T-7).
	runKind string

	// Agent identity (CP-19 / Task-081). All fields are additive and zero-valued
	// for an ordinary parentless "main" run, so existing behavior is unchanged.
	// parentRunID is the spawning run's id ("" for the main/root run); agentName
	// is the AgentDefinition this run embodies; role is the agent's role (e.g.
	// "coder", "reviewer"); dependsOn lists run ids this agent waits on before it
	// may consume work; agentStatus is the orchestration status ("" treated as the
	// normal run lifecycle until the orchestrator sets it).
	parentRunID          string
	agentName            string
	role                 string
	dependsOn            []string
	agentStatus          string
	agentRound           int
	agentCap             int
	gateReason           string
	pendingTurnPrompt    string
	pendingRestartRunID  string
	pendingRestartPrompt string
	// uiInitiated marks a child run spawned from the desktop UI (not the AI spawn_agent
	// tool). UI spawns are invisible to the parent's provider conversation, so the parent
	// must be told about them out-of-band; tool spawns are already in provider history. (BUG-122)
	uiInitiated bool
	// waitForResult records the spawn's wait flag. A tool spawn with wait=true returns the
	// child result synchronously to the model (already in provider history). A tool spawn
	// with wait=false only acks "spawned" — its eventual result must be injected like a UI
	// spawn so the parent still learns the outcome (BUG-126).
	waitForResult bool
	// pendingAgentContext holds notes about UI-spawned children (and their results) that
	// have not yet been folded into this (parent) run's provider conversation. They are
	// prepended to the next provider turn's prompt and then cleared. Persisted to
	// sessions.ndjson so the parent still learns about them after a server restart. (BUG-122)
	pendingAgentContext []string

	status          RunStatus
	createdAt       string
	updatedAt       string
	lastPrompt      string
	lastMessage     string
	sourceMachineID string
	sourceRunID     string
	restoredFrom    string
	syncStatus      string
	syncUpdatedAt   string
	seq             int64
	lastEventType   ProviderEventType
	events          []ProviderEvent

	turnInFlight      bool
	repromptAttempts  int    // CP-35 P-5: number of flow-gate reprompts issued this turn
	turnStartGitHead  string // CP-35: git HEAD captured at turn start for committed-diff detection
	lastTurnStepID    string // CP-35: stepID of the most-recently started turn, used by gate reprompts
	currentTurnID     string
	turnCancel        context.CancelFunc

	pendingApprovalID string
	pendingQuestionID string

	subs    map[int64]chan ProviderEvent
	nextSub int64

	idempotency     map[string]string // Idempotency-Key -> turnId
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
	svc := &InteractiveService{
		catalog:           catalog,
		skillsCatalog:     newInteractiveCatalog(),
		agentCatalog:      newAgentCatalog(),
		agentOrchestrator: newAgentOrchestrator(),
		registry:          registry,
		policy:            DefaultApprovalPolicyEngine(),
		finalizer:         newFinalizer(),
		workflowStore:     workflowStore,
		orchestrator:      NewWorkflowOrchestrator(workflowStore),
		runs:              map[string]*interactiveRun{},
		approvals:         map[string]*approvalRecord{},
		questions:         map[string]*questionRecord{},
		activeAccountID:   "default",
		approvalTTL:       10 * time.Minute,
		questionTTL:       10 * time.Minute,
		maxTurnAttempts:   3,
	}
	// Seed the id counter above the highest persisted run id so a runner restart does NOT
	// reuse ids (run-1, run-2, …). Reuse made a fresh chat collide with a previous run of
	// the same id and inherit its persisted child agents — old sub-agents appeared in a
	// brand-new session's Agents panel. (BUG-117)
	svc.seedIDCounter()
	return svc
}

// seedIDCounter advances idCounter past the largest numeric suffix among persisted run ids
// (and their parent ids) so freshly minted ids never collide with runs from before a
// restart. Best-effort: no store / read error simply leaves the counter at zero.
func (s *InteractiveService) seedIDCounter() {
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return
	}
	sessions, err := indexReader.ListAllProviderSessions(context.Background())
	if err != nil {
		return
	}
	var max int64
	for _, sess := range sessions {
		if n := numericIDSuffix(sess.RunID); n > max {
			max = n
		}
		if n := numericIDSuffix(sess.ParentRunID); n > max {
			max = n
		}
	}
	for {
		cur := s.idCounter.Load()
		if cur >= max {
			return
		}
		if s.idCounter.CompareAndSwap(cur, max) {
			log.Printf("[runner] id counter seeded to %d from %d persisted runs", max, len(sessions))
			return
		}
	}
}

// numericIDSuffix returns the trailing integer of an id like "run-14" (→ 14), or 0 when
// the id has no numeric suffix.
func numericIDSuffix(id string) int64 {
	idx := strings.LastIndex(id, "-")
	if idx < 0 || idx == len(id)-1 {
		return 0
	}
	n, err := strconv.ParseInt(id[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func (s *InteractiveService) agentGraphSnapshot(parentRunID string) AgentGraphSnapshot {
	return s.agentOrchestrator.graphSnapshot(parentRunID)
}

func (s *InteractiveService) agentBusHistory(parentRunID string) []AgentBusMessage {
	return s.agentOrchestrator.graphSnapshot(parentRunID).BusMessages
}

func (s *InteractiveService) pauseAgentLoop(parentRunID, reason string) AgentGraphSnapshot {
	s.agentOrchestrator.pause(parentRunID, reason)
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	return snap
}
func (s *InteractiveService) resumeAgentLoop(parentRunID string) AgentGraphSnapshot {
	s.agentOrchestrator.resume(parentRunID)
	s.resumePendingLoopWork(parentRunID)
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	return snap
}
func (s *InteractiveService) stopAgentLoop(parentRunID string) AgentGraphSnapshot {
	s.agentOrchestrator.stop(parentRunID)
	s.mu.Lock()
	if parent := s.runs[parentRunID]; parent != nil {
		parent.pendingRestartRunID = ""
		parent.pendingRestartPrompt = ""
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		if child := s.runs[childID]; child != nil {
			child.pendingTurnPrompt = ""
			if child.turnInFlight && child.turnCancel != nil {
				child.turnCancel()
			}
		}
	}
	s.mu.Unlock()
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	return snap
}
func (s *InteractiveService) injectAgentFeedback(parentRunID, toRunID, message string) AgentGraphSnapshot {
	snap := s.agentOrchestrator.queueFeedback(parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: parentRunID, ToRunID: toRunID, Kind: "user-feedback", Message: message, Queued: true, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if len(snap.BusMessages) > 0 {
		s.emitAgentBus(parentRunID, snap.BusMessages[len(snap.BusMessages)-1])
	}
	snap = s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	return snap
}

func isAgentRole(rs *interactiveRun, role string) bool {
	if rs == nil {
		return false
	}
	role = strings.ToLower(role)
	return strings.Contains(strings.ToLower(rs.agentName), role) || strings.Contains(strings.ToLower(rs.role), role)
}

func (s *InteractiveService) dependenciesSatisfiedLocked(rs *interactiveRun) bool {
	if rs == nil || len(rs.dependsOn) == 0 {
		return true
	}
	for _, depID := range rs.dependsOn {
		dep := s.runs[depID]
		if dep == nil {
			return false
		}
		if dep.status != RunStatusCompleted {
			return false
		}
	}
	return true
}

func (s *InteractiveService) loopAllowsNextTurnLocked(parentRunID string) bool {
	state := s.agentOrchestrator.loop[parentRunID]
	return state.Status != "paused" && state.Status != "stopped"
}

func (s *InteractiveService) queueChildTurnLocked(rs *interactiveRun, prompt, status string) {
	if rs == nil {
		return
	}
	rs.pendingTurnPrompt = prompt
	if status != "" {
		rs.agentStatus = status
	}
}

func (s *InteractiveService) recordAgentBus(parentRunID string, msg AgentBusMessage) {
	_ = s.agentOrchestrator.addBus(parentRunID, msg)
	s.emitAgentBus(parentRunID, msg)
}

func (s *InteractiveService) takeQueuedFeedbackPrompt(parentRunID, runID, prompt string) (string, *AgentBusMessage) {
	queued := s.agentOrchestrator.nextQueuedFeedbackFor(parentRunID, runID)
	if queued == nil || queued.Message == "" {
		return prompt, nil
	}
	queued.Queued = false
	return strings.TrimSpace(prompt + "\n\n" + queued.Message), queued
}

func (s *InteractiveService) scheduleChildTurn(runID, stepID, prompt string) {
	if runID == "" || stepID == "" || prompt == "" {
		return
	}
	go func(runID, stepID, prompt string) {
		_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
	}(runID, stepID, prompt)
}

func (s *InteractiveService) releaseDependentAgents(parentRunID, completedRunID, handoffText string, occurredAt string) {
	type queuedTurn struct {
		runID   string
		stepID  string
		prompt  string
		busMsg  AgentBusMessage
		summary AgentRunSummary
	}
	queued := []queuedTurn{}
	s.mu.Lock()
	if !s.loopAllowsNextTurnLocked(parentRunID) {
		for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
			child := s.runs[childID]
			if child == nil || child.pendingTurnPrompt == "" || !s.dependenciesSatisfiedLocked(child) {
				continue
			}
			if handoffText != "" && !strings.Contains(child.pendingTurnPrompt, handoffText) {
				child.pendingTurnPrompt = strings.TrimSpace(child.pendingTurnPrompt + "\n\n" + handoffText)
			}
		}
		s.mu.Unlock()
		return
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil || child.pendingTurnPrompt == "" || !s.dependenciesSatisfiedLocked(child) {
			continue
		}
		prompt := child.pendingTurnPrompt
		child.pendingTurnPrompt = ""
		child.agentStatus = string(RunStatusRunning)
		if handoffText != "" {
			prompt = strings.TrimSpace(prompt + "\n\n" + handoffText)
		}
		queued = append(queued, queuedTurn{
			runID:  child.id,
			stepID: child.stepID,
			prompt: prompt,
			busMsg: AgentBusMessage{
				ID:          s.nextID("bus"),
				ParentRunID: parentRunID,
				FromRunID:   completedRunID,
				ToRunID:     child.id,
				Kind:        "handoff",
				Message:     handoffText,
				Queued:      false,
				OccurredAt:  occurredAt,
			},
			summary: AgentRunSummary{
				RunID:       child.id,
				AgentName:   child.agentName,
				Role:        child.role,
				Status:      child.status,
				ParentRunID: child.parentRunID,
				CreatedAt:   child.createdAt,
				DependsOn:   append([]string(nil), child.dependsOn...),
				AgentStatus: child.agentStatus,
				ProviderKey: string(child.providerKey),
				ModelName:   child.modelName,
			},
		})
	}
	s.mu.Unlock()
	for _, item := range queued {
		if item.busMsg.Message != "" {
			s.recordAgentBus(parentRunID, item.busMsg)
		}
		s.agentOrchestrator.upsertSummary(parentRunID, item.summary)
		prompt, queued := s.takeQueuedFeedbackPrompt(parentRunID, item.runID, item.prompt)
		if queued != nil {
			s.recordAgentBus(parentRunID, *queued)
		}
		s.scheduleChildTurn(item.runID, item.stepID, prompt)
	}
	if len(queued) > 0 {
		s.emitAgentGraph(parentRunID, s.agentGraphSnapshot(parentRunID))
	}
}

func (s *InteractiveService) resumePendingLoopWork(parentRunID string) {
	type pendingTurn struct {
		runID  string
		stepID string
		prompt string
	}
	var next *pendingTurn
	s.mu.Lock()
	if !s.loopAllowsNextTurnLocked(parentRunID) {
		s.mu.Unlock()
		return
	}
	if parent := s.runs[parentRunID]; parent != nil && parent.pendingRestartRunID != "" && parent.pendingRestartPrompt != "" {
		for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
			child := s.runs[childID]
			if child == nil || child.id != parent.pendingRestartRunID || child.turnInFlight {
				continue
			}
			next = &pendingTurn{runID: child.id, stepID: child.stepID, prompt: parent.pendingRestartPrompt}
			parent.pendingRestartRunID = ""
			parent.pendingRestartPrompt = ""
			break
		}
	}
	if next == nil {
		for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
			child := s.runs[childID]
			if child == nil || child.pendingTurnPrompt == "" || child.turnInFlight || !s.dependenciesSatisfiedLocked(child) {
				continue
			}
			next = &pendingTurn{runID: child.id, stepID: child.stepID, prompt: child.pendingTurnPrompt}
			child.pendingTurnPrompt = ""
			child.agentStatus = string(RunStatusRunning)
			break
		}
	}
	s.mu.Unlock()
	if next != nil {
		prompt, queued := s.takeQueuedFeedbackPrompt(parentRunID, next.runID, next.prompt)
		if queued != nil {
			s.recordAgentBus(parentRunID, *queued)
		}
		s.scheduleChildTurn(next.runID, next.stepID, prompt)
	}
}

func (s *InteractiveService) emitAgentGraph(parentRunID string, snap AgentGraphSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitAgentGraphLocked(parentRunID, snap)
}

func (s *InteractiveService) emitAgentGraphLocked(parentRunID string, snap AgentGraphSnapshot) {
	if rs := s.runs[parentRunID]; rs != nil {
		_ = s.emitLocked(rs, ProviderEvent{Type: EventAgentGraphUpdated, AgentGraphSnapshot: &snap})
	}
}

func (s *InteractiveService) emitAgentBus(parentRunID string, msg AgentBusMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitAgentBusLocked(parentRunID, msg)
}

func (s *InteractiveService) emitAgentBusLocked(parentRunID string, msg AgentBusMessage) {
	if rs := s.runs[parentRunID]; rs != nil {
		_ = s.emitLocked(rs, ProviderEvent{Type: EventAgentBusMessage, AgentBusMessage: &msg})
	}
}

// emitOnParentRun persists and broadcasts ev on the parent run's event stream (BUG-121).
// Safe to call without s.mu held; acquires it internally.
func (s *InteractiveService) emitOnParentRun(parentRunID string, ev ProviderEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[parentRunID]; rs != nil {
		_ = s.emitLocked(rs, ev)
	}
}

// maxPendingAgentNotes caps the parent's UI-spawn context buffer so a user who spawns
// many children without chatting cannot grow the next prompt without bound (BUG-122).
const maxPendingAgentNotes = 50

// composeAgentContextBlock renders the parent's pending UI-spawn notes as a single
// system-note prefix folded into the next provider turn's prompt (BUG-122).
func composeAgentContextBlock(notes []string) string {
	var b strings.Builder
	b.WriteString("[FlowPilot system note — sub-agents started in this session via the UI (not by you):\n")
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString("Use this when the user asks which sub-agents were started, their providers/models, or their results.]")
	return b.String()
}

// composeAgentSpawnPrompt builds the first-turn prompt for a spawned sub-agent
// in a provider-independent way so the same agent name yields an identical prompt
// shape on Claude and Codex (BUG-128). Structure:
//
//	<agent system prompt>      (when any; kept FIRST so built-in-agent prompt
//	                            detection in isAgentHistoryRun keeps matching)
//	<agent identity line>      (names the agent + role and links its definition
//	                            file so the model can open the full spec itself)
//	<user prompt>
//
// A nil agentDef (unknown agent) returns the user prompt unchanged.
func composeAgentSpawnPrompt(agentDef *AgentDefinition, userPrompt string) string {
	if agentDef == nil {
		return userPrompt
	}
	var b strings.Builder
	if sp := strings.TrimSpace(agentDef.SystemPrompt); sp != "" {
		b.WriteString(sp)
		b.WriteString("\n\n")
	}
	b.WriteString(composeAgentIdentityLine(agentDef))
	b.WriteString("\n\n")
	b.WriteString(userPrompt)
	return b.String()
}

// composeAgentIdentityLine renders the consistent, single-line agent reference
// shared by every provider: the agent name, role, and a link to its definition
// file (or a built-in marker when the agent has no on-disk path) (BUG-128).
func composeAgentIdentityLine(def *AgentDefinition) string {
	parts := make([]string, 0, 3)
	if name := strings.TrimSpace(def.Name); name != "" {
		parts = append(parts, "agent: "+name)
	}
	if role := strings.TrimSpace(def.Role); role != "" {
		parts = append(parts, "role: "+role)
	}
	if path := strings.TrimSpace(def.Path); path != "" {
		parts = append(parts, "definition: "+path)
	} else {
		source := strings.TrimSpace(def.Source)
		if source == "" {
			source = "unknown"
		}
		parts = append(parts, "definition: built-in ("+source+")")
	}
	return "[FlowPilot sub-agent — " + strings.Join(parts, " | ") + "]"
}

// appendPendingAgentContextLocked appends a note to the parent's UI-spawn context buffer
// and schedules a best-effort persist so it survives a restart. Caller holds s.mu (BUG-122).
func (s *InteractiveService) appendPendingAgentContextLocked(parentRunID, note string) {
	if note == "" {
		return
	}
	parent := s.runs[parentRunID]
	if parent == nil {
		return
	}
	parent.pendingAgentContext = append(parent.pendingAgentContext, note)
	if len(parent.pendingAgentContext) > maxPendingAgentNotes {
		parent.pendingAgentContext = parent.pendingAgentContext[len(parent.pendingAgentContext)-maxPendingAgentNotes:]
	}
	snap := sessionStateOf(parent)
	go func() { _ = s.persistProviderSession(snap) }()
}

// appendPendingAgentContext is the lock-acquiring variant of appendPendingAgentContextLocked.
func (s *InteractiveService) appendPendingAgentContext(parentRunID, note string) {
	s.mu.Lock()
	s.appendPendingAgentContextLocked(parentRunID, note)
	s.mu.Unlock()
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
	s.agentCatalog.providerHomeFn = func() []AgentDefinition {
		return discoverActiveProviderHomeAgents(r)
	}
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
		RunID:               rs.id,
		ProjectID:           rs.projectID,
		WorkflowID:          rs.workflowID,
		ProviderSessionID:   providerSessionID,
		ProviderKey:         rs.providerKey,
		ProviderAccountID:   rs.providerAccountID,
		WorkingDirectory:    rs.workspaceCwd,
		Status:              rs.status,
		LastPrompt:          rs.lastPrompt,
		LastMessage:         rs.lastMessage,
		StartedAt:           rs.createdAt,
		UpdatedAt:           rs.updatedAt,
		RunKind:             rs.runKind,
		SourceMachineID:     rs.sourceMachineID,
		SourceRunID:         rs.sourceRunID,
		RestoredFrom:        rs.restoredFrom,
		SyncStatus:          rs.syncStatus,
		SyncUpdatedAt:       rs.syncUpdatedAt,
		ParentRunID:         rs.parentRunID,
		AgentName:           rs.agentName,
		Role:                rs.role,
		DependsOn:           append([]string(nil), rs.dependsOn...),
		AgentStatus:         rs.agentStatus,
		ModelName:           rs.modelName,
		PendingAgentContext: append([]string(nil), rs.pendingAgentContext...),
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
		rs.agentStatus = string(RunStatusWaitingApproval)
	case EventUserQuestionRequired:
		rs.status = RunStatusWaitingQuestion
		rs.agentStatus = string(RunStatusWaitingQuestion)
	case EventTurnCompleted:
		rs.status = RunStatusCompleted
		rs.agentStatus = string(RunStatusCompleted)
		// Signal any wait:true spawn_agent waiter — non-blocking (buffered channel).
		// Must be called under s.mu so signalChild races after status is set.
		// Fall back to the last EventMessageCompleted text when FinalMessage is empty:
		// Codex new-protocol (turn/completed) may not carry finalMessage directly; the
		// actual content arrives via streaming EventMessageCompleted events first.
		finalMsg := ev.FinalMessage
		if finalMsg == "" {
			for i := len(rs.events) - 1; i >= 0; i-- {
				if rs.events[i].Type == EventMessageCompleted && rs.events[i].Text != "" {
					finalMsg = rs.events[i].Text
					break
				}
			}
		}
		s.agentOrchestrator.signalChild(rs.id, finalMsg, false, "", RunStatusCompleted)
		if rs.parentRunID != "" {
			// Tell the parent's provider conversation that a child finished, so the parent
			// agent can report its result on the next turn. UI spawns (BUG-122) and tool
			// spawns with wait=false (BUG-126) both need this; a tool spawn with wait=true
			// already returned the result synchronously as the tool result, so skip it.
			if rs.uiInitiated || !rs.waitForResult {
				s.appendPendingAgentContextLocked(rs.parentRunID, fmt.Sprintf(
					"Sub-agent %q (provider: %s) completed. Result: %s",
					rs.agentName, rs.providerKey, truncateDisplayField(finalMsg, 2000)))
			}
			switch {
			case isAgentRole(rs, "coder"):
				s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.transition(rs.parentRunID, "ready-for-review"))
				go s.recordAgentBus(rs.parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: rs.parentRunID, FromRunID: rs.id, Kind: "ready-for-review", Message: ev.FinalMessage, Queued: false, OccurredAt: ev.OccurredAt})
			case isAgentRole(rs, "review"):
				msg := strings.ToLower(ev.FinalMessage)
				switch {
				case strings.Contains(msg, "changes requested"):
					go s.recordAgentBus(rs.parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: rs.parentRunID, FromRunID: rs.id, Kind: "changes-requested", Message: ev.FinalMessage, Queued: false, OccurredAt: ev.OccurredAt})
					s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.transition(rs.parentRunID, "changes-requested"))
					roundSnap := s.agentOrchestrator.advanceRound(rs.parentRunID)
					s.emitAgentGraphLocked(rs.parentRunID, roundSnap)
					coderID := ""
					for _, childID := range s.agentOrchestrator.listChildren(rs.parentRunID) {
						if child := s.runs[childID]; isAgentRole(child, "coder") {
							coderID = childID
							break
						}
					}
					if coderID != "" {
						reviewText := ev.FinalMessage
						if roundSnap.LoopState.Status != "stopped" {
							runID := ""
							prompt := ""
							if parent := s.runs[rs.parentRunID]; parent != nil {
								if child := s.runs[coderID]; child != nil {
									if !s.loopAllowsNextTurnLocked(rs.parentRunID) || child.turnInFlight {
										parent.pendingRestartRunID = child.id
										parent.pendingRestartPrompt = reviewText
									} else {
										runID = child.id
										prompt = reviewText
									}
								}
							}
							if runID != "" && prompt != "" {
								if child := s.runs[runID]; child != nil {
									stepID := child.stepID
									go func(runID, stepID, prompt string) {
										prompt, queued := s.takeQueuedFeedbackPrompt(rs.parentRunID, runID, prompt)
										if queued != nil {
											s.recordAgentBus(rs.parentRunID, *queued)
										}
										_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
									}(runID, stepID, prompt)
								}
							}
						}
					}
				case strings.Contains(msg, "rejected"):
					go s.recordAgentBus(rs.parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: rs.parentRunID, FromRunID: rs.id, Kind: "rejected", Message: ev.FinalMessage, Queued: false, OccurredAt: ev.OccurredAt})
					s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.transition(rs.parentRunID, "rejected"))
				default:
					go s.recordAgentBus(rs.parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: rs.parentRunID, FromRunID: rs.id, Kind: "approved", Message: ev.FinalMessage, Queued: false, OccurredAt: ev.OccurredAt})
					s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.transition(rs.parentRunID, "approved"))
				}
			}
		}
	case EventTurnFailed:
		rs.status = RunStatusFailed
		rs.agentStatus = string(RunStatusFailed)
		s.agentOrchestrator.signalChild(rs.id, "", true, ev.Error, RunStatusFailed)
		if rs.parentRunID != "" {
			if rs.uiInitiated || !rs.waitForResult {
				s.appendPendingAgentContextLocked(rs.parentRunID, fmt.Sprintf(
					"Sub-agent %q (provider: %s) failed: %s",
					rs.agentName, rs.providerKey, truncateDisplayField(ev.Error, 500)))
			}
			s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.transition(rs.parentRunID, "rejected"))
		}
	default:
		// agent_graph_updated / agent_bus_message are orchestration/panel relays emitted on
		// the PARENT run to refresh the Agents panel; they are NOT the parent's own turn
		// progress. agent_spawned_by_user / agent_result_injected are timeline annotations
		// written when the user triggers a UI spawn (BUG-121). None of these should flip
		// the parent's run status to running. Only genuine turn-progress events advance status.
		if ev.Type != EventAgentGraphUpdated && ev.Type != EventAgentBusMessage &&
			ev.Type != EventAgentSpawnedByUser && ev.Type != EventAgentResultInjected {
			rs.status = RunStatusRunning
			if rs.parentRunID != "" {
				rs.agentStatus = string(RunStatusRunning)
			}
		}
	}
	shouldEmitParentGraph := false
	if rs.parentRunID != "" {
		modelName := rs.modelName
		if modelName == "" {
			// Preserve a model name set at spawn time; rs.modelName may be empty
			// when the child inherits an unresolved parent model.
			if prev, ok := s.agentOrchestrator.currentSummary(rs.parentRunID, rs.id); ok {
				modelName = prev.ModelName
			}
		}
		s.agentOrchestrator.upsertSummary(rs.parentRunID, AgentRunSummary{
			RunID:         rs.id,
			AgentName:     rs.agentName,
			Role:          rs.role,
			Status:        rs.status,
			ParentRunID:   rs.parentRunID,
			CreatedAt:     rs.createdAt,
			DependsOn:     append([]string(nil), rs.dependsOn...),
			AgentStatus:   rs.agentStatus,
			ProviderKey:   string(rs.providerKey),
			ModelName:     modelName,
			WaitForResult: rs.waitForResult,
		})
		shouldEmitParentGraph = shouldEmitAgentGraphForChildEvent(ev.Type)
	}
	if rs.parentRunID != "" && (ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed) {
		// [BUG-113 diag] A child run reaching a terminal state. finalMsgLen=0 with a low
		// event count flags a child that completed without producing any assistant output
		// (the "empty transcript" sub-agents seen in the Agents panel).
		log.Printf("[agent-spawn] child terminal parent=%q child=%q agent=%q type=%q status=%q finalMsgLen=%d events=%d",
			rs.parentRunID, rs.id, rs.agentName, ev.Type, rs.status, len(ev.FinalMessage), len(rs.events))
	}
	if rs.parentRunID != "" && ev.Type == EventTurnCompleted {
		go s.releaseDependentAgents(rs.parentRunID, rs.id, ev.FinalMessage, ev.OccurredAt)
	}
	if rs.parentRunID != "" && shouldEmitParentGraph {
		s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.graphSnapshot(rs.parentRunID))
	}

	for _, ch := range rs.subs {
		select {
		case ch <- ev:
		default: // subscriber slow/full — it reconnects via afterSeq, no gap
		}
	}
	return ev
}

func shouldEmitAgentGraphForChildEvent(eventType ProviderEventType) bool {
	switch eventType {
	case EventTurnStarted, EventPermissionRequired, EventUserQuestionRequired, EventTurnCompleted, EventTurnFailed:
		return true
	default:
		return false
	}
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

// SpawnAgent creates a child agent run from the current turn (CP-19 / Task-082).
// When in.Wait==true it blocks until the child run's first turn completes, returning
// its final message. Cancellation follows b.ctx (parent turn interrupt).
func (b *turnBridge) SpawnAgent(in SpawnAgentInput) (SpawnAgentResult, error) {
	return b.svc.spawnChildRun(b.ctx, b.rs.id, in)
}

// spawnChildRun is the shared spawn path for the spawn_agent tool and the HTTP handler.
// It creates a child interactiveRun, tags it with agent identity, fires its first turn
// asynchronously, and (when in.Wait==true) blocks until that turn completes.
func (s *InteractiveService) spawnChildRun(ctx context.Context, parentRunID string, in SpawnAgentInput) (SpawnAgentResult, error) {
	// [BUG-113 diag] One line per spawn_agent invocation. If the agents list shows more
	// children than expected, this reveals whether the orchestrator called spawn_agent
	// multiple times (and with what params) vs. a single intended spawn.
	log.Printf("[agent-spawn] request parent=%q agent=%q provider=%q wait=%t dependsOn=%v promptLen=%d",
		parentRunID, in.Agent, in.Provider, in.Wait, in.DependsOn, len(in.Prompt))
	// Validate that the parent run exists before creating any child resource.
	var agentDef *AgentDefinition
	s.mu.Lock()
	parentRun := s.runs[parentRunID]
	cwd := ""
	projectID := ""
	workflowID := ""
	parentModel := ""
	parentReasoningEffort := ""
	parentProviderKey := ProviderKey("")
	parentYolo := false
	if parentRun != nil {
		cwd = parentRun.workspaceCwd
		projectID = parentRun.projectID
		workflowID = parentRun.workflowID
		parentModel = parentRun.modelName
		parentReasoningEffort = parentRun.reasoningEffort
		parentProviderKey = parentRun.providerKey
		parentYolo = parentRun.yolo
	}
	s.mu.Unlock()
	if parentRun == nil {
		return SpawnAgentResult{}, fmt.Errorf("parent run %q not found", parentRunID)
	}
	if defs := s.agentCatalog.listAgents(cwd); len(defs) > 0 {
		for i := range defs {
			if strings.EqualFold(defs[i].Name, in.Agent) {
				def := defs[i]
				agentDef = &def
				break
			}
		}
	}

	// Determine provider: explicit input > agent definition > parent run's provider.
	providerKey := ProviderKey(in.Provider)
	if providerKey == "" && agentDef != nil && agentDef.Provider != "" {
		providerKey = ProviderKey(agentDef.Provider)
	}
	if providerKey == "" {
		s.mu.Lock()
		if parent := s.runs[parentRunID]; parent != nil {
			providerKey = parent.providerKey
		}
		s.mu.Unlock()
	}

	// Resolve the child model and reasoning effort.
	// Priority: agent definition > same-provider inheritance > per-provider default.
	// When the child runs on a different provider than the parent, the parent's model
	// name is invalid for the child (e.g. "sonnet" sent to Codex → 400).
	childModel := parentModel
	childReasoningEffort := parentReasoningEffort
	if agentDef != nil && agentDef.Model != "" {
		childModel = agentDef.Model
		childReasoningEffort = agentDef.ModelReasoningEffort
	} else if providerKey != parentProviderKey {
		childModel = defaultModelForProvider(providerKey)
		childReasoningEffort = ""
	}
	// If model is still unresolved (parent was started without an explicit model),
	// fall back to the provider default so the UI always shows a model name.
	if childModel == "" {
		childModel = defaultModelForProvider(providerKey)
	}

	// Create the child run. createRun acquires s.mu internally; call it unlocked.
	// The child inherits the parent's YOLO posture (BUG-129): with YOLO on, the
	// child's gated actions must auto-approve just like the parent's, instead of
	// stalling the (often wait=true) parent turn on a child approval prompt.
	startIn := StartRunInput{
		ProjectID:       projectID,
		WorkflowID:      workflowID,
		ChatMode:        "normal_chat",
		Cwd:             cwd,
		ProviderKey:     providerKey,
		Model:           childModel,
		ReasoningEffort: childReasoningEffort,
		YoloMode:        parentYolo,
	}
	handle, apiErr := s.createRun(startIn)
	if apiErr != nil {
		return SpawnAgentResult{}, fmt.Errorf("%s: %s", apiErr.code, apiErr.msg)
	}

	// Register the wait:true completion channel BEFORE starting the child turn to
	// eliminate the race between turn completion and the caller's select.
	var waiterCh <-chan agentCompletion
	if in.Wait {
		waiterCh = s.agentOrchestrator.openWaiter(handle.RunID)
	}

	// Compose the first user turn the same way for every provider so a given
	// agent name produces an identical prompt shape on Claude and Codex (BUG-128).
	// The agent's system prompt (when any) stays first so built-in-agent prompt
	// detection keeps working; a single identity line then names the agent and
	// links its definition file so the model can open the full spec itself.
	firstPrompt := composeAgentSpawnPrompt(agentDef, in.Prompt)

	// Stamp agent identity on the newly created child run.
	s.mu.Lock()
	var childSnap ProviderSessionState
	agentStatus := "spawned"
	blockedStart := false
	if rs := s.runs[handle.RunID]; rs != nil {
		rs.parentRunID = parentRunID
		rs.dependsOn = in.DependsOn
		rs.agentStatus = "spawned"
		rs.stepID = handle.StepID
		rs.uiInitiated = in.UIInitiated
		rs.waitForResult = in.Wait
		if agentDef != nil {
			rs.agentName = agentDef.Name
			rs.role = agentDef.Role
		} else {
			rs.agentName = in.Agent
			rs.role = strings.ToLower(in.Agent)
		}
		if len(rs.dependsOn) > 0 && (!s.dependenciesSatisfiedLocked(rs) || !s.loopAllowsNextTurnLocked(parentRunID)) {
			rs.agentStatus = "waiting_dependency"
			rs.pendingTurnPrompt = firstPrompt
			agentStatus = rs.agentStatus
			blockedStart = true
		}
		childSnap = sessionStateOf(rs)
	}
	s.mu.Unlock()
	if childSnap.RunID != "" {
		if err := s.persistProviderSession(childSnap); err != nil {
			return SpawnAgentResult{}, err
		}
	}

	// [BUG-113 diag] The minted child run + resolved identity. Pair this with the
	// "[agent-spawn] request" line above to map each spawn call to its child run id.
	log.Printf("[agent-spawn] child created parent=%q child=%q agent=%q role=%q provider=%q model=%q blockedStart=%t",
		parentRunID, handle.RunID, childSnap.AgentName, childSnap.Role, childSnap.ProviderKey, childModel, blockedStart)

	// Record the tree edge.
	s.agentOrchestrator.registerChild(parentRunID, handle.RunID)
	s.agentOrchestrator.mu.Lock()
	s.agentOrchestrator.edges[parentRunID] = append(s.agentOrchestrator.edges[parentRunID], AgentDependencyEdge{FromRunID: parentRunID, ToRunID: handle.RunID, Kind: "spawn"})
	for _, depID := range in.DependsOn {
		s.agentOrchestrator.edges[parentRunID] = append(s.agentOrchestrator.edges[parentRunID], AgentDependencyEdge{FromRunID: depID, ToRunID: handle.RunID, Kind: "depends-on"})
	}
	st := s.agentOrchestrator.ensureLoopLocked(parentRunID)
	if st.Status == "" {
		st.Status = "running"
	}
	s.agentOrchestrator.loop[parentRunID] = st
	s.agentOrchestrator.mu.Unlock()
	s.agentOrchestrator.upsertSummary(parentRunID, AgentRunSummary{
		RunID:         handle.RunID,
		AgentName:     childSnap.AgentName,
		Role:          childSnap.Role,
		Status:        RunStatus(childSnap.Status),
		ParentRunID:   parentRunID,
		CreatedAt:     childSnap.StartedAt,
		DependsOn:     append([]string(nil), in.DependsOn...),
		AgentStatus:   agentStatus,
		ProviderKey:   string(childSnap.ProviderKey),
		ModelName:     childModel,
		WaitForResult: in.Wait,
	})
	_ = s.agentOrchestrator.addBus(parentRunID, AgentBusMessage{ID: s.nextID("bus"), ParentRunID: parentRunID, FromRunID: parentRunID, ToRunID: handle.RunID, Kind: "handoff", Message: in.Prompt, Queued: false, OccurredAt: time.Now().UTC().Format(time.RFC3339Nano)})
	s.emitAgentGraph(parentRunID, s.agentOrchestrator.graphSnapshot(parentRunID))
	// Persist a spawn annotation on the parent run (BUG-121). This writes a permanent
	// event to the parent's event log so the spawn is visible in the parent timeline
	// after a server restart, regardless of whether the spawn came from the UI or a tool.
	s.emitOnParentRun(parentRunID, ProviderEvent{
		Type:       EventAgentSpawnedByUser,
		AgentName:  childSnap.AgentName,
		ChildRunID: handle.RunID,
	})
	// Queue a context note for the parent's next provider turn so the parent agent learns
	// about a child the UI started (the AI tool path is already in provider history). (BUG-122)
	if in.UIInitiated {
		s.appendPendingAgentContext(parentRunID, fmt.Sprintf(
			"Sub-agent %q (provider: %s, model: %s) was started from the FlowPilot UI.",
			childSnap.AgentName, childSnap.ProviderKey, childModel))
	}

	// Fire the first turn asynchronously; the child streams via its own SSE.
	if !blockedStart {
		go func() {
			_, turnErr := s.startTurn(handle.RunID, TurnInput{
				StepID: handle.StepID,
				Prompt: firstPrompt,
			}, "", "")
			if turnErr != nil {
				// startTurn failed before the adapter ran — signal the waiter explicitly.
				s.agentOrchestrator.signalChild(handle.RunID, "", true, turnErr.msg, RunStatusFailed)
			}
		}()
	}

	result := SpawnAgentResult{
		RunID:             handle.RunID,
		ProviderSessionID: handle.ProviderSessionID,
		ProviderKey:       string(handle.ProviderKey),
		Status:            "spawned",
	}
	if blockedStart {
		result.Status = agentStatus
	}

	if in.Wait && !blockedStart {
		select {
		case completion := <-waiterCh:
			if completion.failed {
				return SpawnAgentResult{}, fmt.Errorf("child agent failed: %s", completion.errMsg)
			}
			result.Status = string(completion.status)
			if completion.status == RunStatusCompleted {
				result.FinalMessage = completion.finalMessage
				// Persist the child's result as an annotation on the parent run (BUG-121).
				if result.FinalMessage != "" {
					s.emitOnParentRun(parentRunID, ProviderEvent{
						Type:         EventAgentResultInjected,
						AgentName:    childSnap.AgentName,
						FinalMessage: result.FinalMessage,
					})
				}
			}
		case <-ctx.Done():
			return SpawnAgentResult{}, ctx.Err()
		}
	}

	return result, nil
}

// listAgentRunSummaries returns the child run summaries for a given parent run.
// It includes live children (in-memory runs) and historical children persisted
// from a previous sync/restore round-trip (CP-19 / Task-082).
func (s *InteractiveService) listAgentRunSummaries(parentRunID string) []AgentRunSummary {
	childIDs := s.agentOrchestrator.listChildren(parentRunID)
	s.mu.Lock()
	liveIDs := make(map[string]struct{}, len(childIDs))
	out := make([]AgentRunSummary, 0, len(childIDs))
	for _, id := range childIDs {
		rs := s.runs[id]
		if rs == nil {
			continue
		}
		liveIDs[id] = struct{}{}
		out = append(out, AgentRunSummary{
			RunID:         rs.id,
			AgentName:     rs.agentName,
			Role:          rs.role,
			Status:        rs.status,
			ParentRunID:   rs.parentRunID,
			CreatedAt:     rs.createdAt,
			DependsOn:     append([]string(nil), rs.dependsOn...),
			AgentStatus:   rs.agentStatus,
			ProviderKey:   string(rs.providerKey),
			ModelName:     rs.modelName,
			WaitForResult: rs.waitForResult,
		})
	}
	s.mu.Unlock()
	// Append historical summaries from a prior sync/restore that are not in the live map.
	seenIDs := make(map[string]struct{}, len(out))
	for _, summary := range out {
		seenIDs[summary.RunID] = struct{}{}
	}
	for _, h := range s.agentOrchestrator.historicalChildren(parentRunID) {
		if _, live := liveIDs[h.RunID]; !live {
			out = append(out, h)
			seenIDs[h.RunID] = struct{}{}
		}
	}
	if indexReader, ok := s.workflowStore.(SessionIndexReader); ok {
		sessions, err := indexReader.ListAllProviderSessions(context.Background())
		if err == nil {
			for _, session := range sessions {
				if session.ParentRunID != parentRunID {
					continue
				}
				if _, seen := seenIDs[session.RunID]; seen {
					continue
				}
				out = append(out, AgentRunSummary{
					RunID:       session.RunID,
					AgentName:   session.AgentName,
					Role:        session.Role,
					Status:      session.Status,
					ParentRunID: session.ParentRunID,
					CreatedAt:   session.StartedAt,
					DependsOn:   append([]string(nil), session.DependsOn...),
					AgentStatus: session.AgentStatus,
					ProviderKey: string(session.ProviderKey),
					ModelName:   session.ModelName,
				})
				seenIDs[session.RunID] = struct{}{}
			}
		}
	}
	return out
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
	providerSessionID := rs.providerSessionID
	if rs.providerKey == ProviderKeyCodex && rs.realProviderSessionID != "" {
		providerSessionID = rs.realProviderSessionID
	}
	// Fold any pending UI-spawn context into the provider prompt (NOT the displayed prompt,
	// which was already emitted via turn_started with in.Prompt). This is how the parent
	// agent learns about children started from the UI. Cleared once consumed; the cleared
	// state is persisted by the post-turn sessionStateOf snapshot below. (BUG-122)
	providerPrompt := in.Prompt
	s.mu.Lock()
	// Persist the per-turn YOLO posture as the run's current default (BUG-129). The UI
	// toggle is sticky, so an explicit YoloMode this turn must update rs.yolo; otherwise a
	// child spawned during this turn (spawnChildRun reads parentRun.yolo) would inherit the
	// stale run-level default instead of the posture the user actually has enabled.
	if in.YoloMode != nil {
		rs.yolo = yolo
	}
	if len(rs.pendingAgentContext) > 0 {
		providerPrompt = composeAgentContextBlock(rs.pendingAgentContext) + "\n\n" + in.Prompt
		rs.pendingAgentContext = nil
	}
	s.mu.Unlock()
	// Live ledger refresh (CP-35): pick up commits made during this session so the
	// oracle always sees the current change history, not just what existed at bind time.
	s.rebuildLedgerIfDirty(rs.workspaceCwd)
	req := TurnRequest{
		RunID:             rs.id,
		StepID:            in.StepID,
		ProviderSessionID: providerSessionID,
		ProviderTurnID:    turnID,
		Prompt:            providerPrompt,
		ModelName:         model,
		SelectedSkills:    in.SelectedSkills,
		YoloMode:          yolo,
		ReasoningEffort:   effort,
		Cwd:               rs.workspaceCwd,
		Scenario:          scenario,
		Attachments:       in.Attachments,
	}
	// CP-35: snapshot HEAD before the AI runs so gate_hook can diff committed changes.
	if head, headErr := captureGitHead(rs.workspaceCwd); headErr == nil {
		s.mu.Lock()
		rs.turnStartGitHead = head
		s.mu.Unlock()
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
	newCodexSessionID := s.refreshResumeHandleLocked(rs, adapter)
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	_ = s.persistProviderSession(snap)
	// Log the new Codex rollout session id so seedTranscriptFromDisk can load
	// every per-turn rollout file on resume (BUG-083 F-3).
	if newCodexSessionID != "" {
		if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
			_ = logger.AppendTurnLog(context.Background(), rs.id, turnLogLine{Kind: turnLogKindCodexSession, SessionID: newCodexSessionID})
		}
	}
	if err == nil {
		_ = s.syncCodexStableSessionToKnownAccounts(rs)
	}
	s.mu.Lock()
	rs.turnInFlight = false
	pendingRestartRunID := ""
	pendingRestartPrompt := ""
	if rs.parentRunID != "" {
		if parent := s.runs[rs.parentRunID]; parent != nil {
			pendingRestartRunID = parent.pendingRestartRunID
			pendingRestartPrompt = parent.pendingRestartPrompt
			parent.pendingRestartRunID = ""
			parent.pendingRestartPrompt = ""
		}
	}
	s.mu.Unlock()

	// Post-turn flow gate (CP-35 P-4/P-5): observe diff, evaluate rules, enforce.
	// Non-fatal: any internal error inside runFlowGate degrades to pass.
	if completed {
		if s.runFlowGate(ctx, rs, turnID, fin) {
			completed = false
		}
	}

	// Finalizer hook runs OUTSIDE s.mu and only on a clean completion. A finalize
	// failure is recorded as retryable and must not erase the completed turn (04-04).
	if completed {
		_ = s.finalizer.Finalize(fin)
	}
	if completed && pendingRestartRunID == rs.id && pendingRestartPrompt != "" {
		prompt, queued := s.takeQueuedFeedbackPrompt(rs.parentRunID, rs.id, pendingRestartPrompt)
		if queued != nil {
			s.recordAgentBus(rs.parentRunID, *queued)
		}
		go func(runID, stepID, prompt string) {
			_, _ = s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
		}(rs.id, rs.stepID, prompt)
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
	if rs.providerAccountID != s.activeAccountForProvider(rs.providerKey) {
		if rs.runKind != "chat" {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusConflict, "provider_account_changed", "active provider account changed since the run started")
		}
		s.mu.Unlock()
		if err := s.ensureResumeReady(rs); err != nil {
			return "", err
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
		}
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
	if rs.providerKey == ProviderKeyCodex && rs.realProviderSessionID != "" && !strings.HasPrefix(rs.realProviderSessionID, "thread-") {
		// Live Codex app-server adapters resume provider-owned rollout threads in-process so
		// approvals, MCP confirmations, and ask_user still bridge through FlowPilot on follow-up
		// turns. The CLI resume adapter remains as a non-app-server fallback only.
		if _, ok := adapter.(*codexAdapter); !ok && !codexAppServerEnabled() {
			home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
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
	}
	if rs.resumedFromDisk && rs.providerKey == ProviderKeyClaude {
		if live, ok := adapter.(*claudeAdapter); ok && rs.realProviderSessionID != "" {
			live.pool.setRealSession(rs.providerSessionID, rs.realProviderSessionID)
			live.pool.setRealSession(rs.realProviderSessionID, rs.realProviderSessionID)
		}
	}

	turnID := s.nextID("turn")
	rs.turnInFlight = true
	rs.lastTurnStepID = in.StepID // CP-35: remember for gate reprompts
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
	// Persist raw user prompt for transcript replay (BUG-083 F-1): the provider
	// session file records the composed prompt (raw + reinforcement + skill preamble),
	// so we keep the raw input separately and prefer it on resume.
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		_ = logger.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, Prompt: in.Prompt})
	}

	go s.runTurn(ctx, rs, adapter, in, scenario, turnID)
	return turnID, nil
}

// refreshResumeHandleLocked updates the in-memory resume handle after a turn
// completes.  For Codex it always re-discovers the newest rollout session id
// (Codex writes one file per turn) and returns it if it changed — the caller
// logs the new id outside the lock for multi-rollout replay (BUG-083 F-3).
// Returns "" for Claude or when the Codex session id did not change.
func (s *InteractiveService) refreshResumeHandleLocked(rs *interactiveRun, adapter ProviderRuntimeAdapter) string {
	switch rs.providerKey {
	case ProviderKeyClaude:
		live, ok := adapter.(*claudeAdapter)
		if !ok {
			return ""
		}
		real := live.pool.realSession(rs.providerSessionID)
		if real == "" {
			real = live.pool.realSession(rs.realProviderSessionID)
		}
		if real != "" {
			rs.realProviderSessionID = real
		}
	case ProviderKeyCodex:
		home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
		if !ok {
			return ""
		}
		if rolloutID, found := DiscoverCodexRolloutSessionID(home, rs.workspaceCwd); found {
			if rs.realProviderSessionID == "" {
				rs.realProviderSessionID = rolloutID
			}
			if rolloutID != rs.lastCodexTurnSessionID {
				rs.lastCodexTurnSessionID = rolloutID
				return rolloutID
			}
		}
	}
	return ""
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
//   - flips the active account. Workflow runs remain scoped to the account they
//     started with, while chat runs prepare their provider session files on the
//     newly active same-provider account before the next turn.
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

// defaultModelForProvider returns the baseline model name to use when spawning a
// child run on a different provider than the parent and the agent definition does
// not declare an explicit model. Using the parent's model name cross-provider
// causes a 400 from the target provider (e.g. "sonnet" sent to Codex).
func defaultModelForProvider(key ProviderKey) string {
	switch key {
	case ProviderKeyCodex:
		return "gpt-5.4-mini"
	case ProviderKeyClaude:
		return "sonnet"
	default:
		return ""
	}
}

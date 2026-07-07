package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flowpilot-runner/internal/agentpack"
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
	// flowDefinitionStore backs CP-42/Task-177 flowRef resolution for
	// startResolvedFlow. Nil is valid: FlowDefinitionResolver falls back to
	// resolving directly from the embedded agentpack, which is sufficient for
	// built-in flows like Review Loop. Set via SetFlowDefinitionStore once a
	// real store (file- or Supabase-backed) is available (see
	// runner.FlowDefinitionStoreFor, called from cmd/flowpilot's serve command).
	flowDefinitionStore FlowDefinitionStore

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

	syncContextEngineFilesHook func(projectID, dotFlowpilotDir string) (error, string)

	// summaryTimers holds the per-run idle timer that fires a rolling chat-summary
	// after the chat has been quiet for the idle window. A new turn cancels it; a
	// completed turn (re)schedules it. Guarded by summaryMu (not s.mu) so the timer
	// callback never contends with turn handling.
	summaryMu     sync.Mutex
	summaryTimers map[string]*time.Timer
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
	changeType      string
	sourceDocID     string
	turnCount       int
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
	// flowCohortId groups siblings into a barrier; non-empty means this child's result
	// is buffered until all cohort members are terminal, then one consolidated note is
	// delivered to the hub (Task-092 / CP-36 P-6).
	flowCohortId string
	// label is the display name used in consolidated cohort notes; defaults to agentName.
	label string
	// waitForResult records the spawn's wait flag. A tool spawn with wait=true returns the
	// child result synchronously to the model (already in provider history). A tool spawn
	// with wait=false only acks "spawned" — its eventual result must be injected like a UI
	// spawn so the parent still learns the outcome (BUG-126).
	waitForResult bool
	// activationSeq tracks how many times this child has been reinvoked. Incremented
	// in reinvokeExistingFlowChild each time the node is reactivated (lifecycle: reinvoke).
	// Surfaced via AgentRunSummary.ActivationSeq so the desktop can distinguish a genuine
	// completed→running transition from a stale HTTP snapshot (BUG-Rnd2 Bug B).
	activationSeq int
	// autoOrchestrate enables bounded hub auto-reinvocation (Task-093 / CP-36 P-7). Set on
	// root (parent) runs only; child runs leave this false. When true, maybeAutoReinvokeHub
	// is called after each cohort join to re-prompt the hub without a human typing.
	autoOrchestrate bool
	// activeFlowEdges is the resolved flow's edge list, set once on the hub/parent
	// run when startResolvedFlow spawns its entry node (CP-42/Task-180). When
	// non-empty, maybeReinvokeCoderForContinue resolves the "continue" reinvoke
	// target from this data (matching a child's label to a back-edge's target
	// node id) instead of the isCoderRun role-name fallback. Empty for runs
	// started without a flowRef (the AI-driven spawn_agent path), which keeps
	// using the role-based fallback unchanged.
	activeFlowEdges []agentpack.FlowEdge
	// activeFlowNodes is the resolved flow's node list, set alongside
	// activeFlowEdges. tryAdvanceFlowFromNode resolves a completed node's
	// outgoing forward edges' targets against this list (agent/behavior
	// binding) to auto-spawn the next node(s) deterministically instead of
	// leaving a bare completion note for the hub AI to infer from.
	activeFlowNodes []agentpack.FlowNode
	// flowEngineDriven marks a run whose step-runtime timeline is driven by the
	// CP-42 flow executor's real node lifecycle (spawn → RUNNING, complete →
	// DONE, flow "done" → run DONE) rather than the legacy per-turn
	// PlanWorkflowProgress bulk planner (BUG-174). Set only for a Flow-Mode
	// workflow-picker launch whose selected workflow auto-resolves to a
	// flow-engine flow (see handleStartTurn → resolveWorkflowFlowRef), never for
	// the explicit chat/flowRef path or a plain workflow, so those keep their
	// exact existing behavior. When true, startTurn skips the bulk Progress call
	// and the executor owns every step transition for this run.
	flowEngineDriven bool
	// planContextPackage is the FlowContextPackage built for the Plan step of this Flow
	// Mode run. Non-nil only for workflow runs with a Coding step. Cached here so retries
	// reuse the same package without rebuilding; cleared when a Plan step reruns (Task-169).
	planContextPackage *FlowContextPackage
	// reinvokeInFlight is the single-flight guard for auto-reinvocation. Set true when a
	// hub re-prompt has been scheduled; cleared when the new turn starts (in startTurn).
	reinvokeInFlight bool
	// pendingHubReinvoke is set when maybeAutoReinvokeHub is called while the hub turn
	// is still in flight (turnInFlight==true). runTurn clears turnInFlight and then
	// retries the reinvoke so the coder-completion note is not silently dropped.
	pendingHubReinvoke bool
	// pendingAgentContext holds notes about UI-spawned children (and their results) that
	// have not yet been folded into this (parent) run's provider conversation. They are
	// prepended to the next provider turn's prompt and then cleared. Persisted to
	// sessions.ndjson so the parent still learns about them after a server restart. (BUG-122)
	pendingAgentContext []string
	// lastCohortNote holds the most recently joined reviewer-cohort note (BUG-233),
	// so it survives past pendingAgentContext being drained into the hub's synthesis
	// turn. Used to build a findings-based fallback GateReason if that turn completes
	// without calling submit_review_outcome (CA-226), instead of an internal
	// diagnostic sentence. Overwritten on each new cohort join.
	lastCohortNote string

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

	turnInFlight          bool
	repromptAttempts      int    // CP-35 P-5: number of flow-gate reprompts issued this turn
	turnStartGitHead      string // CP-35: git HEAD captured at turn start for committed-diff detection
	lastTurnStepID        string // CP-35: stepID of the most-recently started turn, used by gate reprompts
	currentTurnID         string
	lastFlowControlTurnID string
	lastTurnID            string // id of the most-recently completed turn, for the rolling chat summary
	turnCancel            context.CancelFunc

	pendingApprovalID string
	pendingQuestionID string
	// pendingGateBlock holds r-reg details for the decision handler (Task-155).
	// Cleared when the user submits a decision via handleGateDecision.
	pendingGateBlock *gateBlockInfo
	// proposalTurnPending is set true when the user picks opt-2 (suggest requirement change).
	// The next turn is a proposal-only turn where the AI proposes but does not fix code yet;
	// runFlowGate must not re-block on r-reg/r-tests during that turn.
	proposalTurnPending bool

	subs    map[int64]chan ProviderEvent
	nextSub int64

	idempotency     map[string]string // Idempotency-Key -> turnId
	resumedFromDisk bool
	// transcriptSeeded guards against duplicate seedTranscriptFromDisk calls.
	// It is set to true the first time the transcript (or Gemini turn log) is
	// loaded from disk, so that pre-loaded flow events in rs.events (from the
	// CP-41 flow-events sidecar) do not suppress transcript seeding.
	transcriptSeeded bool
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
		summaryTimers:     map[string]*time.Timer{},
	}
	// Seed the id counter above the highest persisted run id so a runner restart does NOT
	// reuse ids (run-1, run-2, …). Reuse made a fresh chat collide with a previous run of
	// the same id and inherit its persisted child agents — old sub-agents appeared in a
	// brand-new session's Agents panel. (BUG-117)
	svc.seedIDCounter()
	return svc
}

func (s *InteractiveService) syncContextEngineFilesBestEffort(projectID, dotFlowpilotDir string) (error, string) {
	if s.syncContextEngineFilesHook != nil {
		return s.syncContextEngineFilesHook(projectID, dotFlowpilotDir)
	}
	return s.syncContextEngineFiles(projectID, dotFlowpilotDir)
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
		parent.autoOrchestrate = false
		parent.reinvokeInFlight = false
		parent.pendingHubReinvoke = false
		if parent.turnInFlight && parent.turnCancel != nil {
			parent.turnCancel()
			// BUG-248: turnCancel() only signals cancellation — the run's own
			// finishTurn() flips `status` to cancelled asynchronously once the turn's
			// goroutine observes ctx.Done(), and that happens off this lock.
			parent.status = RunStatusCancelled
		}
	}
	cancelledChildIDs := make([]string, 0)
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		if child := s.runs[childID]; child != nil {
			child.pendingTurnPrompt = ""
			if child.turnInFlight && child.turnCancel != nil {
				child.turnCancel()
				child.status = RunStatusCancelled // BUG-248: see parent.status comment above.
				child.agentStatus = string(RunStatusCancelled)
				cancelledChildIDs = append(cancelledChildIDs, childID)
			}
		}
	}
	s.mu.Unlock()
	// BUG-248: the snapshot below is built from AgentOrchestrator's own summary cache
	// (upsertSummary), not from interactiveRun.status directly — the two are only kept
	// in sync by emitLocked reacting to turn-progress events, which for a cancelled
	// turn only happens later, asynchronously, once finishTurn's goroutine observes
	// ctx.Done(). Patch the cached summaries eagerly here too, or this snapshot (and
	// the desktop's derived run status computed from it) would still read the child as
	// "running" with no later agent_graph_updated event ever correcting it.
	for _, childID := range cancelledChildIDs {
		if summary, ok := s.agentOrchestrator.currentSummary(parentRunID, childID); ok {
			summary.Status = RunStatusCancelled
			summary.AgentStatus = string(RunStatusCancelled)
			s.agentOrchestrator.upsertSummary(parentRunID, summary)
		}
	}
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

// applyFlowControl advances the generic flow engine for parentRunID.
// This is the synchronous core: it records the transition, emits the graph update,
// and returns a FlowControlResult. Re-entry of target nodes is wired by Task-091/092.
func (s *InteractiveService) applyFlowControl(parentRunID string, in FlowControlInput) (FlowControlResult, error) {
	s.flowDiagLog(parentRunID, "flow_control_received", "received flow control input",
		"status", in.Status,
		"summary_len", len(strings.TrimSpace(in.Summary)),
	)
	// Validate the target run exists before mutating orchestrator state (MEDIUM finding).
	s.mu.Lock()
	rs, runExists := s.runs[parentRunID]
	if runExists && rs.currentTurnID != "" {
		rs.lastFlowControlTurnID = rs.currentTurnID
	}
	s.mu.Unlock()
	if !runExists {
		s.flowDiagLog(parentRunID, "flow_control_missing_run", "flow control target run was not found",
			"status", in.Status,
		)
		return FlowControlResult{}, fmt.Errorf("applyFlowControl: run %q not found", parentRunID)
	}
	switch in.Status {
	case "done":
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "done"
			st.OpenIssues = 0
			st.GateReason = ""
			return st
		})
		s.appendPendingAgentContext(parentRunID, strings.TrimSpace("Flow completed. "+in.Summary))
		// BUG-174/BUG-242: settle the step timeline (inline hub node DONE, run
		// DONE) BEFORE emitAgentGraph below, not after. emitAgentGraph fires the
		// agent_graph_updated SSE event the desktop reacts to by refreshing the
		// step-runtime snapshot (refreshWorkflowStepRuntime) — BUG-233 already
		// established this exact ordering requirement for the "continue"/
		// looping branch above, but the "done" branch still emitted first and
		// settled the steps after. That let the desktop's refresh race ahead of
		// these writes and read the still-RUNNING hub/synthesis node from the
		// prior instant, and since setFlowStepStatus emits no event of its own,
		// nothing corrected the stale RUNNING display until an unrelated event
		// (e.g. a manual agent focus switch) triggered another refresh.
		if s.isFlowEngineDriven(parentRunID) {
			s.markFlowRunComplete(context.Background(), parentRunID)
		}
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.flowDiagLog(parentRunID, "flow_control_done", "flow marked done",
			"next_action", "done",
			"round", snap.LoopState.Round,
			"cap", effectiveCap(snap.LoopState),
		)
		return FlowControlResult{Status: "done", Round: snap.LoopState.Round, Cap: effectiveCap(snap.LoopState), NextAction: "done"}, nil

	case "continue":
		// Extract open issue count from the payload. The payload may carry either
		// []ReviewIssue (set by reviewOutcomeToFlowControl) or []any (decoded from
		// raw JSON); handle both so the board always sees the correct open count.
		issueCount := 0
		switch v := in.Payload["issues"].(type) {
		case []ReviewIssue:
			issueCount = len(v)
		case []any:
			issueCount = len(v)
		}
		var result FlowControlResult
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.OpenIssues = issueCount
			cap := effectiveCap(st)
			st.Round++
			if cap > 0 && st.Round >= cap {
				st.Status = "blocked"
				st.BlockReason = "cap" // BUG-231: distinguishes cap-reached from escalate for the resume UX
				st.GateReason = fmt.Sprintf("cap %d reached with %d open issue(s)", cap, st.OpenIssues)
				result = FlowControlResult{Status: "blocked", Round: st.Round, Cap: cap, OpenIssues: st.OpenIssues, NextAction: "awaiting_user"}
			} else {
				if st.Status == "" || st.Status == "blocked" {
					st.Status = "running"
					st.BlockReason = ""
				}
				result = FlowControlResult{Status: "continue", Round: st.Round, Cap: cap, OpenIssues: st.OpenIssues, NextAction: "looping"}
			}
			return st
		})
		if result.NextAction == "looping" {
			// BUG-174: a new review round is starting — reset the downstream nodes
			// to PENDING and re-run the entry (coder) node on the step timeline so
			// the loop reads honestly instead of every node staying DONE.
			//
			// BUG-233: this MUST complete before emitAgentGraph below. emitAgentGraph
			// fires the agent_graph_updated SSE event that the desktop reacts to by
			// refreshing the step-runtime snapshot; doing the reset in a goroutine
			// raced that refresh against these writes, so the desktop could read the
			// backend's step store before the new round's PENDING/RUNNING transitions
			// landed and render the stale "all done" snapshot from the prior round.
			if s.isFlowEngineDriven(parentRunID) {
				nodes := s.activeFlowNodesFor(parentRunID)
				entryID := flowEntryNodeID(nodes)
				for _, n := range nodes {
					if n.ID != entryID {
						s.setFlowStepStatus(context.Background(), parentRunID, n.ID, StepStatusPending)
					}
				}
				s.setFlowStepStatus(context.Background(), parentRunID, entryID, StepStatusRunning)
			}
		} else if result.NextAction == "awaiting_user" {
			// BUG-231/BUG-233: the round cap paused the loop awaiting the user —
			// settle the hub node to WAITING_USER_APPROVAL (synchronously, before
			// emitAgentGraph — see BUG-233 note above) instead of leaving it RUNNING.
			s.setFlowStepAwaitingUser(context.Background(), parentRunID)
		}
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.flowDiagLog(parentRunID, "flow_control_continue", "flow continue applied",
			"next_action", result.NextAction,
			"round", result.Round,
			"cap", result.Cap,
			"open_issues", result.OpenIssues,
		)
		if result.NextAction == "looping" {
			// Re-enter the coder so the back-edge in ReviewLoopFlowConfig fires
			// (CRITICAL finding: continue returned "looping" but never restarted the coder).
			go s.maybeReinvokeCoderForContinue(parentRunID, buildCoderReentryPrompt(in))
		}
		return result, nil

	case "escalate":
		// BUG-231: escalate is a deliberate, non-terminal "awaiting user" pause —
		// settle the hub node to WAITING_USER_APPROVAL (not RUNNING, which reads
		// as a hang, and not FAILED, which reads as an error) so the step
		// timeline honestly shows the flow is waiting on the human, not stuck.
		// BUG-233: done before emitAgentGraph so the desktop's step-runtime
		// refresh (triggered by the SSE event below) can't observe a stale
		// RUNNING snapshot.
		// BUG-244: settle the step BEFORE mutateLoop flips the loop to
		// "blocked", not after. The step reaching its awaiting-user state is the
		// CAUSE; the loop being blocked is the effect — so "loop blocked" must
		// imply "step already settled" for every observer, including one that
		// keys off the loop status alone (e.g. a poller/waiter reading the
		// orchestrator loop state, which flips in-memory here with no SSE of its
		// own). Doing mutateLoop first left a window where the loop read
		// "blocked" while the step was still RUNNING.
		s.setFlowStepAwaitingUser(context.Background(), parentRunID)
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = "escalate" // BUG-231
			if in.Summary != "" {
				st.GateReason = in.Summary
			} else {
				st.GateReason = "escalated"
			}
			return st
		})
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.flowDiagLog(parentRunID, "flow_control_escalate", "flow escalated to awaiting user",
			"next_action", "awaiting_user",
			"round", snap.LoopState.Round,
			"cap", effectiveCap(snap.LoopState),
			"summary", strings.TrimSpace(in.Summary),
		)
		return FlowControlResult{Status: "blocked", Round: snap.LoopState.Round, Cap: effectiveCap(snap.LoopState), NextAction: "awaiting_user"}, nil

	default:
		s.flowDiagLog(parentRunID, "flow_control_unknown_status", "unknown flow control status received",
			"status", in.Status,
		)
		return FlowControlResult{}, fmt.Errorf("applyFlowControl: unknown status %q", in.Status)
	}
}

func (s *InteractiveService) flowControlSubmittedForTurn(runID, turnID string) bool {
	if runID == "" || turnID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	return rs != nil && rs.lastFlowControlTurnID == turnID
}

// extendCap raises the flow cap by ExtendBy and resumes from blocked.
//
// BUG-231 (D-8): ExtendMax previously rejected this once ExtendCount reached
// 2. That limit only ever bounded this USER-triggered action (never any auto
// path), so its only remaining effect was to block the human after two
// extensions — reproducing the exact "awaiting user with no way to act"
// wedge this bug fixes. Retired: ExtendCount still increments for
// display/telemetry, but no longer rejects the extend.
func (s *InteractiveService) extendCap(parentRunID string) (FlowControlResult, error) {
	var result FlowControlResult
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		cap := effectiveCap(st)
		st.Cap = cap + effectiveExtendBy(st)
		// mirror RoundCap so existing board readers see the new limit
		st.RoundCap = st.Cap
		st.ExtendCount++
		if st.Status == "blocked" {
			st.Status = "running"
			st.GateReason = ""
			st.BlockReason = ""
		}
		result = FlowControlResult{Status: st.Status, Round: st.Round, Cap: st.Cap, NextAction: "looping"}
		return st
	})
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	if snap.LoopState.Status == "running" {
		s.resumePendingLoopWork(parentRunID)
	}
	return result, nil
}

// resumeFlowWithFeedback is the BUG-231 "Continue" action: it answers the
// hub's escalate/cap-reached pause and lets the hub re-decide, rather than
// hard-routing anywhere itself (D-5). It:
//  1. clears the block — auto-raising the cap by ExtendBy only when the block
//     reason was the round cap (D-6); an escalate block needs no cap change,
//  2. settles the hub inline node back to RUNNING,
//  3. re-invokes the hub's synthesis turn with the user's feedback (if any)
//     embedded directly in the prompt, the same reliable-delivery pattern
//     CA-226/cohort-join notes use, instead of relying solely on
//     pendingAgentContext rendering.
//
// The hub then calls submit_review_outcome again and this same applyFlowControl
// routes the result: approved->done, changes_requested->coder, blocked->pause
// again (the desktop form reappears). No-op (returns the current snapshot,
// no error) if the loop is not currently blocked — safe to call more than once.
func (s *InteractiveService) resumeFlowWithFeedback(parentRunID, feedback string) (AgentGraphSnapshot, error) {
	s.mu.Lock()
	_, runExists := s.runs[parentRunID]
	s.mu.Unlock()
	if !runExists {
		return AgentGraphSnapshot{}, fmt.Errorf("resumeFlowWithFeedback: run %q not found", parentRunID)
	}

	feedback = strings.TrimSpace(feedback)
	wasBlocked := false
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status != "blocked" {
			return st
		}
		wasBlocked = true
		if st.BlockReason == "cap" {
			cap := effectiveCap(st)
			st.Cap = cap + effectiveExtendBy(st)
			st.RoundCap = st.Cap
			st.ExtendCount++
		}
		st.Status = "running"
		st.GateReason = ""
		st.BlockReason = ""
		return st
	})
	if !wasBlocked {
		return snap, nil
	}

	// BUG-233: settle the hub node before emitAgentGraph (not in a goroutine) —
	// emitAgentGraph fires the SSE event that triggers the desktop's step-runtime
	// refresh, which could otherwise read the store before this write landed and
	// render the pre-Continue (WAITING_USER_APPROVAL / stale-done) snapshot.
	if s.isFlowEngineDriven(parentRunID) {
		if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" {
			s.setFlowStepStatus(context.Background(), parentRunID, hubID, StepStatusRunning)
		}
	}
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)

	resumeNote := ""
	if feedback != "" {
		resumeNote = "[flow-engine] The flow was paused awaiting your input. User guidance:\n" + feedback +
			"\n\n---\n\nRe-evaluate with this guidance in mind, then call submit_review_outcome with your decision."
	}
	go s.maybeAutoReinvokeHubWithNote(parentRunID, resumeNote)
	return snap, nil
}

// buildCohortNote constructs the single consolidated pendingAgentContext note for a
// completed cohort.  Each member is labelled with its config-supplied label and provider.
// Failed members appear as "failed: <err>".
func buildCohortNote(parentRunID, cohortID string, entries []cohortEntry, round int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[flow-engine joined result note]\n")
	fmt.Fprintf(&b, "Flow round %d — %d results joined.\n", round, len(entries))
	for _, e := range entries {
		label := e.Label
		if label == "" {
			label = "agent"
		}
		if e.Status == "failed" {
			fmt.Fprintf(&b, "%q (%s): failed: %s\n", label, e.Provider, e.Err)
		} else {
			msg := strings.TrimSpace(e.FinalMessage)
			if msg == "" {
				msg = "(completed with no final message captured)"
			}
			fmt.Fprintf(&b, "%q (%s): %s\n", label, e.Provider, msg)
		}
	}
	b.WriteString("---\n")
	b.WriteString("This joined result note is part of your current prompt context.\n")
	b.WriteString("Synthesize: dedup the findings, flag any conflicting verdicts, resolve them " +
		"using the task context, then call the flow's control tool with the consolidated result.")
	return b.String()
}

// lastCohortNoteFor returns runID's most recently joined reviewer-cohort note
// (BUG-233), or "" if none has joined yet / the run doesn't exist.
func (s *InteractiveService) lastCohortNoteFor(runID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[runID]; rs != nil {
		return rs.lastCohortNote
	}
	return ""
}

// summarizeCohortNoteForUser extracts the per-reviewer findings lines from a
// buildCohortNote-shaped note, dropping the engine-internal header ("[flow-engine
// joined result note]", "Flow round N — ...") and the hub-only instructions after
// the "---" separator (BUG-233). Returns "" if note is empty or has no findings
// lines, so callers can fall back to other content.
func summarizeCohortNoteForUser(note string) string {
	if strings.TrimSpace(note) == "" {
		return ""
	}
	var findings []string
	for _, line := range strings.Split(note, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[flow-engine") || strings.HasPrefix(trimmed, "Flow round") {
			continue
		}
		if trimmed == "---" || strings.HasPrefix(trimmed, "This joined result note") || strings.HasPrefix(trimmed, "Synthesize:") {
			break
		}
		findings = append(findings, trimmed)
	}
	if len(findings) == 0 {
		return ""
	}
	return truncateDisplayField(strings.Join(findings, "\n"), 800)
}

// autoReinvokePromptText returns the minimal hub re-prompt used by maybeAutoReinvokeHub.
// The actual agent results are already in pendingAgentContext and get prepended by
// runTurn automatically; this text is the "user turn" trigger, not the content.
func autoReinvokePromptText() string {
	if prompt, ok, err := loadBuiltinPromptText("prompts/auto-reinvoke.md"); err == nil && ok {
		return prompt
	}
	return "[flow-engine] Agent results ready. Synthesize the join note above. You must call submit_review_outcome. Do not answer in prose only. If you cannot determine the result, call submit_review_outcome with status=blocked and feedback explaining why."
}

// maybeAutoReinvokeHub schedules exactly one hub turn after a cohort join, if and only if
// ALL guards pass (T-3 / Task-093 / CP-36 P-7):
//  1. autoOrchestrate == true on the parent run
//  2. Loop status ∉ {paused, stopped, blocked, done}
//  3. !parent.turnInFlight
//  4. !parent.reinvokeInFlight (single-flight guard, cleared when turn starts)
//  5. Round < effectiveCap(loopState)
//
// Safe to call while emitLocked holds s.mu — the function itself acquires s.mu at the
// start, so callers must NOT hold s.mu when calling directly; goroutine callers (go
// s.maybeAutoReinvokeHub) are the only valid pattern since emitLocked holds the lock.
func (s *InteractiveService) maybeAutoReinvokeHub(parentRunID string) {
	s.maybeAutoReinvokeHubWithNote(parentRunID, "")
}

// maybeAutoReinvokeHubWithNote is like maybeAutoReinvokeHub but embeds cohortNote
// directly into the synthesis turn prompt (BUG-synthesis-hang). The joined cohort
// result note was previously stored only in pendingAgentContext and rendered via
// composeAgentContextBlock, which is invisible to Codex agents that look for it as
// a visible user-turn message. Embedding it inline in the prompt guarantees the
// synthesizer always sees the note regardless of provider or adapter. When
// cohortNote is empty the prompt is identical to the plain autoReinvokePromptText().
func (s *InteractiveService) maybeAutoReinvokeHubWithNote(parentRunID, cohortNote string) {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil || !parent.autoOrchestrate || parent.reinvokeInFlight || parent.turnInFlight {
		if parent != nil && parent.autoOrchestrate && strings.TrimSpace(cohortNote) != "" {
			// A note-bearing cohort join can race with a previously scheduled empty
			// hub reinvoke. Do not drop the joined note just because the single-flight
			// guard is closed; the scheduled/current turn will either drain it, or a
			// deferred reinvoke below will consume it on the next turn.
			s.appendPendingAgentContextLocked(parentRunID, cohortNote)
		}
		// startTurn drains pendingAgentContext atomically with setting turnInFlight=true
		// (both under s.mu) before releasing the lock. So when we see turnInFlight=true
		// here, pendingAgentContext is only non-empty if NEW context arrived after
		// startTurn's unlock — which means the in-flight turn will NOT consume it.
		// Defer exactly one follow-up reinvoke in that case.
		if parent != nil && parent.autoOrchestrate && parent.turnInFlight &&
			!parent.reinvokeInFlight && len(parent.pendingAgentContext) > 0 {
			parent.pendingHubReinvoke = true
		}
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_reinvoke_deferred", "hub reinvoke was deferred or skipped by current state",
			"has_parent", parent != nil,
			"cohort_note_len", len(strings.TrimSpace(cohortNote)),
		)
		return
	}
	// Read loop state under o.mu (s.mu → o.mu is the established order; loopStateFor is safe here).
	st := s.agentOrchestrator.loopStateFor(parentRunID)
	switch st.Status {
	case "paused", "stopped", "blocked", "done":
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_reinvoke_blocked", "hub reinvoke blocked by loop status",
			"cohort_note_len", len(strings.TrimSpace(cohortNote)),
		)
		return
	}
	if st.Round >= effectiveCap(st) {
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_reinvoke_cap_blocked", "hub reinvoke blocked by round cap",
			"cohort_note_len", len(strings.TrimSpace(cohortNote)),
		)
		return
	}
	stepID := s.nextID("step")
	parent.reinvokeInFlight = true
	s.mu.Unlock()
	s.flowDiagLog(parentRunID, "hub_reinvoke_scheduled", "hub reinvoke scheduled",
		"step_id", stepID,
		"cohort_note_len", len(strings.TrimSpace(cohortNote)),
	)

	// Build the synthesis turn prompt. When a cohort note is provided, embed it
	// directly in the prompt so the synthesizer always sees it as a visible user
	// message — regardless of provider (Codex, Claude, Gemini) or YOLO mode.
	// The pendingAgentContext context-block path alone is insufficient: Codex
	// agents operating on resumed threads may not see prepended context blocks
	// as part of their visible conversation, causing the "joined result note not
	// present in visible context" block (BUG-synthesis-hang / BUG-review-feedback).
	prompt := autoReinvokePromptText()
	if strings.TrimSpace(cohortNote) != "" {
		prompt = cohortNote + "\n\n---\n\n" + prompt
	}
	s.scheduleChildTurn(parentRunID, stepID, prompt)
}

// buildCoderReentryPrompt composes the full prompt for coder re-entry from the
// FlowControlInput that came from submit_review_outcome. It includes the plain
// feedback text and a numbered issue list (title, severity, file, resolution)
// so the coder receives actionable details, not just the generic fallback.
func buildCoderReentryPrompt(in FlowControlInput) string {
	var sb strings.Builder
	if base, ok, err := loadBuiltinPromptText("prompts/coder-reentry.md"); err == nil && ok && base != "" {
		sb.WriteString(base)
		sb.WriteString("\n\n")
	}
	if fb := strings.TrimSpace(in.Summary); fb != "" {
		sb.WriteString(fb)
		sb.WriteString("\n\n")
	}
	writeIssue := func(i int, sev, title, file, resolution string) {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, sev, title))
		if file != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", file))
		}
		if resolution != "" {
			sb.WriteString(fmt.Sprintf("\n   Fix: %s", resolution))
		}
		sb.WriteString("\n")
	}
	if issues, ok := in.Payload["issues"]; ok {
		switch v := issues.(type) {
		case []ReviewIssue:
			if len(v) > 0 {
				sb.WriteString("Issues to address:\n")
				for i, issue := range v {
					writeIssue(i, issue.Severity, issue.Title, issue.File, issue.Resolution)
				}
			}
		case []any:
			if len(v) > 0 {
				sb.WriteString("Issues to address:\n")
				for i, raw := range v {
					m, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					sev, _ := m["severity"].(string)
					title, _ := m["title"].(string)
					file, _ := m["file"].(string)
					res, _ := m["resolution"].(string)
					writeIssue(i, sev, title, file, res)
				}
			}
		}
	}
	if sb.Len() == 0 {
		return "[flow-engine] Changes were requested on your last submission. Address the feedback and resubmit."
	}
	return strings.TrimSpace(sb.String())
}

func loadBuiltinPromptText(relPath string) (string, bool, error) {
	prompt, ok, err := agentpack.LoadBuiltinPrompt(relPath)
	if err != nil || !ok {
		return "", false, err
	}
	return strings.TrimSpace(prompt.Contents), true, nil
}

// parentHasTrackedFlow reports whether parentRunID is a run with a resolved
// flow's edges tracked on it (activeFlowEdges/activeFlowNodes, set by
// startResolvedFlow/startInlineEntryChain). Must be called with s.mu already
// held, matching every other call site in the EventTurnCompleted handler
// this is used from (BUG-NOTE-CP42 #17).
func parentHasTrackedFlow(s *InteractiveService, parentRunID string) bool {
	parent := s.runs[parentRunID]
	return parent != nil && len(parent.activeFlowEdges) > 0 && len(parent.activeFlowNodes) > 0
}

// advanceOrNotifyHub is the async continuation of a completed coder-like
// node (Task-180 follow-up): try to auto-spawn the flow's next node(s) from
// its own edge data via tryAdvanceFlowFromNode; if that's not applicable (no
// tracked flow, or the next step isn't a spawnable agent.delegate node — e.g.
// review-loop.yaml's "synthesis" node, which is the hub's own inline turn,
// not a spawned child), fall back to the original behavior: leave a
// completion note and reinvoke the hub so its own reasoning decides what
// happens next.
func (s *InteractiveService) advanceOrNotifyHub(parentRunID, completedLabel, completedAgentName, finalMsg string) {
	if s.tryAdvanceFlowFromNode(parentRunID, completedLabel, finalMsg) {
		return
	}
	note := fmt.Sprintf("[flow-engine] Coder %q completed. Result:\n%s",
		completedAgentName, truncateDisplayField(finalMsg, 2000))
	s.mu.Lock()
	s.appendPendingAgentContextLocked(parentRunID, note)
	s.mu.Unlock()
	s.maybeAutoReinvokeHub(parentRunID)
}

// maybeReinvokeCoderForContinue finds the child of parentRunID that a
// "continue" flow_control signal should reinvoke and schedules a new turn
// with the supplied prompt. Called from applyFlowControl when status ==
// "continue" so the synthesis→coder back-edge actually fires (CRITICAL
// finding: continue never restarted the coder).
//
// Task-180: for a run started from a resolved flowRef, the target is
// resolved from the flow's own edge data (activeFlowEdges) — matching a
// spawned child's label against the back-edge's target node id — instead of
// scanning for a role literally named "coder". A run with no tracked edges
// (the AI-driven spawn_agent path, which never sets activeFlowEdges) falls
// back to the original isCoderRun role match unchanged.
func (s *InteractiveService) maybeReinvokeCoderForContinue(parentRunID, prompt string) {
	if strings.TrimSpace(prompt) == "" {
		prompt = "[flow-engine] Changes were requested on your last submission. Address the feedback and resubmit."
	}
	s.mu.Lock()
	var targetNodeID string
	var targetNode agentpack.FlowNode
	var hasTargetNode bool
	var flowDriven bool
	if parent := s.runs[parentRunID]; parent != nil {
		targetNodeID, _ = resolveContinueBackEdgeTarget(parent.activeFlowEdges)
		if targetNodeID != "" {
			targetNode, hasTargetNode = findFlowNode(parent.activeFlowNodes, targetNodeID)
		}
		flowDriven = parent.flowEngineDriven
	}
	if hasTargetNode && !flowNodeReusesChild(targetNode) {
		s.mu.Unlock()
		agentName := flowNodeAgentName(targetNode)
		if agentName == "" {
			return
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent:            agentName,
			Prompt:           prompt,
			Wait:             false,
			Label:            targetNode.ID,
			AutoOrchestrate:  true,
			AgentDefOverride: agentDef,
			Model:            s.resolveFlowNodeModel(context.Background(), targetNode),
		}); err != nil {
			log.Printf("[flow-executor] continue: spawn node %q (agent %q) failed: %v", targetNode.ID, agentName, err)
			return
		}
		if flowDriven {
			s.setFlowStepStatus(context.Background(), parentRunID, targetNode.ID, StepStatusRunning)
			s.stampFlowNodePosture(context.Background(), parentRunID, targetNode)
		}
		return
	}
	s.mu.Unlock()
	// BUG-242: delegate to the single reinvoke implementation instead of a
	// second, older inline copy that never got the BUG-Rnd2 activationSeq/
	// EventAgentSpawnedByUser fixes — this is the actual code path a
	// review-loop round-2+ coder re-entry takes (the synthesis→coder back-edge
	// "continue"), so its coder reappeared miscategorized as "closed" in the
	// desktop with no new main-chat card even though the backend was already
	// running it again.
	s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
		if targetNodeID != "" {
			return child.label == targetNodeID
		}
		return isCoderRun(child)
	})
}

func isAgentRole(rs *interactiveRun, role string) bool {
	if rs == nil {
		return false
	}
	role = strings.ToLower(role)
	return strings.Contains(strings.ToLower(rs.agentName), role) || strings.Contains(strings.ToLower(rs.role), role)
}

// isCoderRun is the single named predicate for "is this run the coder agent"
// (CP-42/Task-180 T-3: isolate an unavoidable role-name compatibility check
// into one place instead of scattering the literal at each call site). It
// wraps isAgentRole rather than eliminating the role-name check entirely: a
// full elimination would mean resolving "which child corresponds to the
// synthesis→coder back-edge" from the active FlowDefinition's edges instead
// of the agent's declared role, which requires interactiveRun to carry a
// flow-node identity and a real generic executor walking FlowEdge data — that
// executor does not exist in the live run loop yet (ReviewLoopFlowConfig's
// FlowNode/FlowEdge values are only exercised by tests today). Until that
// executor exists, this is the narrowest safe compatibility shim: every
// "coder" role check in the live cohort/hub loop goes through this one
// function, so the domain-hardcode guard (domain_hardcode_guard_test.go) only
// has to track one occurrence instead of four.
func isCoderRun(rs *interactiveRun) bool {
	return isAgentRole(rs, "coder")
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

// loopAllowsNextTurnLocked reports whether parentRunID's loop may fire another
// turn (release a dependent child, resume a pending turn, re-enter the coder).
// BUG-234: this now also excludes "blocked" and "done" — a loop that is blocked
// awaiting the user, or already finished, must not auto-fire the next turn.
// Previously it excluded only "paused"/"stopped", so after an escalate settled
// the loop to "blocked", releaseDependentAgents still released dependent
// reviewers — one of the two runaway auto-advance paths that left the run
// spinning. When the user hits Continue, resumeFlowWithFeedback flips the loop
// back to "running", so normal progress resumes then. Delegates to
// loopIsAdvancing so the "which statuses are live" set has one definition.
func (s *InteractiveService) loopAllowsNextTurnLocked(parentRunID string) bool {
	return s.loopIsAdvancing(parentRunID)
}

// loopIsAdvancing reports whether the flow loop for parentRunID is still in a
// state that should auto-advance (spawn the next node, re-run the hub). It
// mirrors the gate maybeAutoReinvokeHub already applies: a loop that is paused,
// stopped, blocked, or done must NOT advance. BUG-234: the auto-advance paths
// (tryAdvanceFlowFromNode and the cohort-join reviewer-DONE/hub-RUNNING write)
// previously had NO status guard, so a child completion arriving after the loop
// had legitimately blocked (e.g. an escalate awaiting the user) kept re-spawning
// reviewers and flipping the hub node back to RUNNING — the synthesis step
// spun forever and the run never settled. maybeAutoReinvokeHub was gated, so the
// loop STATE was correct while the STEPS/spawns ran away; this closes that gap.
func (s *InteractiveService) loopIsAdvancing(parentRunID string) bool {
	switch s.agentOrchestrator.loopStateFor(parentRunID).Status {
	case "paused", "stopped", "blocked", "done":
		return false
	default:
		return true
	}
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
	isCohortMember := false
	trackedFlow := false
	s.mu.Lock()
	if completedRun := s.runs[completedRunID]; completedRun != nil && completedRun.flowCohortId != "" {
		isCohortMember = true
	}
	trackedFlow = parentHasTrackedFlow(s, parentRunID)
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
				RunID:         child.id,
				AgentName:     child.agentName,
				Role:          child.role,
				Status:        child.status,
				ParentRunID:   child.parentRunID,
				CreatedAt:     child.createdAt,
				DependsOn:     append([]string(nil), child.dependsOn...),
				AgentStatus:   child.agentStatus,
				ProviderKey:   string(child.providerKey),
				ModelName:     child.modelName,
				ActivationSeq: child.activationSeq, // BUG-242: preserve, don't silently reset to 0
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
	// Flow-graph runs route child completion through advanceOrNotifyHub. Do not
	// also schedule this dependency-release fallback, or the hub can synthesize
	// from a stale wait notice while the auto-spawned reviewer cohort is running.
	if !isCohortMember && !trackedFlow {
		go s.maybeAutoReinvokeHub(parentRunID)
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
		if cohortDiagEnabled() {
			runs := make([]string, len(snap.Runs))
			for i, r := range snap.Runs {
				runs[i] = fmt.Sprintf("%s(%s):%s", r.RunID, r.AgentName, r.Status)
			}
			cohortDiagLog("emitAgentGraph parent=%q loopStatus=%q loopRound=%d runs=%v",
				parentRunID, snap.LoopState.Status, snap.LoopState.Round, runs)
		}
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

// SetFlowDefinitionStore attaches the FlowDefinitionStore startResolvedFlow
// resolves flowRefs against (CP-42/Task-177). Optional: a nil store (the
// default) makes resolution fall back directly to the embedded agentpack,
// which built-in flows like Review Loop always resolve against anyway.
func (s *InteractiveService) SetFlowDefinitionStore(store FlowDefinitionStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flowDefinitionStore = store
}

func (s *InteractiveService) persistProviderSession(session ProviderSessionState) error {
	store := s.persistenceStore()
	if store == nil {
		return nil
	}
	return store.UpsertProviderSession(context.Background(), session)
}

// snapshotWithLoop returns a ProviderSessionState for rs that also includes the
// current orchestrator loop state.  Call for root (parent) runs only; child runs
// do not own a loop state so the field stays zero-valued.
// Caller must NOT hold s.mu (snapshotWithLoop acquires it internally).
func (s *InteractiveService) snapshotWithLoop(rs *interactiveRun) ProviderSessionState {
	s.mu.Lock()
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	if rs.parentRunID == "" {
		snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState
	}
	return snap
}

// persistParentSession persists the root run's full state (including loop) to
// sessions.ndjson.  Best-effort: errors are silently dropped.
func (s *InteractiveService) persistParentSession(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	snap.LoopState = s.agentOrchestrator.graphSnapshot(parentRunID).LoopState
	_ = s.persistProviderSession(snap)
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
		ChangeType:          rs.changeType,
		SourceDocID:         rs.sourceDocID,
		TurnCount:           rs.turnCount,
		PendingAgentContext: append([]string(nil), rs.pendingAgentContext...),
		AutoOrchestrate:     rs.autoOrchestrate,
		FlowCohortID:        rs.flowCohortId,
		ActiveFlowEdges:     append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...),
		ActiveFlowNodes:     append([]agentpack.FlowNode(nil), rs.activeFlowNodes...),
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
			// Cohort children always buffer regardless of wait mode; isolated note
			// path only applies to UI-initiated or wait=false spawns (BUG-122/BUG-126).
			if rs.flowCohortId != "" {
				s.agentOrchestrator.appendCohortResult(rs.parentRunID, rs.flowCohortId, cohortEntry{
					Label:        rs.label,
					Provider:     string(rs.providerKey),
					FinalMessage: truncateDisplayField(finalMsg, 1500),
					Status:       "completed",
				})
				s.flowDiagLog(rs.parentRunID, "cohort_member_completed", "cohort member completed and buffered",
					"child_run_id", rs.id,
					"cohort_id", rs.flowCohortId,
					"label", rs.label,
					"provider", string(rs.providerKey),
					"final_message_len", len(strings.TrimSpace(finalMsg)),
				)
				// BUG-234 (#1): settle THIS cohort member's own timeline node the moment
				// it finishes, so a done reviewer reads DONE while its sibling is still
				// RUNNING. Previously the reviewer nodes were only settled together at the
				// barrier below, so one-done-one-running showed both as RUNNING. rs.label
				// is the flow node id for a flow-spawned cohort member.
				if parent := s.runs[rs.parentRunID]; parent != nil && parent.flowEngineDriven && rs.label != "" {
					s.setFlowStepStatus(context.Background(), rs.parentRunID, rs.label, StepStatusDone)
					cohortDiagLog("member self-settled DONE parent=%q run=%q label=%q", rs.parentRunID, rs.id, rs.label)
				}
				if s.agentOrchestrator.cohortComplete(rs.parentRunID, rs.flowCohortId) {
					entries := s.agentOrchestrator.drainCohort(rs.parentRunID, rs.flowCohortId)
					note := buildCohortNote(rs.parentRunID, rs.flowCohortId, entries, s.agentOrchestrator.graphSnapshot(rs.parentRunID).LoopState.Round)
					s.flowDiagLog(rs.parentRunID, "cohort_join_complete", "cohort barrier completed and note built",
						"cohort_id", rs.flowCohortId,
						"entry_count", len(entries),
						"note_len", len(note),
					)
					cohortDiagLog("cohortNote built parent=%q cohort=%q entries=%d noteLen=%d", rs.parentRunID, rs.flowCohortId, len(entries), len(note))
					s.appendPendingAgentContextLocked(rs.parentRunID, note)
					if parent := s.runs[rs.parentRunID]; parent != nil {
						// BUG-233: retain the joined note past pendingAgentContext being
						// drained into the hub's synthesis turn, so the CA-226 fallback
						// (hub completes without calling submit_review_outcome) can build
						// a findings-based GateReason instead of an internal diagnostic one.
						parent.lastCohortNote = note
					}
					parentRunID := rs.parentRunID
					// BUG-174/BUG-181: the review cohort just joined — mark each reviewer
					// node DONE and the inline hub node RUNNING (synchronously, under s.mu
					// — see BUG-233 note below), THEN reinvoke the hub's synthesis turn in a
					// goroutine: the synthesis turn finalizes via markFlowRunComplete
					// (synthesis DONE), and running the step writes async here previously
					// let that race the step writes — landing before the reviewer-DONE
					// writes (synthesis DONE while reviewers still RUNNING) or after the
					// synthesis-RUNNING write (synthesis stuck RUNNING after the flow is
					// done). Sequencing the writes before the reinvoke removes both races.
					var reviewerNodeIDs []string
					var hubNodeID string
					flowDriven := false
					if parent := s.runs[parentRunID]; parent != nil && parent.flowEngineDriven {
						flowDriven = true
						for _, e := range entries {
							if e.Label != "" {
								reviewerNodeIDs = append(reviewerNodeIDs, e.Label)
							}
						}
						hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
					}
					// Capture the cohort note to embed directly in the synthesis prompt.
					// This fixes BUG-synthesis-hang: Codex agents on resumed threads do not
					// see the joined result note when it is only in pendingAgentContext
					// (rendered as a context block), causing the synthesizer to report
					// "joined result note not present in visible context" and stall.
					capturedCohortNote := note
					// BUG-233: keep the reviewer-DONE / hub-RUNNING step writes synchronous
					// (still under s.mu via emitLocked) instead of inside the goroutine below.
					// workflowStore.ApplyStepTransition uses its own lock, so this is safe here
					// and guarantees the writes land before this handler returns and any
					// subsequent agent_graph_updated event fires — otherwise the desktop's
					// step-runtime refresh could race ahead of these writes and render the
					// prior round's stale "all done"/RUNNING snapshot.
					if flowDriven {
						for _, id := range reviewerNodeIDs {
							s.setFlowStepStatus(context.Background(), parentRunID, id, StepStatusDone)
						}
						// BUG-234 (#4): only drive the hub node to RUNNING and re-invoke the
						// synthesis turn while the loop is still advancing. Once the loop has
						// settled (blocked/awaiting-user, done, stopped), a late or stray
						// cohort join must NOT flip the hub node back to RUNNING — that flap
						// is what left the synthesis step spinning forever after an escalate.
						// maybeAutoReinvokeHubWithNote is itself gated on loop status, but the
						// hub-RUNNING write below is not, so guard it here too.
						if hubNodeID != "" && s.loopIsAdvancing(parentRunID) {
							s.setFlowStepStatus(context.Background(), parentRunID, hubNodeID, StepStatusRunning)
						}
					}
					cohortDiagLog("scheduling hub reinvoke parent=%q flowDriven=%t loopAdvancing=%t reviewerNodeIDs=%v noteLen=%d",
						parentRunID, flowDriven, s.loopIsAdvancing(parentRunID), reviewerNodeIDs, len(capturedCohortNote))
					go s.maybeAutoReinvokeHubWithNote(parentRunID, capturedCohortNote)
				}
			} else if s.agentOrchestrator.loopMode(rs.parentRunID) == "explicit" && (isCoderRun(rs) || parentHasTrackedFlow(s, rs.parentRunID)) {
				// In explicit mode the hub drives all transitions. When a node completes
				// outside a cohort barrier (waitForResult=true, no flowCohortId), try to
				// auto-spawn the flow's next node(s) from its own edge data (Task-180
				// follow-up); dispatched async since tryAdvanceFlowFromNode/
				// maybeAutoReinvokeHub manage their own locking and must not run while
				// this handler still holds s.mu.
				//
				// BUG-NOTE-CP42 #17: gating this solely on isCoderRun(rs) meant a flow
				// whose entry delegate node uses a non-"coder" agent/role name (any
				// user-authored flow, not just review-loop/rag-harness) never reached
				// tryAdvanceFlowFromNode at all — its completion fell through to the
				// generic "Sub-agent completed" note below, which never reinvokes the
				// hub, silently stalling the flow. parentHasTrackedFlow additionally
				// covers any run whose parent is a flowRef-tracked hub, regardless of
				// role name; tryAdvanceFlowFromNode's own internal checks (activeFlowEdges
				// set, a forward edge exists from this node) still safely no-op and fall
				// back to the same legacy note+reinvoke path for anything it doesn't
				// recognize, so this only adds coverage, it narrows nothing.
				parentRunID := rs.parentRunID
				completedLabel := rs.label
				completedAgentName := rs.agentName
				msg := finalMsg
				go s.advanceOrNotifyHub(parentRunID, completedLabel, completedAgentName, msg)
			} else if rs.uiInitiated || !rs.waitForResult {
				s.appendPendingAgentContextLocked(rs.parentRunID, fmt.Sprintf(
					"Sub-agent %q (provider: %s) completed. Result: %s",
					rs.agentName, rs.providerKey, truncateDisplayField(finalMsg, 2000)))
			}
			// Legacy keyword-mode loop: in explicit mode the hub drives all transitions via
			// submit_review_outcome → flow_control, so this branch must stay silent. (Task-091 T-5)
			if s.agentOrchestrator.loopMode(rs.parentRunID) != "explicit" {
				switch {
				case isCoderRun(rs):
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
							if child := s.runs[childID]; isCoderRun(child) {
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
			} // end if loopMode != "explicit"
		}
	case EventTurnFailed:
		rs.status = RunStatusFailed
		rs.agentStatus = string(RunStatusFailed)
		s.agentOrchestrator.signalChild(rs.id, "", true, ev.Error, RunStatusFailed)
		if rs.parentRunID != "" {
			if rs.flowCohortId != "" {
				s.agentOrchestrator.appendCohortResult(rs.parentRunID, rs.flowCohortId, cohortEntry{
					Label:    rs.label,
					Provider: string(rs.providerKey),
					Status:   "failed",
					Err:      truncateDisplayField(ev.Error, 500),
				})
				s.flowDiagLog(rs.parentRunID, "cohort_member_failed", "cohort member failed and buffered",
					"child_run_id", rs.id,
					"cohort_id", rs.flowCohortId,
					"label", rs.label,
					"provider", string(rs.providerKey),
					"error", truncateDisplayField(ev.Error, 500),
				)
				// BUG-234 (#1): settle this member's own node to FAILED on its own
				// failure, mirroring the completed path, so a failed reviewer reads
				// FAILED immediately instead of RUNNING until the barrier.
				if parent := s.runs[rs.parentRunID]; parent != nil && parent.flowEngineDriven && rs.label != "" {
					s.setFlowStepStatus(context.Background(), rs.parentRunID, rs.label, StepStatusFailed)
					cohortDiagLog("member self-settled FAILED parent=%q run=%q label=%q", rs.parentRunID, rs.id, rs.label)
				}
				if s.agentOrchestrator.cohortComplete(rs.parentRunID, rs.flowCohortId) {
					entries := s.agentOrchestrator.drainCohort(rs.parentRunID, rs.flowCohortId)
					note := buildCohortNote(rs.parentRunID, rs.flowCohortId, entries, s.agentOrchestrator.graphSnapshot(rs.parentRunID).LoopState.Round)
					s.flowDiagLog(rs.parentRunID, "cohort_join_complete_after_failure", "cohort barrier completed after member failure",
						"cohort_id", rs.flowCohortId,
						"entry_count", len(entries),
						"note_len", len(note),
					)
					s.appendPendingAgentContextLocked(rs.parentRunID, note)
					if parent := s.runs[rs.parentRunID]; parent != nil {
						parent.lastCohortNote = note // BUG-233: retained for the CA-226 fallback GateReason
					}
					parentRunID := rs.parentRunID
					capturedCohortNote := note // embed note directly in synthesis prompt (BUG-synthesis-hang)
					go s.maybeAutoReinvokeHubWithNote(parentRunID, capturedCohortNote)
				}
			} else if rs.uiInitiated || !rs.waitForResult {
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
			// BUG-242: this generic per-event summary write used to omit
			// ActivationSeq entirely, so the very next turn-progress event after
			// a reinvokeMatchingFlowChild reactivation (e.g. the reinvoked
			// child's own completion) silently reset the reported activationSeq
			// back to 0 — undermining the desktop's monotonic terminal-status
			// guard (mergeAgentRunsById), which relies on activationSeq only
			// ever increasing to recognize a genuine reinvoke.
			ActivationSeq: rs.activationSeq,
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
	//
	// Denylist wins over everything (including a user-remembered rule), so it is
	// evaluated first.
	outcome := s.policy.Decide(details)
	if outcome == PolicyAutoDeny {
		s.recordAutoApproval(b.rs, details, "deny", "policy_denylist")
		return "deny", nil
	}

	// BUG-246: a shell command the user previously chose "don't ask again" for
	// auto-approves without a card. Provider-neutral — both Claude and Codex route
	// shell approvals through here. Only "exec" approvals are eligible, and a
	// compound command never matches (matchesApprovalRule rejects it), so a
	// remembered `git status` never green-lights `git status && rm -rf /`.
	if details.Kind == "exec" && b.rs.workspaceCwd != "" {
		dotFP := filepath.Join(b.rs.workspaceCwd, ".flowpilot")
		if matchesApprovalRule(details.Command, readApprovalAllowRules(dotFP)) {
			s.recordAutoApproval(b.rs, details, "approve", "policy_user_remembered")
			return "approve", nil
		}
	}

	if outcome == PolicyAutoApprove {
		s.recordAutoApproval(b.rs, details, "approve", "policy_allowlist")
		return "approve", nil
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
	// A sub-agent's approval is emitted on its own run stream and surfaced when the
	// user focuses that agent. (BUG-177 briefly mirrored it onto the hub stream so
	// concurrent cohort approvals showed on main, but that flooded the main view
	// with unresolved approvals and was reverted per user direction — sub-agent
	// approvals stay in the agent view; YOLO covers the hands-off UX.)
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

// SubmitFlowControl advances the generic flow engine on the controlling hub run.
// When called from a child turn (synthesizer, reviewer), b.rs.parentRunID is
// the hub run that owns the loop state; route there instead of the child.
//
// BUG-176: enforce the cohort-join barrier in code. A cohort member (e.g. a
// review-loop reviewer, flowCohortId != "") must NOT drive the flow's control
// tool: routing its call straight to the parent's applyFlowControl let a single
// reviewer terminate/advance the whole round before its siblings finished
// (observed as "synthesis marked done while the other reviewer is still
// running"). Only the hub's own synthesis turn — which runs after the cohort
// joins and is NOT a cohort member — may finalize the flow. A cohort member's
// call is rejected with guidance so its findings flow through its final message
// into the cohort note the hub synthesizes, exactly as the auto-spawn prompt
// already instructs (BUG-NOTE-CP42 #13). Non-cohort children (flowCohortId ==
// "") and the hub itself are unaffected.
func (b *turnBridge) SubmitFlowControl(in FlowControlInput) (FlowControlResult, error) {
	if b.rs.flowCohortId != "" {
		return FlowControlResult{}, fmt.Errorf(
			"this is a cohort review step, not the hub: do not call the flow control tool here. " +
				"Report your findings (approve or request changes, with specifics) in your final message; " +
				"the hub will synthesize the full cohort and finalize the flow after every reviewer has finished")
	}
	targetRunID := b.rs.id
	if b.rs.parentRunID != "" {
		targetRunID = b.rs.parentRunID
	}
	return b.svc.applyFlowControl(targetRunID, in)
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
	s.flowDiagLog(parentRunID, "child_spawn_requested", "child spawn requested",
		"agent", in.Agent,
		"provider", in.Provider,
		"wait", in.Wait,
		"depends_on_count", len(in.DependsOn),
		"flow_cohort_id", in.FlowCohortID,
		"cohort_size", in.CohortSize,
		"label", in.Label,
		"auto_orchestrate", in.AutoOrchestrate,
		"model_override", in.Model,
		"prompt_len", len(in.Prompt),
	)
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
		s.flowDiagLog(parentRunID, "child_spawn_missing_parent", "child spawn parent run not found",
			"agent", in.Agent,
		)
		return SpawnAgentResult{}, fmt.Errorf("parent run %q not found", parentRunID)
	}
	if in.AgentDefOverride != nil {
		agentDef = in.AgentDefOverride
	} else if defs := s.agentCatalog.listAgents(cwd); len(defs) > 0 {
		for i := range defs {
			if strings.EqualFold(defs[i].Name, in.Agent) {
				def := defs[i]
				agentDef = &def
				break
			}
		}
	}

	// Determine provider. BUG-228: in.Model — a flow node's own step-configured
	// model, set by the flow executor for an agent.delegate node with a
	// purpose-named step_definitions row — is authoritative over any
	// explicit/inherited provider, the same "model wins" pattern BUG-171
	// established for the run's own launch. Otherwise: explicit input > agent
	// definition > parent run's provider.
	providerKey := ProviderKey(in.Provider)
	if pk, ok := providerKeyFromModel(in.Model); in.Model != "" && ok {
		providerKey = pk
	} else if providerKey == "" && agentDef != nil && agentDef.Provider != "" {
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
	// Priority: BUG-228's in.Model (a flow node's own step-configured model) >
	// agent definition > same-provider inheritance > per-provider default.
	// When the child runs on a different provider than the parent, the parent's model
	// name is invalid for the child (e.g. "sonnet" sent to Codex → 400).
	childModel := parentModel
	childReasoningEffort := parentReasoningEffort
	if in.Model != "" {
		childModel = in.Model
	} else if agentDef != nil && agentDef.Model != "" {
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
		rs.flowCohortId = in.FlowCohortID
		if rs.flowCohortId != "" {
			if in.CohortSize > 0 {
				s.agentOrchestrator.preRegisterCohort(parentRunID, rs.flowCohortId, in.CohortSize)
			} else {
				s.agentOrchestrator.registerCohortMember(parentRunID, rs.flowCohortId)
			}
		}
		if in.AutoOrchestrate {
			if parent := s.runs[parentRunID]; parent != nil {
				parent.autoOrchestrate = true
			}
			// Set explicit mode so the legacy keyword gate stays silent for this
			// review flow (CP-36 P-7 / CRITICAL finding: mode never wired).
			s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
				st.Mode = "explicit"
				return st
			})
		}
		if agentDef != nil {
			rs.agentName = agentDef.Name
			rs.role = agentDef.Role
		} else {
			rs.agentName = in.Agent
			rs.role = strings.ToLower(in.Agent)
		}
		rs.label = in.Label
		if rs.label == "" {
			rs.label = rs.agentName
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
			s.flowDiagLog(parentRunID, "child_spawn_persist_failed", "child run snapshot persistence failed",
				"child_run_id", childSnap.RunID,
				"error", err.Error(),
			)
			return SpawnAgentResult{}, err
		}
	}

	// [BUG-113 diag] The minted child run + resolved identity. Pair this with the
	// "[agent-spawn] request" line above to map each spawn call to its child run id.
	log.Printf("[agent-spawn] child created parent=%q child=%q agent=%q role=%q provider=%q model=%q blockedStart=%t",
		parentRunID, handle.RunID, childSnap.AgentName, childSnap.Role, childSnap.ProviderKey, childModel, blockedStart)
	s.flowDiagLog(parentRunID, "child_spawn_created", "child run created",
		"child_run_id", handle.RunID,
		"agent_name", childSnap.AgentName,
		"agent_role", childSnap.Role,
		"provider", string(childSnap.ProviderKey),
		"model", childModel,
		"blocked_start", blockedStart,
		"agent_status", agentStatus,
	)

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
	if note := strings.TrimSpace(in.ParentContextNote); note != "" {
		s.appendPendingAgentContext(parentRunID, note)
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

func (s *InteractiveService) runTurn(ctx context.Context, rs *interactiveRun, adapter ProviderRuntimeAdapter, in TurnInput, scenario, turnID string, capturedCtx []string) {
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
	// BUG-179: only offer the flow control tool (submit_review_outcome) on the
	// hub's post-join synthesis turn, never its first turn or a child's turn.
	// spawnChildRun sets the hub's autoOrchestrate=true as soon as the entry coder
	// is spawned (from startResolvedFlow's goroutine), so gating solely on
	// autoOrchestrate handed the tool to the hub's *first* model turn — letting it
	// call flow_control("done") before the reviewer cohort had even run, which
	// marked synthesis DONE while the reviewers were still RUNNING. The hub's own
	// turns are the only ones that may finalize (parentRunID == ""), and only after
	// its first turn — the coder spawns on turn 1, and maybeAutoReinvokeHub only
	// reinvokes the hub (turn ≥ 2) after the cohort join, so turnCount > 1 is the
	// genuine synthesis turn. (A cohort reviewer is additionally blocked in
	// SubmitFlowControl by flowCohortId, BUG-176.)
	offerReviewOutcomeTool := rs.autoOrchestrate && rs.parentRunID == "" && rs.turnCount > 1
	providerPrompt = prependModePrefix(providerPrompt, rs.turnCount, rs.changeType, rs.sourceDocID)
	// Persist the per-turn YOLO posture as the run's current default (BUG-129). The UI
	// toggle is sticky, so an explicit YoloMode this turn must update rs.yolo; otherwise a
	// child spawned during this turn (spawnChildRun reads parentRun.yolo) would inherit the
	// stale run-level default instead of the posture the user actually has enabled.
	if in.YoloMode != nil {
		rs.yolo = yolo
	}
	// pendingAgentContext was drained into capturedCtx by startTurn (atomically with
	// turnInFlight=true) so rs.pendingAgentContext is already nil here.
	if len(capturedCtx) > 0 {
		providerPrompt = composeAgentContextBlock(capturedCtx) + "\n\n" + in.Prompt
	}
	s.mu.Unlock()
	// Live ledger refresh (CP-35): pick up commits made during this session so the
	// oracle always sees the current change history, not just what existed at bind time.
	s.rebuildLedgerIfDirty(rs.workspaceCwd)
	// Flow Mode: prepend FlowContextPackage for Coding steps before normal feature
	// history injection. injectFeatureHistoryPrompt skips re-injection when the prompt
	// already carries the flowContextHandoffPrefix sentinel (Task-169).
	providerPrompt = s.injectFlowContextIfCoding(ctx, rs, in.StepID, in.Prompt, providerPrompt)
	if s.shouldInjectFeatureHistory(rs.providerKey) {
		providerPrompt = injectFeatureHistoryPrompt(rs.workspaceCwd, providerPrompt, transcriptTurnsFromRun(rs))
	}
	// Observability for E2E: persist/log the fully-composed turn prompt (feature
	// history + discussion + mode prefix + user text) under the FlowPilot tool
	// workspace (namespaced by project id), NOT inside the target project. On by
	// default; disable with FLOWPILOT_LOG_PROMPT=0. The adapter prepends skill
	// content downstream; this captures everything the injection seam produced.
	toolWorkspace := ""
	if s.runner != nil {
		toolWorkspace = s.runner.workspace
	}
	logComposedPrompt(toolWorkspace, rs.projectID, rs.id, turnID, providerPrompt)
	req := TurnRequest{
		RunID:                  rs.id,
		StepID:                 in.StepID,
		ProjectID:              rs.projectID,
		ProviderSessionID:      providerSessionID,
		ProviderTurnID:         turnID,
		Prompt:                 providerPrompt,
		ModelName:              model,
		SelectedSkills:         in.SelectedSkills,
		YoloMode:               yolo,
		ReasoningEffort:        effort,
		Cwd:                    rs.workspaceCwd,
		Scenario:               scenario,
		Attachments:            in.Attachments,
		OfferReviewOutcomeTool: offerReviewOutcomeTool,
	}
	// CP-35: snapshot HEAD and seed test baseline before the AI runs.
	// Both must happen before sendTurnWithRetry so the gate sees pre-change state.
	if head, headErr := captureGitHead(rs.workspaceCwd); headErr == nil {
		s.mu.Lock()
		rs.turnStartGitHead = head
		s.mu.Unlock()
	}
	s.ensureBaseline(rs.workspaceCwd)
	bridge := &turnBridge{svc: s, rs: rs, ctx: ctx, turnID: turnID, yolo: yolo}
	err := s.sendTurnWithRetry(ctx, adapter, req, bridge)
	if err == nil && !rs.flowEngineDriven {
		// The provider turn owns interactive UX/events; once it returns cleanly we
		// advance the shared workflow planner so the live run path no longer bypasses
		// WorkflowStore/WorkflowOrchestrator entirely. A2 will replace the fake
		// backing store and make this durable/auditable.
		//
		// BUG-174: skip this for flow-engine-driven runs. PlanWorkflowProgress
		// walks every step and bulk-marks them DONE in one post-turn pass, which
		// is exactly what made a Flow-Mode run flip all steps to DONE out of
		// order after the hub's turn. For these runs the flow executor emits the
		// real per-node transitions instead (flow_step_runtime.go).
		_, _ = s.orchestrator.Progress(ctx, rs.id, yolo)
	}

	completed, fin := s.finishTurn(rs, turnID, err)

	// BUG-226: the hub synthesis turn must finish by calling submit_review_outcome.
	// A prose-only answer leaves the flow engine without a terminal transition, so
	// conservatively escalate and settle the inline hub step instead of leaving it RUNNING.
	if completed && offerReviewOutcomeTool && rs.parentRunID == "" && rs.flowEngineDriven && !s.flowControlSubmittedForTurn(rs.id, turnID) {
		log.Printf("[flow-step] hub synthesis turn %q completed without submit_review_outcome, escalating", turnID)
		// BUG-233: the awaiting-user card renders this Summary verbatim as
		// GateReason, so prefer a concise summary of what the reviewers actually
		// found over the hub's raw prose or an internal diagnostic sentence.
		summary := "Hub synthesis turn completed without calling submit_review_outcome. Final message: " + fin.FinalMessage
		if findings := summarizeCohortNoteForUser(s.lastCohortNoteFor(rs.id)); findings != "" {
			summary = "Reviewers reported:\n" + findings
		}
		_, _ = s.applyFlowControl(rs.id, FlowControlInput{
			Status:  "escalate",
			Summary: summary,
		})
		// BUG-231/BUG-233: applyFlowControl's "escalate" case above already settles
		// the hub node to WAITING_USER_APPROVAL — do not override it to FAILED here.
		// FAILED reads as a terminal error, not the intended non-terminal
		// awaiting-user pause this fallback triggers.
	}

	// Persist settled state (status, lastMessage, updatedAt) for history survival
	// across restarts (BUG-080 F-3). Take a snapshot under lock; persist outside.
	s.mu.Lock()
	newCodexSessionID := s.refreshResumeHandleLocked(rs, adapter)
	geminiTurn := transcriptTurn{}
	if rs.providerKey == ProviderKeyGemini && completed {
		geminiTurn = transcriptTurnForProviderTurnLocked(rs, turnID)
	}
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
	if strings.TrimSpace(geminiTurn.User) != "" || strings.TrimSpace(geminiTurn.Assistant) != "" {
		if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
			_ = logger.AppendTurnLog(context.Background(), rs.id, turnLogLine{
				Kind:      turnLogKindTranscriptTurn,
				TurnID:    turnID,
				Prompt:    geminiTurn.User,
				Assistant: geminiTurn.Assistant,
			})
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
	// Retry a hub reinvoke that was deferred because turnInFlight was true when the
	// coder completion fired. This prevents the review loop from stalling when the
	// coder finishes before the hub's current turn has cleared.
	pendingHubReinvoke := rs.parentRunID == "" && rs.pendingHubReinvoke
	if pendingHubReinvoke {
		rs.pendingHubReinvoke = false
	}
	s.mu.Unlock()

	if pendingHubReinvoke {
		go s.maybeAutoReinvokeHub(rs.id)
	}

	// Post-turn flow gate (CP-35 P-4/P-5): observe diff, evaluate rules, enforce.
	// Non-fatal: any internal error inside runFlowGate degrades to pass.
	// Child agent runs (coder, reviewer) are exempt: CA note enforcement is the
	// hub/root run's responsibility. Gating child turns causes false violations
	// because children make code changes but never write CA notes. (BUG-152)
	if completed && rs.parentRunID == "" {
		if s.runFlowGate(ctx, rs, turnID, fin) {
			completed = false
		}
	}

	// Finalizer hook runs OUTSIDE s.mu and only on a clean completion. A finalize
	// failure is recorded as retryable and must not erase the completed turn (04-04).
	if completed {
		_ = s.finalizer.Finalize(fin)
		// Rolling chat summary is no longer produced per turn; arm the idle timer
		// so it runs once the chat has been quiet for the idle window. A new turn
		// cancels this (see startTurn), restarting the window from zero.
		s.mu.Lock()
		rs.lastTurnID = turnID
		s.mu.Unlock()
		s.scheduleChatSummary(rs.id)
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

func (s *InteractiveService) shouldInjectFeatureHistory(providerKey ProviderKey) bool {
	switch providerKey {
	case ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGemini:
		return true
	default:
		return false
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
	if errors.Is(err, errGeminiWorkspaceRequired) {
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
	if rs.resumedFromDisk && rs.providerKey == ProviderKeyGemini {
		if live, ok := adapter.(*geminiAdapter); ok && rs.realProviderSessionID != "" {
			scopeKey := strings.TrimSpace(rs.providerAccountID)
			if scopeKey == "" {
				scopeKey = strings.TrimSpace(live.scopeKey)
			}
			live.sessions.setRealSession(scopeKey, rs.providerSessionID, rs.realProviderSessionID)
			live.sessions.setRealSession(scopeKey, rs.realProviderSessionID, rs.realProviderSessionID)
			if scopeKey != strings.TrimSpace(live.scopeKey) {
				live.sessions.setRealSession(live.scopeKey, rs.providerSessionID, rs.realProviderSessionID)
				live.sessions.setRealSession(live.scopeKey, rs.realProviderSessionID, rs.realProviderSessionID)
			}
		}
	}

	turnID := s.nextID("turn")
	rs.turnInFlight = true
	rs.reinvokeInFlight = false // the turn the reinvoke scheduled is now in flight
	flowStartOnly := false
	// A new turn resets the idle-summary window to zero (a pending summary timer
	// is cancelled here and re-armed when this turn completes).
	s.cancelChatSummary(rs.id)
	if rs.turnCount == 0 {
		if changeType := normalizeChangeType(in.ChangeType); changeType != "" {
			rs.changeType = changeType
		}
		rs.sourceDocID = resolveSourceDocID(rs.workspaceCwd, rs.changeType, in.SourceDocID)
		// CP-42/Task-177: a validated flowRef on the first turn starts the
		// built-in flow's entry node(s) deterministically instead of relying on
		// the hub's own AI judgement to decide whether to spawn a review loop.
		// The hub's first provider turn is suppressed; it will be reinvoked only
		// after the flow reaches a hub.inline node. Scheduled async because
		// spawnChildRun manages its own locking and must not run while this
		// function still holds s.mu.
		//
		// BUG-NOTE (Chat Mode Review Loop): flowEngineDriven is set HERE,
		// inline (not via the markFlowEngineDriven helper, which would
		// deadlock on s.mu — already held across this whole function), so
		// every caller that reaches this branch is covered by construction.
		// It used to be set only by handleStartTurn's separate
		// resolveWorkflowFlowRef branch (the Flow-Mode workflow-picker path),
		// so a Chat Mode "bug" sub-mode launch — which sends flowRef directly
		// and never goes through that branch — spawned its flow's entry node
		// correctly but never got flagged flow-engine-driven. That silently
		// disabled every isFlowEngineDriven-gated behavior for Chat Mode: the
		// legacy bulk step planner kept running alongside the executor
		// (BUG-174's fix, undone for chat), the BUG-226 no-tool-call escalation
		// safety net never fired, and BUG-234's per-node step settlement was
		// skipped. Both the workflowID-resolved and explicit chat flowRef
		// paths set in.FlowRef before calling startTurn, so flagging it here
		// once covers both.
		if flowRef := strings.TrimSpace(in.FlowRef); flowRef != "" {
			flowStartOnly = true
			rs.flowEngineDriven = true
			go s.startResolvedFlow(context.Background(), runID, flowRef, in.Prompt)
		}
	}
	rs.turnCount++
	rs.lastTurnStepID = in.StepID // CP-35: remember for gate reprompts
	rs.currentTurnID = turnID
	// BUG-235: lastPrompt is the history list's title source (Navigator.tsx renders
	// runTitle(item.lastPrompt || item.lastMessage)). Every internal flow-engine-
	// generated turn — the hub auto-reinvoke ("[flow-engine] Agent results ready...",
	// autoReinvokePromptText), the coder re-entry, and the Continue-resume note — is
	// prefixed "[flow-engine]" and used to overwrite it unconditionally, so a flow
	// run's history entry ended up titled by whichever internal prompt ran last
	// instead of the user's original request. Skip the overwrite for these internal
	// prompts (unless lastPrompt is still empty, so a run always has SOME title).
	if p := strings.TrimSpace(in.Prompt); !strings.HasPrefix(p, "[flow-engine]") || rs.lastPrompt == "" {
		rs.lastPrompt = truncateDisplayField(in.Prompt, 100)
	}
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
	isParent := rs.parentRunID == ""
	// Drain pendingAgentContext atomically with turnInFlight=true so that any concurrent
	// maybeAutoReinvokeHub call sees an empty slice after this unlock and does not set
	// pendingHubReinvoke spuriously. runTurn receives the captured slice directly.
	capturedCtx := rs.pendingAgentContext
	rs.pendingAgentContext = nil
	s.mu.Unlock()
	if isParent {
		snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState
	}
	_ = s.persistProviderSession(snap) // BUG-080 F-3: persist outside lock, best-effort
	// Persist raw user prompt for transcript replay (BUG-083 F-1): the provider
	// session file records the composed prompt (raw + reinforcement + skill preamble),
	// so we keep the raw input separately and prefer it on resume.
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		_ = logger.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, TurnID: turnID, Prompt: in.Prompt})
	}
	if flowStartOnly {
		var snap ProviderSessionState
		s.mu.Lock()
		if current := s.runs[runID]; current != nil {
			// Flow-start handoff suppresses the hub's provider turn, but the desktop
			// still opened a live turn stream for this providerTurnId. Emit a terminal
			// event so the client can settle the synthetic turn and begin the separate
			// orchestration stream that carries later hub/agent updates.
			s.emitLocked(current, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: ""})
			current.turnInFlight = false
			current.currentTurnID = ""
			current.turnCancel = nil
			current.lastTurnID = turnID
			snap = sessionStateOf(current)
		}
		s.mu.Unlock()
		if snap.RunID != "" {
			_ = s.persistProviderSession(snap)
		}
		cancel()
		return turnID, nil
	}
	// Flow Mode: clear cached FlowContextPackage when a Plan step reruns so the next
	// Coding step rebuilds the package from the new Plan output (T-3, Task-169).
	s.maybeClearPlanContextForPlanStep(ctx, runID, rs, in.StepID)

	go s.runTurn(ctx, rs, adapter, in, scenario, turnID, capturedCtx)
	return turnID, nil
}

func transcriptTurnForProviderTurnLocked(rs *interactiveRun, turnID string) transcriptTurn {
	if rs == nil {
		return transcriptTurn{}
	}
	turn := transcriptTurn{}
	for i := len(rs.events) - 1; i >= 0; i-- {
		ev := rs.events[i]
		if turnID != "" && ev.ProviderTurnID != turnID {
			continue
		}
		if turn.Assistant == "" && ev.Type == EventMessageCompleted && strings.TrimSpace(ev.Text) != "" {
			turn.Assistant = ev.Text
		}
		if turn.User == "" && ev.Type == EventTurnStarted && strings.TrimSpace(ev.Prompt) != "" {
			turn.User = ev.Prompt
		}
	}
	if turn.Assistant == "" {
		for i := len(rs.events) - 1; i >= 0; i-- {
			ev := rs.events[i]
			if turnID != "" && ev.ProviderTurnID != turnID {
				continue
			}
			if ev.Type == EventTurnCompleted && strings.TrimSpace(ev.FinalMessage) != "" {
				turn.Assistant = ev.FinalMessage
				break
			}
		}
	}
	return turn
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
	case ProviderKeyGemini:
		live, ok := adapter.(*geminiAdapter)
		if !ok {
			return ""
		}
		scopeKey := strings.TrimSpace(rs.providerAccountID)
		if scopeKey == "" {
			scopeKey = strings.TrimSpace(live.scopeKey)
		}
		real := live.sessions.realSession(scopeKey, rs.providerSessionID)
		if real == "" {
			real = live.sessions.realSession(scopeKey, rs.realProviderSessionID)
		}
		if real == "" && scopeKey != strings.TrimSpace(live.scopeKey) {
			real = live.sessions.realSession(live.scopeKey, rs.providerSessionID)
			if real == "" {
				real = live.sessions.realSession(live.scopeKey, rs.realProviderSessionID)
			}
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
	return s.submitApprovalDecision(approvalID, decision, false)
}

// submitApprovalDecision resolves an approval card. When remember==true and the
// user approved a shell command, it persists an executable+subcommand rule to
// the project's approval-allowlist so the same command auto-approves next time
// (BUG-246). Compound commands and non-exec approvals are never remembered.
func (s *InteractiveService) submitApprovalDecision(approvalID, decision string, remember bool) *apiErr {
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
	details := rec.details
	var rememberCwd string
	if rs := s.runs[rec.runID]; rs != nil {
		if rs.pendingApprovalID == approvalID {
			rs.pendingApprovalID = ""
		}
		rememberCwd = rs.workspaceCwd
	}
	s.mu.Unlock()

	// Persist the "don't ask again" rule outside the lock (file IO). Only shell
	// commands the user actually approved are eligible; deriveApprovalRule
	// rejects compound commands.
	if remember && decision == "approve" && details.Kind == "exec" && rememberCwd != "" {
		if rule, ok := deriveApprovalRule(details.Command); ok {
			_ = addApprovalAllowRule(filepath.Join(rememberCwd, ".flowpilot"), rule)
		}
	}

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
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	var cancels []context.CancelFunc
	if rs.turnCancel != nil {
		cancels = append(cancels, rs.turnCancel)
	}
	if rs.parentRunID == "" {
		for _, childID := range s.agentOrchestrator.listChildren(runID) {
			if child := s.runs[childID]; child != nil && child.turnInFlight && child.turnCancel != nil {
				cancels = append(cancels, child.turnCancel)
			}
		}
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
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

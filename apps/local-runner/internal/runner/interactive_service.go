package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/flowgate"
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
	// Chat SSOT capture stack (CP-59 / SD-26). Always ON on dev branch; lazily
	// initialized by ensureChatTranscriptWriter. Zero values valid before init.
	chatOnce        sync.Once
	chatRuns        *chatRunRegistry
	chatTranscripts *chatTranscriptWriter
	// chatSwitchInFlight guards one in-flight provider switch per chat
	// (SD26-S-2); guarded by s.mu, lazily initialized.
	chatSwitchInFlight map[string]bool
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
	// gitnexusAnalyzeOnce tracks workspaces where an auto `gitnexus analyze`
	// was already kicked off this process (see ensureGitNexusIndexAsync).
	gitnexusAnalyzeOnce map[string]bool

	// dispatchLogSyncMu guards dispatchLogSyncHash, kept separate from the main
	// s.mu since a Drive upload is slow network I/O unrelated to run-state locking.
	// dispatchLogSyncHash caches the sha256 of the last successfully-uploaded
	// per-project dispatch.ndjson so a chat-sync batch (which calls the per-run
	// sync handler once per run, each of which re-exports+re-uploads the whole
	// project-wide dispatch log) uploads it once instead of once per run.
	dispatchLogSyncMu   sync.Mutex
	dispatchLogSyncHash map[string]string

	// chatSessionIndexMu guards chatSessionIndexLocks and chatSessionIndexRepairing.
	// Each project gets its own mutex covering Drive sessions.ndjson
	// read-merge-write so concurrent syncChatRunToDrive / list reconcile cannot
	// last-write-wins the index. chatSessionIndexRepairing debounces the
	// CA-555 background discover so repeated list calls do not stack walks.
	chatSessionIndexMu        sync.Mutex
	chatSessionIndexLocks     map[string]*sync.Mutex
	chatSessionIndexRepairing map[string]struct{}

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

	// markerSecret is this service's durable HMAC secret (BUG-288 R18-4). Mint
	// paths must use withMarkerSecret so concurrent services do not share
	// runMarkerActive from another store's Init.
	markerSecret []byte
	markerDir    string

	// dispatchStore is the dedicated durable turn-dispatch store (CP-51 / SD-24).
	// Nil keeps V1 prep:/bare idempotency behavior until V2 is activated per run.
	dispatchStore DispatchStore
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
	// lastGrokTurnSessionID is the Grok twin of lastCodexTurnSessionID: the
	// newest ~/.grok/sessions/<enc-cwd>/<id>/ session id discovered after a Grok
	// turn, so each turn's id is logged once for precise transcript replay
	// (BUG-GrokReplay-Restart).
	lastGrokTurnSessionID string
	// lastOpencodeTurnSessionID is the Opencode twin (CP-57).
	lastOpencodeTurnSessionID string
	providerAccountID     string
	workspaceCwd          string
	stepID                string
	modelName             string
	yolo                  bool
	// reasoningEffort is the desktop-selected effort level passed per-turn (T-4).
	reasoningEffort string
	// chatPosture is the per-turn posture (scan/plan/code, "" = code). Persisted
	// on the run like yolo/model so children spawned mid-turn inherit it and the
	// approval bridge applies the read-only policy for scan/plan.
	chatPosture string
	changeType  string
	sourceDocID string
	turnCount   int
	// runKind is "chat" for normal-chat runs, "" / "workflow" for workflow runs (T-7).
	runKind string

	// Chat SSOT leg fields (CP-59 / SD-26 §5.1). Zero-valued for workflow runs
	// and for legacy chat runs until ensureChatTagging self-tags them.
	// switchFromRunID is the durable switch intent (SD26-S-2 phase A).
	chatID          string
	legSeq          int
	legState        string
	legClosedReason string
	switchFromRunID string

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
	// pendingRestartGen (BUG-288 R15-P0) durable delivery generation for stall-retry.
	pendingRestartGen int64
	// uiInitiated marks a child run spawned from the desktop UI (not the AI spawn_agent
	// tool). UI spawns are invisible to the parent's provider conversation, so the parent
	// must be told about them out-of-band; tool spawns are already in provider history. (BUG-122)
	uiInitiated bool
	// flowCohortId groups siblings into a barrier; non-empty means this child's result
	// is buffered until all cohort members are terminal, then one consolidated note is
	// delivered to the hub (Task-092 / CP-36 P-6).
	flowCohortId string
	// cohortSkipConsumed (V9-25): stall Skip already appended a synthetic failed
	// cohort entry; ignore late provider TurnFailed so we do not double-drain.
	cohortSkipConsumed bool
	// stalledSkipCause (BUG-288 P1-14/P1-19) marks that this run's in-flight
	// turn is being cancelled AS PART OF a Stall Skip decision (Task-241
	// contract: Skip -> terminal FAILED), not a generic Stop/interrupt. Without
	// this, finishTurn's context.Canceled branch below unconditionally
	// overwrote the Failed status the skip handler had just set to Cancelled
	// when the cancellation propagated back through the adapter.
	stalledSkipCause bool
	// stalledRetryCause (BUG-288 R13-02) marks that this run's in-flight turn is
	// being cancelled as part of Stall Retry (restart intent parked on parent).
	// finishTurn/emitLocked must not append a cohort "failed" entry or stamp the
	// node FAILED — the real retry result will land later.
	stalledRetryCause bool
	// stalledRetrySuppressCohort is set only around the emitLocked call for a
	// stall-retry cancel so EventTurnFailed does not buffer/join the cohort.
	stalledRetrySuppressCohort bool
	// parkCancelCause (CP-58 run-203966) marks that this run's in-flight turn
	// is being cancelled by parkFlowForAwaitingUser(Locked) itself — the flow
	// is parking awaiting a user decision, NOT the user interrupting the
	// turn. finishTurn must not map this to the generic "interrupted by
	// user" Cancelled stamp: a terminal parent makes flowRunTerminalLocked
	// true, silently skipping every later advance ("run terminal/stopped")
	// until the watchdog parks hub_stalled. One-shot, same shape as
	// stalledRetryCause: cleared at finishTurn entry for any non-
	// context.Canceled error, consumed in the park branch, and cleared when
	// Stop already sealed the loop (Stop wins).
	parkCancelCause bool
	// parkCancelSuppress (CP-58 run-203966) is set at park time (before
	// turnCancel) so an adapter that emits EventTurnFailed mid-cancel keeps
	// the run non-terminal and skips the root-flow failure side effects
	// (canonical-head abandon, signalChild), and again around finishTurn's
	// own park-cancel emitLocked.
	parkCancelSuppress bool
	// skipNextTurnIdleNotify (BUG-288 R13-05) suppresses one notifyTurnIdle at
	// runTurn tail when block/reprompt persist failed (or already notified).
	skipNextTurnIdleNotify bool
	// dispatch is a RAM cache of DispatchRecord keyed by turnID (CP-51). Loaded
	// from the dispatch store at boot; never sourced from the session snapshot.
	dispatch map[string]*DispatchRecord
	// dispatchProtocolVersion / repair* / markerProvenance* are session-mirror
	// scalars only (SD-24 D-1) — never a DispatchRecord slice.
	dispatchProtocolVersion int
	repairRequired          bool
	repairReason            string
	markerProvenanceRunIDs  []string
	// Mint-time provenance for FCP marker binding (Task-252 / SD-24 §6.6).
	pendingRestartProvenanceRunID      string
	pendingGateRepromptProvenanceRunID string
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
	// activeHubNodeID (Task-235) is the flow node id of whichever hub-driven
	// inline node (hub.inline / hub.notify) is currently awaiting the hub's own
	// flow_control("done") call. Empty until the first such node completes;
	// advanceHubDoneThroughEdge falls back to hubInlineNodeID(nodes) (the
	// flow's sole hub.inline node, e.g. "synthesis") when this is empty, which
	// reproduces the pre-Task-235 behavior for every existing built-in flow
	// unchanged. Set to a hub.notify node's own id when that node is dispatched,
	// so a SECOND hub-turn's "done" (e.g. after synthesis --done--> notify) is
	// resolved against notify's own forward edge instead of re-finding synthesis
	// and looping forever. Known limitation: not persisted across a restart —
	// resuming mid-hub.notify-turn falls back to the flow's sole hub.inline node,
	// which is wrong for that one edge case (see Task-235 Open Questions).
	activeHubNodeID string
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
	// turnStartedAfterLoopDone marks that THIS turn was admitted while the flow
	// loop had ALREADY reached "done" (BUG-305). Such a turn is a plain chat
	// follow-up (admitted by BUG-302), not flow work: emitLocked must not defer
	// its TurnCompleted behind the post-turn flow gate, and runTurn must skip
	// that gate. Otherwise the gate force-blocks a "done" loop (gateEpochStillValid
	// returns false for "done"), the real TurnCompleted is never broadcast to live
	// subscribers, and the desktop's turn stream hangs forever (thinking row / Stop
	// button / history spinner stuck until the chat is reopened). Captured at
	// startTurn admission — NOT at emit time — because the hub's own synthesis turn
	// transitions running->done DURING its own turn and must still be gated.
	turnStartedAfterLoopDone bool
	// pendingFlowRefInvalidErr is set by resolveWorkflowFlowRef (BUG-270) when
	// a run's selected workflowID resolved to an actual flow definition that
	// then failed validation (agentpack.ValidateFlowDefinition,
	// ValidateFlowContextSources, ValidateFlowArtifactBindings) — as opposed
	// to the workflowID simply not being a flow at all. handleStartTurn reads
	// and clears this right after resolveWorkflowFlowRef returns ok=false, so
	// a genuine data problem surfaces to the user as an HTTP error instead of
	// silently falling back to a normal chat turn with no explanation.
	pendingFlowRefInvalidErr error
	// chatSubMode/chatFlowRef record the explicit Chat-Mode orchestration
	// picker selection (CP-42/Task-177 — Bug sub-mode's "Built-in
	// orchestration" select) that started this run, e.g. subMode="bug",
	// flowRef="flowpilot-core-flow-pack/review-loop". BUG-263: these used to
	// exist only as transient TurnInput fields inside startTurn, never stored
	// on the run itself or persisted, so a restart-and-resume (or reopening
	// from run history) had nothing to restore the Chat Intent panel's Bug
	// tab / Built-in orchestration selection from — it silently fell back to
	// "Normal", even though the run's own flow (activeFlowNodes/Edges,
	// flowEngineDriven) kept working correctly underneath. Set once, on the
	// same first-turn branch that already sets flowEngineDriven, and
	// round-tripped through ProviderSessionState so a resumed/reopened run
	// can restore the exact picker selection it was started with.
	chatSubMode string
	chatFlowRef string
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
	// pendingHubReinvokePrompt (Task-235 follow-up, BUG-284) is the FULL custom
	// prompt a deferred hub.notify reinvoke (maybeAutoReinvokeHubWithPrompt)
	// must retry with once the current turn clears turnInFlight. Without this,
	// the generic pendingHubReinvoke retry (runTurn, below) always re-fires the
	// synthesis-shaped maybeAutoReinvokeHub instead — silently losing the
	// hub.notify dispatch, since a hub.notify "done" is ALWAYS observed while
	// turnInFlight is still true (it arrives as a tool call from the very turn
	// that is finishing), so this defer path is the COMMON case, not an edge
	// case. Empty means "no custom prompt pending" — the existing generic
	// retry behavior for cohort-join reinvokes is unaffected.
	pendingHubReinvokePrompt string
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
	// activeFlowAcceptanceNodes mirrors the resolved flow's acceptance_nodes
	// (CP-55 / CP-53 P-2). When it includes "synthesis", hub approved→done
	// requires reviewer machine verdicts recorded via submit_review_outcome.
	activeFlowAcceptanceNodes []string
	// pendingReviewVerdictByLabel buffers reviewer submit_review_outcome calls
	// until the cohort member's turn completes and appendCohortResult runs.
	pendingReviewVerdictByLabel map[string]string
	// lastReviewCohortVerdicts is a snapshot taken at the most recent review
	// cohort join (label → approved|changes_requested|blocked).
	lastReviewCohortVerdicts map[string]string

	status          RunStatus
	createdAt       string
	updatedAt       string
	lastPrompt      string
	lastFullPrompt  string
	lastMessage     string
	sourceMachineID string
	sourceRunID     string
	restoredFrom    string
	syncStatus      string
	syncUpdatedAt   string
	seq             int64
	lastEventType   ProviderEventType
	events          []ProviderEvent

	turnInFlight     bool
	repromptAttempts int    // CP-35 P-5: number of flow-gate reprompts issued this turn
	turnStartGitHead string // CP-35: git HEAD captured at turn start for committed-diff detection
	// turnStartWorktree maps dirty path → content fingerprint at turn start
	// (Task-242). Child gate diffs against this so pre-existing coder dirt does
	// not look like this turn's edits.
	turnStartWorktree     map[string]string
	lastTurnStepID        string // CP-35: stepID of the most-recently started turn, used by gate reprompts
	currentTurnID         string
	lastFlowControlTurnID string
	// lastEscalateReason / lastEscalateCohortLen track Task-241 T-8 bounded
	// progress against confirm-loop: re-escalate with the same reason and no
	// new cohort results appends a "no progress" GateReason suffix.
	lastEscalateReason    string
	lastEscalateCohortLen int
	// lastEscalatedInlineNodeID (BUG-289 A5/F-9) remembers which inline
	// validate/audit node escalated in a hub-less flow so Continue re-enters
	// that node instead of reinvoking a nonexistent hub.
	lastEscalatedInlineNodeID string
	// lastFailedDelegateNodeID (CA-616) remembers which hub-less delegate
	// (preflight_contract_plan) failed so Continue can retry that same node
	// via reinvokeMatchingFlowChild instead of a generic hub reinvoke.
	lastFailedDelegateNodeID string
	// lastProviderEventAt is stamped on every emitLocked for stall detection
	// (Task-241 T-11). Zero means no event yet (member just spawned).
	lastProviderEventAt time.Time
	// hubLastProgressAt is stamped when the hub/root makes progress (turn,
	// gate, reinvoke arm) for BUG-289 F-0 hub_stalled watchdog (I-16 hub).
	hubLastProgressAt time.Time
	// hubReinvokeStartFailCount counts consecutive startTurn rejects from a
	// scheduled hub reinvoke (BUG-289 H1 review). Caps active notifyTurnIdle
	// retries so a permanent reject cannot tight-loop.
	hubReinvokeStartFailCount int
	// stallTimeout is the parent-run stall window (Task-241). Default 10m when
	// flow policy leaves StallTimeoutSec at 0.
	stallTimeout time.Duration
	// flowStartGitHead is the workspace HEAD captured once at startResolvedFlow
	// (Task-242 tier-3). Audit uses it as the aggregate-diff base so multi-child
	// edits are visible, not only the hub's last turnStartGitHead.
	flowStartGitHead string
	// flowStartWorktreeFingerprint is the dirty-file content fingerprint
	// captured once at startResolvedFlow, alongside flowStartGitHead (CP-55
	// P-3). changedFilesSince(workspace, flowStartGitHead) reports every
	// currently-uncommitted path regardless of baseSHA — it cannot by itself
	// distinguish "the planner just touched this" from "this was already
	// dirty before the flow started." runContractFreezeNode diffs a fresh
	// fingerprint against this baseline (new path, or same path with a
	// different hash, is a real mutation; same path/same hash is pre-existing
	// dirt) instead of treating any nonempty diff as the planner's doing.
	flowStartWorktreeFingerprint map[string]string
	// gateCheckpointNotDurable (BUG-288 R17-P1): settle is set in RAM but all
	// persist attempts failed — must not run gate pass / completion fan-out
	// until a durable checkpoint succeeds.
	gateCheckpointNotDurable bool
	// pendingFlowGateSettle defers cohort join / step DONE / tryAdvance /
	// signalChild / releaseDependentAgents until the post-turn child gate passes
	// (Task-242 D-1/D-3/D-9). Set on EventTurnCompleted for flow-engine children;
	// cleared after settle or gate block/reprompt.
	pendingFlowGateSettle     bool
	pendingFlowGateFinalMsg   string
	pendingFlowGateOccurredAt string
	// pendingFlowGateTurnID is the provider turn id for deferred TurnCompleted
	// materialization after restart (V10R P1).
	pendingFlowGateTurnID string
	// pendingGateChangedFiles are EventFileChanged paths captured for the
	// deferred gate so resume after restart has the same WrittenPaths (V10 residual P0).
	pendingGateChangedFiles []string
	// pendingGateCodePaths are paths from a prior gate-failing coding turn.
	// A follow-up turn with empty new diff still re-checks until remediated (BUG-288 #8).
	pendingGateCodePaths []string
	// pendingGateRepromptPrompt/StepID defer gate-reprompt startTurn until after
	// pendingFlowGateSettle / postTurnGateCancel / turnInFlight are cleared
	// (V10R3 P0 — avoid 409 gate_in_progress / turn_in_progress).
	// Durable on ProviderSessionState (V10R4 P1).
	pendingGateRepromptPrompt string
	pendingGateRepromptStepID string
	// hubContinueDelegatedTurnID is the provider turn that applied
	// flow_control continue and already advanced a delegate writer (CP-51 A1
	// residual / run-9437). Same-turn hub gate reprompts are suppressed so the
	// hub does not open a concurrent write turn.
	hubContinueDelegatedTurnID string
	// pendingResumePrompt/StepID is a durable continuation after rehydrated
	// approval/question resolve; cleared only after startTurn succeeds (V10R4 P1).
	pendingResumePrompt string
	pendingResumeStepID string
	// pendingResumeGen / pendingGateRepromptGen: compare-and-clear tokens.
	pendingResumeGen       int64
	pendingGateRepromptGen int64
	// intentClaim* is an in-process lease so only one delivery can startTurn
	// for a given durable intent generation (V10R4 P1). Not persisted — crash
	// drops the lease so reconstruct can reclaim.
	intentClaimKind  string
	intentClaimGen   int64
	intentClaimUntil time.Time
	// Durable fail budgets keyed by generation (V10R4 P1).
	pendingResumeFailCount       int
	pendingResumeFailGen         int64
	pendingGateRepromptFailCount int
	pendingGateRepromptFailGen   int64
	// pending*DeliveredGen + AcceptedTurn: set ONLY after startTurn accepts
	// (V10R4 P0-02). Never treat pre-call markers as delivery proof.
	pendingResumeDeliveredGen       int64
	pendingGateRepromptDeliveredGen int64
	pendingResumeAcceptedTurn       string
	pendingGateRepromptAcceptedTurn string
	// pendingResumeApprovalID/Decision reconcile card vs intent across two writes.
	pendingResumeApprovalID string
	pendingResumeDecision   string
	// pendingResumeQuestionChoices preserves the original multi-select answer
	// slice for reconciliation (BUG-288 P2-01) instead of relying on
	// pendingResumeDecision's joined display string, which is lossy to
	// re-split when a choice itself contains the join separator.
	pendingResumeQuestionChoices []string
	// intentBlockedKind/Reason/At durably record that a durable turn intent
	// ("resume" or "reprompt") exhausted its permanent-failure retry budget
	// (BUG-288 P1-09). Before this, the flush loop just returned silently —
	// no durable state told the caller/UI the intent generation was inert, and
	// there was no way to retry once the underlying condition (bad config,
	// unavailable provider/account) was fixed. RequeueBlockedIntent clears
	// these and re-attempts.
	intentBlockedKind   string
	intentBlockedReason string
	intentBlockedAt     string
	// transitionLogDegraded durably marks that a step-transition-log append
	// failed (BUG-288 P2-03). Task-239's contract is "every transition is
	// persisted; restore replays the log" — a silent log-warn-only failure
	// breaks that invariant with no visible trace. Restore/replay logic can
	// check this flag and escalate (treat the in-memory step timeline as
	// untrustworthy) instead of silently treating it as authoritative.
	transitionLogDegraded       bool
	transitionLogDegradedAt     string
	transitionLogDegradedReason string
	// gateEpoch increments on Stop; deferred gate side effects revalidate before
	// completing/reprompting (V10R4 P0 Stop vs in-flight gate race).
	gateEpoch int64
	// stopGeneration is the durable, monotonically increasing stop tombstone for
	// a ROOT/parent run (BUG-288 P1-04). It is the single authority for whether
	// a Stop happened "after" a given child checkpoint — unlike gateEpoch (only
	// meaningful within one process lifetime) or agentOrchestrator's in-memory
	// loop state (populated at reconstruction in arbitrary order), this field is
	// loaded directly from the persisted parent session at process start, so
	// the check does not depend on reconstruction ordering.
	stopGeneration int64
	// parentStopGenSeen is a CHILD run's snapshot of its parent's stopGeneration
	// as of this child's last durable checkpoint (BUG-288 P1-04). Child
	// reconstruction and resumePendingFlowGate reject/no-op when the parent's
	// current stopGeneration is newer than this value — the child was
	// checkpointed before (or during) a Stop that it never itself observed.
	parentStopGenSeen int64
	// gateClaimID is a single-flight CAS for resumePendingFlowGate / live gate apply.
	gateClaimID string
	// flowContextInjected is set when ComposeFlowCodingPrompt/package handoff is
	// applied by the runner (not user text). injectFeatureHistoryPrompt skips
	// only when this flag is true — text markers alone are forgeable (P1 envelope).
	flowContextInjected bool
	// changeContractInjected is set after appendChangeContractIfAny writes the
	// run-scoped trusted marker for this turn composition.
	changeContractInjected bool
	// suppressAutoGateResume delays resumePendingFlowGate until cohort restore
	// finishes (V10R4 P0 atomic cohort recovery).
	suppressAutoGateResume bool
	// postTurnGateCancel cancels an in-flight post-turn oracle/gate after Stop
	// (BUG-288 #10). Distinct from turnCancel which finishTurn clears first.
	postTurnGateCancel context.CancelFunc
	// flowInlineCtx/Cancel covers in-process inline nodes (validate/audit) that
	// run without a hub turn. stopAgentLoop cancels this so suites abort.
	flowInlineCtx    context.Context
	flowInlineCancel context.CancelFunc
	lastTurnID       string // id of the most-recently completed turn, for the rolling chat summary
	// flowValidationRetryState is the in-memory Testing<->Coding retry loop
	// state for a run driving rag-harness's validate node (BUG-243 F-1),
	// keyed on the PARENT/hub run (not the per-turn coder child). Durable
	// audit trail is the EventFlowValidationRetry events PersistRetryState
	// emits on every transition; this in-memory copy is what the next
	// validate dispatch reads to decide retrying/passed/failed without
	// replaying the event log on every turn.
	flowValidationRetryState *FlowValidationRetryState
	turnCancel               context.CancelFunc

	pendingApprovalID string
	pendingQuestionID string
	// pendingGateBlock holds r-reg details for the decision handler (Task-155).
	// Cleared when the user submits a decision via handleGateDecision.
	pendingGateBlock *gateBlockInfo
	// gateFixCodeActive is set when the user chooses keep-test-fix-code.
	// While active, further r-reg hits auto-reprompt (no decision modal) until
	// maxGateFixCodeAutoReprompts or the suite goes green (run-11262 loop).
	gateFixCodeActive   bool
	gateFixCodeAttempts int
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
	// sidecarPrefixCount is the number of CP-41/question sidecar events
	// reconstructRun prepended to rs.events before any real transcript was
	// loaded (reconstructRun runs before seedTranscriptFromDisk, so it has no
	// choice but to seed them first). seedTranscriptFromDisk moves this many
	// leading events to the end of the timeline once the real transcript has
	// been appended, so a restored resolved question (or other sidecar event)
	// no longer renders pinned above the entire prior conversation after a
	// full server restart (see reorderSidecarPrefixToEnd).
	sidecarPrefixCount int64
}

type approvalRecord struct {
	id       string
	runID    string
	details  ApprovalDetails
	status   string // pending | resolving | resolved | expired
	decision string
	// policy records why an auto-decision was made (empty for human decisions):
	// "yolo_gating_disabled" | "policy_allowlist" | "policy_denylist".
	policy  string
	resolve chan string
	// rehydrated is true when this record was rebuilt after restart and no
	// live provider turn is waiting on resolve (V10 residual P1).
	rehydrated bool
	// expiresAt is the durable absolute TTL deadline (RFC3339Nano), persisted
	// at creation (BUG-288 P1-08). Submit/Answer re-check this under the lock
	// so a restart cannot accept a decision on a card whose TTL already
	// elapsed while no in-process timer was running to expire it.
	expiresAt string
	// resolving* stash the durable-write payload computed the first time this
	// record transitions pending->resolving (BUG-288 P1-01). A retry that
	// observes status=="resolving" (the prior attempt persisted nothing, or
	// only partially) replays these writes instead of re-deriving RAM state a
	// second time or short-circuiting to a false success.
	resolvingSnapshot      *ProviderApprovalState
	resolvingSession       *ProviderSessionState
	resolvingStepParent    string
	resolvingStepLabel     string
	resolvingStepTurn      bool
	resolvingRestartRunID  string
	resolvingRestartStepID string
	resolvingRestartPrompt string
	resolvingRestartGen    int64
	resolvingRememberCwd   string
}

type questionRecord struct {
	id          string
	runID       string
	prompt      string
	options     []QuestionOption
	multiSelect bool
	status      string // pending | resolving | resolved | expired
	choice      []string
	// resolve carries a typed result so cancellation (Stop) can be
	// distinguished from a genuine empty multi-select submission (BUG-288
	// P2-02). Sending a bare []string{} for interruption was indistinguishable
	// from a real answer when select{} raced rec.resolve against ctx.Done().
	resolve chan questionResolveResult
	// rehydrated is true when rebuilt after restart with no live turn waiting
	// on resolve (V10 residual P1).
	rehydrated bool
	// expiresAt is the durable absolute TTL deadline (RFC3339Nano), persisted
	// at creation (BUG-288 P1-08). See approvalRecord.expiresAt.
	expiresAt string
	// resolving* stash the durable-write payload computed the first time this
	// record transitions pending->resolving (BUG-288 P1-01). See
	// approvalRecord.resolving* for the shared rationale.
	resolvingSnapshot      *ProviderQuestionState
	resolvingSession       *ProviderSessionState
	resolvingRunID         string
	resolvingStepParent    string
	resolvingStepLabel     string
	resolvingStepTurn      bool
	resolvingRestartRunID  string
	resolvingRestartStepID string
	resolvingRestartPrompt string
	resolvingRestartGen    int64
}

// questionResolveResult is the typed payload sent on questionRecord.resolve
// (BUG-288 P2-02). err is non-nil only for interruption/expiry signaled
// through the channel (e.g. errInterrupted); a genuine user submission always
// has err == nil, even when choices is empty (free-text/"Other" answers are
// validated for non-empty before resolve, so an empty choices+nil err should
// not occur from AnswerQuestion, but callers must still check err first).
type questionResolveResult struct {
	choices []string
	err     error
}

// errInterrupted signals that a pending question/approval wait was unblocked
// by Stop/cancellation rather than a genuine user answer (BUG-288 P2-02).
var errInterrupted = errors.New("interrupted")

func approvalStateFromRecord(rs *interactiveRun, rec *approvalRecord, expiresAt string) ProviderApprovalState {
	state := ProviderApprovalState{
		ApprovalID: rec.id,
		RunID:      rec.runID,
		Status:     persistedGateStatus(rec.status),
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
		Status:         persistedGateStatus(rec.status),
		Choice:         rec.choice,
		ExpiresAt:      expiresAt,
	}
}

// persistedGateStatus maps an approval/question record's in-memory status to
// the value written to the durable store (BUG-288 P1-01). "resolving" is a
// transitional, process-local state used only to detect an in-flight
// resolution that has not yet been durably confirmed — the persisted record
// itself must show "resolved" once its snapshot is taken, since by the time
// this snapshot reaches disk the write IS the confirmation. Any other status
// (pending/resolved/expired) passes through unchanged.
func persistedGateStatus(status string) string {
	if status == "resolving" {
		return "resolved"
	}
	return status
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
	// BUG-288 R14-04 / R18-4: load this service's durable marker secret (per data dir).
	markerDir, markerSec := loadMarkerSecretForStore(workflowStore)
	// Reclaim Codex image-attachment temp dirs orphaned by a prior hard crash/kill
	// (Task-052); the per-turn deferred cleanup cannot run in that case. Best-effort.
	sweepCodexImageAttachments(time.Hour, time.Now())
	// Grok path-fallback images under .tmp/images (CA-483); cwd unknown at boot
	// so this only sweeps the no-cwd temp root (project-local dirs age out later).
	sweepGrokImagePathFallback("", time.Hour, time.Now())
	svc := &InteractiveService{
		catalog:                   catalog,
		skillsCatalog:             newInteractiveCatalog(),
		agentCatalog:              newAgentCatalog(),
		agentOrchestrator:         newAgentOrchestrator(),
		registry:                  registry,
		policy:                    DefaultApprovalPolicyEngine(),
		finalizer:                 newFinalizer(),
		workflowStore:             workflowStore,
		orchestrator:              NewWorkflowOrchestrator(workflowStore),
		runs:                      map[string]*interactiveRun{},
		approvals:                 map[string]*approvalRecord{},
		questions:                 map[string]*questionRecord{},
		activeAccountID:           "default",
		approvalTTL:               10 * time.Minute,
		questionTTL:               10 * time.Minute,
		maxTurnAttempts:           3,
		summaryTimers:             map[string]*time.Timer{},
		markerSecret:              markerSec,
		markerDir:                 markerDir,
		dispatchLogSyncHash:       map[string]string{},
		chatSessionIndexLocks:     map[string]*sync.Mutex{},
		chatSessionIndexRepairing: map[string]struct{}{},
		gitnexusAnalyzeOnce:       map[string]bool{},
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

// stopAgentLoop terminals the agent loop and durable recovery state.
// On persistence failure it still cancels in-memory work but returns an apiErr
// so the client does not treat Stop as durable (V10R4 P0 fail-closed).
func (s *InteractiveService) stopAgentLoop(parentRunID string) (AgentGraphSnapshot, *apiErr) {
	// CP-51 Task-249 (P1): advance durable RunStopState BEFORE any RAM cancel so
	// a concurrent send's `send_claimed → send_started` CAS is fenced in revision
	// order (INV-3). This is the live-Stop linearization point; requestRunStopV2
	// is a no-op for runs without V2 dispatch records. Parent stop also fences
	// every child via its ParentStopFence (checkParentFenceLocked); we additionally
	// stop each child so in-flight child records are cancel-requested directly.
	//
	// Fail-closed (Codex review 2026-07-17): requestRunStopV2 now reports a
	// durable-store failure instead of silently swallowing it. RAM cancel still
	// proceeds below (defense-in-depth — it cancels any turn this process has
	// in flight right now, store or no store), but if the durable fence itself
	// could not be written, this handler must not return unqualified success:
	// a concurrent send elsewhere could still pass the CAS with no fence ever
	// recorded. dispatchStopFenceErr is checked again at the end alongside the
	// existing persistErr check.
	var dispatchStopFenceErr error
	if s.dispatchStore != nil {
		stopCtx := context.Background()
		if err := s.requestRunStopV2(stopCtx, parentRunID); err != nil {
			dispatchStopFenceErr = err
			log.Printf("[dispatch] durable Stop fence failed run=%s: %v", parentRunID, err)
		}
		for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
			if err := s.requestRunStopV2(stopCtx, childID); err != nil && dispatchStopFenceErr == nil {
				dispatchStopFenceErr = err
				log.Printf("[dispatch] durable Stop fence failed run=%s (child of %s): %v", childID, parentRunID, err)
			}
		}
	}
	// BUG-288 R17-P0: take s.mu and bump gateEpoch on parent+children BEFORE
	// agentOrchestrator.stop, so withGateEpochDurable cannot pass epoch/loop
	// checks then write override/contract after Stop was requested. Previously
	// stop() flipped loop status without s.mu while gate held s.mu mid-write.
	s.mu.Lock()
	if parent := s.runs[parentRunID]; parent != nil {
		parent.gateEpoch++
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		if child := s.runs[childID]; child != nil {
			child.gateEpoch++
		}
	}
	// Stop loop under s.mu so concurrent gateEpochStillValidLocked sees stopped.
	// AgentOrchestrator only takes its own mu (no s.mu re-entry).
	s.agentOrchestrator.stop(parentRunID)
	// BUG-288 P1-04: bump the parent's durable stop tombstone BEFORE any child
	// is stamped/checkpointed below, so every child snapshot this Stop call
	// persists carries proof it was checkpointed at-or-after this stop. A
	// child checkpointed before this line (e.g. one that failed to persist
	// during this same Stop, or was never reached) keeps its older
	// parentStopGenSeen and is correctly recognized as stale on reconstruct.
	var newStopGen int64
	var cwdForAbandonedCanonical string
	if parent := s.runs[parentRunID]; parent != nil {
		cwdForAbandonedCanonical = parent.workspaceCwd
		parent.pendingRestartRunID = ""
		parent.pendingRestartPrompt = ""
		parent.pendingRestartGen = 0
		parent.autoOrchestrate = false
		parent.reinvokeInFlight = false
		parent.pendingHubReinvoke = false
		parent.stopGeneration++
		newStopGen = parent.stopGeneration
		// V10R4 P0: clear ALL root recovery intents before persist — otherwise
		// crash after Stop reloads PendingFlowGate* and resurrects the flow.
		clearDurableRecoveryStateLocked(parent)
		if parent.turnInFlight && parent.turnCancel != nil {
			parent.turnCancel()
		}
		// Cancel in-process validate/audit suites (no hub turnCancel).
		if parent.flowInlineCancel != nil {
			parent.flowInlineCancel()
			parent.flowInlineCancel = nil
			parent.flowInlineCtx = nil
		}
		// Cancel post-turn gate/oracle if still running after finishTurn cleared turnCancel.
		if parent.postTurnGateCancel != nil {
			parent.postTurnGateCancel()
			parent.postTurnGateCancel = nil
		}
		// V10R4 P1-03: every non-terminal root becomes Cancelled on Stop —
		// deferred gate / waiting card / idle-running must not persist Running
		// with LoopState=stopped.
		if parent.status != RunStatusCompleted && parent.status != RunStatusFailed &&
			parent.status != RunStatusCancelled {
			parent.status = RunStatusCancelled
			parent.agentStatus = string(RunStatusCancelled)
			parent.turnInFlight = false
		}
	}
	cancelledChildIDs := make([]string, 0)
	persistChildIDs := make([]string, 0)
	// Task-241 B1: cancelled cohort members must append as terminal so the
	// barrier can join (cancelled = terminal). restart-cancel path appends via
	// Task-239 resume rebuild, not here.
	type cancelledCohortMember struct {
		parentRunID, cohortID, label, provider string
	}
	var cancelledCohort []cancelledCohortMember
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		if child := s.runs[childID]; child != nil {
			child.pendingTurnPrompt = ""
			// BUG-288 #18: terminalize every non-terminal child on Stop, not only
			// those currently in-flight (waiting-dependency / pendingTurnPrompt).
			if child.turnInFlight && child.turnCancel != nil {
				child.turnCancel()
			}
			if child.postTurnGateCancel != nil {
				child.postTurnGateCancel()
				child.postTurnGateCancel = nil
			}
			// Always clear child durable recovery so restart cannot re-queue.
			clearDurableRecoveryStateLocked(child)
			// BUG-288 P1-04: stamp this child as checkpointed at-or-after the
			// parent's new stop generation. reconstructRun/resumePendingFlowGate
			// use this to reject a stale pending gate for a child whose own
			// persist failed/crashed before reaching this generation.
			child.parentStopGenSeen = newStopGen
			persistChildIDs = append(persistChildIDs, childID)
			if child.status != RunStatusCompleted && child.status != RunStatusFailed &&
				child.status != RunStatusCancelled {
				child.status = RunStatusCancelled
				child.agentStatus = string(RunStatusCancelled)
				child.turnInFlight = false
				cancelledChildIDs = append(cancelledChildIDs, childID)
				if child.flowCohortId != "" {
					cancelledCohort = append(cancelledCohort, cancelledCohortMember{
						parentRunID: child.parentRunID,
						cohortID:    child.flowCohortId,
						label:       child.label,
						provider:    string(child.providerKey),
					})
				}
			}
		}
	}
	// V10R4 P1: terminalize pending approval/question cards for parent+children
	// so restart cannot rehydrate stale actionable cards for a stopped flow.
	gateRunIDs := append([]string{parentRunID}, persistChildIDs...)
	cancelledApprovals, cancelledQuestions := s.cancelPendingGatesForRunsLocked(gateRunIDs)
	// Capture parent snap under lock after all clears.
	var parentSnap ProviderSessionState
	if parent := s.runs[parentRunID]; parent != nil {
		parentSnap = sessionStateOf(parent)
		parentSnap.LoopState = s.agentOrchestrator.loopStateFor(parentRunID)
	}
	childSnaps := make([]ProviderSessionState, 0, len(persistChildIDs))
	for _, childID := range persistChildIDs {
		if child := s.runs[childID]; child != nil {
			childSnaps = append(childSnaps, sessionStateOf(child))
		}
	}
	s.mu.Unlock()
	// CP-55 P-5: this Flow (if any) never reached genuine terminal acceptance
	// — abandon any Canonical Head updates its coder children staged, so
	// nothing pending is left resolvable as "the latest version" after a
	// Stop/Cancel. The real Canonical Head file is never touched here.
	// Review finding I-3: a staging record is keyed by the coder's
	// *immediate* parent run — for a sub-hub coding child that parent is the
	// sub-hub, not this Stop's own parentRunID, so also abandon for every
	// direct child collected above (persistChildIDs) or a nested coder's
	// pending record would never be resolved by this Stop.
	if strings.TrimSpace(cwdForAbandonedCanonical) != "" {
		abandonPendingCanonicalHeadsForRun(cwdForAbandonedCanonical, parentRunID, "flow stopped", s.markerSecret)
		for _, childID := range persistChildIDs {
			abandonPendingCanonicalHeadsForRun(cwdForAbandonedCanonical, childID, "flow stopped", s.markerSecret)
		}
	}
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
	// V10R4 P0: durable Stop checkpoint BEFORE response — parent + children
	// with cleared gate/intents (serialized cancellation barrier). Persist
	// failures must surface to the client (no fail-open success).
	var persistErr error
	if parentSnap.RunID != "" {
		if err := s.persistProviderSession(parentSnap); err != nil {
			persistErr = err
		}
	}
	for _, snap := range childSnaps {
		if err := s.persistProviderSession(snap); err != nil && persistErr == nil {
			persistErr = err
		}
	}
	// Cancel pending approval/question cards so restart cannot rehydrate them
	// as actionable for a stopped flow (V10R4 P1).
	for _, st := range cancelledApprovals {
		if err := s.persistApproval(st); err != nil && persistErr == nil {
			persistErr = err
		}
	}
	for _, st := range cancelledQuestions {
		if err := s.persistQuestion(st); err != nil && persistErr == nil {
			persistErr = err
		}
	}
	// Task-241: append cancelled members to cohort buffer (orchestrator lock only —
	// never re-enter s.mu). If barrier completes, join via maybeAutoReinvokeHubWithNote.
	for _, m := range cancelledCohort {
		s.agentOrchestrator.appendCohortResult(m.parentRunID, m.cohortID, cohortEntry{
			Label: m.label, Provider: m.provider, Status: "cancelled",
		})
		if s.isFlowEngineDriven(m.parentRunID) && m.label != "" {
			s.setFlowStepStatus(context.Background(), m.parentRunID, m.label, StepStatusCanceled)
		}
		s.ensureCohortExpected(m.parentRunID, m.cohortID)
		if s.agentOrchestrator.cohortComplete(m.parentRunID, m.cohortID) {
			entries := s.agentOrchestrator.drainCohort(m.parentRunID, m.cohortID)
			round := s.agentOrchestrator.loopStateFor(m.parentRunID).Round
			note := buildCohortNote(m.parentRunID, m.cohortID, entries, round)
			// BUG-307: this Stop request just cancelled every member above, so the
			// cohort can complete ITS join right here with the loop already
			// "stopped" (agentOrchestrator.stop ran earlier in this same handler).
			// No reinvoke will ever drain the note in that case — see
			// loopSealedForReinvoke.
			if !s.loopSealedForReinvoke(m.parentRunID) {
				s.appendPendingAgentContext(m.parentRunID, note)
			}
			s.mu.Lock()
			if parent := s.runs[m.parentRunID]; parent != nil {
				parent.lastCohortNote = note
			}
			s.mu.Unlock()
			if s.isFlowEngineDriven(m.parentRunID) {
				for _, e := range entries {
					if e.Label == "" {
						continue
					}
					switch e.Status {
					case "completed":
						s.setFlowStepStatus(context.Background(), m.parentRunID, e.Label, StepStatusDone)
					case "failed":
						s.setFlowStepStatus(context.Background(), m.parentRunID, e.Label, StepStatusFailed)
					case "cancelled":
						s.setFlowStepStatus(context.Background(), m.parentRunID, e.Label, StepStatusCanceled)
					}
				}
				if hubID := hubInlineNodeID(s.activeFlowNodesFor(m.parentRunID)); hubID != "" && s.loopIsAdvancing(m.parentRunID) {
					s.setFlowStepStatus(context.Background(), m.parentRunID, hubID, StepStatusRunning)
				}
			}
			go s.maybeAutoReinvokeHubWithNote(m.parentRunID, note)
		}
	}
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	if persistErr != nil {
		return snap, newAPIErr(http.StatusInternalServerError, "persist_failed",
			"stop applied in-memory but durable checkpoint failed: "+persistErr.Error())
	}
	if dispatchStopFenceErr != nil {
		// CP-51 Task-249 fail-closed (Codex review 2026-07-17): RAM cancel above
		// already ran, but the durable send-fence (INV-3) was not persisted, so a
		// concurrent send elsewhere could still pass its CAS unfenced. Report
		// failure — do not let the client believe Stop is durably guaranteed.
		return snap, newAPIErr(http.StatusInternalServerError, "dispatch_stop_fence_failed",
			"stop applied in-memory but the durable stop fence could not be persisted: "+dispatchStopFenceErr.Error())
	}
	return snap, nil
}

// cancelPendingGatesForRunsLocked marks in-memory pending approval/question
// records expired for the given runs and returns snapshots to persist.
// Caller holds s.mu.
func (s *InteractiveService) cancelPendingGatesForRunsLocked(runIDs []string) (approvals []ProviderApprovalState, questions []ProviderQuestionState) {
	if len(runIDs) == 0 {
		return nil, nil
	}
	want := make(map[string]bool, len(runIDs))
	for _, id := range runIDs {
		if id != "" {
			want[id] = true
		}
	}
	for id, rec := range s.approvals {
		if rec == nil || !want[rec.runID] {
			continue
		}
		if rec.status != "pending" {
			continue
		}
		rec.status = "expired"
		// Unblock any live waiter so the turn can observe cancellation.
		select {
		case rec.resolve <- "deny":
		default:
		}
		if rs := s.runs[rec.runID]; rs != nil {
			if rs.pendingApprovalID == id {
				rs.pendingApprovalID = ""
			}
			st := approvalStateFromRecord(rs, rec, "")
			approvals = append(approvals, st)
		} else {
			st := approvalStateFromRecord(nil, rec, "")
			approvals = append(approvals, st)
		}
	}
	for id, rec := range s.questions {
		if rec == nil || !want[rec.runID] {
			continue
		}
		if rec.status != "pending" {
			continue
		}
		rec.status = "expired"
		// BUG-288 P2-02: signal interruption distinctly from a real empty
		// multi-select submission so a live AskQuestion waiter cannot observe
		// a fabricated "success" if select{} races this against ctx.Done().
		select {
		case rec.resolve <- questionResolveResult{err: errInterrupted}:
		default:
		}
		if rs := s.runs[rec.runID]; rs != nil {
			if rs.pendingQuestionID == id {
				rs.pendingQuestionID = ""
			}
		}
		st := questionStateFromRecord(rec, "", "")
		questions = append(questions, st)
	}
	return approvals, questions
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
	// Task-241 I-5: at most one effective flow_control decision per provider turn —
	// reject a second apply with the same currentTurnID (tool bridge also checks).
	// BUG-288 #12: do not mutate loop after stopped/done (Stop race after audit guard).
	s.mu.Lock()
	rs, runExists := s.runs[parentRunID]
	if !runExists {
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "flow_control_missing_run", "flow control target run was not found",
			"status", in.Status,
		)
		return FlowControlResult{}, fmt.Errorf("applyFlowControl: run %q not found", parentRunID)
	}
	// BUG-288 #26: validate status enum BEFORE stamping one-decision. An
	// unknown status (typo) must not burn the turn's decision slot, or a later
	// valid done/continue/escalate on the same turn is wrongly rejected.
	status := strings.TrimSpace(in.Status)
	switch status {
	case "done", "continue", "escalate":
	default:
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "flow_control_unknown_status", "unknown flow control status received",
			"status", in.Status,
		)
		return FlowControlResult{}, fmt.Errorf("applyFlowControl: unknown status %q", in.Status)
	}
	// Terminal guard before stamp (BUG-288 #12 / #26).
	loopSnap := s.agentOrchestrator.graphSnapshot(parentRunID)
	if loopSnap.LoopState.Status == "stopped" || loopSnap.LoopState.Status == "done" {
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "flow_control_rejected_terminal",
			"flow control ignored: loop already terminal",
			"status", in.Status,
			"loop_status", loopSnap.LoopState.Status,
		)
		return FlowControlResult{
			Status:     in.Status,
			Round:      loopSnap.LoopState.Round,
			Cap:        effectiveCap(loopSnap.LoopState),
			NextAction: "rejected_terminal",
		}, nil
	}
	if rs.currentTurnID != "" {
		if rs.lastFlowControlTurnID == rs.currentTurnID {
			s.mu.Unlock()
			s.flowDiagLog(parentRunID, "flow_control_rejected_one_decision",
				"flow control already submitted for this provider turn",
				"status", in.Status,
				"turn_id", rs.currentTurnID,
			)
			return FlowControlResult{}, fmt.Errorf("flow control already submitted for this provider turn")
		}
		// Stamp only after status+terminal checks. Soft-defer (open cohort) undoes
		// this below so join-then-done on the same turn still works.
		rs.lastFlowControlTurnID = rs.currentTurnID
	}
	// CP-55 P-5 review finding I-2: capture workspaceCwd while s.mu is still
	// held — rs is a shared *interactiveRun read again below (the "done"
	// case) after this unlock, and workspaceCwd is a mutable field a
	// concurrent resume/reconstruct can write.
	flowControlWorkspaceCwd := rs.workspaceCwd
	s.mu.Unlock()
	// Task-240 T-4 / BUG-179: soft-defer done/continue while a registered cohort
	// is still incomplete. Do not mutate loop or settle steps — the hub will be
	// reinvoked after join and can call flow_control again.
	// Soft-defer does NOT count as a decision — clear lastFlowControlTurnID so the
	// hub can re-apply after join on the same turn if needed. (Decision was stamped
	// above; for incomplete-cohort we undo so join-then-done still works.)
	if (status == "done" || status == "continue") && s.agentOrchestrator.hasOpenCohort(parentRunID) {
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil && rs.currentTurnID != "" && rs.lastFlowControlTurnID == rs.currentTurnID {
			rs.lastFlowControlTurnID = ""
		}
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "flow_control_rejected_cohort_incomplete",
			"flow control deferred: cohort still incomplete",
			"status", in.Status,
		)
		snap := s.agentOrchestrator.graphSnapshot(parentRunID)
		return FlowControlResult{
			Status:     in.Status,
			Round:      snap.LoopState.Round,
			Cap:        effectiveCap(snap.LoopState),
			NextAction: "rejected_cohort_incomplete",
		}, nil
	}
	switch status {
	case "done":
		// CP-53 P-2 / Task-274: on flows with synthesis acceptance, hub
		// approved→done requires machine reviewer verdicts — not a self-grade.
		if in.viaReviewOutcome && in.reviewOutcomeStatus == "approved" {
			s.mu.Lock()
			requireVerdict := false
			if rs := s.runs[parentRunID]; rs != nil && flowRequiresSynthesisMachineVerdict(rs) {
				requireVerdict = true
			}
			s.mu.Unlock()
			if requireVerdict {
				if err := s.synthesisDoneVerdictError(parentRunID); err != nil {
					s.flowDiagLog(parentRunID, "flow_control_rejected_missing_review_verdict",
						"synthesis done blocked: reviewer machine verdict missing or not approved",
						"error", err.Error(),
					)
					s.mu.Lock()
					if r := s.runs[parentRunID]; r != nil && r.currentTurnID != "" && r.lastFlowControlTurnID == r.currentTurnID {
						r.lastFlowControlTurnID = ""
					}
					s.mu.Unlock()
					if _, escErr := s.applyFlowControl(parentRunID, FlowControlInput{
						Status:  "escalate",
						Summary: err.Error(),
					}); escErr != nil {
						log.Printf("[flow-control] escalate after missing review verdict: %v", escErr)
					}
					return FlowControlResult{}, err
				}
			}
		}
		// CP-55 P-5: finalize any Canonical Head updates staged by this Flow's
		// coder children BEFORE settling the step timeline / publishing
		// loop=done — a Flow's Canonical Head mutation is a terminal
		// acceptance effect, not a coder-turn effect (spec §8.2: "do not fall
		// back from failed terminal Canonical finalization to publishing
		// done"). A finalization failure aborts this whole "done" transition;
		// the loop status is never touched and the caller sees the error.
		if s.isFlowEngineDriven(parentRunID) {
			cwd := strings.TrimSpace(flowControlWorkspaceCwd)
			if cwd != "" {
				if err := finalizePendingCanonicalHeadsForRun(cwd, parentRunID, s.markerSecret); err != nil {
					s.flowDiagLog(parentRunID, "flow_done_canonical_finalize_failed", "canonical head finalization failed; not publishing done",
						"error", err.Error(),
					)
					// CP-55 P-5 review finding C-3: a finalize failure must not
					// silently burn this turn's one-decision slot with no path
					// forward for the hub to retry, and must not leave the loop
					// stuck "running" with no operator-actionable surface — the
					// same recurring hang failure mode this codebase has
					// regressed on before (BUG-288 #9 / Task-242 T-9). Unstamp
					// so a retried flow_control on this same turn is not
					// wrongly rejected as a duplicate, then escalate exactly
					// like every other fail-closed gate site (mirrors
					// gate_hook.go's own commit-failure handling) so the
					// operator sees an awaiting-user card instead of a wedge.
					s.mu.Lock()
					if r := s.runs[parentRunID]; r != nil && r.currentTurnID != "" && r.lastFlowControlTurnID == r.currentTurnID {
						r.lastFlowControlTurnID = ""
					}
					s.mu.Unlock()
					if _, escErr := s.applyFlowControl(parentRunID, FlowControlInput{
						Status:  "escalate",
						Summary: "flow done blocked: canonical head finalization failed: " + err.Error(),
					}); escErr != nil {
						log.Printf("[flow-control] escalate after canonical finalize failure: %v", escErr)
					}
					return FlowControlResult{}, fmt.Errorf("applyFlowControl: canonical head finalization failed, not publishing done: %w", err)
				}
			}
			// Re-check terminal status after the (potentially slow) finalize
			// I/O: a concurrent Stop must still win over a stale done attempt
			// racing it — finalize itself is idempotent/safe to have already
			// run, but this run must not go on to publish done afterward.
			if snap := s.agentOrchestrator.graphSnapshot(parentRunID); snap.LoopState.Status == "stopped" {
				s.flowDiagLog(parentRunID, "flow_done_rejected_stale", "loop went terminal (stopped) during canonical head finalization; not publishing done")
				return FlowControlResult{Status: in.Status, Round: snap.LoopState.Round, Cap: effectiveCap(snap.LoopState), NextAction: "rejected_terminal"}, nil
			}
		}
		// BUG-288 #13 / Task-240 I-2: settle step timeline BEFORE publishing
		// loop=done so consumers never see terminal loop with steps still RUNNING.
		if s.isFlowEngineDriven(parentRunID) {
			s.markFlowRunComplete(context.Background(), parentRunID)
		}
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "done"
			st.OpenIssues = 0
			st.GateReason = ""
			return st
		})
		s.appendPendingAgentContext(parentRunID, strings.TrimSpace("Flow completed. "+in.Summary))
		s.emitAgentGraph(parentRunID, snap)
		// BUG-StaleCancel: persist synchronously here (not go goroutine) so the
		// completed state is guaranteed to land in the NDJSON session file before
		// any subsequent persist (e.g. from a concurrent AnswerQuestion on the
		// same run) can overwrite the status back to "running". The goroutine was
		// cheap but racy: if the runner stopped between the goroutine schedule and
		// execution, the last persisted status was "running" → showed as cancelled
		// on restart.
		s.persistParentSession(parentRunID)

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
		if in.Payload != nil {
			switch v := in.Payload["issues"].(type) {
			case []ReviewIssue:
				issueCount = len(v)
			case []any:
				issueCount = len(v)
			}
		}
		// V9-19: peek whether this continue will hit cap BEFORE mutateLoop, so we
		// can settle hub WAITING before publishing loop=blocked (Task-240 order).
		pre := s.agentOrchestrator.loopStateFor(parentRunID)
		preCap := effectiveCap(pre)
		willHitCap := preCap > 0 && pre.Round+1 >= preCap
		if willHitCap {
			s.setFlowStepAwaitingUser(context.Background(), parentRunID)
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
				// A new loop round is actively running, regardless of the prior
				// reviewer verdict state ("rejected", "blocked", etc.). Leaving
				// stale verdict statuses here prevents the re-entered coder from
				// auto-advancing to the next reviewer cohort when it completes.
				st.Status = "running"
				st.BlockReason = ""
				st.GateReason = ""
				result = FlowControlResult{Status: "continue", Round: st.Round, Cap: cap, OpenIssues: st.OpenIssues, NextAction: "looping"}
			}
			return st
		})
		if result.NextAction == "looping" {
			// BUG-174: a new review round is starting — reset the downstream nodes
			// to PENDING and re-run the re-entry node (e.g. "coder") on the step
			// timeline so the loop reads honestly instead of every node staying DONE.
			//
			// BUG-286: this used to resolve the re-entry node via flowEntryNodeID,
			// which finds the flow's zero-DEPENDENCY entry node (entryDelegateNodes).
			// For a Supabase-mirrored flow, BUG-282's edge-derived DependsOn means
			// "coder" carries DependsOn=["context"] (the forward edge from context),
			// so entryDelegateNodes excludes it — flowEntryNodeID returned "" for
			// every review-loop-shaped flow reached via the "continue" back-edge.
			// The observed effects: setFlowStepStatus(parentRunID, "", RUNNING) was
			// a silent no-op (coder never shown RUNNING — the desktop kept reading
			// it as PENDING through the whole re-run, only flipping to DONE when it
			// actually finished), AND every node including "context" (the flow's
			// once-only entry node, upstream of the loop, never meant to re-run) got
			// wrongly reset to PENDING and then never revisited, leaving it stuck.
			// The correct re-entry node is the "continue" back-edge's own target
			// (resolveContinueBackEdgeTarget, already used elsewhere for this exact
			// purpose), and only nodes FORWARD-reachable from it should reset to
			// PENDING — which naturally excludes an upstream once-only node like
			// "context" while still covering every node in the new round
			// (reviewers, synthesis, and anything chained after it, e.g. hub.notify).
			//
			// BUG-233: this MUST complete before emitAgentGraph below. emitAgentGraph
			// fires the agent_graph_updated SSE event that the desktop reacts to by
			// refreshing the step-runtime snapshot; doing the reset in a goroutine
			// raced that refresh against these writes, so the desktop could read the
			// backend's step store before the new round's PENDING/RUNNING transitions
			// landed and render the stale "all done" snapshot from the prior round.
			if s.isFlowEngineDriven(parentRunID) {
				nodes := s.activeFlowNodesFor(parentRunID)
				edges := s.activeFlowEdgesFor(parentRunID)
				// CP-58 Task-304: the emitter id comes from run state
				// (FlowControlInput carries no node id) so a dual-loop harness
				// routes the hub's continue to ITS loop's writer —
				// plan_synthesis -> plan_writer vs synthesis -> implement —
				// while single-hub flows keep resolving exactly as before.
				hubFrom := s.activeHubNodeIDFor(parentRunID)
				if hubFrom == "" {
					hubFrom = hubInlineNodeID(nodes)
				}
				reentryID, ok := resolveContinueBackEdgeTarget(edges, hubFrom)
				if !ok {
					// Defensive fallback for a looping result with no declared
					// continue back-edge (shouldn't occur for any flow that can
					// legitimately reach NextAction=="looping" today) — preserves
					// the pre-BUG-286 behavior rather than resetting nothing.
					reentryID = flowEntryNodeID(nodes)
				}
				if reentryID != "" {
					// No edges recorded at all (e.g. a minimal/synthetic run whose
					// topology was never populated) means there is nothing to derive
					// reachability from — fall back to the pre-BUG-286 blanket reset
					// (every other node) rather than silently resetting nothing. Once
					// real edges ARE present, trust forwardReachableNodeIDs fully: a
					// node it excludes (e.g. an upstream once-only "context" entry) is
					// a deliberate, correct exclusion, not a gap to paper over.
					var resetIDs map[string]bool
					if len(edges) > 0 {
						resetIDs = forwardReachableNodeIDs(edges, reentryID)
					}
					for _, n := range nodes {
						if n.ID == reentryID {
							continue
						}
						if len(edges) == 0 || resetIDs[n.ID] {
							s.setFlowStepStatus(context.Background(), parentRunID, n.ID, StepStatusPending)
						}
					}
					s.setFlowStepStatus(context.Background(), parentRunID, reentryID, StepStatusRunning)
				}
			}
		} else if result.NextAction == "awaiting_user" && !willHitCap {
			// Cap path already settled WAITING before mutateLoop (V9-19).
			// Non-cap awaiting_user still settles here.
			s.setFlowStepAwaitingUser(context.Background(), parentRunID)
		}
		if result.NextAction == "awaiting_user" {
			// Same freeze as escalate: no hub/child work while Continue form is up.
			// run-45103: when continue hits cap on the same hub turn that just
			// submitted flow_control, do not cancel that parent turn — let it
			// drain cleanly so desktop is not stuck on Thinking.../cancelled.
			// Children and auto-intents still freeze. Escalate/hub_stall keep
			// default cancel-all behavior (preserve only on willHitCap).
			preserveTurnID := ""
			if willHitCap {
				s.mu.Lock()
				if rs := s.runs[parentRunID]; rs != nil &&
					rs.currentTurnID != "" &&
					rs.lastFlowControlTurnID == rs.currentTurnID {
					preserveTurnID = rs.currentTurnID
				}
				s.mu.Unlock()
			}
			s.parkFlowForAwaitingUser(parentRunID, parkFlowForAwaitingUserOptions{
				preserveParentTurnID: preserveTurnID,
			})
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
			// CP-51 A1 residual: durably mark this turn as continue-delegated and
			// clear any hub gate-reprompt intent so restart cannot revive a
			// concurrent hub write while the reinvoked coder is RUNNING.
			s.stampHubContinueDelegatedDurable(parentRunID)
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
		// Task-241 T-8: same reason + no new cohort progress → "no progress" suffix.
		gateReason := strings.TrimSpace(in.Summary)
		if gateReason == "" {
			gateReason = "escalated"
		}
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			// Approximate cohort length via open-cohort presence; use last note as proxy
			// for progress. When re-escalate with identical summary, mark no progress.
			if rs.lastEscalateReason != "" && rs.lastEscalateReason == gateReason {
				gateReason = gateReason + " (no progress since last continue)"
			}
			rs.lastEscalateReason = strings.TrimSpace(in.Summary)
			if rs.lastEscalateReason == "" {
				rs.lastEscalateReason = "escalated"
			}
		}
		s.mu.Unlock()
		// CA-623: if freeze/chain already stamped a specific node (lastEscalated),
		// settle that node instead of the generic helper's audit fallback.
		var escalatedNode string
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			escalatedNode = strings.TrimSpace(rs.lastEscalatedInlineNodeID)
		}
		s.mu.Unlock()
		if escalatedNode != "" {
			s.setFlowStepStatus(context.Background(), parentRunID, escalatedNode, StepStatusWaitingUserApr)
		} else {
			s.setFlowStepAwaitingUser(context.Background(), parentRunID)
		}
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = "escalate" // BUG-231
			st.GateReason = gateReason
			return st
		})
		// run-1675: form "Needs your decision" must freeze the flow — cancel
		// hub/child turns and drop gate-reprompt / hub-reinvoke intents so main
		// cannot keep coding behind the card.
		s.parkFlowForAwaitingUser(parentRunID)
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
		// Unreachable: status enum is validated before the switch (BUG-288 #26).
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

// isMissingChangeAuditNoteReason reports whether a gate reason is the audit
// tier-3 / r-ca missing change-audit-note block (run-202550): "code changed
// but no change-audit note found".
func isMissingChangeAuditNoteReason(reason string) bool {
	return strings.Contains(strings.ToLower(reason), "no change-audit note")
}

// upstreamCodeWriterForNode walks forward edges backwards from nodeID and
// returns the nearest upstream agent.code writer (run-202550: task-harness
// audit <- synthesis <- reviewer <- validate <- implement). Only forward
// edges are followed so continue back-edges cannot loop; "" when none.
func upstreamCodeWriterForNode(edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, nodeID string) string {
	start := strings.TrimSpace(nodeID)
	if start == "" {
		return ""
	}
	visited := map[string]bool{start: true}
	frontier := []string{start}
	for len(frontier) > 0 {
		var next []string
		for _, cur := range frontier {
			for _, e := range edges {
				if !strings.EqualFold(strings.TrimSpace(e.Kind), "forward") {
					continue
				}
				if !strings.EqualFold(strings.TrimSpace(e.To), cur) {
					continue
				}
				from := strings.TrimSpace(e.From)
				if from == "" || visited[from] {
					continue
				}
				visited[from] = true
				if node, ok := findFlowNode(nodes, from); ok {
					if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.code" {
						return from
					}
				}
				next = append(next, from)
			}
		}
		frontier = next
	}
	return ""
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
	prevBlockReason := s.agentOrchestrator.loopStateFor(parentRunID).BlockReason
	prevGateReason := s.agentOrchestrator.loopStateFor(parentRunID).GateReason
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
	_ = prevBlockReason

	// BUG-284: a hub.notify reinvoke that was blocked (re-armed in
	// maybeAutoReinvokeHubWithPrompt) takes priority over the generic
	// synthesis reinvoke below — resuming must retry the SAME node's own
	// prompt, not restart the hub.inline node's review turn again. Without
	// this, activeHubNodeID stays pointed at the notify node while resume
	// fires a generic reinvoke; that reinvoke's eventual "done" then gets
	// misattributed as the notify node's own completion (its edge trivially
	// resolves to the terminal), settling the flow and marking the node DONE
	// without it ever actually running — the message is never sent.
	s.mu.Lock()
	pendingPrompt := ""
	if rs := s.runs[parentRunID]; rs != nil && rs.pendingHubReinvokePrompt != "" {
		pendingPrompt = rs.pendingHubReinvokePrompt
		rs.pendingHubReinvoke = false
		rs.pendingHubReinvokePrompt = ""
	}
	activeHubNodeID := ""
	if rs := s.runs[parentRunID]; rs != nil {
		activeHubNodeID = rs.activeHubNodeID
		// CP-58 run-203966 self-heal: an earlier park cancel may have stamped
		// this parent Cancelled (pre-fix runs, or a restart replaying the
		// poisoned snapshot) while the loop is only PARKED awaiting this very
		// decision. A blocked loop being resumed owns liveness — restore
		// non-terminal so flowRunTerminalLocked does not silently skip every
		// later advance ("run terminal/stopped"). Only Cancelled heals — a
		// Failed root is a real failure, not park poison. Every parked reason
		// goes through parkFlowForAwaitingUser, so any non-empty blocked
		// reason qualifies; a genuinely stopped loop never reaches here
		// (stopAgentLoop seals the loop "stopped", wasBlocked=false).
		if wasBlocked && strings.TrimSpace(prevBlockReason) != "" &&
			rs.status == RunStatusCancelled {
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
		}
		// CP-51 A1 live: hub can retain a stale pendingFlowGateSettle from the
		// entry turn (or a prior incomplete settle) while the real gate lives on
		// the child. User Continue after escalate must not hit startTurn's
		// gate_in_progress reject ("post-turn gate still running").
		s.clearStaleHubPendingGateSettleLocked(rs)
	}
	s.mu.Unlock()

	// BUG-233: settle the hub node before emitAgentGraph (not in a goroutine) —
	// emitAgentGraph fires the SSE event that triggers the desktop's step-runtime
	// refresh, which could otherwise read the store before this write landed and
	// render the pre-Continue (WAITING_USER_APPROVAL / stale-done) snapshot.
	// run-147126: when Continue is retrying an escalated WRITER/audit node, do
	// not flip the hub (synthesis) to RUNNING — that flapped the last two steps
	// while the actual node was still parked. Only hub-targeted resumes stamp it.
	if s.isFlowEngineDriven(parentRunID) {
		hubID := activeHubNodeID
		if hubID == "" {
			hubID = hubInlineNodeID(s.activeFlowNodesFor(parentRunID))
		}
		if hubID != "" {
			escalatedIsHub := false
			if rs := s.runs[parentRunID]; rs != nil {
				esc := strings.TrimSpace(rs.lastEscalatedInlineNodeID)
				escalatedIsHub = esc != "" && esc == hubID
			}
			if escalatedIsHub || pendingPrompt != "" {
				s.setFlowStepStatus(context.Background(), parentRunID, hubID, StepStatusRunning)
			}
		}
	}
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)

	if pendingPrompt != "" {
		go s.maybeAutoReinvokeHubWithPrompt(parentRunID, pendingPrompt)
		return snap, nil
	}

	// BUG-289 A5/F-9: hub-less flows (rag-harness) escalate from inline
	// validate/audit with no hub.inline — Continue must re-enter that node,
	// not reinvoke a nonexistent hub.
	// CA-616: same for a hub-less delegate fail (preflight_contract_plan).
	s.mu.Lock()
	escalatedNodeID := ""
	failedDelegateNodeID := ""
	var edges []agentpack.FlowEdge
	var nodes []agentpack.FlowNode
	if rs := s.runs[parentRunID]; rs != nil {
		escalatedNodeID = strings.TrimSpace(rs.lastEscalatedInlineNodeID)
		if escalatedNodeID != "" {
			edges = rs.activeFlowEdges
			nodes = rs.activeFlowNodes
			rs.lastEscalatedInlineNodeID = ""
		}
		failedDelegateNodeID = strings.TrimSpace(rs.lastFailedDelegateNodeID)
		if failedDelegateNodeID != "" {
			if edges == nil {
				edges = rs.activeFlowEdges
				nodes = rs.activeFlowNodes
			}
			rs.lastFailedDelegateNodeID = ""
		}
	}
	hubInline := hubInlineNodeID(nodes)
	s.mu.Unlock()
	// CA-617 fallback: restart lost RAM field — infer failed delegate from FAILED step with note.
	if failedDelegateNodeID == "" && hubInline == "" && prevBlockReason == "delegate_failed" {
		if steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID); err == nil {
			for _, st := range steps {
				if st.Status == StepStatusFailed && strings.EqualFold(strings.TrimSpace(st.ID), "preflight_contract_plan") {
					failedDelegateNodeID = st.ID
					break
				}
			}
			if failedDelegateNodeID == "" {
				for _, st := range steps {
					if st.Status == StepStatusFailed && strings.TrimSpace(st.RejectionNote) != "" {
						failedDelegateNodeID = st.ID
						break
					}
				}
			}
			if failedDelegateNodeID != "" && len(nodes) == 0 {
				s.mu.Lock()
				if rs := s.runs[parentRunID]; rs != nil {
					nodes = append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
					edges = append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
				}
				s.mu.Unlock()
				hubInline = hubInlineNodeID(nodes)
			}
		}
	}
	if hubInline == "" && escalatedNodeID == "" && failedDelegateNodeID == "" {
		// Still no hub and no remembered node — fall through to generic reinvoke.
	}
	// run-202550: an audit escalate for a missing change-audit note must NOT
	// re-run the audit (the note is still missing → immediate re-escalate
	// "no progress since last continue") and must NOT reinvoke the plan hub.
	// Retry re-enters the nearest upstream agent.code writer with an explicit
	// "write the missing CA note" prompt; the flow then continues through
	// validate → reviewer → synthesis → audit on its own edges.
	if escalatedNodeID != "" && isMissingChangeAuditNoteReason(prevGateReason) {
		if node, ok := findFlowNode(nodes, escalatedNodeID); ok {
			if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "artifact.audit_draft" {
				if writerID := upstreamCodeWriterForNode(edges, nodes, escalatedNodeID); writerID != "" {
					if wnode, ok := findFlowNode(nodes, writerID); ok {
						resumePrompt := "[flow-engine] The audit gate is blocked: code changed but no change-audit note found. Write the missing change-audit note (change-audit/CA-*.md) covering the code you changed this turn, then continue."
						if strings.TrimSpace(feedback) != "" {
							resumePrompt = strings.TrimSpace(feedback) + "\n\n---\n\n" + resumePrompt
						}
						resumePrompt = composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), resumePrompt, wnode)
						if s.reinvokeMatchingFlowChild(parentRunID, resumePrompt, func(child *interactiveRun) bool {
							return child.label == writerID
						}) {
							s.setFlowStepStatus(context.Background(), parentRunID, writerID, StepStatusRunning)
							return snap, nil
						}
					}
				}
			}
		}
		// No writer resolved or no matching child — fall through to the
		// existing audit re-dispatch below rather than stranding the resume.
	}
	// Run-144900: writer parks (agent.code / agent.delegate) must retry the
	// delegate child even on live rag-harness which has hub.inline=synthesis.
	// CA-627's hub-less guard (hubInline == "") is correct for inline helpers
	// like contract.freeze, but for writers it dead-ended at
	// maybeAutoReinvokeHubWithNote and faked a 5-minute synthesis turn.
	if escalatedNodeID != "" {
		if node, ok := findFlowNode(nodes, escalatedNodeID); ok {
			if flowNodeInlineDispatchable(node) {
				go s.tryAdvanceFlowThroughInline(parentRunID, edges, nodes, node, feedback)
				return snap, nil
			}
			// Writer/delegate node (e.g. test_signatures run-144900, implement
			// run-221516): retry the node's own child run via the delegate
			// reinvoke path below — do not fall through to hub reinvoke.
			failedDelegateNodeID = escalatedNodeID
		}
	}
	if failedDelegateNodeID != "" {
		// CA-616: retry the same delegate. Prefer reinvokeMatchingFlowChild so
		// the existing failed child is reset to RUNNING instead of spawning a
		// second child for the same node/activation.
		if node, ok := findFlowNode(nodes, failedDelegateNodeID); ok {
			resumePrompt := "[flow-engine] Retrying failed delegate after user Continue."
			if strings.TrimSpace(feedback) != "" {
				resumePrompt = strings.TrimSpace(feedback) + "\n\n---\n\n" + resumePrompt
			}
			resumePrompt = composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), resumePrompt, node)
			expectedAgent := flowNodeAgentName(node)
			reinvoked := s.reinvokeMatchingFlowChild(parentRunID, resumePrompt, func(child *interactiveRun) bool {
				if child.label == failedDelegateNodeID {
					return true
				}
				if strings.TrimSpace(child.label) == "" && expectedAgent != "" && strings.EqualFold(strings.TrimSpace(child.agentName), expectedAgent) {
					return true
				}
				return false
			})
			if reinvoked {
				s.setFlowStepStatus(context.Background(), parentRunID, failedDelegateNodeID, StepStatusRunning)
				return snap, nil
			}
			// M6: if a child with same label/agent already exists but reinvoke missed
			// (e.g. label empty pre-adapter), do not spawn a second one.
			for _, cid := range s.agentOrchestrator.listChildren(parentRunID) {
				if c := s.runs[cid]; c != nil {
					matchesLabel := c.label == failedDelegateNodeID
					matchesAgent := expectedAgent != "" && strings.EqualFold(strings.TrimSpace(c.agentName), expectedAgent) && strings.TrimSpace(c.label) == ""
					if (matchesLabel || matchesAgent) && c.status == RunStatusRunning {
						s.setFlowStepStatus(context.Background(), parentRunID, failedDelegateNodeID, StepStatusRunning)
						return snap, nil
					}
				}
			}
			// No reusable child — fall through to spawn below (generic path
			// would also miss the node; retry as a fresh spawn).
			agentName := expectedAgent
			if agentName != "" {
				agentDef, _ := resolvePackAgentDefinition(agentName)
				prompt := composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), "[flow-engine] Retrying failed delegate after user Continue.", node)
				if feedback != "" {
					prompt = feedback + "\n\n---\n\n" + prompt
				}
				if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
					Agent:            agentName,
					Prompt:           prompt,
					Wait:             false,
					Label:            node.ID,
					AutoOrchestrate:  true,
					AgentDefOverride: agentDef,
					Model:            s.delegateSpawnModel(context.Background(), parentRunID, node),
				}); err == nil {
					s.setFlowStepStatus(context.Background(), parentRunID, failedDelegateNodeID, StepStatusRunning)
					s.stampFlowNodePosture(context.Background(), parentRunID, node)
					return snap, nil
				}
			}
		}
	}

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
		switch e.Status {
		case "failed":
			fmt.Fprintf(&b, "%q (%s): failed: %s\n", label, e.Provider, e.Err)
		case "cancelled":
			// Task-241: cancelled is terminal at the barrier (I-11 / T-2).
			fmt.Fprintf(&b, "%q (%s): cancelled\n", label, e.Provider)
		default:
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

// ensureCohortExpectedLocked restores cohortExpected from live sibling runs
// when RAM lost the count (run-98153: 2/2 buffered, join never fired).
// Caller holds s.mu.
func (s *InteractiveService) ensureCohortExpectedLocked(parentRunID, cohortID string) {
	parentRunID = strings.TrimSpace(parentRunID)
	cohortID = strings.TrimSpace(cohortID)
	if parentRunID == "" || cohortID == "" {
		return
	}
	known := 0
	for _, r := range s.runs {
		if r == nil {
			continue
		}
		if r.parentRunID == parentRunID && strings.TrimSpace(r.flowCohortId) == cohortID {
			known++
		}
	}
	if known == 0 {
		return
	}
	before := s.agentOrchestrator.cohortExpectedCount(parentRunID, cohortID)
	s.agentOrchestrator.inferCohortExpectedIfMissing(parentRunID, cohortID, known)
	if before == 0 {
		s.flowDiagLog(parentRunID, "cohort_expected_inferred", "restored cohort expected from live siblings",
			"cohort_id", cohortID,
			"known_members", known,
		)
	}
}

func (s *InteractiveService) ensureCohortExpected(parentRunID, cohortID string) {
	s.mu.Lock()
	s.ensureCohortExpectedLocked(parentRunID, cohortID)
	s.mu.Unlock()
}

// hubShouldSkipProseEscalate reports whether BUG-226's "no submit_review_outcome
// → escalate" fallback must not run: an open reviewer cohort or any live child
// turn means the loop is still advancing after continue/spawn — free prose from
// a gate/hub turn must not freeze the flow and cancel those children (run-5296).
func (s *InteractiveService) hubShouldSkipProseEscalate(parentRunID string) bool {
	if parentRunID == "" || s == nil {
		return false
	}
	if s.agentOrchestrator.hasOpenCohort(parentRunID) {
		return true
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		child := s.runs[childID]
		if child == nil {
			s.mu.Unlock()
			continue
		}
		inFlight := child.turnInFlight || child.status == RunStatusRunning ||
			child.status == RunStatusWaitingApproval || child.status == RunStatusWaitingQuestion ||
			child.pendingFlowGateSettle || child.postTurnGateCancel != nil
		s.mu.Unlock()
		if inFlight {
			return true
		}
	}
	return false
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

// parkFlowForAwaitingUserOptions tunes freeze behavior for one park call.
// Zero value preserves historical run-1675 semantics (cancel every in-flight turn).
type parkFlowForAwaitingUserOptions struct {
	// preserveParentTurnID, when equal to the hub's currentTurnID, skips cancelling
	// that parent turn so a same-turn flow_control decision can drain cleanly
	// (run-45103: continue-at-cap must not self-cancel the submitting synthesis turn).
	// Children and auto-intents are still frozen.
	preserveParentTurnID string
}

// parkFlowForAwaitingUser freezes hub + children when a human decision surface
// is up (escalate / cap / hub_stalled Continue form — run-1675).
//
// Invariant: while loop.Status == "blocked", no new turns and no in-flight
// provider work. Clears auto-continuation intents (gate reprompt, hub reinvoke,
// resume) that would otherwise startTurn behind the form, and cancels live
// turnCancel so tool calls stop — except the optional preserveParentTurnID turn.
func (s *InteractiveService) parkFlowForAwaitingUser(parentRunID string, opts ...parkFlowForAwaitingUserOptions) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	var opt parkFlowForAwaitingUserOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if parent := s.runs[parentRunID]; parent != nil {
		parent.reinvokeInFlight = false
		parent.pendingHubReinvoke = false
		parent.pendingHubReinvokePrompt = ""
		parent.pendingGateRepromptPrompt = ""
		parent.pendingGateRepromptStepID = ""
		parent.pendingGateRepromptGen = 0
		parent.pendingGateRepromptDeliveredGen = 0
		parent.pendingGateRepromptAcceptedTurn = ""
		parent.pendingResumePrompt = ""
		parent.pendingResumeStepID = ""
		parent.pendingResumeGen = 0
		parent.pendingResumeDeliveredGen = 0
		parent.pendingResumeAcceptedTurn = ""
		// Drop settle so finishTurn/gate cannot re-queue a reprompt after cancel.
		parent.pendingFlowGateSettle = false
		parent.pendingFlowGateFinalMsg = ""
		parent.pendingFlowGateOccurredAt = ""
		parent.pendingFlowGateTurnID = ""
		parent.pendingGateChangedFiles = nil
		preserveParent := opt.preserveParentTurnID != "" &&
			parent.currentTurnID != "" &&
			parent.currentTurnID == opt.preserveParentTurnID
		if parent.turnInFlight && parent.turnCancel != nil && !preserveParent {
			// CP-58 run-203966: this is the ENGINE parking, not a user
			// interrupt — the cancelled turn must not terminalize the run (see
			// parkCancelCause / finishTurn's context.Canceled branch).
			// parkCancelSuppress is armed together so an adapter that emits
			// EventTurnFailed mid-cancel does not abandon canonical heads or
			// signalChild before finishTurn even runs.
			parent.parkCancelCause = true
			parent.parkCancelSuppress = true
			parent.turnCancel()
		}
		if parent.postTurnGateCancel != nil {
			parent.postTurnGateCancel()
			parent.postTurnGateCancel = nil
		}
		if parent.flowInlineCancel != nil {
			parent.flowInlineCancel()
			parent.flowInlineCancel = nil
			parent.flowInlineCtx = nil
		}
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil {
			continue
		}
		child.pendingTurnPrompt = ""
		child.pendingGateRepromptPrompt = ""
		child.pendingGateRepromptStepID = ""
		child.pendingGateRepromptGen = 0
		child.pendingResumePrompt = ""
		child.pendingResumeStepID = ""
		child.pendingResumeGen = 0
		child.pendingFlowGateSettle = false
		child.pendingFlowGateFinalMsg = ""
		child.pendingFlowGateOccurredAt = ""
		child.pendingFlowGateTurnID = ""
		child.pendingGateChangedFiles = nil
		if child.turnInFlight && child.turnCancel != nil {
			child.turnCancel()
		}
		if child.postTurnGateCancel != nil {
			child.postTurnGateCancel()
			child.postTurnGateCancel = nil
		}
		if child.status == RunStatusRunning {
			child.status = RunStatusWaitingUserApr
			child.agentStatus = "waiting_user_approval"
			if existing, ok := s.agentOrchestrator.currentSummary(parentRunID, child.id); ok {
				existing.Status = RunStatusWaitingUserApr
				existing.AgentStatus = "waiting_user_approval"
				s.agentOrchestrator.upsertSummary(parentRunID, existing)
			} else {
				s.agentOrchestrator.upsertSummary(parentRunID, AgentRunSummary{
					RunID:       child.id,
					ParentRunID: parentRunID,
					AgentName:   child.agentName,
					Role:        child.role,
					Status:      RunStatusWaitingUserApr,
					AgentStatus: "waiting_user_approval",
					Label:       child.label,
				})
			}
		}
	}
	s.flowDiagLog(parentRunID, "flow_parked_awaiting_user",
		"flow frozen for human decision form; cancelled in-flight turns and dropped auto-intents")
}

// parkFlowForAwaitingUserLocked is like parkFlowForAwaitingUser but caller
// already holds s.mu (e.g. notifyHubOfFlowChildFailureLocked via emitLocked).
// CA-616 uses it for hub-less delegate fail to avoid deadlock (park would
// Lock again while emitLocked holds s.mu).
func (s *InteractiveService) parkFlowForAwaitingUserLocked(parentRunID string) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	if parent := s.runs[parentRunID]; parent != nil {
		parent.reinvokeInFlight = false
		parent.pendingHubReinvoke = false
		parent.pendingHubReinvokePrompt = ""
		parent.pendingGateRepromptPrompt = ""
		parent.pendingGateRepromptStepID = ""
		parent.pendingGateRepromptGen = 0
		parent.pendingGateRepromptDeliveredGen = 0
		parent.pendingGateRepromptAcceptedTurn = ""
		parent.pendingResumePrompt = ""
		parent.pendingResumeStepID = ""
		parent.pendingResumeGen = 0
		parent.pendingResumeDeliveredGen = 0
		parent.pendingResumeAcceptedTurn = ""
		parent.pendingFlowGateSettle = false
		parent.pendingFlowGateFinalMsg = ""
		parent.pendingFlowGateOccurredAt = ""
		parent.pendingFlowGateTurnID = ""
		parent.pendingGateChangedFiles = nil
		if parent.turnInFlight && parent.turnCancel != nil {
			// CP-58 run-203966: same park-cancel contract as
			// parkFlowForAwaitingUser (cause + suppress armed together).
			parent.parkCancelCause = true
			parent.parkCancelSuppress = true
			parent.turnCancel()
		}
		if parent.postTurnGateCancel != nil {
			parent.postTurnGateCancel()
			parent.postTurnGateCancel = nil
		}
		if parent.flowInlineCancel != nil {
			parent.flowInlineCancel()
			parent.flowInlineCancel = nil
			parent.flowInlineCtx = nil
		}
	}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil {
			continue
		}
		child.pendingTurnPrompt = ""
		child.pendingGateRepromptPrompt = ""
		child.pendingGateRepromptStepID = ""
		child.pendingGateRepromptGen = 0
		child.pendingResumePrompt = ""
		child.pendingResumeStepID = ""
		child.pendingResumeGen = 0
		child.pendingFlowGateSettle = false
		child.pendingFlowGateFinalMsg = ""
		child.pendingFlowGateOccurredAt = ""
		child.pendingFlowGateTurnID = ""
		child.pendingGateChangedFiles = nil
		if child.turnInFlight && child.turnCancel != nil {
			child.turnCancel()
		}
		if child.postTurnGateCancel != nil {
			child.postTurnGateCancel()
			child.postTurnGateCancel = nil
		}
		if child.status == RunStatusRunning {
			child.status = RunStatusWaitingUserApr
			child.agentStatus = "waiting_user_approval"
			if existing, ok := s.agentOrchestrator.currentSummary(parentRunID, child.id); ok {
				existing.Status = RunStatusWaitingUserApr
				existing.AgentStatus = "waiting_user_approval"
				s.agentOrchestrator.upsertSummary(parentRunID, existing)
			} else {
				s.agentOrchestrator.upsertSummary(parentRunID, AgentRunSummary{
					RunID:       child.id,
					ParentRunID: parentRunID,
					AgentName:   child.agentName,
					Role:        child.role,
					Status:      RunStatusWaitingUserApr,
					AgentStatus: "waiting_user_approval",
					Label:       child.label,
				})
			}
		}
	}
	s.flowDiagLog(parentRunID, "flow_parked_awaiting_user",
		"flow frozen for human decision form; cancelled in-flight turns and dropped auto-intents")
}

// settleChildStatusAfterGateBlockLocked decides a child's status right after a
// post-turn gate block/reprompt settle: keep WAITING_USER_APPROVAL when the
// parent flow loop is already blocked (a park stamp — run-218125/221516) instead
// of resetting to Running, which would keep the TUI Thinking timer on and hide
// the Continue/Stop chips. The decision is also pushed to the orchestrator
// summary the TUI/desktop read (preserving all other fields, e.g. ActivationSeq)
// so a settle-only path never leaves the graph stale. Caller holds s.mu and the
// gate is NOT mid-stop.
func (s *InteractiveService) settleChildStatusAfterGateBlockLocked(rs *interactiveRun) {
	if rs == nil {
		return
	}
	if rs.parentRunID != "" && s.agentOrchestrator.loopStateFor(rs.parentRunID).Status == "blocked" {
		rs.status = RunStatusWaitingUserApr
		rs.agentStatus = "waiting_user_approval"
	} else {
		rs.status = RunStatusRunning
		rs.agentStatus = string(RunStatusRunning)
	}
	if rs.parentRunID == "" {
		return
	}
	if existing, ok := s.agentOrchestrator.currentSummary(rs.parentRunID, rs.id); ok {
		existing.Status = rs.status
		existing.AgentStatus = rs.agentStatus
		s.agentOrchestrator.upsertSummary(rs.parentRunID, existing)
	}
}

// clearStaleHubPendingGateSettleLocked drops hub pendingFlowGateSettle when no
// live post-turn gate is running. Used after flowStartOnly synthetic complete,
// child fail reinvoke (CA-355 / H-A), and entry spawn failure (H-B).
// Caller holds s.mu.
func (s *InteractiveService) clearStaleHubPendingGateSettleLocked(parent *interactiveRun) {
	if parent == nil {
		return
	}
	if parent.pendingFlowGateSettle && parent.postTurnGateCancel == nil {
		parent.pendingFlowGateSettle = false
		parent.pendingFlowGateFinalMsg = ""
		parent.pendingFlowGateOccurredAt = ""
		parent.pendingFlowGateTurnID = ""
		parent.pendingGateChangedFiles = nil
	}
}

// notifyHubOfFlowChildFailureLocked settles a flow-engine non-cohort child
// failure onto the hub: step FAILED, clear stale hub settle, append fail note,
// schedule hub reinvoke when the explicit loop is still advancing.
// Caller holds s.mu. child.parentRunID must be set.
//
// Covers EventTurnFailed (CA-355/run-1618) and pre-adapter startTurn fail (H-A).
//
// CA-616: hub-less flows (rag-harness, no hub.inline) must NOT reinvoke the
// hub on a delegate fail — there is no hub to reinvoke — and must not stay
// "rejected" with Thinking forever. Instead the failed delegate is marked
// FAILED with its RejectionNote, the loop is parked blocked/delegate_failed,
// and the user gets [Continue]/[Stop]. Review-loop (with hub.inline) keeps
// the original reinvoke path unchanged so CA-355's tests stay green.
func (s *InteractiveService) notifyHubOfFlowChildFailureLocked(child *interactiveRun, errMsg string) {
	if child == nil || child.parentRunID == "" {
		return
	}
	parent := s.runs[child.parentRunID]
	if parent == nil || !parent.flowEngineDriven {
		return
	}
	// run-43831: failed child is still flow progress (cohort join / hub reinvoke may
	// follow). Stamp before any async reinvoke so F-0 does not treat the handoff gap as root idleness.
	s.touchParentHubProgressFromChildLocked(child)
	failNote := fmt.Sprintf(
		"Sub-agent %q (provider: %s) failed: %s",
		child.agentName, child.providerKey, truncateDisplayField(errMsg, 500))
	if child.label != "" {
		s.setFlowStepFailedWithReasonLocked(context.Background(), child.parentRunID, child.label, errMsg)
	}
	s.clearStaleHubPendingGateSettleLocked(parent)
	s.appendPendingAgentContextLocked(child.parentRunID, failNote)
	if s.agentOrchestrator.loopMode(child.parentRunID) != "explicit" || !s.loopIsAdvancing(child.parentRunID) {
		return
	}
	hubNodeID := parent.activeHubNodeID
	if hubNodeID == "" {
		hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
	}
	// CA-616 hub-less branch: no hub.inline → park, don't reinvoke.
	if hubNodeID == "" {
		if child.label != "" {
			parent.lastFailedDelegateNodeID = child.label
		}
		snap := s.agentOrchestrator.mutateLoop(child.parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = "delegate_failed"
			st.GateReason = truncateDisplayField(errMsg, 500)
			return st
		})
		s.parkFlowForAwaitingUserLocked(child.parentRunID)
		s.emitAgentGraphLocked(child.parentRunID, snap)
		go s.persistParentSession(child.parentRunID)
		s.flowDiagLog(child.parentRunID, "delegate_failed_parked",
			"hub-less delegate failed; parked blocked/delegate_failed for Continue/Stop",
			"child_run_id", child.id,
			"label", child.label,
			"error", truncateDisplayField(errMsg, 500),
		)
		return
	}
	s.setFlowStepStatusLocked(context.Background(), child.parentRunID, hubNodeID, StepStatusRunning)
	parentRunID := child.parentRunID
	capturedFailNote := failNote
	s.flowDiagLog(parentRunID, "entry_or_delegate_failed_reinvoke_hub",
		"non-cohort flow child failed; reinvoking hub with failure note",
		"child_run_id", child.id,
		"label", child.label,
		"error", truncateDisplayField(errMsg, 500),
	)
	go s.maybeAutoReinvokeHubWithNote(parentRunID, capturedFailNote)
}

// handleChildStartTurnFailure is the spawnChildRun async startTurn error path:
// pre-adapter failure never emits EventTurnFailed, so cohort join / non-cohort
// hub reinvoke must be driven here (BUG-289 H2/F-2 + H-A residual).
func (s *InteractiveService) handleChildStartTurnFailure(childRunID, parentRunID, errMsg string) {
	s.agentOrchestrator.signalChild(childRunID, "", true, errMsg, RunStatusFailed)
	s.mu.Lock()
	child := s.runs[childRunID]
	cohortID := ""
	label := ""
	provider := ""
	if child != nil {
		cohortID = child.flowCohortId
		label = child.label
		provider = string(child.providerKey)
		child.status = RunStatusFailed
		child.agentStatus = string(RunStatusFailed)
		// run-43831: stamp hub progress under s.mu before unlock / cohort join /
		// async reinvoke so pre-adapter start failures do not leave a stale
		// hubLastProgressAt gap (Codex review of residual F-0).
		s.touchParentHubProgressFromChildLocked(child)
		if child.parentRunID != "" && child.flowCohortId != "" && label != "" {
			if parent := s.runs[child.parentRunID]; parent != nil && parent.flowEngineDriven {
				s.setFlowStepStatusLocked(context.Background(), child.parentRunID, label, StepStatusFailed)
			}
		}
	}
	if cohortID != "" {
		s.mu.Unlock()
		// BUG-289 H2/F-2: pre-flight failure never hit appendCohortResult
		// (only turn-completed/failed handlers do). Stall sweep skips
		// members with last.IsZero() && !inFlight, so the barrier hung
		// forever. Buffer FAILED and re-check cohortComplete.
		s.agentOrchestrator.appendCohortResult(parentRunID, cohortID, cohortEntry{
			Label:    label,
			Provider: provider,
			Status:   "failed",
			Err:      errMsg,
		})
		s.ensureCohortExpected(parentRunID, cohortID)
		if s.agentOrchestrator.cohortComplete(parentRunID, cohortID) {
			entries := s.agentOrchestrator.drainCohort(parentRunID, cohortID)
			note := buildCohortNote(parentRunID, cohortID, entries, s.agentOrchestrator.graphSnapshot(parentRunID).LoopState.Round)
			// BUG-307: a pre-flight child failure can complete the cohort after
			// the loop already sealed (stopped/done) — see loopSealedForReinvoke.
			if !s.loopSealedForReinvoke(parentRunID) {
				s.appendPendingAgentContext(parentRunID, note)
			}
			s.mu.Lock()
			if parent := s.runs[parentRunID]; parent != nil {
				parent.lastCohortNote = note
				hubNodeID := parent.activeHubNodeID
				if hubNodeID == "" {
					hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
				}
				// CP-58 Task-304: a failed member can be the join that completes
				// a dual-hub flow's cohort — stamp the hub the cohort feeds
				// into, not first-match hub.inline (single-hub flows unchanged).
				joined := make([]string, 0, len(entries))
				for _, e := range entries {
					if e.Label != "" {
						joined = append(joined, e.Label)
					}
				}
				if resolved, dual := hubNodeIDForCohortJoin(parent.activeFlowNodes, parent.activeFlowEdges, joined); dual && resolved != "" {
					hubNodeID = resolved
					parent.activeHubNodeID = resolved
				}
				if hubNodeID != "" && s.loopIsAdvancing(parentRunID) {
					s.setFlowStepStatusLocked(context.Background(), parentRunID, hubNodeID, StepStatusRunning)
				}
			}
			s.mu.Unlock()
			go s.maybeAutoReinvokeHubWithNote(parentRunID, note)
		} else {
			s.maybeScheduleStallCheck(parentRunID)
		}
		return
	}
	// Non-cohort pre-flight fail (H-A).
	if child != nil {
		if parent := s.runs[parentRunID]; parent != nil && parent.flowEngineDriven {
			s.notifyHubOfFlowChildFailureLocked(child, errMsg)
		} else if child.uiInitiated || !child.waitForResult {
			s.appendPendingAgentContextLocked(parentRunID, fmt.Sprintf(
				"Sub-agent %q (provider: %s) failed: %s",
				child.agentName, child.providerKey, truncateDisplayField(errMsg, 500)))
		}
		s.agentOrchestrator.upsertSummary(parentRunID, AgentRunSummary{
			RunID:         child.id,
			AgentName:     child.agentName,
			Label:         child.label,
			Role:          child.role,
			Status:        RunStatusFailed,
			ParentRunID:   parentRunID,
			CreatedAt:     child.createdAt,
			DependsOn:     append([]string(nil), child.dependsOn...),
			AgentStatus:   string(RunStatusFailed),
			ProviderKey:   string(child.providerKey),
			ModelName:     child.modelName,
			WaitForResult: child.waitForResult,
			ActivationSeq: child.activationSeq,
		})
		s.emitAgentGraphLocked(parentRunID, s.agentOrchestrator.transition(parentRunID, "rejected"))
	}
	s.mu.Unlock()
}

// notifyHubOfFlowEntrySpawnFailure is H-B: every flow entry spawn failed before
// a child run existed — hub would otherwise stay "active" with zero agents.
// Clears stale settle and reinvokes hub with the failure note (best-effort).
func (s *InteractiveService) notifyHubOfFlowEntrySpawnFailure(parentRunID, flowRef string, failedLabels []string, errMsg string) {
	if parentRunID == "" {
		return
	}
	labels := strings.Join(failedLabels, ", ")
	if labels == "" {
		labels = "(none)"
	}
	note := fmt.Sprintf(
		"[flow-engine] Failed to start flow entry node(s) for %q [%s]: %s",
		flowRef, labels, truncateDisplayField(errMsg, 500))
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil {
		s.mu.Unlock()
		return
	}
	// Entry steps may already be FAILED from the spawn loop; still clear settle.
	s.clearStaleHubPendingGateSettleLocked(parent)
	s.appendPendingAgentContextLocked(parentRunID, note)
	shouldReinvoke := parent.autoOrchestrate && parent.flowEngineDriven &&
		s.agentOrchestrator.loopMode(parentRunID) == "explicit" && s.loopIsAdvancing(parentRunID)
	if shouldReinvoke {
		hubNodeID := parent.activeHubNodeID
		if hubNodeID == "" {
			hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
		}
		if hubNodeID != "" {
			s.setFlowStepStatusLocked(context.Background(), parentRunID, hubNodeID, StepStatusRunning)
		}
	}
	s.mu.Unlock()
	s.flowDiagLog(parentRunID, "flow_start_all_entries_failed",
		"all flow entry spawns failed; reinvoking hub with failure note",
		"flow_ref", flowRef,
		"failed_labels", labels,
		"error", truncateDisplayField(errMsg, 500),
		"will_reinvoke", shouldReinvoke,
	)
	if shouldReinvoke {
		go s.maybeAutoReinvokeHubWithNote(parentRunID, note)
	}
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
		if parent != nil && parent.autoOrchestrate && strings.TrimSpace(cohortNote) != "" &&
			!s.loopSealedForReinvoke(parentRunID) {
			// A note-bearing cohort join can race with a previously scheduled empty
			// hub reinvoke. Do not drop the joined note just because the single-flight
			// guard is closed; the scheduled/current turn will either drain it, or a
			// deferred reinvoke below will consume it on the next turn.
			// BUG-275: join path often already appended this note — avoid a second copy.
			// BUG-307: unless the loop already sealed (stopped/done) — then no
			// legitimate turn will ever drain it; see loopSealedForReinvoke.
			if !pendingAgentContextContains(parent.pendingAgentContext, cohortNote) {
				s.appendPendingAgentContextLocked(parentRunID, cohortNote)
			}
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
	touchHubProgressLocked(parent)
	// BUG-275: join handler already put cohortNote in pendingAgentContext, and
	// we embed the same note in the scheduled prompt. Remove one exact copy so
	// startTurn's composeAgentContextBlock does not also wrap it under
	// "[FlowPilot system note — sub-agents...]" (duplicate joined note).
	if strings.TrimSpace(cohortNote) != "" {
		parent.pendingAgentContext = removeOnePendingAgentContext(parent.pendingAgentContext, cohortNote)
	}
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
	s.maybeScheduleHubStallCheck(parentRunID)
}

// maybeAutoReinvokeHubWithPrompt (Task-235) is maybeAutoReinvokeHubWithNote's
// sibling for a hub.notify node: it reinvokes the SAME hub session for one more
// turn using a FULLY custom prompt (not cohortNote + the generic
// autoReinvokePromptText() "synthesize the join note" tail, which does not fit
// a notify node — there is no join note here). It duplicates the single-flight/
// loop-status/cap guard block verbatim rather than refactoring
// maybeAutoReinvokeHubWithNote to take an optional full-prompt override, since
// that function is exercised by every existing review-loop-shaped flow
// (BUG-275/BUG-234/etc.) and GitNexus impact analysis was unavailable this
// session to safely verify a shared-code-path edit's blast radius; a small,
// additive sibling carries zero risk to that path.
//
// BUG-284: a hub.notify "done" is observed via SubmitFlowControl — i.e. from
// INSIDE the very turn that is finishing — so turnInFlight is essentially
// ALWAYS true here; deferring is the common case, not a rare race. Unlike
// maybeAutoReinvokeHubWithNote's defer (whose retry only needs to re-fire the
// generic synthesis reinvoke, since its content already lives in
// pendingAgentContext), this prompt is NOT threaded through pendingAgentContext
// at all, so it must be remembered verbatim: stashed in
// parent.pendingHubReinvokePrompt, consumed by the SAME turn-completion retry
// site that already drains pendingHubReinvoke (runTurn) — see that site for
// the dispatch-the-right-function half of this fix.
func (s *InteractiveService) maybeAutoReinvokeHubWithPrompt(parentRunID, prompt string) {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil || !parent.autoOrchestrate || parent.reinvokeInFlight || parent.turnInFlight {
		// BUG-289 M7/F-1: also re-arm when reinvokeInFlight && !turnInFlight
		// (scheduled-but-not-started window). The old predicate only covered
		// turnInFlight && !reinvokeInFlight, so a notify prompt arriving in
		// that window was dropped while the notify step still swept DONE.
		if parent != nil && parent.autoOrchestrate {
			if (parent.turnInFlight && !parent.reinvokeInFlight) ||
				(parent.reinvokeInFlight && !parent.turnInFlight) {
				parent.pendingHubReinvoke = true
				parent.pendingHubReinvokePrompt = prompt
			}
		}
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_notify_reinvoke_deferred", "hub.notify reinvoke was deferred or skipped by current state",
			"has_parent", parent != nil,
		)
		return
	}
	st := s.agentOrchestrator.loopStateFor(parentRunID)
	switch st.Status {
	case "paused", "stopped", "blocked":
		// BUG-284 follow-up: re-arm rather than silently drop. The turn that
		// dispatched hub.notify can ALSO call escalate()/pause in the same turn
		// (observed live — see the Status/NextAction fix above), so the loop can
		// already be blocked by the time this deferred retry actually runs.
		// Without re-arming here, resumeFlowWithFeedback's own unblock path has
		// no way to know a hub.notify send is still owed, resumes the GENERIC
		// synthesis reinvoke instead, and — because activeHubNodeID is still
		// pointed at the notify node from dispatchHubNotifyNode — a later,
		// unrelated "done" call gets misattributed as that node's own
		// completion: the flow settles and the node is swept to DONE without
		// ever actually running, and no message is ever sent.
		parent.pendingHubReinvoke = true
		parent.pendingHubReinvokePrompt = prompt
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_notify_reinvoke_blocked", "hub.notify reinvoke blocked by loop status; re-armed for resume")
		return
	case "done":
		// The whole flow already settled through some other path; nothing to
		// resume into, so no point re-arming.
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_notify_reinvoke_blocked", "hub.notify reinvoke blocked: loop already done")
		return
	}
	if st.Round >= effectiveCap(st) {
		parent.pendingHubReinvoke = true
		parent.pendingHubReinvokePrompt = prompt
		s.mu.Unlock()
		s.flowDiagLog(parentRunID, "hub_notify_reinvoke_cap_blocked", "hub.notify reinvoke blocked by round cap; re-armed for resume")
		return
	}
	stepID := s.nextID("step")
	parent.reinvokeInFlight = true
	touchHubProgressLocked(parent)
	s.mu.Unlock()
	s.flowDiagLog(parentRunID, "hub_notify_reinvoke_scheduled", "hub.notify reinvoke scheduled", "step_id", stepID)
	s.scheduleChildTurn(parentRunID, stepID, prompt)
	s.maybeScheduleHubStallCheck(parentRunID)
}

func pendingAgentContextContains(notes []string, note string) bool {
	want := strings.TrimSpace(note)
	if want == "" {
		return false
	}
	for _, n := range notes {
		if strings.TrimSpace(n) == want {
			return true
		}
	}
	return false
}

// removeOnePendingAgentContext drops the first note equal to target (trimmed).
func removeOnePendingAgentContext(notes []string, target string) []string {
	want := strings.TrimSpace(target)
	if want == "" || len(notes) == 0 {
		return notes
	}
	out := make([]string, 0, len(notes))
	removed := false
	for _, n := range notes {
		if !removed && strings.TrimSpace(n) == want {
			removed = true
			continue
		}
		out = append(out, n)
	}
	return out
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
	// V9-28: if a reviewer/delegate cohort is still open, do not synthesize the
	// hub early (double synthesis with the join-path reinvoke).
	if s.agentOrchestrator.hasOpenCohort(parentRunID) {
		s.flowDiagLog(parentRunID, "hub_reinvoke_skipped_open_cohort",
			"advanceOrNotifyHub skipped hub reinvoke while cohort still open",
			"completed_label", completedLabel,
		)
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
		// CP-58 Task-304: source-aware re-entry — activeHubNodeID names the
		// hub-driven node whose continue this is (fallback first-match hub for
		// single-hub flows keeps pre-CP-58 behavior).
		hubFrom := parent.activeHubNodeID
		if hubFrom == "" {
			hubFrom = hubInlineNodeID(parent.activeFlowNodes)
		}
		targetNodeID, _ = resolveContinueBackEdgeTarget(parent.activeFlowEdges, hubFrom)
		if targetNodeID != "" {
			targetNode, hasTargetNode = findFlowNode(parent.activeFlowNodes, targetNodeID)
		}
		flowDriven = parent.flowEngineDriven
	}
	cwd := ""
	if parent := s.runs[parentRunID]; parent != nil {
		cwd = parent.workspaceCwd
	}
	// Snapshot node under lock; compose prompts after unlock (filesystem I/O).
	composeNode := targetNode
	composeOK := hasTargetNode
	reuseChild := hasTargetNode && flowNodeReusesChild(targetNode)
	s.mu.Unlock()
	if composeOK {
		// Task-223: re-attach INPUT read + OUTPUT write-contract on continue re-entry.
		// Task-247 / CP-50 P-4: also re-attach change.contract so continue/reprompt
		// turns still see the declared scope (not only first entry via flow_executor).
		prompt = composeFlowNodeAgentPrompt(cwd, prompt, composeNode)
		prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
	}
	spawnContinueChild := func() {
		agentName := flowNodeAgentName(composeNode)
		if agentName == "" {
			return
		}
		// run-198699 review (CA-732 finding 4): the continue spawn must carry
		// the same rendered FlowContextPackage + FCP provenance as the
		// forward-entry spawn (runContextProduceNode / startInlineEntryChain),
		// otherwise a Retry-continue onto a never-spawned plan_writer starts
		// the writer without the context package the S2 handoff expects.
		p := prompt
		fcpProvenanceRunID := ""
		if pkg, ok := s.loadPlanContextPackage(context.Background(), parentRunID); ok {
			p = renderFlowContextPromptWithSecret(context.Background(), pkg, p, s.markerSecret)
			fcpProvenanceRunID = pkg.WorkflowRunID
			if fcpProvenanceRunID == "" {
				fcpProvenanceRunID = pkg.PackageID
			}
		}
		agentDef, _ := resolvePackAgentDefinition(agentName)
		if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
			Agent:                    agentName,
			Prompt:                   p,
			Wait:                     false,
			Label:                    composeNode.ID,
			AutoOrchestrate:          true,
			AgentDefOverride:         agentDef,
			Model:                    s.delegateSpawnModel(context.Background(), parentRunID, composeNode),
			FCPMarkerProvenanceRunID: fcpProvenanceRunID,
		}); err != nil {
			log.Printf("[flow-executor] continue: spawn node %q (agent %q) failed: %v", composeNode.ID, agentName, err)
			return
		}
		if flowDriven {
			s.setFlowStepStatus(context.Background(), parentRunID, composeNode.ID, StepStatusRunning)
			s.stampFlowNodePosture(context.Background(), parentRunID, composeNode)
		}
	}
	if composeOK && !reuseChild {
		spawnContinueChild()
		return
	}
	// run-198699: a continue landing on a reinvoke-lifecycle agent.delegate with
	// no prior child (hub_stalled parked the flow before the writer ever ran —
	// task-harness plan_writer) used to silently no-op and hub_stalled again.
	// Spawn a fresh child so the back-edge actually fires. Scoped to
	// agent.delegate: agent.code writers go through the frozen-contract path and
	// their fixture tests must not gain a cascade here.
	if composeOK && reuseChild {
		if canonical, ok := agentpack.NormalizeBehaviorID(composeNode.Behavior); ok && canonical == "agent.delegate" {
			if s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
				if targetNodeID != "" {
					return child.label == targetNodeID
				}
				return isCoderRun(child)
			}) {
				return
			}
			spawnContinueChild()
			return
		}
	}
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

// loopSealedForReinvoke reports whether parentRunID's loop has permanently
// ended: "stopped" (explicit user Stop) or "done" (the flow's own decision
// loop finished, BUG-302). Unlike loopIsAdvancing, "blocked"/"paused" are NOT
// sealed here — those are temporary holds a later Continue/Resume legitimately
// drains, whereas a stopped/done loop will never schedule another hub reinvoke
// to consume a queued cohort-synthesis note (BUG-307: a cohort that finishes
// joining after the loop already sealed left its "call the flow's control
// tool" note stranded in pendingAgentContext — composeAgentContextBlock then
// silently prepended it to the next ordinary chat turn on the run, sending an
// instruction with no live flow left to satisfy it and stalling the hub
// watchdog waiting for progress that could never come).
func (s *InteractiveService) loopSealedForReinvoke(parentRunID string) bool {
	switch s.agentOrchestrator.loopStateFor(parentRunID).Status {
	case "stopped", "done":
		return true
	default:
		return false
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
		// BUG-289 H1/F-1: do not discard startTurn errors — reinvokeInFlight
		// was set true by the scheduler and is only cleared on success
		// (startTurn) or Stop. A reject left the flag true forever, blocking
		// every later reinvoke and the pendingHubReinvoke re-arm path.
		//
		// Claude review: only re-arming pendingHubReinvoke without notifyTurnIdle
		// renames the stuck flag — hub_stalled treats pending as "busy" so F-0
		// never escalates and nothing ever drains/retries. Active-drain like H4.
		_, err := s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", "")
		if err != nil {
			shouldDrain := false
			transientBusy := false
			s.mu.Lock()
			if rs := s.runs[runID]; rs != nil && rs.reinvokeInFlight {
				rs.reinvokeInFlight = false
				rs.pendingHubReinvoke = true
				// Preserve a custom notify prompt (or the scheduled prompt) so
				// the next drain retries the same work (also covers M7 window).
				if strings.TrimSpace(prompt) != "" && strings.TrimSpace(rs.pendingHubReinvokePrompt) == "" {
					rs.pendingHubReinvokePrompt = prompt
				}
				rs.hubReinvokeStartFailCount++
				// Cap active retries so permanent startTurn failures cannot
				// tight-loop scheduleChildTurn → notifyTurnIdle → reinvoke.
				// Remaining pending is visible to F-0 once not counted as busy.
				shouldDrain = rs.hubReinvokeStartFailCount <= 3
				// turn_in_progress / gate_in_progress / hub_parked: notifyTurnIdle
				// is a no-op while busy, so schedule a short deferred drain once
				// the gate/turn window can clear (BUG-284 stranded reinvoke).
				if err.code == "turn_in_progress" || err.code == "gate_in_progress" || err.code == "hub_parked" {
					transientBusy = true
				}
				touchHubProgressLocked(rs)
			}
			s.mu.Unlock()
			s.flowDiagLog(runID, "hub_reinvoke_start_failed", "scheduled hub reinvoke startTurn failed; cleared reinvokeInFlight and re-armed pending",
				"error", err.Error(),
				"will_drain", shouldDrain,
				"transient_busy", transientBusy,
			)
			if shouldDrain {
				// Only live drain path for pendingHubReinvoke when idle (H5/F-5).
				s.notifyTurnIdle(runID)
				if transientBusy {
					go func(id string) {
						time.Sleep(25 * time.Millisecond)
						s.notifyTurnIdle(id)
					}(runID)
				}
			}
			s.maybeScheduleHubStallCheck(runID)
		}
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
				Label:         child.label,
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
	// V9-28: autoOrchestrate hubs are reinvoked only from cohort join /
	// advanceOrNotifyHub / resume — never from every non-cohort child release
	// (that raced with the join path and double-fired synthesis).
	s.mu.Lock()
	autoOrch := false
	if p := s.runs[parentRunID]; p != nil {
		autoOrch = p.autoOrchestrate
	}
	s.mu.Unlock()
	if !isCohortMember && !trackedFlow && !autoOrch {
		go s.maybeAutoReinvokeHub(parentRunID)
	}
}

func (s *InteractiveService) resumePendingLoopWork(parentRunID string) {
	type pendingTurn struct {
		runID  string
		stepID string
		prompt string
	}
	// BUG-288 R15-P0: stall-retry is durable + claimed; do not clear RAM then
	// fire startTurn without idempotency.
	s.mu.Lock()
	hasRestart := false
	if parent := s.runs[parentRunID]; parent != nil &&
		parent.pendingRestartRunID != "" && parent.pendingRestartPrompt != "" {
		hasRestart = true
	}
	allows := s.loopAllowsNextTurnLocked(parentRunID)
	s.mu.Unlock()
	if allows && hasRestart {
		go s.deliverPendingRestart(parentRunID)
		return
	}

	var next *pendingTurn
	s.mu.Lock()
	if !s.loopAllowsNextTurnLocked(parentRunID) {
		s.mu.Unlock()
		return
	}
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
	s.mu.Unlock()
	if next != nil {
		prompt, queued := s.takeQueuedFeedbackPrompt(parentRunID, next.runID, next.prompt)
		if queued != nil {
			s.recordAgentBus(parentRunID, *queued)
		}
		s.scheduleChildTurn(next.runID, next.stepID, prompt)
	}
}

// deliverPendingRestart delivers a durable stall-retry intent on the parent
// (BUG-288 R15-P0). Clears durable fields only AFTER startTurn accepts, with a
// deterministic idempotency key so crash/replay cannot open a second provider
// turn or leave the intent permanently stuck.
func (s *InteractiveService) deliverPendingRestart(parentRunID string) {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil {
		s.mu.Unlock()
		return
	}
	childID := strings.TrimSpace(parent.pendingRestartRunID)
	prompt := parent.pendingRestartPrompt
	gen := parent.pendingRestartGen
	if childID == "" || strings.TrimSpace(prompt) == "" {
		s.mu.Unlock()
		return
	}
	if gen == 0 {
		// Legacy sessions without gen: assign once and persist so recovery can claim.
		gen = 1
		parent.pendingRestartGen = gen
		snap := sessionStateOf(parent)
		s.mu.Unlock()
		_ = s.persistProviderSession(snap)
		s.mu.Lock()
		parent = s.runs[parentRunID]
		if parent == nil {
			s.mu.Unlock()
			return
		}
	}
	child := s.runs[childID]
	if child == nil || child.turnInFlight {
		s.mu.Unlock()
		return
	}
	if !s.claimDurableIntentLocked(parent, "restart", prompt, childID, gen) {
		s.mu.Unlock()
		return
	}
	stepID := child.stepID
	if stepID == "" {
		stepID = "chat-" + childID
	}
	s.mu.Unlock()

	// BUG-288 R18-1: zero-pad gen so numeric order matches lexical if ever sorted as string.
	idem := fmt.Sprintf("durable-%s-restart-%020d", childID, gen)
	turnID, apiErr := s.startTurn(childID, TurnInput{StepID: stepID, Prompt: prompt}, "", idem)
	if apiErr != nil {
		log.Printf("[stall-restart] startTurn failed parent=%s child=%s gen=%d code=%s: %s",
			parentRunID, childID, gen, apiErr.code, apiErr.msg)
		s.mu.Lock()
		if p := s.runs[parentRunID]; p != nil {
			releaseDurableIntentClaimLocked(p, "restart", gen)
		}
		s.mu.Unlock()
		return
	}
	// BUG-288 R20-1: clear restart intent only when child turn is clear-safe.
	s.mu.Lock()
	if p := s.runs[parentRunID]; p != nil {
		child := s.runs[childID]
		if durableIntentClearOK(child, turnID) &&
			p.pendingRestartGen == gen && p.pendingRestartRunID == childID {
			p.pendingRestartRunID = ""
			p.pendingRestartPrompt = ""
			p.pendingRestartGen = 0
		}
		releaseDurableIntentClaimLocked(p, "restart", gen)
		snap := sessionStateOf(p)
		s.mu.Unlock()
		if err := s.persistProviderSession(snap); err != nil {
			log.Printf("[stall-restart] persist clear after accept parent=%s: %v", parentRunID, err)
		}
		return
	}
	s.mu.Unlock()
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

// emitParentAgentResultLocked writes agent_result_injected onto the parent run so
// the desktop can close the open agent card (finalMessage) for this child. Caller
// MUST hold s.mu. No-op when parent is missing or the message is empty.
func (s *InteractiveService) emitParentAgentResultLocked(child *interactiveRun, finalMsg string) {
	if child == nil || strings.TrimSpace(child.parentRunID) == "" {
		return
	}
	msg := strings.TrimSpace(finalMsg)
	if msg == "" {
		return
	}
	parent := s.runs[child.parentRunID]
	if parent == nil {
		return
	}
	_ = s.emitLocked(parent, ProviderEvent{
		Type:         EventAgentResultInjected,
		AgentName:    child.agentName,
		ChildRunID:   child.id,
		FinalMessage: msg,
	})
}

// maxPendingAgentNotes caps the parent's UI-spawn context buffer so a user who spawns
// many children without chatting cannot grow the next prompt without bound (BUG-122).
const maxPendingAgentNotes = 50

// agentContextBlockOpen / agentContextBlockClose delimit composeAgentContextBlock's
// system-note prefix. They are shared with the reconstruction path
// (stripAgentContextBlock, BUG-306) so the strip logic cannot drift from the
// wrapper text and silently reintroduce the transcript-ordering bug. Keep the
// exact bytes (em dash) identical to what providers persist in their session files.
const agentContextBlockOpen = "[FlowPilot system note — sub-agents started in this session via the UI (not by you):"
const agentContextBlockClose = "Use this when the user asks which sub-agents were started, their providers/models, or their results.]"

// composeAgentContextBlock renders the parent's pending UI-spawn notes as a single
// system-note prefix folded into the next provider turn's prompt (BUG-122).
func composeAgentContextBlock(notes []string) string {
	var b strings.Builder
	b.WriteString(agentContextBlockOpen)
	b.WriteString("\n")
	for _, n := range notes {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString(agentContextBlockClose)
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

// flowRootIDLocked walks a run's parentRunID chain up to its root flow run
// (the highest flowEngineDriven ancestor), or "" when the run is not part of a
// flow-engine tree. CA-642: child questions must surface on the root run's
// event stream because clients subscribe to the root only. Caller must hold
// s.mu.
func (s *InteractiveService) flowRootIDLocked(rs *interactiveRun) string {
	for cur := rs; cur != nil && cur.parentRunID != ""; {
		parent := s.runs[cur.parentRunID]
		if parent == nil || !parent.flowEngineDriven {
			return ""
		}
		if parent.parentRunID == "" {
			return parent.id
		}
		cur = parent
	}
	return ""
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
	// Task-204: wire mcp.driver's production Google Drive backing once a real
	// *Runner is available. DefaultContextSourceRegistry() is a lazily
	// constructed package-wide singleton, so this may run before or after any
	// call that first constructs it — SetMCPDriverAdapter is safe either way
	// (it mutates the already-registered mcp.driver source in place).
	DefaultContextSourceRegistry().SetMCPDriverAdapter(&googleDriveDriverAdapter{runner: r})
	// Task-234 T-5: jira.issue/jira.sprint/firebase.crashlytics's live-fetch
	// adapters (jiraRestIssueAdapter, jiraRestSprintAdapter,
	// firebaseToolsMcpAdapter — wired here since Task-229/231) are
	// deliberately left unwired now. These sources are already filtered out
	// of the live AI-turn collect path (flow_executor.go's filterString
	// calls) in favor of a bounded prompt note — so their Fetch adapters were
	// never actually invoked in a live turn (CP-05-06 R-1: REST is scoped to
	// pick-list use, not live content). Keeping the adapter code/files intact
	// per explicit instruction, only removing the wiring, in case a future
	// non-live consumer (e.g. a Test Console direct-collect path) wants them.
	//
	// DefaultContextSourceRegistry().SetJiraIssueAdapter(&jiraRestIssueAdapter{runner: r})
	// DefaultContextSourceRegistry().SetJiraSprintAdapter(&jiraRestSprintAdapter{runner: r})
	// DefaultContextSourceRegistry().SetFirebaseCrashlyticsAdapter(newFirebaseToolsMcpAdapter(r))
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
		RunID:                           rs.id,
		ProjectID:                       rs.projectID,
		WorkflowID:                      rs.workflowID,
		ProviderSessionID:               providerSessionID,
		ProviderKey:                     rs.providerKey,
		ProviderAccountID:               rs.providerAccountID,
		WorkingDirectory:                rs.workspaceCwd,
		Status:                          rs.status,
		LastPrompt:                      rs.lastPrompt,
		LastFullPrompt:                  rs.lastFullPrompt,
		LastMessage:                     rs.lastMessage,
		StartedAt:                       rs.createdAt,
		UpdatedAt:                       rs.updatedAt,
		RunKind:                         rs.runKind,
		ChatID:                          rs.chatID,
		LegSeq:                          rs.legSeq,
		LegState:                        rs.legState,
		LegClosedReason:                 rs.legClosedReason,
		SwitchFromRunID:                 rs.switchFromRunID,
		SourceMachineID:                 rs.sourceMachineID,
		SourceRunID:                     rs.sourceRunID,
		RestoredFrom:                    rs.restoredFrom,
		SyncStatus:                      rs.syncStatus,
		SyncUpdatedAt:                   rs.syncUpdatedAt,
		ParentRunID:                     rs.parentRunID,
		AgentName:                       rs.agentName,
		Label:                           rs.label,
		Role:                            rs.role,
		DependsOn:                       append([]string(nil), rs.dependsOn...),
		AgentStatus:                     rs.agentStatus,
		ModelName:                       rs.modelName,
		ChangeType:                      rs.changeType,
		SourceDocID:                     rs.sourceDocID,
		TurnCount:                       rs.turnCount,
		PendingAgentContext:             append([]string(nil), rs.pendingAgentContext...),
		AutoOrchestrate:                 rs.autoOrchestrate,
		FlowCohortID:                    rs.flowCohortId,
		ActiveFlowEdges:                 append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...),
		ActiveFlowNodes:                 append([]agentpack.FlowNode(nil), rs.activeFlowNodes...),
		ChatSubMode:                     rs.chatSubMode,
		ChatFlowRef:                     rs.chatFlowRef,
		FlowStartGitHead:                rs.flowStartGitHead,
		PendingFlowGateSettle:           rs.pendingFlowGateSettle,
		PendingFlowGateFinalMsg:         rs.pendingFlowGateFinalMsg,
		PendingFlowGateOccurredAt:       rs.pendingFlowGateOccurredAt,
		PendingFlowGateTurnID:           rs.pendingFlowGateTurnID,
		TurnStartGitHead:                rs.turnStartGitHead,
		TurnStartWorktree:               copyStringMap(rs.turnStartWorktree),
		PendingGateChangedFiles:         append([]string(nil), rs.pendingGateChangedFiles...),
		StepID:                          rs.stepID,
		LastTurnStepID:                  rs.lastTurnStepID,
		PendingGateRepromptPrompt:       rs.pendingGateRepromptPrompt,
		PendingGateRepromptStepID:       rs.pendingGateRepromptStepID,
		HubContinueDelegatedTurnID:      rs.hubContinueDelegatedTurnID,
		PendingGateCodePaths:            append([]string(nil), rs.pendingGateCodePaths...),
		RepromptAttempts:                rs.repromptAttempts,
		PendingResumePrompt:             rs.pendingResumePrompt,
		PendingResumeStepID:             rs.pendingResumeStepID,
		PendingResumeGen:                rs.pendingResumeGen,
		PendingGateRepromptGen:          rs.pendingGateRepromptGen,
		PendingResumeDeliveredGen:       rs.pendingResumeDeliveredGen,
		PendingGateRepromptDeliveredGen: rs.pendingGateRepromptDeliveredGen,
		PendingResumeAcceptedTurn:       rs.pendingResumeAcceptedTurn,
		PendingGateRepromptAcceptedTurn: rs.pendingGateRepromptAcceptedTurn,
		PendingResumeFailCount:          rs.pendingResumeFailCount,
		PendingResumeFailGen:            rs.pendingResumeFailGen,
		PendingGateRepromptFailCount:    rs.pendingGateRepromptFailCount,
		PendingGateRepromptFailGen:      rs.pendingGateRepromptFailGen,
		PendingResumeApprovalID:         rs.pendingResumeApprovalID,
		PendingResumeDecision:           rs.pendingResumeDecision,
		PendingResumeQuestionChoices:    append([]string(nil), rs.pendingResumeQuestionChoices...),
		StopGeneration:                  rs.stopGeneration,
		ParentStopGenSeen:               rs.parentStopGenSeen,
		IntentBlockedKind:               rs.intentBlockedKind,
		IntentBlockedReason:             rs.intentBlockedReason,
		IntentBlockedAt:                 rs.intentBlockedAt,
		TransitionLogDegraded:           rs.transitionLogDegraded,
		TransitionLogDegradedAt:         rs.transitionLogDegradedAt,
		TransitionLogDegradedReason:     rs.transitionLogDegradedReason,
		PendingRestartRunID:             rs.pendingRestartRunID,
		PendingRestartPrompt:            rs.pendingRestartPrompt,
		PendingRestartGen:               rs.pendingRestartGen,
		IdempotencyKeys:                 durableIdempotencySnapshotWithNonTerminal(rs.idempotency, rs.nonTerminalIdemKeys()),
		FlowContextInjected:             rs.flowContextInjected,
		// CP-51: dispatch protocol/repair/provenance scalars only — never DispatchRecord slices.
		DispatchProtocolVersion:            rs.dispatchProtocolVersion,
		RepairRequired:                     rs.repairRequired,
		RepairReason:                       rs.repairReason,
		MarkerProvenanceRunIDs:             append([]string(nil), rs.markerProvenanceRunIDs...),
		PendingRestartProvenanceRunID:      rs.pendingRestartProvenanceRunID,
		PendingGateRepromptProvenanceRunID: rs.pendingGateRepromptProvenanceRunID,
		// BUG-299 residual: round-trip YOLO so chat restart keeps the toggle and
		// flow rehydrate has a durable value to force against when missing.
		Yolo:                      rs.yolo,
		LastFailedDelegateNodeID:  rs.lastFailedDelegateNodeID,
		LastEscalatedInlineNodeID: rs.lastEscalatedInlineNodeID,
	}
}

// sessionStateOfProtectingIdem builds a session snapshot that always retains
// the given durable idempotency key (BUG-288 R18-1).
func sessionStateOfProtectingIdem(rs *interactiveRun, protectKey string) ProviderSessionState {
	snap := sessionStateOf(rs)
	if protectKey != "" && strings.HasPrefix(protectKey, "durable-") && rs != nil && rs.idempotency != nil {
		if v := rs.idempotency[protectKey]; v != "" {
			if snap.IdempotencyKeys == nil {
				snap.IdempotencyKeys = map[string]string{}
			}
			snap.IdempotencyKeys[protectKey] = v
		}
	}
	return snap
}

// durableIdempotencySnapshot keeps durable-* keys for disk (BUG-288 R16-P0 / CP-51 Task-254).
// Non-terminal keys are ALWAYS retained (authority: DispatchRecord / nonTerminal set).
// Terminal history is capped per-namespace so restart-1 is never evicted by resume-*.
func durableIdempotencySnapshot(m map[string]string, protectKeys ...string) map[string]string {
	return durableIdempotencySnapshotWithNonTerminal(m, nil, protectKeys...)
}

// durableIdempotencySnapshotWithNonTerminal is the Task-254 retention path.
func durableIdempotencySnapshotWithNonTerminal(m map[string]string, nonTerminal map[string]bool, protectKeys ...string) map[string]string {
	if len(m) == 0 && len(protectKeys) == 0 {
		return nil
	}
	type kv struct {
		k, v, ns string
		gen      int64
	}
	out := map[string]string{}
	perNS := map[string][]kv{}
	protect := map[string]bool{}
	for _, pk := range protectKeys {
		if pk != "" && strings.HasPrefix(pk, "durable-") {
			protect[pk] = true
		}
	}
	for k, v := range m {
		if !strings.HasPrefix(k, "durable-") || v == "" {
			continue
		}
		if nonTerminal[k] || protect[k] {
			out[k] = v
			continue
		}
		ns, gen := durableIdemKeyParts(k)
		perNS[ns] = append(perNS[ns], kv{k: k, v: v, ns: ns, gen: gen})
	}
	// Also pin protect keys present in m but not yet written into out.
	for pk := range protect {
		if v, ok := m[pk]; ok && v != "" {
			out[pk] = v
		}
	}
	const capPerNS = 16
	for _, list := range perNS {
		sort.Slice(list, func(i, j int) bool {
			if list[i].gen != list[j].gen {
				return list[i].gen > list[j].gen
			}
			return list[i].k < list[j].k
		})
		for i, e := range list {
			if i >= capPerNS {
				break
			}
			if _, ok := out[e.k]; ok {
				continue
			}
			out[e.k] = e.v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// durableIdemKeyParts splits "durable-<run>-restart-12" into namespace + gen.
func durableIdemKeyParts(k string) (namespace string, gen int64) {
	i := strings.LastIndex(k, "-")
	if i < 0 || i+1 >= len(k) {
		return k, 0
	}
	n, err := strconv.ParseInt(k[i+1:], 10, 64)
	if err != nil {
		return k, 0
	}
	return k[:i], n
}

// durableIdempotencyKeyGen extracts a trailing integer generation from keys like
// "durable-child-restart-12" or "durable-run-reprompt-3". Non-numeric → 0.
func durableIdempotencyKeyGen(k string) int64 {
	_, gen := durableIdemKeyParts(k)
	return gen
}

// nonTerminalIdemKeys derives active keys from the dispatch RAM cache (Task-254).
func (rs *interactiveRun) nonTerminalIdemKeys() map[string]bool {
	out := map[string]bool{}
	if rs == nil {
		return out
	}
	for _, rec := range rs.dispatch {
		if rec == nil || rec.OuterIntentKey == "" {
			continue
		}
		if !rec.State.IsTerminal() {
			out[rec.OuterIntentKey] = true
		}
	}
	return out
}

// BUG-288 R19-2: durable idempotency is two-phase.
//
//	prep:<turnID>  — pre-persist succeeded; provider not launched yet
//	<turnID>       — launch-ack; full idempotency replay is valid
//
// Bare (non-prefixed) values are treated as launched for backward compatibility
// with sessions written before R19.
const durableIdemPreparedPrefix = "prep:"

func durableIdemPreparedValue(turnID string) string {
	return durableIdemPreparedPrefix + turnID
}

func parseDurableIdemValue(raw string) (turnID string, launched bool) {
	if strings.HasPrefix(raw, durableIdemPreparedPrefix) {
		return strings.TrimPrefix(raw, durableIdemPreparedPrefix), false
	}
	return raw, true
}

// durableIdemReplaySafe reports whether a durable idempotency hit may short-
// circuit without re-entering the launch path (BUG-288 R20-1).
//
// Bare/"launched" alone is NOT enough: a crash after launch-ack but before the
// provider actually ran would otherwise clear outer intents on a ghost accept.
// Recovery owns incomplete keys — only short-circuit when this process still
// holds the turn in flight, or durable evidence shows the turn finished / is
// owned by gate settle.
//
// CP-51 DOD-G8 / RecoveryInferenceRule: EventTurnStarted alone is NOT
// sufficient recovery evidence (emitted before go runTurn). Prefer
// DispatchRecord.State when the V2 dispatch store is authoritative for the run.
func durableIdemReplaySafe(rs *interactiveRun, turnID string, launched bool) bool {
	if rs == nil || turnID == "" || !launched {
		return false
	}
	// Live in this process: caller may safely treat as accepted (provider path
	// already scheduled or running under turnInFlight).
	if rs.turnInFlight && rs.currentTurnID == turnID {
		return true
	}
	if rs.lastTurnID == turnID {
		return true
	}
	if rs.pendingFlowGateSettle && rs.pendingFlowGateTurnID == turnID {
		return true
	}
	for _, ev := range rs.events {
		if ev.ProviderTurnID != turnID {
			continue
		}
		switch ev.Type {
		case EventTurnCompleted, EventTurnFailed:
			return true
		}
	}
	return false
}

// durableIntentClearOK is the caller-side gate for clearing outer durable
// intents after startTurn returns (BUG-288 R20-1). Intent must not clear unless
// recovery can observe a live or terminal turn for turnID.
func durableIntentClearOK(rs *interactiveRun, turnID string) bool {
	return durableIdemReplaySafe(rs, turnID, true)
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
	// CP-59 chat SSOT capture (SD-26 §5.3): mirrors this function's delta-skip
	// policy, flag-gated, never fails the turn. Kept beside persistEvent so the
	// two writers share one call graph.
	s.recordChatTranscript(event)
	store := s.persistenceStore()
	if store == nil || event.Type == EventMessageDelta {
		return nil
	}
	return store.AppendEvent(context.Background(), event)
}

// rehydratePendingGatesLocked rebuilds in-memory approval/question records from
// durable store so SubmitApprovalDecision/AnswerQuestion do not 404 after restart
// (V10 P1). Records are marked rehydrated so submit can restart the turn when the
// original provider goroutine is gone. Caller holds s.mu.
//
// Skips rehydrate when the loop is stopped/done so Stopped flows do not
// surface stale actionable cards (V10R4 P1). Also reconciles two-write crash:
// session PendingResumeApprovalID+Decision means the card was decided even if
// the resolved card write was lost — and repairs durable card on disk (P1-02).
//
// V10R4 P0-01: do NOT skip merely because normalize mapped status to Cancelled
// before cards were known; restore WaitingApproval/WaitingQuestion when durable
// pending cards exist and the parent loop is still live.
func (s *InteractiveService) rehydratePendingGatesLocked(runID string) {
	if runID == "" {
		return
	}
	rs := s.runs[runID]
	if rs == nil {
		return
	}
	// Truly terminal outcomes never rehydrate.
	if rs.status == RunStatusFailed || rs.status == RunStatusCompleted {
		return
	}
	// Parent or own loop stopped/done — no user action is valid (Stop path).
	loopParent := runID
	if rs.parentRunID != "" {
		loopParent = rs.parentRunID
	}
	if st := s.agentOrchestrator.loopStateFor(loopParent).Status; st == "stopped" || st == "done" {
		return
	}
	// Decision already recorded on session (crash between intent and card write).
	decidedApproval := strings.TrimSpace(rs.pendingResumeApprovalID)
	decidedDecision := strings.TrimSpace(rs.pendingResumeDecision)
	// Cards to repair on disk after unlock (cannot I/O under s.mu long-term;
	// queue and flush via deferred persist after rehydrate).
	var repairApprovals []ProviderApprovalState
	var repairQuestions []ProviderQuestionState
	gotPendingApproval := false
	gotPendingQuestion := false
	if ahr, ok := s.workflowStore.(ApprovalHistoryReader); ok {
		if states, err := ahr.ListApprovalsByRun(context.Background(), runID); err == nil {
			for _, st := range states {
				if !strings.EqualFold(strings.TrimSpace(st.Status), "pending") {
					continue
				}
				if s.approvals[st.ApprovalID] != nil {
					continue
				}
				// Reconcile: session already has a decision for this card — treat resolved
				// and queue durable card repair so restart #2 cannot resurrect pending.
				if decidedApproval != "" && st.ApprovalID == decidedApproval && decidedDecision != "" {
					rec := &approvalRecord{
						id:       st.ApprovalID,
						runID:    st.RunID,
						status:   "resolved",
						decision: decidedDecision,
						details: ApprovalDetails{
							Kind:    "exec",
							Command: st.Command,
							Cwd:     st.Cwd,
							Reason:  st.Reason,
							Decisions: []ApprovalDecisionOption{
								{Value: "approve", Label: "Approve"},
								{Value: "deny", Label: "Deny"},
							},
						},
						resolve:    make(chan string, 1),
						rehydrated: true,
					}
					s.approvals[st.ApprovalID] = rec
					repairApprovals = append(repairApprovals, approvalStateFromRecord(rs, rec, st.ExpiresAt))
					continue
				}
				// BUG-288 P1-08: a TTL that already elapsed while the process was
				// down must durably expire on rehydrate, not resurrect as a live
				// pending card (there was never an in-process timer to catch it).
				if approvalExpiryElapsed(st.ExpiresAt) {
					expiredRec := &approvalRecord{
						id:     st.ApprovalID,
						runID:  st.RunID,
						status: "expired",
						details: ApprovalDetails{
							Kind: "exec", Command: st.Command, Cwd: st.Cwd, Reason: st.Reason,
							Decisions: []ApprovalDecisionOption{{Value: "approve", Label: "Approve"}, {Value: "deny", Label: "Deny"}},
						},
						resolve: make(chan string, 1),
					}
					s.approvals[st.ApprovalID] = expiredRec
					repairApprovals = append(repairApprovals, approvalStateFromRecord(rs, expiredRec, st.ExpiresAt))
					// BUG-289 H4/F-4: rehydrate already-expired must not leave
					// rs.status stuck at WaitingApproval with no live card.
					if rs.status == RunStatusWaitingApproval && rs.pendingApprovalID == st.ApprovalID {
						rs.pendingApprovalID = ""
						rs.status = RunStatusRunning
						rs.agentStatus = string(RunStatusRunning)
						touchHubProgressLocked(rs)
					} else if rs.pendingApprovalID == st.ApprovalID {
						rs.pendingApprovalID = ""
					}
					continue
				}
				rec := &approvalRecord{
					id:     st.ApprovalID,
					runID:  st.RunID,
					status: "pending",
					details: ApprovalDetails{
						Kind:    "exec",
						Command: st.Command,
						Cwd:     st.Cwd,
						Reason:  st.Reason,
						Decisions: []ApprovalDecisionOption{
							{Value: "approve", Label: "Approve"},
							{Value: "deny", Label: "Deny"},
						},
					},
					resolve:    make(chan string, 1),
					rehydrated: true,
					expiresAt:  st.ExpiresAt,
				}
				s.approvals[st.ApprovalID] = rec
				if rs.pendingApprovalID == "" {
					rs.pendingApprovalID = st.ApprovalID
				}
				gotPendingApproval = true
				// Schedule the remaining TTL so a card that was pending-but-not-yet-
				// expired at crash still expires durably instead of only ever being
				// caught by a fresh in-process AskQuestion/RequestApproval timer
				// (which does not exist for a rehydrated record).
				scheduleApprovalExpiry(s, st.ApprovalID, st.ExpiresAt)
			}
		}
	}
	if qhr, ok := s.workflowStore.(QuestionHistoryReader); ok {
		if states, err := qhr.ListQuestionsByRun(context.Background(), runID); err == nil {
			for _, st := range states {
				if !strings.EqualFold(strings.TrimSpace(st.Status), "pending") {
					continue
				}
				if s.questions[st.QuestionID] != nil {
					continue
				}
				// Reconcile decision recorded on session for this question id.
				if decidedApproval != "" && st.QuestionID == decidedApproval && decidedDecision != "" {
					// BUG-288 P2-01: prefer the durable, un-flattened choices slice
					// over re-wrapping the joined display string, which would
					// corrupt any choice containing the join separator.
					choice := append([]string(nil), rs.pendingResumeQuestionChoices...)
					if len(choice) == 0 {
						choice = []string{decidedDecision}
					}
					rec := &questionRecord{
						id:          st.QuestionID,
						runID:       st.RunID,
						prompt:      st.Prompt,
						options:     st.Options,
						multiSelect: st.MultiSelect,
						status:      "resolved",
						choice:      choice,
						resolve:     make(chan questionResolveResult, 1),
						rehydrated:  true,
					}
					s.questions[st.QuestionID] = rec
					repairQuestions = append(repairQuestions, questionStateFromRecord(rec, "", st.ExpiresAt))
					continue
				}
				// BUG-288 P1-08: see the mirrored approval-side comment above.
				if approvalExpiryElapsed(st.ExpiresAt) {
					expiredRec := &questionRecord{
						id: st.QuestionID, runID: st.RunID, prompt: st.Prompt,
						options: st.Options, multiSelect: st.MultiSelect,
						status: "expired", resolve: make(chan questionResolveResult, 1),
					}
					s.questions[st.QuestionID] = expiredRec
					repairQuestions = append(repairQuestions, questionStateFromRecord(expiredRec, "", st.ExpiresAt))
					// BUG-289 H4/F-4: rehydrate already-expired question.
					if rs.status == RunStatusWaitingQuestion && rs.pendingQuestionID == st.QuestionID {
						rs.pendingQuestionID = ""
						rs.status = RunStatusRunning
						rs.agentStatus = string(RunStatusRunning)
						touchHubProgressLocked(rs)
					} else if rs.pendingQuestionID == st.QuestionID {
						rs.pendingQuestionID = ""
					}
					continue
				}
				s.questions[st.QuestionID] = &questionRecord{
					id:          st.QuestionID,
					runID:       st.RunID,
					prompt:      st.Prompt,
					options:     st.Options,
					multiSelect: st.MultiSelect,
					status:      "pending",
					resolve:     make(chan questionResolveResult, 1),
					rehydrated:  true,
					expiresAt:   st.ExpiresAt,
				}
				if rs.pendingQuestionID == "" {
					rs.pendingQuestionID = st.QuestionID
				}
				gotPendingQuestion = true
				scheduleQuestionExpiry(s, st.QuestionID, st.ExpiresAt)
			}
		}
	}
	// Restore waiting status after normalize/cancel so parentHasLivePendingChildren
	// and the UI see a live card (V10R4 P0-01).
	if gotPendingApproval {
		rs.status = RunStatusWaitingApproval
		rs.agentStatus = string(RunStatusWaitingApproval)
	} else if gotPendingQuestion {
		rs.status = RunStatusWaitingQuestion
		rs.agentStatus = string(RunStatusWaitingQuestion)
	}
	// Persist durable card repairs outside the hot path — spawn after unlock via
	// a deferred list stored on the service is awkward under lock; repair inline
	// after the caller unlocks by using a short unlock-free persist (store IO).
	// Callers of rehydratePendingGatesLocked hold s.mu; schedule repairs async.
	if len(repairApprovals) > 0 || len(repairQuestions) > 0 {
		go s.persistGateRepairs(repairApprovals, repairQuestions)
	}
}

// persistGateRepairs writes reconciled resolved cards so a second restart cannot
// resurrect pending cards after session decision tombstones are cleared (P1-02).
func (s *InteractiveService) persistGateRepairs(approvals []ProviderApprovalState, questions []ProviderQuestionState) {
	for _, st := range approvals {
		if err := s.persistApproval(st); err != nil {
			log.Printf("[rehydrate] repair approval %s: %v", st.ApprovalID, err)
		}
	}
	for _, st := range questions {
		if err := s.persistQuestion(st); err != nil {
			log.Printf("[rehydrate] repair question %s: %v", st.QuestionID, err)
		}
	}
}

// resumePendingFlowGate re-runs the post-turn gate after restart when durable
// PendingFlowGateSettle was true (V10 P0). Works for flow-engine children and
// root. On pass, materializes EventTurnCompleted (persist + broadcast) like the
// live post-gate path so SSE/reconnect see a terminal event (V10 residual P1).
func (s *InteractiveService) resumePendingFlowGate(runID string) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || !rs.pendingFlowGateSettle {
		s.mu.Unlock()
		return
	}
	// P1-04: never resume a child gate when parent loop is already
	// stopped/done/blocked. Blocked = awaiting user (cap/escalate): boot used
	// to re-eval the post-turn gate, arm a gate reprompt, hit startTurn →
	// flow_awaiting_user 409, wipe UX into Failed with no Continue/Stop card.
	// Terminal statuses also clear stale settle so every restart does not
	// re-schedule the same gate (safe-fix residual vs stopped/done only).
	loopParent := runID
	if rs.parentRunID != "" {
		loopParent = rs.parentRunID
	}
	if st := s.agentOrchestrator.loopStateFor(loopParent).Status; st == "stopped" || st == "done" || st == "blocked" {
		// Drop settle/reprompt under lock; preserve reprompt gen high-water.
		hadStale := clearStaleFlowGateIntentsLocked(rs)
		var snap ProviderSessionState
		if hadStale {
			snap = sessionStateOf(rs)
			if rs.parentRunID == "" {
				snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
			}
		}
		s.mu.Unlock()
		if hadStale {
			_ = s.persistProviderSession(snap)
		}
		return
	}
	// BUG-288 P1-04: durable stop-tombstone check. The in-memory loop-state
	// check above depends on agentOrchestrator having already observed Stop —
	// which after a restart depends on reconstruction ordering (parent vs
	// child). This check is ordering-independent: both stopGeneration and
	// parentStopGenSeen are loaded directly from the persisted session at
	// reconstruction, so a child whose last checkpoint predates its parent's
	// current stop generation is rejected regardless of load order.
	if rs.parentRunID != "" {
		if parent := s.runs[rs.parentRunID]; parent != nil && parent.stopGeneration > rs.parentStopGenSeen {
			s.mu.Unlock()
			return
		}
	}
	// P1-06: single-flight claim — only one resumePendingFlowGate evaluates.
	if rs.gateClaimID != "" {
		s.mu.Unlock()
		return
	}
	msg := rs.pendingFlowGateFinalMsg
	at := rs.pendingFlowGateOccurredAt
	if at == "" {
		at = time.Now().UTC().Format(time.RFC3339Nano)
	}
	// Prefer durable pending-gate turn id (persisted); fall back to in-memory.
	turnID := rs.pendingFlowGateTurnID
	if turnID == "" {
		turnID = rs.lastTurnID
	}
	if turnID == "" {
		turnID = rs.currentTurnID
	}
	fin := finalizeInput{
		RunID:        rs.id,
		TurnID:       turnID,
		FinalMessage: msg,
		ChangedFiles: append([]string(nil), rs.pendingGateChangedFiles...),
	}
	isRoot := rs.parentRunID == ""
	epoch := rs.gateEpoch
	claimID := s.nextID("gateclaim")
	rs.gateClaimID = claimID
	s.mu.Unlock()
	gateCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.mu.Lock()
	rs = s.runs[runID]
	if rs == nil || rs.gateEpoch != epoch || rs.gateClaimID != claimID {
		if rs != nil && rs.gateClaimID == claimID {
			rs.gateClaimID = ""
		}
		s.mu.Unlock()
		return
	}
	rs.postTurnGateCancel = cancel
	s.mu.Unlock()
	var blocked bool
	if isRoot {
		blocked = s.runFlowGateAtEpoch(gateCtx, rs, fin.TurnID, fin, epoch)
	} else {
		blocked = s.runChildArtifactOutputGateAtEpoch(gateCtx, rs, fin.TurnID, fin, epoch)
	}
	s.mu.Lock()
	rs = s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// V10R4 P0: Stop bumped gateEpoch — discard all gate side effects.
	if rs.gateEpoch != epoch || rs.gateClaimID != claimID {
		if rs.gateClaimID == claimID {
			rs.gateClaimID = ""
		}
		rs.postTurnGateCancel = nil
		s.mu.Unlock()
		return
	}
	rs.postTurnGateCancel = nil
	rs.gateClaimID = ""
	if blocked {
		rs.pendingFlowGateSettle = false
		rs.pendingFlowGateFinalMsg = ""
		rs.pendingFlowGateOccurredAt = ""
		rs.pendingFlowGateTurnID = ""
		rs.pendingGateChangedFiles = nil
		s.settleChildStatusAfterGateBlockLocked(rs)
		// V10R4 P1: keep durable reprompt intent on session (do not clear before
		// startTurn succeeds). Code paths + attempts already on rs.
		repromptPrompt := rs.pendingGateRepromptPrompt
		repromptStep := rs.pendingGateRepromptStepID
		repromptGen := rs.pendingGateRepromptGen
		repromptRun := rs.id
		blockTurnID := turnID
		snap := sessionStateOf(rs)
		if isRoot {
			snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
		}
		s.mu.Unlock()
		_ = s.persistProviderSession(snap)
		// Task-251 T-2b: durable settle disposition for gate block/reprompt.
		s.scheduleSettleAfterGateBlock(repromptRun, blockTurnID)
		if repromptPrompt != "" && repromptStep != "" {
			if isRoot {
				// CP-51 A1: park/suppress hub gate remediations that would race
				// a continue-delegated coder (or other active flow children).
				s.scheduleRootGateRepromptOrPark(repromptRun, repromptStep, repromptPrompt, blockTurnID, repromptGen)
			} else {
				go s.startTurnClearingIntent(repromptRun, repromptStep, repromptPrompt, "reprompt", repromptGen)
			}
		} else {
			s.notifyTurnIdle(repromptRun)
		}
		return
	}
	// Gate pass — publish Completed + materialize terminal TurnCompleted event.
	// BUG-288 P1-05: compute the post-gate snapshot and persist it FIRST. Before
	// this fix, rs.status flipped to Completed, the terminal event broadcast to
	// subscribers, cohort/dependency settlement (settleFlowChildTurnCompletedLocked
	// / releaseDependentAgents) and the finalizer all ran BEFORE the durable
	// session write was confirmed — a persistence error was swallowed (fail-open),
	// so a restart could re-run gate evaluation, the finalizer, or release a
	// dependency a second time. Now: mutate RAM, persist, and only proceed to
	// broadcast/cohort/finalizer if the persist succeeds; otherwise restore the
	// gate to an actionable retry state and return without any downstream effect.
	prevStatus := rs.status
	prevAgentStatus := rs.agentStatus
	prevPendingSettle := rs.pendingFlowGateSettle
	prevFinalMsg := rs.pendingFlowGateFinalMsg
	prevOccurredAt := rs.pendingFlowGateOccurredAt
	prevTurnID := rs.pendingFlowGateTurnID
	prevChangedFiles := rs.pendingGateChangedFiles
	prevLastTurnID := rs.lastTurnID

	rs.pendingFlowGateSettle = false
	rs.pendingFlowGateFinalMsg = ""
	rs.pendingFlowGateOccurredAt = ""
	rs.pendingFlowGateTurnID = ""
	rs.pendingGateChangedFiles = nil
	rs.status = RunStatusCompleted
	rs.agentStatus = string(RunStatusCompleted)
	// Durable turn id for finalizer / summary (live path sets lastTurnID).
	if turnID != "" {
		rs.lastTurnID = turnID
	}
	parentID := rs.parentRunID
	childID := rs.id
	// V10R3 P1: include LoopState for root so we do not wipe durable loop on disk.
	snap := sessionStateOf(rs)
	if parentID == "" {
		snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
	}
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		// BUG-288 P1-05: persistence failed — do NOT broadcast/settle/finalize.
		// Restore the gate to its pre-completion, actionable retry state so a
		// later resumePendingFlowGate (or restart) re-attempts instead of the
		// completion having silently fanned out with no durable record.
		s.mu.Lock()
		if rs2 := s.runs[runID]; rs2 != nil {
			rs2.status = prevStatus
			rs2.agentStatus = prevAgentStatus
			rs2.pendingFlowGateSettle = prevPendingSettle
			rs2.pendingFlowGateFinalMsg = prevFinalMsg
			rs2.pendingFlowGateOccurredAt = prevOccurredAt
			rs2.pendingFlowGateTurnID = prevTurnID
			rs2.pendingGateChangedFiles = prevChangedFiles
			rs2.lastTurnID = prevLastTurnID
		}
		s.mu.Unlock()
		log.Printf("[flow-gate] persist post-gate completion run=%q turn=%q: %v (gate left actionable for retry)", runID, turnID, err)
		return
	}

	s.mu.Lock()
	rs = s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// BUG-288 R11 #4: gateClaimID was already released before the persist call
	// above, so it cannot detect a concurrent Stop here — revalidate gateEpoch
	// instead (Stop bumps it via clearDurableRecoveryStateLocked). Without this,
	// a Stop landing during the unlocked persist call would let this resumed
	// completion still signal/broadcast/settle/release.
	if rs.gateEpoch != epoch {
		s.mu.Unlock()
		return
	}
	if parentID != "" {
		if summary, ok := s.agentOrchestrator.currentSummary(parentID, childID); ok {
			summary.Status = RunStatusCompleted
			summary.AgentStatus = string(RunStatusCompleted)
			s.agentOrchestrator.upsertSummary(parentID, summary)
		}
	}
	s.agentOrchestrator.signalChild(childID, msg, false, "", RunStatusCompleted)
	completedEv := ProviderEvent{
		Type:              EventTurnCompleted,
		FinalMessage:      msg,
		OccurredAt:        at,
		ProviderTurnID:    turnID,
		WorkflowRunID:     rs.id,
		ProviderSessionID: rs.providerSessionID,
		ProviderKey:       rs.providerKey,
	}
	rs.seq++
	completedEv.Seq = rs.seq
	completedEv.ID = s.nextID("evt")
	// CP-51 Task-251 (T-2, ledger EL): keyed by (turnID, type), not type alone —
	// the prior check only compared rs.events[n-1].Type == EventTurnCompleted, so
	// a replay for a DIFFERENT turnID whose last event happened to also be
	// EventTurnCompleted would silently overwrite that other turn's completion
	// event instead of appending its own. Keying on ProviderTurnID makes this an
	// idempotent upsert for THIS turn's completion specifically.
	if n := len(rs.events); n > 0 && rs.events[n-1].Type == EventTurnCompleted && rs.events[n-1].ProviderTurnID == turnID {
		rs.events[n-1] = completedEv
	} else {
		rs.events = append(rs.events, completedEv)
	}
	rs.lastEventType = EventTurnCompleted
	if err := s.persistEvent(completedEv); err != nil {
		// Not fail-closed here (T-3's retry worker is the real fix — out of
		// scope for this pass, see Task-251 §8): at minimum this must be
		// observable, never silently discarded.
		log.Printf("[flow-gate] persist post-gate completion event run=%q turn=%q: %v", runID, turnID, err)
	}
	for _, ch := range rs.subs {
		select {
		case ch <- completedEv:
		default:
		}
	}
	if parentID != "" {
		s.settleFlowChildTurnCompletedLocked(rs, msg, completedEv)
	}
	s.mu.Unlock()
	// V10R4 P1: match live post-gate path — finalizer + idle chat summary.
	_ = s.finalizer.Finalize(finalizeInput{
		RunID:        childID,
		TurnID:       turnID,
		FinalMessage: msg,
		ChangedFiles: append([]string(nil), fin.ChangedFiles...),
	})
	s.scheduleChatSummary(childID)
	if parentID != "" {
		go s.releaseDependentAgents(parentID, childID, msg, at)
	}
	// Task-251 T-1: after gate pass + completion durable, drive settle phases.
	s.scheduleSettleAfterGatePass(childID, turnID)
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

// durableResumeStepID returns a stable step id for restarting a turn after
// rehydrate (V10R P1). Prefer persisted stepID / lastTurnStepID; fall back to
// a deterministic label/run-scoped id rather than inventing a random one each
// approve (which would orphan the original execution step).
func durableResumeStepID(rs *interactiveRun) string {
	if rs == nil {
		return ""
	}
	if id := strings.TrimSpace(rs.stepID); id != "" {
		return id
	}
	if id := strings.TrimSpace(rs.lastTurnStepID); id != "" {
		return id
	}
	if label := strings.TrimSpace(rs.label); label != "" {
		return "resume-" + label
	}
	if rs.id != "" {
		return "chat-" + rs.id
	}
	return ""
}

// markPendingFlowGateSettleLocked defers Completed until the post-turn gate
// passes, and snapshots turn-scoped gate inputs so resume after restart uses the
// same base SHA / worktree / written paths (V10 residual P0). Caller holds s.mu.
// Returns false when the checkpoint could not be made durable (BUG-288 R17-P1):
// gate pass / completion fan-out must not proceed until retry succeeds.
func (s *InteractiveService) markPendingFlowGateSettleLocked(rs *interactiveRun, finalMsg, occurredAt string) bool {
	if rs == nil {
		return false
	}
	rs.pendingFlowGateSettle = true
	rs.gateCheckpointNotDurable = false
	rs.pendingFlowGateFinalMsg = finalMsg
	rs.pendingFlowGateOccurredAt = occurredAt
	// Durable turn id for EventTurnCompleted after restart (V10R P1).
	if tid := strings.TrimSpace(rs.currentTurnID); tid != "" {
		rs.pendingFlowGateTurnID = tid
	} else if tid := strings.TrimSpace(rs.lastTurnID); tid != "" {
		rs.pendingFlowGateTurnID = tid
	}
	// Snapshot WrittenPaths already emitted; finishTurn refreshes once more
	// after the adapter returns with the full event list.
	var paths []string
	seen := map[string]struct{}{}
	for _, e := range rs.events {
		if e.Type == EventFileChanged && e.Path != "" {
			if _, ok := seen[e.Path]; ok {
				continue
			}
			seen[e.Path] = struct{}{}
			paths = append(paths, e.Path)
		}
	}
	rs.pendingGateChangedFiles = paths
	// Keep non-terminal status so dependenciesSatisfiedLocked stays false and
	// resume does not normalize to Completed/Cancelled.
	if rs.status != RunStatusRunning {
		rs.status = RunStatusRunning
		rs.agentStatus = string(RunStatusRunning)
	}
	if finalMsg != "" {
		rs.lastMessage = truncateDisplayField(finalMsg, 100)
	}
	// V10R3 P1: synchronous checkpoint BEFORE release of emitLocked's lock so a
	// crash cannot lose PendingFlowGateSettle (async write race → resume cancel).
	snap := sessionStateOf(rs)
	if rs.parentRunID == "" {
		snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
	}
	// BUG-288 P1-16 / R15–R17 / R20-3: checkpoint must be durable. Retry twice
	// more without stamping a transient intentBlockedKind to disk — a durable
	// gate_settle_checkpoint marker that later "succeeds" left false blocked
	// state when the cleanup persist failed (R19 fourth write). Only when all
	// three settle persists fail do we set RAM blocked + gateCheckpointNotDurable
	// (no durable false-blocked marker for a healthy path).
	if err := s.persistProviderSession(snap); err != nil {
		log.Printf("[flow-gate] persist pendingFlowGateSettle checkpoint run=%q turn=%q: %v (retrying once)", rs.id, rs.pendingFlowGateTurnID, err)
		if err2 := s.persistProviderSession(snap); err2 != nil {
			log.Printf("[flow-gate] persist pendingFlowGateSettle FAILED run=%q turn=%q: %v (third try, no blocked marker)", rs.id, rs.pendingFlowGateTurnID, err2)
			if err3 := s.persistProviderSession(snap); err3 != nil {
				log.Printf("[flow-gate] persist pendingFlowGateSettle third FAILED run=%q: %v (blocking gate pass; RAM blocked only)", rs.id, err3)
				rs.intentBlockedKind = "gate_settle_checkpoint"
				rs.intentBlockedReason = "pendingFlowGateSettle checkpoint not durable: " + err3.Error()
				rs.intentBlockedAt = time.Now().UTC().Format(time.RFC3339Nano)
				rs.gateCheckpointNotDurable = true
				return false
			}
		}
	}
	// Settle durable — never leave a transient blocked stamp.
	if rs.intentBlockedKind == "gate_settle_checkpoint" {
		rs.intentBlockedKind = ""
		rs.intentBlockedReason = ""
		rs.intentBlockedAt = ""
	}
	rs.gateCheckpointNotDurable = false
	return true
}

// settleFlowChildTurnCompletedLocked advances parent flow state after a child turn
// completes and (for flow-engine children) the post-turn gate has passed.
// Caller holds s.mu. Task-242: must not run until runChildArtifactOutputGate allows.
func (s *InteractiveService) settleFlowChildTurnCompletedLocked(rs *interactiveRun, finalMsg string, ev ProviderEvent) {
	// Close the parent's agent card for this child. Flow auto-spawn uses wait:false,
	// so the Wait=true path in spawnAgent never emits agent_result_injected — without
	// this the live main-chat card stays open forever and reinvoke of the same
	// childRunId (Review Loop coder round 2+) is suppressed or stuck at the top
	// (run-9034 / run-5695). Caller holds s.mu — use emitLocked on the parent, never
	// emitOnParentRun (which re-locks).
	s.emitParentAgentResultLocked(rs, finalMsg)
	// run-43831: stamp hub progress BEFORE dispatching advance/reinvoke goroutines so
	// a pending F-0 watchdog cannot observe stale hubLastProgressAt in the gap between
	// this child going non-active and the next child becoming active.
	s.touchParentHubProgressFromChildLocked(rs)
	// BUG-318 P2 (watchdog hardening): arm the hub stall watchdog on EVERY flow
	// child settle, independent of whether this completion goes on to schedule a
	// hub reinvoke. Every other maybeScheduleHubStallCheck call site is coupled to
	// a reinvoke/notify actually being scheduled, so when a bug drops the reinvoke
	// (e.g. a reviewer completing into a dead/incomplete cohort) the watchdog was
	// never armed for that window either — the safety net shared the exact blind
	// spot it exists to catch. checkAndBlockStalledHub re-arms itself while a child
	// is active or the hub is busy and only blocks after a real timeout of no
	// progress, so this never false-trips a live flow. Dispatched via a goroutine:
	// this runs under s.mu and maybeScheduleHubStallCheck acquires s.mu.
	if rs.parentRunID != "" {
		go s.maybeScheduleHubStallCheck(rs.parentRunID)
	}
	if rs.flowCohortId != "" {
		machineVerdict := ""
		if parent := s.runs[rs.parentRunID]; parent != nil && parent.pendingReviewVerdictByLabel != nil {
			machineVerdict = parent.pendingReviewVerdictByLabel[rs.label]
			delete(parent.pendingReviewVerdictByLabel, rs.label)
		}
		s.agentOrchestrator.appendCohortResult(rs.parentRunID, rs.flowCohortId, cohortEntry{
			Label:          rs.label,
			Provider:       string(rs.providerKey),
			FinalMessage:   truncateDisplayField(finalMsg, 1500),
			Status:         "completed",
			MachineVerdict: machineVerdict,
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
			// Caller holds s.mu (settleFlowChildTurnCompletedLocked).
			s.setFlowStepStatusLocked(context.Background(), rs.parentRunID, rs.label, StepStatusDone)
			cohortDiagLog("member self-settled DONE parent=%q run=%q label=%q", rs.parentRunID, rs.id, rs.label)
		}
		// run-98153 / run-91842: both reviewers buffered but join never fired
		// because cohortExpected was 0 in RAM. Heal from live siblings.
		s.ensureCohortExpectedLocked(rs.parentRunID, rs.flowCohortId)
		if s.agentOrchestrator.cohortComplete(rs.parentRunID, rs.flowCohortId) {
			entries := s.agentOrchestrator.drainCohort(rs.parentRunID, rs.flowCohortId)
			s.snapshotReviewCohortVerdictsLocked(rs.parentRunID, entries)
			note := buildCohortNote(rs.parentRunID, rs.flowCohortId, entries, s.agentOrchestrator.graphSnapshot(rs.parentRunID).LoopState.Round)
			s.flowDiagLog(rs.parentRunID, "cohort_join_complete", "cohort barrier completed and note built",
				"cohort_id", rs.flowCohortId,
				"entry_count", len(entries),
				"note_len", len(note),
			)
			cohortDiagLog("cohortNote built parent=%q cohort=%q entries=%d noteLen=%d", rs.parentRunID, rs.flowCohortId, len(entries), len(note))
			// BUG-307: a cohort can finish joining after the parent loop already
			// sealed (Stop, or the flow's own "done") — no legitimate reinvoke will
			// ever drain this note then, so it would sit in pendingAgentContext,
			// persist to disk, and get silently prepended (BUG-122's
			// composeAgentContextBlock) to the next turn on this run, including an
			// ordinary chat follow-up typed after a restart (run-19500 live repro).
			if !s.loopSealedForReinvoke(rs.parentRunID) {
				s.appendPendingAgentContextLocked(rs.parentRunID, note)
			}
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
					if e.Status == "completed" && e.Label != "" {
						reviewerNodeIDs = append(reviewerNodeIDs, e.Label)
					}
				}
				hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
				// CP-58 Task-304: dual-hub harnesses must stamp (and track) the
				// hub the joined reviewers actually feed into — plan_synthesis
				// vs synthesis — so the hub's own continue/done resolves against
				// ITS loop instead of first-match hub.inline. Single-hub flows
				// keep Task-235's tracking untouched.
				if resolved, dual := hubNodeIDForCohortJoin(parent.activeFlowNodes, parent.activeFlowEdges, reviewerNodeIDs); dual && resolved != "" {
					hubNodeID = resolved
					parent.activeHubNodeID = resolved
				}
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
					// Caller holds s.mu (settleFlowChildTurnCompletedLocked).
					s.setFlowStepStatusLocked(context.Background(), parentRunID, id, StepStatusDone)
				}
				// BUG-234 (#4): only drive the hub node to RUNNING and re-invoke the
				// synthesis turn while the loop is still advancing. Once the loop has
				// settled (blocked/awaiting-user, done, stopped), a late or stray
				// cohort join must NOT flip the hub node back to RUNNING — that flap
				// is what left the synthesis step spinning forever after an escalate.
				// maybeAutoReinvokeHubWithNote is itself gated on loop status, but the
				// hub-RUNNING write below is not, so guard it here too.
				if hubNodeID != "" && s.loopIsAdvancing(parentRunID) {
					s.setFlowStepStatusLocked(context.Background(), parentRunID, hubNodeID, StepStatusRunning)
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
									if parent.pendingRestartGen <= 0 {
										parent.pendingRestartGen = 1
									} else {
										parent.pendingRestartGen++
									}
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

	// V10 P1 / residual P0: defer raw EventTurnCompleted persist/broadcast for
	// flow-engine children AND root until gate pass — subscribers must not treat
	// event type alone as terminal. Still record in-memory for finalizeInput.
	deferGateCompleted := false
	// BUG-305: a follow-up turn admitted onto an already-"done" loop is plain chat,
	// not flow work — never defer it behind the post-turn gate (which force-blocks a
	// "done" loop and would strand the live TurnCompleted, hanging the desktop's
	// turn stream). Only defer while the flow loop is still active.
	if ev.Type == EventTurnCompleted && !rs.turnStartedAfterLoopDone {
		if rs.parentRunID != "" {
			if parent := s.runs[rs.parentRunID]; parent != nil && parent.flowEngineDriven {
				deferGateCompleted = true
			}
		} else if rs.flowEngineDriven {
			// Root hub/coder turn: same defer so crash mid-gate cannot restore Completed.
			deferGateCompleted = true
		}
	}
	rs.events = append(rs.events, ev)
	rs.lastEventType = ev.Type
	rs.updatedAt = ev.OccurredAt
	// Task-241 T-11: stall detector measures "no provider event" from this stamp.
	rs.lastProviderEventAt = time.Now().UTC()
	// Arm delayed stall sweep for the parent when this child is in an open cohort
	// (Codex review Important #2 — do not rely solely on Continue).
	if rs.parentRunID != "" && rs.flowCohortId != "" {
		go s.maybeScheduleStallCheck(rs.parentRunID)
	}
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
	if !deferGateCompleted {
		_ = s.persistEvent(ev)
	}

	switch ev.Type {
	case EventPermissionRequired:
		rs.status = RunStatusWaitingApproval
		rs.agentStatus = string(RunStatusWaitingApproval)
		// BUG-288 #22: flow-engine child approval must stamp the parent step
		// WAITING_USER_APPROVAL (by node label) so resume/replay can keep it
		// instead of collapsing RUNNING → CANCELED after restart.
		s.settleFlowChildStepAwaitingUserLocked(rs)
	case EventUserQuestionRequired:
		rs.status = RunStatusWaitingQuestion
		rs.agentStatus = string(RunStatusWaitingQuestion)
		// BUG-288 #22: same WAITING stamp for child ask_user questions.
		s.settleFlowChildStepAwaitingUserLocked(rs)
	case EventTurnCompleted:
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
		if rs.parentRunID != "" {
			// Task-242 D-1/D-3/D-9 / BUG-288 #1 / V10: do NOT publish Completed until
			// the post-turn child gate passes. Durable flag + turn snapshot survive
			// restart (V10 residual P0).
			if parent := s.runs[rs.parentRunID]; parent != nil && parent.flowEngineDriven {
				// R17-P1: ignore bool — gateCheckpointNotDurable blocks pass later.
				_ = s.markPendingFlowGateSettleLocked(rs, finalMsg, ev.OccurredAt)
				break
			}
			rs.status = RunStatusCompleted
			rs.agentStatus = string(RunStatusCompleted)
			s.agentOrchestrator.signalChild(rs.id, finalMsg, false, "", RunStatusCompleted)
			s.settleFlowChildTurnCompletedLocked(rs, finalMsg, ev)
		} else if !rs.turnStartedAfterLoopDone {
			// Root: defer Completed until post-turn gate when the turn touched
			// code. Plain chat previously published Completed immediately even
			// when code changed, so dispatch settle saw Completed and returned
			// allow:true before the gate queued its reprompt — UI stalled at
			// one auto-reprompt line (run-208282). Arming the same pending gate
			// contract as children/flow roots makes settle wait for the real
			// disposition. Plain turns without code (e.g. "hi") still complete
			// immediately to preserve the original 3-event shape.
			hasCode := false
			for _, e := range rs.events {
				if e.Type == EventFileChanged && e.Path != "" && !flowgate.IsDocOrAuditFile(e.Path) {
					hasCode = true
					break
				}
			}
			if hasCode {
				_ = s.markPendingFlowGateSettleLocked(rs, finalMsg, ev.OccurredAt)
			} else {
				rs.status = RunStatusCompleted
				rs.agentStatus = string(RunStatusCompleted)
				s.agentOrchestrator.signalChild(rs.id, finalMsg, false, "", RunStatusCompleted)
				break
			}
		} else {
			// BUG-305: a plain follow-up on an already-"done" loop (rs.turnStartedAfterLoopDone)
			// falls through here and completes like normal chat — publish Completed and let
			// this event broadcast live (deferGateCompleted was likewise skipped above), so the
			// desktop's turn stream ends instead of hanging on a never-broadcast completion.
			rs.status = RunStatusCompleted
			rs.agentStatus = string(RunStatusCompleted)
			s.agentOrchestrator.signalChild(rs.id, finalMsg, false, "", RunStatusCompleted)
		}
	case EventTurnFailed:
		// BUG-288 R13-02: stall-retry cancel is not a real member failure.
		if rs.stalledRetrySuppressCohort {
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			break
		}
		// CP-58 run-203966: the flow park's own turn cancel (from the adapter
		// mid-cancel or from finishTurn itself) is not a real failure either —
		// keep the run non-terminal and skip the root-flow failure side
		// effects (canonical-head abandon, signalChild).
		if rs.parkCancelSuppress {
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			break
		}
		rs.status = RunStatusFailed
		rs.agentStatus = string(RunStatusFailed)
		s.agentOrchestrator.signalChild(rs.id, "", true, ev.Error, RunStatusFailed)
		// CP-55 P-5: this run IS the Flow root/hub failing (a child's own
		// failure is handled by the cohort/notify branch below and does not
		// itself terminate the parent Flow) — the Flow never reached genuine
		// terminal acceptance, so abandon any Canonical Head updates its
		// coder children staged. Async + best-effort (abandonPendingCanonicalHeadsForRun
		// already only logs on error) so file I/O never runs while s.mu is held.
		if rs.parentRunID == "" && rs.flowEngineDriven {
			if cwd := strings.TrimSpace(rs.workspaceCwd); cwd != "" {
				failedRunID := rs.id
				// Review finding I-3: also abandon for every direct child — a
				// sub-hub coding child stages under its own (sub-hub) parent
				// run ID, not this root's, so a root-only abandon would leave
				// that nested pending record dangling forever.
				childIDs := s.agentOrchestrator.listChildren(failedRunID)
				go func() {
					abandonPendingCanonicalHeadsForRun(cwd, failedRunID, "flow failed", s.markerSecret)
					for _, childID := range childIDs {
						abandonPendingCanonicalHeadsForRun(cwd, childID, "flow failed", s.markerSecret)
					}
				}()
			}
		}
		if rs.parentRunID != "" {
			if rs.flowCohortId != "" {
				// V9-25: stall Skip already synthetic-appended; ignore late cancel fail.
				if rs.cohortSkipConsumed {
					break
				}
				// Also skip if already buffered (memberAlreadyBuffered).
				if s.agentOrchestrator.memberAlreadyBuffered(rs.parentRunID, rs.flowCohortId, rs.label) {
					break
				}
				s.agentOrchestrator.appendCohortResult(rs.parentRunID, rs.flowCohortId, cohortEntry{
					Label:    rs.label,
					Provider: string(rs.providerKey),
					Status:   "failed",
					Err:      truncateDisplayField(ev.Error, 500),
				})
				// run-43831: cohort member terminal failure is still parent flow progress.
				s.touchParentHubProgressFromChildLocked(rs)
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
					// Caller holds s.mu (emitLocked EventTurnFailed). Stamp the
					// real failure reason (run-142155: codex reviewer fail showed
					// "(no detail from runner)" because the cohort path used the
					// reason-less setFlowStepStatusLocked, unlike the non-cohort
					// delegate-fail path which uses setFlowStepFailedWithReasonLocked).
					s.setFlowStepFailedWithReasonLocked(context.Background(), rs.parentRunID, rs.label, truncateDisplayField(ev.Error, 500))
					cohortDiagLog("member self-settled FAILED parent=%q run=%q label=%q", rs.parentRunID, rs.id, rs.label)
				}
				s.ensureCohortExpectedLocked(rs.parentRunID, rs.flowCohortId)
				if s.agentOrchestrator.cohortComplete(rs.parentRunID, rs.flowCohortId) {
					entries := s.agentOrchestrator.drainCohort(rs.parentRunID, rs.flowCohortId)
					note := buildCohortNote(rs.parentRunID, rs.flowCohortId, entries, s.agentOrchestrator.graphSnapshot(rs.parentRunID).LoopState.Round)
					s.flowDiagLog(rs.parentRunID, "cohort_join_complete_after_failure", "cohort barrier completed after member failure",
						"cohort_id", rs.flowCohortId,
						"entry_count", len(entries),
						"note_len", len(note),
					)
					// BUG-307: same sealed-loop guard as the completed-join path
					// above — a failed cohort member can complete the join after
					// the loop already stopped/done.
					if !s.loopSealedForReinvoke(rs.parentRunID) {
						s.appendPendingAgentContextLocked(rs.parentRunID, note)
					}
					if parent := s.runs[rs.parentRunID]; parent != nil {
						parent.lastCohortNote = note // BUG-233: retained for the CA-226 fallback GateReason
						// BUG-289 L4/F-10: stamp hub RUNNING on failed-member join
						// (completed-join already does this at :3587-3589).
						hubNodeID := parent.activeHubNodeID
						if hubNodeID == "" {
							hubNodeID = hubInlineNodeID(parent.activeFlowNodes)
						}
						// CP-58 Task-304: a failed member completing the cohort
						// must still activate ITS hub on dual-hub flows (the
						// member's forward edge names it), not first-match.
						if resolved, dual := hubNodeIDForCohortJoin(parent.activeFlowNodes, parent.activeFlowEdges, []string{rs.label}); dual && resolved != "" {
							hubNodeID = resolved
							parent.activeHubNodeID = resolved
						}
						if hubNodeID != "" && s.loopIsAdvancing(rs.parentRunID) {
							s.setFlowStepStatusLocked(context.Background(), rs.parentRunID, hubNodeID, StepStatusRunning)
						}
					}
					parentRunID := rs.parentRunID
					capturedCohortNote := note // embed note directly in synthesis prompt (BUG-synthesis-hang)
					go s.maybeAutoReinvokeHubWithNote(parentRunID, capturedCohortNote)
				}
			} else {
				// Non-cohort child failure (run-1618 / CA-355 + H-A residual).
				if parent := s.runs[rs.parentRunID]; parent != nil && parent.flowEngineDriven {
					s.notifyHubOfFlowChildFailureLocked(rs, ev.Error)
				} else if rs.uiInitiated || !rs.waitForResult {
					s.appendPendingAgentContextLocked(rs.parentRunID, fmt.Sprintf(
						"Sub-agent %q (provider: %s) failed: %s",
						rs.agentName, rs.providerKey, truncateDisplayField(ev.Error, 500)))
				}
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
			// BUG-327: never unpark a child already stamped WAITING_USER_APPROVAL
			// (escalate/cap park). A late stray event (e.g. a second
			// flow_gate_violation) must not flip it back to Running — that would
			// re-arm the TUI Thinking timer and hide Continue/Stop.
			if rs.status != RunStatusWaitingUserApr {
				rs.status = RunStatusRunning
				if rs.parentRunID != "" {
					rs.agentStatus = string(RunStatusRunning)
				}
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
		// BUG-334: the child's isolated opencode acp process is throwaway —
		// tear it down with the child's terminal event so short-lived children
		// do not leak processes. Chat-scope handles are never touched.
		if s.runner != nil {
			s.runner.CloseOpencodeProcessesForChildRun(rs.id)
		}
	}
	// Flow-engine children: releaseDependentAgents runs only after gate pass
	// (settleFlowChildTurnCompletedLocked / post-gate branch). Immediate release
	// would start reviewers before a coding-child reprompt can fire (Task-242 D-1/D-9).
	if rs.parentRunID != "" && ev.Type == EventTurnCompleted && !rs.pendingFlowGateSettle {
		go s.releaseDependentAgents(rs.parentRunID, rs.id, ev.FinalMessage, ev.OccurredAt)
	}
	if rs.parentRunID != "" && shouldEmitParentGraph {
		// Still emit graph for deferred completed so UI sees Running (not terminal).
		s.emitAgentGraphLocked(rs.parentRunID, s.agentOrchestrator.graphSnapshot(rs.parentRunID))
	}

	// V10 P1: do not broadcast deferred TurnCompleted to stream subscribers.
	if !deferGateCompleted {
		for _, ch := range rs.subs {
			select {
			case ch <- ev:
			default: // subscriber slow/full — it reconnects via afterSeq, no gap
			}
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
		if e.Seq <= after {
			continue
		}
		// BUG-StaleQuestion: a resolved question must still be visible on
		// reconnect (e.g. switches view and comes back) — just not
		// interactive. Stamp the resolved choice onto the replayed event so
		// the client renders the QuestionCard in its read-only "answered"
		// state instead of a fresh interactive form. An expired question (no
		// answer to show) is dropped, matching the pre-existing behavior.
		if e.Type == EventUserQuestionRequired && e.QuestionID != "" {
			if rec := s.questions[e.QuestionID]; rec != nil {
				switch rec.status {
				case "resolved":
					e.Answer = append([]string(nil), rec.choice...)
				case "expired":
					continue
				}
			}
		}
		snapshot = append(snapshot, e)
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
	// posture is the effective chat posture for THIS turn (scan/plan/code, "" = code).
	// Scan/Plan are read-only: RequestApproval auto-approves reads and auto-denies writes
	// without asking (checked before the YOLO branch so a profile YOLO never leaks a
	// write through).
	posture string
}

func (b *turnBridge) Emit(ev ProviderEvent) {
	b.svc.mu.Lock()
	if ev.ProviderTurnID == "" {
		ev.ProviderTurnID = b.turnID
	}
	b.svc.emitLocked(b.rs, ev)
	b.svc.mu.Unlock()
	// CP-51 Task-249: provider-backed terminal events drive CommitTerminalAndSettleIntent.
	// A SendTurn error is never proof — only completed/failed/cancelled terminal events.
	switch ev.Type {
	case EventTurnCompleted, EventTurnFailed:
		outcome := "completed"
		if ev.Type == EventTurnFailed {
			outcome = "failed"
		}
		// Cancelled is represented as failed+context cancel in some adapters; keep closed enum.
		if ev.Type == EventTurnFailed && (ev.Error == "context canceled" || strings.Contains(strings.ToLower(ev.Error), "cancel")) {
			outcome = "cancelled"
		}
		payload, _ := json.Marshal(map[string]string{
			"type": string(ev.Type), "error": ev.Error, "final": ev.FinalMessage,
		})
		b.Terminal(TerminalEvidence{
			ProviderKey:          string(b.rs.providerKey),
			EvidenceKind:         string(ev.Type),
			Outcome:              outcome,
			PayloadCanonicalJSON: payload,
			PayloadSHA256:        HashBytes(payload),
			ObservedAt:           nowRFC3339Nano(),
		})
	}
}

func (b *turnBridge) RequestApproval(details ApprovalDetails) (string, error) {
	s := b.svc

	// Task-242 T-4 / D-7: deny git commit on flow-mode coding children BEFORE
	// YOLO auto-approve (denylist after YOLO is bypassed when YOLO=on). Commit
	// is reserved for the audit/commit-prep step (CP-41 P-6).
	if isFlowCodingCommitAttempt(s, b.rs, details) {
		s.recordAutoApproval(b.rs, details, "deny", "flow_coding_commit_reserved_for_audit")
		return "deny", nil
	}

	// Read-only posture (Scan/Plan): auto-decide WITHOUT asking the human. Reads
	// are approved, writes and anything unclassified are denied, and the reply is
	// recorded like every auto-decision so the adapter never hangs. Checked BEFORE
	// the YOLO branch below so a profile YOLO never auto-approves a write through
	// a read-only posture (the posture's permission discipline wins).
	if IsReadOnlyChatPosture(b.posture) {
		decision := readOnlyApprovalDecision(details)
		s.recordAutoApproval(b.rs, details, decision, "posture_read_only_"+b.posture)
		return decision, nil
	}

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
	expiresAt := time.Now().UTC().Add(s.approvalTTL).Format(time.RFC3339Nano)
	rec := &approvalRecord{
		id:        s.nextID("appr"),
		runID:     b.rs.id,
		details:   details,
		status:    "pending",
		resolve:   make(chan string, 1),
		expiresAt: expiresAt,
	}
	s.approvals[rec.id] = rec
	b.rs.pendingApprovalID = rec.id
	// Persist pending BEFORE emit/wait (BUG-288 P1-07) so a persist failure
	// never leaves a card that is emitted/waited-on in-process but invisible
	// on disk. ExpiresAt is now the durable absolute TTL deadline (BUG-288
	// P1-08) instead of empty, so a restart can recompute the remaining TTL.
	pendingSnap := approvalStateFromRecord(b.rs, rec, expiresAt)
	s.mu.Unlock()
	if err := s.persistApproval(pendingSnap); err != nil {
		s.mu.Lock()
		delete(s.approvals, rec.id)
		if b.rs.pendingApprovalID == rec.id {
			b.rs.pendingApprovalID = ""
		}
		s.mu.Unlock()
		return "", fmt.Errorf("persist pending approval: %w", err)
	}
	s.mu.Lock()
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
	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	rec := &questionRecord{
		id:          s.nextID("q"),
		runID:       b.rs.id,
		prompt:      prompt,
		options:     options,
		multiSelect: multiSelect,
		status:      "pending",
		resolve:     make(chan questionResolveResult, 1),
		expiresAt:   expiresAt,
	}
	s.questions[rec.id] = rec
	b.rs.pendingQuestionID = rec.id
	// Persist pending BEFORE emit/wait (BUG-288 P1-07); ExpiresAt is now the
	// durable absolute TTL deadline (BUG-288 P1-08).
	pendingSnap := questionStateFromRecord(rec, b.turnID, expiresAt)
	s.mu.Unlock()
	if err := s.persistQuestion(pendingSnap); err != nil {
		s.mu.Lock()
		delete(s.questions, rec.id)
		if b.rs.pendingQuestionID == rec.id {
			b.rs.pendingQuestionID = ""
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("persist pending question: %w", err)
	}
	s.mu.Lock()
	s.emitLocked(b.rs, ProviderEvent{
		Type:           EventUserQuestionRequired,
		ProviderTurnID: b.turnID,
		QuestionID:     rec.id,
		Prompt:         prompt,
		Options:        options,
		MultiSelect:    multiSelect,
	})
	// CA-642: a model ask_user question on a flow-engine child (planner/tester/
	// coder) lives on the child's event stream, but the TUI/desktop subscribe to
	// the root run stream — the operator sees the step WAITING_USER_APPROVAL but
	// no card to answer. Mirror the question onto the root flow run's stream
	// (walking nested sub-hub chains); AnswerQuestion resolves globally by
	// question id, so answering from the main timeline unblocks the child.
	if rootID := s.flowRootIDLocked(b.rs); rootID != "" {
		if root := s.runs[rootID]; root != nil {
			s.emitLocked(root, ProviderEvent{
				Type:        EventUserQuestionRequired,
				QuestionID:  rec.id,
				Prompt:      prompt,
				Options:     options,
				MultiSelect: multiSelect,
			})
		}
	}
	s.mu.Unlock()

	timer := time.NewTimer(s.questionTTL)
	defer timer.Stop()
	select {
	case r := <-rec.resolve:
		// BUG-288 P2-02: a typed result lets interruption win the select race
		// against ctx.Done() without being mistaken for a real empty answer.
		if r.err != nil {
			return nil, r.err
		}
		return r.choices, nil
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
// opencodeChildScopeHint returns the run id for spawned child runs (parentRunID
// set) so the opencode factory isolates their acp process (BUG-334); "" for
// normal runs, which keep the shared chat process (BUG-329 reuse).
func opencodeChildScopeHint(rs *interactiveRun) string {
	if rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return ""
	}
	return strings.TrimSpace(rs.id)
}

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
		targetParentID := b.rs.parentRunID
		if targetParentID == "" {
			return FlowControlResult{}, fmt.Errorf(
				"this is a cohort review step, not the hub: do not call the flow control tool here. " +
					"Report your findings (approve or request changes, with specifics) in your final message; " +
					"the hub will synthesize the full cohort and finalize the flow after every reviewer has finished")
		}
		b.svc.mu.Lock()
		parent := b.svc.runs[targetParentID]
		requireVerdict := parent != nil && flowRequiresSynthesisMachineVerdict(parent)
		b.svc.mu.Unlock()
		if requireVerdict {
			if !in.viaReviewOutcome {
				return FlowControlResult{}, fmt.Errorf(
					"reviewer cohort member: call submit_review_outcome with status=approved|changes_requested|blocked to record a machine verdict")
			}
			label := strings.TrimSpace(b.rs.label)
			if label == "" {
				label = strings.TrimSpace(b.rs.agentName)
			}
			b.svc.recordReviewCohortMemberVerdict(targetParentID, label, in.reviewOutcomeStatus)
			return FlowControlResult{
				Status:     in.reviewOutcomeStatus,
				NextAction: "review_verdict_recorded",
			}, nil
		}
		return FlowControlResult{}, fmt.Errorf(
			"this is a cohort review step, not the hub: do not call the flow control tool here. " +
				"Report your findings (approve or request changes, with specifics) in your final message; " +
				"the hub will synthesize the full cohort and finalize the flow after every reviewer has finished")
	}
	targetRunID := b.rs.id
	if b.rs.parentRunID != "" {
		targetRunID = b.rs.parentRunID
	}
	if b.rs.currentTurnID != "" && b.svc.flowControlSubmittedForTurn(targetRunID, b.rs.currentTurnID) {
		return FlowControlResult{}, fmt.Errorf("flow control already submitted for this provider turn")
	}
	if res, handled := b.svc.advanceHubDoneThroughEdge(targetRunID, in); handled {
		return res, nil
	}
	return b.svc.applyFlowControl(targetRunID, in)
}

// advanceHubDoneThroughEdge generalizes flow completion so a hub/synthesis node
// is not hardcoded as the flow's last node. When the flow's inline hub
// (hub.inline / synthesis) node has a forward "done" edge to a REAL successor
// node — not the terminal "done"/"ask_user" — a synthesizer's
// submit_review_outcome(done) must dispatch that successor (e.g. a
// telegram.notify node that sends the run summary) instead of settling the
// whole flow immediately. It returns (result, true) when it took over the
// transition; (_, false) means "not applicable — settle exactly as before".
//
// The built-in flows are unaffected: review-loop / context-coding-review-
// synthesis both wire "synthesis --done--> done" (the terminal), which resolves
// to "done" here and returns false, so applyFlowControl settles them unchanged.
// Only a user-authored flow that points synthesis at another node takes the new
// path. Non-flow runs (no tracked topology / no hub.inline node) also return
// false.
func (s *InteractiveService) advanceHubDoneThroughEdge(targetRunID string, in FlowControlInput) (FlowControlResult, bool) {
	if in.Status != "done" || !s.isFlowEngineDriven(targetRunID) {
		return FlowControlResult{}, false
	}
	s.mu.Lock()
	rs := s.runs[targetRunID]
	var edges []agentpack.FlowEdge
	var nodes []agentpack.FlowNode
	var activeHubNodeID string
	if rs != nil {
		edges = rs.activeFlowEdges
		nodes = rs.activeFlowNodes
		activeHubNodeID = rs.activeHubNodeID
	}
	s.mu.Unlock()
	// Task-235: a second (or later) hub-driven node — e.g. hub.notify reached
	// after synthesis — must resolve this "done" against ITS OWN forward edge,
	// not synthesis's again. Fall back to the flow's sole hub.inline node only
	// when no hub-driven node has taken over yet (every pre-Task-235 flow).
	hubID := activeHubNodeID
	if hubID == "" {
		hubID = hubInlineNodeID(nodes)
	}
	if hubID == "" {
		return FlowControlResult{}, false
	}
	target, ok := edgeTargetFrom(edges, hubID, "done", "forward")
	if !ok || target == "" || target == "done" || target == "ask_user" {
		s.mu.Lock()
		if rs := s.runs[targetRunID]; rs != nil {
			rs.activeHubNodeID = ""
		}
		s.mu.Unlock()
		return FlowControlResult{}, false
	}
	if targetNode, ok := findFlowNode(nodes, target); ok {
		if canonical, ok := agentpack.NormalizeBehaviorID(targetNode.Behavior); ok && canonical == "hub.notify" {
			// hub.notify runs as another turn of the SAME hub session (Task-235),
			// not a Go-deterministic handler and not a spawned child. Leave the
			// loop running — it settles for real once that turn calls
			// flow_control("done") and this function runs again, resolved against
			// the new activeHubNodeID dispatchHubNotifyNode just set.
			s.dispatchHubNotifyNode(targetRunID, targetNode)
			// BUG-289 A6/F-9: stamp the one-decision guard so a second same-turn
			// flow_control cannot bypass I-5 and settle past hub.notify early.
			s.mu.Lock()
			if rs := s.runs[targetRunID]; rs != nil && rs.currentTurnID != "" {
				rs.lastFlowControlTurnID = rs.currentTurnID
			}
			s.mu.Unlock()
			st := s.agentOrchestrator.loopStateFor(targetRunID)
			// BUG-284 follow-up: report Status:"done" (not "continue") — the
			// caller's own review verdict WAS accepted; "continue" reads to the
			// model as "your done call was rejected, keep looping" (observed live:
			// a synthesizer turn called done(), got "continue" back, then called
			// escalate() in the same turn out of apparent confusion, which — via a
			// separate gap this task also fixes — caused the eventual notify send
			// to be silently dropped). NextAction "advancing" (not "looping") is
			// similarly non-review-loop-specific language for this generic engine.
			return FlowControlResult{Status: "done", Round: st.Round, Cap: effectiveCap(st), OpenIssues: st.OpenIssues, NextAction: "advancing"}, true
		}
	}
	// Dispatch the successor chain synchronously (telegram.notify / audit /
	// contract.freeze / etc. are Go-inline); a terminal reached at the end still
	// calls applyFlowControl, so the loop settles for real by the time this returns.
	ok = s.advanceToNextInlineOrDelegate(context.Background(), targetRunID, edges, nodes, hubID, "done", in.Summary)
	if !ok {
		// run-201295: the done successor (e.g. contract.freeze) could not be
		// dispatched. CA-731 stamped the one-decision guard and reported
		// "advancing" anyway, so BUG-226's escalate was gated off and the flow
		// idled RUNNING until the 2m watchdog parked hub_stalled (plan_synthesis
		// WAITING_USER_APPROVAL with no successor and no actionable card). Fail
		// closed instead: escalate THIS turn so the operator gets a Retry/Stop
		// card immediately. applyFlowControl stamps the one-decision guard
		// itself; only stamp manually if the escalate path rejected it.
		res, escErr := s.applyFlowControl(targetRunID, FlowControlInput{
			Status:  "escalate",
			Summary: fmt.Sprintf("Flow done successor %q could not be dispatched; escalating for review", target),
		})
		if escErr != nil {
			s.mu.Lock()
			if rs := s.runs[targetRunID]; rs != nil && rs.currentTurnID != "" {
				rs.lastFlowControlTurnID = rs.currentTurnID
			}
			s.mu.Unlock()
			log.Printf("[flow-executor] escalate after undispatchable done successor: %v", escErr)
			st := s.agentOrchestrator.loopStateFor(targetRunID)
			return FlowControlResult{Status: "blocked", Round: st.Round, Cap: effectiveCap(st), OpenIssues: st.OpenIssues, NextAction: "awaiting_user"}, true
		}
		return res, true
	}
	// BUG-353 (run-198468): the hub.notify branch stamps the one-decision guard
	// (BUG-289), but this generic-successor branch never did — so a hub whose
	// submit_review_outcome(approved) routed through a real successor edge
	// (plan_synthesis --done--> preflight_contract_freeze) still read as
	// "completed without submit_review_outcome", and BUG-226 escalated the hub
	// to WAITING_USER_APPROVAL even though the tool succeeded (reproduced 3x
	// incl. a Retry). Stamp here exactly like the hub.notify branch so the
	// completed turn counts as its flow-control decision.
	s.mu.Lock()
	if rs := s.runs[targetRunID]; rs != nil && rs.currentTurnID != "" {
		rs.lastFlowControlTurnID = rs.currentTurnID
	}
	s.mu.Unlock()
	st := s.agentOrchestrator.loopStateFor(targetRunID)
	nextAction := "looping"
	status := st.Status
	switch st.Status {
	case "done":
		nextAction = "done"
	case "blocked":
		nextAction = "awaiting_user"
	case "running":
		// BUG-284 follow-up, extended from the hub.notify branch: report the
		// caller's verdict as accepted. "continue"/"looping" reads to the model
		// as "your done call was rejected, keep looping" (observed live: a
		// synthesizer called escalate() in the same turn after getting
		// "continue"). The engine is still advancing internally, but the model
		// must not retry the decision.
		status = "done"
		nextAction = "advancing"
	}
	return FlowControlResult{Status: status, Round: st.Round, Cap: effectiveCap(st), OpenIssues: st.OpenIssues, NextAction: nextAction}, true
}

// dispatchHubNotifyNode (Task-235) is the single entry point for reaching a
// hub.notify node, shared by advanceHubDoneThroughEdge (reached from a
// hub-driven node's own "done") and advanceToNextInlineOrDelegate (reached
// from any other inline node's forward edge, e.g. audit --done--> notify) so
// hub.notify behaves identically regardless of what precedes it in the graph.
// Marks node as the run's active hub node (so its OWN eventual "done" resolves
// against its own forward edge, not an earlier hub node's) and reinvokes the
// hub session with node's write-contract prompt.
//
// BUG-284: also stamps node RUNNING on the step timeline immediately. Without
// this the node stays PENDING until the flow settles — markFlowRunComplete
// (flow_step_runtime.go) sweeps any still-PENDING step to SKIPPED (only a
// RUNNING/other-non-terminal step is swept to DONE), so a hub.notify node
// whose reinvoke got deferred (the common case — see
// maybeAutoReinvokeHubWithPrompt) rendered as silently "skipped" even on a run
// where the notification never actually needed to be dropped.
func (s *InteractiveService) dispatchHubNotifyNode(parentRunID string, node agentpack.FlowNode) {
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.activeHubNodeID = node.ID
	}
	s.mu.Unlock()
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(context.Background(), parentRunID, node.ID, StepStatusRunning)
		s.stampFlowNodePosture(context.Background(), parentRunID, node)
	}
	s.maybeAutoReinvokeHubWithPrompt(parentRunID, composeHubNotifyPrompt(node))
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
		// in.Agent may be a bare agent name (built-in flow-pack agents and
		// name-authored refs) or a full definition path (the Agent-ref dropdown
		// stores agent.path to disambiguate same-named files across sources).
		// Match precisely first — Name, then full Path — so a path always
		// resolves; otherwise agentDef stays nil and the child runs with the
		// raw task prompt (no system prompt, no identity line).
		for i := range defs {
			if strings.EqualFold(defs[i].Name, in.Agent) ||
				(defs[i].Path != "" && strings.EqualFold(defs[i].Path, in.Agent)) {
				def := defs[i]
				agentDef = &def
				break
			}
		}
		// Fallback: degrade a path that did not match exactly to its base name
		// without extension, then match that against Name. This rescues a
		// hand-typed or extensionless ref (e.g. "...\coder-agent" for the file
		// "...\coder-agent.toml") and a definition that has since moved, rather
		// than silently spawning an agent with no identity.
		if agentDef == nil {
			base := strings.TrimSuffix(filepath.Base(in.Agent), filepath.Ext(in.Agent))
			if base != "" && !strings.EqualFold(base, in.Agent) {
				for i := range defs {
					if strings.EqualFold(defs[i].Name, base) {
						def := defs[i]
						agentDef = &def
						break
					}
				}
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
		// CP-51 Task-252: stamp the mint-time provenance for the trusted FCP marker
		// embedded in this child's first prompt (Prompt was composed with
		// ComposeFlowCodingPrompt(pkg, ...) by the caller, embedding pkg.WorkflowRunID
		// — typically the flow/hub run, not this new child's own id). Recorded here,
		// under the same lock as run creation, so the child's own first-turn
		// feature-history check (isFlowContextHandoffWithSecret) can trust this
		// specific recorded binding instead of inferring trust from parentRunID
		// topology alone (DOD-I4).
		if in.FCPMarkerProvenanceRunID != "" && in.FCPMarkerProvenanceRunID != rs.id {
			rs.markerProvenanceRunIDs = append(rs.markerProvenanceRunIDs, in.FCPMarkerProvenanceRunID)
		}
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
		Label:         in.Label,
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
				// startTurn failed before the adapter ran — signal waiter + settle
				// flow/cohort (H-A non-cohort + BUG-289 H2/F-2 cohort).
				s.handleChildStartTurnFailure(handle.RunID, parentRunID, turnErr.msg)
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
				// ChildRunID is required so the desktop binds the result to the correct
				// card (and the correct reinvoke activation). Flow wait:false children
				// are covered by emitParentAgentResultLocked in settle; this Wait=true
				// path still needs ChildRunID for UI-spawned sequential spawns.
				if result.FinalMessage != "" {
					s.emitOnParentRun(parentRunID, ProviderEvent{
						Type:         EventAgentResultInjected,
						AgentName:    childSnap.AgentName,
						ChildRunID:   handle.RunID,
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
			Label:         rs.label,
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
				// BUG-251: a child neither in the live map (liveIDs) nor the
				// orchestrator's in-memory historical cache is being read straight
				// from disk after a restart -- nothing is actually tracking or
				// executing it anymore. reconstructRun already normalizes this exact
				// situation for a run's own top-level status via
				// normalizeResumedStatus (running/starting/waiting_* -> cancelled);
				// this disk-fallback branch skipped that normalization entirely, so
				// a reviewer/coder child whose process was killed mid-turn showed a
				// permanently stale "running" badge in the Agents panel with no way
				// to ever tell it apart from one that's genuinely still executing.
				out = append(out, AgentRunSummary{
					RunID:     session.RunID,
					AgentName: session.AgentName,
					// BUG-320: preserve the persisted Label so matchFlowNodeForSession
					// (interactive_resume.go) can match this child back to its exact
					// flow node by id -- falling back to AgentName/Role risks an
					// ambiguous match when two nodes share the same agent (e.g. two
					// reviewer nodes in one flow).
					Label:       session.Label,
					Role:        session.Role,
					Status:      normalizeResumedStatus(session.Status),
					ParentRunID: session.ParentRunID,
					CreatedAt:   session.StartedAt,
					DependsOn:   append([]string(nil), session.DependsOn...),
					AgentStatus: string(normalizeResumedStatus(RunStatus(session.AgentStatus))),
					ProviderKey: string(session.ProviderKey),
					ModelName:   session.ModelName,
				})
				seenIDs[session.RunID] = struct{}{}
			}
		}
	}
	return out
}

// approvalExpiryElapsed reports whether a durable ExpiresAt (RFC3339Nano) has
// already passed. Empty/unparseable values are treated as not-yet-expired so
// old records without a durable TTL (pre-P1-08) do not spuriously expire.
func approvalExpiryElapsed(expiresAt string) bool {
	if strings.TrimSpace(expiresAt) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return false
	}
	return time.Now().UTC().After(t)
}

// scheduleApprovalExpiry arranges for a rehydrated pending approval to expire
// durably once its remaining TTL elapses (BUG-288 P1-08) — a rehydrated
// record has no in-process caller blocked in a select{} with its own timer,
// so without this it would only ever expire if a live caller happened to
// call SubmitApprovalDecision after the deadline.
func scheduleApprovalExpiry(s *InteractiveService, id, expiresAt string) {
	if strings.TrimSpace(expiresAt) == "" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return
	}
	remaining := time.Until(t)
	if remaining <= 0 {
		remaining = time.Millisecond
	}
	time.AfterFunc(remaining, func() { s.expireApproval(id) })
}

// scheduleQuestionExpiry is the question-side twin of scheduleApprovalExpiry.
func scheduleQuestionExpiry(s *InteractiveService, id, expiresAt string) {
	if strings.TrimSpace(expiresAt) == "" {
		return
	}
	t, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return
	}
	remaining := time.Until(t)
	if remaining <= 0 {
		remaining = time.Millisecond
	}
	time.AfterFunc(remaining, func() { s.expireQuestion(id) })
}

func (s *InteractiveService) expireApproval(id string) {
	var snapshot *ProviderApprovalState
	var settleRunID string
	s.mu.Lock()
	if rec := s.approvals[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		rs := s.runs[rec.runID]
		if rs != nil && rs.pendingApprovalID == id {
			rs.pendingApprovalID = ""
			// BUG-289 H4/F-4: flip off WAITING so the run is not 409-locked forever
			// after TTL (live path relied on blocked RequestApproval receiving
			// errApprovalExpired — that goroutine is gone after restart).
			if rs.status == RunStatusWaitingApproval {
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
				settleRunID = rs.id
				touchHubProgressLocked(rs)
			}
		}
		// BUG-288 P1-07: always persist the expiry transition, not only when a
		// live rs still points its pendingApprovalID at this card — otherwise a
		// rehydrated/orphaned card's expiry never reaches disk.
		state := approvalStateFromRecord(rs, rec, rec.expiresAt)
		snapshot = &state
	}
	s.mu.Unlock()
	if snapshot != nil {
		if err := s.persistApproval(*snapshot); err != nil {
			// BUG-288 P1-07: do not silently swallow — an un-persisted expiry
			// means a restart could resurrect this card as pending again. Best
			// effort within this change's scope: log loudly so it is visible in
			// ops/monitoring rather than vanishing; a full retryable outbox is a
			// larger change out of scope here.
			log.Printf("[gate] persist approval expiry %s: %v (expiry left un-durable — may resurrect as pending on restart)", id, err)
		}
	}
	if settleRunID != "" {
		s.notifyTurnIdle(settleRunID)
		s.maybeScheduleHubStallCheck(settleRunID)
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
	var settleRunID string
	s.mu.Lock()
	if rec := s.questions[id]; rec != nil && rec.status == "pending" {
		rec.status = "expired"
		if rs := s.runs[rec.runID]; rs != nil && rs.pendingQuestionID == id {
			rs.pendingQuestionID = ""
			// BUG-289 H4/F-4: flip off WAITING (mirror expireApproval).
			if rs.status == RunStatusWaitingQuestion {
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
				settleRunID = rs.id
				touchHubProgressLocked(rs)
			}
		}
		state := questionStateFromRecord(rec, "", rec.expiresAt)
		snapshot = &state
	}
	s.mu.Unlock()
	if snapshot != nil {
		if err := s.persistQuestion(*snapshot); err != nil {
			// BUG-288 P1-07: see expireApproval's mirrored comment.
			log.Printf("[gate] persist question expiry %s: %v (expiry left un-durable — may resurrect as pending on restart)", id, err)
		}
	}
	if settleRunID != "" {
		s.notifyTurnIdle(settleRunID)
		s.maybeScheduleHubStallCheck(settleRunID)
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

// resolveTurnModelAndEffort applies this turn's model/reasoning-effort override
// onto the run-level default (BUG-063) — the single source of truth used both
// before the adapter is constructed (startTurn, so Grok's process-launch-time
// model selection sees the right value) and when building TurnRequest (runTurn).
func resolveTurnModelAndEffort(rs *interactiveRun, in TurnInput) (model, effort string) {
	effort = rs.reasoningEffort
	if in.ReasoningEffort != "" {
		effort = in.ReasoningEffort
	}
	model = rs.modelName
	if in.Model != nil {
		model = *in.Model
	}
	return model, effort
}

// turnResumeProviderSessionID is the id put on TurnRequest for this turn.
// Claude keeps the synthetic pool key (BUG-295). Grok/Codex prefer the durable
// real id. Grok also falls back to lastGrokTurnSessionID when the synthetic
// thread-* is still on providerSessionID and real is empty — a model-change
// respawn (run-92955) must session/load that ACP id, not session/new.
// Opencode mirrors the Grok fallback with lastOpencodeTurnSessionID (BUG-329,
// run-307050): a mid-chat model switch must session/load the real ses_* id.
func turnResumeProviderSessionID(rs *interactiveRun) string {
	if rs == nil {
		return ""
	}
	id := rs.providerSessionID
	if rs.realProviderSessionID != "" && rs.providerKey != ProviderKeyClaude {
		id = rs.realProviderSessionID
	}
	if rs.providerKey != ProviderKeyGrok && rs.providerKey != ProviderKeyOpencode {
		return id
	}
	trimmed := strings.TrimSpace(id)
	if trimmed != "" && !strings.HasPrefix(trimmed, "thread-") {
		return id
	}
	if rs.providerKey == ProviderKeyOpencode {
		if alt := strings.TrimSpace(rs.lastOpencodeTurnSessionID); isOpencodeRealSessionID(alt) {
			return alt
		}
		return id
	}
	if alt := strings.TrimSpace(rs.lastGrokTurnSessionID); isGrokRealSessionID(alt) {
		return alt
	}
	return id
}

func (s *InteractiveService) runTurn(ctx context.Context, rs *interactiveRun, adapter ProviderRuntimeAdapter, in TurnInput, scenario, turnID string, capturedCtx []string) {
	// Turn-level model/reasoning/YOLO override the run-level defaults when supplied
	// (BUG-063). Chat mode resends these every turn so they can change between prompts;
	// the providers re-apply them per turn (Codex thread/start per turn, Claude spawn-per-
	// turn, Grok respawn-per-turn). A nil pointer means "not supplied" and keeps the
	// run-level default.
	model, effort := resolveTurnModelAndEffort(rs, in)
	yolo := resolveTurnYolo(rs, in)
	posture := resolveTurnChatPosture(rs, in)
	log.Printf("[turn] run_id=%q model_override=%v resolved_model=%q effort=%q run_default=%q provider=%q turn_id=%q", rs.id, in.Model, model, effort, rs.modelName, rs.providerKey, turnID)
	// Fold any pending UI-spawn context into the provider prompt (NOT the displayed prompt,
	// which was already emitted via turn_started with in.Prompt). This is how the parent
	// agent learns about children started from the UI. Cleared once consumed; the cleared
	// state is persisted by the post-turn sessionStateOf snapshot below. (BUG-122)
	providerPrompt := in.Prompt
	var providerSessionID string
	s.mu.Lock()
	// Prefer the durable real provider handle when present (Codex rollouts and
	// Grok ACP session ids). Read under lock with lastGrokTurnSessionID fallback
	// (run-92955). Claude keeps the synthetic pool key (BUG-295).
	providerSessionID = turnResumeProviderSessionID(rs)
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
	// BUG-302: once the loop already reached "done" before this turn even
	// started, this turn is a plain chat follow-up, not the hub's own
	// review-decision turn — do not offer/require submit_review_outcome for
	// it (this also scopes BUG-226's "completed without calling it" escalate
	// fallback below, which is gated on this same flag), or a normal
	// follow-up answer gets misread as an abandoned review decision and
	// escalates a stale "Needs your decision" card.
	// BUG-308: "stopped" joins "done" here — a follow-up admitted onto a
	// terminal-sealed loop is a plain chat turn, never the hub's own
	// review-decision turn, so it must not offer submit_review_outcome or trip
	// BUG-226's escalate fallback (which would reopen a stale "Needs your
	// decision" card on the very next plain-prose message after a Stop).
	stAtTurnStart := s.agentOrchestrator.loopStateFor(rs.id).Status
	loopAlreadySealedAtTurnStart := stAtTurnStart == "done" || stAtTurnStart == "stopped"
	offerReviewOutcomeTool := rs.autoOrchestrate && rs.parentRunID == "" && rs.turnCount > 1 && !loopAlreadySealedAtTurnStart
	// CP-53 P-2: reviewer cohort members on synthesis-acceptance flows record
	// machine verdicts via submit_review_outcome (record-only — no flow advance).
	if !offerReviewOutcomeTool && rs.flowCohortId != "" && rs.parentRunID != "" {
		if parent := s.runs[rs.parentRunID]; parent != nil && flowRequiresSynthesisMachineVerdict(parent) {
			offerReviewOutcomeTool = true
		}
	}
	providerPrompt = prependModePrefix(providerPrompt, rs.turnCount, rs.changeType, rs.sourceDocID)
	// Persist the per-turn YOLO posture as the run's current default (BUG-129). The UI
	// toggle is sticky, so an explicit YoloMode this turn must update rs.yolo; otherwise a
	// child spawned during this turn (spawnChildRun reads parentRun.yolo) would inherit the
	// stale run-level default instead of the posture the user actually has enabled.
	// Also stick Flow-forced true so later follow-ups / children see the product lock
	// even when the turn request omitted yoloMode.
	if in.YoloMode != nil || shouldForceFlowYolo(rs.runKind, rs.workflowID, rs.flowEngineDriven) {
		rs.yolo = yolo
	}
	// Persist the per-turn posture as the run's current default for the SAME
	// reason YOLO is persisted just above: the approval bridge for the rest of
	// this turn (and any child spawned mid-turn) must see the posture the user
	// actually selected, and a scan/plan posture must stay read-only even after
	// the composing client stops resending it.
	if in.ChatPosture != "" || shouldForceFlowYolo(rs.runKind, rs.workflowID, rs.flowEngineDriven) {
		rs.chatPosture = posture
	}
	// Persist the per-turn model/reasoning-effort as the run's current default,
	// for the SAME reason YOLO is persisted just above. A child spawned during
	// this turn reads parentRun.modelName / parentRun.reasoningEffort in
	// spawnChildRun; without this it inherits the model the chat was CREATED
	// with, not the model the user switched to via the sticky per-turn override
	// (chat mode resends model/effort every turn). That stale inheritance made a
	// Grok child launch on the wrong model (a "sonnet" label on a grok child)
	// and, because model is a `grok agent` launch flag, forced a needless
	// respawn that tore down the parent's in-flight process mid-turn.
	if in.Model != nil {
		rs.modelName = *in.Model
	}
	if in.ReasoningEffort != "" {
		rs.reasoningEffort = effort
	}
	// pendingAgentContext was drained into capturedCtx by startTurn (atomically with
	// turnInFlight=true) so rs.pendingAgentContext is already nil here.
	if len(capturedCtx) > 0 {
		providerPrompt = composeAgentContextBlock(capturedCtx) + "\n\n" + in.Prompt
	}
	// Capture reattach state for envelope (first turn on reattached leg must carry
	// prior chat history — same envelope as live switch; TUI reattach via
	// createRun with ChatID+SwitchFromRunID does not fire a separate seed turn).
	reattachChatID := ""
	reattachSwitchFrom := ""
	reattachTurnCount := 0
	reattachLegSeq := 0
	reattachProviderKey := ProviderKey("")
	reattachModel := ""
	// At this point turnCount is already 1 for the first turn (incremented in
	// startTurn before runTurn), so check for 1, not 0. Observed in
	// TestReattachFirstTurnIncludesPriorHistory: first turn had turnCount=1.
	if rs.switchFromRunID != "" && rs.turnCount == 1 && rs.legSeq > 0 && !isHandoffPrompt(in.Prompt) && !isHandoffPrompt(providerPrompt) {
		reattachChatID = rs.chatID
		reattachSwitchFrom = rs.switchFromRunID
		reattachTurnCount = rs.turnCount
		reattachLegSeq = rs.legSeq
		reattachProviderKey = rs.providerKey
		reattachModel = rs.modelName
		log.Printf("[reattach] captured chatId=%q legSeq=%d switchFrom=%q", reattachChatID, reattachLegSeq, reattachSwitchFrom)
	}
	s.mu.Unlock()
	// Reattach envelope: prepend handoff history on first turn of reattached leg
	if reattachChatID != "" {
		log.Printf("[reattach] building envelope chatId=%q", reattachChatID)
		fakeSrc := &interactiveRun{providerKey: reattachProviderKey, id: reattachSwitchFrom, chatID: reattachChatID}
		req := chatSwitchRequest{TargetProviderKey: reattachProviderKey, Model: reattachModel, ReasoningEffort: effort}
		env := s.buildChatHandoffContext(ctx, reattachChatID, fakeSrc, req)
		if env.Prompt != "" && !isHandoffPrompt(providerPrompt) {
			providerPrompt = env.Prompt + "\n\n" + providerPrompt
			log.Printf("[reattach] envelope prepended chatId=%q legSeq=%d switchFrom=%q included=%d mode=%s prompt_len=%d", reattachChatID, reattachLegSeq, reattachSwitchFrom, env.Stats.IncludedTurnCount, env.Stats.Mode, len(providerPrompt))
		}
		s.mu.Lock()
		if rs.switchFromRunID == reattachSwitchFrom {
			rs.switchFromRunID = ""
		}
		s.mu.Unlock()
		// Use turnCount captured before unlock to avoid race
		_ = reattachTurnCount
		_ = reattachLegSeq
	}
	// Live ledger refresh (CP-35): pick up commits made during this session so the
	// oracle always sees the current change history, not just what existed at bind time.
	s.rebuildLedgerIfDirty(rs.workspaceCwd)
	// Flow Mode: prepend FlowContextPackage for Coding steps before normal feature
	// history injection. Skip history when the runner itself injected the package
	// (structural flag — not user-forgeable text markers) (Task-169 / V10R4 P1).
	providerPrompt = s.injectFlowContextIfCoding(ctx, rs, in.StepID, in.Prompt, providerPrompt)
	s.mu.Lock()
	skipHistory := rs.flowContextInjected || isHandoffPrompt(providerPrompt) ||
		isFlowEnginePrompt(providerPrompt) || isFlowReviewHandoffPrompt(providerPrompt)
	// Legacy text marker still honored only when paired with trusted HTML comment
	// (keeps unit tests / non-interactive paths working).
	// BUG-288 R13-15 / R19-4: bind marker id to this run and verify with this
	// service's secret only (not the global multi-secret bag).
	if !skipHistory && isFlowContextHandoffWithSecret(s.markerSecret, providerPrompt, allowedFCPMarkerIDs(rs)...) {
		skipHistory = true
	}
	allowedIDs := allowedFCPMarkerIDs(rs)
	markerSecret := s.markerSecret
	s.mu.Unlock()
	if s.shouldInjectFeatureHistory(rs.providerKey) && !skipHistory {
		// CP-51 Task-252: required MarkerVerificationContext — empty allowed set fails closed.
		providerPrompt = injectFeatureHistoryPromptCtx(rs.workspaceCwd, providerPrompt, transcriptTurnsFromRun(rs),
			MarkerVerificationContext{Secret: markerSecret, AllowedMarkerIDs: allowedIDs})
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
	// Chat-mode parity with Flow mode's flowDiagLog: persist the actual
	// provider/model/reasoning/cwd/yolo values passed for this turn, not just
	// the prompt — needed to diagnose provider-side failures (e.g. a bad cwd)
	// without guessing what was actually sent.
	logTurnProviderParams(toolWorkspace, rs.projectID, rs.id, turnID, string(rs.providerKey), model, effort, rs.workspaceCwd, yolo)
	// V9-21 / V10R3 P0: flow coding children need shell approval bridge for
	// commit denylist when YOLO would otherwise auto-approve. Gemini has no
	// RequestApproval path even when YOLO is off, so always force the bridge
	// (PATH shim + commit undo) for Gemini coding children.
	forceShellBridge := false
	if rs.parentRunID != "" && s.isFlowEngineDriven(rs.parentRunID) {
		if node, ok := flowNodeForRun(s, rs); ok {
			if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "agent.delegate" {
				if !isFlowReviewerChild(rs, node, true) {
					if yolo || rs.providerKey == ProviderKeyGemini {
						forceShellBridge = true
					}
				}
			}
		}
	}
	// Task-260: auto-mention safe-fix-contract on Chat Plan/Code (pointer only, not full content).
	mergedSkills := mergeChatSafeFixContractSkills(posture, rs.runKind, rs.flowEngineDriven, in.SelectedSkills)
	req := TurnRequest{
		RunID:                  rs.id,
		StepID:                 in.StepID,
		ProjectID:              rs.projectID,
		ProviderSessionID:      providerSessionID,
		ProviderTurnID:         turnID,
		Prompt:                 providerPrompt,
		ModelName:              model,
		SelectedSkills:         mergedSkills,
		YoloMode:               yolo,
		ForceShellBridge:       forceShellBridge,
		ReasoningEffort:        effort,
		ChatPosture:            posture,
		Cwd:                    rs.workspaceCwd,
		Scenario:               scenario,
		Attachments:            in.Attachments,
		OfferReviewOutcomeTool: offerReviewOutcomeTool,
	}
	// CP-35 / Task-242: snapshot HEAD + dirty worktree fingerprints before the AI
	// runs so the post-turn gate measures only this turn's changes (not prior
	// uncommitted coder dirt still on the tree).
	// BUG-288 #30: always reset turnStartGitHead (empty on capture fail) so a
	// stale prior-turn SHA is never reused.
	head, headErr := captureGitHead(rs.workspaceCwd)
	wtSnap := snapshotWorktreeFingerprints(rs.workspaceCwd)
	s.mu.Lock()
	if headErr == nil {
		rs.turnStartGitHead = head
	} else {
		rs.turnStartGitHead = ""
	}
	rs.turnStartWorktree = wtSnap
	s.mu.Unlock()
	// BUG-288 R13-14: warm-up baseline under the turn's cancellable ctx so Stop
	// can cut the first capture short (not always Background).
	s.ensureBaselineWithContext(ctx, rs.workspaceCwd)
	// BUG-308 residual run-33289: release hub stop fence before linearize so a
	// post-Stop plain-chat follow-up is not immediately terminal_cancelled.
	// Generation stays elevated (children remain fenced). Provider-agnostic.
	if rs.turnStartedAfterLoopDone {
		s.releaseHubStopFenceForFollowUp(ctx, rs.id)
	}
	// CP-51 Task-249: durable send_claimed → send_started linearization before
	// the adapter's first external byte. Stop winning this CAS means zero send.
	// BUG-289 A1/F-6: non-Stop store errors must emit a terminal turn event and
	// settle the RUNNING step — not only clear turnInFlight and return.
	// run-33289: stop-fence cancels (storeErr=false) also left the UI without a
	// terminal event after TurnStarted — emit TurnFailed so Thinking settles.
	if ok, storeErr := s.linearizeSendStarted(ctx, rs, turnID); !ok {
		s.mu.Lock()
		rs.turnInFlight = false
		rs.currentTurnID = ""
		rs.turnCancel = nil
		errMsg := "turn cancelled before send (run stop fence)"
		if storeErr {
			errMsg = "dispatch linearize failed (store/CAS error before send)"
			rs.status = RunStatusFailed
			rs.agentStatus = string(RunStatusFailed)
		}
		s.emitLocked(rs, ProviderEvent{
			Type:           EventTurnFailed,
			ProviderTurnID: turnID,
			Error:          errMsg,
			Status:         string(RunStatusFailed),
		})
		s.mu.Unlock()
		s.notifyTurnIdle(rs.id)
		return
	}
	if ctx.Err() != nil {
		// Defense-in-depth only; correctness is the CAS above.
		s.recordDispatchTransportError(ctx, rs.id, turnID, ctx.Err())
		s.mu.Lock()
		rs.turnInFlight = false
		rs.currentTurnID = ""
		rs.turnCancel = nil
		s.mu.Unlock()
		s.notifyTurnIdle(rs.id)
		return
	}
	bridge := &turnBridge{svc: s, rs: rs, ctx: ctx, turnID: turnID, yolo: yolo, posture: posture}
	err := s.sendTurnWithRetry(ctx, adapter, req, bridge)
	if err != nil && s.dispatchStore != nil && s.dispatchV2ActiveForRun(ctx, rs.id) {
		// Ambiguous: may have reached the provider. Never terminalize/clear.
		s.recordDispatchTransportError(ctx, rs.id, turnID, err)
		s.scheduleDispatchReconcile(rs.id, turnID)
	}
	s.mu.Lock()
	turnFailed := rs.status == RunStatusFailed
	s.mu.Unlock()
	if err == nil && !turnFailed && !rs.flowEngineDriven {
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
		//
		// BUG-259: some adapters (codex_adapter.go's SendTurn) return err==nil
		// even when the terminal event was EventTurnFailed (e.g. a provider
		// usage-limit error) — bridge.Emit already ran emitLocked synchronously
		// and settled rs.status to Failed before SendTurn returned, so trusting
		// err alone let a genuinely failed turn's still-PENDING workflow_steps
		// get bulk-marked DONE by this legacy planner. Check the run's actual
		// settled status too, not just the adapter's (unreliable) return value.
		_, _ = s.orchestrator.Progress(ctx, rs.id, yolo)
	} else if turnFailed && !rs.flowEngineDriven {
		// BUG-259 (follow-up): skipping Progress() above is correct but leaves
		// in.StepID stuck at whatever markStepRunning set it to (RUNNING) —
		// nothing else ever settles it. A step that never actually finished
		// must read as FAILED, not hang at "running" forever.
		_ = s.markStepFailed(ctx, rs.id, in.StepID)
	}

	completed, fin := s.finishTurn(rs, turnID, err)

	// BUG-226: the hub synthesis turn must finish by calling submit_review_outcome.
	// A prose-only answer leaves the flow engine without a terminal transition, so
	// conservatively escalate and settle the inline hub step instead of leaving it RUNNING.
	//
	// run-5296 / run-2047: do NOT fire this fallback when a coding/review round is
	// still in flight. After submit_review_outcome(continue), a separate hub turn
	// (gate reprompt / BugFix doc) can complete without the review tool; escalating
	// then parkFlowForAwaitingUser cancels the brand-new reviewer cohort and shows
	// a form with the STALE round-0 lastCohortNote ("Reviewers reported: …").
	//
	// run-199617 (hub_stalled 2m): a flow-engine hub stays status=running while the
	// loop is live, so `completed` (status Completed || pendingFlowGateSettle) is
	// false even after the provider turn itself finished via EventTurnCompleted —
	// the hub prose-answered without submit_review_outcome and the watchdog parked
	// hub_stalled after 2m instead of this fallback surfacing an actionable card.
	// Treat a terminal provider event as "the turn finished" for this fallback;
	// the tool-submitted / open-cohort / sealed-loop guards below still apply.
	hubTurnFinished := completed || rs.lastEventType == EventTurnCompleted
	if hubTurnFinished && offerReviewOutcomeTool && rs.parentRunID == "" && rs.flowEngineDriven && !s.flowControlSubmittedForTurn(rs.id, turnID) {
		if s.hubShouldSkipProseEscalate(rs.id) {
			log.Printf("[flow-step] hub turn %q completed without submit_review_outcome, but children/cohort still active — skip BUG-226 escalate", turnID)
		} else if s.advanceHubFromCohortMachineVerdicts(rs.id) {
			// run-200816: the reviewers already recorded machine verdicts via
			// submit_review_outcome, but the hub finished in prose. Drive the
			// transition from those verdicts (all approved → done/freeze, any
			// changes_requested → continue) instead of forcing the operator to
			// Retry an already-decided plan. Empty/blocked verdicts fall
			// through to the BUG-226 escalate below (CA-735 preserved).
			log.Printf("[flow-step] hub turn %q completed without submit_review_outcome; cohort machine verdicts derived the flow transition", turnID)
		} else {
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
		}
	}

	// Persist settled state (status, lastMessage, updatedAt) for history survival
	// across restarts (BUG-080 F-3). Take a snapshot under lock; persist outside.
	s.mu.Lock()
	newTurnSessionID := s.refreshResumeHandleLocked(rs, adapter)
	// Persist prompt/assistant pairs for providers whose on-disk transcript is
	// incomplete or rotated per-turn. Gemini always needed this; Grok flow hubs
	// also do — synthesis turns use system prompts (filtered on resume) and a
	// single grok_session id may only reload the latest segment, so without a
	// turn-log assistant the main chat loses res-after-review-round-N (run-9034).
	durableTurn := transcriptTurn{}
	// CA-688: opencode has NO provider-owned transcript file at all (sessions
	// live in the shared opencode.db), so its prompt/assistant pairs must ride
	// the durable turn log for /open replay — same rationale as Gemini/Grok.
	if completed && (rs.providerKey == ProviderKeyGemini || rs.providerKey == ProviderKeyGrok || rs.providerKey == ProviderKeyOpencode) {
		durableTurn = transcriptTurnForProviderTurnLocked(rs, turnID)
	}
	snap := sessionStateOf(rs)
	isParent := rs.parentRunID == ""
	s.mu.Unlock()
	// BUG-257: sessionStateOf never carries LoopState (it isn't a field on
	// interactiveRun; the live value lives in agentOrchestrator.loop). The
	// startTurn persist already patches it in for parent/hub runs — this
	// post-turn persist must do the same, or it silently overwrites an
	// already-terminal "done" LoopState (written moments earlier by
	// markFlowRunComplete when the hub's own synthesis turn called
	// submit_review_outcome) with a zero-value LoopState once that same turn
	// finishes. Since sessions.ndjson resume reads only the latest record,
	// that clobbered record made a fully-completed flow run's hub/synthesis
	// step resume as CANCELED instead of DONE after a server restart.
	if isParent {
		snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState
	}
	_ = s.persistProviderSession(snap)
	// Log each newly observed provider session id so seedTranscriptFromDisk can
	// replay every transcript segment on resume. Codex normally rotates rollout
	// files per turn; Grok may either reuse its ACP session or rotate it.
	if newTurnSessionID != "" {
		if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
			kind := turnLogKindCodexSession
			if rs.providerKey == ProviderKeyGrok {
				kind = turnLogKindGrokSession
			} else if rs.providerKey == ProviderKeyOpencode {
				kind = turnLogKindOpencodeSession
			}
			_ = logger.AppendTurnLog(context.Background(), rs.id, turnLogLine{Kind: kind, SessionID: newTurnSessionID})
		}
	}
	if strings.TrimSpace(durableTurn.User) != "" || strings.TrimSpace(durableTurn.Assistant) != "" {
		if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
			_ = logger.AppendTurnLog(context.Background(), rs.id, turnLogLine{
				Kind:      turnLogKindTranscriptTurn,
				TurnID:    turnID,
				Prompt:    durableTurn.User,
				Assistant: durableTurn.Assistant,
			})
		}
	}
	if err == nil {
		_ = s.syncCodexStableSessionToKnownAccounts(rs)
	}
	s.mu.Lock()
	// V9-03: keep turnInFlight true while post-turn gate will run so startTurn
	// cannot race a concurrent turn over a settling child/root.
	willRunGate := completed
	if !willRunGate {
		rs.turnInFlight = false
	}
	pendingRestartRunID := ""
	pendingRestartPrompt := ""
	if rs.parentRunID != "" {
		if parent := s.runs[rs.parentRunID]; parent != nil {
			pendingRestartRunID = parent.pendingRestartRunID
			pendingRestartPrompt = parent.pendingRestartPrompt
			// V9-06: keep pendingRestart when this child is the target so cancel
			// path (completed=false) still restarts after finishTurn.
			if pendingRestartRunID != rs.id {
				parent.pendingRestartRunID = ""
				parent.pendingRestartPrompt = ""
				parent.pendingRestartGen = 0
			}
		}
	}
	// Retry a hub reinvoke that was deferred because turnInFlight was true when the
	// coder completion fired. This prevents the review loop from stalling when the
	// coder finishes before the hub's current turn has cleared.
	//
	// BUG-284 / CP-51 A1: when a post-turn gate will keep turnInFlight (and may
	// arm pendingFlowGateSettle), do NOT drain pendingHubReinvoke here. An early
	// scheduleChildTurn races gate and often gets turn_in_progress /
	// gate_in_progress; the fail path re-arms after runTurn's final
	// notifyTurnIdle already ran, stranding hub.notify forever. Leave the
	// pending flag for notifyTurnIdle after the gate settles (H5/F-5).
	pendingHubReinvoke := rs.parentRunID == "" && rs.pendingHubReinvoke && !willRunGate
	// BUG-284: a hub.notify dispatch stashes its FULL prompt in
	// pendingHubReinvokePrompt (see maybeAutoReinvokeHubWithPrompt) because a
	// hub.notify "done" is observed from INSIDE the very turn that is
	// finishing, so this defer/retry path is the common case for it, not rare.
	// Retrying with the generic maybeAutoReinvokeHub (as below) would fire the
	// wrong prompt and silently drop the notify dispatch entirely.
	pendingHubReinvokePrompt := ""
	if pendingHubReinvoke {
		rs.pendingHubReinvoke = false
		pendingHubReinvokePrompt = rs.pendingHubReinvokePrompt
		rs.pendingHubReinvokePrompt = ""
	}
	s.mu.Unlock()

	if pendingHubReinvoke {
		if pendingHubReinvokePrompt != "" {
			go s.maybeAutoReinvokeHubWithPrompt(rs.id, pendingHubReinvokePrompt)
		} else {
			go s.maybeAutoReinvokeHub(rs.id)
		}
	}

	// Post-turn flow gate (CP-35 P-4/P-5): observe diff, evaluate rules, enforce.
	// Non-fatal: any internal error inside runFlowGate degrades to pass.
	// Child agent runs (coder, reviewer) are exempt from full CA/task/test
	// gates: that is the hub/root responsibility (BUG-152). Task-223 still
	// evaluates the file_artifact OUTPUT write contract on the child that
	// owns the OUTPUT binding — otherwise coder never gets r-artifact-output.
	// Task-242 D-1/D-3/D-9: flow-engine children defer cohort join / step DONE
	// until this gate passes (see pendingFlowGateSettle in emitLocked).
	// BUG-288 #10: gate uses a cancelable ctx that Stop can cancel via
	// postTurnGateCancel (turnCancel is already nil after finishTurn).
	if completed {
		// BUG-288 R17-P1 / R18-2: if settle checkpoint was never durable, do not
		// run gate / fan-out — retry persist; if still failing, leave
		// non-terminal WITH settle intent intact (do not set completed=false
		// in a way that triggers wipe — stay in this branch with skip gate).
		s.mu.Lock()
		checkpointBlocked := rs.pendingFlowGateSettle && rs.gateCheckpointNotDurable
		if checkpointBlocked {
			snapRetry := sessionStateOf(rs)
			if rs.parentRunID == "" {
				snapRetry.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
			}
			s.mu.Unlock()
			if err := s.persistProviderSession(snapRetry); err != nil {
				log.Printf("[flow-gate] settle checkpoint still not durable run=%q: %v (keeping settle intent; skipping gate)", rs.id, err)
				s.mu.Lock()
				// Keep pendingFlowGateSettle + gateCheckpointNotDurable for retry.
				rs.turnInFlight = false
				s.mu.Unlock()
				// completed=false so we do not finalize/fan-out; else-branch must
				// NOT wipe settle while gateCheckpointNotDurable (R18-2).
				completed = false
			} else {
				s.mu.Lock()
				rs.gateCheckpointNotDurable = false
				s.mu.Unlock()
				checkpointBlocked = false
			}
		} else {
			s.mu.Unlock()
		}
		// If the loop is already blocked for a human form, do not run post-turn
		// gate / reprompt — park already dropped settle intents (run-1675).
		if completed {
			loopID := rs.id
			if rs.parentRunID != "" {
				loopID = rs.parentRunID
			}
			if st := s.agentOrchestrator.loopStateFor(loopID).Status; st == "blocked" {
				s.mu.Lock()
				rs.pendingFlowGateSettle = false
				rs.pendingFlowGateFinalMsg = ""
				rs.pendingFlowGateOccurredAt = ""
				rs.pendingFlowGateTurnID = ""
				rs.pendingGateChangedFiles = nil
				rs.turnInFlight = false
				s.mu.Unlock()
				completed = false
			}
		}
		if completed && !checkpointBlocked {
			gateCtx, gateCancel := context.WithCancel(context.Background())
			s.mu.Lock()
			gateEpoch := rs.gateEpoch
			rs.postTurnGateCancel = gateCancel
			s.mu.Unlock()
			defer func() {
				s.mu.Lock()
				if rs.postTurnGateCancel != nil {
					rs.postTurnGateCancel = nil
				}
				s.mu.Unlock()
				gateCancel()
			}()

			gateBlocked := false
			// V9-18: Stop during gate — cancel ctx; do not finalize as success.
			stoppedMidGate := false
			s.mu.Lock()
			if rs.status == RunStatusCancelled || rs.status == RunStatusFailed || rs.gateEpoch != gateEpoch {
				stoppedMidGate = true
			}
			s.mu.Unlock()
			if gateCtx.Err() != nil {
				stoppedMidGate = true
			}
			if !stoppedMidGate {
				if rs.parentRunID == "" {
					// BUG-305/BUG-308: a follow-up admitted onto an already-sealed loop
					// ("done" or a Stop-ped loop) is plain chat, not flow work. Skip the
					// flow gate EVALUATION but keep the pass-path bookkeeping below
					// (turnInFlight clear, settle, finalizer). The gate would force-block
					// such a loop anyway — runFlowGateAtEpoch's gateEpochStillValid check
					// returns false for both "done" and "stopped" and returns block —
					// which would set completed=false and suppress the live TurnCompleted
					// that emitLocked already published as plain chat. There is no active
					// flow decision left to protect, so treat it as a pass.
					if loopAlreadySealedAtTurnStart {
						gateBlocked = false
					} else {
						gateBlocked = s.runFlowGateAtEpoch(gateCtx, rs, turnID, fin, gateEpoch)
					}
				} else {
					gateBlocked = s.runChildArtifactOutputGateAtEpoch(gateCtx, rs, turnID, fin, gateEpoch)
				}
			}
			if gateCtx.Err() != nil {
				stoppedMidGate = true
			}
			// Revalidate epoch after gate returns (Stop may have raced).
			s.mu.Lock()
			if rs.gateEpoch != gateEpoch {
				stoppedMidGate = true
			}
			s.mu.Unlock()
			if stoppedMidGate || gateBlocked {
				completed = false
				s.mu.Lock()
				if rs.gateEpoch != gateEpoch {
					// Stop invalidated this gate — drop settle/reprompt side effects.
					rs.postTurnGateCancel = nil
					rs.turnInFlight = false
					s.mu.Unlock()
				} else if rs.pendingFlowGateSettle {
					// Reprompt/block/stop: keep non-terminal Running (never published Completed).
					rs.pendingFlowGateSettle = false
					rs.pendingFlowGateFinalMsg = ""
					rs.pendingFlowGateOccurredAt = ""
					rs.pendingFlowGateTurnID = ""
					rs.pendingGateChangedFiles = nil
					if !stoppedMidGate {
						s.settleChildStatusAfterGateBlockLocked(rs)
					}
					rs.turnInFlight = false
					rs.postTurnGateCancel = nil
					// V10R3/R4: keep durable reprompt on session; flush after settle clear.
					repromptPrompt := rs.pendingGateRepromptPrompt
					repromptStep := rs.pendingGateRepromptStepID
					repromptGen := rs.pendingGateRepromptGen
					repromptRun := rs.id
					// BUG-288 P1-16: sync-persist the block/reprompt checkpoint
					// unconditionally (not only when a reprompt prompt exists) BEFORE
					// allowing a remediation turn to start or considering the block
					// decision committed. A persist failure must not let a new turn
					// start nor let this be treated as durable — otherwise a crash
					// right after loses the gate decision, or a restart replays the
					// gate against stale (pre-block) state.
					snapRep := sessionStateOf(rs)
					if rs.parentRunID == "" {
						snapRep.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
					}
					s.mu.Unlock()
					persistRepErr := s.persistProviderSession(snapRep)
					skipTailNotify := false
					if persistRepErr != nil {
						log.Printf("[flow-gate] persist block/reprompt checkpoint run=%q turn=%q: %v (gate left actionable for retry, remediation turn NOT started)", repromptRun, turnID, persistRepErr)
						// BUG-288 R13-05: do not fall through to notifyTurnIdle, which
						// would claim RAM intents and start the remediation turn that
						// we just refused because the checkpoint is not durable.
						skipTailNotify = true
						// Back off durable flush for this generation so a later crash
						// recovery path does not immediately re-fire a non-durable intent.
						s.mu.Lock()
						if r := s.runs[repromptRun]; r != nil && repromptGen != 0 {
							r.pendingGateRepromptFailCount++
							r.pendingGateRepromptFailGen = repromptGen
						}
						s.mu.Unlock()
					}
					if persistRepErr == nil {
						// Task-251 T-2b: durable settle disposition for gate block/reprompt.
						// resumePendingFlowGate already does this on the boot/resume path
						// (line ~3697 above); this live post-turn-gate block never had the
						// matching call, so every LIVE gate-block/reprompt turn left its
						// dispatch record's SettlePhase stuck at settle_pending forever —
						// maybeScheduleSettleAfterTerminal had already bailed out earlier
						// (pendingFlowGateSettle was armed) expecting this exact gate
						// resolution to drive the deferred settle, which it never did.
						s.scheduleSettleAfterGateBlock(repromptRun, turnID)
						if repromptPrompt != "" && repromptStep != "" && !stoppedMidGate {
							// CP-51 A1: root hub gate remediations must not race
							// active flow children after continue-delegate.
							s.mu.Lock()
							isRootReprompt := false
							if r := s.runs[repromptRun]; r != nil {
								isRootReprompt = r.parentRunID == ""
							}
							s.mu.Unlock()
							if isRootReprompt {
								s.scheduleRootGateRepromptOrPark(repromptRun, repromptStep, repromptPrompt, turnID, repromptGen)
							} else {
								go s.startTurnClearingIntent(repromptRun, repromptStep, repromptPrompt, "reprompt", repromptGen)
							}
						} else if !stoppedMidGate {
							s.notifyTurnIdle(repromptRun)
							// notify already ran for this branch — avoid double at tail.
							skipTailNotify = true
						}
					}
					// Stash skip flag on a stack variable consumed after unlock path.
					// (gateBlockedSkipNotify is checked at the runTurn tail.)
					if skipTailNotify {
						s.mu.Lock()
						if r := s.runs[repromptRun]; r != nil {
							r.skipNextTurnIdleNotify = true
						}
						s.mu.Unlock()
					}
				} else {
					rs.turnInFlight = false
					rs.postTurnGateCancel = nil
					s.mu.Unlock()
				}
			} else {
				s.mu.Lock()
				// V10R4 P0: revalidate epoch before any Completed/TurnCompleted side effect.
				if rs.gateEpoch != gateEpoch {
					completed = false
					rs.turnInFlight = false
					rs.postTurnGateCancel = nil
					s.mu.Unlock()
				} else if rs.pendingFlowGateSettle {
					// BUG-288 R13-06: Skip already terminal-failed this member — never
					// publish Completed over Failed from a late gate-pass.
					if rs.cohortSkipConsumed || rs.status == RunStatusFailed {
						rs.pendingFlowGateSettle = false
						rs.pendingFlowGateFinalMsg = ""
						rs.pendingFlowGateOccurredAt = ""
						rs.pendingFlowGateTurnID = ""
						rs.pendingGateChangedFiles = nil
						rs.turnInFlight = false
						rs.postTurnGateCancel = nil
						s.mu.Unlock()
					} else {
						msg := rs.pendingFlowGateFinalMsg
						at := rs.pendingFlowGateOccurredAt
						parentID := rs.parentRunID
						childID := rs.id
						prevStatus := rs.status
						prevAgentStatus := rs.agentStatus
						prevPendingSettle := rs.pendingFlowGateSettle
						prevFinalMsg := rs.pendingFlowGateFinalMsg
						prevOccurredAt := rs.pendingFlowGateOccurredAt
						prevTurnID := rs.pendingFlowGateTurnID
						prevChangedFiles := rs.pendingGateChangedFiles
						rs.pendingFlowGateSettle = false
						rs.pendingFlowGateFinalMsg = ""
						rs.pendingFlowGateOccurredAt = ""
						rs.pendingFlowGateTurnID = ""
						rs.pendingGateChangedFiles = nil
						// Publish terminal Completed only after gate pass (BUG-288 #1 / V10).
						rs.status = RunStatusCompleted
						rs.agentStatus = string(RunStatusCompleted)
						// V9-03/BUG-288 R11 #3: compute + persist the post-gate snapshot BEFORE
						// signalChild/broadcast/settle/release (matches resumePendingFlowGate) —
						// a persist failure must not let the completion fan out with no durable
						// record; a restart would then re-run the gate against stale state.
						snapPass := sessionStateOf(rs)
						if parentID == "" {
							snapPass.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
						}
						s.mu.Unlock()
						persistErr := s.persistProviderSession(snapPass)
						s.mu.Lock()
						if rs2 := s.runs[rs.id]; rs2 != nil {
							rs = rs2
						}
						if persistErr != nil {
							rs.status = prevStatus
							rs.agentStatus = prevAgentStatus
							rs.pendingFlowGateSettle = prevPendingSettle
							rs.pendingFlowGateFinalMsg = prevFinalMsg
							rs.pendingFlowGateOccurredAt = prevOccurredAt
							rs.pendingFlowGateTurnID = prevTurnID
							rs.pendingGateChangedFiles = prevChangedFiles
							rs.turnInFlight = false
							completed = false
							s.mu.Unlock()
							log.Printf("[flow-gate] persist post-gate completion run=%q turn=%q: %v (gate left actionable for retry)", childID, turnID, persistErr)
						} else if rs.gateEpoch != gateEpoch {
							// BUG-288 R11 #4 (live path): Stop landed during the unlocked
							// persist call above (persistProviderSession releases s.mu).
							// The durable Completed write already succeeded, but a Stop
							// mid-persist must still suppress every completion fan-out —
							// summary update, signalChild, event broadcast, settle, and
							// release — matching resumePendingFlowGate's post-persist
							// gateEpoch revalidation.
							rs.turnInFlight = false
							completed = false
							s.mu.Unlock()
						} else {
							if parentID != "" {
								if summary, ok := s.agentOrchestrator.currentSummary(parentID, childID); ok {
									summary.Status = RunStatusCompleted
									summary.AgentStatus = string(RunStatusCompleted)
									s.agentOrchestrator.upsertSummary(parentID, summary)
								}
							}
							s.agentOrchestrator.signalChild(childID, msg, false, "", RunStatusCompleted)
							// Persist+broadcast deferred TurnCompleted without re-entering
							// pendingFlowGateSettle (V10 P1).
							completedEv := ProviderEvent{
								Type:              EventTurnCompleted,
								FinalMessage:      msg,
								OccurredAt:        at,
								ProviderTurnID:    turnID,
								WorkflowRunID:     rs.id,
								ProviderSessionID: rs.providerSessionID,
								ProviderKey:       rs.providerKey,
							}
							rs.seq++
							completedEv.Seq = rs.seq
							completedEv.ID = s.nextID("evt")
							// Replace deferred in-memory terminal (last event) if still the gate placeholder.
							if n := len(rs.events); n > 0 && rs.events[n-1].Type == EventTurnCompleted {
								rs.events[n-1] = completedEv
							} else {
								rs.events = append(rs.events, completedEv)
							}
							rs.lastEventType = EventTurnCompleted
							_ = s.persistEvent(completedEv)
							for _, ch := range rs.subs {
								select {
								case ch <- completedEv:
								default:
								}
							}
							s.settleFlowChildTurnCompletedLocked(rs, msg, completedEv)
							rs.turnInFlight = false
							rs.postTurnGateCancel = nil
							s.mu.Unlock()
							// Match the restart path: once the live post-turn gate has
							// passed and completion is durable, settle the dispatch ledger.
							s.scheduleSettleAfterGatePass(childID, turnID)
							if parentID != "" {
								go s.releaseDependentAgents(parentID, childID, msg, at)
							}
						}
					} // end R13-06 else (non-skip gate-pass settle)
				} else {
					rs.turnInFlight = false
					// BUG-284 CP-51 A1: without nilling postTurnGateCancel here, it
					// stays non-nil until the deferred cleanup (above) runs at
					// function return — but the tail's notifyTurnIdle call fires
					// BEFORE that defer, sees postTurnGateCancel != nil as "busy",
					// and skips draining pendingHubReinvoke/pendingHubReinvokePrompt.
					// A deferred hub.notify reinvoke armed mid-turn (turnInFlight was
					// true when SubmitFlowControl fired) then never gets a second
					// drain opportunity and is stranded forever.
					rs.postTurnGateCancel = nil
					s.mu.Unlock()
					// A root synthesis turn has no deferred child settle, but its
					// dispatch record still becomes terminal only after this gate
					// passes. Drive the same durable settlement as child turns.
					s.scheduleSettleAfterGatePass(rs.id, turnID)
				}
			}
		} // end if completed && !checkpointBlocked
	} else {
		// Turn did not complete cleanly. BUG-288 R18-2: if settle is waiting
		// on durable checkpoint (gateCheckpointNotDurable), KEEP the recovery
		// intent so storage recovery can retry gate later — do not wipe it.
		s.mu.Lock()
		if rs.pendingFlowGateSettle && !rs.gateCheckpointNotDurable {
			rs.pendingFlowGateSettle = false
			rs.pendingFlowGateFinalMsg = ""
			rs.pendingFlowGateOccurredAt = ""
			rs.pendingFlowGateTurnID = ""
			rs.pendingGateChangedFiles = nil
			rs.gateCheckpointNotDurable = false
		}
		rs.turnInFlight = false
		s.mu.Unlock()
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
	// V10R4 P1: re-flush durable intents after turn/gate becomes idle so a
	// conflicted delivery is not stranded until process restart.
	// BUG-288 R13-05: skip when block/reprompt checkpoint persist failed.
	s.mu.Lock()
	skipIdle := rs.skipNextTurnIdleNotify
	rs.skipNextTurnIdleNotify = false
	s.mu.Unlock()
	if !skipIdle {
		s.notifyTurnIdle(rs.id)
	}
	// V9-06 / BUG-288 R15-P0: deliver durable stall-retry via claim + idempotency
	// (do not clear PendingRestart* before startTurn accepts).
	if pendingRestartRunID == rs.id && pendingRestartPrompt != "" && rs.parentRunID != "" {
		go s.deliverPendingRestart(rs.parentRunID)
	}
}

func (s *InteractiveService) shouldInjectFeatureHistory(providerKey ProviderKey) bool {
	switch providerKey {
	case ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGemini, ProviderKeyGrok:
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

// markStepFailed settles stepID (started by markStepRunning) to FAILED. Used by
// runTurn (BUG-259 follow-up) when the run's own turn fails on a non-flow-engine-
// driven workflow run: skipping the legacy bulk Progress() call is correct, but
// something still has to settle the step that was actually in flight — otherwise
// it hangs at RUNNING forever, which reads just as misleadingly as the bulk-DONE
// bug it replaces.
func (s *InteractiveService) markStepFailed(ctx context.Context, runID, stepID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.workflowStore.ApplyStepTransition(ctx, runID, WorkflowStepTransition{
		StepID: stepID,
		Patch: WorkflowStepPatch{
			Status:     StepStatusFailed,
			FinishedAt: strptr(now),
		},
		Logs: []WorkflowLog{{LogError, "Provider turn failed."}},
	})
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
		strings.Contains(message, "rate limit") ||
		// Grok Build (CP-46/Task-210, GR-19): live-observed 402 signature during
		// CP-46 authoring. Appended additively; other providers' classification
		// above is unchanged.
		strings.Contains(message, "personal-team-blocked") ||
		strings.Contains(message, "spending-limit") ||
		strings.Contains(message, "spending_limit")
}

// finishTurn does the locked post-turn bookkeeping: clears in-flight state, emits
// the terminal event when the adapter did not, and maps the error to a status. It
// returns whether the turn completed cleanly (so the caller runs the finalizer) and
// the finalize input gathered from the run's events.
func (s *InteractiveService) finishTurn(rs *interactiveRun, turnID string, err error) (bool, finalizeInput) {
	s.mu.Lock()
	rs.turnCancel = nil
	rs.currentTurnID = ""
	// CP-58 run-203966: park-cancel flags are one-shot for the context.Canceled
	// branch below. Any other error (adapter returned nil, approval expiry, a
	// real failure) must not keep them armed — a stale parkCancelCause would
	// misclassify the NEXT real user interrupt as a park cancel, and a stale
	// parkCancelSuppress would swallow a real failure (before the switch, so
	// this branch's own emitLocked is not suppressed either).
	if !errors.Is(err, context.Canceled) {
		rs.parkCancelCause = false
		rs.parkCancelSuppress = false
	}
	// lastTurnID used by resumePendingFlowGate after restart.
	if turnID != "" {
		rs.lastTurnID = turnID
	}

	// CP-51: finishTurn emits terminal events via emitLocked, which does NOT
	// go through turnBridge.Emit — so CommitTerminalAndSettleIntent never ran
	// for adapter-return failures (run-1618: child stuck at send_started +
	// transport_error effect, UI Failed, dispatch non-terminal). Capture
	// outcome outside the lock and terminalize after unlock.
	var dispatchOutcome string
	var dispatchErrMsg string

	switch {
	case err == nil:
		if rs.lastEventType != EventTurnCompleted && rs.lastEventType != EventTurnFailed {
			s.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: ""})
		}
		// pendingFlowGateSettle: provider turn finished but Completed not published
		// until gate pass (BUG-288 #1). Still treat as completed for post-turn gate.
		fin := s.finalizeInputLocked(rs, turnID)
		if rs.pendingFlowGateSettle {
			// Refresh durable WrittenPaths + turn id + re-persist so resume has full snapshot.
			// Sync (not go) — crash before async write must not drop PendingFlowGateSettle.
			rs.pendingGateChangedFiles = append([]string(nil), fin.ChangedFiles...)
			if turnID != "" {
				rs.pendingFlowGateTurnID = turnID
				rs.lastTurnID = turnID
			}
			snap := sessionStateOf(rs)
			if rs.parentRunID == "" {
				snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
			}
			_ = s.persistProviderSession(snap)
		}
		// If adapter already Terminal'd via bridge.Emit, Terminal is a no-op
		// (state already terminal). If not, complete the durable record.
		if !rs.pendingFlowGateSettle {
			dispatchOutcome = "completed"
		}
		completed := rs.status == RunStatusCompleted || rs.pendingFlowGateSettle
		s.mu.Unlock()
		if dispatchOutcome != "" {
			s.commitFinishTurnDispatchTerminal(rs, turnID, dispatchOutcome, "")
		}
		return completed, fin
	case errors.Is(err, context.Canceled):
		// BUG-288 P1-14/P1-19: a Stall Skip cancels the in-flight turn AFTER
		// already setting this run Failed (Task-241 contract: Skip -> FAILED,
		// never Cancelled). Preserve that terminal cause instead of falling
		// through to the generic cancel-as-Cancelled mapping below, which
		// would otherwise overwrite Failed with Cancelled the moment this
		// cancellation reaches finishTurn.
		if rs.stalledSkipCause {
			rs.stalledSkipCause = false
			// CP-58 run-203966: the Skip contract stamps the run terminal
			// Failed here — a stale park-cancel flag must not survive it.
			rs.parkCancelCause = false
			rs.parkCancelSuppress = false
			s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: "skipped by user (stalled)", Recoverable: false})
			dispatchOutcome = "failed"
			dispatchErrMsg = "skipped by user (stalled)"
			break
		}
		// BUG-288 R13-02 / R14-02: stall-retry cancel — one-shot
		// stalledRetrySuppressCohort so emitLocked(EventTurnFailed) skips
		// cohort-append + node-FAILED; keep Running for the pending restart.
		if rs.stalledRetryCause {
			rs.stalledRetryCause = false
			// CP-58 run-203966: the retry cancel must not leave a stale
			// park-cancel flag armed for the NEXT interrupt.
			rs.parkCancelCause = false
			rs.parkCancelSuppress = false
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			rs.stalledRetrySuppressCohort = true
			s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: "interrupted for stall retry", Recoverable: true})
			rs.stalledRetrySuppressCohort = false
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			// Not a durable terminal — retry will continue the same logical turn
			// intent on a new startTurn. Leave dispatch for recovery if needed.
			break
		}
		// CP-58 run-203966: the flow park's own parent-turn cancel lands here.
		// It is the engine parking awaiting a user decision, NOT a user
		// interrupt — the generic "interrupted by user" Cancelled stamp below
		// would make the parent terminal, so flowRunTerminalLocked silently
		// skipped every later advance ("run terminal/stopped") and the
		// watchdog parked hub_stalled. Keep the run non-terminal; the parked
		// loop owns liveness. Stop still wins: when stopAgentLoop already
		// sealed the loop "stopped" and stamped the run Cancelled before this
		// cancel propagated, fall through to the generic interrupt branch
		// instead of reviving the run (BUG-248 stop contract intact).
		if rs.parkCancelCause &&
			s.agentOrchestrator.loopStateFor(rs.id).Status != "stopped" &&
			rs.status != RunStatusCancelled {
			rs.parkCancelCause = false
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			rs.parkCancelSuppress = true
			s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: "interrupted by flow park (awaiting user decision)", Recoverable: true})
			rs.parkCancelSuppress = false
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
			// Patch the summary cache emitLocked just stamped (same shape as the
			// stalledRetry branch above / the Cancelled branch below).
			if parentID := strings.TrimSpace(rs.parentRunID); parentID != "" {
				if summary, ok := s.agentOrchestrator.currentSummary(parentID, rs.id); ok {
					summary.Status = RunStatusRunning
					summary.AgentStatus = string(RunStatusRunning)
					s.agentOrchestrator.upsertSummary(parentID, summary)
				}
			}
			// The TURN is durably over (parked); settle its dispatch record like
			// the user-cancel path, without the run-level terminal stamp.
			dispatchOutcome = "cancelled"
			dispatchErrMsg = "interrupted by flow park"
			break
		}
		// Real user interrupt (or a Stop-guard fallthrough): the park-cancel
		// flags must not leak into the generic "interrupted by user" branch —
		// a Cancelled stamp here is genuine, not park poison.
		rs.parkCancelCause = false
		rs.parkCancelSuppress = false
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: "interrupted by user", Recoverable: true})
		// Task-240 T-5 / BUG-248: emitLocked maps TurnFailed → Failed in the
		// summary cache before we override the run status. Patch summary + parent
		// graph so consumers see cancelled, not failed (stopAgentLoop already
		// does this for the stop-from-hub path; finishTurn covers Interrupt).
		rs.status = RunStatusCancelled
		rs.agentStatus = string(RunStatusCancelled)
		if parentID := strings.TrimSpace(rs.parentRunID); parentID != "" {
			if summary, ok := s.agentOrchestrator.currentSummary(parentID, rs.id); ok {
				summary.Status = RunStatusCancelled
				summary.AgentStatus = string(RunStatusCancelled)
				s.agentOrchestrator.upsertSummary(parentID, summary)
			}
			s.emitAgentGraphLocked(parentID, s.agentOrchestrator.graphSnapshot(parentID))
		}
		dispatchOutcome = "cancelled"
		dispatchErrMsg = "interrupted by user"
	case errors.Is(err, errApprovalExpired) || errors.Is(err, errQuestionExpired):
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: err.Error(), Recoverable: true})
		dispatchOutcome = "failed"
		dispatchErrMsg = err.Error()
	default:
		// This EventTurnFailed only ever reached the UI's event stream, never
		// runner.log — diagnosing a failed turn required reproducing it live.
		// Logging the terminal error here (alongside logTurnProviderParams'
		// model/reasoning/cwd/yolo) makes the failure reason findable after the fact.
		log.Printf("[turn-failed] run=%s turn=%s provider=%s error=%q", rs.id, turnID, rs.providerKey, err.Error())
		s.emitLocked(rs, ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: err.Error(), Recoverable: false})
		dispatchOutcome = "failed"
		dispatchErrMsg = err.Error()
	}
	s.mu.Unlock()
	if dispatchOutcome != "" {
		s.commitFinishTurnDispatchTerminal(rs, turnID, dispatchOutcome, dispatchErrMsg)
	}
	return false, finalizeInput{}
}

// commitFinishTurnDispatchTerminal records CP-51 terminal evidence for turns
// whose terminal event was emitted by finishTurn (emitLocked) rather than
// turnBridge.Emit. Idempotent when the record is already terminal.
func (s *InteractiveService) commitFinishTurnDispatchTerminal(rs *interactiveRun, turnID, outcome, errMsg string) {
	if s == nil || rs == nil || turnID == "" || s.dispatchStore == nil {
		return
	}
	if !s.dispatchV2ActiveForRun(context.Background(), rs.id) {
		return
	}
	bridge := &turnBridge{svc: s, rs: rs, ctx: context.Background(), turnID: turnID}
	payload, _ := json.Marshal(map[string]string{
		"type": string(EventTurnFailed), "error": errMsg, "final": "", "source": "finishTurn",
	})
	if outcome == "completed" {
		payload, _ = json.Marshal(map[string]string{
			"type": string(EventTurnCompleted), "error": "", "final": "", "source": "finishTurn",
		})
	}
	evKind := string(EventTurnFailed)
	if outcome == "completed" {
		evKind = string(EventTurnCompleted)
	}
	bridge.Terminal(TerminalEvidence{
		ProviderKey:          string(rs.providerKey),
		EvidenceKind:         evKind,
		Outcome:              outcome,
		PayloadCanonicalJSON: payload,
		PayloadSHA256:        HashBytes(payload),
		ObservedAt:           nowRFC3339Nano(),
	})
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

// clearDurableRecoveryStateLocked drops gate-pending and resume/reprompt intents
// so Stop/cancel cannot resurrect work after restart. Caller holds s.mu.
func clearDurableRecoveryStateLocked(rs *interactiveRun) {
	if rs == nil {
		return
	}
	rs.pendingFlowGateSettle = false
	rs.pendingFlowGateFinalMsg = ""
	rs.pendingFlowGateOccurredAt = ""
	rs.pendingFlowGateTurnID = ""
	rs.pendingGateChangedFiles = nil
	rs.pendingGateCodePaths = nil
	rs.pendingGateRepromptPrompt = ""
	rs.pendingGateRepromptStepID = ""
	rs.pendingGateRepromptGen = 0
	rs.pendingGateRepromptDeliveredGen = 0
	rs.pendingGateRepromptFailCount = 0
	rs.pendingGateRepromptFailGen = 0
	rs.gateFixCodeActive = false
	rs.gateFixCodeAttempts = 0
	rs.pendingResumePrompt = ""
	rs.pendingResumeStepID = ""
	rs.pendingResumeGen = 0
	rs.pendingResumeDeliveredGen = 0
	rs.pendingResumeAcceptedTurn = ""
	rs.pendingGateRepromptAcceptedTurn = ""
	rs.pendingResumeFailCount = 0
	rs.pendingResumeFailGen = 0
	rs.pendingResumeApprovalID = ""
	rs.pendingResumeDecision = ""
	rs.pendingResumeQuestionChoices = nil
	rs.intentClaimKind = ""
	rs.intentClaimGen = 0
	rs.intentClaimUntil = time.Time{}
	rs.gateClaimID = ""
	// Invalidate any in-flight post-turn gate so it cannot complete after Stop.
	rs.gateEpoch++
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
	// Idempotency replay must win over every other gate below: a retry carrying
	// the same key as an already-started turn is not a new turn request, so it
	// must not be rejected by state that only applies to genuinely new turns.
	//
	// BUG-288 R19-2 / R20-1: durable keys are two-phase, and launch-ack alone is
	// not a full short-circuit. prep:<turnID> or bare turnID without recoverable
	// progress must re-enter the launch path with the same turnID so recovery
	// owns incomplete dispatches (callers must not clear outer intent on a ghost).
	var preparedReuseTurnID string
	if idempotencyKey != "" {
		if raw, ok := rs.idempotency[idempotencyKey]; ok {
			tid, launched := parseDurableIdemValue(raw)
			if tid != "" && !strings.HasPrefix(idempotencyKey, "durable-") {
				s.mu.Unlock()
				return tid, nil
			}
			if tid != "" && strings.HasPrefix(idempotencyKey, "durable-") {
				if durableIdemReplaySafe(rs, tid, launched) {
					s.mu.Unlock()
					return tid, nil
				}
				// Incomplete prep or orphan launch-ack: relaunch with same turnID.
				preparedReuseTurnID = tid
			}
		}
	}
	// V10R4 P0: reject turns while a durable approval/question card is still pending
	// (crash between session-intent and resolved card must not start continuation).
	if rs.pendingApprovalID != "" || rs.pendingQuestionID != "" {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "awaiting_user", "run has a pending approval or question; resolve it before a new turn")
	}
	// CP-51 A1 residual (run-9437): flow hub must not start a write/tool/gate
	// turn while any flow child is still RUNNING/waiting. Unlock before the
	// child scan — hasActiveFlowChild takes per-child locks.
	parkHub := rs.parentRunID == "" && rs.flowEngineDriven
	parkRunID := rs.id
	// V10R4 P0: Stop sets loop Status=stopped — reject new turns on this run and
	// on children whose parent flow was stopped (intent race after Stop).
	//
	// run-1675: also reject Status=blocked (escalate / cap / hub_stalled form).
	// resumeFlowWithFeedback flips blocked→running BEFORE scheduling the next
	// hub turn, so Continue still works. Gate reprompt / hub reinvoke must not
	// start while the human decision card is open.
	// BUG-305/BUG-308: reset the per-turn flag every turn; set below for a root
	// run whose loop is already terminal-sealed ("done" or "stopped") at
	// admission (a plain follow-up chat turn).
	rs.turnStartedAfterLoopDone = false
	if rs.parentRunID == "" {
		st := s.agentOrchestrator.loopStateFor(rs.id).Status
		// BUG-302: "done" means the flow's own decision loop finished (CP-36
		// "loop ends, no further spawns") — it does not mean the run itself is
		// sealed forever. The desktop's composer (ChatInput) is the same for
		// every run regardless of Chat vs Workflow mode, with no distinct
		// "closed" affordance either way, so both must stay usable for a
		// follow-up turn once their flow completes.
		//
		// BUG-308: a genuinely Stop-ped loop is now treated the same as "done"
		// for a NEW user follow-up — the composer is identical and a user
		// naturally keeps typing after Stop; sealing it with 409 (and silently
		// dropping the optimistic prompt on restart) was the run-19845 bug.
		// This deliberately reverses BUG-302's V-1 scope (which kept "stopped"
		// sealed). Safe: stopAgentLoop already cancelled children + cleared
		// durable approval/question/resume intents under s.mu before this
		// admission can run (s.mu serializes them), the dispatch stop-fence
		// still fences any stale child send, BUG-307 stops a stranded cohort
		// note from poisoning this turn, and the intent-race guard (BUG-289
		// R19-1) is a separate durable-persist-abort path that never depended
		// on this loop-status seal. "blocked" still seals — it is a live
		// Continue/Stop decision the user must resolve first.
		//
		// BUG-305: remember a follow-up admitted onto an already-sealed loop so
		// emitLocked/runTurn treat it as plain chat (no post-turn flow gate).
		rs.turnStartedAfterLoopDone = st == "done" || st == "stopped"
		// BUG-308 residual run-33289 (Grok Review Loop, also provider-agnostic):
		// Stop leaves (1) rs.status=cancelled and (2) durable dispatch
		// run_stop.stopped=true. Admission alone is not enough — startTurn then
		// claim succeeds but linearizeSendStarted hits ErrRunStopFence, commits
		// terminal_cancelled, and returns WITHOUT EventTurnFailed/Completed so
		// the desktop hangs on Thinking after the optimistic prompt. Revive the
		// run for this plain-chat turn and release the hub's own stop flag
		// (generation kept so children stamped under the Stop stay fenced).
		if rs.turnStartedAfterLoopDone {
			if rs.status == RunStatusCancelled {
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
			}
		}
		if st == "blocked" {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusConflict, "flow_awaiting_user",
				"flow is waiting for your decision (Continue/Stop); resolve the form before a new turn")
		}
	} else if st := s.agentOrchestrator.loopStateFor(rs.parentRunID).Status; st == "stopped" || st == "done" {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "flow_stopped", "parent flow loop is stopped; cannot start a child turn")
	} else if st == "blocked" {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "flow_awaiting_user",
			"parent flow is waiting for your decision; resolve the form before a child turn")
	}
	// CP-51 A1 residual: park hub write turns while flow children are active.
	// Must drop s.mu before shouldParkHubWriteTurn (it re-locks for the scan).
	// System/orchestration reinvokes (hub.notify, synthesis join) must still
	// run when no writer child is active — park is child-scoped only.
	if parkHub {
		s.mu.Unlock()
		if s.shouldParkHubWriteTurn(parkRunID) {
			return "", newAPIErr(http.StatusConflict, "hub_parked",
				"hub is parked while flow children are running or waiting; retry after children settle")
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
		}
	}
	// V9-03: reject overlapping turns while post-turn gate is still settling.
	if rs.pendingFlowGateSettle || rs.postTurnGateCancel != nil {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "gate_in_progress", "post-turn gate still running; wait for gate pass/block before a new turn")
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
	if rs.turnInFlight {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusConflict, "turn_in_progress", "a turn is already in flight for this session")
	}
	if in.StepID == "" {
		s.mu.Unlock()
		return "", newAPIErr(http.StatusBadRequest, "invalid_request", "stepId is required")
	}

	// Resolved here (not just in runTurn) so a provider whose adapter construction
	// is model-dependent (Grok — see ProviderRegistration.newAdapterForTurn) gets
	// the right value at construction time, not just when TurnRequest is built.
	turnModel, turnEffort := resolveTurnModelAndEffort(rs, in)
	// BUG-334: spawned child runs isolate their opencode acp process so their
	// per-turn session/new (fresh MCP token) cannot reset a concurrent parent
	// turn's MCP connection (opencode keys MCP clients by server NAME per
	// process — the F-section spawn_agent "Connection closed" failure).
	adapter, aerr := s.registry.AdapterWithScope(rs.providerKey, turnModel, turnEffort, opencodeChildScopeHint(rs))
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

	// BUG-288 R19-2: prepared relaunch reuses the pre-persisted turnID and skips
	// first-turn bookkeeping that already applied before the crash window.
	isPreparedRelaunch := preparedReuseTurnID != ""
	turnID := s.nextID("turn")
	if isPreparedRelaunch {
		turnID = preparedReuseTurnID
	}
	// CP-51 Task-249: CreatePrepared at the prep barrier (fail-closed).
	if s.dispatchV2ActiveForRun(context.Background(), rs.id) {
		if !providerV2Enabled(rs.providerKey) {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusConflict, "provider_v2_disabled",
				fmt.Sprintf("provider %s is V2-disabled pending Task-257 capability evidence", rs.providerKey))
		}
		if err := s.prepareDispatchV2(context.Background(), rs, in, turnID, idempotencyKey); err != nil {
			s.mu.Unlock()
			return "", newAPIErr(http.StatusBadGateway, "dispatch_prepare_failed", err.Error())
		}
	}
	rs.turnInFlight = true
	rs.reinvokeInFlight = false // the turn the reinvoke scheduled is now in flight
	// Successful launch resets H1 consecutive-fail counter.
	rs.hubReinvokeStartFailCount = 0
	flowStartOnly := false
	// A new turn resets the idle-summary window to zero (a pending summary timer
	// is cancelled here and re-armed when this turn completes).
	s.cancelChatSummary(rs.id)
	if !isPreparedRelaunch {
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
			// BUG-315: only a genuinely new chat's first turn starts the flow. A
			// Drive-restored run is a continuation whose flow already ran on the
			// source machine; it keeps its chatFlowRef/workflowID, so without the
			// restoredFrom guard a plain follow-up (explicit flowRef, or one
			// re-resolved from workflowID) would re-spawn the entire flow instead of
			// reaching the hub. The primary fix carries TurnCount through the manifest
			// so a restored run has turnCount>0 and never enters this turnCount==0
			// block at all; this guard additionally heals chats synced by a pre-fix
			// manifest, which still restore with turnCount==0. flowEngineDriven is
			// already restored by reconstructRun (via ActiveFlowNodes), so skipping
			// here does not demote a restored hub to a plain chat.
			if flowRef := strings.TrimSpace(in.FlowRef); flowRef != "" && strings.TrimSpace(rs.restoredFrom) == "" {
				flowStartOnly = true
				rs.flowEngineDriven = true
				// BUG-299 residual: chat-mode Review Loop (and any explicit flowRef)
				// still uses hub/cohort/gate machinery — lock YOLO=true before the
				// async entry spawn so children inherit the product posture.
				rs.yolo = true
				rs.chatSubMode = strings.TrimSpace(in.SubMode)
				rs.chatFlowRef = flowRef
				go s.startResolvedFlow(context.Background(), runID, flowRef, in.Prompt)
			}
		}
		rs.turnCount++
	}
	// BUG-289 M1/F-8: repromptAttempts is a per-turn cap (maxFlowGateReprompts=2),
	// not a lifetime budget. Reset at every new turn start so resume/reinvoke
	// does not inherit a prior turn's exhausted counter.
	rs.repromptAttempts = 0
	touchHubProgressLocked(rs)
	rs.stepID = in.StepID         // durable for rehydrate-approve restart (V10R P1)
	rs.lastTurnStepID = in.StepID // CP-35: remember for gate reprompts
	// CP-51 A1: a new hub turn (N+1) consumes any prior continue-delegate marker
	// so only the original continue transfer suppresses a gate reprompt.
	if rs.parentRunID == "" {
		clearHubContinueDelegatedMarkerLocked(rs, turnID)
	}
	rs.currentTurnID = turnID
	// BUG-235: lastPrompt is the history list's title source (Navigator.tsx renders
	// runTitle(item.lastPrompt || item.lastMessage)). Every internal flow-engine-
	// generated turn — the hub auto-reinvoke ("[flow-engine] Agent results ready...",
	// autoReinvokePromptText), the coder re-entry, and the Continue-resume note — is
	// prefixed "[flow-engine]" and used to overwrite it unconditionally, so a flow
	// run's history entry ended up titled by whichever internal prompt ran last
	// instead of the user's original request. Skip the overwrite for all system
	// prompts (flow-engine joined notes, gate reprompts, handoffs; unless
	// lastPrompt is still empty, so a run always has SOME title).
	if p := strings.TrimSpace(in.Prompt); !isSystemPrompt(p) || rs.lastPrompt == "" {
		rs.lastPrompt = truncateDisplayField(in.Prompt, 100)
	}
	// CP-43 P-1 Plan A: keep the full user prompt for Change Contract fallback.
	// lastPrompt is truncated to 100 chars for display, so a declared contract
	// from the prompt would be lost at gate time. Store the complete text when
	// it is a real user prompt (not a system reprompt/flow-engine note).
	if p := strings.TrimSpace(in.Prompt); !isSystemPrompt(p) {
		rs.lastFullPrompt = in.Prompt
	} else if strings.TrimSpace(rs.lastFullPrompt) == "" {
		rs.lastFullPrompt = in.Prompt
	}
	rs.updatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	ctx, cancel := context.WithCancel(context.Background())
	rs.turnCancel = cancel
	// BUG-288 R19-2: durable pre-persist stores prep:<turnID> only. Full launch
	// ack (bare turnID) is written after revalidation + side effects, right before
	// the provider goroutine starts.
	durableKey := strings.HasPrefix(idempotencyKey, "durable-")
	if idempotencyKey != "" {
		if rs.idempotency == nil {
			rs.idempotency = map[string]string{}
		}
		if durableKey {
			rs.idempotency[idempotencyKey] = durableIdemPreparedValue(turnID)
		} else {
			rs.idempotency[idempotencyKey] = turnID
		}
	}
	// BUG-288 R18-7: for durable-* keys, persist session BEFORE markStepRunning
	// / EventTurnStarted so a persist failure cannot leave a ghost RUNNING step
	// and TurnStarted event with no provider turn.
	// BUG-288 R19-1: capture start token before unlock; after relock revalidate
	// before any markStepRunning / EventTurnStarted so Stop mid-persist cannot
	// resurrect a Cancelled workflow.
	isParent := rs.parentRunID == ""
	parentRunID := rs.parentRunID
	startToken := turnID
	if durableKey {
		snap := sessionStateOfProtectingIdem(rs, idempotencyKey)
		s.mu.Unlock()
		if isParent {
			snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState
		}
		if err := s.persistProviderSession(snap); err != nil {
			log.Printf("[turn] persist durable idempotency run=%s key=%s: %v (retrying)", runID, idempotencyKey, err)
			if err2 := s.persistProviderSession(snap); err2 != nil {
				log.Printf("[turn] persist durable idempotency FAILED run=%s key=%s: %v (aborting before step/event)", runID, idempotencyKey, err2)
				s.mu.Lock()
				if current := s.runs[runID]; current != nil {
					current.turnInFlight = false
					current.currentTurnID = ""
					current.turnCancel = nil
					delete(current.idempotency, idempotencyKey)
				}
				s.mu.Unlock()
				cancel()
				return "", newAPIErr(http.StatusBadGateway, "session_persist_failed",
					"durable turn idempotency could not be persisted; not starting provider turn: "+err2.Error())
			}
		}
		s.mu.Lock()
		rs = s.runs[runID]
		if abort, aerr := s.abortDurableStartIfStaleLocked(runID, rs, startToken, idempotencyKey, parentRunID, isParent, cancel); abort {
			return "", aerr
		}
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
	// BUG-293: redact the DISPLAY prompt for internal flow-engine/system prompts
	// so the live bubble matches replay (which hides them). Provider still gets the
	// full in.Prompt via runTurn and the turn log below.
	// BUG-326 (run-104296): stamp every system-generated prompt (gate reprompt,
	// handoff envelope, flow-engine/hub orchestration) with the durable
	// systemPromptTag at the wire boundary. Replay classifies a prompt as system
	// by the tag alone, so it survives provider-side wrapping — e.g. Grok embeds
	// the prompt inside a "## History" preamble plus tool reinforcement within
	// <user_query> and the old HasPrefix gate-detector missed it, shifting every
	// Q/A pair by one on reopen. The stamp lands on what the provider receives
	// AND what the turn log records (both derive from in.Prompt below), so replay
	// from either source classifies identically. User prompts are never stamped —
	// their ask_user/spawn_agent reinforcement is provider-adapter text, not a
	// system prompt, and must stay visible.
	if isSystemPrompt(in.Prompt) && !strings.Contains(in.Prompt, systemPromptTag) {
		in.Prompt = systemPromptTag + "\n" + in.Prompt
	}
	s.emitLocked(rs, ProviderEvent{Type: EventTurnStarted, ProviderTurnID: turnID, WorkflowStepRunID: in.StepID, Prompt: liveTurnStartedDisplayPrompt(in.Prompt)})
	// BUG-288 R20-1 / CP-51 DOD-G8: promote prep → bare launch-ack ONLY after
	// TurnStarted is in RAM, and only launch the provider AFTER launch-ack is
	// durable. Fail-closed on persist: keep prep on disk, abort without go
	// runTurn so recovery can relaunch instead of double-running after a partial
	// launch with orphan prep.
	//
	// Recovery authority is DispatchRecord.State (see RecoveryInferenceRule),
	// NOT EventTurnStarted — that event is emitted before go runTurn and is
	// insufficient evidence that the provider received the turn.
	if durableKey && idempotencyKey != "" {
		if rs.idempotency == nil {
			rs.idempotency = map[string]string{}
		}
		rs.idempotency[idempotencyKey] = turnID
	}
	snap := sessionStateOfProtectingIdem(rs, idempotencyKey)
	// Drain pendingAgentContext atomically with turnInFlight=true so that any concurrent
	// maybeAutoReinvokeHub call sees an empty slice after this unlock and does not set
	// pendingHubReinvoke spuriously. runTurn receives the captured slice directly.
	capturedCtx := rs.pendingAgentContext
	rs.pendingAgentContext = nil
	// BUG-307 (extended): a follow-up admitted onto an already-sealed loop
	// ("done" or Stop-ped — rs.turnStartedAfterLoopDone) is plain chat, not a
	// flow turn. Any flow-engine ORCHESTRATION note still queued here — the
	// entry-spawn "wait for its result; you will be reinvoked automatically once
	// this step of the flow completes" note (spawnChildRun's ParentContextNote),
	// or a cohort-join "…call the flow's control tool" note — is an instruction
	// for a hub flow turn that will never run again on a sealed loop. If it rides
	// into this user turn (composeAgentContextBlock wraps it) the model sits
	// waiting for a reinvoke that never comes and the hub stalls (live repro
	// run-21028). Guarding the append sites alone is insufficient: the entry-spawn
	// note is appended at flow START (loop still running) and only goes stale when
	// the Stop lands, so it can only be dropped here at the consumption point. A
	// legitimate synthesis reinvoke runs only while the loop is still advancing
	// (turnStartedAfterLoopDone=false there), so its note is never stripped.
	// Informational notes (UI-spawn, "sub-agent completed") are not flow-engine
	// prompts and are preserved.
	if rs.turnStartedAfterLoopDone && len(capturedCtx) > 0 {
		kept := make([]string, 0, len(capturedCtx))
		for _, note := range capturedCtx {
			if isFlowEnginePrompt(note) {
				continue
			}
			kept = append(kept, note)
		}
		capturedCtx = kept
	}
	s.mu.Unlock()
	if isParent {
		snap.LoopState = s.agentOrchestrator.graphSnapshot(rs.id).LoopState
	}
	if durableKey {
		if err := s.persistProviderSession(snap); err != nil {
			log.Printf("[turn] persist durable launch-ack run=%s key=%s: %v (retrying)", runID, idempotencyKey, err)
			if err2 := s.persistProviderSession(snap); err2 != nil {
				log.Printf("[turn] persist durable launch-ack FAILED run=%s key=%s: %v (aborting; not launching provider)", runID, idempotencyKey, err2)
				s.mu.Lock()
				if current := s.runs[runID]; current != nil {
					// Revert to prep so recovery re-enters launch; do not leave bare
					// without a provider, and do not leave prep while provider runs.
					if current.idempotency == nil {
						current.idempotency = map[string]string{}
					}
					current.idempotency[idempotencyKey] = durableIdemPreparedValue(turnID)
					current.turnInFlight = false
					current.currentTurnID = ""
					current.turnCancel = nil
					prepSnap := sessionStateOfProtectingIdem(current, idempotencyKey)
					s.mu.Unlock()
					_ = s.persistProviderSession(prepSnap) // best-effort restore prep
				} else {
					s.mu.Unlock()
				}
				cancel()
				return "", newAPIErr(http.StatusBadGateway, "session_persist_failed",
					"durable launch-ack could not be persisted; not starting provider turn: "+err2.Error())
			}
		}
	}
	// CP-51 Task-249: durable prepared → send_claimed at launch-ack (fail-closed).
	if s.dispatchStore != nil && s.dispatchV2ActiveForRun(context.Background(), runID) {
		s.mu.Lock()
		rs = s.runs[runID]
		if rs == nil {
			s.mu.Unlock()
			cancel()
			return "", newAPIErr(http.StatusNotFound, "run_not_found", "run disappeared before claim")
		}
		if err := s.claimDispatchV2(context.Background(), rs, turnID); err != nil {
			rs.turnInFlight = false
			rs.currentTurnID = ""
			rs.turnCancel = nil
			s.mu.Unlock()
			cancel()
			return "", newAPIErr(http.StatusBadGateway, "dispatch_claim_failed", err.Error())
		}
		s.mu.Unlock()
	}
	if durableKey {
		// already persisted above
	} else {
		// Non-durable: best-effort session persist after start (BUG-080 F-3).
		_ = s.persistProviderSession(snap)
	}
	// Persist raw user prompt for transcript replay (BUG-083 F-1): the provider
	// session file records the composed prompt (raw + reinforcement + skill preamble),
	// so we keep the raw input separately and prefer it on resume.
	if logger, logOK := s.workflowStore.(TurnLogStore); logOK {
		_ = logger.AppendTurnLog(context.Background(), runID, turnLogLine{Kind: turnLogKindPrompt, TurnID: turnID, Prompt: in.Prompt})
	}
	if flowStartOnly {
		var snap ProviderSessionState
		var terminalRS *interactiveRun
		s.mu.Lock()
		if current := s.runs[runID]; current != nil {
			// Flow-start handoff suppresses the hub's provider turn, but the desktop
			// still opened a live turn stream for this providerTurnId. Emit a terminal
			// event so the client can settle the synthetic turn and begin the separate
			// orchestration stream that carries later hub/agent updates.
			s.emitLocked(current, ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: ""})
			// run-1618 / A1: emitLocked on a flowEngineDriven root marks
			// pendingFlowGateSettle for a real post-turn gate. The synthetic
			// handoff has no provider work and no gate — leave it set and the
			// hub is stuck gate_in_progress forever (child fail cannot reinvoke).
			s.clearStaleHubPendingGateSettleLocked(current)
			current.turnInFlight = false
			current.currentTurnID = ""
			current.turnCancel = nil
			current.lastTurnID = turnID
			// Synthetic completion is terminal evidence for durable keys.
			if durableKey && idempotencyKey != "" {
				if current.idempotency == nil {
					current.idempotency = map[string]string{}
				}
				current.idempotency[idempotencyKey] = turnID
			}
			snap = sessionStateOf(current)
			terminalRS = current
		}
		s.mu.Unlock()
		if snap.RunID != "" {
			_ = s.persistProviderSession(snap)
		}
		// CP-51: claim left the record at send_claimed; no provider bytes are
		// sent on this path, so linearize+Terminal as a clean synthetic complete
		// so recovery does not redispath a dangling flow-start claim.
		if terminalRS != nil && s.dispatchStore != nil && s.dispatchV2ActiveForRun(context.Background(), runID) {
			if ok, _ := s.linearizeSendStarted(context.Background(), terminalRS, turnID); ok {
				bridge := &turnBridge{svc: s, rs: terminalRS, ctx: context.Background(), turnID: turnID}
				payload, _ := json.Marshal(map[string]string{
					"type": string(EventTurnCompleted), "final": "", "synthetic": "flow_start_only",
				})
				bridge.Terminal(TerminalEvidence{
					ProviderKey:          string(terminalRS.providerKey),
					EvidenceKind:         string(EventTurnCompleted),
					Outcome:              "completed",
					PayloadCanonicalJSON: payload,
					PayloadSHA256:        HashBytes(payload),
					ObservedAt:           nowRFC3339Nano(),
				})
			}
		}
		cancel()
		return turnID, nil
	}
	// Flow Mode: clear cached FlowContextPackage when a Plan step reruns so the next
	// Coding step rebuilds the package from the new Plan output (T-3, Task-169).
	s.maybeClearPlanContextForPlanStep(ctx, runID, rs, in.StepID)

	// Provider launch only after durable launch-ack (or non-durable path).
	go s.runTurn(ctx, rs, adapter, in, scenario, turnID, capturedCtx)
	return turnID, nil
}

// abortDurableStartIfStaleLocked validates post-persist state before side effects
// (BUG-288 R19-1). Caller holds s.mu. On abort, unlocks and cancels.
func (s *InteractiveService) abortDurableStartIfStaleLocked(
	runID string, rs *interactiveRun, startToken, idempotencyKey, parentRunID string, isParent bool, cancel context.CancelFunc,
) (aborted bool, err *apiErr) {
	if rs == nil {
		s.mu.Unlock()
		cancel()
		return true, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	// Stop / concurrent clear must win over late durable start.
	if rs.currentTurnID != startToken || !rs.turnInFlight {
		if rs.currentTurnID == startToken {
			rs.turnInFlight = false
			rs.currentTurnID = ""
			rs.turnCancel = nil
			if idempotencyKey != "" {
				delete(rs.idempotency, idempotencyKey)
			}
		}
		s.mu.Unlock()
		cancel()
		return true, newAPIErr(http.StatusConflict, "turn_aborted", "durable start aborted: turn no longer in flight after persist")
	}
	if rs.status == RunStatusCancelled && !rs.turnStartedAfterLoopDone {
		rs.turnInFlight = false
		rs.currentTurnID = ""
		rs.turnCancel = nil
		if idempotencyKey != "" {
			delete(rs.idempotency, idempotencyKey)
		}
		s.mu.Unlock()
		cancel()
		return true, newAPIErr(http.StatusConflict, "flow_stopped", "run was cancelled during durable start persist")
	}
	// Loop / parent loop stopped (Stop bumps loop under s.mu before persist of cancel).
	// BUG-308 residual: a plain-chat follow-up admitted onto stopped/done sets
	// turnStartedAfterLoopDone — do not abort that intentional turn.
	loopID := runID
	if !isParent && parentRunID != "" {
		loopID = parentRunID
	}
	if st := s.agentOrchestrator.loopStateFor(loopID).Status; (st == "stopped" || st == "done") && !rs.turnStartedAfterLoopDone {
		rs.turnInFlight = false
		rs.currentTurnID = ""
		rs.turnCancel = nil
		if idempotencyKey != "" {
			delete(rs.idempotency, idempotencyKey)
		}
		s.mu.Unlock()
		cancel()
		return true, newAPIErr(http.StatusConflict, "flow_stopped", "flow loop stopped during durable start persist")
	}
	return false, nil
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
		// run-75035 / Grok run-536 class: prefer the id THIS turn opened via
		// thread/start|resume. Workspace-wide newest-cwd discovery is not run
		// ownership — hub+children share working_directory, so the newest
		// rollout often belongs to another run and was previously logged into
		// the child's turn log (seed then imported hub freeform into coder).
		if reporter, ok := adapter.(interface{ LastCodexSessionID() string }); ok {
			if id := strings.TrimSpace(reporter.LastCodexSessionID()); isCodexRealSessionID(id) {
				if rs.realProviderSessionID == "" {
					rs.realProviderSessionID = id
				}
				if id != rs.lastCodexTurnSessionID {
					rs.lastCodexTurnSessionID = id
					return id
				}
				return ""
			}
			// Adapter present but reported nothing for this turn: do not invent
			// a foreign cwd rollout (mirror Grok no-steal discipline).
			return ""
		}
		// Compatibility path for callers with no reporting adapter (legacy
		// multi-rollout unit tests pass adapter=nil). Still refuse ids already
		// owned by parent/sibling/other runs in the session store.
		home, ok := s.resolveAccountHome(rs.providerKey, rs.providerAccountID)
		if !ok {
			return ""
		}
		if rolloutID, found := DiscoverCodexRolloutSessionID(home, rs.workspaceCwd); found {
			if s.isForeignProviderSessionID(rs, rolloutID) {
				return ""
			}
			if rs.realProviderSessionID == "" {
				rs.realProviderSessionID = rolloutID
			}
			if rolloutID != rs.lastCodexTurnSessionID {
				rs.lastCodexTurnSessionID = rolloutID
				return rolloutID
			}
		}
	case ProviderKeyGrok:
		// Task-210 Option B + run-536 hotfix:
		// NEVER promote realProviderSessionID from workspace-wide session-dir
		// discovery. Multiple Grok chats share one cwd under GROK_HOME; the
		// newest dir often belongs to a different run. Stealing it made a
		// brand-new chat persist another chat's ACP id and break first-turn
		// continuity (run-536: first chat logged 019f60f2 from an older dir).
		//
		// Only promote an id the adapter actually opened this turn via
		// session/new or session/load (LastGrokSessionID). That is the durable
		// handle for same-account session/load and cross-account relocate.
		// CA-502: also refuse ids already owned by parent/sibling (shared
		// chat_history.jsonl — run-98153 dual reviewers).
		if reporter, ok := adapter.(interface{ LastGrokSessionID() string }); ok {
			if id := strings.TrimSpace(reporter.LastGrokSessionID()); isGrokRealSessionID(id) {
				if s.isForeignProviderSessionID(rs, id) {
					return ""
				}
				rs.realProviderSessionID = id
				// Promote the in-memory placeholder so paths that still read
				// rs.providerSessionID (dispatch envelope, some persist helpers)
				// pass the ACP id on the next model/effort/YOLO respawn.
				// Claude must keep the synthetic pool key (BUG-295); Grok's
				// thread-* is only a first-turn placeholder.
				if !isGrokRealSessionID(rs.providerSessionID) {
					rs.providerSessionID = id
				}
				if id != rs.lastGrokTurnSessionID {
					rs.lastGrokTurnSessionID = id
					return id
				}
				return ""
			}
		}
		// No adapter-reported id (failed ensureSession, or non-live double):
		// do not invent one from disk. Leave realProviderSessionID unchanged so
		// a failed first turn cannot rebind the run onto someone else's session.
		return ""
	case ProviderKeyOpencode:
		// Appended last (CP-57 P-0/Task-303 T-7): mirror Grok's durable resume.
		// Only promote an id the adapter actually opened this turn via
		// session/new or session/load (LastOpencodeSessionID). Never steal from disk.
		if reporter, ok := adapter.(interface{ LastOpencodeSessionID() string }); ok {
			if id := strings.TrimSpace(reporter.LastOpencodeSessionID()); isOpencodeRealSessionID(id) {
				if s.isForeignProviderSessionID(rs, id) {
					return ""
				}
				rs.realProviderSessionID = id
				if !isOpencodeRealSessionID(rs.providerSessionID) {
					rs.providerSessionID = id
				}
				if id != rs.lastOpencodeTurnSessionID {
					rs.lastOpencodeTurnSessionID = id
					return id
				}
				return ""
			}
		}
		return ""
	}
	return ""
}

// isCodexRealSessionID is true for durable Codex rollout/thread ids (not the
// synthetic thread-<n> placeholders used before a real app-server session exists).
func isCodexRealSessionID(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && !strings.HasPrefix(id, "thread-")
}

// isForeignProviderSessionID reports whether sessionID is already owned by a
// different run that is related to rs (same project/cwd, parent, or sibling).
// Used to refuse workspace-newest discovery theft and to filter polluted turn
// logs on seed (run-75035). Grok ACP ids use the same non-thread-* durable
// shape as Codex rollouts for ownership checks (CA-502 / run-98153).
func (s *InteractiveService) isForeignProviderSessionID(rs *interactiveRun, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if rs == nil || sessionID == "" || strings.HasPrefix(sessionID, "thread-") {
		return false
	}
	// Own durable handles always count as non-foreign.
	if sessionID == strings.TrimSpace(rs.realProviderSessionID) || sessionID == strings.TrimSpace(rs.providerSessionID) {
		return false
	}
	if sessionID == strings.TrimSpace(rs.lastCodexTurnSessionID) || sessionID == strings.TrimSpace(rs.lastGrokTurnSessionID) || sessionID == strings.TrimSpace(rs.lastOpencodeTurnSessionID) {
		return false
	}
	foreign := s.foreignProviderSessionIDs(rs)
	_, ok := foreign[sessionID]
	return ok
}

// foreignProviderSessionIDs returns provider session ids owned by other related
// runs (parent, siblings under the same parent, or other sessions sharing
// project+cwd). Includes live in-memory related runs even when the store is empty.
func (s *InteractiveService) foreignProviderSessionIDs(rs *interactiveRun) map[string]struct{} {
	out := map[string]struct{}{}
	if s == nil || rs == nil {
		return out
	}
	ownParent := strings.TrimSpace(rs.parentRunID)
	ownProject := strings.TrimSpace(rs.projectID)
	ownCwd := strings.TrimSpace(rs.workspaceCwd)
	var sessions []ProviderSessionState
	if s.workflowStore != nil {
		if indexReader, ok := s.workflowStore.(SessionIndexReader); ok {
			if listed, err := indexReader.ListAllProviderSessions(context.Background()); err == nil {
				sessions = listed
			}
		}
	}
	for _, st := range sessions {
		otherRun := strings.TrimSpace(st.RunID)
		if otherRun == "" || otherRun == rs.id {
			continue
		}
		otherParent := strings.TrimSpace(st.ParentRunID)
		related := false
		if ownParent != "" && (otherRun == ownParent || otherParent == ownParent) {
			related = true
		}
		if otherParent != "" && otherParent == rs.id {
			related = true // our child
		}
		if ownProject != "" && st.ProjectID == ownProject && ownCwd != "" && st.WorkingDirectory == ownCwd {
			related = true
		}
		if !related {
			continue
		}
		if sid := strings.TrimSpace(st.ProviderSessionID); isCodexRealSessionID(sid) || isGrokRealSessionID(sid) || isOpencodeRealSessionID(sid) {
			// Do not treat our own id as foreign if listed under another row erroneously.
			if sid == strings.TrimSpace(rs.realProviderSessionID) || sid == strings.TrimSpace(rs.providerSessionID) {
				continue
			}
			out[sid] = struct{}{}
		}
	}
	// Live in-memory related runs (siblings may not be durable yet — CA-502).
	// Caller may already hold s.mu (refreshResumeHandleLocked); do not re-lock.
	for _, other := range s.runs {
		if other == nil || other.id == rs.id {
			continue
		}
		otherParent := strings.TrimSpace(other.parentRunID)
		related := false
		if ownParent != "" && (other.id == ownParent || otherParent == ownParent) {
			related = true
		}
		if otherParent != "" && otherParent == rs.id {
			related = true
		}
		if ownProject != "" && other.projectID == ownProject && ownCwd != "" && other.workspaceCwd == ownCwd {
			related = true
		}
		if !related {
			continue
		}
		for _, sid := range []string{other.realProviderSessionID, other.providerSessionID, other.lastGrokTurnSessionID, other.lastCodexTurnSessionID, other.lastOpencodeTurnSessionID} {
			sid = strings.TrimSpace(sid)
			if sid == "" || strings.HasPrefix(sid, "thread-") {
				continue
			}
			if sid == strings.TrimSpace(rs.realProviderSessionID) || sid == strings.TrimSpace(rs.providerSessionID) {
				continue
			}
			out[sid] = struct{}{}
		}
	}
	// Parent/sibling turn-log codex_session / grok_session entries are also foreign
	// ownership signals (covers polluted historical logs where sessions.ndjson last
	// row still shows the child's own id).
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		candidateRuns := make([]string, 0, 8)
		if ownParent != "" {
			candidateRuns = append(candidateRuns, ownParent)
		}
		for _, st := range sessions {
			otherRun := strings.TrimSpace(st.RunID)
			if otherRun == "" || otherRun == rs.id {
				continue
			}
			otherParent := strings.TrimSpace(st.ParentRunID)
			if ownParent != "" && (otherRun == ownParent || otherParent == ownParent) {
				candidateRuns = append(candidateRuns, otherRun)
			}
		}
		seenRun := map[string]bool{}
		for _, runID := range candidateRuns {
			if seenRun[runID] {
				continue
			}
			seenRun[runID] = true
			entries, rerr := logger.ReadTurnLog(context.Background(), runID)
			if rerr != nil {
				continue
			}
			for _, e := range entries {
				if e.Kind != turnLogKindCodexSession && e.Kind != turnLogKindGrokSession && e.Kind != turnLogKindOpencodeSession {
					continue
				}
				if sid := strings.TrimSpace(e.SessionID); isCodexRealSessionID(sid) || isGrokRealSessionID(sid) || isOpencodeRealSessionID(sid) {
					if sid == strings.TrimSpace(rs.realProviderSessionID) || sid == strings.TrimSpace(rs.providerSessionID) {
						continue
					}
					out[sid] = struct{}{}
				}
			}
		}
	}
	return out
}

// filterOwnedProviderSessionIDs drops session ids known to belong to related
// foreign runs, preserving this run's own ids and unknown (not-yet-catalogued)
// rotation ids. Order is preserved.
func (s *InteractiveService) filterOwnedProviderSessionIDs(rs *interactiveRun, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	foreign := s.foreignProviderSessionIDs(rs)
	if len(foreign) == 0 {
		return ids
	}
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, isForeign := foreign[id]; isForeign {
			// Never drop our own durable handles even if also listed elsewhere.
			if id == strings.TrimSpace(rs.realProviderSessionID) || id == strings.TrimSpace(rs.providerSessionID) {
				out = append(out, id)
			}
			continue
		}
		out = append(out, id)
	}
	return out
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
	// BUG-288 P1-08: re-check the durable TTL under the lock before accepting a
	// decision. A rehydrated card's timer is a separate goroutine (see
	// scheduleApprovalExpiry) that may not have fired yet even though the
	// deadline has already passed (e.g. immediately after restart).
	if rec.status == "pending" && approvalExpiryElapsed(rec.expiresAt) {
		rec.status = "expired"
		state := approvalStateFromRecord(s.runs[rec.runID], rec, rec.expiresAt)
		s.mu.Unlock()
		if err := s.persistApproval(state); err != nil {
			log.Printf("[gate] persist approval expiry-on-submit %s: %v", approvalID, err)
		}
		return newAPIErr(http.StatusConflict, "question_expired", "approval expired")
	}

	var details ApprovalDetails
	var rememberCwd string
	var snapshot *ProviderApprovalState
	var sessionSnap *ProviderSessionState
	var resumeStepParent, resumeStepLabel string
	var resumeStepTurn bool
	var restartRunID, restartStepID, restartPrompt string
	var restartGen int64

	if rec.status == "resolving" {
		// BUG-288 P1-01: a retry of an in-flight resolution. The prior attempt
		// already mutated RAM and stashed the durable-write payload but did not
		// confirm persistence (network/store error) — replay only the missing
		// writes below instead of re-deriving RAM state (double gen bump,
		// duplicate restart, etc.) or short-circuiting to a false success.
		decision = rec.decision
		details = rec.details
		snapshot = rec.resolvingSnapshot
		sessionSnap = rec.resolvingSession
		resumeStepParent = rec.resolvingStepParent
		resumeStepLabel = rec.resolvingStepLabel
		resumeStepTurn = rec.resolvingStepTurn
		restartRunID = rec.resolvingRestartRunID
		restartStepID = rec.resolvingRestartStepID
		restartPrompt = rec.resolvingRestartPrompt
		restartGen = rec.resolvingRestartGen
		rememberCwd = rec.resolvingRememberCwd
		s.mu.Unlock()
	} else {
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
		// BUG-288 P1-01: "resolving" is a transitional state — RAM reflects the
		// decision so a concurrent read sees it, but it is NOT yet "resolved"
		// (which short-circuits future calls to a bare nil success). Only a
		// confirmed durable write below advances to "resolved".
		rec.status = "resolving"
		rec.decision = decision
		details = rec.details
		rehydrated := rec.rehydrated
		if rs := s.runs[rec.runID]; rs != nil {
			if rs.pendingApprovalID == approvalID {
				rs.pendingApprovalID = ""
			}
			rememberCwd = rs.workspaceCwd
			state := approvalStateFromRecord(rs, rec, "")
			snapshot = &state
			// BUG-288 #22: after child approval, return the flow step to RUNNING so
			// the timeline does not stay WAITING while the turn continues.
			// V10R4 P1: auto-resume provider turn ONLY when rehydrated after restart
			// (explicit provenance). Live workflow/provider cards use resolve channel.
			liveTurn := rs.turnInFlight && !rehydrated
			needsResume := rehydrated
			if rs.parentRunID != "" && rs.label != "" && (liveTurn || needsResume) {
				// V10R P1: flowEngineDriven is restored when ActiveFlowNodes exist;
				// also accept activeFlowNodes so step RUNNING still works if the flag
				// lagged on an older session.
				if parent := s.runs[rs.parentRunID]; parent != nil && (parent.flowEngineDriven || len(parent.activeFlowNodes) > 0) {
					resumeStepParent = rs.parentRunID
					resumeStepLabel = rs.label
					resumeStepTurn = true
				}
			}
			// After restart the provider goroutine is gone — resolve channel has no
			// waiter. On approve, restart the turn so the flow actually continues.
			if needsResume && decision == "approve" {
				restartRunID = rs.id
				restartStepID = durableResumeStepID(rs)
				if restartStepID != "" {
					rs.stepID = restartStepID
				}
				base := strings.TrimSpace(rs.lastPrompt)
				if base == "" {
					base = "Continue the previous task."
				}
				cmd := strings.TrimSpace(details.Command)
				if cmd != "" {
					restartPrompt = base + "\n\n[FlowPilot] The user approved the pending shell command after a runner restart: " + cmd +
						". Continue from where you left off."
				} else {
					restartPrompt = base + "\n\n[FlowPilot] The user approved a pending permission after a runner restart. Continue from where you left off."
				}
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
				// V10R4 P1: durable continuation intent — only cleared after startTurn OK.
				rs.pendingResumePrompt = restartPrompt
				rs.pendingResumeStepID = restartStepID
				rs.pendingResumeGen++
				restartGen = rs.pendingResumeGen
				// Reconciliation record so a crash before card persist can resolve
				// the pending card on rehydrate without re-asking the user.
				rs.pendingResumeApprovalID = approvalID
				rs.pendingResumeDecision = decision
			} else if needsResume && decision == "deny" {
				// Deny with no live turn: leave Running so the user can re-prompt;
				// do not invent a Failed terminal without provider context.
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
				rs.pendingResumeApprovalID = approvalID
				rs.pendingResumeDecision = decision
			} else {
				// Live resolve path still records decision for two-write reconcile.
				rs.pendingResumeApprovalID = approvalID
				rs.pendingResumeDecision = decision
			}
		} else {
			state := approvalStateFromRecord(nil, rec, "")
			snapshot = &state
		}
		// Capture session snap under lock when we mutated resume fields.
		if restartRunID != "" {
			if child := s.runs[restartRunID]; child != nil {
				st := sessionStateOf(child)
				sessionSnap = &st
			}
		} else if rs := s.runs[rec.runID]; rs != nil && strings.TrimSpace(rs.pendingResumeDecision) != "" {
			st := sessionStateOf(rs)
			sessionSnap = &st
		}
		// Stash the computed payload so a retry (status still "resolving" if we
		// fail below) redoes only the missing durable writes.
		rec.resolvingSnapshot = snapshot
		rec.resolvingSession = sessionSnap
		rec.resolvingStepParent = resumeStepParent
		rec.resolvingStepLabel = resumeStepLabel
		rec.resolvingStepTurn = resumeStepTurn
		rec.resolvingRestartRunID = restartRunID
		rec.resolvingRestartStepID = restartStepID
		rec.resolvingRestartPrompt = restartPrompt
		rec.resolvingRestartGen = restartGen
		rec.resolvingRememberCwd = rememberCwd
		s.mu.Unlock()
	}

	// V10R4 P1: persist session intent+decision BEFORE resolved card so a crash
	// leaves recoverable state (decision on session, card may still be pending).
	if sessionSnap != nil {
		if err := s.persistProviderSession(*sessionSnap); err != nil {
			return newAPIErr(http.StatusInternalServerError, "persist_failed",
				"failed to persist resume intent: "+err.Error())
		}
	}
	// Persist the resolved approval so its decision survives a full server
	// restart (BUG-272). Order after session intent for rehydrate recovery.
	if snapshot != nil {
		if err := s.persistApproval(*snapshot); err != nil {
			return newAPIErr(http.StatusInternalServerError, "persist_failed",
				"failed to persist approval decision: "+err.Error())
		}
	}

	// BUG-288 P1-01: durable writes above succeeded — commit resolved and drop
	// the retry stash. Only now is it safe for a concurrent/retried call to
	// observe "resolved" and short-circuit to success.
	s.mu.Lock()
	if rec.status == "resolving" {
		rec.status = "resolved"
		rec.resolvingSnapshot = nil
		rec.resolvingSession = nil
	}
	s.mu.Unlock()

	if resumeStepTurn {
		s.setFlowStepStatus(context.Background(), resumeStepParent, resumeStepLabel, StepStatusRunning)
	}

	// Persist the "don't ask again" rule outside the lock (file IO). Only shell
	// commands the user actually approved are eligible; deriveApprovalRule
	// rejects compound commands.
	if remember && decision == "approve" && details.Kind == "exec" && rememberCwd != "" {
		if rule, ok := deriveApprovalRule(details.Command); ok {
			_ = addApprovalAllowRule(filepath.Join(rememberCwd, ".flowpilot"), rule)
		}
	}

	// Non-blocking send: live turns wait on resolve; rehydrated has no waiter.
	select {
	case rec.resolve <- decision:
	default:
	}
	if restartRunID != "" && restartPrompt != "" && restartStepID != "" {
		go s.startTurnClearingIntent(restartRunID, restartStepID, restartPrompt, "resume", restartGen)
	}
	return nil
}

// AnswerQuestion is idempotent + first-write-wins. Free-text ("Other") is allowed,
// so any non-empty choice is accepted.
func (s *InteractiveService) AnswerQuestion(questionID string, choice []string) *apiErr {
	var snapshot *ProviderQuestionState
	var sessionSnap *ProviderSessionState
	var runID string
	var resumeStepParent, resumeStepLabel string
	var resumeStepTurn bool
	var restartRunID, restartStepID, restartPrompt string
	var restartGen int64

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
	// BUG-288 P1-08: see submitApprovalDecision's mirrored comment.
	if rec.status == "pending" && approvalExpiryElapsed(rec.expiresAt) {
		rec.status = "expired"
		state := questionStateFromRecord(rec, "", rec.expiresAt)
		s.mu.Unlock()
		if err := s.persistQuestion(state); err != nil {
			log.Printf("[gate] persist question expiry-on-answer %s: %v", questionID, err)
		}
		return newAPIErr(http.StatusConflict, "question_expired", "question expired")
	}

	if rec.status == "resolving" {
		// BUG-288 P1-01: retry of an in-flight resolution — replay the missing
		// durable writes from the stash instead of re-deriving RAM state.
		choice = rec.choice
		runID = rec.resolvingRunID
		snapshot = rec.resolvingSnapshot
		sessionSnap = rec.resolvingSession
		resumeStepParent = rec.resolvingStepParent
		resumeStepLabel = rec.resolvingStepLabel
		resumeStepTurn = rec.resolvingStepTurn
		restartRunID = rec.resolvingRestartRunID
		restartStepID = rec.resolvingRestartStepID
		restartPrompt = rec.resolvingRestartPrompt
		restartGen = rec.resolvingRestartGen
		s.mu.Unlock()
	} else {
		if len(choice) == 0 {
			s.mu.Unlock()
			return newAPIErr(http.StatusBadRequest, "invalid_decision", "a choice is required")
		}
		// BUG-288 P1-01: transitional state — see submitApprovalDecision.
		rec.status = "resolving"
		rec.choice = choice
		rehydrated := rec.rehydrated
		runID = rec.runID
		state := questionStateFromRecord(rec, "", "")
		snapshot = &state
		if rs := s.runs[rec.runID]; rs != nil {
			if rs.pendingQuestionID == questionID {
				rs.pendingQuestionID = ""
			}
			if rs.pendingApprovalID != "" {
				rs.status = RunStatusWaitingApproval
				rs.agentStatus = string(RunStatusWaitingApproval)
			} else {
				rs.status = RunStatusRunning
				rs.agentStatus = string(RunStatusRunning)
				// BUG-288 #22: child question answered → step back to RUNNING.
				// V10R4 P1: auto-resume ONLY when rehydrated after restart. Live
				// workflow-driven AskWorkflowQuestion has turnInFlight=false but is
				// not a dead provider turn — scheduling startTurn regressed
				// TestWorkflowDrivenQuestion (run flipped to completed).
				liveTurn := rs.turnInFlight && !rehydrated
				needsResume := rehydrated
				if rs.parentRunID != "" && rs.label != "" && (liveTurn || needsResume) {
					if parent := s.runs[rs.parentRunID]; parent != nil && (parent.flowEngineDriven || len(parent.activeFlowNodes) > 0) {
						resumeStepParent = rs.parentRunID
						resumeStepLabel = rs.label
						resumeStepTurn = true
					}
				}
				// Restart turn only after restart rehydration (dead provider turn).
				if needsResume {
					restartRunID = rs.id
					restartStepID = durableResumeStepID(rs)
					if restartStepID != "" {
						rs.stepID = restartStepID
					}
					base := strings.TrimSpace(rs.lastPrompt)
					if base == "" {
						base = "Continue the previous task."
					}
					answer := strings.Join(choice, ", ")
					restartPrompt = base + "\n\n[FlowPilot] The user answered a pending question after a runner restart: " +
						answer + ". Continue from where you left off."
					rs.pendingResumePrompt = restartPrompt
					rs.pendingResumeStepID = restartStepID
					rs.pendingResumeGen++
					restartGen = rs.pendingResumeGen
					// Reconciliation: question id + first choice as decision token.
					rs.pendingResumeApprovalID = questionID
					rs.pendingResumeDecision = answer
					// BUG-288 P2-01: keep the un-flattened slice too so a second
					// restart reconciles the real answer, not a re-wrapped join.
					rs.pendingResumeQuestionChoices = append([]string(nil), choice...)
				} else {
					rs.pendingResumeApprovalID = questionID
					rs.pendingResumeDecision = strings.Join(choice, ", ")
					rs.pendingResumeQuestionChoices = append([]string(nil), choice...)
				}
			}
		}
		if restartRunID != "" {
			if child := s.runs[restartRunID]; child != nil {
				st := sessionStateOf(child)
				sessionSnap = &st
			}
		} else if rs := s.runs[rec.runID]; rs != nil && strings.TrimSpace(rs.pendingResumeDecision) != "" {
			st := sessionStateOf(rs)
			sessionSnap = &st
		}
		// Stash for retry (BUG-288 P1-01).
		rec.resolvingRunID = runID
		rec.resolvingSnapshot = snapshot
		rec.resolvingSession = sessionSnap
		rec.resolvingStepParent = resumeStepParent
		rec.resolvingStepLabel = resumeStepLabel
		rec.resolvingStepTurn = resumeStepTurn
		rec.resolvingRestartRunID = restartRunID
		rec.resolvingRestartStepID = restartStepID
		rec.resolvingRestartPrompt = restartPrompt
		rec.resolvingRestartGen = restartGen
		s.mu.Unlock()
	}

	// V10R4 P1: session intent+decision before resolved question card.
	if sessionSnap != nil {
		if err := s.persistProviderSession(*sessionSnap); err != nil {
			return newAPIErr(http.StatusInternalServerError, "persist_failed",
				"failed to persist resume intent: "+err.Error())
		}
	}
	if snapshot != nil {
		if err := s.persistQuestion(*snapshot); err != nil {
			return newAPIErr(http.StatusInternalServerError, "persist_failed",
				"failed to persist question answer: "+err.Error())
		}
	}

	// BUG-288 P1-01: durable writes confirmed — commit resolved.
	s.mu.Lock()
	if rec.status == "resolving" {
		rec.status = "resolved"
		rec.resolvingSnapshot = nil
		rec.resolvingSession = nil
	}
	s.mu.Unlock()

	if resumeStepTurn {
		s.setFlowStepStatus(context.Background(), resumeStepParent, resumeStepLabel, StepStatusRunning)
	}
	if runID != "" {
		s.persistParentSession(runID)
	}
	select {
	case rec.resolve <- questionResolveResult{choices: choice}:
	default:
	}
	if restartRunID != "" && restartPrompt != "" && restartStepID != "" {
		go s.startTurnClearingIntent(restartRunID, restartStepID, restartPrompt, "resume", restartGen)
	}
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
	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	rec := &questionRecord{
		id:          s.nextID("q"),
		runID:       runID,
		prompt:      prompt,
		options:     options,
		multiSelect: multiSelect,
		status:      "pending",
		resolve:     make(chan questionResolveResult, 1),
		expiresAt:   expiresAt,
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	// Persist pending BEFORE emit/wait (BUG-288 P1-07); mirrors AskQuestion.
	// ExpiresAt is now the durable absolute TTL deadline (BUG-288 P1-08).
	pendingSnap := questionStateFromRecord(rec, "", expiresAt)
	s.mu.Unlock()
	if err := s.persistQuestion(pendingSnap); err != nil {
		s.mu.Lock()
		delete(s.questions, rec.id)
		if rs2 := s.runs[runID]; rs2 != nil && rs2.pendingQuestionID == rec.id {
			rs2.pendingQuestionID = ""
		}
		s.mu.Unlock()
		return nil, newAPIErr(http.StatusInternalServerError, "persist_failed", "failed to persist pending question: "+err.Error())
	}
	s.mu.Lock()
	if rs2 := s.runs[runID]; rs2 != nil {
		s.emitLocked(rs2, ProviderEvent{
			Type:        EventUserQuestionRequired,
			QuestionID:  rec.id,
			Prompt:      prompt,
			Options:     options,
			MultiSelect: multiSelect,
		})
	}
	s.mu.Unlock()

	timer := time.NewTimer(s.questionTTL)
	defer timer.Stop()
	select {
	case r := <-rec.resolve:
		if r.err != nil {
			return nil, newAPIErr(http.StatusConflict, "interrupted", "question interrupted")
		}
		return r.choices, nil
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
	case ProviderKeyGrok:
		// Appended last (CP-46 P-0/Task-209 T-11): codex/claude cases above unchanged.
		return "grok-4.5"
	case ProviderKeyOpencode:
		// Appended last (CP-57 P-0/Task-303 T-1).
		return "opencode/muse-spark-1.2-contributor-free"
	default:
		return ""
	}
}

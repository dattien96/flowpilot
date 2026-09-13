// ============================================================================
// The client<->runner contract (04-01 "The contract (key output)").
//
// This is the ONE artifact Phase 1 must lock. Both MockRunnerClient (Part A) and
// HttpWsRunnerClient (Part B) implement `RunnerClient` against these types; the
// renderer only ever talks to `RunnerClient`. The `ProviderEventDTO` union is the
// serialized form of the shared `ProviderEvent` union in 04-Detailed-Coding-Plan.md.
// Keep this in sync with the Go runner's event DTOs.
// ============================================================================

export type ProviderKey = "codex" | "claude" | "gemini" | "grok" | "opencode";

// ---- Domain (navigator) ----------------------------------------------------

export interface Project {
  id: string;
  name: string;
  /** Workspace path bound as the provider `cwd` (display only in Part A). */
  path: string;
  model?: string;
}

export interface Workflow {
  id: string;
  projectId: string;
  name: string;
  description?: string;
  model?: string;
  yoloMode?: boolean;
}

export interface Step {
  id: string;
  workflowId?: string;
  name: string;
  order: number;
  /** Skill auto-selected for this step, if any (source: workflow_default). */
  defaultSkill?: string;
  model?: string;
  yoloMode?: boolean;
}

export interface ProviderSkill {
  name: string;
  path?: string;
  description?: string;
  source: "provider" | "flowpilot" | "workspace";
}

/**
 * A loadable sub-agent definition (CP-19 / Task-081). Served by the runner's
 * AgentCatalog: project-local `.claude/agents` + `.codex/agents`, provider homes,
 * and FlowPilot built-ins, merged by name precedence. `provider`/`model` are
 * optional preferences (empty = inherit the spawning run's provider).
 */
export interface AgentDefinition {
  name: string;
  description: string;
  role: string;
  provider?: string;
  model?: string;
  modelReasoningEffort?: string;
  tools?: string[];
  systemPrompt?: string;
  /** Where the definition came from: "claude" | "codex" | "provider" | "flowpilot". */
  source: string;
  path?: string;
}

/**
 * Input for spawning a child agent run (CP-19 / Task-082).
 */
export interface SpawnAgentInput {
  agent: string;
  prompt: string;
  provider?: string;
  dependsOn?: string[];
  wait?: boolean;
}

/**
 * Result returned after spawning a child agent run.
 * When `wait` was true, `finalMessage` holds the child's completed turn text.
 */
export interface SpawnAgentResult {
  runId: string;
  providerSessionId: string;
  providerKey: string;
  status: string;
  finalMessage?: string;
}

/**
 * Summary of one agent run in the tree (CP-19 / Task-082).
 */
export interface AgentRunSummary {
  runId: string;
  agentName: string;
  label?: string;
  role: string;
  status: RunStatus;
  parentRunId?: string;
  createdAt: string;
  dependsOn?: string[];
  agentStatus?: string;
  providerKey?: string;
  modelName?: string;
  /** True when spawned with wait=true; such a running child blocks the main run (BUG-133). */
  waitForResult?: boolean;
  /**
   * Incremented each time a reinvoke-lifecycle child is reactivated (completed → running).
   * mergeAgentRunsById uses this to distinguish a genuine reinvoke from a stale HTTP
   * snapshot — allowing the completed→running transition only when activationSeq increases.
   * Zero / absent for first activation and for spawn-lifecycle children (BUG-Rnd2).
   */
  activationSeq?: number;
  /** Vibe task this child was spawned for (BUG-369). */
  vibeTaskIndex?: number;
  vibeTaskTotal?: number;
  vibeTaskName?: string;
}


export interface AgentDependencyEdge {
  fromRunId: string;
  toRunId: string;
  kind: string;
}

export interface AgentBusMessage {
  id: string;
  parentRunId: string;
  fromRunId?: string;
  toRunId?: string;
  kind: string;
  message: string;
  queued: boolean;
  occurredAt: string;
}

export interface AgentLoopState {
  status: string;
  round: number;
  roundCap: number;
  cap?: number;           // flow-engine cap (Task-090); use cap ?? roundCap for display
  gateReason?: string;
  openIssues?: number;
  mode?: string;          // "keyword" | "explicit"
  activeNode?: string;
  extendCount?: number;
  /** Why status=="blocked" (BUG-231): "cap" | "escalate" | "member_stalled" (Task-241). */
  blockReason?: string;
  /** Vibe task progress (BUG-367). 1-based current / total. */
  vibeTaskIndex?: number;
  vibeTaskTotal?: number;
  vibeTaskName?: string;
}

export interface AgentGraphSnapshot {
  parentRunId: string;
  runs: AgentRunSummary[];
  edges: AgentDependencyEdge[];
  busMessages: AgentBusMessage[];
  loopState: AgentLoopState;
}

/**
 * Runtime status of one workflow-defined step, mirroring Go's
 * RuntimeWorkflowStepStatus (workflow_state_machine.go). Source of truth is the
 * Go runner's workflow state machine, not AI inference or timeline text (BUG-153 V-2).
 */
export type WorkflowStepRuntimeStatus =
  | "PENDING"
  | "RUNNING"
  | "WAITING_USER_APPROVAL"
  | "CANCELED"
  | "DONE"
  | "FAILED"
  | "SKIPPED";

/**
 * Client-facing projection of one RuntimeWorkflowStep (BUG-153 F-1), served by
 * `GET /client/workflow-runs/{runId}/steps-runtime`. `rejectionNote` doubles as
 * the display-only retry reason; `retryCount > 0` with status RUNNING/PENDING
 * indicates a retried pass rather than a fresh one.
 */
export interface WorkflowStepRuntimeDTO {
  stepId: string;
  stepType: string;
  status: WorkflowStepRuntimeStatus;
  retryCount: number;
  rejectionNote?: string;
  startedAt?: string;
  finishedAt?: string;
  requiresApproval: boolean;
  behaviorId?: string;
  // nodeId is the flow-graph node id (e.g. "coder", "reviewer_correctness").
  // For CP-42 flow-engine steps, stepType is a shared generic dispatch
  // category (e.g. "flow-agent-delegate") identical across every node
  // running the same behavior — nodeId is the actual per-step name (BUG-155).
  nodeId?: string;
  agentRef?: string;
  provider?: string;
  model?: string;
  yoloMode?: boolean;
}

export interface WorkflowStepsRuntimeSnapshot {
  runId: string;
  steps: WorkflowStepRuntimeDTO[];
  // provider/model/yoloMode are the RUN's own posture (BUG-158) — a built-in
  // flow node has no per-node override of its own (its agent definition
  // inherits the parent run's model/provider), and yolo is a run-wide toggle,
  // not a per-step-type default.
  provider?: string;
  model?: string;
  yoloMode?: boolean;
}

// ---- Flow-engine contract types (CP-36 / Task-095) -------------------------

export interface FlowControlInput {
  signal: "advance" | "complete" | "block" | "extend_cap";
  cap?: number;
  reason?: string;
}

export interface FlowControlResult {
  status: string;
  round: number;
  cap: number;
}

export interface ReviewIssue {
  id: string;
  severity: "error" | "warning" | "info";
  /** Human-readable issue summary. Alias "title" accepted by the Go server. */
  description: string;
  /** File path or code location. Alias "file" accepted by the Go server. */
  location?: string;
}

export interface ReviewOutcomeInput {
  outcome: "approved" | "changes_requested";
  issues?: ReviewIssue[];
  /** Required by the server on the MCP/agent path for changes_requested; optional on the board path. */
  feedback?: string;
}

export interface ReviewOutcomeResult {
  outcome: string;
  issueCount: number;
  loopStatus: string;
}

// ---------------------------------------------------------------------------

export interface ProviderAccountUsageLine {
  label: string;
  remainingPercent: number;
  resetAt: string | null;
}

export interface ProviderAccountSummary {
  id: string;
  providerKey: ProviderKey;
  displayName: string;
  displayLabel: string;
  homePath: string;
  authStorePath: string | null;
  slotIndex: number;
  authStatus: "pending" | "connecting" | "connected" | "failed";
  isActive: boolean;
  createdAt: string;
  lastAuthenticatedAt: string | null;
  accountEmail: string | null;
  accountName: string | null;
  usageSummary: string | null;
  remaining5hPercent: number | null;
  remaining7dPercent: number | null;
  remaining5hResetAt: string | null;
  remaining7dResetAt: string | null;
  usageSource: "provider_api" | "unavailable";
  accessTokenExpiresAt: string | null;
  refreshTokenExpiresAt: string | null;
  refreshTokenExpiryNote: string | null;
  usageDetailLines: ProviderAccountUsageLine[];
}

export interface Artifact {
  id: string;
  runId: string;
  kind: "final_response" | "diff_snapshot" | "summary" | "rag" | "audit";
  name: string;
  /** Short human label / preview; full content is fetched lazily in Part B. */
  preview?: string;
  createdAt: string;
}

// ---- Run lifecycle ---------------------------------------------------------

export type RunStatus =
  | "idle"
  | "starting"
  | "running"
  | "waiting_approval"
  | "waiting_question"
  /** BUG-231: the flow's agent loop paused awaiting the user (escalate or round-cap reached) — distinct from "running" so the composer unlocks and a recovery affordance can render. */
  | "blocked"
  | "completed"
  | "failed"
  | "cancelled";

export interface StartRunInput {
  projectId: string;
  workflowId?: string;
  /** Omitted for normal_chat runs; the runner mints a synthetic chat step. */
  stepId?: string;
  /**
   * Explicit provider for direct chat (the provider selector). When omitted the runner
   * auto-selects: from `model` if given (workflow/step mode), else the first available provider.
   */
  providerKey?: ProviderKey;
  /** Workflow/step model; the runner derives the provider from it when `providerKey` is empty. */
  model?: string;
  yoloMode?: boolean;
  reasoningEffort?: string;
  /** Task-326 wire enum. "dev" | "vibe". Never "normal". Empty → runner defaults to dev. */
  workingMode?: "dev" | "vibe";
  flowRef?: string;
  /** "normal_chat" signals provider-chat mode; the runner tags the run as chat and mints a synthetic step. */
  chatMode?: string;
  /** Active project binding path used as the provider working directory. */
  cwd?: string;
  /** CP-59 chat SSOT: attach this run to an existing chat (switch/reattach). */
  chatId?: string;
  switchFromRunId?: string;
  legSeq?: number;
}

export interface RunHandle {
  runId: string;
  providerSessionId: string; // for Codex this equals the thread id
  providerKey: ProviderKey;
  status: RunStatus;
  stepId?: string;
  /**
   * Seq of the last persisted event at resume time. History replay starts at seq 0 and
   * stops here so a multi-turn run is replayed in full instead of truncating at the first
   * turn_completed. Undefined when the runner predates this field. (BUG-112)
   */
  lastEventSeq?: number;
  /** CP-59 chat SSOT: the logical chat this run belongs to + its leg ordinal. */
  chatId?: string;
  legSeq?: number;
}

export interface RunHistoryItem {
  runId: string;
  projectId: string;
  workflowId?: string;
  providerKey: ProviderKey;
  status: RunStatus;
  startedAt: string;
  updatedAt: string;
  lastPrompt?: string;
  lastMessage?: string;
  /** "chat" for normal_chat runs; undefined for workflow/step runs. */
  runKind?: string;
  /** CP-59 chat SSOT: chat grouping for the navigator. */
  chatId?: string;
  legSeq?: number;
  sourceMachineId?: string;
  sourceRunId?: string;
  syncStatus?: string;
  unavailableReason?: string;
  parentRunId?: string;
  agentName?: string;
  role?: string;
  agentStatus?: string;
  /**
   * Chat-Mode orchestration picker selection this run was started with
   * (CP-42/Task-177), e.g. subMode="bug", flowRef="flowpilot-core-flow-pack/review-loop"
   * (BUG-263). Undefined for a plain chat run or a Flow-Mode workflow launch.
   */
  subMode?: string;
  flowRef?: string;
}

export interface ChatSessionSyncRequest {
  googleDriveProjectId?: string;
  googleDriveFolderId?: string;
}

export interface ChatSessionSyncResult {
  runId: string;
  sourceMachineId: string;
  sourceRunId: string;
  syncStatus: string;
  syncedAt: string;
  remotePath: string;
}

export interface RemoteChatSessionSummary {
  runId: string;
  projectId: string;
  workflowId?: string;
  providerKey: ProviderKey;
  status?: string;
  runKind?: string;
  sourceMachineId: string;
  sourceRunId: string;
  lastPrompt?: string;
  lastMessage?: string;
  startedAt?: string;
  updatedAt?: string;
  syncedAt?: string;
  unavailableReason?: string;
}

export interface ChatSessionRestoreRequest {
  projectId: string;
  sourceMachineId: string;
  sourceRunId: string;
  cwd?: string;
}

export interface ChatSessionRestoreResult {
  runId: string;
  sourceMachineId: string;
  sourceRunId: string;
  providerKey: ProviderKey;
  restoreStatus: string;
}

export interface HandoffContextRequest {
  targetProviderKey: ProviderKey;
  maxBytes?: number;
}

export interface HandoffContextResponse {
  sourceRunId: string;
  sourceProviderKey: ProviderKey;
  targetProviderKey: ProviderKey;
  prompt: string;
  includedTurnCount: number;
  omittedTurnCount: number;
  truncated: boolean;
  handoffMode: "raw" | "hybrid" | "target_summary";
}

export interface ChatSummaryResult {
  runId: string;
  generated: boolean;
  skipped: boolean;
  reason?: string;
}

export interface SkillSelection {
  name: string;
  path?: string;
  source: "slash_picker" | "text_shortcut" | "workflow_default";
}

/**
 * An image attached to a chat turn (Task-052). Filled by the renderer AFTER
 * normalization (downscale + recompress); `data` is the base64 of the normalized
 * bytes (no `data:` prefix) and is carried inline in the turn payload (V1 — D-2).
 * A desktop-only local path/preview URL is NOT part of this wire shape: the runner
 * is a separate process and a renderer path is not guaranteed readable there.
 */
export interface PromptAttachment {
  id: string;
  kind: "image";
  originalName: string;
  /** Encoded MIME after normalization: image/webp | image/jpeg | image/png | image/gif. */
  mimeType: string;
  /** Base64 of the normalized bytes (no `data:...;base64,` prefix). */
  data: string;
  sizeBytes: number;
  width?: number;
  height?: number;
}

export interface TurnInput {
  runId: string;
  stepId: string;
  prompt: string;
  /** Declared chat-start intent for flow-gate task/bug handling (Task-114). */
  changeType?: "task" | "bugfix";
  /** Optional tracked document id declared alongside changeType, e.g. Task-114 or BUG-141. */
  sourceDocId?: string;
  /** One or more skills attached to this turn (via the `/` picker). */
  selectedSkills?: SkillSelection[];
  /**
   * Per-turn chat overrides (BUG-063). In chat mode the composer resends the current
   * control values on every turn so model, reasoning, and YOLO can be changed between
   * prompts — the providers re-apply them per turn. Omitted in workflow/step mode, where
   * the run-level values captured at startRun are used.
   */
  reasoningEffort?: string;
  model?: string;
  yoloMode?: boolean;
  /**
   * Chat posture (scan/plan/code) — OpenCode-style mode switching (Task-xxx/
   * CA-xxx). Omitted in workflow/step mode. Scan/Plan are read-only: the runner
   * auto-approves reads and auto-denies writes without asking the user. The
   * composer resends the current posture on every chat turn, like model/YOLO.
   */
  chatPosture?: ChatPosture;
  /**
   * Image attachments for this chat turn (Task-052). Carried inline as base64.
   * Omitted in workflow/step mode and when no images are attached. Supported only
   * by vision-capable providers; the composer gates the attach control accordingly.
   */
  attachments?: PromptAttachment[];
  /**
   * Built-in Chat Mode orchestration selection (CP-42/Task-177). subMode is the
   * runner's sub-mode key (currently "bug" for the Bug chat intent); flowRef
   * selects a built-in flow such as "flowpilot-core-flow-pack/review-loop".
   * Both are sent only on the first turn of a run, alongside changeType/
   * sourceDocId, and are optional — omitting them is normal chat with no
   * orchestration. The runner validates flowRef against the sub-mode's
   * built-in options and rejects an invalid pairing before starting the turn.
   */
  subMode?: string;
  flowRef?: string;
}

/** One selectable built-in orchestration flow for a given chat subMode
 * (CP-42/Task-177), as served by GET /client/chat/builtin-orchestration-options. */
export interface BuiltinFlowOption {
  flowRef: string;
  label: string;
  description: string;
}

/**
 * Chat posture (Task-xxx / CA-xxx): OpenCode-style Scan/Plan/Code mode
 * switching. Each posture has an optional pinned profile (provider/model/
 * reasoning/yolo) that the client applies when the posture is activated; empty
 * fields inherit the current session selection. The document is owned by the
 * runner (SSOT, shared file) and both Desktop + TUI reach it ONLY through the
 * runner's GET/PUT /client/chat-posture endpoints.
 */
export type ChatPosture = "scan" | "plan" | "code" | "non";

export interface ChatPostureProfile {
  provider?: string;
  model?: string;
  reasoningEffort?: string;
  yolo?: boolean;
}

export interface ChatPostureConfig {
  active: ChatPosture;
  // CA-685: partial by design — the runner omits empty profiles and the "non"
  // posture has none, so every key access must tolerate a missing entry.
  profiles: Partial<Record<ChatPosture, ChatPostureProfile>>;
}

/** Posture labels/descriptions for the composer tabs + setup modal. */
export const CHAT_POSTURES: { key: ChatPosture; label: string; hint: string }[] = [
  { key: "scan", label: "Scan", hint: "Read-only exploration — reads auto-approve, writes auto-deny." },
  { key: "plan", label: "Plan", hint: "Read-only planning — reads auto-approve, writes auto-deny." },
  { key: "code", label: "Code", hint: "Normal approvals / YOLO — full tool access." },
  { key: "non", label: "Non", hint: "No posture — keeps your last model choice across restarts." },
];

// ---- ProviderEventDTO (serialized ProviderEvent union) ---------------------

export interface ProviderEventBaseDTO {
  id: string;
  workflowRunId: string;
  workflowStepRunId?: string;
  providerSessionId: string;
  providerKey: ProviderKey;
  providerTurnId?: string;
  /** Monotonic per-run sequence — the reconnect/replay cursor (04-02). */
  seq: number;
  occurredAt: string;
}

export interface TokenUsageBreakdown {
  cachedInputTokens: number;
  inputTokens: number;
  outputTokens: number;
  reasoningOutputTokens: number;
  totalTokens: number;
}

export interface TokenUsageSnapshot {
  last?: TokenUsageBreakdown;
  total?: TokenUsageBreakdown;
  modelContextWindow?: number | null;
}

export type ProviderEventDTO =
  | (ProviderEventBaseDTO & { type: "turn_started"; providerTurnId: string; prompt?: string })
  | (ProviderEventBaseDTO & { type: "message_delta"; text: string })
  | (ProviderEventBaseDTO & { type: "message_completed"; text: string })
  | (ProviderEventBaseDTO & { type: "token_usage_updated"; tokenUsage: TokenUsageSnapshot })
  | (ProviderEventBaseDTO & { type: "tool_started"; toolName: string; input?: unknown })
  | (ProviderEventBaseDTO & {
      type: "tool_completed";
      toolName: string;
      output?: unknown;
      status: "success" | "failed" | "cancelled";
    })
  | (ProviderEventBaseDTO & {
      type: "file_changed";
      path: string;
      changeType?: "created" | "modified" | "deleted" | "renamed";
    })
  | (ProviderEventBaseDTO & {
      type: "permission_required";
      approvalId: string;
      provider: ProviderKey;
      details: ApprovalDetails;
      /** Populated only when replaying an already-resolved approval on a full
       *  server restart (BUG-ApprovalReplay-Restart) — render the approval card
       *  read-only with this decision instead of a fresh interactive prompt.
       *  The approval-side twin of user_question_required.answer. */
      decision?: string;
    })
  | (ProviderEventBaseDTO & {
      type: "user_question_required";
      questionId: string;
      prompt: string;
      options: QuestionOption[];
      multiSelect?: boolean;
      /** Populated only when replaying an already-resolved question on
       *  reconnect (BUG-StaleQuestion) — render read-only/answered instead of
       *  a fresh interactive form. */
      answer?: string | string[];
    })
  | (ProviderEventBaseDTO & {
      /** CP-62 P-3 (Task-339/345): structured escalation card from the
       *  request_user_decision tool. The prose card stays the fallback
       *  (Q-1). Answered by sending the chosen option id as the next
       *  prompt — the runner matches it back to the parked card. */
      type: "user_decision_card_requested";
      /** Runner ProviderEvent.Input — the UserDecisionCard payload. */
      input: DecisionCardDTO;
    })
  | (ProviderEventBaseDTO & { type: "turn_failed"; error: string; recoverable: boolean })
  | (ProviderEventBaseDTO & { type: "turn_completed"; finalMessage: string })
  | (ProviderEventBaseDTO & { type: "agent_graph_updated"; agentGraphSnapshot: AgentGraphSnapshot })
  | (ProviderEventBaseDTO & { type: "agent_bus_message"; agentBusMessage: AgentBusMessage })
  | (ProviderEventBaseDTO & { type: "agent_spawned_by_user"; agentName: string; childRunId: string })
  | (ProviderEventBaseDTO & { type: "agent_result_injected"; agentName: string; childRunId: string; finalMessage: string })
  | (ProviderEventBaseDTO & {
      type: "flow_gate_violation";
      error: string;
      status?: string;
      /** Decision options for the r-reg block card (Task-155). Present only on regression blocks. */
      gateOptions?: string[];
      /** Specifically-identified regressed test names (Task-155). May be ["suite_regressed"]. */
      gateRegressedTests?: string[];
    })
  | (ProviderEventBaseDTO & {
      /** BUG-243 F-3: rag-harness's Audit step draft, produced by the real
       *  BuildAuditDraft (Task-171) once validate/audit are wired into the
       *  live dispatch path (F-0/F-1/F-2). Never implies a write/commit
       *  happened — this is strictly an inspectable draft. */
      type: "flow_audit_draft";
      flowAuditDraft: FlowAuditDraftDTO;
    });

/** Mirrors the Go FlowAuditDraft struct (flow_audit_draft.go) field-for-field. */
export interface FlowAuditDraftDTO {
  workflowRunId: string;
  planStepId?: string;
  codingStepId?: string;
  testingStepId?: string;
  auditStepId?: string;
  originalPackageId?: string;
  featureKey: string;
  sourceDocId?: string;
  changeType?: string;
  summary?: string;
  whatChanged?: string;
  whyChanged?: string;
  changedFiles?: string[];
  validationCommands?: string[];
  validationResult: string;
  residualNotes?: string;
  commitMessage?: string;
  changeLedgerBlock?: string;
  /** "ready" | "blocked_missing_feature_key" | "blocked_validation_failed" */
  status: string;
}

export type ProviderEventType = ProviderEventDTO["type"];

export interface ApprovalDetails {
  /** e.g. the command the runtime wants to run, or the file write target. */
  command?: string;
  cwd?: string;
  reason?: string;
  /** Classifies the approval. Only "exec" (a shell command) is eligible for the
   *  per-project "don't ask again" allowlist (BUG-246). */
  kind?: "exec" | "file" | "mcp" | "other";
  /** Decisions the runtime offers (e.g. approve / deny / approve_for_session). */
  decisions: { value: string; label: string }[];
}

export interface QuestionOption {
  label: string;
  description?: string;
  /** Value submitted back; defaults to label when omitted. */
  value?: string;
}

// ---- CP-62 P-3 (Task-339/345): structured escalation card --------------------

export interface DecisionCardOptionDTO {
  id: string;
  label: string;
  /** What happens when this option is chosen — a non-tech user decides from consequences. */
  consequence: string;
}

export interface DecisionCardEvidenceDTO {
  path: string;
  line?: number;
  excerpt?: string;
}

/** Payload of the runner's user_decision_card_requested event (UserDecisionCard). */
export interface DecisionCardDTO {
  question: string;
  detail?: string;
  /** Option id the runner recommends; highlighted on the card. */
  recommended?: string;
  options: DecisionCardOptionDTO[];
  evidence?: DecisionCardEvidenceDTO[];
}

// ---- CP-51 / SS-17 dispatch operator surface --------------------------------

export interface DispatchAttentionItem {
  kind: "uncertain" | "repair_required" | string;
  runId: string;
  turnId?: string;
  reason?: string;
  updatedAt?: string;
}

export type DispatchResolveAction =
  | "mark_completed"
  | "mark_failed"
  | "confirm_cancelled"
  | "abandon";

export interface DispatchSettlementDisposition {
  state: string;
  settlePhase: string;
  stopOutcome?: string;
}

export interface DispatchInspectResult extends DispatchSettlementDisposition {
  runId: string;
  turnId: string;
  revision: number;
  cancelRequested: boolean;
  envelopeHash: string;
  outerIntentKey: string;
  outerIntentGen: number;
  intentOwnerRunID: string;
  receiptEvidence?: { providerKey: string; receiptId: string; evidenceKind: string; payloadSHA256: string };
  terminalEvidence?: { providerKey: string; evidenceKind: string; outcome: string; payloadSHA256: string };
  openRepair?: { repairRevision: number; reason: string; quarantineHash: string; state: string; createdAt?: string };
}

// ---- The contract ----------------------------------------------------------

export interface RunnerClient {
  listProjects(): Promise<Project[]>;
  listWorkflows(): Promise<Workflow[]>;
  listSteps(): Promise<Step[]>;
  listProviderAccounts(): Promise<ProviderAccountSummary[]>;
  listRunHistory(projectId: string): Promise<RunHistoryItem[]>;
  listRemoteChatSessions(projectId: string): Promise<RemoteChatSessionSummary[]>;
  startRun(input: StartRunInput): Promise<RunHandle>;
  resumeRun(runId: string): Promise<RunHandle>;
  syncChatRun(runId: string, input?: ChatSessionSyncRequest): Promise<ChatSessionSyncResult>;
  deleteRun(runId: string): Promise<void>;
  restoreChatRun(input: ChatSessionRestoreRequest): Promise<ChatSessionRestoreResult>;
  handoffContext(runId: string, input: HandoffContextRequest): Promise<HandoffContextResponse>;
  /** CP-59 Task-314: chat-scoped cross-provider switch (runner mints the new leg). */
  switchChatProvider(chatId: string, input: ChatSwitchInput): Promise<ChatSwitchResponse>;
  /** CP-59 Task-313: joined multi-leg chat timeline. */
  chatTimeline(chatId: string, afterSeq?: number, limit?: number): Promise<ChatTimelineResponse>;
  generateChatSummary(runId: string): Promise<ChatSummaryResult>;
  /** Streaming turn: yields normalized provider events until terminal. */
  sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO>;
  submitApproval(approvalId: string, decision: string, remember?: boolean): Promise<void>;
  answerQuestion(questionId: string, choice: string | string[]): Promise<void>;
  /** Stop the in-flight turn (POST /client/workflow-runs/{runId}/interrupt). */
  interrupt(runId: string): Promise<void>;
  /** Attach to a run's event stream and replay from afterSeq — used on reconnect. */
  streamRun(runId: string, afterSeq?: number, signal?: AbortSignal): AsyncIterable<ProviderEventDTO>;
  listArtifacts(runId: string): Promise<Artifact[]>;
  listSkills(provider: string, cwd?: string): Promise<ProviderSkill[]>;
  /** Workspace paths for the @file picker. Optional so older mocks stay valid. */
  listWorkspaceFiles?(cwd: string, query?: string): Promise<string[]>;
  /**
   * List built-in Chat Mode orchestration flow options for subMode (CP-42/
   * Task-177), e.g. "Review Loop" for subMode="bug". Optional so existing
   * clients (mock) need not implement it until a real backend is present.
   */
  listBuiltinOrchestrationOptions?(subMode: string): Promise<BuiltinFlowOption[]>;
  /**
   * List loadable sub-agent definitions for the spawn picker (CP-19 / Task-081).
   * Optional so existing clients (mock) need not implement it until the Agents
   * UI lands (Task-083); the real HTTP client implements it now.
   */
  listAgents?(cwd?: string): Promise<AgentDefinition[]>;
  /**
   * List child agent run summaries for a parent run (CP-19 / Task-082).
   * Optional until the Agents panel lands (Task-083).
   */
  listAgentRuns?(parentRunId: string): Promise<AgentRunSummary[]>;
  refreshAgentGraph?(parentRunId: string): Promise<AgentGraphSnapshot>;
  /**
   * Load the Flow-mode workflow-step runtime projection (BUG-153 F-2). Optional
   * so mock/older clients degrade gracefully; the real HTTP client implements it.
   */
  getWorkflowStepsRuntime?(runId: string): Promise<WorkflowStepsRuntimeSnapshot>;
  pauseAgentLoop?(parentRunId: string): Promise<AgentGraphSnapshot>;
  resumeAgentLoop?(parentRunId: string): Promise<AgentGraphSnapshot>;
  injectAgentFeedback?(parentRunId: string, toRunId: string, message: string): Promise<AgentGraphSnapshot>;
  stopAgentLoop?(parentRunId: string): Promise<AgentGraphSnapshot>;
  /**
   * Programmatically spawn a child agent run (CP-19 / Task-082).
   * Optional until the Agents panel lands (Task-083).
   */
  spawnAgent?(input: SpawnAgentInput & { parentRunId: string }): Promise<SpawnAgentResult>;
  /**
   * Attach to a child run by switching the active stream to that run and returning
   * its SSE iterator. Implemented client-side on top of `streamRun` in Phase 1.
   */
  focusAgentRun?(runId: string, signal?: AbortSignal): AsyncIterable<ProviderEventDTO>;
  /**
   * Submit a reviewer verdict (approved / changes_requested) to the flow engine
   * (CP-36 / Task-095). Calls POST .../flow-control under the hood.
   */
  submitReviewOutcome?(parentRunId: string, input: ReviewOutcomeInput): Promise<AgentGraphSnapshot>;
  /**
   * Extend the round cap by 2 on a blocked loop (CP-36 / Task-095).
   * Calls POST .../agent-loop/extend-cap.
   */
  extendCap?(parentRunId: string): Promise<AgentGraphSnapshot>;
  /**
   * Resume a blocked/awaiting-user loop (BUG-231's "Continue" action): injects
   * optional user feedback, auto-raises the cap only when the block reason was
   * the round cap, and re-invokes the hub's synthesis turn so the hub itself
   * re-decides the route. Calls POST .../agent-loop/continue.
   */
  continueFlow?(
    parentRunId: string,
    feedback: string,
    memberAction?: { action: "retry" | "skip"; node?: string },
  ): Promise<AgentGraphSnapshot>;
  /**
   * SS-17 / CP-51 Task-256: list dispatch attention items (uncertain + repair_required)
   * for ONE run — the runner's endpoint is per-run (GET .../workflow-runs/{runId}/
   * dispatch-attention), never a global root listing. Optional so mock/older
   * runners degrade gracefully.
   */
  listDispatchAttention?(runId: string): Promise<DispatchAttentionItem[]>;
  /**
   * Inspect one turn's full dispatch record (states/revision/envelope summary/
   * receipt-evidence canonical hash/open-repair metadata) — never a raw
   * secret-bearing payload.
   */
  inspectDispatch?(runId: string, turnId: string): Promise<DispatchInspectResult>;
  /** Audit trail for one turn (who/what/when, embedded in the store commits). */
  getDispatchAudit?(runId: string, turnId: string): Promise<{ entries: unknown[] }>;
  /**
   * Resolve an uncertain dispatch record (atomic store op). Response carries the
   * ALREADY-COMMITTED settlement disposition — never call a second endpoint to
   * learn it. expectedRev should come from a prior inspect; when unknown, pass 0
   * and let the runner reject stale.
   */
  resolveDispatchUncertain?(
    runId: string,
    turnId: string,
    input: { expectedRev: number; resolutionId: string; action: DispatchResolveAction; detail?: string },
  ): Promise<DispatchSettlementDisposition>;
  /**
   * Retry-as-new over a superseded/held record. A 409 with a "superseded"
   * signal means the run got a newer prompt since this record was held (SS-17
   * §8) — the card must offer abandon instead of retrying again.
   */
  retryDispatchAsNew?(
    runId: string,
    turnId: string,
    input: { expectedRev: number; resolutionId: string; newTurnId?: string; expectedIntentGen: number; expectedEnvelopeHash: string },
  ): Promise<DispatchSettlementDisposition & { newTurnId: string }>;
  /**
   * The single defined two-phase repair-resolution flow (SD-24 §6.7) — one
   * call, not a client-driven begin/commit split. `retry_load` re-validates the
   * quarantined raw against the run's persisted session server-side.
   */
  resolveDispatchRepair?(
    runId: string,
    input: { expectedRepairRev: number; resolutionId: string; action: "retry_load" | "abandon" },
  ): Promise<{ revision: number; outcome: "resolved_retry_load" | "failed_still_open" | "resolved_abandon"; detail: string }>;
  connectProviderAccount(providerKey: ProviderKey): Promise<void>;
  activateProviderAccount(accountId: string): Promise<void>;
  /**
   * Grok-only YOLO enforcement (Task-218). Unlike Claude/Codex, where YOLO is
   * just a CLI flag re-derived on the next turn, Grok's YOLO=false direction
   * requires the backend to rewrite the active account's own config.toml and
   * respawn its shared process for gating to actually take effect -- so this
   * call is awaited (with a loading modal) instead of a synchronous local
   * state flip. See store.ts's toggleYoloForActiveProvider.
   */
  applyGrokYoloPosture(yolo: boolean): Promise<void>;
  /**
   * Chat posture document (Task-xxx/CA-xxx): GET /client/chat-posture.
   * The runner owns the shared file (SSOT); the client never touches it directly.
   * Optional so mock/older clients degrade gracefully.
   */
  getChatPosture?(): Promise<ChatPostureConfig>;
  /**
   * CA-689c: the model's real reasoning effort options (live ACP probe on the
   * runner, cached runner-side). Opencode-only today — the models CLI cannot
   * report per-model variants. Optional so mock/older clients degrade.
   */
  getOpencodeModelVariants?(modelId: string): Promise<{ supportedEfforts: string[]; defaultReasoningEffort: string }>;
  /** PUT /client/chat-posture — persists the active posture + profile pins. */
  setChatPosture?(config: ChatPostureConfig): Promise<ChatPostureConfig>;
  openProviderAccountTerminal(accountId: string): Promise<void>;
  /** System control — mirrors admin-web's runner gateway (`POST /system/restart`). */
  restartStack(): Promise<void>;
  /** System control — mirrors admin-web's runner gateway (`POST /system/shutdown`). */
  shutdownStack(): Promise<void>;
  /**
   * Submit a user decision for the r-reg gate block card (Task-155).
   * option: "keep-test-fix-code" | "suggest-requirement-change" | "custom"
   * customText: required when option === "custom".
   */
  submitGateDecision?(runId: string, option: string, customText?: string): Promise<void>;
  /**
   * Task-309: widen the frozen contract for a scope-drift block and resume.
   * Calls POST /client/workflow-runs/{runId}/agent-loop/amend {"paths":[...] }.
   */
  amendFlow?(runId: string, paths: string[]): Promise<AgentGraphSnapshot>;
  /**
   * Record explicit user agreement to the AI's opt-2 requirement proposal (Task-155).
   * testNames: the test names to unlock (written as overrides).
   */
  submitGateAgreement?(runId: string, testNames: string[]): Promise<void>;
  /** Dev-only fake-adapter scenario hint (mock + P2 fake adapter); real runtimes ignore it. */
  setScenario?(scenario: string): void;
}

// ---- IdeBridge (Part A stub) -----------------------------------------------

export interface IdeBridge {
  openInIde(file: string, line?: number): Promise<void>;
  /** Open a URL in the user's default browser (e.g. the admin-web app). */
  openExternal(url: string): Promise<void>;
}


// ---- CP-59 chat SSOT (Task-316) -------------------------------------------

/** Mirrors the runner's handoff marker — parity pinned in the TUI runner suite. */
export const HANDOFF_PROMPT_PREFIX = "[FlowPilot cross-provider chat handoff]";

export interface ChatSwitchInput {
  targetProviderKey: ProviderKey;
  model?: string;
  reasoningEffort?: string;
  yoloMode?: boolean;
}

export interface ChatSwitchHandoffStats {
  handoffMode: "raw" | "hybrid" | "target_summary" | "fresh_start";
  includedTurnCount: number;
  omittedTurnCount: number;
  truncated: boolean;
  actionsDigestIncluded: boolean;
}

export interface ChatSwitchResponse {
  handle: RunHandle;
  chatId: string;
  legSeq: number;
  model: string;
  handoff: ChatSwitchHandoffStats;
}

export interface ChatTimelineLeg {
  runId: string;
  providerKey: string;
  legSeq: number;
  legState: string;
  legClosedReason?: string;
  status?: string;
}

export interface ChatTranscriptRecord {
  chatId: string;
  chatSeq: number;
  legRunId: string;
  type: string;
  payload: unknown;
}

export interface ChatTimelineResponse {
  chatId: string;
  legs: ChatTimelineLeg[];
  records: ChatTranscriptRecord[];
  nextSeq: number;
  truncated: boolean;
  degraded: boolean;
}

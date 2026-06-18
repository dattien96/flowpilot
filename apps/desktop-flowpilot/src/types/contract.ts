// ============================================================================
// The client<->runner contract (04-01 "The contract (key output)").
//
// This is the ONE artifact Phase 1 must lock. Both MockRunnerClient (Part A) and
// HttpWsRunnerClient (Part B) implement `RunnerClient` against these types; the
// renderer only ever talks to `RunnerClient`. The `ProviderEventDTO` union is the
// serialized form of the shared `ProviderEvent` union in 04-Detailed-Coding-Plan.md.
// Keep this in sync with the Go runner's event DTOs.
// ============================================================================

export type ProviderKey = "codex" | "claude" | "gemini";

// ---- Domain (navigator) ----------------------------------------------------

export interface Project {
  id: string;
  name: string;
  /** Workspace path bound as the provider `cwd` (display only in Part A). */
  path: string;
}

export interface Workflow {
  id: string;
  projectId: string;
  name: string;
  description?: string;
}

export interface Step {
  id: string;
  workflowId?: string;
  name: string;
  order: number;
  /** Skill auto-selected for this step, if any (source: workflow_default). */
  defaultSkill?: string;
}

export interface ProviderSkill {
  name: string;
  path?: string;
  description?: string;
  source: "provider" | "flowpilot" | "workspace";
}

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
  /** YOLO is the single source of truth for approval posture (see 04-04). */
  yoloMode?: boolean;
  reasoningEffort?: string;
  /** "normal_chat" signals provider-chat mode; the runner tags the run as chat and mints a synthetic step. */
  chatMode?: string;
  /** Active project binding path used as the provider working directory. */
  cwd?: string;
}

export interface RunHandle {
  runId: string;
  providerSessionId: string; // for Codex this equals the thread id
  providerKey: ProviderKey;
  status: RunStatus;
  stepId?: string;
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
  sourceMachineId?: string;
  sourceRunId?: string;
  syncStatus?: string;
  unavailableReason?: string;
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
   * Image attachments for this chat turn (Task-052). Carried inline as base64.
   * Omitted in workflow/step mode and when no images are attached. Supported only
   * by vision-capable providers; the composer gates the attach control accordingly.
   */
  attachments?: PromptAttachment[];
}

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
    })
  | (ProviderEventBaseDTO & {
      type: "user_question_required";
      questionId: string;
      prompt: string;
      options: QuestionOption[];
      multiSelect?: boolean;
    })
  | (ProviderEventBaseDTO & { type: "turn_failed"; error: string; recoverable: boolean })
  | (ProviderEventBaseDTO & { type: "turn_completed"; finalMessage: string });

export type ProviderEventType = ProviderEventDTO["type"];

export interface ApprovalDetails {
  /** e.g. the command the runtime wants to run, or the file write target. */
  command?: string;
  cwd?: string;
  reason?: string;
  /** Decisions the runtime offers (e.g. approve / deny / approve_for_session). */
  decisions: { value: string; label: string }[];
}

export interface QuestionOption {
  label: string;
  description?: string;
  /** Value submitted back; defaults to label when omitted. */
  value?: string;
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
  /** Streaming turn: yields normalized provider events until terminal. */
  sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO>;
  submitApproval(approvalId: string, decision: string): Promise<void>;
  answerQuestion(questionId: string, choice: string | string[]): Promise<void>;
  /** Stop the in-flight turn (POST /client/workflow-runs/{runId}/interrupt). */
  interrupt(runId: string): Promise<void>;
  /** Attach to a run's event stream and replay from afterSeq — used on reconnect. */
  streamRun(runId: string, afterSeq?: number): AsyncIterable<ProviderEventDTO>;
  listArtifacts(runId: string): Promise<Artifact[]>;
  listSkills(provider: string, cwd?: string): Promise<ProviderSkill[]>;
  connectProviderAccount(providerKey: ProviderKey): Promise<void>;
  activateProviderAccount(accountId: string): Promise<void>;
  openProviderAccountTerminal(accountId: string): Promise<void>;
  /** System control — mirrors admin-web's runner gateway (`POST /system/restart`). */
  restartStack(): Promise<void>;
  /** System control — mirrors admin-web's runner gateway (`POST /system/shutdown`). */
  shutdownStack(): Promise<void>;
  /** Dev-only fake-adapter scenario hint (mock + P2 fake adapter); real runtimes ignore it. */
  setScenario?(scenario: string): void;
}

// ---- IdeBridge (Part A stub) -----------------------------------------------

export interface IdeBridge {
  openInIde(file: string, line?: number): Promise<void>;
  /** Open a URL in the user's default browser (e.g. the admin-web app). */
  openExternal(url: string): Promise<void>;
}

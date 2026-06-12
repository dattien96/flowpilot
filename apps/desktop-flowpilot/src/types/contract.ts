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
  workflowId: string;
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
  workflowId: string;
  stepId: string;
  /** YOLO is the single source of truth for approval posture (see 04-04). */
  yoloMode?: boolean;
}

export interface RunHandle {
  runId: string;
  providerSessionId: string; // for Codex this equals the thread id
  providerKey: ProviderKey;
  status: RunStatus;
}

export interface SkillSelection {
  name: string;
  path?: string;
  source: "slash_picker" | "text_shortcut" | "workflow_default";
}

export interface TurnInput {
  runId: string;
  stepId: string;
  prompt: string;
  /** One or more skills attached to this turn (via the `/` picker). */
  selectedSkills?: SkillSelection[];
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

export type ProviderEventDTO =
  | (ProviderEventBaseDTO & { type: "turn_started"; providerTurnId: string })
  | (ProviderEventBaseDTO & { type: "message_delta"; text: string })
  | (ProviderEventBaseDTO & { type: "message_completed"; text: string })
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
  listSteps(workflowId: string): Promise<Step[]>;
  startRun(input: StartRunInput): Promise<RunHandle>;
  resumeRun(runId: string): Promise<RunHandle>;
  /** Streaming turn: yields normalized provider events until terminal. */
  sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO>;
  submitApproval(approvalId: string, decision: string): Promise<void>;
  answerQuestion(questionId: string, choice: string | string[]): Promise<void>;
  /** Stop the in-flight turn (POST /client/workflow-runs/{runId}/interrupt). */
  interrupt(runId: string): Promise<void>;
  /** Attach to a run's event stream and replay from afterSeq — used on reconnect. */
  streamRun(runId: string, afterSeq?: number): AsyncIterable<ProviderEventDTO>;
  listArtifacts(runId: string): Promise<Artifact[]>;
  listSkills(provider: string): Promise<ProviderSkill[]>;
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

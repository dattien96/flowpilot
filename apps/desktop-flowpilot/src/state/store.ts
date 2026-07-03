import { create } from "zustand";
import type {
  Artifact,
  AgentDefinition,
  AgentGraphSnapshot,
  AgentRunSummary,
  BuiltinFlowOption,
  ChatSessionRestoreRequest,
  Project,
  ProviderAccountSummary,
  ProviderEventDTO,
  AgentBusMessage,
  ProviderKey,
  ProviderSkill,
  PromptAttachment,
  RemoteChatSessionSummary,
  RunHistoryItem,
  RunStatus,
  Step,
  TokenUsageSnapshot,
  TurnInput,
  Workflow,
  WorkflowStepRuntimeDTO,
} from "@/types/contract";
import type { RunnerClient } from "@/types/contract";
import type { SupportedModel } from "@flowpilot/client-core";
import type { LocalRunnerProvider } from "@flowpilot/client-core";
import { createRunnerClient } from "@/client/createRunnerClient";
import { RunnerApiError } from "@/client/HttpWsRunnerClient";
import type { ScenarioName } from "@/client/mockData";
import { ideBridge } from "@/client/ideBridge";
import { getAdminUseCases } from "@/clientCore";
import { ADMIN_WEB_URL } from "@/config";
import {
  mapNavigatorStep,
  mapNavigatorWorkflow,
} from "@/app/navigatorCatalog";
import {
  applyTimelineEvent,
  shouldApplyRunEvent,
  type PendingApproval,
  type PendingQuestion,
  type TimelineItem,
} from "./timelineReducer";

export type { TimelineItem } from "./timelineReducer";

const LAST_PROJECT_KEY = "fp:lastProjectId";

let loadProjectsInFlight: Promise<void> | null = null;
let activeHistoryReplayController: AbortController | undefined;
let activeOrchestrationStreamController: AbortController | undefined;
let activeAgentFocusStreamController: AbortController | undefined;

export type LaunchMode = "workflow" | "step";
export type ChatMode = "normal_chat" | "workflow_step_auto";
export type WorkspaceMainView = "chat" | "board";
export type ChatStartMode = "normal" | "task" | "bugfix";

interface RunSnapshot {
  timeline: TimelineItem[];
  artifacts: Artifact[];
  status: RunStatus;
  pendingApprovals: PendingApproval[];
  pendingQuestions: PendingQuestion[];
  latestTokenUsage?: TokenUsageSnapshot;
  lastTurnInput?: TurnInput;
  recoverable: boolean;
  _streamingAssistantId?: string;
  activeStepId?: string;
  lastEventSeq?: number;
}

function applyAgentGraphSnapshot(snapshot: AgentGraphSnapshot): Partial<AppState> {
  return {
    agentRuns: snapshot.runs,
    agentGraphSnapshot: snapshot,
    agentBusMessages: snapshot.busMessages,
  };
}

function pickDefaultModel(provider: ProviderKey | undefined, models: SupportedModel[]): string | undefined {
  if (!provider) return undefined;
  const enabled = models.filter((m) => m.providerKey === provider && m.isEnabled);
  if (provider === "codex") {
    return (
      enabled.find((m) => m.modelId.toLowerCase().includes("5.5"))?.modelId ??
      enabled.find((m) => m.modelId.toLowerCase().includes("5.4") && !m.modelId.toLowerCase().includes("mini"))?.modelId ??
      enabled.find((m) => !m.modelId.toLowerCase().includes("mini"))?.modelId ??
      enabled[0]?.modelId
    );
  }
  if (provider === "claude") {
    return enabled.find((m) => m.modelId.toLowerCase().includes("sonnet"))?.modelId;
  }
  return undefined;
}

function selectedProjectPath(state: Pick<AppState, "projects" | "selectedProjectId">): string | undefined {
  return state.projects.find((project) => project.id === state.selectedProjectId)?.path;
}

function cancelHistoryReplayStream(): void {
  activeHistoryReplayController?.abort();
  activeHistoryReplayController = undefined;
}

function cancelOrchestrationStream(): void {
  activeOrchestrationStreamController?.abort();
  activeOrchestrationStreamController = undefined;
}

function cancelAgentFocusStream(): void {
  activeAgentFocusStreamController?.abort();
  activeAgentFocusStreamController = undefined;
}

interface AppState {
  client: RunnerClient;

  // navigator
  projects: Project[];
  workflows: Workflow[];
  steps: Step[];
  skills: ProviderSkill[];
  providerAccounts: ProviderAccountSummary[];
  localProviders: LocalRunnerProvider[];
  supportedModels: SupportedModel[];
  selectedProjectId?: string;
  selectedWorkflowId?: string;
  selectedStepId?: string;
  launchMode: LaunchMode;
  /** Top-level mode: direct provider chat vs workflow/step auto-routing. */
  chatMode: ChatMode;
  /** Provider override. Required in normal_chat; optional (Auto) in workflow_step_auto. */
  selectedProvider?: ProviderKey;
  /** Model selected in normal_chat mode. */
  selectedModel?: string;
  reasoningEffort?: string;
  yoloMode: boolean;
  summaryGenerating: boolean;
  chatStartMode: ChatStartMode;
  chatSourceDocId: string;
  /**
   * Selected built-in orchestration flowRef for the current chat start
   * (CP-42/Task-177), e.g. "flowpilot-core-flow-pack/review-loop". Only
   * meaningful when chatStartMode === "bugfix"; cleared whenever chatStartMode
   * changes to anything else (setChatStartMode) so a stale selection never
   * leaks into an unrelated sub-mode.
   */
  flowRef?: string;
  builtinOrchestrationOptions: BuiltinFlowOption[];

  // run
  runId?: string;
  mainRunId?: string;
  activeAgentRunId?: string;
  workspaceMainView: WorkspaceMainView;
  agentRuns: AgentRunSummary[];
  agentGraphSnapshot?: AgentGraphSnapshot;
  agentBusMessages: AgentBusMessage[];
  /** Flow-mode workflow-step runtime projection (BUG-153), Go orchestrator state only. */
  workflowStepRuntime: WorkflowStepRuntimeDTO[];
  workflowStepRuntimeLoading: boolean;
  /** Run-level provider/model/yolo posture for the current flow (BUG-158) — not per-step. */
  workflowStepRuntimeMeta: { provider?: string; model?: string; yoloMode?: boolean };
  /** Step id for the active run's turns; the synthetic chat step in normal_chat. */
  activeStepId?: string;
  status: RunStatus;
  timeline: TimelineItem[];
  artifacts: Artifact[];
  runHistory: RunHistoryItem[];
  remoteChatSessions: RemoteChatSessionSummary[];
  historyLoading: boolean;
  historyLoadError?: string;
  remoteHistoryLoading: boolean;
  remoteHistoryLoadError?: string;
  pendingApprovals: PendingApproval[];
  pendingQuestions: PendingQuestion[];
  /** A hard-blocking flow-gate violation (e.g. failed tests) the user must acknowledge.
   *  Set only for action === "block"; surfaced as a modal or decision card. (CP-35 / Task-155) */
  gateBlock?: {
    message: string;
    /** Decision card options — present only for r-reg regression blocks (Task-155). */
    options?: string[];
    regressedTests?: string[];
    /** The runId that originated the block, for gate-decision API calls (Task-155). */
    runId?: string;
  };
  /** Run IDs that received a live gate block. Persists across chat switches so Navigator
   *  can suppress the "Running" spinner for a blocked-but-inactive chat whose server
   *  status hasn't settled to idle yet. Cleared per-run when turn_started fires. (CP-35) */
  _gateBlockedRunIds: Record<string, boolean>;
  lastTurnInput?: TurnInput;
  latestTokenUsage?: TokenUsageSnapshot;
  recoverable: boolean;
  scenario: ScenarioName;
  pendingAccountSwitch?: {
    providerKey: ProviderKey;
    failedAccountId: string;
    failedAccountLabel: string;
    candidateAccount: ProviderAccountSummary;
    reason: "usage_limit" | "manual";
  };
  accountSwitchLoading: boolean;
  pendingProviderSwitch?: {
    sourceRunId: string;
    sourceProviderKey: ProviderKey;
    sourceRunStatus: RunStatus;
    targetProviderKey: ProviderKey;
    targetModel?: string;
  };
  providerSwitchLoading: boolean;
  _accountSwitchTriedIds: string[];

  // internal: id of the assistant bubble currently accumulating deltas
  _streamingAssistantId?: string;
  // stale-response guard for loadRunHistory (BUG-060 F-3)
  _historyLoadSeq: number;
  // stale-response guard for loadRemoteChatSessions (mirrors _historyLoadSeq)
  _remoteHistoryLoadSeq: number;
  // stale-response guard for refreshAgentRuns; a fire-and-forget fetch must not
  // overwrite a newer SSE-delivered agent list and flicker the count (BUG-130)
  _agentRunsLoadSeq: number;
  // stale-response guard for refreshWorkflowStepRuntime, same shape as _agentRunsLoadSeq
  _workflowStepRuntimeLoadSeq: number;
  _runSnapshots: Record<string, RunSnapshot>;
  _runReplaySeq: Record<string, number>;
  agentSpawnGuideOpen: boolean;
  agentSpawnGuideAgentName?: string;
  // True while openHistoryRun is replaying a persisted transcript. The replay drives the
  // global status through running→completed just like a live turn, which would otherwise
  // fire the "AI response complete" toast/notification on every chat open. (BUG-118)
  _historyReplaying: boolean;
  // stream generation counter: incremented on every new consumeStream start so that
  // a prior stream for the same runId exits immediately (BUG-079)
  _streamRunSeq: number;
  // separate generation for the live parent orchestration stream
  _orchestrationStreamSeq: number;

  // actions
  loadProjects(): Promise<void>;
  loadProviderAccounts(): Promise<void>;
  loadLocalProviders(): Promise<void>;
  loadSkills(provider: string, cwd?: string): Promise<void>;
  selectProject(projectId: string): Promise<void>;
  setLaunchMode(mode: LaunchMode): void;
  setChatMode(mode: ChatMode): void;
  selectProvider(provider?: ProviderKey): void;
  setSelectedModel(model?: string): void;
  setReasoningEffort(effort?: string): void;
  setYoloMode(yolo: boolean): void;
  generateChatSummary(): Promise<void>;
  setChatStartMode(mode: ChatStartMode): void;
  setChatSourceDocId(sourceDocId: string): void;
  setFlowRef(flowRef: string | undefined): void;
  loadBuiltinOrchestrationOptions(subMode: string): Promise<void>;
  selectWorkflow(workflowId: string): Promise<void>;
  selectStep(stepId: string): void;
  setScenario(scenario: ScenarioName): void;
  sendPrompt(prompt: string, skills?: string[], attachments?: PromptAttachment[]): Promise<void>;
  approve(approvalId: string, decision: string): Promise<void>;
  answer(questionId: string, choice: string | string[]): Promise<void>;
  stop(): Promise<void>;
  reconnect(): Promise<void>;
  loadRunHistory(): Promise<void>;
  loadRemoteChatSessions(): Promise<void>;
  refreshAgentRuns(): Promise<void>;
  refreshWorkflowStepRuntime(): Promise<void>;
  refreshAgentGraph(): Promise<void>;
  pauseAgentLoop(): Promise<void>;
  resumeAgentLoop(): Promise<void>;
  injectAgentFeedback(toRunId: string, message: string): Promise<void>;
  stopAgentLoop(): Promise<void>;
  submitReviewOutcome(outcome: "approved" | "changes_requested", issues?: import("@/types/contract").ReviewIssue[]): Promise<void>;
  extendCap(): Promise<void>;
  /** BUG-231's unified "Continue" action for a blocked/awaiting-user loop. */
  continueFlow(feedback: string): Promise<void>;
  listAgents(cwd?: string): Promise<AgentDefinition[]>;
  focusAgentRun(runId: string): Promise<void>;
  backToMainRun(): void;
  openOrchestrationBoard(): void;
  closeOrchestrationBoard(): void;
  appendSystemMessage(text: string, tone?: "info" | "error"): void;
  openAgentSpawnGuide(agentName?: string): void;
  clearAgentSpawnGuide(): void;
  syncHistoryRun(runId: string, projectId?: string): Promise<void>;
  syncAllInProject(projectId: string): Promise<void>;
  deleteHistoryRun(runId: string): Promise<void>;
  restoreRemoteChatSession(summary: RemoteChatSessionSummary, cwd?: string): Promise<void>;
  openHistoryRun(runId: string): Promise<void>;
  resetRun(): void;
  openInIde(path: string, line?: number): void;
  openAdminWeb(): void;
  restartSystem(): Promise<void>;
  shutdownSystem(): Promise<void>;
  confirmAccountSwitch(): Promise<void>;
  cancelAccountSwitch(): void;
  requestManualAccountSwitch(): void;
  confirmProviderSwitch(): Promise<void>;
  cancelProviderSwitch(): void;
  dismissGateBlock(): void;
  /** Submit the user's choice on the r-reg gate decision card (Task-155). */
  submitGateDecision(option: string, customText?: string): Promise<void>;
}

export const useStore = create<AppState>((set, get) => ({
  client: createRunnerClient(),
  projects: [],
  workflows: [],
  steps: [],
  skills: [],
  providerAccounts: [],
  localProviders: [],
  supportedModels: [],
  status: "idle",
  timeline: [],
  artifacts: [],
  pendingApprovals: [],
  pendingQuestions: [],
  runHistory: [],
  remoteChatSessions: [],
  agentRuns: [],
  agentGraphSnapshot: undefined,
  agentBusMessages: [],
  workflowStepRuntime: [],
  workflowStepRuntimeLoading: false,
  workflowStepRuntimeMeta: {},
  historyLoading: false,
  remoteHistoryLoading: false,
  latestTokenUsage: undefined,
  recoverable: false,
  scenario: "normal",
  accountSwitchLoading: false,
  providerSwitchLoading: false,
  _accountSwitchTriedIds: [],
  launchMode: "workflow",
  chatMode: "normal_chat",
  selectedProvider: "codex",
  yoloMode: false,
  summaryGenerating: false,
  chatStartMode: "normal",
  chatSourceDocId: "",
  flowRef: undefined,
  builtinOrchestrationOptions: [],
  workspaceMainView: "chat",
  _historyReplaying: false,
  _historyLoadSeq: 0,
  _remoteHistoryLoadSeq: 0,
  _agentRunsLoadSeq: 0,
  _workflowStepRuntimeLoadSeq: 0,
  _runSnapshots: {},
  _runReplaySeq: {},
  _gateBlockedRunIds: {},
  _streamRunSeq: 0,
  _orchestrationStreamSeq: 0,
  agentSpawnGuideAgentName: undefined,
  agentSpawnGuideOpen: false,

  async loadProjects() {
    if (loadProjectsInFlight) return loadProjectsInFlight;
    const client = get().client;
    if (!get().runId) {
      set((s) => (s.status === "idle" ? { status: "starting" } : {}));
    }

    loadProjectsInFlight = (async () => {
      // The runner starts via `go run`, which compiles first (~10-30s) before it
      // listens — so the first fetches can hit connection-refused ("Failed to fetch").
      // Retry ONLY connection-level errors (not HTTP errors like 502, which won't fix
      // themselves) so the navigator fills in once the runner is up, without a manual
      // reload. Load navigator data independently and surface a final error.
      const withRetry = async <T>(fn: () => Promise<T>): Promise<T> => {
        let lastErr: unknown;
        for (let i = 0; i < 10; i++) {
          try {
            return await fn();
          } catch (err) {
            lastErr = err;
            if (err instanceof RunnerApiError) throw err; // got an HTTP response — real error
            await new Promise((r) => setTimeout(r, 1500)); // connection refused — runner still booting
          }
        }
        throw lastErr;
      };

      try {
        const projects = await withRetry(() => client.listProjects());
        set((s) => ({ projects, ...(s.runId ? {} : { status: "idle" }) }));
        if (!get().selectedProjectId && projects.length > 0) {
          const saved = localStorage.getItem(LAST_PROJECT_KEY);
          const match = saved ? projects.find((p) => p.id === saved) : undefined;
          void get().selectProject((match ?? projects[0]).id);
        }
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] listProjects failed:", err);
        set((s) => ({
          ...(s.runId ? {} : { status: "failed" }),
          timeline: [
            ...s.timeline,
            { kind: "system", id: `err-projects-${s.timeline.length}`, text: `Failed to load projects: ${String(err)}`, tone: "error" },
          ],
        }));
      }
      try {
        const admin = await getAdminUseCases();
        const [workflowDefinitions, stepDefinitions, supportedModels, localProviders] = await Promise.all([
          admin.workflows.listWorkflows(),
          admin.workflows.listStepDefinitions(),
          admin.providers.listSupportedModels(),
          admin.providers.listLocalProviders(),
        ]);
        set({
          workflows: workflowDefinitions.map(mapNavigatorWorkflow),
          steps: stepDefinitions.map(mapNavigatorStep),
          localProviders,
          supportedModels,
          // Apply default model for the current provider if none is selected yet
          ...(!get().selectedModel ? { selectedModel: pickDefaultModel(get().selectedProvider, supportedModels) } : {}),
        });
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] definition catalog failed:", err);
      }
      try {
        const provider = get().selectedProvider ?? "codex";
        const skills = await withRetry(() => client.listSkills(provider, selectedProjectPath(get())));
        set({ skills });
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] listSkills failed:", err);
      }
      try {
        const providerAccounts = await withRetry(() => client.listProviderAccounts());
        set({ providerAccounts });
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] listProviderAccounts failed:", err);
      }
    })().finally(() => {
      loadProjectsInFlight = null;
    });

    return loadProjectsInFlight;
  },

  async loadProviderAccounts() {
    const client = get().client;
    try {
      const providerAccounts = await client.listProviderAccounts();
      set({ providerAccounts });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listProviderAccounts refresh failed:", err);
    }
  },

  async loadLocalProviders() {
    try {
      const admin = await getAdminUseCases();
      const localProviders = await admin.providers.listLocalProviders();
      set({ localProviders });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listLocalProviders refresh failed:", err);
    }
  },

  async refreshAgentRuns() {
    const { client, mainRunId, runId } = get();
    const parentRunId = mainRunId ?? runId;
    if (!parentRunId || !client.listAgentRuns) {
      set({ agentRuns: [] });
      return;
    }
    // Stale-response guard (BUG-130): this fetch is fire-and-forget and can land after a
    // newer SSE agent-graph update. Only apply the result if no later refresh started.
    const seq = get()._agentRunsLoadSeq + 1;
    set({ _agentRunsLoadSeq: seq });
    try {
      const agentRuns = await client.listAgentRuns(parentRunId);
      if (get()._agentRunsLoadSeq === seq && (get().mainRunId === parentRunId || get().runId === parentRunId)) {
        // BUG-169: merge (not replace) with whatever is already in state, same as the SSE
        // agent_graph_updated path (mergeAgentRunsById, BUG-132). This is a fire-and-forget
        // HTTP snapshot racing a live SSE channel — AgentsPanel fires a refresh the instant
        // mainRunId is set (before anything has spawned), and if that request is slow enough
        // to resolve AFTER a later SSE update already merged a freshly-spawned child in,
        // replacing wholesale here would wipe that child back out even though it is really
        // running. Merging makes the two sources converge instead of racing.
        set((s) => ({ agentRuns: mergeAgentRunsById(s.agentRuns, agentRuns) }));
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listAgentRuns failed:", err);
    }
  },
  async refreshWorkflowStepRuntime() {
    const { client, mainRunId, runId, chatMode } = get();
    const targetRunId = mainRunId ?? runId;
    // Normal chat has no workflow-step list to show; skip the request entirely (8.2).
    if (chatMode !== "workflow_step_auto" || !targetRunId || !client.getWorkflowStepsRuntime) {
      set({ workflowStepRuntime: [], workflowStepRuntimeMeta: {} });
      return;
    }
    const seq = get()._workflowStepRuntimeLoadSeq + 1;
    set({ _workflowStepRuntimeLoadSeq: seq, workflowStepRuntimeLoading: true });
    try {
      const snapshot = await client.getWorkflowStepsRuntime(targetRunId);
      if (get()._workflowStepRuntimeLoadSeq === seq && (get().mainRunId === targetRunId || get().runId === targetRunId)) {
        set({
          workflowStepRuntime: snapshot.steps,
          workflowStepRuntimeMeta: { provider: snapshot.provider, model: snapshot.model, yoloMode: snapshot.yoloMode },
          workflowStepRuntimeLoading: false,
        });
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] getWorkflowStepsRuntime failed:", err);
      if (get()._workflowStepRuntimeLoadSeq === seq) {
        set({ workflowStepRuntimeLoading: false });
      }
    }
  },
  async refreshAgentGraph() {
    const { client, mainRunId, runId } = get();
    const parentRunId = mainRunId ?? runId;
    if (!parentRunId || !client.refreshAgentGraph) return;
    set(applyAgentGraphSnapshot(await client.refreshAgentGraph(parentRunId)));
  },
  async pauseAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.pauseAgentLoop) set(applyAgentGraphSnapshot(await client.pauseAgentLoop(parentRunId))); },
  async resumeAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.resumeAgentLoop) set(applyAgentGraphSnapshot(await client.resumeAgentLoop(parentRunId))); },
  async injectAgentFeedback(toRunId, message) { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.injectAgentFeedback) set(applyAgentGraphSnapshot(await client.injectAgentFeedback(parentRunId, toRunId, message))); },
  async stopAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.stopAgentLoop) { set(applyAgentGraphSnapshot(await client.stopAgentLoop(parentRunId))); /* Bug 3 fix: also interrupt to forcefully terminate the in-flight provider turn */ if (client.interrupt) { try { await client.interrupt(parentRunId); } catch { /* best-effort: interrupt may 404 if no turn is in flight */ } } } },
  async submitReviewOutcome(outcome, issues) { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.submitReviewOutcome) set(applyAgentGraphSnapshot(await client.submitReviewOutcome(parentRunId, { outcome, issues }))); },
  async extendCap() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.extendCap) set(applyAgentGraphSnapshot(await client.extendCap(parentRunId))); },
  async continueFlow(feedback) {
    const { client, mainRunId, runId, activeAgentRunId } = get();
    const parentRunId = mainRunId ?? runId;
    if (!parentRunId || !client.continueFlow) return;
    // BUG-231 follow-up: resuming while a child agent is focused must not let the
    // hub's resumed turn stream into the focused child's transcript — return to
    // the main run first so the response lands where the user is looking.
    if (activeAgentRunId && activeAgentRunId !== parentRunId) {
      get().backToMainRun();
    }
    set(applyAgentGraphSnapshot(await client.continueFlow(parentRunId, feedback)));
  },

  async listAgents(cwd) {
    const { client } = get();
    if (!client.listAgents) return [];
    try {
      return await client.listAgents(cwd ?? selectedProjectPath(get()));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listAgents failed:", err);
      return [];
    }
  },

  async focusAgentRun(runId) {
    const { client } = get();
    const currentRunId = get().runId;
    const mainRunId = get().mainRunId ?? currentRunId;
    if (!mainRunId) return;
    cacheRunSnapshot(get(), currentRunId);
    const restore = get()._runSnapshots[runId];
    const streamRunSeq = get()._streamRunSeq + 1;
    const afterSeq = restore ? get()._runReplaySeq[runId] ?? restore.lastEventSeq ?? 0 : 0;
    cancelHistoryReplayStream();
    cancelAgentFocusStream();
    const agentFocusController = new AbortController();
    activeAgentFocusStreamController = agentFocusController;
    set({
      mainRunId,
      activeAgentRunId: runId,
      workspaceMainView: "chat",
      runId,
      ...(restore ? restoreRunSnapshot(restore) : emptyRunSnapshot("running")),
      agentSpawnGuideOpen: false,
      agentSpawnGuideAgentName: undefined,
      _streamRunSeq: streamRunSeq,
    });
    let handle;
    try {
      handle = await client.resumeRun(runId);
    } catch (err) {
      if (activeAgentFocusStreamController === agentFocusController) {
        activeAgentFocusStreamController = undefined;
      }
      if (!shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== streamRunSeq) return;
      set((s) => ({
        status: "failed",
        timeline: [
          ...s.timeline,
          { kind: "system", id: `agent-focus-error-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
        ],
      }));
      return;
    }
    if (!shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== streamRunSeq) {
      if (activeAgentFocusStreamController === agentFocusController) {
        activeAgentFocusStreamController = undefined;
      }
      return;
    }
    if (!restore) {
      set({ status: handle.status, activeStepId: handle.stepId });
    }
    const stream = client.focusAgentRun
      ? client.focusAgentRun(runId, agentFocusController.signal)
      : client.streamRun(runId, 0, agentFocusController.signal);
    void consumeAgentStream(runId, stream, streamRunSeq, afterSeq, handle.status, set, get).catch((err) => {
      if (!shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== streamRunSeq) return;
      set((s) => ({
        status: "failed",
        timeline: [
          ...s.timeline,
          { kind: "system", id: `agent-focus-error-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
        ],
      }));
    }).finally(() => {
      if (activeAgentFocusStreamController === agentFocusController) {
        activeAgentFocusStreamController = undefined;
      }
    });
    void get().refreshAgentRuns();
    void get().refreshWorkflowStepRuntime();
  },

  backToMainRun() {
    const currentRunId = get().runId;
    const { mainRunId, _runSnapshots } = get();
    if (!mainRunId) return;
    cacheRunSnapshot(get(), currentRunId);
    const restore = _runSnapshots[mainRunId];
    if (!restore) return;
    cancelAgentFocusStream();
    const agentFocusController = new AbortController();
    activeAgentFocusStreamController = agentFocusController;
    const streamRunSeq = get()._streamRunSeq + 1;
    // Use the snapshot's lastEventSeq (captured before child focus) as the stream
    // start point. _runReplaySeq[mainRunId] can be inflated by consumeOrchestrationStream
    // processing agent_graph_updated events while viewing the child — using it would
    // skip real timeline events interleaved with those orchestration events. (BUG-109)
    const afterSeq = restore.lastEventSeq ?? 0;
    set({
      runId: mainRunId,
      mainRunId,
      activeAgentRunId: undefined,
      workspaceMainView: "chat",
      ...restore,
      _streamRunSeq: streamRunSeq,
    });
    void consumeAgentStream(
      mainRunId,
      get().client.streamRun(mainRunId, afterSeq, agentFocusController.signal),
      streamRunSeq,
      afterSeq,
      restore.status,
      set,
      get,
    ).finally(() => {
      if (activeAgentFocusStreamController === agentFocusController) {
        activeAgentFocusStreamController = undefined;
      }
    });
    startOrchestrationStream(mainRunId, get().client, set, get);
  },

  openOrchestrationBoard() {
    set({ workspaceMainView: "board" });
  },

  closeOrchestrationBoard() {
    set({ workspaceMainView: "chat" });
  },

  appendSystemMessage(text, tone = "info") {
    set((s) => ({
      timeline: [...s.timeline, { kind: "system", id: `sys-${s.timeline.length}`, text, tone }],
    }));
  },

  openAgentSpawnGuide(agentName) {
    set({ agentSpawnGuideOpen: true, agentSpawnGuideAgentName: agentName });
  },

  clearAgentSpawnGuide() {
    set({ agentSpawnGuideOpen: false, agentSpawnGuideAgentName: undefined });
  },

  async selectProject(projectId) {
    localStorage.setItem(LAST_PROJECT_KEY, projectId);
    const projectChanged = get().selectedProjectId !== projectId;
    set({
      selectedProjectId: projectId,
      selectedWorkflowId: undefined,
      selectedStepId: undefined,
      runHistory: [],
      remoteChatSessions: [],
      historyLoadError: undefined,
      remoteHistoryLoadError: undefined,
    });
    if (projectChanged) {
      get().resetRun();
    }
    void get().loadSkills(get().selectedProvider ?? "codex");
  },

  setLaunchMode(mode) {
    set({ launchMode: mode });
  },

  setChatMode(mode) {
    set({ chatMode: mode });
    get().resetRun();
  },

  async selectWorkflow(workflowId) {
    set({ selectedWorkflowId: workflowId });
  },

  selectStep(stepId) {
    set({ selectedStepId: stepId });
  },

  setScenario(scenario) {
    get().client.setScenario?.(scenario);
    set({ scenario });
  },

  selectProvider(provider) {
    const state = get();
    if (!provider) {
      set({
        selectedProvider: undefined,
        selectedModel: undefined,
        pendingProviderSwitch: undefined,
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
      });
      return;
    }
    if (state.selectedProvider === provider) {
      return;
    }
    const targetModel = pickDefaultModel(provider, state.supportedModels);
    const canRequestHandoff =
      state.chatMode === "normal_chat" &&
      Boolean(state.runId) &&
      !isInteractiveChatBlocked(state.status);
    if (canRequestHandoff && state.selectedProvider) {
      const sourceRunId = state.runId;
      if (!sourceRunId) return;
      set({
        pendingProviderSwitch: {
          sourceRunId,
          sourceProviderKey: state.selectedProvider,
          sourceRunStatus: state.status,
          targetProviderKey: provider,
          targetModel,
        },
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
      });
      return;
    }
    set({ selectedProvider: provider, selectedModel: targetModel, pendingProviderSwitch: undefined });
    void get().loadSkills(provider);
  },

  cancelProviderSwitch() {
    set({ pendingProviderSwitch: undefined, providerSwitchLoading: false });
  },

  async confirmProviderSwitch() {
    const state = get();
    const pending = state.pendingProviderSwitch;
    if (!pending || state.providerSwitchLoading) return;
    if (state.chatMode !== "normal_chat" || !state.selectedProjectId) {
      set({ pendingProviderSwitch: undefined, providerSwitchLoading: false });
      return;
    }

    const targetProviderKey = pending.targetProviderKey;
    const targetModel = pending.targetModel ?? pickDefaultModel(targetProviderKey, state.supportedModels);
    const cwd = selectedProjectPath(state);
    set({ providerSwitchLoading: true });

    try {
      const handoff = await state.client.handoffContext(pending.sourceRunId, { targetProviderKey });
      const handle = await state.client.startRun({
        projectId: state.selectedProjectId,
        providerKey: targetProviderKey,
        model: targetModel,
        reasoningEffort: state.reasoningEffort,
        yoloMode: state.yoloMode,
        chatMode: "normal_chat",
        cwd,
      });
      set({
        selectedProvider: targetProviderKey,
        selectedModel: targetModel,
        runId: handle.runId,
        mainRunId: handle.runId,
        activeAgentRunId: undefined,
        activeStepId: handle.stepId,
        status: handle.status,
        timeline: [],
        artifacts: [],
        pendingApprovals: [],
        pendingQuestions: [],
        gateBlock: undefined,
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
        pendingProviderSwitch: undefined,
        providerSwitchLoading: false,
        _accountSwitchTriedIds: [],
        _streamingAssistantId: undefined,
        agentRuns: [],
        agentGraphSnapshot: undefined,
        agentBusMessages: [],
        workflowStepRuntime: [],
        workflowStepRuntimeMeta: {},
        agentSpawnGuideOpen: false,
        agentSpawnGuideAgentName: undefined,
        _runReplaySeq: {},
        _runSnapshots: {},
        _historyReplaying: false,
        _streamRunSeq: state._streamRunSeq + 1,
      });
      set((s) => ({
        timeline: [
          ...s.timeline,
          {
            kind: "system",
            id: `handoff-mode-${s.timeline.length}`,
            text: `Handoff from ${providerLabel(handoff.sourceProviderKey)} used ${handoff.handoffMode} context.`,
            tone: "info",
          },
        ],
      }));
      void get().loadSkills(targetProviderKey);
      await get().sendPrompt(handoff.prompt);
      void get().loadRunHistory();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] provider handoff failed:", err);
      set((s) => ({
        providerSwitchLoading: false,
        timeline: [
          ...s.timeline,
          {
            kind: "system",
            id: `handoff-err-${s.timeline.length}`,
            text: `Failed to start a new chat with ${providerLabel(targetProviderKey)}: ${String(err)}`,
            tone: "error",
          },
        ],
      }));
    }
  },

  setSelectedModel(model) {
    set({ selectedModel: model });
  },

  setReasoningEffort(effort) {
    set({ reasoningEffort: effort });
  },

  setYoloMode(yolo) {
    set({ yoloMode: yolo });
  },

  async generateChatSummary() {
    const state = get();
    const runId = state.runId;
    if (!runId || state.summaryGenerating) return;
    if (isInteractiveChatBlocked(state.status)) return;
    set({ summaryGenerating: true });
    try {
      const result = await state.client.generateChatSummary(runId);
      set((s) => ({
        summaryGenerating: false,
        timeline: [
          ...s.timeline,
          {
            kind: "system",
            id: `chat-summary-${s.timeline.length}`,
            text: result.generated
              ? "Chat summary updated."
              : `Chat summary unchanged${result.reason ? ` (${result.reason})` : ""}.`,
            tone: "info",
          },
        ],
      }));
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] generateChatSummary failed:", err);
      set((s) => ({
        summaryGenerating: false,
        timeline: [
          ...s.timeline,
          {
            kind: "system",
            id: `chat-summary-err-${s.timeline.length}`,
            text: `Failed to generate chat summary: ${String(err)}`,
            tone: "error",
          },
        ],
      }));
    }
  },

  setChatStartMode(mode) {
    set((state) => ({
      chatStartMode: mode,
      chatSourceDocId: mode === "normal" ? "" : state.chatSourceDocId,
      // A built-in orchestration selection only makes sense for the sub-mode
      // it was offered under (CP-42 Task-177 T-7: switching away from Bug
      // clears the Review Loop selection); clear both the pick and the
      // stale option list on every intent change, then reload below if the
      // new mode has options to offer.
      flowRef: undefined,
      builtinOrchestrationOptions: [],
    }));
    if (mode === "bugfix") {
      void get().loadBuiltinOrchestrationOptions("bug");
    }
  },

  setChatSourceDocId(sourceDocId) {
    set({ chatSourceDocId: sourceDocId });
  },

  setFlowRef(flowRef) {
    set({ flowRef });
  },

  async loadBuiltinOrchestrationOptions(subMode) {
    const { client } = get();
    if (!client.listBuiltinOrchestrationOptions) {
      set({ builtinOrchestrationOptions: [] });
      return;
    }
    try {
      const options = await client.listBuiltinOrchestrationOptions(subMode);
      set({ builtinOrchestrationOptions: options });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] loadBuiltinOrchestrationOptions failed:", err);
      set({ builtinOrchestrationOptions: [] });
    }
  },

  async loadSkills(provider, cwd) {
    const { client } = get();
    try {
      const skills = await client.listSkills(provider, cwd ?? selectedProjectPath(get()));
      set({ skills });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] loadSkills failed:", err);
    }
  },

  async sendPrompt(prompt, skills, attachments) {
    const {
      client, chatMode, launchMode,
      selectedProjectId, selectedWorkflowId, selectedStepId,
      selectedProvider, selectedModel, reasoningEffort, yoloMode, chatStartMode, chatSourceDocId, flowRef,
    } = get();
    const focusedRunId = get().activeAgentRunId;
    const mainRunId = get().mainRunId ?? get().runId;
    if (chatMode === "normal_chat" && focusedRunId && mainRunId && focusedRunId !== mainRunId) {
      get().appendSystemMessage("Child transcript is read-only. Return to the main chat to send prompts.");
      return;
    }
    const cwd = selectedProjectPath(get());

    if (chatMode === "normal_chat") {
      if (!selectedProjectId || !selectedProvider) return;
    } else {
      const launchTargetId = launchMode === "workflow" ? selectedWorkflowId : selectedStepId;
      if (!selectedProjectId || !launchTargetId) return;
    }

    const launchTargetId = launchMode === "workflow" ? selectedWorkflowId : selectedStepId;
    // In normal_chat the runner mints one synthetic step ("chat-<runId>") for the whole
    // run and surfaces it via startRun (turn 1) / resumeRun (from history). It is held in
    // `activeStepId` so follow-up turns reuse it instead of sending an empty stepId, which
    // startTurn rejects with 400. Workflow/step mode keeps using its stable launchTargetId.
    let turnStepId =
      chatMode === "normal_chat"
        ? get().activeStepId ?? launchTargetId ?? ""
        : launchTargetId ?? "";

    // Render the prompt + a "Thinking…" bubble UP FRONT. The composer clears its input
    // the instant it calls us, so if startRun/sendTurn rejects (e.g. an unsupported
    // provider → 422 provider_unavailable) we must not be left with a blank screen and
    // no record of what the user typed (BUG-050). The catch below replaces the thinking
    // bubble with a visible error instead of failing silently.
    set((s) => ({
      recoverable: false,
      latestTokenUsage: undefined,
      status: "running",
      _streamingAssistantId: undefined,
      timeline: [
        ...s.timeline,
        {
          kind: "prompt",
          id: `prompt-${s.timeline.length}`,
          text: prompt,
          selectedSkills: skills && skills.length > 0 ? [...skills] : undefined,
          attachments:
            attachments && attachments.length > 0
              ? attachments.map((a) => ({
                  id: a.id,
                  originalName: a.originalName,
                  mimeType: a.mimeType,
                  // Normalized images are already small; the base64 doubles as the
                  // preview so history-rendered bubbles still show a thumbnail.
                  previewUrl: `data:${a.mimeType};base64,${a.data}`,
                }))
              : undefined,
        },
        { kind: "thinking", id: `thinking-${s.timeline.length}`, text: "Thinking..." },
      ],
    }));

    let runId = get().runId;
    const isFirstChatTurn = chatMode === "normal_chat" && !runId;
    try {
      if (!runId) {
        const handle = await client.startRun(
          chatMode === "normal_chat"
            ? {
                projectId: selectedProjectId!,
                providerKey: selectedProvider,
                model: selectedModel,
                reasoningEffort,
                yoloMode,
                chatMode: "normal_chat",
                cwd,
              }
            : {
                projectId: selectedProjectId!,
                workflowId: launchMode === "workflow" ? selectedWorkflowId : undefined,
                stepId: launchTargetId || undefined,
                providerKey: selectedProvider,
                cwd,
              },
        );
        runId = handle.runId;
        if (handle.stepId) {
          turnStepId = handle.stepId;
        }
        set({
          mainRunId: handle.runId,
          activeAgentRunId: undefined,
        });
      }

      const turnInput: TurnInput = {
        runId,
        stepId: turnStepId,
        prompt,
        changeType: isFirstChatTurn && chatStartMode !== "normal" ? chatStartMode : undefined,
        sourceDocId:
          isFirstChatTurn && chatStartMode !== "normal" && chatSourceDocId.trim().length > 0
            ? chatSourceDocId.trim()
            : undefined,
        selectedSkills:
          skills && skills.length > 0
            ? skills.map((name) => {
                // Carry the picker's absolute skill path so the runner reads the exact
                // selected skill file (BUG-063 follow-up) instead of matching by name/id,
                // which is fragile across provider layouts and the run's cwd.
                const found = get().skills.find((s) => s.name === name);
                return { name, path: found?.path, source: "slash_picker" as const };
              })
            : undefined,
        reasoningEffort: chatMode === "normal_chat" ? reasoningEffort : undefined,
        // Resend model + YOLO on every chat turn so they can change between prompts
        // (BUG-063); the runner applies them per turn. An empty string is sent for
        // "Default" so switching back to Default reliably clears a previously-set model
        // (undefined would be dropped by JSON and fall back to the run-level value).
        // Workflow/step mode keeps the run-level values captured at startRun.
        model: chatMode === "normal_chat" ? (selectedModel ?? "") : undefined,
        yoloMode: chatMode === "normal_chat" ? yoloMode : undefined,
        attachments:
          chatMode === "normal_chat" && attachments && attachments.length > 0
            ? attachments
            : undefined,
        // Built-in orchestration selection (CP-42/Task-177): only meaningful
        // alongside a first-turn Bug intent, and only when the user actually
        // picked a flow. chatStartMode's runner-facing sub-mode key is "bug",
        // distinct from the UI's "bugfix" tab value.
        subMode: isFirstChatTurn && chatStartMode === "bugfix" && flowRef ? "bug" : undefined,
        flowRef: isFirstChatTurn && chatStartMode === "bugfix" ? flowRef : undefined,
      };
      set({ runId, lastTurnInput: turnInput, activeStepId: turnStepId, _streamRunSeq: get()._streamRunSeq + 1 });
      cancelHistoryReplayStream();
      cancelOrchestrationStream();
      cancelAgentFocusStream();

      await consumeStream(runId, client.sendTurn(turnInput), set, get);
      const orchestrationRunId = get().mainRunId ?? runId;
      if (orchestrationRunId) {
        startOrchestrationStream(orchestrationRunId, client, set, get);
      }
      void get().refreshAgentRuns();
    void get().refreshWorkflowStepRuntime();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] sendPrompt failed:", err);
      if (runId && !shouldApplyRunEvent(get().runId, runId)) {
        return;
      }
      set((s) => ({
        status: "failed",
        // Only offer Reconnect if a run was actually created; a failed startRun has none.
        recoverable: Boolean(runId),
        timeline: [
          ...s.timeline.filter((it) => it.kind !== "thinking"),
          { kind: "system", id: `err-run-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
        ],
      }));
    } finally {
      void get().loadRunHistory();
    }
  },

  async approve(approvalId, decision) {
    // Resolve the specific card the user clicked, not "whatever is pending" — a turn
    // can fan out several parallel tool calls awaiting approval at once, so more than
    // one entry may be in pendingApprovals simultaneously (BUG-157).
    const pending = get().pendingApprovals.find((p) => p.approvalId === approvalId);
    if (!pending) return;
    set((s) => ({
      pendingApprovals: s.pendingApprovals.filter((p) => p.approvalId !== approvalId),
      status: s.pendingApprovals.length > 1 ? "waiting_approval" : "running",
      timeline: s.timeline.map((it) =>
        it.kind === "approval" && it.approvalId === approvalId ? { ...it, decision } : it,
      ),
    }));
    try {
      await get().client.submitApproval(approvalId, decision);
    } catch (err) {
      // BUG-172: mirror sendPrompt's error handling — an unhandled rejection here
      // (e.g. a transient network blip while YOLO fires off rapid step
      // transitions) used to leave the run silently stuck in whatever status the
      // optimistic update above set, with nothing telling the user it never
      // reached the server.
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] approve failed:", err);
      set((s) => ({
        status: "failed",
        recoverable: true,
        timeline: [
          ...s.timeline,
          { kind: "system", id: `err-approve-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
        ],
      }));
    }
  },

  async answer(questionId, choice) {
    // Same rationale as approve() (BUG-157) — resolve the specific card the user
    // acted on, not "whatever is pending", since more than one question can be
    // outstanding at once.
    const pending = get().pendingQuestions.find((q) => q.questionId === questionId);
    if (!pending) return;
    set((s) => ({
      pendingQuestions: s.pendingQuestions.filter((q) => q.questionId !== questionId),
      status: s.pendingQuestions.length > 1 ? "waiting_question" : "running",
      timeline: s.timeline.map((it) =>
        it.kind === "question" && it.questionId === questionId ? { ...it, answer: choice } : it,
      ),
    }));
    try {
      await get().client.answerQuestion(questionId, choice);
    } catch (err) {
      // BUG-172: see approve() above — same failure mode for the question path.
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] answer failed:", err);
      set((s) => ({
        status: "failed",
        recoverable: true,
        timeline: [
          ...s.timeline,
          { kind: "system", id: `err-answer-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
        ],
      }));
    }
  },

  async stop() {
    const { client, runId, mainRunId, activeAgentRunId } = get();
    if (!runId) return;
    const parentRunId = mainRunId ?? runId;
    const childFocused = Boolean(activeAgentRunId && parentRunId && activeAgentRunId !== parentRunId);
    if (!childFocused && parentRunId && client.stopAgentLoop && hasActiveParentAgentLoop(get(), parentRunId)) {
      const snapshot = await client.stopAgentLoop(parentRunId);
      set((s) => ({
        ...applyAgentGraphSnapshot(snapshot),
        status: deriveOrchestrationRunStatus(s.status, snapshot),
        timeline: s.timeline.filter((it) => it.kind !== "thinking"),
      }));
      await client.interrupt(parentRunId);
      void get().refreshAgentRuns();
      void get().refreshWorkflowStepRuntime();
      return;
    }
    if (!childFocused && get().chatMode === "workflow_step_auto" && parentRunId) {
      if (client.stopAgentLoop) {
        set(applyAgentGraphSnapshot(await client.stopAgentLoop(parentRunId)));
      }
      await client.interrupt(parentRunId);
      return;
    }
    await client.interrupt(runId);
  },

  async reconnect() {
    const { client, runId } = get();
    if (!runId) return;
    await client.resumeRun(runId);
    // Clear the timeline so the replay visibly rebuilds it from persisted events
    // via the run's event stream (attach + replay from seq 0).
    set({ timeline: [], recoverable: false, status: "running", _streamingAssistantId: undefined, _streamRunSeq: get()._streamRunSeq + 1 });
    await consumeStream(runId, client.streamRun(runId, 0), set, get);
    startOrchestrationStream(runId, client, set, get);
  },

  async loadRunHistory() {
    const { client, selectedProjectId } = get();
    if (!selectedProjectId) {
      set({ runHistory: [], historyLoading: false, historyLoadError: undefined });
      return;
    }
    // F-3: stale-response guard — bump seq before the async call, discard result if seq moved on
    const seq = get()._historyLoadSeq + 1;
    set({ historyLoading: true, _historyLoadSeq: seq });
    try {
      const runHistory = await client.listRunHistory(selectedProjectId);
      if (get()._historyLoadSeq !== seq) return;
      set({ runHistory, historyLoading: false, historyLoadError: undefined });
    } catch (err) {
      if (get()._historyLoadSeq !== seq) return;
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listRunHistory failed:", err);
      // F-4: surface error in the history panel instead of injecting into the chat timeline
      set({ historyLoading: false, historyLoadError: String(err) });
    }
  },

  async loadRemoteChatSessions() {
    const { client, selectedProjectId } = get();
    if (!selectedProjectId) {
      set({ remoteChatSessions: [], remoteHistoryLoading: false, remoteHistoryLoadError: undefined });
      return;
    }
    const seq = get()._remoteHistoryLoadSeq + 1;
    set({ remoteHistoryLoading: true, _remoteHistoryLoadSeq: seq });
    try {
      const remoteChatSessions = await client.listRemoteChatSessions(selectedProjectId);
      if (get()._remoteHistoryLoadSeq !== seq) return;
      set({ remoteChatSessions, remoteHistoryLoading: false, remoteHistoryLoadError: undefined });
    } catch (err) {
      if (get()._remoteHistoryLoadSeq !== seq) return;
      set({ remoteHistoryLoading: false, remoteHistoryLoadError: String(err) });
    }
  },

  async syncHistoryRun(runId, projectId) {
    const { client, selectedProjectId } = get();
    const driveProjectId = projectId ?? selectedProjectId;
    set((s) => ({
      runHistory: s.runHistory.map((item) =>
        item.runId === runId
          ? {
              ...item,
              syncStatus: "syncing",
              unavailableReason: undefined,
            }
          : item,
      ),
    }));
    try {
      const result = await client.syncChatRun(runId, driveProjectId ? { googleDriveProjectId: driveProjectId } : undefined);
      set((s) => ({
        runHistory: s.runHistory.map((item) =>
          item.runId === runId
            ? {
                ...item,
                sourceMachineId: result.sourceMachineId,
                sourceRunId: result.sourceRunId,
                syncStatus: result.syncStatus,
                unavailableReason: undefined,
              }
            : item,
        ),
      }));
      void get().loadRemoteChatSessions();
    } catch (err) {
      if (
        err instanceof RunnerApiError &&
        (err.code === "session_unavailable" ||
          err.code === "account_not_signed_in" ||
          err.code === "resume_unsupported" ||
          err.code === "account_unavailable" ||
          err.code === "google_drive_not_connected")
      ) {
        set((s) => ({
          runHistory: s.runHistory.map((item) =>
            item.runId === runId ? { ...item, syncStatus: "failed", unavailableReason: err.message } : item,
          ),
        }));
        return;
      }
      set((s) => ({
        runHistory: s.runHistory.map((item) =>
          item.runId === runId ? { ...item, syncStatus: "failed" } : item,
        ),
      }));
      throw err;
    }
  },

  async syncAllInProject(projectId) {
    // Sync every not-yet-synced chat run in the project, one at a time so we do
    // not hammer Drive. Per-item failures are swallowed (syncHistoryRun marks
    // the row failed) so one broken session does not abort the whole batch.
    const targets = get()
      .runHistory.filter(
        (item) =>
          item.projectId === projectId &&
          item.runKind === "chat" &&
          item.syncStatus !== "synced" &&
          !item.unavailableReason,
      )
      .map((item) => item.runId);
    for (const runId of targets) {
      try {
        await get().syncHistoryRun(runId, projectId);
      } catch {
        // already reflected as syncStatus: "failed" on the row
      }
    }
  },

  async deleteHistoryRun(runId) {
    const { client } = get();
    const wasActive = get().runId === runId;
    // Optimistically remove from local history so the UI responds immediately.
    set((s) => ({ runHistory: s.runHistory.filter((item) => item.runId !== runId) }));
    // If the deleted run was the active session, reset the main panel to idle.
    if (wasActive) {
      set({
        runId: undefined,
        activeStepId: undefined,
        status: "idle",
        timeline: [],
        artifacts: [],
        pendingApprovals: [],
        pendingQuestions: [],
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
        pendingProviderSwitch: undefined,
        providerSwitchLoading: false,
        _accountSwitchTriedIds: [],
        _streamingAssistantId: undefined,
      });
    }
    try {
      await client.deleteRun(runId);
    } catch (err) {
      // Restore the item on failure by refreshing history from the runner.
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] deleteRun failed:", err);
      const { selectedProjectId } = get();
      if (selectedProjectId) {
        try {
          const runHistory = await client.listRunHistory(selectedProjectId);
          set({ runHistory });
        } catch {
          // best-effort refresh
        }
      }
      throw err;
    }
  },

  async restoreRemoteChatSession(summary, cwd) {
    const { client, selectedProjectId } = get();
    if (!selectedProjectId) return;
    const request: ChatSessionRestoreRequest = {
      projectId: selectedProjectId,
      sourceMachineId: summary.sourceMachineId,
      sourceRunId: summary.sourceRunId,
      cwd,
    };
    try {
      const result = await client.restoreChatRun(request);
      await Promise.all([get().loadRunHistory(), get().loadRemoteChatSessions()]);
      void get().openHistoryRun(result.runId);
    } catch (err) {
      if (err instanceof RunnerApiError && err.code === "cwd_remap_required" && !cwd) {
        const retryCwd = selectedProjectPath(get());
        if (retryCwd) {
          await get().restoreRemoteChatSession(summary, retryCwd);
          return;
        }
      }
      if (
        err instanceof RunnerApiError &&
        (err.code === "sync_remote_not_found" ||
          err.code === "sync_integrity_failed" ||
          err.code === "account_not_signed_in" ||
          err.code === "account_unavailable" ||
          err.code === "cwd_remap_required" ||
          err.code === "session_file_conflict")
      ) {
        set((s) => ({
          remoteChatSessions: s.remoteChatSessions.map((item) =>
            item.sourceMachineId === summary.sourceMachineId && item.sourceRunId === summary.sourceRunId
              ? { ...item, unavailableReason: err.message }
              : item,
          ),
        }));
        return;
      }
      throw err;
    }
  },

  async openHistoryRun(runId) {
    const { client } = get();
    const historyItem = get().runHistory.find((item) => item.runId === runId);
    const historyProvider = historyItem?.providerKey;
    console.info("[FlowPilot][history-open] start", {
      runId,
      providerKey: historyProvider,
      status: historyItem?.status,
      syncStatus: historyItem?.syncStatus,
      sourceMachineId: historyItem?.sourceMachineId,
      sourceRunId: historyItem?.sourceRunId,
    });
    let handle;
    try {
      handle = await client.resumeRun(runId);
    } catch (err) {
      if (err instanceof RunnerApiError) {
        console.error("[FlowPilot][history-open] resume failed", {
          runId,
          status: err.status,
          code: err.code,
          message: err.message,
        });
        set((s) => ({
          runHistory: s.runHistory.map((item) =>
            item.runId === runId ? { ...item, unavailableReason: err.message } : item
          ),
        }));
        return;
      }
      console.error("[FlowPilot][history-open] unexpected resume failure", { runId, error: err });
      throw err;
    }
    console.info("[FlowPilot][history-open] resume succeeded", {
      requestedRunId: runId,
      runId: handle.runId,
      providerKey: handle.providerKey,
      providerSessionId: handle.providerSessionId,
      status: handle.status,
      stepId: handle.stepId,
    });
    // BUG-170: restore the mode this run actually was, not whatever the UI happened to be
    // in before the user clicked a history item. Without this, reopening a workflow/flow-
    // mode run left chatMode stuck (often "normal_chat"), so the reopened run rendered
    // without its Flow Mode surfaces (step-timeline sidebar, agents panel gating) even
    // though the runner resumed it correctly. runKind is "chat" for normal_chat runs and
    // "workflow" (or, for older persisted rows, undefined) for everything else.
    const isWorkflowHistoryItem = historyItem?.runKind !== "chat";
    set({
      runId: handle.runId,
      mainRunId: handle.runId,
      activeAgentRunId: undefined,
      status: handle.status,
      activeStepId: handle.stepId,
      chatMode: isWorkflowHistoryItem ? "workflow_step_auto" : "normal_chat",
      ...(isWorkflowHistoryItem && historyItem?.workflowId
        ? { launchMode: "workflow", selectedWorkflowId: historyItem.workflowId }
        : {}),
      timeline: [],
      artifacts: [],
      pendingApprovals: [],
      pendingQuestions: [],
      gateBlock: undefined,
      latestTokenUsage: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      pendingAccountSwitch: undefined,
      accountSwitchLoading: false,
      pendingProviderSwitch: undefined,
      providerSwitchLoading: false,
      _accountSwitchTriedIds: [],
      _streamingAssistantId: undefined,
      agentRuns: [],
      agentGraphSnapshot: undefined,
      agentBusMessages: [],
      agentSpawnGuideOpen: false,
      agentSpawnGuideAgentName: undefined,
      _runReplaySeq: {},
      // Drop snapshots from the previously-open run so a later focus/back round-trip
      // can't restore a stale timeline from an unrelated chat. (BUG-111)
      _runSnapshots: {},
      // Suppress the "AI response complete" toast while the transcript replays. (BUG-118)
      _historyReplaying: true,
      _streamRunSeq: get()._streamRunSeq + 1,
      ...(historyProvider
        ? {
            selectedProvider: historyProvider,
            selectedModel: pickDefaultModel(historyProvider, get().supportedModels),
          }
        : {}),
      runHistory: get().runHistory.map((item) =>
        item.runId === runId ? { ...item, unavailableReason: undefined } : item
      ),
    });
    if (historyProvider) {
      void get().loadSkills(historyProvider);
    }
    cancelHistoryReplayStream();
    cancelOrchestrationStream();
    cancelAgentFocusStream();
    const historyReplayController = new AbortController();
    activeHistoryReplayController = historyReplayController;
    console.info("[FlowPilot][history-open] stream replay start", { runId: handle.runId });
    void consumeHistoryReplayStream(
      handle.runId,
      handle.status,
      client.streamRun(handle.runId, 0, historyReplayController.signal),
      set,
      get,
      handle.lastEventSeq,
    )
      .then(() => {
        console.info("[FlowPilot][history-open] stream replay complete", {
          runId: handle.runId,
          timelineItems: get().timeline.length,
        });
      })
      .catch((err) => {
        console.error("[FlowPilot][history-open] stream replay failed", { runId: handle.runId, error: err });
      })
      .finally(() => {
        set(() => ({ _historyReplaying: false }));
        if (activeHistoryReplayController === historyReplayController) {
          activeHistoryReplayController = undefined;
        }
      });
    startOrchestrationStream(handle.runId, client, set, get);
    void get().refreshAgentRuns();
    void get().refreshWorkflowStepRuntime();
  },

  resetRun() {
    const { selectedProvider, supportedModels } = get();
    cancelHistoryReplayStream();
    cancelOrchestrationStream();
    cancelAgentFocusStream();
    set({
      runId: undefined,
      mainRunId: undefined,
      activeAgentRunId: undefined,
      activeStepId: undefined,
      status: "idle",
      timeline: [],
      artifacts: [],
      agentRuns: [],
      agentGraphSnapshot: undefined,
      agentBusMessages: [],
      workflowStepRuntime: [],
      workflowStepRuntimeMeta: {},
      agentSpawnGuideOpen: false,
      agentSpawnGuideAgentName: undefined,
      pendingApprovals: [],
      pendingQuestions: [],
      latestTokenUsage: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      pendingAccountSwitch: undefined,
      accountSwitchLoading: false,
      pendingProviderSwitch: undefined,
      providerSwitchLoading: false,
      _accountSwitchTriedIds: [],
      _streamingAssistantId: undefined,
      _runSnapshots: {},
      _runReplaySeq: {},
      chatStartMode: "normal",
      chatSourceDocId: "",
      flowRef: undefined,
      builtinOrchestrationOptions: [],
      selectedModel: pickDefaultModel(selectedProvider, supportedModels),
    });
  },

  openInIde(path, line) {
    void ideBridge.openInIde(path, line);
  },

  openAdminWeb() {
    void ideBridge.openExternal(ADMIN_WEB_URL);
  },

  async restartSystem() {
    await get().client.restartStack();
  },

  async shutdownSystem() {
    await get().client.shutdownStack();
  },

  cancelAccountSwitch() {
    set({ pendingAccountSwitch: undefined });
  },

  dismissGateBlock() {
    set({ gateBlock: undefined });
  },

  async submitGateDecision(option: string, customText?: string) {
    const { gateBlock, client } = get();
    if (!gateBlock?.runId || !client.submitGateDecision) return;
    const runId = gateBlock.runId;
    set({ gateBlock: undefined });
    await client.submitGateDecision(runId, option, customText);
  },

  requestManualAccountSwitch() {
    const { selectedProvider, providerAccounts } = get();
    if (!selectedProvider) return;
    const activeAccount = providerAccounts.find((a) => a.providerKey === selectedProvider && a.isActive);
    // For manual pick, ignore _accountSwitchTriedIds — user is proactively choosing.
    const skipIds = activeAccount ? [activeAccount.id] : [];
    const candidate = findBestCandidate(providerAccounts, selectedProvider, skipIds);
    if (!candidate) return;
    set({
      pendingAccountSwitch: {
        providerKey: selectedProvider,
        failedAccountId: activeAccount?.id ?? "",
        failedAccountLabel: activeAccount ? accountLabel(activeAccount) : "current account",
        candidateAccount: candidate,
        reason: "manual",
      },
    });
  },

  async confirmAccountSwitch() {
    const { client, pendingAccountSwitch, lastTurnInput } = get();
    if (!pendingAccountSwitch) return;

    const { providerKey, candidateAccount, reason } = pendingAccountSwitch;
    const newLabel = accountLabel(candidateAccount);
    const shouldRetry = reason === "usage_limit" && !!lastTurnInput;

    set({ accountSwitchLoading: true, pendingAccountSwitch: undefined });

    try {
      await client.activateProviderAccount(candidateAccount.id);
      await get().loadProviderAccounts();
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] activateProviderAccount failed:", err);
      set((s) => ({
        accountSwitchLoading: false,
        timeline: [
          ...s.timeline,
          {
            kind: "system",
            id: `switch-err-${s.timeline.length}`,
            text: `Failed to switch to ${providerLabel(providerKey)} account "${newLabel}": ${String(err)}`,
            tone: "error",
          },
        ],
      }));
      return;
    }

    const noticeText = shouldRetry
      ? `Switched to ${providerLabel(providerKey)} account "${newLabel}". Retrying your request...`
      : `Switched to ${providerLabel(providerKey)} account "${newLabel}".`;

    set((s) => ({
      accountSwitchLoading: false,
      selectedProvider: providerKey,
      timeline: s.timeline.length > 0
        ? [...s.timeline, { kind: "system", id: `switch-notice-${s.timeline.length}`, text: noticeText, tone: "info" }]
        : s.timeline,
    }));

    if (shouldRetry) {
      await retryWithTurnInput(client, lastTurnInput!, set, get);
    }
  },
}));

// ── Account-switch helpers ─────────────────────────────────────────────────

function isUsageLimitMessage(msg: string): boolean {
  const lower = msg.toLowerCase();
  return (
    lower.includes("usage limit reached") ||
    lower.includes("extra usage unavailable") ||
    lower.includes("out of credits") ||
    lower.includes("out_of_credits") ||
    lower.includes("quota reset") ||
    lower.includes("rate limit")
  );
}

export function accountLabel(account: ProviderAccountSummary): string {
  return account.accountEmail ?? account.accountName ?? account.displayLabel ?? account.displayName;
}

export function providerLabel(providerKey: string): string {
  if (providerKey === "claude") return "Claude";
  if (providerKey === "codex") return "Codex";
  return providerKey;
}

// ── Workflow-step runtime helpers (BUG-153) ────────────────────────────────
// Derived from workflowStepRuntime + chatMode rather than stored separately,
// so there is exactly one source of truth to keep in sync.

/** Flow mode only; Review Loop / normal chat has no linear workflow-step list (F-14). */
export function isFlowModeRun(chatMode: ChatMode): boolean {
  return chatMode === "workflow_step_auto";
}

/** The step currently RUNNING or WAITING_USER_APPROVAL, if any. */
export function activeWorkflowStep(steps: WorkflowStepRuntimeDTO[]): WorkflowStepRuntimeDTO | undefined {
  return steps.find((s) => s.status === "RUNNING" || s.status === "WAITING_USER_APPROVAL");
}

/** True when any step has been retried at least once (F-5). */
export function hasRetries(steps: WorkflowStepRuntimeDTO[]): boolean {
  return steps.some((s) => s.retryCount > 0);
}

function isInteractiveChatBlocked(status: RunStatus): boolean {
  return status === "running" || status === "waiting_approval" || status === "waiting_question";
}

function findBestCandidate(
  accounts: ProviderAccountSummary[],
  providerKey: string,
  triedAccountIds: string[],
): ProviderAccountSummary | undefined {
  const candidates = accounts.filter(
    (a) =>
      a.providerKey === providerKey &&
      a.authStatus === "connected" &&
      !a.isActive &&
      !triedAccountIds.includes(a.id),
  );

  // Prefer candidates with valid numeric quota data (both windows > 0)
  const withQuota = candidates.filter(
    (a) =>
      a.remaining5hPercent !== null &&
      a.remaining7dPercent !== null &&
      a.remaining5hPercent > 0 &&
      a.remaining7dPercent > 0,
  );

  if (withQuota.length > 0) {
    return [...withQuota].sort((a, b) => {
      const d5h = (b.remaining5hPercent ?? 0) - (a.remaining5hPercent ?? 0);
      if (d5h !== 0) return d5h;
      const d7d = (b.remaining7dPercent ?? 0) - (a.remaining7dPercent ?? 0);
      if (d7d !== 0) return d7d;
      return a.slotIndex - b.slotIndex;
    })[0];
  }

  // Fall back to candidates where quota telemetry is genuinely unavailable (both null).
  // Accounts with known-zero quota (0) are excluded — they're confirmed exhausted.
  const withUnknownQuota = candidates.filter(
    (a) => a.remaining5hPercent === null && a.remaining7dPercent === null,
  );
  return [...withUnknownQuota].sort((a, b) => a.slotIndex - b.slotIndex)[0];
}

async function retryWithTurnInput(
  client: RunnerClient,
  turnInput: TurnInput,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  set((s) => ({
    status: "running",
    recoverable: false,
    latestTokenUsage: undefined,
    _streamingAssistantId: undefined,
    timeline: [
      ...s.timeline,
      { kind: "thinking", id: `thinking-retry-${s.timeline.length}`, text: "Thinking..." },
    ],
  }));
  try {
    await consumeStream(turnInput.runId, client.sendTurn(turnInput), set, get);
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error("[FlowPilot] retryWithTurnInput failed:", err);
    set((s) => ({
      status: "failed",
      recoverable: Boolean(turnInput.runId),
      timeline: [
        ...s.timeline.filter((it) => it.kind !== "thinking"),
        { kind: "system", id: `err-retry-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
      ],
    }));
  } finally {
    void get().loadRunHistory();
  }
}

// ── Stream consumer ────────────────────────────────────────────────────────

// Consumes a turn stream and folds each event into the timeline + status.
// mySeq captures _streamRunSeq at call time; if the counter advances (because
// openHistoryRun or reconnect started a newer stream for the same runId) this
// stream exits immediately rather than applying stale events. (BUG-079)
async function consumeStream(
  runId: string,
  stream: AsyncIterable<ProviderEventDTO>,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  const mySeq = get()._streamRunSeq;
  const isStale = () => !shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== mySeq;
  for await (const e of stream) {
    if (isStale()) {
      return;
    }
    if (!isEventForRun(e, runId)) continue;
    set((s) => applyEvent(s, e));
    // BUG-180: the hub's first turn is consumed here (not the orchestration
    // stream, which only starts after the turn), and the flow executor reseeds +
    // transitions steps while the coder runs during that turn. Each transition
    // rides alongside an agent_graph_updated, so refresh the step runtime here too
    // to keep the timeline live during the initial coder phase. (Self-guarded.)
    if (e.type === "agent_graph_updated") void get().refreshWorkflowStepRuntime();
    if (e.type === "agent_graph_updated") void get().refreshAgentRuns();
    if (e.type === "turn_failed" && !e.recoverable && isUsageLimitMessage(e.error)) {
      const s = get();
      if (s.chatMode === "normal_chat" && s.selectedProvider && !s.pendingAccountSwitch && !s.accountSwitchLoading) {
        const failedAccount = s.providerAccounts.find((a) => a.providerKey === s.selectedProvider && a.isActive);
        if (failedAccount) {
          const tried = [...s._accountSwitchTriedIds, failedAccount.id];
          const candidate = findBestCandidate(s.providerAccounts, s.selectedProvider, tried);
          if (candidate) {
            const providerKey = s.selectedProvider;
            const failedId = failedAccount.id;
            const failedLbl = accountLabel(failedAccount);
            set((_) => ({
              pendingAccountSwitch: {
                providerKey,
                failedAccountId: failedId,
                failedAccountLabel: failedLbl,
                candidateAccount: candidate,
                reason: "usage_limit" as const,
              },
              _accountSwitchTriedIds: tried,
            }));
          }
        }
      }
    }
  }
  if (isStale()) {
    return;
  }
  // settle recoverable flag for the Reconnect affordance
  const last = get().timeline[get().timeline.length - 1];
  if (last?.kind === "system" && last.tone === "error") {
    // handled in applyEvent
  }
}

async function consumeHistoryReplayStream(
  runId: string,
  resumedStatus: RunStatus,
  stream: AsyncIterable<ProviderEventDTO>,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
  lastEventSeq?: number,
): Promise<void> {
  const mySeq = get()._streamRunSeq;
  const isStale = () => !shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== mySeq;
  for await (const e of stream) {
    if (isStale()) return;
    if (!isEventForRun(e, runId)) continue;
    if (e.type !== "agent_graph_updated" && e.type !== "agent_bus_message") {
      set((s) => applyEvent(s, e));
      settleTerminalReplayVisuals(runId, resumedStatus, set);
    }
    if (shouldStopHistoryReplay(resumedStatus, e, lastEventSeq)) break;
  }
  if (!isStale()) {
    settleHistoryReplayPendingState(runId, resumedStatus, set);
  }
}

function shouldStopHistoryReplay(resumedStatus: RunStatus, e: ProviderEventDTO, lastEventSeq?: number): boolean {
  // Preferred path (BUG-112): the runner reports the seq of the last persisted event.
  // Stop only once we've replayed up to it, so a multi-turn transcript is replayed in
  // full instead of being truncated at the first turn_completed. A "running" run keeps
  // live-tailing (more events will arrive), so never stop it on the seq cursor.
  if (typeof lastEventSeq === "number" && lastEventSeq > 0) {
    if (resumedStatus === "running" || resumedStatus === "starting") return false;
    return e.seq >= lastEventSeq;
  }
  // Fallback for runners that predate lastEventSeq: stop at the first terminal event.
  if (resumedStatus === "waiting_approval") return e.type === "permission_required";
  if (resumedStatus === "waiting_question") return e.type === "user_question_required";
  if (resumedStatus === "completed") return e.type === "turn_completed";
  if (resumedStatus === "failed" || resumedStatus === "cancelled") return e.type === "turn_failed";
  return false;
}

function settleHistoryReplayPendingState(
  runId: string,
  resumedStatus: RunStatus,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
): void {
  if (resumedStatus === "waiting_approval" || resumedStatus === "waiting_question") return;
  set((s) => {
    if (s.runId !== runId || (s.pendingApprovals.length === 0 && s.pendingQuestions.length === 0)) return {};
    // An approval/question is still genuinely open if its own card was never stamped
    // with a decision — checked per-id (not just the last timeline item) because
    // concurrent tool calls can leave several cards outstanding at once (BUG-157).
    // This also covers BUG-105: resumedStatus is stale server ground truth when the
    // replay stream itself ends on an unresolved permission_required/question.
    const stillOpenApprovals = s.pendingApprovals.filter((pending) =>
      s.timeline.some((it) => it.kind === "approval" && it.approvalId === pending.approvalId && it.decision === undefined),
    );
    const stillOpenQuestions = s.pendingQuestions.filter((pending) =>
      s.timeline.some((it) => it.kind === "question" && it.questionId === pending.questionId && it.answer === undefined),
    );

    if (stillOpenApprovals.length > 0 || stillOpenQuestions.length > 0) {
      return {
        pendingApprovals: stillOpenApprovals,
        pendingQuestions: stillOpenQuestions,
        status: stillOpenApprovals.length > 0 ? "waiting_approval" : "waiting_question",
      };
    }

    const staleApprovalIds = new Set(s.pendingApprovals.map((p) => p.approvalId));
    const staleQuestionIds = new Set(s.pendingQuestions.map((q) => q.questionId));
    return {
      pendingApprovals: [],
      pendingQuestions: [],
      timeline: s.timeline.map((it) => {
        if (it.kind === "approval" && staleApprovalIds.has(it.approvalId) && it.decision === undefined) {
          return { ...it, decision: "resolved" };
        }
        if (it.kind === "question" && staleQuestionIds.has(it.questionId) && it.answer === undefined) {
          return { ...it, answer: "answered" };
        }
        return it;
      }),
    };
  });
}

async function consumeAgentStream(
  runId: string,
  stream: AsyncIterable<ProviderEventDTO>,
  streamRunSeq: number,
  afterSeq: number,
  replayStatus: RunStatus,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  const isStale = () => !shouldApplyRunEvent(get().runId, runId) || get()._streamRunSeq !== streamRunSeq;
  for await (const e of stream) {
    if (isStale()) return;
    if (!isEventForRun(e, runId)) continue;
    if (e.seq <= afterSeq) continue;
    // Skip orchestration events — consumeOrchestrationStream owns agent_graph_updated
    // and agent_bus_message. Processing them here would duplicate agentBusMessages
    // entries when the gap between restore.lastEventSeq and afterSeq is replayed. (BUG-109)
    if (e.type === "agent_graph_updated" || e.type === "agent_bus_message") continue;
    set((s) => applyEvent(s, e));
    settleTerminalReplayVisuals(runId, replayStatus, set);
    set((s) => ({
      _runReplaySeq: {
        ...s._runReplaySeq,
        [runId]: e.seq,
      },
    }));
  }
}

async function consumeOrchestrationStream(
  runId: string,
  stream: AsyncIterable<ProviderEventDTO>,
  orchestrationSeq: number,
  afterSeq: number,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  const isStale = () => !shouldApplyRunEvent(get().mainRunId ?? get().runId, runId) || get()._orchestrationStreamSeq !== orchestrationSeq;
  for await (const e of stream) {
    if (isStale()) return;
    if (!isEventForRun(e, runId)) continue;
    if (e.seq <= afterSeq) continue;
    if (e.type === "agent_graph_updated" || e.type === "agent_bus_message") {
      set((s) => applyOrchestrationEvent(s, e));
      // BUG-180: the flow executor's step transitions (node spawn → RUNNING,
      // cohort-join → DONE, etc.) are store writes with no dedicated event, so the
      // step timeline was stale until a focus switch re-fetched it. Every such
      // transition rides alongside an agent_graph_updated (child lifecycle change),
      // so refresh the step-runtime here to make the timeline update live.
      // refreshWorkflowStepRuntime self-guards (chatMode + load-seq + target-run),
      // so this is safe and de-duped against races.
      if (e.type === "agent_graph_updated") {
        void get().refreshWorkflowStepRuntime();
        void get().refreshAgentRuns();
      }
    } else {
      // CP-35: gate reprompt events (turn_started, message_delta, turn_completed, etc.)
      // arrive after sendTurn() has already closed on turn_completed. Apply them via
      // applyEvent so the timeline shows the reprompt turn without a tab-switch.
      //
      // Dynamic watermark: skip events already covered by the concurrent history replay
      // stream (_runReplaySeq tracks its progress as it goes). Without this guard, when
      // openHistoryRun triggers both a replay stream and this orchestration stream from
      // seq 0, the orchestration stream re-processes flow_gate_violation after
      // _historyReplaying turns false — re-popping the block modal on every chat open.
      // (CP-35 BUG-138)
      if (e.seq <= (get()._runReplaySeq[runId] ?? afterSeq)) continue;
      set((s) => applyEvent(s, e));
    }
  }
}

/**
 * [coding-skill]: keeps the parent orchestration SSE independent from the turn stream.
 * [testing-skill]: isolates the live graph/bus path so store tests can assert late updates.
 */
function startOrchestrationStream(
  runId: string | undefined,
  client: RunnerClient,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): void {
  if (!runId) return;
  cancelOrchestrationStream();
  const orchestrationController = new AbortController();
  activeOrchestrationStreamController = orchestrationController;
  const orchestrationSeq = get()._orchestrationStreamSeq + 1;
  const afterSeq = get()._runReplaySeq[runId] ?? 0;
  set((_s) => ({ _orchestrationStreamSeq: orchestrationSeq }));
  void consumeOrchestrationStream(
    runId,
    client.streamRun(runId, afterSeq, orchestrationController.signal),
    orchestrationSeq,
    afterSeq,
    set,
    get,
  ).finally(() => {
    if (activeOrchestrationStreamController === orchestrationController) {
      activeOrchestrationStreamController = undefined;
    }
  });
}

function isEventForRun(e: ProviderEventDTO, runId: string): boolean {
  if (e.type === "agent_graph_updated") {
    return e.agentGraphSnapshot.parentRunId === runId;
  }
  if (e.type === "agent_bus_message") {
    return e.agentBusMessage.parentRunId === runId;
  }
  return e.workflowRunId === runId;
}

function isTerminalRunStatus(status: RunStatus): boolean {
  return status === "completed" || status === "failed" || status === "cancelled";
}

function hasActiveParentAgentLoop(state: AppState, parentRunId: string): boolean {
  const snapshot = state.agentGraphSnapshot;
  if (!snapshot || snapshot.parentRunId !== parentRunId) return false;
  const status = snapshot.loopState.status;
  return Boolean(status) && status !== "done" && status !== "stopped";
}

function settleTerminalReplayVisuals(
  runId: string,
  replayStatus: RunStatus,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
): void {
  if (!isTerminalRunStatus(replayStatus)) return;
  set((s) => {
    if (s.runId !== runId) return {};
    return {
      status: replayStatus,
      timeline: s.timeline.filter((it) => it.kind !== "thinking"),
    };
  });
}

// Merge an incoming agent-run snapshot into the existing list by runId (incoming wins).
// The live SSE graph snapshot is in-memory only and omits disk-persisted closed children
// that the HTTP list (listAgentRunSummaries) includes; replacing wholesale dropped the
// "Recently closed" entries while an agent was running. Merging preserves them (BUG-132).
//
// BUG-235: a run's status is monotonic once terminal (completed/failed/cancelled never
// goes back to running/waiting) — but incoming can be a STALE snapshot: refreshAgentRuns'
// HTTP request is fire-and-forget and can be captured server-side before a child finished,
// then resolve and land AFTER the SSE agent_graph_updated event that already correctly
// marked it terminal. Unconditional "incoming wins" let that late, stale "running" revert
// the already-correct terminal status — and since no further event fires for an already-
// finished child, it stayed wrongly "running" forever (Agents panel + the leftover
// "reviewer · running" card in the main chat, even after the whole flow completed). Never
// let a non-terminal incoming status overwrite an existing terminal one.
export function mergeAgentRunsById(existing: AgentRunSummary[], incoming: AgentRunSummary[]): AgentRunSummary[] {
  const byId = new Map<string, AgentRunSummary>();
  for (const run of existing) byId.set(run.runId, run);
  for (const run of incoming) {
    const prev = byId.get(run.runId);
    if (prev && isTerminalRunStatus(prev.status) && !isTerminalRunStatus(run.status)) {
      continue;
    }
    byId.set(run.runId, run);
  }
  return [...byId.values()];
}

function applyEvent(s: AppState, e: ProviderEventDTO): Partial<AppState> {
  const next = applyTimelineEvent(s, e);
  const nextReplaySeq = {
    ...s._runReplaySeq,
    [e.workflowRunId]: e.seq,
  };
  if (e.type === "agent_graph_updated") {
    return {
      ...next,
      // Merge (not replace) so disk-persisted closed children stay visible while a new
      // agent runs and emits in-memory-only snapshots (BUG-132).
      agentRuns: mergeAgentRunsById(s.agentRuns, e.agentGraphSnapshot.runs),
      agentGraphSnapshot: e.agentGraphSnapshot,
      agentBusMessages: e.agentGraphSnapshot.busMessages,
      _runReplaySeq: nextReplaySeq,
    };
  }
  if (e.type === "agent_bus_message") {
    return {
      ...next,
      agentBusMessages: [...s.agentBusMessages, e.agentBusMessage],
      agentGraphSnapshot: s.agentGraphSnapshot
        ? { ...s.agentGraphSnapshot, busMessages: [...s.agentGraphSnapshot.busMessages, e.agentBusMessage] }
        : s.agentGraphSnapshot,
      _runReplaySeq: nextReplaySeq,
    };
  }
  if (e.type === "flow_gate_violation" && (e.status === "block" || e.status === "warn") && !s._historyReplaying) {
    // block: hard stop — surface a modal the user must acknowledge. The modal must appear
    // EXACTLY ONCE: Reopening the chat re-streams the persisted flow_gate_violation, and
    // the `_historyReplaying` guard is racy. `_gateBlockedRunIds` is the authoritative guard:
    // added on the first block, cleared on the next turn_started. (CP-35, BUG-138)
    //
    // warn: store settles to "completed" (statusFromEvent), but the backend runner keeps the
    // run in "running" state until the user re-prompts — the same mismatch as block. Without
    // tracking in _gateBlockedRunIds, switching to another chat makes the Navigator fall back
    // to item.status = "running" and show an infinite spinner. (BUG-145)
    //
    // Both cases: add to _gateBlockedRunIds so Navigator shows a stable completed icon for
    // inactive gate-settled runs. Cleared on the next turn_started. (BUG-137)
    const alreadyBlocked = Boolean(s._gateBlockedRunIds[e.workflowRunId]);
    const newGateBlock =
      e.status === "block" && !alreadyBlocked
        ? {
            message: e.error,
            options: e.gateOptions,
            regressedTests: e.gateRegressedTests,
            runId: e.workflowRunId,
          }
        : undefined;
    return {
      ...next,
      ...(newGateBlock ? { gateBlock: newGateBlock } : {}),
      _gateBlockedRunIds: { ...s._gateBlockedRunIds, [e.workflowRunId]: true },
      _runReplaySeq: nextReplaySeq,
    };
  }
  if (e.type === "turn_started") {
    // A fresh turn (incl. a gate reprompt) clears any prior block modal and removes the
    // run from the gate-blocked set (the user re-prompted, so the run is running again).
    const { [e.workflowRunId]: _cleared, ...remainingGateBlockedRunIds } = s._gateBlockedRunIds;
    return {
      ...next,
      latestTokenUsage: undefined,
      gateBlock: undefined,
      _gateBlockedRunIds: remainingGateBlockedRunIds,
      _runReplaySeq: nextReplaySeq,
    };
  }
  if (e.type === "token_usage_updated") {
    return { ...next, latestTokenUsage: e.tokenUsage, _runReplaySeq: nextReplaySeq };
  }
  return { ...next, _runReplaySeq: nextReplaySeq };
}

// Used exclusively by consumeOrchestrationStream. Unlike applyEvent, this does NOT call
// applyTimelineEvent — so the timeline and thinking row are never touched. The orchestration
// stream only needs to update agent graph data; letting it touch the timeline causes a thinking
// row to re-appear after history replay has already settled to a completed state. (BUG-110)
function applyOrchestrationEvent(s: AppState, e: ProviderEventDTO): Partial<AppState> {
  const nextReplaySeq = { ...s._runReplaySeq, [e.workflowRunId]: e.seq };
  if (e.type === "agent_graph_updated") {
    const nextStatus = deriveOrchestrationRunStatus(s.status, e.agentGraphSnapshot);
    return {
      // Merge (not replace) so disk-persisted closed children stay visible (BUG-132).
      agentRuns: mergeAgentRunsById(s.agentRuns, e.agentGraphSnapshot.runs),
      agentGraphSnapshot: e.agentGraphSnapshot,
      agentBusMessages: e.agentGraphSnapshot.busMessages,
      status: nextStatus,
      _runReplaySeq: nextReplaySeq,
    };
  }
  if (e.type === "agent_bus_message") {
    return {
      agentBusMessages: [...s.agentBusMessages, e.agentBusMessage],
      agentGraphSnapshot: s.agentGraphSnapshot
        ? { ...s.agentGraphSnapshot, busMessages: [...s.agentGraphSnapshot.busMessages, e.agentBusMessage] }
        : s.agentGraphSnapshot,
      _runReplaySeq: nextReplaySeq,
    };
  }
  return {};
}

export function deriveOrchestrationRunStatus(current: RunStatus, snapshot: AgentGraphSnapshot): RunStatus {
  const childStatuses = snapshot.runs.map((run) => run.status);
  if (childStatuses.some((status) => status === "waiting_approval")) {
    return "waiting_approval";
  }
  if (childStatuses.some((status) => status === "waiting_question")) {
    return "waiting_question";
  }
  if (
    childStatuses.some(
      (status) => status === "running" || status === "waiting_approval" || status === "waiting_question",
    )
  ) {
    return "running";
  }
  switch (snapshot.loopState.status) {
    case "running":
    case "paused":
      return "running";
    case "blocked":
      // BUG-231: a blocked loop is a deliberate, non-terminal "awaiting user"
      // pause (escalate, or the round cap reached) — it must NOT read as
      // "running", or the composer stays locked ("Waiting for the current
      // turn…") with no way for the very user the flow is waiting on to respond.
      return "blocked";
    case "stopped":
      return "cancelled";
    case "done":
      return current === "failed" || current === "cancelled" ? current : "completed";
    default:
      return current;
  }
}

function runErrorMessage(err: unknown): string {
  if (err instanceof RunnerApiError) {
    const suffix = `HTTP ${err.status}${err.code ? ` / ${err.code}` : ""}`;
    return err.message ? `${err.message} (${suffix})` : `Runner request failed (${suffix})`;
  }
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

function snapshotRunState(state: AppState): RunSnapshot {
  return {
    timeline: state.timeline,
    artifacts: state.artifacts,
    status: state.status,
    pendingApprovals: state.pendingApprovals,
    pendingQuestions: state.pendingQuestions,
    latestTokenUsage: state.latestTokenUsage,
    lastTurnInput: state.lastTurnInput,
    recoverable: state.recoverable,
    _streamingAssistantId: state._streamingAssistantId,
    activeStepId: state.activeStepId,
    lastEventSeq: state._runReplaySeq[state.runId ?? ""] ?? state._runReplaySeq[state.mainRunId ?? ""] ?? undefined,
  };
}

function restoreRunSnapshot(snapshot: RunSnapshot): Partial<AppState> {
  return {
    timeline: snapshot.timeline,
    artifacts: snapshot.artifacts,
    status: snapshot.status,
    pendingApprovals: snapshot.pendingApprovals,
    pendingQuestions: snapshot.pendingQuestions,
    latestTokenUsage: snapshot.latestTokenUsage,
    lastTurnInput: snapshot.lastTurnInput,
    recoverable: snapshot.recoverable,
    _streamingAssistantId: snapshot._streamingAssistantId,
    activeStepId: snapshot.activeStepId,
  };
}

function emptyRunSnapshot(status: RunStatus): Partial<AppState> {
  return {
    timeline: [],
    artifacts: [],
    status,
    pendingApprovals: [],
    pendingQuestions: [],
    gateBlock: undefined,
    latestTokenUsage: undefined,
    lastTurnInput: undefined,
    recoverable: false,
    _streamingAssistantId: undefined,
    activeStepId: undefined,
  };
}

function cacheRunSnapshot(state: AppState, runId?: string): void {
  if (!runId) return;
  state._runSnapshots[runId] = snapshotRunState(state);
}

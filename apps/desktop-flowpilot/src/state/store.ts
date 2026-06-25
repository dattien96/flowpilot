import { create } from "zustand";
import type {
  Artifact,
  AgentDefinition,
  AgentGraphSnapshot,
  AgentRunSummary,
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
} from "@/types/contract";
import type { RunnerClient } from "@/types/contract";
import type { SupportedModel } from "@flowpilot/client-core";
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
  pendingApproval?: PendingApproval;
  pendingQuestion?: PendingQuestion;
  latestTokenUsage?: TokenUsageSnapshot;
  lastTurnInput?: TurnInput;
  recoverable: boolean;
  _streamingAssistantId?: string;
  activeStepId?: string;
  lastEventSeq?: number;
}

function applyAgentGraphSnapshot(snapshot: AgentGraphSnapshot): Partial<AppState> {
  return {
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
  chatStartMode: ChatStartMode;
  chatSourceDocId: string;

  // run
  runId?: string;
  mainRunId?: string;
  activeAgentRunId?: string;
  workspaceMainView: WorkspaceMainView;
  agentRuns: AgentRunSummary[];
  agentGraphSnapshot?: AgentGraphSnapshot;
  agentBusMessages: AgentBusMessage[];
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
  pendingApproval?: PendingApproval;
  pendingQuestion?: PendingQuestion;
  /** A hard-blocking flow-gate violation (e.g. failed tests) the user must acknowledge.
   *  Set only for action === "block"; surfaced as a modal. (CP-35) */
  gateBlock?: { message: string };
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
  loadSkills(provider: string, cwd?: string): Promise<void>;
  selectProject(projectId: string): Promise<void>;
  setLaunchMode(mode: LaunchMode): void;
  setChatMode(mode: ChatMode): void;
  selectProvider(provider?: ProviderKey): void;
  setSelectedModel(model?: string): void;
  setReasoningEffort(effort?: string): void;
  setYoloMode(yolo: boolean): void;
  setChatStartMode(mode: ChatStartMode): void;
  setChatSourceDocId(sourceDocId: string): void;
  selectWorkflow(workflowId: string): Promise<void>;
  selectStep(stepId: string): void;
  setScenario(scenario: ScenarioName): void;
  sendPrompt(prompt: string, skills?: string[], attachments?: PromptAttachment[]): Promise<void>;
  approve(decision: string): Promise<void>;
  answer(choice: string | string[]): Promise<void>;
  stop(): Promise<void>;
  reconnect(): Promise<void>;
  loadRunHistory(): Promise<void>;
  loadRemoteChatSessions(): Promise<void>;
  refreshAgentRuns(): Promise<void>;
  refreshAgentGraph(): Promise<void>;
  pauseAgentLoop(): Promise<void>;
  resumeAgentLoop(): Promise<void>;
  injectAgentFeedback(toRunId: string, message: string): Promise<void>;
  stopAgentLoop(): Promise<void>;
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
  dismissGateBlock(): void;
}

export const useStore = create<AppState>((set, get) => ({
  client: createRunnerClient(),
  projects: [],
  workflows: [],
  steps: [],
  skills: [],
  providerAccounts: [],
  supportedModels: [],
  status: "idle",
  timeline: [],
  artifacts: [],
  runHistory: [],
  remoteChatSessions: [],
  agentRuns: [],
  agentGraphSnapshot: undefined,
  agentBusMessages: [],
  historyLoading: false,
  remoteHistoryLoading: false,
  latestTokenUsage: undefined,
  recoverable: false,
  scenario: "normal",
  accountSwitchLoading: false,
  _accountSwitchTriedIds: [],
  launchMode: "workflow",
  chatMode: "normal_chat",
  selectedProvider: "codex",
  yoloMode: false,
  chatStartMode: "normal",
  chatSourceDocId: "",
  workspaceMainView: "chat",
  _historyReplaying: false,
  _historyLoadSeq: 0,
  _remoteHistoryLoadSeq: 0,
  _agentRunsLoadSeq: 0,
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
        const [workflowDefinitions, stepDefinitions, supportedModels] = await Promise.all([
          admin.workflows.listWorkflows(),
          admin.workflows.listStepDefinitions(),
          admin.providers.listSupportedModels(),
        ]);
        set({
          workflows: workflowDefinitions.map(mapNavigatorWorkflow),
          steps: stepDefinitions.map(mapNavigatorStep),
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
        set({ agentRuns });
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listAgentRuns failed:", err);
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
  async stopAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.stopAgentLoop) set(applyAgentGraphSnapshot(await client.stopAgentLoop(parentRunId))); },

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
    set({
      selectedProjectId: projectId,
      selectedWorkflowId: undefined,
      selectedStepId: undefined,
      runHistory: [],
      remoteChatSessions: [],
      historyLoadError: undefined,
      remoteHistoryLoadError: undefined,
    });
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
    set({ selectedProvider: provider, selectedModel: pickDefaultModel(provider, get().supportedModels) });
    if (provider) {
      void get().loadSkills(provider);
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

  setChatStartMode(mode) {
    set((state) => ({
      chatStartMode: mode,
      chatSourceDocId: mode === "normal" ? "" : state.chatSourceDocId,
    }));
  },

  setChatSourceDocId(sourceDocId) {
    set({ chatSourceDocId: sourceDocId });
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
      selectedProvider, selectedModel, reasoningEffort, yoloMode, chatStartMode, chatSourceDocId,
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

  async approve(decision) {
    const pending = get().pendingApproval;
    if (!pending) return;
    set((s) => ({
      pendingApproval: undefined,
      status: "running",
      timeline: s.timeline.map((it) =>
        it.kind === "approval" && it.approvalId === pending.approvalId ? { ...it, decision } : it,
      ),
    }));
    await get().client.submitApproval(pending.approvalId, decision);
  },

  async answer(choice) {
    const pending = get().pendingQuestion;
    if (!pending) return;
    set((s) => ({
      pendingQuestion: undefined,
      status: "running",
      timeline: s.timeline.map((it) =>
        it.kind === "question" && it.questionId === pending.questionId ? { ...it, answer: choice } : it,
      ),
    }));
    await get().client.answerQuestion(pending.questionId, choice);
  },

  async stop() {
    const { client, runId } = get();
    if (!runId) return;
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
        pendingApproval: undefined,
        pendingQuestion: undefined,
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        pendingAccountSwitch: undefined,
        accountSwitchLoading: false,
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
    set({
      runId: handle.runId,
      mainRunId: handle.runId,
      activeAgentRunId: undefined,
      status: handle.status,
      activeStepId: handle.stepId,
      timeline: [],
      artifacts: [],
      pendingApproval: undefined,
      pendingQuestion: undefined,
      gateBlock: undefined,
      latestTokenUsage: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      pendingAccountSwitch: undefined,
      accountSwitchLoading: false,
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
      agentSpawnGuideOpen: false,
      agentSpawnGuideAgentName: undefined,
      pendingApproval: undefined,
      pendingQuestion: undefined,
      latestTokenUsage: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      pendingAccountSwitch: undefined,
      accountSwitchLoading: false,
      _accountSwitchTriedIds: [],
      _streamingAssistantId: undefined,
      _runSnapshots: {},
      _runReplaySeq: {},
      chatStartMode: "normal",
      chatSourceDocId: "",
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
    if (s.runId !== runId || (!s.pendingApproval && !s.pendingQuestion)) return {};
    const pendingApproval = s.pendingApproval;
    const pendingQuestion = s.pendingQuestion;
    const lastMeaningfulItem = [...s.timeline].reverse().find((it) => it.kind !== "thinking");
    const approvalStillOpen =
      pendingApproval !== undefined &&
      lastMeaningfulItem?.kind === "approval" &&
      lastMeaningfulItem.approvalId === pendingApproval.approvalId &&
      lastMeaningfulItem.decision === undefined;
    const questionStillOpen =
      pendingQuestion !== undefined &&
      lastMeaningfulItem?.kind === "question" &&
      lastMeaningfulItem.questionId === pendingQuestion.questionId &&
      lastMeaningfulItem.answer === undefined;

    if (approvalStillOpen || questionStillOpen) {
      return {
        ...(approvalStillOpen ? { pendingApproval } : { pendingApproval: undefined }),
        ...(questionStillOpen ? { pendingQuestion } : { pendingQuestion: undefined }),
        status: approvalStillOpen ? "waiting_approval" : "waiting_question",
      };
    }

    return {
      pendingApproval: undefined,
      pendingQuestion: undefined,
      timeline: s.timeline.map((it) => {
        if (
          pendingApproval &&
          it.kind === "approval" &&
          it.approvalId === pendingApproval.approvalId &&
          it.decision === undefined
        ) {
          return { ...it, decision: "resolved" };
        }
        if (
          pendingQuestion &&
          it.kind === "question" &&
          it.questionId === pendingQuestion.questionId &&
          it.answer === undefined
        ) {
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
function mergeAgentRunsById(existing: AgentRunSummary[], incoming: AgentRunSummary[]): AgentRunSummary[] {
  const byId = new Map<string, AgentRunSummary>();
  for (const run of existing) byId.set(run.runId, run);
  for (const run of incoming) byId.set(run.runId, run);
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
    return {
      ...next,
      ...(e.status === "block" && !alreadyBlocked ? { gateBlock: { message: e.error } } : {}),
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
    return {
      // Merge (not replace) so disk-persisted closed children stay visible (BUG-132).
      agentRuns: mergeAgentRunsById(s.agentRuns, e.agentGraphSnapshot.runs),
      agentGraphSnapshot: e.agentGraphSnapshot,
      agentBusMessages: e.agentGraphSnapshot.busMessages,
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
    pendingApproval: state.pendingApproval,
    pendingQuestion: state.pendingQuestion,
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
    pendingApproval: snapshot.pendingApproval,
    pendingQuestion: snapshot.pendingQuestion,
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
    pendingApproval: undefined,
    pendingQuestion: undefined,
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

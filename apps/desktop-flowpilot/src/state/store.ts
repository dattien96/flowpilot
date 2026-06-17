import { create } from "zustand";
import type {
  Artifact,
  Project,
  ProviderAccountSummary,
  ProviderEventDTO,
  ProviderKey,
  ProviderSkill,
  PromptAttachment,
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

let loadProjectsInFlight: Promise<void> | null = null;

export type LaunchMode = "workflow" | "step";
export type ChatMode = "normal_chat" | "workflow_step_auto";

function selectedProjectPath(state: Pick<AppState, "projects" | "selectedProjectId">): string | undefined {
  return state.projects.find((project) => project.id === state.selectedProjectId)?.path;
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

  // run
  runId?: string;
  /** Step id for the active run's turns; the synthetic chat step in normal_chat. */
  activeStepId?: string;
  status: RunStatus;
  timeline: TimelineItem[];
  artifacts: Artifact[];
  runHistory: RunHistoryItem[];
  historyOpen: boolean;
  historyLoading: boolean;
  historyLoadError?: string;
  pendingApproval?: PendingApproval;
  pendingQuestion?: PendingQuestion;
  lastTurnInput?: TurnInput;
  latestTokenUsage?: TokenUsageSnapshot;
  recoverable: boolean;
  scenario: ScenarioName;

  // internal: id of the assistant bubble currently accumulating deltas
  _streamingAssistantId?: string;
  // stale-response guard for loadRunHistory (BUG-060 F-3)
  _historyLoadSeq: number;

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
  selectWorkflow(workflowId: string): Promise<void>;
  selectStep(stepId: string): void;
  setScenario(scenario: ScenarioName): void;
  sendPrompt(prompt: string, skills?: string[], attachments?: PromptAttachment[]): Promise<void>;
  approve(decision: string): Promise<void>;
  answer(choice: string | string[]): Promise<void>;
  stop(): Promise<void>;
  reconnect(): Promise<void>;
  loadRunHistory(): Promise<void>;
  toggleRunHistory(): Promise<void>;
  openHistoryRun(runId: string): Promise<void>;
  resetRun(): void;
  openInIde(path: string, line?: number): void;
  openAdminWeb(): void;
  restartSystem(): Promise<void>;
  shutdownSystem(): Promise<void>;
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
  historyOpen: false,
  historyLoading: false,
  latestTokenUsage: undefined,
  recoverable: false,
  scenario: "normal",
  launchMode: "workflow",
  chatMode: "normal_chat",
  selectedProvider: "codex",
  yoloMode: false,
  _historyLoadSeq: 0,

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

  async selectProject(projectId) {
    set({
      selectedProjectId: projectId,
      selectedWorkflowId: undefined,
      selectedStepId: undefined,
      runHistory: [],
      historyOpen: false,
      historyLoadError: undefined,
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
    set({ selectedProvider: provider });
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
      selectedProvider, selectedModel, reasoningEffort, yoloMode,
    } = get();
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
      }

      const turnInput: TurnInput = {
        runId,
        stepId: turnStepId,
        prompt,
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
      set({ runId, lastTurnInput: turnInput, activeStepId: turnStepId });

      await consumeStream(runId, client.sendTurn(turnInput), set, get);
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
      if (get().historyOpen) {
        void get().loadRunHistory();
      }
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
    set({ timeline: [], recoverable: false, status: "running", _streamingAssistantId: undefined });
    await consumeStream(runId, client.streamRun(runId, 0), set, get);
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

  async toggleRunHistory() {
    const open = !get().historyOpen;
    set({ historyOpen: open });
    if (open) {
      await get().loadRunHistory();
    }
  },

  async openHistoryRun(runId) {
    const { client } = get();
    const historyItem = get().runHistory.find((item) => item.runId === runId);
    const handle = await client.resumeRun(runId);
    set({
      runId: handle.runId,
      status: handle.status,
      activeStepId: handle.stepId,
      timeline: [],
      artifacts: [],
      pendingApproval: undefined,
      pendingQuestion: undefined,
      latestTokenUsage: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      historyOpen: false,
      _streamingAssistantId: undefined,
      ...(historyItem ? { selectedProvider: historyItem.providerKey } : {}),
    });
    if (historyItem?.providerKey) {
      void get().loadSkills(historyItem.providerKey);
    }
    await consumeStream(handle.runId, client.streamRun(handle.runId, 0), set, get);

    // Post-stream stale cleanup (BUG-074): permission_required events are persisted
    // in the event log but their resolution (approve() action) only clears
    // pendingApproval on the client — no resolution event is emitted. If the stream
    // ends and pendingApproval is still set, and the run is not genuinely waiting for
    // approval (handle.status is the server's source of truth), stamp all unresolved
    // approval cards as resolved and clear the stale pending state.
    if (handle.status !== "waiting_approval" && handle.status !== "waiting_question") {
      set((s) => {
        if (!s.pendingApproval && !s.pendingQuestion) return {};
        return {
          pendingApproval: undefined,
          pendingQuestion: undefined,
          timeline: s.timeline.map((it) => {
            if (it.kind === "approval" && it.decision === undefined) {
              return { ...it, decision: "resolved" };
            }
            if (it.kind === "question" && it.answer === undefined) {
              return { ...it, answer: "answered" };
            }
            return it;
          }),
        };
      });
    }
  },

  resetRun() {
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
      historyOpen: false,
      _streamingAssistantId: undefined,
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
}));

// Consumes a turn stream and folds each event into the timeline + status.
async function consumeStream(
  runId: string,
  stream: AsyncIterable<ProviderEventDTO>,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  for await (const e of stream) {
    if (!shouldApplyRunEvent(get().runId, runId)) {
      return;
    }
    set((s) => applyEvent(s, e));
  }
  if (!shouldApplyRunEvent(get().runId, runId)) {
    return;
  }
  // settle recoverable flag for the Reconnect affordance
  const last = get().timeline[get().timeline.length - 1];
  if (last?.kind === "system" && last.tone === "error") {
    // handled in applyEvent
  }
}

function applyEvent(s: AppState, e: ProviderEventDTO): Partial<AppState> {
  const next = applyTimelineEvent(s, e);
  if (e.type === "turn_started") {
    return { ...next, latestTokenUsage: undefined };
  }
  if (e.type === "token_usage_updated") {
    return { ...next, latestTokenUsage: e.tokenUsage };
  }
  return next;
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

import { create } from "zustand";
import type {
  Artifact,
  Project,
  ProviderAccountSummary,
  ProviderEventDTO,
  ProviderSkill,
  RunHistoryItem,
  RunStatus,
  Step,
  TurnInput,
  Workflow,
} from "@/types/contract";
import type { RunnerClient } from "@/types/contract";
import { createRunnerClient } from "@/client/createRunnerClient";
import { RunnerApiError } from "@/client/HttpWsRunnerClient";
import type { ScenarioName } from "@/client/mockData";
import { ideBridge } from "@/client/ideBridge";
import { ADMIN_WEB_URL } from "@/config";
import {
  applyTimelineEvent,
  type PendingApproval,
  type PendingQuestion,
  type TimelineItem,
} from "./timelineReducer";

export type { TimelineItem } from "./timelineReducer";

let loadProjectsInFlight: Promise<void> | null = null;

export type LaunchMode = "workflow" | "step";

interface AppState {
  client: RunnerClient;

  // navigator
  projects: Project[];
  workflows: Workflow[];
  steps: Step[];
  skills: ProviderSkill[];
  providerAccounts: ProviderAccountSummary[];
  selectedProjectId?: string;
  selectedWorkflowId?: string;
  selectedStepId?: string;
  launchMode: LaunchMode;

  // run
  runId?: string;
  status: RunStatus;
  timeline: TimelineItem[];
  artifacts: Artifact[];
  runHistory: RunHistoryItem[];
  historyOpen: boolean;
  historyLoading: boolean;
  pendingApproval?: PendingApproval;
  pendingQuestion?: PendingQuestion;
  lastTurnInput?: TurnInput;
  recoverable: boolean;
  scenario: ScenarioName;

  // internal: id of the assistant bubble currently accumulating deltas
  _streamingAssistantId?: string;

  // actions
  loadProjects(): Promise<void>;
  loadProviderAccounts(): Promise<void>;
  selectProject(projectId: string): Promise<void>;
  setLaunchMode(mode: LaunchMode): void;
  selectWorkflow(workflowId: string): Promise<void>;
  selectStep(stepId: string): void;
  setScenario(scenario: ScenarioName): void;
  sendPrompt(prompt: string, skills?: string[]): Promise<void>;
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
  status: "idle",
  timeline: [],
  artifacts: [],
  runHistory: [],
  historyOpen: false,
  historyLoading: false,
  recoverable: false,
  scenario: "normal",
  launchMode: "workflow",

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
        const workflows = await withRetry(() => client.listWorkflows());
        set({ workflows });
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] listWorkflows failed:", err);
      }
      try {
        const steps = await withRetry(() => client.listSteps());
        set({ steps });
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error("[FlowPilot] listSteps failed:", err);
      }
      try {
        const skills = await withRetry(() => client.listSkills("codex"));
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
    set({ selectedProjectId: projectId, runHistory: [], historyOpen: false });
  },

  setLaunchMode(mode) {
    set({
      launchMode: mode,
    });
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

  async sendPrompt(prompt, skills) {
    const { client, launchMode, selectedProjectId, selectedWorkflowId, selectedStepId } = get();
    const launchTargetId = launchMode === "workflow" ? selectedWorkflowId : selectedStepId;
    if (!selectedProjectId || !launchTargetId) return;

    let runId = get().runId;
    if (!runId) {
      const handle = await client.startRun({
        projectId: selectedProjectId,
        workflowId: launchMode === "workflow" ? selectedWorkflowId : undefined,
        stepId: launchTargetId,
      });
      runId = handle.runId;
    }

    const turnInput: TurnInput = {
      runId,
      stepId: launchTargetId,
      prompt,
      selectedSkills:
        skills && skills.length > 0
          ? skills.map((name) => ({ name, source: "slash_picker" as const }))
          : undefined,
    };

    set((s) => ({
      runId,
      lastTurnInput: turnInput,
      recoverable: false,
      status: "running",
      _streamingAssistantId: undefined,
      timeline: [
        ...s.timeline,
        { kind: "prompt", id: `prompt-${s.timeline.length}`, text: prompt },
        { kind: "thinking", id: `thinking-${s.timeline.length}`, text: "Thinking..." },
      ],
    }));

    try {
      await consumeStream(client.sendTurn(turnInput), set, get);
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
    await consumeStream(client.streamRun(runId, 0), set, get);
  },

  async loadRunHistory() {
    const { client, selectedProjectId } = get();
    if (!selectedProjectId) {
      set({ runHistory: [], historyLoading: false });
      return;
    }
    set({ historyLoading: true });
    try {
      const runHistory = await client.listRunHistory(selectedProjectId);
      set({ runHistory, historyLoading: false });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listRunHistory failed:", err);
      set((s) => ({
        historyLoading: false,
        timeline: [
          ...s.timeline,
          { kind: "system", id: `err-history-${s.timeline.length}`, text: `Failed to load run history: ${String(err)}`, tone: "error" },
        ],
      }));
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
    const handle = await client.resumeRun(runId);
    set({
      runId: handle.runId,
      status: handle.status,
      timeline: [],
      artifacts: [],
      pendingApproval: undefined,
      pendingQuestion: undefined,
      lastTurnInput: undefined,
      recoverable: false,
      historyOpen: false,
      _streamingAssistantId: undefined,
    });
    await consumeStream(client.streamRun(runId, 0), set, get);
  },

  resetRun() {
    set({
      runId: undefined,
      status: "idle",
      timeline: [],
      artifacts: [],
      pendingApproval: undefined,
      pendingQuestion: undefined,
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
  stream: AsyncIterable<ProviderEventDTO>,
  set: (fn: (s: AppState) => Partial<AppState>) => void,
  get: () => AppState,
): Promise<void> {
  for await (const e of stream) {
    set((s) => applyEvent(s, e));
  }
  // settle recoverable flag for the Reconnect affordance
  const last = get().timeline[get().timeline.length - 1];
  if (last?.kind === "system" && last.tone === "error") {
    // handled in applyEvent
  }
}

function applyEvent(s: AppState, e: ProviderEventDTO): Partial<AppState> {
  return applyTimelineEvent(s, e);
}

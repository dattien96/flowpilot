import { create } from "zustand";
import type {
  ApprovalDetails,
  Artifact,
  Project,
  ProviderEventDTO,
  ProviderSkill,
  QuestionOption,
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

// ---- Timeline item model (what the renderer draws) -------------------------

export type TimelineItem =
  | { kind: "assistant"; id: string; text: string; finalized: boolean }
  | { kind: "tool"; id: string; toolName: string; status: "running" | "success" | "failed" | "cancelled"; input?: unknown; output?: unknown }
  | { kind: "file"; id: string; path: string; changeType?: string }
  | { kind: "approval"; id: string; approvalId: string; details: ApprovalDetails; decision?: string }
  | { kind: "question"; id: string; questionId: string; prompt: string; options: QuestionOption[]; multiSelect?: boolean; answer?: string | string[] }
  | { kind: "system"; id: string; text: string; tone: "info" | "error" };

interface PendingApproval {
  approvalId: string;
  details: ApprovalDetails;
}
interface PendingQuestion {
  questionId: string;
  prompt: string;
  options: QuestionOption[];
  multiSelect?: boolean;
}

interface AppState {
  client: RunnerClient;

  // navigator
  projects: Project[];
  workflows: Workflow[];
  steps: Step[];
  skills: ProviderSkill[];
  selectedProjectId?: string;
  selectedWorkflowId?: string;
  selectedStepId?: string;

  // run
  runId?: string;
  status: RunStatus;
  timeline: TimelineItem[];
  artifacts: Artifact[];
  pendingApproval?: PendingApproval;
  pendingQuestion?: PendingQuestion;
  lastTurnInput?: TurnInput;
  recoverable: boolean;
  scenario: ScenarioName;

  // internal: id of the assistant bubble currently accumulating deltas
  _streamingAssistantId?: string;

  // actions
  loadProjects(): Promise<void>;
  selectProject(projectId: string): Promise<void>;
  selectWorkflow(workflowId: string): Promise<void>;
  selectStep(stepId: string): void;
  setScenario(scenario: ScenarioName): void;
  sendPrompt(prompt: string, skills?: string[]): Promise<void>;
  approve(decision: string): Promise<void>;
  answer(choice: string | string[]): Promise<void>;
  stop(): Promise<void>;
  reconnect(): Promise<void>;
  resetRun(): void;
  openInIde(path: string, line?: number): void;
  openAdminWeb(): void;
  restartSystem(): Promise<void>;
  shutdownSystem(): Promise<void>;
}

function statusFromEvent(e: ProviderEventDTO, prev: RunStatus): RunStatus {
  switch (e.type) {
    case "turn_started":
      return "running";
    case "permission_required":
      return "waiting_approval";
    case "user_question_required":
      return "waiting_question";
    case "turn_completed":
      return "completed";
    case "turn_failed":
      return "failed";
    default:
      return prev === "waiting_approval" || prev === "waiting_question" ? "running" : prev;
  }
}

export const useStore = create<AppState>((set, get) => ({
  client: createRunnerClient(),
  projects: [],
  workflows: [],
  steps: [],
  skills: [],
  status: "idle",
  timeline: [],
  artifacts: [],
  recoverable: false,
  scenario: "normal",

  async loadProjects() {
    const client = get().client;
    // The runner starts via `go run`, which compiles first (~10-30s) before it
    // listens — so the first fetches can hit connection-refused ("Failed to fetch").
    // Retry ONLY connection-level errors (not HTTP errors like 502, which won't fix
    // themselves) so the navigator fills in once the runner is up, without a manual
    // reload. Load projects/skills independently and surface a final error.
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
      set({ projects });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listProjects failed:", err);
      set((s) => ({
        timeline: [
          ...s.timeline,
          { kind: "system", id: `err-projects-${s.timeline.length}`, text: `Failed to load projects: ${String(err)}`, tone: "error" },
        ],
      }));
    }
    try {
      const skills = await withRetry(() => client.listSkills("codex"));
      set({ skills });
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error("[FlowPilot] listSkills failed:", err);
    }
  },

  async selectProject(projectId) {
    const workflows = await get().client.listWorkflows(projectId);
    set({ selectedProjectId: projectId, selectedWorkflowId: undefined, selectedStepId: undefined, workflows, steps: [] });
  },

  async selectWorkflow(workflowId) {
    const steps = await get().client.listSteps(workflowId);
    set({ selectedWorkflowId: workflowId, selectedStepId: steps[0]?.id, steps });
  },

  selectStep(stepId) {
    set({ selectedStepId: stepId });
  },

  setScenario(scenario) {
    get().client.setScenario?.(scenario);
    set({ scenario });
  },

  async sendPrompt(prompt, skills) {
    const { client, selectedProjectId, selectedWorkflowId, selectedStepId } = get();
    if (!selectedProjectId || !selectedWorkflowId || !selectedStepId) return;

    let runId = get().runId;
    if (!runId) {
      const handle = await client.startRun({
        projectId: selectedProjectId,
        workflowId: selectedWorkflowId,
        stepId: selectedStepId,
      });
      runId = handle.runId;
    }

    const turnInput: TurnInput = {
      runId,
      stepId: selectedStepId,
      prompt,
      selectedSkills:
        skills && skills.length > 0
          ? skills.map((name) => ({ name, source: "slash_picker" as const }))
          : undefined,
    };

    // Echo the user's prompt into the timeline as a system line for context.
    set((s) => ({
      runId,
      lastTurnInput: turnInput,
      recoverable: false,
      status: "running",
      _streamingAssistantId: undefined,
      timeline: [...s.timeline, { kind: "system", id: `user-${s.timeline.length}`, text: `▸ ${prompt}`, tone: "info" }],
    }));

    await consumeStream(client.sendTurn(turnInput), set, get);
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
  const status = statusFromEvent(e, s.status);
  const timeline = [...s.timeline];
  let streamingAssistantId = s._streamingAssistantId;

  const closeAssistant = () => {
    streamingAssistantId = undefined;
  };

  switch (e.type) {
    case "turn_started":
      break;

    case "message_delta": {
      if (streamingAssistantId) {
        const idx = timeline.findIndex((it) => it.id === streamingAssistantId);
        if (idx >= 0 && timeline[idx].kind === "assistant") {
          const cur = timeline[idx] as Extract<TimelineItem, { kind: "assistant" }>;
          timeline[idx] = { ...cur, text: cur.text + e.text };
        }
      } else {
        const id = e.id;
        streamingAssistantId = id;
        timeline.push({ kind: "assistant", id, text: e.text, finalized: false });
      }
      break;
    }

    case "message_completed": {
      if (streamingAssistantId) {
        const idx = timeline.findIndex((it) => it.id === streamingAssistantId);
        if (idx >= 0 && timeline[idx].kind === "assistant") {
          timeline[idx] = { kind: "assistant", id: streamingAssistantId, text: e.text, finalized: true };
        }
      } else {
        timeline.push({ kind: "assistant", id: e.id, text: e.text, finalized: true });
      }
      closeAssistant();
      break;
    }

    case "tool_started":
      closeAssistant();
      timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: "running", input: e.input });
      break;

    case "tool_completed": {
      closeAssistant();
      // update the most recent running tool with the same name
      for (let i = timeline.length - 1; i >= 0; i--) {
        const it = timeline[i];
        if (it.kind === "tool" && it.toolName === e.toolName && it.status === "running") {
          timeline[i] = { ...it, status: e.status, output: e.output };
          return { timeline, status, _streamingAssistantId: streamingAssistantId };
        }
      }
      timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: e.status, output: e.output });
      break;
    }

    case "file_changed":
      closeAssistant();
      timeline.push({ kind: "file", id: e.id, path: e.path, changeType: e.changeType });
      break;

    case "permission_required":
      closeAssistant();
      timeline.push({ kind: "approval", id: e.id, approvalId: e.approvalId, details: e.details });
      return {
        timeline,
        status,
        _streamingAssistantId: streamingAssistantId,
        pendingApproval: { approvalId: e.approvalId, details: e.details },
      };

    case "user_question_required":
      closeAssistant();
      timeline.push({
        kind: "question",
        id: e.id,
        questionId: e.questionId,
        prompt: e.prompt,
        options: e.options,
        multiSelect: e.multiSelect,
      });
      return {
        timeline,
        status,
        _streamingAssistantId: streamingAssistantId,
        pendingQuestion: { questionId: e.questionId, prompt: e.prompt, options: e.options, multiSelect: e.multiSelect },
      };

    case "turn_completed":
      closeAssistant();
      timeline.push({ kind: "system", id: e.id, text: "Turn completed.", tone: "info" });
      break;

    case "turn_failed":
      closeAssistant();
      timeline.push({ kind: "system", id: e.id, text: e.error, tone: "error" });
      return { timeline, status, _streamingAssistantId: streamingAssistantId, recoverable: e.recoverable };
  }

  return { timeline, status, _streamingAssistantId: streamingAssistantId };
}

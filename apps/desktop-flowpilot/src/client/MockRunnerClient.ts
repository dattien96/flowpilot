import type {
  Artifact,
  Project,
  ProviderEventBaseDTO,
  ProviderEventDTO,
  ProviderSkill,
  RunHandle,
  RunnerClient,
  StartRunInput,
  Step,
  TurnInput,
  Workflow,
} from "@/types/contract";
import {
  MOCK_PROJECTS,
  MOCK_SKILLS,
  MOCK_STEPS,
  MOCK_WORKFLOWS,
  mockArtifacts,
  scriptFor,
  type ScenarioName,
  type ScriptStep,
} from "./mockData";

const delay = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));

let seq = 0;
const nextId = (prefix: string): string => `${prefix}-${++seq}`;

interface RunState {
  runId: string;
  providerSessionId: string;
  providerTurnId: string;
  /** Monotonic per-run event sequence (reconnect cursor, 04-02). */
  seq: number;
  lastTurnInput?: TurnInput;
}

/**
 * Phase 1 mock implementation of the RunnerClient contract (04-01 Part A).
 *
 * Streams scripted ProviderEventDTOs on a timer. Approval/question scenarios
 * block the stream on a gate that submitApproval/answerQuestion resolve — the
 * same pause/resume shape the real runner uses (04-04), so the renderer code is
 * identical for Part B.
 */
export class MockRunnerClient implements RunnerClient {
  /** Current scenario the dev scenario-switcher selects. */
  scenario: ScenarioName = "normal";

  private readonly runs = new Map<string, RunState>();
  private readonly approvalGates = new Map<string, (decision: string) => void>();
  private readonly questionGates = new Map<string, (choice: string | string[]) => void>();
  private readonly replayRuns = new Set<string>();
  /** Persisted per-run event log so streamRun can replay on reconnect. */
  private readonly eventLog = new Map<string, ProviderEventDTO[]>();
  /** Runs an interrupt has been requested for. */
  private readonly aborted = new Set<string>();
  /** Cancels the pending approval/question gate for a run (used by interrupt). */
  private readonly pendingGateCancel = new Map<string, () => void>();

  setScenario(scenario: ScenarioName): void {
    this.scenario = scenario;
  }

  private rec(runId: string, ev: ProviderEventDTO): ProviderEventDTO {
    const log = this.eventLog.get(runId);
    if (log) log.push(ev);
    else this.eventLog.set(runId, [ev]);
    return ev;
  }

  async interrupt(runId: string): Promise<void> {
    this.aborted.add(runId);
    const cancel = this.pendingGateCancel.get(runId);
    if (cancel) cancel();
  }

  async *streamRun(runId: string, afterSeq = 0): AsyncIterable<ProviderEventDTO> {
    const log = this.eventLog.get(runId) ?? [];
    for (const ev of log) {
      if (ev.seq > afterSeq) {
        await delay(40); // small pause so the rebuilt timeline is visible
        yield ev;
      }
    }
  }

  async listProjects(): Promise<Project[]> {
    await delay(60);
    return MOCK_PROJECTS;
  }

  async listWorkflows(): Promise<Workflow[]> {
    await delay(60);
    return Object.values(MOCK_WORKFLOWS).flat();
  }

  async listSteps(): Promise<Step[]> {
    await delay(60);
    return Object.values(MOCK_STEPS).flat().map((step, index) => ({ ...step, workflowId: undefined, order: index + 1 }));
  }

  async listSkills(_provider: string): Promise<ProviderSkill[]> {
    await delay(40);
    return MOCK_SKILLS;
  }

  async listArtifacts(runId: string): Promise<Artifact[]> {
    await delay(60);
    return mockArtifacts(runId);
  }

  async restartStack(): Promise<void> {
    // Part A stub. Part B → POST /system/restart on the runner.
    await delay(120);
    // eslint-disable-next-line no-console
    console.log("[MockRunnerClient] restartStack (stub)");
  }

  async shutdownStack(): Promise<void> {
    // Part A stub. Part B → POST /system/shutdown on the runner.
    await delay(120);
    // eslint-disable-next-line no-console
    console.log("[MockRunnerClient] shutdownStack (stub)");
  }

  async startRun(input: StartRunInput): Promise<RunHandle> {
    await delay(80);
    const runId = nextId("run");
    const providerSessionId = nextId("thread");
    this.runs.set(runId, { runId, providerSessionId, providerTurnId: "", seq: 0 });
    return { runId, providerSessionId, providerKey: "codex", status: "running" };
  }

  async resumeRun(runId: string): Promise<RunHandle> {
    await delay(80);
    const state = this.runs.get(runId);
    const providerSessionId = state?.providerSessionId ?? nextId("thread");
    // Mark this run so the next sendTurn streams the full happy path (replay).
    this.replayRuns.add(runId);
    return { runId, providerSessionId, providerKey: "codex", status: "running" };
  }

  async submitApproval(approvalId: string, decision: string): Promise<void> {
    const resolve = this.approvalGates.get(approvalId);
    if (resolve) {
      this.approvalGates.delete(approvalId);
      resolve(decision);
    }
  }

  async answerQuestion(questionId: string, choice: string | string[]): Promise<void> {
    const resolve = this.questionGates.get(questionId);
    if (resolve) {
      this.questionGates.delete(questionId);
      resolve(choice);
    }
  }

  async *sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO> {
    const state = this.runs.get(input.runId) ?? {
      runId: input.runId,
      providerSessionId: nextId("thread"),
      providerTurnId: "",
      seq: 0,
    };
    state.providerTurnId = nextId("turn");
    state.lastTurnInput = input;
    this.runs.set(input.runId, state);

    const isReplay = this.replayRuns.has(input.runId);
    if (isReplay) this.replayRuns.delete(input.runId);
    this.aborted.delete(input.runId);

    const base = (): ProviderEventBaseDTO => ({
      id: nextId("evt"),
      workflowRunId: input.runId,
      workflowStepRunId: input.stepId,
      providerSessionId: state.providerSessionId,
      providerKey: "codex",
      providerTurnId: state.providerTurnId,
      seq: ++state.seq,
      occurredAt: new Date().toISOString(),
    });

    // turn_started always leads.
    await delay(120);
    yield this.rec(input.runId, { ...base(), type: "turn_started", providerTurnId: state.providerTurnId });

    const steps = scriptFor(this.scenario, { replay: isReplay });
    // Record every scripted event into the per-run log as it streams.
    for await (const ev of this.runSteps(steps, base, input.runId)) {
      yield this.rec(input.runId, ev);
    }
  }

  private async *runSteps(
    steps: ScriptStep[],
    base: () => ProviderEventBaseDTO,
    runId: string,
  ): AsyncIterable<ProviderEventDTO> {
    for (const step of steps) {
      await delay(step.delay);
      if (this.aborted.has(runId)) {
        this.aborted.delete(runId);
        yield { ...base(), type: "turn_failed", error: "interrupted by user", recoverable: true };
        return;
      }
      switch (step.kind) {
        case "delta":
          yield { ...base(), type: "message_delta", text: step.text };
          break;
        case "tool_started":
          yield { ...base(), type: "tool_started", toolName: step.toolName, input: step.input };
          break;
        case "tool_completed":
          yield { ...base(), type: "tool_completed", toolName: step.toolName, status: step.status, output: step.output };
          break;
        case "file_changed":
          yield { ...base(), type: "file_changed", path: step.path, changeType: step.changeType };
          break;
        case "completed":
          yield { ...base(), type: "message_completed", text: step.finalMessage };
          yield { ...base(), type: "turn_completed", finalMessage: step.finalMessage };
          return;
        case "failed":
          yield { ...base(), type: "turn_failed", error: step.error, recoverable: step.recoverable };
          return;
        case "disconnect":
          // Simulate a dropped stream: recoverable failure, then stop. The UI
          // offers Reconnect, which calls resumeRun + sendTurn to replay.
          yield { ...base(), type: "turn_failed", error: step.error, recoverable: true };
          return;
        case "await_approval": {
          const approvalId = nextId("appr");
          yield {
            ...base(),
            type: "permission_required",
            approvalId,
            provider: "codex",
            details: step.details,
          };
          const decision = await new Promise<string>((resolve) => {
            this.approvalGates.set(approvalId, resolve);
            this.pendingGateCancel.set(runId, () => resolve("__cancelled__"));
          });
          this.approvalGates.delete(approvalId);
          this.pendingGateCancel.delete(runId);
          if (decision === "__cancelled__" || this.aborted.has(runId)) {
            this.aborted.delete(runId);
            yield { ...base(), type: "turn_failed", error: "interrupted by user", recoverable: true };
            return;
          }
          const cont = decision === "deny" ? step.onDeny : step.onApprove;
          yield* this.runSteps(cont, base, runId);
          return;
        }
        case "await_question": {
          const questionId = nextId("q");
          yield {
            ...base(),
            type: "user_question_required",
            questionId,
            prompt: step.prompt,
            options: step.options,
            multiSelect: step.multiSelect,
          };
          const choice = await new Promise<string | string[]>((resolve) => {
            this.questionGates.set(questionId, resolve);
            this.pendingGateCancel.set(runId, () => resolve("__cancelled__"));
          });
          this.questionGates.delete(questionId);
          this.pendingGateCancel.delete(runId);
          if (choice === "__cancelled__" || this.aborted.has(runId)) {
            this.aborted.delete(runId);
            yield { ...base(), type: "turn_failed", error: "interrupted by user", recoverable: true };
            return;
          }
          yield* this.runSteps(step.onAnswer(choice), base, runId);
          return;
        }
      }
    }
  }
}

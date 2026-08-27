import type {
  Artifact,
  AgentDefinition,
  AgentRunSummary,
  AgentGraphSnapshot,
  BuiltinFlowOption,
  ChatPostureConfig,
  ChatPostureProfile,
  ChatSessionRestoreRequest,
  ChatSessionRestoreResult,
  ChatSessionSyncRequest,
  ChatSessionSyncResult,
  HandoffContextRequest,
  HandoffContextResponse,
  ChatSummaryResult,
  Project,
  ProviderAccountSummary,
  ProviderEventBaseDTO,
  ProviderEventDTO,
  ProviderSkill,
  RemoteChatSessionSummary,
  ReviewOutcomeInput,
  SpawnAgentInput,
  SpawnAgentResult,
  RunHandle,
  RunHistoryItem,
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

const MOCK_PROVIDER_ACCOUNTS: ProviderAccountSummary[] = [
  {
    id: "acct-codex-1",
    providerKey: "codex",
    displayName: "Account 1",
    displayLabel: "codex.dev@example.com",
    homePath: "/Users/demo/.codexHome1",
    authStorePath: "/Users/demo/.codexHome1/.codex",
    slotIndex: 1,
    authStatus: "connected",
    isActive: true,
    createdAt: "2026-06-10T10:00:00.000Z",
    lastAuthenticatedAt: "2026-06-12T08:00:00.000Z",
    accountEmail: "codex.dev@example.com",
    accountName: "Codex Dev",
    usageSummary: "plus until 2026-06-30",
    remaining5hPercent: 56,
    remaining7dPercent: 92,
    remaining5hResetAt: "2026-06-13T06:25:00.000Z",
    remaining7dResetAt: "2026-06-18T15:11:00.000Z",
    usageSource: "provider_api",
    accessTokenExpiresAt: "2026-06-20T09:20:00.000Z",
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: "Unknown. Re-login is required only when a refresh attempt fails.",
    usageDetailLines: [
      { label: "Remaining 5h", remainingPercent: 56, resetAt: "2026-06-13T06:25:00.000Z" },
      { label: "Remaining 7d", remainingPercent: 92, resetAt: "2026-06-18T15:11:00.000Z" },
    ],
  },
  {
    id: "acct-claude-1",
    providerKey: "claude",
    displayName: "Account 1",
    displayLabel: "claude.user@example.com",
    homePath: "/Users/demo/.claudeHome1",
    authStorePath: "/Users/demo/.claudeHome1/.claude",
    slotIndex: 1,
    authStatus: "connected",
    isActive: true,
    createdAt: "2026-06-09T10:00:00.000Z",
    lastAuthenticatedAt: "2026-06-12T07:30:00.000Z",
    accountEmail: "claude.user@example.com",
    accountName: "Claude User",
    usageSummary: "pro since 2026-05-30 | extra usage enabled",
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "unavailable",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  },
  {
    id: "acct-gemini-1",
    providerKey: "gemini",
    displayName: "Account 1",
    displayLabel: "gemini.user@example.com",
    homePath: "/Users/demo/.geminiHome1",
    authStorePath: "/Users/demo/.geminiHome1/.gemini",
    slotIndex: 1,
    authStatus: "connected",
    isActive: true,
    createdAt: "2026-06-08T10:00:00.000Z",
    lastAuthenticatedAt: "2026-06-12T07:00:00.000Z",
    accountEmail: "gemini.user@example.com",
    accountName: "Gemini User",
    usageSummary: "Google AI Pro",
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "provider_api",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [{ label: "Remaining quota", remainingPercent: 73, resetAt: "2026-06-13T00:00:00.000Z" }],
  },
  {
    // Appended last (CP-46 P-0/Task-211 T-11).
    id: "acct-grok-1",
    providerKey: "grok",
    displayName: "Account 1",
    displayLabel: "grok.user@example.com",
    homePath: "/Users/demo/.grok",
    authStorePath: "/Users/demo/.grok",
    slotIndex: 0,
    authStatus: "connected",
    isActive: true,
    createdAt: "2026-07-09T10:00:00.000Z",
    lastAuthenticatedAt: "2026-07-09T10:00:00.000Z",
    accountEmail: "grok.user@example.com",
    accountName: "Grok User",
    usageSummary: "Personal",
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "provider_api",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  },
  {
    // Appended last (CP-57 P-0/Task-302 T-2).
    id: "acct-opencode-1",
    providerKey: "opencode",
    displayName: "Account 1",
    displayLabel: "opencode.user@example.com",
    homePath: "/Users/demo/.config/opencode",
    authStorePath: "/Users/demo/.config/opencode/auth.json",
    slotIndex: 0,
    authStatus: "connected",
    isActive: true,
    createdAt: "2026-08-27T10:00:00.000Z",
    lastAuthenticatedAt: "2026-08-27T10:00:00.000Z",
    accountEmail: "opencode.user@example.com",
    accountName: "Opencode User",
    usageSummary: "Zen Proxy",
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "provider_api",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [{ label: "cost: $0.00", remainingPercent: 0, resetAt: null }],
  },
  {
    id: "acct-codex-2",
    providerKey: "codex",
    displayName: "Account 2",
    displayLabel: "backup.codex@example.com",
    homePath: "/Users/demo/.codexHome2",
    authStorePath: "/Users/demo/.codexHome2/.codex",
    slotIndex: 2,
    authStatus: "failed",
    isActive: false,
    createdAt: "2026-06-01T10:00:00.000Z",
    lastAuthenticatedAt: null,
    accountEmail: "backup.codex@example.com",
    accountName: "Backup Codex",
    usageSummary: null,
    remaining5hPercent: null,
    remaining7dPercent: null,
    remaining5hResetAt: null,
    remaining7dResetAt: null,
    usageSource: "unavailable",
    accessTokenExpiresAt: null,
    refreshTokenExpiresAt: null,
    refreshTokenExpiryNote: null,
    usageDetailLines: [],
  },
];

interface RunState {
  runId: string;
  projectId: string;
  workflowId?: string;
  providerSessionId: string;
  providerTurnId: string;
  status: RunHistoryItem["status"];
  startedAt: string;
  updatedAt: string;
  lastPrompt?: string;
  lastMessage?: string;
  /** Monotonic per-run event sequence (reconnect cursor, 04-02). */
  seq: number;
  lastTurnInput?: TurnInput;
  parentRunId?: string;
  agentName?: string;
  role?: string;
}

interface ParentGraphState {
  snapshot: AgentGraphSnapshot;
}

const MOCK_AGENTS: AgentDefinition[] = [
  { name: "architect", description: "Designs the approach", role: "architecture", source: "flowpilot" },
  { name: "builder", description: "Implements the plan", role: "implementation", source: "flowpilot" },
  { name: "reviewer", description: "Checks the result", role: "review", source: "flowpilot" },
];

/**
 * Phase 1 mock implementation of the RunnerClient contract (04-01 Part A).
 *
 * [coding-skill]: keeps the desktop-only orchestration contract mutable in-memory.
 * [testing-skill]: preserves replayable parent graph state for targeted store tests.
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
  /** Mutable orchestration snapshots keyed by parent run. */
  private readonly parentGraphs = new Map<string, ParentGraphState>();
  /** Runs an interrupt has been requested for. */
  private readonly aborted = new Set<string>();
  /** Cancels the pending approval/question gate for a run (used by interrupt). */
  private readonly pendingGateCancel = new Map<string, () => void>();

  /** In-memory chat-posture document (mock stand-in for the runner's shared file). */
  private chatPostureActive: ChatPostureConfig["active"] = "code";
  private chatPostureProfiles: Record<ChatPostureConfig["active"], ChatPostureProfile> = {
    scan: {},
    plan: {},
    code: {},
  };

  setScenario(scenario: ScenarioName): void {
    this.scenario = scenario;
  }

  private rec(runId: string, ev: ProviderEventDTO): ProviderEventDTO {
    const log = this.eventLog.get(runId);
    if (log) log.push(ev);
    else this.eventLog.set(runId, [ev]);
    const state = this.runs.get(runId);
    if (state) {
      state.updatedAt = ev.occurredAt;
      if (ev.type === "turn_started") state.status = "running";
      if (ev.type === "permission_required") state.status = "waiting_approval";
      if (ev.type === "user_question_required") state.status = "waiting_question";
      if (ev.type === "message_completed" && ev.text) state.lastMessage = ev.text;
      if (ev.type === "turn_completed") {
        state.status = "completed";
        if (ev.finalMessage) state.lastMessage = ev.finalMessage;
      }
      if (ev.type === "turn_failed") {
        state.status = "failed";
        state.lastMessage = ev.error;
      }
    }
    return ev;
  }

  async interrupt(runId: string): Promise<void> {
    this.aborted.add(runId);
    const cancel = this.pendingGateCancel.get(runId);
    if (cancel) cancel();
  }

  async submitGateDecision(_runId: string, _option: string, _customText?: string): Promise<void> {
    // Mock stub — real gate decisions are only possible with a live runner (Task-155).
  }

  async submitGateAgreement(_runId: string, _testNames: string[]): Promise<void> {
    // Mock stub — real gate agreement is only possible with a live runner (Task-155).
  }

  async *streamRun(runId: string, afterSeq = 0, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    const log = this.eventLog.get(runId) ?? [];
    for (const ev of log) {
      if (signal?.aborted) return;
      if (ev.seq > afterSeq) {
        await delay(40); // small pause so the rebuilt timeline is visible
        if (signal?.aborted) return;
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

  async listProviderAccounts(): Promise<ProviderAccountSummary[]> {
    await delay(80);
    return MOCK_PROVIDER_ACCOUNTS;
  }

  async listRemoteChatSessions(projectId: string): Promise<RemoteChatSessionSummary[]> {
    await delay(40);
    return [];
  }

  async syncChatRun(runId: string, _input?: ChatSessionSyncRequest): Promise<ChatSessionSyncResult> {
    await delay(40);
    return {
      runId,
      sourceMachineId: "mch_mock",
      sourceRunId: runId,
      syncStatus: "synced",
      syncedAt: new Date().toISOString(),
      remotePath: `chat-sessions/runs/mch_mock/${runId}/manifest.json`,
    };
  }

  async deleteRun(_runId: string): Promise<void> {
    await delay(40);
  }

  async restoreChatRun(input: ChatSessionRestoreRequest): Promise<ChatSessionRestoreResult> {
    await delay(40);
    return {
      runId: input.sourceRunId,
      sourceMachineId: input.sourceMachineId,
      sourceRunId: input.sourceRunId,
      providerKey: "codex",
      restoreStatus: "restored",
    };
  }

  async handoffContext(runId: string, input: HandoffContextRequest): Promise<HandoffContextResponse> {
    await delay(40);
    return {
      sourceRunId: runId,
      sourceProviderKey: "codex",
      targetProviderKey: input.targetProviderKey,
      prompt: `[mock handoff] source=${runId} target=${input.targetProviderKey}`,
      includedTurnCount: 0,
      omittedTurnCount: 0,
      truncated: false,
      handoffMode: "raw",
    };
  }

  async generateChatSummary(runId: string): Promise<ChatSummaryResult> {
    await delay(20);
    return { runId, generated: true, skipped: false };
  }

  async connectProviderAccount(providerKey: "codex" | "claude" | "gemini" | "grok" | "opencode"): Promise<void> {
    await delay(80);
    const nextSlotIndex =
      MOCK_PROVIDER_ACCOUNTS.filter((account) => account.providerKey === providerKey).reduce(
        (max, account) => Math.max(max, account.slotIndex),
        0,
      ) + 1;
    MOCK_PROVIDER_ACCOUNTS.push({
      id: `acct-${providerKey}-${nextSlotIndex}`,
      providerKey,
      displayName: `Account ${nextSlotIndex}`,
      displayLabel: `${providerKey}.new${nextSlotIndex}@example.com`,
      homePath: `/Users/demo/.${providerKey}Home${nextSlotIndex}`,
      authStorePath: `/Users/demo/.${providerKey}Home${nextSlotIndex}/.${providerKey}`,
      slotIndex: nextSlotIndex,
      authStatus: "connected",
      isActive: false,
      createdAt: new Date().toISOString(),
      lastAuthenticatedAt: new Date().toISOString(),
      accountEmail: `${providerKey}.new${nextSlotIndex}@example.com`,
      accountName: `${providerKey} Account ${nextSlotIndex}`,
      usageSummary: null,
      remaining5hPercent: null,
      remaining7dPercent: null,
      remaining5hResetAt: null,
      remaining7dResetAt: null,
      usageSource: "unavailable",
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null,
      refreshTokenExpiryNote: null,
      usageDetailLines: [],
    });
  }

  async listWorkspaceFiles(_cwd: string, query?: string): Promise<string[]> {
    await delay(20);
    const all = [
      "apps/desktop-flowpilot/src/components/ChatInput.tsx",
      "apps/local-runner/internal/runner/workspace_files.go",
    ];
    const q = (query ?? "").toLowerCase();
    return q ? all.filter((path) => path.toLowerCase().includes(q)) : all;
  }

  async listSkills(provider: string, _cwd?: string): Promise<ProviderSkill[]> {
    await delay(40);
    if (provider === "claude") {
      return MOCK_SKILLS.filter((skill) => skill.name !== "test-writer");
    }
    if (provider === "gemini") {
      return MOCK_SKILLS.filter((skill) => skill.name === "architect" || skill.name === "reviewer");
    }
    return MOCK_SKILLS;
  }

  async listAgents(_cwd?: string): Promise<AgentDefinition[]> {
    await delay(40);
    return MOCK_AGENTS;
  }

  async listBuiltinOrchestrationOptions(subMode: string): Promise<BuiltinFlowOption[]> {
    await delay(20);
    if (subMode === "bug") {
      return [
        {
          flowRef: "flowpilot-core-flow-pack/review-loop",
          label: "Review Loop",
          description: "Coder -> reviewers -> synthesis, review-until-clean.",
        },
      ];
    }
    return [];
  }

  async listAgentRuns(parentRunId: string): Promise<AgentRunSummary[]> {
    await delay(40);
    return Array.from(this.runs.values())
      .filter((run) => run.parentRunId === parentRunId)
      .map((run) => ({
        runId: run.runId,
        agentName: run.agentName ?? "agent",
        role: run.role ?? "assistant",
        status: run.status,
        parentRunId: run.parentRunId,
        createdAt: run.startedAt,
        agentStatus: run.status,
      }));
  }

  private async syncParentGraph(parentRunId: string): Promise<ParentGraphState> {
    const current = this.parentGraphs.get(parentRunId);
    const snapshot: AgentGraphSnapshot = {
      parentRunId,
      runs: await this.listAgentRuns(parentRunId),
      edges: current?.snapshot.edges ?? [],
      busMessages: current?.snapshot.busMessages ?? [],
      loopState: current?.snapshot.loopState ?? { status: "running", round: 0, roundCap: 3 },
    };
    const next = { snapshot };
    this.parentGraphs.set(parentRunId, next);
    return next;
  }

  async refreshAgentGraph(parentRunId: string): Promise<AgentGraphSnapshot> {
    return (await this.syncParentGraph(parentRunId)).snapshot;
  }

  async pauseAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    graph.snapshot = {
      ...graph.snapshot,
      loopState: { ...graph.snapshot.loopState, status: "paused" },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async resumeAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    graph.snapshot = {
      ...graph.snapshot,
      loopState: { ...graph.snapshot.loopState, status: "running" },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async injectAgentFeedback(parentRunId: string, _toRunId: string, message: string): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    graph.snapshot = {
      ...graph.snapshot,
      busMessages: [
        ...graph.snapshot.busMessages,
        {
          id: nextId("bus"),
          parentRunId,
          kind: "user-feedback",
          message,
          queued: true,
          occurredAt: new Date().toISOString(),
        },
      ],
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async stopAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    graph.snapshot = {
      ...graph.snapshot,
      loopState: { ...graph.snapshot.loopState, status: "stopped" },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async submitReviewOutcome(parentRunId: string, input: ReviewOutcomeInput): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    const nextStatus = input.outcome === "approved" ? "done" : "running";
    graph.snapshot = {
      ...graph.snapshot,
      loopState: {
        ...graph.snapshot.loopState,
        status: nextStatus,
        openIssues: input.issues?.length ?? 0,
      },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async extendCap(parentRunId: string): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    const prev = graph.snapshot.loopState;
    const currentCap = prev.cap ?? prev.roundCap ?? 3;
    graph.snapshot = {
      ...graph.snapshot,
      loopState: {
        ...prev,
        cap: currentCap + 2,
        extendCount: (prev.extendCount ?? 0) + 1,
        status: "running",
        blockReason: undefined,
      },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async amendFlow(runId: string, _paths: string[]): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(runId);
    const prev = graph.snapshot.loopState;
    if (prev.status !== "blocked") return graph.snapshot;
    graph.snapshot = { ...graph.snapshot, loopState: { ...prev, status: "running", gateReason: undefined, blockReason: undefined } };
    this.parentGraphs.set(runId, graph);
    return graph.snapshot;
  }

  // BUG-231: mirrors the runner's resumeFlowWithFeedback — auto-extends the
  // cap only when the block reason was "cap", leaves it alone for "escalate".
  async continueFlow(
    parentRunId: string,
    _feedback: string,
    memberAction?: { action: "retry" | "skip"; node?: string },
  ): Promise<AgentGraphSnapshot> {
    const graph = await this.syncParentGraph(parentRunId);
    const prev = graph.snapshot.loopState;
    if (prev.status !== "blocked") {
      return graph.snapshot;
    }
    // Task-241: member_stalled retry/skip just clears the block in the mock.
    if (prev.blockReason === "member_stalled" && memberAction) {
      graph.snapshot = {
        ...graph.snapshot,
        loopState: {
          ...prev,
          status: "running",
          gateReason: undefined,
          blockReason: undefined,
        },
      };
      this.parentGraphs.set(parentRunId, graph);
      return graph.snapshot;
    }
    const currentCap = prev.cap ?? prev.roundCap ?? 3;
    const extendForCap = prev.blockReason === "cap";
    graph.snapshot = {
      ...graph.snapshot,
      loopState: {
        ...prev,
        cap: extendForCap ? currentCap + 2 : currentCap,
        extendCount: extendForCap ? (prev.extendCount ?? 0) + 1 : prev.extendCount,
        status: "running",
        gateReason: undefined,
        blockReason: undefined,
      },
    };
    this.parentGraphs.set(parentRunId, graph);
    return graph.snapshot;
  }

  async spawnAgent(input: SpawnAgentInput & { parentRunId: string }): Promise<SpawnAgentResult> {
    await delay(50);
    const runId = nextId("agent");
    const providerSessionId = nextId("thread");
    const now = new Date().toISOString();
    const parent = this.runs.get(input.parentRunId);
    this.runs.set(runId, {
      runId,
      projectId: parent?.projectId ?? "",
      workflowId: parent?.workflowId,
      providerSessionId,
      providerTurnId: "",
      status: "completed",
      startedAt: now,
      updatedAt: now,
      seq: 0,
      parentRunId: input.parentRunId,
      agentName: input.agent,
      role: input.agent,
      lastMessage: input.prompt,
    });
    return { runId, providerSessionId, providerKey: "codex", status: "completed", finalMessage: input.prompt };
  }

  async *focusAgentRun(runId: string, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    yield* this.streamRun(runId, 0, signal);
  }

  async listArtifacts(runId: string): Promise<Artifact[]> {
    await delay(60);
    return mockArtifacts(runId);
  }

  async listRunHistory(projectId: string): Promise<RunHistoryItem[]> {
    await delay(60);
    return Array.from(this.runs.values())
      .filter((run) => run.projectId === projectId && !run.parentRunId)
      .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
      .map((run) => ({
        runId: run.runId,
        projectId: run.projectId,
        workflowId: run.workflowId,
        providerKey: "codex",
        status: run.status,
        startedAt: run.startedAt,
        updatedAt: run.updatedAt,
        lastPrompt: run.lastPrompt,
        lastMessage: run.lastMessage,
        parentRunId: run.parentRunId,
        agentName: run.agentName,
        role: run.role,
        agentStatus: run.agentName ? run.status : undefined,
      }));
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

  async activateProviderAccount(accountId: string): Promise<void> {
    await delay(60);
    const target = MOCK_PROVIDER_ACCOUNTS.find((account) => account.id === accountId);
    if (!target) return;
    for (const account of MOCK_PROVIDER_ACCOUNTS) {
      if (account.providerKey === target.providerKey) {
        account.isActive = account.id === accountId;
      }
    }
  }

  async applyGrokYoloPosture(_yolo: boolean): Promise<void> {
    await delay(300); // mirrors the real config.toml rewrite + process respawn taking a moment
  }

  async getChatPosture(): Promise<ChatPostureConfig> {
    await delay(30);
    return {
      active: this.chatPostureActive,
      profiles: {
        scan: this.chatPostureProfiles.scan,
        plan: this.chatPostureProfiles.plan,
        code: this.chatPostureProfiles.code,
      },
    };
  }

  async setChatPosture(config: ChatPostureConfig): Promise<ChatPostureConfig> {
    await delay(30);
    this.chatPostureActive = config.active;
    this.chatPostureProfiles = {
      scan: { ...config.profiles.scan },
      plan: { ...config.profiles.plan },
      code: { ...config.profiles.code },
    };
    return this.getChatPosture();
  }

  async openProviderAccountTerminal(_accountId: string): Promise<void> {
    await delay(60);
  }

  async startRun(input: StartRunInput): Promise<RunHandle> {
    await delay(80);
    const runId = nextId("run");
    const providerSessionId = nextId("thread");
    const now = new Date().toISOString();
    this.runs.set(runId, {
      runId,
      projectId: input.projectId,
      workflowId: input.workflowId,
      providerSessionId,
      providerTurnId: "",
      status: "running",
      startedAt: now,
      updatedAt: now,
      seq: 0,
    });
    const providerKey = input.providerKey ?? "codex";
    const stepId = input.chatMode === "normal_chat" ? `chat-${runId}` : undefined;
    return { runId, providerSessionId, providerKey, status: "running", ...(stepId ? { stepId } : {}) };
  }

  async resumeRun(runId: string): Promise<RunHandle> {
    await delay(80);
    const state = this.runs.get(runId);
    const providerSessionId = state?.providerSessionId ?? nextId("thread");
    // Mark this run so the next sendTurn streams the full happy path (replay).
    this.replayRuns.add(runId);
    return { runId, providerSessionId, providerKey: "codex", status: "running" };
  }

  async submitApproval(approvalId: string, decision: string, _remember?: boolean): Promise<void> {
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
      projectId: "",
      providerSessionId: nextId("thread"),
      providerTurnId: "",
      status: "running" as const,
      startedAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      seq: 0,
    };
    state.providerTurnId = nextId("turn");
    state.lastTurnInput = input;
    state.lastPrompt = input.prompt;
    state.status = "running";
    state.updatedAt = new Date().toISOString();
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
    yield this.rec(input.runId, {
      ...base(),
      type: "turn_started",
      providerTurnId: state.providerTurnId,
      prompt: input.prompt,
    });

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
        case "token_usage":
          yield { ...base(), type: "token_usage_updated", tokenUsage: step.tokenUsage };
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

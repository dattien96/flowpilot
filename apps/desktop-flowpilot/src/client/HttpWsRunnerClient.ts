import type {
  AgentDefinition,
  AgentRunSummary,
  AgentGraphSnapshot,
  Artifact,
  BuiltinFlowOption,
  ChatPostureConfig,
  ChatSessionRestoreRequest,
  ChatSessionRestoreResult,
  ChatSessionSyncRequest,
  ChatSwitchInput,
  ChatSwitchResponse,
  ChatTimelineResponse,
  ChatSessionSyncResult,
  HandoffContextRequest,
  HandoffContextResponse,
  ChatSummaryResult,
  LSPStatus,
  Project,
  ProviderAccountSummary,
  ProviderEventDTO,
  ProviderSkill,
  QuotaRoutingSettings,
  RemoteChatSessionSummary,
  RunHandle,
  RunHistoryItem,
  RunRealtimeFrame,
  RunnerClient,
  ReviewOutcomeInput,
  SpawnAgentInput,
  SpawnAgentResult,
  StartRunInput,
  Step,
  TurnInput,
  Workflow,
  WorkflowStepsRuntimeSnapshot,
} from "@/types/contract";

interface RawProviderAccountUsageLine {
  label: string;
  remaining_percent: number;
  reset_at: string | null;
}

interface RawProviderAccountSummary {
  id: string;
  provider_key: ProviderAccountSummary["providerKey"];
  display_name: string;
  display_label: string;
  home_path: string;
  auth_store_path: string | null;
  slot_index: number;
  auth_status: ProviderAccountSummary["authStatus"];
  is_active: boolean;
  created_at: string;
  last_authenticated_at: string | null;
  account_email: string | null;
  account_name: string | null;
  usage_summary: string | null;
  remaining_5h_percent: number | null;
  remaining_7d_percent: number | null;
  remaining_5h_reset_at: string | null;
  remaining_7d_reset_at: string | null;
  usage_source: ProviderAccountSummary["usageSource"];
  access_token_expires_at: string | null;
  refresh_token_expires_at: string | null;
  refresh_token_expiry_note: string | null;
  usage_detail_lines: RawProviderAccountUsageLine[];
}

function mapProviderAccountSummary(raw: RawProviderAccountSummary): ProviderAccountSummary {
  return {
    id: raw.id,
    providerKey: raw.provider_key,
    displayName: raw.display_name,
    displayLabel: raw.display_label,
    homePath: raw.home_path,
    authStorePath: raw.auth_store_path,
    slotIndex: raw.slot_index,
    authStatus: raw.auth_status,
    isActive: raw.is_active,
    createdAt: raw.created_at,
    lastAuthenticatedAt: raw.last_authenticated_at,
    accountEmail: raw.account_email,
    accountName: raw.account_name,
    usageSummary: raw.usage_summary,
    remaining5hPercent: raw.remaining_5h_percent,
    remaining7dPercent: raw.remaining_7d_percent,
    remaining5hResetAt: raw.remaining_5h_reset_at,
    remaining7dResetAt: raw.remaining_7d_reset_at,
    usageSource: raw.usage_source,
    accessTokenExpiresAt: raw.access_token_expires_at,
    refreshTokenExpiresAt: raw.refresh_token_expires_at,
    refreshTokenExpiryNote: raw.refresh_token_expiry_note,
    usageDetailLines: raw.usage_detail_lines.map((line) => ({
      label: line.label,
      remainingPercent: line.remaining_percent,
      resetAt: line.reset_at,
    })),
  };
}

// HttpWsRunnerClient (04-01 Part B) — the real transport against the Phase 2
// runner API (04-02). Implements the SAME RunnerClient interface as
// MockRunnerClient, so the renderer is unchanged; only this transport differs.
//
// Streaming model (04-02): `POST .../turns` returns { turnId } immediately; events
// flow on the per-run SSE stream. sendTurn POSTs the turn, then yields the run's
// stream filtered to that turnId until the turn terminates. Reconnect uses
// streamRun (attach + replay via afterSeq). Uses fetch + a manual SSE parser so it
// runs in the Electron renderer and in Node smoke tests.
export class HttpWsRunnerClient implements RunnerClient {
  private readonly base: string;
  private scenario?: string;
  /** Highest seq seen per run, so a follow-up turn/stream resumes after it. */
  private readonly lastSeq = new Map<string, number>();

  constructor(baseUrl: string) {
    this.base = baseUrl.replace(/\/+$/, "");
  }

  setScenario(scenario: string): void {
    this.scenario = scenario;
  }

  // ---- plain JSON helpers --------------------------------------------------

  private async getJSON<T>(path: string): Promise<T> {
    const resp = await fetch(this.base + path, { headers: { Accept: "application/json" } });
    return this.parse<T>(resp);
  }

  private async postJSON<T>(path: string, body?: unknown, headers?: Record<string, string>, timeoutMs?: number): Promise<T> {
    const resp = await fetch(this.base + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", "X-Client": "desktop", ...(headers ?? {}) },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: timeoutMs === undefined ? undefined : AbortSignal.timeout(timeoutMs),
    });
    return this.parse<T>(resp);
  }

  private async putJSON<T>(path: string, body?: unknown): Promise<T> {
    const resp = await fetch(this.base + path, {
      method: "PUT",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return this.parse<T>(resp);
  }

  private async parse<T>(resp: Response): Promise<T> {
    const text = await resp.text();
    const data = text ? JSON.parse(text) : undefined;
    if (!resp.ok) {
      const err = (data && data.error) || {};
      // stopAgentLoop may attach graph snapshot after RAM cancel even when durable
      // fence/persist fails (CP-51 A1) — surface it on the error for UI settle.
      throw new RunnerApiError(
        resp.status,
        err.code ?? "http_error",
        err.message ?? resp.statusText,
        data?.snapshot,
        // Task-435 T-3: full body — 409 confirm/conflict payloads carry
        // requiresConfirm/uncommitted/conflictPaths at top level.
        data as Record<string, unknown> | undefined,
      );
    }
    return data as T;
  }

  // ---- catalog -------------------------------------------------------------

  listProjects(): Promise<Project[]> {
    return this.getJSON<Project[]>("/client/projects");
  }
  listWorkflows(): Promise<Workflow[]> {
    return this.getJSON<Workflow[]>("/client/workflows");
  }
  listSteps(): Promise<Step[]> {
    return this.getJSON<Step[]>("/client/steps");
  }
  listProviderAccounts(): Promise<ProviderAccountSummary[]> {
    return this.getJSON<RawProviderAccountSummary[]>("/client/provider-accounts").then((accounts) =>
      accounts.map(mapProviderAccountSummary),
    );
  }
  listArtifacts(runId: string): Promise<Artifact[]> {
    return this.getJSON<Artifact[]>(`/client/workflow-runs/${encodeURIComponent(runId)}/artifacts`);
  }
  listRunHistory(projectId: string): Promise<RunHistoryItem[]> {
    return this.getJSON<RunHistoryItem[]>(`/client/projects/${encodeURIComponent(projectId)}/workflow-runs`);
  }
  listRemoteChatSessions(projectId: string): Promise<RemoteChatSessionSummary[]> {
    return this.getJSON<RemoteChatSessionSummary[]>(`/client/projects/${encodeURIComponent(projectId)}/chat-sessions/remote`);
  }
  listSkills(provider: string, cwd?: string): Promise<ProviderSkill[]> {
    let url = `/client/provider-skills?provider=${encodeURIComponent(provider)}`;
    if (cwd) url += `&cwd=${encodeURIComponent(cwd)}`;
    return this.getJSON<ProviderSkill[]>(url);
  }
  listWorkspaceFiles(cwd: string, query?: string): Promise<string[]> {
    let url = `/client/workspace-files?cwd=${encodeURIComponent(cwd)}`;
    if (query) url += `&q=${encodeURIComponent(query)}`;
    return this.getJSON<string[]>(url);
  }
  getLSPStatus(cwd: string): Promise<LSPStatus> {
    return this.getJSON<LSPStatus>(`/client/lsp-status?path=${encodeURIComponent(cwd)}`);
  }

  listBuiltinOrchestrationOptions(subMode: string): Promise<BuiltinFlowOption[]> {
    const url = `/client/chat/builtin-orchestration-options?subMode=${encodeURIComponent(subMode)}`;
    return this.getJSON<BuiltinFlowOption[]>(url);
  }

  listAgents(cwd?: string): Promise<AgentDefinition[]> {
    const url = cwd ? `/client/agents?cwd=${encodeURIComponent(cwd)}` : "/client/agents";
    return this.getJSON<AgentDefinition[]>(url);
  }

  listAgentRuns(parentRunId: string): Promise<AgentRunSummary[]> {
    return this.getJSON<AgentRunSummary[]>(
      `/client/workflow-runs/${encodeURIComponent(parentRunId)}/agents`,
    );
  }
  refreshAgentGraph(parentRunId: string): Promise<AgentGraphSnapshot> {
    return this.getJSON<AgentGraphSnapshot>(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-graph`);
  }
  getWorkflowStepsRuntime(runId: string): Promise<WorkflowStepsRuntimeSnapshot> {
    return this.getJSON<WorkflowStepsRuntimeSnapshot>(`/client/workflow-runs/${encodeURIComponent(runId)}/steps-runtime`);
  }
  pauseAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/pause`); }
  resumeAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/resume`); }
  injectAgentFeedback(parentRunId: string, toRunId: string, message: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/feedback`, { toRunId, message }); }
  stopAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/stop`); }
  submitReviewOutcome(parentRunId: string, input: ReviewOutcomeInput): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/flow-control`, input); }
  extendCap(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/extend-cap`); }
  continueFlow(parentRunId: string, feedback: string, memberAction?: { action: "retry" | "skip"; node?: string }): Promise<AgentGraphSnapshot> {
    return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/continue`, {
      feedback,
      ...(memberAction ? { memberAction } : {}),
    });
  }
  amendFlow(runId: string, paths: string[]): Promise<AgentGraphSnapshot> {
    return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/agent-loop/amend`, { paths });
  }

  spawnAgent(input: SpawnAgentInput & { parentRunId: string }): Promise<SpawnAgentResult> {
    const { parentRunId, ...body } = input;
    return this.postJSON<SpawnAgentResult>(
      `/client/workflow-runs/${encodeURIComponent(parentRunId)}/spawn-agent`,
      body,
    );
  }

  focusAgentRun(runId: string, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    return this.streamRun(runId, 0, signal);
  }

  // ---- run lifecycle -------------------------------------------------------

  startRun(input: StartRunInput): Promise<RunHandle> {
    return this.postJSON<RunHandle>("/client/workflow-runs", input);
  }
  resumeRun(runId: string): Promise<RunHandle> {
    return this.postJSON<RunHandle>(`/client/workflow-runs/${encodeURIComponent(runId)}/resume`);
  }
  syncChatRun(runId: string, input?: ChatSessionSyncRequest): Promise<ChatSessionSyncResult> {
    return this.postJSON<ChatSessionSyncResult>(`/client/workflow-runs/${encodeURIComponent(runId)}/sync-chat`, input ?? {});
  }
  async deleteRun(runId: string): Promise<void> {
    const resp = await fetch(this.base + `/client/workflow-runs/${encodeURIComponent(runId)}`, {
      method: "DELETE",
      headers: { Accept: "application/json" },
    });
    await this.parse<unknown>(resp);
  }
  restoreChatRun(input: ChatSessionRestoreRequest): Promise<ChatSessionRestoreResult> {
    return this.postJSON<ChatSessionRestoreResult>("/client/chat-sessions/restore", input);
  }
  handoffContext(runId: string, input: HandoffContextRequest): Promise<HandoffContextResponse> {
    return this.postJSON<HandoffContextResponse>(`/client/workflow-runs/${encodeURIComponent(runId)}/handoff-context`, input);
  }
  switchChatProvider(chatId: string, input: ChatSwitchInput): Promise<ChatSwitchResponse> {
    return this.postJSON<ChatSwitchResponse>(`/client/chats/${encodeURIComponent(chatId)}/switch-provider`, input);
  }
  async chatTimeline(chatId: string, afterSeq?: number, limit?: number, beforeSeq?: number): Promise<ChatTimelineResponse> {
    let path = `/client/chats/${encodeURIComponent(chatId)}/timeline`;
    if (afterSeq !== undefined || limit !== undefined || beforeSeq !== undefined) {
      const q = new URLSearchParams();
      if (afterSeq !== undefined) q.set("afterSeq", String(afterSeq));
      if (limit !== undefined) q.set("limit", String(limit));
      if (beforeSeq !== undefined) q.set("beforeSeq", String(beforeSeq));
      path += `?${q.toString()}`;
    }
    return this.getJSON<ChatTimelineResponse>(path);
  }
  /** Task-421: run-scoped timeline (workflow runs key transcript by runId). */
  async runTimeline(runId: string, opts?: { afterSeq?: number; limit?: number; beforeSeq?: number }): Promise<ChatTimelineResponse> {
    let path = `/client/workflow-runs/${encodeURIComponent(runId)}/timeline`;
    const q = new URLSearchParams();
    if (opts?.afterSeq !== undefined) q.set("afterSeq", String(opts.afterSeq));
    if (opts?.limit !== undefined) q.set("limit", String(opts.limit));
    if (opts?.beforeSeq !== undefined) q.set("beforeSeq", String(opts.beforeSeq));
    const qs = q.toString();
    if (qs) path += `?${qs}`;
    return this.getJSON<ChatTimelineResponse>(path);
  }
  generateChatSummary(runId: string): Promise<ChatSummaryResult> {
    return this.postJSON<ChatSummaryResult>(`/client/workflow-runs/${encodeURIComponent(runId)}/chat-summary`, {});
  }
  submitApproval(approvalId: string, decision: string, remember?: boolean): Promise<void> {
    return this.postJSON<void>(`/client/approvals/${encodeURIComponent(approvalId)}/decision`, {
      decision,
      ...(remember ? { remember: true } : {}),
    });
  }
  answerQuestion(questionId: string, choice: string | string[]): Promise<void> {
    return this.postJSON<void>(`/client/questions/${encodeURIComponent(questionId)}/answer`, { choice });
  }
  interrupt(runId: string): Promise<void> {
    return this.postJSON<void>(`/client/workflow-runs/${encodeURIComponent(runId)}/interrupt`);
  }
  // SS-17 / CP-51 Task-256 — per-run REST paths (never a root /dispatch namespace).
  async listDispatchAttention(runId: string): Promise<import("@/types/contract").DispatchAttentionItem[]> {
    const body = await this.getJSON<{ items?: Array<Record<string, string>> }>(
      `/client/workflow-runs/${encodeURIComponent(runId)}/dispatch-attention`,
    );
    return (body.items ?? []).map((it) => ({
      kind: it.kind ?? "",
      runId: it.run_id ?? it.runId ?? "",
      turnId: it.turn_id ?? it.turnId,
      reason: it.reason,
      updatedAt: it.updated_at ?? it.updatedAt,
    }));
  }
  inspectDispatch(runId: string, turnId: string): Promise<import("@/types/contract").DispatchInspectResult> {
    return this.getJSON(
      `/client/workflow-runs/${encodeURIComponent(runId)}/dispatches/${encodeURIComponent(turnId)}`,
    );
  }
  getDispatchAudit(runId: string, turnId: string): Promise<{ entries: unknown[] }> {
    return this.getJSON(
      `/client/workflow-runs/${encodeURIComponent(runId)}/dispatches/${encodeURIComponent(turnId)}/audit`,
    );
  }
  resolveDispatchUncertain(
    runId: string,
    turnId: string,
    input: {
      expectedRev: number;
      resolutionId: string;
      action: import("@/types/contract").DispatchResolveAction;
      detail?: string;
    },
  ): Promise<import("@/types/contract").DispatchSettlementDisposition> {
    return this.postJSON(
      `/client/workflow-runs/${encodeURIComponent(runId)}/dispatches/${encodeURIComponent(turnId)}/resolve`,
      input,
    );
  }
  retryDispatchAsNew(
    runId: string,
    turnId: string,
    input: { expectedRev: number; resolutionId: string; newTurnId?: string; expectedIntentGen: number; expectedEnvelopeHash: string },
  ): Promise<import("@/types/contract").DispatchSettlementDisposition & { newTurnId: string }> {
    return this.postJSON(
      `/client/workflow-runs/${encodeURIComponent(runId)}/dispatches/${encodeURIComponent(turnId)}/retry-as-new`,
      input,
    );
  }
  resolveDispatchRepair(
    runId: string,
    input: { expectedRepairRev: number; resolutionId: string; action: "retry_load" | "abandon" },
  ): Promise<{ revision: number; outcome: "resolved_retry_load" | "failed_still_open" | "resolved_abandon"; detail: string }> {
    return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/repair-resolution`, input);
  }
  submitGateDecision(runId: string, option: string, customText?: string): Promise<void> {
    return this.postJSON<void>(
      `/client/workflow-runs/${encodeURIComponent(runId)}/gate-decision`,
      { option, ...(customText ? { customText } : {}) },
    );
  }
  submitGateAgreement(runId: string, testNames: string[]): Promise<void> {
    return this.postJSON<void>(
      `/client/workflow-runs/${encodeURIComponent(runId)}/gate-agreement`,
      { testNames },
    );
  }
  // CP-84 (Task-431): ID-scoped worktree merge resolution for inbox actions.
  resolveWorktreeMerge(runId: string, mode: string, confirm?: boolean): Promise<unknown> {
    return this.postJSON<unknown>(
      `/client/workflow-runs/${encodeURIComponent(runId)}/worktree/resolve`,
      { mode, confirm: confirm === true },
    );
  }
  // CP-84 (Task-431): ID-scoped SS-lock gate answer for inbox actions.
  confirmSSLock(runId: string, action: "approve" | "reject", edits?: string): Promise<unknown> {
    return this.postJSON<unknown>(
      `/client/workflow-runs/${encodeURIComponent(runId)}/confirm`,
      { action, ...(edits ? { edits } : {}) },
    );
  }
  async connectProviderAccount(providerKey: string): Promise<void> {
    await this.postJSON<unknown>("/provider-accounts/connect", { providerKey });
  }
  activateProviderAccount(accountId: string): Promise<void> {
    return this.postJSON<void>("/provider-accounts/activate", { accountId });
  }
  applyGrokYoloPosture(yolo: boolean): Promise<void> {
    return this.postJSON<void>("/provider-accounts/grok-yolo-posture", { yolo });
  }
  async getOpencodeModelVariants(modelId: string): Promise<{ supportedEfforts: string[]; defaultReasoningEffort: string }> {
    return this.getJSON<{ supportedEfforts: string[]; defaultReasoningEffort: string }>(
      `/client/providers/opencode-variants?model=${encodeURIComponent(modelId)}`,
    );
  }
  getChatPosture(): Promise<ChatPostureConfig> {
    return this.getJSON<ChatPostureConfig>("/client/chat-posture");
  }
  setChatPosture(config: ChatPostureConfig): Promise<ChatPostureConfig> {
    return this.putJSON<ChatPostureConfig>("/client/chat-posture", config);
  }

  getQuotaRoutingSettings(): Promise<QuotaRoutingSettings> {
    return this.getJSON<QuotaRoutingSettings>("/client/quota-routing-settings");
  }

  setQuotaRoutingSettings(settings: QuotaRoutingSettings): Promise<QuotaRoutingSettings> {
    return this.putJSON<QuotaRoutingSettings>("/client/quota-routing-settings", settings);
  }
  openProviderAccountTerminal(accountId: string): Promise<void> {
    return this.postJSON<void>("/provider-accounts/test", { accountId });
  }
  // CP-81 Task-418 T-3: inside Electron, destructive system actions route
  // through the lifecycle bridge — Electron main owns the lease token and
  // performs the fenced two-phase call (confirm token + expectedInstanceId).
  // Browser/test mode (no bridge) keeps the direct POST. Typed self-contained
  // so the phase1 harness need not include flowpilotBridge.d.ts.
  private lifecycleBridge(): {
    requestRestart(): Promise<unknown>;
    requestGlobalShutdown(): Promise<unknown>;
    getSnapshot(): Promise<unknown>;
  } | undefined {
    return (globalThis as { flowpilot?: { lifecycle?: {
      requestRestart(): Promise<unknown>;
      requestGlobalShutdown(): Promise<unknown>;
      getSnapshot(): Promise<unknown>;
    } } }).flowpilot?.lifecycle;
  }

  restartStack(): Promise<void> {
    const bridge = this.lifecycleBridge();
    if (bridge) {
      return bridge.requestRestart().then(() => undefined);
    }
    // The runner answers before running cleanup, so this should return fast —
    // but bound it anyway so a wedged teardown can never leave the button on
    // "Shutting down…" forever.
    return this.postJSON<void>("/system/restart", undefined, undefined, 10_000);
  }
  shutdownStack(): Promise<void> {
    const bridge = this.lifecycleBridge();
    if (bridge) {
      return bridge.requestGlobalShutdown().then(() => undefined);
    }
    return this.postJSON<void>("/system/shutdown", undefined, undefined, 10_000);
  }

  // CP-81: renderer-side snapshot read (no token material crosses the bridge).
  getLifecycleSnapshot(): Promise<unknown> {
    const bridge = this.lifecycleBridge();
    if (bridge) {
      return bridge.getSnapshot();
    }
    return this.getJSON<unknown>("/system/lifecycle");
  }

  // ---- streaming -----------------------------------------------------------

  async *sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO> {
    const after = this.lastSeq.get(input.runId) ?? 0;
    const { turnId } = await this.postJSON<{ turnId: string }>(
      `/client/workflow-runs/${encodeURIComponent(input.runId)}/turns`,
      {
        stepId: input.stepId,
        prompt: input.prompt,
        changeType: input.changeType,
        sourceDocId: input.sourceDocId,
        selectedSkills: input.selectedSkills,
        reasoningEffort: input.reasoningEffort,
        model: input.model,
        yoloMode: input.yoloMode,
        chatPosture: input.chatPosture,
        attachments: input.attachments,
        subMode: input.subMode,
        flowRef: input.flowRef,
        scenario: this.scenario,
      },
      input.idempotencyKey ? { "Idempotency-Key": input.idempotencyKey } : undefined,
    );

    for await (const ev of this.openStream(input.runId, after)) {
      this.lastSeq.set(input.runId, Math.max(this.lastSeq.get(input.runId) ?? 0, ev.seq));
      if (ev.providerTurnId && ev.providerTurnId !== turnId) continue; // filter to this turn
      yield ev;
      if ((ev.type === "turn_completed" || ev.type === "turn_failed") && ev.providerTurnId === turnId) {
        return;
      }
    }
  }

  async *streamRun(runId: string, afterSeq = 0, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    for await (const ev of this.openStream(runId, afterSeq, signal)) {
      this.lastSeq.set(runId, Math.max(this.lastSeq.get(runId) ?? 0, ev.seq));
      yield ev;
    }
  }

  // CP-84 / Task-429 T-5: one multiplexed lane stream. Same fetch+SSE parser
  // shape as openStream (left byte-identical for compat); frames are
  // level-triggered RunRealtimeProjection envelopes — the consumer owns
  // reconnect/backoff (store.ts consumeRunUpdatesLoop).
  async *streamRunUpdates(signal?: AbortSignal): AsyncIterable<RunRealtimeFrame> {
    const ctrl = new AbortController();
    if (signal?.aborted) return;
    const abort = () => ctrl.abort();
    signal?.addEventListener("abort", abort, { once: true });
    let resp: Response;
    try {
      resp = await fetch(`${this.base}/client/events/stream`, {
        headers: { Accept: "text/event-stream" },
        signal: ctrl.signal,
      });
    } catch (err) {
      if (ctrl.signal.aborted) return;
      throw err;
    }
    if (!resp.ok || !resp.body) {
      ctrl.abort();
      throw new RunnerApiError(resp.status, "stream_failed", `run-updates stream failed: ${resp.status}`);
    }
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    try {
      for (;;) {
        let chunk: ReadableStreamReadResult<Uint8Array>;
        try {
          chunk = await reader.read();
        } catch (err) {
          if (ctrl.signal.aborted) return;
          throw err;
        }
        const { done, value } = chunk;
        if (done) return;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf("\n\n")) >= 0) {
          const frame = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          const dataLine = frame.split("\n").find((l) => l.startsWith("data:"));
          if (!dataLine) continue;
          const json = dataLine.slice(dataLine.indexOf(":") + 1).trim();
          try {
            yield JSON.parse(json) as RunRealtimeFrame;
          } catch {
            // skip malformed frame
          }
        }
      }
    } finally {
      signal?.removeEventListener("abort", abort);
      ctrl.abort();
    }
  }

  // openStream parses the SSE body, yielding each event until the connection
  // closes or the consumer stops iterating (which aborts the fetch via finally).
  private async *openStream(runId: string, afterSeq: number, signal?: AbortSignal): AsyncIterable<ProviderEventDTO> {
    const ctrl = new AbortController();
    if (signal?.aborted) return;
    const abort = () => ctrl.abort();
    signal?.addEventListener("abort", abort, { once: true });
    let resp: Response;
    try {
      resp = await fetch(
        `${this.base}/client/workflow-runs/${encodeURIComponent(runId)}/events/stream?afterSeq=${afterSeq}`,
        { headers: { Accept: "text/event-stream" }, signal: ctrl.signal },
      );
    } catch (err) {
      // Aborted because a newer stream superseded this one — end quietly.
      if (ctrl.signal.aborted) return;
      throw err;
    }
    if (!resp.ok || !resp.body) {
      ctrl.abort();
      throw new RunnerApiError(resp.status, "stream_failed", `event stream failed: ${resp.status}`);
    }
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    try {
      for (;;) {
        let chunk: ReadableStreamReadResult<Uint8Array>;
        try {
          chunk = await reader.read();
        } catch (err) {
          if (ctrl.signal.aborted) return; // superseded — stop without surfacing AbortError
          throw err;
        }
        const { done, value } = chunk;
        if (done) return;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf("\n\n")) >= 0) {
          const frame = buf.slice(0, idx);
          buf = buf.slice(idx + 2);
          const dataLine = frame.split("\n").find((l) => l.startsWith("data:"));
          if (!dataLine) continue;
          const json = dataLine.slice(dataLine.indexOf(":") + 1).trim();
          try {
            yield JSON.parse(json) as ProviderEventDTO;
          } catch {
            // skip malformed frame
          }
        }
      }
    } finally {
      signal?.removeEventListener("abort", abort);
      ctrl.abort();
    }
  }
}

export class RunnerApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    /** Optional body field (e.g. stopAgentLoop snapshot after partial durable failure). */
    readonly snapshot?: unknown,
    /** Task-435: full parsed response body for error-envelope side channels
     *  (worktree resolve 409s carry requiresConfirm/uncommitted/untracked). */
    readonly details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = "RunnerApiError";
  }
}

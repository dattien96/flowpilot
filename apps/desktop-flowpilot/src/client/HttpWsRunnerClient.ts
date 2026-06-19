import type {
  AgentDefinition,
  AgentRunSummary,
  AgentGraphSnapshot,
  Artifact,
  ChatSessionRestoreRequest,
  ChatSessionRestoreResult,
  ChatSessionSyncRequest,
  ChatSessionSyncResult,
  Project,
  ProviderAccountSummary,
  ProviderEventDTO,
  ProviderSkill,
  RemoteChatSessionSummary,
  RunHandle,
  RunHistoryItem,
  RunnerClient,
  SpawnAgentInput,
  SpawnAgentResult,
  StartRunInput,
  Step,
  TurnInput,
  Workflow,
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

  private async postJSON<T>(path: string, body?: unknown, headers?: Record<string, string>): Promise<T> {
    const resp = await fetch(this.base + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", ...(headers ?? {}) },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return this.parse<T>(resp);
  }

  private async parse<T>(resp: Response): Promise<T> {
    const text = await resp.text();
    const data = text ? JSON.parse(text) : undefined;
    if (!resp.ok) {
      const err = (data && data.error) || {};
      throw new RunnerApiError(resp.status, err.code ?? "http_error", err.message ?? resp.statusText);
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
  pauseAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/pause`); }
  resumeAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/resume`); }
  injectAgentFeedback(parentRunId: string, toRunId: string, message: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/feedback`, { toRunId, message }); }
  stopAgentLoop(parentRunId: string): Promise<AgentGraphSnapshot> { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/stop`); }

  spawnAgent(input: SpawnAgentInput & { parentRunId: string }): Promise<SpawnAgentResult> {
    const { parentRunId, ...body } = input;
    return this.postJSON<SpawnAgentResult>(
      `/client/workflow-runs/${encodeURIComponent(parentRunId)}/spawn-agent`,
      body,
    );
  }

  focusAgentRun(runId: string): AsyncIterable<ProviderEventDTO> {
    return this.streamRun(runId);
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
  submitApproval(approvalId: string, decision: string): Promise<void> {
    return this.postJSON<void>(`/client/approvals/${encodeURIComponent(approvalId)}/decision`, { decision });
  }
  answerQuestion(questionId: string, choice: string | string[]): Promise<void> {
    return this.postJSON<void>(`/client/questions/${encodeURIComponent(questionId)}/answer`, { choice });
  }
  interrupt(runId: string): Promise<void> {
    return this.postJSON<void>(`/client/workflow-runs/${encodeURIComponent(runId)}/interrupt`);
  }
  async connectProviderAccount(providerKey: string): Promise<void> {
    await this.postJSON<unknown>("/provider-accounts/connect", { providerKey });
  }
  activateProviderAccount(accountId: string): Promise<void> {
    return this.postJSON<void>("/provider-accounts/activate", { accountId });
  }
  openProviderAccountTerminal(accountId: string): Promise<void> {
    return this.postJSON<void>("/provider-accounts/test", { accountId });
  }
  restartStack(): Promise<void> {
    return this.postJSON<void>("/system/restart");
  }
  shutdownStack(): Promise<void> {
    return this.postJSON<void>("/system/shutdown");
  }

  // ---- streaming -----------------------------------------------------------

  async *sendTurn(input: TurnInput): AsyncIterable<ProviderEventDTO> {
    const after = this.lastSeq.get(input.runId) ?? 0;
    const { turnId } = await this.postJSON<{ turnId: string }>(
      `/client/workflow-runs/${encodeURIComponent(input.runId)}/turns`,
      {
        stepId: input.stepId,
        prompt: input.prompt,
        selectedSkills: input.selectedSkills,
        reasoningEffort: input.reasoningEffort,
        model: input.model,
        yoloMode: input.yoloMode,
        attachments: input.attachments,
        scenario: this.scenario,
      },
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

  async *streamRun(runId: string, afterSeq = 0): AsyncIterable<ProviderEventDTO> {
    for await (const ev of this.openStream(runId, afterSeq)) {
      this.lastSeq.set(runId, Math.max(this.lastSeq.get(runId) ?? 0, ev.seq));
      yield ev;
    }
  }

  // openStream parses the SSE body, yielding each event until the connection
  // closes or the consumer stops iterating (which aborts the fetch via finally).
  private async *openStream(runId: string, afterSeq: number, ctrl: AbortController = new AbortController()): AsyncIterable<ProviderEventDTO> {
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
      ctrl.abort();
    }
  }
}

export class RunnerApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "RunnerApiError";
  }
}

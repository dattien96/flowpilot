import type {
  Artifact,
  Project,
  ProviderEventDTO,
  ProviderSkill,
  RunHandle,
  RunnerClient,
  StartRunInput,
  Step,
  TurnInput,
  Workflow,
} from "@/types/contract";

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
  listWorkflows(projectId: string): Promise<Workflow[]> {
    return this.getJSON<Workflow[]>(`/client/projects/${encodeURIComponent(projectId)}/workflows`);
  }
  listSteps(workflowId: string): Promise<Step[]> {
    return this.getJSON<Step[]>(`/client/workflows/${encodeURIComponent(workflowId)}/steps`);
  }
  listArtifacts(runId: string): Promise<Artifact[]> {
    return this.getJSON<Artifact[]>(`/client/workflow-runs/${encodeURIComponent(runId)}/artifacts`);
  }
  listSkills(provider: string): Promise<ProviderSkill[]> {
    return this.getJSON<ProviderSkill[]>(`/client/provider-skills?provider=${encodeURIComponent(provider)}`);
  }

  // ---- run lifecycle -------------------------------------------------------

  startRun(input: StartRunInput): Promise<RunHandle> {
    return this.postJSON<RunHandle>("/client/workflow-runs", input);
  }
  resumeRun(runId: string): Promise<RunHandle> {
    return this.postJSON<RunHandle>(`/client/workflow-runs/${encodeURIComponent(runId)}/resume`);
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
  private async *openStream(runId: string, afterSeq: number): AsyncIterable<ProviderEventDTO> {
    const ctrl = new AbortController();
    const resp = await fetch(
      `${this.base}/client/workflow-runs/${encodeURIComponent(runId)}/events/stream?afterSeq=${afterSeq}`,
      { headers: { Accept: "text/event-stream" }, signal: ctrl.signal },
    );
    if (!resp.ok || !resp.body) {
      ctrl.abort();
      throw new RunnerApiError(resp.status, "stream_failed", `event stream failed: ${resp.status}`);
    }
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    try {
      for (;;) {
        const { done, value } = await reader.read();
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

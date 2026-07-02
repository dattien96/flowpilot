"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.RunnerApiError = exports.HttpWsRunnerClient = void 0;
function mapProviderAccountSummary(raw) {
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
class HttpWsRunnerClient {
    base;
    scenario;
    /** Highest seq seen per run, so a follow-up turn/stream resumes after it. */
    lastSeq = new Map();
    constructor(baseUrl) {
        this.base = baseUrl.replace(/\/+$/, "");
    }
    setScenario(scenario) {
        this.scenario = scenario;
    }
    // ---- plain JSON helpers --------------------------------------------------
    async getJSON(path) {
        const resp = await fetch(this.base + path, { headers: { Accept: "application/json" } });
        return this.parse(resp);
    }
    async postJSON(path, body, headers) {
        const resp = await fetch(this.base + path, {
            method: "POST",
            headers: { "Content-Type": "application/json", Accept: "application/json", ...(headers ?? {}) },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        return this.parse(resp);
    }
    async parse(resp) {
        const text = await resp.text();
        const data = text ? JSON.parse(text) : undefined;
        if (!resp.ok) {
            const err = (data && data.error) || {};
            throw new RunnerApiError(resp.status, err.code ?? "http_error", err.message ?? resp.statusText);
        }
        return data;
    }
    // ---- catalog -------------------------------------------------------------
    listProjects() {
        return this.getJSON("/client/projects");
    }
    listWorkflows() {
        return this.getJSON("/client/workflows");
    }
    listSteps() {
        return this.getJSON("/client/steps");
    }
    listProviderAccounts() {
        return this.getJSON("/client/provider-accounts").then((accounts) => accounts.map(mapProviderAccountSummary));
    }
    listArtifacts(runId) {
        return this.getJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/artifacts`);
    }
    listRunHistory(projectId) {
        return this.getJSON(`/client/projects/${encodeURIComponent(projectId)}/workflow-runs`);
    }
    listRemoteChatSessions(projectId) {
        return this.getJSON(`/client/projects/${encodeURIComponent(projectId)}/chat-sessions/remote`);
    }
    listSkills(provider, cwd) {
        let url = `/client/provider-skills?provider=${encodeURIComponent(provider)}`;
        if (cwd)
            url += `&cwd=${encodeURIComponent(cwd)}`;
        return this.getJSON(url);
    }
    listBuiltinOrchestrationOptions(subMode) {
        const url = `/client/chat/builtin-orchestration-options?subMode=${encodeURIComponent(subMode)}`;
        return this.getJSON(url);
    }
    listAgents(cwd) {
        const url = cwd ? `/client/agents?cwd=${encodeURIComponent(cwd)}` : "/client/agents";
        return this.getJSON(url);
    }
    listAgentRuns(parentRunId) {
        return this.getJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agents`);
    }
    refreshAgentGraph(parentRunId) {
        return this.getJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-graph`);
    }
    pauseAgentLoop(parentRunId) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/pause`); }
    resumeAgentLoop(parentRunId) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/resume`); }
    injectAgentFeedback(parentRunId, toRunId, message) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/feedback`, { toRunId, message }); }
    stopAgentLoop(parentRunId) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/stop`); }
    submitReviewOutcome(parentRunId, input) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/flow-control`, input); }
    extendCap(parentRunId) { return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/agent-loop/extend-cap`); }
    spawnAgent(input) {
        const { parentRunId, ...body } = input;
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(parentRunId)}/spawn-agent`, body);
    }
    focusAgentRun(runId, signal) {
        return this.streamRun(runId, 0, signal);
    }
    // ---- run lifecycle -------------------------------------------------------
    startRun(input) {
        return this.postJSON("/client/workflow-runs", input);
    }
    resumeRun(runId) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/resume`);
    }
    syncChatRun(runId, input) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/sync-chat`, input ?? {});
    }
    async deleteRun(runId) {
        const resp = await fetch(this.base + `/client/workflow-runs/${encodeURIComponent(runId)}`, {
            method: "DELETE",
            headers: { Accept: "application/json" },
        });
        await this.parse(resp);
    }
    restoreChatRun(input) {
        return this.postJSON("/client/chat-sessions/restore", input);
    }
    handoffContext(runId, input) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/handoff-context`, input);
    }
    generateChatSummary(runId) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/chat-summary`, {});
    }
    submitApproval(approvalId, decision) {
        return this.postJSON(`/client/approvals/${encodeURIComponent(approvalId)}/decision`, { decision });
    }
    answerQuestion(questionId, choice) {
        return this.postJSON(`/client/questions/${encodeURIComponent(questionId)}/answer`, { choice });
    }
    interrupt(runId) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/interrupt`);
    }
    submitGateDecision(runId, option, customText) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/gate-decision`, { option, ...(customText ? { customText } : {}) });
    }
    submitGateAgreement(runId, testNames) {
        return this.postJSON(`/client/workflow-runs/${encodeURIComponent(runId)}/gate-agreement`, { testNames });
    }
    async connectProviderAccount(providerKey) {
        await this.postJSON("/provider-accounts/connect", { providerKey });
    }
    activateProviderAccount(accountId) {
        return this.postJSON("/provider-accounts/activate", { accountId });
    }
    openProviderAccountTerminal(accountId) {
        return this.postJSON("/provider-accounts/test", { accountId });
    }
    restartStack() {
        return this.postJSON("/system/restart");
    }
    shutdownStack() {
        return this.postJSON("/system/shutdown");
    }
    // ---- streaming -----------------------------------------------------------
    async *sendTurn(input) {
        const after = this.lastSeq.get(input.runId) ?? 0;
        const { turnId } = await this.postJSON(`/client/workflow-runs/${encodeURIComponent(input.runId)}/turns`, {
            stepId: input.stepId,
            prompt: input.prompt,
            changeType: input.changeType,
            sourceDocId: input.sourceDocId,
            selectedSkills: input.selectedSkills,
            reasoningEffort: input.reasoningEffort,
            model: input.model,
            yoloMode: input.yoloMode,
            attachments: input.attachments,
            subMode: input.subMode,
            flowRef: input.flowRef,
            scenario: this.scenario,
        });
        for await (const ev of this.openStream(input.runId, after)) {
            this.lastSeq.set(input.runId, Math.max(this.lastSeq.get(input.runId) ?? 0, ev.seq));
            if (ev.providerTurnId && ev.providerTurnId !== turnId)
                continue; // filter to this turn
            yield ev;
            if ((ev.type === "turn_completed" || ev.type === "turn_failed") && ev.providerTurnId === turnId) {
                return;
            }
        }
    }
    async *streamRun(runId, afterSeq = 0, signal) {
        for await (const ev of this.openStream(runId, afterSeq, signal)) {
            this.lastSeq.set(runId, Math.max(this.lastSeq.get(runId) ?? 0, ev.seq));
            yield ev;
        }
    }
    // openStream parses the SSE body, yielding each event until the connection
    // closes or the consumer stops iterating (which aborts the fetch via finally).
    async *openStream(runId, afterSeq, signal) {
        const ctrl = new AbortController();
        if (signal?.aborted)
            return;
        const abort = () => ctrl.abort();
        signal?.addEventListener("abort", abort, { once: true });
        let resp;
        try {
            resp = await fetch(`${this.base}/client/workflow-runs/${encodeURIComponent(runId)}/events/stream?afterSeq=${afterSeq}`, { headers: { Accept: "text/event-stream" }, signal: ctrl.signal });
        }
        catch (err) {
            // Aborted because a newer stream superseded this one — end quietly.
            if (ctrl.signal.aborted)
                return;
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
                let chunk;
                try {
                    chunk = await reader.read();
                }
                catch (err) {
                    if (ctrl.signal.aborted)
                        return; // superseded — stop without surfacing AbortError
                    throw err;
                }
                const { done, value } = chunk;
                if (done)
                    return;
                buf += decoder.decode(value, { stream: true });
                let idx;
                while ((idx = buf.indexOf("\n\n")) >= 0) {
                    const frame = buf.slice(0, idx);
                    buf = buf.slice(idx + 2);
                    const dataLine = frame.split("\n").find((l) => l.startsWith("data:"));
                    if (!dataLine)
                        continue;
                    const json = dataLine.slice(dataLine.indexOf(":") + 1).trim();
                    try {
                        yield JSON.parse(json);
                    }
                    catch {
                        // skip malformed frame
                    }
                }
            }
        }
        finally {
            signal?.removeEventListener("abort", abort);
            ctrl.abort();
        }
    }
}
exports.HttpWsRunnerClient = HttpWsRunnerClient;
class RunnerApiError extends Error {
    status;
    code;
    constructor(status, code, message) {
        super(message);
        this.status = status;
        this.code = code;
        this.name = "RunnerApiError";
    }
}
exports.RunnerApiError = RunnerApiError;

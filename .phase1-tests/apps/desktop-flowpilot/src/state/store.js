"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useStore = void 0;
exports.accountLabel = accountLabel;
exports.providerLabel = providerLabel;
const zustand_1 = require("zustand");
const createRunnerClient_1 = require("@/client/createRunnerClient");
const HttpWsRunnerClient_1 = require("@/client/HttpWsRunnerClient");
const ideBridge_1 = require("@/client/ideBridge");
const clientCore_1 = require("@/clientCore");
const config_1 = require("@/config");
const navigatorCatalog_1 = require("@/app/navigatorCatalog");
const timelineReducer_1 = require("./timelineReducer");
let loadProjectsInFlight = null;
let activeHistoryReplayController;
let activeOrchestrationStreamController;
let activeAgentFocusStreamController;
function applyAgentGraphSnapshot(snapshot) {
    return {
        agentGraphSnapshot: snapshot,
        agentBusMessages: snapshot.busMessages,
    };
}
function pickDefaultModel(provider, models) {
    if (!provider)
        return undefined;
    const enabled = models.filter((m) => m.providerKey === provider && m.isEnabled);
    if (provider === "codex") {
        return (enabled.find((m) => m.modelId.toLowerCase().includes("5.5"))?.modelId ??
            enabled.find((m) => m.modelId.toLowerCase().includes("5.4") && !m.modelId.toLowerCase().includes("mini"))?.modelId ??
            enabled.find((m) => !m.modelId.toLowerCase().includes("mini"))?.modelId ??
            enabled[0]?.modelId);
    }
    if (provider === "claude") {
        return enabled.find((m) => m.modelId.toLowerCase().includes("sonnet"))?.modelId;
    }
    return undefined;
}
function selectedProjectPath(state) {
    return state.projects.find((project) => project.id === state.selectedProjectId)?.path;
}
function cancelHistoryReplayStream() {
    activeHistoryReplayController?.abort();
    activeHistoryReplayController = undefined;
}
function cancelOrchestrationStream() {
    activeOrchestrationStreamController?.abort();
    activeOrchestrationStreamController = undefined;
}
function cancelAgentFocusStream() {
    activeAgentFocusStreamController?.abort();
    activeAgentFocusStreamController = undefined;
}
exports.useStore = (0, zustand_1.create)((set, get) => ({
    client: (0, createRunnerClient_1.createRunnerClient)(),
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
    workspaceMainView: "chat",
    _historyLoadSeq: 0,
    _remoteHistoryLoadSeq: 0,
    _runSnapshots: {},
    _runReplaySeq: {},
    _streamRunSeq: 0,
    _orchestrationStreamSeq: 0,
    agentSpawnGuideAgentName: undefined,
    agentSpawnGuideOpen: false,
    async loadProjects() {
        if (loadProjectsInFlight)
            return loadProjectsInFlight;
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
            const withRetry = async (fn) => {
                let lastErr;
                for (let i = 0; i < 10; i++) {
                    try {
                        return await fn();
                    }
                    catch (err) {
                        lastErr = err;
                        if (err instanceof HttpWsRunnerClient_1.RunnerApiError)
                            throw err; // got an HTTP response — real error
                        await new Promise((r) => setTimeout(r, 1500)); // connection refused — runner still booting
                    }
                }
                throw lastErr;
            };
            try {
                const projects = await withRetry(() => client.listProjects());
                set((s) => ({ projects, ...(s.runId ? {} : { status: "idle" }) }));
            }
            catch (err) {
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
                const admin = await (0, clientCore_1.getAdminUseCases)();
                const [workflowDefinitions, stepDefinitions, supportedModels] = await Promise.all([
                    admin.workflows.listWorkflows(),
                    admin.workflows.listStepDefinitions(),
                    admin.providers.listSupportedModels(),
                ]);
                set({
                    workflows: workflowDefinitions.map(navigatorCatalog_1.mapNavigatorWorkflow),
                    steps: stepDefinitions.map(navigatorCatalog_1.mapNavigatorStep),
                    supportedModels,
                    // Apply default model for the current provider if none is selected yet
                    ...(!get().selectedModel ? { selectedModel: pickDefaultModel(get().selectedProvider, supportedModels) } : {}),
                });
            }
            catch (err) {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] definition catalog failed:", err);
            }
            try {
                const provider = get().selectedProvider ?? "codex";
                const skills = await withRetry(() => client.listSkills(provider, selectedProjectPath(get())));
                set({ skills });
            }
            catch (err) {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] listSkills failed:", err);
            }
            try {
                const providerAccounts = await withRetry(() => client.listProviderAccounts());
                set({ providerAccounts });
            }
            catch (err) {
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
        }
        catch (err) {
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
        try {
            const agentRuns = await client.listAgentRuns(parentRunId);
            if (get().mainRunId === parentRunId || get().runId === parentRunId) {
                set({ agentRuns });
            }
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] listAgentRuns failed:", err);
        }
    },
    async refreshAgentGraph() {
        const { client, mainRunId, runId } = get();
        const parentRunId = mainRunId ?? runId;
        if (!parentRunId || !client.refreshAgentGraph)
            return;
        set(applyAgentGraphSnapshot(await client.refreshAgentGraph(parentRunId)));
    },
    async pauseAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.pauseAgentLoop)
        set(applyAgentGraphSnapshot(await client.pauseAgentLoop(parentRunId))); },
    async resumeAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.resumeAgentLoop)
        set(applyAgentGraphSnapshot(await client.resumeAgentLoop(parentRunId))); },
    async injectAgentFeedback(toRunId, message) { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.injectAgentFeedback)
        set(applyAgentGraphSnapshot(await client.injectAgentFeedback(parentRunId, toRunId, message))); },
    async stopAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.stopAgentLoop)
        set(applyAgentGraphSnapshot(await client.stopAgentLoop(parentRunId))); },
    async listAgents(cwd) {
        const { client } = get();
        if (!client.listAgents)
            return [];
        try {
            return await client.listAgents(cwd ?? selectedProjectPath(get()));
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] listAgents failed:", err);
            return [];
        }
    },
    async focusAgentRun(runId) {
        const { client } = get();
        const currentRunId = get().runId;
        const mainRunId = get().mainRunId ?? currentRunId;
        if (!mainRunId)
            return;
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
        }
        catch (err) {
            if (activeAgentFocusStreamController === agentFocusController) {
                activeAgentFocusStreamController = undefined;
            }
            if (!(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== streamRunSeq)
                return;
            set((s) => ({
                status: "failed",
                timeline: [
                    ...s.timeline,
                    { kind: "system", id: `agent-focus-error-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
                ],
            }));
            return;
        }
        if (!(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== streamRunSeq) {
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
            if (!(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== streamRunSeq)
                return;
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
        if (!mainRunId)
            return;
        cacheRunSnapshot(get(), currentRunId);
        const restore = _runSnapshots[mainRunId];
        if (!restore)
            return;
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
        void consumeAgentStream(mainRunId, get().client.streamRun(mainRunId, afterSeq, agentFocusController.signal), streamRunSeq, afterSeq, restore.status, set, get).finally(() => {
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
    async loadSkills(provider, cwd) {
        const { client } = get();
        try {
            const skills = await client.listSkills(provider, cwd ?? selectedProjectPath(get()));
            set({ skills });
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] loadSkills failed:", err);
        }
    },
    async sendPrompt(prompt, skills, attachments) {
        const { client, chatMode, launchMode, selectedProjectId, selectedWorkflowId, selectedStepId, selectedProvider, selectedModel, reasoningEffort, yoloMode, } = get();
        const focusedRunId = get().activeAgentRunId;
        const mainRunId = get().mainRunId ?? get().runId;
        if (chatMode === "normal_chat" && focusedRunId && mainRunId && focusedRunId !== mainRunId) {
            get().appendSystemMessage("Child transcript is read-only. Return to the main chat to send prompts.");
            return;
        }
        const cwd = selectedProjectPath(get());
        if (chatMode === "normal_chat") {
            if (!selectedProjectId || !selectedProvider)
                return;
        }
        else {
            const launchTargetId = launchMode === "workflow" ? selectedWorkflowId : selectedStepId;
            if (!selectedProjectId || !launchTargetId)
                return;
        }
        const launchTargetId = launchMode === "workflow" ? selectedWorkflowId : selectedStepId;
        // In normal_chat the runner mints one synthetic step ("chat-<runId>") for the whole
        // run and surfaces it via startRun (turn 1) / resumeRun (from history). It is held in
        // `activeStepId` so follow-up turns reuse it instead of sending an empty stepId, which
        // startTurn rejects with 400. Workflow/step mode keeps using its stable launchTargetId.
        let turnStepId = chatMode === "normal_chat"
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
                    attachments: attachments && attachments.length > 0
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
                const handle = await client.startRun(chatMode === "normal_chat"
                    ? {
                        projectId: selectedProjectId,
                        providerKey: selectedProvider,
                        model: selectedModel,
                        reasoningEffort,
                        yoloMode,
                        chatMode: "normal_chat",
                        cwd,
                    }
                    : {
                        projectId: selectedProjectId,
                        workflowId: launchMode === "workflow" ? selectedWorkflowId : undefined,
                        stepId: launchTargetId || undefined,
                        providerKey: selectedProvider,
                        cwd,
                    });
                runId = handle.runId;
                if (handle.stepId) {
                    turnStepId = handle.stepId;
                }
                set({
                    mainRunId: handle.runId,
                    activeAgentRunId: undefined,
                });
            }
            const turnInput = {
                runId,
                stepId: turnStepId,
                prompt,
                selectedSkills: skills && skills.length > 0
                    ? skills.map((name) => {
                        // Carry the picker's absolute skill path so the runner reads the exact
                        // selected skill file (BUG-063 follow-up) instead of matching by name/id,
                        // which is fragile across provider layouts and the run's cwd.
                        const found = get().skills.find((s) => s.name === name);
                        return { name, path: found?.path, source: "slash_picker" };
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
                attachments: chatMode === "normal_chat" && attachments && attachments.length > 0
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
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] sendPrompt failed:", err);
            if (runId && !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId)) {
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
        }
        finally {
            void get().loadRunHistory();
        }
    },
    async approve(decision) {
        const pending = get().pendingApproval;
        if (!pending)
            return;
        set((s) => ({
            pendingApproval: undefined,
            status: "running",
            timeline: s.timeline.map((it) => it.kind === "approval" && it.approvalId === pending.approvalId ? { ...it, decision } : it),
        }));
        await get().client.submitApproval(pending.approvalId, decision);
    },
    async answer(choice) {
        const pending = get().pendingQuestion;
        if (!pending)
            return;
        set((s) => ({
            pendingQuestion: undefined,
            status: "running",
            timeline: s.timeline.map((it) => it.kind === "question" && it.questionId === pending.questionId ? { ...it, answer: choice } : it),
        }));
        await get().client.answerQuestion(pending.questionId, choice);
    },
    async stop() {
        const { client, runId } = get();
        if (!runId)
            return;
        await client.interrupt(runId);
    },
    async reconnect() {
        const { client, runId } = get();
        if (!runId)
            return;
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
            if (get()._historyLoadSeq !== seq)
                return;
            set({ runHistory, historyLoading: false, historyLoadError: undefined });
        }
        catch (err) {
            if (get()._historyLoadSeq !== seq)
                return;
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
            if (get()._remoteHistoryLoadSeq !== seq)
                return;
            set({ remoteChatSessions, remoteHistoryLoading: false, remoteHistoryLoadError: undefined });
        }
        catch (err) {
            if (get()._remoteHistoryLoadSeq !== seq)
                return;
            set({ remoteHistoryLoading: false, remoteHistoryLoadError: String(err) });
        }
    },
    async syncHistoryRun(runId, projectId) {
        const { client, selectedProjectId } = get();
        const driveProjectId = projectId ?? selectedProjectId;
        set((s) => ({
            runHistory: s.runHistory.map((item) => item.runId === runId
                ? {
                    ...item,
                    syncStatus: "syncing",
                    unavailableReason: undefined,
                }
                : item),
        }));
        try {
            const result = await client.syncChatRun(runId, driveProjectId ? { googleDriveProjectId: driveProjectId } : undefined);
            set((s) => ({
                runHistory: s.runHistory.map((item) => item.runId === runId
                    ? {
                        ...item,
                        sourceMachineId: result.sourceMachineId,
                        sourceRunId: result.sourceRunId,
                        syncStatus: result.syncStatus,
                        unavailableReason: undefined,
                    }
                    : item),
            }));
            void get().loadRemoteChatSessions();
        }
        catch (err) {
            if (err instanceof HttpWsRunnerClient_1.RunnerApiError &&
                (err.code === "session_unavailable" ||
                    err.code === "account_not_signed_in" ||
                    err.code === "resume_unsupported" ||
                    err.code === "account_unavailable" ||
                    err.code === "google_drive_not_connected")) {
                set((s) => ({
                    runHistory: s.runHistory.map((item) => item.runId === runId ? { ...item, syncStatus: "failed", unavailableReason: err.message } : item),
                }));
                return;
            }
            set((s) => ({
                runHistory: s.runHistory.map((item) => item.runId === runId ? { ...item, syncStatus: "failed" } : item),
            }));
            throw err;
        }
    },
    async syncAllInProject(projectId) {
        // Sync every not-yet-synced chat run in the project, one at a time so we do
        // not hammer Drive. Per-item failures are swallowed (syncHistoryRun marks
        // the row failed) so one broken session does not abort the whole batch.
        const targets = get()
            .runHistory.filter((item) => item.projectId === projectId &&
            item.runKind === "chat" &&
            item.syncStatus !== "synced" &&
            !item.unavailableReason)
            .map((item) => item.runId);
        for (const runId of targets) {
            try {
                await get().syncHistoryRun(runId, projectId);
            }
            catch {
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
        }
        catch (err) {
            // Restore the item on failure by refreshing history from the runner.
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] deleteRun failed:", err);
            const { selectedProjectId } = get();
            if (selectedProjectId) {
                try {
                    const runHistory = await client.listRunHistory(selectedProjectId);
                    set({ runHistory });
                }
                catch {
                    // best-effort refresh
                }
            }
            throw err;
        }
    },
    async restoreRemoteChatSession(summary, cwd) {
        const { client, selectedProjectId } = get();
        if (!selectedProjectId)
            return;
        const request = {
            projectId: selectedProjectId,
            sourceMachineId: summary.sourceMachineId,
            sourceRunId: summary.sourceRunId,
            cwd,
        };
        try {
            const result = await client.restoreChatRun(request);
            await Promise.all([get().loadRunHistory(), get().loadRemoteChatSessions()]);
            void get().openHistoryRun(result.runId);
        }
        catch (err) {
            if (err instanceof HttpWsRunnerClient_1.RunnerApiError && err.code === "cwd_remap_required" && !cwd) {
                const retryCwd = selectedProjectPath(get());
                if (retryCwd) {
                    await get().restoreRemoteChatSession(summary, retryCwd);
                    return;
                }
            }
            if (err instanceof HttpWsRunnerClient_1.RunnerApiError &&
                (err.code === "sync_remote_not_found" ||
                    err.code === "sync_integrity_failed" ||
                    err.code === "account_not_signed_in" ||
                    err.code === "account_unavailable" ||
                    err.code === "cwd_remap_required" ||
                    err.code === "session_file_conflict")) {
                set((s) => ({
                    remoteChatSessions: s.remoteChatSessions.map((item) => item.sourceMachineId === summary.sourceMachineId && item.sourceRunId === summary.sourceRunId
                        ? { ...item, unavailableReason: err.message }
                        : item),
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
        }
        catch (err) {
            if (err instanceof HttpWsRunnerClient_1.RunnerApiError) {
                console.error("[FlowPilot][history-open] resume failed", {
                    runId,
                    status: err.status,
                    code: err.code,
                    message: err.message,
                });
                set((s) => ({
                    runHistory: s.runHistory.map((item) => item.runId === runId ? { ...item, unavailableReason: err.message } : item),
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
            _streamRunSeq: get()._streamRunSeq + 1,
            ...(historyProvider
                ? {
                    selectedProvider: historyProvider,
                    selectedModel: pickDefaultModel(historyProvider, get().supportedModels),
                }
                : {}),
            runHistory: get().runHistory.map((item) => item.runId === runId ? { ...item, unavailableReason: undefined } : item),
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
        void consumeHistoryReplayStream(handle.runId, handle.status, client.streamRun(handle.runId, 0, historyReplayController.signal), set, get, handle.lastEventSeq)
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
            selectedModel: pickDefaultModel(selectedProvider, supportedModels),
        });
    },
    openInIde(path, line) {
        void ideBridge_1.ideBridge.openInIde(path, line);
    },
    openAdminWeb() {
        void ideBridge_1.ideBridge.openExternal(config_1.ADMIN_WEB_URL);
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
    requestManualAccountSwitch() {
        const { selectedProvider, providerAccounts } = get();
        if (!selectedProvider)
            return;
        const activeAccount = providerAccounts.find((a) => a.providerKey === selectedProvider && a.isActive);
        // For manual pick, ignore _accountSwitchTriedIds — user is proactively choosing.
        const skipIds = activeAccount ? [activeAccount.id] : [];
        const candidate = findBestCandidate(providerAccounts, selectedProvider, skipIds);
        if (!candidate)
            return;
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
        if (!pendingAccountSwitch)
            return;
        const { providerKey, candidateAccount, reason } = pendingAccountSwitch;
        const newLabel = accountLabel(candidateAccount);
        const shouldRetry = reason === "usage_limit" && !!lastTurnInput;
        set({ accountSwitchLoading: true, pendingAccountSwitch: undefined });
        try {
            await client.activateProviderAccount(candidateAccount.id);
            await get().loadProviderAccounts();
        }
        catch (err) {
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
            await retryWithTurnInput(client, lastTurnInput, set, get);
        }
    },
}));
// ── Account-switch helpers ─────────────────────────────────────────────────
function isUsageLimitMessage(msg) {
    const lower = msg.toLowerCase();
    return (lower.includes("usage limit reached") ||
        lower.includes("extra usage unavailable") ||
        lower.includes("out of credits") ||
        lower.includes("out_of_credits") ||
        lower.includes("quota reset") ||
        lower.includes("rate limit"));
}
function accountLabel(account) {
    return account.accountEmail ?? account.accountName ?? account.displayLabel ?? account.displayName;
}
function providerLabel(providerKey) {
    if (providerKey === "claude")
        return "Claude";
    if (providerKey === "codex")
        return "Codex";
    return providerKey;
}
function findBestCandidate(accounts, providerKey, triedAccountIds) {
    const candidates = accounts.filter((a) => a.providerKey === providerKey &&
        a.authStatus === "connected" &&
        !a.isActive &&
        !triedAccountIds.includes(a.id));
    // Prefer candidates with valid numeric quota data (both windows > 0)
    const withQuota = candidates.filter((a) => a.remaining5hPercent !== null &&
        a.remaining7dPercent !== null &&
        a.remaining5hPercent > 0 &&
        a.remaining7dPercent > 0);
    if (withQuota.length > 0) {
        return [...withQuota].sort((a, b) => {
            const d5h = (b.remaining5hPercent ?? 0) - (a.remaining5hPercent ?? 0);
            if (d5h !== 0)
                return d5h;
            const d7d = (b.remaining7dPercent ?? 0) - (a.remaining7dPercent ?? 0);
            if (d7d !== 0)
                return d7d;
            return a.slotIndex - b.slotIndex;
        })[0];
    }
    // Fall back to candidates where quota telemetry is genuinely unavailable (both null).
    // Accounts with known-zero quota (0) are excluded — they're confirmed exhausted.
    const withUnknownQuota = candidates.filter((a) => a.remaining5hPercent === null && a.remaining7dPercent === null);
    return [...withUnknownQuota].sort((a, b) => a.slotIndex - b.slotIndex)[0];
}
async function retryWithTurnInput(client, turnInput, set, get) {
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
    }
    catch (err) {
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
    }
    finally {
        void get().loadRunHistory();
    }
}
// ── Stream consumer ────────────────────────────────────────────────────────
// Consumes a turn stream and folds each event into the timeline + status.
// mySeq captures _streamRunSeq at call time; if the counter advances (because
// openHistoryRun or reconnect started a newer stream for the same runId) this
// stream exits immediately rather than applying stale events. (BUG-079)
async function consumeStream(runId, stream, set, get) {
    const mySeq = get()._streamRunSeq;
    const isStale = () => !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== mySeq;
    for await (const e of stream) {
        if (isStale()) {
            return;
        }
        if (!isEventForRun(e, runId))
            continue;
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
                                reason: "usage_limit",
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
async function consumeHistoryReplayStream(runId, resumedStatus, stream, set, get, lastEventSeq) {
    const mySeq = get()._streamRunSeq;
    const isStale = () => !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== mySeq;
    for await (const e of stream) {
        if (isStale())
            return;
        if (!isEventForRun(e, runId))
            continue;
        if (e.type !== "agent_graph_updated" && e.type !== "agent_bus_message") {
            set((s) => applyEvent(s, e));
            settleTerminalReplayVisuals(runId, resumedStatus, set);
        }
        if (shouldStopHistoryReplay(resumedStatus, e, lastEventSeq))
            break;
    }
    if (!isStale()) {
        settleHistoryReplayPendingState(runId, resumedStatus, set);
    }
}
function shouldStopHistoryReplay(resumedStatus, e, lastEventSeq) {
    // Preferred path (BUG-112): the runner reports the seq of the last persisted event.
    // Stop only once we've replayed up to it, so a multi-turn transcript is replayed in
    // full instead of being truncated at the first turn_completed. A "running" run keeps
    // live-tailing (more events will arrive), so never stop it on the seq cursor.
    if (typeof lastEventSeq === "number" && lastEventSeq > 0) {
        if (resumedStatus === "running" || resumedStatus === "starting")
            return false;
        return e.seq >= lastEventSeq;
    }
    // Fallback for runners that predate lastEventSeq: stop at the first terminal event.
    if (resumedStatus === "waiting_approval")
        return e.type === "permission_required";
    if (resumedStatus === "waiting_question")
        return e.type === "user_question_required";
    if (resumedStatus === "completed")
        return e.type === "turn_completed";
    if (resumedStatus === "failed" || resumedStatus === "cancelled")
        return e.type === "turn_failed";
    return false;
}
function settleHistoryReplayPendingState(runId, resumedStatus, set) {
    if (resumedStatus === "waiting_approval" || resumedStatus === "waiting_question")
        return;
    set((s) => {
        if (s.runId !== runId || (!s.pendingApproval && !s.pendingQuestion))
            return {};
        const pendingApproval = s.pendingApproval;
        const pendingQuestion = s.pendingQuestion;
        const lastMeaningfulItem = [...s.timeline].reverse().find((it) => it.kind !== "thinking");
        const approvalStillOpen = pendingApproval !== undefined &&
            lastMeaningfulItem?.kind === "approval" &&
            lastMeaningfulItem.approvalId === pendingApproval.approvalId &&
            lastMeaningfulItem.decision === undefined;
        const questionStillOpen = pendingQuestion !== undefined &&
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
                if (pendingApproval &&
                    it.kind === "approval" &&
                    it.approvalId === pendingApproval.approvalId &&
                    it.decision === undefined) {
                    return { ...it, decision: "resolved" };
                }
                if (pendingQuestion &&
                    it.kind === "question" &&
                    it.questionId === pendingQuestion.questionId &&
                    it.answer === undefined) {
                    return { ...it, answer: "answered" };
                }
                return it;
            }),
        };
    });
}
async function consumeAgentStream(runId, stream, streamRunSeq, afterSeq, replayStatus, set, get) {
    const isStale = () => !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId) || get()._streamRunSeq !== streamRunSeq;
    for await (const e of stream) {
        if (isStale())
            return;
        if (!isEventForRun(e, runId))
            continue;
        if (e.seq <= afterSeq)
            continue;
        // Skip orchestration events — consumeOrchestrationStream owns agent_graph_updated
        // and agent_bus_message. Processing them here would duplicate agentBusMessages
        // entries when the gap between restore.lastEventSeq and afterSeq is replayed. (BUG-109)
        if (e.type === "agent_graph_updated" || e.type === "agent_bus_message")
            continue;
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
async function consumeOrchestrationStream(runId, stream, orchestrationSeq, afterSeq, set, get) {
    const isStale = () => !(0, timelineReducer_1.shouldApplyRunEvent)(get().mainRunId ?? get().runId, runId) || get()._orchestrationStreamSeq !== orchestrationSeq;
    for await (const e of stream) {
        if (isStale())
            return;
        if (!isEventForRun(e, runId))
            continue;
        if (e.seq <= afterSeq)
            continue;
        if (e.type !== "agent_graph_updated" && e.type !== "agent_bus_message")
            continue;
        set((s) => applyOrchestrationEvent(s, e));
    }
}
/**
 * [coding-skill]: keeps the parent orchestration SSE independent from the turn stream.
 * [testing-skill]: isolates the live graph/bus path so store tests can assert late updates.
 */
function startOrchestrationStream(runId, client, set, get) {
    if (!runId)
        return;
    cancelOrchestrationStream();
    const orchestrationController = new AbortController();
    activeOrchestrationStreamController = orchestrationController;
    const orchestrationSeq = get()._orchestrationStreamSeq + 1;
    const afterSeq = get()._runReplaySeq[runId] ?? 0;
    set((_s) => ({ _orchestrationStreamSeq: orchestrationSeq }));
    void consumeOrchestrationStream(runId, client.streamRun(runId, afterSeq, orchestrationController.signal), orchestrationSeq, afterSeq, set, get).finally(() => {
        if (activeOrchestrationStreamController === orchestrationController) {
            activeOrchestrationStreamController = undefined;
        }
    });
}
function isEventForRun(e, runId) {
    if (e.type === "agent_graph_updated") {
        return e.agentGraphSnapshot.parentRunId === runId;
    }
    if (e.type === "agent_bus_message") {
        return e.agentBusMessage.parentRunId === runId;
    }
    return e.workflowRunId === runId;
}
function isTerminalRunStatus(status) {
    return status === "completed" || status === "failed" || status === "cancelled";
}
function settleTerminalReplayVisuals(runId, replayStatus, set) {
    if (!isTerminalRunStatus(replayStatus))
        return;
    set((s) => {
        if (s.runId !== runId)
            return {};
        return {
            status: replayStatus,
            timeline: s.timeline.filter((it) => it.kind !== "thinking"),
        };
    });
}
function applyEvent(s, e) {
    const next = (0, timelineReducer_1.applyTimelineEvent)(s, e);
    const nextReplaySeq = {
        ...s._runReplaySeq,
        [e.workflowRunId]: e.seq,
    };
    if (e.type === "agent_graph_updated") {
        return {
            ...next,
            agentRuns: e.agentGraphSnapshot.runs,
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
    if (e.type === "turn_started") {
        return { ...next, latestTokenUsage: undefined, _runReplaySeq: nextReplaySeq };
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
function applyOrchestrationEvent(s, e) {
    const nextReplaySeq = { ...s._runReplaySeq, [e.workflowRunId]: e.seq };
    if (e.type === "agent_graph_updated") {
        return {
            agentRuns: e.agentGraphSnapshot.runs,
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
function runErrorMessage(err) {
    if (err instanceof HttpWsRunnerClient_1.RunnerApiError) {
        const suffix = `HTTP ${err.status}${err.code ? ` / ${err.code}` : ""}`;
        return err.message ? `${err.message} (${suffix})` : `Runner request failed (${suffix})`;
    }
    if (err instanceof Error) {
        return err.message;
    }
    return String(err);
}
function snapshotRunState(state) {
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
function restoreRunSnapshot(snapshot) {
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
function emptyRunSnapshot(status) {
    return {
        timeline: [],
        artifacts: [],
        status,
        pendingApproval: undefined,
        pendingQuestion: undefined,
        latestTokenUsage: undefined,
        lastTurnInput: undefined,
        recoverable: false,
        _streamingAssistantId: undefined,
        activeStepId: undefined,
    };
}
function cacheRunSnapshot(state, runId) {
    if (!runId)
        return;
    state._runSnapshots[runId] = snapshotRunState(state);
}

"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useStore = void 0;
exports.accountLabel = accountLabel;
exports.providerLabel = providerLabel;
exports.isFlowModeRun = isFlowModeRun;
exports.activeWorkflowStep = activeWorkflowStep;
exports.hasRetries = hasRetries;
exports.orderHistoryReplayEvents = orderHistoryReplayEvents;
exports.mergeAgentRunsById = mergeAgentRunsById;
exports.deriveOrchestrationRunStatus = deriveOrchestrationRunStatus;
exports.settleCompletedFlowTimeline = settleCompletedFlowTimeline;
const zustand_1 = require("zustand");
const createRunnerClient_1 = require("@/client/createRunnerClient");
const HttpWsRunnerClient_1 = require("@/client/HttpWsRunnerClient");
const ideBridge_1 = require("@/client/ideBridge");
const clientCore_1 = require("@/clientCore");
const config_1 = require("@/config");
const navigatorHistory_1 = require("@/components/navigatorHistory");
const navigatorCatalog_1 = require("@/app/navigatorCatalog");
const timelineReducer_1 = require("./timelineReducer");
const LAST_PROJECT_KEY = "fp:lastProjectId";
let loadProjectsInFlight = null;
let activeHistoryReplayController;
let activeOrchestrationStreamController;
let activeAgentFocusStreamController;
function sanitizePendingSnapshotState(status, pendingApprovals, pendingQuestions) {
    return {
        pendingApprovals: status === "waiting_approval" ? pendingApprovals : [],
        pendingQuestions: status === "waiting_question" ? pendingQuestions : [],
    };
}
function applyAgentGraphSnapshot(snapshot) {
    return {
        agentRuns: snapshot.runs,
        agentGraphSnapshot: snapshot,
        agentBusMessages: snapshot.busMessages,
    };
}
/**
 * Fire-and-forget HTTP agent-graph refresh with stale-response guard
 * (same pattern as refreshAgentRuns / BUG-130). Used when loop is blocked
 * after restart so FlowAwaitingUserCard gets loopState without inventing an
 * SSE seq that can race later control actions (Continue/Stop).
 */
function requestAgentGraphRefresh(parentRunId, get, set, opts) {
    const client = get().client;
    if (!client.refreshAgentGraph)
        return;
    const loadSeq = get()._agentGraphLoadSeq + 1;
    set({ _agentGraphLoadSeq: loadSeq });
    void client
        .refreshAgentGraph(parentRunId)
        .then((snap) => {
        if (get()._agentGraphLoadSeq !== loadSeq)
            return;
        if (!(0, timelineReducer_1.shouldApplyRunEvent)(get().mainRunId ?? get().runId, parentRunId))
            return;
        set((s) => {
            // Do not regress a post-Continue/Stop graph with a late blocked HTTP snap.
            const currentLoop = s.agentGraphSnapshot?.loopState?.status;
            if (currentLoop &&
                currentLoop !== "blocked" &&
                snap.loopState?.status === "blocked") {
                return {};
            }
            const nextStatus = deriveOrchestrationRunStatus(s.status, snap);
            const settle = opts?.settleBlockedTimeline &&
                (snap.loopState?.status === "blocked" ||
                    (snap.loopState?.status === "done" && nextStatus === "completed"));
            return {
                agentRuns: mergeAgentRunsById(s.agentRuns, snap.runs),
                agentGraphSnapshot: snap,
                agentBusMessages: snap.busMessages,
                ...(opts?.preferBlockedStatus && nextStatus === "blocked" ? { status: nextStatus } : {}),
                ...(settle ? { timeline: settleCompletedFlowTimeline(s.timeline) } : {}),
            };
        });
    })
        .catch(() => {
        /* best-effort */
    });
}
/**
 * Stop is terminal from the user's perspective even if its durable checkpoint
 * reply fails after the runner has already applied the RAM cancellation. Keep
 * cached parent/child views consistent with that contract so child focus cannot
 * restore a stale running snapshot when the user returns to main.
 */
function reconcileStoppedRunSnapshots(snapshots, parentRunID, graph) {
    const reportedStatusByID = new Map(graph?.runs.map((run) => [run.runId, run.status]) ?? []);
    return Object.fromEntries(Object.entries(snapshots).map(([snapshotRunID, saved]) => {
        const reportedStatus = reportedStatusByID.get(snapshotRunID);
        const status = snapshotRunID === parentRunID
            ? "cancelled"
            : reportedStatus && isTerminalRunStatus(reportedStatus)
                ? reportedStatus
                : isTerminalRunStatus(saved.status)
                    ? saved.status
                    : "cancelled";
        if (status === saved.status)
            return [snapshotRunID, saved];
        return [
            snapshotRunID,
            {
                ...saved,
                status,
                timeline: saved.timeline.filter((item) => item.kind !== "thinking"),
                pendingApprovals: [],
                pendingQuestions: [],
                recoverable: false,
                _streamingAssistantId: undefined,
            },
        ];
    }));
}
// CA-685: mirror the TUI's posturePinProvider — infer the provider a pinned
// model belongs to (prefix rules first, runner BUG-171 routing stays the SSOT).
function providerKeyForPinnedModel(modelId) {
    const id = (modelId ?? "").trim();
    if (!id)
        return null;
    if (id.startsWith("gpt-"))
        return "codex";
    if (id.startsWith("gemini-") || id.startsWith("auto-gemini-"))
        return "gemini";
    if (id.startsWith("claude-"))
        return "claude";
    if (id.startsWith("grok-") || id === "grok-build")
        return "grok";
    if (id.startsWith("opencode/") || id.startsWith("opencode-go/"))
        return "opencode";
    return null;
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
    if (provider === "grok") {
        // Appended last (CP-46 P-0/Task-211 T-6). Prefer grok-4.5 (the model
        // live-verified against Grok Build 0.2.93) over the grok-build alias.
        return (enabled.find((m) => m.modelId.toLowerCase() === "grok-4.5")?.modelId ??
            enabled[0]?.modelId);
    }
    if (provider === "opencode") {
        // Appended last (CP-57 P-0/Task-303 T-1).
        return (enabled.find((m) => m.modelId === "opencode/muse-spark-1.2-contributor-free")?.modelId ??
            enabled.find((m) => m.modelId === "opencode/gpt-5.4-nano")?.modelId ??
            enabled[0]?.modelId);
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
    localProviders: [],
    supportedModels: [],
    status: "idle",
    timeline: [],
    artifacts: [],
    pendingApprovals: [],
    pendingQuestions: [],
    runHistory: [],
    remoteChatSessions: [],
    syncBatchProgress: undefined,
    agentRuns: [],
    agentGraphSnapshot: undefined,
    agentBusMessages: [],
    workflowStepRuntime: [],
    workflowStepRuntimeLoading: false,
    workflowStepRuntimeMeta: {},
    historyLoading: false,
    remoteHistoryLoading: false,
    latestTokenUsage: undefined,
    recoverable: false,
    scenario: "normal",
    accountSwitchLoading: false,
    providerSwitchLoading: false,
    _accountSwitchTriedIds: [],
    historyOpenError: undefined,
    launchMode: "workflow",
    chatMode: "normal_chat",
    selectedProvider: "codex",
    yoloMode: false,
    grokYoloPostureLoading: false,
    summaryGenerating: false,
    chatStartMode: "normal",
    chatSourceDocId: "",
    // CA-685: "non" is the no-mode default — the session keeps the user's
    // choices; the runner SSOT overrides this once the posture doc loads.
    chatPosture: "non",
    chatPostureConfig: {
        active: "non",
        profiles: { scan: {}, plan: {}, code: {}, non: {} },
    },
    chatPostureSetupOpen: false,
    flowRef: undefined,
    builtinOrchestrationOptions: [],
    workspaceMainView: "chat",
    _historyReplaying: false,
    _historyLoadSeq: 0,
    _remoteHistoryLoadSeq: 0,
    _agentRunsLoadSeq: 0,
    _agentGraphLoadSeq: 0,
    _workflowStepRuntimeLoadSeq: 0,
    _runSnapshots: {},
    _runReplaySeq: {},
    _gateBlockedRunIds: {},
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
                if (!get().selectedProjectId && projects.length > 0) {
                    const saved = localStorage.getItem(LAST_PROJECT_KEY);
                    const match = saved ? projects.find((p) => p.id === saved) : undefined;
                    void get().selectProject((match ?? projects[0]).id);
                }
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
                const [workflowDefinitions, stepDefinitions, supportedModels, localProviders] = await Promise.all([
                    admin.workflows.listWorkflows(),
                    admin.workflows.listStepDefinitions(),
                    admin.providers.listSupportedModels(),
                    admin.providers.listLocalProviders(),
                ]);
                set({
                    workflows: workflowDefinitions.map(navigatorCatalog_1.mapNavigatorWorkflow),
                    steps: stepDefinitions.map(navigatorCatalog_1.mapNavigatorStep),
                    localProviders,
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
            // CP-56 restart restore: resume the runner-persisted active posture
            // (scan/plan/code) and its pinned profile after Desktop reopen — mirrors
            // the TUI SessionDefaultsMsg restore. Uses the same withRetry so the
            // boot-time GET survives the runner compile window.
            try {
                const getPosture = client.getChatPosture;
                if (getPosture) {
                    const config = await withRetry(() => getPosture());
                    const active = config.active ?? get().chatPosture;
                    const profile = config.profiles[active] ?? {};
                    set(() => ({
                        chatPostureConfig: config,
                        chatPosture: active,
                        ...(profile.provider ? { selectedProvider: profile.provider } : {}),
                        ...(profile.model ? { selectedModel: profile.model } : {}),
                        ...(profile.reasoningEffort ? { reasoningEffort: profile.reasoningEffort } : {}),
                        ...(typeof profile.yolo === "boolean" ? { yoloMode: profile.yolo } : {}),
                    }));
                    if (get().selectedProvider === "grok" && typeof profile.yolo === "boolean") {
                        void get().toggleYoloForActiveProvider(profile.yolo);
                    }
                }
            }
            catch (err) {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] loadChatPostureConfig (boot) failed:", err);
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
    async loadLocalProviders() {
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const localProviders = await admin.providers.listLocalProviders();
            set({ localProviders });
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] listLocalProviders refresh failed:", err);
        }
    },
    async refreshAgentRuns() {
        const { client, mainRunId, runId } = get();
        const parentRunId = mainRunId ?? runId;
        if (!parentRunId || !client.listAgentRuns) {
            set({ agentRuns: [] });
            return;
        }
        // Stale-response guard (BUG-130): this fetch is fire-and-forget and can land after a
        // newer SSE agent-graph update. Only apply the result if no later refresh started.
        const seq = get()._agentRunsLoadSeq + 1;
        set({ _agentRunsLoadSeq: seq });
        try {
            const agentRuns = await client.listAgentRuns(parentRunId);
            if (get()._agentRunsLoadSeq === seq && (get().mainRunId === parentRunId || get().runId === parentRunId)) {
                // BUG-169: merge (not replace) with whatever is already in state, same as the SSE
                // agent_graph_updated path (mergeAgentRunsById, BUG-132). This is a fire-and-forget
                // HTTP snapshot racing a live SSE channel — AgentsPanel fires a refresh the instant
                // mainRunId is set (before anything has spawned), and if that request is slow enough
                // to resolve AFTER a later SSE update already merged a freshly-spawned child in,
                // replacing wholesale here would wipe that child back out even though it is really
                // running. Merging makes the two sources converge instead of racing.
                set((s) => ({ agentRuns: mergeAgentRunsById(s.agentRuns, agentRuns) }));
            }
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] listAgentRuns failed:", err);
        }
    },
    async refreshWorkflowStepRuntime() {
        const { client, mainRunId, runId, chatMode } = get();
        const targetRunId = mainRunId ?? runId;
        // Normal chat has no workflow-step list to show; skip the request entirely (8.2).
        if (chatMode !== "workflow_step_auto" || !targetRunId || !client.getWorkflowStepsRuntime) {
            set({ workflowStepRuntime: [], workflowStepRuntimeMeta: {} });
            return;
        }
        const seq = get()._workflowStepRuntimeLoadSeq + 1;
        set({ _workflowStepRuntimeLoadSeq: seq, workflowStepRuntimeLoading: true });
        try {
            const snapshot = await client.getWorkflowStepsRuntime(targetRunId);
            if (get()._workflowStepRuntimeLoadSeq === seq && (get().mainRunId === targetRunId || get().runId === targetRunId)) {
                set({
                    workflowStepRuntime: snapshot.steps,
                    workflowStepRuntimeMeta: { provider: snapshot.provider, model: snapshot.model, yoloMode: snapshot.yoloMode },
                    workflowStepRuntimeLoading: false,
                });
            }
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] getWorkflowStepsRuntime failed:", err);
            if (get()._workflowStepRuntimeLoadSeq === seq) {
                set({ workflowStepRuntimeLoading: false });
            }
        }
    },
    async refreshAgentGraph() {
        const { client, mainRunId, runId } = get();
        const parentRunId = mainRunId ?? runId;
        if (!parentRunId || !client.refreshAgentGraph)
            return;
        const loadSeq = get()._agentGraphLoadSeq + 1;
        set({ _agentGraphLoadSeq: loadSeq });
        const snap = await client.refreshAgentGraph(parentRunId);
        if (get()._agentGraphLoadSeq !== loadSeq)
            return;
        if (!(0, timelineReducer_1.shouldApplyRunEvent)(get().mainRunId ?? get().runId, parentRunId))
            return;
        set(applyAgentGraphSnapshot(snap));
    },
    async pauseAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.pauseAgentLoop)
        set({ ...applyAgentGraphSnapshot(await client.pauseAgentLoop(parentRunId)), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); },
    async resumeAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.resumeAgentLoop)
        set({ ...applyAgentGraphSnapshot(await client.resumeAgentLoop(parentRunId)), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); },
    async injectAgentFeedback(toRunId, message) { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.injectAgentFeedback)
        set({ ...applyAgentGraphSnapshot(await client.injectAgentFeedback(parentRunId, toRunId, message)), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); },
    async stopAgentLoop() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.stopAgentLoop) {
        set({ ...applyAgentGraphSnapshot(await client.stopAgentLoop(parentRunId)), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); /* Bug 3 fix: also interrupt to forcefully terminate the in-flight provider turn */
        if (client.interrupt) {
            try {
                await client.interrupt(parentRunId);
            }
            catch { /* best-effort: interrupt may 404 if no turn is in flight */ }
        }
    } },
    async submitReviewOutcome(outcome, issues) { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.submitReviewOutcome)
        set({ ...applyAgentGraphSnapshot(await client.submitReviewOutcome(parentRunId, { outcome, issues })), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); },
    async extendCap() { const { client, mainRunId, runId } = get(); const parentRunId = mainRunId ?? runId; if (parentRunId && client.extendCap)
        set({ ...applyAgentGraphSnapshot(await client.extendCap(parentRunId)), _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1 }); },
    async continueFlow(feedback, memberAction) {
        const { client, mainRunId, runId, activeAgentRunId } = get();
        const parentRunId = mainRunId ?? runId;
        if (!parentRunId || !client.continueFlow)
            return;
        // BUG-231 follow-up: resuming while a child agent is focused must not let the
        // hub's resumed turn stream into the focused child's transcript — return to
        // the main run first so the response lands where the user is looking.
        if (activeAgentRunId && activeAgentRunId !== parentRunId) {
            get().backToMainRun();
        }
        // Bump graph load seq so a late blocked HTTP refresh cannot restore the card.
        set({
            ...applyAgentGraphSnapshot(await client.continueFlow(parentRunId, feedback, memberAction)),
            _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1,
        });
    },
    async amendFlow(paths) {
        const { client, mainRunId, runId, activeAgentRunId } = get();
        const parentRunId = mainRunId ?? runId;
        if (!parentRunId || !client.amendFlow)
            return;
        if (activeAgentRunId && activeAgentRunId !== parentRunId) {
            get().backToMainRun();
        }
        set({
            ...applyAgentGraphSnapshot(await client.amendFlow(parentRunId, paths)),
            _agentGraphLoadSeq: get()._agentGraphLoadSeq + 1,
        });
    },
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
        void get().refreshWorkflowStepRuntime();
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
            ...restoreRunSnapshot(restore),
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
        localStorage.setItem(LAST_PROJECT_KEY, projectId);
        const projectChanged = get().selectedProjectId !== projectId;
        set({
            selectedProjectId: projectId,
            selectedWorkflowId: undefined,
            selectedStepId: undefined,
            runHistory: [],
            remoteChatSessions: [],
            historyLoadError: undefined,
            remoteHistoryLoadError: undefined,
        });
        if (projectChanged) {
            get().resetRun();
        }
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
        const state = get();
        if (!provider) {
            set({
                selectedProvider: undefined,
                selectedModel: undefined,
                pendingProviderSwitch: undefined,
                pendingAccountSwitch: undefined,
                accountSwitchLoading: false,
            });
            return;
        }
        if (state.selectedProvider === provider) {
            return;
        }
        const targetModel = pickDefaultModel(provider, state.supportedModels);
        const canRequestHandoff = state.chatMode === "normal_chat" &&
            Boolean(state.runId) &&
            !isInteractiveChatBlocked(state.status);
        if (canRequestHandoff && state.selectedProvider) {
            const sourceRunId = state.runId;
            if (!sourceRunId)
                return;
            set({
                pendingProviderSwitch: {
                    sourceRunId,
                    sourceProviderKey: state.selectedProvider,
                    sourceRunStatus: state.status,
                    targetProviderKey: provider,
                    targetModel,
                },
                pendingAccountSwitch: undefined,
                accountSwitchLoading: false,
            });
            return;
        }
        set({ selectedProvider: provider, selectedModel: targetModel, pendingProviderSwitch: undefined });
        void get().loadSkills(provider);
    },
    cancelProviderSwitch() {
        set({ pendingProviderSwitch: undefined, providerSwitchLoading: false });
    },
    async confirmProviderSwitch() {
        const state = get();
        const pending = state.pendingProviderSwitch;
        if (!pending || state.providerSwitchLoading)
            return;
        if (state.chatMode !== "normal_chat" || !state.selectedProjectId) {
            set({ pendingProviderSwitch: undefined, providerSwitchLoading: false });
            return;
        }
        const targetProviderKey = pending.targetProviderKey;
        const targetModel = pending.targetModel ?? pickDefaultModel(targetProviderKey, state.supportedModels);
        const cwd = selectedProjectPath(state);
        set({ providerSwitchLoading: true });
        try {
            // CP-59 Task-316: chat-scoped switch when the chat SSOT knows the chat
            // (runner mints the leg and seeds the envelope server-side). The
            // timeline is KEPT — one divider, transcript continuous (no `timeline: []`).
            const chatId = get().chatId;
            if (chatId) {
                const resp = await state.client.switchChatProvider(chatId, {
                    targetProviderKey,
                    model: targetModel,
                    reasoningEffort: state.reasoningEffort,
                    yoloMode: state.yoloMode,
                });
                const carried = resp.handoff.truncated && resp.handoff.omittedTurnCount > 0
                    ? `${resp.handoff.includedTurnCount} of ${resp.handoff.includedTurnCount + resp.handoff.omittedTurnCount} turns`
                    : `${resp.handoff.includedTurnCount} turns`;
                set({
                    selectedProvider: targetProviderKey,
                    selectedModel: targetModel,
                    runId: resp.handle.runId,
                    chatId: resp.handle.chatId ?? chatId,
                    mainRunId: resp.handle.runId,
                    activeAgentRunId: undefined,
                    activeStepId: resp.handle.stepId,
                    status: resp.handle.status,
                    pendingApprovals: [],
                    pendingQuestions: [],
                    gateBlock: undefined,
                    latestTokenUsage: undefined,
                    lastTurnInput: undefined,
                    recoverable: false,
                    pendingAccountSwitch: undefined,
                    accountSwitchLoading: false,
                    pendingProviderSwitch: undefined,
                    providerSwitchLoading: false,
                    _accountSwitchTriedIds: [],
                    _streamingAssistantId: undefined,
                    agentRuns: [],
                    agentGraphSnapshot: undefined,
                    agentBusMessages: [],
                    workflowStepRuntime: [],
                    workflowStepRuntimeMeta: {},
                    agentSpawnGuideOpen: false,
                    agentSpawnGuideAgentName: undefined,
                    _runReplaySeq: {},
                    _runSnapshots: {},
                    _historyReplaying: false,
                    _streamRunSeq: state._streamRunSeq + 1,
                    timeline: [
                        ...state.timeline,
                        {
                            kind: "system",
                            id: `seed-divider-${resp.handle.runId}`,
                            text: `⇄ switched to ${providerLabel(targetProviderKey)} · ${targetModel} — carried ${carried} (${resp.handoff.handoffMode})`,
                            tone: "info",
                        },
                    ],
                });
                void get().loadSkills(targetProviderKey);
                void get().loadRunHistory();
                return;
            }
            // Legacy Task-078 path (pre-CP-59 runner / flag off): summary + new chat.
            const handoff = await state.client.handoffContext(pending.sourceRunId, { targetProviderKey });
            const handle = await state.client.startRun({
                projectId: state.selectedProjectId,
                providerKey: targetProviderKey,
                model: targetModel,
                reasoningEffort: state.reasoningEffort,
                yoloMode: state.yoloMode,
                chatMode: "normal_chat",
                cwd,
            });
            set({
                selectedProvider: targetProviderKey,
                selectedModel: targetModel,
                runId: handle.runId,
                chatId: handle.chatId,
                mainRunId: handle.runId,
                activeAgentRunId: undefined,
                activeStepId: handle.stepId,
                status: handle.status,
                timeline: [],
                artifacts: [],
                pendingApprovals: [],
                pendingQuestions: [],
                gateBlock: undefined,
                latestTokenUsage: undefined,
                lastTurnInput: undefined,
                recoverable: false,
                pendingAccountSwitch: undefined,
                accountSwitchLoading: false,
                pendingProviderSwitch: undefined,
                providerSwitchLoading: false,
                _accountSwitchTriedIds: [],
                _streamingAssistantId: undefined,
                agentRuns: [],
                agentGraphSnapshot: undefined,
                agentBusMessages: [],
                workflowStepRuntime: [],
                workflowStepRuntimeMeta: {},
                agentSpawnGuideOpen: false,
                agentSpawnGuideAgentName: undefined,
                _runReplaySeq: {},
                _runSnapshots: {},
                _historyReplaying: false,
                _streamRunSeq: state._streamRunSeq + 1,
            });
            set((s) => ({
                timeline: [
                    ...s.timeline,
                    {
                        kind: "system",
                        id: `handoff-mode-${s.timeline.length}`,
                        text: `Handoff from ${providerLabel(handoff.sourceProviderKey)} used ${handoff.handoffMode} context.`,
                        tone: "info",
                    },
                ],
            }));
            void get().loadSkills(targetProviderKey);
            await get().sendPrompt(handoff.prompt);
            void get().loadRunHistory();
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] provider handoff failed:", err);
            set((s) => ({
                providerSwitchLoading: false,
                timeline: [
                    ...s.timeline,
                    {
                        kind: "system",
                        id: `handoff-err-${s.timeline.length}`,
                        text: `Failed to start a new chat with ${providerLabel(targetProviderKey)}: ${String(err)}`,
                        tone: "error",
                    },
                ],
            }));
        }
    },
    setSelectedModel(model) {
        // CA-686 parity with the TUI: reasoning is dynamic per model — a stale
        // effort the new model does not advertise resets to that model's catalog
        // default (or empty = model default). No catalog data → keep as-is.
        const state = get();
        const next = state.supportedModels.find((m) => m.modelId === model);
        const efforts = next?.supportedReasoningEfforts ?? [];
        const current = (state.reasoningEffort ?? "").trim();
        let reasoningEffort = state.reasoningEffort;
        if (efforts.length > 0 && current && !efforts.some((e) => e.toLowerCase() === current.toLowerCase())) {
            const def = (next?.defaultReasoningEffort ?? "").trim();
            reasoningEffort = def && efforts.some((e) => e.toLowerCase() === def.toLowerCase()) ? def : "";
        }
        set({ selectedModel: model, reasoningEffort });
        // CA-689c parity with the TUI: opencode model variants live only in the
        // ACP session config — fetch the real list on selection (runner-side
        // cache short-circuits repeat picks) and patch the catalog entry so the
        // Reasoning dropdown re-derives from truth. Failures are silent: the turn
        // still works with the guessed list.
        const client = get().client;
        const providerKey = get().selectedProvider;
        if (providerKey === "opencode" && model && client.getOpencodeModelVariants) {
            void client
                .getOpencodeModelVariants(model)
                .then((variants) => {
                const st = get();
                const patched = st.supportedModels.map((m) => m.modelId === model
                    ? {
                        ...m,
                        supportedReasoningEfforts: variants.supportedEfforts,
                        defaultReasoningEffort: variants.defaultReasoningEffort || null,
                    }
                    : m);
                set({ supportedModels: patched });
                // Re-clamp if the user is still on this model.
                if (get().selectedModel === model && get().selectedProvider === "opencode") {
                    const live = variants.supportedEfforts;
                    const cur = (get().reasoningEffort ?? "").trim();
                    let updated = get().reasoningEffort;
                    if (live.length > 0 && cur && !live.some((e) => e.toLowerCase() === cur.toLowerCase())) {
                        const def = (variants.defaultReasoningEffort ?? "").trim();
                        updated = def && live.some((e) => e.toLowerCase() === def.toLowerCase()) ? def : "";
                    }
                    if (updated !== get().reasoningEffort) {
                        set({ reasoningEffort: updated });
                    }
                }
            })
                .catch((err) => {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] opencode variants fetch failed:", err);
            });
        }
    },
    setReasoningEffort(effort) {
        set({ reasoningEffort: effort });
    },
    setYoloMode(yolo) {
        set({ yoloMode: yolo });
    },
    async toggleYoloForActiveProvider(next) {
        const { client, selectedProvider } = get();
        if (selectedProvider !== "grok") {
            set({ yoloMode: next });
            return;
        }
        set({ grokYoloPostureLoading: true });
        try {
            await client.applyGrokYoloPosture(next);
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] applyGrokYoloPosture failed:", err);
            set((s) => ({
                grokYoloPostureLoading: false,
                timeline: [
                    ...s.timeline,
                    {
                        kind: "system",
                        id: `grok-yolo-err-${s.timeline.length}`,
                        text: `Failed to ${next ? "enable" : "disable"} YOLO for Grok: ${String(err)}`,
                        tone: "error",
                    },
                ],
            }));
            return;
        }
        set({ yoloMode: next, grokYoloPostureLoading: false });
    },
    async generateChatSummary() {
        const state = get();
        const runId = state.runId;
        if (!runId || state.summaryGenerating)
            return;
        if (isInteractiveChatBlocked(state.status))
            return;
        set({ summaryGenerating: true });
        try {
            const result = await state.client.generateChatSummary(runId);
            set((s) => ({
                summaryGenerating: false,
                timeline: [
                    ...s.timeline,
                    {
                        kind: "system",
                        id: `chat-summary-${s.timeline.length}`,
                        text: result.generated
                            ? "Chat summary updated."
                            : result.reason
                                ? `Chat summary skipped (${result.reason}).`
                                : "Chat summary unchanged.",
                        tone: "info",
                    },
                ],
            }));
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] generateChatSummary failed:", err);
            set((s) => ({
                summaryGenerating: false,
                timeline: [
                    ...s.timeline,
                    {
                        kind: "system",
                        id: `chat-summary-err-${s.timeline.length}`,
                        text: `Failed to generate chat summary: ${String(err)}`,
                        tone: "error",
                    },
                ],
            }));
        }
    },
    setChatStartMode(mode) {
        set((state) => ({
            chatStartMode: mode,
            chatSourceDocId: mode === "normal" ? "" : state.chatSourceDocId,
            // A built-in orchestration selection only makes sense for the sub-mode
            // it was offered under (CP-42 Task-177 T-7: switching away from Bug
            // clears the Review Loop selection); clear both the pick and the
            // stale option list on every intent change, then reload below if the
            // new mode has options to offer.
            flowRef: undefined,
            builtinOrchestrationOptions: [],
        }));
        if (mode === "bugfix") {
            void get().loadBuiltinOrchestrationOptions("bug");
        }
    },
    setChatSourceDocId(sourceDocId) {
        set({ chatSourceDocId: sourceDocId });
    },
    async setChatPosture(posture) {
        const { client, chatPostureConfig } = get();
        let config = chatPostureConfig;
        if (client.getChatPosture) {
            try {
                config = await client.getChatPosture();
            }
            catch (err) {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] loadChatPostureConfig failed on posture switch:", err);
            }
        }
        const profile = config.profiles[posture] ?? {};
        // CA-685 (BUG-330 parity with the TUI): a pinned model without an explicit
        // provider pin infers its provider from the model id so a grok-4.5 pin
        // never stamps an opencode session.
        const pinnedProvider = profile.provider ?? providerKeyForPinnedModel(profile.model) ?? undefined;
        set((state) => ({
            chatPosture: posture,
            chatPostureConfig: { ...config, active: posture },
            // Apply the posture's pinned profile fields when set; empty inherits the
            // current session selection (mirrors the TUI's /mode apply). The "non"
            // posture has no profile — nothing is applied.
            ...(pinnedProvider ? { selectedProvider: pinnedProvider } : {}),
            ...(profile.model ? { selectedModel: profile.model } : {}),
            ...(profile.reasoningEffort ? { reasoningEffort: profile.reasoningEffort } : {}),
            ...(typeof profile.yolo === "boolean" ? { yoloMode: profile.yolo } : {}),
        }));
        if (client.setChatPosture) {
            try {
                await client.setChatPosture({ ...config, active: posture });
            }
            catch (err) {
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] setChatPosture failed:", err);
            }
        }
        const provider = get().selectedProvider;
        if (get().chatMode === "normal_chat" && provider) {
            void get().loadSkills(provider, selectedProjectPath(get()));
        }
        // Grok YOLO sync: if the active profile pins YOLO, the Grok process
        // config.toml must be rewritten (Task-218). Desktop previously skipped
        // this — TUI did it via /mode but the same posture switch on Desktop left
        // Grok's runtime YOLO stale. Call the existing sync path (same as the
        // YOLO toggle button) so both surfaces stay consistent.
        if (get().selectedProvider === "grok" && typeof profile.yolo === "boolean") {
            void get().toggleYoloForActiveProvider(profile.yolo);
        }
    },
    async loadChatPostureConfig() {
        const { client } = get();
        if (!client.getChatPosture)
            return;
        try {
            const config = await client.getChatPosture();
            const active = config.active ?? get().chatPosture;
            const profile = config.profiles[active] ?? {};
            set(() => ({
                chatPostureConfig: config,
                chatPosture: active,
                ...(profile.provider ? { selectedProvider: profile.provider } : {}),
                ...(profile.model ? { selectedModel: profile.model } : {}),
                ...(profile.reasoningEffort ? { reasoningEffort: profile.reasoningEffort } : {}),
                ...(typeof profile.yolo === "boolean" ? { yoloMode: profile.yolo } : {}),
            }));
            if (get().selectedProvider === "grok" && typeof profile.yolo === "boolean") {
                void get().toggleYoloForActiveProvider(profile.yolo);
            }
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] loadChatPostureConfig failed:", err);
        }
    },
    async saveChatPostureConfig(config) {
        const client = get().client;
        if (!client.setChatPosture)
            return;
        try {
            const saved = await client.setChatPosture(config);
            set({ chatPostureConfig: saved });
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] saveChatPostureConfig failed:", err);
            throw err;
        }
    },
    openChatPostureSetup() {
        set({ chatPostureSetupOpen: true });
    },
    closeChatPostureSetup() {
        set({ chatPostureSetupOpen: false });
    },
    setFlowRef(flowRef) {
        set({ flowRef });
    },
    async loadBuiltinOrchestrationOptions(subMode) {
        const { client } = get();
        if (!client.listBuiltinOrchestrationOptions) {
            set({ builtinOrchestrationOptions: [] });
            return;
        }
        try {
            const options = await client.listBuiltinOrchestrationOptions(subMode);
            set({ builtinOrchestrationOptions: options });
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] loadBuiltinOrchestrationOptions failed:", err);
            set({ builtinOrchestrationOptions: [] });
        }
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
        const { client, chatMode, launchMode, selectedProjectId, selectedWorkflowId, selectedStepId, selectedProvider, selectedModel, reasoningEffort, yoloMode, chatStartMode, chatSourceDocId, flowRef, } = get();
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
        const isFirstChatTurn = chatMode === "normal_chat" && !runId;
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
                changeType: isFirstChatTurn && chatStartMode !== "normal" ? chatStartMode : undefined,
                sourceDocId: isFirstChatTurn && chatStartMode !== "normal" && chatSourceDocId.trim().length > 0
                    ? chatSourceDocId.trim()
                    : undefined,
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
                // Resend the current posture every chat turn like model/YOLO (BUG-063
                // pattern, Task-xxx/CA-xxx); the runner's read-only policy for scan/plan
                // is keyed off this per turn. Workflow/step mode omits it (never read-only).
                chatPosture: chatMode === "normal_chat" ? get().chatPosture : undefined,
                attachments: chatMode === "normal_chat" && attachments && attachments.length > 0
                    ? attachments
                    : undefined,
                // Built-in orchestration selection (CP-42/Task-177): only meaningful
                // alongside a first-turn Bug intent, and only when the user actually
                // picked a flow. chatStartMode's runner-facing sub-mode key is "bug",
                // distinct from the UI's "bugfix" tab value.
                subMode: isFirstChatTurn && chatStartMode === "bugfix" && flowRef ? "bug" : undefined,
                flowRef: isFirstChatTurn && chatStartMode === "bugfix" ? flowRef : undefined,
            };
            set({ runId, lastTurnInput: turnInput, activeStepId: turnStepId, _streamRunSeq: get()._streamRunSeq + 1 });
            cancelHistoryReplayStream();
            cancelOrchestrationStream();
            cancelAgentFocusStream();
            // A follow-up sent the instant a flow *looks* done can race the hub's own
            // final turn: the loop is marked "done" (which unblocks the composer via
            // deriveOrchestrationRunStatus) from INSIDE that turn, while the turn's
            // provider stream is still open — so the runner still holds turnInFlight and
            // rejects POST /turns with 409 turn_in_progress. gate_in_progress (post-turn
            // gate settling) and hub_parked (children still active) are the sibling
            // transient windows. All three are rejected BEFORE a turn is minted, so
            // re-POSTing is side-effect-free and can never duplicate a turn. Retry briefly
            // until the turn clears instead of dropping the user's message with a raw
            // error and forcing a re-type (the pre-fix symptom on flow completion).
            const TRANSIENT_SEND_CODES = new Set(["turn_in_progress", "gate_in_progress", "hub_parked"]);
            const TRANSIENT_SEND_MAX_RETRIES = 6;
            const TRANSIENT_SEND_RETRY_MS = 700;
            for (let attempt = 0;; attempt++) {
                try {
                    await consumeStream(runId, client.sendTurn(turnInput), set, get);
                    break;
                }
                catch (err) {
                    const transient = err instanceof HttpWsRunnerClient_1.RunnerApiError && TRANSIENT_SEND_CODES.has(err.code ?? "");
                    // Give up (fall to the catch below) if it is a real error, we have waited
                    // long enough, or the user switched runs out from under this send.
                    if (!transient || attempt >= TRANSIENT_SEND_MAX_RETRIES || !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId)) {
                        throw err;
                    }
                    // Keep the optimistic prompt + thinking bubbles; just reflect the wait.
                    set((s) => ({
                        status: "running",
                        timeline: s.timeline.map((it) => it.kind === "thinking" ? { ...it, text: "Waiting for the current step to finish…" } : it),
                    }));
                    await new Promise((r) => setTimeout(r, TRANSIENT_SEND_RETRY_MS));
                }
            }
            const orchestrationRunId = get().mainRunId ?? runId;
            if (orchestrationRunId) {
                startOrchestrationStream(orchestrationRunId, client, set, get);
            }
            void get().refreshAgentRuns();
            void get().refreshWorkflowStepRuntime();
        }
        catch (err) {
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] sendPrompt failed:", err);
            if (runId && !(0, timelineReducer_1.shouldApplyRunEvent)(get().runId, runId)) {
                return;
            }
            // run-63960: flow_awaiting_user means the engine is parked for Continue/Stop —
            // never map that to Failed (which hides FlowAwaitingUserCard and removes Stop).
            // Restrict message fallback to HTTP 409 so non-conflict errors cannot fake blocked.
            const awaitingUser = err instanceof HttpWsRunnerClient_1.RunnerApiError &&
                (err.code === "flow_awaiting_user" ||
                    (err.status === 409 && /flow_awaiting_user|waiting for your decision/i.test(err.message)));
            if (awaitingUser) {
                const parentId = get().mainRunId ?? get().runId;
                set((s) => ({
                    status: "blocked",
                    recoverable: false,
                    timeline: [
                        ...s.timeline.filter((it) => it.kind !== "thinking"),
                        {
                            kind: "system",
                            id: `await-user-${s.timeline.length}`,
                            text: runErrorMessage(err),
                            tone: "warn",
                        },
                    ],
                }));
                if (parentId) {
                    requestAgentGraphRefresh(parentId, get, set, {
                        settleBlockedTimeline: true,
                        preferBlockedStatus: true,
                    });
                }
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
    async approve(approvalId, decision, remember) {
        // Resolve the specific card the user clicked, not "whatever is pending" — a turn
        // can fan out several parallel tool calls awaiting approval at once, so more than
        // one entry may be in pendingApprovals simultaneously (BUG-157).
        const pending = get().pendingApprovals.find((p) => p.approvalId === approvalId);
        if (!pending)
            return;
        set((s) => ({
            pendingApprovals: s.pendingApprovals.filter((p) => p.approvalId !== approvalId),
            status: s.pendingApprovals.length > 1 ? "waiting_approval" : "running",
            timeline: s.timeline.map((it) => it.kind === "approval" && it.approvalId === approvalId ? { ...it, decision } : it),
        }));
        try {
            await get().client.submitApproval(approvalId, decision, remember);
        }
        catch (err) {
            // BUG-172: mirror sendPrompt's error handling — an unhandled rejection here
            // (e.g. a transient network blip while YOLO fires off rapid step
            // transitions) used to leave the run silently stuck in whatever status the
            // optimistic update above set, with nothing telling the user it never
            // reached the server.
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] approve failed:", err);
            set((s) => ({
                status: "failed",
                recoverable: true,
                timeline: [
                    ...s.timeline,
                    { kind: "system", id: `err-approve-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
                ],
            }));
        }
    },
    async answer(questionId, choice) {
        // Same rationale as approve() (BUG-157) — resolve the specific card the user
        // acted on, not "whatever is pending", since more than one question can be
        // outstanding at once.
        const pending = get().pendingQuestions.find((q) => q.questionId === questionId);
        if (!pending)
            return;
        set((s) => ({
            pendingQuestions: s.pendingQuestions.filter((q) => q.questionId !== questionId),
            status: s.pendingQuestions.length > 1 ? "waiting_question" : "running",
            timeline: s.timeline.map((it) => it.kind === "question" && it.questionId === questionId ? { ...it, answer: choice } : it),
        }));
        try {
            await get().client.answerQuestion(questionId, choice);
        }
        catch (err) {
            // BUG-172: see approve() above — same failure mode for the question path.
            // eslint-disable-next-line no-console
            console.error("[FlowPilot] answer failed:", err);
            set((s) => ({
                status: "failed",
                recoverable: true,
                timeline: [
                    ...s.timeline,
                    { kind: "system", id: `err-answer-${s.timeline.length}`, text: runErrorMessage(err), tone: "error" },
                ],
            }));
        }
    },
    async stop() {
        const { client, runId, mainRunId, activeAgentRunId, agentRuns, agentGraphSnapshot } = get();
        if (!runId)
            return;
        const parentRunId = mainRunId ?? runId;
        const childFocused = Boolean(activeAgentRunId && parentRunId && activeAgentRunId !== parentRunId);
        // Always dismiss regression/gate modal on Stop so main hang does not leave an
        // orphaned overlay after the loop is cancelled (CP-51 A1).
        set({ gateBlock: undefined });
        // BUG-247: stop() must always cascade to the parent run and every running child in a
        // single press, whether it was triggered from the main chat or a focused child's
        // read-only view — Task-088's original child-only routing left the parent (and its
        // loop) running until the user switched back and pressed Stop a second time.
        //
        // CP-51 A1: also stop the parent loop when we have any orchestration snapshot or
        // agent children even if hasActiveParentAgentLoop is false (stale snapshot /
        // parentRunId mismatch) — otherwise main Stop only interrupts the hub (no turn)
        // while the child keeps running and UI looks stuck.
        const shouldStopLoop = Boolean(parentRunId && client.stopAgentLoop) &&
            (hasActiveParentAgentLoop(get(), parentRunId) ||
                get().chatMode === "workflow_step_auto" ||
                // run-63960: freeform 409 maps to blocked before graph refresh lands —
                // Stop must still seal the parked loop (Continue/Stop surface).
                get().status === "blocked" ||
                Boolean(agentGraphSnapshot?.loopState?.status) ||
                agentRuns.some((r) => r.parentRunId === parentRunId || r.runId === parentRunId));
        if (shouldStopLoop && parentRunId && client.stopAgentLoop) {
            try {
                const snapshot = await client.stopAgentLoop(parentRunId);
                set((s) => {
                    // Force cancelled at the Stop press (BUG-248). Do not use
                    // deriveOrchestrationRunStatus alone: after BUG-308 it preserves
                    // running/completed for post-Stop chat, which would leave the header
                    // on Running when the user just hit Stop while a turn was active.
                    // Bump _agentGraphLoadSeq so a late blocked HTTP graph refresh
                    // (run-63960 open/history seed) cannot restore Continue/Stop over Stop.
                    return {
                        ...applyAgentGraphSnapshot(snapshot),
                        status: "cancelled",
                        timeline: s.timeline.filter((it) => it.kind !== "thinking"),
                        gateBlock: undefined,
                        _runSnapshots: reconcileStoppedRunSnapshots(s._runSnapshots, parentRunId, snapshot),
                        _agentGraphLoadSeq: s._agentGraphLoadSeq + 1,
                    };
                });
            }
            catch (err) {
                // Durable fence/persist may 5xx after RAM cancel (V10R4). Prefer any
                // snapshot embedded in the error body; otherwise still interrupt hard.
                // eslint-disable-next-line no-console
                console.error("[FlowPilot] stopAgentLoop failed (still interrupting):", err);
                const embedded = extractStopSnapshot(err);
                if (embedded) {
                    set((s) => ({
                        ...applyAgentGraphSnapshot(embedded),
                        status: "cancelled",
                        timeline: s.timeline.filter((it) => it.kind !== "thinking"),
                        gateBlock: undefined,
                        _runSnapshots: reconcileStoppedRunSnapshots(s._runSnapshots, parentRunId, embedded),
                        _agentGraphLoadSeq: s._agentGraphLoadSeq + 1,
                    }));
                }
                else {
                    set((s) => ({
                        status: "cancelled",
                        gateBlock: undefined,
                        timeline: s.timeline.filter((it) => it.kind !== "thinking"),
                        agentGraphSnapshot: s.agentGraphSnapshot
                            ? {
                                ...s.agentGraphSnapshot,
                                loopState: { ...s.agentGraphSnapshot.loopState, status: "stopped", gateReason: "stopped" },
                            }
                            : s.agentGraphSnapshot,
                        _runSnapshots: reconcileStoppedRunSnapshots(s._runSnapshots, parentRunId),
                        _agentGraphLoadSeq: s._agentGraphLoadSeq + 1,
                    }));
                }
            }
            try {
                await client.interrupt(parentRunId);
            }
            catch {
                /* best-effort */
            }
            const childIds = new Set();
            if (childFocused && runId !== parentRunId)
                childIds.add(runId);
            for (const r of get().agentRuns) {
                if (r.runId && r.runId !== parentRunId)
                    childIds.add(r.runId);
            }
            for (const childId of childIds) {
                try {
                    await client.interrupt(childId);
                }
                catch {
                    /* best-effort: loop stop may already have cancelled the child */
                }
            }
            void get().refreshAgentRuns();
            void get().refreshWorkflowStepRuntime();
            return;
        }
        await client.interrupt(runId);
        if (childFocused && parentRunId !== runId) {
            try {
                await client.interrupt(parentRunId);
            }
            catch {
                /* best-effort: parent may have no in-flight turn to cancel */
            }
        }
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
    async syncRuns(runIds, projectId) {
        // Shared batch-sync loop for both the project-level "Sync all" chip
        // (syncAllInProject) and the selection-mode "Sync" confirm action, so both
        // surfaces drive the same x/y progress counter instead of two parallel ones.
        // One at a time so we do not hammer Drive; per-item failures are swallowed
        // (syncHistoryRun marks the row failed) so one broken session does not abort
        // the whole batch.
        set({ syncBatchProgress: { projectId, done: 0, total: runIds.length } });
        try {
            for (const runId of runIds) {
                try {
                    await get().syncHistoryRun(runId, projectId);
                }
                catch {
                    // already reflected as syncStatus: "failed" on the row
                }
                set((s) => s.syncBatchProgress && s.syncBatchProgress.projectId === projectId
                    ? { syncBatchProgress: { ...s.syncBatchProgress, done: s.syncBatchProgress.done + 1 } }
                    : {});
            }
        }
        finally {
            set((s) => (s.syncBatchProgress?.projectId === projectId ? { syncBatchProgress: undefined } : {}));
        }
    },
    async syncAllInProject(projectId) {
        // Sync every not-yet-synced chat run in the project.
        const { remoteChatSessions } = get();
        const targets = get()
            .runHistory.filter((item) => item.projectId === projectId && (0, navigatorHistory_1.isSyncableRun)(item, remoteChatSessions))
            .map((item) => item.runId);
        await get().syncRuns(targets, projectId);
    },
    async deleteHistoryRun(runId) {
        const { client } = get();
        const wasActive = get().runId === runId;
        // Optimistically remove from local history so the UI responds immediately.
        set((s) => ({ runHistory: s.runHistory.filter((item) => item.runId !== runId) }));
        // If the deleted run was the active session, reset the whole workspace back to
        // an empty new chat — reuse resetRun() (not a hand-rolled subset) so Flow Timeline
        // and Agents panel state (mainRunId, agentRuns, workflowStepRuntime, etc.) and the
        // orchestration/agent-focus streams are cleared the same way a fresh chat start
        // clears them (BUG-258).
        if (wasActive) {
            get().resetRun();
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
    async restoreRemoteChatSession(summary, cwd, options) {
        const { client, selectedProjectId } = get();
        if (!selectedProjectId)
            return;
        const refresh = options?.refresh ?? true;
        const open = options?.open ?? true;
        const request = {
            projectId: selectedProjectId,
            sourceMachineId: summary.sourceMachineId,
            sourceRunId: summary.sourceRunId,
            cwd,
        };
        try {
            const result = await client.restoreChatRun(request);
            // Batch restores (restoreAll) opt out of the per-item refresh/open: refreshing
            // the full history + remote list after every single item in a multi-item
            // restore serializes N extra round trips into the loop (each item waits for
            // the previous one's full refresh before starting), which is what made a bulk
            // restore's per-item spinner look "stuck" until the whole batch finished; and
            // opening every restored run in turn would hijack the active chat panel N
            // times over. The caller does one combined refresh after the whole batch.
            if (refresh) {
                await Promise.all([get().loadRunHistory(), get().loadRemoteChatSessions()]);
            }
            if (open) {
                void get().openHistoryRun(result.runId);
            }
        }
        catch (err) {
            if (err instanceof HttpWsRunnerClient_1.RunnerApiError && err.code === "cwd_remap_required" && !cwd) {
                const retryCwd = selectedProjectPath(get());
                if (retryCwd) {
                    await get().restoreRemoteChatSession(summary, retryCwd, options);
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
                    // BUG-267: unavailableReason alone only surfaces in the disabled row/tooltip;
                    // provider-account mismatches need an immediate modal at click time.
                    ...((err.code === "account_not_signed_in" || err.code === "account_unavailable")
                        ? { historyOpenError: { code: err.code, message: err.message, providerKey: summary.providerKey } }
                        : {}),
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
                    // BUG-267: unavailableReason alone only surfaces in the disabled row/tooltip;
                    // provider-account mismatches need an immediate modal at click time.
                    ...((err.code === "account_not_signed_in" || err.code === "account_unavailable")
                        ? { historyOpenError: { code: err.code, message: err.message, providerKey: historyProvider } }
                        : {}),
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
        // BUG-170: restore the mode this run actually was, not whatever the UI happened to be
        // in before the user clicked a history item. Without this, reopening a workflow/flow-
        // mode run left chatMode stuck (often "normal_chat"), so the reopened run rendered
        // without its Flow Mode surfaces (step-timeline sidebar, agents panel gating) even
        // though the runner resumed it correctly. runKind is "chat" for normal_chat runs and
        // "workflow" (or, for older persisted rows, undefined) for everything else.
        const isWorkflowHistoryItem = historyItem?.runKind !== "chat";
        // BUG-263: same "restore the mode this run actually was" gap as BUG-170
        // above, but for the Chat-Mode orchestration picker (Bug tab / Built-in
        // orchestration select) instead of chatMode/launchMode. Without this,
        // reopening a run started via the picker left chatStartMode stuck at its
        // default "normal", so the Chat Intent panel showed "Normal" selected
        // (and locked) even though the run itself was correctly resumed as a
        // flow-engine-driven Review Loop run underneath.
        const chatStartMode = historyItem?.subMode === "bug" ? "bugfix" : "normal";
        set({
            runId: handle.runId,
            chatId: handle.chatId,
            mainRunId: handle.runId,
            activeAgentRunId: undefined,
            status: handle.status,
            activeStepId: handle.stepId,
            chatMode: isWorkflowHistoryItem ? "workflow_step_auto" : "normal_chat",
            ...(isWorkflowHistoryItem && historyItem?.workflowId
                ? { launchMode: "workflow", selectedWorkflowId: historyItem.workflowId }
                : {}),
            chatStartMode,
            flowRef: chatStartMode === "bugfix" ? historyItem?.flowRef : undefined,
            timeline: [],
            artifacts: [],
            pendingApprovals: [],
            pendingQuestions: [],
            gateBlock: undefined,
            latestTokenUsage: undefined,
            lastTurnInput: undefined,
            recoverable: false,
            pendingAccountSwitch: undefined,
            accountSwitchLoading: false,
            pendingProviderSwitch: undefined,
            providerSwitchLoading: false,
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
            // Suppress the "AI response complete" toast while the transcript replays. (BUG-118)
            _historyReplaying: true,
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
        // run-63960: seed agent graph ASAP so FlowAwaitingUserCard can render
        // Continue/Stop when loop is blocked after restart — do not wait only on SSE.
        // Stale-response guarded; does not invent SSE seq (race with Continue/Stop).
        requestAgentGraphRefresh(handle.runId, get, set, { settleBlockedTimeline: true });
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
            set(() => ({ _historyReplaying: false }));
            if (activeHistoryReplayController === historyReplayController) {
                activeHistoryReplayController = undefined;
            }
        });
        startOrchestrationStream(handle.runId, client, set, get);
        void get().refreshAgentRuns();
        void get().refreshWorkflowStepRuntime();
    },
    resetRun() {
        const { selectedProvider, supportedModels } = get();
        cancelHistoryReplayStream();
        cancelOrchestrationStream();
        cancelAgentFocusStream();
        set({
            runId: undefined,
            chatId: undefined,
            mainRunId: undefined,
            activeAgentRunId: undefined,
            activeStepId: undefined,
            status: "idle",
            timeline: [],
            artifacts: [],
            agentRuns: [],
            agentGraphSnapshot: undefined,
            agentBusMessages: [],
            workflowStepRuntime: [],
            workflowStepRuntimeMeta: {},
            agentSpawnGuideOpen: false,
            agentSpawnGuideAgentName: undefined,
            pendingApprovals: [],
            pendingQuestions: [],
            latestTokenUsage: undefined,
            lastTurnInput: undefined,
            recoverable: false,
            pendingAccountSwitch: undefined,
            accountSwitchLoading: false,
            pendingProviderSwitch: undefined,
            providerSwitchLoading: false,
            _accountSwitchTriedIds: [],
            _streamingAssistantId: undefined,
            _runSnapshots: {},
            _runReplaySeq: {},
            chatStartMode: "normal",
            chatSourceDocId: "",
            flowRef: undefined,
            builtinOrchestrationOptions: [],
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
    dismissGateBlock() {
        set({ gateBlock: undefined });
    },
    dismissHistoryOpenError() {
        set({ historyOpenError: undefined });
    },
    async submitGateDecision(option, customText) {
        const { gateBlock, client } = get();
        if (!gateBlock?.runId || !client.submitGateDecision)
            return;
        const runId = gateBlock.runId;
        set({ gateBlock: undefined });
        await client.submitGateDecision(runId, option, customText);
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
    if (providerKey === "gemini")
        return "Gemini";
    if (providerKey === "grok")
        return "Grok";
    if (providerKey === "opencode")
        return "OpenCode";
    return providerKey;
}
// ── Workflow-step runtime helpers (BUG-153) ────────────────────────────────
// Derived from workflowStepRuntime + chatMode rather than stored separately,
// so there is exactly one source of truth to keep in sync.
/** Flow mode only; Review Loop / normal chat has no linear workflow-step list (F-14). */
function isFlowModeRun(chatMode) {
    return chatMode === "workflow_step_auto";
}
/** The step currently RUNNING or WAITING_USER_APPROVAL, if any. */
function activeWorkflowStep(steps) {
    return steps.find((s) => s.status === "RUNNING" || s.status === "WAITING_USER_APPROVAL");
}
/** True when any step has been retried at least once (F-5). */
function hasRetries(steps) {
    return steps.some((s) => s.retryCount > 0);
}
function isInteractiveChatBlocked(status) {
    return status === "running" || status === "waiting_approval" || status === "waiting_question";
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
        // BUG-180: the hub's first turn is consumed here (not the orchestration
        // stream, which only starts after the turn), and the flow executor reseeds +
        // transitions steps while the coder runs during that turn. Each transition
        // rides alongside an agent_graph_updated, so refresh the step runtime here too
        // to keep the timeline live during the initial coder phase. (Self-guarded.)
        if (e.type === "agent_graph_updated")
            void get().refreshWorkflowStepRuntime();
        if (e.type === "agent_graph_updated")
            void get().refreshAgentRuns();
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
    const persistedEvents = [];
    const replayBoundary = lastEventSeq && lastEventSeq > 0 ? lastEventSeq : undefined;
    let replayingPersistedEvents = replayBoundary !== undefined;
    const flushPersistedEvents = () => {
        if (persistedEvents.length === 0 || isStale())
            return;
        applyHistoryReplayEvents(runId, persistedEvents, set);
        persistedEvents.length = 0;
        settleTerminalReplayVisuals(runId, resumedStatus, set);
    };
    for await (const e of stream) {
        if (isStale())
            return;
        if (!isEventForRun(e, runId))
            continue;
        if (replayingPersistedEvents) {
            persistedEvents.push(e);
            if (e.seq < replayBoundary)
                continue;
            flushPersistedEvents();
            replayingPersistedEvents = false;
            if (shouldStopHistoryReplay(resumedStatus, e, lastEventSeq))
                break;
            continue;
        }
        if (e.type !== "agent_graph_updated" && e.type !== "agent_bus_message") {
            set((s) => applyEvent(s, e));
            settleTerminalReplayVisuals(runId, resumedStatus, set);
        }
        if (shouldStopHistoryReplay(resumedStatus, e, lastEventSeq))
            break;
    }
    flushPersistedEvents();
    if (!isStale()) {
        settleHistoryReplayPendingState(runId, resumedStatus, set);
    }
}
/**
 * Persisted events can be appended after recovery even when their observed time
 * belongs in an earlier turn. Replay uses that durable time, then the stream
 * sequence as a stable tie-breaker, so cards stay in their original turn.
 */
function orderHistoryReplayEvents(events) {
    const entries = events.map((event, index) => ({
        event,
        index,
        observedAt: Date.parse(event.occurredAt),
        replayAt: Date.parse(event.occurredAt),
    }));
    let latestAgentSpawnAt = Number.NaN;
    // A replay stream's sequence is causal. Some legacy transcript frames have
    // run-created timestamps rather than their original observed time (run-1264),
    // so never let an event persisted after an agent spawn render ahead of it.
    for (const entry of [...entries].sort((left, right) => {
        if (left.event.seq !== right.event.seq)
            return left.event.seq - right.event.seq;
        return left.index - right.index;
    })) {
        if (entry.event.type === "agent_spawned_by_user" && Number.isFinite(entry.observedAt)) {
            latestAgentSpawnAt = entry.observedAt;
        }
        if (Number.isFinite(latestAgentSpawnAt) &&
            Number.isFinite(entry.observedAt) &&
            entry.event.type !== "agent_spawned_by_user" &&
            entry.observedAt < latestAgentSpawnAt) {
            entry.replayAt = latestAgentSpawnAt;
        }
    }
    // run-24377: hub turn-log prose is often untimed while agent cards carry
    // child wall-clock starts. Preferring timed events over untimed ones dumps
    // every agent card above the original user prompt on history reopen.
    // When any frame lacks a finite time, preserve server Seq (causal order).
    const anyUntimed = entries.some((entry) => !Number.isFinite(entry.replayAt));
    if (anyUntimed) {
        return entries
            .sort((left, right) => {
            if (left.event.seq !== right.event.seq)
                return left.event.seq - right.event.seq;
            return left.index - right.index;
        })
            .map(({ event }) => event);
    }
    return entries
        .sort((left, right) => {
        if (left.replayAt !== right.replayAt) {
            return left.replayAt - right.replayAt;
        }
        if (left.event.seq !== right.event.seq)
            return left.event.seq - right.event.seq;
        return left.index - right.index;
    })
        .map(({ event }) => event);
}
function applyHistoryReplayEvents(runId, events, set) {
    const orderedEvents = orderHistoryReplayEvents(events);
    const highestSeq = events.reduce((highest, event) => Math.max(highest, event.seq), 0);
    set((state) => {
        if (state.runId !== runId)
            return {};
        let next = state;
        for (const event of orderedEvents) {
            if (event.type === "agent_graph_updated" || event.type === "agent_bus_message")
                continue;
            next = { ...next, ...applyEvent(next, event) };
        }
        return {
            ...next,
            _runReplaySeq: {
                ...next._runReplaySeq,
                [runId]: highestSeq,
            },
        };
    });
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
        if (s.runId !== runId || (s.pendingApprovals.length === 0 && s.pendingQuestions.length === 0))
            return {};
        // An approval/question is still genuinely open if its own card was never stamped
        // with a decision — checked per-id (not just the last timeline item) because
        // concurrent tool calls can leave several cards outstanding at once (BUG-157).
        // This also covers BUG-105: resumedStatus is stale server ground truth when the
        // replay stream itself ends on an unresolved permission_required/question.
        const stillOpenApprovals = s.pendingApprovals.filter((pending) => s.timeline.some((it) => it.kind === "approval" && it.approvalId === pending.approvalId && it.decision === undefined));
        const stillOpenQuestions = s.pendingQuestions.filter((pending) => s.timeline.some((it) => it.kind === "question" && it.questionId === pending.questionId && it.answer === undefined));
        if (stillOpenApprovals.length > 0 || stillOpenQuestions.length > 0) {
            return {
                pendingApprovals: stillOpenApprovals,
                pendingQuestions: stillOpenQuestions,
                status: stillOpenApprovals.length > 0 ? "waiting_approval" : "waiting_question",
            };
        }
        const staleApprovalIds = new Set(s.pendingApprovals.map((p) => p.approvalId));
        const staleQuestionIds = new Set(s.pendingQuestions.map((q) => q.questionId));
        return {
            pendingApprovals: [],
            pendingQuestions: [],
            timeline: s.timeline.map((it) => {
                if (it.kind === "approval" && staleApprovalIds.has(it.approvalId) && it.decision === undefined) {
                    return { ...it, decision: "resolved" };
                }
                if (it.kind === "question" && staleQuestionIds.has(it.questionId) && it.answer === undefined) {
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
        if (e.type === "agent_graph_updated" || e.type === "agent_bus_message") {
            set((s) => applyOrchestrationEvent(s, e));
            // BUG-180: the flow executor's step transitions (node spawn → RUNNING,
            // cohort-join → DONE, etc.) are store writes with no dedicated event, so the
            // step timeline was stale until a focus switch re-fetched it. Every such
            // transition rides alongside an agent_graph_updated (child lifecycle change),
            // so refresh the step-runtime here to make the timeline update live.
            // refreshWorkflowStepRuntime self-guards (chatMode + load-seq + target-run),
            // so this is safe and de-duped against races.
            if (e.type === "agent_graph_updated") {
                void get().refreshWorkflowStepRuntime();
                void get().refreshAgentRuns();
            }
        }
        else {
            // CP-35: gate reprompt events (turn_started, message_delta, turn_completed, etc.)
            // arrive after sendTurn() has already closed on turn_completed. Apply them via
            // applyEvent so the timeline shows the reprompt turn without a tab-switch.
            //
            // Dynamic watermark: skip events already covered by the concurrent history replay
            // stream (_runReplaySeq tracks its progress as it goes). Without this guard, when
            // openHistoryRun triggers both a replay stream and this orchestration stream from
            // seq 0, the orchestration stream re-processes flow_gate_violation after
            // _historyReplaying turns false — re-popping the block modal on every chat open.
            // (CP-35 BUG-138)
            if (e.seq <= (get()._runReplaySeq[runId] ?? afterSeq))
                continue;
            // BUG-297: this stream stays bound to MAIN (runId) for the whole session, even
            // while the user has focused a DIFFERENT run's transcript (s.timeline is one
            // shared field, not partitioned per run). Applying unconditionally bled MAIN's
            // own live events (e.g. a sibling agent_spawned_by_user for a reviewer child)
            // straight into whatever child transcript happened to be on screen. Only apply
            // to the shared timeline when MAIN is actually the currently displayed run —
            // backToMainRun already replays everything from its pre-focus snapshot on
            // return (store.ts, afterSeq: restore.lastEventSeq), so skipping here while a
            // child is focused loses nothing: the event is still fully caught up on return.
            if (get().runId !== runId)
                continue;
            set((s) => applyEvent(s, e));
        }
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
function hasActiveParentAgentLoop(state, parentRunId) {
    const snapshot = state.agentGraphSnapshot;
    if (!snapshot || snapshot.parentRunId !== parentRunId)
        return false;
    const status = snapshot.loopState.status;
    return Boolean(status) && status !== "done" && status !== "stopped";
}
/** Pull AgentGraphSnapshot from stopAgentLoop partial-failure body (CP-51 A1). */
function extractStopSnapshot(err) {
    if (err instanceof HttpWsRunnerClient_1.RunnerApiError && err.snapshot && typeof err.snapshot === "object") {
        const snap = err.snapshot;
        if (snap.loopState && typeof snap.loopState.status === "string") {
            return snap;
        }
    }
    return undefined;
}
function settleTerminalReplayVisuals(runId, replayStatus, set) {
    if (!isTerminalRunStatus(replayStatus))
        return;
    set((s) => {
        if (s.runId !== runId)
            return {};
        return {
            status: replayStatus,
            timeline: settleCompletedFlowTimeline(s.timeline),
        };
    });
}
// Merge an incoming agent-run snapshot into the existing list by runId (incoming wins).
// The live SSE graph snapshot is in-memory only and omits disk-persisted closed children
// that the HTTP list (listAgentRunSummaries) includes; replacing wholesale dropped the
// "Recently closed" entries while an agent was running. Merging preserves them (BUG-132).
//
// BUG-235: a run's status is monotonic once terminal (completed/failed/cancelled never
// goes back to running/waiting) — but incoming can be a STALE snapshot: refreshAgentRuns'
// HTTP request is fire-and-forget and can be captured server-side before a child finished,
// then resolve and land AFTER the SSE agent_graph_updated event that already correctly
// marked it terminal. Unconditional "incoming wins" let that late, stale "running" revert
// the already-correct terminal status — and since no further event fires for an already-
// finished child, it stayed wrongly "running" forever (Agents panel + the leftover
// "reviewer · running" card in the main chat, even after the whole flow completed). Never
// let a non-terminal incoming status overwrite an existing terminal one.
function mergeAgentRunsById(existing, incoming) {
    const byId = new Map();
    for (const run of existing)
        byId.set(run.runId, run);
    for (const run of incoming) {
        const prev = byId.get(run.runId);
        if (prev && isTerminalRunStatus(prev.status) && !isTerminalRunStatus(run.status)) {
            // BUG-235: never let a stale HTTP snapshot revert an already-terminal status.
            // Exception (BUG-Rnd2): when the backend genuinely reinvokes the same runId
            // (lifecycle: reinvoke), it increments activationSeq. A higher activationSeq
            // means this is a real completed→running transition, not a stale snapshot.
            const isGenuineReinvoke = (run.activationSeq ?? 0) > (prev.activationSeq ?? 0);
            if (!isGenuineReinvoke)
                continue;
        }
        byId.set(run.runId, run);
    }
    return [...byId.values()];
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
            // Merge (not replace) so disk-persisted closed children stay visible while a new
            // agent runs and emits in-memory-only snapshots (BUG-132).
            agentRuns: mergeAgentRunsById(s.agentRuns, e.agentGraphSnapshot.runs),
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
    if (e.type === "flow_gate_violation" && (e.status === "block" || e.status === "warn") && !s._historyReplaying) {
        // block: hard stop — surface a modal the user must acknowledge. The modal must appear
        // EXACTLY ONCE: Reopening the chat re-streams the persisted flow_gate_violation, and
        // the `_historyReplaying` guard is racy. `_gateBlockedRunIds` is the authoritative guard:
        // added on the first block, cleared on the next turn_started. (CP-35, BUG-138)
        //
        // warn: store settles to "completed" (statusFromEvent), but the backend runner keeps the
        // run in "running" state until the user re-prompts — the same mismatch as block. Without
        // tracking in _gateBlockedRunIds, switching to another chat makes the Navigator fall back
        // to item.status = "running" and show an infinite spinner. (BUG-145)
        //
        // Both cases: add to _gateBlockedRunIds so Navigator shows a stable completed icon for
        // inactive gate-settled runs. Cleared on the next turn_started. (BUG-137)
        const alreadyBlocked = Boolean(s._gateBlockedRunIds[e.workflowRunId]);
        const newGateBlock = e.status === "block" && !alreadyBlocked
            ? {
                message: e.error,
                options: e.gateOptions,
                regressedTests: e.gateRegressedTests,
                runId: e.workflowRunId,
            }
            : undefined;
        return {
            ...next,
            ...(newGateBlock ? { gateBlock: newGateBlock } : {}),
            _gateBlockedRunIds: { ...s._gateBlockedRunIds, [e.workflowRunId]: true },
            _runReplaySeq: nextReplaySeq,
        };
    }
    if (e.type === "turn_started") {
        // A fresh turn (incl. a gate reprompt) clears any prior block modal and removes the
        // run from the gate-blocked set (the user re-prompted, so the run is running again).
        const { [e.workflowRunId]: _cleared, ...remainingGateBlockedRunIds } = s._gateBlockedRunIds;
        return {
            ...next,
            latestTokenUsage: undefined,
            gateBlock: undefined,
            _gateBlockedRunIds: remainingGateBlockedRunIds,
            _runReplaySeq: nextReplaySeq,
        };
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
        const nextStatus = deriveOrchestrationRunStatus(s.status, e.agentGraphSnapshot);
        const loopStatus = e.agentGraphSnapshot.loopState.status;
        // Terminal/control-flow graph is authoritative even when a provider never
        // emits trailing tool_completed/turn_completed after flow control:
        // - done → completed (existing)
        // - blocked (cap/escalate awaiting user, run-63960) → clear stale Thinking
        //   residue. Status derivation stays separate (BUG-231 suite: running child
        //   can still derive "running"; timeline residue must not linger either way).
        const settleTimelineResidue = (loopStatus === "done" && nextStatus === "completed") || loopStatus === "blocked";
        return {
            // Merge (not replace) so disk-persisted closed children stay visible (BUG-132).
            agentRuns: mergeAgentRunsById(s.agentRuns, e.agentGraphSnapshot.runs),
            agentGraphSnapshot: e.agentGraphSnapshot,
            agentBusMessages: e.agentGraphSnapshot.busMessages,
            status: nextStatus,
            timeline: settleTimelineResidue ? settleCompletedFlowTimeline(s.timeline) : s.timeline,
            _runReplaySeq: nextReplaySeq,
            // Invalidate in-flight HTTP graph refreshes so they cannot overwrite SSE.
            _agentGraphLoadSeq: s._agentGraphLoadSeq + 1,
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
function deriveOrchestrationRunStatus(current, snapshot) {
    // BUG-248: a "stopped" loop is a definitive, user-initiated full halt of the
    // FLOW — stopAgentLoop already cancelled children. Stale child snapshots can
    // still report "running", so we must not derive "running" from children when
    // the loop is stopped.
    //
    // BUG-308 residual (run-33289 UI): Stop ends the flow, not the chat. A plain
    // follow-up turn_started/turn_completed updates `current` to running/completed
    // while loopState stays "stopped". Do NOT force "cancelled" over those chat
    // statuses or the header/history stick on Cancelled after a successful reply.
    // stop() itself still forces cancelled at the Stop press (see stop handler).
    if (snapshot.loopState.status === "stopped") {
        // Preserve a successful/failed plain-chat follow-up (turn_completed already
        // set completed). Still force cancelled for running/waiting so BUG-248 holds:
        // right after Stop, current is often still "running" while children look
        // live — that must read Cancelled until a later turn event advances it.
        // stop() also forces cancelled at the Stop press.
        if (current === "completed" || current === "failed") {
            return current;
        }
        return "cancelled";
    }
    // Like a stopped loop, a done loop is authoritative over an older child
    // snapshot or a provider stream that remains open after submit_review_outcome.
    if (snapshot.loopState.status === "done") {
        return current === "failed" || current === "cancelled" ? current : "completed";
    }
    const childStatuses = snapshot.runs.map((run) => run.status);
    if (childStatuses.some((status) => status === "waiting_approval")) {
        return "waiting_approval";
    }
    if (childStatuses.some((status) => status === "waiting_question")) {
        return "waiting_question";
    }
    if (childStatuses.some((status) => status === "running" || status === "waiting_approval" || status === "waiting_question")) {
        return "running";
    }
    switch (snapshot.loopState.status) {
        case "running":
        case "paused":
            return "running";
        case "blocked":
            // BUG-231: a blocked loop is a deliberate, non-terminal "awaiting user"
            // pause (escalate, or the round cap reached) — it must NOT read as
            // "running", or the composer stays locked ("Waiting for the current
            // turn…") with no way for the very user the flow is waiting on to respond.
            return "blocked";
        case "stopped":
            return "cancelled";
        default:
            return current;
    }
}
/** Closes UI-only residue when durable flow control has already reached done. */
function settleCompletedFlowTimeline(timeline) {
    return timeline
        .filter((item) => item.kind !== "thinking")
        .map((item) => {
        if (item.kind === "tool" && item.status === "running") {
            return { ...item, status: "success" };
        }
        return item;
    });
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
    const pending = sanitizePendingSnapshotState(state.status, state.pendingApprovals, state.pendingQuestions);
    return {
        timeline: state.timeline,
        artifacts: state.artifacts,
        status: state.status,
        pendingApprovals: pending.pendingApprovals,
        pendingQuestions: pending.pendingQuestions,
        latestTokenUsage: state.latestTokenUsage,
        lastTurnInput: state.lastTurnInput,
        recoverable: state.recoverable,
        _streamingAssistantId: state._streamingAssistantId,
        activeStepId: state.activeStepId,
        lastEventSeq: state._runReplaySeq[state.runId ?? ""] ?? state._runReplaySeq[state.mainRunId ?? ""] ?? undefined,
    };
}
function restoreRunSnapshot(snapshot) {
    const pending = sanitizePendingSnapshotState(snapshot.status, snapshot.pendingApprovals, snapshot.pendingQuestions);
    return {
        timeline: snapshot.timeline,
        artifacts: snapshot.artifacts,
        status: snapshot.status,
        pendingApprovals: pending.pendingApprovals,
        pendingQuestions: pending.pendingQuestions,
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
        pendingApprovals: [],
        pendingQuestions: [],
        gateBlock: undefined,
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

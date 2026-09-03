"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const MockRunnerClient_1 = require("../client/MockRunnerClient");
const chatHistory_1 = require("./chatHistory");
// CP-59 Task-316 (CA-700): chat-scoped provider switch keeps the timeline,
// appends exactly one divider, mints exactly one leg per confirm, and the
// legacy Task-078 path stays verbatim when the chat SSOT is unknown.
const baseState = () => ({
    client: new MockRunnerClient_1.MockRunnerClient(),
    selectedProjectId: "proj-1",
    selectedProvider: "codex",
    selectedModel: "gpt-5.4",
    reasoningEffort: "medium",
    yoloMode: false,
    chatMode: "normal_chat",
    runId: "run-1",
    chatId: "cht_a",
    providerSwitchLoading: false,
    pendingProviderSwitch: {
        sourceRunId: "run-1",
        sourceProviderKey: "codex",
        sourceRunStatus: "idle",
        targetProviderKey: "grok",
        targetModel: "grok-4.5",
    },
});
(0, node_test_1.default)("provider switch keeps the timeline and appends exactly one divider", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    const state = { ...baseState(), client };
    store_1.useStore.setState(state);
    store_1.useStore.setState({
        timeline: [
            { kind: "prompt", id: "u1", text: "hello ban la model gi" },
            { kind: "assistant", id: "a1", text: "toi la Muse Spark", finalized: true },
        ],
    });
    await store_1.useStore.getState().confirmProviderSwitch();
    const s = store_1.useStore.getState();
    strict_1.default.equal(s.runId, `mock-run-cht_a-1`);
    strict_1.default.equal(s.chatId, "cht_a");
    strict_1.default.equal(s.providerSwitchLoading, false);
    strict_1.default.equal(s.pendingProviderSwitch, undefined);
    const dividers = s.timeline.filter((t) => t.id === "seed-divider-mock-run-cht_a-1");
    strict_1.default.equal(dividers.length, 1, "exactly one divider");
    strict_1.default.ok(String(dividers[0].text ?? "").includes("carried 2 turns (raw)"));
    // Transcript continuity: the two prior messages are still in place.
    strict_1.default.equal(s.timeline.length, 3);
    strict_1.default.equal(s.timeline[0].kind, "prompt");
});
(0, node_test_1.default)("double confirm mints one leg", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    store_1.useStore.setState({ ...baseState(), client });
    const p1 = store_1.useStore.getState().confirmProviderSwitch();
    const p2 = store_1.useStore.getState().confirmProviderSwitch(); // second while in flight
    await Promise.allSettled([p1, p2]);
    strict_1.default.equal(store_1.useStore.getState().runId, "mock-run-cht_a-1");
});
(0, node_test_1.default)("operational state resets, artifacts retained", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    store_1.useStore.setState({ ...baseState(), client, artifacts: [{ id: "art-1" }] });
    await store_1.useStore.getState().confirmProviderSwitch();
    const s = store_1.useStore.getState();
    strict_1.default.equal(s.pendingApprovals.length, 0);
    strict_1.default.equal(s.gateBlock, undefined);
    strict_1.default.equal(s.artifacts.length, 1, "artifacts must be retained");
});
(0, node_test_1.default)("legacy fallback path unchanged when the chat SSOT is unknown", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    let handoffCalled = false;
    let startRunCalls = 0;
    client.handoffContext = async (runId, input) => {
        handoffCalled = true;
        return {
            sourceRunId: runId,
            sourceProviderKey: "codex",
            targetProviderKey: input.targetProviderKey,
            prompt: "[mock handoff]",
            includedTurnCount: 0,
            omittedTurnCount: 0,
            truncated: false,
            handoffMode: "raw",
        };
    };
    const origStart = client.startRun.bind(client);
    client.startRun = async (input) => {
        startRunCalls++;
        return origStart(input);
    };
    store_1.useStore.setState({ ...baseState(), client, chatId: undefined, runId: "run-legacy-1" });
    store_1.useStore.setState({
        timeline: [{ kind: "prompt", id: "u1", text: "old" }],
    });
    await store_1.useStore.getState().confirmProviderSwitch();
    strict_1.default.equal(handoffCalled, true);
    strict_1.default.equal(startRunCalls, 1);
    const s = store_1.useStore.getState();
    // Legacy path resets the timeline (Task-078 parity).
    strict_1.default.equal(s.timeline.filter((t) => t.id === "u1").length, 0);
    // Mock startRun mints its own id — the adoption must land on it.
    strict_1.default.equal(s.runId, store_1.useStore.getState().runId);
    strict_1.default.notEqual(s.runId, "run-legacy-1");
});
(0, node_test_1.default)("same-provider chip selection skips the endpoint (in-place)", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    let switchCalls = 0;
    client.switchChatProvider = async (chatId, input) => {
        switchCalls++;
        throw new Error("should not be called");
    };
    store_1.useStore.setState({
        client,
        chatId: "cht_a",
        runId: "run-1",
        selectedProvider: "codex",
        chatMode: "normal_chat",
    });
    store_1.useStore.getState().selectProvider("codex");
    strict_1.default.equal(switchCalls, 0);
});
(0, node_test_1.default)("run history groups legs under one chat", () => {
    const rows = (0, chatHistory_1.groupRunsByChatId)([
        { runId: "run-1", projectId: "p", providerKey: "codex", status: "completed", startedAt: "", updatedAt: "", chatId: "cht_a", legSeq: 0 },
        { runId: "run-2", projectId: "p", providerKey: "grok", status: "running", startedAt: "", updatedAt: "", chatId: "cht_a", legSeq: 1 },
    ]);
    const flat = (0, chatHistory_1.flattenGroupedHistory)(rows);
    strict_1.default.equal(flat.length, 1);
    strict_1.default.equal(flat[0].legsCount, 2);
});

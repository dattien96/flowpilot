"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const MockRunnerClient_1 = require("../client/MockRunnerClient");
// TUI+Desktop restart bug: closing and reopening must resume the runner's
// persisted active posture (scan/plan/code) — not reset to code.
(0, node_test_1.default)("loadChatPostureConfig restores runner active and pinned profile", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    client.getChatPosture = async () => ({
        active: "plan",
        profiles: {
            scan: {},
            plan: { provider: "claude", model: "sonnet", reasoningEffort: "high", yolo: true },
            code: {},
        },
    });
    store_1.useStore.setState({
        client: client,
        chatPosture: "code",
        chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
        selectedProvider: "codex",
        selectedModel: "o3",
        yoloMode: false,
        reasoningEffort: "medium",
    });
    await store_1.useStore.getState().loadChatPostureConfig();
    const s = store_1.useStore.getState();
    strict_1.default.equal(s.chatPosture, "plan", "must restore active=plan from runner");
    strict_1.default.equal(s.chatPostureConfig.active, "plan");
    strict_1.default.equal(s.selectedProvider, "claude", "must apply plan provider pin");
    strict_1.default.equal(s.selectedModel, "sonnet", "must apply plan model pin");
    strict_1.default.equal(s.reasoningEffort, "high");
    strict_1.default.equal(s.yoloMode, true);
});
(0, node_test_1.default)("loadChatPostureConfig restores scan with Grok YOLO and triggers sync", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    const yoloCalls = [];
    const orig = client.applyGrokYoloPosture.bind(client);
    client.applyGrokYoloPosture = async (yolo) => {
        yoloCalls.push(yolo);
        return orig(yolo);
    };
    client.getChatPosture = async () => ({
        active: "scan",
        profiles: { scan: { provider: "grok", yolo: false }, plan: {}, code: {} },
    });
    store_1.useStore.setState({
        client: client,
        chatPosture: "code",
        chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
        selectedProvider: "codex",
        yoloMode: true,
    });
    await store_1.useStore.getState().loadChatPostureConfig();
    const s = store_1.useStore.getState();
    strict_1.default.equal(s.chatPosture, "scan");
    strict_1.default.equal(s.selectedProvider, "grok");
    strict_1.default.equal(s.yoloMode, false);
    strict_1.default.equal(yoloCalls.length, 1, "Grok restore must sync YOLO");
    strict_1.default.equal(yoloCalls[0], false);
});
(0, node_test_1.default)("loadChatPostureConfig keeps code when runner says code", async () => {
    const client = new MockRunnerClient_1.MockRunnerClient();
    client.getChatPosture = async () => ({
        active: "code",
        profiles: { scan: {}, plan: {}, code: {} },
    });
    store_1.useStore.setState({
        client: client,
        chatPosture: "code",
        chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
        selectedProvider: "codex",
    });
    await store_1.useStore.getState().loadChatPostureConfig();
    strict_1.default.equal(store_1.useStore.getState().chatPosture, "code");
});

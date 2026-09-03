"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const store_1 = require("./store");
const MockRunnerClient_1 = require("../client/MockRunnerClient");
// R2/R3: Desktop setChatPosture with Grok provider + YOLO pin must call
// applyGrokYoloPosture so the Grok config.toml is rewritten (Task-218).
(0, node_test_1.default)("setChatPosture calls applyGrokYoloPosture for Grok + YOLO pin", async () => {
    const calls = [];
    const client = new MockRunnerClient_1.MockRunnerClient();
    // Override getChatPosture to return scan profile with Grok + YOLO pin
    client.getChatPosture = async () => ({
        active: "scan",
        profiles: {
            scan: { provider: "grok", yolo: false },
            plan: {},
            code: {},
        },
    });
    // Spy on applyGrokYoloPosture
    const orig = client.applyGrokYoloPosture.bind(client);
    client.applyGrokYoloPosture = async (yolo) => {
        calls.push(yolo);
        return orig(yolo);
    };
    store_1.useStore.setState({
        client,
        selectedProvider: "grok",
        chatPosture: "code",
        chatPostureConfig: {
            active: "code",
            profiles: { scan: { provider: "grok", yolo: false }, plan: {}, code: {} },
        },
        chatMode: "normal_chat",
    });
    await store_1.useStore.getState().setChatPosture("scan");
    // applyGrokYoloPosture must have been called with false (the YOLO pin)
    strict_1.default.equal(calls.length, 1, `expected 1 applyGrokYoloPosture call, got ${calls.length}`);
    strict_1.default.equal(calls[0], false, "applyGrokYoloPosture should be called with yolo=false");
    // yoloMode must also be set in the store
    strict_1.default.equal(store_1.useStore.getState().yoloMode, false, "yoloMode must be false after scan pin");
});
// R2/R3: Claude provider must NOT trigger Grok YOLO sync.
(0, node_test_1.default)("setChatPosture does NOT call applyGrokYoloPosture for Claude", async () => {
    const calls = [];
    const client = new MockRunnerClient_1.MockRunnerClient();
    client.getChatPosture = async () => ({
        active: "scan",
        profiles: { scan: { provider: "claude", yolo: false }, plan: {}, code: {} },
    });
    const orig = client.applyGrokYoloPosture.bind(client);
    client.applyGrokYoloPosture = async (yolo) => {
        calls.push(yolo);
        return orig(yolo);
    };
    store_1.useStore.setState({
        client,
        selectedProvider: "claude",
        chatPosture: "code",
        chatPostureConfig: {
            active: "code",
            profiles: { scan: { provider: "claude", yolo: false }, plan: {}, code: {} },
        },
        chatMode: "normal_chat",
    });
    await store_1.useStore.getState().setChatPosture("scan");
    strict_1.default.equal(calls.length, 0, `Claude must not trigger applyGrokYoloPosture, got ${calls.length} calls`);
});

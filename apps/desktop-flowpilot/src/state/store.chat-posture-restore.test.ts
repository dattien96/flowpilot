import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

// TUI+Desktop restart bug: closing and reopening must resume the runner's
// persisted active posture (scan/plan/code) — not reset to code.
test("loadChatPostureConfig restores runner active and pinned profile", async () => {
  const client = new MockRunnerClient();
  client.getChatPosture = async () => ({
    active: "plan" as const,
    profiles: {
      scan: {},
      plan: { provider: "claude", model: "sonnet", reasoningEffort: "high", yolo: true },
      code: {},
    },
  });

  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatPosture: "code",
    chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
    selectedProvider: "codex",
    selectedModel: "o3",
    yoloMode: false,
    reasoningEffort: "medium",
  });

  await useStore.getState().loadChatPostureConfig();

  const s = useStore.getState();
  assert.equal(s.chatPosture, "plan", "must restore active=plan from runner");
  assert.equal(s.chatPostureConfig.active, "plan");
  assert.equal(s.selectedProvider, "claude", "must apply plan provider pin");
  assert.equal(s.selectedModel, "sonnet", "must apply plan model pin");
  assert.equal(s.reasoningEffort, "high");
  assert.equal(s.yoloMode, true);
});

test("loadChatPostureConfig restores scan with Grok YOLO and triggers sync", async () => {
  const client = new MockRunnerClient();
  const yoloCalls: boolean[] = [];
  const orig = client.applyGrokYoloPosture.bind(client);
  client.applyGrokYoloPosture = async (yolo: boolean) => {
    yoloCalls.push(yolo);
    return orig(yolo);
  };
  client.getChatPosture = async () => ({
    active: "scan" as const,
    profiles: { scan: { provider: "grok", yolo: false }, plan: {}, code: {} },
  });

  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatPosture: "code",
    chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
    selectedProvider: "codex",
    yoloMode: true,
  });

  await useStore.getState().loadChatPostureConfig();

  const s = useStore.getState();
  assert.equal(s.chatPosture, "scan");
  assert.equal(s.selectedProvider, "grok");
  assert.equal(s.yoloMode, false);
  assert.equal(yoloCalls.length, 1, "Grok restore must sync YOLO");
  assert.equal(yoloCalls[0], false);
});

test("loadChatPostureConfig keeps code when runner says code", async () => {
  const client = new MockRunnerClient();
  client.getChatPosture = async () => ({
    active: "code" as const,
    profiles: { scan: {}, plan: {}, code: {} },
  });

  useStore.setState({
    client: client as unknown as ReturnType<typeof useStore.getState>["client"],
    chatPosture: "code",
    chatPostureConfig: { active: "code", profiles: { scan: {}, plan: {}, code: {} } },
    selectedProvider: "codex",
  });

  await useStore.getState().loadChatPostureConfig();
  assert.equal(useStore.getState().chatPosture, "code");
});

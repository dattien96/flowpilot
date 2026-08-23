import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";
import type { ChatPostureConfig } from "../types/contract";

// R2/R3: Desktop setChatPosture with Grok provider + YOLO pin must call
// applyGrokYoloPosture so the Grok config.toml is rewritten (Task-218).
test("setChatPosture calls applyGrokYoloPosture for Grok + YOLO pin", async () => {
  const calls: boolean[] = [];
  const client = new MockRunnerClient();

  // Override getChatPosture to return scan profile with Grok + YOLO pin
  client.getChatPosture = async () => ({
    active: "scan" as const,
    profiles: {
      scan: { provider: "grok", yolo: false },
      plan: {},
      code: {},
    },
  });

  // Spy on applyGrokYoloPosture
  const orig = client.applyGrokYoloPosture.bind(client);
  client.applyGrokYoloPosture = async (yolo: boolean) => {
    calls.push(yolo);
    return orig(yolo);
  };

  useStore.setState({
    client,
    selectedProvider: "grok",
    chatPosture: "code",
    chatPostureConfig: {
      active: "code",
      profiles: { scan: { provider: "grok", yolo: false }, plan: {}, code: {} },
    },
    chatMode: "normal_chat",
  });

  await useStore.getState().setChatPosture("scan");

  // applyGrokYoloPosture must have been called with false (the YOLO pin)
  assert.equal(calls.length, 1, `expected 1 applyGrokYoloPosture call, got ${calls.length}`);
  assert.equal(calls[0], false, "applyGrokYoloPosture should be called with yolo=false");

  // yoloMode must also be set in the store
  assert.equal(useStore.getState().yoloMode, false, "yoloMode must be false after scan pin");
});

// R2/R3: Claude provider must NOT trigger Grok YOLO sync.
test("setChatPosture does NOT call applyGrokYoloPosture for Claude", async () => {
  const calls: boolean[] = [];
  const client = new MockRunnerClient();

  client.getChatPosture = async () => ({
    active: "scan" as const,
    profiles: { scan: { provider: "claude", yolo: false }, plan: {}, code: {} },
  });

  const orig = client.applyGrokYoloPosture.bind(client);
  client.applyGrokYoloPosture = async (yolo: boolean) => {
    calls.push(yolo);
    return orig(yolo);
  };

  useStore.setState({
    client,
    selectedProvider: "claude",
    chatPosture: "code",
    chatPostureConfig: {
      active: "code",
      profiles: { scan: { provider: "claude", yolo: false }, plan: {}, code: {} },
    },
    chatMode: "normal_chat",
  });

  await useStore.getState().setChatPosture("scan");

  assert.equal(calls.length, 0, `Claude must not trigger applyGrokYoloPosture, got ${calls.length} calls`);
});

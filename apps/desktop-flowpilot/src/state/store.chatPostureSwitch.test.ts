import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import { MockRunnerClient } from "../client/MockRunnerClient";

// BUG-349 (TUI routePostureSwitch parity): a cross-provider posture Tab on a
// live chat must route to the switch endpoint (new leg + one divider,
// timeline kept) instead of silently flipping the session provider. A
// bare-model pin derives its provider once, persists it, and notifies once.

const baseState = (client: MockRunnerClient) => ({
  client,
  selectedProjectId: "proj-1",
  selectedProvider: "opencode" as const,
  selectedModel: "opencode-model",
  reasoningEffort: "medium",
  yoloMode: false,
  chatMode: "normal_chat" as const,
  chatPosture: "code" as const,
  runId: "run-1",
  chatId: "cht_a",
  status: "idle" as const,
  timeline: [] as never,
  pendingQuestions: [] as never,
  pendingApprovals: [] as never,
  chatDetached: false,
  providerSwitchLoading: false,
  pendingProviderSwitch: undefined,
  supportedModels: [] as never,
});

function spySwitch(client: MockRunnerClient) {
  const calls: { chatId: string; input: { targetProviderKey: string; model?: string } }[] = [];
  const orig = client.switchChatProvider.bind(client);
  client.switchChatProvider = (async (chatId: string, input: never) => {
    calls.push({ chatId, input: input as { targetProviderKey: string; model?: string } });
    return orig(chatId, input);
  }) as typeof client.switchChatProvider;
  return calls;
}

test("cross-provider posture Tab routes to the switch endpoint with the pinned model", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  client.getChatPosture = (async () => ({
    active: "code",
    profiles: {
      plan: { provider: "grok", model: "grok-4.5" },
      code: { provider: "opencode", model: "opencode-model" },
    },
  })) as typeof client.getChatPosture;
  useStore.setState({ ...baseState(client) });
  useStore.setState({
    timeline: [{ kind: "prompt", id: "u1", text: "hi" }] as never,
  });

  await useStore.getState().setChatPosture("plan");

  assert.equal(calls.length, 1, "must call the switch endpoint once");
  assert.equal(calls[0].chatId, "cht_a");
  assert.equal(calls[0].input.targetProviderKey, "grok");
  assert.equal(calls[0].input.model, "grok-4.5", "must carry the pinned model, not the provider default");
  const s = useStore.getState();
  assert.equal(s.runId, "mock-run-cht_a-1", "must adopt the new leg");
  const dividers = s.timeline.filter((t) => String(t.id).startsWith("seed-divider-"));
  assert.equal(dividers.length, 1, "exactly one divider");
  assert.equal(s.timeline[0].kind, "prompt", "timeline must be kept, not reset");
});

test("same-provider posture Tab stays in place (no endpoint call)", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  client.getChatPosture = (async () => ({
    active: "code",
    profiles: { plan: { provider: "opencode", model: "opencode-model" }, code: {} },
  })) as typeof client.getChatPosture;
  useStore.setState({ ...baseState(client) });

  await useStore.getState().setChatPosture("plan");

  assert.equal(calls.length, 0);
  assert.equal(useStore.getState().selectedProvider, "opencode");
});

test("posture Tab without a live chat applies locally (no endpoint call)", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  client.getChatPosture = (async () => ({
    active: "code",
    profiles: { plan: { provider: "grok", model: "grok-4.5" }, code: {} },
  })) as typeof client.getChatPosture;
  useStore.setState({ ...baseState(client), chatId: undefined });

  await useStore.getState().setChatPosture("plan");

  assert.equal(calls.length, 0);
  assert.equal(useStore.getState().selectedProvider, "grok", "session pin still applies");
});

test("posture Tab on a detached chat applies locally (no endpoint call)", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  client.getChatPosture = (async () => ({
    active: "code",
    profiles: { plan: { provider: "grok", model: "grok-4.5" }, code: {} },
  })) as typeof client.getChatPosture;
  useStore.setState({ ...baseState(client), chatDetached: true });

  await useStore.getState().setChatPosture("plan");

  assert.equal(calls.length, 0);
  assert.equal(useStore.getState().selectedProvider, "grok", "session pin still applies");
});

test("posture Tab while a switch is in flight stays in place", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  client.getChatPosture = (async () => ({
    active: "code",
    profiles: { plan: { provider: "grok", model: "grok-4.5" }, code: {} },
  })) as typeof client.getChatPosture;
  useStore.setState({ ...baseState(client), providerSwitchLoading: true });

  await useStore.getState().setChatPosture("plan");

  assert.equal(calls.length, 0);
});

test("bare-model pin derives once, persists, and switches with the derived provider", async () => {
  const client = new MockRunnerClient();
  const calls = spySwitch(client);
  // Seed the runner-owned posture doc with a bare pin (no provider).
  await client.setChatPosture({
    active: "code",
    profiles: { plan: { model: "grok-4.5" }, code: {} },
  } as never);
  useStore.setState({ ...baseState(client) });

  await useStore.getState().setChatPosture("plan");

  const derives = () =>
    useStore.getState().timeline.filter((t) => String(t.id).startsWith("posture-derive-"));
  assert.equal(derives().length, 1, "derive notice must fire exactly once");
  assert.ok(String((derives()[0] as { text?: string }).text ?? "").includes('"grok"'));
  const saved = await client.getChatPosture();
  assert.equal(saved.profiles.plan?.provider, "grok", "derived provider must persist");
  assert.equal(calls.length, 1, "must switch with the derived provider");
  assert.equal(calls[0].input.targetProviderKey, "grok");

  // Second Tab: provider already pinned → no second notice, no second switch.
  // Fresh switch spy state: the mock rejects same-provider switches, and the
  // session is already grok so the route guard skips the endpoint anyway.
  await useStore.getState().setChatPosture("plan");
  assert.equal(derives().length, 1, "no second derive notice");
  assert.equal(calls.length, 1, "no second switch call");
});
